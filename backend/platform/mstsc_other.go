//go:build !windows

package platform

import "fmt"

// LaunchMstsc is unavailable off Windows: mstsc.exe does not exist there.
func LaunchMstsc(opts MstscOptions) error {
	return fmt.Errorf("launching mstsc is only supported on Windows")
}
