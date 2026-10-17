package main

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const webFileMaxUpload int64 = 2 << 30
const webFileTTL = 2 * time.Hour

// WebFileStage is a temporary host copy, never a visitor filesystem path.
// Only opaque IDs are accepted by the HTTP download/status/delete endpoints.
type WebFileStage struct {
	ID        string   `json:"id"`
	Files     []string `json:"files"`
	State     string   `json:"state"`
	SessionID string   `json:"sessionId,omitempty"`
	TaskID    string   `json:"taskId,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type webFileStage struct {
	WebFileStage
	dir, target string
	expires     time.Time
	active      bool // an existing Get/Put is still writing/reading this directory
	users       int  // HTTP streams / synchronous consumers / pending Get/Put calls
	discard     bool
	cancel      context.CancelFunc
	cancelTask  func(string) error
}

type webFileStore struct {
	mu       sync.Mutex
	root     string
	stages   map[string]*webFileStage
	closed   bool
	stop     chan struct{}
	zmodem   map[string]*webZmodemRequest
	zTargets map[string]webZmodemTarget
}

// Registered beneath the shared Web listener's Host/Origin/token middleware.
func (a *App) registerWebFileRoutes(handle func(string, http.Handler)) {
	handle("/web/files/", http.HandlerFunc(a.serveWebFiles))
}

func webFileRelative(p string) error {
	for _, r := range p {
		if r < 32 || r == 127 {
			return errors.New("control character in filename")
		}
	}
	if p == "" || len(p) > 4096 || strings.ContainsAny(p, "\\:\x00\r\n") || path.IsAbs(p) || path.Clean(p) != p {
		return fmt.Errorf("invalid relative file path %q", p)
	}
	for _, part := range strings.Split(p, "/") {
		if part == "." || part == ".." || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || strings.ContainsAny(part, "<>\"|?*") {
			return fmt.Errorf("invalid filename %q", part)
		}
		// Reject Windows device names on every platform, including extensions.
		stem := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9') {
			return fmt.Errorf("reserved filename %q", part)
		}
	}
	return nil
}

func (a *App) newWebFileStage(state, sessionID string) (*webFileStage, error) {
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, errors.New("web files closed")
	}
	if w.root == "" {
		// Crash leftovers only; active instances refresh their root's mtime.
		old, _ := filepath.Glob(filepath.Join(os.TempDir(), "uniterm-web-files-*"))
		for _, dir := range old {
			if st, err := os.Stat(dir); err == nil && time.Since(st.ModTime()) > 24*time.Hour {
				_ = os.RemoveAll(dir)
			}
		}
		root, err := os.MkdirTemp("", "uniterm-web-files-*")
		if err != nil {
			return nil, err
		}
		w.root, w.stages, w.stop = root, make(map[string]*webFileStage), make(chan struct{})
		go func() {
			ticker := time.NewTicker(time.Minute)
			defer ticker.Stop()
			for {
				select {
				case <-ticker.C:
					a.sweepWebFiles(time.Now())
				case <-w.stop:
					return
				}
			}
		}()
	}
	if len(w.stages) >= 128 {
		return nil, errors.New("too many staged files; dismiss previous transfers")
	}
	if state == "uploading" {
		n := 0
		for _, s := range w.stages {
			if s.State == state {
				n++
			}
		}
		if n >= 4 {
			return nil, errors.New("too many simultaneous uploads")
		}
	}
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(b)
	dir := filepath.Join(w.root, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	s := &webFileStage{WebFileStage: WebFileStage{ID: id, State: state, SessionID: sessionID}, dir: dir, expires: time.Now().Add(webFileTTL), users: 1}
	w.stages[id] = s
	return s, nil
}

func (a *App) acquireWebFile(id string) (*webFileStage, error) {
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	s := w.stages[id]
	if w.closed || s == nil || s.discard || time.Now().After(s.expires) {
		return nil, errors.New("file stage expired or dismissed")
	}
	s.users++
	s.expires = time.Now().Add(webFileTTL)
	return s, nil
}

func (a *App) releaseWebFile(s *webFileStage) {
	w := &a.webFiles
	w.mu.Lock()
	s.users--
	a.removeWebFileLocked(s)
	w.mu.Unlock()
}

func (a *App) removeWebFileLocked(s *webFileStage) {
	w := &a.webFiles
	if s.discard && !s.active && s.users == 0 {
		// On Windows a completion event may precede the engine's deferred Close.
		// Retain the record on failure; the next sweep retries the removal.
		if os.RemoveAll(s.dir) == nil {
			delete(w.stages, s.ID)
		}
	}
	if w.closed && len(w.stages) == 0 && w.root != "" {
		_ = os.Remove(w.root)
	}
}

func (a *App) webFileSnapshot(s *webFileStage) WebFileStage {
	a.webFiles.mu.Lock()
	defer a.webFiles.mu.Unlock()
	return s.WebFileStage
}

func (a *App) dismissWebFile(id string) {
	w := &a.webFiles
	w.mu.Lock()
	s := w.stages[id]
	if s == nil {
		w.mu.Unlock()
		return
	}
	s.discard = true
	if s.cancel != nil {
		s.cancel()
	}
	task, active, cancelTask := s.TaskID, s.active, s.cancelTask
	a.removeWebFileLocked(s)
	w.mu.Unlock()
	if active && task != "" && cancelTask != nil {
		_ = cancelTask(task)
	}
}

func (a *App) sweepWebFiles(now time.Time) {
	w := &a.webFiles
	w.mu.Lock()
	ids := []string{}
	for id, s := range w.stages {
		if now.After(s.expires) || s.discard {
			ids = append(ids, id)
		}
	}
	root := w.root
	w.mu.Unlock()
	if root != "" {
		_ = os.Chtimes(root, now, now)
	}
	for _, id := range ids {
		a.dismissWebFile(id)
	}
}

// Call after the session's engine has closed, and at app shutdown after closing
// the Web listener. Neither browser refresh nor an SSE disconnect closes a PTY.
func (a *App) closeWebFileSession(sessionID string) {
	w := &a.webFiles
	w.mu.Lock()
	var clients []string
	for _, r := range w.zmodem {
		if r.SessionID == sessionID {
			clients = append(clients, r.ClientID)
		}
	}
	delete(w.zTargets, sessionID)
	var ids []string
	for _, s := range w.stages {
		if s.SessionID == sessionID {
			ids = append(ids, s.ID)
		}
	}
	w.mu.Unlock()
	for _, client := range clients {
		a.WebZmodemCancel(sessionID, client)
	}
	for _, id := range ids {
		a.dismissWebFile(id)
	}
}

func (a *App) closeWebFiles() {
	w := &a.webFiles
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	var requests []WebZmodemChoice
	for _, r := range w.zmodem {
		r.cancel()
		requests = append(requests, r.WebZmodemChoice)
	}
	if w.stop != nil {
		close(w.stop)
	}
	ids := []string{}
	for id := range w.stages {
		ids = append(ids, id)
	}
	w.mu.Unlock()
	for _, r := range requests {
		a.WebZmodemCancel(r.SessionID, r.ClientID)
	}
	for _, id := range ids {
		a.dismissWebFile(id)
	}
	// CancelTransfer is asynchronous. Keep scratch files until engines and HTTP
	// readers have released them, including completion-before-Close on Windows.
	deadline := time.Now().Add(5 * time.Second)
	for {
		w.mu.Lock()
		for _, s := range w.stages {
			a.removeWebFileLocked(s)
		}
		done := len(w.stages) == 0
		if done && w.root != "" {
			_ = os.Remove(w.root)
		}
		w.mu.Unlock()
		if done || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (a *App) serveWebFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == "/web/files/upload" && r.Method == http.MethodPost {
		a.webFileUpload(w, r)
		return
	}
	if r.URL.Path == "/web/files/export" && r.Method == http.MethodPost {
		a.webFileTextExport(w, r)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/web/files/")
	download := strings.HasSuffix(id, "/download")
	if download {
		id = strings.TrimSuffix(id, "/download")
	}
	s, err := a.acquireWebFile(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer a.releaseWebFile(s)
	switch {
	case r.Method == http.MethodDelete && !download:
		a.dismissWebFile(id)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && !download:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(a.webFileSnapshot(s))
	case (r.Method == http.MethodGet || r.Method == http.MethodHead) && download:
		a.webFileDownload(w, r, s)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Paths are multipart field names; Go intentionally strips directories from
// Part.FileName. No whole-file ReadAll, base64, or multipart.ParseForm spill.
func (a *App) webFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.ContentLength > webFileMaxUpload {
		http.Error(w, "upload exceeds 2 GiB", http.StatusRequestEntityTooLarge)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, webFileMaxUpload)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s, err := a.newWebFileStage("uploading", "")
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	defer a.releaseWebFile(s)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stop()
	a.webFiles.mu.Lock()
	s.cancel = cancel
	a.webFiles.mu.Unlock()
	files := []string{}
	for count := 0; ; count++ {
		var partErr error
		p, nextErr := mr.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			err = nextErr
			break
		}
		if count >= 10000 {
			err = errors.New("too many file entries")
			break
		}
		rel := p.FormName()
		_, params, _ := mime.ParseMediaType(p.Header.Get("Content-Disposition"))
		if name := params["filename"]; name != "" {
			if webFileRelative(name) != nil || path.Base(name) != name {
				err = errors.New("invalid upload filename")
				break
			}
		}
		isDir := strings.HasSuffix(rel, "/")
		if isDir {
			rel = strings.TrimSuffix(rel, "/")
		}
		if partErr = webFileRelative(rel); partErr != nil {
			err = partErr
			break
		}
		if ctx.Err() != nil {
			err = ctx.Err()
			break
		}
		dest := filepath.Join(s.dir, filepath.FromSlash(rel))
		if isDir {
			partErr = os.MkdirAll(dest, 0700)
		} else if partErr = os.MkdirAll(filepath.Dir(dest), 0700); partErr == nil {
			var f *os.File
			f, partErr = os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if partErr == nil {
				_, partErr = io.Copy(f, &webFileReader{ctx, p})
				if e := f.Close(); partErr == nil {
					partErr = e
				}
			}
		}
		if partErr != nil {
			err = partErr
			break
		}
		files = append(files, rel)
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil && len(files) == 0 {
		err = errors.New("empty upload")
	}
	if err != nil {
		a.dismissWebFile(s.ID)
		code := http.StatusBadRequest
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			code = http.StatusRequestEntityTooLarge
		}
		http.Error(w, err.Error(), code)
		return
	}
	a.webFiles.mu.Lock()
	s.State, s.Files, s.cancel = "ready", files, nil
	a.webFiles.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.webFileSnapshot(s))
}

type webFileReader struct {
	ctx context.Context
	io.Reader
}

// Text exports already live in the editor/query result. Accept ordinary HTTP
// bytes instead of wrapping them in base64/JSON or a second browser-sized Blob.
func (a *App) webFileTextExport(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if webFileRelative(name) != nil || path.Base(name) != name {
		http.Error(w, "invalid export filename", 400)
		return
	}
	if r.ContentLength > webFileMaxUpload {
		http.Error(w, "export exceeds 2 GiB", 413)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, webFileMaxUpload)
	s, err := a.newWebFileStage("uploading", "")
	if err != nil {
		http.Error(w, err.Error(), 503)
		return
	}
	defer a.releaseWebFile(s)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = r.Body.Close() })
	defer stop()
	a.webFiles.mu.Lock()
	s.cancel = cancel
	a.webFiles.mu.Unlock()
	target := filepath.Join(s.dir, name)
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = io.Copy(out, &webFileReader{ctx, r.Body})
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		a.dismissWebFile(s.ID)
		http.Error(w, err.Error(), 400)
		return
	}
	a.webFiles.mu.Lock()
	s.State, s.target, s.Files, s.cancel = "ready", target, []string{name}, nil
	a.webFiles.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a.webFileSnapshot(s))
}

func (r *webFileReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func (a *App) webFileDownload(w http.ResponseWriter, r *http.Request, s *webFileStage) {
	a.webFiles.mu.Lock()
	state, target := s.State, s.target
	a.webFiles.mu.Unlock()
	if state != "ready" || target == "" {
		http.Error(w, "download is not ready", http.StatusConflict)
		return
	}
	f, err := os.Open(target)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	name := st.Name()
	if st.IsDir() {
		name += ".zip"
	}
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	if !st.IsDir() {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, name, st.ModTime(), f)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	if r.Method == http.MethodHead {
		return
	}
	z := zip.NewWriter(w)
	err = filepath.WalkDir(target, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := r.Context().Err(); err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("symlinks cannot be downloaded as an archive")
		}
		if p == target {
			return nil
		}
		rel, err := filepath.Rel(filepath.Dir(target), p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if err := webFileRelative(name); err != nil {
			return err
		}
		if entry.IsDir() {
			name += "/"
		}
		out, err := z.Create(name)
		if err != nil || entry.IsDir() {
			return err
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		_, err = io.Copy(out, &webFileReader{r.Context(), in})
		return err
	})
	if err != nil {
		panic(http.ErrAbortHandler)
	} // abort rather than a valid-looking partial ZIP
	if err := z.Close(); err != nil {
		panic(http.ErrAbortHandler)
	}
}

// Start events carry the actual host path, so even synchronous start+complete
// events before Get/Put returns are correlated without locking around engines.
// Task IDs alone are not unique across sessions (notably S3's dl-1/ul-1).
func (a *App) webFileTransferEvent(sid string, p map[string]any) {
	w := &a.webFiles
	w.mu.Lock()
	defer w.mu.Unlock()
	task, _ := p["taskId"].(string)
	event, _ := p["event"].(string)
	lp, _ := p["localPath"].(string)
	for _, s := range w.stages {
		if s.SessionID != sid {
			continue
		}
		if event == "start" && s.target == lp {
			s.TaskID, s.State, s.Error, s.active = task, "transferring", "", true
		}
		if task == "" || s.TaskID != task {
			continue
		}
		s.expires = time.Now().Add(webFileTTL)
		if event == "complete" {
			s.active = false
			status, _ := p["status"].(string)
			s.State = status
			if status == "done" {
				s.State = "ready"
			}
			s.Error, _ = p["error"].(string)
			if status == "error" && s.Error == "" {
				s.Error = "transfer failed; inspect the existing transfer task"
			}
			// Keep failed inputs for the existing RetryTransfer path.
			a.removeWebFileLocked(s)
		}
	}
}

func (a *App) WebFilesPut(id, relative, sessionID, destination string) (WebFileStage, error) {
	s, err := a.acquireWebFile(id)
	if err != nil {
		return WebFileStage{}, err
	}
	defer a.releaseWebFile(s)
	if err := webFileRelative(relative); err != nil {
		return WebFileStage{}, err
	}
	target := filepath.Join(s.dir, filepath.FromSlash(relative))
	st, err := os.Stat(target)
	if err != nil {
		return WebFileStage{}, err
	}
	f, err := a.getSftp(sessionID)
	if err != nil {
		return WebFileStage{}, err
	}
	return a.startWebFileTransfer(s, sessionID, target, f.CancelTransfer, func() (string, error) { return f.Put(target, destination, st.IsDir()) })
}

func (a *App) WebFilesGet(sessionID, remotePath, name string, recursive bool) (WebFileStage, error) {
	if err := webFileRelative(name); err != nil || path.Base(name) != name {
		return WebFileStage{}, errors.New("invalid download filename")
	}
	f, err := a.getSftp(sessionID)
	if err != nil {
		return WebFileStage{}, err
	}
	s, err := a.newWebFileStage("ready", sessionID)
	if err != nil {
		return WebFileStage{}, err
	}
	defer a.releaseWebFile(s)
	target := filepath.Join(s.dir, name)
	result, err := a.startWebFileTransfer(s, sessionID, target, f.CancelTransfer, func() (string, error) { return f.Get(remotePath, target, recursive) })
	if err != nil {
		a.dismissWebFile(s.ID)
	}
	return result, err
}

func (a *App) startWebFileTransfer(s *webFileStage, sid, target string, cancelTask func(string) error, start func() (string, error)) (WebFileStage, error) {
	w := &a.webFiles
	w.mu.Lock()
	if s.State != "ready" || s.TaskID != "" || s.discard {
		w.mu.Unlock()
		return WebFileStage{}, errors.New("stage is already in use")
	}
	s.SessionID, s.target, s.active, s.State = sid, target, true, "transferring"
	s.cancelTask = cancelTask
	w.mu.Unlock()
	task, err := start()
	w.mu.Lock()
	if err != nil {
		s.State, s.Error, s.active = "error", err.Error(), false
	}
	s.TaskID = task
	discard := s.discard
	result := s.WebFileStage
	w.mu.Unlock()
	if discard {
		a.dismissWebFile(s.ID)
	}
	return result, err
}

// Existing parsers and explicit host operations take paths. Resolve only a
// completed stage; callers dismiss it after their synchronous consumer returns.
func (a *App) WebFilesPath(id, relative string) (string, error) {
	s, err := a.acquireWebFile(id)
	if err != nil {
		return "", err
	}
	defer a.releaseWebFile(s)
	if a.webFileSnapshot(s).State != "ready" {
		return "", errors.New("stage is not ready")
	}
	if err := webFileRelative(relative); err != nil {
		return "", err
	}
	p := filepath.Join(s.dir, filepath.FromSlash(relative))
	_, err = os.Stat(p)
	return p, err
}

// Saved config references must outlive staging, refresh, and shutdown. These
// private host copies are deliberately separate from the temporary root.
func (a *App) WebFilesPersist(id, relative string) (string, error) {
	s, err := a.acquireWebFile(id)
	if err != nil {
		return "", err
	}
	defer a.releaseWebFile(s)
	src, err := a.WebFilesPath(id, relative)
	if err != nil {
		return "", err
	}
	if a.dataDir == "" {
		return "", errors.New("data directory not initialized")
	}
	dir := filepath.Join(a.dataDir, "web-files")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	dir, err = os.MkdirTemp(dir, "saved-*")
	if err != nil {
		return "", err
	}
	dest := filepath.Join(dir, filepath.Base(src))
	if st, statErr := os.Stat(src); statErr == nil && st.IsDir() {
		if err := os.CopyFS(dest, os.DirFS(src)); err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
		return dest, nil
	}
	in, err := os.Open(src)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, err = io.Copy(out, in)
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dest, nil
}

func (a *App) WebFilesExportConnections(password string) (WebFileStage, error) {
	s, err := a.newWebFileStage("preparing", "")
	if err != nil {
		return WebFileStage{}, err
	}
	defer a.releaseWebFile(s)
	target := filepath.Join(s.dir, "uniterm-connections.utm")
	if err := a.ExportConnections(target, password); err != nil {
		a.dismissWebFile(s.ID)
		return WebFileStage{}, err
	}
	a.webFiles.mu.Lock()
	s.target, s.State = target, "ready"
	a.webFiles.mu.Unlock()
	return a.webFileSnapshot(s), nil
}

func (a *App) WebFilesHostDownload(hostPath string) (WebFileStage, error) {
	if !filepath.IsAbs(hostPath) {
		return WebFileStage{}, errors.New("expected an explicit host path")
	}
	if _, err := os.Stat(hostPath); err != nil {
		return WebFileStage{}, err
	}
	s, err := a.newWebFileStage("ready", "")
	if err != nil {
		return WebFileStage{}, err
	}
	defer a.releaseWebFile(s)
	a.webFiles.mu.Lock()
	s.target = hostPath
	a.webFiles.mu.Unlock()
	return a.webFileSnapshot(s), nil
}

func (a *App) WebFilesCopyToHost(id, relative, sessionID, destination string) error {
	s, err := a.acquireWebFile(id)
	if err != nil {
		return err
	}
	defer a.releaseWebFile(s)
	src, err := a.WebFilesPath(id, relative)
	if err != nil {
		return err
	}
	f, err := a.getSftp(sessionID)
	if err != nil {
		return err
	}
	return f.LocalCopy(src, destination)
}

func (a *App) dismissWebFileTask(sid, taskID string) {
	a.webFiles.mu.Lock()
	ids := []string{}
	for id, s := range a.webFiles.stages {
		if s.SessionID == sid && s.TaskID == taskID {
			ids = append(ids, id)
		}
	}
	a.webFiles.mu.Unlock()
	for _, id := range ids {
		a.dismissWebFile(id)
	}
}
