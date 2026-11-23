package localsrv

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TFTP opcodes (RFC 1350).
const (
	tftpOpRRQ   uint16 = 1 // Read request
	tftpOpWRQ   uint16 = 2 // Write request
	tftpOpDATA  uint16 = 3 // Data
	tftpOpACK   uint16 = 4 // Acknowledgement
	tftpOpERROR uint16 = 5 // Error
	tftpOpOACK  uint16 = 6 // Option Acknowledgement (RFC 2347)
)

// TFTP error codes.
const (
	tftpErrNotDefined   uint16 = 0
	tftpErrFileNotFound uint16 = 1
	tftpErrAccess       uint16 = 2
	tftpErrDiskFull     uint16 = 3
	tftpErrIllegalOp    uint16 = 4
	tftpErrUnknownTID   uint16 = 5
	tftpErrFileExists   uint16 = 6
	tftpErrNoUser       uint16 = 7
)

const tftpDefaultBlockSize = 512

// StartTFTPServer starts a TFTP server serving rootDir on the given UDP port.
// blockSize overrides the default 512-byte block size when the client requests
// option negotiation. Returns the server id.
func (m *Manager) StartTFTPServer(cfg TFTPConfig) (string, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("invalid port: %d", cfg.Port)
	}
	blockSize := cfg.BlockSize
	if blockSize <= 0 {
		blockSize = tftpDefaultBlockSize
	}
	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return "", fmt.Errorf("invalid root directory: %w", err)
	}

	id, ctx := m.newServer()
	m.log(ServiceTFTP, id, fmt.Sprintf("Starting TFTP server on port %d (block size %d) ...", cfg.Port, blockSize))

	conn, err := net.ListenPacket("udp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to listen on UDP port %d: %w", cfg.Port, err)
	}

	m.setStop(id, func() {
		_ = conn.Close()
	})

	go func() {
		defer m.finish(id)
		m.state(ServiceTFTP, id, true)
		m.log(ServiceTFTP, id, "TFTP server started")

		buf := make([]byte, 65535)
		for {
			select {
			case <-ctx.Done():
				m.state(ServiceTFTP, id, false)
				m.log(ServiceTFTP, id, "TFTP server stopped")
				return
			default:
			}
			_ = conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
			n, addr, err := conn.ReadFrom(buf)
			if err != nil {
				if ctx.Err() != nil {
					m.state(ServiceTFTP, id, false)
					m.log(ServiceTFTP, id, "TFTP server stopped")
					return
				}
				continue
			}
			if n < 2 {
				continue
			}
			op := binary.BigEndian.Uint16(buf[:2])
			switch op {
			case tftpOpRRQ, tftpOpWRQ:
				go m.handleTFTPRequest(ctx, id, addr, buf[:n], absRoot, blockSize)
			default:
				// Ignore unexpected packets on the main socket.
			}
		}
	}()

	m.startTimeout(id, ServiceTFTP, cfg.TimeoutSeconds)
	return id, nil
}

// parseTFTPRequest extracts the filename, mode, and options from an RRQ/WRQ packet.
func parseTFTPRequest(pkt []byte) (filename, mode string, options map[string]string) {
	// Format: opcode(2) + filename\0 + mode\0 + [opt1\0 val1\0 ...]
	rest := pkt[2:]
	parts := strings.Split(string(rest), "\x00")
	if len(parts) < 2 {
		return
	}
	filename = parts[0]
	mode = parts[1]
	options = make(map[string]string)
	for i := 2; i+1 < len(parts); i += 2 {
		if parts[i] == "" {
			break
		}
		options[strings.ToLower(parts[i])] = parts[i+1]
	}
	return
}

// handleTFTPRequest processes an RRQ or WRQ by opening a new UDP socket for
// the transfer and sending data (read) or receiving data (write).
func (m *Manager) handleTFTPRequest(ctx context.Context, id string, clientAddr net.Addr, pkt []byte, rootDir string, blockSize int) {
	op := binary.BigEndian.Uint16(pkt[:2])
	filename, _, options := parseTFTPRequest(pkt)
	if filename == "" {
		m.sendTFTPError(clientAddr, tftpErrIllegalOp, "bad request")
		return
	}

	// Sanitize: prevent path traversal. Only paths that escape above the root
	// ("..") are denied; dotfiles inside the root are allowed.
	cleanName := filepath.Clean("/" + filename)
	fullPath := filepath.Join(rootDir, cleanName)
	rel, err := filepath.Rel(rootDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		m.sendTFTPError(clientAddr, tftpErrAccess, "access denied")
		return
	}

	// Open a new UDP socket for this transfer (TID).
	conn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		m.sendTFTPError(clientAddr, tftpErrNotDefined, "no resources")
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Negotiate block size via OACK if the client sent "blksize".
	negotiatedBlock := blockSize
	didOACK := false
	if bs, ok := options["blksize"]; ok {
		var requested int
		fmt.Sscanf(bs, "%d", &requested)
		if requested > 0 {
			negotiatedBlock = requested
		}
		// Send OACK with the negotiated block size.
		oack := make([]byte, 0, 16)
		oack = binary.BigEndian.AppendUint16(oack, tftpOpOACK)
		oack = append(oack, []byte("blksize\x00")...)
		oack = append(oack, []byte(fmt.Sprintf("%d\x00", negotiatedBlock))...)
		if _, err := conn.WriteTo(oack, clientAddr); err != nil {
			return
		}
		didOACK = true
		// OACK is itself the implicit acknowledgement. Don't sit here waiting
		// for an explicit ACK(0) — many TFTP clients (notably Huawei network
		// devices) skip it and jump straight to the first DATA block. The
		// data/ACK loops in tftpWriteFile/tftpReadFile will handle whichever
		// packet shows up first, and learn the peer's current source address
		// from it.
	}

	if op == tftpOpRRQ {
		m.tftpReadFile(ctx, id, conn, clientAddr, fullPath, negotiatedBlock, didOACK)
	} else {
		m.tftpWriteFile(ctx, id, conn, clientAddr, fullPath, negotiatedBlock, didOACK)
	}
}

// tftpReadFile sends a file to the client in DATA packets.
func (m *Manager) tftpReadFile(ctx context.Context, id string, conn net.PacketConn, clientAddr net.Addr, fullPath string, blockSize int, didOACK bool) {
	f, err := os.Open(fullPath)
	if err != nil {
		m.sendTFTPError(clientAddr, tftpErrFileNotFound, "file not found")
		return
	}
	defer f.Close()

	m.log(ServiceTFTP, id, fmt.Sprintf("RRQ %s -> %s", clientAddr, filepath.Base(fullPath)))

	block := uint16(1)
	buf := make([]byte, blockSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := f.Read(buf)
		if err != nil && err.Error() != "EOF" {
			m.log(ServiceTFTP, id, fmt.Sprintf("read error: %v", err))
			return
		}
		data := make([]byte, 4+n)
		binary.BigEndian.PutUint16(data[0:2], tftpOpDATA)
		binary.BigEndian.PutUint16(data[2:4], block)
		copy(data[4:], buf[:n])

		// Retry on timeout.
		for retries := 0; retries < 5; retries++ {
			if _, werr := conn.WriteTo(data, clientAddr); werr != nil {
				return
			}
			if ok, newAddr := m.recvTFTPAck(conn, clientAddr, block); ok {
				// Learn the peer's current address from every ACK we get —
				// the client may re-source to a different ephemeral port
				// mid-transfer, so keep our target up-to-date.
				if newAddr != nil {
					clientAddr = newAddr
				}
				break
			}
			if retries == 4 {
				return
			}
		}
		block++
		if n < blockSize {
			// Last packet: transfer complete.
			m.log(ServiceTFTP, id, fmt.Sprintf("sent %s", filepath.Base(fullPath)))
			return
		}
	}
}

// tftpWriteFile receives a file from the client in DATA packets.
func (m *Manager) tftpWriteFile(ctx context.Context, id string, conn net.PacketConn, clientAddr net.Addr, fullPath string, blockSize int, didOACK bool) {
	// Standard WRQ flow: if no OACK was negotiated we send ACK(0) now to
	// signal we are ready to receive. When OACK was negotiated that already
	// serves as the implicit acknowledgement — sending ACK(0) on top would
	// be redundant and, with clients that close their initial socket right
	// after WRQ (Huawei network devices), would just generate a spurious
	// ICMP port-unreachable back at us.
	if !didOACK {
		if !m.sendTFTPAck(conn, clientAddr, 0) {
			return
		}
	}

	f, err := os.Create(fullPath)
	if err != nil {
		m.sendTFTPError(clientAddr, tftpErrAccess, "cannot create file")
		return
	}
	defer f.Close()

	m.log(ServiceTFTP, id, fmt.Sprintf("WRQ %s -> %s", clientAddr, filepath.Base(fullPath)))

	block := uint16(1)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		buf := make([]byte, blockSize+4)
		n, addr, err := conn.ReadFrom(buf)
		if err != nil {
			m.log(ServiceTFTP, id, fmt.Sprintf("receive error: %v", err))
			return
		}
		// Learn the peer's current address from every incoming packet. Many
		// TFTP clients (notably Huawei/YuZhou network devices) re-source
		// subsequent DATA/ACK packets from a *different* ephemeral port than
		// the one used for the initial WRQ/RRQ. If we still send our ACKs to
		// the stale address captured at WRQ time, the peer will never see them
		// and time out mid-transfer — the classic "51 % dead" symptom.
		if addr.String() != clientAddr.String() {
			clientAddr = addr
		}
		if n < 4 {
			continue
		}
		op := binary.BigEndian.Uint16(buf[:2])
		if op == tftpOpERROR {
			m.log(ServiceTFTP, id, "client error")
			return
		}
		// After an OACK negotiation the client may send an explicit ACK(0)
		// (RFC-compliant) or jump straight to the first DATA block (Huawei
		// and other terse clients). Either is fine — an ACK(0) just means
		// "proceed", so skip it and keep waiting for real data.
		if op == tftpOpACK {
			continue
		}
		if op != tftpOpDATA {
			continue
		}
		recvBlock := binary.BigEndian.Uint16(buf[2:4])
		if recvBlock == block {
			// Expected block: append payload and advance.
			data := buf[4:n]
			if _, werr := f.Write(data); werr != nil {
				m.sendTFTPError(clientAddr, tftpErrDiskFull, "write failed")
				return
			}
			m.sendTFTPAck(conn, clientAddr, recvBlock)
			if recvBlock != 0 && len(data) < blockSize {
				m.log(ServiceTFTP, id, fmt.Sprintf("received %s", filepath.Base(fullPath)))
				return
			}
			block++
			continue
		}
		// Old/duplicate block (our ACK was lost, so the client resends) or an
		// out-of-order block: don't advance, but idempotently re-ACK it so the
		// client can continue instead of timing out and aborting the transfer.
		if recvBlock > 0 && recvBlock < block {
			m.sendTFTPAck(conn, clientAddr, recvBlock)
		}
	}
}

// recvTFTPAck reads a packet and returns true if it is an ACK for block.
// The caller-provided clientAddr is used only as a hint for the initial peer
// address; we intentionally accept an ACK from *any* source address and
// return the actual peer address so the caller can learn if the client has
// re-sourced to a different ephemeral port (as Huawei network devices do).
func (m *Manager) recvTFTPAck(conn net.PacketConn, clientAddr net.Addr, block uint16) (bool, net.Addr) {
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 64)
	n, addr, err := conn.ReadFrom(buf)
	if err != nil {
		return false, clientAddr
	}
	if n < 4 {
		return false, clientAddr
	}
	op := binary.BigEndian.Uint16(buf[:2])
	if op != tftpOpACK {
		return false, clientAddr
	}
	recvBlock := binary.BigEndian.Uint16(buf[2:4])
	return recvBlock == block, addr
}

// sendTFTPAck sends an ACK packet for the given block number.
func (m *Manager) sendTFTPAck(conn net.PacketConn, clientAddr net.Addr, block uint16) bool {
	ack := make([]byte, 4)
	binary.BigEndian.PutUint16(ack[0:2], tftpOpACK)
	binary.BigEndian.PutUint16(ack[2:4], block)
	_, err := conn.WriteTo(ack, clientAddr)
	return err == nil
}

// sendTFTPError sends an ERROR packet directly to the client (used on the main socket).
func (m *Manager) sendTFTPError(clientAddr net.Addr, code uint16, msg string) {
	conn, err := net.Dial("udp", clientAddr.String())
	if err != nil {
		return
	}
	defer conn.Close()
	pkt := make([]byte, 0, 5+len(msg))
	pkt = binary.BigEndian.AppendUint16(pkt, tftpOpERROR)
	pkt = binary.BigEndian.AppendUint16(pkt, code)
	pkt = append(pkt, []byte(msg)...)
	pkt = append(pkt, 0)
	_, _ = conn.Write(pkt)
}
