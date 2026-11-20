package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudsoda/go-smb2"
)

type SMBSession struct {
	baseSession
	localFSOps
	conn      *smb2.Session
	share     *smb2.Share
	cwd       string
	mu        sync.RWMutex
	transfers map[string]*TransferTask
	taskSeq   int64
}

func NewSMBSession(id string) *SMBSession {
	return &SMBSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "smb",
			status:      StatusDisconnected,
		},
		localFSOps: newLocalFSOps(),
		cwd:        "/",
		transfers:  make(map[string]*TransferTask),
	}
}

func (s *SMBSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	s.title = fmt.Sprintf("%s@%s", config.User, config.Host)

	port := config.Port
	if port <= 0 {
		port = 445
	}
	addr := net.JoinHostPort(config.Host, strconv.Itoa(port))

	conn, err := dialFirstHop(addr, config.Proxy)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("smb dial: %w", err)
	}

	d := &smb2.Dialer{
		Initiator: &smb2.NTLMInitiator{
			User:     config.User,
			Password: config.Password,
			Domain:   config.SmbDomain,
		},
	}

	smbConn, err := d.DialConn(context.Background(), conn, addr)
	if err != nil {
		conn.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("smb handshake: %w", err)
	}

	// Always start at share list root. If a share was specified, the frontend
	// will auto-navigate into it after the initial listing.
	s.conn = smbConn
	s.cwd = "/"
	s.setStatus(StatusConnected)
	go s.startKeepAlive()
	return nil
}

// smbKeepAliveInterval keeps NAT/firewall mappings alive and doubles as a
// death probe. SMB has no client.Wait() equivalent (unlike the SSH-based file
// sessions), so a dead TCP link used to leave the session reporting
// "connected" forever and no operation could trigger the reconnect flow.
const smbKeepAliveInterval = 60 * time.Second

func (s *SMBSession) startKeepAlive() {
	ticker := time.NewTicker(smbKeepAliveInterval)
	defer ticker.Stop()
	for range ticker.C {
		if s.Status() != StatusConnected {
			return
		}
		s.mu.RLock()
		conn := s.conn
		s.mu.RUnlock()
		if conn == nil {
			return
		}
		if err := conn.Echo(); err != nil {
			// Link is dead: mark the session disconnected so the frontend's
			// status check (not just error wording) sees the loss.
			s.Disconnect()
			return
		}
	}
}

func (s *SMBSession) Write(data []byte) error  { return nil }
func (s *SMBSession) Resize(cols, rows int) error { return nil }

func (s *SMBSession) Disconnect() error {
	if s.share != nil {
		s.share.Umount()
		s.share = nil
	}
	if s.conn != nil {
		s.conn.Logoff()
		s.conn = nil
	}
	s.cwd = "/"
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *SMBSession) IsConnected() bool {
	return s.Status() == StatusConnected && s.conn != nil
}

func (s *SMBSession) requireShare() error {
	if s.share == nil {
		return fmt.Errorf("SMB session not connected")
	}
	return nil
}

// smbDir returns the parent directory for an SMB path.
func smbDir(p string) string {
	p = strings.TrimSuffix(p, "/")
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return ""
	}
	return p[:i]
}

// smbJoin joins two SMB path elements with a forward slash.
func smbJoin(base, p string) string {
	if base == "" {
		return p
	}
	return base + "/" + p
}

// smbInternal converts a frontend/internal path (with share prefix) to the raw
// SMB path used by the share's ReadDir/Open/etc. methods.
// E.g. "/sharename/dir/file" → "dir/file", "/sharename" → ""
func (s *SMBSession) smbInternal(p string) string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return ""
	}
	// Find the first segment (share name) and strip it
	idx := strings.Index(p, "/")
	if idx < 0 {
		// p is just the share name → share root
		return ""
	}
	// Return everything after the share name
	return p[idx+1:]
}

func (s *SMBSession) resolveRemote(p string) (string, error) {
	// "" means "list current directory" (used by refresh).
	if p == "" {
		return s.cwd, nil
	}
	return p, nil
}

func (s *SMBSession) nextTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&s.taskSeq, 1))
}

func (s *SMBSession) readRemoteFile(remotePath string) ([]byte, error) {
	f, err := s.share.Open(remotePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *SMBSession) writeRemoteFile(remotePath string, content []byte) error {
	f, err := s.share.Create(remotePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}

func (s *SMBSession) mkdirAllRemote(dir string) error {
	if dir == "" || dir == "." || dir == "/" {
		return nil
	}
	fi, err := s.share.Stat(dir)
	if err == nil && fi.IsDir() {
		return nil
	}
	if err := s.mkdirAllRemote(smbDir(dir)); err != nil {
		return err
	}
	return s.share.Mkdir(dir, 0755)
}

// listShares returns all available shares as directory entries.
func (s *SMBSession) listShares() (FileListResult, error) {
	if s.conn == nil {
		return FileListResult{}, fmt.Errorf("SMB session not connected")
	}
	names, err := s.conn.ListSharenames()
	if err != nil {
		return FileListResult{}, fmt.Errorf("smb list shares: %w", err)
	}
	files := make([]FileItem, 0, len(names))
	for _, name := range names {
		files = append(files, FileItem{
			Name:  name,
			Size:  0,
			Mode:  "drwxr-xr-x",
			IsDir: true,
		})
	}
	return FileListResult{Files: files, Dir: "/"}, nil
}

func (s *SMBSession) ListRemote(dir string) (FileListResult, error) {
	// No share mounted: show share list at root
	if s.share == nil {
		if s.conn == nil {
			return FileListResult{}, fmt.Errorf("SMB session not connected")
		}
		return s.listShares()
	}

	target, err := s.resolveRemote(dir)
	if err != nil {
		return FileListResult{}, err
	}
	internal := s.smbInternal(target)
	entries, err := s.share.ReadDir(internal)
	if err != nil {
		return FileListResult{}, err
	}
	files := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		modTime := ""
		if !e.ModTime().IsZero() {
			modTime = e.ModTime().Format(time.RFC3339)
		}
		files = append(files, FileItem{
			Name:    e.Name(),
			Size:    e.Size(),
			ModTime: modTime,
			Mode:    e.Mode().String(),
			IsDir:   e.IsDir(),
		})
	}
	return FileListResult{Files: files, Dir: target}, nil
}

func (s *SMBSession) ChangeRemoteDir(dir string) (FileListResult, error) {
	// No share mounted yet: navigating into a share name mounts it. A deeper
	// path ("/share/sub/dir") mounts the share and lands directly in the
	// subdirectory, so a post-reconnect re-navigate with the previous cwd
	// restores the location in one step.
	if s.share == nil {
		if s.conn == nil {
			return FileListResult{}, fmt.Errorf("SMB session not connected")
		}
		sharePath := strings.TrimPrefix(dir, "/")
		if sharePath == "" || sharePath == "/" {
			return s.listShares()
		}
		shareName, subDir := sharePath, ""
		if idx := strings.Index(sharePath, "/"); idx >= 0 {
			shareName, subDir = sharePath[:idx], sharePath[idx+1:]
		}
		share, err := s.conn.Mount(shareName)
		if err != nil {
			return FileListResult{}, fmt.Errorf("smb mount share %s: %w", shareName, err)
		}
		s.share = share
		s.cwd = "/" + sharePath
		if subDir != "" {
			fi, err := share.Stat(subDir)
			if err != nil {
				share.Umount()
				s.share = nil
				s.cwd = "/"
				return FileListResult{}, fmt.Errorf("no such directory: %s: %w", dir, err)
			}
			if !fi.IsDir() {
				share.Umount()
				s.share = nil
				s.cwd = "/"
				return FileListResult{}, fmt.Errorf("not a directory: %s", dir)
			}
		}
		return s.ListRemote("")
	}

	target, err := s.resolveRemote(dir)
	if err != nil {
		return FileListResult{}, err
	}

	// "/" from inside a share → go back to share list
	if target == "/" {
		s.share.Umount()
		s.share = nil
		s.cwd = "/"
		return s.listShares()
	}

	// Navigate up: handle going from share root back to share list
	if dir == ".." {
		// If at share root (cwd is "/sharename" or ""), unmount and list shares
		cwdTrimmed := strings.TrimPrefix(strings.TrimSuffix(s.cwd, "/"), "/")
		if cwdTrimmed == "" || !strings.Contains(cwdTrimmed, "/") {
			// At share root, go back to share list
			s.share.Umount()
			s.share = nil
			s.cwd = "/"
			return s.listShares()
		}
		// Navigate to parent directory
		parent := smbDir(target)
		if parent == "" {
			parent = ""
		}
		s.mu.Lock()
		s.cwd = parent
		s.mu.Unlock()
		return s.ListRemote(parent)
	}

	// Root directory: skip Stat (SMB can't stat empty path)
	internal := s.smbInternal(target)
	if internal != "" {
		fi, err := s.share.Stat(internal)
		if err != nil {
			// Wrap the underlying error: a dead link surfaces here as Stat
			// failure, and the frontend's reconnect detection matches on the
			// transport wording (broken pipe / connection reset / ...).
			return FileListResult{}, fmt.Errorf("no such directory: %s: %w", target, err)
		}
		if !fi.IsDir() {
			return FileListResult{}, fmt.Errorf("not a directory: %s", target)
		}
	}
	s.mu.Lock()
	s.cwd = target
	s.mu.Unlock()
	return s.ListRemote(target)
}

// Symlink is not supported: the SMB client library does not expose reparse
// point creation.
func (s *SMBSession) Symlink(_, _ string) error {
	return fmt.Errorf("symlink is not supported by SMB")
}

func (s *SMBSession) MakeDir(dir string) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	p, err := s.resolveRemote(dir)
	if err != nil {
		return err
	}
	return s.share.Mkdir(s.smbInternal(p), 0755)
}

func (s *SMBSession) Remove(p string, recursive bool) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	target, err := s.resolveRemote(p)
	if err != nil {
		return err
	}
	internal := s.smbInternal(target)
	if recursive {
		return s.share.RemoveAll(internal)
	}
	fi, err := s.share.Stat(internal)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		entries, err := s.share.ReadDir(internal)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory not empty (%d items)", len(entries))
		}
	}
	return s.share.Remove(internal)
}

func (s *SMBSession) Rename(oldName, newName string) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	old, err := s.resolveRemote(oldName)
	if err != nil {
		return err
	}
	n, err := s.resolveRemote(newName)
	if err != nil {
		return err
	}
	return s.share.Rename(s.smbInternal(old), s.smbInternal(n))
}

func (s *SMBSession) Chmod(p string, mode os.FileMode) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	target, err := s.resolveRemote(p)
	if err != nil {
		return err
	}
	return s.share.Chmod(s.smbInternal(target), mode)
}

func (s *SMBSession) Copy(oldPath, newPath string) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	old, err := s.resolveRemote(oldPath)
	if err != nil {
		return err
	}
	n, err := s.resolveRemote(newPath)
	if err != nil {
		return err
	}
	oldInternal := s.smbInternal(old)
	fi, err := s.share.Stat(oldInternal)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("cannot copy directory via SMB: %s", old)
	}
	newInternal := s.smbInternal(n)
	if err := s.mkdirAllRemote(smbDir(newInternal)); err != nil {
		return err
	}
	src, err := s.share.Open(oldInternal)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := s.share.Create(newInternal)
	if err != nil {
		return err
	}
	defer dst.Close()
	// Both files are on the same share, so io.Copy lands in File.ReadFrom,
	// which issues a server-side FSCTL_SRV_COPYCHUNK (data never leaves the
	// server). If the server doesn't support it, the library falls back to a
	// max-size buffered copy instead of loading the file into memory.
	_, err = io.Copy(dst, src)
	return err
}

func (s *SMBSession) Move(oldPath, newPath string) error {
	return s.Rename(oldPath, newPath)
}

func (s *SMBSession) GetContent(remotePath string) ([]byte, error) {
	if err := s.requireShare(); err != nil {
		return nil, err
	}
	p, err := s.resolveRemote(remotePath)
	if err != nil {
		return nil, err
	}
	return s.readRemoteFile(s.smbInternal(p))
}

func (s *SMBSession) PutContent(remotePath string, content []byte) error {
	if err := s.requireShare(); err != nil {
		return err
	}
	p, err := s.resolveRemote(remotePath)
	if err != nil {
		return err
	}
	internal := s.smbInternal(p)
	parentDir := smbDir(internal)
	if err := s.mkdirAllRemote(parentDir); err != nil {
		return err
	}
	return s.writeRemoteFile(internal, content)
}

func (s *SMBSession) Get(remotePath, localPath string, recursive bool) (string, error) {
	if err := s.requireShare(); err != nil {
		return "", err
	}
	rp, err := s.resolveRemote(remotePath)
	if err != nil {
		return "", err
	}
	rp = s.smbInternal(rp)
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	task := &TransferTask{
		ID:         s.nextTaskID("dl"),
		Type:       "download",
		LocalPath:  lp,
		RemotePath: rp,
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
		var err error
		if recursive {
			err = s.downloadDir(rp, lp, task)
		} else {
			err = s.downloadFile(task, rp, lp)
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

func (s *SMBSession) Put(localPath, remotePath string, recursive bool) (string, error) {
	if err := s.requireShare(); err != nil {
		return "", err
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	rp, err := s.resolveRemote(remotePath)
	if err != nil {
		return "", err
	}
	rp = s.smbInternal(rp)
	task := &TransferTask{
		ID:         s.nextTaskID("ul"),
		Type:       "upload",
		LocalPath:  lp,
		RemotePath: rp,
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
		var err error
		if recursive {
			err = s.uploadDir(lp, rp, task)
		} else {
			err = s.uploadFile(task, lp, rp)
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

func (s *SMBSession) CancelTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	if task.cancel != nil {
		task.cancel()
	}
	return nil
}

// RetryTransfer (re)starts a transfer from a frontend-held checkpoint. The
// task is re-created directly instead of going through Get/Put: the spec
// carries the already-resolved internal remote path from the original task,
// and smbInternal's share-name stripping is not idempotent. skipCompleted is
// ignored — the transfer restarts from scratch.
func (s *SMBSession) RetryTransfer(spec TransferSpec, skipCompleted []string) (string, error) {
	if err := s.requireShare(); err != nil {
		return "", err
	}
	tfType := spec.Type
	prefix := "dl"
	if tfType != "download" {
		tfType = "upload"
		prefix = "ul"
	}
	task := &TransferTask{
		ID:         s.nextTaskID(prefix),
		Type:       tfType,
		LocalPath:  spec.LocalPath,
		RemotePath: spec.RemotePath,
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
				return // retained for retry
			}
			s.mu.Lock()
			delete(s.transfers, task.ID)
			s.mu.Unlock()
		}()
		var err error
		if spec.Recursive {
			if tfType == "download" {
				err = s.downloadDir(spec.RemotePath, spec.LocalPath, task)
			} else {
				err = s.uploadDir(spec.LocalPath, spec.RemotePath, task)
			}
		} else {
			if tfType == "download" {
				err = s.downloadFile(task, spec.RemotePath, spec.LocalPath)
			} else {
				err = s.uploadFile(task, spec.LocalPath, spec.RemotePath)
			}
		}
		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
	return task.ID, nil
}

// DismissTransfer drops a retained (failed) task from the transfers map.
func (s *SMBSession) DismissTransfer(taskID string) error {
	s.mu.Lock()
	delete(s.transfers, taskID)
	s.mu.Unlock()
	return nil
}

func (s *SMBSession) PauseTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.setPaused(true)
	task.Status = "paused"
	s.emitTransferComplete(task)
	return nil
}

func (s *SMBSession) ResumeTransfer(taskID string) error {
	s.mu.Lock()
	task, ok := s.transfers[taskID]
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	task.setPaused(false)
	task.Status = "running"
	close(task.pauseCh)
	task.pauseCh = make(chan struct{})
	s.emitTransferStart(task)
	return nil
}

// calcSmbRemoteDirSize recursively sums up all file sizes under a remote SMB directory.
func (s *SMBSession) calcSmbRemoteDirSize(remoteDir string) (int64, error) {
	var total int64
	entries, err := s.share.ReadDir(remoteDir)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if e.IsDir() {
			sub, err := s.calcSmbRemoteDirSize(smbJoin(remoteDir, e.Name()))
			if err != nil {
				return 0, err
			}
			total += sub
		} else {
			total += e.Size()
		}
	}
	return total, nil
}

func (s *SMBSession) downloadDir(remoteDir, localDir string, task *TransferTask) error {
	// Calculate total size for progress tracking
	if task.loadTotal() <= 0 {
		if total, err := s.calcSmbRemoteDirSize(remoteDir); err == nil {
			task.setTotal(total)
		}
	}

	if err := os.MkdirAll(localDir, 0755); err != nil {
		return err
	}
	entries, err := s.share.ReadDir(remoteDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		select {
		case <-task.ctx.Done():
			return task.ctx.Err()
		default:
		}
		rp := smbJoin(remoteDir, e.Name())
		lp := filepath.Join(localDir, e.Name())
		if e.IsDir() {
			if err := s.downloadDir(rp, lp, task); err != nil {
				return err
			}
		} else {
			if err := s.downloadFile(task, rp, lp); err != nil {
				return err
			}
		}
	}
	return nil
}

// Transfer chunking constants. A purely sequential 64KB loop turns every
// chunk into one full SMB round trip and caps throughput well below what the
// link allows (e.g. ~35MB/s at 1.8ms RTT). SMB2 credits permit several
// requests in flight on a single connection, so we copy in large pipelined
// chunks instead.
const (
	// One SMB request payload per chunk. The library further caps each
	// request at the negotiated maximum (1MB on SMB3) and blocks on credits,
	// so an oversized buffer is safe.
	smbChunkSize = 1024 * 1024
	// Concurrent SMB requests per file transfer.
	smbInFlight = 4
)

// pipelinedCopy copies size bytes from readAt to writeAt using smbInFlight
// concurrent chunk workers. Both sides must accept random offsets (io.ReaderAt
// / io.WriterAt semantics). size <= 0 (unknown) falls back to a sequential
// loop with a full-size buffer. Progress, cancel and pause follow the task.
func (s *SMBSession) pipelinedCopy(task *TransferTask, size int64,
	readAt func(buf []byte, off int64) (int, error),
	writeAt func(buf []byte, off int64) error,
) error {
	if size <= 0 {
		buf := make([]byte, smbChunkSize)
		var off int64
		for {
			select {
			case <-task.ctx.Done():
				return task.ctx.Err()
			default:
			}
			task.waitIfPaused()
			n, err := readAt(buf, off)
			if n > 0 {
				if werr := writeAt(buf[:n], off); werr != nil {
					return werr
				}
				task.addProgress(int64(n))
				s.emitTransferProgress(task)
			}
			if err != nil {
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					return nil
				}
				return err
			}
			if n < len(buf) {
				return nil
			}
			off += int64(n)
		}
	}

	var (
		next     atomic.Int64 // next unclaimed offset
		stopped  atomic.Bool  // stop claiming (error, early EOF or cancel)
		errOnce  sync.Once
		firstErr error
	)
	abort := func(err error) {
		if err == nil {
			return
		}
		errOnce.Do(func() {
			firstErr = err
			stopped.Store(true)
		})
	}
	// claim reserves the next chunk; ok=false when nothing is left.
	claim := func() (off int64, length int, ok bool) {
		for {
			if stopped.Load() {
				return 0, 0, false
			}
			off := next.Load()
			if off >= size {
				return 0, 0, false
			}
			length := int(min(int64(smbChunkSize), size-off))
			if next.CompareAndSwap(off, off+int64(length)) {
				return off, length, true
			}
		}
	}

	var wg sync.WaitGroup
	for i := 0; i < smbInFlight; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := make([]byte, smbChunkSize)
			for {
				select {
				case <-task.ctx.Done():
					abort(task.ctx.Err())
					return
				default:
				}
				task.waitIfPaused()
				off, length, ok := claim()
				if !ok {
					return
				}
				n, err := readAt(buf[:length], off)
				if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
					abort(err)
					return
				}
				if n > 0 {
					if werr := writeAt(buf[:n], off); werr != nil {
						abort(werr)
						return
					}
					task.addProgress(int64(n))
					s.emitTransferProgress(task)
				}
				if n < length {
					// Source ended before the expected size: stop claiming.
					stopped.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
	}
	select {
	case <-task.ctx.Done():
		return task.ctx.Err()
	default:
	}
	return nil
}

func (s *SMBSession) downloadFile(task *TransferTask, remotePath, localPath string) error {
	// Stat the file for its own size (needed to chunk the reads); only set
	// the task total if it isn't already carrying a directory-wide total.
	var size int64
	if fi, err := s.share.Stat(remotePath); err == nil {
		size = fi.Size()
		if task.loadTotal() <= 0 && size > 0 {
			task.setTotal(size)
		}
	}

	f, err := s.share.Open(remotePath)
	if err != nil {
		return err
	}
	defer f.Close()
	dst, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	return s.pipelinedCopy(task, size, f.ReadAt,
		func(buf []byte, off int64) error {
			_, err := dst.WriteAt(buf, off)
			return err
		})
}

func (s *SMBSession) uploadDir(localDir, remoteDir string, task *TransferTask) error {
	// Calculate total size for progress tracking
	if task.loadTotal() <= 0 {
		if total, err := calcLocalDirSize(localDir); err == nil {
			task.setTotal(total)
		}
	}

	if err := s.mkdirAllRemote(remoteDir); err != nil {
		return err
	}
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		select {
		case <-task.ctx.Done():
			return task.ctx.Err()
		default:
		}
		lp := filepath.Join(localDir, entry.Name())
		rp := remoteDir + "/" + entry.Name()
		if entry.IsDir() {
			if err := s.uploadDir(lp, rp, task); err != nil {
				return err
			}
		} else {
			if err := s.uploadFile(task, lp, rp); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *SMBSession) uploadFile(task *TransferTask, localPath, remotePath string) error {
	// Stat the local file for its own size (needed to chunk the reads); only
	// set the task total if it isn't already carrying a directory-wide total.
	var size int64
	if fi, err := os.Stat(localPath); err == nil {
		size = fi.Size()
		if task.loadTotal() <= 0 && size > 0 {
			task.setTotal(size)
		}
	}

	src, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := s.share.Create(remotePath)
	if err != nil {
		return err
	}
	defer dst.Close()

	return s.pipelinedCopy(task, size, src.ReadAt,
		func(buf []byte, off int64) error {
			_, err := dst.WriteAt(buf, off)
			return err
		})
}

