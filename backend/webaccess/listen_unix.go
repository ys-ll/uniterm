//go:build unix

package webaccess

import (
	"errors"
	"syscall"
)

func setExclusiveListener(fd uintptr) error {
	// Go enables reuse before ListenConfig.Control. On macOS/BSD that permits
	// wildcard/specific overlaps, so a successful bind may not own every URL.
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 0)
}

func addressInUse(err error) bool { return errors.Is(err, syscall.EADDRINUSE) }
