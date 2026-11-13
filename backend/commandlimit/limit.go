// Package commandlimit defines bounds shared by MCP command executors and settings.
package commandlimit

const (
	Default = 8
	Maximum = 64
)

// Normalize preserves the default for older configurations and bounds manually
// edited values. There is deliberately no unlimited mode.
func Normalize(value int) int {
	if value <= 0 {
		return Default
	}
	if value > Maximum {
		return Maximum
	}
	return value
}
