package packetcap

import (
	"log"
	"sync"
	"sync/atomic"
)

// CaptureConfig carries the options for one capture session.
type CaptureConfig struct {
	// InterfaceIP is the IPv4 address of the interface to capture on
	// (from Interface.IP). On Windows this is the address we bind the raw
	// socket to so only that interface's traffic is seen.
	InterfaceIP string `json:"interfaceIP"`
	// InterfaceName is a display name for messages/events.
	InterfaceName string `json:"interfaceName"`
	// Filter is a comma/space separated set of active simple filters
	// (see Filter.Active). Empty means capture everything.
	Filter string `json:"filter"`
	// MaxPackets caps the capture buffer so memory cannot grow unbounded.
	// When reached the capture stops itself (reason "limit").
	MaxPackets int `json:"maxPackets"`

	// Remote selects remote capture over SSH tcpdump instead of the local raw
	// socket. When true, RemoteConnID/RemoteIface are used and SSHConfig (filled
	// by the app layer from the saved connection) carries the resolved dial
	// target — it never crosses the JSON wire to the UI.
	Remote       bool   `json:"remote"`
	RemoteConnID string `json:"remoteConnId"`
	RemoteIface  string `json:"remoteIface"`

	// SSHConfig is populated internally (app layer) with the resolved
	// ConnectionConfig for the remote host. Not serialized.
	SSHConfig *ResolvedSSH `json:"-"`
}

// ResolvedSSH carries the fully-resolved SSH dial target for a remote capture.
// The app layer fills it from a saved connection (+ inline credential override);
// the engine only consumes it and never sends it back to the frontend.
type ResolvedSSH struct {
	// Host / Port / User / Password / KeyPath / KeyContent mirror the relevant
	// session.ConnectionConfig fields needed by DialSSHClient.
	Host       string
	Port       int
	User       string
	Password   string
	AuthType   string
	KeyPath    string
	KeyContent string
	IdentityID string
}

// Manager owns the lifecycle of capture sessions. Only a small number of
// concurrent captures are expected (typically one); each has its own raw
// socket. Stop cancels the goroutine so no leak occurs on tab close.
type Manager struct {
	mu       sync.Mutex
	captures map[string]*captureSession
	emitter  Emitter
}

// captureSession ties a capture id to its stop function and stats.
type captureSession struct {
	cancel func()
	count  atomic.Int64
}

func NewManager() *Manager {
	return &Manager{captures: make(map[string]*captureSession)}
}

// SetEventEmitter wires the manager to the frontend event bus.
func (m *Manager) SetEventEmitter(fn Emitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = fn
}

func (m *Manager) emit(name string, payload any) {
	m.mu.Lock()
	e := m.emitter
	m.mu.Unlock()
	if e != nil {
		e(name, payload)
	}
}

// ListInterfaces returns the capture-able interfaces, ordered with the most
// likely-to-work (real, non-loopback, has an IP, not APIPA) first.
func ListInterfaces() []Interface {
	return enumerateInterfaces()
}

// Stop stops the capture with the given id. A no-op for unknown/finished ids.
func (m *Manager) Stop(id string) {
	m.mu.Lock()
	s, ok := m.captures[id]
	delete(m.captures, id)
	m.mu.Unlock()
	log.Printf("pktcap Manager.Stop id=%q found=%v", id, ok)
	if ok && s.cancel != nil {
		s.cancel()
	}
}

// StopAll stops every running capture (app-shutdown safety net).
func (m *Manager) StopAll() {
	m.mu.Lock()
	all := m.captures
	m.captures = make(map[string]*captureSession)
	m.mu.Unlock()
	for _, s := range all {
		if s != nil && s.cancel != nil {
			s.cancel()
		}
	}
}
