package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ansiStripper removes ANSI escape sequences from a byte stream chunk
// by chunk. Because emitData is invoked with arbitrary chunk boundaries,
// an escape sequence may be split across two calls. State is tracked in
// the pending slice.
//
// Sequences handled:
//   - CSI: ESC '[' ... final byte (0x40-0x7E)
//   - OSC: ESC ']' ... (BEL 0x07 or ESC '\')
//   - SS2/SS3: ESC 'N' single-char / ESC 'O' single-char
//   - Single-char ESC: ESC + one letter
//   - BEL (0x07) outside OSC context is dropped
//
// Bytes preserved: printable ASCII (0x20-0x7E), UTF-8 continuation bytes
// (0x80-0xFF, passed through verbatim — UTF-8 multi-byte characters are
// never split by ANSI logic), and control bytes \r \n \t \b.
type ansiStripper struct {
	// pending holds bytes from an incomplete escape sequence carried over
	// from the previous Strip call. When Strip is called again, pending
	// is prepended to the new input before parsing resumes.
	pending []byte
}

// Strip returns in with all ANSI escape sequences removed. Incomplete
// sequences at the tail are held in the stripper until the next call.
func (s *ansiStripper) Strip(in []byte) []byte {
	if len(in) == 0 && len(s.pending) == 0 {
		return nil
	}
	data := in
	if len(s.pending) > 0 {
		data = append(s.pending, in...)
		s.pending = nil
	}
	out := make([]byte, 0, len(data))
	i := 0
	for i < len(data) {
		b := data[i]
		if b == 0x1b { // ESC
			end, complete := scanEscape(data, i)
			if !complete {
				// Incomplete tail: save for next chunk.
				s.pending = append(s.pending[:0], data[i:]...)
				return out
			}
			i = end
			continue
		}
		if b == 0x07 { // stray BEL, drop
			i++
			continue
		}
		out = append(out, b)
		i++
	}
	return out
}

// scanEscape parses one escape sequence starting at data[start] (which
// must be ESC 0x1b). Returns the index just past the sequence and true
// if a complete sequence was consumed; otherwise returns start and false.
func scanEscape(data []byte, start int) (int, bool) {
	if start+1 >= len(data) {
		return start, false // just ESC, need more
	}
	kind := data[start+1]
	switch kind {
	case '[': // CSI: ESC [ params final(0x40-0x7E)
		for j := start + 2; j < len(data); j++ {
			c := data[j]
			if c >= 0x40 && c <= 0x7e {
				return j + 1, true
			}
		}
		return start, false
	case ']': // OSC: ESC ] ... BEL or ESC \
		for j := start + 2; j < len(data); j++ {
			if data[j] == 0x07 {
				return j + 1, true
			}
			if data[j] == 0x1b && j+1 < len(data) && data[j+1] == '\\' {
				return j + 2, true
			}
		}
		return start, false
	case 'N', 'O': // SS2/SS3: ESC N x / ESC O x — one char follows
		if start+2 >= len(data) {
			return start, false
		}
		return start + 3, true
	default:
		// Single-char ESC + letter: consume 2 bytes.
		return start + 2, true
	}
}

// lineProcessor turns a raw byte stream (already ANSI-stripped) into a
// sequence of complete logical lines suitable for a human-readable log
// file. It models a one-row terminal: a cursor moves through a byte
// buffer, printable bytes overwrite (or extend) at the cursor, \b moves
// the cursor left, \r sends the cursor to column 0 (without clearing —
// subsequent bytes overwrite), and \n flushes the whole row.
//
// This is what SecureCRT / Xshell / PuTTY do in "Printable text" mode:
// the log ends up containing the text the user actually saw on screen,
// not every backspace-shuffle or CR-repaint intermediate.
//
// Rules:
//   - Printable byte → write at cursor, advance cursor (extending the
//     buffer when the cursor was already at the end).
//   - \b (0x08) → cursor moves left by one (bounded at 0). The buffer
//     is unchanged; the next write overwrites in place. This matches
//     the shell BS-space-BS erase pattern once you follow through.
//   - \r (0x0D) → cursor jumps to column 0 without clearing. This is
//     the key semantic: \r\n now correctly flushes the accumulated
//     line, and progress-bar-style repainting still yields the final
//     state because later writes overwrite the earlier ones in place.
//   - \n (0x0A) → append \n and flush the whole buffer; buffer + cursor
//     reset.
//   - \t (0x09) → treated as a printable byte and preserved.
//   - After flushTimeout of inactivity, the next Feed call flushes the
//     partial line (without appending \n) so long-running commands
//     without newline output — top, less, monitoring shells — still
//     make it into the file eventually. The buffer is preserved so the
//     next line's start doesn't get lost mid-write.
type lineProcessor struct {
	line         []byte
	pos          int
	lastActivity time.Time
	flushTimeout time.Duration
}

// Feed consumes in and returns whatever complete-line output is ready
// to append to the log file. Multi-byte UTF-8 sequences are opaque:
// backspace moves one byte at a time, so an inflight multi-byte
// character hit by \b could leak a partial rune. In practice, servers
// echo one grapheme per keystroke, so \b almost always lands on ASCII.
func (p *lineProcessor) Feed(in []byte) []byte {
	if p.flushTimeout == 0 {
		p.flushTimeout = 500 * time.Millisecond
	}
	if len(in) == 0 {
		return nil
	}
	now := time.Now()
	var out []byte
	// If we have been idle long enough and there is a partial line,
	// emit it before we start appending new bytes so timestamps in the
	// log roughly track the wall clock. The partial line stays in the
	// buffer — this is a flush, not a discard.
	if !p.lastActivity.IsZero() && len(p.line) > 0 && now.Sub(p.lastActivity) >= p.flushTimeout {
		out = append(out, p.line...)
	}
	for _, b := range in {
		switch b {
		case '\n':
			// The cursor may be somewhere in the middle of the line
			// (e.g. mid-\r overwrite); flush the whole buffer regardless.
			p.line = append(p.line, '\n')
			out = append(out, p.line...)
			p.line = p.line[:0]
			p.pos = 0
		case '\r':
			p.pos = 0
		case '\b':
			if p.pos > 0 {
				p.pos--
			}
		default:
			if p.pos < len(p.line) {
				p.line[p.pos] = b
			} else {
				p.line = append(p.line, b)
			}
			p.pos++
		}
	}
	p.lastActivity = now
	return out
}

// FlushPartial returns the current partial line (without appending \n)
// and empties the buffer. Called on Disable so the last-written line
// is not lost when the session ends without a trailing newline.
func (p *lineProcessor) FlushPartial() []byte {
	if len(p.line) == 0 {
		return nil
	}
	out := make([]byte, len(p.line))
	copy(out, p.line)
	p.line = p.line[:0]
	p.pos = 0
	p.lastActivity = time.Time{}
	return out
}

// Reset clears buffer and idle state — used when a logger is repurposed.
func (p *lineProcessor) Reset() {
	p.line = p.line[:0]
	p.pos = 0
	p.lastActivity = time.Time{}
}

// OutputLogger owns a single .log file for one session's lifetime.
// The zero value is a disabled logger; Enable installs a file, Disable
// closes it. All methods are safe for concurrent use.
type OutputLogger struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	stripper ansiStripper
	lines    lineProcessor
}

const bannerHeader = "=== uniTerm session log ==="

// Enable opens the log file and writes the header banner. Returns the
// final path. If dir is empty, defaultSessionLogDir() is used. If name
// sanitizes to empty, "session" is used as the base.
// Filename convention: sanitize(name) + "_" + yyyymmdd_hhmmss + ".log";
// on same-second name collision, "_2"/"_3"/... is appended before .log.
// Any previous file is closed first.
func (l *OutputLogger) Enable(dir, name, protocol string) (string, error) {
	if dir == "" {
		dir = defaultSessionLogDir()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir log dir %s: %w", dir, err)
	}
	base := sanitizeLogName(name)
	if base == "" {
		base = "session"
	}
	now := time.Now()
	stamp := now.Format("20060102_150405")

	var file *os.File
	var final string
	for suffix := 1; suffix <= 100; suffix++ {
		var candidate string
		if suffix == 1 {
			candidate = filepath.Join(dir, base+"_"+stamp+".log")
		} else {
			candidate = filepath.Join(dir, fmt.Sprintf("%s_%s_%d.log", base, stamp, suffix))
		}
		f, err := os.OpenFile(candidate, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			file = f
			final = candidate
			break
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("open log %s: %w", candidate, err)
		}
	}
	if file == nil {
		return "", fmt.Errorf("could not allocate log filename in %s", dir)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_, _ = fmt.Fprintf(l.file, "\n=== Ended: %s ===\n", now.Format("2006-01-02 15:04:05 -0700"))
		_ = l.file.Sync()
		_ = l.file.Close()
	}
	l.file = file
	l.path = final
	l.stripper = ansiStripper{}
	l.lines.Reset()

	fmt.Fprintf(file, "%s\nName: %s\nProtocol: %s\nStarted: %s\n\n",
		bannerHeader, name, protocol, now.Format("2006-01-02 15:04:05 -0700"))
	_ = file.Sync()
	return final, nil
}

// Disable writes the footer banner and closes the file. Idempotent.
func (l *OutputLogger) Disable() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	// Flush any buffered partial line so an unterminated last command
	// still appears in the log.
	if partial := l.lines.FlushPartial(); len(partial) > 0 {
		_, _ = l.file.Write(partial)
	}
	fmt.Fprintf(l.file, "\n=== Ended: %s ===\n", time.Now().Format("2006-01-02 15:04:05 -0700"))
	_ = l.file.Sync()
	_ = l.file.Close()
	l.file = nil
	l.path = ""
}

// Enabled reports whether a log file is currently open.
func (l *OutputLogger) Enabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file != nil
}

// Path returns the current log path, or "" if disabled.
func (l *OutputLogger) Path() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// WriteOutput strips ANSI, runs the byte stream through the line
// processor, and appends any complete lines to the log. No-op if
// disabled or if there is nothing to write. Errors from writing are
// swallowed — a session must not fail because a log file cannot be
// written.
func (l *OutputLogger) WriteOutput(data []byte) {
	if len(data) == 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	stripped := l.stripper.Strip(data)
	if len(stripped) == 0 {
		return
	}
	toWrite := l.lines.Feed(stripped)
	if len(toWrite) == 0 {
		return
	}
	if _, err := l.file.Write(toWrite); err == nil {
		_ = l.file.Sync()
	}
}
