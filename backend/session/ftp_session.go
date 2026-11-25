package session

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
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

// ftpShutTimeout bounds the wait for the "closing data connection" status
// reply after each data operation (issue #1453). A var so tests can shorten
// it.
var ftpShutTimeout = 30 * time.Second

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
	// Route control AND data (PASV) connections through the configured proxy.
	if config.Proxy != nil {
		dialOpts = append(dialOpts, ftp.DialWithDialFunc(func(network, address string) (net.Conn, error) {
			return dialFirstHop(address, config.Proxy)
		}))
	}
	// Bound the wait for the "closing data connection" status after each
	// data transfer; some servers never send it (library default is infinite).
	dialOpts = append(dialOpts, ftp.DialWithShutTimeout(ftpShutTimeout))
	if enc != nil {
		dialOpts = append(dialOpts, ftp.DialWithDisabledUTF8(true))
	}

	tlsConfig := &tls.Config{
		InsecureSkipVerify: config.FtpSkipVerify,
	}

	tryConnect := func(opts []ftp.DialOption) (*ftp.ServerConn, error) {
		var c *ftp.ServerConn
		var e error

		switch encryption {
		case "required":
			c, e = ftp.Dial(addr,
				append(opts, ftp.DialWithExplicitTLS(tlsConfig))...)
			if e != nil {
				return nil, fmt.Errorf("ftp dial (TLS required): %w", e)
			}
		case "auto":
			c, e = ftp.Dial(addr,
				append(opts, ftp.DialWithExplicitTLS(tlsConfig))...)
			if e != nil {
				// Fall back to plain FTP
				c, e = ftp.Dial(addr, opts...)
			}
		default: // "none"
			c, e = ftp.Dial(addr, opts...)
		}

		if e != nil {
			return nil, fmt.Errorf("ftp dial: %w", e)
		}

		if e := c.Login(config.User, config.Password); e != nil {
			c.Quit()
			return nil, fmt.Errorf("ftp login: %w", e)
		}

		return c, nil
	}

	// 1. Try standard connection first (with OPTS UTF8 negotiation if enc == nil).
	conn, err := tryConnect(dialOpts)

	// 2. Smart fallback for legacy/embedded FTP servers (e.g., Xbox 360 Aurora, PS3)
	// that reject "OPTS UTF8 ON" with 500 command not found.
	if err != nil && enc == nil {
		fallbackOpts := make([]ftp.DialOption, 0, len(dialOpts)+1)
		fallbackOpts = append(fallbackOpts, dialOpts...)
		fallbackOpts = append(fallbackOpts, ftp.DialWithDisabledUTF8(true))
		if fallbackConn, fallbackErr := tryConnect(fallbackOpts); fallbackErr == nil {
			conn = fallbackConn
			err = nil
		}
	}

	if err != nil {
		s.setStatus(StatusError)
		return err
	}

	s.conn = conn
	s.mu.Lock()
	s.cwd = "/"
	s.mu.Unlock()
	// Seed the start path from the connection config before reporting
	// connected: CWD resolves ~ and relative paths server-side; failure
	// keeps the login default. The local pane is seeded independently.
	if dir := strings.TrimSpace(config.Cwd); dir != "" {
		if err := conn.ChangeDir(dir); err == nil {
			// Query the server-resolved directory so the session cwd stays
			// absolute even when the start path was relative.
			s.mu.Lock()
			if wd, werr := conn.CurrentDir(); werr == nil && wd != "" {
				s.cwd = path.Clean(wd)
			} else {
				s.cwd = path.Clean(dir)
			}
			s.mu.Unlock()
		}
	}
	applyLocalStartDir(&s.localFSOps, config.LocalDir)
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
	s.rawNames = sync.Map{}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *FTPSession) IsConnected() bool {
	return s.Status() == StatusConnected && s.conn != nil
}

// --- Internal helpers ---

func (s *FTPSession) resolveRemotePath(p string) string {
	if path.IsAbs(p) {
		return p
	}
	s.mu.RLock()
	cwd := s.cwd
	s.mu.RUnlock()
	return path.Join(cwd, p)
}

func (s *FTPSession) remoteDirExists(dir string) bool {
	return s.conn.ChangeDir(s.ftpEncodePath(dir)) == nil
}

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
	encPath := s.ftpEncodePath(dir)

	// Legacy/embedded FTP servers (such as Xbox 360 Aurora) do not support
	// "LIST <path>" directly and return an empty 226 response even for
	// non-existent directories. Use "CWD <path>" + "LIST" instead so both
	// directory listing and existence checks work reliably.
	if encPath != "" {
		if err := s.conn.ChangeDir(encPath); err != nil {
			return nil, err
		}
	}

	entries, err := s.conn.List("")
	if err != nil {
		return nil, err
	}

	filtered := make([]*ftp.Entry, 0, len(entries))
	for _, e := range entries {
		// Device-style `ls` output pads columns with several spaces; the
		// parser only consumes one delimiter, so the surplus spaces end up
		// inside the name. They are never part of the real name but would
		// break every path built from it later (e.g. CWD "/ led_data"),
		// while the panel hides them because HTML collapses whitespace.
		// Trim the ends only — interior spaces are real name characters.
		e.Name = strings.TrimSpace(e.Name)
		if e.Name == "." || e.Name == ".." {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered, nil
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
		s.mu.RLock()
		dir = s.cwd
		s.mu.RUnlock()
	} else {
		dir = s.resolveRemotePath(dir)
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
	target := s.resolveRemotePath(dir)
	// Validate directory exists by listing it — must hold connMu because
	// the FTP control connection is not concurrent-safe (SESSION-15).
	s.connMu.Lock()
	entries, err := s.ftpList(target)
	s.connMu.Unlock()
	if err != nil {
		return FileListResult{}, fmt.Errorf("no such directory: %s: %w", target, err)
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
	p := s.resolveRemotePath(dir)
	return s.ftpMakeDir(p)
}

func (s *FTPSession) Remove(p string, recursive bool) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	p = s.resolveRemotePath(p)
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
	old := s.resolveRemotePath(oldName)
	newPath := s.resolveRemotePath(newName)
	return s.ftpRename(old, newPath)
}

func (s *FTPSession) Chmod(p string, mode os.FileMode) error {
	return fmt.Errorf("FTP does not support chmod")
}

func (s *FTPSession) Get(remotePath, localPath string, recursive bool) (string, error) {
	if err := s.requireClient(); err != nil {
		return "", err
	}
	rp := s.resolveRemotePath(remotePath)
	lp := localPath
	if !filepath.IsAbs(lp) {
		lp = filepath.Join(s.localCwd, lp)
	}
	if recursive {
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
			if total, err := s.dirSizeRemote(rp); err == nil {
				task.setTotal(total)
				s.emitTransferProgress(task)
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
	rp := s.resolveRemotePath(remotePath)
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
	s.connMu.Lock()
	defer s.connMu.Unlock()
	rp := s.resolveRemotePath(remotePath)
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
	rp := s.resolveRemotePath(remotePath)
	r, err := s.ftpRetr(rp)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// Copy copies a remote file via FTP (streaming download to temp file + re-upload
// to avoid buffering entire files in memory).
func (s *FTPSession) Copy(oldPath, newPath string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	s.connMu.Lock()
	defer s.connMu.Unlock()
	old := s.resolveRemotePath(oldPath)
	n := s.resolveRemotePath(newPath)
	// FTP cannot copy directories via Retr, check first via CWD
	if s.remoteDirExists(old) {
		return fmt.Errorf("cannot copy directory via FTP: %s", old)
	}
	// Stream download to a temporary file so large files do not exhaust memory
	r, err := s.ftpRetr(old)
	if err != nil {
		return err
	}
	tmpFile, err := os.CreateTemp("", "uniterm-ftp-copy-*")
	if err != nil {
		_ = r.Close()
		return err
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, copyErr := io.Copy(tmpFile, r)
	// Close the RETR response before issuing further control commands so the
	// server's 226 transfer-complete reply is drained cleanly.
	closeErr := r.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if _, err := tmpFile.Seek(0, io.SeekStart); err != nil {
		return err
	}
	// Create parent directories for destination
	parentDir := path.Dir(n)
	if err := s.mkdirAllRemote(parentDir); err != nil {
		return err
	}
	// Stream upload from temporary file
	return s.ftpStor(n, tmpFile)
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
	// Check directory existence via CWD on the control connection instead of
	// opening a PASV data connection with LIST for every path level.
	if s.remoteDirExists(dir) {
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
			err := s.ftpStor(remotePath, pr)
			_ = pr.Close()
			doneCh <- err
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
				if e != io.EOF {
					pw.CloseWithError(e)
					return e
				}
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
