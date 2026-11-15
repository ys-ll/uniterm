//go:build !darwin

package packetcap

// serverDarwinDaemon only exists on darwin (daemon_darwin.go). On every other
// platform this hook is a no-op and returns false so ServeWorker falls through
// to the normal worker (or the linux daemon hook).
func serverDarwinDaemon(args []string) bool { return false }
