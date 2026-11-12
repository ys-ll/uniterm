package mcp

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePty is a scriptable Pty for tests: it replays queued output chunks on
// data listeners and can be marked disconnected.
type fakePty struct {
	alive        bool
	disconnected bool
	wrote        []string
	dataCbs      []func([]byte)
	statusCbs    []func(string)
}

type immediatePty struct{ fakePty }

func (f *immediatePty) Write(data []byte) error {
	f.emit("instant output\r\n")
	return f.fakePty.Write(data)
}

func TestCaptureInstalledBeforeWrite(t *testing.T) {
	r := NewRegistry()
	p := &immediatePty{fakePty{alive: true}}
	j, err := r.StartWithScreen(p, "s", "echo instant", "\n", time.Second, 0, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, _, _, _ := j.Snapshot()
	if !strings.Contains(string(out), "instant output") {
		t.Fatalf("missed synchronous command output: %q", out)
	}
}

func TestPassiveCollectionSharesEngineWithoutWritingOrLocking(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	observer, err := r.ObserveWithScreen(p, "s", 80*time.Millisecond, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.wrote) != 0 {
		t.Fatal("passive collection wrote to terminal")
	}
	job, err := r.StartWithScreen(p, "s", "cmd", "\n", time.Second, 0, 80, 24)
	if err != nil {
		t.Fatalf("observer locked session: %v", err)
	}
	p.emit("shared output\r\n")
	p.complete(job, 0)
	waitDone(t, observer, time.Second)
	waitDone(t, job, time.Second)
	a, _, _, state, _ := observer.Snapshot()
	b, _, _, _, _ := job.Snapshot()
	if string(a) != string(b) || state != StateTimeout || !strings.Contains(string(a), "shared output") {
		t.Fatalf("different execution and observation output: %q / %q (%s)", a, b, state)
	}
	if len(p.wrote) != 1 || !strings.Contains(p.wrote[0], "eval 'cmd'") {
		t.Fatalf("unexpected writes: %v", p.wrote)
	}
}

// A fresh emulator grid is blank, but the stream's absolute cursor addressing
// already points at the prompt row before any content is painted — the rows
// in between were real screen content from before the job started. Emitting
// them fronts the AI result with a wall of leading blank lines the real
// screen never shows.
func TestFreshScreenSkipsLeadingBlankRows(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	// ConPTY jumps the cursor to the prompt row before painting anything.
	p.emit("\x1b[20;1H")
	p.emit("prompt> ")
	p.emit("hello\r\nworld")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, code, state, _ := j.Snapshot()
	if state != StateExited || code != 0 {
		t.Fatalf("state=%s code=%d", state, code)
	}
	if strings.HasPrefix(string(out), "\n") {
		t.Fatalf("leading blank lines in output: %q", out)
	}
	if !strings.Contains(string(out), "hello") || !strings.Contains(string(out), "world") {
		t.Fatalf("missing content: %q", out)
	}
}

// The prompt line was painted before the job started, so the PTY stream never
// resends it — ConPTY only relays incremental input-line repaints that CUP
// into the middle of the prompt row. Seeding from the capture emulator's
// cursor row makes the first returned line start at the prompt instead of an
// unpainted gap.
func TestSeededPromptRowJoinsStreamRepaint(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 80, 24,
		ScreenSeed{Line: "PS C:\\Users\\me", X: 14, Y: 4})
	if err != nil {
		t.Fatal(err)
	}
	// ConPTY repaints the input line mid-prompt-row; the prompt text itself
	// never arrives.
	p.emit("\x1b[5;15H> echo hi")
	p.emit("\r\noutput-line\r\n")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, _, state, _ := j.Snapshot()
	if state != StateExited {
		t.Fatalf("state=%s", state)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if !strings.HasPrefix(lines[0], "PS C:\\Users\\me> echo hi") {
		t.Fatalf("seeded prompt missing from first line: %q", lines[0])
	}
	if len(lines) < 2 || lines[1] != "output-line" {
		t.Fatalf("missing output: %q", lines)
	}
}

// An all-blank cursor row must not be seeded: it would emit a blank filler
// line the stream never produced.
func TestSeedSkipsBlankCursorRow(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 80, 24,
		ScreenSeed{Line: "", X: 0, Y: 3})
	if err != nil {
		t.Fatal(err)
	}
	p.emit("hello")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, _, _, _ := j.Snapshot()
	if strings.Contains(string(out), "\n\n") {
		t.Fatalf("blank seed row leaked as filler: %q", out)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("missing content: %q", out)
	}
}

// A scroll at the bottom of a fresh grid (prompt row near the bottom, long
// echo wrapping past it) pushes undeclared rows into scrollback; they are
// pre-job screen content and must not be emitted, unlike the declared rows
// that scroll off with real content.
func TestScrollbackSkipsUndeclaredRows(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 40, 10)
	if err != nil {
		t.Fatal(err)
	}
	// Prompt row 7 declared, echo + output wraps past the bottom → scrolls.
	p.emit("\x1b[8;1Hprompt> ")
	p.emit("long-line-one\r\nlong-line-two\r\nlong-line-three\r\nlong-line-four\r\nlong-line-five\r\nlast")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, _, state, _ := j.Snapshot()
	if state != StateExited {
		t.Fatalf("state=%s", state)
	}
	if strings.HasPrefix(string(out), "\n") {
		t.Fatalf("undeclared scrollback rows leaked as leading blanks: %q", out)
	}
	for _, want := range []string{"long-line-one", "long-line-three", "long-line-five", "last"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestEmittedOutputSkipsUndeclaredRows(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	// Painted rows 0 and 4, jumped (CUP) over rows 1-3; later painted row 9
	// after jumping over 5-8. Only linefeed-reached or painted rows are real.
	p.emit("\x1b[1;1Htop")
	p.emit("\x1b[5;1Hline5")
	p.emit("\x1b[10;1Hbottom")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out := string(j.screen.LinesSince(0, j.screen.CursorAbs())) + j.screen.FlushFrom(j.screen.CursorAbs())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want exactly 3 lines, got %d: %q", len(lines), lines)
	}
	if lines[0] != "top" || lines[1] != "line5" || lines[2] != "bottom" {
		t.Fatalf("unexpected lines: %q", lines)
	}
}

// Interior blank lines produced by real linefeeds must survive emission.
func TestEmittedOutputKeepsGenuineBlankLines(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "echo hi", "\n", time.Second, 0, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	p.emit("\x1b[1;1Hone\r\n\r\n\r\nfour")
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out := string(j.screen.LinesSince(0, j.screen.CursorAbs())) + j.screen.FlushFrom(j.screen.CursorAbs())
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 || lines[0] != "one" || lines[1] != "" || lines[2] != "" || lines[3] != "four" {
		t.Fatalf("want one/blank/blank/four, got %q", lines)
	}
}

func TestConcurrentStartsShareSessionLock(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	results := make(chan error, 32)
	for range 32 {
		wg.Go(func() {
			_, err := r.Start(&fakePty{alive: true}, "same-session", "cmd", "\n", time.Minute, 0)
			results <- err
		})
	}
	wg.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		} else if err != ErrSessionBusy {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("started %d jobs in one session", success)
	}
	r.mu.Lock()
	id := r.busy["same-session"]
	r.mu.Unlock()
	_ = r.Interrupt(id)
}

// A command typed after the previous one's wait timed out supersedes it: the
// wedged job is finished as killed, an interrupt reaches the terminal, and the
// new command runs — a missed completion marker can no longer wedge the
// session until the job TTL.
func TestTakeOverSupersedesTimedOutJob(t *testing.T) {
	r := NewRegistry()
	r.takeoverSettle = 0
	p := &fakePty{alive: true}
	old, err := r.Start(p, "s", "sleep 60", "\n", 40*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	waitState(t, old, StateTimeout)

	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatalf("timed-out job did not yield the session: %v", err)
	}
	if _, _, _, st, jerr := old.Snapshot(); st != StateKilled || jerr != ErrSuperseded {
		t.Fatalf("old job state=%q err=%v, want killed/%v", st, jerr, ErrSuperseded)
	}
	// First the original command, then the supersede interrupt, then the new
	// command.
	if len(p.wrote) != 3 || p.wrote[1] != "\x03" || !strings.Contains(p.wrote[2], "next") {
		t.Fatalf("unexpected writes: %q", p.wrote)
	}
	_ = r.Interrupt(next.ID)
}

// Same takeover rule for a job that is still nominally running but has held
// the session longer than the takeover grace (marker will never arrive —
// typed into a TUI or a non-shell prompt).
func TestTakeOverGraceSupersedesLongHeldSession(t *testing.T) {
	r := NewRegistry()
	r.takeoverGrace = 50 * time.Millisecond
	r.takeoverSettle = 0
	p := &fakePty{alive: true}
	old, err := r.Start(p, "s", "vim", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)

	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatalf("long-held job did not yield the session: %v", err)
	}
	if _, _, _, st, jerr := old.Snapshot(); st != StateKilled || jerr != ErrSuperseded {
		t.Fatalf("old job state=%q err=%v, want killed/%v", st, jerr, ErrSuperseded)
	}
	_ = r.Interrupt(next.ID)
}

// Freshly started jobs keep rejecting with ErrSessionBusy so genuinely
// concurrent tool calls serialize instead of killing each other.
func TestTakeOverDoesNotKillFreshRunningJob(t *testing.T) {
	r := NewRegistry()
	r.takeoverSettle = 0
	p := &fakePty{alive: true}
	old, err := r.Start(p, "s", "sleep 60", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Interrupt(old.ID) })
	if _, err := r.Start(p, "s", "next", "\n", time.Second, 0); err != ErrSessionBusy {
		t.Fatalf("fresh running job superseded: %v", err)
	}
	if _, _, _, st, _ := old.Snapshot(); st != StateRunning {
		t.Fatalf("old job state=%q, want running", st)
	}
}

// Manual keyboard input on the session finishes the foreground job (the
// keystrokes land in the running command's stdin, breaking its completion
// coherence) and frees the lock; input on a session without a job is a no-op.
func TestUserInputKillsForegroundJob(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.Start(p, "s", "sleep 60", "\n", time.Minute, 0)
	if err != nil {
		t.Fatal(err)
	}
	r.UserInput("s")
	if _, _, _, st, jerr := j.Snapshot(); st != StateKilled || jerr != ErrUserInterrupted {
		t.Fatalf("state=%q err=%v, want killed/%v", st, jerr, ErrUserInterrupted)
	}
	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatalf("lock not released after user input: %v", err)
	}
	t.Cleanup(func() { _ = r.Interrupt(next.ID) })
	r.UserInput("other-session")
	if _, _, _, st, _ := next.Snapshot(); st != StateRunning {
		t.Fatalf("unrelated session input killed the job: state=%q", st)
	}
}

func TestOutputCapPreservesPollingOffsets(t *testing.T) {
	j := &Job{}
	j.appendLocked(&j.stdout, []byte("prefix"))
	j.appendLocked(&j.stdout, []byte(strings.Repeat("x", defaultOutputCap*2)))
	if len(j.stdout) != defaultOutputCap || !strings.HasPrefix(string(j.stdout), "prefix") {
		t.Fatalf("output cap did not preserve prefix: length %d", len(j.stdout))
	}
	j.appendLocked(&j.stdout, []byte("ignored"))
	if len(j.stdout) != defaultOutputCap {
		t.Fatal("output grew past cap")
	}
}

func (f *fakePty) Write(data []byte) error {
	f.wrote = append(f.wrote, string(data))
	return nil
}
func (f *fakePty) OnData(cb func([]byte)) func() {
	f.dataCbs = append(f.dataCbs, cb)
	return func() {}
}
func (f *fakePty) OnStatusChange(cb func(string)) func() {
	f.statusCbs = append(f.statusCbs, cb)
	if f.disconnected {
		cb("disconnected")
	}
	return func() {}
}
func (f *fakePty) Alive() bool             { return f.alive }
func (f *fakePty) ForegroundShell() string { return "bash" }
func (f *fakePty) complete(j *Job, code int) {
	f.emit(fmt.Sprintf("\x1b[8m__UT_%s;%d__\x1b[0m", j.ID, code))
}
func (f *fakePty) emit(data string) {
	for _, cb := range f.dataCbs {
		cb([]byte(data))
	}
}
func (f *fakePty) disconnect() {
	f.disconnected = true
	for _, cb := range f.statusCbs {
		cb("disconnected")
	}
}

func TestForegroundWaitsForCompletionAcrossQuietOutput(t *testing.T) {
	reg := NewRegistry()
	pty := &fakePty{alive: true}
	job, err := reg.Start(pty, "s1", "ls -la", "\n", 5*time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	pty.emit("total 12\n")
	pty.emit("a.txt\n")
	// Reproduce ping's one-second output gaps. Silence and prompt-shaped
	// output must never finish the job.
	pty.emit("$ fake prompt\n")
	time.Sleep(1100 * time.Millisecond)
	_, _, _, pending, _ := job.Snapshot()
	if pending != StateRunning {
		t.Fatalf("silence completed job: %s", pending)
	}
	pty.emit("last reply\nstatistics\n")
	pty.complete(job, 7)

	waitDone(t, job, 500*time.Millisecond)
	_, _, code, state, rerr := job.Snapshot()
	if state != StateExited {
		t.Fatalf("state=%s want exited", state)
	}
	if rerr != nil {
		t.Fatalf("err=%v", rerr)
	}
	if code != 7 {
		t.Fatalf("exit=%d", code)
	}
	if !strings.Contains(pty.wrote[0], "ls -la") {
		t.Fatalf("command not typed: %q", pty.wrote)
	}
}

func TestForegroundEOFOnDisconnect(t *testing.T) {
	reg := NewRegistry()
	pty := &fakePty{alive: true}
	job, _ := reg.Start(pty, "s1", "cat big.log", "\n", 5*time.Second, 0)
	pty.emit("partial line\n")
	pty.disconnect()

	waitDone(t, job, 500*time.Millisecond)
	_, _, _, state, rerr := job.Snapshot()
	if state != StateSessionClosed {
		t.Fatalf("state=%s want session_closed", state)
	}
	if rerr != ErrSessionClosed {
		t.Fatalf("err=%v", rerr)
	}
}

func TestForegroundHardTimeoutKeepsPollable(t *testing.T) {
	reg := NewRegistry()
	pty := &fakePty{alive: true}
	// Very short hard limit so the job moves to timeout without hanging.
	job, _ := reg.Start(pty, "s1", "sleep 3600", "\n", 40*time.Millisecond, 0)
	pty.emit("running...\n")

	waitState(t, job, StateTimeout)
	_, _, _, state, _ := job.Snapshot()
	if state != StateTimeout {
		t.Fatalf("state=%s want timeout", state)
	}
	select {
	case <-job.done:
		t.Fatal("timeout detached the output tap")
	default:
	}
	pty.emit("late output\n")
	resumed, ok := reg.Collect("s1", time.Second)
	if !ok || resumed != job {
		t.Fatal("collection did not reuse original job")
	}
	pty.complete(job, 0)
	waitDone(t, job, 500*time.Millisecond)
	out, _, code, state, _ := job.Snapshot()
	if state != StateExited || code != 0 || !strings.Contains(string(out), "late output") {
		t.Fatalf("late completion lost: %q %s %d", out, state, code)
	}
	// Still interruptible after timeout.
	if err := reg.Interrupt(job.ID); err != nil {
		t.Fatal(err)
	}
}

func waitState(t *testing.T, job *Job, state string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		_, _, _, current, _ := job.Snapshot()
		if current == state {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("job did not reach state %s", state)
}

func TestInterruptKillsRunning(t *testing.T) {
	reg := NewRegistry()
	pty := &fakePty{alive: true}
	job, _ := reg.Start(pty, "s1", "top", "\n", 5*time.Second, 0)
	if err := reg.Interrupt(job.ID); err != nil {
		t.Fatal(err)
	}
	waitDone(t, job, 200*time.Millisecond)
	_, _, _, state, _ := job.Snapshot()
	if state != StateKilled {
		t.Fatalf("state=%s want killed", state)
	}
}

func TestStartOnDeadSession(t *testing.T) {
	reg := NewRegistry()
	pty := &fakePty{alive: false}
	job, _ := reg.Start(pty, "s1", "ls", "\n", time.Second, 0)
	waitDone(t, job, 200*time.Millisecond)
	_, _, _, state, _ := job.Snapshot()
	if state != StateSessionClosed {
		t.Fatalf("state=%s", state)
	}
}

func TestTruncateCutsAtNewline(t *testing.T) {
	in := []byte("aaaa\nbbbb\ncccc\ndddd")
	out := Truncate(in, 12)
	if !strings.Contains(string(out), "truncated") {
		t.Fatalf("missing marker: %q", out)
	}
}

func waitDone(t *testing.T, job *Job, within time.Duration) {
	t.Helper()
	select {
	case <-job.done:
	case <-time.After(within):
		t.Fatal("job did not finish within", within)
	}
}

// firstLines returns the first n rendered lines, trimmed, joined with "\n"
// — a compact way to assert on the top of the screen.
func firstLines(s *Screen, n int) string {
	lines := s.Lines()
	if n > len(lines) {
		n = len(lines)
	}
	return strings.Join(lines[:n], "\n")
}

func TestNewClampsDimensions(t *testing.T) {
	s := NewScreen(1, 1)
	if s.Cols() != minCols || s.Rows() != minRows {
		t.Fatalf("NewScreen(1,1) = %dx%d, want %dx%d", s.Cols(), s.Rows(), minCols, minRows)
	}
	s = NewScreen(120, 40)
	if s.Cols() != 120 || s.Rows() != 40 {
		t.Fatalf("NewScreen(120,40) = %dx%d", s.Cols(), s.Rows())
	}
}

func TestScreenRender(t *testing.T) {
	tests := []struct {
		name  string
		cols  int
		rows  int
		input string
		want  string
	}{
		{
			name:  "plain text render",
			input: "hello world",
			want:  "hello world",
		},
		{
			// The ConPTY duplication killer: a bare \r overwrite of a
			// progress bar must collapse to a single final value.
			name:  "CR-overwrite progress bar",
			input: "10%\r20%\r30%",
			want:  "30%",
		},
		{
			// Prompt redraw via cursor-left re-emit: shell moves to col 0,
			// clears to eol and rewrites the line — no duplicated "$ ls".
			name:  "prompt redraw with EL",
			input: "$ ls\r\x1b[K$ ls -la",
			want:  "$ ls -la",
		},
		{
			name:  "prompt redraw via CUB overwrite",
			input: "$ ls -laXXX\x1b[3Dfix",
			want:  "$ ls -lafix",
		},
		{
			name:  "CUP positioning overwrites cell",
			input: "abcdef\x1b[1;2HX",
			want:  "aXcdef",
		},
		{
			name:  "CUP defaults to 1;1",
			input: "abcdef\x1b[HX",
			want:  "Xbcdef",
		},
		{
			name:  "EL0 clears cursor to eol",
			input: "abcdef\r\x1b[3Kx",
			want:  "x",
		},
		{
			name:  "EL1 clears bol to cursor",
			input: "abcdef\x1b[3D\x1b[1Kx",
			want:  "   xef",
		},
		{
			name:  "EL2 clears whole line",
			input: "abcdef\r\x1b[2Kx",
			want:  "x",
		},
		{
			name:  "ED2 clears screen",
			input: "one\r\ntwo\r\x1b[2Jdone",
			want:  "\ndone",
		},
		{
			name:  "ED1 clears start through cursor",
			input: "AAAA\r\nBBBB\r\nCCCC\x1b[2;3H\x1b[1J",
			want:  "\n   B\nCCCC",
		},
		{
			name:  "ED0 clears cursor through end",
			input: "AAAA\r\nBBBB\x1b[1;3H\x1b[0J",
			want:  "AA",
		},
		{
			name:  "LF scrolling drops first line",
			input: "1\r\n2\r\n3\r\n4\r\n5\r\n6",
			want:  "2\n3\n4\n5\n6",
		},
		{
			name:  "backspace moves cursor left",
			input: "abc\b\bX",
			want:  "aXc",
		},
		{
			name:  "backspace at col 0 is clamped",
			input: "\b\bX",
			want:  "X",
		},
		{
			name:  "tab advances to next 8-col stop",
			input: "a\tb",
			want:  "a       b",
		},
		{
			name:  "tab near eol clamps to last column",
			input: "\x1b[1;19Ha\tb",
			want:  strings.Repeat(" ", 18) + "ab",
		},
		{
			name:  "BEL is ignored",
			input: "a\x07b",
			want:  "ab",
		},
		{
			name:  "SGR sequences leave no artifacts",
			input: "\x1b[31mred\x1b[0m plain",
			want:  "red plain",
		},
		{
			name:  "concealed completion text does not appear in screen captures",
			input: "visible\r\n\x1b[8mhidden ack\x1b[0m\r\nnext",
			want:  "visible\n\nnext",
		},
		{
			name:  "RGB components do not change conceal state",
			input: "\x1b[38;2;8;0;28mvisible\x1b[0m",
			want:  "visible",
		},
		{
			name:  "OSC with BEL terminator is swallowed",
			input: "\x1b]0;window title\x07ok",
			want:  "ok",
		},
		{
			name:  "OSC with ST terminator is swallowed",
			input: "\x1b]8;;http://x\x1b\\link",
			want:  "link",
		},
		{
			name:  "DCS is swallowed until ST",
			input: "\x1bP1$r0;1|1\x1b\\x",
			want:  "x",
		},
		{
			name:  "SOS/PM/APC are swallowed until ST",
			input: "\x1bXsos\x1b\\\x1b^pm\x1b\\\x1b_apc\x1b\\z",
			want:  "z",
		},
		{
			name:  "charset designation swallows next byte",
			input: "\x1b(B\x1b)0ok",
			want:  "ok",
		},
		{
			name:  "private modes leave no artifacts",
			input: "\x1b[?25l\x1b[?1049h\x1b[?25hvisible",
			want:  "visible",
		},
		{
			name:  "DSR and device attrs are ignored",
			input: "\x1b[6n\x1b[0cok",
			want:  "ok",
		},
		{
			name:  "save/restore cursor",
			input: "abc\x1b7\r\n\x1b8z",
			want:  "abcz",
		},
		{
			name:  "SCP/RCP save/restore cursor",
			input: "abc\x1b[s\r\n\x1b[uz",
			want:  "abcz",
		},
		{
			name:  "IND is like LF",
			input: "a\x1bD\rb",
			want:  "a\nb",
		},
		{
			name:  "NEL is CR+LF",
			input: "abc\x1bEd",
			want:  "abc\nd",
		},
		{
			name:  "RI moves cursor up",
			input: "top\r\nbottom\x1b[2;1H\x1bMXXX",
			want:  "XXX\nbottom",
		},
		{
			name:  "RI at top reverse-scrolls",
			input: "1\r\n2\x1b[1;1H\x1bMx",
			want:  "x\n1\n2",
		},
		{
			name: "RIS clears screen and homes",
			// Note: ESC c consumes the 'c'; the text after RIS is "rest".
			input: "junk\x1b[3;3H\x1bcrest",
			want:  "rest",
		},
		{
			name:  "CNL/CPL move down/up and to col 0",
			input: "AAA\x1b[1EBBB\x1b[1FZ",
			want:  "ZAA\nBBB",
		},
		{
			name:  "SU scrolls content up",
			input: "1\r\n2\r\n3\r\n4\r\n5\x1b[2S",
			want:  "3\n4\n5",
		},
		{
			name:  "SD scrolls content down",
			input: "1\r\n2\r\n3\x1b[1T",
			want:  "\n1\n2\n3",
		},
		{
			name:  "IL inserts blank lines at cursor",
			input: "1\r\n2\r\n3\r\x1b[1Lx",
			want:  "1\n2\nx\n3",
		},
		{
			name:  "DL deletes line at cursor",
			input: "1\r\n2\r\n3\r\x1b[1M\x1b[2Ky",
			want:  "1\n2\ny",
		},
		{
			name:  "wide CJK chars render, one cell each",
			input: "你好Z",
			want:  "你好Z",
		},
		{
			name:  "combining marks do not consume a cell",
			input: "éx",
			want:  "ex",
		},
		{
			name:  "plain wrap at last column",
			cols:  minCols,
			input: strings.Repeat("A", minCols) + "B",
			want:  strings.Repeat("A", minCols) + "\nB",
		},
		{
			name:  "cursor movement clamps to bounds",
			input: "\x1b[999;999HX\x1b[999AY\x1b[999DZ",
			want:  "Z" + strings.Repeat(" ", 18) + "Y\n\n\n\n" + strings.Repeat(" ", 19) + "X",
		},
		{
			// Empty parameter fields and zero params default, not crash.
			name:  "malformed CSI params are tolerated",
			input: "abcdef\x1b[;;HX\x1b[0CZ",
			want:  "XbZdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewScreen(tt.cols, tt.rows)
			s.Feed([]byte(tt.input))
			if got := s.Text(); got != tt.want {
				t.Errorf("Text() =\n%q\nwant\n%q", got, tt.want)
			}
			if len(s.Lines()) != s.Rows() {
				t.Errorf("Lines() has %d rows, want %d", len(s.Lines()), s.Rows())
			}
		})
	}
}

func TestLinesNeverNilAndExactRowCount(t *testing.T) {
	s := NewScreen(80, 24)
	lines := s.Lines()
	if lines == nil {
		t.Fatal("Lines() is nil on fresh screen")
	}
	if len(lines) != 24 {
		t.Fatalf("len(Lines()) = %d, want 24", len(lines))
	}
}

func TestCursorLine(t *testing.T) {
	s := NewScreen(20, 5)
	s.Feed([]byte("a\nb\n"))
	if got := s.CursorLine(); got != 2 {
		t.Errorf("CursorLine() = %d, want 2", got)
	}
}

func TestLinesSinceRecoversScrolledLines(t *testing.T) {
	s := NewScreen(20, minRows)
	s.Feed([]byte("1\r\n2\r\n3\r\n4\r\n5\r\n6\r\n7"))
	// 6 newlines on a 5-row grid: rows "1" and "2" scrolled off.
	if got := s.AbsTop(); got != 2 {
		t.Fatalf("AbsTop() = %d, want 2", got)
	}
	if got := s.LinesSince(0, 2); got != "1\n2\n" {
		t.Errorf("LinesSince(0,2) = %q, want %q", got, "1\n2\n")
	}
	if got := s.LinesSince(2, 7); got != "3\n4\n5\n6\n7\n" {
		t.Errorf("LinesSince(2,7) = %q, want grid lines", got)
	}
	// Index 7 is past the grid bottom: not (yet) anywhere.
	if got := s.LinesSince(7, 8); got != "" {
		t.Errorf("LinesSince(7,8) = %q, want empty", got)
	}
	// Aged-out range below the scrollback window is skipped, not panicking.
	if got := s.LinesSince(-5, 0); got != "" {
		t.Errorf("LinesSince(-5,0) = %q, want empty", got)
	}
}

func TestWatermarkAcrossScrollBoundary(t *testing.T) {
	s := NewScreen(20, minRows)

	s.Feed([]byte("line1\r\nline2\r\n"))
	prev := s.CursorAbs()
	if got := s.LinesSince(0, prev); got != "line1\nline2\n" {
		t.Fatalf("chunk A emit = %q", got)
	}

	s.Feed([]byte("line3\r\nline4\r\n"))
	if cur := s.CursorAbs(); cur != prev+2 {
		t.Fatalf("CursorAbs after B = %d, want %d", cur, prev+2)
	}
	if got := s.LinesSince(prev, s.CursorAbs()); got != "line3\nline4\n" {
		t.Fatalf("chunk B emit = %q, want only new lines", got)
	}
	prev = s.CursorAbs()

	// Force several scrolls: emitted lines must stay unique across the
	// scrollback/grid boundary.
	s.Feed([]byte("l5\r\nl6\r\nl7\r\n"))
	if got := s.LinesSince(prev, s.CursorAbs()); got != "l5\nl6\nl7\n" {
		t.Fatalf("chunk C emit = %q, want l5+l6+l7 only", got)
	}
}

func TestFlushFrom(t *testing.T) {
	s := NewScreen(20, minRows)
	s.Feed([]byte("out1\r\nout2\r\npartial"))
	// Cursor sits mid-line on "partial"; trailing grid rows are blank and
	// must not be emitted.
	if got := s.FlushFrom(0); got != "out1\nout2\npartial\n" {
		t.Errorf("FlushFrom(0) = %q", got)
	}
	if got := s.FlushFrom(2); got != "partial\n" {
		t.Errorf("FlushFrom(2) = %q", got)
	}
	// Scroll part of it off; flush must stitch scrollback + grid.
	s.Feed([]byte("\r\nmore\r\n"))
	if got := s.FlushFrom(0); got != "out1\nout2\npartial\nmore\n" {
		t.Errorf("FlushFrom(0) after scroll = %q", got)
	}
	// Whole-blank screen flushes nothing.
	s2 := NewScreen(20, minRows)
	s2.Feed([]byte("x\r\n\x1b[2J"))
	if got := s2.FlushFrom(0); got != "" {
		t.Errorf("blank screen FlushFrom = %q, want empty", got)
	}
}

func TestTail(t *testing.T) {
	s := NewScreen(20, minRows)
	s.Feed([]byte("a\r\nb\r\n\r\nc"))
	if got := s.Tail(2); got != "b\nc" {
		t.Errorf("Tail(2) = %q, want %q", got, "b\nc")
	}
	if got := s.Tail(10); got != "a\nb\nc" {
		t.Errorf("Tail(10) = %q, want %q", got, "a\nb\nc")
	}
	if got := s.Tail(0); got != "" {
		t.Errorf("Tail(0) = %q, want empty", got)
	}
	if got := s.Tail(-1); got != "" {
		t.Errorf("Tail(-1) = %q, want empty", got)
	}
}

// TestRandomBytesNoPanic is a fuzz-style soak: the parser sees untrusted
// remote bytes, so no input may panic or corrupt internal invariants.
func TestRandomBytesNoPanic(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	buf := make([]byte, 100<<10)
	rng.Read(buf)

	s := NewScreen(120, 40)
	prevAbs := 0
	offset := 0
	for offset < len(buf) {
		// Random chunk sizes exercise the cross-Feed state machine and
		// the split-UTF-8 stash path.
		n := 1 + rng.Intn(997)
		if offset+n > len(buf) {
			n = len(buf) - offset
		}
		s.Feed(buf[offset : offset+n])
		offset += n

		if a := s.AbsTop(); a < prevAbs {
			t.Fatalf("AbsTop went backwards: %d -> %d", prevAbs, a)
		} else {
			prevAbs = a
		}
		// Cursor may legally move up (RI/CUU), so only bound-check it.
		if c := s.CursorAbs(); c < s.AbsTop() || c >= s.AbsTop()+s.Rows() {
			t.Fatalf("CursorAbs %d outside grid window [%d,%d)",
				c, s.AbsTop(), s.AbsTop()+s.Rows())
		}
		if len(s.Lines()) != s.Rows() {
			t.Fatalf("Lines() row count %d != Rows() %d", len(s.Lines()), s.Rows())
		}
		for _, line := range s.Lines() {
			if len([]rune(line)) > s.Cols() {
				t.Fatalf("line longer than cols: %q", line)
			}
		}
	}
}

// TestRandomBytesByteAtATime feeds the same corpus one byte per Feed to
// hammer the pending-UTF-8 and mid-sequence state paths.
func TestRandomBytesByteAtATime(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	buf := make([]byte, 16<<10)
	rng.Read(buf)
	s := NewScreen(80, 24)
	for _, b := range buf {
		s.Feed([]byte{b})
	}
}
