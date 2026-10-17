package webaccess

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const maxQueueBytes = 2 << 20
const maxQueueFrames = 256

type Event struct {
	Name     string `json:"name"`
	Data     any    `json:"data"`
	Sender   string `json:"sender,omitempty"`
	Sequence uint64 `json:"sequence"`
}

type subscriber struct {
	frames chan []byte
	done   chan struct{}
	bytes  int
}

// Broker never writes to a socket in the event dispatcher. A slow client is
// disconnected, then takes a new desktop snapshot on its next subscription.
type Broker struct {
	mu       sync.Mutex
	clients  map[*subscriber]bool
	sequence uint64
}

func NewBroker() *Broker { return &Broker{clients: make(map[*subscriber]bool)} }
func (b *Broker) Publish(event Event) {
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sequence = event.Sequence
	for c := range b.clients {
		if c.bytes+len(data) > maxQueueBytes || len(c.frames) == cap(c.frames) {
			b.removeLocked(c)
			continue
		}
		c.bytes += len(data)
		c.frames <- data
	}
}

func (b *Broker) removeLocked(c *subscriber) {
	if b.clients[c] {
		delete(b.clients, c)
		close(c.done)
	}
}
func (b *Broker) DisconnectAll() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for c := range b.clients {
		b.removeLocked(c)
	}
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "GET required", 405)
		return
	}
	rc := http.NewResponseController(w)
	b.mu.Lock()
	if len(b.clients) >= 64 {
		b.mu.Unlock()
		http.Error(w, "too many event streams", 429)
		return
	}
	c := &subscriber{frames: make(chan []byte, maxQueueFrames), done: make(chan struct{})}
	b.clients[c] = true
	seq := b.sequence
	b.mu.Unlock()
	defer func() { b.mu.Lock(); b.removeLocked(c); b.mu.Unlock() }()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	write := func(frame string) bool {
		_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprint(w, frame); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	// Registration precedes ready. Always resnapshot after ready; Last-Event-ID
	// is deliberately not treated as a replay request.
	if !write(fmt.Sprintf("event: ready\ndata: {\"sequence\":%d,\"resnapshot\":true}\n\n", seq)) {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-c.done:
			return
		case <-ticker.C:
			if !write(": keepalive\n\n") {
				return
			}
		case frame := <-c.frames:
			b.mu.Lock()
			c.bytes -= len(frame)
			b.mu.Unlock()
			select {
			case <-c.done:
				return
			default:
			}
			if !write("data: " + string(frame) + "\n\n") {
				return
			}
		}
	}
}
