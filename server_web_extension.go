//go:build server

package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
)

// RemoteFileTransferSession defines the interface that uniTerm's backend/session
// SFTP/SCP session manager satisfies (or can be wrapped with via SessionResolver).
type RemoteFileTransferSession interface {
	// OpenFileReader opens a remote file for streaming read and returns its reader, size, and filename.
	OpenFileReader(ctx context.Context, remotePath string) (io.ReadCloser, int64, error)
	// OpenFileWriter opens/creates a remote file for streaming write.
	OpenFileWriter(ctx context.Context, remotePath string, mode os.FileMode, size int64) (io.WriteCloser, error)
	// WalkRemoteDir walks a remote directory recursively for archive streaming.
	WalkRemoteDir(ctx context.Context, remoteRoot string, fn func(relPath string, info os.FileInfo, openReader func() (io.ReadCloser, error)) error) error
	// MkdirAll creates remote directories recursively.
	MkdirAll(ctx context.Context, remoteDir string) error
}

// SessionResolverFunc resolves an active uniTerm session ID to a RemoteFileTransferSession.
type SessionResolverFunc func(sessionID string) (RemoteFileTransferSession, error)

// TransferProgressPayload is emitted via Wails Events ("web:sftp:progress") during browser streaming.
type TransferProgressPayload struct {
	TransferID       string `json:"transferId"`
	SessionID        string `json:"sessionId"`
	Direction        string `json:"direction"` // "upload" | "download"
	FileName         string `json:"fileName"`
	RemotePath       string `json:"remotePath"`
	BytesTransferred int64  `json:"bytesTransferred"`
	TotalBytes       int64  `json:"totalBytes"`
	SpeedBps         int64  `json:"speedBps"`
	Status           string `json:"status"` // "running" | "completed" | "cancelled" | "error"
	Error            string `json:"error,omitempty"`
}

// WebStreamService implements a Wails v3 Service + http.Handler mounted at "/api/web".
// It provides zero-disk-buffering browser SFTP/SCP streaming upload/download and
// safe file staging for replacing native desktop file pickers in Server Mode.
type WebStreamService struct {
	app             *application.App
	unitermApp      *App
	resolveSession  SessionResolverFunc
	activeTransfers sync.Map // map[string]context.CancelFunc
	stagingDir      string
}

// NewWebStreamService creates the WebStreamService for Wails v3 server mode.
func NewWebStreamService(unitermApp *App, resolver SessionResolverFunc) *WebStreamService {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		homeDir = "/tmp"
	}
	stagingDir := filepath.Join(homeDir, ".cache", "uniterm-web-staging")
	_ = os.MkdirAll(stagingDir, 0o700)

	s := &WebStreamService{
		unitermApp:     unitermApp,
		resolveSession: resolver,
		stagingDir:     stagingDir,
	}
	go s.cleanupStagingLoop()
	return s
}

// cleanupStagingLoop removes staged files left behind by crashed uploads (and
// anything older than a day) so the staging directory cannot grow without
// bound. Staged files have no handle to invalidate — anything already wired to
// a staged path is expected to be consumed promptly.
func (s *WebStreamService) cleanupStagingLoop() {
	purge := func() {
		entries, err := os.ReadDir(s.stagingDir)
		if err != nil {
			return
		}
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, e := range entries {
			info, err := e.Info()
			if err != nil || info.IsDir() {
				continue
			}
			if info.ModTime().Before(cutoff) {
				_ = os.Remove(filepath.Join(s.stagingDir, e.Name()))
			}
		}
	}
	purge()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		purge()
	}
}

// registerWebStreamService mounts WebStreamService at /api/web for the
// headless server build.
func registerWebStreamService(w3app *application.App, app *App) {
	w3app.RegisterService(application.NewServiceWithOptions(
		NewWebStreamService(app, app.resolveFileTransferSession),
		application.ServiceOptions{Route: "/api/web"},
	))
}

// resolveFileTransferSession maps an active session ID to a streaming
// adapter. Only SFTP exposes a random-access client; other session types
// keep their desktop transfer pipeline. Deliberately unexported so the Wails
// bindings generator does not try to emit a TS type for the interface return.
func (a *App) resolveFileTransferSession(sessionID string) (RemoteFileTransferSession, error) {
	sess, ok := a.sessionManager.Get(sessionID)
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	sftpSession, ok := sess.(*session.SFTPSession)
	if !ok {
		return nil, fmt.Errorf("session %s (%s) does not support browser streaming (SFTP only)", sessionID, sess.Type())
	}
	return sftpSession, nil
}

// ServiceName returns the Wails v3 service identifier.
func (s *WebStreamService) ServiceName() string {
	return "uniTerm Web Stream Service"
}

// ServiceStartup is invoked by Wails v3 when the application starts.
func (s *WebStreamService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.app = application.Get()
	return nil
}

// ServiceShutdown cleans up active transfers and temporary staged files on exit.
func (s *WebStreamService) ServiceShutdown() error {
	s.activeTransfers.Range(func(key, value any) bool {
		if cancel, ok := value.(context.CancelFunc); ok {
			cancel()
		}
		return true
	})
	return nil
}

// serverMode lets shared (untagged) code branch on the -tags server build.
const serverMode = true

// BuildServerOptions constructs application.ServerOptions from environment variables.
func BuildServerOptions() application.ServerOptions {
	host := os.Getenv("WAILS_SERVER_HOST")
	if host == "" {
		host = "0.0.0.0"
	}
	port := 8080
	if p, err := strconv.Atoi(os.Getenv("WAILS_SERVER_PORT")); err == nil && p > 0 {
		port = p
	}

	var origins []string
	if raw := strings.TrimSpace(os.Getenv("UNITERM_ALLOWED_ORIGINS")); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				origins = append(origins, trimmed)
			}
		}
	}

	return application.ServerOptions{
		Host: host,
		Port: port,
		// Wails treats 0 as "fall back to its 30s default", while net/http only
		// installs a deadline for d > 0 — so a negative value is what actually
		// disables the timeout for large streaming SFTP transfers.
		ReadTimeout:             -1,
		WriteTimeout:            -1,
		IdleTimeout:             180 * time.Second,
		ShutdownTimeout:         30 * time.Second,
		WebSocketOriginPatterns: origins,
	}
}

// lastWebActivityNano records when the app last emitted an event to the
// frontend (terminal output included). The multi-user gateway's idle reaper
// polls /api/web/activity for it, because WebSocket-only traffic — a user
// watching tail -f or an AI stream — produces no proxied HTTP requests to
// refresh the gateway-side timestamp.
var lastWebActivityNano atomic.Int64

// touchWebActivity marks a frontend-visible event as web activity. Called
// from App.emit; a no-op stub exists in the desktop build.
func touchWebActivity() {
	lastWebActivityNano.Store(time.Now().UnixNano())
}

// ServeHTTP routes "/api/web/*" requests.
func (s *WebStreamService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Shared-secret check: when the gateway spawned this worker it passed a
	// per-worker random token via UNITERM_WORKER_TOKEN and injects it on every
	// proxied request. Without it, any process inside the container could talk
	// to this loopback port and drive another tenant's session.
	if want := os.Getenv("UNITERM_WORKER_TOKEN"); want != "" {
		got := r.Header.Get("X-Uniterm-Worker-Token")
		if subtle.ConstantTimeCompare([]byte(got), []byte(want)) != 1 {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	// Enforce same-origin / gateway header check
	w.Header().Set("X-Content-Type-Options", "nosniff")

	subPath := strings.TrimPrefix(r.URL.Path, "/api/web")
	switch {
	case subPath == "/mode" && r.Method == http.MethodGet:
		s.handleModeInfo(w, r)
	case subPath == "/credentials/status" && r.Method == http.MethodGet:
		s.handleCredentialsStatus(w, r)
	case subPath == "/credentials/rekey" && r.Method == http.MethodPost:
		s.handleCredentialsRekey(w, r)
	case subPath == "/credentials/reset" && r.Method == http.MethodPost:
		s.handleCredentialsReset(w, r)
	case subPath == "/activity" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"lastActivityNano": lastWebActivityNano.Load()})
	case subPath == "/sftp/download" && r.Method == http.MethodGet:
		s.handleSFTPDownload(w, r)
	case subPath == "/sftp/download-archive" && r.Method == http.MethodGet:
		s.handleSFTPDownloadArchive(w, r)
	case subPath == "/sftp/upload" && r.Method == http.MethodPost:
		s.handleSFTPUpload(w, r)
	case subPath == "/sftp/cancel" && r.Method == http.MethodPost:
		s.handleSFTPCancel(w, r)
	case subPath == "/dialog/stage-file" && r.Method == http.MethodPost:
		s.handleStageLocalFile(w, r)
	default:
		http.Error(w, "Not Found", http.StatusNotFound)
	}
}

// handleModeInfo returns the current runtime mode and isolated tenant user information.
func (s *WebStreamService) handleModeInfo(w http.ResponseWriter, r *http.Request) {
	tenantUser := os.Getenv("UNITERM_TENANT_USER")
	if headerUser := r.Header.Get("X-Uniterm-User"); headerUser != "" {
		tenantUser = headerUser
	}
	if tenantUser == "" {
		tenantUser = "default"
	}
	homeDir, _ := os.UserHomeDir()
	writeJSON(w, http.StatusOK, map[string]any{
		"serverMode": true,
		"user":       tenantUser,
		"role":       r.Header.Get("X-Uniterm-Role"),
		"homeDir":    homeDir,
	})
}

// handleSFTPDownload streams a single remote file from SFTP/SCP directly to the browser.
// Query params: ?sessionId=<id>&path=<remotePath>&transferId=<optional>
func (s *WebStreamService) handleSFTPDownload(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	remotePath := strings.TrimSpace(r.URL.Query().Get("path"))
	transferID := strings.TrimSpace(r.URL.Query().Get("transferId"))
	if sessionID == "" || remotePath == "" {
		http.Error(w, "missing sessionId or path query parameter", http.StatusBadRequest)
		return
	}
	if transferID == "" {
		transferID = newTransferID()
	}

	if s.resolveSession == nil {
		http.Error(w, "SFTP session resolver is not configured", http.StatusServiceUnavailable)
		return
	}
	sess, err := s.resolveSession(sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("session lookup failed: %v", err), http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	s.activeTransfers.Store(transferID, cancel)
	defer func() {
		s.activeTransfers.Delete(transferID)
		cancel()
	}()

	reader, fileSize, err := sess.OpenFileReader(ctx, remotePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to open remote file: %v", err), http.StatusBadGateway)
		return
	}
	defer reader.Close()

	fileName := path.Base(remotePath)
	if fileName == "" || fileName == "." || fileName == "/" {
		fileName = "download.bin"
	}

	// Disable proxy buffering so the browser download bar shows real-time progress immediately
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", formatContentDisposition(fileName))
	if fileSize >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	s.streamWithProgress(ctx, w, reader, TransferProgressPayload{
		TransferID: transferID,
		SessionID:  sessionID,
		Direction:  "download",
		FileName:   fileName,
		RemotePath: remotePath,
		TotalBytes: fileSize,
	})
}

// handleSFTPDownloadArchive streams one or more remote files/directories as an on-the-fly .tar.gz archive.
// Query params: ?sessionId=<id>&paths=<jsonArrayOrCommaSeparated>&name=<archiveName>
func (s *WebStreamService) handleSFTPDownloadArchive(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	rawPaths := strings.TrimSpace(r.URL.Query().Get("paths"))
	archiveName := strings.TrimSpace(r.URL.Query().Get("name"))
	if sessionID == "" || rawPaths == "" {
		http.Error(w, "missing sessionId or paths parameter", http.StatusBadRequest)
		return
	}

	var remotePaths []string
	if strings.HasPrefix(rawPaths, "[") {
		if err := json.Unmarshal([]byte(rawPaths), &remotePaths); err != nil {
			http.Error(w, "invalid paths JSON array", http.StatusBadRequest)
			return
		}
	} else {
		remotePaths = strings.Split(rawPaths, ",")
	}

	if archiveName == "" {
		archiveName = fmt.Sprintf("uniterm-files-%s.tar.gz", time.Now().Format("20060102-150405"))
	} else if !strings.HasSuffix(archiveName, ".tar.gz") {
		archiveName += ".tar.gz"
	}

	sess, err := s.resolveSession(sessionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("session not found: %v", err), http.StatusNotFound)
		return
	}

	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", formatContentDisposition(archiveName))

	gzWriter := gzip.NewWriter(w)
	defer gzWriter.Close()
	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	ctx := r.Context()
	buf := make([]byte, 64*1024)

	transferID := newTransferID()
	for _, targetPath := range remotePaths {
		targetPath = strings.TrimSpace(targetPath)
		if targetPath == "" {
			continue
		}
		// The tar stream is already committed to the response by the time a
		// walk fails, so the status code can no longer change; surface the
		// failure through the log and the progress-event channel instead of
		// silently delivering a truncated archive.
		if err := sess.WalkRemoteDir(ctx, targetPath, func(relPath string, info os.FileInfo, openReader func() (io.ReadCloser, error)) error {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(relPath)
			if info.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
				hdr.Name += "/"
			}
			if err := tarWriter.WriteHeader(hdr); err != nil {
				return err
			}
			if info.IsDir() || !info.Mode().IsRegular() {
				return nil
			}
			rc, err := openReader()
			if err != nil {
				return err
			}
			defer rc.Close()
			_, err = io.CopyBuffer(tarWriter, rc, buf)
			return err
		}); err != nil {
			log.Printf("[web-stream] archive walk %s failed: %v", targetPath, err)
			s.emitProgress(TransferProgressPayload{
				TransferID: transferID,
				SessionID:  sessionID,
				Direction:  "download",
				FileName:   archiveName,
				RemotePath: targetPath,
				Status:     "error",
				Error:      fmt.Sprintf("archive walk %s failed: %v", targetPath, err),
			})
		}
	}
}

// handleSFTPUpload streams multipart/form-data files directly from the browser into the remote SFTP/SCP session.
// Query params: ?sessionId=<id>&remoteDir=<path>&transferId=<optional>
func (s *WebStreamService) handleSFTPUpload(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimSpace(r.URL.Query().Get("sessionId"))
	remoteDir := strings.TrimSpace(r.URL.Query().Get("remoteDir"))
	transferID := strings.TrimSpace(r.URL.Query().Get("transferId"))
	if transferID == "" {
		transferID = strings.TrimSpace(r.Header.Get("X-Transfer-ID"))
	}
	if transferID == "" {
		transferID = newTransferID()
	}
	if sessionID == "" || remoteDir == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing sessionId or remoteDir"})
		return
	}

	if s.resolveSession == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "SFTP session resolver is not configured"})
		return
	}
	sess, err := s.resolveSession(sessionID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": fmt.Sprintf("session lookup failed: %v", err)})
		return
	}

	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("expected multipart/form-data: %v", err)})
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	s.activeTransfers.Store(transferID, cancel)
	defer func() {
		s.activeTransfers.Delete(transferID)
		cancel()
	}()

	var uploadedFiles []string
	var currentRelativePath string

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.emitProgress(TransferProgressPayload{
				TransferID: transferID,
				SessionID:  sessionID,
				Direction:  "upload",
				Status:     "error",
				Error:      err.Error(),
			})
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("multipart stream error: %v", err)})
			return
		}

		formName := part.FormName()
		// Support webkitRelativePath metadata field sent before a file part for directory drag-and-drop
		if formName == "relativePath" {
			data, _ := io.ReadAll(io.LimitReader(part, 4096))
			currentRelativePath = sanitizeRelativeUploadPath(string(data))
			_ = part.Close()
			continue
		}

		fileName := part.FileName()
		if fileName == "" {
			_ = part.Close()
			continue
		}

		relDest := currentRelativePath
		currentRelativePath = ""
		if relDest == "" {
			relDest = sanitizeRelativeUploadPath(fileName)
		}
		if relDest == "" {
			_ = part.Close()
			continue
		}

		targetRemotePath := path.Join(remoteDir, relDest)
		targetParentDir := path.Dir(targetRemotePath)
		if targetParentDir != "" && targetParentDir != "." && targetParentDir != remoteDir {
			if err := sess.MkdirAll(ctx, targetParentDir); err != nil {
				_ = part.Close()
				writeJSON(w, http.StatusBadGateway, map[string]any{"error": fmt.Sprintf("mkdir %s failed: %v", targetParentDir, err)})
				return
			}
		}

		var declaredSize int64 = -1
		if szStr := part.Header.Get("X-File-Size"); szStr != "" {
			if parsed, err := strconv.ParseInt(szStr, 10, 64); err == nil {
				declaredSize = parsed
			}
		}

		remoteWriter, err := sess.OpenFileWriter(ctx, targetRemotePath, 0o644, declaredSize)
		if err != nil {
			_ = part.Close()
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": fmt.Sprintf("open remote writer %s failed: %v", targetRemotePath, err)})
			return
		}

		ok := s.streamWithProgress(ctx, remoteWriter, part, TransferProgressPayload{
			TransferID: transferID,
			SessionID:  sessionID,
			Direction:  "upload",
			FileName:   path.Base(targetRemotePath),
			RemotePath: targetRemotePath,
			TotalBytes: declaredSize,
		})
		_ = remoteWriter.Close()
		_ = part.Close()

		if !ok {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "upload interrupted or failed"})
			return
		}
		uploadedFiles = append(uploadedFiles, targetRemotePath)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"transferId":    transferID,
		"uploadedFiles": uploadedFiles,
	})
}

// handleSFTPCancel cancels an in-flight upload or download stream by transferId.
func (s *WebStreamService) handleSFTPCancel(w http.ResponseWriter, r *http.Request) {
	transferID := strings.TrimSpace(r.URL.Query().Get("transferId"))
	if transferID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing transferId"})
		return
	}
	if val, ok := s.activeTransfers.LoadAndDelete(transferID); ok {
		if cancel, ok := val.(context.CancelFunc); ok {
			cancel()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "cancelled", "transferId": transferID})
}

// handleStageLocalFile replaces native OS file pickers (app.Dialog.OpenFile) in Web Server Mode.
// It receives a file from the browser (e.g., an SSH private key, SQLite DB, or Navicat export)
// and stages it inside the user's isolated $HOME/.cache/uniterm-web-staging directory.
func (s *WebStreamService) handleStageLocalFile(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart request"})
		return
	}

	// keep=1 places the file under staging/keep/, which the 24h janitor
	// never touches: staged paths referenced by saved connection configs
	// (SSH keys, SQLite DBs, kubeconfigs) must outlive any single session,
	// while one-shot imports are fine with the janitored default area.
	keep := false
	var part io.ReadCloser
	var pickedName string
	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid multipart stream"})
			return
		}
		if p.FormName() == "keep" {
			data, _ := io.ReadAll(io.LimitReader(p, 8))
			keep = strings.TrimSpace(string(data)) == "1" || strings.EqualFold(strings.TrimSpace(string(data)), "true")
			_ = p.Close()
			continue
		}
		if p.FileName() != "" {
			part = p
			pickedName = p.FileName()
			break
		}
		_ = p.Close()
	}
	if part == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing file part"})
		return
	}
	defer part.Close()

	origName := filepath.Base(pickedName)
	if origName == "" || origName == "." || origName == "/" {
		origName = "staged-file.bin"
	}

	stagingDir := s.stagingDir
	if keep {
		stagingDir = filepath.Join(s.stagingDir, "keep")
		_ = os.MkdirAll(stagingDir, 0o700)
	}
	token := newTransferID()[:12]
	stagedFileName := fmt.Sprintf("%s_%s", token, origName)
	stagedPath := filepath.Join(stagingDir, stagedFileName)

	// Containment check
	cleanStaging := filepath.Clean(s.stagingDir) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(stagedPath), cleanStaging) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid file name"})
		return
	}

	outFile, err := os.OpenFile(stagedPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer outFile.Close()

	// Read one byte past the cap so an oversized upload is detected as an
	// error instead of being silently truncated into a valid-looking file.
	const stagingCap = 512 * 1024 * 1024 // 512 MiB cap for staged config/DB files
	written, err := io.Copy(outFile, io.LimitReader(part, stagingCap+1))
	if err != nil {
		_ = os.Remove(stagedPath)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if written > stagingCap {
		_ = os.Remove(stagedPath)
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
			"error": fmt.Sprintf("staged file exceeds the %d MiB staging cap", stagingCap/(1024*1024)),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status":     "ok",
		"stagedPath": stagedPath,
		"fileName":   origName,
		"size":       written,
	})
}

// streamWithProgress copies from src to dst in 64 KiB chunks while emitting throttled Wails progress events.
func (s *WebStreamService) streamWithProgress(ctx context.Context, dst io.Writer, src io.Reader, meta TransferProgressPayload) bool {
	buf := make([]byte, 64*1024)
	var transferred int64
	startTime := time.Now()
	lastEmit := time.Now()

	meta.Status = "running"
	s.emitProgress(meta)

	flusher, canFlush := dst.(http.Flusher)

	for {
		select {
		case <-ctx.Done():
			meta.BytesTransferred = atomic.LoadInt64(&transferred)
			meta.Status = "cancelled"
			meta.Error = ctx.Err().Error()
			s.emitProgress(meta)
			return false
		default:
		}

		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw > 0 {
				transferred += int64(nw)
			}
			if ew != nil {
				meta.BytesTransferred = transferred
				meta.Status = "error"
				meta.Error = ew.Error()
				s.emitProgress(meta)
				return false
			}
			if nr != nw {
				meta.BytesTransferred = transferred
				meta.Status = "error"
				meta.Error = io.ErrShortWrite.Error()
				s.emitProgress(meta)
				return false
			}
			if canFlush {
				flusher.Flush()
			}
		}

		now := time.Now()
		if now.Sub(lastEmit) >= 200*time.Millisecond {
			elapsedSec := now.Sub(startTime).Seconds()
			var speed int64
			if elapsedSec > 0 {
				speed = int64(float64(transferred) / elapsedSec)
			}
			meta.BytesTransferred = transferred
			meta.SpeedBps = speed
			meta.Status = "running"
			s.emitProgress(meta)
			lastEmit = now
		}

		if er != nil {
			if errors.Is(er, io.EOF) {
				break
			}
			meta.BytesTransferred = transferred
			meta.Status = "error"
			meta.Error = er.Error()
			s.emitProgress(meta)
			return false
		}
	}

	elapsedSec := time.Since(startTime).Seconds()
	if elapsedSec > 0 {
		meta.SpeedBps = int64(float64(transferred) / elapsedSec)
	}
	meta.BytesTransferred = transferred
	meta.Status = "completed"
	s.emitProgress(meta)
	return true
}

func (s *WebStreamService) emitProgress(payload TransferProgressPayload) {
	if s.app != nil && s.app.Event != nil {
		s.app.Event.Emit("web:sftp:progress", payload)
	}
}

// handleCredentialsStatus reports the credential store state so the gateway
// can decide whether a live rekey is possible before accepting a password
// change.
func (s *WebStreamService) handleCredentialsStatus(w http.ResponseWriter, _ *http.Request) {
	a := s.unitermApp
	if a == nil || a.credentialStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{"initialized": false, "unlocked": false, "needsSetup": true})
		return
	}
	st := a.credentialStore.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"initialized": !st.NeedsSetup,
		"unlocked":    st.Unlocked,
		"needsSetup":  st.NeedsSetup,
	})
}

// handleCredentialsRekey re-encrypts the tenant's secret stores under a new
// master password. Server mode ties the master password to the gateway login
// password, so the gateway calls this whenever a tenant changes theirs. The
// store must be unlocked: the gateway only offers password change from an
// active, unlocked browser session.
func (s *WebStreamService) handleCredentialsRekey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "oldPassword and newPassword are required"})
		return
	}
	a := s.unitermApp
	if err := a.ChangeMasterPassword(req.OldPassword, req.NewPassword); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleCredentialsReset wipes the tenant's credential store (the master
// password reset). The gateway invokes it as part of an admin password
// reset: the login password and the master password are one and the same in
// server mode, so resetting the login password destroys the data key.
func (s *WebStreamService) handleCredentialsReset(w http.ResponseWriter, _ *http.Request) {
	if err := s.unitermApp.ResetCredentials(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func formatContentDisposition(fileName string) string {
	escaped := url.PathEscape(fileName)
	return mime.FormatMediaType("attachment", map[string]string{
		"filename": fileName,
	}) + "; filename*=UTF-8''" + escaped
}

func sanitizeRelativeUploadPath(raw string) string {
	raw = strings.ReplaceAll(raw, "\\", "/")
	cleaned := path.Clean("/" + strings.TrimPrefix(raw, "/"))
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." || strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return cleaned
}

func newTransferID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
