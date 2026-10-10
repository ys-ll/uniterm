package session

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ys-ll/uniterm/backend/log"
)

// TerminalCwdSink, when installed, receives each cwd reported through OSC-7
// so the App layer can forward it to the frontend as a Wails event. Installed
// once in NewApp, next to TransferEventSink.
var TerminalCwdSink func(sessionID, cwd string)

// sessionCwds remembers the last OSC-7 cwd per session so MCP list_sessions
// can report it without a frontend round-trip.
var sessionCwds = struct {
	mu sync.Mutex
	m  map[string]string
}{m: make(map[string]string)}

func recordSessionCwd(sessionID, cwd string) {
	sessionCwds.mu.Lock()
	sessionCwds.m[sessionID] = cwd
	sessionCwds.mu.Unlock()
}

// GetSessionCwd returns the last OSC-7 reported cwd for a session ("" when
// unknown).
func GetSessionCwd(sessionID string) string {
	sessionCwds.mu.Lock()
	defer sessionCwds.mu.Unlock()
	return sessionCwds.m[sessionID]
}

const (
	osc7Prefix            = "\x1b]7;"
	osc7BEL               = "\x07"
	osc7ST                = "\x1b\\"
	sshIntegrationTimeout = 5 * time.Second

	// maxOSCPending caps how long an unterminated OSC-7 payload may buffer
	// display output before it is dropped as garbage. Without this a program
	// emitting a bare "\x1b]7;" with no terminator would swallow the whole
	// rest of the terminal stream.
	maxOSCPending = 4096
)

// sshCwdHookReadyMarker is printed by the injected cwd hook once it is fully
// installed and terminal echo has been restored. The read loop strips it from
// the display stream and uses it to confirm the hook came up.
const sshCwdHookReadyMarker = "\x1b]7777;uniterm-ok\x07"

// osc7Scanner extracts OSC-7 cwd reports from a terminal byte stream,
// tolerating sequences split across read chunks, and removes them from the
// display output (xterm.js would hide them, but stripping here keeps the
// raw stream clean for decoding/logging paths).
//
// State is a single leftover buffer that holds everything from the start of
// an unfinished sequence (prefix included) or, when no sequence is open, the
// tail bytes that could be a partial "\x1b]7;" prefix. Bytes before an
// unfinished sequence are emitted immediately, so the cleaned output of a
// chunk is always complete display text except for the held tail.
type osc7Scanner struct {
	leftover []byte
}

// Feed consumes the next chunk of the terminal byte stream. It returns the
// cwd of any OSC-7 sequence completed within this chunk (percent-decoded,
// "file://host" prefix stripped), the cleaned display bytes with all OSC-7
// sequences removed, and whether a cwd was found. cleaned must always be
// used in place of the input for display, even when nothing was found: a
// partially arrived sequence is withheld and flushed on a later Feed.
func (sc *osc7Scanner) Feed(data []byte) (cwd string, cleaned []byte, found bool) {
	buf := make([]byte, 0, len(sc.leftover)+len(data))
	buf = append(buf, sc.leftover...)
	buf = append(buf, data...)
	sc.leftover = nil

	var out []byte
	for {
		i := bytes.Index(buf, []byte(osc7Prefix))
		if i < 0 {
			// No sequence start in buf: flush everything except a tail that
			// could be a split prefix.
			keep := partialPrefixLen(buf)
			out = append(out, buf[:len(buf)-keep]...)
			sc.leftover = append(sc.leftover, buf[len(buf)-keep:]...)
			break
		}
		out = append(out, buf[:i]...)
		rest := buf[i+len(osc7Prefix):]
		// The payload ends at whichever terminator comes FIRST. Real prompts
		// emit an ST-terminated OSC-7 immediately followed by a BEL-terminated
		// OSC-0 title; searching for BEL first would swallow the ST plus the
		// whole title into the payload (field-reproduced).
		belIdx := bytes.IndexByte(rest, osc7BEL[0])
		stIdx := bytes.Index(rest, []byte(osc7ST))
		end, termLen := -1, 0
		if belIdx >= 0 && (stIdx < 0 || belIdx < stIdx) {
			end, termLen = belIdx, 1
		} else if stIdx >= 0 {
			end, termLen = stIdx, len(osc7ST)
		}
		if end < 0 {
			// Terminator not yet arrived: hold everything from the sequence
			// start and re-process it on the next Feed. Display bytes
			// collected so far (out) are returned now.
			if len(rest) > maxOSCPending {
				// Garbage (never-terminated sequence): drop it rather than
				// stalling the display stream forever.
				log.Writef("osc7: dropping unterminated sequence (%d bytes)", len(rest))
				buf = rest
				continue
			}
			sc.leftover = append(sc.leftover, buf[i:]...)
			return cwd, out, found
		}
		raw := string(rest[:end])
		buf = rest[end+termLen:]
		cwd = decodeOSC7Payload(raw)
		found = true
	}
	return cwd, out, found
}

// partialPrefixLen returns the length of the longest suffix of buf that is a
// proper prefix of osc7Prefix, i.e. how many trailing bytes must be withheld
// because a sequence start may be split across the chunk boundary.
func partialPrefixLen(buf []byte) int {
	max := len(osc7Prefix) - 1
	if len(buf) < max {
		max = len(buf)
	}
	for k := max; k > 0; k-- {
		if bytes.HasPrefix([]byte(osc7Prefix), buf[len(buf)-k:]) {
			return k
		}
	}
	return 0
}

// decodeOSC7Payload converts an OSC-7 payload ("file://host/path" or a bare
// path) into a plain path. url.Parse already percent-decodes u.Path, so the
// result is proper UTF-8 regardless of the session's display encoding; the
// value never re-enters the terminal stream, only the cwd sink.
func decodeOSC7Payload(raw string) string {
	if u, err := url.Parse(raw); err == nil && u.Path != "" {
		return u.Path
	}
	return raw
}

// cwdHookEchoOffLine is the first line typed into a bash/zsh login shell. It
// turns the pty echo off itself so the long hook line that follows never
// renders even when the remote rc files re-enable echo or the server ignores
// the pty-req ECHO mode (the visible-command leak of the old single-line
// injection), then clears the first-prompt row — which may hold an echo of
// this short line — and advances one row. Everything the injection renders
// from here on is deterministic: the shell prints its second prompt on the
// fresh row, the hook line is read silently, and the cleanup below knows
// exactly how many rows the injection produced.
const cwdHookEchoOffLine = " stty -echo; printf '\\r\\033[2K\\n'\n"

// buildStartupCwdHook returns the keystrokes typed into a freshly started SSH
// login shell (typed, never executed as a remote command, so sshd's native
// login flow and banner are untouched). The hook ends by restoring echo,
// clearing the rows the injection produced (so exactly one prompt remains on
// screen), and printing the ready marker last — a missing stty must leave the
// marker unsent so the session's blind echo-restore fallback fires.
// ok=false for unsupported shells, which get a plain ECHO-on shell and no
// injection.
//
// Row accounting for bash/zsh: shells differ in whether accepting a line
// moves the cursor to a fresh row (zsh yes, bash no — the old fixed one-line
// clear left a duplicate prompt on fresh-row shells). The cleanup therefore
// asks the terminal for the cursor position (ESC[6n, answered by xterm.js)
// and distinguishes the two layouts: column 1 means the shell started command
// output on a fresh row and four rows belong to the injection (prompt #1, the
// row cleared by the echo-off line, prompt #2, the cursor row); any other
// column means the shell reused the prompt row and two rows suffice. A
// missing or late CPR reply falls back to the two-row clear — still correct
// for prompt-reusing shells and no worse than the old behavior for fresh-row
// shells.
const cwdHookCleanup = `printf '\033[6n'; if IFS=';' read -rs -d R -t 1 __u7y __u7x < /dev/tty && [ "${__u7x-}" = 1 ]; then printf '\033[2K\033[1A\033[2K\033[1A\033[2K\033[1A\033[2K\r'; else printf '\r\033[2K\033[1A\033[2K\r'; fi;`

// buildStartupCwdHook returns the keystrokes typed into a freshly started SSH
// login shell (typed, never executed as a remote command, so sshd's native
// login flow and banner are untouched). The hook ends by restoring echo,
// clearing the rows the injection produced (so exactly one prompt remains on
// screen), and printing the ready marker last — a missing stty must leave the
// marker unsent so the session's blind echo-restore fallback fires.
// ok=false for unsupported shells, which get a plain ECHO-on shell and no
// injection.
//
// Row accounting for bash/zsh: shells differ in whether accepting a line
// moves the cursor to a fresh row (zsh yes, bash no — the old fixed one-line
// clear left a duplicate prompt on fresh-row shells). The cleanup therefore
// asks the terminal for the cursor position (ESC[6n, answered by xterm.js)
// and distinguishes the two layouts: column 1 means the shell started command
// output on a fresh row and four rows belong to the injection (prompt #1, the
// row cleared by the echo-off line, prompt #2, the cursor row); any other
// column means the shell reused the prompt row and two rows suffice. A
// missing or late CPR reply falls back to the two-row clear — still correct
// for prompt-reusing shells and no worse than the old behavior for fresh-row
// shells.
func buildStartupCwdHook(shell string) (string, bool) {
	body, ok := buildShellCwdHookBody(shell)
	if !ok {
		return "", false
	}
	return cwdHookEchoOffLine + body, true
}

// buildShellCwdHookBody returns the hook line WITHOUT the echo-off prefix
// (cwdHookEchoOffLine). WSL uses this variant: ConPTY cannot pre-disable
// pty echo, so the WSL injection arms echo-off first (printing an
// echo-armed marker) and only then sends the hook body, which must never
// render.
func buildShellCwdHookBody(shell string) (string, bool) {	base := shellBasename(shell)
	const oscFn = `__uniterm_osc7() { printf '\033]7;file://%s\033\\' "$PWD" 2>/dev/null; }`
	switch base {
	case "bash":
		return " " + oscFn + "; " +
			`case "$(declare -p PROMPT_COMMAND 2>/dev/null)" in` + " " +
			`"declare -a"*) [[ "${PROMPT_COMMAND[*]}" == *__uniterm_osc7* ]] || PROMPT_COMMAND+=("__uniterm_osc7") ;;` + " " +
			// ${PROMPT_COMMAND-} keeps the guard from erroring under set -u
			// when the variable is unset.
			`*) [[ "${PROMPT_COMMAND-}" == *__uniterm_osc7* ]] || PROMPT_COMMAND="__uniterm_osc7${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;` + " " +
			"esac; stty echo; " + cwdHookCleanup + ` printf '\033]7777;uniterm-ok\007'` + "\n", true
	case "zsh":
		// -0 default guards against set -u when precmd_functions is unset.
		return " " + oscFn + "; " +
			`(( ${precmd_functions[(I)__uniterm_osc7]-0} )) || precmd_functions+=(__uniterm_osc7)` +
			`; stty echo; ` + cwdHookCleanup + ` printf '\033]7777;uniterm-ok\007'` + "\n", true
	case "fish":
		// fish has no read-until-delimiter for the CPR probe, so it keeps the
		// old single-line injection: echo can still leak on fish and the
		// prompt may render twice on fresh-row fish setups.
		return " if not functions -q __uniterm_osc7; " +
			"functions -c fish_prompt __uniterm_orig_prompt; " +
			"function fish_prompt; __uniterm_osc7; __uniterm_orig_prompt; end; " +
			`function __uniterm_osc7; printf '\e]7;file://%s\e\\' $PWD; end; end` +
			`; stty echo; printf '\r\e[2K'; printf '\e]7777;uniterm-ok\a'` + "\n", true
	}
	return "", false
}

// cwdHookEchoArmedMarker is printed by the phase-1 arm line of the typed
// cwd-hook injection once stty -echo has taken effect. Shared by the WSL and
// the on-demand SSH paths; the read loop strips it from the display stream
// and starts phase 2 (the hook body) only after seeing it, so the body can
// never render. Kept short so the arm line stays single-row when echoed.
const cwdHookEchoArmedMarker = "\x1b]7777;e\x07"

// cwdHookArmLine is phase 1 of the on-demand typed cwd-hook injection into a
// LIVE shell (directory follow). The pty of such a session was created
// echo-on (no startup injection happened), so the arm line turns the echo off
// itself and prints the echo-armed marker; the row it leaves behind is
// erased by its own trailing cursor-up/clear. Content matches the WSL arm
// line.
const cwdHookArmLine = " stty -echo;printf '\\033]7777;e\\007\\033[1A\\033[2K\\r\\n'\n"

// buildTwoPhaseCwdHook returns the phase-1 arm line and the phase-2 hook body
// for typing the cwd hook into a running shell. bash/zsh get the two-phase
// treatment (arm echo-off, wait for the armed marker, then the body — issue
// #1113: a single-shot write into a live echo-on shell renders both lines
// because the line discipline echoes bytes before stty -echo executes).
// fish has no stty-based arm and keeps the single-line injection with its
// known cosmetic leak. ok=false for unsupported shells.
func buildTwoPhaseCwdHook(shell string) (arm, body string, ok bool) {
	body, ok = buildShellCwdHookBody(shell)
	if !ok {
		return "", "", false
	}
	switch shellBasename(shell) {
	case "bash", "zsh":
		return cwdHookArmLine, body, true
	}
	return "", body, true
}

// cwdHookBlindRestoreLine is typed into the shell when the injected hook
// never confirmed within the watchdog timeout: the arm phase left the pty
// echo off and nothing else will turn it back on. Leading space keeps it out
// of bash history (HISTCONTROL=ignorespace).
const cwdHookBlindRestoreLine = " stty echo\n"

// typedCwdHookState carries the state machine of the two-phase typed
// cwd-hook injection shared by the WSL and the on-demand SSH paths: phase 1
// arms stty -echo, and the read loop writes the stored phase-2 body only once
// the echo-armed marker shows the echo actually went off, so the body can
// never render. The ready marker (printed last by the body) confirms the
// whole injection. Single-phase injections (fish, or the SSH startup path
// with its echo-off pty) never set an arm pending, so the armed event never
// fires and only the ready marker matters. onOutput must only be called from
// the session's read loop; arm/disarm come from the injecting goroutine.
type typedCwdHookState struct {
	mu          sync.Mutex
	armPending  bool
	pendingBody string
	echoReady   hookReadyScanner
	hookReady   hookReadyScanner
	installed   atomic.Bool
}

func newTypedCwdHookState() typedCwdHookState {
	return typedCwdHookState{
		echoReady: newHookReadyScanner(cwdHookEchoArmedMarker),
		hookReady: newHookReadyScanner(sshCwdHookReadyMarker),
	}
}

// arm stores the phase-2 body; the write happens when the read loop sees the
// echo-armed marker.
func (h *typedCwdHookState) arm(body string) {
	h.mu.Lock()
	h.armPending = true
	h.pendingBody = body
	h.mu.Unlock()
}

// disarm drops a pending phase-2 body (the phase-1 write failed).
func (h *typedCwdHookState) disarm() {
	h.mu.Lock()
	h.armPending = false
	h.pendingBody = ""
	h.mu.Unlock()
}

// takeArmedBody hands out the stored body once the echo-armed marker arrived
// (stty -echo took effect) and clears the pending state.
func (h *typedCwdHookState) takeArmedBody() string {
	h.mu.Lock()
	h.armPending = false
	body := h.pendingBody
	h.mu.Unlock()
	return body
}

// confirm marks the injection complete; from then on onOutput is a
// passthrough and the confirm watchdog becomes a no-op.
func (h *typedCwdHookState) confirm() { h.installed.Store(true) }

// isInstalled reports whether the ready marker confirmed the hook.
func (h *typedCwdHookState) isInstalled() bool { return h.installed.Load() }

// onOutput feeds the next display chunk through the marker scanners. cleaned
// must always be used in place of the input: partially arrived markers are
// withheld and flushed on a later call. armed fires once per arm phase,
// confirmed exactly once.
func (h *typedCwdHookState) onOutput(data []byte) (cleaned []byte, armed, confirmed bool) {
	if h.installed.Load() {
		return data, false, false
	}
	h.mu.Lock()
	pending := h.armPending
	h.mu.Unlock()
	if pending {
		cleaned, armed = h.echoReady.Feed(data)
		data = cleaned
	}
	cleaned, confirmed = h.hookReady.Feed(data)
	return cleaned, armed, confirmed
}

// hookReadyScanner strips control markers from the terminal byte stream (they
// are pure control output, never meant to render) and reports each completed
// marker so the session can confirm a phase of the cwd-hook injection. It
// tolerates markers split across read chunks by holding back a trailing
// partial-prefix tail, exactly like osc7Scanner. Once a marker is confirmed
// the caller sets done and the scanner becomes a passthrough, so the hold-back
// never delays steady-state output.
type hookReadyScanner struct {
	marker   string
	leftover []byte
	done     bool
}

// newHookReadyScanner returns a scanner for the given control marker.
func newHookReadyScanner(marker string) hookReadyScanner {
	return hookReadyScanner{marker: marker}
}

// Feed consumes the next chunk of the terminal byte stream. It returns the
// bytes to display (markers removed) and whether a marker completed in this
// chunk. cleaned must always be used in place of the input: a partially
// arrived marker is withheld and flushed on a later Feed.
func (sc *hookReadyScanner) Feed(data []byte) (cleaned []byte, found bool) {
	if sc.done {
		return data, false
	}
	buf := append(append([]byte{}, sc.leftover...), data...)
	sc.leftover = nil
	for {
		i := bytes.Index(buf, []byte(sc.marker))
		if i < 0 {
			keep := sc.partialMarkerLen(buf)
			cleaned = append(cleaned, buf[:len(buf)-keep]...)
			sc.leftover = append(sc.leftover, buf[len(buf)-keep:]...)
			return cleaned, found
		}
		cleaned = append(cleaned, buf[:i]...)
		buf = buf[i+len(sc.marker):]
		found = true
	}
}

// partialMarkerLen returns the length of the longest suffix of buf that is a
// proper prefix of the scanner's marker.
func (sc *hookReadyScanner) partialMarkerLen(buf []byte) int {
	max := len(sc.marker) - 1
	if len(buf) < max {
		max = len(buf)
	}
	for k := max; k > 0; k-- {
		if bytes.HasPrefix([]byte(sc.marker), buf[len(buf)-k:]) {
			return k
		}
	}
	return 0
}

// shellBasename returns the basename of a shell path ("/usr/bin/zsh" →
// "zsh").
func shellBasename(shell string) string {
	if i := strings.LastIndexByte(shell, '/'); i >= 0 {
		return shell[i+1:]
	}
	return shell
}

// sshShellProbeCommand reports the login shell and whether stty exists, in
// one exec that always exits 0. Line 1 is the shell path; a non-empty line 2
// is stty's path. The injected cwd hook relies on stty both to silence the
// injection and to restore terminal echo — without it the ECHO-off pty stays
// echo-off forever (minimal busybox builds may ship a shell without stty), so
// callers must skip the injection entirely when stty is missing.
const sshShellProbeCommand = `echo "$SHELL"; command -v stty || true`

// parseShellProbe splits the sshShellProbeCommand output into the detected
// shell and whether stty is available. shell is "" when the first line is
// missing or empty.
func parseShellProbe(out string) (shell string, hasStty bool) {
	// Only strip \r and the trailing newline: a leading empty line is
	// meaningful (empty $SHELL) and must not collapse into the shell line.
	out = strings.TrimSuffix(strings.ReplaceAll(out, "\r", ""), "\n")
	lines := strings.SplitN(out, "\n", 2)
	if len(lines) == 0 {
		return "", false
	}
	shell = strings.TrimSpace(lines[0])
	if shell == "" {
		return "", false
	}
	hasStty = len(lines) == 2 && strings.TrimSpace(lines[1]) != ""
	return shell, hasStty
}

func sshRunCommand(client *ssh.Client, cmd, stdin string, timeout time.Duration) (string, error) {
	// Start the timeout before opening the channel so the caller always returns
	// within its budget. x/crypto/ssh cannot cancel one pending channel-open;
	// if the peer never replies, this goroutine exits when the client closes.
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	type sessionResult struct {
		sess *ssh.Session
		err  error
	}
	sessionReady := make(chan sessionResult)
	cancelOpen := make(chan struct{})
	defer close(cancelOpen)
	go func() {
		sess, err := client.NewSession()
		select {
		case sessionReady <- sessionResult{sess, err}:
		case <-cancelOpen:
			if sess != nil {
				_ = sess.Close()
			}
		}
	}()

	var sess *ssh.Session
	select {
	case opened := <-sessionReady:
		if opened.err != nil {
			return "", opened.err
		}
		sess = opened.sess
	case <-timer.C:
		return "", fmt.Errorf("command timed out opening SSH session after %s", timeout)
	}
	if stdin != "" {
		sess.Stdin = strings.NewReader(stdin)
	}
	type result struct {
		out []byte
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := sess.Output(cmd)
		done <- result{out, err}
	}()
	select {
	case r := <-done:
		closeSSHSessionAsync(sess)
		return string(r.out), r.err
	case <-timer.C:
		// Session.Close writes a channel-close packet and can itself block when
		// the transport is wedged. Do it asynchronously so the timeout remains
		// a bound on this function; closing the client will eventually release it.
		closeSSHSessionAsync(sess)
		return "", fmt.Errorf("command timed out after %s", timeout)
	}
}

func closeSSHSessionAsync(sess *ssh.Session) {
	if sess != nil {
		go func() { _ = sess.Close() }()
	}
}

