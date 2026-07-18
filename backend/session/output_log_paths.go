package session

import (
	"os"
	"path/filepath"
	"strings"
)

// windowsReservedNames are file names that Windows treats as devices,
// regardless of extension. Comparison is case-insensitive.
var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// sanitizeLogName produces a filesystem-safe base name from a
// user-supplied connection name. Returns "" if the result would be
// empty; the caller should fall back to a session-id based default.
//
// Rules (spec §5.6):
//  1. Replace [/\:*?"<>|] and control bytes with '_'
//  2. Trim leading/trailing whitespace
//  3. Collapse runs of '_' to a single '_'
//  4. If the result (uppercase) matches a Windows reserved device
//     name, wrap it as _NAME_
//  5. Truncate to 100 chars
func sanitizeLogName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch {
		case r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' ||
			r == '"' || r == '<' || r == '>' || r == '|':
			b.WriteByte('_')
		case r < 0x20:
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	s := strings.TrimSpace(b.String())
	// Collapse consecutive underscores.
	var out strings.Builder
	out.Grow(len(s))
	prevUnderscore := false
	for _, r := range s {
		if r == '_' {
			if !prevUnderscore {
				out.WriteByte('_')
			}
			prevUnderscore = true
		} else {
			out.WriteRune(r)
			prevUnderscore = false
		}
	}
	s = out.String()
	// Windows reserved names.
	upper := strings.ToUpper(s)
	if _, ok := windowsReservedNames[upper]; ok {
		s = "_" + s + "_"
	}
	// Truncate.
	if len(s) > 100 {
		s = s[:100]
	}
	return s
}

// defaultSessionLogDir is the fallback log root: <home>/Documents/uniTerm/logs.
// Callers must MkdirAll before use. Returns a temp-dir path if the
// user's home cannot be determined.
func defaultSessionLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "uniTerm", "logs")
	}
	return filepath.Join(home, "Documents", "uniTerm", "logs")
}

// DefaultSessionLogDir exposes the OS-default log directory for the
// App layer (settings UI displays it as the placeholder path).
func DefaultSessionLogDir() string { return defaultSessionLogDir() }
