package session

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"sync/atomic"
	"time"
)

// ContainerFileBackend 是容器文件会话的最小命令通道（由 container 包的
// FileBackend 适配器实现；容器包已依赖 session 包，故在 session 侧定义接口）。
type ContainerFileBackend interface {
	// ExecRun 在容器内以 sh -c 执行脚本，返回 stdout；stderr 并入错误。
	ExecRun(ctx context.Context, script string) ([]byte, error)
	// ExecStdin 带全量标准输入执行（小内容写入用）。
	ExecStdin(ctx context.Context, script string, stdin []byte) ([]byte, error)
	// ExecStdinStream 流式标准输入（本机 tar 流直达容器，大文件不落内存）。
	ExecStdinStream(ctx context.Context, script string, stdin io.Reader) ([]byte, error)
	// CpRaw 打开容器路径的 tar 导出流（docker cp <cid>:<path> -）。
	CpRaw(ctx context.Context, containerPath string) (RawStream, error)
	// CpToFile 把容器路径复制到宿主机文件（nerdctl 等不支持 stdout 导出时的中转）。
	CpToFile(ctx context.Context, cid, remotePath, destPath string) error
	// HostRun 在宿主机执行命令（中转文件的 cat/rm 清理）。
	HostRun(ctx context.Context, argv []string) ([]byte, error)
	// HostRaw 打开宿主机命令的 stdout 二进制流。
	HostRaw(ctx context.Context, argv []string) (RawStream, error)
}

// RawStream 是二进制子流：读毕 EOF 后 Wait 回收，提前中止用 Close。
type RawStream interface {
	io.ReadCloser
	Wait() error
}

// ContainerFileSession 实现文件传输会话契约，"远端"是容器文件系统，通过
// docker exec（ls/rm/mv/mkdir）+ docker cp（tar 流导入导出）完成。与
// SCPSession 同一思路：一次性 shell 命令 + 客户端解析，无 SFTP 子系统。
//
// 限制：操作类（上传/删除/重命名）要求容器 running（exec 需要）；下载走
// docker cp，容器停止也可用。Windows 本地 Docker Desktop 同样支持——全部
// 走 docker CLI，不碰宿主文件系统。
type ContainerFileSession struct {
	baseSession
	localFSOps
	backend ContainerFileBackend
	cid     string
	cwd     string

	mu        sync.RWMutex
	transfers map[string]*TransferTask
	taskSeq   int64
}

var _ Session = (*ContainerFileSession)(nil)

func NewContainerFileSession(id string, backend ContainerFileBackend, cid, title string) *ContainerFileSession {
	return &ContainerFileSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "container-file",
			title:       title,
			status:      StatusDisconnected,
		},
		// localFSOps 必须显式初始化：零值的 localCwd 为空串，本地栏会报
		// "open : The system cannot find the file specified"。
		localFSOps: newLocalFSOps(),
		backend:    backend,
		cid:        cid,
		cwd:        "/",
		transfers:  make(map[string]*TransferTask),
	}
}

// Connect 探测容器可用（exec 一次 true），成功即标记连接成功；初始目录 "/"。
func (s *ContainerFileSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := s.backend.ExecRun(ctx, "true"); err != nil {
		s.setStatus(StatusError)
		msg := err.Error()
		if strings.Contains(msg, "executable file not found") || strings.Contains(msg, "no such file or directory") {
			return fmt.Errorf("容器内没有可用的 shell（k8s pause 等最小化镜像），无法浏览文件: %w", err)
		}
		if strings.Contains(msg, "is not running") {
			return fmt.Errorf("容器未运行，无法浏览文件: %w", err)
		}
		return fmt.Errorf("container is not browsable: %w", err)
	}
	s.setStatus(StatusConnected)
	// Seed the start path via cd + pwd -P (resolves ~ and relative paths
	// exactly like ChangeRemoteDir) and the local pane; failure keeps the
	// container default. Must run after the connected status so runCommand's
	// requireConnected guard passes.
	if dir := strings.TrimSpace(config.Cwd); dir != "" {
		if out, err := s.runCommand("cd "+shellEscape(dir)+" 2>/dev/null && pwd -P", 15*time.Second); err == nil {
			if real := strings.TrimSpace(out); real != "" {
				s.setCwd(real)
			}
		}
	}
	applyLocalStartDir(&s.localFSOps, config.LocalDir)
	return nil
}

func (s *ContainerFileSession) Write(data []byte) error { return nil }

func (s *ContainerFileSession) Resize(cols, rows int) error { return nil }

func (s *ContainerFileSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

func (s *ContainerFileSession) Disconnect() error {
	s.mu.Lock()
	for _, t := range s.transfers {
		if t.cancel != nil {
			t.cancel()
		}
	}
	s.mu.Unlock()
	s.setStatus(StatusDisconnected)
	return nil
}

// --- 命令执行 ---------------------------------------------------------------

// runCommand 执行一段容器内脚本，返回 stdout；带超时。
func (s *ContainerFileSession) runCommand(script string, timeout time.Duration) (string, error) {
	if err := s.requireConnected(); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out, err := s.backend.ExecRun(ctx, script)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func (s *ContainerFileSession) requireConnected() error {
	if !s.IsConnected() {
		return fmt.Errorf("container file session not connected")
	}
	return nil
}

// resolveRemote 把相对路径解析到会话 cwd；结果为干净的绝对路径。
func (s *ContainerFileSession) resolveRemote(p string) string {
	if p == "" {
		return path.Clean(s.getCwd())
	}
	if !strings.HasPrefix(p, "/") {
		p = path.Join(s.getCwd(), p)
	}
	return path.Clean(p)
}

func (s *ContainerFileSession) getCwd() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cwd
}

func (s *ContainerFileSession) setCwd(dir string) {
	s.mu.Lock()
	s.cwd = dir
	s.mu.Unlock()
}

func (s *ContainerFileSession) nextTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&s.taskSeq, 1))
}

// --- 浏览 -------------------------------------------------------------------

func (s *ContainerFileSession) ListRemote(dir string) (FileListResult, error) {
	if err := s.requireConnected(); err != nil {
		return FileListResult{}, err
	}
	d := s.resolveRemote(dir)
	out, err := s.runCommand("ls -la "+shellEscape(d), 25*time.Second)
	if err != nil {
		return FileListResult{}, err
	}
	parsed := parseLsLongListing(out)
	files := make([]FileItem, 0, len(parsed))

	// 软链目录单次往返探测（与 SCP 后端一致），保证软链目录可进入。
	var symlinks []string
	for _, e := range parsed {
		if strings.HasPrefix(e.Mode, "l") {
			symlinks = append(symlinks, path.Join(d, e.Name))
		}
	}
	symDirs := s.resolveSymlinkDirs(symlinks, 15*time.Second)

	for _, e := range parsed {
		isDir := strings.HasPrefix(e.Mode, "d")
		if strings.HasPrefix(e.Mode, "l") && symDirs[path.Join(d, e.Name)] {
			isDir = true
		}
		files = append(files, FileItem{
			Name:     e.Name,
			Size:     e.Size,
			ModTime:  e.ModTime.Format(time.RFC3339),
			Mode:     e.Mode,
			IsDir:    isDir,
			IsHidden: strings.HasPrefix(e.Name, "."),
			Owner:    e.Owner,
			Group:    e.Group,
		})
	}
	return FileListResult{Files: files, Dir: d}, nil
}

// ChangeRemoteDir 进入目录并解析物理路径（cd + pwd -P，软链落到真实目标）。
func (s *ContainerFileSession) ChangeRemoteDir(dir string) (FileListResult, error) {
	if err := s.requireConnected(); err != nil {
		return FileListResult{}, err
	}
	target := dir
	if !path.IsAbs(dir) {
		target = path.Join(s.getCwd(), dir)
	}
	out, err := s.runCommand("cd "+shellEscape(target)+" 2>/dev/null && pwd -P", 15*time.Second)
	if err != nil || lastNonEmptyLine(out) == "" {
		return FileListResult{}, fmt.Errorf("no such directory: %s", target)
	}
	real := lastNonEmptyLine(out)
	s.setCwd(real)
	return s.ListRemote(real)
}

// resolveSymlinkDirs 单次往返探测一批软链哪些指向目录。
func (s *ContainerFileSession) resolveSymlinkDirs(paths []string, timeout time.Duration) map[string]bool {
	out := make(map[string]bool)
	if len(paths) == 0 {
		return out
	}
	var b strings.Builder
	b.WriteString("n=0; for p in")
	for _, p := range paths {
		b.WriteString(" " + shellEscape(p))
	}
	b.WriteString("; do n=$((n+1)); if [ -d \"$p\" ]; then echo \"$n d\"; else echo \"$n f\"; fi; done")
	res, err := s.runCommand(b.String(), timeout)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(res, "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		idx, err := strconv.Atoi(f[0])
		if err != nil || idx < 1 || idx > len(paths) {
			continue
		}
		if f[1] == "d" {
			out[paths[idx-1]] = true
		}
	}
	return out
}

// --- 远端元数据操作 ----------------------------------------------------------

func (s *ContainerFileSession) MakeDir(dir string) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	p := s.resolveRemote(dir)
	_, err := s.runCommand("mkdir -p "+shellEscape(p), 15*time.Second)
	return err
}

func (s *ContainerFileSession) Symlink(target, linkPath string) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	p := s.resolveRemote(linkPath)
	_, err := s.runCommand("ln -s "+shellEscape(target)+" "+shellEscape(p), 15*time.Second)
	return err
}

func (s *ContainerFileSession) Remove(p string, recursive bool) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	c := s.resolveRemote(p)
	if c == "/" || c == "." || c == "" {
		return fmt.Errorf("refusing to delete path: %s", c)
	}
	if recursive {
		_, err := s.runCommand("rm -rf -- "+shellEscape(c), 5*time.Minute)
		return err
	}
	_, err := s.runCommand("rm -f -- "+shellEscape(c), 5*time.Minute)
	return err
}

func (s *ContainerFileSession) Rename(oldName, newName string) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	o := s.resolveRemote(oldName)
	n := s.resolveRemote(newName)
	_, err := s.runCommand("mv -- "+shellEscape(o)+" "+shellEscape(n), 5*time.Minute)
	return err
}

func (s *ContainerFileSession) Chmod(p string, mode os.FileMode) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	path2 := s.resolveRemote(p)
	_, err := s.runCommand(fmt.Sprintf("chmod %o %s", mode.Perm(), shellEscape(path2)), 15*time.Second)
	return err
}

// Copy 在容器内复制（服务端 cp，数据不过本机）。
func (s *ContainerFileSession) Copy(oldPath, newPath string) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	o := s.resolveRemote(oldPath)
	n := s.resolveRemote(newPath)
	_, err := s.runCommand("cp -r -- "+shellEscape(o)+" "+shellEscape(n), 5*time.Minute)
	return err
}

func (s *ContainerFileSession) Move(oldPath, newPath string) error {
	return s.Rename(oldPath, newPath)
}

// 本文件实现 ContainerFileSession 的内容读写与传输（Get/Put/GetContent/
// PutContent + tar 流助手），与 container_file_session.go 同包分工。

// --- 内容读写 ----------------------------------------------------------------

// GetContent 读取文件内容：cp 导出 tar 流取首个普通文件。
func (s *ContainerFileSession) GetContent(remotePath string) ([]byte, error) {
	if err := s.requireConnected(); err != nil {
		return nil, err
	}
	rp := s.resolveRemote(remotePath)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	// 通道 1：cp 导出 tar 流到 stdout（docker/podman 支持）
	rs, err := s.backend.CpRaw(ctx, rp)
	if err == nil {
		type outcome struct {
			b   []byte
			err error
		}
		ch := make(chan outcome, 1)
		go func() {
			b, err := readTarSingleFile(rs)
			ch <- outcome{b: b, err: err}
		}()
		select {
		case out := <-ch:
			rs.Close()
			if out.err == nil {
				return out.b, nil
			}
			// cp stdout 不可用（如 nerdctl 未实现），落到中转方案
		case <-time.After(40 * time.Second):
			rs.Close()
			return nil, fmt.Errorf("read container file content timeout")
		}
	} else {
		rs.Close()
	}
	// 通道 2：cp 到宿主机临时文件 + cat 回传（nerdctl 等的中转方案）
	b, err := s.readFileViaHostTemp(ctx, rp)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// readFileViaHostTemp 把容器文件 cp 到宿主机临时文件后经 cat 流式读取，
// 读毕清理。nerdctl 等未实现 cp 到 stdout 的运行时走这条中转。
func (s *ContainerFileSession) readFileViaHostTemp(ctx context.Context, rp string) ([]byte, error) {
	tmp := fmt.Sprintf("/tmp/ubt-cp-%d", time.Now().UnixNano())
	if err := s.backend.CpToFile(ctx, s.cid, rp, tmp); err != nil {
		return nil, fmt.Errorf("container cp to temp file failed: %w", err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s.backend.HostRun(cctx, []string{"rm", "-f", tmp})
	}()
	type outcome struct {
		b   []byte
		err error
	}
	ch := make(chan outcome, 1)
	go func() {
		rs, err := s.backend.HostRaw(ctx, []string{"cat", tmp})
		if err != nil {
			ch <- outcome{err: err}
			return
		}
		defer rs.Close()
		b, err := io.ReadAll(rs)
		ch <- outcome{b: b, err: err}
	}()
	select {
	case out := <-ch:
		return out.b, out.err
	case <-time.After(50 * time.Second):
		return nil, fmt.Errorf("read container file via temp file timeout")
	}
}

// PutContent 写文件内容：tar 单文件从 stdin 导入（容器需 running）。
func (s *ContainerFileSession) PutContent(remotePath string, content []byte) error {
	if err := s.requireConnected(); err != nil {
		return err
	}
	rp := s.resolveRemote(remotePath)
	if _, err := s.runCommand("mkdir -p "+shellEscape(path.Dir(rp)), 15*time.Second); err != nil {
		return err
	}
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	if err := writeTarFile(tw, path.Base(rp), content, 0o644, time.Now()); err != nil {
		return err
	}
	tw.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := s.backend.ExecStdin(ctx, "tar xf - -C "+shellEscape(path.Dir(rp)), buf.Bytes())
	return err
}

// --- Get（下载：docker cp tar 流 → 本机解包）---------------------------------

func (s *ContainerFileSession) Get(remotePath, localPath string, recursive bool) (string, error) {
	if err := s.requireConnected(); err != nil {
		return "", err
	}
	rp := s.resolveRemote(remotePath)
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	task := &TransferTask{
		ID:         s.nextTaskID("dl"),
		Type:       "download",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "pending",
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer task.done()
		removeTask := func() {
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}
		task.Status = "running"
		s.emitTransferStart(task)

		ctx, cancel := context.WithCancel(task.ctx)
		defer cancel()
		rs, err := s.backend.CpRaw(ctx, rp)
		if err != nil {
			s.emitTransferEvent(task, err)
			return
		}
		defer rs.Close()

		werr := s.extractTarInto(rs, rp, lp, task)
		if werr != nil && isCpUnavailable(werr) {
			// cp stdout 未实现（nerdctl 等）：cp 到宿主机临时文件中转
			if terr := s.fetchViaHostTempInto(ctx, rp, lp, task); terr != nil {
				werr = terr
			} else {
				werr = nil
			}
		}
		if task.ctx.Err() != nil {
			task.Status = "cancelled"
			s.emitTransferComplete(task)
			removeTask()
			return
		}
		if werr != nil {
			task.Status = "error"
			s.emitTransferEvent(task, werr)
			return // 保留在 transfers 里供重试
		}
		task.Status = "done"
		s.emitTransferProgressForced(task)
		s.emitTransferComplete(task)
		removeTask()
	}()
	return task.ID, nil
}

// isCpUnavailable 判断是否为 cp stdout 导出未实现类失败（此时可用临时
// 文件中转重试）；路径不存在等真实错误不重试。
func isCpUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "not implemented yet") || strings.Contains(msg, "no data returned")
}

// fetchViaHostTempInto 把容器路径 cp 到宿主机临时文件，cat 流回本机解包，
// 读毕清理临时文件。
func (s *ContainerFileSession) fetchViaHostTempInto(ctx context.Context, rp, lp string, task *TransferTask) error {
	tmp := fmt.Sprintf("/tmp/ubt-cp-%d", time.Now().UnixNano())
	if err := s.backend.CpToFile(ctx, s.cid, rp, tmp); err != nil {
		return fmt.Errorf("container cp to temp file failed: %w", err)
	}
	defer func() {
		cctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = s.backend.HostRun(cctx, []string{"rm", "-f", tmp})
	}()
	rs, err := s.backend.HostRaw(ctx, []string{"cat", tmp})
	if err != nil {
		return err
	}
	defer rs.Close()
	// 中转文件的 tar 与 cp stdout 同构，解包逻辑复用
	return s.extractTarInto(rs, rp, lp, task)
}

// extractTarInto 把容器路径的 tar 流解到本机。tar 根 entry 名是路径 basename：
// 文件语义直接落盘为 localPath；目录语义（递归）根目录映射到 localPath 本身，
// 子项落到其下。流零产出即 EOF 时，用 Wait 的退出错误解释真实原因。
func (s *ContainerFileSession) extractTarInto(rs RawStream, rp, lp string, task *TransferTask) error {
	base := path.Base(rp)
	tr := tar.NewReader(rs)
	var rootHandled bool
	entries := 0
	for {
		if task.ctx.Err() != nil {
			return task.ctx.Err()
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			if entries == 0 {
				if werr := rs.Wait(); werr != nil {
					return fmt.Errorf("container cp failed: %w", werr)
				}
				return fmt.Errorf("container cp: no data returned")
			}
			break
		}
		if err != nil {
			return err
		}
		entries++
		name := strings.TrimSuffix(hdr.Name, "/")
		rel := strings.TrimPrefix(strings.TrimPrefix(name, base), "/")
		if !rootHandled {
			rootHandled = true
			if rel == "" {
				// 根 entry：文件落盘为 lp；目录建 lp 目录后继续
				if hdr.Typeflag == tar.TypeDir {
					if mkErr := os.MkdirAll(lp, 0o755); mkErr != nil {
						return mkErr
					}
					continue
				}
				return s.extractTarFile(tr, hdr, lp, task, "")
			}
		}
		if rel == "" {
			continue
		}
		local := filepath.Join(lp, filepath.FromSlash(rel))
		if err := s.extractTarFile(tr, hdr, local, task, rel); err != nil {
			return err
		}
	}
	return nil
}

// extractTarFile 把一个 tar entry 落盘；进度按字节累加并上报。
func (s *ContainerFileSession) extractTarFile(tr *tar.Reader, hdr *tar.Header, local string, task *TransferTask, rel string) error {
	switch hdr.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(local, 0o755)
	case tar.TypeSymlink, tar.TypeLink:
		_ = os.Remove(local)
		_ = os.Symlink(hdr.Linkname, local) // Windows 无特权时尽力而为
		return nil
	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(local, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(hdr.Mode)&0o777)
		if err != nil {
			return err
		}
		defer f.Close()
		buf := make([]byte, 64*1024)
		for {
			task.waitIfPaused()
			if task.ctx.Err() != nil {
				return task.ctx.Err()
			}
			n, rerr := tr.Read(buf)
			if n > 0 {
				if _, werr := f.Write(buf[:n]); werr != nil {
					return werr
				}
				task.addProgress(int64(n))
				s.emitTransferProgress(task)
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				return rerr
			}
		}
		return f.Close()
	}
	return nil
}

// readTarSingleFile 从 tar 流取首个普通文件的内容（GetContent 用）。
// 流提前结束（docker cp 失败）时用 Wait 的退出错误解释原因。
func readTarSingleFile(rs RawStream) ([]byte, error) {
	tr := tar.NewReader(rs)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			if werr := rs.Wait(); werr != nil {
				return nil, fmt.Errorf("container cp failed: %w", werr)
			}
			return nil, fmt.Errorf("container cp: no data returned")
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeReg {
			// 注意：只消费 entry 自身，不能等流结束——tar.Reader 不会读
			// trailing 数据，等待会造成 ExecRaw 写侧死锁（编辑器内容为空）。
			// 流的清理由调用方 defer rs.Close() 负责。
			return io.ReadAll(tr)
		}
	}
}

// --- Put（上传：本机 tar 流 → 容器内解包）------------------------------------

func (s *ContainerFileSession) Put(localPath, remotePath string, recursive bool) (string, error) {
	if err := s.requireConnected(); err != nil {
		return "", err
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	rp := s.resolveRemote(remotePath)
	task := &TransferTask{
		ID:         s.nextTaskID("ul"),
		Type:       "upload",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "pending",
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer task.done()
		removeTask := func() {
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}
		task.Status = "running"
		s.emitTransferStart(task)

		// 目标存在为目录 → 解到该目录下；否则解到父目录（保留目标名）。
		dest := rp
		if s.remoteIsDir(rp) {
			dest = path.Join(rp, filepath.Base(lp))
		}
		destDir := path.Dir(dest)
		if _, err := s.runCommand("mkdir -p "+shellEscape(destDir), 15*time.Second); err != nil {
			s.emitTransferEvent(task, err)
			return
		}
		total, terr := localTreeStats(lp)
		if terr == nil {
			task.setTotal(total)
		}

		pr, pw := io.Pipe()
		tw := tar.NewWriter(pw)
		go func() {
			werr := s.tarWalkInto(tw, lp, path.Base(dest), task)
			tw.Close()
			pw.CloseWithError(werr)
		}()
		ctx, cancel := context.WithCancel(task.ctx)
		defer cancel()
		_, err := s.backend.ExecStdinStream(ctx, "tar xf - -C "+shellEscape(destDir), pr)
		if err != nil && task.ctx.Err() == nil {
			s.emitTransferEvent(task, err)
			return
		}
		if task.ctx.Err() != nil {
			task.Status = "cancelled"
			s.emitTransferComplete(task)
			removeTask()
			return
		}
		task.Status = "done"
		s.emitTransferProgressForced(task)
		s.emitTransferComplete(task)
		removeTask()
	}()
	return task.ID, nil
}

// remoteIsDir 探测容器内路径是否为目录。
func (s *ContainerFileSession) remoteIsDir(p string) bool {
	out, err := s.runCommand("test -d "+shellEscape(p)+" && echo y", 15*time.Second)
	return err == nil && strings.TrimSpace(out) == "y"
}

// --- tar 写入助手 ------------------------------------------------------------

// writeTarFile 把内存内容写成单文件 tar entry。
func writeTarFile(tw *tar.Writer, name string, content []byte, mode int64, mod time.Time) error {
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeReg,
		Mode:     mode,
		Size:     int64(len(content)),
		ModTime:  mod,
	}); err != nil {
		return err
	}
	_, err := tw.Write(content)
	return err
}

// tarWalkInto 把本地文件/目录树写入 tar writer：目录树 entry 名为
// "<base>/<rel>"，对齐 docker cp 导出的命名，容器内解包后路径一致。
func (s *ContainerFileSession) tarWalkInto(tw *tar.Writer, local, base string, task *TransferTask) error {
	fi, err := os.Stat(local)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return s.tarWalkFile(tw, local, base, task)
	}
	return filepath.WalkDir(local, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(local, p)
		if err != nil {
			return err
		}
		name := base
		if rel != "." {
			name = base + "/" + filepath.ToSlash(rel)
		}
		if d.IsDir() {
			info, ierr := d.Info()
			if ierr != nil {
				return ierr
			}
			return tw.WriteHeader(&tar.Header{
				Name:     name + "/",
				Typeflag: tar.TypeDir,
				Mode:     0o755,
				ModTime:  info.ModTime(),
			})
		}
		return s.tarWalkFile(tw, p, name, task)
	})
}

// extractTarInto 的 werr 清理

// tarWalkFile 把单个本地文件写进 tar writer，边读边计数上报进度。
// task 可为 nil。
func (s *ContainerFileSession) tarWalkFile(tw *tar.Writer, local, name string, task *TransferTask) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     name,
		Typeflag: tar.TypeReg,
		Mode:     int64(fi.Mode().Perm()),
		Size:     fi.Size(),
		ModTime:  fi.ModTime(),
	}); err != nil {
		return err
	}
	if task != nil {
		task.beginFile(name)
		s.emitFileStart(task, name, name)
	}
	buf := make([]byte, 64*1024)
	for {
		task.waitIfPaused()
		if task.ctx.Err() != nil {
			return task.ctx.Err()
		}
		n, rerr := f.Read(buf)
		if n > 0 {
			if _, werr := tw.Write(buf[:n]); werr != nil {
				return werr
			}
			task.addProgress(int64(n))
			s.emitTransferProgress(task)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if task != nil {
		task.finishFile(name)
		s.emitFileDone(task, name)
	}
	return tw.Flush()
}

// localTreeStats 统计本地文件/目录树字节数（上传 Total 用）。
func localTreeStats(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		return fi.Size(), nil
	}
	var total int64
	err = filepath.WalkDir(p, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, err
}

// --- 任务簿记 ----------------------------------------------------------------

// RetryTransfer (re)starts a transfer from a frontend-held checkpoint.
func (s *ContainerFileSession) RetryTransfer(spec TransferSpec, skipCompleted []string) (string, error) {
	if err := s.requireConnected(); err != nil {
		return "", err
	}
	if spec.Recursive {
		if spec.Type == "download" {
			return s.Get(spec.RemotePath, spec.LocalPath, true)
		}
		return s.Put(spec.LocalPath, spec.RemotePath, true)
	}
	if spec.Type == "download" {
		return s.Get(spec.RemotePath, spec.LocalPath, false)
	}
	return s.Put(spec.LocalPath, spec.RemotePath, false)
}

// DismissTransfer 丢弃一个失败的（保留在 transfers 里的）任务。
func (s *ContainerFileSession) DismissTransfer(taskID string) error {
	s.mu.Lock()
	delete(s.transfers, taskID)
	s.mu.Unlock()
	return nil
}

func (s *ContainerFileSession) CancelTransfer(taskID string) error {
	s.mu.Lock()
	t, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

func (s *ContainerFileSession) PauseTransfer(taskID string) error {
	s.mu.Lock()
	t, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	t.setPaused(true)
	t.Status = "paused"
	s.emitTransferPaused(t)
	return nil
}

// ResumeTransfer 仅当任务处于 paused 时有效。
func (s *ContainerFileSession) ResumeTransfer(taskID string) error {
	s.mu.Lock()
	t, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if t.Status != "paused" {
		return fmt.Errorf("task not active: %s", taskID)
	}
	t.setPaused(false)
	t.Status = "running"
	close(t.pauseCh)
	t.pauseCh = make(chan struct{})
	s.emitTransferResumed(t)
	return nil
}
