package session

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

func TestWebExecDisconnectBeforeAttachCancelsK8sOutput(t *testing.T) {
	for _, channel := range []byte{execChStdout, execChStderr} {
		t.Run(string(rune('0'+channel)), func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			_ = client.SetDeadline(time.Now().Add(5 * time.Second))
			_ = server.SetDeadline(time.Now().Add(5 * time.Second))
			sent := make(chan error, 1)
			go func() {
				defer server.Close()
				r, err := http.ReadRequest(bufio.NewReader(server))
				if err != nil {
					sent <- err
					return
				}
				accept := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				if _, err := fmt.Fprintf(server, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(accept[:])); err != nil {
					sent <- err
					return
				}
				// net.Pipe acknowledges each full WebSocket frame only after the
				// adapter reads it, making the full-queue send deterministic.
				for i := 0; i < 33; i++ {
					if _, err := server.Write([]byte{0x82, 2, channel, byte(i)}); err != nil {
						sent <- err
						return
					}
				}
				sent <- nil
				// Keep the upstream alive until the failed attachment closes it.
				_, _ = io.Copy(io.Discard, server)
			}()
			conn, _, err := websocket.NewClient(client, &url.URL{Scheme: "ws", Host: "exec.test", Path: "/"}, nil, 1024, 1024)
			if err != nil {
				t.Fatal(err)
			}
			stream := &k8sExecStream{conn: conn, data: make(chan []byte, 32), done: make(chan struct{})}
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				stream.readLoop()
			}()
			s := NewContainerExecSession("unattached-k8s", stream)
			t.Cleanup(func() {
				_ = s.Disconnect()
				// Also reclaim the blocked producer when running this test against
				// the regression, after the no-reader assertion has failed.
				for range stream.Data() {
				}
			})
			select {
			case err := <-sent:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("upstream did not fill the pre-attachment output queue")
			}
			if err := s.Disconnect(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("K8s output producer leaked after Disconnect without StartOutput")
			}
			if s.IsConnected() {
				t.Fatal("cancelled exec session remained connected")
			}
			for i := 0; i < cap(stream.data); i++ {
				if b := <-stream.Data(); len(b) != 1 || b[0] != byte(i) {
					t.Fatalf("buffered pre-attachment output changed at %d: %v", i, b)
				}
			}
			if _, ok := <-stream.Data(); ok {
				t.Fatal("cancelled producer left output open")
			}
		})
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
