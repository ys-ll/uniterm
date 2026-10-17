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
	output := map[string]any{"id": s.id, "data": "PTY output", "webTimestamp": int64(-1)}
	beforeOutput := time.Now().UnixMilli()
	a.emit("session:data", output)
	afterOutput := time.Now().UnixMilli()
	if output["webTimestamp"] != int64(-1) {
		t.Fatal("transport mutated the original event")
	}
	dataSequence := a.webTransport.terminalSequence(s.id)
	prompt := "\r\nPress Enter to retry...\r\n"
	write := WebTerminalPresentationOperation{Kind: "write", Data: &prompt}
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, write); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, WebTerminalPresentationOperation{Kind: "clear"}); err != nil {
		t.Fatal(err)
	}
	timestamp := int64(1750000000123)
	if err := a.PublishWebTerminalPresentation(browser, s.id, WebTerminalPresentationOperation{Kind: "stamp", Timestamp: &timestamp}); err != nil {
		t.Fatal(err)
	}
	if err := a.ResizeWebTerminal(s.id, "desktop", 90, 30, false); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, WebTerminalPresentationOperation{Kind: "reset"}); err != nil {
		t.Fatal(err)
	}
	blink, scrollback := false, 2500
	configure := WebTerminalPresentationOperation{Kind: "configure", CursorBlink: &blink, Scrollback: &scrollback}
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, configure); err != nil {
		t.Fatal(err)
	}
	boundary := a.webTransport.terminalSequence(s.id)
	if boundary != dataSequence+6 {
		t.Fatal("presentation did not advance the terminal watermark")
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, write); err == nil || !strings.Contains(err.Error(), "native_only") {
		t.Fatalf("browser prompt write: %v", err)
	}
	if err := a.PublishWebTerminalPresentation(browser, s.id, configure); err == nil || !strings.Contains(err.Error(), "native_only") {
		t.Fatalf("browser configuration: %v", err)
	}
	if a.webTransport.terminalSequence(s.id) != boundary {
		t.Fatal("rejected browser presentation emitted a frame")
	}
	result := make(chan error, 1)
	go func() {
		snapshot, err := a.GetWebTerminalSnapshot(ctx, s.id)
		if err == nil && snapshot.Sequence != boundary {
			err = errors.New("snapshot boundary changed")
		}
		if err == nil && (snapshot.ANSI != "" || snapshot.State["error"] != "serializer unavailable") {
			err = errors.New("snapshot error sidecar was not passed through unchanged")
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
	snapshot := WebTerminalSnapshot{SessionID: s.id, Sequence: dataSequence, Cols: 90, Rows: 30, State: map[string]any{"error": "serializer unavailable"}}
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
	wantNames := []string{"session:data", "session:presentation", "session:presentation", "session:presentation", "session:resize", "session:presentation", "session:presentation", "session:presentation"}
	wantKinds := []string{"", "write", "clear", "stamp", "", "reset", "configure", "write"}
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
		if frame.Name == "session:data" {
			deliveredAt, ok := data["webTimestamp"].(int64)
			if !ok || deliveredAt < beforeOutput || deliveredAt > afterOutput {
				t.Fatalf("output did not carry host delivery time: %+v", data)
			}
		} else if _, ok := data["webTimestamp"]; ok {
			t.Fatal("host output timestamp added to a non-output event")
		}
		if kind := wantKinds[seen]; kind != "" {
			operation := data["operation"].(map[string]any)
			if operation["kind"] != kind || (kind == "write" && operation["data"] != prompt) {
				t.Fatalf("presentation payload: %+v", operation)
			}
			if kind == "stamp" && operation["timestamp"] != float64(timestamp) {
				t.Fatalf("stamp timestamp changed: %+v", operation)
			}
			if kind == "configure" && (operation["cursorBlink"] != blink || operation["scrollback"] != float64(scrollback)) {
				t.Fatalf("configuration changed: %+v", operation)
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

func TestWebTerminalPresentationStampAndConfigureValidation(t *testing.T) {
	a := webTestApp(t)
	s := addControlledTerminal(a, "presentation-types", false)
	for _, input := range []string{
		`{"kind":"stamp"}`, `{"kind":"stamp","timestamp":null}`,
		`{"kind":"stamp","timestamp":-1}`, `{"kind":"stamp","timestamp":8640000000000001}`,
		`{"kind":"stamp","timestamp":9223372036854775808}`, `{"kind":"stamp","timestamp":1.5}`,
		`{"kind":"stamp","timestamp":"123"}`, `{"kind":"stamp","timestamp":true}`,
		`{"kind":"stamp","timestamp":1,"data":"extra"}`, `{"kind":"stamp","timestamp":1,"scrollback":0}`,
		`{"kind":"configure"}`, `{"kind":"configure","cursorBlink":false}`,
		`{"kind":"configure","scrollback":0}`, `{"kind":"configure","cursorBlink":null,"scrollback":0}`,
		`{"kind":"configure","cursorBlink":true,"scrollback":null}`,
		`{"kind":"configure","cursorBlink":"true","scrollback":0}`,
		`{"kind":"configure","cursorBlink":1,"scrollback":0}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":-1}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":50001}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":1.5}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":"100"}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":100,"timestamp":0}`,
		`{"kind":"write","data":"prompt","timestamp":0}`,
		`{"kind":"clear","cursorBlink":false}`, `{"kind":"reset","timestamp":0}`,
	} {
		var operation WebTerminalPresentationOperation
		before := a.webTransport.sequence
		err := json.Unmarshal([]byte(input), &operation)
		if err == nil {
			err = a.PublishWebTerminalPresentation(context.Background(), s.id, operation)
		}
		if err == nil || a.webTransport.sequence != before {
			t.Fatalf("invalid presentation accepted or emitted: %s (%v)", input, err)
		}
	}
	for _, input := range []string{
		`{"kind":"stamp","timestamp":0}`, `{"kind":"stamp","timestamp":8640000000000000}`,
		`{"kind":"configure","cursorBlink":false,"scrollback":0}`,
		`{"kind":"configure","cursorBlink":true,"scrollback":50000}`,
	} {
		var operation WebTerminalPresentationOperation
		if err := json.Unmarshal([]byte(input), &operation); err != nil {
			t.Fatal(err)
		}
		before := a.webTransport.sequence
		if err := a.PublishWebTerminalPresentation(context.Background(), s.id, operation); err != nil {
			t.Fatalf("valid boundary rejected: %s (%v)", input, err)
		}
		if a.webTransport.sequence != before+1 {
			t.Fatal("accepted presentation did not emit exactly once")
		}
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
	old = a.webTransport.terminalSequence(s.id)
	timestamp, blink, scrollback := int64(0), false, 0
	for i := 0; i < 2050; i++ {
		operation := WebTerminalPresentationOperation{Kind: "stamp", Timestamp: &timestamp}
		if i%2 == 0 {
			operation = WebTerminalPresentationOperation{Kind: "configure", CursorBlink: &blink, Scrollback: &scrollback}
		}
		if err := a.PublishWebTerminalPresentation(context.Background(), s.id, operation); err != nil {
			t.Fatal(err)
		}
	}
	if len(j.frames) > 2048 || j.floor <= old || a.webTransport.validateSnapshot(s.id, old, 0) == nil {
		t.Fatal("stamp/configure bypassed bounded journal or stale-snapshot validation")
	}
	if err := a.CloseSession(s.id); err != nil {
		t.Fatal(err)
	}
	if err := a.PublishWebTerminalPresentation(context.Background(), s.id, WebTerminalPresentationOperation{Kind: "clear"}); err == nil {
		t.Fatal("presentation to closed session accepted")
	}
}
