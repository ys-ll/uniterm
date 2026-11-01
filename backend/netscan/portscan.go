package netscan

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// commonPorts is the "known ports" list for the quick scan mode.
var commonPorts = []int{
	21, 22, 23, 25, 53, 80, 110, 135, 139, 143, 443, 445, 993, 995,
	1433, 1521, 3306, 3389, 5432, 5900, 6379, 8080, 8443, 27017,
}

// serviceNames maps well-known ports to service names for display.
var serviceNames = map[int]string{
	21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp", 53: "dns", 80: "http",
	110: "pop3", 135: "epmap", 139: "netbios-ssn", 143: "imap", 443: "https",
	445: "microsoft-ds", 993: "imaps", 995: "pop3s", 1433: "ms-sql-s",
	1521: "oracle", 3306: "mysql", 3389: "rdp", 5432: "postgresql",
	5900: "vnc", 6379: "redis", 8080: "http-proxy", 8443: "https-alt",
	27017: "mongodb",
}

const (
	// DefaultPortScanWorkers is the worker count used when the caller does
	// not pass one. Kept deliberately low: a SYN burst against one host gets
	// rate-limited or dropped by many firewalls, which silently hides open
	// ports (the "IP scan finds it, port scan doesn't" effect).
	DefaultPortScanWorkers = 32
	maxPortScanWorkers     = 512
	// portProbeTimeout tolerates the slow first TCP connect to a target (DNS /
	// ARP resolution, routing table warm-up) so a cold scan isn't lost to timeouts.
	portProbeTimeout  = 1200 * time.Millisecond
	progressInterval  = 200 * time.Millisecond
	progressBatchSize = 512
)

// StartPortScan launches an asynchronous port scan against one host. When
// commonOnly is true only the known-ports list is scanned; otherwise the
// inclusive range [startPort, endPort] (clamped to 1-65535). workers caps the
// concurrent probes (clamped to [1, maxPortScanWorkers]; <=0 uses the default).
func (m *Manager) StartPortScan(host string, commonOnly bool, startPort, endPort, workers int) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("host is required")
	}
	if workers <= 0 {
		workers = DefaultPortScanWorkers
	}
	if workers > maxPortScanWorkers {
		workers = maxPortScanWorkers
	}
	var ports []int
	if commonOnly {
		ports = append([]int(nil), commonPorts...)
	} else {
		if startPort < 1 {
			startPort = 1
		}
		if endPort > 65535 {
			endPort = 65535
		}
		if endPort < startPort {
			return "", fmt.Errorf("invalid port range")
		}
		ports = make([]int, 0, endPort-startPort+1)
		for p := startPort; p <= endPort; p++ {
			ports = append(ports, p)
		}
	}

	id, ctx := m.newScan()
	go m.runPortScan(ctx, id, host, ports, workers)
	return id, nil
}

func (m *Manager) runPortScan(ctx context.Context, id, host string, ports []int, workers int) {
	defer m.finish(id)
	total := len(ports)

	var (
		wg       sync.WaitGroup
		done     atomic.Int32
		emitMu   sync.Mutex
		lastEmit time.Time
		mu       sync.Mutex
		filtered []int // ports that timed out: retried once below
	)
	sem := make(chan struct{}, workers)

	// emitProgress throttles progress events: at most one per
	// progressInterval, unless force is set (final update).
	emitProgress := func(n int32, force bool) {
		emitMu.Lock()
		defer emitMu.Unlock()
		if !force && time.Since(lastEmit) < progressInterval && n%progressBatchSize != 0 {
			return
		}
		lastEmit = time.Now()
		m.emit(EventProgress, map[string]any{"scanId": id, "kind": "port", "done": n, "total": total})
	}

	openPort := func(port int) {
		m.emit(EventPortOpen, map[string]any{
			"scanId":  id,
			"port":    port,
			"service": serviceNames[port],
			"state":   "listening",
		})
	}

scanLoop:
	for _, p := range ports {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		// Acquire a worker slot without blocking forever: if every slot is
		// taken by a probe that is still in flight (its DNS/TCP connect can
		// take the full timeout), this select lets a cancellation through so
		// Stop() is honoured and the done event reaches the frontend. Without
		// it the loop would wedge on a full semaphore until a slow probe
		// finishes, making the scan appear stuck and Stop unresponsive.
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Done()
			break scanLoop
		}
		go func(port int) {
			defer wg.Done()
			defer func() { <-sem }()
			switch probeTCP(ctx, host, port, portProbeTimeout) {
			case portOpen:
				openPort(port)
			case portTimeout:
				mu.Lock()
				filtered = append(filtered, port)
				mu.Unlock()
			}
			emitProgress(done.Add(1), false)
		}(p)
	}
	wg.Wait()

	// Retry pass: firewalls rate-limiting a SYN burst silently drop probes
	// that succeed on a quieter second attempt, so re-probe every filtered
	// port once before giving up (the same idea as nmap's --max-retries 1).
	// The retries are counted in the progress: the total is extended and a
	// checkpoint is emitted first, so the counter keeps moving to 100%
	// instead of freezing at the pre-retry count.
	if ctx.Err() == nil && len(filtered) > 0 {
		total += len(filtered)
		emitProgress(done.Load(), true)
	retryLoop:
		for _, p := range filtered {
			if ctx.Err() != nil {
				break
			}
			wg.Add(1)
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				wg.Done()
				break retryLoop
			}
			go func(port int) {
				defer wg.Done()
				defer func() { <-sem }()
				if probeTCP(ctx, host, port, portProbeTimeout) == portOpen {
					openPort(port)
				}
				emitProgress(done.Add(1), false)
			}(p)
		}
		wg.Wait()
	}
	emitProgress(done.Load(), true)

	reason := "finished"
	if ctx.Err() != nil {
		reason = "stopped"
	}
	m.emit(EventDone, map[string]any{"scanId": id, "kind": "port", "reason": reason})
}
