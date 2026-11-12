package session

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jlaffaye/ftp"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// ftpDataIdleTimeout bounds a single read on the data connection. Old
// single-threaded servers (e.g. AIX ftpd) can leave the data socket silent
// forever; without it a stuck transfer never progresses and its cancel
// button is a no-op.
const ftpDataIdleTimeout = 60 * time.Second

type FTPSession struct {
	baseSession
	localFSOps
	conn      *ftp.ServerConn
	cwd       string
	mu        sync.RWMutex
	transfers map[string]*TransferTask
	taskSeq   int64
	connMu    sync.Mutex // serialize data transfer operations (FTP is not concurrent)

	// Filename charset handling. enc is nil in UTF-8 mode:
	// names pass through as-is, except names that are not valid UTF-8 get a
	// GBK fallback decode. rawNames remembers the original server bytes for
	// every decoded name so outgoing operations can address the file.
	encName  string            // "" means UTF-8
	enc      encoding.Encoding // nil means UTF-8
	rawNames sync.Map          // decoded name -> raw server bytes
}

func NewFTPSession(id string) *FTPSession {
	return &FTPSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "ftp",
			status:      StatusDisconnected,
		},
		localFSOps: newLocalFSOps(),
		cwd:        "/",
		transfers:  make(map[string]*TransferTask),
	}
}

func (s *FTPSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	s.title = fmt.Sprintf("%s@%s", config.User, config.Host)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	if config.Port <= 0 {
		addr = fmt.Sprintf("%s:21", config.Host)
	}

	encryption := config.FtpEncryption
	if encryption == "" {
		encryption = "none"
	}

	// Charset: an explicit non-UTF-8 setting means the server must keep
	// sending names in its local codepage, so stop the library from
	// switching it to UTF-8 via "OPTS UTF8 ON".
	encName := config.Encoding
	if encName == "utf-8" {
		encName = ""
	}
	enc := encodingByName(encName)
	dialOpts := []ftp.DialOption{ftp.DialWithTimeout(30 * time.Second)}
	// Bound the wait for the "closing data connection" status after each
	// data transfer; some servers never send it (library default is infinite).
	dialOpts = append(dialOpts, ftp.DialWithShutTimeout(30*time.Second))
	if enc != nil {
		dialOpts = append(dialOpts, ftp.DialWithDisabledUTF8(true))
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.FtpSkipVerify,
	}

	var conn *ftp.ServerConn
	var err error

	switch encryption {
	case "required":
		conn, err = ftp.Dial(addr,
			append(dialOpts, ftp.DialWithExplicitTLS(tlsConfig))...)
		if err != nil {
			s.setStatus(StatusError)
			return fmt.Errorf("ftp dial (TLS required): %w", err)
		}
	case "auto":
		conn, err = ftp.Dial(addr,
			append(dialOpts, ftp.DialWithExplicitTLS(tlsConfig))...)
		if err != nil {
			// Fall back to plain FTP
			conn, err = ftp.Dial(addr, dialOpts...)
		}
	default: // "none"
		conn, err = ftp.Dial(addr, dialOpts...)
	}

	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("ftp dial: %w", err)
	}

	if err := conn.Login(config.User, config.Password); err != nil {
		conn.Quit()
		s.setStatus(StatusError)
		return fmt.Errorf("ftp login: %w", err)
	}

	s.conn = conn
	s.cwd = "/"
	s.encName = encName
	s.enc = enc
	s.rawNames = sync.Map{} // fresh connection, no names seen yet
	s.setStatus(StatusConnected)
	// One-shot session log warning when the user has opted in to
	// InsecureSkipVerify. Surfaces the MITM risk in the session feed so
	// it can't be silently enabled by a forgotten checkbox.
	if config.FtpSkipVerify && encryption != "none" {
		s.emitData([]byte("\x1b[33m[FTP TLS verify disabled - connection is vulnerable to MITM]\x1b[0m\r\n"))
	}
	return nil
}

func (s *FTPSession) Write(data []byte) error {
	return nil
}

func (s *FTPSession) Resize(cols, rows int) error {
	return nil
}

func (s *FTPSession) Disconnect() error {
	// Serialize close against in-flight data transfers and ChangeRemoteDir
	// (which also touches s.conn without holding connMu — see SESSION-15).
	// Without connMu here, ftp.ServerConn.Quit() can race a concurrent
	// Stor/Retr and panic inside the FTP library.
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if s.conn != nil {
		s.conn.Quit()
		s.conn = nil
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *FTPSession) IsConnected() bool {
	return s.Status() == StatusConnected && s.conn != nil
}

// --- Internal helpers ---

// ftpDecodeName converts a raw name from the server's listing into UTF-8 for
// display. In UTF-8 mode names that are not valid UTF-8 are assumed to be GBK
// (same fallback FileZilla uses). The raw bytes of every decoded name are
// remembered so later operations can address the file on the server.
func (s *FTPSession) ftpDecodeName(name string) string {
	if s.enc == nil {
		if utf8.ValidString(name) {
			return name
		}
		dec, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), name)
		if err != nil {
			return name
		}
		s.rawNames.Store(dec, name)
		return dec
	}
	dec, _, err := transform.String(s.enc.NewDecoder(), name)
	if err != nil {
		return name
	}
	s.rawNames.Store(dec, name)
	return dec
}

// ftpEncodePath converts a UTF-8 path into the server's byte encoding, one
// segment at a time. Segments whose raw server bytes were seen in a listing
// are restored verbatim; unknown segments are encoded with the configured
// charset, or sent as UTF-8 when the server is in UTF-8 mode.
func (s *FTPSession) ftpEncodePath(p string) string {
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		if raw, ok := s.rawNames.Load(seg); ok {
			segs[i] = raw.(string)
			continue
		}
		if s.enc == nil {
			continue // UTF-8 mode: pass through
		}
		enc, _, err := transform.String(s.enc.NewEncoder(), seg)
		if err == nil {
			segs[i] = enc
		}
	}
	return strings.Join(segs, "/")
}

// Encoded-path wrappers around the raw connection. All outgoing paths go
// through ftpEncodePath so non-UTF-8 servers see names in their own charset.
func (s *FTPSession) ftpList(dir string) ([]*ftp.Entry, error) {
	return s.conn.List(s.ftpEncodePath(dir))
}
func (s *FTPSession) ftpMakeDir(dir string) error {
	return s.conn.MakeDir(s.ftpEncodePath(dir))
}
func (s *FTPSession) ftpDelete(p string) error {
	return s.conn.Delete(s.ftpEncodePath(p))
}
func (s *FTPSession) ftpRemoveDir(p string) error {
	return s.conn.RemoveDir(s.ftpEncodePath(p))
}
func (s *FTPSession) ftpRename(from, to string) error {
	return s.conn.Rename(s.ftpEncodePath(from), s.ftpEncodePath(to))
}
func (s *FTPSession) ftpRetr(p string) (*ftp.Response, error) {
	return s.conn.Retr(s.ftpEncodePath(p))
}
func (s *FTPSession) ftpStor(p string, r io.Reader) error {
	return s.conn.Stor(s.ftpEncodePath(p), r)
}
func (s *FTPSession) ftpFileSize(p string) (int64, error) {
	return s.conn.FileSize(s.ftpEncodePath(p))
}

func (s *FTPSession) nextTaskID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&s.taskSeq, 1))
}

func (s *FTPSession) requireClient() error {
	if s.conn == nil {
		return fmt.Errorf("FTP session not connected")
	}
	return nil
}

func (s *FTPSession) requireConn() (*ftp.ServerConn, error) {
	if s.conn == nil {
		return nil, fmt.Errorf("FTP session not connected")
	}
	return s.conn, nil
}

// --- Public API methods ---

func (s *FTPSession) ListRemote(dir string) (FileListResult, error) {
	if err := s.requireClient(); err != nil {
		return FileListResult{}, err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if dir == "" {
		dir = s.cwd
	} else if !path.IsAbs(dir) {
		dir = path.Join(s.cwd, dir)
	}
	entries, err := s.ftpList(dir)
	if err != nil {
		return FileListResult{}, err
	}
	files := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		isDir := e.Type == ftp.EntryTypeFolder
		modTime := ""
		if !e.Time.IsZero() {
			modTime = e.Time.Format(time.RFC3339)
		}
		files = append(files, FileItem{
			Name:    s.ftpDecodeName(e.Name),
			Size:    int64(e.Size),
			ModTime: modTime,
			Mode:    ftpEntryMode(e),
			IsDir:   isDir,
		})
	}
	return FileListResult{Files: files, Dir: dir}, nil
}

func ftpEntryMode(e *ftp.Entry) string {
	switch e.Type {
	case ftp.EntryTypeFolder:
		return "drwxr-xr-x"
	case ftp.EntryTypeLink:
		return "Lrwxrwxrwx"
	default:
		return "-rw-r--r--"
	}
}

func (s *FTPSession) ChangeRemoteDir(dir string) (FileListResult, error) {
	if err := s.requireClient(); err != nil {
		return FileListResult{}, err
	}
	target := dir
	if !path.IsAbs(dir) {
		target = path.Join(s.cwd, dir)
	}
	// Validate directory exists by listing it — must hold connMu because
	// the FTP control connection is not concurrent-safe (SESSION-15).
	s.connMu.Lock()
	entries, err := s.ftpList(target)
	s.connMu.Unlock()
	if err != nil {
		return FileListResult{}, fmt.Errorf("no such directory: %s", target)
	}
	s.mu.Lock()
	s.cwd = target
	s.mu.Unlock()
	files := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		isDir := e.Type == ftp.EntryTypeFolder
		modTime := ""
		if !e.Time.IsZero() {
			modTime = e.Time.Format(time.RFC3339)
		}
		files = append(files, FileItem{
			Name:    s.ftpDecodeName(e.Name),
			Size:    int64(e.Size),
			ModTime: modTime,
			Mode:    ftpEntryMode(e),
			IsDir:   isDir,
		})
	}
	return FileListResult{Files: files, Dir: target}, nil
}

// Symlink is not supported: the FTP protocol has no link semantics.
func (s *FTPSession) Symlink(_, _ string) error {
	return fmt.Errorf("symlink is not supported by FTP")
}

func (s *FTPSession) MakeDir(dir string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	p := dir
	if !path.IsAbs(p) {
		p = path.Join(s.cwd, p)
	}
	return s.ftpMakeDir(p)
}

func (s *FTPSession) Remove(p string, recursive bool) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	if !path.IsAbs(p) {
		p = path.Join(s.cwd, p)
	}
	if recursive {
		return s.rmRecursive(p)
	}
	// Try file deletion first; if that fails (e.g. it's a directory), try RemoveDir
	err := s.ftpDelete(p)
	if err == nil {
		return nil
	}
	// Not a plain file — check if it's an empty directory
	entries, listErr := s.ftpList(p)
	if listErr == nil && len(entries) > 0 {
		return fmt.Errorf("directory not empty (%d items), use recursive=true", len(entries))
	}
	return s.ftpRemoveDir(p)
}

func (s *FTPSession) Rename(oldName, newName string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	old := oldName
	if !path.IsAbs(old) {
		old = path.Join(s.cwd, old)
	}
	newPath := newName
	if !path.IsAbs(newPath) {
		newPath = path.Join(s.cwd, newPath)
	}
	return s.ftpRename(old, newPath)
}

func (s *FTPSession) Chmod(p string, mode os.FileMode) error {
	return fmt.Errorf("FTP does not support chmod")
}

func (s *FTPSession) Get(remotePath, localPath string, recursive bool) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	rp := remotePath
	if !path.IsAbs(rp) {
		rp = path.Join(s.cwd, rp)
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	if recursive {
		total, err := s.dirSizeRemote(rp)
		if err != nil {
			return "", err
		}
		task := &TransferTask{
			ID:         s.nextTaskID("dl"),
			Type:       "download",
			LocalPath:  lp,
			RemotePath: rp,
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
				s.mu.Lock()
				delete(s.transfers, task.ID)
				s.mu.Unlock()
			}()
			// Serialize against the control connection like single-file
			// transfers; a queued cancel takes effect without waiting.
			s.connMu.Lock()
			defer s.connMu.Unlock()
			if task.ctx.Err() != nil {
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			if err := s.downloadDir(rp, lp, task); err != nil {
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
	task := &TransferTask{
		ID:         s.nextTaskID("dl"),
		Type:       "download",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "pending",
	}
	s.startTransfer(task)
	return task.ID, nil
}

func (s *FTPSession) Put(localPath, remotePath string, recursive bool) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	rp := remotePath
	if !path.IsAbs(rp) {
		rp = path.Join(s.cwd, rp)
	}
	if recursive {
		total, err := s.dirSizeLocal(lp)
		if err != nil {
			return "", err
		}
		task := &TransferTask{
			ID:         s.nextTaskID("ul"),
			Type:       "upload",
			LocalPath:  lp,
			RemotePath: rp,
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
				s.mu.Lock()
				delete(s.transfers, task.ID)
				s.mu.Unlock()
			}()
			// Serialize against the control connection like single-file
			// transfers; a queued cancel takes effect without waiting.
			s.connMu.Lock()
			defer s.connMu.Unlock()
			if task.ctx.Err() != nil {
				task.Status = "cancelled"
				s.emitTransferComplete(task)
				return
			}
			if err := s.uploadDir(lp, rp, task); err != nil {
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
	task := &TransferTask{
		ID:         s.nextTaskID("ul"),
		Type:       "upload",
		LocalPath:  lp,
		RemotePath: rp,
		Status:     "pending",
	}
	s.startTransfer(task)
	return task.ID, nil
}

// PutContent writes raw content directly to a remote file via FTP.
func (s *FTPSession) PutContent(remotePath string, content []byte) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	rp := remotePath
	if !path.IsAbs(rp) {
		rp = path.Join(s.cwd, rp)
	}
	// Ensure parent directory exists
	parentDir := path.Dir(rp)
	if err := s.mkdirAllRemote(parentDir); err != nil {
		return err
	}
	reader := strings.NewReader(string(content))
	return s.ftpStor(rp, reader)
}

// GetContent reads the full content of a remote file via FTP.
func (s *FTPSession) GetContent(remotePath string) ([]byte, error) {
	if err := s.requireClient(); err != nil {
		return nil, err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	rp := remotePath
	if !path.IsAbs(rp) {
		rp = path.Join(s.cwd, rp)
	}
	r, err := s.ftpRetr(rp)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Copy copies a remote file via FTP (download + re-upload — FTP has no server-side copy).
func (s *FTPSession) Copy(oldPath, newPath string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	old := oldPath
	if !path.IsAbs(old) {
		old = path.Join(s.cwd, old)
	}
	n := newPath
	if !path.IsAbs(n) {
		n = path.Join(s.cwd, n)
	}
	// FTP cannot copy directories via Retr, check first
	if _, listErr := s.ftpList(old); listErr == nil {
		return fmt.Errorf("cannot copy directory via FTP: %s", old)
	}
	// Download
	r, err := s.ftpRetr(old)
	if err != nil {
		return err
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	// Create parent directories for destination
	parentDir := path.Dir(n)
	if err := s.mkdirAllRemote(parentDir); err != nil {
		return err
	}
	// Upload
	return s.ftpStor(n, strings.NewReader(string(data)))
}

// Move moves a remote file via FTP Rename (server-side, no data transfer).
func (s *FTPSession) Move(oldPath, newPath string) error {
	return s.Rename(oldPath, newPath)
}

// mkdirAllRemote creates intermediate directories as needed.
func (s *FTPSession) mkdirAllRemote(dir string) error {
	if dir == "/" || dir == "." || dir == "" {
		return nil
	}
	// Try to list the directory; if it fails, create parent then this one
	_, err := s.ftpList(dir)
	if err == nil {
		return nil // already exists
	}
	// Create parent first
	if err := s.mkdirAllRemote(path.Dir(dir)); err != nil {
		return err
	}
	return s.ftpMakeDir(dir)
}

// CancelTransfer cancels an ongoing transfer task.
// RetryTransfer (re)starts a transfer from a frontend-held checkpoint.
// Get/Put resolve idempotently on the absolute paths the spec carries.
// skipCompleted is ignored — the transfer restarts from scratch.
func (s *FTPSession) RetryTransfer(spec TransferSpec, skipCompleted []string) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	if spec.Type == "download" {
		return s.Get(spec.RemotePath, spec.LocalPath, spec.Recursive)
	}
	return s.Put(spec.LocalPath, spec.RemotePath, spec.Recursive)
}

// DismissTransfer drops a retained (failed) task from the transfers map.
func (s *FTPSession) DismissTransfer(taskID string) error {
	s.mu.Lock()
	delete(s.transfers, taskID)
	s.mu.Unlock()
	return nil
}

func (s *FTPSession) CancelTransfer(taskID string) error {
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

// PauseTransfer pauses an ongoing transfer task.
func (s *FTPSession) PauseTransfer(taskID string) error {
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

// ResumeTransfer resumes a paused transfer task.
func (s *FTPSession) ResumeTransfer(taskID string) error {
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

// --- Recursive helpers ---

func (s *FTPSession) rmRecursive(p string) error {
	entries, err := s.ftpList(p)
	if err != nil {
		// Not a directory or cannot list; try deleting as file
		return s.ftpDelete(p)
	}
	for _, e := range entries {
		childPath := path.Join(p, s.ftpDecodeName(e.Name))
		if e.Type == ftp.EntryTypeFolder {
			if err := s.rmRecursive(childPath); err != nil {
				return err
			}
		} else {
			if err := s.ftpDelete(childPath); err != nil {
				return err
			}
		}
	}
	return s.ftpRemoveDir(p)
}

func (s *FTPSession) dirSizeRemote(dir string) (int64, error) {
	entries, err := s.ftpList(dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range entries {
		if e.Type == ftp.EntryTypeFolder {
			sz, err := s.dirSizeRemote(path.Join(dir, s.ftpDecodeName(e.Name)))
			if err != nil {
				return 0, err
			}
			total += sz
		} else {
			total += int64(e.Size)
		}
	}
	return total, nil
}

func (s *FTPSession) dirSizeLocal(dir string) (int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			sz, err := s.dirSizeLocal(filepath.Join(dir, e.Name()))
			if err != nil {
				return 0, err
			}
			total += sz
		} else {
			fi, err := e.Info()
			if err != nil {
				return 0, err
			}
			total += fi.Size()
		}
	}
	return total, nil
}

// --- Transfer methods ---

func (s *FTPSession) startTransfer(task *TransferTask) {
	task.start()
	s.mu.Lock()
	s.transfers[task.ID] = task
	s.mu.Unlock()
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

		// FTP control connection: serialize data transfers. The task stays
		// "pending" while queued behind the lock, and a cancel that arrives
		// while queued takes effect immediately.
		s.connMu.Lock()
		defer s.connMu.Unlock()
		if task.ctx.Err() != nil {
			task.Status = "cancelled"
			s.emitTransferComplete(task)
			return
		}

		// Respect a pause that arrived while the task was queued: surface it
		// as paused and let waitIfPaused park it until resumed (the resume
		// entry point re-emits the start event). Skip the start event when
		// the task was resumed while queued — it was already emitted.
		if task.isPaused() {
			task.Status = "paused"
		} else if task.Status != "running" {
			task.Status = "running"
			s.emitTransferStart(task)
		}

		var err error
		if task.Type == "download" {
			// Fetch the file size before issuing RETR. Single-threaded
			// servers (e.g. AIX ftpd) do not read the control connection
			// while RETR is streaming, so any control command sent between
			// RETR and the last data byte deadlocks the transfer at 0%
			//.
			if fi, e := s.ftpFileSize(task.RemotePath); e == nil && fi > 0 {
				task.setTotal(fi)
			}

			resp, e := s.ftpRetr(task.RemotePath)
			if e != nil {
				s.emitTransferEvent(task, e)
				return
			}
			defer resp.Close()

			// Unblock the data-socket read parked inside the library when
			// the task is cancelled mid-transfer.
			watchdogDone := make(chan struct{})
			defer close(watchdogDone)
			go func() {
				select {
				case <-task.ctx.Done():
					_ = resp.SetDeadline(time.Now())
				case <-watchdogDone:
				}
			}()

			localFile, e := os.Create(task.LocalPath)
			if e != nil {
				s.emitTransferEvent(task, e)
				return
			}
			defer localFile.Close()

			_, err = io.Copy(localFile, &progressReader{r: resp, task: task, s: s, deadline: resp})
		} else {
			localFile, e := os.Open(task.LocalPath)
			if e != nil {
				s.emitTransferEvent(task, e)
				return
			}
			defer localFile.Close()

			fi, _ := localFile.Stat()
			if fi != nil {
				task.setTotal(fi.Size())
			}

			err = s.ftpStor(task.RemotePath, &progressReader{r: localFile, task: task, s: s})
		}

		if err != nil {
			if task.ctx.Err() != nil {
				// cancelled mid-transfer: not a transfer error
				task.Status = "cancelled"
				if task.Type == "download" {
					// drop the partially written local file
					os.Remove(task.LocalPath)
				}
				s.emitTransferComplete(task)
				return
			}
			s.emitTransferEvent(task, err)
			return
		}
		task.Status = "done"
		s.emitTransferComplete(task)
	}()
}

type progressReader struct {
	r    io.Reader
	task *TransferTask
	s    *FTPSession
	// deadline, when non-nil, gets an idle deadline (ftpDataIdleTimeout)
	// re-armed before every read so a silent server cannot park the read
	// forever.
	deadline interface{ SetDeadline(time.Time) error }
}

func (pr *progressReader) Read(p []byte) (int, error) {
	// Abort promptly when the task is cancelled; without this io.Copy never
	// observes the context and the cancel button is a no-op.
	select {
	case <-pr.task.ctx.Done():
		return 0, pr.task.ctx.Err()
	default:
	}
	pr.task.waitIfPaused()
	if pr.deadline != nil {
		// Idle timeout: a server that stops sending data must not park the
		// read forever.
		_ = pr.deadline.SetDeadline(time.Now().Add(ftpDataIdleTimeout))
	}
	n, err := pr.r.Read(p)
	if n > 0 {
		pr.task.addProgress(int64(n))
		pr.s.emitTransferProgress(pr.task)
	}
	return n, err
}

// --- Recursive transfer ---

func (s *FTPSession) downloadDir(remoteDir, localDir string, task *TransferTask) error {
	select {
	case <-task.ctx.Done():
		return task.ctx.Err()
	default:
	}
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return err
	}
	entries, err := s.ftpList(remoteDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := s.ftpDecodeName(e.Name)
		rp := path.Join(remoteDir, name)
		lp := filepath.Join(localDir, name)
		if e.Type == ftp.EntryTypeFolder {
			if err := s.downloadDir(rp, lp, task); err != nil {
				return err
			}
		} else {
			if err := s.transferFile(task, lp, rp, "download"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *FTPSession) uploadDir(localDir, remoteDir string, task *TransferTask) error {
	select {
	case <-task.ctx.Done():
		return task.ctx.Err()
	default:
	}
	if err := s.mkdirAllRemote(remoteDir); err != nil {
		return err
	}
	entries, err := os.ReadDir(localDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		rp := path.Join(remoteDir, entry.Name())
		lp := filepath.Join(localDir, entry.Name())
		if entry.IsDir() {
			if err := s.uploadDir(lp, rp, task); err != nil {
				return err
			}
		} else {
			if err := s.transferFile(task, lp, rp, "upload"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *FTPSession) transferFile(task *TransferTask, localPath, remotePath, tfType string) error {
	if tfType == "download" {
		resp, err := s.ftpRetr(remotePath)
		if err != nil {
			return err
		}
		defer resp.Close()

		// Unblock the data-socket read parked inside the library when the
		// task is cancelled mid-transfer.
		watchdogDone := make(chan struct{})
		defer close(watchdogDone)
		go func() {
			select {
			case <-task.ctx.Done():
				_ = resp.SetDeadline(time.Now())
			case <-watchdogDone:
			}
		}()

		dst, err := os.Create(localPath)
		if err != nil {
			return err
		}
		defer dst.Close()
		// progressReader carries the cancel/pause checks and the data-socket
		// idle deadline. Read errors propagate (the old loop swallowed them,
		// silently completing on a truncated file).
		_, err = io.Copy(dst, &progressReader{r: resp, task: task, s: s, deadline: resp})
		return err
	} else {
		src, err := os.Open(localPath)
		if err != nil {
			return err
		}
		defer src.Close()
		pr, pw := io.Pipe()
		doneCh := make(chan error, 1)
		go func() {
			doneCh <- s.ftpStor(remotePath, pr)
		}()
		buf := make([]byte, 64*1024)
		for {
			select {
			case <-task.ctx.Done():
				pw.CloseWithError(task.ctx.Err())
				return task.ctx.Err()
			default:
			}
			task.waitIfPaused()
			n, e := src.Read(buf)
			if n > 0 {
				_, we := pw.Write(buf[:n])
				if we != nil {
					pw.Close()
					return we
				}
				task.addProgress(int64(n))
				s.emitTransferProgress(task)
			}
			if e != nil {
				break
			}
		}
		pw.Close()
		// Wait for Stor to complete
		if err := <-doneCh; err != nil {
			return err
		}
	}
	return nil
}
