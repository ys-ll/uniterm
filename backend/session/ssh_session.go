package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"

	"github.com/ys-ll/uniterm/backend/diag"
	"github.com/ys-ll/uniterm/backend/log"
)

const (
	// Keepalive cadence. This is pure keep-alive (no reply awaited): every
	// interval we send one global request just to keep traffic flowing so a
	// server/NAT/firewall idle timeout doesn't drop an otherwise-healthy
	// connection. Dead-connection detection is NOT done here — see readLoop
	// (EOF) and the OS-level TCP keepalive set in Connect.
	sshKeepAliveInterval = 90 * time.Second
)

type SSHSession struct {
	baseSession
	client       *ssh.Client
	session      *ssh.Session
	stdin        io.WriteCloser
	stdout       io.Reader
	stderr       io.Reader
	quit         chan struct{}
	// quitMu guards quit and quitClosed so Disconnect can run safely even
	// when called concurrently from multiple goroutines (session.Wait,
	// readLoop EOF, keepalive tick, user close) without permanently
	// consuming the SSHSession — close(s.quit) fires at most once per
	// Connect, and Connect resets the channel and flag for the next use.
	quitMu      sync.Mutex
	quitClosed  bool
	authAnswerCh chan []byte
	expectOutput *postLoginOutputBuffer

	enc            encoding.Encoding // input(write) codec; nil = utf-8 passthrough
	encoder        transform.Transformer // cached encoder; nil = utf-8 passthrough (F-003)
	decoder        *encoding.Decoder // persistent streaming decoder for output(read)
	decodeLeftover []byte            // trailing partial multibyte bytes between reads
	decodeScratch  []byte            // reusable src buffer for decodeOutput (F-002)
	encScratch     []byte            // reusable dst buffer for encodeInput (F-003)

	// Disconnect diagnostics (see readLoop / disconnect logs).
	lastRecv atomic.Value // []byte: tail of most recent server output (diagnostics)
	lastSent atomic.Value // []byte: most recent input sent to server (diagnostics)
}

func NewSSHSession(id string) *SSHSession {
	return &SSHSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "ssh",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
}

func shouldPromptForSSHPassword(config ConnectionConfig) bool {
	if config.Password != "" {
		return false
	}
	return config.AuthType == "" || config.AuthType == "password"
}

func (s *SSHSession) Connect(config ConnectionConfig) (err error) {
	start := time.Now()
	defer func() {
		// Record connects (success or failure) under a stable op name so
		// diag.Snapshot() groups repeated connects in the percentile
		// sketch without leaking host/port into the bucket key.
		diag.Record("ssh.connect", time.Since(start), err)
	}()
	s.SetLogOnConnect(config.LogOnConnect)
	s.setStatus(StatusConnecting)
	if config.Name != "" {
		s.title = config.Name
	} else {
		s.title = fmt.Sprintf("%s@%s", config.User, config.Host)
	}

	// Fresh connect on a reused instance: replace s.quit so background
	// goroutines (startKeepAlive, runPostLoginExpect) can block on it
	// again, and clear quitClosed so Disconnect can fire exactly once
	// per connect cycle.
	s.quitMu.Lock()
	s.quitClosed = false
	s.quit = make(chan struct{})
	s.quitMu.Unlock()

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

	kbCallback := func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
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

	authMethods := makeSSHAuthMethods(config, kbCallback)
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	clientConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config: ssh.Config{
			KeyExchanges: sshKeyExchanges(),
		},
	}

	conn, err := net.DialTimeout("tcp", addr, clientConfig.Timeout)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("tcp dial: %w", err)
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(sshKeepAliveInterval)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	session, err := client.NewSession()
	if err != nil {
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("new session: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 38400,
		ssh.TTY_OP_OSPEED: 38400,
	}

	cols, rows := s.getInitialSize(80, 24)
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		session.Close()
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("request pty: %w", err)
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stdin pipe: %w", err)
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stdout pipe: %w", err)
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		session.Close()
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("shell: %w", err)
	}

	go func() {
		werr := session.Wait()
		last, _ := s.lastRecv.Load().([]byte)
		sent, _ := s.lastSent.Load().([]byte)
		log.Writef("ssh disconnect: session.Wait returned (%v), %s lastRecv=%s lastSent=%s", werr, s.kaDiag(), tailHex(last, 64), tailHex(sent, 32))
		s.Disconnect()
	}()

	s.client = client
	s.session = session
	s.stdin = stdinPipe
	s.stdout = stdoutPipe
	s.stderr = stderrPipe
	s.setStatus(StatusConnected)

	// Apply pending terminal size if one was set before connection.
	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		_ = s.session.WindowChange(rows, cols)
	}

	go s.readLoop()
	go s.readStderr()
	go s.startKeepAlive()
	go s.runPostLoginAutomation(config)

	return nil
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
	// copies into its own storage (decodeOutput, offerExpectOutput's string
	// conversion) or passes the slice to a callback that owns the data
	// lifecycle (emitData / emitBinary), so reusing the backing array is safe.
	// F-004: lastRecv is only read on the disconnect path below, so we defer
	// its copy to that point to avoid one allocation per chunk.
	buf := make([]byte, 16*1024)
	var lastData []byte
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			data := buf[:n]
			lastData = data
			s.offerExpectOutput(data)
			if s.IsZmodemMode() {
				s.emitBinary(data)
			} else if looksLikeZmodemHeader(data) {
				log.Writef("ssh: zmodem header detected in output, switching to binary mode (may be a false positive on vim/TUI output)")
				s.SetZmodemMode(true)
				s.emitBinary(data)
			} else {
				s.emitData(s.decodeOutput(data))
			}
		}
		if err != nil {
			// Copy the last received chunk into lastRecv so the
			// disconnect diagnostics in this loop and session.Wait()
			// can show what the server sent right before the link
			// dropped. Only one copy per session, not per read.
			if lastData != nil {
				s.lastRecv.Store(append([]byte(nil), lastData...))
			}
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

// Disconnect tears down the SSH session. It is safe to call from multiple
// goroutines (session.Wait, readLoop EOF, keepalive tick, user close) and
// safe to call again after a subsequent Connect — quitMu guards close(s.quit)
// so it fires at most once per Connect, and Connect resets the channel and
// quitClosed flag so a reused SSHSession tears down cleanly the next time.
func (s *SSHSession) Disconnect() error {
	s.quitMu.Lock()
	if s.quitClosed {
		s.quitMu.Unlock()
		return nil
	}
	s.quitClosed = true
	close(s.quit)
	ses := s.session
	cli := s.client
	s.quitMu.Unlock()

	if ses != nil {
		ses.Close()
	}
	if cli != nil {
		cli.Close()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *SSHSession) Resize(cols, rows int) error {
	// Always save the desired size so it can be applied after Connect finishes.
	s.SetPendingSize(cols, rows)
	if s.session == nil {
		return fmt.Errorf("session not connected")
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
// call. Must only be called from the single readLoop goroutine.
func (s *SSHSession) decodeOutput(data []byte) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	default: // "", "utf-8"
		return nil
	}
}
