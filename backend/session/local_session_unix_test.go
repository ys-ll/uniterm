//go:build darwin || (linux && !android)

package session

import (
	"strings"
	"testing"
	"time"
)

func TestLocalSessionEmitsStartupOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	s := NewLocalSession(t.Name())
	startup := make(chan struct{})
	var output strings.Builder
	s.SetOnDataCallback(func(data []byte) {
		select {
		case <-startup:
			return
		default:
		}
		output.Write(data)
		if strings.Contains(output.String(), "uniterm startup ready") {
			close(startup)
		}
	})
	if err := s.Connect(ConnectionConfig{
		ShellPath: "/bin/sh",
		ShellArgs: `-c "printf 'uniterm startup ready\n'; read -r line"`,
		Cwd:       home,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Disconnect()
		s.stopEmitLoop()
	})

	select {
	case <-startup:
	case <-time.After(3 * time.Second):
		t.Fatal("local PTY did not emit startup output")
	}
}
