package session

import (
	"strings"
	"testing"
	"time"
)

func TestSSHAuthInputRejectsUnavailableCancelledAndFullQueue(t *testing.T) {
	s := NewSSHSession("auth-input")
	if s.Authenticating() || s.WriteAuthInput([]byte("early")) == nil {
		t.Fatal("accepted authentication input without a channel")
	}
	s.authAnswerCh = make(chan []byte, 1)
	if err := s.WriteAuthInput([]byte("first")); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- s.Write([]byte("overflow")) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("full authentication queue accepted input")
		}
	case <-time.After(time.Second):
		t.Fatal("full authentication queue blocked input/close serialization")
	}
	s.CancelConnect()
	<-s.authAnswerCh
	if s.Authenticating() || s.WriteAuthInput([]byte("late")) == nil {
		t.Fatal("cancelled authentication accepted input")
	}
}

// A rejected keyboard-interactive answer must print "Permission denied, please
// try again." before the next prompt (matching the OpenSSH client), so the
// user knows the previous answer failed instead of staring at a silent
// re-prompt while the server's fail delay runs (issue #949).
func TestKeyboardInteractiveChallengeDenialFeedback(t *testing.T) {
	s := NewSSHSession("auth-test")
	var out []byte
	s.SetOnDataCallback(func(d []byte) { out = append(out, d...) })
	s.mu.Lock()
	s.authAnswerCh = make(chan []byte, 4)
	s.mu.Unlock()

	var autoAnswered int32
	cb := s.keyboardInteractiveChallenge(ConnectionConfig{AuthType: "password", Password: "saved"}, &autoAnswered)

	// First challenge: the saved password is auto-submitted silently.
	answers, err := cb("", "", []string{"Password1: "}, []bool{false})
	if err != nil {
		t.Fatalf("auto-answer challenge failed: %v", err)
	}
	if len(answers) != 1 || answers[0] != "saved" {
		t.Fatalf("auto-answer = %v, want [saved]", answers)
	}
	if len(out) != 0 {
		t.Fatalf("auto-answer must not print anything, got %q", out)
	}

	// Second challenge: the saved password was rejected — the denial line
	// precedes the new prompt, and the typed answer is collected.
	go func() { s.authAnswerCh <- []byte("typed\r\n") }()
	answers, err = cb("", "", []string{"Password2: "}, []bool{false})
	if err != nil {
		t.Fatalf("interactive challenge failed: %v", err)
	}
	if len(answers) != 1 || answers[0] != "typed" {
		t.Fatalf("interactive answer = %v, want [typed]", answers)
	}
	if !strings.Contains(string(out), "Permission denied, please try again.") {
		t.Fatalf("re-challenge must print the denial line, got %q", out)
	}
	if !strings.Contains(string(out), "Password2: ") {
		t.Fatalf("re-challenge must print the prompt, got %q", out)
	}

	// A challenge in the same callback invocation is one rejection round: no
	// denial line between the questions of a single multi-prompt challenge.
	out = nil
	s.authAnswerCh <- []byte("a\r\n")
	s.authAnswerCh <- []byte("b\r\n")
	answers, err = cb("", "", []string{"OTP: ", "Password: "}, []bool{false, false})
	if err != nil {
		t.Fatalf("multi-prompt challenge failed: %v", err)
	}
	if len(answers) != 2 || answers[0] != "a" || answers[1] != "b" {
		t.Fatalf("multi-prompt answers = %v, want [a b]", answers)
	}
	if strings.Count(string(out), "Permission denied") != 1 {
		t.Fatalf("multi-prompt challenge must print the denial once, got %q", out)
	}
}
