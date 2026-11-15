//go:build darwin

package packetcap

import (
	"context"
	"fmt"
	"net"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// captureElevated is the non-root path on macOS: it reuses the single resident
// elevated daemon (osascript admin helper, prompted once per app run) which owns
// the /dev/bpf raw device and streams packets back over a pipe. See
// daemon_darwin.go for the persist-privilege design.
func captureElevated(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	return darwinDaemonCapture(ctx, cfg, onPacket)
}

// platformCapture implements the macOS Berkeley Packet Filter (/dev/bpf) raw
// capture. It opens a BPF device, binds it to the selected interface, enables
// promiscuous + immediate mode, and parses the BPF record stream into L2 frames
// (Ethernet header included) that it delivers to onPacket.
func platformCapture(ctx context.Context, cfg CaptureConfig, onPacket func([]byte)) error {
	iface, err := net.InterfaceByName(cfg.InterfaceName)
	if err != nil {
		iface, err = darwinIfaceByIP(cfg.InterfaceIP)
	}
	if err != nil {
		return fmt.Errorf("找不到网卡 %q：%v", cfg.InterfaceIP, err)
	}

	fd, err := darwinOpenBPF()
	if err != nil {
		return fmt.Errorf("打开 /dev/bpf 失败: %v", err)
	}
	defer unix.Close(fd)

	// Bump the kernel buffer so bursts don't drop records on a busy interface.
	const buflen = 1 << 20
	_ = unix.IoctlSetPointerInt(fd, unix.BIOCSBLEN, buflen)

	// Bind to the selected interface via BIOSETIF (ifreq containing the name).
	if err := darwinSetIf(fd, iface.Name); err != nil {
		return fmt.Errorf("绑定网卡 %s 失败: %v", iface.Name, err)
	}

	// Promiscuous mode.
	_ = unix.IoctlSetPointerInt(fd, unix.BIOCPROMISC, 1)
	// Deliver a packet as soon as it arrives (low latency).
	_ = unix.IoctlSetPointerInt(fd, unix.BIOCIMMEDIATE, 1)

	var readerr error
	buf := make([]byte, 65536)
	for {
		select {
		case <-ctx.Done():
			return errStopped
		default:
		}
		n, err := unix.Read(fd, buf)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			readerr = err
			break
		}
		if n <= 0 {
			continue
		}
		darwinEmitBPFRecords(buf[:n], onPacket)
	}
	if readerr != nil {
		return fmt.Errorf("读取 bpf 失败: %v", readerr)
	}
	return errStopped
}

// darwinOpenBPF opens the first available /dev/bpfN device (N = 0..7).
func darwinOpenBPF() (int, error) {
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("/dev/bpf%d", i)
		fd, err := unix.Open(name, unix.O_RDWR, 0)
		if err == nil {
			return fd, nil
		}
		if !os.IsNotExist(err) {
			continue // busy; try the next one
		}
	}
	return -1, fmt.Errorf("没有可用的 /dev/bpf 设备（可能需要 root 权限）")
}

// darwinSetIf issues BIOSETIF, passing the interface name in an ifreq buffer.
func darwinSetIf(fd int, name string) error {
	if len(name) >= 16 {
		name = name[:15]
	}
	// struct ifreq { char ifr_name[16]; ... } — the kernel only reads ifr_name
	// for BIOSETIF, so a 16-byte NUL-padded name buffer is sufficient.
	var buf [16]byte
	copy(buf[:], name)
	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(unix.BIOCSETIF), uintptr(unsafe.Pointer(&buf[0])))
	if errno != 0 {
		return errno
	}
	return nil
}

// darwinEmitBPFRecords walks the BPF record stream, extracting each frame (data
// region after a struct bpf_hdr) and passing it to onPacket. Records are
// word-aligned: the next header starts at align4(hdrlen + caplen) past the
// current header.
func darwinEmitBPFRecords(data []byte, onPacket func([]byte)) {
	off := 0
	for off+28 <= len(data) { // bpf_hdr is 28 bytes on LP64 (24 fields + skew)
		// struct bpf_hdr on 64-bit macOS:
		//   bh_tstamp (timeval, 16B) | bh_caplen u32 (16) | bh_datalen u32 (20) | bh_hdrlen u32 (24)
		hlen := int(le32(data, 24))
		caplen := int(le32(data, 16))
		if hlen < 0 || hlen > 64 {
			return // corrupt header; stop
		}
		start := off + hlen
		end := start + caplen
		if end > len(data) {
			return // truncated final record
		}
		frame := make([]byte, caplen)
		copy(frame, data[start:end])
		onPacket(frame)
		off += wordAlign(hlen + caplen)
		if off >= len(data) {
			break
		}
	}
}

// le32 reads a little-endian uint32 at b[i:i+4].
func le32(b []byte, i int) int {
	return int(b[i]) | int(b[i+1])<<8 | int(b[i+2])<<16 | int(b[i+3])<<24
}

// wordAlign rounds x up to the next 4-byte (word) boundary, matching BPF_WORDALIGN.
func wordAlign(x int) int {
	return (x + 3) &^ 3
}

func darwinIfaceByIP(ip string) (*net.Interface, error) {
	target := net.ParseIP(ip)
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, ifc := range ifaces {
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.Equal(target) {
				c := ifc
				return &c, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface with IP %s", ip)
}
