package packetcap

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ys-ll/uniterm/backend/session"
	"golang.org/x/crypto/ssh"
)

// remoteDialConfig converts a ResolvedSSH into a session.ConnectionConfig for
// DialSSHClient. remote_capture.go owns the mapping so the app layer can hand
// us a plain resolved struct instead of reaching into the session package.
func remoteDialConfig(r *ResolvedSSH) session.ConnectionConfig {
	return session.ConnectionConfig{
		Host:       r.Host,
		Port:       r.Port,
		User:       r.User,
		Password:   r.Password,
		AuthType:   r.AuthType,
		KeyPath:    r.KeyPath,
		KeyContent: r.KeyContent,
		IdentityId: r.IdentityID,
	}
}

// remoteCapture connects to the remote Linux/macOS host over SSH and runs
// tcpdump, streaming raw pcap records (tcpdump -w -) back over stdout. Each
// captured frame is fed through the shared onPacket pipeline (parse + filter +
// emit), so remote traffic looks identical to local to the frontend.
//
// The SSH dial is non-interactive (session.DialSSHClient): no credential prompt
// appears here — the app layer already injected resolved credentials via
// SSHConfig (including any one-time password from the UI dialog).
func remoteCapture(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	if cfg.SSHConfig == nil {
		return fmt.Errorf("remote capture: missing resolved SSH config")
	}
	if cfg.SSHConfig.Host == "" {
		return fmt.Errorf("remote capture: missing remote host")
	}

	client, err := session.DialSSHClient(remoteDialConfig(cfg.SSHConfig))
	if err != nil {
		return fmt.Errorf("remote ssh dial: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("remote ssh session: %w", err)
	}
	defer sess.Close()

	// tcpdump writes the raw pcap stream to stdout with -U (unbuffered: each
	// captured packet is flushed immediately, keeping the stream near-real-time
	// instead of buffering many packets in kernel/stdio).
	iface := cfg.RemoteIface
	if iface == "" {
		iface = "any"
	}
	command := fmt.Sprintf("tcpdump -i %s -s 0 -U -w -", shellQuote(iface))
	stdout, err := sess.StdoutPipe()
	if err != nil {
		return fmt.Errorf("remote stdout pipe: %w", err)
	}
	stderr, err := sess.StderrPipe()
	if err != nil {
		return fmt.Errorf("remote stderr pipe: %w", err)
	}
	if err := sess.Start(command); err != nil {
		return fmt.Errorf("remote capture start: %w", err)
	}

	// Surface tcpdump's stderr (e.g. "Operation not permitted", "no suitable
	// device") to the frontend as a friendly error, and to a terminal log.
	errCh := make(chan error, 1)
	go func() {
		b, _ := io.ReadAll(stderr)
		if len(bytes.TrimSpace(b)) > 0 {
			errCh <- fmt.Errorf("remote tcpdump: %s", strings.TrimSpace(string(b)))
			return
		}
		errCh <- nil
	}()

	// Drain the pcap stream and hand every frame to the shared pipeline. Runs
	// until ctx is cancelled (Stop) or tcpdump exits.
	streamErr := streamPcap(ctx, stdout, onPacket)

	// Tell tcpdump to stop cleanly, then reap the process so no orphan remains.
	if ctx.Err() == nil {
		_ = sess.Signal(ssh.SIGINT)
	}
	// Wait for tcpdump exit and the stderr reader; the Wait must complete before
	// Close so the remote session is reaped.
	_ = sess.Wait()
	select {
	case se := <-errCh:
		if se != nil {
			if streamErr == nil {
				streamErr = se
			} else if ctx.Err() == nil {
				// Attach stderr detail to the stream error.
				streamErr = fmt.Errorf("%v; %w", se, streamErr)
			}
		}
	default:
	}

	if streamErr != nil && ctx.Err() != nil {
		// User cancelled: a stream error during shutdown is expected.
		return errStopped
	}
	if streamErr != nil {
		return streamErr
	}
	return errStopped
}

// streamPcap reads a pcap byte stream from r and calls onPacket for every frame
// with its capture timestamp. Returns nil on success, the pcap read error on
// failure, or errStopped if ctx is cancelled.
func streamPcap(ctx context.Context, r io.Reader, onPacket func(*Packet)) error {
	br := bufio.NewReaderSize(r, 1<<16)

	// pcap global header: 24 bytes. Magic 0xa1b2c3d4 (linktype at offset 20).
	var gh [24]byte
	if _, err := io.ReadFull(br, gh[:]); err != nil {
		return fmt.Errorf("remote pcap global header: %w", err)
	}
	// Endianness: little-endian magic (d4 c3 b2 a1) is the common case; check
	// the byte order so linktype/timestamps parse correctly on all hosts.
	bo := binary.ByteOrder(binary.LittleEndian)
	if bytes.Equal(gh[0:4], []byte{0xa1, 0xb2, 0xc3, 0xd4}) {
		bo = binary.BigEndian
	}
	snaplen := bo.Uint32(gh[16:20])

	var prev []byte
	for {
		if ctx.Err() != nil {
			return errStopped
		}
		var ph [16]byte
		if _, err := io.ReadFull(br, ph[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			// A short read while stopping is normal.
			if ctx.Err() != nil {
				return errStopped
			}
			return fmt.Errorf("remote pcap record: %w", err)
		}
		sec := bo.Uint32(ph[0:4])
		usec := bo.Uint32(ph[4:8])
		incl := bo.Uint32(ph[8:12])
		if incl == 0 || (snaplen != 0 && incl > snaplen*2) {
			// Sanity guard against corrupt lengths; skip malformed records.
			continue
		}
		if cap(prev) < int(incl) {
			prev = make([]byte, int(incl))
		}
		buf := prev[:int(incl)]
		if _, err := io.ReadFull(br, buf); err != nil {
			if ctx.Err() != nil {
				return errStopped
			}
			return fmt.Errorf("remote pcap frame: %w", err)
		}
		ts := time.Unix(int64(sec), int64(usec)*1000)
		if p := ParseFrame(buf, ts); p != nil {
			onPacket(p)
		}
	}
}

// ListRemoteInterfaces enumerates the remote Linux/macOS host's network
// interfaces over SSH. It reuses the local Interface shape so the frontend
// dropdown is uniform. A synthetic "any" entry is NOT added here — the UI adds
// it (tcpdump supports -i any).
func ListRemoteInterfaces(r *ResolvedSSH) ([]Interface, error) {
	if r == nil || r.Host == "" {
		return nil, fmt.Errorf("remote capture: missing resolved SSH config")
	}
	client, err := session.DialSSHClient(remoteDialConfig(r))
	if err != nil {
		return nil, fmt.Errorf("remote ssh dial: %w", err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("remote ssh session: %w", err)
	}
	defer sess.Close()

	// One shell snippet lists each interface name plus its type (1 = Ethernet)
	// and IPv4 addresses. Uses /sys/class/net which exists on Linux hosts.
	script := "for i in /sys/class/net/*; do n=${i##*/}; t=$(cat $i/type 2>/dev/null); ips=$(ip -4 -o addr show $n 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | paste -sd, -); echo \"$n|$t|$ips\"; done 2>/dev/null"
	out, err := sess.Output(script)
	if err != nil {
		// Output conflates stdout/stderr; fall back to a simpler listing.
		out, err2 := sess.Output("ls /sys/class/net/ 2>/dev/null")
		if err2 != nil {
			return nil, fmt.Errorf("remote interface list: %v", err)
		}
		var res []Interface
		for _, name := range strings.Fields(string(out)) {
			res = append(res, Interface{Name: name, Flags: "remote"})
		}
		return res, nil
	}

	var res []Interface
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 3)
		name := parts[0]
		ip := ""
		flags := "remote"
		if len(parts) > 2 {
			ip = parts[2]
		}
		if len(parts) > 1 && parts[1] == "1" {
			flags = "ethernet"
		}
		res = append(res, Interface{Name: name, IP: ip, Flags: flags})
	}
	return res, nil
}

// shellQuote single-quotes a shell argument safely (tcpdump -i needs the iface
// name verbatim; iface names are alphanumeric but quoting keeps it robust).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
