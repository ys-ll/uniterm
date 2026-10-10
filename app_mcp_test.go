package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMcpNotifyStrings(t *testing.T) {
	en := mcpNotifyStringsFor("en")
	zh := mcpNotifyStringsFor("zh-CN")
	if en.Title != "uniTerm · MCP approval request" {
		t.Errorf("en title = %q", en.Title)
	}
	if !strings.Contains(zh.BodyConnect, "请求建立新连接") {
		t.Errorf("zh connect template = %q", zh.BodyConnect)
	}
	if want := fmt.Sprintf(zh.BodyConnect, "cc"); mcpNotifyBody(zh, "cc", "") != want {
		t.Errorf("zh connect body = %q, want %q", mcpNotifyBody(zh, "cc", ""), want)
	}
	if got := mcpNotifyBody(en, "cc", "ls -la"); got != "cc: ls -la" {
		t.Errorf("short command preview = %q", got)
	}
	long := string(make([]byte, 100))
	if got := mcpNotifyBody(en, "cc", long); !strings.HasSuffix(got, "…") {
		t.Errorf("long command should end with ellipsis, got %q", got)
	}
	if mcpNotifyStringsFor("xx-unknown") != mcpNotifyLabels["en"] {
		t.Errorf("unknown language must fall back to English")
	}
}

// ── MCP file transfer waiters (see app_mcp.go) ──────────────────────────

func newMcpTransferTestApp() *App {
	return &App{
		mcpTransferWaiters: make(map[string]chan mcpTransferOutcome),
	}
}

func TestMcpRunTransferSuccess(t *testing.T) {
	a := newMcpTransferTestApp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.mcpDispatchTransferEvent(map[string]any{"taskId": "t1", "event": "complete", "status": "done"})
	}()
	if err := a.mcpRunTransfer(func() (string, error) { return "t1", nil }, 2*time.Second); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestMcpRunTransferErrorCarriesMessage(t *testing.T) {
	a := newMcpTransferTestApp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.mcpDispatchTransferEvent(map[string]any{"taskId": "t2", "event": "complete", "status": "error", "error": "permission denied"})
	}()
	err := a.mcpRunTransfer(func() (string, error) { return "t2", nil }, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected error carrying event message, got %v", err)
	}
}

// The completion event may fire between start() returning and the waiter
// registering — the run-transfer mutex must make dispatch wait for the
// registration instead of dropping the event. Simulated deterministically:
// the worker goroutine dispatches (and blocks on the held mutex) while
// start() is still running.
func TestMcpRunTransferCompletionDuringStart(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) {
		go a.mcpDispatchTransferEvent(map[string]any{"taskId": "t3", "event": "complete", "status": "done"})
		time.Sleep(50 * time.Millisecond) // let the dispatch block on the mutex
		return "t3", nil
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected success despite completion during start, got %v", err)
	}
}

func TestMcpRunTransferStartError(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) { return "", fmt.Errorf("start failed") }, 2*time.Second)
	if err == nil || err.Error() != "start failed" {
		t.Fatalf("expected start error passthrough, got %v", err)
	}
}

func TestMcpRunTransferTimeout(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) { return "t4", nil }, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

// Non-complete events must not resolve a waiter.
func TestMcpDispatchIgnoresProgressEvents(t *testing.T) {
	a := newMcpTransferTestApp()
	a.mcpDispatchTransferEvent(map[string]any{"taskId": "t5", "event": "start", "status": "done"})
	if len(a.mcpTransferWaiters) != 0 {
		t.Fatalf("waiter map should stay empty without registered waiters, got %d", len(a.mcpTransferWaiters))
	}
}
