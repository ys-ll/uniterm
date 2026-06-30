package session

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
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
	s.setStatus(StatusConnecting)
	s.title = fmt.Sprintf("%s:%d", config.Host, config.Port)

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	dialer := net.Dialer{Timeout: 15 * time.Second}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("telnet dial: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.conn = conn
	s.cancel = cancel
	s.setStatus(StatusConnected)

	// Proactively negotiate binary transmission, character-at-a-time mode,
	// and terminal type. These are essential for arrow keys, backspace, etc.
	s.conn.Write([]byte{telnetIAC, telnetWILL, telnetOptBinary})
	s.conn.Write([]byte{telnetIAC, telnetDO, telnetOptSuppressGoAhead})
	s.conn.Write([]byte{telnetIAC, telnetWILL, telnetOptTerminalType})

	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		s.sendNAWS(cols, rows)
	} else {
		s.sendNAWS(80, 24)
	}

	go s.readLoop(ctx)
	go s.runPostLoginScript(ctx, config.PostLoginScript)

	// Auto-login: send username/password if configured
	if config.User != "" {
		go s.sendAutoLogin(ctx, config.User, config.Password)
	}

	return nil
}

func (s *TelnetSession) readLoop(ctx context.Context) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := s.conn.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			filtered := s.filterIAC(buf[:n])
			if len(filtered) > 0 {
				s.emitData(filtered)
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

func (s *TelnetSession) filterIAC(data []byte) []byte {
	var out []byte
	i := 0
	for i < len(data) {
		if data[i] == telnetIAC && i+1 < len(data) {
			cmd := data[i+1]
			switch cmd {
			case telnetWILL, telnetWONT, telnetDO, telnetDONT:
				if i+2 < len(data) {
					s.handleNegotiation(cmd, data[i+2])
					i += 3
					continue
				}
			case telnetSB:
				s.handleSubNegotiation(data, i)
				// Skip to end of sub-negotiation.
				found := false
				for j := i + 2; j < len(data)-1; j++ {
					if data[j] == telnetIAC && data[j+1] == telnetSE {
						i = j + 2
						found = true
						break
					}
				}
				if !found {
					i = len(data)
				}
				continue
			case telnetIAC:
				out = append(out, telnetIAC)
				i += 2
				continue
			default:
				i += 2
				continue
			}
		} else {
			out = append(out, data[i])
			i++
		}
	}
	return out
}

func (s *TelnetSession) handleSubNegotiation(data []byte, start int) {
	if start+3 >= len(data) {
		return
	}
	opt := data[start+2]
	if opt == telnetOptTerminalType && data[start+3] == telnetTTYPESend {
		// Server requests terminal type: reply with "xterm-256color"
		term := []byte("xterm-256color")
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
		case telnetOptEcho:
			s.reply(telnetDO, opt) // Accept server echoing
		case telnetOptTerminalType:
			s.reply(telnetDO, opt) // Accept server knows terminal type
		default:
			s.reply(telnetDONT, opt)
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

func (s *TelnetSession) sendNAWS(cols, rows int) {
	if s.conn == nil {
		return
	}
	data := []byte{
		telnetIAC, telnetSB, telnetOptNAWS,
		byte(cols >> 8), byte(cols & 0xff),
		byte(rows >> 8), byte(rows & 0xff),
		telnetIAC, telnetSE,
	}
	s.conn.Write(data)
}

func (s *TelnetSession) sendAutoLogin(ctx context.Context, user, password string) {
	time.Sleep(1500 * time.Millisecond)

	select {
	case <-ctx.Done():
		return
	default:
	}

	if s.conn != nil {
		s.conn.Write([]byte(user + "\r\n"))
	}

	if password != "" {
		time.Sleep(1200 * time.Millisecond)
		select {
		case <-ctx.Done():
			return
		default:
		}
		if s.conn != nil {
			s.conn.Write([]byte(password + "\r\n"))
		}
	}
}

func (s *TelnetSession) runPostLoginScript(ctx context.Context, script string) {
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
	_, err := s.conn.Write(data)
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
