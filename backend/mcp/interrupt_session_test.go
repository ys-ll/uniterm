package mcp

import (
	"errors"
	"testing"
	"time"
)

func TestInterruptSessionReleasesUnknownShell(t *testing.T) {
	for _, timedOut := range []bool{false, true} {
		name := "running"
		if timedOut {
			name = "timed-out"
		}
		t.Run(name, func(t *testing.T) {
			r := NewRegistry()
			p := &fakePty{alive: true}
			unknown := struct{ Pty }{p} // Network CLI without completion markers.
			j, err := r.Start(unknown, "fortigate", "get system status", "\n", 40*time.Millisecond, 0)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Interrupt(j.ID) })
			if timedOut {
				p.emit("FortiGate-60E-POE # ")
				waitState(t, j, StateTimeout)
			}
			if err := r.InterruptSession("fortigate", func() error { return p.Write([]byte{3}) }); err != nil {
				t.Fatal(err)
			}
			_, _, _, state, jobErr := j.Snapshot()
			if state != StateKilled || jobErr != ErrInterrupted {
				t.Fatalf("state=%s err=%v", state, jobErr)
			}
			if len(p.wrote) != 2 || p.wrote[1] != "\x03" {
				t.Fatalf("interrupt not delivered: %q", p.wrote)
			}
			next, err := r.Start(unknown, "fortigate", "get system status", "\n", time.Second, 0)
			if err != nil {
				t.Fatalf("next command blocked after interrupt: %v", err)
			}
			t.Cleanup(func() { _ = r.Interrupt(next.ID) })
		})
	}
}

func TestInterruptSessionFailedWriteReleasesLock(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.Start(p, "s", "sleep 60", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := errors.New("write failed")
	if err := r.InterruptSession("s", func() error { return want }); err != want {
		t.Fatalf("lost write error: %v", err)
	}
	// A failed interrupt write must not hold the session lock: the job is
	// finished as killed anyway, so a wedged PTY cannot wedge the AI.
	if _, _, _, st, jerr := j.Snapshot(); st != StateKilled || jerr != ErrInterrupted {
		t.Fatalf("state=%s err=%v, want killed/%v", st, jerr, ErrInterrupted)
	}
	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatalf("lock not released after failed interrupt write: %v", err)
	}
	_ = r.Interrupt(next.ID)
}

func TestInterruptSessionHangingWriteReleasesLock(t *testing.T) {
	r := NewRegistry()
	r.interruptWriteTimeout = 100 * time.Millisecond
	p := &fakePty{alive: true}
	j, err := r.Start(p, "s", "sleep 60", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	// The write never returns (wedged PTY): InterruptSession must cap it and
	// still release the session lock.
	start := time.Now()
	err = r.InterruptSession("s", func() error {
		time.Sleep(time.Minute)
		return nil
	})
	if err != ErrWriteTimeout {
		t.Fatalf("err=%v, want %v", err, ErrWriteTimeout)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("interrupt blocked %v", elapsed)
	}
	if _, _, _, st, _ := j.Snapshot(); st != StateKilled {
		t.Fatalf("state=%s, want killed", st)
	}
	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatalf("lock not released after hanging interrupt write: %v", err)
	}
	_ = r.Interrupt(next.ID)
}

func TestInterruptSessionDoesNotCancelOtherOrReplacementJob(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.Start(p, "s", "first", "\n", time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Interrupt(j.ID) })
	other, err := r.Start(p, "other", "sleep 60", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Interrupt(other.ID) })
	var next *Job
	err = r.InterruptSession("s", func() error {
		// Model output completing the old job while the interrupt is sent.
		p.complete(j, 0)
		next, err = r.Start(p, "s", "next", "\n", time.Minute, 0)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Interrupt(next.ID) })
	for _, job := range []*Job{other, next} {
		_, _, _, state, _ := job.Snapshot()
		if state != StateRunning {
			t.Fatalf("unrelated job %s cancelled: %s", job.ID, state)
		}
	}
}

func TestInterruptSessionWithoutJobStillSendsInterrupt(t *testing.T) {
	sent := false
	if err := NewRegistry().InterruptSession("s", func() error { sent = true; return nil }); err != nil || !sent {
		t.Fatalf("interrupt without job: sent=%v err=%v", sent, err)
	}
}
