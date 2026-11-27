package localsrv

import (
"bytes"
	"context"
"encoding/binary"
	"errors"
"fmt"
	"io"
	"net"
"os"
"path/filepath"
	"strconv"
"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func startForwardTestSSH(t *testing.T) (*Manager, string, string) {
	t.Helper()
	m := NewManager()
	port := freePort(t)
	id, err := m.StartSSHServer(SSHConfig{RootDir: t.TempDir(), Port: port, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(id) })
	return m, id, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func TestSSHForwardJumpHost(t *testing.T) {
	_, _, jumpAddr := startForwardTestSSH(t)
	_, _, targetAddr := startForwardTestSSH(t)
	jump := dialTestSSH(t, jumpAddr, "user", "pass")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := jump.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		t.Fatalf("dial through jump host: %v", err)
	}
	defer raw.Close()
	// Bound the inner handshake, which runs over an SSH channel without deadlines.
	stopClose := context.AfterFunc(ctx, func() { raw.Close() })
	defer stopClose()
	conn, chans, reqs, err := ssh.NewClientConn(raw, targetAddr, &ssh.ClientConfig{
		User: "user", Auth: []ssh.AuthMethod{ssh.Password("pass")}, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("target SSH handshake: %v", err)
	}
	target := ssh.NewClient(conn, chans, reqs)
	defer target.Close()
	session, err := target.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.CombinedOutput("test"); err != nil {
		t.Fatalf("target session through jump host: %v", err)
	}
}

func TestSSHForwardHalfClose(t *testing.T) {
	_, _, addr := startForwardTestSSH(t)
	client := dialTestSSH(t, addr, "user", "pass")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		data, err := io.ReadAll(conn)
		if err == nil {
			_, err = conn.Write(append([]byte("reply:"), data...))
		}
		serverDone <- err
	}()
	ch, reqs, err := client.OpenChannel("direct-tcpip", ssh.Marshal(directTCPIPRequest{
		Host: "127.0.0.1", Port: uint32(ln.Addr().(*net.TCPAddr).Port), OriginHost: "127.0.0.1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	go ssh.DiscardRequests(reqs)
	if _, err := io.WriteString(ch, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := ch.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(ch)
	if err != nil || string(data) != "reply:hello" {
		t.Fatalf("response after half-close = %q, %v", data, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSSHForwardCleanup(t *testing.T) {
	for _, action := range []string{"channel", "client", "server"} {
		t.Run(action, func(t *testing.T) {
			m, id, addr := startForwardTestSSH(t)
			client := dialTestSSH(t, addr, "user", "pass")
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			forward, err := client.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer forward.Close()
			_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
			target, err := ln.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			switch action {
			case "channel":
				forward.Close()
			case "client":
				client.Close()
			case "server":
				m.Stop(id)
			}
			_ = target.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, err := target.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("forwarded connection remains open: %v", err)
			}
		})
	}
}

func TestSSHForwardRejectsInvalidDestination(t *testing.T) {
	_, _, addr := startForwardTestSSH(t)
	client := dialTestSSH(t, addr, "user", "pass")
	for _, payload := range [][]byte{
		{0},
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: 0}),
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: 65536}),
		ssh.Marshal(directTCPIPRequest{Port: 22}),
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: uint32(freePort(t))}),
	} {
		ch, _, err := client.OpenChannel("direct-tcpip", payload)
		if err == nil {
			ch.Close()
			t.Fatal("invalid or unreachable destination accepted")
		}
		var openErr *ssh.OpenChannelError
		if !errors.As(err, &openErr) || openErr.Reason != ssh.ConnectionFailed {
			t.Fatalf("expected ConnectionFailed, got %v", err)
		}
	}
	if session, err := client.NewSession(); err != nil {
		t.Fatalf("failed forwarding damaged SSH connection: %v", err)
	} else {
		session.Close()
	}
}


// TestTftpPlainWRQ512 simulates a client that performs a plain RFC 1350 WRQ
// with NO option negotiation (so 512-byte blocks), as many network devices do.
func TestTftpPlainWRQ512(t *testing.T) {
	for _, size := range []int{1024, 1200, 1337, 4096, 2000} {
		t.Run(fmt.Sprintf("size=%d", size), func(t *testing.T) {
			root := t.TempDir()
			mgr := NewManager()
			l, _ := net.ListenPacket("udp", "127.0.0.1:0")
			port := l.LocalAddr().(*net.UDPAddr).Port
			l.Close()
			// 8192 is the default "TFTP buffer size" in the UI; it must only be
			// offered when a client negotiates blksize, never used as the block
			// size of a plain RFC 1350 transfer (which always uses 512).
			id, err := mgr.StartTFTPServer(TFTPConfig{RootDir: root, Port: port, BlockSize: 8192})
			if err != nil {
				t.Fatalf("start tftp: %v", err)
			}
			defer mgr.Stop(id)

			payload := make([]byte, size)
			for i := range payload {
				payload[i] = byte(i * 13)
			}
			if err := plainWrq(t, port, "dorm-w4-conf", payload); err != nil {
				t.Fatalf("WRQ failed: %v", err)
			}
			onDisk, err := os.ReadFile(filepath.Join(root, "dorm-w4-conf"))
			if err != nil {
				t.Fatalf("not persisted: %v", err)
			}
			if !bytes.Equal(onDisk, payload) {
				t.Fatalf("mismatch: got %d bytes, want %d", len(onDisk), len(payload))
			}
			t.Logf("OK %d bytes", size)
		})
	}
}

// plainWrq behaves like a minimal RFC 1350 client: WRQ with no options, wait for
// ACK(0) from the server TID, then push 512-byte DATA blocks.
func plainWrq(t *testing.T, port int, name string, payload []byte) error {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer conn.Close()
	server := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}

	req := binary.BigEndian.AppendUint16(nil, 2)
	req = append(req, []byte(name)...)
	req = append(req, 0)
	req = append(req, []byte("octet")...)
	req = append(req, 0)
	if _, err := conn.WriteTo(req, server); err != nil {
		return err
	}

	buf := make([]byte, 4096)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, from, err := conn.ReadFrom(buf)
	if err != nil {
		return fmt.Errorf("no ACK(0): %w", err)
	}
	if binary.BigEndian.Uint16(buf[:2]) != 4 || binary.BigEndian.Uint16(buf[2:4]) != 0 {
		return fmt.Errorf("unexpected negotiation reply: %v (n=%d)", buf[:n], n)
	}
	tid := from.(*net.UDPAddr)

	const bs = 512
	block := uint16(1)
	off := 0
	for {
		chunk := payload[off:]
		if len(chunk) > bs {
			chunk = chunk[:bs]
		}
		pkt := binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, 3), block)
		pkt = append(pkt, chunk...)
		acked := false
		for attempt := 0; attempt < 4 && !acked; attempt++ {
			if _, err := conn.WriteTo(pkt, tid); err != nil {
				return err
			}
			deadline := time.Now().Add(1 * time.Second)
			for time.Now().Before(deadline) {
				_ = conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
				n, _, err := conn.ReadFrom(buf)
				if err != nil {
					continue
				}
				op := binary.BigEndian.Uint16(buf[:2])
				if op == 5 {
					return fmt.Errorf("server ERROR: %s", string(buf[4:n]))
				}
				if op == 4 && binary.BigEndian.Uint16(buf[2:4]) == block {
					acked = true
					break
				}
			}
		}
		if !acked {
			return fmt.Errorf("no ACK for block %d (offset %d)", block, off)
		}
		off += len(chunk)
		if len(chunk) < bs {
			return nil
		}
		block++
	}
}


// TestTftpWriteMultiBlock simulates a network device doing a WRQ (put) with
// a blksize option and a multi-block payload — the scenario that previously
// stalled part-way through the upload.
func TestTftpWriteMultiBlock(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager()

	// find a free port
	l, _ := net.ListenPacket("udp", "127.0.0.1:0")
	port := l.LocalAddr().(*net.UDPAddr).Port
	l.Close()

	id, err := mgr.StartTFTPServer(TFTPConfig{
		RootDir:   root,
		Port:      port,
		BlockSize: 8192,
	})
	if err != nil {
		t.Fatalf("start tftp: %v", err)
	}
	defer mgr.Stop(id)

	payloadLen := 2780 // vrpcfg.zip size
	payload := make([]byte, payloadLen)
	for i := range payload {
		payload[i] = byte(i*7 + i/3)
	}

	const blksize = 1428 // commonly negotiated by network devices
	if err := wrqTransfer(t, port, "cfg.bin", payload, blksize); err != nil {
		t.Fatalf("WRQ transfer failed: %v", err)
	}

	outPath := filepath.Join(root, "cfg.bin")
	onDisk, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("server did not persist file: %v", err)
	}
	if !bytes.Equal(onDisk, payload) {
		t.Fatalf("on-disk mismatch: got %d bytes, want %d", len(onDisk), len(payload))
	}
	t.Logf("OK: WRQ %d bytes @ blksize=%d persisted %d bytes", payloadLen, blksize, len(onDisk))
}

// oackIntOption extracts an integer option from an OACK payload
// (opcode already stripped): "name\0value\0" pairs.
func oackIntOption(body []byte, name string) int {
	parts := strings.Split(string(body), "\x00")
	for i := 0; i+1 < len(parts); i += 2 {
		if strings.EqualFold(parts[i], name) {
			v := 0
			fmt.Sscanf(parts[i+1], "%d", &v)
			return v
		}
	}
	return 0
}

// wrqTransfer performs a complete WRQ client exchange against the TFTP server
// using an unconnected UDP socket (so it can receive ACKs from the server's
// per-transfer TID socket, exactly like a real network device).
func wrqTransfer(t *testing.T, port int, name string, payload []byte, blksize int) error {
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer conn.Close()
	server := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}

	// WRQ + blksize option
	req := binary.BigEndian.AppendUint16(nil, 2) // WRQ
	req = append(req, []byte(name)...)
	req = append(req, 0)
	req = append(req, []byte("octet")...)
	req = append(req, 0)
	req = append(req, []byte("blksize")...)
	req = append(req, 0)
	req = append(req, []byte(fmt.Sprintf("%d", blksize))...)
	req = append(req, 0)
	if _, err := conn.WriteTo(req, server); err != nil {
		return err
	}

	buf := make([]byte, 4096)
	// negotiation: expect OACK(6) or ack(0). Remember the server's TID address
	// (an ephemeral port) — subsequent DATA must be sent there, not to port 69.
	var tid *net.UDPAddr
	for {
		if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			return err
		}
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			return fmt.Errorf("read negotiation: %w", err)
		}
		op := binary.BigEndian.Uint16(buf[:2])
		fromUDP := from.(*net.UDPAddr)
		if op == 5 {
			return fmt.Errorf("server ERROR during negotiation: %s", string(buf[4:n]))
		}
		// ACK(0) either way
		ack := binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, 4), 0)
		conn.WriteTo(ack, fromUDP)
		if op == 6 || (op == 4 && binary.BigEndian.Uint16(buf[2:4]) == 0) {
			if op == 6 {
				// RFC 2347: the server's OACK value is authoritative. It may be
				// smaller than what we asked for (capped by the server config).
				if v := oackIntOption(buf[2:n], "blksize"); v > 0 {
					blksize = v
				}
			}
			tid = fromUDP
			break
		}
	}

	block := uint16(1)
	off := 0
	for {
		chunk := payload[off:]
		if len(chunk) > blksize {
			chunk = chunk[:blksize]
		}
		pkt := binary.BigEndian.AppendUint16(binary.BigEndian.AppendUint16(nil, 3), block)
		pkt = append(pkt, chunk...)
		if _, err := conn.WriteTo(pkt, tid); err != nil {
			return fmt.Errorf("write DATA block %d: %w", block, err)
		}

		// wait for ACK(block); retransmit once on timeout like a real device
		acked := false
		for attempt := 0; attempt < 2 && !acked; attempt++ {
			if attempt > 0 {
				conn.WriteTo(pkt, tid) // retransmit
			}
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
					return err
				}
				n, _, err := conn.ReadFrom(buf)
				if err != nil {
					continue
				}
				op := binary.BigEndian.Uint16(buf[:2])
				if op == 5 {
					return fmt.Errorf("server ERROR: %s at block %d (offset %d)", string(buf[4:n]), block, off)
				}
				if op == 4 && binary.BigEndian.Uint16(buf[2:4]) == block {
					acked = true
					break
				}
			}
		}
		if !acked {
			return fmt.Errorf("no ACK for block %d after retransmit (offset %d)", block, off)
		}
		off += len(chunk)
		if len(chunk) < blksize {
			break
		}
		block++
	}
	return nil
}
