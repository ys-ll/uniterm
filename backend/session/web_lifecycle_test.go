package session

import (
	"sync"
	"testing"
	"time"
)

type webExecTestPTY struct {
	data chan []byte
	once sync.Once
}

func (p *webExecTestPTY) Data() <-chan []byte   { return p.data }
func (p *webExecTestPTY) Write([]byte) error    { return nil }
func (p *webExecTestPTY) Resize(int, int) error { return nil }
func (p *webExecTestPTY) Close() error          { p.once.Do(func() { close(p.data) }); return nil }

func TestWebExecPreservesFirstBytesUntilCallbacksAttached(t *testing.T) {
	p := &webExecTestPTY{data: make(chan []byte, 1)}
	p.data <- []byte("first prompt")
	s := NewContainerExecSession("web-exec", p)
	defer s.Disconnect()
	// Construction must not consume and discard output before authority setup.
	if len(p.data) != 1 {
		t.Fatal("constructor consumed output")
	}
	got := make(chan string, 4)
	s.SetOnDataCallback(func(b []byte) { got <- string(b) })
	s.StartOutput()
	s.StartOutput()
	select {
	case out := <-got:
		if out != "first prompt" {
			t.Fatal(out)
		}
	case <-time.After(time.Second):
		t.Fatal("first bytes missing")
	}
}

func TestWebOutputGapPrecedesRemainingBytes(t *testing.T) {
	s := &baseSession{id: "web-gap"}
	var events []string
	s.SetOnOutputGap(func() { events = append(events, "gap") })
	s.SetOnDataCallback(func(b []byte) { events = append(events, string(b)) })
	s.emitQueue = [][]byte{[]byte("remaining")}
	s.emitDropped = 3
	s.flushEmitChunks(s.drainEmitQueue())
	if len(events) != 2 || events[0] != "gap" || events[1] != "remaining" {
		t.Fatalf("events: %v", events)
	}
}
