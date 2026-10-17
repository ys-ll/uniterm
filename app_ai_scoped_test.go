package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Two real streaming HTTP calls on one App, with cancellation while both are
// producing events. Covers the protocol parsers, not just an event-name helper.
func TestScopedAIStreamsAndCancellation(t *testing.T) {
	for _, protocol := range []string{"anthropic", "openai", "responses"} {
		t.Run(protocol, func(t *testing.T) {
			finishB := make(chan struct{})
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model string `json:"model"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				id := body.Model
				w.Header().Set("Content-Type", "text/event-stream")
				send := func(s string) { fmt.Fprintln(w, "data: "+s+"\n"); w.(http.Flusher).Flush() }
				switch protocol {
				case "anthropic":
					send(`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`)
					send(fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"%s-think"}}`, id))
					send(`{"type":"content_block_stop","index":0}`)
					send(`{"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}`)
					send(fmt.Sprintf(`{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"%s-text"}}`, id))
				case "openai":
					send(fmt.Sprintf(`{"choices":[{"delta":{"reasoning_content":"%s-think","content":"%s-text"}}]}`, id, id))
				case "responses":
					send(`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning"}}`)
					send(fmt.Sprintf(`{"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"%s-think"}`, id))
					send(`{"type":"response.output_item.done","output_index":0}`)
					send(`{"type":"response.output_item.added","output_index":1,"item":{"type":"message"}}`)
					send(fmt.Sprintf(`{"type":"response.output_text.delta","output_index":1,"delta":"%s-text"}`, id))
				}
				if id == "alpha" {
					<-r.Context().Done()
					return
				}
				select {
				case <-finishB:
				case <-r.Context().Done():
					return
				}
				switch protocol {
				case "anthropic":
					send(`{"type":"content_block_stop","index":1}`)
					send(`{"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tool-beta","name":"inspect","input":{}}}`)
					send(`{"type":"content_block_stop","index":2}`)
					send(`{"type":"message_delta","delta":{"stop_reason":"tool_use"}}`)
					send(`{"type":"message_stop"}`)
				case "openai":
					send(`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"tool-beta","function":{"name":"inspect","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`)
					send(`[DONE]`)
				case "responses":
					send(`{"type":"response.output_item.done","output_index":1}`)
					send(`{"type":"response.output_item.added","output_index":2,"item":{"type":"function_call","call_id":"tool-beta","name":"inspect"}}`)
					send(`{"type":"response.function_call_arguments.delta","output_index":2,"delta":"{}"}`)
					send(`{"type":"response.output_item.done","output_index":2}`)
					send(`{"type":"response.completed"}`)
				}
			}))
			defer srv.Close()
			ctxA, cancelA := context.WithCancel(context.Background())
			ctxB, cancelB := context.WithCancel(context.Background())
			defer cancelA()
			defer cancelB()
			seen := make(chan string, 8)
			events := map[string][]string{}
			var mu sync.Mutex
			emit := func(name string, payload ...any) {
				encoded, _ := json.Marshal(payload)
				mu.Lock()
				events[name] = append(events[name], string(encoded))
				mu.Unlock()
				if strings.HasPrefix(name, "ai:token:") {
					seen <- name
				}
			}
			a := &App{}
			doneA, doneB := make(chan error, 1), make(chan error, 1)
			call := func(ctx context.Context, id string, done chan error) {
				result, err := a.chatCompletion(aiRequest{ctx: ctx, id: id, emit: emit}, "key", srv.URL, id, fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hello"}]}`, id), protocol, "test", "", "")
				if err == nil && (!strings.Contains(result, "beta-text") || !strings.Contains(result, "tool-beta")) {
					err = fmt.Errorf("missing text/tool result: %s", result)
				}
				done <- err
			}
			go call(ctxA, "alpha", doneA)
			go call(ctxB, "beta", doneB)
			for range 2 {
				select {
				case <-seen:
				case <-time.After(5 * time.Second):
					t.Fatal("stream did not start")
				}
			}
			a.CancelChatStream() // legacy cancellation must not touch either request
			cancelA()
			select {
			case err := <-doneA:
				if err == nil {
					t.Fatal("cancelled call succeeded")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancel did not reach upstream")
			}
			close(finishB)
			select {
			case err := <-doneB:
				if err != nil {
					t.Fatalf("other client interrupted: %v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("other client did not complete")
			}
			mu.Lock()
			defer mu.Unlock()
			for name := range events {
				if !strings.HasSuffix(name, ":alpha") && !strings.HasSuffix(name, ":beta") {
					t.Fatalf("unscoped event %q", name)
				}
			}
			for _, id := range []string{"alpha", "beta"} {
				for _, kind := range []string{"token", "thinking", "end"} {
					if len(events["ai:"+kind+":"+id]) == 0 {
						t.Errorf("missing %s for %s", kind, id)
					}
				}
				if text := strings.Join(events["ai:token:"+id], ""); !strings.Contains(text, id+"-text") {
					t.Errorf("wrong stream: %s", text)
				}
			}
			if len(events["ai:error:alpha"]) != 1 || len(events["ai:error:beta"]) != 0 {
				t.Fatal("error routed to wrong request")
			}
		})
	}
}

func TestScopedAIPreflightErrorEndsRequest(t *testing.T) {
	var names []string
	_, err := (&App{}).chatCompletion(aiRequest{id: "request", emit: func(name string, _ ...any) { names = append(names, name) }}, "", "", "", "{", "", "", "", "")
	if err == nil || strings.Join(names, ",") != "ai:error:request,ai:end:request" {
		t.Fatalf("events=%v err=%v", names, err)
	}
}
