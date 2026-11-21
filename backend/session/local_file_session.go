package session

// LocalFileSession backs the "local file" connection type: a dual-pane local
// file browser (local <-> local). The "remote" pane is a second view of the
// local filesystem with its own working directory, so every remote operation
// is a plain os call -- no network, no credentials -- and cross-pane transfers
// are local copies reported through the same transfer events the frontend
// TransferPanel already consumes (same machinery as WSLFileSession, whose
// copy helpers this type shares with WSLFileSession).
//
// Remote paths are kept in forward-slash form ("C:/Users/x" on Windows,
// "/home/x" on Unix) so the frontend breadcrumb and joinPath -- which split
// on "/" -- work unchanged. Windows os calls accept forward slashes. The
// remote root "/" lists the drives (Windows) as directory entries named
// "C:" / "D:" / ... so navigation switches drives through ordinary entries.

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LocalFileSession struct {
	baseSession
	localFSOps            // local pane (embedded; shares localCwd)
	remote     localFSOps // "remote" pane -- a second view with its own cwd

	mu        sync.RWMutex
	transfers map[string]*TransferTask
	taskSeq   int64
}

var _ Session = (*LocalFileSession)(nil)

// drivePrefixRe matches Windows drive-prefixed paths ("C:/..."), which must
// bypass path.Clean/Join's POSIX assumptions in resolveRemote.
var drivePrefixRe = regexp.MustCompile(`^[A-Za-z]:`)

// localFileHasDrives marks platforms where the "/" root is a synthetic drive
// picker rather than a listable directory (Windows).
var localFileHasDrives = runtime.GOOS == "windows"

func NewLocalFileSession(id string) *LocalFileSession {
	return &LocalFileSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "local-file",
			status:      StatusDisconnected,
		},
		localFSOps: newLocalFSOps(),
		remote:     newLocalFSOps(),
		transfers:  make(map[string]*TransferTask),
	}
}

// Connect resolves the start paths (config.Cwd for both panes when it is an
// existing directory, home otherwise) and marks the session connected.
func (s *LocalFileSession) Connect(config ConnectionConfig) error {
	start := filepath.ToSlash(strings.TrimSpace(config.Cwd))
	if start != "" && !isLocalFileDir(start) {
		start = ""
	}
	if start == "" {
		home, _ := os.UserHomeDir()
		start = filepath.ToSlash(home)
	}
	s.localCwd = filepath.FromSlash(start)
	s.remote.localCwd = start
	s.title = config.Name
	if s.title == "" {
		s.title = "Local Files"
	}
	s.setStatus(StatusConnected)
	return nil
}

// isLocalFileDir reports whether p (forward- or native-slash form) is an
// existing directory.
func isLocalFileDir(p string) bool {
	fi, err := os.Stat(filepath.FromSlash(p))
	return err == nil && fi.IsDir()
}

// --- remote path handling ------------------------------------------------------

// resolveRemote maps a display path to canonical forward-slash form:
// drive-prefixed paths stay as-is ("C:" gains a trailing slash -- path.Clean
// would strip it), leading-slash paths are Unix roots (real root on non-
// Windows), and relative paths resolve against the remote cwd. A drive name
// arriving joined under the root ("/C:") is recognized as the drive itself.
func (s *LocalFileSession) resolveRemote(p string) string {
	p = strings.ReplaceAll(strings.TrimSpace(p), "\\", "/")
	if p == "" {
		return path.Clean(s.remote.localCwd)
	}
	if strings.HasPrefix(p, "/") && drivePrefixRe.MatchString(p[1:]) {
		p = p[1:]
	}
	if drivePrefixRe.MatchString(p) {
		if strings.HasSuffix(p, ":") {
			p += "/"
		}
		return p
	}
	if strings.HasPrefix(p, "/") {
		return path.Clean(p)
	}
	return path.Join(path.Clean(s.remote.localCwd), p)
}

// remoteOSPath converts a forward-slash display path to the native os path.
// Windows os calls accept forward slashes, so this is a plain FromSlash.
func (s *LocalFileSession) remoteOSPath(p string) string {
	return filepath.FromSlash(p)
}

// resolveLocal resolves a local-pane path against the local cwd.
func (s *LocalFileSession) resolveLocal(p string) string {
	if p == "" {
		return filepath.Clean(s.localCwd)
	}
	if !filepath.IsAbs(p) {
		return filepath.Join(s.localCwd, p)
	}
	return filepath.Clean(p)
}

// fileItemsFromDir builds FileItem entries from an os.ReadDir result. Shared
// with WSLFileSession (Windows-only), which is why it lives here: the local
// file session compiles on every platform.
func fileItemsFromDir(dir string, entries []os.DirEntry) []FileItem {
	files := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		fi, err := e.Info()
		var mode os.FileMode
		var modTime time.Time
		var size int64
		if err == nil {
			mode = fi.Mode()
			modTime = fi.ModTime()
			size = fi.Size()
		}
		isDir := e.IsDir()
		full := joinEntry(dir, e.Name())
		if err == nil && mode&os.ModeSymlink != 0 {
			if target, terr := os.Stat(full); terr == nil && target.IsDir() {
				isDir = true
			}
		}
		isHidden := e.Name() != "" && e.Name()[0] == '.'
		if !isHidden {
			isHidden = isPathHidden(full)
		}
		files = append(files, FileItem{
			Name:     e.Name(),
			Size:     size,
			ModTime:  modTime.Format(time.RFC3339),
			Mode:     mode.String(),
			IsDir:    isDir,
			IsHidden: isHidden,
			Owner:    "",
		})
	}
	return files
}

// --- listing -------------------------------------------------------------------

// ListRemote lists a "remote" pane directory. "/" is special on Windows: it
// renders the drive entries ("C:" ...) so navigation can switch drives;
// on non-Windows it lists the real filesystem root.
func (s *LocalFileSession) ListRemote(dir string) (FileListResult, error) {
	d := s.resolveRemote(dir)
	if d == "/" && localFileHasDrives {
		return s.listRemoteDrives(), nil
	}
	entries, err := os.ReadDir(s.remoteOSPath(d))
	if err != nil {
		return FileListResult{}, err
	}
	files := fileItemsFromDir(s.remoteOSPath(d), entries)
	return FileListResult{Files: files, Dir: d}, nil
}

// listRemoteDrives synthesizes the remote root listing: each existing drive
// renders as a directory entry named "C:" so entering it navigates to the
// drive root ("C:/").
func (s *LocalFileSession) listRemoteDrives() FileListResult {
	files := make([]FileItem, 0, 8)
	if drives, err := s.localFSOps.ListLocalDrives(); err == nil {
		for _, d := range drives {
			letter := strings.TrimSuffix(d.Name, `:\`)
			files = append(files, FileItem{
				Name:    letter + ":",
				ModTime: d.ModTime,
				Mode:    "drwxr-xr-x",
				IsDir:   true,
			})
		}
	}
	return FileListResult{Files: files, Dir: "/"}
}

// --- remote attribute / dir ops -------------------------------------------------

// ChangeRemoteDir validates dir and makes it the remote pane's cwd.
func (s *LocalFileSession) ChangeRemoteDir(dir string) (FileListResult, error) {
	target := s.resolveRemote(dir)
	fi, err := os.Stat(s.remoteOSPath(target))
	if err != nil {
		return FileListResult{}, fmt.Errorf("no such directory: %s", target)
	}
	if !fi.IsDir() {
		return FileListResult{}, fmt.Errorf("not a directory: %s", target)
	}
	s.remote.localCwd = target
	return s.ListRemote(target)
}

func (s *LocalFileSession) MakeDir(dir string) error {
	return os.MkdirAll(s.remoteOSPath(s.resolveRemote(dir)), 0o755)
}

// Symlink creates a symbolic link on the remote side. Works where the OS
// allows it (Unix, Windows with developer mode); the frontend hides the entry
// for this type (supportsRemoteSymlink), so failures only surface for direct
// API use.
func (s *LocalFileSession) Symlink(target, linkPath string) error {
	return os.Symlink(target, s.remoteOSPath(s.resolveRemote(linkPath)))
}

func (s *LocalFileSession) Remove(p string, recursive bool) error {
	target := s.remoteOSPath(s.resolveRemote(p))
	if target == "" || target == "/" || filepath.VolumeName(target) == target {
		return fmt.Errorf("refusing to delete path: %s", p)
	}
	if recursive {
		return os.RemoveAll(target)
	}
	fi, err := os.Stat(target)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		entries, err := os.ReadDir(target)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory not empty (%d items)", len(entries))
		}
	}
	return os.Remove(target)
}

func (s *LocalFileSession) Rename(oldName, newName string) error {
	return os.Rename(s.remoteOSPath(s.resolveRemote(oldName)), s.remoteOSPath(s.resolveRemote(newName)))
}

// Chmod maps to os.Chmod. Meaningful on Unix; on Windows only the read-only
// bit reacts, which matches what the local filesystem can express.
func (s *LocalFileSession) Chmod(p string, mode os.FileMode) error {
	return os.Chmod(s.remoteOSPath(s.resolveRemote(p)), mode)
}

// --- content read/write ---------------------------------------------------------

func (s *LocalFileSession) GetContent(remotePath string) ([]byte, error) {
	return os.ReadFile(s.remoteOSPath(s.resolveRemote(remotePath)))
}

func (s *LocalFileSession) PutContent(remotePath string, content []byte) error {
	p := s.remoteOSPath(s.resolveRemote(remotePath))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, content, 0o644)
}

func (s *LocalFileSession) Copy(oldPath, newPath string) error {
	return copyPath(s.remoteOSPath(s.resolveRemote(oldPath)), s.remoteOSPath(s.resolveRemote(newPath)), nil)
}

func (s *LocalFileSession) Move(oldPath, newPath string) error {
	old := s.remoteOSPath(s.resolveRemote(oldPath))
	new := s.remoteOSPath(s.resolveRemote(newPath))
	if err := os.Rename(old, new); err == nil {
		return nil
	}
	// Cross-device or otherwise un-renameable: fall back to copy+delete,
	// mirroring the WSL session's behavior.
	if err := copyPath(old, new, nil); err != nil {
		return err
	}
	return os.RemoveAll(old)
}

// --- transfers ------------------------------------------------------------------

// Get copies from the remote pane's filesystem to the local pane's (both are
// local views). Returns the task id; progress flows through the usual
// transfer events so the frontend TransferPanel stays in sync.
func (s *LocalFileSession) Get(remotePath, localPath string, recursive bool) (string, error) {
	return s.startLocalTransfer("download", s.resolveLocal(localPath), s.resolveRemote(remotePath))
}

// Put copies from the local pane to the remote pane's filesystem.
func (s *LocalFileSession) Put(localPath, remotePath string, recursive bool) (string, error) {
	return s.startLocalTransfer("upload", s.resolveLocal(localPath), s.resolveRemote(remotePath))
}

// startLocalTransfer copies a file or tree between the two local views. A
// Task is reported through the usual transfer events so the frontend
// TransferPanel shows progress, pause, cancel and conflicts.
func (s *LocalFileSession) startLocalTransfer(tfType, local, remote string) (string, error) {
	src := s.remoteOSPath(remote)
	dst := local
	if tfType == "upload" {
		src = local
		dst = s.remoteOSPath(remote)
	}
	total, err := dirSize(src)
	if err != nil {
		return "", err
	}
	prefix := "ul"
	if tfType == "download" {
		prefix = "dl"
	}
	task := &TransferTask{
		ID:         s.nextTaskID(prefix),
		Type:       tfType,
		LocalPath:  local,
		RemotePath: remote,
		Total:      total,
		Status:     "running",
	}
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
	s.emitTransferStart(task)
	go func() {
		defer func() {
			task.done()
			if task.Status == "error" {
				// Retained in s.transfers for retry; the frontend drops it
				// via DismissTransfer.
				return
			}
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}()
		if err := copyPath(src, dst, task); err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.setProgress(task.loadTotal())
		task.Status = "done"
		s.emitTransferProgress(task)
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

// RetryTransfer (re)starts a transfer from a frontend-held checkpoint.
// resolveLocal/resolveRemote are idempotent on the resolved paths the spec
// carries, so the retry goes through Get/Put like the original request.
// skipCompleted is ignored -- the transfer restarts from scratch.
func (s *LocalFileSession) RetryTransfer(spec TransferSpec, skipCompleted []string) (string, error) {
	if spec.Type == "download" {
		return s.Get(spec.RemotePath, spec.LocalPath, spec.Recursive)
	}
	return s.Put(spec.LocalPath, spec.RemotePath, spec.Recursive)
}

func (s *LocalFileSession) DismissTransfer(taskID string) error {
	s.mu.Lock()
	delete(s.transfers, taskID)
	s.mu.Unlock()
	return nil
}

func (s *LocalFileSession) CancelTransfer(taskID string) error {
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

func (s *LocalFileSession) PauseTransfer(taskID string) error {
	s.mu.Lock()
	t, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	t.setPaused(true)
	t.Status = "paused"
	s.emitTransferComplete(t)
	return nil
}

func (s *LocalFileSession) ResumeTransfer(taskID string) error {
	s.mu.Lock()
	t, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	t.setPaused(false)
	t.Status = "running"
	close(t.pauseCh)
	t.pauseCh = make(chan struct{})
	s.emitTransferStart(t)
	return nil
}

func (s *LocalFileSession) nextTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&s.taskSeq, 1))
}

// --- Session interface extras ----------------------------------------------------

// Write is a no-op: file sessions carry no terminal stream.
func (s *LocalFileSession) Write(_ []byte) error { return nil }

// Resize is a no-op: file sessions have no PTY to size.
func (s *LocalFileSession) Resize(_, _ int) error { return nil }

// IsConnected reports the session status. A local-file session is connected
// from Connect until Disconnect; there is no transport to drop.
func (s *LocalFileSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

// Disconnect is a no-op (no transport); it only flips the status back.
func (s *LocalFileSession) Disconnect() error {
	s.setStatus(StatusDisconnected)
	return nil
}

// --- shared local copy helpers ---------------------------------------------------
//
// These back LocalFileSession's cross-pane transfers AND WSLFileSession's
// Windows-pane <-> wsl.localhost copies (same package; one home for both).

// joinEntry appends name to a directory. UNC roots stay forward-slashed (see
// the WSLFileSession type doc); ordinary local paths use filepath.Join.
func joinEntry(dir, name string) string {
	if strings.HasPrefix(dir, `//`) || strings.HasPrefix(dir, `\`) {
		return dir + "/" + name
	}
	return filepath.Join(dir, name)
}

// dirSize sums the bytes of a file or all files under a directory tree.
func dirSize(p string) (int64, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		return fi.Size(), nil
	}
	var total int64
	var walk func(string) error
	walk = func(dir string) error {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if ei, err := e.Info(); err == nil && !ei.IsDir() {
				total += ei.Size()
				continue
			}
			full := joinEntry(dir, e.Name())
			if fi2, err := os.Stat(full); err == nil && fi2.IsDir() {
				if err := walk(full); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return total, walk(p)
}

// copyPath copies a file or directory tree. task, if non-nil, receives byte
// progress updates and pauses/cancels via transferTask.
func copyPath(src, dst string, task *TransferTask) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(joinEntry(src, e.Name()), joinEntry(dst, e.Name()), task); err != nil {
				return err
			}
		}
		return nil
	}
	return copyFile(src, dst, task)
}

func copyFile(src, dst string, task *TransferTask) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if task != nil {
		task.waitIfPaused() // blocks while paused / returns on cancel
	}
	buf := make([]byte, 64*1024)
	for {
		if task != nil {
			task.waitIfPaused() // blocks while paused / returns on cancel
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return werr
			}
			if task != nil {
				task.addProgress(int64(n))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	return out.Close()
}
