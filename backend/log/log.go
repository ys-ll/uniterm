// Package log is a backwards-compatibility shim: diag now owns the
// initialised file logger with NDJSON output, rate limiting, dedup, and
// rotation, so this package just delegates the legacy Writef path into
// diag.Info under the \"legacy\" tag. Init / Close are preserved as
// no-ops because every existing caller still expects them — main.go
// runs them around startup/shutdown regardless of which subsystem owns
// the underlying file handle.
package log

import (
	"fmt"

	"github.com/ys-ll/uniterm/backend/diag"
)

// Init is a no-op kept for the existing main.go wiring. diag.Init runs
// separately in main once the log directory is known.
func Init() error { return nil }

// Close is a no-op. diag.Close owns the flush + close of the underlying
// file handle.
func Close() {}

// Writef forwards the format+args into diag.Info under the shared
// "legacy" tag. The original timestamp + file logging behaviour is
// replaced by diag's NDJSON structure (each log line gets ts/level/tag/
// fields) — see backend/diag/logger.go for the new shape.
func Writef(format string, args ...interface{}) {
	diag.Info("legacy", fmt.Sprintf(format, args...), nil)
}
