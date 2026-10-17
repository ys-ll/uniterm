package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/webaccess"
)

func webTestApp(t *testing.T) *App {
	t.Helper()
	a := &App{sessionManager: session.NewSessionManager(), webAccess: webaccess.New()}
	a.webTransport = newWebTransport(a.webAccess)
	a.webTransport.native = func(*application.CustomEvent) {}
	t.Cleanup(func() { a.webAccess.Close(); a.sessionManager.CloseAll() })
	return a
}
func enableWebTestListener(t *testing.T, a *App) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	a.webAccess.SetAssets(http.NotFoundHandler())
	if err := a.webAccess.Load(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := a.webAccess.Configure(webaccess.Config{Enabled: true, Host: "127.0.0.1", StartPort: port}, ""); err != nil {
		t.Fatal(err)
	}
	return a.webAccess.Status().URLs[0]
}
func webTestBrowserContext(t *testing.T, a *App) context.Context {
	t.Helper()
	ch := make(chan context.Context, 1)
	a.webAccess.Handle("/test-context", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { ch <- context.WithoutCancel(r.Context()) }))
	base := enableWebTestListener(t, a)
	r, err := http.Get(base + "/test-context")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	return <-ch
}

// Embedding the public Window interface allows a recording native window to
// exercise Window.Add/GetAll/Dispatch without constructing a platform WebView.
type recordingWebWindow struct {
	application.Window
	events chan *application.CustomEvent
}

func (w *recordingWebWindow) ID() uint                                      { return 91003 }
func (w *recordingWebWindow) Name() string                                  { return "test-native" }
func (w *recordingWebWindow) DispatchWailsEvent(e *application.CustomEvent) { w.events <- e }

func TestWebTransportPreservesActualWailsNativeAndAssetsSeams(t *testing.T) {
	a := webTestApp(t)
	a.webTransport.native = dispatchNativeWebEvent
	wails := application.New(application.Options{Transport: a.webTransport, Assets: application.AssetOptions{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, r.URL.Path) })}})
	window := &recordingWebWindow{events: make(chan *application.CustomEvent, 8)}
	wails.Window.Add(window)
	t.Cleanup(func() { wails.Window.Remove(window.ID()); a.webTransport.Stop() })
	wails.Event.Emit("session:data", map[string]any{"id": "native", "data": "hello"})
	var first *application.CustomEvent
	select {
	case first = <-window.events:
	case <-time.After(3 * time.Second):
		t.Fatal("native dispatch lost while Web disabled")
	}
	data, ok := first.Data.(map[string]any)
	if !ok || data["id"] != "native" || data["data"] != "hello" || data["webSequence"] != uint64(1) {
		t.Fatalf("single argument shape changed: %#v", first.Data)
	}
	// Start using the handler passed by Wails' real ServeAssets callback.
	l, _ := net.Listen("tcp4", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	if err := a.webAccess.Load(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := a.webAccess.Configure(webaccess.Config{Enabled: true, Host: "127.0.0.1", StartPort: port}, ""); err != nil {
		t.Fatal(err)
	}
	base := a.webAccess.Status().URLs[0]
	for _, path := range []string{"/wails/runtime.js", "/wails/transport.js", "/"} {
		r, err := http.Get(base + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		if r.StatusCode != 200 {
			t.Fatalf("Wails assets %s: %d", path, r.StatusCode)
		}
		if path == "/" && string(b) != "/web.html" {
			t.Fatalf("root: %q", b)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", base+"/web/events", nil)
	stream, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	scanner := bufio.NewScanner(stream.Body)
	if !scanner.Scan() || scanner.Text() != "event: ready" {
		t.Fatal("SSE ready missing")
	}
	wails.Event.Emit("web:snapshot-request", map[string]any{"requestId": "native-capability", "sessionId": "native"})
	if event := <-window.events; event.Name != "web:snapshot-request" {
		t.Fatal("native request lost")
	}
	wails.Event.Emit("session:data", map[string]any{"id": "native", "data": "world"})
	second := <-window.events
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame webaccess.Event
		json.Unmarshal([]byte(line[6:]), &frame)
		if frame.Name == "" {
			continue
		}
		if frame.Name != "session:data" || frame.Sequence != second.Data.(map[string]any)["webSequence"] {
			t.Fatalf("native and SSE diverged: %+v", frame)
		}
		return
	}
	t.Fatal("SSE event missing")
}

type controlledTerminal struct {
	session.Session
	id                                   string
	connected                            atomic.Bool
	connects                             atomic.Int32
	writes                               [][]byte
	cols, rows, pendingCols, pendingRows int
	failResize                           bool
}

func (s *controlledTerminal) ID() string    { return s.id }
func (s *controlledTerminal) Type() string  { return "local" }
func (s *controlledTerminal) Title() string { return "controlled" }
func (s *controlledTerminal) Status() session.SessionStatus {
	if s.connected.Load() {
		return session.StatusConnected
	}
	return session.StatusDisconnected
}
func (s *controlledTerminal) IsConnected() bool { return s.connected.Load() }
func (s *controlledTerminal) Connect(session.ConnectionConfig) error {
	s.connects.Add(1)
	s.connected.Store(true)
	return nil
}
func (s *controlledTerminal) Disconnect() error       { s.connected.Store(false); return nil }
func (s *controlledTerminal) SetPendingSize(c, r int) { s.pendingCols, s.pendingRows = c, r }
func (s *controlledTerminal) Resize(c, r int) error {
	s.SetPendingSize(c, r)
	if s.failResize {
		return fmt.Errorf("device resize failed")
	}
	s.cols, s.rows = c, r
	return nil
}
func (s *controlledTerminal) Write(b []byte) error {
	s.writes = append(s.writes, append([]byte(nil), b...))
	return nil
}
func addControlledTerminal(a *App, id string, connected bool) *controlledTerminal {
	s := &controlledTerminal{id: id, cols: 80, rows: 24}
	s.connected.Store(connected)
	a.sessionManager.Add(s)
	a.registerWebTerminal(s, session.ConnectionConfig{Name: "controlled", ID: "saved-connection", InitialCols: 80, InitialRows: 24})
	return s
}

func TestWebTerminalSharedInputOwnershipAndAppliedSize(t *testing.T) {
	a := webTestApp(t)
	s := addControlledTerminal(a, "shared", true)
	if err := a.WebTerminalInput(s.id, "browser", "abc", 100, 30, false); err != nil {
		t.Fatal(err)
	}
	if err := a.SessionWrite(s.id, "\x1b[1;1R"); err != nil {
		t.Fatal(err)
	}
	if err := a.SessionWriteBinary(s.id, "AP8="); err != nil {
		t.Fatal(err)
	}
	if err := a.SessionResize(s.id, 90, 20); err == nil || !strings.Contains(err.Error(), "inactive_client") {
		t.Fatalf("passive desktop stole grid: %v", err)
	}
	info := a.GetWebSessions()[0]
	if info.Cols != 100 || info.Rows != 30 || info.ActiveClient != "browser" || info.ConnectionID != "saved-connection" {
		t.Fatalf("descriptor: %+v", info)
	}
	s.failResize = true
	if err := a.ResizeWebTerminal(s.id, "desktop", 120, 40, true); err == nil {
		t.Fatal("resize failure lost")
	}
	info = a.GetWebSessions()[0]
	if info.Cols != 100 || info.Rows != 30 || info.ActiveClient != "browser" || s.pendingCols != 120 {
		t.Fatal("pending dimensions advertised as applied")
	}
	s.failResize = false
	if err := a.ResizeWebTerminal(s.id, "desktop", 120, 40, true); err != nil {
		t.Fatal(err)
	}
	if err := a.ResizeWebTerminal(s.id, "browser", 90, 20, false); err == nil {
		t.Fatal("inactive browser resized")
	}
	if err := a.WebTerminalInput(s.id, "browser", "", 65536, 24, false); err == nil {
		t.Fatal("size overflow accepted")
	}
	if err := a.WebTerminalInput(s.id, "browser", "not base64", 80, 24, true); err == nil {
		t.Fatal("invalid binary accepted")
	}
	if len(s.writes) != 3 || string(s.writes[2]) != "\x00\xff" {
		t.Fatalf("binary semantics: %q", s.writes)
	}
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				err = a.WebTerminalInput(s.id, fmt.Sprint(i), "user", 80+i, 24, false)
			} else {
				err = a.SessionWrite(s.id, "reply")
			}
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if len(s.writes) != 43 {
		t.Fatal("lost concurrent writes")
	}
	if err := a.CloseSession(s.id); err != nil {
		t.Fatal(err)
	}
	if len(a.GetWebSessions()) != 0 {
		t.Fatal("closed session in inventory")
	}
	if a.WebTerminalInput(s.id, "browser", "bad", 80, 24, false) == nil {
		t.Fatal("write to closed session accepted")
	}
}

func TestWebTerminalSnapshotCapabilityOrderingAndLifecycle(t *testing.T) {
	a := webTestApp(t)
	browser := webTestBrowserContext(t, a)
	s := addControlledTerminal(a, "snapshot", true)
	requests := make(chan *application.CustomEvent, 8)
	a.webTransport.native = func(e *application.CustomEvent) {
		if nativeWebEvent(e.Name) {
			requests <- e
		}
	}
	a.emit("session:data", map[string]any{"id": s.id, "data": "old"})
	results := make(chan error, 1)
	go func() {
		snapshot, err := a.GetWebTerminalSnapshot(context.Background(), s.id)
		if err == nil && snapshot.ANSI != "exact" {
			err = fmt.Errorf("incorrect snapshot: %+v", snapshot)
		}
		results <- err
	}()
	request := <-requests
	payload := request.Data.(map[string]any)
	id := payload["requestId"].(string)
	if len(id) < 32 {
		t.Fatal("predictable request ID")
	}
	seq := payload["sequence"].(uint64)
	snapshot := WebTerminalSnapshot{SessionID: s.id, Sequence: seq, Cols: 80, Rows: 24, ANSI: "exact", State: map[string]any{"parser": "ground"}}
	if a.ProvideWebTerminalSnapshot(context.Background(), s.id, snapshot) == nil {
		t.Fatal("session ID accepted as capability")
	}
	if a.ProvideWebTerminalSnapshot(browser, id, snapshot) == nil {
		t.Fatal("browser authored authoritative snapshot")
	}
	stale := snapshot
	stale.Sequence = seq - 1
	if a.ProvideWebTerminalSnapshot(context.Background(), id, stale) == nil {
		t.Fatal("stale snapshot accepted")
	}
	a.emit("session:data", map[string]any{"id": s.id, "data": "after"})
	if err := a.ProvideWebTerminalSnapshot(context.Background(), id, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if a.ProvideWebTerminalSnapshot(context.Background(), id, snapshot) == nil {
		t.Fatal("capability reused")
	}
	go func() { _, err := a.GetWebTerminalSnapshot(context.Background(), s.id); results <- err }()
	<-requests
	if err := a.CloseSession(s.id); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-results:
		if err == nil {
			t.Fatal("closed snapshot succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("close left pending snapshot")
	}
}

func TestWebTerminalStartWaitsForNativeAndStartsOnce(t *testing.T) {
	a := webTestApp(t)
	browser := webTestBrowserContext(t, a)
	s := addControlledTerminal(a, "start", false)
	requests := make(chan *application.CustomEvent, 4)
	a.webTransport.native = func(e *application.CustomEvent) {
		if e.Name == "web:terminal-attach" {
			requests <- e
		}
	}
	result := make(chan error, 1)
	go func() {
		result <- a.SessionStart(browser, s.id, session.ConnectionConfig{InitialCols: 90, InitialRows: 30})
	}()
	req := <-requests
	id := req.Data.(map[string]any)["requestId"].(string)
	if s.connects.Load() != 0 {
		t.Fatal("started before desktop attached")
	}
	if a.AcknowledgeWebTerminal(browser, id) == nil {
		t.Fatal("browser acknowledged itself")
	}
	if err := a.AcknowledgeWebTerminal(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if s.connects.Load() != 1 {
		t.Fatalf("started %d times", s.connects.Load())
	}
	if info := a.GetWebSessions()[0]; info.Cols != 90 || info.Rows != 30 {
		t.Fatalf("start grid: %+v", info)
	}
}

func TestWebTerminalJournalBoundAndGap(t *testing.T) {
	a := webTestApp(t)
	s := addControlledTerminal(a, "journal", true)
	for i := 0; i < 2100; i++ {
		a.emit("session:data", map[string]any{"id": s.id, "data": strings.Repeat("x", 2048)})
	}
	j := a.webTransport.journals[s.id]
	if j.bytes > 2<<20 || len(j.frames) > 2048 || j.floor == 0 {
		t.Fatalf("unbounded journal: %+v", j)
	}
	if a.webTransport.validateSnapshot(s.id, 1, 0) == nil {
		t.Fatal("snapshot outside journal accepted")
	}
	a.emit("session:gap", map[string]any{"id": s.id})
	if a.webTransport.validateSnapshot(s.id, a.webTransport.terminalSequence(s.id), 0) == nil {
		t.Fatal("lossy authority claimed exact")
	}
	a.CloseSession(s.id)
	if len(a.webTransport.journals) != 0 {
		t.Fatal("closed journal retained")
	}
}

func TestWebTerminalCancelledSnapshotAndClosedStart(t *testing.T) {
	a := webTestApp(t)
	s := addControlledTerminal(a, "cancelled", false)
	requests := make(chan *application.CustomEvent, 1)
	a.webTransport.native = func(e *application.CustomEvent) {
		if nativeWebEvent(e.Name) {
			requests <- e
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, err := a.GetWebTerminalSnapshot(ctx, s.id); result <- err }()
	request := <-requests
	id := request.Data.(map[string]any)["requestId"].(string)
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
	if err := a.ProvideWebTerminalSnapshot(context.Background(), id, WebTerminalSnapshot{SessionID: s.id, Cols: 80, Rows: 24}); err == nil {
		t.Fatal("expired snapshot accepted")
	}
	if err := a.CloseSession(s.id); err != nil {
		t.Fatal(err)
	}
	if err := a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}); err == nil {
		t.Fatal("closed terminal restarted")
	}
	// The asynchronous connect path must also reject a removed session, rather
	// than interpreting a missing control record as a non-terminal connection.
	a.launchConnectGoroutine(s, "local", session.ConnectionConfig{})
	if s.connects.Load() != 0 {
		t.Fatal("closed terminal connected")
	}
	a.emit("session:data", map[string]any{"id": s.id, "data": "late output"})
	if len(a.webTransport.journals) != 0 {
		t.Fatal("late output resurrected closed journal")
	}
}
