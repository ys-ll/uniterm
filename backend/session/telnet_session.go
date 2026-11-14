package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"

	"github.com/ys-ll/uniterm/backend/log"
)

const (
	// Telnet protocol constants
	telnetIAC  = 255
	telnetWILL = 251
	telnetWONT = 252
	telnetDO   = 253
	telnetDONT = 254
	telnetSB   = 250
	telnetSE   = 240
	telnetDM   = 242 // Data Mark, sent as half of the SYNCH sequence
	telnetNUL  = 0

	// Telnet options
	telnetOptBinary          = 0
	telnetOptEcho            = 1
	telnetOptSuppressGoAhead = 3
	telnetOptTerminalType    = 24
	telnetOptNAWS            = 31

	// Sub-negotiation
	telnetTTYPEIs   = 0
	telnetTTYPESend = 1
)

type TelnetSession struct {
	*baseSession
	conn     net.Conn
	cancel   context.CancelFunc
	quit     chan struct{}
	quitOnce sync.Once

	enc            encoding.Encoding    // input(write) codec; nil = utf-8 passthrough
	encoder        transform.Transformer // cached encoder; nil = utf-8 passthrough
	decoder        *encoding.Decoder     // persistent streaming decoder for output(read)
	decodeLeftover []byte                // trailing partial multibyte bytes between reads
	decodeScratch  []byte                // reusable src buffer for decodeOutput
	encScratch     []byte                // reusable dst buffer for encodeInput

	// Telnet option state (configured in Connect, consumed by Write)
	localEcho   bool
	telnetSendMode    string // "character" | "line"
	newlineMode string // "cr" | "crlf"
	termType    string // terminal name replied to the server's TTYPE SEND
	lineBuf           []byte

	// IAC filter state spanning TCP read boundaries (issue #1415): TCP may
	// split an IAC sequence anywhere, so an incomplete sequence is held in
	// iacPending and merged with the next read. lastWasCR backs the CR NUL
	// collapse; serverBinary decides whether NUL is padding or real data.
	iacPending   []byte
	lastWasCR    bool
	serverBinary bool

	// Login-phase state for auto-login and the post-login script (issue
	// #527): loginOutput keeps recent decoded output so credentials are sent
	// when the server actually asks for them and the script waits until a
	// shell prompt shows up past the login prompts.
	loginOutput *postLoginOutputBuffer
}

func NewTelnetSession(id string) *TelnetSession {
	return &TelnetSession{
		baseSession: &baseSession{
			id:          id,
			sessionType: "telnet",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
}

func (s *TelnetSession) Connect(config ConnectionConfig) error {
	s.SetLogOnConnect(config.LogOnConnect)
	s.setStatus(StatusConnecting)
	if config.Name != "" {
		s.title = config.Name
	} else {
		s.title = fmt.Sprintf("%s:%d", config.Host, config.Port)
	}

	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	// Route through the configured proxy chain (SOCKS5/HTTP) like SSH does,
	// and pick up the shared 30s dial timeout and TCP keepalive defaults.
	// Telnet previously dialed directly, ignoring config.Proxy (issue #1424).
	conn, err := dialFirstHop(addr, config.Proxy)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("telnet dial: %w", err)
	}
	// Deliver telnet urgent data (the SYNCH IAC DM sequence) in the normal
	// stream: without SO_OOBINLINE the kernel diverts the 0xFF byte to the
	// out-of-band queue that conn.Read never touches, leaving a stray 0xF2
	// Data Mark byte leaking into the terminal and session log (issue #1415).
	enableTelnetOOBInline(conn)

	ctx, cancel := context.WithCancel(context.Background())
	s.conn = conn
	s.cancel = cancel
	s.iacPending = nil
	s.lastWasCR = false
	s.serverBinary = false
	s.loginOutput = newPostLoginOutputBuffer()
	s.setStatus(StatusConnected)

	// Store telnet option configuration for use by Write()
	s.localEcho = config.LocalEcho
	s.telnetSendMode = config.TelnetSendMode
	s.newlineMode = config.NewlineMode
	s.termType = EffectiveTermType(config.TermType)

	// Proactively negotiate character-at-a-time mode and terminal type —
	// unless the user opted for passive negotiation.
	if config.TelnetNegotiationMode != "passive" {
		s.sendActiveNegotiation()
	}

	s.startEmitLoop()
	go s.readLoop(ctx)
	go s.runPostLoginScript(ctx, config.PostLoginScript)

	// Auto-login: send username/password if configured
	if config.User != "" {
		go s.sendAutoLogin(ctx, config.User, config.Password)
	}

	return nil
}

func (s *TelnetSession) readLoop(ctx context.Context) {
	buf := make([]byte, 16384)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := s.conn.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			s.handleRead(buf[:n])
		}
		if err != nil {
			if err != io.EOF {
				s.emitData([]byte(fmt.Sprintf("\r\n\x1b[31m[read error: %v]\x1b[0m\r\n", err)))
			} else {
				s.emitData(disconnectNotice("Connection closed by remote host."))
			}
			s.Disconnect()
			return
		}
	}
}

func (s *TelnetSession) handleRead(data []byte) {
	filtered := s.filterIAC(data)
	if len(filtered) > 0 {
		decoded := s.decodeOutput(filtered)
		if s.loginOutput != nil {
			s.loginOutput.Append(decoded)
		}
		s.emitData(decoded)
	}
}

// filterIAC strips telnet protocol bytes from an incoming stream, answering
// negotiation and sub-negotiation as they arrive.
//
// The filter is stateful across reads (issue #1415): TCP may split an IAC
// sequence anywhere, so an incomplete sequence is held in iacPending and
// merged with the next read instead of leaking its bytes into the data
// stream — or spinning forever when a read ends with "IAC WILL" and no
// option byte.
//
// NVT CR handling: outside binary mode a bare CR is transmitted as CR NUL
// (RFC 854); the NUL is padding, not data, so it is dropped. In binary mode
// (server offered WILL BINARY and we accepted) NUL bytes are real data and
// pass through untouched.
func (s *TelnetSession) filterIAC(data []byte) []byte {
	buf := data
	if len(s.iacPending) > 0 {
		buf = append(s.iacPending, data...)
		s.iacPending = nil
	}
	var out []byte
	i := 0
	for i < len(buf) {
		b := buf[i]
		if b != telnetIAC {
			if b == telnetNUL && s.lastWasCR && !s.serverBinary {
				// CR NUL = escaped bare CR; the CR was already forwarded.
				s.lastWasCR = false
				i++
				continue
			}
			s.lastWasCR = b == '\r'
			out = append(out, b)
			i++
			continue
		}
		s.lastWasCR = false
		if i+1 >= len(buf) {
			// Lone IAC at the end of the read: hold it for the next one.
			s.holdPending(buf[i:])
			return out
		}
		switch buf[i+1] {
		case telnetWILL, telnetWONT, telnetDO, telnetDONT:
			if i+2 >= len(buf) {
				s.holdPending(buf[i:])
				return out
			}
			s.handleNegotiation(buf[i+1], buf[i+2])
			i += 3
		case telnetSB:
			end := subnegotiationEnd(buf, i)
			if end < 0 {
				// Sub-negotiation split across reads: hold the tail.
				s.holdPending(buf[i:])
				return out
			}
			s.handleSubNegotiation(buf[i:end], 0)
			i = end
		case telnetIAC:
			out = append(out, telnetIAC) // escaped literal 0xFF
			i += 2
		default:
			// IAC DM (SYNCH) and any other unhandled command: consume
			// silently — the DM byte must not leak into the data stream.
			i += 2
		}
	}
	return out
}

// holdPending stashes an incomplete telnet sequence for the next read. The
// copy is required: buf may alias s.iacPending's backing array, and a later
// append(s.iacPending, ...) would overwrite the bytes we just stashed.
func (s *TelnetSession) holdPending(tail []byte) {
	s.iacPending = append([]byte(nil), tail...)
}

// subnegotiationEnd returns the index just past the "IAC SE" that terminates
// the sub-negotiation starting at start ("IAC SB"), or -1 when the sequence
// spans reads.
func subnegotiationEnd(buf []byte, start int) int {
	for j := start + 2; j+1 < len(buf); j++ {
		if buf[j] == telnetIAC && buf[j+1] == telnetSE {
			return j + 2
		}
	}
	return -1
}

func (s *TelnetSession) handleSubNegotiation(data []byte, start int) {
	if start+3 >= len(data) {
		return
	}
	opt := data[start+2]
	if opt == telnetOptTerminalType && data[start+3] == telnetTTYPESend {
		if s.conn == nil {
			return
		}
		// Server requests terminal type: reply with the configured value.
		term := []byte(s.termType)
		msg := make([]byte, 0, 4+len(term))
		msg = append(msg, telnetIAC, telnetSB, telnetOptTerminalType, telnetTTYPEIs)
		msg = append(msg, term...)
		msg = append(msg, telnetIAC, telnetSE)
		s.conn.Write(msg)
	}
}

func (s *TelnetSession) handleNegotiation(cmd byte, opt byte) {
	switch cmd {
	case telnetWILL:
		// Server offers to do something.
		switch opt {
		case telnetOptBinary, telnetOptSuppressGoAhead:
			s.reply(telnetDO, opt) // Accept
			if opt == telnetOptBinary {
				// Server sends binary: NUL bytes are data, not CR padding.
				s.serverBinary = true
			}
		case telnetOptEcho:
			s.reply(telnetDO, opt) // Accept server echoing
		case telnetOptTerminalType:
			s.reply(telnetDO, opt) // Accept server knows terminal type
		default:
			s.reply(telnetDONT, opt)
		}
	case telnetWONT:
		if opt == telnetOptBinary {
			s.serverBinary = false
		}
	case telnetDO:
		// Server asks us to do something.
		switch opt {
		case telnetOptBinary, telnetOptSuppressGoAhead, telnetOptNAWS, telnetOptTerminalType:
			s.reply(telnetWILL, opt) // Accept
		case telnetOptEcho:
			s.reply(telnetWONT, opt) // We don't echo locally
		default:
			s.reply(telnetWONT, opt)
		}
	}
}

func (s *TelnetSession) reply(cmd byte, opt byte) {
	if s.conn == nil {
		return
	}
	s.conn.Write([]byte{telnetIAC, cmd, opt})
}

// sendActiveNegotiation proactively offers the options the client drives in
// active mode: character-at-a-time (suppress Go Ahead) and terminal type,
// plus an initial window size. BINARY is deliberately not offered (issue
// #1486): Cisco's telnet server switches to its binary path, where the login
// code skips the echo-off before the password prompt, so passwords come back
// echoed in cleartext. Servers that need binary negotiate it themselves —
// handleNegotiation answers DO BINARY with WILL BINARY as before.
func (s *TelnetSession) sendActiveNegotiation() {
	s.conn.Write([]byte{telnetIAC, telnetDO, telnetOptSuppressGoAhead})
	s.conn.Write([]byte{telnetIAC, telnetWILL, telnetOptTerminalType})

	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		s.sendNAWS(cols, rows)
	} else {
		s.sendNAWS(80, 24)
	}
}

func (s *TelnetSession) sendNAWS(cols, rows int) {
	if s.conn == nil {
		return
	}
	// RFC 1073: any 0xFF byte inside the sub-negotiation must be doubled
	// (IAC IAC) or the server parses it as a command and the whole
	// negotiation desynchronizes (e.g. cols=255 leaking a bare IAC).
	data := []byte{telnetIAC, telnetSB, telnetOptNAWS}
	for _, v := range []int{cols >> 8, cols & 0xff, rows >> 8, rows & 0xff} {
		if b := byte(v); b == telnetIAC {
			data = append(data, telnetIAC, telnetIAC)
		} else {
			data = append(data, b)
		}
	}
	data = append(data, telnetIAC, telnetSE)
	s.conn.Write(data)
}

// Auto-login prompt markers. "assword:" matches both "Password:" and
// "password:" while avoiding unrelated words such as "passwordless".
var (
	autoLoginUserMarkers     = []string{"login:", "username:"}
	autoLoginPasswordMarkers = []string{"assword:"}
)

// Grace periods for the prompt-driven auto-login. When a server shows no
// recognizable prompt, credentials are sent blind after the grace timeout —
// the same net effect as the old fixed-delay schedule, just later, so
// prompt-less servers still log in (PTY input is buffered remotely).
// Variables rather than constants so tests can shorten the waits.
var (
	autoLoginPromptTimeout = 5 * time.Second
	autoLoginPromptPoll    = 50 * time.Millisecond
	autoLoginGraceTimeout  = 3 * time.Second
)

// sendAutoLogin sends the configured credentials when the server asks for
// them (issue #1415). The password is written only after the server has
// printed its password prompt — servers disable echo right before showing
// it, so writing there keeps the password from being echoed back in
// cleartext. Without a recognizable prompt, credentials are sent blind after
// a grace period.
func (s *TelnetSession) sendAutoLogin(ctx context.Context, user, password string) {
	if !s.waitForLoginPrompt(ctx, autoLoginUserMarkers) {
		log.Writef("telnet auto-login: no login prompt seen, sending username blind")
		if !s.sleepCtx(ctx, autoLoginGraceTimeout) {
			return
		}
	}
	s.writeCredential(ctx, user)

	if password == "" {
		return
	}
	if !s.waitForLoginPrompt(ctx, autoLoginPasswordMarkers) {
		log.Writef("telnet auto-login: no password prompt seen, sending password blind")
		if !s.sleepCtx(ctx, autoLoginGraceTimeout) {
			return
		}
	}
	s.writeCredential(ctx, password)
}

// waitForLoginPrompt polls the recent output stream for any of the given
// prompt markers, up to autoLoginPromptTimeout.
func (s *TelnetSession) waitForLoginPrompt(ctx context.Context, markers []string) bool {
	deadline := time.Now().Add(autoLoginPromptTimeout)
	for time.Now().Before(deadline) {
		if s.loginPhaseHasMarker(markers) {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(autoLoginPromptPoll):
		}
	}
	return false
}

func (s *TelnetSession) loginPhaseHasMarker(markers []string) bool {
	if s.loginOutput == nil {
		return false
	}
	out := strings.ToLower(s.loginOutput.TextSince(0))
	for _, m := range markers {
		if strings.Contains(out, m) {
			return true
		}
	}
	return false
}

func (s *TelnetSession) writeCredential(ctx context.Context, cred string) {
	if cred == "" {
		return
	}
	s.mu.RLock()
	conn := s.conn
	s.mu.RUnlock()
	if conn == nil {
		return
	}
	if _, err := conn.Write([]byte(cred + "\r\n")); err != nil {
		log.Writef("telnet auto-login: write: %v", err)
	}
}

func (s *TelnetSession) sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// waitForTelnetLoginDone blocks until the telnet login phase has settled so
// the post-login script is not typed at the "login:" or "Password:" prompt
// (issue #527): a shell prompt must be visible after the last login prompt —
// whether credentials were typed by auto-login or manually. Falls back after
// postLoginInitTimeout — odd servers still get the script, just later.
func (s *TelnetSession) waitForTelnetLoginDone(ctx context.Context) {
	deadline := time.Now().Add(postLoginInitTimeout)
	for time.Now().Before(deadline) {
		if s.loginPhaseOver() {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	log.Writef("telnet post-login: login phase not detected within %s, sending script anyway", postLoginInitTimeout)
}

// loginPhaseOver reports whether the output stream has progressed past the
// login phase: either a shell prompt is already visible with no login
// prompt seen at all (pre-authenticated server), or a shell prompt appeared
// after the last login/password prompt.
func (s *TelnetSession) loginPhaseOver() bool {
	if s.loginOutput == nil {
		return true
	}
	out := s.loginOutput.TextSince(0)
	st := ansiStripper{}
	clean := st.Strip([]byte(out))
	lower := strings.ToLower(string(clean))
	last := -1
	for _, m := range telnetLoginPhaseMarkers {
		if i := strings.LastIndex(lower, m); i > last {
			last = i
		}
	}
	if last < 0 {
		return endsWithShellPrompt(clean)
	}
	return endsWithShellPrompt(clean[last:])
}

var telnetLoginPhaseMarkers = []string{"login:", "username:", "assword:"}

// endsWithShellPrompt reports whether the last non-empty line looks like a
// shell prompt ($, #, > after ANSI stripping). A heuristic, used only to
// time the post-login script; the timeout fallback covers misses.
func endsWithShellPrompt(text []byte) bool {
	t := strings.TrimRight(string(text), " \t\r\n\x00")
	if t == "" {
		return false
	}
	line := t
	if i := strings.LastIndexByte(t, '\n'); i >= 0 {
		line = t[i+1:]
	}
	line = strings.TrimRight(line, " \t")
	if line == "" {
		return false
	}
	switch line[len(line)-1] {
	case '$', '#', '>':
		return true
	}
	return false
}

func (s *TelnetSession) runPostLoginScript(ctx context.Context, script string) {
	if strings.TrimSpace(script) == "" {
		return
	}
	// Issue #527: the script must not fire while the telnet login phase is
	// still in progress — it would land at the "login:" prompt and be
	// dropped or interleaved with the credentials.
	s.waitForTelnetLoginDone(ctx)
	s.baseSession.RunPostLoginScript(ctx, script, func(data []byte) {
		if s.conn != nil {
			s.conn.Write(data)
		}
	}, s.IsConnected)
}

func (s *TelnetSession) Write(data []byte) error {
	if s.conn == nil {
		return fmt.Errorf("not connected")
	}

	encoded := s.encodeInput(data)

	// Line mode: buffer until \r or \n, then flush
	if s.telnetSendMode == "line" {
		for _, b := range encoded {
			s.lineBuf = append(s.lineBuf, b)
			if b == '\r' || b == '\n' {
				flush := s.lineBuf
				// Apply CRLF translation on the flushed line
				if s.newlineMode == "crlf" {
					var translated []byte
					for _, fb := range flush {
						if fb == '\r' {
							translated = append(translated, '\r', '\n')
						} else {
							translated = append(translated, fb)
						}
					}
					flush = translated
				}
				_, err := s.conn.Write(flush)
				s.lineBuf = s.lineBuf[:0]
				return err
			}
		}
		// Echo each character locally while buffering for visibility
		s.emitData(data)
		return nil
	}

	// Character mode: apply CRLF translation, then send immediately
	if s.newlineMode == "crlf" {
		var translated []byte
		for _, b := range encoded {
			if b == '\r' {
				translated = append(translated, '\r', '\n')
			} else {
				translated = append(translated, b)
			}
		}
		encoded = translated
	}

	_, err := s.conn.Write(encoded)

	// Local echo: show typed characters in terminal (for servers that don't echo)
	if err == nil && s.localEcho {
		s.emitData(data)
	}

	return err
}

func (s *TelnetSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
	if s.cancel != nil {
		s.cancel()
	}
	if s.conn != nil {
		s.conn.Close()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *TelnetSession) Resize(cols, rows int) error {
	s.SetPendingSize(cols, rows)
	if s.conn == nil {
		return fmt.Errorf("session not connected")
	}
	s.sendNAWS(cols, rows)
	return nil
}

func (s *TelnetSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

// SetEncoding configures the character encoding for this session.
// name: "" / "utf-8" (passthrough) | "gbk" | "gb2312" | "gb18030" |
// "big5" | "shift-jis" | "euc-jp" | "euc-kr".
func (s *TelnetSession) SetEncoding(name string) {
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
func (s *TelnetSession) decodeOutput(data []byte) []byte {
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
			continue
		}
		break
	}
	if len(src) > 0 {
		s.decodeLeftover = append(s.decodeLeftover[:0], src...)
	} else {
		s.decodeLeftover = src[:0]
	}
	return out
}

// encodeInput converts user keystrokes (UTF-8) to the configured encoding
// before writing to the remote. Each call handles a complete UTF-8 input.
func (s *TelnetSession) encodeInput(data []byte) []byte {
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
