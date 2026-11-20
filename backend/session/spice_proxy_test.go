package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func spiceProxyEchoServer(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var sockets []net.Conn
	var nextID atomic.Uint32
	t.Cleanup(func() {
		listener.Close()
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range sockets {
			conn.Close()
		}
	})
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			sockets = append(sockets, conn)
			mu.Unlock()
			id := nextID.Add(1)
			go func() {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
				var greeting [4]byte
				binary.LittleEndian.PutUint32(greeting[:], id)
				if _, err := conn.Write(greeting[:]); err != nil {
					return
				}
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

func spiceProxySocket(t *testing.T, addr string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{Subprotocols: []string{"binary"}, HandshakeTimeout: 3 * time.Second}
	ws, _, err := dialer.Dial(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ws.Close() })
	if ws.Subprotocol() != "binary" {
		t.Fatal("SPICE binary subprotocol was not negotiated")
	}
	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	return ws
}

func spiceProxyRead(t *testing.T, ws *websocket.Conn, size int) []byte {
	t.Helper()
	var data []byte
	for len(data) < size {
		kind, part, err := ws.ReadMessage()
		if err != nil {
			t.Fatal(err)
		}
		if kind != websocket.BinaryMessage {
			t.Fatal("SPICE data was not binary")
		}
		data = append(data, part...)
	}
	if len(data) != size {
		t.Fatalf("received %d bytes, want %d", len(data), size)
	}
	return data
}

func TestSPICEProxyIndependentChannels(t *testing.T) {
	proxy := NewSPICEProxy(spiceProxyEchoServer(t), nil)
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proxy.Stop)
	var sockets []*websocket.Conn
	ids := make(map[uint32]bool)
	for range 5 {
		ws := spiceProxySocket(t, addr)
		id := binary.LittleEndian.Uint32(spiceProxyRead(t, ws, 4))
		if ids[id] {
			t.Fatal("two SPICE channels shared the same TCP connection")
		}
		ids[id] = true
		sockets = append(sockets, ws)
	}
	for i, ws := range sockets {
		payload := bytes.Repeat([]byte{byte(i), 0, 0xff, 'R', 'E', 'D', 'Q'}, 20000)
		if err := ws.WriteMessage(websocket.BinaryMessage, payload); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(spiceProxyRead(t, ws, len(payload)), payload) {
			t.Fatalf("SPICE channel %d data crossed into another channel", i)
		}
	}
	sockets[0].Close()
	if err := sockets[1].WriteMessage(websocket.BinaryMessage, []byte("still connected")); err != nil {
		t.Fatal(err)
	}
	if string(spiceProxyRead(t, sockets[1], len("still connected"))) != "still connected" {
		t.Fatal("closing one channel disrupted another")
	}
	replacement := spiceProxySocket(t, addr)
	if id := binary.LittleEndian.Uint32(spiceProxyRead(t, replacement, 4)); ids[id] {
		t.Fatal("replacement SPICE channel did not receive a fresh TCP connection")
	}
}

func TestSPICEProxyEndpointIsLocalAndPrivate(t *testing.T) {
	proxy := NewSPICEProxy("guest.invalid:5900", nil)
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proxy.Stop)
	endpoint, err := url.Parse(addr)
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Scheme != "ws" || endpoint.Hostname() != "127.0.0.1" || len(endpoint.Path) != 33 {
		t.Fatalf("unexpected local endpoint %q", addr)
	}
	endpoint.Path = "/"
	endpoint.Scheme = "http"
	client := http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(endpoint.String())
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown proxy path returned %d", response.StatusCode)
	}
}

func TestSPICEProxyStopClosesEveryChannel(t *testing.T) {
	proxy := NewSPICEProxy(spiceProxyEchoServer(t), nil)
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(proxy.Stop)
	var sockets []*websocket.Conn
	for range 5 {
		ws := spiceProxySocket(t, addr)
		spiceProxyRead(t, ws, 4)
		sockets = append(sockets, ws)
	}
	done := make(chan struct{})
	go func() { proxy.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("stopping SPICE proxy blocked on an active channel")
	}
	for _, ws := range sockets {
		if _, _, err := ws.ReadMessage(); err == nil {
			t.Fatal("SPICE channel survived proxy shutdown")
		}
	}
	if len(proxy.conns) != 0 {
		t.Fatal("SPICE proxy retained stopped channels")
	}
	if ws, _, err := websocket.DefaultDialer.Dial(addr, nil); err == nil {
		ws.Close()
		t.Fatal("SPICE listener accepted a connection after shutdown")
	}
}

func TestSPICEProxyStopDuringChannelUpgrade(t *testing.T) {
	for i := range 10 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			proxy := NewSPICEProxy("127.0.0.1:1", nil)
			addr, err := proxy.Start()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(proxy.Stop)
			var clients sync.WaitGroup
			for range 5 {
				clients.Go(func() {
					if ws, _, err := websocket.DefaultDialer.Dial(addr, nil); err == nil {
						ws.Close()
					}
				})
			}
			stopped := make(chan struct{})
			go func() { proxy.Stop(); clients.Wait(); close(stopped) }()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("SPICE shutdown raced a channel upgrade")
			}
		})
	}
}
