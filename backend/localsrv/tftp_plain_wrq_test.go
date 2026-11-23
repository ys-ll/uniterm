package localsrv

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
