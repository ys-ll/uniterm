package session

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"
)

// readNegotiation reads n bytes from the pipe with a deadline, so the
// blocking net.Pipe writes inside sendActiveNegotiation/sendNAWS complete.
func readNegotiation(t *testing.T, server net.Conn, n int) []byte {
	t.Helper()
	server.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, n)
	total := 0
	for total < n {
		k, err := server.Read(got[total:])
		if err != nil {
			t.Fatalf("read negotiation bytes: %v (got %d/%d)", err, total, n)
		}
		total += k
	}
	return got
}

// expectSilent asserts the session sends nothing on the pipe within a short
// deadline.
func expectSilent(t *testing.T, server net.Conn) {
	t.Helper()
	server.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	reply := make([]byte, 64)
	if n, err := server.Read(reply); err != io.EOF && n > 0 {
		t.Fatalf("unexpected bytes sent: % x", reply[:n])
	}
}

func TestActiveNegotiationDoesNotOfferBinary(t *testing.T) {
	s := NewTelnetSession("test")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sendActiveNegotiation()
	}()

	// DO SGA (3) + WILL TTYPE (3) + WILL NAWS (3) = 9 bytes. The window-size
	// sub-negotiation itself must NOT appear here: it is only sent after the
	// server answers DO NAWS, otherwise peers that never negotiated NAWS
	// (IRIS/Caché, legacy consoles) read its four payload bytes as keystrokes.
	got := readNegotiation(t, server, 9)
	<-done

	want := append([]byte{},
		telnetIAC, telnetDO, telnetOptSuppressGoAhead,
		telnetIAC, telnetWILL, telnetOptTerminalType,
		telnetIAC, telnetWILL, telnetOptNAWS)
	if !bytes.Equal(got, want) {
		t.Fatalf("active negotiation = % x, want % x", got, want)
	}
	if bytes.Contains(got, []byte{telnetIAC, telnetWILL, telnetOptBinary}) {
		t.Fatal("active negotiation offered WILL BINARY (Cisco echoes passwords in binary mode)")
	}
}

// TestNAWSSentAfterDoNaws: the DO NAWS handshake is what authorizes the
// sub-negotiation; the pending size is delivered immediately on agreement.
func TestNAWSSentAfterDoNaws(t *testing.T) {
	s := NewTelnetSession("test")
	s.SetPendingSize(132, 43)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleNegotiation(telnetDO, telnetOptNAWS)
	}()

	// WILL NAWS (3) + SB NAWS 132x43 (9) = 12 bytes.
	got := readNegotiation(t, server, 12)
	<-done

	want := append([]byte{},
		telnetIAC, telnetWILL, telnetOptNAWS,
		telnetIAC, telnetSB, telnetOptNAWS,
		0, 132, 0, 43,
		telnetIAC, telnetSE)
	if !bytes.Equal(got, want) {
		t.Fatalf("DO NAWS reply = % x, want % x", got, want)
	}
}

// TestNAWSNotSentWithoutAgreement: Resize before any DO NAWS handshake must
// stay silent on the wire.
func TestNAWSNotSentWithoutAgreement(t *testing.T) {
	s := NewTelnetSession("test")

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	if err := s.Resize(115, 41); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	expectSilent(t, server)
}

// TestNAWSStoppedAfterDont: after the server withdraws agreement (WONT/DONT
// NAWS) further resizes must not send anything.
func TestNAWSStoppedAfterDont(t *testing.T) {
	s := NewTelnetSession("test")
	s.SetPendingSize(100, 30)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.handleNegotiation(telnetDO, telnetOptNAWS)
	}()
	readNegotiation(t, server, 12) // WILL NAWS + initial sub-negotiation
	<-done

	done = make(chan struct{})
	go func() {
		defer close(done)
		s.handleNegotiation(telnetDONT, telnetOptNAWS)
		s.Resize(120, 40)
	}()
	expectSilent(t, server)
	<-done
}

func TestSendNAWSExactBytes(t *testing.T) {
	cases := []struct {
		name       string
		cols, rows int
		want       []byte
	}{
		{
			"normal size",
			80, 24,
			[]byte{telnetIAC, telnetSB, telnetOptNAWS, 0, 80, 0, 24, telnetIAC, telnetSE},
		},
		{
			"cols low byte 255 must be IAC IAC escaped (RFC 1073)",
			255, 24,
			[]byte{telnetIAC, telnetSB, telnetOptNAWS, 0, telnetIAC, telnetIAC, 0, 24, telnetIAC, telnetSE},
		},
		{
			"rows 255 escaped",
			80, 255,
			[]byte{telnetIAC, telnetSB, telnetOptNAWS, 0, 80, 0, telnetIAC, telnetIAC, telnetIAC, telnetSE},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewTelnetSession("test")
			s.nawsAgreed = true

			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			s.conn = client

			done := make(chan struct{})
			go func() {
				defer close(done)
				s.sendNAWS(tc.cols, tc.rows)
			}()

			got := readNegotiation(t, server, len(tc.want))
			<-done
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("NAWS = % x, want % x", got, tc.want)
			}
		})
	}
}
