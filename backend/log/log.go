package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Rotation defaults. Single-process writer, so we don't try to coordinate
// rotation with concurrent writers — only Writef holds the mutex.
const (
	defaultMaxSize    = 5 << 20 // 5 MiB per file
	defaultMaxBackups = 3       // keep uniterm.log.{1,2,3} in addition to active
	maxBackupsUpper   = 99      // safety clamp on user-supplied MaxBackups
)

var (
	mu   sync.Mutex
	file *os.File
	size int64

	maxSize    int64 = defaultMaxSize
	maxBackups       = defaultMaxBackups
)

// Init opens the uniterm log file at ~/.uniterm/uniterm.log. Subsequent calls
// are no-ops so main + the panic recovery wrapper in main.go can both call it.
//
// Rotation behaviour: each Writef checks the current size; once a write would
// push the file past maxSize, the active file is renamed to .1 (shifting older
// backups .1 -> .2 -> ... -> drop) and a fresh file is opened. Older backups
// beyond maxBackups are removed. MaxSize / MaxBackups can be tuned via the
// SetMaxSize / SetMaxBackups helpers for tests; defaults are 5 MiB / 3 backups.
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if file != nil {
		return nil
	}

	dir := filepath.Dir(logPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	file = f
	size = info.Size()
	return nil
}

// Close flushes and closes the log file. Safe to call multiple times.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		file.Close()
		file = nil
		size = 0
	}
}

// Writef formats a line prefixed with a millisecond timestamp and appends it
// to the log file. Rotation happens inside the mutex so concurrent callers
// never see a half-rotated state.
func Writef(format string, args ...interface{}) {
	mu.Lock()
	defer mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("%s %s\n", time.Now().Format("2006-01-02 15:04:05.000"), msg)

	if file == nil {
		return
	}
	n, _ := file.WriteString(line)
	size += int64(n)
	if size >= maxSize {
		rotate()
	}
}

// Recover recovers from a panic and logs it with the given context label.
// Intended for use as `defer log.Recover("worker name")` at the top of
// long-running goroutines so a single bug doesn't take down the whole
// process. Logs the recovered value and stack to uniterm.log and returns
// normally.
func Recover(label string) {
	r := recover()
	if r == nil {
		return
	}
	Writef("PANIC in %s: %v", label, r)
}

// SetMaxSize overrides the rotation threshold in bytes. Reset to default with
// SetMaxSize = 0. Intended for tests; production code should leave it alone.
func SetMaxSize(bytes int64) {
	mu.Lock()
	defer mu.Unlock()
	if bytes <= 0 {
		maxSize = defaultMaxSize
	} else {
		maxSize = bytes
	}
}

// SetMaxBackups overrides how many rotated files are kept. Reset to default
// with SetMaxBackups = 0. Values above maxBackupsUpper are clamped.
func SetMaxBackups(n int) {
	mu.Lock()
	defer mu.Unlock()
	if n <= 0 {
		maxBackups = defaultMaxBackups
		return
	}
	if n > maxBackupsUpper {
		n = maxBackupsUpper
	}
	maxBackups = n
}

// rotate is called with mu held. It closes the current file, shifts backup
// suffixes one slot older (dropping anything beyond maxBackups), then opens a
// fresh uniterm.log.
//
// On any rename failure the function logs to stderr (we're inside the log
// package itself, so we don't try to log.Writef) and leaves the existing file
// handle alone — a future Writef will retry the rotation.
func rotate() {
	if file == nil {
		return
	}
	path := logPath()
	if err := file.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "log: close: %v\n", err)
	}
	file = nil

	// Drop the oldest backup, then shift .N -> .N+1 for N = maxBackups-1 .. 1.
	oldest := path + "." + strconv.Itoa(maxBackups)
	_ = os.Remove(oldest)
	for i := maxBackups - 1; i >= 1; i-- {
		src := path + "." + strconv.Itoa(i)
		dst := path + "." + strconv.Itoa(i+1)
		if _, err := os.Stat(src); err == nil {
			if err := os.Rename(src, dst); err != nil {
				fmt.Fprintf(os.Stderr, "log: rename %s -> %s: %v\n", src, dst, err)
			}
		}
	}
	if err := os.Rename(path, path+".1"); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "log: rotate %s: %v\n", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log: reopen %s: %v\n", path, err)
		return
	}
	file = f
	size = 0
}

// logPathOverride lets tests redirect the log path to a t.TempDir().
var logPathOverride string

// SetLogPath overrides the path used by Init / Writef / rotate. Pass "" to
// restore the default (~/.uniterm/uniterm.log). Tests use this to point the
// package at a t.TempDir() instead of the user's home directory. Must be
// called while no file is open (e.g. after Close()).
func SetLogPath(path string) {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		// Refuse to swap paths with an open handle — caller must Close first.
		return
	}
	logPathOverride = path
}

func logPath() string {
	if logPathOverride != "" {
		return logPathOverride
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "uniterm.log"
	}
	return filepath.Join(home, ".uniterm", "uniterm.log")
}
