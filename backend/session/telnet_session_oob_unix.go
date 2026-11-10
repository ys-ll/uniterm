//go:build !windows

package session

import (
	"net"
	"syscall"
)

// enableTelnetOOBInline turns on SO_OOBINLINE so telnet urgent data (the
// SYNCH IAC DM sequence: 0xFF sent out-of-band followed by the 0xF2 Data
// Mark in-band) is delivered inside the normal stream. Without it the
// kernel diverts the 0xFF byte to the out-of-band queue that conn.Read
// never touches, and the stray 0xF2 Data Mark byte leaks into the terminal
// and the session log (issue #1415).
func enableTelnetOOBInline(conn net.Conn) {
	tcp, ok := conn.(*net.TCPConn)
	if !ok {
		return
	}
	raw, err := tcp.SyscallConn()
	if err != nil {
		return
	}
	raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_OOBINLINE, 1)
	})
}
