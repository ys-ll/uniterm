package session

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRunPostLoginExpectAutomationSendsAfterExpectedOutput(t *testing.T) {
	output := newPostLoginOutputBuffer()
	var sent []string

	errCh := make(chan error, 1)
	go func() {
		errCh <- runPostLoginExpectAutomation(context.Background(), postLoginExpectAutomationConfig{
			Steps: []PostLoginExpectStep{
				{Expect: "$", Send: "ssh ${user}@${host}", Enter: true},
				{Expect: "password:", Send: "${password}", Enter: true},
			},
			Variables: map[string]string{
				"host":     "10.0.0.2",
				"user":     "root",
				"password": "secret",
			},
			Output: output,
			Send: func(data []byte) error {
				payload := string(data)
				sent = append(sent, payload)
				if payload == "ssh root@10.0.0.2\r" {
					output.Append([]byte("root@10.0.0.2's Password: "))
				}
				return nil
			},
			IsConnected:    func() bool { return true },
			DefaultTimeout: time.Second,
		})
	}()

	output.Append([]byte("Welcome\r\nroot@jump:~$ "))

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runPostLoginExpectAutomation returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runPostLoginExpectAutomation did not finish")
	}

	want := []string{"ssh root@10.0.0.2\r", "secret\r"}
	if len(sent) != len(want) {
		t.Fatalf("sent %d payloads, want %d: %#v", len(sent), len(want), sent)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("sent[%d] = %q, want %q", i, sent[i], want[i])
		}
	}
}

func TestRunPostLoginExpectAutomationTimesOutWithoutSending(t *testing.T) {
	output := newPostLoginOutputBuffer()
	var sent []string

	err := runPostLoginExpectAutomation(context.Background(), postLoginExpectAutomationConfig{
		Steps: []PostLoginExpectStep{
			{Expect: "never appears", Send: "ssh root@10.0.0.2", Enter: true},
		},
		Output: output,
		Send: func(data []byte) error {
			sent = append(sent, string(data))
			return nil
		},
		IsConnected:    func() bool { return true },
		DefaultTimeout: 20 * time.Millisecond,
	})

	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("error = %v, want timeout error", err)
	}
	if len(sent) != 0 {
		t.Fatalf("sent %d payloads, want none: %#v", len(sent), sent)
	}
}

func TestRunPostLoginExpectAutomationDrainsStaleOutputBeforeNextStep(t *testing.T) {
	output := newPostLoginOutputBuffer()
	output.Append([]byte("root@jump:~$ "))
	output.Append([]byte("stale startup text root@jump:~$ "))

	var sentMu sync.Mutex
	var sent []string
	errCh := make(chan error, 1)
	go func() {
		errCh <- runPostLoginExpectAutomation(context.Background(), postLoginExpectAutomationConfig{
			Steps: []PostLoginExpectStep{
				{Expect: "$", Send: "cd /tmp", Enter: true},
				{Expect: "$", Send: "pwd", Enter: true},
			},
			Output: output,
			Send: func(data []byte) error {
				sentMu.Lock()
				sent = append(sent, string(data))
				sentMu.Unlock()
				return nil
			},
			IsConnected:    func() bool { return true },
			DefaultTimeout: time.Second,
		})
	}()

	time.Sleep(50 * time.Millisecond)
	sentMu.Lock()
	gotLen := len(sent)
	gotSnapshot := append([]string(nil), sent...)
	sentMu.Unlock()
	if gotLen != 1 {
		t.Fatalf("sent %d payloads before fresh second prompt, want 1: %#v", gotLen, gotSnapshot)
	}

	output.Append([]byte("root@jump:/tmp$ "))

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runPostLoginExpectAutomation returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runPostLoginExpectAutomation did not finish")
	}

	sentMu.Lock()
	want := []string{"cd /tmp\r", "pwd\r"}
	if len(sent) != len(want) {
		t.Fatalf("sent %d payloads, want %d: %#v", len(sent), len(want), sent)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("sent[%d] = %q, want %q", i, sent[i], want[i])
		}
	}
	sentMu.Unlock()
}
