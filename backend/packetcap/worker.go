package packetcap

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// WorkerArgPrefix is the argv prefix that selects the elevated capture worker
// mode (mirrors the session package's --local-pty-broker pattern): the app
// runs at normal (unelevated) integrity, and when a capture is started it
// re-launches itself with the "runas" verb plus this flag so the raw socket +
// SIO_RCVALL work runs in a High-integrity (administrator) child process.
//
// The worker does NOT build the GUI — it connects back over a named pipe
// restricted to the current user and streams newline-delimited JSON Packet
// objects, which the parent turns into Wails events for the frontend. This is
// exactly how MobaXterm stays "silent": the user's system runs with
// ConsentPromptBehaviorAdmin = 0 so the runas elevation completes without a
// UAC prompt.
const WorkerArgPrefix = "--packetcap-worker"

// workerHello is the first line a worker writes once its pipe connection is
// up. It doubles as the connect synchronization point (see captureViaWorker in
// worker_windows.go) and lets the parent distinguish "worker crashed" from
// "worker still connecting".
const workerHello = "packetcap:hello"

// ServeWorker is the early branch in main() (next to the local-pty-broker
// branch). It parses the worker argv, and when this process was re-launched
// elevated to do a capture, it runs the worker loop (does not build the GUI)
// and returns true so main returns before creating the application.
func ServeWorker(args []string) bool {
	// Linux and macOS run one resident elevated daemon for the whole app
	// lifetime so the admin password is only ever prompted once (see
	// daemon_linux.go / daemon_darwin.go). On other platforms these hooks are
	// no-ops and return false.
	if serverLinuxDaemon(args) {
		return true
	}
	if serverDarwinDaemon(args) {
		return true
	}

	pipeName := ""
	ifaceIP := ""
	filter := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case WorkerArgPrefix:
			if i+1 < len(args) {
				pipeName = args[i+1]
			}
		case WorkerFlagIface:
			if i+1 < len(args) {
				ifaceIP = args[i+1]
			}
		case WorkerFlagFilter:
			if i+1 < len(args) {
				filter = args[i+1]
			}
		}
	}
	if pipeName == "" {
		return false
	}

	// The worker is always spawned by the parent via "runas", so it runs
	// elevated — the raw socket open will succeed. We just stream results back
	// to the parent over the pipe.
	if err := runWorker(pipeName, ifaceIP, filter); err != nil {
		os.Stderr.WriteString("packetcap worker: " + err.Error() + "\n")
	}
	return true
}

// WorkerFlagIface / WorkerFlagFilter carry the capture config into the worker
// through argv (the pipe handshake stays a pure byte stream).
const (
	WorkerFlagIface  = "--packetcap-iface"
	WorkerFlagFilter = "--packetcap-filter"
)

// runWorker connects to the parent-created pipe and streams parsed packets as
// newline-delimited JSON until the pipe closes (user Stop) or the capture ends
// itself (packet limit / stream error).
func runWorker(pipeName, ifaceIP, filter string) error {
	if ifaceIP == "" {
		return errors.New("missing interface IP")
	}
	f, err := connectWorkerPipe(pipeName)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.WriteString(f, workerHello+"\n"); err != nil {
		return err
	}

	cfg := CaptureConfig{InterfaceIP: ifaceIP, Filter: filter, MaxPackets: 3000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := bufio.NewWriter(f)
	enc := json.NewEncoder(w)

	// 并发保护：onFrame（capture 主 goroutine）与下方的 flushTicker goroutine
	// 都会调用 w.Flush()，用互斥锁串行化对 bufio.Writer 的访问，避免数据竞争。
	var writeMu sync.Mutex

	// Frames come raw from the platform capture; we parse them here and stream
	// the resulting Packet as one JSON line and flush so each packet reaches
	// the parent (fields are strings/ints/hex, so Encode never emits a bare
	// newline). 逐包 Flush 是 Windows 高流量下 UI 卡死/自动中断的大头：大多数
	// 网卡驱动一次把大量帧交给 recv，逐包刷新管道会瞬间产生上千次小写、把父
	// 进程事件通道和前端渲染挤爆。这里改为“每 20 包或每 50ms”批量 flush ——
	// 既保持近实时，又把写管道频率降一个数量级。写失败容错（consecWrite）保留。
	flushedAt := time.Now()
	packetCount := 0
	var firstWriteErr time.Time
	flushTicker := time.NewTicker(50 * time.Millisecond)
	defer flushTicker.Stop()
	flush := func() bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		if err := w.Flush(); err != nil {
			// 关键：管道“瞬时写满”是可恢复的积压，绝不是父进程关闭。无线网卡
			// （SIO_RCVALL）收到的原始 IP 包量远超有线，瞬时突发会把 worker→
			// 父进程命名管道写满；若像旧逻辑那样“连续 200 次失败就 cancel”，
			// 就会把整个抓包误判为中断——这正是“抓有线正常、抓无线就时不时
			// 自动中断”的根因。这里只把“管道确实关闭”的永久错误视为致命，
			// 其余临时写满一律退避重试，绝不中止抓包。
			if isFatalPipeError(err) {
				return false
			}
			if firstWriteErr.IsZero() {
				firstWriteErr = time.Now()
			}
			// 保险：若持续很久都无法写入（父进程真的停止消费 / 已退出），
			// 才中止，避免 worker 无限泄漏。有流量空档时 firstWriteErr 会清零。
			if time.Since(firstWriteErr) > 15*time.Second {
				return false
			}
		} else {
			firstWriteErr = time.Time{}
		}
		return true
	}
	onFrame := func(frame []byte) {
		// 兜底：解码阶段 panic 不摧毁 worker 进程，避免畸形帧导致抓包会话被杀。
		defer func() {
			if r := recover(); r != nil {
				os.Stderr.WriteString("packetcap worker: 解码帧时恢复 panic: " + fmt.Sprintf("%v\n", r))
			}
		}()
		pkt := ParseFrame(frame, time.Now())
		if pkt == nil {
			return
		}
		writeMu.Lock()
		err := enc.Encode(pkt)
		if err == nil && !firstWriteErr.IsZero() && time.Since(firstWriteErr) > time.Second {
			// 恢复写入后清掉积压计时，避免误累计。
			firstWriteErr = time.Time{}
		}
		writeMu.Unlock()
		if err != nil {
			// Encode 触发底层写入失败：同样区分永久/临时，临时写满则退避继续，
			// 并让 flushTicker 在空档期恢复。永久错误才中止。
			writeMu.Lock()
			fatal := isFatalPipeError(err)
			progressed := !firstWriteErr.IsZero()
			if !progressed {
				firstWriteErr = time.Now()
			}
			tooLong := time.Since(firstWriteErr) > 15*time.Second
			writeMu.Unlock()
			if fatal || tooLong {
				cancel()
				return
			}
			return
		}
		packetCount++
		if packetCount >= 20 || time.Since(flushedAt) >= 50*time.Millisecond {
			packetCount = 0
			flushedAt = time.Now()
			if !flush() {
				cancel()
			}
		}
	}
	// 主循环：把时间驱动和包驱动的 flush 汇总到这个 goroutine 里做，避免并发写。
	go func() {
		for range flushTicker.C {
			if !flush() {
				cancel()
				return
			}
		}
	}()
	err = platformCapture(ctx, cfg, onFrame)
	flush()
	return err
}

// isFatalPipeError 判定管道写入失败是否为“父进程已不再读”的永久错误。
// 仅当父进程真正关闭 / 崩溃 / 断开连接时才是；而普通的“写满/积压”属于
// 可恢复的临时错误，不应据此中止抓包。
func isFatalPipeError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
		return true
	}
	low := strings.ToLower(err.Error())
	return strings.Contains(low, "broken pipe") ||
		strings.Contains(low, "the pipe is being closed") ||
		strings.Contains(low, "no data available") ||
		strings.Contains(low, "nul error received") ||
		strings.Contains(low, "closed")
}

// readWorkerStream drains newline-delimited Packet JSON from the worker's pipe
// into onPacket. It skips the hello line and returns when the pipe closes
// (worker exited / parent stopped the capture). The caller owns closing r.
func readWorkerStream(r io.Reader, onPacket func(*Packet)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line == workerHello {
			continue
		}
		var p Packet
		if err := json.Unmarshal([]byte(line), &p); err != nil {
			continue
		}
		onPacket(&p)
	}
	return sc.Err()
}
