// Package firewall implements the Linux Firewall Manager tool: it manages the
// host firewall (firewalld or ufw) of a remote Linux server over SSH, exposing
// a unified model to the frontend regardless of which backend is installed.
//
// The UI always speaks the same vocabulary — firewall on/off, allow/deny a
// port (with direction, protocol, source and persistence) — and this package
// translates that into the concrete command for the detected backend.
package firewall

// BackendKind identifies which host firewall tool was detected on the target.
type BackendKind string

const (
	BackendFirewalld BackendKind = "firewalld"
	BackendUfw       BackendKind = "ufw"
	BackendNone      BackendKind = "none"
)

// Status is the unified firewall status returned to the frontend.
type Status struct {
	// Backend is the detected firewall tool; empty when none is present.
	Backend BackendKind `json:"backend"`
	// Active is whether the firewall service is currently running/enabled.
	Active bool `json:"active"`
	// AutoStart is whether the firewall service is configured to start on boot
	// (systemctl-enabled). It is independent of Active: a firewall can be
	// running now but disabled at boot, or enabled at boot but stopped now.
	AutoStart bool `json:"autoStart"`
	// DefaultPolicy is the zone/default policy (e.g. "default", "enabled").
	DefaultPolicy string `json:"defaultPolicy"`
}

// Direction of a port rule. "in" means external clients may reach this port on
// the server (the common "open a port" case); "out" means the server may reach
// that port outbound.
type Direction string

const (
	DirectionIn  Direction = "in"
	DirectionOut Direction = "out"
)

// Protocol of a port rule.
type Protocol string

const (
	ProtocolTCP Protocol = "tcp"
	ProtocolUDP Protocol = "udp"
)

// Persistence of a port rule: permanent survives reboot, temporary does not.
type Persistence string

const (
	PersistPermanent  Persistence = "permanent"
	PersistTemporary  Persistence = "temporary"
)

// PortRule is one allow/deny rule shown in the list and used to add/remove it.
type PortRule struct {
	// Direction of the rule (in/out).
	Direction Direction `json:"direction"`
	// Protocol tcp or udp.
	Protocol Protocol `json:"protocol"`
	// Port is a single port number or a "start-end" range.
	Port string `json:"port"`
	// Source is the allowed source; empty/any means all sources (inbound only).
	Source string `json:"source,omitempty"`
	// Persist whether the rule is permanent (survives reboot).
	Persist Persistence `json:"persist"`
}

// Action for adding/removing a rule.
type Action string

const (
	ActionAllow Action = "allow"
	ActionDeny  Action = "deny"
)

// Emitter forwards firewall events to the frontend (wired to Wails event bus).
type Emitter func(name string, payload any)

// Event names emitted during firewall operations.
const (
	EventStatus = "firewall:status" // {host, status: Status}
	EventError  = "firewall:error"  // {host, message} error, permission, not installed
)

// ResolvedSSH carries the fully-resolved SSH dial target. The app layer fills
// it from a saved connection (+ inline credential override); the engine only
// consumes it and never sends it back to the frontend.
type ResolvedSSH struct {
	Host       string
	Port       int
	User       string
	Password   string
	AuthType   string
	KeyPath    string
	KeyContent string
	IdentityID string
}
