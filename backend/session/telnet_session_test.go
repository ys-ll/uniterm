package session

import (
	"bytes"
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

	// DO SGA (3) + WILL TTYPE (3) + NAWS 80x24 (9) = 15 bytes.
	got := readNegotiation(t, server, 15)
	<-done

	want := append([]byte{},
		telnetIAC, telnetDO, telnetOptSuppressGoAhead,
		telnetIAC, telnetWILL, telnetOptTerminalType,
		telnetIAC, telnetSB, telnetOptNAWS,
		0, 80, 0, 24,
		telnetIAC, telnetSE)
	if !bytes.Equal(got, want) {
		t.Fatalf("active negotiation = % x, want % x", got, want)
	}
	if bytes.Contains(got, []byte{telnetIAC, telnetWILL, telnetOptBinary}) {
		t.Fatal("active negotiation offered WILL BINARY (issue #1486: Cisco echoes passwords in binary mode)")
	}
}

func TestActiveNegotiationUsesPendingSize(t *testing.T) {
	s := NewTelnetSession("test")
	s.SetPendingSize(132, 43)

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.sendActiveNegotiation()
	}()

	// DO SGA (3) + WILL TTYPE (3) + NAWS 132x43 (9) = 15 bytes.
	got := readNegotiation(t, server, 15)
	<-done

	want := append([]byte{},
		telnetIAC, telnetDO, telnetOptSuppressGoAhead,
		telnetIAC, telnetWILL, telnetOptTerminalType,
		telnetIAC, telnetSB, telnetOptNAWS,
		0, 132, 0, 43,
		telnetIAC, telnetSE)
	if !bytes.Equal(got, want) {
		t.Fatalf("active negotiation = % x, want % x", got, want)
	}
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
