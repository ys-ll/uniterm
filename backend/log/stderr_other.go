//go:build !windows

package log

// CaptureStderr is a no-op off Windows: the runtime panic/fatal redirect
// (kernel32 SetStdHandle) is Windows-specific. macOS and Linux panics from
// Finder/.desktop launches are still lost; a dup2-based equivalent would go
// here if that ever needs covering.
func CaptureStderr() {}
