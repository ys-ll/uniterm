package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
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
)

const (
	sshKeepAliveInterval = 60 * time.Second
	sshKeepAliveTimeout  = 10 * time.Second
	sshKeepAliveMaxFail  = 3
)

type SSHSession struct {
	baseSession
	client       *ssh.Client
	session      *ssh.Session
	stdin        io.WriteCloser
	stdout       io.Reader
	stderr       io.Reader
	quit         chan struct{}
	quitOnce     sync.Once
	lastReadTime atomic.Int64
	authAnswerCh chan []byte
	expectOutput *postLoginOutputBuffer

	enc            encoding.Encoding // input(write) codec; nil = utf-8 passthrough
	decoder        *encoding.Decoder // persistent streaming decoder for output(read)
	decodeLeftover []byte            // trailing partial multibyte bytes between reads
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

func (s *SSHSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	s.title = fmt.Sprintf("%s@%s", config.User, config.Host)

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
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
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
		_ = session.Wait()
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
	buf := make([]byte, 4096)
	for {
		n, err := s.stdout.Read(buf)
		if n > 0 {
			s.lastReadTime.Store(time.Now().UnixNano())
			data := append([]byte(nil), buf[:n]...)
			s.offerExpectOutput(data)
			if s.IsZmodemMode() {
				s.emitBinary(data)
			} else if looksLikeZmodemHeader(data) {
				s.SetZmodemMode(true)
				s.emitBinary(data)
			} else {
				s.emitData(s.decodeOutput(data))
			}
		}
		if err != nil {
			if err != io.EOF {
				s.emitData([]byte(fmt.Sprintf("\r\n\x1b[31m[read error: %v]\x1b[0m\r\n", err)))
			} else {
				s.emitData([]byte("\r\n\x1b[31mConnection closed by remote host. Press Enter to reconnect.\x1b[0m\r\n"))
			}
			s.Disconnect()
			return
		}
	}
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
	if strings.TrimSpace(script) == "" {
		return
	}
	// Wait for shell to finish initialization (first idle period).
	if !s.waitIdle(5*time.Second, 300*time.Millisecond) {
		return
	}
	lines := strings.Split(script, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if s.Status() != StatusConnected {
			return
		}
		if s.stdin != nil {
			_, _ = s.stdin.Write(s.encodeInput([]byte(line + "\r")))
		}
		// Wait for command output to settle before sending the next line.
		s.waitIdle(3*time.Second, 300*time.Millisecond)
	}
}

// waitIdle blocks until stdout has been idle for the given duration,
// or until the overall timeout expires. It returns true on idle detection.
func (s *SSHSession) waitIdle(timeout, idle time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		last := time.Unix(0, s.lastReadTime.Load())
		if !last.IsZero() && time.Since(last) >= idle {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

func (s *SSHSession) startKeepAlive() {
	ticker := time.NewTicker(sshKeepAliveInterval)
	defer ticker.Stop()

	failures := 0
	for {
		select {
		case <-ticker.C:
			if s.Status() != StatusConnected {
				return
			}

			done := make(chan error, 1)
			go func() {
				defer func() {
					if r := recover(); r != nil {
						done <- fmt.Errorf("panic: %v", r)
					}
				}()
				// Use global request for keepalive, matching standard OpenSSH
				// ServerAliveInterval behavior. Session channel requests for
				// keepalive@openssh.com are not recognized by most SSH servers,
				// causing timeouts that eventually disconnect.
				_, _, err := s.client.SendRequest("keepalive@openssh.com", true, nil)
				done <- err
			}()

			select {
			case err := <-done:
				if err != nil {
					failures++
				} else {
					failures = 0
				}
			case <-time.After(sshKeepAliveTimeout):
				failures++
			}

			if failures >= sshKeepAliveMaxFail {
				s.emitData([]byte("\r\n\x1b[31mConnection lost. Press Enter to reconnect.\x1b[0m\r\n"))
				s.Disconnect()
				return
			}

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
	_, err := s.stdin.Write(s.encodeInput(data))
	return err
}

// Disconnect tears down the SSH session. It uses sync.Once so the entire
// teardown sequence executes exactly once, regardless of how many goroutines
// call Disconnect concurrently (session.Wait, readLoop error, keepalive
// failure, or explicit user close).
func (s *SSHSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
		if s.session != nil {
			s.session.Close()
		}
		if s.client != nil {
			s.client.Close()
		}
		s.setStatus(StatusDisconnected)
	})
	return nil
}

func (s *SSHSession) Resize(cols, rows int) error {
	// Always save the desired size so it can be applied after Connect finishes.
	s.SetPendingSize(cols, rows)
	if s.session == nil {
		return fmt.Errorf("session not connected")
	}
	fmt.Printf("[Resize] %s -> cols=%d rows=%d\n", s.id, cols, rows)
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
	} else {
		s.decoder = enc.NewDecoder()
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
	src := make([]byte, 0, len(s.decodeLeftover)+len(data))
	src = append(src, s.decodeLeftover...)
	src = append(src, data...)

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
	s.decodeLeftover = append([]byte(nil), src...)
	return out
}

// encodeInput converts user keystrokes (UTF-8) to the configured encoding
// before writing to the remote. Each call handles a complete UTF-8 input.
func (s *SSHSession) encodeInput(data []byte) []byte {
	s.mu.RLock()
	enc := s.enc
	s.mu.RUnlock()
	if enc == nil {
		return data
	}
	out, err := enc.NewEncoder().Bytes(data)
	if err != nil {
		return data
	}
	return out
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
