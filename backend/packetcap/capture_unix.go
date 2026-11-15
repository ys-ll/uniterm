//go:build !windows

package packetcap

import (
	"context"
	"net"
	"os"
	"time"
)

// platformCapturePackets is the platform-agnostic entry the engine calls. On
// Unix the raw-socket/BPF capture requires privileges (root, or CAP_NET_RAW).
// When the app runs as root we capture in-process; otherwise we hand the single
// privileged action (opening the raw socket) to an elevated worker subprocess
// and stream the packets back over a pipe, so the GUI itself never needs to run
// as root.
func platformCapturePackets(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	if os.Geteuid() == 0 {
		return platformCapture(ctx, cfg, func(frame []byte) {
			if p := ParseFrame(frame, time.Now()); p != nil {
				onPacket(p)
			}
		})
	}
	// Non-root: delegate the capture to an elevated worker. captureElevated is
	// implemented per-OS (Linux spawns a pkexec/sudo worker).
	return captureElevated(ctx, cfg, onPacket)
}

// enumerateInterfaces returns local IPv4 interfaces suited for capture, real
// ones first, loopback/APIPA as fallback.
func enumerateInterfaces() []Interface {
	byIP := localIPv4ByIndex()
	var out []Interface
	var real, fallback []Interface

	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		ip := byIP[ifc.Index]
		if ip == "" {
			continue
		}
		flags := describeFlags(ifc.Flags)
		itf := Interface{Name: ifc.Name, IP: ip, HardwareAddr: ifc.HardwareAddr.String(), Flags: flags}
		if ip[:7] == "169.254" || ip == "127.0.0.1" || (ifc.Flags&net.FlagLoopback) != 0 {
			fallback = append(fallback, itf)
		} else {
			real = append(real, itf)
		}
	}
	out = append(out, real...)
	out = append(out, fallback...)
	return out
}

// localIPv4ByIndex maps interface index -> its primary IPv4 address.
func localIPv4ByIndex() map[int]string {
	m := map[int]string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return m
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok {
				if ip4 := ipn.IP.To4(); ip4 != nil {
					m[ifc.Index] = ip4.String()
					break
				}
			}
		}
	}
	return m
}

func describeFlags(f net.Flags) string {
	var s []string
	if f&net.FlagUp != 0 {
		s = append(s, "up")
	}
	if f&net.FlagLoopback != 0 {
		s = append(s, "loopback")
	}
	if f&net.FlagRunning != 0 {
		s = append(s, "running")
	}
	if len(s) == 0 {
		return ""
	}
	out := s[0]
	for _, v := range s[1:] {
		out += "|" + v
	}
	return out
}

// platformCapture is defined per-OS in capture_linux.go / capture_darwin.go.
