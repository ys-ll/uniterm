package webaccess

import (
	"errors"
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

func setExclusiveListener(fd uintptr) error {
	// Winsock defines SO_EXCLUSIVEADDRUSE as ~SO_REUSEADDR. Leaving reuse off
	// alone does not reserve the address against subsequent overlapping binds.
	return windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, ^windows.SO_REUSEADDR, 1)
}

func addressInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, windows.WSAEADDRINUSE) {
		return true
	}
	// Exclusive binds can report WSAEACCES for an occupied/reserved port.
	// Permission failures from socket setup or other operations must still stop.
	var call *os.SyscallError
	return errors.As(err, &call) && call.Syscall == "bind" && errors.Is(call.Err, windows.WSAEACCES)
}
