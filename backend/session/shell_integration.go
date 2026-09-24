package session

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"
	"sync"
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

// buildWSLShellBootstrap preserves the existing WSL startup behavior. SSH and
// WSL use different launch mechanisms, so changes made to approximate SSH
// login-shell semantics must not silently alter WSL initialization.
func buildWSLShellBootstrap(shell string) (files map[string]string, ok bool) {
	base := shellBasename(shell)
	const oscFn = `__uniterm_osc7() { printf '\033]7;file://%s\033\\' "$PWD" 2>/dev/null; }`
	switch base {
	case "bash":
		rc := "[ -f \"$HOME/.bashrc\" ] && . \"$HOME/.bashrc\"\n" +
			"[ -f \"$HOME/.bash_profile\" ] && . \"$HOME/.bash_profile\"\n" +
			oscFn + "\n" +
			"case \"$(declare -p PROMPT_COMMAND 2>/dev/null)\" in\n" +
			"  \"declare -a\"*) PROMPT_COMMAND=(\"__uniterm_osc7\" \"${PROMPT_COMMAND[@]}\") ;;\n" +
			"  *) PROMPT_COMMAND=\"__uniterm_osc7${PROMPT_COMMAND:+;$PROMPT_COMMAND}\" ;;\n" +
			"esac\n"
		return map[string]string{"rcfile": rc}, true
	case "zsh":
		rc := "[ -f \"$HOME/.zshrc\" ] && . \"$HOME/.zshrc\"\n" +
			oscFn + "\nprecmd_functions+=(__uniterm_osc7)\n"
		env := "[ -f \"$HOME/.zshenv\" ] && . \"$HOME/.zshenv\"\n"
		return map[string]string{".zshrc": rc, ".zshenv": env}, true
	}
	return nil, false
}

// buildStartupCwdHook returns the one-line hook typed into a freshly started
// SSH login shell (typed, never executed as a remote command, so sshd's
// native login flow and banner are untouched). The pty is requested with ECHO
// off so the line never renders; the hook ends by restoring echo, clearing
// the current line (so a prompt printed before the injection landed is
// overwritten by the one the shell prints after the hook — the prompt renders
// exactly once), and then printing the ready marker last — a missing stty
// must leave the marker unsent so the session's blind echo-restore fallback
// fires. ok=false for unsupported shells, which get a plain ECHO-on shell and
// no injection.
func buildStartupCwdHook(shell string) (string, bool) {
	base := shellBasename(shell)
	const oscFn = `__uniterm_osc7() { printf '\033]7;file://%s\033\\' "$PWD" 2>/dev/null; }`
	switch base {
	case "bash":
		return " " + oscFn + "; " +
			`case "$(declare -p PROMPT_COMMAND 2>/dev/null)" in` + " " +
			`"declare -a"*) [[ "${PROMPT_COMMAND[*]}" == *__uniterm_osc7* ]] || PROMPT_COMMAND+=("__uniterm_osc7") ;;` + " " +
			// ${PROMPT_COMMAND-} keeps the guard from erroring under set -u
			// when the variable is unset.
			`*) [[ "${PROMPT_COMMAND-}" == *__uniterm_osc7* ]] || PROMPT_COMMAND="__uniterm_osc7${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;` + " " +
			"esac" + `; stty echo; printf '\r\033[2K'; printf '\033]7777;uniterm-ok\007'` + "\n", true
	case "zsh":
		// -0 default guards against set -u when precmd_functions is unset.
		return " " + oscFn + "; " +
			`(( ${precmd_functions[(I)__uniterm_osc7]-0} )) || precmd_functions+=(__uniterm_osc7)` +
			`; stty echo; printf '\r\033[2K'; printf '\033]7777;uniterm-ok\007'` + "\n", true
	case "fish":
		return " if not functions -q __uniterm_osc7; " +
			"functions -c fish_prompt __uniterm_orig_prompt; " +
			"function fish_prompt; __uniterm_osc7; __uniterm_orig_prompt; end; " +
			`function __uniterm_osc7; printf '\e]7;file://%s\e\\' $PWD; end; end` +
			`; stty echo; printf '\r\e[2K'; printf '\e]7777;uniterm-ok\a'` + "\n", true
	}
	return "", false
}

// hookReadyScanner strips the cwd hook's ready marker from the terminal byte
// stream (it is pure control output, never meant to render) and reports each
// completed marker so the session can confirm the hook came up. It tolerates
// markers split across read chunks by holding back a trailing partial-prefix
// tail, exactly like osc7Scanner. Once a marker is confirmed the caller sets
// done and the scanner becomes a passthrough, so the hold-back never delays
// steady-state output.
type hookReadyScanner struct {
	leftover []byte
	done     bool
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
		i := bytes.Index(buf, []byte(sshCwdHookReadyMarker))
		if i < 0 {
			keep := partialMarkerLen(buf)
			cleaned = append(cleaned, buf[:len(buf)-keep]...)
			sc.leftover = append(sc.leftover, buf[len(buf)-keep:]...)
			return cleaned, found
		}
		cleaned = append(cleaned, buf[:i]...)
		buf = buf[i+len(sshCwdHookReadyMarker):]
		found = true
	}
}

// partialMarkerLen returns the length of the longest suffix of buf that is a
// proper prefix of the ready marker.
func partialMarkerLen(buf []byte) int {
	max := len(sshCwdHookReadyMarker) - 1
	if len(buf) < max {
		max = len(buf)
	}
	for k := max; k > 0; k-- {
		if bytes.HasPrefix([]byte(sshCwdHookReadyMarker), buf[len(buf)-k:]) {
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

func isSSHIntegrationTempPath(path string) bool {
	const prefix = "/tmp/uniterm-"
	if !strings.HasPrefix(path, prefix) || len(path) != len(prefix)+6 {
		return false
	}
	for _, ch := range path[len(prefix):] {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
			return false
		}
	}
	return true
}

// cleanRemoteTempPath extracts exactly one standalone, strictly validated
// mktemp path. Login banners or shell startup messages may surround the path,
// but zero or multiple candidates are rejected so cleanup is never ambiguous.
// Shared by the SSH and WSL bootstrap temp-file paths.
func cleanRemoteTempPath(out string) (string, error) {
	var path string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !isSSHIntegrationTempPath(line) {
			continue
		}
		if path != "" {
			return "", fmt.Errorf("multiple remote temp paths in output %q", out)
		}
		path = line
	}
	if path == "" {
		return "", fmt.Errorf("remote temp path not found in output %q", out)
	}
	return path, nil
}
