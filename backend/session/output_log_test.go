package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnsiStripperBasic(t *testing.T) {
	var s ansiStripper
	got := string(s.Strip([]byte("hello \x1b[31mred\x1b[0m world")))
	want := "hello red world"
	if got != want {
		t.Errorf("Strip = %q, want %q", got, want)
	}
}

func TestAnsiStripperSplitAcrossChunks(t *testing.T) {
	var s ansiStripper
	// Split ESC [ 31 m across two calls: pending state must survive.
	a := string(s.Strip([]byte("hello \x1b[")))
	b := string(s.Strip([]byte("31mred\x1b[0m")))
	got := a + b
	if !strings.Contains(got, "hello red") {
		t.Errorf("split-chunk strip got %q, want to contain %q", got, "hello red")
	}
	if strings.Contains(got, "\x1b") {
		t.Errorf("ESC leaked through: %q", got)
	}
}

func TestAnsiStripperOSC(t *testing.T) {
	var s ansiStripper
	// OSC title set: ESC ] 0 ; title BEL
	got := string(s.Strip([]byte("prompt\x1b]0;mytitle\x07$ ")))
	want := "prompt$ "
	if got != want {
		t.Errorf("OSC strip = %q, want %q", got, want)
	}
}

func TestAnsiStripperPreservesControl(t *testing.T) {
	var s ansiStripper
	// \r \n \t \b must survive.
	got := string(s.Strip([]byte("a\r\nb\tc\bd")))
	want := "a\r\nb\tc\bd"
	if got != want {
		t.Errorf("control-byte strip = %q, want %q", got, want)
	}
}

func TestAnsiStripperUTF8(t *testing.T) {
	var s ansiStripper
	// Chinese chars are UTF-8 multi-byte; must pass through verbatim
	// even when interleaved with ANSI.
	got := string(s.Strip([]byte("你好 \x1b[32m世界\x1b[0m")))
	want := "你好 世界"
	if got != want {
		t.Errorf("UTF-8 strip = %q, want %q", got, want)
	}
}

func TestSanitizeLogName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"prod-switch-01", "prod-switch-01"},
		{"a/b:c*.log", "a_b_c_.log"},
		{"   trim me   ", "trim me"},
		{"a__b___c", "a_b_c"},
		{"", ""},
		{"CON", "_CON_"},
		{"con", "_con_"},
		{"COM1", "_COM1_"},
		{"COM10", "COM10"}, // COM10 is NOT reserved
		{"你好 服务器", "你好 服务器"},
		{strings.Repeat("x", 150), strings.Repeat("x", 100)},
	}
	for _, c := range cases {
		got := sanitizeLogName(c.in)
		if got != c.want {
			t.Errorf("sanitizeLogName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOutputLoggerBasic(t *testing.T) {
	var l OutputLogger
	dir := t.TempDir()

	path, err := l.Enable(dir, "test-conn", "ssh")
	if err != nil {
		t.Fatalf("Enable: %v", err)
	}
	if !l.Enabled() {
		t.Fatal("Enabled = false after Enable")
	}
	l.WriteOutput([]byte("hello \x1b[31mred\x1b[0m world\n"))
	l.WriteOutput([]byte("second line\n"))
	l.Disable()
	if l.Enabled() {
		t.Fatal("Enabled = true after Disable")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(content)
	if !strings.Contains(s, bannerHeader) {
		t.Errorf("header banner missing:\n%s", s)
	}
	if !strings.Contains(s, "Name: test-conn") {
		t.Errorf("Name line missing:\n%s", s)
	}
	if !strings.Contains(s, "Protocol: ssh") {
		t.Errorf("Protocol line missing:\n%s", s)
	}
	if !strings.Contains(s, "hello red world") {
		t.Errorf("ANSI not stripped:\n%s", s)
	}
	if strings.Contains(s, "\x1b") {
		t.Errorf("ESC leaked into log:\n%s", s)
	}
	if !strings.Contains(s, "=== Ended:") {
		t.Errorf("footer banner missing:\n%s", s)
	}
}

func TestOutputLoggerFileNameCollision(t *testing.T) {
	var l1, l2 OutputLogger
	dir := t.TempDir()

	p1, err := l1.Enable(dir, "same", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := l2.Enable(dir, "same", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	if p1 == p2 {
		t.Fatalf("same-second collision produced same path: %s", p1)
	}
	if filepath.Dir(p1) != filepath.Dir(p2) {
		t.Errorf("paths in different dirs: %s vs %s", p1, p2)
	}
	l1.Disable()
	l2.Disable()
}

func TestOutputLoggerDisableIdempotent(t *testing.T) {
	var l OutputLogger
	dir := t.TempDir()
	_, err := l.Enable(dir, "t", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	l.Disable()
	l.Disable()
	l.WriteOutput([]byte("after disable"))
}

func TestOutputLoggerWriteAfterDisable(t *testing.T) {
	var l OutputLogger
	dir := t.TempDir()
	path, err := l.Enable(dir, "t", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	l.WriteOutput([]byte("before\n"))
	l.Disable()
	l.WriteOutput([]byte("this should not land\n"))

	content, _ := os.ReadFile(path)
	if strings.Contains(string(content), "this should not land") {
		t.Errorf("write after disable landed: %s", content)
	}
}

func TestBaseSessionLogOnConnectRoundtrip(t *testing.T) {
	s := &baseSession{id: "x", sessionType: "ssh"}
	if s.AutoLogOnConnect() {
		t.Errorf("default AutoLogOnConnect should be false")
	}
	s.SetLogOnConnect(true)
	if !s.AutoLogOnConnect() {
		t.Errorf("AutoLogOnConnect not set")
	}
	s.SetLogOnConnect(false)
	if s.AutoLogOnConnect() {
		t.Errorf("AutoLogOnConnect not cleared")
	}
}

func TestBaseSessionEmitDataTeesToWriter(t *testing.T) {
	s := &baseSession{id: "abc12345xxx", sessionType: "ssh", title: "myconn"}

	// Install a writer that appends every byte received.
	var logged []byte
	s.SetOutputLogWriter(func(b []byte) { logged = append(logged, b...) })

	// Capture the frontend callback to ensure it still fires with raw data.
	var seen []byte
	s.SetOnDataCallback(func(b []byte) { seen = append(seen, b...) })

	payload := []byte("hello \x1b[32mgreen\x1b[0m\n")
	s.emitData(payload)

	if string(seen) != string(payload) {
		t.Errorf("frontend callback data mutated: %q", seen)
	}
	if string(logged) != string(payload) {
		t.Errorf("outputLogWriter did not receive raw payload: %q", logged)
	}
}

func TestBaseSessionEmitDataNoWriterIsSafe(t *testing.T) {
	s := &baseSession{id: "id", sessionType: "ssh"}
	// No writer installed. Must not panic.
	s.emitData([]byte("hello"))
}

func TestBaseSessionClearingWriterStopsDelivery(t *testing.T) {
	s := &baseSession{id: "id", sessionType: "ssh"}
	var seen []byte
	s.SetOutputLogWriter(func(b []byte) { seen = append(seen, b...) })
	s.emitData([]byte("first "))
	s.SetOutputLogWriter(nil)
	s.emitData([]byte("second"))
	if string(seen) != "first " {
		t.Errorf("post-clear delivery leaked: %q", seen)
	}
}

func TestOutputLoggerConcurrentWrites(t *testing.T) {
	var l OutputLogger
	dir := t.TempDir()
	path, err := l.Enable(dir, "conc", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				l.WriteOutput([]byte("goroutine data\n"))
			}
		}(g)
	}
	wg.Wait()
	l.Disable()
	content, _ := os.ReadFile(path)
	count := strings.Count(string(content), "goroutine data")
	if count != 1000 {
		t.Errorf("expected 1000 lines, got %d", count)
	}
}

func TestLineProcessorBackspace(t *testing.T) {
	// Typical server-echoed typo: 'helllo' then two BS-space-BS erases,
	// then continues to '... world\n'. Users see 'hello world' on
	// screen; the log should match.
	var p lineProcessor
	got := string(p.Feed([]byte("helllo\b \b\b \bo world\n")))
	want := "hello world\n"
	if got != want {
		t.Errorf("Feed = %q, want %q", got, want)
	}
}

func TestLineProcessorCarriageReturn(t *testing.T) {
	// Progress-bar style repaint: multiple \r-overwrites, only the last
	// state should reach the log.
	var p lineProcessor
	got := string(p.Feed([]byte("progress 10%\rprogress 50%\rprogress 100%\n")))
	want := "progress 100%\n"
	if got != want {
		t.Errorf("Feed = %q, want %q", got, want)
	}
}

func TestLineProcessorCRLFPassesThrough(t *testing.T) {
	// Regression: an earlier implementation cleared the line buffer on
	// \r and then flushed an empty line on \n, so servers that end
	// every line with \r\n (nearly all of them) lost every line of
	// output. \r must only move the cursor to column 0; the following
	// \n still flushes the buffered content.
	var p lineProcessor
	got := string(p.Feed([]byte("$ ls\r\ntotal 4\r\nfile.txt\r\n$ ")))
	want := "$ ls\ntotal 4\nfile.txt\n"
	if got != want {
		t.Errorf("Feed = %q, want %q", got, want)
	}
}

func TestLineProcessorFlushOnTimeout(t *testing.T) {
	// A partial line with no newline sits in the buffer, awaiting more
	// bytes. After flushTimeout the next Feed pushes the pending buffer
	// to output so long-running commands (top/less/monitor) don't lose
	// their content to buffering.
	p := lineProcessor{flushTimeout: 1 * time.Millisecond}
	out1 := string(p.Feed([]byte("partial line")))
	if out1 != "" {
		t.Errorf("first Feed emitted %q, expected empty", out1)
	}
	time.Sleep(5 * time.Millisecond)
	// Any subsequent Feed must trigger the timeout flush and emit the
	// pending line even before its terminating \n.
	out2 := string(p.Feed([]byte("!")))
	if !strings.HasPrefix(out2, "partial line") {
		t.Errorf("timeout flush missing: %q", out2)
	}
}

func TestLineProcessorFlushPartialOnDisable(t *testing.T) {
	// Disable happens mid-line (session ended without a trailing \n).
	// FlushPartial should return the pending buffer so the last
	// unterminated line is not silently discarded.
	var p lineProcessor
	_ = p.Feed([]byte("last partial"))
	got := string(p.FlushPartial())
	if got != "last partial" {
		t.Errorf("FlushPartial = %q, want %q", got, "last partial")
	}
	// Second call after empty state is a no-op.
	if x := p.FlushPartial(); len(x) != 0 {
		t.Errorf("second FlushPartial should be empty, got %q", x)
	}
}

func TestOutputLoggerLineBufferedEndToEnd(t *testing.T) {
	// Exercise the whole pipeline: ANSI stripping, backspace erase,
	// and line buffering all working together via WriteOutput.
	var l OutputLogger
	dir := t.TempDir()
	path, err := l.Enable(dir, "e2e", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	// Simulated server echo with a color escape and a typo the user
	// corrected before pressing Enter. The typed sequence is:
	//   $ echo helllo   (typo: 3 l's)
	//   \b\b\b          (erase 'o', 'l', 'l')  → '$ echo hel'
	//   lo world        (retype: 'lo world')   → '$ echo hello world'
	l.WriteOutput([]byte("\x1b[32m$ ec\x1b[0mho helllo\b\b\blo world\n"))
	l.Disable()

	content, _ := os.ReadFile(path)
	if !strings.Contains(string(content), "$ echo hello world") {
		t.Errorf("line-buffered echo missing:\n%s", content)
	}
	if strings.Contains(string(content), "helllo") {
		t.Errorf("uncorrected typo leaked into log:\n%s", content)
	}
	if strings.Contains(string(content), "\x1b") {
		t.Errorf("ESC leaked into log:\n%s", content)
	}
}

func TestOutputLoggerReusableAcrossWriters(t *testing.T) {
	// This mirrors the App-layer scenario where a single OutputLogger
	// stays alive across session disconnect/reconnect: both sessions'
	// output should land in the same file with no gap.
	var l OutputLogger
	dir := t.TempDir()
	path, err := l.Enable(dir, "reconnect", "ssh")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate session A writing a full line, disconnecting, session B
	// writing another. The logger is not touched between them —
	// mirroring the App's SetOutputLogWriter swap.
	l.WriteOutput([]byte("line from session A\n"))
	l.WriteOutput([]byte("line from session B\n"))
	l.Disable()

	content, _ := os.ReadFile(path)
	s := string(content)
	if !strings.Contains(s, "line from session A") {
		t.Errorf("missing session A output: %s", s)
	}
	if !strings.Contains(s, "line from session B") {
		t.Errorf("missing session B output: %s", s)
	}
}
