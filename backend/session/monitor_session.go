package session

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type monitorState struct {
	lastCpuTotal uint64
	lastCpuIdle  uint64
	lastNetRx    uint64
	lastNetTx    uint64
	hasPrev      bool
}

type PortInfo struct {
	Protocol  string `json:"protocol"`
	LocalAddr string `json:"localAddr"`
	State     string `json:"state"`
	Process   string `json:"process"`
}

type DiskInfo struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Size       string `json:"size"`
	MountPoint string `json:"mountPoint"`
	Used       string `json:"used"`
	Total      string `json:"total"`
	Usage      int    `json:"usage"`
	Media      string `json:"media"`
	FSType     string `json:"fsType"`
	UUID       string `json:"uuid"`
	Vendor     string `json:"vendor"`
	Model      string `json:"model"`
}

type NetCardInfo struct {
	Name       string   `json:"name"`
	State      string   `json:"state"`
	MAC        string   `json:"mac"`
	Speed      string   `json:"speed"`
	Type       string   `json:"type"`
	BondMaster string   `json:"bondMaster"`
	BondSlaves []string `json:"bondSlaves"`
	IPAddrs    []string `json:"ipAddrs"`
}

type MonitorSession struct {
	baseSession
	client    *ssh.Client
	config    ConnectionConfig
	ticker    *time.Ticker
	quit      chan struct{}
	quitOnce  sync.Once
	state     monitorState
	activeTab string
	paused    bool
	mu        sync.RWMutex
}

func NewMonitorSession(id string) *MonitorSession {
	return &MonitorSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "monitor",
			status:      StatusDisconnected,
		},
		quit:      make(chan struct{}),
		activeTab: "performance",
	}
}

func (s *MonitorSession) SetActiveTab(tab string) {
	s.mu.Lock()
	s.activeTab = tab
	s.mu.Unlock()
}

func (s *MonitorSession) SetPaused(paused bool) {
	s.mu.Lock()
	s.paused = paused
	s.mu.Unlock()
}

func (s *MonitorSession) IsPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.paused
}

func (s *MonitorSession) ActiveTab() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeTab
}

func (s *MonitorSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	s.config = config
	s.title = fmt.Sprintf("%s@%s", config.User, config.Host)

	authMethods := []ssh.AuthMethod{}
	switch config.AuthType {
	case "password":
		authMethods = append(authMethods, ssh.Password(config.Password))
	case "key":
		key, err := os.ReadFile(config.KeyPath)
		if err != nil {
			s.setStatus(StatusError)
			return fmt.Errorf("read key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			s.setStatus(StatusError)
			return fmt.Errorf("parse key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	case "agent":
		authMethods = append(authMethods, ssh.Password(config.Password))
	}

	clientConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Config: ssh.Config{
			KeyExchanges: sshKeyExchanges(),
		},
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", config.Host, config.Port), clientConfig)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("ssh dial: %w", err)
	}

	s.client = client
	s.setStatus(StatusConnected)

	go s.pushSystemInfo()

	s.ticker = time.NewTicker(1 * time.Second)
	go s.pollLoop()

	return nil
}

func (s *MonitorSession) pushSystemInfo() {
	info := s.collectSystemInfo()
	if info != nil {
		data, _ := json.Marshal(map[string]interface{}{
			"type":   "system",
			"system": info,
		})
		s.emitData(data)
	}
}

func (s *MonitorSession) collectSystemInfo() map[string]interface{} {
	session, err := s.client.NewSession()
	if err != nil {
		return nil
	}
	defer session.Close()

	script := `cat /etc/os-release 2>/dev/null | grep -E '^(PRETTY_NAME|VERSION_ID)=' || true; echo "---"; uname -r; echo "---"; hostname; echo "---"; readlink -f /etc/localtime 2>/dev/null | sed 's|/usr/share/zoneinfo/||' || date +%Z; echo "---"; uname -m; echo "---"; cat /proc/cpuinfo 2>/dev/null | grep 'model name' | head -1 || true; echo "---"; nproc; echo "---"; cat /proc/cpuinfo 2>/dev/null | grep 'cpu MHz' | head -1 || true; echo "---"; awk '/MemTotal:/{print $2}' /proc/meminfo; echo "---"; df -h / 2>/dev/null | awk 'NR==2{print $2}'; echo "---"; ip route get 1.1.1.1 2>/dev/null | grep -oP 'src \K\S+'`
	out, err := session.CombinedOutput(script)
	if err != nil {
		return nil
	}

	parts := strings.Split(string(out), "---")
	osInfo := strings.TrimSpace(safeIndex(parts, 0))
	kernel := strings.TrimSpace(safeIndex(parts, 1))
	hostname := strings.TrimSpace(safeIndex(parts, 2))
	timezone := strings.TrimSpace(safeIndex(parts, 3))
	arch := strings.TrimSpace(safeIndex(parts, 4))
	cpuModel := strings.TrimSpace(safeIndex(parts, 5))
	cpuModel = strings.TrimPrefix(cpuModel, "model name\t:")
	cpuModel = strings.TrimSpace(cpuModel)
	coresStr := strings.TrimSpace(safeIndex(parts, 6))
	cores, _ := strconv.Atoi(coresStr)
	cpuFreq := strings.TrimSpace(safeIndex(parts, 7))
	cpuFreq = strings.TrimPrefix(cpuFreq, "cpu MHz\t:")
	cpuFreq = strings.TrimSpace(cpuFreq)
	memTotalKBStr := strings.TrimSpace(safeIndex(parts, 8))
	memTotalKB, _ := strconv.ParseFloat(memTotalKBStr, 64)
	diskTotal := strings.TrimSpace(safeIndex(parts, 9))
	localIP := strings.TrimSpace(safeIndex(parts, 10))

	osName := "Linux"
	versionID := ""
	for _, line := range strings.Split(osInfo, "\n") {
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			osName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), `"`)
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), `"`)
		}
	}

	return map[string]interface{}{
		"os":        osName,
		"version":   versionID,
		"kernel":    kernel,
		"hostname":  hostname,
		"timezone":  timezone,
		"arch":      arch,
		"cpuModel":  cpuModel,
		"cores":     cores,
		"cpuFreq":   cpuFreq,
		"memTotal":  roundFloat(memTotalKB/1024/1024, 2),
		"diskTotal": diskTotal,
		"localIP":   localIP,
	}
}

func safeIndex(arr []string, idx int) string {
	if idx < len(arr) {
		return arr[idx]
	}
	return ""
}

func (s *MonitorSession) pollLoop() {
	for {
		select {
		case <-s.quit:
			return
		case <-s.ticker.C:
			s.mu.RLock()
			tab := s.activeTab
			paused := s.paused
			s.mu.RUnlock()

			switch tab {
			case "performance":
				s.collectPerformance()
			case "processes":
				if !paused {
					s.collectProcesses()
				}
			}
		}
	}
}

func (s *MonitorSession) collectPerformance() {
	session, err := s.client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()

	script := `exec 2>/dev/null
cpu_line=$(head -1 /proc/stat)
cpu_total=$(echo "$cpu_line" | awk '{print $2+$3+$4+$5+$6+$7+$8+$9}')
cpu_idle=$(echo "$cpu_line" | awk '{print $5+$6}')

mem_total=$(awk '/MemTotal:/{print $2}' /proc/meminfo)
mem_avail=$(awk '/MemAvailable:/{print $2}' /proc/meminfo)
[ -z "$mem_avail" ] && mem_avail=$(awk '/MemFree:/{print $2}' /proc/meminfo)
mem_cached=$(awk '/^Cached:/{print $2}' /proc/meminfo)
[ -z "$mem_cached" ] && mem_cached=0
mem_buffers=$(awk '/^Buffers:/{print $2}' /proc/meminfo)
[ -z "$mem_buffers" ] && mem_buffers=0

disk_total=$(df -h / | awk 'NR==2{print $2}')
disk_used=$(df -h / | awk 'NR==2{print $3}')
disk_usage=$(df -h / | awk 'NR==2{gsub(/%/,""); print $5}')

net_rx=$(awk '/^[ ]*[^ ]+:/ && !/lo:/{print $2; exit}' /proc/net/dev)
net_tx=$(awk '/^[ ]*[^ ]+:/ && !/lo:/{print $10; exit}' /proc/net/dev)
[ -z "$net_rx" ] && net_rx=0
[ -z "$net_tx" ] && net_tx=0

handles=$(awk '{print $1}' /proc/sys/fs/file-nr)
[ -z "$handles" ] && handles=0

proc_count=$(ps -e | wc -l)
proc_count=$((proc_count > 0 ? proc_count - 1 : 0))

cores=$(nproc || echo 1)

loadavg=$(cat /proc/loadavg 2>/dev/null | awk '{print $1,$2,$3}')
[ -z "$loadavg" ] && loadavg="0 0 0"

printf '{"cpu_total":%s,"cpu_idle":%s,"cores":%s,"processes":%s,"mem_total":%s,"mem_avail":%s,"mem_cached":%s,"mem_buffers":%s,"disk_total":"%s","disk_used":"%s","disk_usage":%s,"net_rx":%s,"net_tx":%s,"handles":%s,"load1":%s,"load5":%s,"load15":%s}\n' \
  "$cpu_total" "$cpu_idle" "$cores" "$proc_count" \
  "$mem_total" "$mem_avail" "$mem_cached" "$mem_buffers" \
  "${disk_total:-}" "${disk_used:-}" "${disk_usage:-0}" \
  "$net_rx" "$net_tx" "$handles" \
  $loadavg`

	out, err := session.Output(script)
	if err != nil {
		return
	}

	type rawMetrics struct {
		CpuTotal   uint64  `json:"cpu_total"`
		CpuIdle    uint64  `json:"cpu_idle"`
		Cores      int     `json:"cores"`
		Processes  int     `json:"processes"`
		MemTotal   uint64  `json:"mem_total"`
		MemAvail   uint64  `json:"mem_avail"`
		MemCached  uint64  `json:"mem_cached"`
		MemBuffers uint64  `json:"mem_buffers"`
		DiskTotal  string  `json:"disk_total"`
		DiskUsed   string  `json:"disk_used"`
		DiskUsage  int     `json:"disk_usage"`
		NetRx      uint64  `json:"net_rx"`
		NetTx      uint64  `json:"net_tx"`
		Handles    uint64  `json:"handles"`
		Load1      float64 `json:"load1"`
		Load5      float64 `json:"load5"`
		Load15     float64 `json:"load15"`
	}

	var m rawMetrics
	if err := json.Unmarshal(out, &m); err != nil {
		return
	}

	// CPU usage: need delta between two samples
	cpuUsage := 0.0
	if s.state.hasPrev && m.CpuTotal > s.state.lastCpuTotal {
		totalDiff := m.CpuTotal - s.state.lastCpuTotal
		idleDiff := m.CpuIdle - s.state.lastCpuIdle
		cpuUsage = float64(totalDiff-idleDiff) / float64(totalDiff) * 100
	}

	// Network rate (bytes per second, since we poll every 1s)
	netRxRate := uint64(0)
	netTxRate := uint64(0)
	if s.state.hasPrev {
		if m.NetRx >= s.state.lastNetRx {
			netRxRate = m.NetRx - s.state.lastNetRx
		}
		if m.NetTx >= s.state.lastNetTx {
			netTxRate = m.NetTx - s.state.lastNetTx
		}
	}

	memTotalGB := float64(m.MemTotal) / 1024 / 1024
	memUsedGB := float64(m.MemTotal-m.MemAvail) / 1024 / 1024
	memFreeGB := float64(m.MemAvail) / 1024 / 1024
	memUsage := 0.0
	if m.MemTotal > 0 {
		memUsage = float64(m.MemTotal-m.MemAvail) / float64(m.MemTotal) * 100
	}
	memCachedGB := float64(m.MemCached) / 1024 / 1024
	memBuffersGB := float64(m.MemBuffers) / 1024 / 1024

	payload := map[string]interface{}{
		"type": "performance",
		"cpu": map[string]interface{}{
			"usage":     roundFloat(cpuUsage, 1),
			"cores":     m.Cores,
			"processes": m.Processes,
			"handles":   m.Handles,
			"load1":     roundFloat(m.Load1, 2),
			"load5":     roundFloat(m.Load5, 2),
			"load15":    roundFloat(m.Load15, 2),
		},
		"memory": map[string]interface{}{
			"total":   roundFloat(memTotalGB, 2),
			"used":    roundFloat(memUsedGB, 2),
			"free":    roundFloat(memFreeGB, 2),
			"usage":   roundFloat(memUsage, 1),
			"cached":  roundFloat(memCachedGB, 2),
			"buffers": roundFloat(memBuffersGB, 2),
		},
		"disk": map[string]interface{}{
			"total": m.DiskTotal,
			"used":  m.DiskUsed,
			"usage": m.DiskUsage,
		},
		"network": map[string]interface{}{
			"rx": netRxRate,
			"tx": netTxRate,
		},
	}

	jsonData, _ := json.Marshal(payload)
	s.emitData(jsonData)

	// Save state for next delta calculation
	s.state.lastCpuTotal = m.CpuTotal
	s.state.lastCpuIdle = m.CpuIdle
	s.state.lastNetRx = m.NetRx
	s.state.lastNetTx = m.NetTx
	s.state.hasPrev = true
}

func (s *MonitorSession) collectProcesses() {
	session, err := s.client.NewSession()
	if err != nil {
		return
	}
	defer session.Close()

	script := `exec 2>/dev/null
cpu_line=$(head -1 /proc/stat)
cpu_total=$(echo "$cpu_line" | awk '{print $2+$3+$4+$5+$6+$7+$8+$9}')
cpu_idle=$(echo "$cpu_line" | awk '{print $5+$6}')
mem_total=$(awk '/MemTotal:/{print $2}' /proc/meminfo)
mem_avail=$(awk '/MemAvailable:/{print $2}' /proc/meminfo)
[ -z "$mem_avail" ] && mem_avail=$(awk '/MemFree:/{print $2}' /proc/meminfo)
mem_cached=$(awk '/^Cached:/{print $2}' /proc/meminfo)
[ -z "$mem_cached" ] && mem_cached=0
mem_buffers=$(awk '/^Buffers:/{print $2}' /proc/meminfo)
[ -z "$mem_buffers" ] && mem_buffers=0
loadavg=$(cat /proc/loadavg 2>/dev/null | awk '{print $1,$2,$3}')
[ -z "$loadavg" ] && loadavg="0 0 0"
proc_count=$(ps -e | wc -l)
proc_count=$((proc_count > 0 ? proc_count - 1 : 0))
cores=$(nproc || echo 1)
ps -eo pid,ppid,user,stat,pcpu,pmem,comm,args --sort=-pcpu | tail -n +2 | head -30 || true
echo "---SUMMARY---"
printf '{"cpu_total":%s,"cpu_idle":%s,"cores":%s,"proc_count":%s,"mem_total":%s,"mem_avail":%s,"mem_cached":%s,"mem_buffers":%s,"load1":%s,"load5":%s,"load15":%s}\n' \
  "$cpu_total" "$cpu_idle" "$cores" "$proc_count" \
  "$mem_total" "$mem_avail" "$mem_cached" "$mem_buffers" \
  $loadavg`

	out, err := session.Output(script)
	if err != nil {
		return
	}

	output := string(out)
	parts := strings.Split(output, "---SUMMARY---\n")
	procPart := strings.TrimSpace(parts[0])

	// Parse summary
	summary := map[string]interface{}{}
	if len(parts) > 1 {
		summaryJSON := strings.TrimSpace(parts[1])
		var rawSummary struct {
			CpuTotal   uint64  `json:"cpu_total"`
			CpuIdle    uint64  `json:"cpu_idle"`
			Cores      int     `json:"cores"`
			ProcCount  int     `json:"proc_count"`
			MemTotal   uint64  `json:"mem_total"`
			MemAvail   uint64  `json:"mem_avail"`
			MemCached  uint64  `json:"mem_cached"`
			MemBuffers uint64  `json:"mem_buffers"`
			Load1      float64 `json:"load1"`
			Load5      float64 `json:"load5"`
			Load15     float64 `json:"load15"`
		}
		if err := json.Unmarshal([]byte(summaryJSON), &rawSummary); err == nil {
			// CPU usage: need delta between two samples
			cpuUsage := 0.0
			if s.state.hasPrev && rawSummary.CpuTotal > s.state.lastCpuTotal {
				totalDiff := rawSummary.CpuTotal - s.state.lastCpuTotal
				idleDiff := rawSummary.CpuIdle - s.state.lastCpuIdle
				cpuUsage = float64(totalDiff-idleDiff) / float64(totalDiff) * 100
			}

			memTotalGB := float64(rawSummary.MemTotal) / 1024 / 1024
			memUsedGB := float64(rawSummary.MemTotal-rawSummary.MemAvail) / 1024 / 1024
			memFreeGB := float64(rawSummary.MemAvail) / 1024 / 1024
			memUsage := 0.0
			if rawSummary.MemTotal > 0 {
				memUsage = float64(rawSummary.MemTotal-rawSummary.MemAvail) / float64(rawSummary.MemTotal) * 100
			}

			summary["cpu"] = map[string]interface{}{
				"usage":     roundFloat(cpuUsage, 1),
				"cores":     rawSummary.Cores,
				"processes": rawSummary.ProcCount,
				"load1":     roundFloat(rawSummary.Load1, 2),
				"load5":     roundFloat(rawSummary.Load5, 2),
				"load15":    roundFloat(rawSummary.Load15, 2),
			}
			summary["memory"] = map[string]interface{}{
				"total":   roundFloat(memTotalGB, 2),
				"used":    roundFloat(memUsedGB, 2),
				"free":    roundFloat(memFreeGB, 2),
				"usage":   roundFloat(memUsage, 1),
				"cached":  roundFloat(float64(rawSummary.MemCached)/1024/1024, 2),
				"buffers": roundFloat(float64(rawSummary.MemBuffers)/1024/1024, 2),
			}

			// Save state for next delta calculation
			s.state.lastCpuTotal = rawSummary.CpuTotal
			s.state.lastCpuIdle = rawSummary.CpuIdle
			s.state.hasPrev = true
		}
	}

	// Parse processes
	processes := []map[string]interface{}{}
	for _, line := range strings.Split(procPart, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}
		pid, _ := strconv.Atoi(parts[0])
		ppid, _ := strconv.Atoi(parts[1])
		cpu, _ := strconv.ParseFloat(parts[4], 64)
		mem, _ := strconv.ParseFloat(parts[5], 64)
		processes = append(processes, map[string]interface{}{
			"pid":   pid,
			"ppid":  ppid,
			"user":  parts[2],
			"state": parts[3],
			"cpu":   cpu,
			"mem":   mem,
			"name":  parts[6],
			"cmd":   strings.Join(parts[7:], " "),
		})
	}

	payload := map[string]interface{}{
		"type":      "processes",
		"processes": processes,
	}
	if len(summary) > 0 {
		payload["summary"] = summary
	}

	jsonData, _ := json.Marshal(payload)
	s.emitData(jsonData)
}

func (s *MonitorSession) GetProcessDetail(pid int) (map[string]interface{}, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	script := fmt.Sprintf(`exec 2>/dev/null
PID=%d

# Basic info from status
cat /proc/$PID/status 2>/dev/null | grep -E '^(Pid|PPid|Name|State|Threads|VmRSS|VmSize|VmPeak|VmData|VmStk|VmExe|VmLib|voluntary_ctxt_switches|nonvoluntary_ctxt_switches):' || true
echo "---EXE---"
readlink -f /proc/$PID/exe 2>/dev/null || echo '-'
echo "---CWD---"
readlink -f /proc/$PID/cwd 2>/dev/null || echo '-'
echo "---CMDLINE---"
cat /proc/$PID/cmdline 2>/dev/null | tr '\0' ' '
echo "---FD---"
total=0; files=0; sockets=0; pipes=0; anons=0; devs=0; others=0
for fd in /proc/$PID/fd/[0-9]*; do
    [ -L "$fd" ] || continue
    target=$(readlink "$fd" 2>/dev/null)
    total=$((total + 1))
    case "$target" in
        socket:*) sockets=$((sockets + 1)) ;;
        pipe:*) pipes=$((pipes + 1)) ;;
        anon_inode:*) anons=$((anons + 1)) ;;
        /dev/*) devs=$((devs + 1)) ;;
        /*) files=$((files + 1)) ;;
        *) others=$((others + 1)) ;;
    esac
done
printf '{"total":%%d,"files":%%d,"sockets":%%d,"pipes":%%d,"anons":%%d,"devs":%%d,"others":%%d}\n' $total $files $sockets $pipes $anons $devs $others
echo "---IO---"
cat /proc/$PID/io 2>/dev/null | grep -E '^(rchar|wchar|syscr|syscw|read_bytes|write_bytes):' || true
echo "---CPU---"
awk '{print $14+$15}' /proc/$PID/stat 2>/dev/null || echo '0'
echo "---STARTTIME---"
start_epoch=$(stat -c %%Y /proc/$PID 2>/dev/null)
date -d "@$start_epoch" "+%%Y-%%m-%%d %%H:%%M:%%S" 2>/dev/null || echo '-'`, pid)

	out, err := session.Output(script)
	if err != nil {
		return nil, fmt.Errorf("query process detail: %w", err)
	}

	output := string(out)
	sections := strings.Split(output, "---EXE---\n")
	statusPart := strings.TrimSpace(sections[0])
	rest := ""
	if len(sections) > 1 {
		rest = sections[1]
	}

	result := map[string]interface{}{
		"pid": pid,
	}

	// Parse status
	for _, line := range strings.Split(statusPart, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Pid":
			result["pid"], _ = strconv.Atoi(val)
		case "PPid":
			result["ppid"], _ = strconv.Atoi(val)
		case "Name":
			result["name"] = val
		case "State":
			result["state"] = val
		case "Threads":
			result["threads"], _ = strconv.Atoi(val)
		case "VmRSS":
			result["vmRss"] = val
		case "VmSize":
			result["vmSize"] = val
		case "VmPeak":
			result["vmPeak"] = val
		case "VmData":
			result["vmData"] = val
		case "VmStk":
			result["vmStk"] = val
		case "VmExe":
			result["vmExe"] = val
		case "VmLib":
			result["vmLib"] = val
		case "voluntary_ctxt_switches":
			result["voluntaryCtxSwitches"], _ = strconv.Atoi(val)
		case "nonvoluntary_ctxt_switches":
			result["nonvoluntaryCtxSwitches"], _ = strconv.Atoi(val)
		}
	}

	// Parse rest sections
	if rest != "" {
		sections2 := strings.Split(rest, "---CWD---\n")
		result["exe"] = strings.TrimSpace(strings.Split(sections2[0], "---CMDLINE---")[0])

		if len(sections2) > 1 {
			sections3 := strings.Split(sections2[1], "---CMDLINE---\n")
			result["cwd"] = strings.TrimSpace(sections3[0])

			if len(sections3) > 1 {
				sections4 := strings.Split(sections3[1], "---FD---\n")
				result["cmdline"] = strings.TrimSpace(sections4[0])

				if len(sections4) > 1 {
					sections5 := strings.Split(sections4[1], "---IO---\n")
					fdJSON := strings.TrimSpace(sections5[0])
					var fdStats map[string]interface{}
					_ = json.Unmarshal([]byte(fdJSON), &fdStats)
					result["fd"] = fdStats

					if len(sections5) > 1 {
						sections6 := strings.Split(sections5[1], "---CPU---\n")
						ioPart := strings.TrimSpace(sections6[0])
						io := map[string]interface{}{}
						for _, line := range strings.Split(ioPart, "\n") {
							line = strings.TrimSpace(line)
							if line == "" {
								continue
							}
							parts := strings.SplitN(line, ":", 2)
							if len(parts) == 2 {
								io[strings.TrimSpace(parts[0])], _ = strconv.ParseUint(strings.TrimSpace(parts[1]), 10, 64)
							}
						}
						result["io"] = io

						if len(sections6) > 1 {
							sections7 := strings.Split(sections6[1], "---STARTTIME---\n")
							cpuTicksStr := strings.TrimSpace(sections7[0])
							result["cpuTicks"], _ = strconv.ParseUint(cpuTicksStr, 10, 64)

							if len(sections7) > 1 {
								result["startTime"] = strings.TrimSpace(sections7[1])
							}
						}
					}
				}
			}
		}
	}

	return result, nil
}
func (s *MonitorSession) KillProcess(pid int, signal string) error {
	session, err := s.client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	var cmd string
	switch signal {
	case "KILL":
		cmd = fmt.Sprintf("kill -9 %d", pid)
	case "TERM":
		cmd = fmt.Sprintf("kill -15 %d", pid)
	case "HUP":
		cmd = fmt.Sprintf("kill -1 %d", pid)
	case "INT":
		cmd = fmt.Sprintf("kill -2 %d", pid)
	default:
		cmd = fmt.Sprintf("kill %d", pid)
	}

	return session.Run(cmd)
}

// parsePortProcess converts ss output like users:(("nginx",pid=1001,fd=6),("nginx",pid=1002,fd=6))
// into "1001/nginx, 1002/nginx"
func parsePortProcess(raw string) string {
	if raw == "" || raw == "-" {
		return "-"
	}
	// Match "name",pid=123
	re := regexp.MustCompile(`"([^"]+)",pid=(\d+)`)
	matches := re.FindAllStringSubmatch(raw, -1)
	if len(matches) == 0 {
		return raw
	}
	seen := map[string]bool{}
	var parts []string
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		name := m[1]
		pid := m[2]
		key := pid + "/" + name
		if !seen[key] {
			seen[key] = true
			parts = append(parts, pid+"/"+name)
		}
	}
	if len(parts) == 0 {
		return raw
	}
	return strings.Join(parts, ", ")
}

// collectMounts recursively gathers all unique non-empty mountpoints from lsblk JSON devices.
func collectMounts(devs []map[string]interface{}) []string {
	seen := map[string]bool{}
	var mounts []string
	var walk func([]map[string]interface{})
	walk = func(ds []map[string]interface{}) {
		for _, dev := range ds {
			if mp, ok := dev["mountpoint"].(string); ok && mp != "" && !seen[mp] {
				seen[mp] = true
				mounts = append(mounts, mp)
			}
			if children, ok := dev["children"].([]interface{}); ok {
				childMaps := make([]map[string]interface{}, 0, len(children))
				for _, c := range children {
					if cm, ok := c.(map[string]interface{}); ok {
						childMaps = append(childMaps, cm)
					}
				}
				walk(childMaps)
			}
		}
	}
	walk(devs)
	return mounts
}

func (s *MonitorSession) GetPorts() ([]PortInfo, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	script := `exec 2>/dev/null
if command -v ss >/dev/null 2>&1; then
    ss -tulnp | tail -n +2
else
    netstat -tulnp 2>/dev/null | tail -n +2
fi`
	out, err := session.Output(script)
	if err != nil {
		return nil, err
	}

	var ports []PortInfo
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		var protocol, localAddr, state, process string
		if fields[0] == "tcp" || fields[0] == "udp" || fields[0] == "tcp6" || fields[0] == "udp6" {
			protocol = fields[0]
			state = fields[1]
			if len(fields) >= 6 {
				localAddr = fields[4]
				if len(fields) >= 7 {
					process = parsePortProcess(fields[6])
				}
			}
		} else {
			state = fields[0]
			if len(fields) >= 4 {
				localAddr = fields[3]
			}
			if len(fields) >= 5 {
				processField := fields[len(fields)-1]
				if idx := strings.Index(processField, `"`); idx >= 0 {
					endIdx := strings.Index(processField[idx+1:], `"`)
					if endIdx >= 0 {
						process = processField[idx+1 : idx+1+endIdx]
					}
				} else {
					process = processField
				}
			}
			// Infer protocol from state and address
			isUDP := state == "UNCONN"
			if strings.Contains(localAddr, ":") && !strings.Contains(localAddr, ".") && !strings.HasPrefix(localAddr, "[::]") {
				if isUDP {
					protocol = "udp6"
				} else {
					protocol = "tcp6"
				}
			} else if strings.HasPrefix(localAddr, "[::]") {
				if isUDP {
					protocol = "udp6"
				} else {
					protocol = "tcp6"
				}
			} else {
				if isUDP {
					protocol = "udp"
				} else {
					protocol = "tcp"
				}
			}
		}
		if localAddr == "" {
			continue
		}
		ports = append(ports, PortInfo{
			Protocol:  protocol,
			LocalAddr: localAddr,
			State:     state,
			Process:   process,
		})
	}
	return ports, nil
}

func (s *MonitorSession) GetDisks() ([]DiskInfo, error) {
	// Run lsblk and df in a single shell script to avoid extra SSH round-trips.
	// df only queries paths that actually have a mountpoint.
	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	script := `lsblk -J -b -o NAME,SIZE,TYPE,MOUNTPOINT,MODEL,ROTA,FSTYPE,UUID,VENDOR 2>/dev/null
echo "__SPLIT__"
mp=$(lsblk -n -o MOUNTPOINT 2>/dev/null | grep -v '^$' | sort -u | tr '\n' ' ')
[ -n "$mp" ] && df -h $mp 2>/dev/null`
	out, err := session.Output(script)
	session.Close()

	parts := strings.Split(string(out), "__SPLIT__")
	var jsonOut, dfOut []byte
	if len(parts) > 0 {
		jsonOut = []byte(strings.TrimSpace(parts[0]))
	}
	if len(parts) > 1 {
		dfOut = []byte(strings.TrimSpace(parts[1]))
	}

	mountUsage := map[string]struct{ Used, Total string; Usage int }{}
	for _, line := range strings.Split(string(dfOut), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Filesystem") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 6 {
			mount := fields[5]
			usageStr := strings.TrimSuffix(fields[4], "%")
			usage, _ := strconv.Atoi(usageStr)
			mountUsage[mount] = struct{ Used, Total string; Usage int }{
				Used:  fields[2],
				Total: fields[1],
				Usage: usage,
			}
		}
	}

	var disks []DiskInfo

	// Parse JSON output
	var lsblkJSON struct {
		BlockDevices []map[string]interface{} `json:"blockdevices"`
	}
	if err == nil && json.Unmarshal(jsonOut, &lsblkJSON) == nil {
		var walk func(devs []map[string]interface{}, depth int)
		walk = func(devs []map[string]interface{}, depth int) {
				for _, dev := range devs {
					name, _ := dev["name"].(string)
					devType, _ := dev["type"].(string)

					var sizeBytes uint64
					switch s := dev["size"].(type) {
					case float64:
						sizeBytes = uint64(s)
					case string:
						sizeBytes, _ = strconv.ParseUint(s, 10, 64)
					}

					var mount string
					switch mp := dev["mountpoint"].(type) {
					case string:
						if mp != "" {
							mount = mp
						}
					}

					var model string
					switch m := dev["model"].(type) {
					case string:
						if m != "" {
							model = m
						}
					}

					media := "-"
					if devType == "rom" {
						media = "ROM"
					} else {
						switch r := dev["rota"].(type) {
						case bool:
							if r {
								media = "HDD"
							} else {
								media = "SSD"
							}
						case string:
							if r == "1" || r == "true" {
								media = "HDD"
							} else if r == "0" || r == "false" {
								media = "SSD"
							}
						case float64:
							if r != 0 {
								media = "HDD"
							} else {
								media = "SSD"
							}
						}
					}

					var fsType, uuid, vendor string
					if v, ok := dev["fstype"].(string); ok && v != "" {
						fsType = v
					}
					if v, ok := dev["uuid"].(string); ok && v != "" {
						uuid = v
					}
					if v, ok := dev["vendor"].(string); ok && v != "" {
						vendor = v
					}

					usage := mountUsage[mount]
					disk := DiskInfo{
						Name:       strings.Repeat("  ", depth) + name,
						Type:       devType,
						Size:       formatBytes(sizeBytes),
						MountPoint: mount,
						Media:      media,
						FSType:     fsType,
						UUID:       uuid,
						Vendor:     vendor,
						Model:      model,
					}
					if mount != "" {
						disk.Used = usage.Used
						disk.Total = usage.Total
						disk.Usage = usage.Usage
					}
					disks = append(disks, disk)

					if children, ok := dev["children"].([]interface{}); ok {
						childMaps := make([]map[string]interface{}, 0, len(children))
						for _, c := range children {
							if cm, ok := c.(map[string]interface{}); ok {
								childMaps = append(childMaps, cm)
							}
						}
						walk(childMaps, depth+1)
					}
				}
			}
			walk(lsblkJSON.BlockDevices, 0)
			return disks, nil
		}

	// Fallback to text parsing
	session2, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session2.Close()
	out2, err := session2.Output(`lsblk -b -o NAME,SIZE,TYPE,MOUNTPOINT,MODEL,ROTA 2>/dev/null || lsblk -o NAME,SIZE,TYPE,MOUNTPOINT,MODEL,ROTA 2>/dev/null`)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out2), "\n")
	var headers []string
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if i == 0 {
			headers = fields
			continue
		}
		rawName := fields[0]
		depth := 0
		trimmedName := rawName
		for strings.HasPrefix(trimmedName, "|") || strings.HasPrefix(trimmedName, "-") || strings.HasPrefix(trimmedName, "\\") || strings.HasPrefix(trimmedName, " ") {
			if strings.HasPrefix(trimmedName, " ") {
				trimmedName = strings.TrimPrefix(trimmedName, " ")
			} else {
				trimmedName = trimmedName[1:]
				if strings.HasPrefix(trimmedName, "-") {
					trimmedName = trimmedName[1:]
				}
			}
			depth++
		}
		if len(fields) > 0 {
			fields[0] = trimmedName
		}
		colMap := map[string]string{}
		for j, h := range headers {
			if j < len(fields) {
				colMap[h] = fields[j]
			} else {
				colMap[h] = ""
			}
		}
		sizeBytes, _ := strconv.ParseUint(colMap["SIZE"], 10, 64)
		sizeStr := formatBytes(sizeBytes)
		media := "-"
		if colMap["TYPE"] == "rom" {
			media = "ROM"
		} else if colMap["ROTA"] == "1" {
			media = "HDD"
		} else if colMap["ROTA"] == "0" {
			media = "SSD"
		}
		mount := colMap["MOUNTPOINT"]
		usage := mountUsage[mount]
		disk := DiskInfo{
			Name:       strings.Repeat("  ", depth) + colMap["NAME"],
			Type:       colMap["TYPE"],
			Size:       sizeStr,
			Model:      colMap["MODEL"],
			MountPoint: mount,
			Media:      media,
		}
		if mount != "" {
			disk.Used = usage.Used
			disk.Total = usage.Total
			disk.Usage = usage.Usage
		}
		disks = append(disks, disk)
	}
	return disks, nil
}

func (s *MonitorSession) GetNetworkCards() ([]NetCardInfo, error) {
	session, err := s.client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()

	script := `exec 2>/dev/null
if ip -j link show >/dev/null 2>&1; then
    echo "---LINKJSON---"
    ip -j link show
else
    echo "---LINKTEXT---"
    ip link show
fi
echo "---ADDRJSON---"
ip -j addr show 2>/dev/null || echo "[]"
echo "---BOND---"
for f in /proc/net/bonding/*; do
    [ -f "$f" ] || continue
    echo "---BONDNAME---$(basename "$f")"
    cat "$f"
done
echo "---SPEED---"
for iface in $(ls /sys/class/net/ 2>/dev/null); do
    val=$(cat /sys/class/net/$iface/speed 2>/dev/null || echo -1)
    echo "$iface $val"
done
echo "---KIND---"
for d in /sys/class/net/*; do
    iface=$(basename "$d")
    [ "$iface" = "lo" ] && continue
    is_virt=0
    readlink -f "$d" | grep -q '/devices/virtual/net/' && is_virt=1
    has_dev=0; [ -L "$d/device" ] && has_dev=1
    if [ "$is_virt" -eq 0 ] && [ "$has_dev" -eq 1 ]; then
        kind=Physical
    elif [ "$is_virt" -eq 1 ] && [ -d "$d/bridge" ]; then
        kind=Bridge
    elif [ -f "$d/bonding/mode" ]; then
        kind=Bond
    else
        kind=Virtual
    fi
    echo "$iface $kind"
done`

	out, err := session.Output(script)
	if err != nil {
		return nil, err
	}

	output := string(out)
	sections := strings.Split(output, "---LINKJSON---\n")
	if len(sections) == 1 {
		sections = strings.Split(output, "---LINKTEXT---\n")
	}
	if len(sections) < 2 {
		return []NetCardInfo{}, nil
	}

	linkPart := strings.TrimSpace(sections[1])
	rest := ""
	if idx := strings.Index(linkPart, "---ADDRJSON---"); idx >= 0 {
		rest = linkPart[idx:]
		linkPart = strings.TrimSpace(linkPart[:idx])
	}

	// Parse bond info
	bondMasters := map[string][]string{}
	bondSlaves := map[string]string{}
	if rest != "" {
		bondSections := strings.Split(rest, "---BOND---\n")
		if len(bondSections) > 1 {
			for _, bs := range strings.Split(bondSections[1], "---BONDNAME---") {
				bs = strings.TrimSpace(bs)
				if bs == "" {
					continue
				}
				lines := strings.SplitN(bs, "\n", 2)
				bondName := strings.TrimSpace(lines[0])
				var slaves []string
				if len(lines) > 1 {
					for _, line := range strings.Split(lines[1], "\n") {
						line = strings.TrimSpace(line)
						if strings.HasPrefix(line, "Slave Interface:") {
							slave := strings.TrimSpace(strings.TrimPrefix(line, "Slave Interface:"))
							slaves = append(slaves, slave)
							bondSlaves[slave] = bondName
						}
					}
				}
				bondMasters[bondName] = slaves
			}
		}
	}

	// Parse addresses
	var addrData []map[string]interface{}
	if rest != "" {
		addrSections := strings.Split(rest, "---ADDRJSON---\n")
		if len(addrSections) > 1 {
			addrPart := strings.TrimSpace(addrSections[1])
			if idx := strings.Index(addrPart, "---BOND---"); idx >= 0 {
				addrPart = strings.TrimSpace(addrPart[:idx])
			}
			_ = json.Unmarshal([]byte(addrPart), &addrData)
		}
	}
	addrMap := map[string][]string{}
	for _, iface := range addrData {
		ifName, _ := iface["ifname"].(string)
		if ifName == "" {
			continue
		}
		addrs, _ := iface["addr_info"].([]interface{})
		for _, a := range addrs {
			ai, _ := a.(map[string]interface{})
			if ai == nil {
				continue
			}
			ip, _ := ai["local"].(string)
			if ip != "" {
				addrMap[ifName] = append(addrMap[ifName], ip)
			}
		}
	}

	// Parse speed info
	speedMap := map[string]int{}
	if rest != "" {
		if idx := strings.Index(rest, "---SPEED---"); idx >= 0 {
			for _, line := range strings.Split(strings.TrimSpace(rest[idx+11:]), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					if val, err := strconv.Atoi(fields[1]); err == nil {
						speedMap[fields[0]] = val
					}
				}
			}
		}
	}

	// Parse kind info
	kindMap := map[string]string{}
	if rest != "" {
		if idx := strings.Index(rest, "---KIND---"); idx >= 0 {
			for _, line := range strings.Split(strings.TrimSpace(rest[idx+10:]), "\n") {
				fields := strings.Fields(line)
				if len(fields) == 2 {
					kindMap[fields[0]] = fields[1]
				}
			}
		}
	}

	// Parse link info
	var cards []NetCardInfo
	if strings.HasPrefix(linkPart, "[") {
		var linkData []map[string]interface{}
		if err := json.Unmarshal([]byte(linkPart), &linkData); err == nil {
			for _, iface := range linkData {
				name, _ := iface["ifname"].(string)
				if name == "" {
					continue
				}
				state, _ := iface["operstate"].(string)
				mac, _ := iface["address"].(string)
				linkType, _ := iface["link_type"].(string)
				speed := "-"
				if speedVal, ok := speedMap[name]; ok && speedVal > 0 {
					speed = fmt.Sprintf("%d Mbps", speedVal)
				}
				ifType := kindMap[name]
				if ifType == "" {
					if linkType == "loopback" {
						ifType = "Loopback"
					} else {
						ifType = "Virtual"
					}
				}
				card := NetCardInfo{
					Name:       name,
					State:      state,
					MAC:        mac,
					Speed:      speed,
					Type:       ifType,
					BondMaster: bondSlaves[name],
					BondSlaves: bondMasters[name],
					IPAddrs:    addrMap[name],
				}
				cards = append(cards, card)
			}
		}
	} else {
		// Text format fallback
		var currentCard *NetCardInfo
		for _, line := range strings.Split(linkPart, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, " ") {
				if currentCard != nil {
					cards = append(cards, *currentCard)
				}
				parts := strings.SplitN(line, ":", 3)
				if len(parts) >= 2 {
					name := strings.TrimSpace(parts[1])
					ifType := kindMap[name]
					if ifType == "" {
						if strings.Contains(line, "loopback") {
							ifType = "Loopback"
						} else {
							ifType = "Virtual"
						}
					}
					currentCard = &NetCardInfo{
						Name:    name,
						State:   "UNKNOWN",
						Speed:   "-",
						Type:    ifType,
						IPAddrs: addrMap[name],
					}
					if len(parts) >= 3 && strings.Contains(parts[2], "UP") {
						currentCard.State = "UP"
					}
					currentCard.BondMaster = bondSlaves[name]
					currentCard.BondSlaves = bondMasters[name]
				}
			} else if currentCard != nil {
				if strings.Contains(line, "link/ether") {
					fields := strings.Fields(line)
					if len(fields) >= 2 {
						currentCard.MAC = fields[1]
					}
				}
				if strings.Contains(line, "state ") {
					stateIdx := strings.Index(line, "state ")
					if stateIdx >= 0 {
						stateVal := strings.Fields(line[stateIdx+6:])
						if len(stateVal) > 0 {
							currentCard.State = strings.ToUpper(stateVal[0])
						}
					}
				}
			}
		}
		if currentCard != nil {
			cards = append(cards, *currentCard)
		}
	}
	return cards, nil
}

func roundFloat(v float64, prec int) float64 {
	p := math.Pow(10, float64(prec))
	return math.Round(v*p) / p
}

func formatBytes(bytes uint64) string {
	if bytes == 0 {
		return "0 B"
	}
	const k = 1024
	sizes := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	f := float64(bytes)
	for f >= k && i < len(sizes)-1 {
		f /= k
		i++
	}
	return fmt.Sprintf("%.1f %s", f, sizes[i])
}

func (s *MonitorSession) Write(data []byte) error {
	return nil
}

func (s *MonitorSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
	if s.ticker != nil {
		s.ticker.Stop()
	}
	if s.client != nil {
		s.client.Close()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *MonitorSession) Resize(cols, rows int) error {
	return nil
}

func (s *MonitorSession) IsConnected() bool {
	return s.Status() == StatusConnected
}
