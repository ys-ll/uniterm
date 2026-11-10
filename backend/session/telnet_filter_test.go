package session

// Tests for the telnet IAC filter's cross-read state machine and NVT byte
// handling (issues #527 / #1415 regression coverage).

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

func newFilterTestSession(t *testing.T) *TelnetSession {
	t.Helper()
	s := NewTelnetSession("test")
	s.loginOutput = newPostLoginOutputBuffer()
	return s
}

func TestFilterIACNegotiationAndData(t *testing.T) {
	s := newFilterTestSession(t)
	got := s.filterIAC([]byte{'h', 'i', telnetIAC, telnetWILL, telnetOptEcho, 'x'})
	if want := []byte("hix"); !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFilterIACSequenceSplitAcrossReads(t *testing.T) {
	s := newFilterTestSession(t)
	// "IAC WILL ECHO" split between the WILL and the option byte: the
	// first read must not leak the IAC/WILL bytes and must not spin.
	got1 := s.filterIAC([]byte{'a', telnetIAC, telnetWILL})
	if string(got1) != "a" {
		t.Fatalf("first read leaked %q", got1)
	}
	got2 := s.filterIAC([]byte{telnetOptEcho, 'b'})
	if string(got2) != "b" {
		t.Fatalf("second read leaked %q", got2)
	}
}

func TestFilterIACLoneIACTail(t *testing.T) {
	s := newFilterTestSession(t)
	got1 := s.filterIAC([]byte{'x', telnetIAC})
	if string(got1) != "x" {
		t.Fatalf("lone IAC leaked into data: %q", got1)
	}
	// The held IAC merges with the next read: "IAC WILL ECHO" completes.
	got2 := s.filterIAC([]byte{telnetWILL, telnetOptEcho, 'y'})
	if string(got2) != "y" {
		t.Fatalf("held IAC not merged: %q", got2)
	}
}

func TestFilterIACSubNegotiationSpansReads(t *testing.T) {
	s := newFilterTestSession(t)
	// TTYPE SEND sub-negotiation split after "IAC SB TTYPE".
	got1 := s.filterIAC([]byte{'a', telnetIAC, telnetSB, telnetOptTerminalType})
	if string(got1) != "a" {
		t.Fatalf("split sub-negotiation leaked %q", got1)
	}
	got2 := s.filterIAC([]byte{telnetTTYPESend, telnetIAC, telnetSE, 'b'})
	if string(got2) != "b" {
		t.Fatalf("sub-negotiation tail leaked %q", got2)
	}
}

func TestFilterIACConsumesDataMark(t *testing.T) {
	s := newFilterTestSession(t)
	// SYNCH: 0xFF sent out-of-band, Data Mark 0xF2 in-band. With
	// SO_OOBINLINE both arrive inline and must be swallowed whole.
	got := s.filterIAC([]byte{'a', telnetIAC, telnetDM, 'b'})
	if string(got) != "ab" {
		t.Fatalf("IAC DM leaked into data: %q", got)
	}
}

func TestFilterIACCRNULCollapsed(t *testing.T) {
	s := newFilterTestSession(t)
	got := s.filterIAC([]byte{'o', 'k', '\r', telnetNUL, 'n', 'e', 'x', 't', '\r', telnetNUL})
	if want := []byte("ok\rnext\r"); !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFilterIACCRNULSplitAcrossReads(t *testing.T) {
	s := newFilterTestSession(t)
	got1 := s.filterIAC([]byte{'o', 'k', '\r'})
	if string(got1) != "ok\r" {
		t.Fatalf("CR lost: %q", got1)
	}
	got2 := s.filterIAC([]byte{telnetNUL, 'x'})
	if string(got2) != "x" {
		t.Fatalf("NUL after split CR not collapsed: %q", got2)
	}
}

func TestFilterIACBinaryModeKeepsNUL(t *testing.T) {
	s := newFilterTestSession(t)
	s.serverBinary = true
	got := s.filterIAC([]byte{'o', 'k', '\r', telnetNUL})
	if want := []byte{'o', 'k', '\r', 0}; !bytes.Equal(got, want) {
		t.Fatalf("binary-mode NUL dropped: %q", got)
	}
}

func TestFilterIACServerBinaryTracked(t *testing.T) {
	s := newFilterTestSession(t)
	s.filterIAC([]byte{telnetIAC, telnetWILL, telnetOptBinary})
	if !s.serverBinary {
		t.Fatal("server WILL BINARY not tracked")
	}
	s.filterIAC([]byte{telnetIAC, telnetWONT, telnetOptBinary})
	if s.serverBinary {
		t.Fatal("server WONT BINARY not tracked")
	}
}

func TestFilterIACWONTailDoesNotSpin(t *testing.T) {
	s := newFilterTestSession(t)
	// A read ending exactly in "IAC DONT" (no option byte yet) must not
	// loop forever — regression for the pre-#1415 filterIAC busy loop.
	done := make(chan []byte, 1)
	go func() {
		done <- s.filterIAC([]byte{'z', telnetIAC, telnetDONT})
	}()
	select {
	case got := <-done:
		if string(got) != "z" {
			t.Fatalf("unexpected output %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("filterIAC spins on trailing IAC+cmd")
	}
}

func TestLoginPhaseOver(t *testing.T) {
	s := newFilterTestSession(t)
	if s.loginPhaseOver() {
		t.Fatal("empty output should not count as past login")
	}
	s.loginOutput.Append([]byte("Fedora Linux 44\r\nlocalhost login: "))
	if s.loginPhaseOver() {
		t.Fatal("login prompt seen, phase should still be open")
	}
	s.loginOutput.Append([]byte("zhanglq\r\n\r\n[telnettest@localhost ~]$ "))
	if !s.loginPhaseOver() {
		t.Fatal("shell prompt after login prompt should close the phase")
	}
}

func TestLoginPhaseOverPreAuthenticated(t *testing.T) {
	s := newFilterTestSession(t)
	// Servers that drop straight into a shell: prompt with no login prompt.
	s.loginOutput.Append([]byte("\x1b]0;user@host:~\x07\x1b[?2004h\x1b[32muser@host\x1b[0m:\x1b[32m~\x1b[0m$ "))
	if !s.loginPhaseOver() {
		t.Fatal("pre-authenticated prompt should close the phase")
	}
}

func TestSendAutoLoginPromptDriven(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s := newFilterTestSession(t)
	s.conn = client

	var mu sync.Mutex
	var received []byte
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := server.Read(buf)
			mu.Lock()
			received = append(received, buf[:n]...)
			mu.Unlock()
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.sendAutoLogin(ctx, "telnettest", "secret123")

	// Server asks for the username, then for the password.
	deadline := time.Now().Add(3 * time.Second)
	waitFor := func(substr string) {
		t.Helper()
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := bytes.Contains(received, []byte(substr))
			mu.Unlock()
			if ok {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %q in %q", substr, received)
	}

	s.loginOutput.Append([]byte("localhost login: "))
	waitFor("telnettest\r\n")
	s.loginOutput.Append([]byte("Password: "))
	waitFor("secret123\r\n")
}

func TestSendAutoLoginBlindFallback(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s := newFilterTestSession(t)
	s.conn = client

	var mu sync.Mutex
	var received []byte
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := server.Read(buf)
			mu.Lock()
			received = append(received, buf[:n]...)
			mu.Unlock()
			if err != nil {
				close(done)
				return
			}
		}
	}()

	// Shorten the waits so the test does not take 8+ seconds of grace time.
	savedPromptTimeout, savedGrace := autoLoginPromptTimeout, autoLoginGraceTimeout
	autoLoginPromptTimeout, autoLoginGraceTimeout = 100*time.Millisecond, 100*time.Millisecond
	defer func() { autoLoginPromptTimeout, autoLoginGraceTimeout = savedPromptTimeout, savedGrace }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.sendAutoLogin(ctx, "u", "p")

	deadline := time.Now().Add(3 * time.Second)
	waitFor := func(substr string) {
		t.Helper()
		for time.Now().Before(deadline) {
			mu.Lock()
			ok := bytes.Contains(received, []byte(substr))
			mu.Unlock()
			if ok {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("timed out waiting for %q in %q", substr, received)
	}
	waitFor("u\r\n")
	waitFor("p\r\n")
}
