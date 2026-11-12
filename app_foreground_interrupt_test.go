package main

import (
	"errors"
	"testing"
	"time"

	"github.com/ys-ll/uniterm/backend/mcp"
	"github.com/ys-ll/uniterm/backend/session"
)

type foregroundInterruptSession struct {
	*session.SSHSession
	writes   []string
	writeErr error
}

func (s *foregroundInterruptSession) Status() session.SessionStatus { return session.StatusConnected }
func (s *foregroundInterruptSession) Write(data []byte) error {
	s.writes = append(s.writes, string(data))
	return s.writeErr
}

func TestSendKeyInterruptReleasesForegroundLock(t *testing.T) {
	for _, tc := range []struct {
		name, text, control string
		writeFails          bool
	}{
		{name: "ctrl-c", control: "ctrl_c"},
		{name: "failed-ctrl-c", control: "ctrl_c", writeFails: true},
		{name: "ctrl-d", control: "ctrl_d"},
		{name: "enter", control: "enter"},
		{name: "text", text: "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &foregroundInterruptSession{SSHSession: session.NewSSHSession("network-cli")}
			a := &App{sessionManager: session.NewSessionManager()}
			a.sessionManager.Add(s)
			t.Cleanup(func() { a.removeCapture(s.ID()) })
			// The unknown shell has no completion marker. Wait for its execution
			// to time out before touching the session.
			id, err := a.StartForeground(s.ID(), "get system status", 1)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = a.InterruptForeground(id, "SIGINT") })
			job, _ := a.jobRegistry().Lookup(id)
			deadline := time.Now().Add(time.Second)
			for {
				_, _, _, state, _ := job.Snapshot()
				if state == mcp.StateTimeout {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("job did not time out")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if tc.writeFails {
				s.writeErr = errors.New("session write failed")
			}
			err = a.SendKey(s.ID(), tc.text, tc.control, true)
			if err != s.writeErr {
				t.Fatalf("write error = %v, want %v", err, s.writeErr)
			}
			if tc.control == "ctrl_c" && s.writes[len(s.writes)-1] != "\x03" {
				t.Fatalf("Ctrl+C not sent: %q", s.writes)
			}
			s.writeErr = nil

			// Ctrl+C finishes the job immediately — even when the write fails —
			// so a wedged PTY can never hold the execution lock. Other keys leave
			// the job in place; the next command then supersedes it.
			if tc.control == "ctrl_c" {
				if _, _, _, st, jerr := job.Snapshot(); st != mcp.StateKilled || jerr != mcp.ErrInterrupted {
					t.Fatalf("job state=%q err=%v, want killed/%v", st, jerr, mcp.ErrInterrupted)
				}
			}
			next, err := a.StartForeground(s.ID(), "get system status", 1000)
			if err != nil {
				t.Fatalf("next command blocked: %v", err)
			}
			t.Cleanup(func() { _ = a.InterruptForeground(next, "SIGINT") })
			if tc.control != "ctrl_c" {
				if _, _, _, st, jerr := job.Snapshot(); st != mcp.StateKilled || jerr != mcp.ErrSuperseded {
					t.Fatalf("old job state=%q err=%v, want killed by takeover", st, jerr)
				}
			}
		})
	}
}
