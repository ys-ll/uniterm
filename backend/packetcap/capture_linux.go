//go:build linux

package packetcap

import (
	"context"
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// captureElevated is the non-root path on Linux: it reuses the single resident
// elevated daemon (pkexec/sudo, prompted once per app run) which owns the raw
// socket and streams packets back over a pipe.
func captureElevated(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	return linuxDaemonCapture(ctx, cfg, onPacket)
}

// platformCapture implements the Linux AF_PACKET raw capture. It binds a raw
// AF_PACKET socket to the selected interface and delivers L2 frames (with the
// Ethernet header included) to onPacket.
func platformCapture(ctx context.Context, cfg CaptureConfig, onPacket func([]byte)) error {
	iface, err := net.InterfaceByName(cfg.InterfaceName)
	if err != nil {
		iface, err = ifaceByIP(cfg.InterfaceIP)
	}
	if err != nil {
		return fmt.Errorf("找不到网卡 %q：%v", cfg.InterfaceIP, err)
	}

	fd, err := unix.Socket(unix.AF_PACKET, unix.SOCK_RAW, int(htons(unix.ETH_P_ALL)))
	if err != nil {
		return fmt.Errorf("创建 AF_PACKET socket 失败: %v（需要 root / CAP_NET_RAW）", err)
	}
	defer unix.Close(fd)

	sa := &unix.SockaddrLinklayer{
		Protocol: htons(unix.ETH_P_ALL),
		Ifindex:  iface.Index,
	}
	if err := unix.Bind(fd, sa); err != nil {
		return fmt.Errorf("绑定 %s 失败: %v", iface.Name, err)
	}

	// 1s receive timeout so we can observe context cancellation between frames.
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 1})

	buf := make([]byte, 65536)
	consecErr := 0
	for {
		select {
		case <-ctx.Done():
			return errStopped
		default:
		}
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			transient := false
			switch err {
			case unix.EAGAIN, unix.EINTR,
				unix.ENOBUFS, unix.ENETDOWN, unix.ENETRESET,
				unix.ECONNRESET, unix.ENETUNREACH, unix.EHOSTUNREACH:
				// 高流量瞬间丢缓冲（ENOBUFS）或网络抖动均属常见情况，继续抓包
				// 而不是终止整个会话——这与 Windows 侧对齐，是"扫描中自动中断"。
				transient = true
			}
			if transient {
				consecErr++
				// 连续瞬时错误过多才退出，防止死循环空转；成功收包清零。
				if consecErr < 200 {
					continue
				}
			}
			select {
			case <-ctx.Done():
				return errStopped
			default:
			}
			return fmt.Errorf("接收失败: %v", err)
		}
		consecErr = 0
		if n <= 0 {
			continue
		}
		frame := make([]byte, n)
		copy(frame, buf[:n])
		onPacket(frame)
	}
}

func ifaceByIP(ip string) (*net.Interface, error) {
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

// htons converts a 16-bit host-order value to network order. ETH_P_ALL only
// takes host order within the sockaddr field but being explicit avoids an
// endianness surprise when a frame-size header differs.
func htons(v uint16) uint16 {
	return v<<8 | v>>8
}
