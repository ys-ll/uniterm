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

// TestTftpWriteMultiBlock simulates a network device doing a WRQ (put) with
// a blksize option and a multi-block payload — mirroring the Huawei YuZhou
// S5735 `tftp put vrpcfg.zip` scenario that previously died at ~51%.
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
		BlockSize: 512,
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

	const blksize = 1428 // Huawei class devices commonly negotiate ~1428
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
