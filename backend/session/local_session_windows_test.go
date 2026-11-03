package session

import (
	"strconv"
	"testing"
)

// ExecFilterEncoding is empty for every non-MSYS shell and reports the system
// OEM code page for MSYS bash (the shell-exec wrapper's filter input).
func TestExecFilterEncoding(t *testing.T) {
	s := NewLocalSession("t")
	if got := s.ExecFilterEncoding(); got != "" {
		t.Fatalf("non-MSYS session reported %q", got)
	}
	s.mu.Lock()
	s.msysShell = true
	s.mu.Unlock()
	enc := s.ExecFilterEncoding()
	if len(enc) < 3 || enc[:2] != "cp" {
		t.Fatalf("MSYS session reported %q, want cp<oemcp>", enc)
	}
	if _, err := strconv.Atoi(enc[2:]); err != nil {
		t.Fatalf("MSYS session reported %q, want numeric code page", enc)
	}
}
