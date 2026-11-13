package mcp

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/ys-ll/uniterm/backend/commandlimit"
)

// Job states shared by both modes. The MCP tool layer maps these onto the
// agent-facing state strings.
const (
	StateRunning       = "running"
	StateExited        = "exited"
	StateError         = "error"
	StateKilled        = "killed"
	StateTimeout       = "timeout"
	StateSessionClosed = "session_closed"
)

// Default caps; the MCP layer can override per call.
const (
	DefaultHardLimit     = 5 * time.Minute
	DefaultMaxOutput     = 64 * 1024
	defaultOutputCap     = DefaultMaxOutput * 4
	defaultJobTTL        = 10 * time.Minute
	defaultMaxConcurrent = commandlimit.Default

	// takeoverGrace is how long a still-running foreground job may hold the
	// session before a newer Start supersedes it instead of failing with
	// ErrSessionBusy. Without it, a command whose completion marker never
	// arrives (TUI program, command typed into a non-shell prompt, user
	// Ctrl+C mid-command) wedges the AI's terminal until the job TTL expires.
	takeoverGrace = time.Minute
	// takeoverSettle is the pause between the supersede interrupt and the new
	// command being typed, so the shell processes the SIGINT first and the new
	// bytes are not swallowed by the dying command's stdin.
	takeoverSettle = 200 * time.Millisecond
	// interruptWriteTimeout bounds the Ctrl+C write: a wedged PTY (SSH channel
	// window exhausted by a stuck remote) must never hold the session lock.
	interruptWriteTimeout = 2 * time.Second
)

// Job is one foreground execution lifecycle. Background jobs are tracked by the
// session package's own registry; this struct models the foreground path.
type Job struct {
	ID        string
	SessionID string

	mu       sync.Mutex
	stdout   []byte
	stderr   []byte
	exitCode int
	state    string
	err      error

	// screen is the redraw-resolving emulator fed by the data tap. When nil
	// (raw mode, used by tests and by Start) the job accumulates the raw
	// stream — which on Windows OpenSSH/ConPTY carries redraw frames the
	// screen never shows. Set via StartWithScreen.
	screen *Screen
	// watermark is the absolute line index of the last line already emitted
	// into stdout. Emission happens after each Feed: lines above the cursor
	// are final; the cursor line flushes at completion (FlushFrom).
	watermark int
	partial   string

	done       chan struct{}
	finished   bool
	completion *completionScanner
	waitUntil  time.Time
	// createdAt is the job's start time, used by the takeover rule: a newer
	// Start may supersede a job that has held the session longer than the
	// takeover grace, even while it still looks running.
	createdAt time.Time
	observe   bool
	released  bool // guarded by Registry.mu
}

// Snapshot returns a consistent copy of the current output/state.
func (j *Job) Snapshot() (stdout, stderr []byte, exitCode int, state string, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]byte, len(j.stdout))
	copy(out, j.stdout)
	errOut := make([]byte, len(j.stderr))
	copy(errOut, j.stderr)
	return out, errOut, j.exitCode, j.state, j.err
}

func (j *Job) setState(state string, exitCode int, err error) bool {
	j.mu.Lock()
	if j.finished {
		j.mu.Unlock()
		return false
	}
	j.state = state
	j.exitCode = exitCode
	j.err = err
	j.finished = state != StateRunning
	if j.finished {
		close(j.done)
	}
	j.mu.Unlock()
	return true
}

func (j *Job) appendLocked(dst *[]byte, b []byte) {
	// Keep a stable prefix so byte offsets remain valid throughout polling.
	remaining := defaultOutputCap - len(*dst)
	if len(b) > remaining {
		b = b[:remaining]
	}
	*dst = append(*dst, b...)
}

// Pty is the small live-session surface the foreground tap needs, implemented
// by the session layer (Write + data/status callbacks). Kept narrow so the
// registry tests against a fake.
type Pty interface {
	// Write sends bytes (the command + newline) to the live terminal.
	Write(data []byte) error
	// OnData registers a stream listener; returns an unsubscribe function.
	OnData(cb func(data []byte)) (off func())
	// OnStatusChange registers a connection-lost listener; returns unsub.
	OnStatusChange(cb func(status string)) (off func())

	// Alive reports whether the session is still connected at tap start.
	Alive() bool
}

// Registry tracks foreground jobs across all live sessions.
type Registry struct {
	mu            sync.Mutex
	jobs          map[string]*Job
	running       int
	maxConcurrent int
	// busy maps sessionID → running job id. One executor per session keeps
	// the live terminal coherent (see ErrSessionBusy). A newer Start can
	// still take the slot over when the holder is wedged (see start).
	busy map[string]string
	// newID and now are injectable for tests (deterministic ids and job ages);
	// defaults are uuid and time.Now.
	newID  func() string
	now    func() time.Time
	latest map[string]string
	// Supersede/interrupt knobs, defaulted from the package constants; tests
	// may shorten them to keep fast.
	takeoverGrace         time.Duration
	interruptWriteTimeout time.Duration
	takeoverSettle        time.Duration
}

// NewRegistry builds an empty registry with the uuid id generator.
func NewRegistry() *Registry {
	return &Registry{
		jobs:                  make(map[string]*Job),
		maxConcurrent:         defaultMaxConcurrent,
		busy:                  make(map[string]string),
		latest:                make(map[string]string),
		newID:                 func() string { return "fg-" + uuid.New().String() },
		now:                   time.Now,
		takeoverGrace:         takeoverGrace,
		interruptWriteTimeout: interruptWriteTimeout,
		takeoverSettle:        takeoverSettle,
	}
}

// SetMaxConcurrent changes the foreground job budget without terminating jobs.
// The one-executor-per-terminal restriction remains independent of this budget.
func (r *Registry) SetMaxConcurrent(limit int) {
	r.mu.Lock()
	r.maxConcurrent = commandlimit.Normalize(limit)
	r.mu.Unlock()
}

// Start types a command into the live terminal and returns a pollable job.
// A supported shell acknowledges command return with a per-job nonce and exit
// code. Silence and prompt text never count as completion. On a wait timeout
// the tap and session lock remain active so Collect can resume without loss.
//
// When another foreground job already holds the session, Start normally fails
// with ErrSessionBusy. If that holder is wedged — its first wait timed out, or
// it has held the session longer than the takeover grace — the newer command
// supersedes it instead: the old job is finished as killed with ErrSuperseded,
// an interrupt is typed into the terminal (bounded by interruptWriteTimeout),
// and after takeoverSettle the new command runs. This keeps a missed
// completion marker from wedging the AI's terminal until the job TTL.
func (r *Registry) Start(pty Pty, sessionID, command, newline string, hardLimit time.Duration, maxOutput int) (*Job, error) {
	return r.StartWithScreen(pty, sessionID, command, newline, hardLimit, maxOutput, 0, 0)
}

// StartWithScreen launches a job whose output is the redraw-resolved screen
// text produced by a vt emulator fed by the data tap (see the screen emulator below). On
// Windows OpenSSH/ConPTY the raw stream carries redraw frames the screen never
// displays; the emulator resolves them the same way an xterm.js buffer does.
// cols/rows <= 0 falls back to raw mode. An optional seed (usually the prompt
// line snapshotted from the session's capture emulator) paints the baseline
// the PTY stream never resends.
func (r *Registry) StartWithScreen(pty Pty, sessionID, command, newline string, hardLimit time.Duration, maxOutput int, cols, rows int, seed ...ScreenSeed) (*Job, error) {
	return r.start(pty, sessionID, command, newline, hardLimit, maxOutput, cols, rows, false, seed...)
}

// ObserveWithScreen collects future terminal output without typing a command.
// Read-only observers share the execution lifecycle but do not lock the session.
func (r *Registry) ObserveWithScreen(pty Pty, sessionID string, hardLimit time.Duration, cols, rows int) (*Job, error) {
	return r.start(pty, sessionID, "", "", hardLimit, DefaultMaxOutput, cols, rows, true)
}

func (r *Registry) start(pty Pty, sessionID, command, newline string, hardLimit time.Duration, maxOutput int, cols, rows int, observe bool, seed ...ScreenSeed) (*Job, error) {
	if hardLimit <= 0 {
		hardLimit = DefaultHardLimit
	}
	if maxOutput <= 0 {
		maxOutput = DefaultMaxOutput
	}

	r.mu.Lock()
	if r.running >= commandlimit.Normalize(r.maxConcurrent) {
		limit := commandlimit.Normalize(r.maxConcurrent)
		r.mu.Unlock()
		return nil, fmt.Errorf("%w (max %d); wait for or interrupt existing jobs, or adjust Maximum concurrent commands in MCP settings (higher limits increase memory use and terminal load)", ErrTooMany, limit)
	}
	var superseded *Job
	if _, taken := r.busy[sessionID]; taken && !observe {
		holder := r.jobs[r.busy[sessionID]]
		if !r.canTakeOverLocked(holder) {
			r.mu.Unlock()
			return nil, ErrSessionBusy
		}
		superseded = holder
	}
	r.running++

	job := &Job{
		ID:        r.newID(),
		SessionID: sessionID,
		state:     StateRunning,
		done:      make(chan struct{}),
		exitCode:  -1,
		waitUntil: r.now().Add(hardLimit),
		createdAt: r.now(),
		observe:   observe,
	}
	wireCommand := command
	if shell, ok := pty.(interface{ ForegroundShell() string }); ok && !observe {
		filterEncoding := ""
		if fe, ok := pty.(interface{ ExecFilterEncoding() string }); ok {
			filterEncoding = fe.ExecFilterEncoding()
		}
		if wrapped, scanner, supported := CompletionCommand(command, shell.ForegroundShell(), filterEncoding, job.ID, newline); supported {
			wireCommand = wrapped
			job.completion = scanner
		}
	}
	if cols > 0 && rows > 0 {
		job.screen = NewScreen(cols, rows)
		job.watermark = 0
		for _, sd := range seed {
			job.screen.Seed(sd)
		}
	}
	r.jobs[job.ID] = job
	if !observe {
		// Reassign the busy marker to the new job even when superseding: the
		// old job's release (below) only clears the marker when it still
		// points at the old id, so the new job's slot survives.
		r.busy[sessionID] = job.ID
		r.latest[sessionID] = job.ID
	}
	r.mu.Unlock()

	if superseded != nil {
		r.supersede(pty, superseded)
	}

	if !pty.Alive() {
		r.finish(job, StateSessionClosed, -1, ErrSessionClosed)
		r.release(job)
		return job, nil
	}

	ready := make(chan struct{})
	go r.watch(pty, job, hardLimit, maxOutput, ready)
	<-ready // Install taps before Write: a command may produce output immediately.
	if observe {
		return job, nil
	}
	if err := pty.Write([]byte(wireCommand + newline)); err != nil {
		r.finish(job, StateError, -1, err)
		return job, nil
	}

	return job, nil
}

func (r *Registry) watch(pty Pty, job *Job, hardLimit time.Duration, maxOutput int, ready chan struct{}) {
	defer r.release(job)

	var (
		eof   bool
		start = time.Now()
	)
	dataOff := pty.OnData(func(data []byte) {
		job.mu.Lock()
		if job.finished {
			job.mu.Unlock()
			return
		}
		code, complete := -1, false
		if job.completion != nil {
			data, code, complete = job.completion.feed(data)
		}
		if job.screen != nil {
			// Feed the emulator, then emit the logical lines that became
			// final with this chunk: everything above the cursor row. The
			// cursor line itself flushes at completion — it may still grow.
			// This is what kills the ConPTY duplicate-text problem: redraw
			// frames mutate the grid instead of appending bytes.
			job.screen.Feed(data)
			if chunk := job.screen.LinesSince(job.watermark, job.screen.CursorAbs()); chunk != "" {
				job.appendScreenLocked(chunk)
				job.partial = ""
				job.watermark = job.screen.CursorAbs()
			}
		} else {
			job.appendLocked(&job.stdout, data)
		}
		if complete && job.screen != nil {
			job.flushScreenLocked()
		}
		job.mu.Unlock()
		if complete {
			r.finish(job, StateExited, code, nil)
		}
	})
	statusOff := pty.OnStatusChange(func(status string) {
		job.mu.Lock()
		defer job.mu.Unlock()
		if status == "disconnected" || status == "error" {
			eof = true
		}
	})
	close(ready)
	defer func() {
		if dataOff != nil {
			dataOff()
		}
		if statusOff != nil {
			statusOff()
		}
	}()

	// flushTail pushes the not-yet-emitted remainder (cursor line and any
	// rows below the watermark) into stdout. Only meaningful in screen mode;
	// without it the last output line would sit on the cursor row forever.
	flushTail := func() {
		if job.screen == nil {
			return
		}
		job.mu.Lock()
		job.flushScreenLocked()
		job.mu.Unlock()
	}

	// Wait only for shell acknowledgement, disconnect, cancellation or deadline.
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-job.done:
			return
		case now := <-ticker.C:
			job.mu.Lock()
			closed, deadline := eof, job.waitUntil
			job.mu.Unlock()
			if closed {
				flushTail()
				r.finish(job, StateSessionClosed, -1, ErrSessionClosed)
				return
			}
			if now.Sub(start) >= max(defaultJobTTL, hardLimit) {
				flushTail()
				r.finish(job, StateTimeout, -1, nil)
				return
			}
			if !deadline.IsZero() && !now.Before(deadline) {
				if job.observe {
					flushTail()
					r.finish(job, StateTimeout, -1, nil)
					return
				}
				// A wait timeout is not a process exit. Keep the tap, screen
				// and session lock until real completion, cancellation or TTL.
				job.mu.Lock()
				changed := !job.finished && job.state != StateTimeout && job.waitUntil == deadline
				if changed {
					job.flushScreenLocked()
					job.state = StateTimeout
					job.waitUntil = time.Time{}
				}
				job.mu.Unlock()
			}
		}
	}
}

func (j *Job) appendScreenLocked(text string) {
	if j.partial != "" {
		if strings.HasPrefix(text, j.partial) {
			text = strings.TrimPrefix(text, j.partial)
		} else {
			// A redraw changed a partial line already returned on timeout.
			// Keep byte offsets stable and expose the replacement on a new line.
			text = "\n" + text
		}
	}
	j.appendLocked(&j.stdout, []byte(text))
}

func (j *Job) flushScreenLocked() {
	if j.screen == nil {
		return
	}
	tail := strings.TrimSuffix(j.screen.FlushFrom(j.watermark), "\n")
	j.appendScreenLocked(tail)
	j.partial = tail
}

// release frees the concurrency slot and the per-session busy marker. The
// busy marker is only cleared when it still points at this job — a finished
// job's slot may already have been re-used by a newer StartWithScreen.
func (r *Registry) release(job *Job) {
	r.mu.Lock()
	if job.released {
		r.mu.Unlock()
		return
	}
	job.released = true
	if r.running > 0 {
		r.running--
	}
	if cur, ok := r.busy[job.SessionID]; ok && cur == job.ID {
		delete(r.busy, job.SessionID)
	}
	r.mu.Unlock()
}

// Interrupt stops a running job: foreground taps just close the job (the typed
// command keeps running in the terminal, but the watcher stops collecting).
func (r *Registry) Interrupt(jobID string) error {
	r.mu.Lock()
	job, ok := r.jobs[jobID]
	r.mu.Unlock()
	if !ok {
		return ErrUnknownJob
	}
	job.mu.Lock()
	running := !job.finished
	job.mu.Unlock()
	if running {
		r.finish(job, StateKilled, -1, ErrInterrupted)
	}
	return nil
}

// canTakeOverLocked reports whether a newer Start may take the session slot
// from holder instead of failing with ErrSessionBusy. Callers hold r.mu.
// Two cases qualify: the holder's first wait already timed out (its completion
// marker was missed, so the AI was told the command timed out and moved on),
// or it has held the session longer than the takeover grace. Freshly started
// jobs keep rejecting with ErrSessionBusy so genuinely concurrent tool calls
// still serialize instead of killing each other.
func (r *Registry) canTakeOverLocked(holder *Job) bool {
	if holder == nil {
		return false
	}
	holder.mu.Lock()
	defer holder.mu.Unlock()
	if holder.finished || holder.state == StateTimeout {
		return true
	}
	return r.now().Sub(holder.createdAt) >= r.takeoverGrace
}

// supersede cancels the previous session holder: finish it as killed and stop
// its command with a bounded interrupt write, then pause briefly so the shell
// processes the SIGINT before the new command's bytes are typed. Runs outside
// r.mu; the old job's release sees the busy marker already reassigned to the
// new job and leaves it alone.
func (r *Registry) supersede(pty Pty, old *Job) {
	r.finish(old, StateKilled, -1, ErrSuperseded)
	_ = writeInterruptWithTimeout(pty, r.interruptWriteTimeout)
	time.Sleep(r.takeoverSettle)
}

// writeInterruptWithTimeout types Ctrl+C into the terminal, giving up on the
// write after timeout. The goroutine leaks only when the PTY write is wedged
// past the deadline — the bytes eventually fail or land on their own.
func writeInterruptWithTimeout(pty Pty, timeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		defer func() { _ = recover() }()
		errCh <- pty.Write([]byte{3})
	}()
	select {
	case err := <-errCh:
		return err
	case <-time.After(timeout):
		return ErrWriteTimeout
	}
}

// InterruptSession sends a terminal interrupt, then cancels the execution that
// owned the session when the interrupt began. Snapshot before writing: the write
// can synchronously complete that job, and a new job must not be cancelled.
// The write is best-effort with a deadline, and the job is finished even when
// the write fails or times out — a wedged PTY must not hold the session lock
// (a blocked Ctrl+C used to leave the session busy with no way to recover).
func (r *Registry) InterruptSession(sessionID string, sendInterrupt func() error) error {
	r.mu.Lock()
	job := r.jobs[r.busy[sessionID]]
	r.mu.Unlock()
	err := writeInterruptWithTimeout(
		writeFnPty{write: func([]byte) error { return sendInterrupt() }},
		r.interruptWriteTimeout,
	)
	if job != nil {
		r.finish(job, StateKilled, -1, ErrInterrupted)
	}
	return err
}

// writeFnPty adapts a bare write function to the Pty surface so the bounded
// interrupt helper can drive both real sessions and bare sendInterrupt funcs.
type writeFnPty struct{ write func([]byte) error }

func (w writeFnPty) Write(data []byte) error { return w.write(data) }
func (w writeFnPty) OnData(cb func([]byte)) (off func()) {
	return func() {}
}
func (w writeFnPty) OnStatusChange(cb func(string)) (off func()) {
	return func() {}
}
func (w writeFnPty) Alive() bool { return true }

// UserInput reports direct keyboard input from the user on a session whose
// foreground job is active. Any manual keystroke breaks the job's coherence —
// the bytes land in the running command's stdin and the completion marker may
// never arrive — so the job is finished as killed and the session lock freed.
// The caller still writes the user's bytes to the terminal in the normal way;
// no extra interrupt is sent here.
func (r *Registry) UserInput(sessionID string) {
	r.mu.Lock()
	job := r.jobs[r.busy[sessionID]]
	r.mu.Unlock()
	if job != nil {
		r.finish(job, StateKilled, -1, ErrUserInterrupted)
	}
}

// Collect resumes waiting on the original job, including buffered output
// produced between tool calls. It never sends a command to the terminal.
func (r *Registry) Collect(sessionID string, timeout time.Duration) (*Job, bool) {
	r.mu.Lock()
	job := r.jobs[r.latest[sessionID]]
	r.mu.Unlock()
	if job == nil {
		return nil, false
	}
	if timeout <= 0 {
		timeout = DefaultHardLimit
	}
	job.mu.Lock()
	if !job.finished {
		job.state = StateRunning
		job.waitUntil = time.Now().Add(timeout)
	}
	job.mu.Unlock()
	return job, true
}

// Lookup returns the job for polling.
func (r *Registry) Lookup(jobID string) (*Job, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	job, ok := r.jobs[jobID]
	return job, ok
}

func (r *Registry) finish(job *Job, state string, exitCode int, err error) {
	if job.setState(state, exitCode, err) {
		r.release(job)
		r.scheduleReaper(job.ID)
	}
}

func (r *Registry) scheduleReaper(id string) {
	time.AfterFunc(defaultJobTTL, func() {
		r.mu.Lock()
		if job, ok := r.jobs[id]; ok {
			job.mu.Lock()
			done := job.finished
			job.mu.Unlock()
			if done {
				delete(r.jobs, id)
				if r.latest[job.SessionID] == id {
					delete(r.latest, job.SessionID)
				}
			}
		}
		r.mu.Unlock()
	})
}

// Truncate caps a byte slice at max, cutting at a newline when possible and
// appending a marker. Shared by both modes' result shaping.
func Truncate(b []byte, max int) []byte {
	if max <= 0 || len(b) <= max {
		return b
	}
	const marker = "\n…[uniterm: output truncated]"
	trunc := b[:max]
	if i := strings.LastIndexByte(string(trunc), '\n'); i > max/2 {
		trunc = trunc[:i]
	}
	return append(trunc, []byte(marker)...)
}

// Result is the normalized poll payload both modes return.
type Result struct {
	JobID      string
	State      string
	Stdout     string
	Stderr     string
	ExitCode   int
	NextOffset int
	Truncated  bool
}

// TruncateOutputLines applies the same head/tail display limits used by the
// built-in assistant. Byte paging remains independent of presentation.
func TruncateOutputLines(output string, head, tail int) (string, bool) {
	lines := strings.Split(output, "\n")
	if head < 0 {
		head = 0
	}
	if tail < 0 {
		tail = 0
	}
	if head >= len(lines) || tail >= len(lines)-head {
		return output, false
	}
	return strings.Join(lines[:head], "\n") + fmt.Sprintf(
		"\n\n─────── [TRUNCATED: %d lines total, %d lines omitted] ────────\nAdjust head_lines / tail_lines to see more content.\n\n",
		len(lines), len(lines)-head-tail) + strings.Join(lines[len(lines)-tail:], "\n"), true
}

// Sentinel errors surfaced to the MCP tool layer and mapped onto readable
// agent-facing messages.
var (
	ErrTooMany       = errors.New("too many concurrent jobs")
	ErrSessionClosed = errors.New("session closed")
	ErrUnknownJob    = errors.New("job not found or expired")
	ErrInterrupted   = errors.New("job interrupted")
	// ErrSuperseded: the job was cancelled because a newer command took over
	// the session (the holder was wedged — timed out or past the takeover
	// grace — and the AI moved on to a new command).
	ErrSuperseded = errors.New("superseded by a newer command")
	// ErrUserInterrupted: the user typed into the terminal directly while a
	// foreground job owned it, breaking the job's output coherence.
	ErrUserInterrupted = errors.New("interrupted by user keyboard input")
	// ErrWriteTimeout: the interrupt write did not complete within the
	// deadline (wedged PTY); the job is finished regardless.
	ErrWriteTimeout = errors.New("interrupt write timed out")
	// ErrSessionBusy: another execution (an earlier foreground job) is still
	// typing into / tapping this session. One executor per session keeps the
	// live terminal coherent — two agents interleaving keystrokes corrupts
	// both runs and each other's completion detection.
	ErrSessionBusy = errors.New("session is busy with another execution")
)

const (
	// Sane minimums so hostile dimension requests can't produce a
	// degenerate (0-wide) grid.
	minCols = 20
	minRows = 5

	// Scrollback bound: enough context for watermark-style consumers
	// (they drain promptly) without unbounded growth on long output.
	scrollbackCap = 500

	// CSI param/length caps keep hostile sequences (ESC [ 0;0;0;... )
	// from allocating without bound. Anything past the cap is treated
	// as garbage and the whole sequence is ignored.
	maxCSIParams = 8
	maxCSILen    = 64
)

type parserState int

const (
	stateGround  parserState = iota
	stateEscape              // after ESC
	stateCSI                 // after ESC [
	stateOSC                 // after ESC ] — until BEL or ST
	stateString              // DCS/SOS/PM/APC — until ST
	stateCharset             // ESC ( / ESC ) — swallow one byte
)

// Screen is a cols x rows grid of rune cells plus a small state machine
// that turns raw PTY bytes into grid updates.
type Screen struct {
	cols, rows int
	grid       [][]rune
	x, y       int // cursor; x may equal cols as a "wrap pending" sentinel

	savedX, savedY int
	concealed      bool

	state    parserState
	csiBuf   []byte
	csiInter bool   // intermediate/overflow byte seen → ignore the sequence
	oscEsc   bool   // last byte in OSC/string was ESC (ST detection)
	pending  []byte // trailing bytes of a UTF-8 rune split across Feeds

	// Absolute line tracking for consumers that must emit each logical
	// output line exactly once. absTop is the absolute index of grid
	// row 0; it only grows when lines scroll off the top (LF past the
	// bottom, SU), so numbering is monotonic. scrollback holds the
	// trimmed text of absolute lines [absTop-len(scrollback), absTop).
	// sbDeclared[i] mirrors scrollback[i]: entries pushed from undeclared
	// rows (pre-job screen content a stream scroll displaced) are marked
	// false and lineAt reports them not-ok, so a scroll at the bottom of a
	// fresh grid can't emit the pre-job screen as phantom lines.
	absTop            int
	scrollback        []string
	sbDeclared        []bool

	// declared[i] marks grid row i as part of the logical line stream: the
	// cursor entered it via a linefeed/scroll boundary (or runes were painted
	// on it). Rows reached only by absolute cursor addressing (CUP/VPA) or
	// passed by erase sequences stay undeclared — a fresh emulator has no
	// baseline, so those rows are pre-job screen content the AI must not
	// receive as blank filler lines. LinesSince/FlushFrom skip them.
	declared []bool
}

// ScreenSeed carries the baseline prompt line a fresh job emulator should be
// seeded with: the shell painted it before the job started, so the PTY stream
// only carries incremental input-line repaints and never resends the prompt
// text itself — without seeding, the first returned line begins mid-row.
type ScreenSeed struct {
	Line string // prompt row text, right-trimmed
	X, Y int    // cursor column/row where the live PTY's cursor sits
}

// NewScreen creates a screen with the given grid dimensions (e.g. 120x40).
// Dimensions are clamped to sane minimums (cols>=20, rows>=5) so hostile
// values can't produce a degenerate grid.
func NewScreen(cols, rows int) *Screen {
	if cols < minCols {
		cols = minCols
	}
	if rows < minRows {
		rows = minRows
	}
	s := &Screen{cols: cols, rows: rows, grid: make([][]rune, rows), declared: make([]bool, rows)}
	for i := range s.grid {
		s.grid[i] = s.newRow()
	}
	return s
}

func (s *Screen) newRow() []rune {
	row := make([]rune, s.cols)
	for i := range row {
		row[i] = ' '
	}
	return row
}

// Seed paints a baseline row and places the cursor where the live PTY's
// cursor sits at job start. Must run before the first Feed. The seeded row
// counts as declared; rows the stream later leaves untouched stay undeclared.
// Callers snapshot it from the session's long-lived capture emulator, whose
// cursor row holds the prompt line the shell painted before the job began.
func (s *Screen) Seed(seed ScreenSeed) {
	if seed.Line == "" || seed.Y < 0 || seed.Y >= s.rows {
		return
	}
	s.moveCursor(0, seed.Y)
	for _, r := range seed.Line {
		s.writeRune(r)
	}
	s.moveCursor(seed.X, seed.Y)
}

// PromptSeed snapshots the cursor row (normally the prompt line the shell
// painted before the job started) plus the cursor position, for seeding a
// fresh job emulator via Seed.
func (s *Screen) PromptSeed() ScreenSeed {
	return ScreenSeed{Line: strings.TrimRight(string(s.grid[s.y]), " "), X: s.x, Y: s.y}
}

// Feed parses one chunk of raw PTY output and updates the grid.
// Chunks may split multi-byte UTF-8 runes; the trailing partial rune is
// stashed and prepended to the next chunk.
func (s *Screen) Feed(data []byte) {
	if len(s.pending) > 0 {
		data = append(s.pending, data...)
		s.pending = s.pending[:0]
	}
	for i := 0; i < len(data); {
		b := data[i]
		// Multi-byte runes only matter in ground state; escape
		// sequences are pure ASCII.
		if s.state == stateGround && b >= utf8.RuneSelf {
			r, size := utf8.DecodeRune(data[i:])
			if r == utf8.RuneError && size == 1 {
				// Invalid byte, or a sequence truncated by the end of
				// this chunk. Only stash plausible lead bytes — a stray
				// continuation byte is dropped so a malformed stream
				// can't wedge the parser waiting for bytes that never
				// come.
				if data[i] >= 0xC2 && data[i] <= 0xF4 && !utf8.FullRune(data[i:]) {
					s.pending = append(s.pending, data[i:]...)
					return
				}
				i++
				continue
			}
			i += size
			s.writeRune(r)
			continue
		}
		i++
		s.step(b)
	}
}

func (s *Screen) step(b byte) {
	switch s.state {
	case stateGround:
		s.ground(b)
	case stateEscape:
		s.escape(b)
	case stateCSI:
		s.csi(b)
	case stateOSC, stateString:
		s.swallowString(b)
	case stateCharset:
		// Charset designations affect rendering only; swallow the byte.
		s.state = stateGround
	}
}

func (s *Screen) ground(b byte) {
	switch b {
	case 0x1B:
		s.state = stateEscape
	case '\r':
		s.x = 0
	case '\n', 0x0B, 0x0C: // VT/FF are LF in practice
		s.linefeed()
	case '\b':
		if s.x > 0 {
			s.x--
		}
	case '\t':
		s.tab()
	case 0x07: // BEL: ignored
	default:
		if b < 0x20 || b == 0x7F {
			return // remaining C0 controls and DEL are ignored
		}
		s.writeRune(rune(b))
	}
}

func (s *Screen) escape(b byte) {
	s.state = stateGround
	switch b {
	case '[':
		s.state = stateCSI
		s.csiBuf = s.csiBuf[:0]
		s.csiInter = false
	case ']':
		s.state = stateOSC
		s.oscEsc = false
	case 'P', 'X', '^', '_': // DCS/SOS/PM/APC
		s.state = stateString
		s.oscEsc = false
	case '(', ')', '*', '+': // charset designation: swallow next byte
		s.state = stateCharset
	case '7':
		s.savedX, s.savedY = s.x, s.y
	case '8':
		s.x, s.y = s.savedX, s.savedY
		s.clampCursor()
	case 'D': // IND behaves like LF
		s.linefeed()
	case 'M': // RI: up, reverse-scroll at top
		s.reverseLinefeed()
	case 'E': // NEL: CR+LF
		s.x = 0
		s.linefeed()
	case 'c': // RIS: clear screen, home
		s.reset()
	case 0x1B: // ESC ESC: restart escape
		s.state = stateEscape
	case 0x18, 0x1A: // CAN/SUB abort
	default:
		// Unknown escape (ESC =, ESC >, ...): parse and ignore.
	}
}

// swallowString consumes OSC/DCS/SOS/PM/APC payload bytes until BEL
// (OSC only) or ST (ESC \). CAN/SUB abort, matching xterm.
func (s *Screen) swallowString(b byte) {
	switch {
	case b == 0x07 && s.state == stateOSC:
		s.state = stateGround
	case b == 0x1B:
		s.oscEsc = true
	case s.oscEsc:
		s.oscEsc = false
		if b == '\\' {
			s.state = stateGround
		}
	case b == 0x18 || b == 0x1A:
		s.state = stateGround
	}
}

func (s *Screen) csi(b byte) {
	switch {
	case b >= 0x30 && b <= 0x3F: // parameter bytes (digits ; : ? > =)
		if len(s.csiBuf) < maxCSILen {
			s.csiBuf = append(s.csiBuf, b)
		} else {
			s.csiInter = true // overlong: ignore whole sequence
		}
	case b >= 0x20 && b <= 0x2F: // intermediates → non-standard, ignore
		s.csiInter = true
	case b >= 0x40 && b <= 0x7E: // final byte
		s.state = stateGround
		if !s.csiInter {
			s.csiDispatch(b)
		}
	case b == 0x1B:
		s.state = stateEscape
	case b == 0x18 || b == 0x1A:
		s.state = stateGround
	default:
		// C0 controls inside CSI (e.g. CR mid-sequence): ignored rather
		// than executed — simpler, and ConPTY doesn't rely on it.
	}
}

func (s *Screen) csiDispatch(final byte) {
	ps := s.params()
	switch final {
	case 'A': // CUU
		s.moveCursor(s.x, s.y-param(ps, 0, 1))
	case 'B', 'e': // CUD / VPR
		s.moveCursor(s.x, s.y+param(ps, 0, 1))
	case 'C', 'a': // CUF / HPR
		s.moveCursor(s.x+param(ps, 0, 1), s.y)
	case 'D': // CUB
		s.moveCursor(s.x-param(ps, 0, 1), s.y)
	case 'E': // CNL
		s.moveCursor(0, s.y+param(ps, 0, 1))
	case 'F': // CPL
		s.moveCursor(0, s.y-param(ps, 0, 1))
	case 'G', '`': // CHA / HPA
		s.moveCursor(param(ps, 0, 1)-1, s.y)
	case 'd': // VPA
		s.moveCursor(s.x, param(ps, 0, 1)-1)
	case 'H', 'f': // CUP / HVP (1-based, default 1)
		s.moveCursor(param(ps, 1, 1)-1, param(ps, 0, 1)-1)
	case 'J': // ED
		s.eraseDisplay(param(ps, 0, 0))
	case 'K': // EL
		s.eraseLine(param(ps, 0, 0))
	case 'L': // IL
		s.insertLines(param(ps, 0, 1))
	case 'M': // DL
		s.deleteLines(param(ps, 0, 1))
	case 'S': // SU
		s.scrollUp(param(ps, 0, 1))
	case 'T': // SD
		s.scrollDown(param(ps, 0, 1))
	case 's': // SCP
		s.savedX, s.savedY = s.x, s.y
	case 'u': // RCP
		s.x, s.y = s.savedX, s.savedY
		s.clampCursor()
	case 'm': // SGR conceal affects plain-text capture as well as display.
		for i := 0; i < len(ps); i++ {
			p := ps[i]
			if (p == 38 || p == 48 || p == 58) && i+1 < len(ps) {
				// Indexed/RGB color components can themselves be 0, 8 or 28.
				if ps[i+1] == 2 {
					i += 4
				} else if ps[i+1] == 5 {
					i += 2
				}
				continue
			}
			switch p {
			case -1, 0, 28:
				s.concealed = false
			case 8:
				s.concealed = true
			}
		}
	default:
		// SGR (m), private modes (?...h/l), DSR (n), device attrs (c):
		// parsed and ignored — no attribute model, no response channel.
	}
}

// params splits the buffered CSI parameter bytes into ints; -1 marks a
// missing field. Non-digit parameter bytes (?, :, >, =) are skipped so
// private-prefix sequences still parse their numeric part.
func (s *Screen) params() []int {
	out := make([]int, 0, 4)
	cur := -1
	for _, b := range s.csiBuf {
		switch {
		case b >= '0' && b <= '9':
			if cur < 0 {
				cur = 0
			}
			if cur < 1_000_000 { // cap magnitude; values get clamped later
				cur = cur*10 + int(b-'0')
			}
		case b == ';':
			if len(out) < maxCSIParams {
				out = append(out, cur)
			}
			cur = -1
		}
	}
	if len(out) < maxCSIParams {
		out = append(out, cur)
	}
	return out
}

// param returns the i-th CSI parameter, or def when missing/zero
// (CSI zero params mean the default per spec).
func param(ps []int, i, def int) int {
	if i >= len(ps) || ps[i] <= 0 {
		return def
	}
	return ps[i]
}

func (s *Screen) clampCursor() {
	if s.x > s.cols-1 {
		s.x = s.cols - 1
	}
	if s.x < 0 {
		s.x = 0
	}
	if s.y > s.rows-1 {
		s.y = s.rows - 1
	}
	if s.y < 0 {
		s.y = 0
	}
}

func (s *Screen) moveCursor(x, y int) {
	s.x, s.y = x, y
	s.clampCursor()
}

func (s *Screen) linefeed() {
	s.y++
	if s.y >= s.rows {
		s.y = s.rows - 1
		s.scrollUp(1)
	}
	// A linefeed is a logical line boundary: the row it lands on belongs to
	// the command's output stream even when nothing is ever painted there
	// (interior blank lines). Absolute cursor moves deliberately do not
	// declare rows — see the declared field.
	s.declared[s.y] = true
}

func (s *Screen) reverseLinefeed() {
	if s.y > 0 {
		s.y--
		return
	}
	s.scrollDown(1)
}

func (s *Screen) tab() {
	if s.x >= s.cols {
		return // wrap-pending sentinel: tab is a no-op there
	}
	s.x = (s.x/8 + 1) * 8
	if s.x >= s.cols {
		s.x = s.cols - 1
	}
}

func (s *Screen) writeRune(r rune) {
	// Combining marks have no cell of their own; dropping them keeps
	// column counts accurate without per-cell rune stacking.
	if unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) {
		return
	}
	if r < 0x20 {
		return // stray control byte that slipped through
	}
	if s.concealed {
		r = ' '
	}
	if s.x >= s.cols {
		// Deferred wrap: the previous printable filled the last column.
		s.x = 0
		s.linefeed()
	}
	s.declared[s.y] = true
	s.grid[s.y][s.x] = r
	s.x++
	if s.x >= s.cols {
		s.x = s.cols // wrap-pending sentinel, resolved on next printable
	}
}

func (s *Screen) reset() {
	s.concealed = false
	for i := range s.grid {
		s.grid[i] = s.newRow()
	}
	s.declared = make([]bool, s.rows)
	s.x, s.y, s.savedX, s.savedY = 0, 0, 0, 0
	// absTop/scrollback survive RIS: already-scrolled lines were real
	// output and consumers may still hold watermarks below absTop.
}

// scrollUp pushes the top n rows into scrollback, shifts the grid up and
// blanks the bottom. Each pushed row raises absTop by one — this is the
// only place the absolute counter moves, which keeps it monotonic.
func (s *Screen) scrollUp(n int) {
	if n <= 0 {
		return
	}
	if n > s.rows {
		n = s.rows
	}
	for i := 0; i < n; i++ {
		s.pushScrollback(string(s.grid[i]), s.declared[i])
	}
	copy(s.grid, s.grid[n:])
	copy(s.declared, s.declared[n:])
	for i := 0; i < n; i++ {
		s.grid[s.rows-1-i] = s.newRow()
		s.declared[s.rows-1-i] = false
	}
	s.absTop += n
}

// scrollDown shifts the grid down and blanks the top (SD, RI at top).
// absTop is intentionally NOT decremented: the spec'd contract is that
// the absolute counter never decreases. Lines pushed down keep colliding
// numbers only in this rare path (progress redraws use CR, not SD), and
// watermark consumers are LF-scroll driven.
func (s *Screen) scrollDown(n int) {
	if n <= 0 {
		return
	}
	if n > s.rows {
		n = s.rows
	}
	copy(s.grid[n:], s.grid)
	copy(s.declared[n:], s.declared)
	for i := 0; i < n; i++ {
		s.grid[i] = s.newRow()
		s.declared[i] = false
	}
}

func (s *Screen) pushScrollback(line string, declared bool) {
	s.scrollback = append(s.scrollback, strings.TrimRight(line, " "))
	s.sbDeclared = append(s.sbDeclared, declared)
	if len(s.scrollback) > scrollbackCap {
		// Drop oldest so the ring stays bounded on very long output.
		s.scrollback = s.scrollback[len(s.scrollback)-scrollbackCap:]
		s.sbDeclared = s.sbDeclared[len(s.sbDeclared)-scrollbackCap:]
	}
}

// insertLines blanks n rows at the cursor, shifting rows below down.
// Grid-region only: it does not touch absTop (no lines leave the top).
func (s *Screen) insertLines(n int) {
	if n <= 0 {
		return
	}
	region := s.grid[s.y:]
	if n > len(region) {
		n = len(region)
	}
	copy(region[n:], region)
	decl := s.declared[s.y:]
	copy(decl[n:], decl)
	for i := 0; i < n; i++ {
		region[i] = s.newRow()
		decl[i] = false
	}
}

// deleteLines removes n rows at the cursor, shifting rows below up and
// blanking the region bottom. Like IL it is grid-region only.
func (s *Screen) deleteLines(n int) {
	if n <= 0 {
		return
	}
	region := s.grid[s.y:]
	if n > len(region) {
		n = len(region)
	}
	copy(region, region[n:])
	decl := s.declared[s.y:]
	copy(decl, decl[n:])
	for i := len(region) - n; i < len(region); i++ {
		region[i] = s.newRow()
		decl[i] = false
	}
}

func (s *Screen) eraseDisplay(m int) {
	switch m {
	case 0: // cursor → end of screen
		s.eraseLine(0)
		for y := s.y + 1; y < s.rows; y++ {
			s.grid[y] = s.newRow()
		}
	case 1: // start of screen → cursor
		s.eraseLine(1)
		for y := 0; y < s.y; y++ {
			s.grid[y] = s.newRow()
		}
	default: // 2/3: everything
		for y := range s.grid {
			s.grid[y] = s.newRow()
		}
	}
}

func (s *Screen) eraseLine(m int) {
	row := s.grid[s.y]
	switch m {
	case 0: // cursor → eol
		for i := s.x; i < len(row); i++ {
			row[i] = ' '
		}
	case 1: // bol → cursor (inclusive)
		for i := 0; i <= s.x && i < len(row); i++ {
			row[i] = ' '
		}
	default: // 2: whole line
		s.grid[s.y] = s.newRow()
	}
}

// Lines returns the current visible screen as rows of text, trailing
// whitespace trimmed per line. Never nil; exactly Rows() lines.
func (s *Screen) Lines() []string {
	out := make([]string, s.rows)
	for i, row := range s.grid {
		out[i] = strings.TrimRight(string(row), " ")
	}
	return out
}

// Text returns strings.Join(Lines(), "\n") with trailing blank lines removed.
func (s *Screen) Text() string {
	return strings.TrimRight(strings.Join(s.Lines(), "\n"), "\n")
}

// Tail returns at most the last n non-blank lines joined with "\n".
func (s *Screen) Tail(n int) string {
	if n <= 0 {
		return ""
	}
	lines := s.Lines()
	kept := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		if lines[i] != "" {
			kept = append(kept, lines[i])
		}
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	return strings.Join(kept, "\n")
}

func (s *Screen) Cols() int { return s.cols }
func (s *Screen) Rows() int { return s.rows }

// CursorLine returns the cursor's row index (0-based).
func (s *Screen) CursorLine() int { return s.y }

// AbsTop returns the absolute line index of grid row 0. Every scroll
// that pushes a line off the top (LF past the bottom row, SU) increments
// the numbering; the counter starts at 0 and never decreases.
func (s *Screen) AbsTop() int { return s.absTop }

// CursorAbs returns the absolute line index of the cursor row
// (AbsTop() + cursor row index).
func (s *Screen) CursorAbs() int { return s.absTop + s.y }

// lineAt resolves the text of one absolute line from scrollback or grid.
// Undeclared grid rows report not-ok: they were never part of the logical
// stream (only jumped over by absolute cursor addressing), and a fresh
// emulator has no baseline to show for them.
func (s *Screen) lineAt(idx int) (string, bool) {
	if idx >= s.absTop {
		row := idx - s.absTop
		if row >= s.rows {
			return "", false
		}
		if !s.declared[row] {
			return "", false
		}
		return strings.TrimRight(string(s.grid[row]), " "), true
	}
	sbStart := s.absTop - len(s.scrollback)
	if idx < sbStart {
		return "", false // aged out of the bounded ring
	}
	if !s.sbDeclared[idx-sbStart] {
		return "", false // undeclared: pre-job content a scroll displaced
	}
	return s.scrollback[idx-sbStart], true
}

// LinesSince returns the text of absolute lines [from, to), one trailing
// "\n" each, in order. Lines that scrolled off the grid are recovered
// from scrollback; lines not (yet) in scrollback or the grid are skipped.
// When to <= from, returns "". Callers use AbsTop/CursorAbs watermarks:
// emit [prevCursorAbs, CursorAbs) after each Feed, then flush the tail
// when the command finishes.
func (s *Screen) LinesSince(from, to int) string {
	if to <= from {
		return ""
	}
	var b strings.Builder
	for i := from; i < to; i++ {
		if line, ok := s.lineAt(i); ok {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// FlushFrom returns absolute lines [from, lastNonBlankLine] including
// the final partial cursor line, each with trailing "\n", stopping at
// the last non-blank line on screen so a prompt sitting on empty
// trailing rows is not emitted. Used for the final flush at command
// completion.
func (s *Screen) FlushFrom(from int) string {
	last := -1
	for row := s.rows - 1; row >= 0; row-- {
		if strings.TrimRight(string(s.grid[row]), " ") != "" {
			last = s.absTop + row
			break
		}
	}
	if last < from {
		return ""
	}
	return s.LinesSince(from, last+1)
}
