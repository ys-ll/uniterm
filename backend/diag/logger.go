package diag

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

var (
	mu       sync.Mutex
	global   *Logger
	devBuild = os.Getenv("UNITERM_DEV") == "1"
	rotator  *Rotator
)

type Logger struct {
	mu      sync.Mutex
	dir     string
	path    string
	file    *os.File
	bw      *bufio.Writer
	flushCh chan struct{}
	closed  bool
}

// DiagConfig controls log rotation and minimum level. A nil value is
// equivalent to the defaults used in production.
type DiagConfig struct {
	FileSizeCap int64
	DirSizeCap  int64
	KeepFiles   int
	Level       Level
}

func defaultConfig() *DiagConfig {
	return &DiagConfig{
		FileSizeCap: 10 << 20,
		DirSizeCap:  50 << 20,
		KeepFiles:   5,
		Level:       LevelInfo,
	}
}

func Init(dir string, cfg *DiagConfig) error {
	mu.Lock()
	defer mu.Unlock()
	if global != nil {
		return nil
	}
	if cfg == nil {
		cfg = defaultConfig()
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "uniterm.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	l := &Logger{
		dir:     dir,
		path:    filepath.Join(dir, "uniterm.log"),
		file:    f,
		bw:      bufio.NewWriterSize(f, 64*1024),
		flushCh: make(chan struct{}, 1),
	}
	global = l
	configureRotator(dir, cfg)
	go l.flusher()
	return nil
}

// InitLegacy is kept for tests that don't care about DiagConfig.
func InitLegacy(dir string) error { return Init(dir, nil) }

func Close() {
	mu.Lock()
	defer mu.Unlock()
	if global == nil {
		return
	}
	global.closeLocked()
	global = nil
}

func (l *Logger) closeLocked() {
	if l.closed {
		return
	}
	l.closed = true
	close(l.flushCh)
	_ = l.bw.Flush()
	_ = l.file.Sync()
	_ = l.file.Close()
}

func (l *Logger) flusher() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		l.mu.Lock()
		if l.closed {
			l.mu.Unlock()
			return
		}
		_ = l.bw.Flush()
		l.mu.Unlock()
	}
}

func Debug(tag, msg string, fields map[string]any) { write(LevelDebug, tag, msg, fields) }
func Info(tag, msg string, fields map[string]any)  { write(LevelInfo, tag, msg, fields) }
func Warn(tag, msg string, fields map[string]any)  { write(LevelWarn, tag, msg, fields) }
func Error(tag, msg string, fields map[string]any) { write(LevelError, tag, msg, fields) }

// rb is the package-wide dedup window. Set in Init via the wired
// configureRotator (no-op stub until Task 9 wires it).
var rb = newRingBuffer(5 * time.Second)

func write(level Level, tag, msg string, fields map[string]any) {
	count, _ := rb.Merge(level, tag, msg, fields)
	if !allowRate(tag) {
		return
	}
	mu.Lock()
	l := global
	mu.Unlock()
	if l == nil {
		return
	}
	entry := Entry{
		Ts:         time.Now().UTC().Format(time.RFC3339Nano),
		Level:      level,
		Tag:        tag,
		Msg:        msg,
		Fields:     fields,
		DedupCount: count,
	}
	if devBuild {
		if _, file, line, ok := runtime.Caller(2); ok {
			entry.Caller = &Caller{File: filepath.Base(file), Line: line}
		}
		entry.Goroutine = goroutineID()
	}
	FillLevels(level)
	line, _ := json.Marshal(entry)
	line = append(line, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.bw.Write(line)
	rotateIfNeeded(l)
	select {
	case l.flushCh <- struct{}{}:
	default:
	}
}

// goroutineID returns the runtime goroutine id (best-effort, dev-only).
func goroutineID() string {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := buf[:n]
	const prefix = "goroutine "
	if idx := indexOf(s, prefix); idx >= 0 {
		rest := s[idx+len(prefix):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		return string(rest[:end])
	}
	return ""
}

func indexOf(b []byte, s string) int {
	for i := 0; i+len(s) <= len(b); i++ {
		if string(b[i:i+len(s)]) == s {
			return i
		}
	}
	return -1
}
