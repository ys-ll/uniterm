package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Reads the login shell's own history file (~/.zsh_history, ~/.bash_history)
// so a freshly opened local terminal can suggest commands the user already
// runs, instead of starting from nothing.
//
// Local panels only — the frontend decides that. These are this machine's
// commands, so offering them inside an ssh session to some other host would be
// noise at best. Nothing is written back, nothing leaves the machine, and the
// frontend runs every entry through the same secret filter (shouldSkipCommand)
// it applies to uniTerm's own history before anything is shown.

// Newest entries only. Shell history files grow without bound and the whole
// list is held in memory and scanned per keystroke.
const shellHistoryLimit = 1000

// shellHistoryTailBytes caps how much of the history file is read. The whole
// file is never loaded: a multi-hundred-megabyte ~/.bash_history would
// otherwise cause a memory spike for data the limit above throws away anyway.
// parseShellHistory only needs the newest commands, which live at the end.
const shellHistoryTailBytes = 4 << 20 // 4 MiB

// zsh with EXTENDED_HISTORY writes ": <started>:<elapsed>;<command>".
var zshTimestampPrefix = regexp.MustCompile(`^: \d+:\d*;`)

// shellHistoryPath locates the history file for the user's login shell, or ""
// when there is nothing sensible to read.
func shellHistoryPath() string {
	if histFile := os.Getenv("HISTFILE"); histFile != "" {
		return histFile
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	switch shell := os.Getenv("SHELL"); {
	case strings.Contains(shell, "zsh"):
		return filepath.Join(home, ".zsh_history")
	case strings.Contains(shell, "bash"):
		return filepath.Join(home, ".bash_history")
	}
	// Unknown or unset SHELL: take whichever file exists, zsh first since it is
	// the macOS default. fish is deliberately not handled — its history is YAML
	// rather than one command per line and needs its own parser.
	for _, name := range []string{".zsh_history", ".bash_history"} {
		path := filepath.Join(home, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// parseShellHistory returns commands newest first, de-duplicated, capped at
// limit. Invalid UTF-8 is dropped rather than rejected: history files
// accumulate whatever was typed, including broken encodings, and one bad byte
// should not cost the user the whole file.
func parseShellHistory(raw string, limit int) []string {
	lines := strings.Split(strings.ToValidUTF8(raw, ""), "\n")

	commands := make([]string, 0, len(lines))
	var pending []string
	for _, line := range lines {
		if len(pending) == 0 {
			line = zshTimestampPrefix.ReplaceAllString(line, "")
			// bash writes "#<epoch>" comment lines when HISTTIMEFORMAT is set.
			if strings.HasPrefix(line, "#") {
				continue
			}
		}
		// A trailing backslash encodes a newline inside the command.
		if strings.HasSuffix(line, `\`) {
			pending = append(pending, strings.TrimSuffix(line, `\`))
			continue
		}
		pending = append(pending, line)
		command := strings.TrimSpace(strings.Join(pending, "\n"))
		pending = nil

		// Genuinely multi-line commands (loops, heredocs) cannot be offered as
		// a single-line completion. A trailing newline alone trims away above,
		// so this only drops the ones that really span lines.
		if command == "" || strings.Contains(command, "\n") {
			continue
		}
		commands = append(commands, command)
	}

	// Newest first, keeping the most recent occurrence of a repeated command.
	seen := make(map[string]struct{}, len(commands))
	recent := make([]string, 0, limit)
	for i := len(commands) - 1; i >= 0 && len(recent) < limit; i-- {
		if _, dup := seen[commands[i]]; dup {
			continue
		}
		seen[commands[i]] = struct{}{}
		recent = append(recent, commands[i])
	}
	return recent
}

// readShellHistoryTail reads at most limit bytes from the end of path and
// reports whether the read was truncated. When the file is larger than the
// limit the first (partial) line is dropped — the read starts mid-entry, so
// that line is truncated garbage.
func readShellHistoryTail(path string, limit int64) ([]byte, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if info.Size() <= limit {
		raw, err := os.ReadFile(path)
		return raw, false, err
	}
	if _, err := file.Seek(info.Size()-limit, 0); err != nil {
		return nil, false, err
	}
	raw := make([]byte, limit)
	n, err := io.ReadFull(file, raw)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, false, err
	}
	raw = raw[:n]
	if _, rest, found := bytes.Cut(raw, []byte("\n")); found {
		return rest, true, nil
	}
	return raw, true, nil
}

// LoadShellHistory returns the local login shell's history, newest first.
// A missing or unreadable file is not an error — it just means there is
// nothing extra to suggest.
func (a *App) LoadShellHistory() ([]string, error) {
	path := shellHistoryPath()
	if path == "" {
		return []string{}, nil
	}
	raw, _, err := readShellHistoryTail(path, shellHistoryTailBytes)
	if err != nil {
		return []string{}, nil
	}
	return parseShellHistory(string(raw), shellHistoryLimit), nil
}
