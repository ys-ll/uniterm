package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWebTerminalPresentationSequenceAndSnapshot(t *testing.T) {
	a := webTestApp(t)
	browser := webTestBrowserContext(t, a)
	s := addControlledTerminal(a, "presentation", true)
	native := make(chan *application.CustomEvent, 16)
	requests := make(chan *application.CustomEvent, 1)
	a.webTransport.native = func(event *application.CustomEvent) {
		if event.Name == "web:snapshot-request" {
			requests <- event
		} else {
			native <- event
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", a.webAccess.Status().URLs[0]+"/web/events", nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	scanner := bufio.NewScanner(stream.Body)
	if !scanner.Scan() || scanner.Text() != "event: ready" {
		t.Fatal("SSE ready missing")
	}
	a.emit("session:data", map[string]any{"id": s.id, "data": "PTY output"})
	dataSequence := a.webTransport.terminalSequence(s.id)
	prompt := "\r\nPress Enter to retry...\r\n"
	write := WebTerminalPresentationOperation{Kind: "write", Data: &prompt}
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, write); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, WebTerminalPresentationOperation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	if err := a.ResizeWebTerminal(s.id, "desktop", 90, 30, false); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, WebTerminalPresentationOperation{Kind: "reset"}); err != nil {
		t.Fatal(err)
	}
	boundary := a.webTransport.terminalSequence(s.id)
	if boundary != dataSequence+4 {
		t.Fatal("presentation did not advance the terminal watermark")
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, write); err == nil || !strings.Contains(err.Error(), "native_only") {
		t.Fatalf("browser prompt write: %v", err)
	}
	if a.webTransport.terminalSequence(s.id) != boundary {
		t.Fatal("rejected browser write emitted a frame")
	}
	result := make(chan error, 1)
	go func() {
		snapshot, err := a.GetWebTerminalSnapshot(ctx, s.id)
		if err == nil && snapshot.Sequence != boundary {
			err = errors.New("snapshot boundary changed")
		}
		result <- err
	}()
	var request *application.CustomEvent
	select {
	case request = <-requests:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	payload := request.Data.(map[string]any)
	if payload["sequence"] != boundary {
		t.Fatal("snapshot request ignored presentation watermark")
	}
	requestID := payload["requestId"].(string)
	snapshot := WebTerminalSnapshot{SessionID: s.id, Sequence: dataSequence, Cols: 90, Rows: 30, State: map[string]any{}}
	if err := a.ProvideWebTerminalSnapshot(context.Background(), requestID, snapshot); err == nil {
		t.Fatal("snapshot predating presentation accepted")
	}
	// A later prompt remains a live frame after the reset snapshot boundary.
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, write); err != nil {
		t.Fatal(err)
	}
	snapshot.Sequence = boundary
	if err := a.ProvideWebTerminalSnapshot(context.Background(), requestID, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	wantNames := []string{"session:data", "session:presentation", "session:presentation", "session:resize", "session:presentation", "session:presentation"}
	wantKinds := []string{"", "write", "clear", "", "reset", "write"}
	seen := 0
	for seen < len(wantNames) && scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame struct {
			Name     string          `json:"name"`
			Sequence uint64          `json:"sequence"`
			Data     json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(line[6:]), &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Name == "" { // ready payload
			continue
		}
		event := <-native
		nativeData, _ := json.Marshal(event.Data)
		if frame.Name != wantNames[seen] || event.Name != frame.Name || frame.Sequence != dataSequence+uint64(seen) || !bytes.Equal(frame.Data, nativeData) {
			t.Fatalf("native/SSE sequence or payload diverged at %d: %+v", seen, frame)
		}
		data := event.Data.(map[string]any)
		if data["webSequence"] != frame.Sequence {
			t.Fatal("webSequence differs from event sequence")
		}
		if kind := wantKinds[seen]; kind != "" {
			operation := data["operation"].(map[string]any)
			if operation["kind"] != kind || (kind == "write" && operation["data"] != prompt) {
				t.Fatalf("presentation payload: %+v", operation)
			}
		}
		seen++
	}
	if seen != len(wantNames) {
		t.Fatalf("received %d frames: %v", seen, scanner.Err())
	}
	if len(s.writes) != 0 || a.GetWebSessions()[0].ActiveClient != "desktop" {
		t.Fatal("presentation wrote to PTY or changed input owner")
	}
}

func TestWebTerminalPresentationValidationAndJournalLimit(t *testing.T) {
	a := webTestApp(t)
	s := addControlledTerminal(a, "presentation-validation", false)
	text, invalidUTF8, tooLarge := "text", string([]byte{0xff}), strings.Repeat("x", (1<<20)+1)
	for _, operation := range []WebTerminalPresentationOperation{
		{}, {Kind: "unknown"}, {Kind: "write"},
		{Kind: "write", Data: &invalidUTF8}, {Kind: "write", Data: &tooLarge},
		{Kind: "clear", Data: &text}, {Kind: "reset", Data: &text},
	} {
		before := a.webTransport.sequence
		err := a.PublishWebTerminalPresentation(context.Background(), s.id, operation)
		var typed *WebTerminalError
		if !errors.As(err, &typed) || a.webTransport.sequence != before {
			t.Fatalf("invalid presentation accepted or emitted: kind=%q err=%v", operation.Kind, err)
		}
	}
	// Empty writes are valid strings, including before the PTY starts.
	empty := ""
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, WebTerminalPresentationOperation{Kind: "write", Data: &empty}); err != nil {
		t.Fatal(err)
	}
	old := a.webTransport.terminalSequence(s.id)
	limit := strings.Repeat("x", 1<<20)
	for i := 0; i < 3; i++ {
		if err := a.PublishWebTerminalPresentation(context.Background(), s.id, WebTerminalPresentationOperation{Kind: "write", Data: &limit}); err != nil {
			t.Fatal(err)
		}
	}
	j := a.webTransport.journals[s.id]
	if j.bytes > 2<<20 || j.floor <= old || a.webTransport.validateSnapshot(s.id, old, 0) == nil {
		t.Fatal("presentation bypassed bounded journal or stale-snapshot validation")
	}
	if err := a.CloseSession(s.id); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, WebTerminalPresentationOperation{Kind: "clear"}); err == nil {
		t.Fatal("presentation to closed session accepted")
	}
}
