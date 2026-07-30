package session

import (
	"sync"
	"testing"
	"time"
)

type fakePTY struct {
	data    chan []byte
	written [][]byte
	resizes [][2]int
	closed  bool
}

func (f *fakePTY) Data() <-chan []byte   { return f.data }
func (f *fakePTY) Write(p []byte) error  { f.written = append(f.written, p); return nil }
func (f *fakePTY) Resize(c, r int) error { f.resizes = append(f.resizes, [2]int{c, r}); return nil }
func (f *fakePTY) Close() error          { f.closed = true; return nil }

func TestContainerExecSessionPipes(t *testing.T) {
	// ROOT CAUSE: NewContainerExecSession starts a readLoop goroutine
	// that fires the OnDataCallback (which appends to `got`) while the
	// test goroutine reads `got` after a sleep, with no happens-before
	// between them. Protect the shared slice with a mutex, replace the
	// time.Sleep with a channel-based handoff so the test waits
	// deterministically for the data, and close fp.data at the end so
	// readLoop terminates cleanly instead of leaking.
	fp := &fakePTY{data: make(chan []byte, 4)}
	s := NewContainerExecSession("s1", fp)
	var (
		gotMu sync.Mutex
		got   [][]byte
	)
	delivered := make(chan struct{}, 8)
	s.SetOnDataCallback(func(b []byte) {
		gotMu.Lock()
		got = append(got, b)
		gotMu.Unlock()
		delivered <- struct{}{}
	})
	fp.data <- []byte("hello")
	select {
	case <-delivered:
	case <-time.After(time.Second):
		t.Fatal("data not delivered in time")
	}
	gotMu.Lock()
	if len(got) != 1 || string(got[0]) != "hello" {
		gotMu.Unlock()
		t.Fatalf("data not piped: %v", got)
	}
	gotMu.Unlock()
	if err := s.Write([]byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	if len(fp.written) != 1 {
		t.Fatal("write not forwarded")
	}
	if err := s.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if len(fp.resizes) != 1 || fp.resizes[0] != [2]int{120, 40} {
		t.Fatalf("resize: %v", fp.resizes)
	}
	_ = s.Disconnect()
	_ = s.Disconnect()
	if !fp.closed {
		t.Fatal("pty not closed")
	}
	// Closing fp.data lets readLoop exit (it ranges over Data()).
	// The disconnect notice it emits on exit is drained for cleanliness.
	close(fp.data)
	select {
	case <-delivered:
	case <-time.After(time.Second):
	}
}
