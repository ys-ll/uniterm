package localsrv

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
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
	if cfg.Password != "" {
		fmt.Fprint(conn, "Password: ")
		line, err := readTelnetLine(conn)
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

	// Shell output → client (double-escape any 0xFF). Ends when the PTY's
	// output channel closes (shell exit or teardown), which also covers a
	// client hang-up because the teardown below closes the PTY.
	go func() {
		for data := range sh.Data() {
			if len(data) > 0 {
				_, _ = writeTelnetSafe(conn, outProc.Process(data))
			}
		}
	}()

	// Client input → shell (strip IAC sequences).
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
				if len(filtered) > 0 {
					_ = sh.Write(filtered)
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

// readTelnetLine reads one \r\n-terminated line from the connection, silently
// swallowing Telnet IAC sequences and echoing the characters back as the
// user types (basic local-echo behaviour).
func readTelnetLine(conn net.Conn) (string, error) {
	var b strings.Builder
	r := bufio.NewReader(conn)
	for {
		byt, err := r.ReadByte()
		if err != nil {
			return b.String(), err
		}
		if byt == 0xFF { // IAC
			// skip IAC + command + subnegotiation if any
			c1, err := r.ReadByte()
			if err != nil {
				return b.String(), err
			}
			if c1 == 0xFA { // SB
				for {
					sb, err := r.ReadByte()
					if err != nil {
						return b.String(), err
					}
					if sb == 0xFF {
						b2, err := r.ReadByte()
						if err != nil {
							return b.String(), err
						}
						if b2 == 0xF0 { // SE
							break
						}
					}
				}
			}
			continue
		}
		if byt == '\r' || byt == '\n' {
			// Echo local echo back (for clients that don't)
			_, _ = conn.Write([]byte{byt})
			break
		}
		_, _ = conn.Write([]byte{byt})
		b.WriteByte(byt)
	}
	return b.String(), nil
}

// processTelnetInput parses client data for Telnet IAC command sequences. It
// returns the payload destined for the shell, plus any negotiation responses
// that must be sent back to the client. Standard telnet clients (such as
// MobaXterm's) send WILL/DO negotiation right after connecting and will close
// the session if the server never answers; replying with DONT/WONT keeps them
// alive and sane.
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
		case 0xFD: // DO opt
			if i+1 < len(in) {
				i++
				opt := in[i]
				if opt == 0x01 { // ECHO: accept, we do server-side echo
					resp = append(resp, 0xFF, 0xFD, opt) // IAC DO opt
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
	// Normalize Telnet CR (\r\n, \r\0, bare \r) to plain \n for the shell.
	for i := 0; i < len(data); i++ {
		if data[i] == '\r' {
			if i+1 < len(data) && (data[i+1] == '\n' || data[i+1] == 0) {
				copy(data[i:], data[i+1:])
				data = data[:len(data)-1]
			} else {
				data = append(data[:i], data[i+1:]...)
			}
			i--
		}
	}
	return data, resp
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
//   - strips console control sequences — OSC window-title setters
//     (ESC]0;...BEL, printed by both cmd.exe and PowerShell on startup) and
//     ANSI CSI sequences (ESC[...), which telnet clients do not understand
//     and would otherwise show as garbage.
//   - normalizes bare CR bytes to LF. ConPTY redraws the shell's prompt in
//     place by emitting "\r" (carriage return without line feed), which a
//     telnet client renders by jumping the cursor back to column 0 — gluing
//     the prompt onto the preceding banner line. Turning a standalone CR into
//     LF makes the prompt land on its own line, while a normal CRLF is kept
//     as a single newline.
//
// State is kept across calls so a control sequence or bare CR split across
// TCP chunks is handled correctly.
type telnetOutputProcessor struct {
	// Control-sequence stripping state: 0 normal, 1 escaped, 2 in-OSC, 3 in-CSI.
	cstate int
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
			case ']': // OSC — strip until BEL or ESC\
				o.cstate = 2
			case '[': // CSI — strip until final byte
				o.cstate = 3
			case 'X', '^', 'P', '_': // SOS/DCS/PM/APC — strip until ST
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
				o.cstate = 0
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
	// and the "PS C:\...>" prompt glued to the same line. Anchor on PowerShell's
	// fixed banner tail and insert a newline right after it so the prompt always
	// starts on its own line.
	if !o.bannerDone {
		marker := []byte("All rights reserved.")
		if idx := bytes.Index(out, marker); idx >= 0 {
			insertAt := idx + len(marker)
			out = append(out[:insertAt], append([]byte{'\r', '\n'}, out[insertAt:]...)...)
			o.bannerDone = true
		}
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
			*out = append(*out, '\n') // CRLF -> single LF
			return
		}
		// Standalone CR -> treat as a line feed, then also handle b below.
		*out = append(*out, '\n')
	}
	if b == '\r' {
		o.hadCR = true
		return
	}
	*out = append(*out, b)
}

