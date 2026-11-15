//go:build !linux

package packetcap

// serverLinuxDaemon only exists on Linux (daemon_linux.go). On every other
// platform this hook is a no-op and returns false so ServeWorker falls through
// to the normal worker (or the darwin daemon hook).
func serverLinuxDaemon(args []string) bool { return false }
