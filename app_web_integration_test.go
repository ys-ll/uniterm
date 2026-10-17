package main

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
)

func TestWebIntegratedRoutesAndSessionCleanup(t *testing.T) {
	transfer, cwd := session.TransferEventSink, session.TerminalCwdSink
	a := NewApp(t.TempDir())
	a.dataDir = t.TempDir()
	a.sessionManager = session.NewSessionManager()
	a.webTransport.native = func(*application.CustomEvent) {}
	t.Cleanup(func() {
		a.webAccess.Close()
		a.sessionManager.CloseAll()
		a.closeWebFiles()
		session.TransferEventSink, session.TerminalCwdSink = transfer, cwd
	})
	base := enableWebTestListener(t, a)
	request := func(method, path, body string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Origin", base)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { resp.Body.Close() })
		return resp
	}
	resp := request("POST", "/web/files/export?name=integration.txt", "visitor bytes")
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("file route: %d %s", resp.StatusCode, body)
	}
	var stage WebFileStage
	if err := json.NewDecoder(resp.Body).Decode(&stage); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp = request("GET", "/web/files/"+stage.ID+"/download", "")
	data, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(data) != "visitor bytes" {
		t.Fatalf("download through shared listener: %q", data)
	}
	if resp := request("POST", "/web/remote/vnc", "invalid JSON"); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("remote route not registered: %d", resp.StatusCode)
	}
	config := a.webAccess.Status().Config
	if _, err := a.ConfigureWebAccess(config, "integration-test-token"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/web/files/" + stage.ID + "/download", "/web/remote/vnc/unknown/main"} {
		if resp := request("GET", path, ""); resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("feature route bypassed common authentication: %s = %d", path, resp.StatusCode)
		}
	}
	addControlledTerminal(a, "integration-close", true)
	a.webFiles.mu.Lock()
	stored := a.webFiles.stages[stage.ID]
	stored.SessionID = "integration-close"
	directory := stored.dir
	a.webFiles.mu.Unlock()
	if err := a.CloseSession("integration-close"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("normal session close left staged files: %v", err)
	}
}
