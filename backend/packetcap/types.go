// Package packetcap implements the Packet Capture tool: it receives IP packets
// crossing a selected network interface in real time and streams them to the
// frontend as Wails events, with protocol/port filtering and CSV/pcap export.
//
// The capture engine is pure Go with no cgo and no Npcap/WinPcap dependency:
//   - Windows: raw socket (SOCK_RAW + IPPROTO_IP) bound to the selected
//     interface's IP with SIO_RCVALL enabled. Requires Administrator.
//   - Linux:   AF_PACKET socket bound to the interface. Requires root.
//   - macOS:   /dev/bpf* byte-queue device attached to the interface. Requires
//     root.
//
// Protocol parsing (IPv4/IPv6/TCP/UDP) is hand-rolled over the raw bytes, so it
// works identically across all three platforms.
package packetcap

import "time"

// Interface describes one capture-able network interface.
type Interface struct {
	// Name is the OS interface name (e.g. "以太网", "en0").
	Name string `json:"name"`
	// IP is the IPv4 address to bind/attach; empty for interfaces with none.
	IP string `json:"ip"`
	// HardwareAddr is the MAC address, MAC-style ("aa:bb:..."); may be empty.
	HardwareAddr string `json:"hardwareAddr"`
	// Flags is a human-readable description of interface flags.
	Flags string `json:"flags"`
}

// Packet is one captured packet as delivered to the frontend.
type Packet struct {
	// Time is the capture timestamp (local time, RFC3339 with millis).
	Time string `json:"time"`
	// Protocol is the L4 protocol name ("TCP", "UDP", "ICMP" or "p<N>").
	Protocol string `json:"protocol"`
	// Network is the L3 protocol ("IPv4" or "IPv6").
	Network string `json:"network"`
	// SourceIP / SourcePort of the packet (port 0 when not applicable).
	SourceIP   string `json:"sourceIP"`
	SourcePort uint16 `json:"sourcePort"`
	// DestIP / DestPort of the packet.
	DestIP   string `json:"destIP"`
	DestPort uint16 `json:"destPort"`
	// Service is the well-known service name for DestPort ("" when unknown).
	Service string `json:"service"`
	// Flags is a compact TCP flags string ("SYN", "ACK", "RST|ACK", ...); "" for non-TCP.
	Flags string `json:"flags"`
	// Len is the total captured byte length.
	Len int `json:"len"`
	// Data is the raw packet payload as hex (whole frame/IP datagram).
	Data string `json:"data"`
	// Fields is a Wireshark-style decoded protocol tree (one top-level node per
	// layer: Frame/Ethernet, IPv4/IPv6, ARP, TCP/UDP/ICMP...). Each node carries
	// a human-readable name/value plus absolute byte offset and length.
	Fields []FieldNode `json:"fields"`
}

// FieldNode is one row of the decoded protocol tree. Nodes with Children are
// collapsible layer headers; leaf nodes carry the actual decoded value.
type FieldNode struct {
	// Name is the field label, e.g. "源地址 (Source IP)".
	Name string `json:"name"`
	// Value is the human-readable value, e.g. "192.168.1.10".
	Value string `json:"value"`
	// Offset is the absolute byte offset of this field within the captured frame.
	Offset int `json:"offset"`
	// Len is the byte length of the field within the frame.
	Len int `json:"len"`
	// Children holds the field's sub-fields (layers → leaf fields, etc.).
	Children []FieldNode `json:"children,omitempty"`
}

// Emitter forwards capture events to the frontend (wired to Wails' event bus).
type Emitter func(name string, payload any)

// Event names emitted during a capture session. The "packet" payload carries
// {captureId, packet: Packet}, every other event carries a captureId too, so the
// frontend can isolate concurrent capture sessions.
const (
	EventInterfaceError = "packetcap:iface-error" // {captureId, message} fatal: cannot start on this interface
	EventPacket         = "packetcap:packet"      // {captureId, packet: Packet}
	EventStopped        = "packetcap:stopped"     // {captureId, reason, count}
	EventError          = "packetcap:error"       // {captureId, message} non-fatal stream error
)

// packetTime formats a capture timestamp for the frontend.
func packetTime(t time.Time) string {
	return t.Local().Format("15:04:05.000")
}
