package main

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/webaccess"
)

type WebAccessConfig = webaccess.Config
type WebAccessStatus = webaccess.Status

func (a *App) GetWebAccessStatus() WebAccessStatus { return a.webAccess.Status() }
func (a *App) ConfigureWebAccess(config WebAccessConfig, token string) (WebAccessStatus, error) {
	err := a.webAccess.Configure(config, token)
	status := a.webAccess.Status()
	a.emit("web:access-status", status)
	return status, err
}
func (a *App) GenerateWebAccessToken() string { return webaccess.GenerateToken() }

// webTransport keeps Wails' HTTP RPC/assets and native delivery intact. It is
// installed even with Web disabled so existing terminals have sequence numbers.
type webTransport struct {
	*application.HTTPTransport
	server   *webaccess.Server
	mu       sync.Mutex
	sequence uint64
	journals map[string]*terminalJournal
	native   func(*application.CustomEvent)
}

type terminalJournal struct {
	gap    bool
	last   uint64
	floor  uint64
	bytes  int
	frames []terminalFrame
}

// Only watermarks and byte accounting are retained. The desktop owns the
// framebuffer; retaining an ANSI tail here would not make a valid snapshot.
type terminalFrame struct {
	sequence uint64
	size     int
}

func newWebTransport(server *webaccess.Server) *webTransport {
	return &webTransport{HTTPTransport: application.NewHTTPTransport(), server: server, journals: make(map[string]*terminalJournal), native: dispatchNativeWebEvent}
}

func dispatchNativeWebEvent(event *application.CustomEvent) {
	app := application.Get()
	if app == nil {
		return
	}
	// Match EventIPCTransport, using the public window snapshot seam.
	for _, window := range app.Window.GetAll() {
		if event.IsCancelled() {
			return
		}
		window.DispatchWailsEvent(event)
	}
}

func (t *webTransport) ServeAssets(h http.Handler) error { t.server.SetAssets(h); return nil }
func (t *webTransport) Stop() error                      { t.server.Close(); return t.HTTPTransport.Stop() }

func nativeWebEvent(name string) bool {
	return name == "web:snapshot-request" || name == "web:terminal-attach"
}

func (t *webTransport) DispatchWailsEvent(event *application.CustomEvent) {
	if event.IsCancelled() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if nativeWebEvent(event.Name) {
		t.native(event)
		return
	}
	t.sequence++
	// Never mutate the original CustomEvent: Wails Go listeners run in parallel.
	delivery := &application.CustomEvent{Name: event.Name, Data: event.Data, Sender: event.Sender}
	terminal := event.Name == "session:data" || event.Name == "session:binary" || event.Name == "session:status" || event.Name == "session:closed" || event.Name == "session:resize" || event.Name == "session:gap"
	var id string
	if terminal {
		b, err := json.Marshal(event.Data)
		var data map[string]any
		if err == nil && json.Unmarshal(b, &data) == nil && data != nil {
			id, _ = data["id"].(string)
			data["webSequence"] = t.sequence
			delivery.Data = data
		}
	}
	frame := webaccess.Event{Name: delivery.Name, Data: delivery.Data, Sender: delivery.Sender, Sequence: t.sequence}
	if event.Name == "web:session-created" {
		if info, ok := event.Data.(WebSession); ok {
			t.journals[info.ID] = &terminalJournal{}
		}
	}
	if j := t.journals[id]; id != "" && j != nil {
		j.last = t.sequence
		if event.Name == "session:gap" {
			j.gap = true
		}
		b, _ := json.Marshal(frame)
		j.frames = append(j.frames, terminalFrame{t.sequence, len(b)})
		j.bytes += len(b)
		for j.bytes > 2<<20 || len(j.frames) > 2048 {
			j.floor = j.frames[0].sequence
			j.bytes -= j.frames[0].size
			j.frames = j.frames[1:]
		}
	}
	t.native(delivery)
	t.server.Events.Publish(frame)
	if event.Name == "session:closed" {
		delete(t.journals, id)
	}
}

func (t *webTransport) terminalSequence(id string) uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	if j := t.journals[id]; j != nil {
		return j.last
	}
	return 0
}

func (t *webTransport) validateSnapshot(id string, sequence, minimum uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	last, floor := uint64(0), uint64(0)
	if j := t.journals[id]; j != nil {
		if j.gap {
			return webError("terminal_desynchronised", "terminal lost upstream bytes; reconnect the session")
		}
		last, floor = j.last, j.floor
	}
	if sequence < minimum || sequence < floor || sequence > last {
		return webError("snapshot_stale", "snapshot no longer overlaps the live terminal stream")
	}
	return nil
}
