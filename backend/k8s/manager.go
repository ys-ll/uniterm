package k8s

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
)

// EventEmitter 是 Manager 用来推送 watch 事件的抽象；
// 上层（App）把它接到 Wails runtime.EventsEmit。
type EventEmitter func(name string, payload any)

type Manager struct {
	mu      sync.RWMutex
	conns   map[string]*connection
	watches map[string]*watchHandle
	emit    EventEmitter
}

type connection struct {
	id      string
	client  *http.Client
	base    string
	watches map[string]struct{} // 属于本 conn 的 watchID 集合，Disconnect 时统一停
	onClose func()              // Disconnect 时触发（例如拆 SSH 隧道）；只调一次
}

type watchHandle struct {
	connID string
	cancel context.CancelFunc
}

func NewManager() *Manager {
	return &Manager{
		conns:   make(map[string]*connection),
		watches: make(map[string]*watchHandle),
		emit:    func(string, any) {},
	}
}

func (m *Manager) SetEventEmitter(e EventEmitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emit = e
}

// ListContexts parses the given kubeconfig YAML and returns context metadata.
func (m *Manager) ListContexts(kubeconfigYAML []byte) ([]ContextInfo, error) {
	kc, err := ParseBytes(kubeconfigYAML)
	if err != nil {
		return nil, err
	}
	return kc.ListContexts(), nil
}

func (m *Manager) Connect(_ context.Context, kubeconfigYAML []byte, contextName string) (string, error) {
	return m.ConnectWith(kubeconfigYAML, contextName, ConnectOptions{})
}

// ConnectOptions 是可选的建连参数：
//   - PresetID: 若非空则用作 connID，方便上层把 K8s 连接和外部资源（例如 SSH 隧道）
//     用同一个 key 关联；为空则内部生成 UUID。
//   - DialOverride: 透传给 BuildClientWithDial 的 dial 劫持。
//   - OnClose: Disconnect 时触发一次，用于拆隧道等外部资源清理。
type ConnectOptions struct {
	PresetID     string
	DialOverride DialFunc
	OnClose      func()
}

// ConnectWith 是 Connect 的可选参数版本，供 app.go 在需要 dial 劫持 / 生命周期回调时使用。
func (m *Manager) ConnectWith(kubeconfigYAML []byte, contextName string, opts ConnectOptions) (string, error) {
	kc, err := ParseBytes(kubeconfigYAML)
	if err != nil {
		return "", fmt.Errorf("kubeconfig: %w", err)
	}
	if contextName == "" {
		contextName = kc.CurrentContext
	}
	client, base, err := BuildClientWithDial(kc, contextName, opts.DialOverride)
	if err != nil {
		return "", fmt.Errorf("build client: %w", err)
	}
	id := opts.PresetID
	if id == "" {
		id = uuid.New().String()
	}
	m.mu.Lock()
	if _, exists := m.conns[id]; exists {
		m.mu.Unlock()
		return "", fmt.Errorf("k8s connection %q already exists", id)
	}
	m.conns[id] = &connection{
		id:      id,
		client:  client,
		base:    base,
		watches: make(map[string]struct{}),
		onClose: opts.OnClose,
	}
	m.mu.Unlock()
	return id, nil
}

func (m *Manager) Disconnect(connID string) {
	m.mu.Lock()
	conn, ok := m.conns[connID]
	if !ok {
		m.mu.Unlock()
		return
	}
	// 停掉所有属于本 conn 的 watch
	toStop := make([]string, 0, len(conn.watches))
	for wid := range conn.watches {
		toStop = append(toStop, wid)
	}
	onClose := conn.onClose
	delete(m.conns, connID)
	m.mu.Unlock()

	for _, wid := range toStop {
		m.StopWatch(wid)
	}
	if onClose != nil {
		onClose()
	}
}

func (m *Manager) Request(ctx context.Context, connID, method, path string, body []byte, contentType string) (int, []byte, error) {
	m.mu.RLock()
	conn, ok := m.conns[connID]
	m.mu.RUnlock()
	if !ok {
		return 0, nil, fmt.Errorf("connection %q not found", connID)
	}
	return Do(ctx, conn.client, conn.base, method, path, body, contentType)
}

func (m *Manager) StartWatch(connID, path string) (string, error) {
	watchID := uuid.New().String()
	eventName := "k8s:watch:" + watchID
	endName := "k8s:watch-end:" + watchID
	ctx, cancel := context.WithCancel(context.Background())

	// 先注册再启流：避免 stream 立即 EOF 时 onEnd 抢在注册之前跑，导致句柄泄漏。
	m.mu.Lock()
	conn, ok := m.conns[connID]
	if !ok {
		m.mu.Unlock()
		cancel()
		return "", fmt.Errorf("connection %q not found", connID)
	}
	client := conn.client
	base := conn.base
	emit := m.emit
	m.watches[watchID] = &watchHandle{connID: connID, cancel: cancel}
	conn.watches[watchID] = struct{}{}
	m.mu.Unlock()

	err := startWatchStream(ctx, client, base, path,
		func(ev WatchEvent) { emit(eventName, ev) },
		func(err error) {
			payload := map[string]any{"error": ""}
			if err != nil {
				payload["error"] = err.Error()
			}
			emit(endName, payload)
			// 主动清理 map
			m.mu.Lock()
			delete(m.watches, watchID)
			if c, ok := m.conns[connID]; ok {
				delete(c.watches, watchID)
			}
			m.mu.Unlock()
		},
	)
	if err != nil {
		m.mu.Lock()
		delete(m.watches, watchID)
		if c, ok := m.conns[connID]; ok {
			delete(c.watches, watchID)
		}
		m.mu.Unlock()
		cancel()
		return "", err
	}
	return watchID, nil
}

func (m *Manager) StopWatch(watchID string) {
	m.mu.Lock()
	wh, ok := m.watches[watchID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.watches, watchID)
	if c, ok := m.conns[wh.connID]; ok {
		delete(c.watches, watchID)
	}
	m.mu.Unlock()
	wh.cancel()
}
