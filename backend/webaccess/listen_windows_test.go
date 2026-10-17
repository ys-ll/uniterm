package webaccess

import (
	"net"
	"os"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestListenerScansWinsockBindConflicts(t *testing.T) {
	for _, tc := range []struct {
		call  string
		err   error
		calls int
	}{
		{"bind", windows.WSAEADDRINUSE, 2},
		{"bind", windows.WSAEACCES, 2},
		{"setsockopt", windows.WSAEACCES, 1},
		{"bind", windows.WSAEADDRNOTAVAIL, 1},
		{"bind", syscall.EACCES, 1},
	} {
		calls := 0
		_, err := listenFrom("127.0.0.1", 65534, func(string, string) (net.Listener, error) {
			calls++
			return nil, &net.OpError{Op: "listen", Err: os.NewSyscallError(tc.call, tc.err)}
		})
		if err == nil || calls != tc.calls {
			t.Fatalf("%s %v: calls=%d, err=%v", tc.call, tc.err, calls, err)
		}
	}
}
