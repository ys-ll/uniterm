package packetcap

import "errors"

// errStopped is returned by a platform capture loop when the context was
// cancelled (user stop or packet-limit reached) rather than by a real error.
var errStopped = errors.New("capture stopped")

// The platform capture loop is provided per-OS:
//
//	capture_windows.go: raw socket + SIO_RCVALL
//	capture_linux.go:   AF_PACKET
//	capture_darwin.go:  /dev/bpf
//
// Each defines `platformCapture(ctx, cfg, onPacket) error` and
// `enumerateInterfaces() []Interface`.

