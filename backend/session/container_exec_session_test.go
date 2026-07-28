package session

import (
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
	fp := &fakePTY{data: make(chan []byte, 1)}
	s := NewContainerExecSession("s1", fp)
	var got [][]byte
	s.SetOnDataCallback(func(b []byte) { got = append(got, b) })
	fp.data <- []byte("hello")
	time.Sleep(50 * time.Millisecond)
	if len(got) != 1 || string(got[0]) != "hello" {
		t.Fatalf("data not piped: %v", got)
	}
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
}
