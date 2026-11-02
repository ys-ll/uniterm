package netscan

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ServicePort is one of the well-known service ports probed for every host
// during an IP-range scan (shown as columns in the UI, like MobaXterm).
type ServicePort struct {
	Port int
	Name string
}

// ServicePorts probed per host, in UI column order.
var ServicePorts = []ServicePort{
	{22, "ssh"},
	{23, "telnet"},
	{3389, "rdp"},
	{5900, "vnc"},
	{21, "ftp"},
	{445, "smb"},
	{80, "http"},
}

const (
	maxIPScanHosts  = 1024
	ipScanWorkers   = 64
	ipProbeTimeout  = 400 * time.Millisecond
	hostnameTimeout = 2 * time.Second
)

// StartIPScan launches an asynchronous liveness scan of [startIP, endIP].
// Returns the scan id used to match events and to stop the scan.
func (m *Manager) StartIPScan(startIP, endIP string) (string, error) {
	lo, err := parseIPv4(startIP)
	if err != nil {
		return "", fmt.Errorf("invalid start IP %q", startIP)
	}
	hi, err := parseIPv4(endIP)
	if err != nil {
		return "", fmt.Errorf("invalid end IP %q", endIP)
	}
	if hi < lo {
		return "", fmt.Errorf("end IP must not be smaller than start IP")
	}
	total := int(hi-lo) + 1
	if total > maxIPScanHosts {
		return "", fmt.Errorf("range too large (max %d addresses)", maxIPScanHosts)
	}

	id, ctx := m.newScan()
	go m.runIPScan(ctx, id, lo, hi, total)
	return id, nil
}

func (m *Manager) runIPScan(ctx context.Context, id string, lo, hi uint32, total int) {
	defer m.finish(id)

	var (
		wg    sync.WaitGroup
		done  atomic.Int32
		mu    sync.Mutex
		hosts []ScanHost
	)
	sem := make(chan struct{}, ipScanWorkers)

	for v := lo; ; v++ {
		if ctx.Err() != nil {
			break
		}
		ip := uint32ToIP(v)
		last := v == hi
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if h, alive := probeHost(ctx, ip); alive {
				mu.Lock()
				hosts = append(hosts, h)
				mu.Unlock()
				// Emit immediately with the data we have; MAC is filled in a
				// second pass below once the ARP cache is read in bulk.
				m.emit(EventHostFound, map[string]any{"scanId": id, "host": h})
			}
			n := done.Add(1)
			if n%8 == 0 || int(n) == total {
				m.emit(EventProgress, map[string]any{"scanId": id, "kind": "ip", "done": n, "total": total})
			}
		}()
		if last {
			break
		}
	}
	wg.Wait()

	// Second pass: enrich hosts with MAC addresses from the system ARP cache.
	// The TCP probes above already triggered ARP resolution for live hosts, so
	// the table is warm by now.
	if ctx.Err() == nil && len(hosts) > 0 {
		if macs := readARPTable(); len(macs) > 0 {
			for _, h := range hosts {
				if mac, ok := macs[h.IP]; ok && mac != "" && h.MAC != mac {
					h.MAC = mac
					// Re-emit with the MAC filled in; the frontend upserts by IP.
					m.emit(EventHostFound, map[string]any{"scanId": id, "host": h})
				}
			}
		}
	}

	reason := "finished"
	if ctx.Err() != nil {
		reason = "stopped"
	}
	m.emit(EventDone, map[string]any{"scanId": id, "kind": "ip", "reason": reason})
}

// probeHost checks the service ports of one host and reports it as alive when
// at least one port answers with a SYN-ACK (open) or RST (closed) — a silent
// timeout is the only "dead" signal, which keeps detection working on Windows
// hosts whose firewall drops ICMP but still answer TCP with RST.
func probeHost(ctx context.Context, ip net.IP) (ScanHost, bool) {
	h := ScanHost{IP: ip.String(), Ports: map[int]bool{}}
	alive := false
	for _, sp := range ServicePorts {
		if ctx.Err() != nil {
			return h, false
		}
		switch probeTCP(ctx, h.IP, sp.Port, ipProbeTimeout) {
		case portOpen:
			h.Ports[sp.Port] = true
			alive = true
		case portClosed:
			alive = true
		}
	}
	if !alive {
		return h, false
	}
	h.Hostname = lookupHostname(ctx, h.IP)
	return h, true
}

// lookupHostname does a best-effort reverse-DNS lookup with a short timeout.
func lookupHostname(ctx context.Context, ip string) string {
	ctx, cancel := context.WithTimeout(ctx, hostnameTimeout)
	defer cancel()
	type result struct{ names []string }
	ch := make(chan result, 1)
	go func() {
		names, _ := net.LookupAddr(ip)
		ch <- result{names}
	}()
	select {
	case <-ctx.Done():
		return ""
	case r := <-ch:
		if len(r.names) == 0 {
			return ""
		}
		return strings.TrimSuffix(r.names[0], ".")
	}
}

type portState int

const (
	portTimeout portState = iota // no answer: filtered / host dead
	portOpen
	portClosed // RST: host alive, port closed
)

// probeTCP dials one TCP port and classifies the outcome. The dial runs in a
// goroutine guarded by an explicit select timeout rather than relying on
// net.Dialer.Timeout alone: on some hosts (e.g. an address that exists but
// silently drops SYN) the underlying connect can outlive the dialer timeout,
// which would tie up a worker slot and a scan's main loop forever. This hard
// bound guarantees a scan always advances and that Stop() is honoured.
func probeTCP(ctx context.Context, ip string, port int, timeout time.Duration) portState {
	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		// Keep a dialer-side Timeout as well as the select bound below: on a
		// silently-dropping host the dial goroutine must not keep a socket and
		// worker slot alive for the full OS TCP timeout (seconds-to-minutes)
		// after the outer select has already given up on it.
		d := net.Dialer{Timeout: timeout}
		conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(ip, strconv.Itoa(port)))
		ch <- result{conn, err}
	}()

	select {
	case <-ctx.Done():
		return portTimeout
	case <-time.After(timeout):
		return portTimeout
	case r := <-ch:
		if r.err == nil {
			r.conn.Close()
			return portOpen
		}
		if errors.Is(r.err, context.Canceled) {
			return portTimeout
		}
		msg := strings.ToLower(r.err.Error())
		// Linux: "connection refused" / "connection reset by peer".
		// Windows: "actively refused" / "forcibly closed".
		if strings.Contains(msg, "refused") || strings.Contains(msg, "reset") ||
			strings.Contains(msg, "forcibly closed") {
			return portClosed
		}
		return portTimeout
	}
}

func parseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(strings.TrimSpace(s)).To4()
	if ip == nil {
		return 0, fmt.Errorf("not an IPv4 address")
	}
	return binary.BigEndian.Uint32(ip), nil
}

func uint32ToIP(v uint32) net.IP {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return net.IP(b[:])
}
