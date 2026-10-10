package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"

	"github.com/ys-ll/uniterm/backend/log"
)

const (
	// Keepalive cadence. This is pure keep-alive (no reply awaited): every
	// interval we send one global request just to keep traffic flowing so a
	// server/NAT/firewall idle timeout doesn't drop an otherwise-healthy
	// connection. Dead-connection detection is NOT done here — see readLoop
	// (EOF) and the OS-level TCP keepalive set in Connect.
	sshKeepAliveInterval = 90 * time.Second

	// Windows OpenSSH announces itself with a "for_Windows" marker in its
	// SSH identification string (e.g. "SSH-2.0-OpenSSH_for_Windows_9.5"),
	// unlike Linux/BSD OpenSSH ("SSH-2.0-OpenSSH_9.9p1"). We match on the
	// marker to detect the remote OS at handshake time with zero extra
	// round-trips.
	windowsOpenSSHMarker = "for_Windows"

	// RemoteOS value reported to the frontend when the marker matches.
	remoteOSWindowsOpenSSH = "windows-openssh"
)

type SSHSession struct {
	baseSession
	// outputRouteMu makes the binary/text routing decision atomic with
	// EndZmodem. Without it, readLoop could observe binary mode, get paused,
	// then emit post-transfer shell output as binary after EndZmodem returned.
	outputRouteMu       sync.Mutex
	client              *ssh.Client
	session             *ssh.Session
	stdin               io.WriteCloser
	stdout              io.Reader
	stderr              io.Reader
	quit                chan struct{}
	quitOnce            sync.Once
	authAnswerCh        chan []byte
	expectOutput        *postLoginOutputBuffer
	x11Forwarder        *x11Forwarder

	// osc7 extracts OSC-7 cwd reports emitted by the remote shell or tools.
	// Only used from the readLoop goroutine.
	osc7 osc7Scanner

	// hook carries the typed cwd-hook injection state machine shared with the
	// WSL path (see typedCwdHookState): the marker scanners plus the
	// arm/body handoff of the on-demand two-phase injection. The startup path
	// runs it single-phase: the pty is created echo-off and the hook is typed
	// before the shell reads anything, so no arm phase is ever pending and
	// only the ready marker fires. Scanner Feeds run in the readLoop only.
	hook typedCwdHookState

	enc            encoding.Encoding     // input(write) codec; nil = utf-8 passthrough
	encoder        transform.Transformer // cached encoder; nil = utf-8 passthrough (F-003)
	decoder        *encoding.Decoder     // persistent streaming decoder for output(read)
	decodeLeftover []byte                // trailing partial multibyte bytes between reads
	decodeScratch  []byte                // reusable src buffer for decodeOutput (F-002)
	encScratch     []byte                // reusable dst buffer for encodeInput (F-003)

	// Disconnect diagnostics (see readLoop / disconnect logs).
	lastRecv atomic.Value // []byte: tail of most recent server output (diagnostics)
	lastSent atomic.Value // []byte: most recent input sent to server (diagnostics)

	// remoteOS records the detected remote operating system ("windows-openssh"
	// for Microsoft's OpenSSH for Windows, "" otherwise). Set once during
	// Connect from the server identification string.
	remoteOS string

	// cwdHookFollow marks connections with CwdHookMode "follow": the hook is
	// not injected at attach time; InjectCwdHook types it into the running
	// shell on demand when the frontend enables directory follow.
	cwdHookFollow bool

	// hookConfirmedCh is closed once the injected startup cwd hook confirms
	// via its ready marker, or the blind echo-restore fallback has fired.
	// Post-login automation waits on it before typing anything: the hook's
	// cursor-position probe (read -t 1 < /dev/tty) keeps a read pending on
	// the tty input queue for up to 1s, and a post-login line queued inside
	// that window is silently consumed by the read instead of reaching the
	// shell. Nil when no hook was injected (follow mode / unsupported shell).
	hookConfirmedCh chan struct{}

	// hookConfirmOnce guards the single close of hookConfirmedCh: the ready
	// marker (readLoop) and the blind echo-restore fallback can race.
	hookConfirmOnce sync.Once

	// detectedShell holds the remote login shell reported by the pre-connect
	// probe ("" when the probe failed, was skipped for follow mode, or timed
	// out). attach builds the startup cwd hook from it; channel clones inherit
	// it from their source session so they never probe at all.
	detectedShell string

	// cwdHookTriggered guards the on-demand injection against repeated
	// toggle clicks. Session objects are recreated on reconnect, so the
	// guard re-arms naturally.
	cwdHookTriggered atomic.Bool

	// clientRef wraps the shared *ssh.Client with reference counting so a
	// channel clone (issue #983, Xshell-style "duplicate channel") can hold
	// the authenticated connection open after the source tab closes.
	clientRef *sshClientRef

	// channelClone marks a session that reuses an existing authenticated
	// client instead of dialing — Connect skips handshake/auth and only
	// opens a new session channel.
	channelClone bool

	// connConfig is the fully materialized config this session connected
	// with (identity/proxy resolved by the App layer, tunnel rewrite
	// applied). MCP file tools reuse it to lazily open the companion
	// file-transfer session (SFTP/SCP per FileTransferProto) the same way
	// the frontend file sidebar does.
	connConfig ConnectionConfig
}

// ConnectionConfig returns the materialized config this session connected
// with. Zero value before Connect.
func (s *SSHSession) ConnectionConfig() ConnectionConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connConfig
}

// sshClientRef owns one authenticated SSH transport. The last release closes
// the underlying client, so clones keep the connection alive independently.
type sshClientRef struct {
	client *ssh.Client
	refs   atomic.Int32
}

func (r *sshClientRef) acquire() {
	r.refs.Add(1)
}

// release drops one holder and closes the client when none remain.
func (r *sshClientRef) release() {
	if r.refs.Add(-1) == 0 {
		r.client.Close()
	}
}

func newSSHClientRef(client *ssh.Client) *sshClientRef {
	r := &sshClientRef{client: client}
	r.refs.Store(1)
	return r
}

func NewSSHSession(id string) *SSHSession {
	s := &SSHSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "ssh",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
	s.hook = newTypedCwdHookState()
	return s
}

// keyboardInteractiveChallenge builds the keyboard-interactive callback used
// during the SSH handshake. Saved password-only challenges are auto-answered
// once (see isSavedPasswordChallenge); every other challenge is prompted in
// the terminal and read from authAnswerCh. When the server re-challenges —
// meaning the previous answer was rejected — a denial line precedes the new
// prompt (matching the OpenSSH client) so the user is not left staring at a
// silent re-prompt while the server's auth-fail delay runs (issue #949).
func (s *SSHSession) keyboardInteractiveChallenge(config ConnectionConfig, autoAnswered *int32) ssh.KeyboardInteractiveChallenge {
	challenged := false
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		defer func() { challenged = true }()
		if isSavedPasswordChallenge(config, questions, echos) && atomic.CompareAndSwapInt32(autoAnswered, 0, 1) {
			answers := make([]string, len(questions))
			for i := range questions {
				answers[i] = config.Password
			}
			return answers, nil
		}
		answers := make([]string, len(questions))
		if challenged {
			s.emitData([]byte("\r\nPermission denied, please try again."))
		}
		for i, q := range questions {
			s.emitData([]byte("\r\n" + q + " "))
			var answer string
		loop:
			for {
				select {
				case data := <-s.authAnswerCh:
					for _, b := range data {
						switch b {
						case '\r', '\n':
							break loop
						case '\x03':
							s.emitData([]byte("^C\r\n"))
							return nil, fmt.Errorf("auth cancelled")
						case 127, '\b':
							if len(answer) > 0 {
								answer = answer[:len(answer)-1]
								if echos[i] {
									s.emitData([]byte("\b \b"))
								}
							}
						case '\x15': // Ctrl+U
							answer = ""
						default:
							answer += string(b)
							if echos[i] {
								s.emitData([]byte{b})
							}
						}
					}
				case <-time.After(120 * time.Second):
					s.emitData([]byte("\r\nAuth timeout\r\n"))
					return nil, fmt.Errorf("auth timeout")
				}
			}
			s.emitData([]byte("\r\n"))
			answers[i] = answer
		}
		return answers, nil
	}
}

// NewSSHChannelSession creates a session that opens a fresh channel on the
// source session's already-authenticated client (no re-auth, no 2FA prompt —
// issue #983). The clone inherits the detected remoteOS. The source must be
// connected; the caller validates that.
func NewSSHChannelSession(id string, source *SSHSession) *SSHSession {
	source.mu.RLock()
	ref := source.clientRef
	remoteOS := source.remoteOS
	detectedShell := source.detectedShell
	source.mu.RUnlock()
	clone := NewSSHSession(id)
	if ref != nil {
		ref.acquire()
		clone.clientRef = ref
	}
	clone.remoteOS = remoteOS
	// Inherit the shell probe result so the clone's attach needs no detection
	// of its own — the shared client must not carry exec requests.
	clone.detectedShell = detectedShell
	clone.channelClone = true
	return clone
}

// IsChannelClone reports whether this session shares an authenticated client
// with another session instead of owning its own connection.
func (s *SSHSession) IsChannelClone() bool {
	return s.channelClone
}

// RemoteOS returns the detected remote operating system. For Microsoft's
// OpenSSH for Windows this is "windows-openssh"; otherwise it is empty
// (undetermined). Read-only and set once during Connect.
func (s *SSHSession) RemoteOS() string {
	return s.remoteOS
}

func shouldPromptForSSHPassword(config ConnectionConfig) bool {
	if config.Password != "" {
		return false
	}
	return config.AuthType == "" || config.AuthType == "password"
}

func (s *SSHSession) Connect(config ConnectionConfig) error {
	s.SetLogOnConnect(config.LogOnConnect)
	s.setStatus(StatusConnecting)
	s.mu.Lock()
	s.connConfig = config
	s.mu.Unlock()
	if config.Name != "" {
		s.title = config.Name
	} else {
		s.title = fmt.Sprintf("%s@%s", config.User, config.Host)
	}

	// Channel clone (issue #983): reuse the already-authenticated client from
	// the source session — no dial, no auth, no 2FA prompt. expectOutput is
	// still initialized so post-login automation replays on the new channel.
	if s.channelClone {
		s.mu.Lock()
		s.expectOutput = newPostLoginOutputBuffer()
		s.mu.Unlock()
		if s.clientRef == nil || s.clientRef.client == nil {
			s.setStatus(StatusError)
			return fmt.Errorf("source session is not connected")
		}
		return s.attach(s.clientRef.client, config)
	}

	// Set up keyboard-interactive auth input channel.
	s.mu.Lock()
	s.authAnswerCh = make(chan []byte, 256)
	s.expectOutput = newPostLoginOutputBuffer()
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.authAnswerCh = nil
		s.mu.Unlock()
	}()

	// For password auth without a stored password, prompt in the terminal
	// before the SSH handshake. This covers servers that do not advertise
	// keyboard-interactive support (the kbCallback fallback below).
	if shouldPromptForSSHPassword(config) {
		s.emitData([]byte("\r\nPassword: "))
		var answer string
	promptLoop:
		for {
			select {
			case data := <-s.authAnswerCh:
				for _, b := range data {
					switch b {
					case '\r', '\n':
						break promptLoop
					case '\x03': // Ctrl+C
						s.emitData([]byte("^C\r\n"))
						return fmt.Errorf("auth cancelled")
					case 127, '\b': // Backspace
						if len(answer) > 0 {
							answer = answer[:len(answer)-1]
						}
					case '\x15': // Ctrl+U
						answer = ""
					default:
						answer += string(b)
					}
				}
			case <-time.After(120 * time.Second):
				s.emitData([]byte("\r\nAuth timeout\r\n"))
				return fmt.Errorf("auth timeout")
			}
		}
		s.emitData([]byte("\r\n"))
		config.Password = answer
	}

	// Auto-answer keyboard-interactive challenges with the saved password on
	// the FIRST challenge. Many servers (e.g. SuSE/NetIQ) advertise only
	// keyboard-interactive (no "password" method), so the ssh.Password method
	// above is rejected and the callback gets invoked with a "Password: "
	// prompt. Without this, the user would be prompted for a password that is
	// already saved. If the saved password is wrong, the server re-challenges
	// and we fall through to interactive prompting so the user can type the
	// correct one.
	var kbAutoAnswered int32
	kbCallback := s.keyboardInteractiveChallenge(config, &kbAutoAnswered)

	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	newConfig := func(challenge ssh.KeyboardInteractiveChallenge) sshClientConfigFactory {
		return func() (*ssh.ClientConfig, func(), error) {
			authMethods, cleanup, err := makeSSHAuthMethodsForAttempt(config, challenge)
			if err != nil {
				return nil, nil, err
			}
			return &ssh.ClientConfig{
				User:            config.User,
				Auth:            authMethods,
				Timeout:         30 * time.Second,
				HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			}, cleanup, nil
		}
	}
	// Keyboard-interactive rides the first handshake; the fallback (for
	// servers that cold-reject it) drops it. Kerberos must report its own
	// errors instead of silently falling through to a password prompt.
	var authConfig sshClientConfigFactory
	if config.AuthType != "kerberos" {
		authConfig = newConfig(kbCallback)
	}
	fallbackConfig := newConfig(nil)
	sets, err := resolveSSHDialAlgorithms(config.SSHAlgorithms)
	if err != nil {
		s.setStatus(StatusError)
		return err
	}

	// The login-shell probe for the cwd hook runs on a short-lived connection
	// of its own, dialled in parallel with the main one (issues #1031/#1057).
	// An exec on the main connection before the shell session would
	//   (a) consume sshd's one-shot PAM loginmsg, so the shell session never
	//       displays the MOTD (the "Last login" line is unaffected), and
	//   (b) kill the login outright on CLI-only appliances whose SSH servers
	//       implement only pty-req + shell and tear down the transport on exec.
	// The probe result is an optional enhancement: any failure degrades to no
	// injection, exactly like the old unsupported-shell skip.
	s.cwdHookFollow = config.CwdHookMode == CwdHookModeFollow
	probeCh := make(chan string, 1)
	if !s.cwdHookFollow {
		go func() { probeCh <- probeCwdShellSeparately(config) }()
	} else {
		log.Writef("ssh: cwd hook deferred to directory-follow toggle")
	}

	client, err := dialSSHWithAuthRetry(addr, sets, authConfig, fallbackConfig, func() (net.Conn, error) {
		return dialFirstHop(addr, config.Proxy)
	})
	if err != nil {
		s.setStatus(StatusError)
		return wrapSSHAlgorithmError(sshAlgoModeOf(config.SSHAlgorithms), err)
	}

	// Detect Windows OpenSSH from the server identification string exchanged
	// during the handshake. This is available before any auth/shell and needs
	// no extra round-trip; the marker "for_Windows" only appears in Microsoft's
	// OpenSSH fork.
	if strings.Contains(string(client.ServerVersion()), windowsOpenSSHMarker) {
		s.remoteOS = remoteOSWindowsOpenSSH
	}

	// The probe usually lands while the main dial is still running (it does
	// strictly less work than dial + auth), so the wait below costs nothing in
	// the common case. Bound it so a straggling probe (slow network, TCP
	// black hole) delays the login by at most cwdProbeWaitTimeout — a missing
	// probe result only skips the optional cwd-hook injection.
	var detectedShell string
	if !s.cwdHookFollow {
		select {
		case detectedShell = <-probeCh:
		case <-time.After(cwdProbeWaitTimeout):
			log.Writef("ssh: cwd hook probe did not finish within %s, skipping startup injection", cwdProbeWaitTimeout)
		}
	}
	s.mu.Lock()
	s.detectedShell = detectedShell
	s.mu.Unlock()

	s.mu.Lock()
	s.clientRef = newSSHClientRef(client)
	s.mu.Unlock()
	if err := s.attach(client, config); err != nil {
		// The dial path owns the whole client: a failed attach tears it down
		// too (clones only ever release their ref via Disconnect).
		s.mu.Lock()
		s.clientRef = nil
		s.mu.Unlock()
		client.Close()
		return err
	}
	return nil
}

// attach opens a session channel on an authenticated client and wires this
// SSHSession to it: PTY, pipes, shell start, read loops and post-login
// automation. Shared by the dial path (own client) and channel clones
// (issue #983). On failure the channel is closed but the client stays open —
// the dial path's own error handling closes its client, clones hold a ref.
func (s *SSHSession) attach(client *ssh.Client, config ConnectionConfig) error {
	// The startup cwd-hook injection is decided by s.detectedShell — probed
	// on a separate short-lived connection in Connect, or inherited by channel
	// clones from their source session. The main connection NEVER carries an
	// exec request: on OpenSSH the first exec session consumes the one-shot
	// PAM loginmsg, so the shell session would never display the MOTD, and
	// CLI-only appliances (Netgear M4300 etc.) tear down the whole transport
	// on exec, killing the login. CwdHookMode "follow"
	// skips the startup injection entirely: InjectCwdHook re-detects on
	// demand.
	s.cwdHookFollow = config.CwdHookMode == CwdHookModeFollow
	injectHook, injectShell := "", ""
	if s.cwdHookFollow {
		log.Writef("ssh: cwd hook deferred to directory-follow toggle")
	} else {
		s.mu.RLock()
		shell := s.detectedShell
		s.mu.RUnlock()
		if snippet, ok := buildStartupCwdHook(shell); ok {
			injectHook, injectShell = snippet, shellBasename(shell)
		} else {
			log.Writef("ssh: cwd hook skipped (no probe result or unsupported shell %q)", shell)
		}
	}
	if injectHook != "" {
		s.hookConfirmedCh = make(chan struct{})
	}

	session, err := client.NewSession()
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("new session: %w", err)
	}

	if config.AgentForwarding {
		if err := requestAgentForwarding(client, session); err != nil {
			// Match OpenSSH -A semantics: forwarding failure is visible but does
			// not discard an otherwise usable SSH connection. This commonly
			// happens when AllowAgentForwarding is disabled on the server.
			s.emitData([]byte("\r\n\x1b[33m[ssh agent forwarding: " + err.Error() + "]\x1b[0m\r\n"))
		}
	}

	// Terminal modes: ECHO is only disabled when we are about to inject the
	// cwd hook, which restores it itself once installed. A server that
	// ignores pty modes would echo the hook line once — cosmetic only.
	modes := ssh.TerminalModes{
		ssh.TTY_OP_ISPEED: 38400,
		ssh.TTY_OP_OSPEED: 38400,
	}
	if injectHook != "" {
		modes[ssh.ECHO] = 0
	} else {
		modes[ssh.ECHO] = 1
	}

	cols, rows := s.getInitialSize(80, 24)

	// X11 forwarding: per RFC 4254 §6.3.1, x11-req is a *channel request*
	// sent on the session channel. Sending it via client.SendRequest
	// routes it through the global-request transport path and OpenSSH
	// servers silently return REQUEST_FAILURE. We send it BEFORE
	// RequestPty so the SSH server has $DISPLAY ready for the initial
	// shell.
	if config.X11Forwarding {
		xauthPath := os.Getenv("XAUTHORITY")
		if xauthPath == "" {
			if home, herr := os.UserHomeDir(); herr == nil {
				xauthPath = home + "/.Xauthority"
			}
		}
		resolvedDisplay := ResolveSSHX11Display()
		fwd, ferr := startX11Forward(client, session, xauthPath, resolvedDisplay)
		switch {
		case ferr == nil, errors.Is(ferr, errX11TrustedFallback):
			s.x11Forwarder = fwd
		default:
			s.emitData([]byte("\r\n\x1b[31m[x11: " + ferr.Error() + "]\x1b[0m\r\n"))
		}
		if s.x11Forwarder != nil {
			s.x11Forwarder.onError = func(msg string) {
				s.emitData([]byte("\r\n\x1b[33m" + msg + "\x1b[0m\r\n"))
			}
		}
	}

	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("request pty: %w", err)
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		session.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		session.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stderr pipe: %w", err)
	}

	// The shell ALWAYS starts as sshd's native login shell: no exec, no
	// rc-file replacement, no ZDOTDIR switch. The server prints its own
	// Last login/MOTD banner and every startup file runs exactly as an
	// interactive login would. For supported shells the cwd hook is typed in
	// right after the shell starts: with ECHO off the line never renders, the
	// shell executes it before/at the first prompt, and the hook itself
	// clears the prompt line, restores echo and prints the ready marker the
	// read loop confirms.
	if err := session.Shell(); err != nil {
		session.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("shell: %w", err)
	}

	if injectHook != "" {
		if _, err := stdinPipe.Write(s.encodeInput([]byte(injectHook))); err != nil {
			log.Writef("ssh: cwd hook write failed (shell=%s): %v", injectShell, err)
			_, _ = stdinPipe.Write([]byte(" stty echo\n"))
		} else {
			log.Writef("ssh: cwd hook injected (shell=%s)", injectShell)
			go s.watchCwdHookConfirm()
		}
	}

	// Publish the channels under the session mutex: the on-demand
	// InjectCwdHook path reads them from a frontend-triggered goroutine.
	s.mu.Lock()
	s.client = client
	s.session = session
	s.stdin = stdinPipe
	s.stdout = stdoutPipe
	s.stderr = stderrPipe
	s.mu.Unlock()
	s.setStatus(StatusConnected)

	// Apply pending terminal size if one was set before connection.
	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		_ = s.session.WindowChange(rows, cols)
	}

	go func() {
		werr := session.Wait()
		last, _ := s.lastRecv.Load().([]byte)
		sent, _ := s.lastSent.Load().([]byte)
		log.Writef("ssh disconnect: session.Wait returned (%v), %s lastRecv=%s lastSent=%s", werr, s.kaDiag(), tailHex(last, 64), tailHex(sent, 32))
		s.Disconnect()
	}()

	s.startEmitLoop()
	go s.readLoop()
	go s.readStderr()
	go s.startKeepAlive()
	go s.runPostLoginAutomation(config)

	return nil
}

// isSavedPasswordChallenge limits automatic answers to an unambiguous
// password-only prompt. Key passphrases and password+OTP challenges remain
// interactive so credentials are never sent to the wrong question.
func isSavedPasswordChallenge(config ConnectionConfig, questions []string, echos []bool) bool {
	return (config.AuthType == "" || config.AuthType == "password") &&
		config.Password != "" && len(questions) == 1 && len(echos) == 1 && !echos[0]
}

func (s *SSHSession) readStderr() {
	buf := make([]byte, 4096)
	for {
		n, err := s.stderr.Read(buf)
		if n > 0 {
			// Prefix stderr output so it can be distinguished in the UI
			// stderr is emitted raw (not decoded): it is a separate byte stream and
			// sharing the stdout decoder's leftover buffer could corrupt stdout. In
			// normal PTY shell sessions stderr is merged into the PTY (stdout) anyway.
			data := append([]byte("\r\n\x1b[31m[stderr] \x1b[0m"), buf[:n]...)
			s.emitData(data)
		}
		if err != nil {
			return
		}
	}
}

func (s *SSHSession) readLoop() {
	// 16K read buffer (F-001) reused across iterations. Each consumer either
	// copies into its own storage (lastRecv, decodeOutput, offerExpectOutput's
	// string conversion) or passes the slice to a callback that owns the data
	// lifecycle (emitData / emitBinary), so reusing the backing array is safe.
	buf := make([]byte, 16*1024)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			data := buf[:n]
			// lastRecv outlives this iteration (Disconnect logs it after
			// readLoop returns) so it must hold an independent copy. It
			// keeps the RAW server bytes (diagnostics), not the cleaned
			// stream below.
			s.lastRecv.Store(append([]byte(nil), data...))
			// OSC-7 extraction runs on the RAW byte stream, BEFORE decoding:
			// the sequence is pure ASCII while legacy codecs (GBK/Big5/...)
			// could mangle its bytes or withhold a fragment in their
			// cross-chunk multibyte leftover. The cwd itself is percent-
			// decoded UTF-8 and goes straight to the sink, never back into
			// the terminal stream. The cleaned remainder replaces the data
			// for every downstream consumer so stripped sequences never
			// render. During zmodem transfers the raw bytes are emitted
			// unchanged (binary fidelity); the scanner still runs so its
			// state cannot desync.
			cwd, cleaned, found := s.osc7.Feed(data)
			if found {
				recordSessionCwd(s.id, cwd)
				if TerminalCwdSink != nil {
					TerminalCwdSink(s.id, cwd)
				}
			}
			cleaned, armed, confirmed := s.hook.onOutput(cleaned)
			if armed {
				// stty -echo took effect — the body can no longer echo.
				go s.writeCwdHookBody(s.hook.takeArmedBody())
			}
			if confirmed {
				s.hook.confirm()
				log.Writef("ssh: cwd hook confirmed via ready marker")
				s.closeHookConfirmed()
			}
			s.offerExpectOutput(cleaned)
			s.outputRouteMu.Lock()
			if s.IsZmodemMode() {
				s.emitBinary(data)
			} else if looksLikeZmodemHeader(data) {
				log.Writef("ssh: zmodem header detected in output, switching to binary mode (may be a false positive on vim/TUI output)")
				s.baseSession.SetZmodemMode(true)
				s.emitBinary(data)
			} else {
				s.emitData(s.decodeOutput(cleaned))
			}
			s.outputRouteMu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				log.Writef("ssh disconnect: read error: %v, %s", err, s.kaDiag())
				s.emitData([]byte(fmt.Sprintf("\r\n\x1b[31m[read error: %v]\x1b[0m\r\n", err)))
			} else {
				last, _ := s.lastRecv.Load().([]byte)
				sent, _ := s.lastSent.Load().([]byte)
				log.Writef("ssh disconnect: remote closed (EOF), %s lastRecv=%s lastSent=%s", s.kaDiag(), tailHex(last, 64), tailHex(sent, 32))
				s.emitData(disconnectNotice("Connection closed by remote host."))
			}
			s.Disconnect()
			return
		}
	}
}

// tailHex returns up to the last max bytes of b as hex, for disconnect
// diagnostics (what the server sent right before closing).
func tailHex(b []byte, max int) string {
	if len(b) > max {
		b = b[len(b)-max:]
	}
	return fmt.Sprintf("% x", b)
}

// kaDiag formats idle state for disconnect diagnostics: how long since the
// last byte from the server. Keepalive here is send-only (no reply awaited),
// so there is no failure/last-OK state to report — a disconnect is detected
// by readLoop (EOF) or the OS TCP keepalive, not by this loop.
func (s *SSHSession) kaDiag() string {
	return fmt.Sprintf("idle=%v", s.idleSince().Truncate(time.Second))
}

func (s *SSHSession) offerExpectOutput(data []byte) {
	s.mu.RLock()
	output := s.expectOutput
	s.mu.RUnlock()
	if output != nil {
		output.Append(data)
	}
}

func (s *SSHSession) runPostLoginAutomation(config ConnectionConfig) {
	// Gate on the injected cwd hook before typing anything. The hook's
	// cursor-position probe keeps a `read -t 1 < /dev/tty` pending on the tty
	// input queue for up to 1s (longer when slow rc files delay the hook
	// line); a post-login line queued inside that window is silently consumed
	// by the read instead of reaching the shell (log-timeline reproduced).
	// No startup injection → nil channel → no wait.
	if s.hookConfirmedCh != nil {
		select {
		case <-s.hookConfirmedCh:
		case <-time.After(postLoginHookWaitTimeout):
		}
	}
	if len(config.PostLoginExpectSteps) > 0 {
		s.runPostLoginExpect(config)
		return
	}
	s.runPostLoginScript(config.PostLoginScript)
}

func (s *SSHSession) runPostLoginExpect(config ConnectionConfig) {
	// Wait for shell to finish initialization so the first prompt can be matched.
	if !s.waitIdle(5*time.Second, 300*time.Millisecond) {
		return
	}
	s.mu.RLock()
	output := s.expectOutput
	s.mu.RUnlock()
	if output == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
		}
	}()

	err := runPostLoginExpectAutomation(ctx, postLoginExpectAutomationConfig{
		Steps: config.PostLoginExpectSteps,
		Variables: map[string]string{
			"host":     config.Host,
			"user":     config.User,
			"password": config.Password,
		},
		Output: output,
		Send: func(data []byte) error {
			if s.stdin == nil {
				return fmt.Errorf("not connected")
			}
			_, err := s.stdin.Write(s.encodeInput(data))
			return err
		},
		IsConnected:    func() bool { return s.Status() == StatusConnected },
		DefaultTimeout: 10 * time.Second,
	})
	if err != nil && s.Status() == StatusConnected {
		s.emitData([]byte(fmt.Sprintf("\r\n\x1b[33m[post-login expect: %v]\x1b[0m\r\n", err)))
	}
}

func (s *SSHSession) runPostLoginScript(script string) {
	s.baseSession.RunPostLoginScript(context.Background(), script, func(data []byte) {
		if s.stdin != nil {
			s.stdin.Write(s.encodeInput(data))
		}
	}, s.IsConnected)
}

func (s *SSHSession) startKeepAlive() {
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.Status() != StatusConnected {
				return
			}
			// Pure keep-alive: send one global request just to keep traffic
			// flowing, and do NOT wait for a reply. wantReply=false means
			// crypto/ssh takes no lock and returns immediately, so a slow or
			// silent server can never stall this loop (an earlier wantReply=true
			// version leaked a goroutine holding mux.globalSentMu, which wedged
			// all later keepalives and let the connection idle out). Detecting a
			// dead connection is readLoop's job (EOF) plus the OS TCP keepalive.
			_, _, _ = s.client.SendRequest("keepalive@openssh.com", false, nil)

		case <-s.quit:
			return
		}
	}
}

func (s *SSHSession) Write(data []byte) error {
	// During keyboard-interactive auth, route input to the auth callback.
	s.mu.RLock()
	ch := s.authAnswerCh
	s.mu.RUnlock()
	if ch != nil {
		ch <- data
		return nil
	}
	if s.stdin == nil {
		return fmt.Errorf("not connected")
	}
	enc := s.encodeInput(data)
	s.lastSent.Store(append([]byte(nil), enc...))
	_, err := s.stdin.Write(enc)
	return err
}

// Disconnect tears down the SSH session. It uses sync.Once so the entire
// teardown sequence executes exactly once, regardless of how many goroutines
// call Disconnect concurrently (session.Wait, readLoop error, keepalive
// failure, or explicit user close). The shared client is only closed when
// this was the last holder (channel clones keep it alive — issue #983).
func (s *SSHSession) Disconnect() error {
	s.quitOnce.Do(func() {
		s.SetZmodemMode(false)
		close(s.quit)
		if s.x11Forwarder != nil {
			s.x11Forwarder.stop()
			s.x11Forwarder = nil
		}
		if s.session != nil {
			s.session.Close()
		}
		s.mu.RLock()
		ref := s.clientRef
		s.mu.RUnlock()
		if ref != nil {
			ref.release()
		}
		s.setStatus(StatusDisconnected)
	})
	return nil
}

// writeCwdHookBody is phase 2 of the on-demand two-phase injection: runs once
// the echo-armed marker confirmed that stty -echo took effect, so the body
// cannot echo. The body ends with its own echo restore and ready marker, so
// this is also where the confirm watchdog that InjectCwdHook armed becomes
// meaningful.
func (s *SSHSession) writeCwdHookBody(body string) {
	if body == "" {
		return
	}
	s.mu.RLock()
	stdin := s.stdin
	s.mu.RUnlock()
	if stdin == nil {
		return
	}
	if _, err := stdin.Write(s.encodeInput([]byte(body))); err != nil {
		log.Writef("ssh: cwd hook body write failed: %v", err)
		return
	}
	log.Writef("ssh: cwd hook body written on demand")
}

// cwdProbeDialTimeout bounds the probe connection's TCP dial. It only needs
// to beat the main dial for the probe to be free (they start together); a
// probe that loses that race is skipped rather than delaying the login.
const (
	cwdProbeDialTimeout = 6 * time.Second
	// cwdProbeWaitTimeout bounds how long Connect waits for the probe result
	// after the main dial completes. ...
	cwdProbeWaitTimeout = 8 * time.Second
)

// probeCwdShellSeparately dials a short-lived SSH connection of its own and
// runs `echo $SHELL` there. The main session connection therefore never
// carries an exec request: on OpenSSH the first exec session consumes the
// one-shot PAM loginmsg so the shell session never displays the MOTD, and
// CLI-only appliances tear down the whole transport on exec, killing the
// login. Any failure degrades to no cwd-hook
// injection — the hook is an optional enhancement.
//
// Re-auth on the probe connection works for password/key/agent (the same
// credentials the main dial uses). Interactive-only challenges (2FA) cannot
// be answered non-interactively; the probe fails and the hook is skipped —
// the session itself is unaffected.
func probeCwdShellSeparately(config ConnectionConfig) string {
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	factory := func() (*ssh.ClientConfig, func(), error) {
		authMethods, cleanup, err := makeSSHAuthMethodsForAttempt(config, nil)
		if err != nil {
			return nil, nil, err
		}
		return &ssh.ClientConfig{
			User:            config.User,
			Auth:            authMethods,
			Timeout:         cwdProbeDialTimeout,
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		}, cleanup, nil
	}
	sets, err := resolveSSHDialAlgorithms(config.SSHAlgorithms)
	if err != nil {
		log.Writef("ssh: cwd hook probe skipped (algorithms: %v)", err)
		return ""
	}
	client, err := dialSSHWithAuthRetry(addr, sets, factory, factory, func() (net.Conn, error) {
		return dialFirstHop(addr, config.Proxy)
	})
	if err != nil {
		// Expected on exec-less appliance servers: the probe transport dies,
		// the main connection is unaffected.
		log.Writef("ssh: cwd hook probe connection failed: %v", err)
		return ""
	}
	defer client.Close()
	out, err := sshRunCommand(client, sshShellProbeCommand, "", sshIntegrationTimeout)
	if err != nil {
		log.Writef("ssh: cwd hook probe exec failed: %v", err)
		return ""
	}
	shell, hasStty := parseShellProbe(out)
	if !hasStty {
		// Without stty the injected hook could never restore terminal echo
		// (the pty is created ECHO-off exactly when the hook is injected), so
		// no injection — the shell stays a plain echo-on login.
		log.Writef("ssh: cwd hook skipped (stty not available on remote, shell=%q)", shell)
		return ""
	}
	log.Writef("ssh: login shell probed on a separate connection: %q", shell)
	return shell
}

// cwdHookConfirmTimeout bounds how long the session waits for the injected
// hook's ready marker before restoring terminal echo blindly. It only fires
// when the hook never confirmed; a confirmed hook already restored echo
// itself, so a confirmed session is never touched.
const cwdHookConfirmTimeout = 3 * time.Second

// postLoginHookWaitTimeout bounds the post-login gate on the cwd hook: the
// ready marker normally confirms within a second; the fallback path fires at
// cwdHookConfirmTimeout and its blind echo-restore write lands shortly after,
// so confirm-timeout + margin covers both. Past the gate the post-login lines
// can no longer be swallowed by the hook's cursor-position read.
const postLoginHookWaitTimeout = cwdHookConfirmTimeout + time.Second

// closeHookConfirmed releases the post-login hook gate. Safe on nil (no
// startup injection) and idempotent (marker confirm in the readLoop and the
// blind echo-restore fallback can race).
func (s *SSHSession) closeHookConfirmed() {
	if s.hookConfirmedCh != nil {
		s.hookConfirmOnce.Do(func() { close(s.hookConfirmedCh) })
	}
}

func (s *SSHSession) watchCwdHookConfirm() {
	select {
	case <-time.After(cwdHookConfirmTimeout):
	case <-s.quit:
		return
	}
	if s.hook.isInstalled() || s.Status() != StatusConnected {
		return
	}
	log.Writef("ssh: cwd hook not confirmed after %s, restoring echo blindly", cwdHookConfirmTimeout)
	// Release the post-login gate even on this fallback path: the hook line
	// never confirmed, so the input-eating read window is either gone (hook
	// line executed without the marker) or will never open (hook line never
	// executed). Waiting longer would just delay post-login further.
	s.closeHookConfirmed()
	s.mu.RLock()
	stdin := s.stdin
	s.mu.RUnlock()
	if stdin != nil {
		_, _ = stdin.Write([]byte(cwdHookBlindRestoreLine))
	}
}

// InjectCwdHook types the OSC-7 cwd hook into the running login shell on
// demand, for connections with CwdHookMode "follow" (attach skipped the
// startup injection and shell detection). Called from the frontend when
// directory follow is first enabled for the session's file panel. Repeated
// calls are no-ops; a transiently failed attempt re-arms so a later call can
// retry.
//
// Two-phase for bash/zsh: the pty of such a session is echo-on
// and the shell sits at its prompt, so a single-shot write renders every line
// — the line discipline echoes bytes before stty -echo executes. Phase 1
// arms echo-off and prints the echo-armed marker; the read loop writes the
// phase-2 body only after seeing it. fish keeps the single-line injection.
func (s *SSHSession) InjectCwdHook() error {
	if !s.cwdHookTriggered.CompareAndSwap(false, true) {
		return nil
	}
	if s.Status() != StatusConnected {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("ssh session not connected")
	}
	s.mu.RLock()
	client, stdin := s.client, s.stdin
	s.mu.RUnlock()
	if client == nil || stdin == nil {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("ssh session not attached")
	}
	detected, err := sshRunCommand(client, sshShellProbeCommand, "", sshIntegrationTimeout)
	if err != nil {
		// Failed detection — definitive for this session object. Keep the
		// guard set so later toggles don't re-probe.
		return fmt.Errorf("cwd hook shell detection: %w", err)
	}
	shell, hasStty := parseShellProbe(detected)
	arm, body, ok := buildTwoPhaseCwdHook(shell)
	if !ok || !hasStty {
		return fmt.Errorf("cwd hook unavailable (shell %q, stty=%v)", shell, hasStty)
	}
	if arm != "" {
		// Phase 1: arm echo-off. The watchdog (watchCwdHookConfirm) still
		// runs: if the arm line never confirms, it restores echo blindly at
		// cwdHookConfirmTimeout — the arm line alone leaves echo OFF.
		s.hook.arm(body)
		if _, err := stdin.Write(s.encodeInput([]byte(arm))); err != nil {
			s.hook.disarm()
			s.cwdHookTriggered.Store(false)
			return fmt.Errorf("cwd hook arm write: %w", err)
		}
		log.Writef("ssh: cwd hook armed on demand (shell=%s)", shellBasename(shell))
		go s.watchCwdHookConfirm()
		return nil
	}
	// Single-phase shell (fish): accept the known cosmetic leak.
	if _, err := stdin.Write(s.encodeInput([]byte(body))); err != nil {
		s.cwdHookTriggered.Store(false)
		return fmt.Errorf("cwd hook write: %w", err)
	}
	log.Writef("ssh: cwd hook injected on demand (shell=%s)", shellBasename(shell))
	go s.watchCwdHookConfirm()
	return nil
}

func (s *SSHSession) Resize(cols, rows int) error {
	// Always save the desired size so it can be applied after Connect finishes.
	s.SetPendingSize(cols, rows)
	if s.session == nil {
		// The frontend fits the terminal as soon as a session is created, which
		// races Connect(). The pending size above is retained and applied once
		// Connect establishes the shell channel, so a nil channel here is a
		// no-op (the size still lands), not a failure worth surfacing — it only
		// produces spurious "session not connected" errors in the dev log.
		return nil
	}
	return s.session.WindowChange(rows, cols)
}

func (s *SSHSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

// SetEncoding configures the character encoding for this session.
// name: "" / "utf-8" (passthrough) | "gbk" | "gb2312" | "gb18030" |
// "big5" | "shift-jis" | "euc-jp" | "euc-kr".
func (s *SSHSession) SetEncoding(name string) {
	enc := encodingByName(name)
	s.mu.Lock()
	s.enc = enc
	if enc == nil {
		s.decoder = nil
		s.encoder = nil
	} else {
		s.decoder = enc.NewDecoder()
		s.encoder = enc.NewEncoder()
	}
	s.decodeLeftover = nil
	s.mu.Unlock()
}

// decodeOutput converts a chunk of remote bytes to UTF-8 using the configured
// decoder. Partial trailing multibyte sequences are buffered until the next
// call. The session lock serializes normal reads with ZMODEM trailing output.
func (s *SSHSession) decodeOutput(data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.decodeOutputLocked(data)
}

func (s *SSHSession) decodeOutputLocked(data []byte) []byte {
	if s.decoder == nil {
		return data
	}
	s.decodeScratch = s.decodeScratch[:0]
	s.decodeScratch = append(s.decodeScratch, s.decodeLeftover...)
	s.decodeScratch = append(s.decodeScratch, data...)
	src := s.decodeScratch

	var out []byte
	dst := make([]byte, 8192)
	for {
		nDst, nSrc, err := s.decoder.Transform(dst, src, false)
		out = append(out, dst[:nDst]...)
		src = src[nSrc:]
		if err == transform.ErrShortDst {
			continue // dst full but more src consumable; drain
		}
		break // nil or ErrShortSrc: remaining src is an incomplete trailing rune
	}
	// Buffer any incomplete trailing rune into a fresh slice so the next
	// read's append(s.decodeScratch, data...) isn't racing with decodeLeftover
	// backing storage.
	if len(src) > 0 {
		s.decodeLeftover = append(s.decodeLeftover[:0], src...)
	} else {
		s.decodeLeftover = src[:0]
	}
	return out
}

// SetZmodemMode serializes externally requested mode changes with readLoop's
// output routing decision. Internal readLoop detection already holds this lock
// and therefore calls baseSession.SetZmodemMode directly.
func (s *SSHSession) SetZmodemMode(v bool) {
	s.outputRouteMu.Lock()
	s.baseSession.SetZmodemMode(v)
	s.outputRouteMu.Unlock()
}

// EndZmodem leaves binary mode and runs bytes following the final ZMODEM
// handshake through the same streaming decoder as ordinary SSH output.
func (s *SSHSession) EndZmodem(trailing []byte) {
	s.outputRouteMu.Lock()
	defer s.outputRouteMu.Unlock()
	s.mu.Lock()
	s.setZmodemModeLocked(false)
	decoded := append([]byte(nil), s.decodeOutputLocked(trailing)...)
	w := s.outputLogWriter
	cb := s.onDataCallback
	s.mu.Unlock()
	if w != nil && len(decoded) > 0 {
		w(decoded)
	}
	if cb != nil && len(decoded) > 0 {
		cb(decoded)
	}
}

// encodeInput converts user keystrokes (UTF-8) to the configured encoding
// before writing to the remote. Each call handles a complete UTF-8 input.
func (s *SSHSession) encodeInput(data []byte) []byte {
	s.mu.RLock()
	encoder := s.encoder
	s.mu.RUnlock()
	if encoder == nil {
		return data
	}
	encoder.Reset()
	s.encScratch = s.encScratch[:0]
	nDst, _, err := encoder.Transform(s.encScratch, data, true)
	if err != nil && err != transform.ErrShortSrc {
		return data
	}
	return s.encScratch[:nDst]
}

// encodingByName maps a connection's encoding setting to an x/text codec.
// Returns nil for UTF-8 / empty (no conversion).
func encodingByName(name string) encoding.Encoding {
	switch name {
	case "gbk", "gb2312": // GB2312 is a subset of GBK; decode with GBK
		return simplifiedchinese.GBK
	case "gb18030":
		return simplifiedchinese.GB18030
	case "big5":
		return traditionalchinese.Big5
	case "shift-jis":
		return japanese.ShiftJIS
	case "euc-jp":
		return japanese.EUCJP
	case "euc-kr":
		return korean.EUCKR
	case "latin-1":
		return charmap.ISO8859_1
	default: // "", "utf-8"
		return nil
	}
}
