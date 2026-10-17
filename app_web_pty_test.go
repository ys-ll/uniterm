//go:build !windows

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/webaccess"
)

// Real PTY, App control API, external HTTP SSE, and the native transport sink.
// This is backend integration coverage; desktop xterm/browser UI E2E is separate.
func TestWebLocalPTYSharedStreamGridAndDisconnect(t *testing.T) {
	a := webTestApp(t)
	base := enableWebTestListener(t, a)
	var mu sync.Mutex
	native := map[uint64]string{}
	a.webTransport.native = func(e *application.CustomEvent) {
		if e.Name == "session:data" {
			data := e.Data.(map[string]any)
			mu.Lock()
			native[data["webSequence"].(uint64)] = data["data"].(string)
			mu.Unlock()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/web/events", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	if !scanner.Scan() || scanner.Text() != "event: ready" {
		t.Fatal("not subscribed")
	}
	cfg := session.ConnectionConfig{Type: "local", Name: "web-pty-test", ShellPath: "/bin/sh", InitialCols: 93, InitialRows: 29}
	info, err := a.CreateSession("local", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer a.CloseSession(info.ID)
	if err := a.SessionStart(context.Background(), info.ID, cfg); err != nil {
		t.Fatal(err)
	}
	// GetWebSessions waits for the serialized Connect before returning status.
	deadline := time.Now().Add(3 * time.Second)
	for a.GetWebSessions()[0].Status != session.StatusConnected {
		if time.Now().After(deadline) {
			t.Fatal("local PTY did not connect")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := a.WebTerminalInput(info.ID, "browser-pty", "printf 'WEB_%s\\n' PTY_OK; stty size\r", 103, 31, false); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame webaccess.Event
		if json.Unmarshal([]byte(line[6:]), &frame) != nil || frame.Name != "session:data" {
			continue
		}
		data := frame.Data.(map[string]any)
		chunk := data["data"].(string)
		mu.Lock()
		nativeChunk, ok := native[frame.Sequence]
		mu.Unlock()
		if !ok || nativeChunk != chunk || uint64(data["webSequence"].(float64)) != frame.Sequence {
			t.Fatalf("native/browser byte or sequence mismatch: %+v", frame)
		}
		output.WriteString(chunk)
		if strings.Contains(output.String(), "WEB_PTY_OK") && strings.Contains(output.String(), "31 103") {
			break
		}
	}
	if !strings.Contains(output.String(), "WEB_PTY_OK") || !strings.Contains(output.String(), "31 103") {
		t.Fatalf("PTY command/grid missing: %q (%v)", output.String(), scanner.Err())
	}
	response.Body.Close()
	if len(a.GetWebSessions()) != 1 {
		t.Fatal("browser disconnect removed PTY")
	}
	if err := a.WebTerminalInput(info.ID, "desktop", "printf 'DESKTOP_%s\\n' OK\r", 83, 25, false); err != nil {
		t.Fatal(err)
	}
	a.webAccess.Close()
	if err := a.SessionWrite(info.ID, "printf 'STILL_ALIVE\\n'\r"); err != nil {
		t.Fatal("Web disable closed PTY", err)
	}
	if info := a.GetWebSessions()[0]; info.Cols != 83 || info.Rows != 25 || info.ActiveClient != "desktop" {
		t.Fatalf("desktop ownership: %+v", info)
	}
	if err := a.CloseSession(info.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.GetWebSessions()) != 0 {
		t.Fatal("explicit close retained PTY")
	}
}
