//go:build windows

package packetcap

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const sioRcvAll = uintptr(0x98000001) // SIO_RCVALL (IPv4)

var (
	ws2_32       = windows.NewLazySystemDLL("ws2_32.dll")
	procWSAIoctl = ws2_32.NewProc("WSAIoctl")
)

// platformCapturePackets is the platform-agnostic entry the engine calls. On
// Windows it transparently handles the elevation split: when this process is
// already elevated it captures in-process (raw socket), otherwise it re-launches
// itself elevated as a capture worker and streams the parsed packets back over
// a user-only named pipe. Either way the engine receives fully parsed *Packet
// values and applies filtering/counting locally.
func platformCapturePackets(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	if isProcessElevated() {
		return platformCapture(ctx, cfg, func(frame []byte) {
			if p := ParseFrame(frame, time.Now()); p != nil {
				onPacket(p)
			}
		})
	}
	return captureViaWorker(ctx, cfg, onPacket)
}


// errNoAdmin is used to tell the frontend the limitation clearly.
type adminRequiredError struct{ msg string }

func (e *adminRequiredError) Error() string { return e.msg }

// enumerateInterfaces returns local IPv4 interfaces best suited for capture,
// physical/real ones first. APIPA 169.254.x and loopback are kept as fallback
// entries so the user can still try them if nothing else works.
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
		hw := ifc.HardwareAddr.String()
		itf := Interface{Name: ifc.Name, IP: ip, HardwareAddr: hw, Flags: flags}
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

// localIPv4ByIndex maps interface index -> its primary IPv4 address string.
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

// platformCapture implements the Windows raw-socket capture (SIO_RCVALL).
func platformCapture(ctx context.Context, cfg CaptureConfig, onPacket func([]byte)) error {
	ip := cfg.InterfaceIP
	ipa := net.ParseIP(ip).To4()
	if ipa == nil {
		return fmt.Errorf("invalid interface IP %q", ip)
	}

	fd, err := windows.Socket(windows.AF_INET, windows.SOCK_RAW, windows.IPPROTO_IP)
	if err != nil {
		return &adminRequiredError{fmt.Sprintf("创建 raw socket 失败: %v（抓包需要管理员权限，请以管理员身份运行）", err)}
	}
	defer windows.Closesocket(fd)

	sa := &windows.SockaddrInet4{Addr: [4]byte{ipa[0], ipa[1], ipa[2], ipa[3]}}
	if err := windows.Bind(fd, sa); err != nil {
		return &adminRequiredError{fmt.Sprintf("绑定 %s 失败: %v（该网卡可能无法用于抓包，请换一块或检查权限）", ip, err)}
	}

	enabled := true
	var bytesRet uint32
	r1, _, callErr := procWSAIoctl.Call(
		uintptr(fd), sioRcvAll,
		uintptr(unsafe.Pointer(&enabled)), unsafe.Sizeof(enabled),
		0, 0, uintptr(unsafe.Pointer(&bytesRet)), 0, 0)
	if callErr != nil && callErr != syscall.Errno(0) || r1 == ^uintptr(0) {
		return &adminRequiredError{fmt.Sprintf("SIO_RCVALL 启用失败: %v（抓包需要管理员权限，请以管理员身份运行）", callErr)}
	}

	buf := make([]byte, 65536)
	consecErr := 0
	for {
		select {
		case <-ctx.Done():
			return errStopped
		default:
		}
		// 1s receive timeout so we can observe context cancellation between
		// packets (clean shutdown).
		tv := windows.Timeval{Sec: 1, Usec: 0}
		_ = windows.SetsockoptTimeval(fd, windows.SOL_SOCKET, windows.SO_RCVTIMEO, &tv)
		n, _, err := windows.Recvfrom(fd, buf, 0)
		if err != nil {
			transient := false
			if e, ok := err.(syscall.Errno); ok {
				switch e {
				case windows.WSAETIMEDOUT, windows.WSAEWOULDBLOCK,
					windows.WSAECONNRESET, windows.WSAECONNABORTED,
					windows.WSAENOBUFS, windows.WSAENETDOWN, windows.WSAENETRESET,
					windows.WSAEINTR, windows.WSAENETUNREACH, windows.WSAEHOSTUNREACH:
					transient = true
				}
			}
			if transient {
				// 高流量瞬间丢缓冲（WSAENOBUFS）或网络抖动属常见情况，继续抓包
				// 而不是终止整个会话——这是"扫描中自动中断"的一个来源。
				consecErr++
				if consecErr < 200 { // 连续瞬时错误过多才退出，防止死循环空转
					continue
				}
			}
			select {
			case <-ctx.Done():
				return errStopped
			default:
			}
			return fmt.Errorf("接收数据失败: %v", err)
		}
		consecErr = 0
		if n <= 0 {
			continue
		}
		// Windows raw sockets hand us the IP datagram (header included) with
		// no Ethernet framing. Some NIC drivers zero the IP header checksum /
		// total-length on receive; ParseFrame already tolerates that.
		frame := make([]byte, n)
		copy(frame, buf[:n])
		onPacket(frame)
	}
}
