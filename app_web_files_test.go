package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ys-ll/uniterm/backend/session"
)

func webFileTestApp(t *testing.T) (*App, *httptest.Server) {
	t.Helper()
	a := &App{dataDir: t.TempDir(), sessionManager: session.NewSessionManager()}
	mux := http.NewServeMux()
	a.registerWebFileRoutes(mux.Handle)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() { srv.Close(); a.closeWebFiles() })
	return a, srv
}

func uploadWebTest(t *testing.T, url string, files map[string][]byte) WebFileStage {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, data := range files {
		var out io.Writer
		var err error
		if strings.HasSuffix(name, "/") {
			out, err = mw.CreateFormField(name)
		} else {
			out, err = mw.CreateFormFile(name, filepath.Base(name))
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := out.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url+"/web/files/upload", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload: %d %s", resp.StatusCode, b)
	}
	var stage WebFileStage
	if err := json.NewDecoder(resp.Body).Decode(&stage); err != nil {
		t.Fatal(err)
	}
	return stage
}

func eventuallyWeb(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func webStageCount(a *App) int {
	a.webFiles.mu.Lock()
	defer a.webFiles.mu.Unlock()
	return len(a.webFiles.stages)
}

func TestWebFilesHTTPBytesLayoutPersistenceAndRange(t *testing.T) {
	a, srv := webFileTestApp(t)
	data := bytes.Repeat([]byte{0, 255, 1, 3, 7, 9, 13, 10}, 32768)
	stage := uploadWebTest(t, srv.URL, map[string][]byte{
		"profile/terminal/访客.sessions": data, "profile/user.config": []byte("sidecar"), "profile/empty/": nil,
	})
	if len(stage.ID) != 48 || strings.Contains(stage.ID, "/") {
		t.Fatalf("nonopaque stage: %+v", stage)
	}
	hostPath, err := a.WebFilesPath(stage.ID, "profile/terminal/访客.sessions")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(hostPath)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("bytes differ: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(filepath.Dir(hostPath)), "empty")); err != nil {
		t.Fatal(err)
	}
	if sidecar, _ := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(hostPath)), "user.config")); string(sidecar) != "sidecar" {
		t.Fatal("sidecar layout lost")
	}
	if _, err := a.WebFilesPath(stage.ID, "../escape"); err == nil {
		t.Fatal("accepted traversal")
	}
	persistent, err := a.WebFilesPersist(stage.ID, "profile/terminal/访客.sessions")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(persistent, filepath.Join(a.dataDir, "web-files")) {
		t.Fatal("not a durable host copy")
	}
	persistentDir, err := a.WebFilesPersist(stage.ID, "profile")
	if err != nil {
		t.Fatal(err)
	}
	download, err := a.WebFilesHostDownload(persistent)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", srv.URL+"/web/files/"+download.ID+"/download", nil)
	req.Header.Set("Range", "bytes=13-1036")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 206 || !bytes.Equal(b, data[13:1037]) {
		t.Fatal("range download differs")
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "attachment") {
		t.Fatal("not a download")
	}
	a.dismissWebFile(stage.ID)
	if _, err := os.Stat(hostPath); !os.IsNotExist(err) {
		t.Fatal("dismiss did not clean staging")
	}
	a.closeWebFiles()
	a.closeWebFiles()
	if _, err := os.Stat(a.webFiles.root); !os.IsNotExist(err) {
		t.Fatal("shutdown left scratch root")
	}
	if saved, err := os.ReadFile(persistent); err != nil || !bytes.Equal(saved, data) {
		t.Fatal("shutdown destroyed persistent config material")
	}
	if saved, err := os.ReadFile(filepath.Join(persistentDir, "terminal", "访客.sessions")); err != nil || !bytes.Equal(saved, data) {
		t.Fatal("persistent directory lost its layout")
	}
}

func TestWebFilesHTTPRejectsTraversalDuplicateAndLimits(t *testing.T) {
	a, srv := webFileTestApp(t)
	for _, name := range []string{"../escape", "/absolute", "C:/escape", "a/../../escape", "a\\b", "a//b", "a/./b", "CON.txt", "a/..", "trail.", "file:stream"} {
		t.Run(name, func(t *testing.T) {
			var body bytes.Buffer
			mw := multipart.NewWriter(&body)
			p, _ := mw.CreateFormFile(name, "test.bin")
			_, _ = p.Write([]byte("no"))
			_ = mw.Close()
			resp, err := http.Post(srv.URL+"/web/files/upload", mw.FormDataContentType(), &body)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.StatusCode != 400 {
				t.Fatalf("accepted %q: %d", name, resp.StatusCode)
			}
		})
	}
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for i := 0; i < 2; i++ {
		p, _ := mw.CreateFormFile("same", "same")
		_, _ = p.Write([]byte("bytes"))
	}
	_ = mw.Close()
	resp, err := http.Post(srv.URL+"/web/files/upload", mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatal("duplicate overwrote file")
	}
	req := httptest.NewRequest("POST", "/web/files/upload", strings.NewReader("ignored"))
	req.ContentLength = webFileMaxUpload + 1
	rw := httptest.NewRecorder()
	a.serveWebFiles(rw, req)
	if rw.Code != http.StatusRequestEntityTooLarge {
		t.Fatal("declared upload limit ignored")
	}
	eventuallyWeb(t, func() bool { return webStageCount(a) == 0 })
}

func TestWebFilesHTTPCancelCleansPartialUpload(t *testing.T) {
	a, srv := webFileTestApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)
	wrote := make(chan struct{})
	go func() {
		defer pw.Close()
		p, err := mw.CreateFormFile("partial.bin", "partial.bin")
		if err == nil {
			_, _ = p.Write(bytes.Repeat([]byte("x"), 65536))
		}
		close(wrote)
		<-ctx.Done()
	}()
	req, _ := http.NewRequestWithContext(ctx, "POST", srv.URL+"/web/files/upload", pr)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	result := make(chan error, 1)
	go func() {
		resp, err := http.DefaultClient.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		result <- err
	}()
	<-wrote
	eventuallyWeb(t, func() bool { return webStageCount(a) == 1 })
	cancel()
	if err := <-result; err == nil {
		t.Fatal("cancelled request succeeded")
	}
	eventuallyWeb(t, func() bool { return webStageCount(a) == 0 })
}

type webTestFileSession struct {
	session.Session
	fileTransferSession
	id       string
	put, get func(string, string, bool) (string, error)
	cancel   func(string) error
}

func (s *webTestFileSession) ID() string { return s.id }
func (s *webTestFileSession) Put(l, r string, recursive bool) (string, error) {
	return s.put(l, r, recursive)
}
func (s *webTestFileSession) Get(r, l string, recursive bool) (string, error) {
	return s.get(r, l, recursive)
}
func (s *webTestFileSession) CancelTransfer(id string) error {
	if s.cancel != nil {
		return s.cancel(id)
	}
	return nil
}

func TestWebFilesCompletionBeforeReturnAndTaskKeyIsolation(t *testing.T) {
	a, srv := webFileTestApp(t)
	data := []byte("real bytes \x00\xff 中文")
	upload := uploadWebTest(t, srv.URL, map[string][]byte{"访客.bin": data})
	f := &webTestFileSession{id: "one"}
	f.put = func(local, remote string, recursive bool) (string, error) {
		b, err := os.ReadFile(local)
		if err != nil || !bytes.Equal(b, data) || recursive || remote != "/dest.bin" {
			t.Fatal("Put arguments/bytes differ")
		}
		a.webFileTransferEvent("one", map[string]any{"event": "start", "taskId": "ul-1", "localPath": local})
		a.webFileTransferEvent("other-session", map[string]any{"event": "complete", "taskId": "ul-1", "status": "error"})
		a.webFileTransferEvent("one", map[string]any{"event": "complete", "taskId": "ul-1", "status": "done"})
		return "ul-1", nil
	}
	a.sessionManager.Add(f)
	result, err := a.WebFilesPut(upload.ID, "访客.bin", "one", "/dest.bin")
	if err != nil || result.State != "ready" || result.TaskID != "ul-1" {
		t.Fatalf("lost synchronous completion: %+v %v", result, err)
	}
	a.dismissWebFileTask("other-session", "ul-1")
	if webStageCount(a) != 1 {
		t.Fatal("cross-session task collision")
	}
	a.dismissWebFileTask("one", "ul-1")
	if webStageCount(a) != 0 {
		t.Fatal("dismiss did not release upload")
	}
}

func TestWebFilesDownloadWaitsForCompletionAndArchivesDirectory(t *testing.T) {
	a, srv := webFileTestApp(t)
	var target string
	f := &webTestFileSession{id: "download"}
	f.get = func(remote, local string, recursive bool) (string, error) {
		target = local
		if !recursive || remote != "/remote/dir" {
			t.Fatal("download arguments")
		}
		if err := os.MkdirAll(filepath.Join(local, "empty"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(local, "中文.bin"), []byte{0, 255, 12}, 0600); err != nil {
			t.Fatal(err)
		}
		a.webFileTransferEvent(f.id, map[string]any{"event": "start", "taskId": "dl-1", "localPath": local})
		return "dl-1", nil
	}
	a.sessionManager.Add(f)
	stage, err := a.WebFilesGet(f.id, "/remote/dir", "folder", true)
	if err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/web/files/" + stage.ID + "/download"
	resp, _ := http.Get(url)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatal("partial download exposed")
	}
	a.webFileTransferEvent(f.id, map[string]any{"event": "complete", "taskId": "dl-1", "status": "done"})
	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	z, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != 2 {
		t.Fatalf("archive entries: %d", len(z.File))
	}
	for _, entry := range z.File {
		if entry.Name == "folder/中文.bin" {
			r, _ := entry.Open()
			contents, _ := io.ReadAll(r)
			r.Close()
			if !bytes.Equal(contents, []byte{0, 255, 12}) {
				t.Fatal("zip bytes")
			}
		}
	}
	a.closeWebFileSession(f.id)
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("session did not release stage")
	}
}

func TestWebFilesFailureRetryCancellationTTLAndShutdown(t *testing.T) {
	a, srv := webFileTestApp(t)
	upload := uploadWebTest(t, srv.URL, map[string][]byte{"input": []byte("retry me")})
	var target string
	var cancelled atomic.Bool
	f := &webTestFileSession{id: "retry"}
	f.put = func(local, _ string, _ bool) (string, error) {
		target = local
		a.webFileTransferEvent(f.id, map[string]any{"event": "start", "taskId": "1", "localPath": local})
		a.webFileTransferEvent(f.id, map[string]any{"event": "complete", "taskId": "1", "status": "error", "error": "disk full"})
		return "1", nil
	}
	f.cancel = func(task string) error { cancelled.Store(true); return nil }
	a.sessionManager.Add(f)
	failed, err := a.WebFilesPut(upload.ID, "input", f.id, "/remote")
	if err != nil || failed.State != "error" || failed.Error != "disk full" {
		t.Fatalf("failure lost: %+v %v", failed, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("failed upload removed before retry")
	}
	// Existing RetryTransfer starts a new task with the same actual path.
	a.webFileTransferEvent(f.id, map[string]any{"event": "start", "taskId": "2", "localPath": target})
	a.sweepWebFiles(time.Now().Add(webFileTTL + time.Second))
	if !cancelled.Load() {
		t.Fatal("TTL did not cancel active task")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("deleted input while engine still active")
	}
	a.webFileTransferEvent(f.id, map[string]any{"event": "complete", "taskId": "2", "status": "cancelled"})
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("cancel completion did not remove stage")
	}
	_ = uploadWebTest(t, srv.URL, map[string][]byte{"shutdown": []byte("bytes")})
	a.closeWebFiles()
	if webStageCount(a) != 0 {
		t.Fatal("shutdown stages remain")
	}
	if _, err := a.newWebFileStage("ready", ""); err == nil {
		t.Fatal("stage created after shutdown")
	}
}

func TestWebFilesStartFailureAndDismissDuringStart(t *testing.T) {
	a, srv := webFileTestApp(t)
	f := &webTestFileSession{id: "start"}
	f.get = func(_, _ string, _ bool) (string, error) { return "", errors.New("permission denied") }
	a.sessionManager.Add(f)
	if _, err := a.WebFilesGet(f.id, "/denied", "file", false); err == nil {
		t.Fatal("lost start failure")
	}
	if webStageCount(a) != 0 {
		t.Fatal("failed Get leaked")
	}
	stage := uploadWebTest(t, srv.URL, map[string][]byte{"input": []byte("bytes")})
	var cancelled bool
	f.cancel = func(task string) error {
		cancelled = true
		a.webFileTransferEvent(f.id, map[string]any{"event": "complete", "taskId": task, "status": "cancelled"})
		return nil
	}
	f.put = func(local, _ string, _ bool) (string, error) {
		a.dismissWebFile(stage.ID) // before start returns its task ID
		a.webFileTransferEvent(f.id, map[string]any{"event": "start", "taskId": "late", "localPath": local})
		return "late", nil
	}
	_, err := a.WebFilesPut(stage.ID, "input", f.id, "/remote")
	if err != nil || !cancelled || webStageCount(a) != 0 {
		t.Fatalf("late start escaped cancellation: %v", err)
	}
}

// Opt in against a real OpenSSH internal-sftp, not an in-process protocol fake.
func TestWebFilesLiveSFTP(t *testing.T) {
	key := os.Getenv("UNITERM_WEB_FILES_SFTP_KEY")
	root := os.Getenv("UNITERM_WEB_FILES_SFTP_DIR")
	if key == "" || root == "" {
		t.Skip("set UNITERM_WEB_FILES_SFTP_KEY and UNITERM_WEB_FILES_SFTP_DIR for OpenSSH :33922")
	}
	a, srv := webFileTestApp(t)
	previous := session.TransferEventSink
	session.TransferEventSink = a.webFileTransferEvent
	defer func() { session.TransferEventSink = previous }()
	s := session.NewSFTPSession("real-sftp-web-files")
	if err := s.Connect(session.ConnectionConfig{Host: "127.0.0.1", Port: 33922, User: "zhongqian", AuthType: "key", KeyPath: key}); err != nil {
		t.Fatal(err)
	}
	a.sessionManager.Add(s)
	defer s.Disconnect()
	data := make([]byte, 256*1024)
	for i := range data {
		data[i] = byte(i*37 + i/257)
	}
	if source := os.Getenv("UNITERM_WEB_FILES_SOURCE"); source != "" {
		var err error
		data, err = os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
	}
	upload := uploadWebTest(t, srv.URL, map[string][]byte{"访客上传.bin": data})
	remote := filepath.ToSlash(filepath.Join(root, fmt.Sprintf("web-files-%d.bin", time.Now().UnixNano())))
	defer s.Remove(remote, false)
	up, err := a.WebFilesPut(upload.ID, "访客上传.bin", s.ID(), remote)
	if err != nil {
		t.Fatal(err)
	}
	wait := func(id string) {
		eventuallyWeb(t, func() bool {
			s, err := a.acquireWebFile(id)
			if err != nil {
				t.Fatal(err)
			}
			status := a.webFileSnapshot(s)
			a.releaseWebFile(s)
			if status.State == "error" {
				t.Fatal(status.Error)
			}
			return status.State == "ready"
		})
	}
	wait(up.ID)
	down, err := a.WebFilesGet(s.ID(), remote, "访客下载.bin", false)
	if err != nil {
		t.Fatal(err)
	}
	wait(down.ID)
	resp, err := http.Get(srv.URL + "/web/files/" + down.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || sha256.Sum256(got) != sha256.Sum256(data) {
		t.Fatalf("real SFTP HTTP round trip mismatch: %v", err)
	}
	t.Logf("OpenSSH SFTP → HTTP: %d bytes SHA256 %x", len(got), sha256.Sum256(got))
}

func TestWebFilesImportSidecarsReachExistingParsers(t *testing.T) {
	a, srv := webFileTestApp(t)
	key, _ := hex.DecodeString("babb4a9f774ab853c96c2d653dfe544a")
	block, _ := aes.NewCipher(key)
	plain := append(make([]byte, 16), []byte(`{"c":{"#connection":{"user":"user","password":"sidecar-secret"}}}`)...)
	padding := 16 - len(plain)%16
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	credentials := make([]byte, len(plain))
	cipher.NewCBCEncrypter(block, make([]byte, 16)).CryptBlocks(credentials, plain)
	s := uploadWebTest(t, srv.URL, map[string][]byte{
		"workspace/.dbeaver/data-sources.json":       []byte(`{"connections":{"c":{"name":"db","provider":"mysql","configuration":{"host":"example","port":"3306"}}}}`),
		"workspace/.dbeaver/credentials-config.json": credentials,
		"profile/user.config":                        []byte(`{"application.fingerprint":"fp","application.masterPassword":true}`),
		"profile/terminal/user.sessions":             []byte(`[]`),
	})
	path, err := a.WebFilesPath(s.ID, "workspace/.dbeaver/data-sources.json")
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.ParseImportFile("dbeaver", path, "")
	if err != nil || len(result.Connections) != 1 || result.Connections[0].Password != "sidecar-secret" {
		t.Fatalf("DBeaver lost credentials: %+v %v", result, err)
	}
	path, err = a.WebFilesPath(s.ID, "profile/terminal/user.sessions")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ParseImportFile("windterm", path, ""); err == nil {
		t.Fatal("WindTerm did not see user.config's master-password gate")
	}
}

func TestWebFilesTextExportHTTP(t *testing.T) {
	_, srv := webFileTestApp(t)
	text := strings.Repeat("select '中文';\n", 50000)
	resp, err := http.Post(srv.URL+"/web/files/export?name=query.sql", "text/plain;charset=utf-8", strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	var stage WebFileStage
	if err := json.NewDecoder(resp.Body).Decode(&stage); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = http.Get(srv.URL + "/web/files/" + stage.ID + "/download")
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || string(b) != text {
		t.Fatal("text export bytes changed", err)
	}
}

func TestWebFilesSessionCloseWaitsForEngineCancellation(t *testing.T) {
	a, srv := webFileTestApp(t)
	upload := uploadWebTest(t, srv.URL, map[string][]byte{"input": []byte("still in use")})
	f := &webTestFileSession{id: "close"}
	var target string
	f.put = func(local, _ string, _ bool) (string, error) {
		target = local
		a.webFileTransferEvent(f.id, map[string]any{"event": "start", "taskId": "running", "localPath": local})
		return "running", nil
	}
	var cancelled bool
	f.cancel = func(string) error { cancelled = true; return nil }
	a.sessionManager.Add(f)
	if _, err := a.WebFilesPut(upload.ID, "input", f.id, "/remote"); err != nil {
		t.Fatal(err)
	}
	// The SessionManager may already have removed the engine from its map.
	a.sessionManager = session.NewSessionManager()
	a.closeWebFileSession(f.id)
	if !cancelled {
		t.Fatal("closing session did not cancel its transfer")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal("session close deleted active input")
	}
	a.webFileTransferEvent(f.id, map[string]any{"event": "complete", "taskId": "running", "status": "cancelled"})
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("engine completion did not clean session files")
	}
}

func TestWebFilesZmodemBrokerTargetsAndCompletion(t *testing.T) {
	a, srv := webFileTestApp(t)
	stage := uploadWebTest(t, srv.URL, map[string][]byte{"visitor.bin": []byte{0, 255, 3}})
	if err := a.WebZmodemTarget("terminal", "browser-a", stage.ID); err != nil {
		t.Fatal(err)
	}
	choice, err := a.WebZmodemPick(context.Background(), "terminal", "upload")
	if err != nil || choice.ClientID != "browser-a" || len(choice.Paths) != 1 {
		t.Fatalf("choice: %+v %v", choice, err)
	}
	if b, _ := os.ReadFile(choice.Paths[0]); !bytes.Equal(b, []byte{0, 255, 3}) {
		t.Fatal("native parser did not receive staged bytes")
	}
	if _, err := a.WebZmodemPick(context.Background(), "terminal", "upload"); err == nil {
		t.Fatal("second parser claim allowed")
	}
	if err := a.WebZmodemFinish(choice.RequestID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(choice.Paths[0]); !os.IsNotExist(err) {
		t.Fatal("completed upload stage retained")
	}
	result := make(chan WebZmodemChoice, 1)
	fail := make(chan error, 1)
	go func() {
		c, err := a.WebZmodemPick(context.Background(), "terminal", "download")
		result <- c
		fail <- err
	}()
	var request string
	eventuallyWeb(t, func() bool {
		a.webFiles.mu.Lock()
		defer a.webFiles.mu.Unlock()
		for id := range a.webFiles.zmodem {
			request = id
		}
		return request != ""
	})
	if err := a.WebZmodemAnswer(request, "browser-b", "receive"); err == nil {
		t.Fatal("wrong browser answered picker")
	}
	if err := a.WebZmodemAnswer(request, "browser-a", "receive"); err != nil {
		t.Fatal(err)
	}
	choice = <-result
	if err := <-fail; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(choice.Directory, "received.bin"), []byte("complete"), 0600); err != nil {
		t.Fatal(err)
	}
	url := srv.URL + "/web/files/" + choice.StageID + "/download"
	resp, _ := http.Get(url)
	resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatal("unfinished ZMODEM receive exposed")
	}
	_ = a.WebZmodemFinish(choice.RequestID, "")
	resp, err = http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if _, err := zip.NewReader(bytes.NewReader(b), int64(len(b))); err != nil {
		t.Fatal("completed ZMODEM receive unavailable", err)
	}
	_ = a.WebZmodemTarget("terminal", "", "")
	native, err := a.WebZmodemPick(context.Background(), "terminal", "upload")
	if err != nil || native.RequestID != "" {
		t.Fatal("native picker fallback broken")
	}
}
