package session

import (
	"strings"
	"testing"
	"time"
)

// TestLocalSessionWriteAfterDisconnectReturnsNotConnected guards the
// regression where Disconnect closed the PTY handle but left s.pty
// non-nil, so subsequent Write / Resize calls would silently write to
// a closed fd (returning os.ErrClosed) instead of the documented
// "not connected" / "session not connected" error.
func TestLocalSessionWriteAfterDisconnectReturnsNotConnected(t *testing.T) {
	s := NewLocalSession("write-after-disconnect")
	if err := s.Connect(ConnectionConfig{ShellPath: "/usr/bin/true"}); err != nil {
		t.Skipf("Connect: %v (likely /usr/bin/true missing on this platform)", err)
	}

	// Wait for the shell to exit naturally so the session is fully torn down.
	waitForStatus(t, s, StatusDisconnected, 3*time.Second)

	if err := s.Write([]byte("hello")); err == nil {
		t.Fatal("Write after Disconnect returned nil; expected a not-connected error")
	} else if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Write after Disconnect returned %q; expected \"not connected\"", err)
	}

	if err := s.Resize(80, 24); err == nil {
		t.Fatal("Resize after Disconnect returned nil; expected a not-connected error")
	} else if !strings.Contains(err.Error(), "not connected") {
		t.Fatalf("Resize after Disconnect returned %q; expected \"not connected\"", err)
	}
}

func waitForStatus(t *testing.T, s *LocalSession, want SessionStatus, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.Status() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for status %v; last seen %v", want, s.Status())
}