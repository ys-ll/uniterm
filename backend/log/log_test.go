package log

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestWritef_FormatsWithTimestamp verifies the Writef line layout:
// ISO-ish timestamp + space + message + newline.
func TestWritef_FormatsWithTimestamp(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)

	Writef("hello %s", "world")

	path := filepath.Join(dir, ".uniterm", "uniterm.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasSuffix(line, " hello world") {
		t.Fatalf("unexpected line: %q", line)
	}
	// Timestamp prefix: 23 chars (YYYY-MM-DD HH:MM:SS.mmm)
	if len(line) < len("YYYY-MM-DD HH:MM:SS.mmm hello world") {
		t.Fatalf("line too short: %q", line)
	}
}

// TestWritef_NoFileInit_NoCrash covers Writef before Init or after Close.
func TestWritef_NoFileInit_NoCrash(t *testing.T) {
	mu.Lock()
	saved := file
	file = nil
	mu.Unlock()
	t.Cleanup(func() {
		mu.Lock()
		file = saved
		mu.Unlock()
	})

	// Should not panic.
	Writef("noop after close")
}

// TestInit_Idempotent verifies Init can be called repeatedly without
// leaking file handles.
func TestInit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	for i := 0; i < 3; i++ {
		if err := Init(); err != nil {
			t.Fatalf("Init iteration %d: %v", i, err)
		}
	}
	t.Cleanup(Close)

	mu.Lock()
	count := file
	mu.Unlock()
	if count == nil {
		t.Fatal("file handle should be set after repeated Init")
	}
}

// TestClose_ResetsFile verifies Close nils the file pointer.
func TestClose_ResetsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	Close()

	mu.Lock()
	post := file
	mu.Unlock()
	if post != nil {
		t.Fatalf("expected file nil after Close, got %v", post)
	}
}

// TestWritef_ConcurrentSafe drives Writef from many goroutines.
func TestWritef_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			Writef("goroutine %d message", id)
		}(i)
	}
	wg.Wait()

	path := filepath.Join(dir, ".uniterm", "uniterm.log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != 32 {
		t.Fatalf("expected 32 lines (one per goroutine), got %d", len(lines))
	}
}

// TestLogPath_HomeDir verifies logPath joins HomeDir + .uniterm/uniterm.log.
func TestLogPath_HomeDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	got := logPath()
	want := filepath.Join(dir, ".uniterm", "uniterm.log")
	if got != want {
		t.Fatalf("logPath = %q, want %q", got, want)
	}
}

// TestWritef_PreservesEmbeddedJSON: any structured content embedded in a
// Writef call must survive the timestamp prefix.
func TestWritef_PreservesEmbeddedJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)

	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	t.Cleanup(Close)

	payload := map[string]any{"a": 1, "b": "x"}
	raw, _ := json.Marshal(payload)
	Writef("payload=%s", string(raw))

	data, err := os.ReadFile(filepath.Join(dir, ".uniterm", "uniterm.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, raw) {
		t.Fatalf("log missing payload: %s", data)
	}
}
