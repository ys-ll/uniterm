package localsrv

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
)

// StartTelnetServer starts a Telnet server listening on the given TCP port.
// Each incoming connection is authenticated with cfg.Password, then bridged
// to a local cmd.exe shell (Windows) or the default shell on other platforms.
// The password is required: the shell grants full access to this machine.
// Returns the server id.
func (m *Manager) StartTelnetServer(cfg TelnetConfig) (string, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.Password == "" {
		return "", fmt.Errorf("password is required")
	}

	id, ctx := m.newServer()
	m.log(ServiceTelnet, id, fmt.Sprintf("Starting Telnet server on port %d ...", cfg.Port))

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to listen on TCP port %d: %w", cfg.Port, err)
	}

	m.setStop(id, func() {
		_ = ln.Close()
	})

	go func() {
		defer m.finish(id)
		m.state(ServiceTelnet, id, true)
		m.log(ServiceTelnet, id, "Telnet server started")

		var wg sync.WaitGroup
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil {
					m.state(ServiceTelnet, id, false)
					m.log(ServiceTelnet, id, "Telnet server stopped")
					return
				}
				continue
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				m.handleTelnetConn(ctx, id, conn, cfg)
			}()
		}
	}()

	m.startTimeout(id, ServiceTelnet, cfg.TimeoutSeconds)
	return id, nil
}

// telnetPty abstracts the local interactive shell process that backs a Telnet
// or SSH session. The concrete implementation is platform-specific (ConPTY on
// Windows, a classic PTY via creack/pty on Unix).
type telnetPty interface {
	// Data streams raw bytes read from the shell's terminal. The channel is
	// closed when the shell's output pipe reaches EOF (i.e. the shell exited
	// or the PTY was torn down).
	Data() <-chan []byte
	// Write sends client keystrokes to the shell's terminal.
	Write(b []byte) error
	// Done is closed when the underlying shell process has exited.
	Done() <-chan struct{}
	// Resize resizes the terminal to the given column/row dimensions.
	Resize(cols, rows int) error
	// Close tears down the PTY and reaps the child process.
	Close() error
}

// handleTelnetConn bridges a single Telnet connection to a local shell.
// A basic Telnet IAC (RFC 854) filter strips control sequences from client
// input so they never reach the shell, and any IAC in the shell output is
// double-escaped so the client interprets it as literal 0xFF.
func (m *Manager) handleTelnetConn(ctx context.Context, id string, conn net.Conn, cfg TelnetConfig) {
	defer conn.Close()

	addr := conn.RemoteAddr().String()
	m.log(ServiceTelnet, id, fmt.Sprintf("connection from %s", addr))

	// 1. Send welcome banner.
	fmt.Fprintf(conn, "\r\nWelcome to uniterm Telnet server\r\n")
	fmt.Fprintf(conn, "Type 'exit' or Ctrl+] to close the session.\r\n\r\n")

	// 2. Optional password gate.
	// sgaAccepted records whether the client negotiated SUPPRESS-GO-AHEAD
	// (character/stream mode). In that mode the client drives its own local
	// echo and it would double-display if the server echoed too, so we refuse
	// ECHO for such clients and let them echo locally.
	var wantServerEcho bool // set true if the client asks us to echo (sends IAC DO ECHO)
	var sgaAccepted bool
	if cfg.Password != "" {
		// Negotiate echo handling so the client settles, but read the password
		// with echo permanently off (nil) so the secret never prints — accepting
		// the client's DO ECHO keeps the session alive without ever echoing
		// the password back. (Clients that keep a local echo anyway would
		// otherwise double-display each keystroke AND leak it as cleartext.)
		_, _ = conn.Write([]byte{0xFF, 0xFB, 0x01}) // IAC WILL ECHO
		fmt.Fprint(conn, "Password: ")
		line, err := readTelnetLine(conn, nil, &wantServerEcho, &sgaAccepted)
		if err != nil {
			m.log(ServiceTelnet, id, fmt.Sprintf("auth failed from %s: %v", addr, err))
			return
		}
		if strings.TrimSpace(line) != cfg.Password {
			fmt.Fprint(conn, "\r\nAccess denied.\r\n")
			m.log(ServiceTelnet, id, fmt.Sprintf("auth denied for %s", addr))
			return
		}
		fmt.Fprint(conn, "\r\n")
		m.log(ServiceTelnet, id, fmt.Sprintf("authenticated %s", addr))
	}

	// 3. Launch a local shell and bridge it to the connection via a PTY.
	sh, err := startLocalShell(ctx, 120, 30)
	if err != nil {
		fmt.Fprintf(conn, "\r\nFailed to start shell: %v\r\n", err)
		m.log(ServiceTelnet, id, fmt.Sprintf("shell start failed for %s: %v", addr, err))
		return
	}
	defer sh.Close()

	// clientDisconnected is closed when the remote side hangs up; whichever of
	// the two sides ends first tears the whole session down (fixes a leak where
	// a client disconnect otherwise left the shell running forever).
	clientDisconnected := make(chan struct{})

	// Make the shell's output presentable over telnet: strip console control
	// sequences and (on Windows) transcode the legacy code page to UTF-8.
	outProc := newTelnetOutputProcessor()

	// Shell output 鈫?client (double-escape any 0xFF). Ends when the PTY's
	// output channel closes (shell exit or teardown), which also covers a
	// client hang-up because the teardown below closes the PTY.
	go func() {
		for data := range sh.Data() {
			if len(data) > 0 {
				proc := outProc.Process(data)
				// Chunk output into segments no larger than 64KB so a single huge
				// read (e.g. a large `dir`) is still delivered incrementally to the
				// client instead of one giant TCP write, while never silently
				// dropping data (the old hard 200-byte cap chopped output).
				max := len(proc)
				if max > 65536 {
					max = 65536
				}
				for off := 0; off < len(proc); off += max {
					end := off + max
					if end > len(proc) {
						end = len(proc)
					}
					chunk := proc[off:end]
					_, _ = writeTelnetSafe(conn, chunk)
				}
			}
		}
	}()

	// Client input 鈫?shell (strip IAC sequences).
	go func() {
		defer close(clientDisconnected)
		buf := make([]byte, 4096)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			n, err := conn.Read(buf)
			if n > 0 {
				filtered, resp := processTelnetInput(buf[:n])
				filtered = normalizeEditingKeys(filtered)
				if len(filtered) > 0 {
					// Forward the (already line-normalized) bytes to the shell.
					// sh.Write blocks until the ConPTY accepts them (no partial
					// writes), so a single call with an error check keeps fast
					// typing from being silently dropped. The shell's terminal
					// (ConPTY) echoes every input byte back on Data(), which the
					// goroutine above streams to the client 鈥?that echo is the one
					// rendered by well-behaved clients, so we must NOT also echo
					// here or every key would display twice.
					if err := sh.Write(filtered); err != nil {
						return
					}
					// ConPTY does not echo spaces (0x20) back on its output stream
					// (it has no visible glyph and is folded into the line buffer),
					// so a typed space would never show on the client. Echo it
					// manually 鈥?spaces are the one byte ConPTY won't double up.
					if bytes.IndexByte(filtered, ' ') >= 0 {
						sp := make([]byte, 0, 4)
						for _, b := range filtered {
							if b == ' ' {
								sp = append(sp, ' ')
							}
						}
						if len(sp) > 0 {
							_, _ = writeTelnetSafe(conn, sp)
						}
					}
				}
				if len(resp) > 0 {
					_, _ = writeTelnetSafe(conn, resp)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for either side to end; then tear the session down.
	select {
	case <-clientDisconnected:
		m.log(ServiceTelnet, id, fmt.Sprintf("client %s disconnected; closing shell", addr))
		_ = sh.Close()
	case <-sh.Done():
		m.log(ServiceTelnet, id, fmt.Sprintf("shell exited for %s", addr))
	case <-ctx.Done():
		m.log(ServiceTelnet, id, fmt.Sprintf("server shutting down; closing session for %s", addr))
		_ = sh.Close()
	}
	m.log(ServiceTelnet, id, fmt.Sprintf("shell session ended for %s", addr))
}

// readTelnetLine reads one \n-terminated line from the connection, stripping
// and answering every Telnet IAC negotiation the client sends (WILL/DO/WONT/
// DONT, and SB...SE subnegotiation such as TERMINAL-TYPE and NAWS). Those
// bytes never leak into the returned line — which is what used to break
// password entry on clients like MobaXterm that send a TTYPE request right
// after connecting (the leaked "xterm-256color" payload polluting the line).
// Unlike a per-chunk call to processTelnetInput, parsing here is stateful
// across reads: a negotiation split across TCP segments (a trailing IAC, or an
// SB whose SE hasn't arrived yet) is held back until complete, so no byte is
// lost or mis-parsed. Each typed character is echoed only when *echo is true;
// the password gate passes nil so the secret never prints.
func readTelnetLine(conn net.Conn, echo *bool, echoReq *bool, sga *bool) (string, error) {
	var b strings.Builder
	var buf []byte // data bytes awaiting echo / newline detection
	chunk := make([]byte, 1024)
	// pending is the raw out-of-band bytes (negotiation) accumulated from the
	// network but not yet fully parsed because a sequence is incomplete.
	var pending []byte

	// handle strips complete telnet IAC sequences from raw, moving everything
	// else (plain data) into data. It appends required responses to resp. If
	// raw ends mid-sequence (an incomplete IAC), it stops, moves the leading
	// plain data into data, and returns (true, tail) where tail is the bytes
	// from the incomplete IAC onward that must be retained for the next read.
	handle := func(raw []byte, data *[]byte, resp *[]byte) (partial bool, tail []byte) {
		i := 0
		for i < len(raw) {
			if raw[i] != 0xFF {
				*data = append(*data, raw[i])
				i++
				continue
			}
			// raw[i] == IAC
			if i+1 >= len(raw) {
				return true, raw[i:]
			}
			op := raw[i+1]
			switch op {
			case 0xFF: // escaped literal 0xFF -> data
				*data = append(*data, 0xFF)
				i += 2
			case 0xFA: // SB ... SE
				j := i + 2
				found := false
				for j+1 < len(raw) {
					if raw[j] == 0xFF && raw[j+1] == 0xF0 {
						j += 2
						found = true
						break
					}
					j++
				}
				if !found {
					return true, raw[i:]
				}
				i = j
			case 0xFB: // WILL opt -> DONT
				if i+2 >= len(raw) {
					return true, raw[i:]
				}
				*resp = append(*resp, 0xFF, 0xFE, raw[i+2])
				i += 3
			case 0xFD: // DO opt (client asks US to do opt)
				if i+2 >= len(raw) {
					return true, raw[i:]
				}
				opt := raw[i+2]
				if opt == 0x01 { // ECHO -> accept: reply WILL ECHO, server owns echoing
					*resp = append(*resp, 0xFF, 0xFB, opt)
					if echoReq != nil {
						*echoReq = true
					}
				} else if opt == 0x03 { // SUPPRESS-GO-AHEAD -> accept: switch to character/stream mode
					*resp = append(*resp, 0xFF, 0xFB, opt)
					if sga != nil {
						*sga = true
					}
				} else {
					*resp = append(*resp, 0xFF, 0xFC, opt)
				}
				i += 3
			case 0xFC, 0xFE: // WONT / DONT
				if i+2 >= len(raw) {
					return true, raw[i:]
				}
				i += 3
			default: // NOP / DM / AYT / GA / BRK / IP / AO etc.
				i += 2
			}
		}
		return false, nil
	}

	var resp []byte
	for {
		n, err := conn.Read(chunk)
		if n > 0 {
			// Split the new bytes: separate data from negotiation.
			var data []byte
			var newResp []byte
			pending = append(pending, chunk[:n]...)
			partial, tail := handle(pending, &data, &newResp)
			if partial {
				// Keep only the incomplete IAC tail; its prefix data is already
				// in data and must not be reprocessed next round.
				pending = append(pending[:0], tail...)
			} else {
				pending = pending[:0]
			}
			// Emit negotiation responses collected in this batch.
			if len(newResp) > 0 {
				resp = append(resp, newResp...)
				_, _ = conn.Write(resp)
				resp = resp[:0]
			}
			_ = resp
			// Now process the plain data bytes.
			buf = append(buf, data...)
			// Find the first line terminator. Telnet clients differ: MobaXterm
			// and cmd send CRLF on Enter, but uniterm's own client sends a bare
			// CR by default (NewlineMode "cr"). Treat \r, \n, or \r\n as the end
			// of a line so a password entered from any client is delivered.
			idx := -1
			for i := 0; i < len(buf); i++ {
				if buf[i] == '\r' || buf[i] == '\n' {
					idx = i
					break
				}
			}
			if idx >= 0 {
				line := buf[:idx]
				// The terminator itself is dropped. Handle \r\n as one terminator:
				// if the byte after idx is the complementary one, skip it too.
				next := 1
				if buf[idx] == '\r' && idx+1 < len(buf) && buf[idx+1] == '\n' {
					next = 2
				} else if buf[idx] == '\n' && idx+1 < len(buf) && buf[idx+1] == '\r' {
					next = 2
				}
				if echo != nil && *echo {
					for _, c := range line {
						_, _ = conn.Write([]byte{c})
					}
					_, _ = conn.Write([]byte{'\r', '\n'})
				}
				b.Write(line)
				// keep anything after the terminator for the next call
				buf = append(buf[:0], buf[idx+next:]...)
				return b.String(), nil
			}
			// no terminator yet; echo live input (when echo on) and carry on
			if echo != nil && *echo {
				for _, c := range buf {
					_, _ = conn.Write([]byte{c})
				}
			}
			b.Write(buf)
			buf = buf[:0]
			continue
		}
		if err != nil {
			if err == io.EOF {
				return b.String(), io.EOF
			}
			return b.String(), err
		}
	}
}

func processTelnetInput(in []byte) (data, resp []byte) {
	data = make([]byte, 0, len(in))
	resp = make([]byte, 0, 4)
	for i := 0; i < len(in); i++ {
		if in[i] != 0xFF {
			data = append(data, in[i])
			continue
		}
		// IAC sequence.
		if i+1 >= len(in) {
			break
		}
		c1 := in[i+1]
		i++
		switch c1 {
		case 0xFF: // literal escaped 0xFF
			data = append(data, 0xFF)
		case 0xFA: // SB ... SE, ignore payload
			i++
			for i < len(in) {
				if in[i] == 0xFF {
					if i+1 >= len(in) {
						break
					}
					if in[i+1] == 0xF0 {
						i++
						break
					}
					i++
				}
				i++
			}
		case 0xFB: // WILL opt -> reject
			if i+1 < len(in) {
				i++
				opt := in[i]
				resp = append(resp, 0xFF, 0xFE, opt) // IAC DONT opt
			}
		case 0xFD: // DO opt (client asks US to do opt)
			if i+1 < len(in) {
				i++
				opt := in[i]
				if opt == 0x01 { // ECHO: accept -> reply WILL ECHO, we own echoing
					resp = append(resp, 0xFF, 0xFB, opt) // IAC WILL ECHO
				} else if opt == 0x03 { // SUPPRESS-GO-AHEAD: accept -> character/stream mode
					resp = append(resp, 0xFF, 0xFB, opt) // IAC WILL SUPPRESS-GO-AHEAD
				} else {
					resp = append(resp, 0xFF, 0xFC, opt) // IAC WONT opt
				}
			}
		case 0xFC, 0xFE: // WONT / DONT: no response needed
			if i+1 < len(in) {
				i++
			}
		default: // NOP/DM/BRK/IP/AO/AYT/EC/EL/GA etc.: ignore
		}
	}
	// Normalize every line ending (CRLF, CR NUL, bare CR) to a single CRLF.
	// Windows console apps (cmd.exe) only treat CR as Enter, so a lone LF or CR
	// would leave the return key unhandled and hang every command. Emitting
	// CRLF satisfies both cmd.exe (Enter on CR) and clients that expect CR/LF.
	norm := make([]byte, 0, len(data))
	for i := 0; i < len(data); i++ {
		c := data[i]
		if c == '\r' {
			// swallow the LF/NUL of a CRLF/CR NUL pair
			if i+1 < len(data) && (data[i+1] == '\n' || data[i+1] == 0) {
				i++
			}
			norm = append(norm, '\r', '\n')
		} else if c == '\n' {
			norm = append(norm, '\r', '\n')
		} else {
			norm = append(norm, c)
		}
	}
	return norm, resp
}

// normalizeEditingKeys maps editing keys onto the one delete primitive a dumb
// telnet client can render ("delete the character before the cursor", echoed
// by ConPTY as "\b \b"). Without this, ConPTY would perform its native
// redraw 鈥?full-line redraw for Backspace, in-place redraw for Delete 鈥?and
// our output filter would strip the CSI cursor moves, leaving only junk.
//
//   - 0x08 (BS, Microsoft telnet's Backspace) -> 0x7F so ConPTY emits the
//     canonical "08 20 08" erase echo instead of a burst of CRLFs.
//   - ESC[3~ (the Delete key on most clients) -> 0x7F, since a telnet client
//     cannot reproduce ConPTY's forward-delete redraw without CSI.
func normalizeEditingKeys(in []byte) []byte {
	norm := make([]byte, 0, len(in))
	for i := 0; i < len(in); i++ {
		c := in[i]
		if c == 0x1B && i+3 < len(in) &&
			in[i+1] == '[' && in[i+2] == '3' && in[i+3] == '~' {
			norm = append(norm, 0x7F) // Delete -> delete previous char
			i += 3
			continue
		}
		if c == 0x08 {
			norm = append(norm, 0x7F) // Backspace -> delete previous char
			continue
		}
		norm = append(norm, c)
	}
	return norm
}

// writeTelnetSafe writes data to the Telnet connection, doubling any 0xFF
// bytes so a literal 0xFF is not mistaken for an IAC.
func writeTelnetSafe(conn net.Conn, data []byte) (int, error) {
	out := make([]byte, 0, len(data)+2)
	for _, b := range data {
		out = append(out, b)
		if b == 0xFF {
			out = append(out, 0xFF)
		}
	}
	return conn.Write(out)
}

// telnetOutputProcessor makes a shell's byte stream presentable over a telnet
// session, which telnet clients render as plain text. It:
//
//   - strips console control sequences 鈥?OSC window-title setters
//     (ESC]0;...BEL, printed by both cmd.exe and PowerShell on startup) and
//     ANSI CSI sequences (ESC[...), which telnet clients do not understand
//     and would otherwise show as garbage.
//   - normalizes bare CR bytes to LF. ConPTY redraws the shell's prompt in
//     place by emitting "\r" (carriage return without line feed), which a
//     telnet client renders by jumping the cursor back to column 0 鈥?gluing
//     the prompt onto the preceding banner line. Turning a standalone CR into
//     LF makes the prompt land on its own line, while a normal CRLF is kept
//     as a single newline.
//
// State is kept across calls so a control sequence or bare CR split across
// TCP chunks is handled correctly.
type telnetOutputProcessor struct {
	// Control-sequence stripping state: 0 normal, 1 escaped, 2 in-OSC, 3 in-CSI.
	cstate int
	// csiDigit records that the current CSI sequence carried at least one
	// digit (e.g. "4;1H"), distinguishing CUP/HVP positioning used by ConPTY
	// to lay out the prompt from a parameterless home "ESC[H".
	csiDigit bool
	// hadCR is true when the previous emitted byte was a bare CR whose
	// following byte has not been seen yet (CRLF vs standalone CR is only
	// decidable by peeking at the next byte).
	hadCR bool
	// bannerDone records that PowerShell's startup banner has already been
	// followed by an inserted newline (see stripControls), so the prompt is
	// not forced onto its own line a second time.
	bannerDone bool
}

// newTelnetOutputProcessor creates an output processor for this platform.
func newTelnetOutputProcessor() *telnetOutputProcessor {
	return &telnetOutputProcessor{}
}

// Process cleans one block of shell output before it is sent to the client.
func (o *telnetOutputProcessor) Process(data []byte) []byte {
	return o.stripControls(data)
}

// stripControls removes OSC/ANSI control sequences, keeping plain text, and
// normalizes bare CR bytes to LF so the shell's prompt lands on its own line.
func (o *telnetOutputProcessor) stripControls(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for _, b := range data {
		switch o.cstate {
		case 0:
			switch {
			case b == 0x1B: // ESC
				o.cstate = 1
			case b == '\b': // lone BS is harmless but noisy
				o.emitCR(&out, b)
			default:
				o.emitCR(&out, b)
			}
		case 1: // after ESC
			switch b {
			case ']': // OSC 鈥?strip until BEL or ESC\
				o.cstate = 2
			case '[': // CSI 鈥?strip until final byte
				o.cstate = 3
			case 'X', '^', 'P', '_': // SOS/DCS/PM/APC 鈥?strip until ST
				o.cstate = 2
			// A raw seven-bit control like ESC M, ESC 7/8 is rare in shell
			// output; treat any other byte as ending the sequence.
			default:
				o.cstate = 0
				o.emitCR(&out, b)
			}
		case 2: // in OSC/SOS/... until BEL (0x07) or ESC\ (ST)
			switch b {
			case 0x07: // BEL terminates OSC
				o.cstate = 0
			case 0x1B: // possible ST (ESC\)
				o.cstate = 4
			}
			// bytes inside the sequence are simply consumed
		case 3: // in CSI until final byte (0x40-0x7E)
			if b >= 0x40 && b <= 0x7E {
				// ConPTY moves the shell prompt onto its own line with a
				// cursor-positioning sequence (ESC[r;cH / ESC[r;cf). We strip
				// the sequence, losing that positioning, so translate "move to
				// the first column on a row" (a digit-carrying CUP/HVP) into a
				// newline. Restricted to the startup-banner window so ordinary
				// command output is untouched.
				if !o.bannerDone && o.csiDigit && (b == 'H' || b == 'f') {
					// Use a real CRLF: bare LF advances without returning the
					// cursor to column 1 in Microsoft's telnet client, which
					// leaves the prompt printed at the end of the banner line.
					// Emit two CRLFs: ConPTY's ESC[4;1H lands the prompt on the
					// 4th line (banner line 1, banner line 2, a blank line, then
					// the prompt) — matching Windows' native PowerShell layout.
					out = append(out, '\r', '\n', '\r', '\n')
				}
				o.cstate = 0
				o.csiDigit = false
			} else if b >= '0' && b <= '9' {
				o.csiDigit = true
			}
		case 4: // after ESC inside OSC: ESC\ = ST ends it, else back to OSC
			if b == '\\' {
				o.cstate = 0
			} else {
				o.cstate = 2
			}
		}
	}
	// ConPTY moves the shell's prompt to the start of the next line by emitting
	// a CSI cursor-control sequence, which is stripped above — leaving the banner
	// and the "PS C:\...>" prompt glued to the same line. PowerShell's startup
	// banner is localized (English "All rights reserved." on en-US systems,
	// Chinese "保留所有权利。" on zh-CN), so instead of anchoring on a fixed
	// banner string we look for the PowerShell prompt prefix "PS " and force a
	// newline right before it; this works regardless of the banner language. We
	// also trim the padding spaces PowerShell inserts to right-align the
	// second banner line, which would otherwise stream out as a run of blanks.
	if !o.bannerDone {
		var cleaned []byte
		// lineStart tracks the index of the start of the current line in cleaned
		// so we can detect and drop the banner's leading padding spaces.
		lineStart := 0
		trimmed := false
		for i := 0; i < len(out); i++ {
			c := out[i]
			// Before appending, check whether a PS prompt prefix starts here.
			// We do not require it to already be at the start of a line — that
			// is exactly the broken case (banner glued to prompt) we fix.
			if c == 'P' && i+1 < len(out) && out[i+1] == 'S' && !o.bannerDone &&
				(i+2 >= len(out) || out[i+2] == ' ' || out[i+2] == ':' ||
					out[i+2] == '>' || out[i+2] == '/') {
				// Ensure the prompt starts on its own line.
				clen := len(cleaned)
				if clen == 0 {
					o.bannerDone = true
					cleaned = append(cleaned, c)
					continue
				}
				if cleaned[clen-1] != '\n' && cleaned[clen-1] != '\r' {
					cleaned = append(cleaned, '\r', '\n')
				}
				lineStart = len(cleaned)
				o.bannerDone = true
				trimmed = true
				cleaned = append(cleaned, c)
				continue
			}
			// While still in the banner (before the prompt), drop runs of
			// leading spaces used to pad/center the banner text.
			if !o.bannerDone && c == ' ' {
				// Skip leading spaces at the start of a line (banner padding).
				if lineStart == len(cleaned) {
					continue
				}
			}
			if c == '\n' {
				lineStart = len(cleaned) + 1
			} else if c != '\r' && c != '\n' {
				// any non-newline advances the line content
			}
			cleaned = append(cleaned, c)
		}
		out = cleaned
		_ = trimmed
	}
	return out
}

// emitCR appends byte b to out, normalizing line endings. A bare CR (as ConPTY
// emits to redraw the prompt in place) becomes LF so the prompt starts on a
// fresh line; a CRLF sequence collapses to a single LF.
func (o *telnetOutputProcessor) emitCR(out *[]byte, b byte) {
	if o.hadCR {
		// The previous byte was a CR; its meaning depends on what follows.
		o.hadCR = false
		if b == '\n' {
			*out = append(*out, '\r', '\n') // CRLF -> keep as a real CRLF
			return
		}
		// Standalone CR -> turn into a CRLF, then also handle b below.
		*out = append(*out, '\r', '\n')
	}
	if b == '\r' {
		o.hadCR = true
		return
	}
	if b == '\n' {
		// A bare LF would advance the line without returning to column 1 in
		// Microsoft's telnet client (which only honors CR for carriage return),
		// so emit a real CRLF to keep the prompt and banner lines aligned.
		*out = append(*out, '\r', '\n')
		return
	}
	*out = append(*out, b)
}
