package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ys-ll/uniterm/backend/mcp"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
)

func TestMCPForegroundDiscoversAllTextTerminals(t *testing.T) {
	dir := t.TempDir()
	cs, err := store.NewConnectionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{sessionManager: session.NewSessionManager(), connectionStore: cs, localStateStore: store.NewLocalStateStore(dir)}
	profiles := []session.ConnectionConfig{}
	for _, kind := range []string{"ssh", "local", "wsl", "telnet", "mosh", "serial", "tcp", "sftp"} {
		cfg := session.ConnectionConfig{ID: kind, Type: kind, Name: kind}
		profiles = append(profiles, cfg)
		if _, err := a.sessionManager.Create(kind, cfg); err != nil {
			t.Fatal(err)
		}
	}
	if err := cs.Save(session.ConnectionStoreData{Connections: profiles}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		mode string
		want int
	}{{"foreground", 7}, {"background", 1}} {
		if err := a.localStateStore.Save(store.LocalState{MCP: &store.MCPSettings{ExecMode: tc.mode}}); err != nil {
			t.Fatal(err)
		}
		listed, saved := a.mcpListSessions(), a.mcpListConnections()
		if len(listed) != tc.want || len(saved) != tc.want {
			t.Fatalf("%s: sessions=%d profiles=%d", tc.mode, len(listed), len(saved))
		}
		for _, row := range listed {
			if row.Type == "sftp" {
				t.Fatal("file session exposed as terminal")
			}
		}
	}
}

func TestMcpNotifyStrings(t *testing.T) {
	en := mcpNotifyStringsFor("en")
	zh := mcpNotifyStringsFor("zh-CN")
	if en.Title != "uniTerm · MCP approval request" {
		t.Errorf("en title = %q", en.Title)
	}
	if !strings.Contains(zh.BodyConnect, "请求建立新连接") {
		t.Errorf("zh connect template = %q", zh.BodyConnect)
	}
	if want := fmt.Sprintf(zh.BodyConnect, "cc"); mcpNotifyBody(zh, "cc", "") != want {
		t.Errorf("zh connect body = %q, want %q", mcpNotifyBody(zh, "cc", ""), want)
	}
	if got := mcpNotifyBody(en, "cc", "ls -la"); got != "cc: ls -la" {
		t.Errorf("short command preview = %q", got)
	}
	long := string(make([]byte, 100))
	if got := mcpNotifyBody(en, "cc", long); !strings.HasSuffix(got, "…") {
		t.Errorf("long command should end with ellipsis, got %q", got)
	}
	if mcpNotifyStringsFor("xx-unknown") != mcpNotifyLabels["en"] {
		t.Errorf("unknown language must fall back to English")
	}
}

// ── MCP file transfer waiters (see app_mcp.go) ──────────────────────────

func newMcpTransferTestApp() *App {
	return &App{
		mcpTransferWaiters: make(map[string]chan mcpTransferOutcome),
	}
}

func TestMcpRunTransferSuccess(t *testing.T) {
	a := newMcpTransferTestApp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.mcpDispatchTransferEvent(map[string]any{"taskId": "t1", "event": "complete", "status": "done"})
	}()
	if err := a.mcpRunTransfer(func() (string, error) { return "t1", nil }, 2*time.Second); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
}

func TestMcpRunTransferErrorCarriesMessage(t *testing.T) {
	a := newMcpTransferTestApp()
	go func() {
		time.Sleep(20 * time.Millisecond)
		a.mcpDispatchTransferEvent(map[string]any{"taskId": "t2", "event": "complete", "status": "error", "error": "permission denied"})
	}()
	err := a.mcpRunTransfer(func() (string, error) { return "t2", nil }, 2*time.Second)
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected error carrying event message, got %v", err)
	}
}

// The completion event may fire between start() returning and the waiter
// registering — the run-transfer mutex must make dispatch wait for the
// registration instead of dropping the event. Simulated deterministically:
// the worker goroutine dispatches (and blocks on the held mutex) while
// start() is still running.
func TestMcpRunTransferCompletionDuringStart(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) {
		go a.mcpDispatchTransferEvent(map[string]any{"taskId": "t3", "event": "complete", "status": "done"})
		time.Sleep(50 * time.Millisecond) // let the dispatch block on the mutex
		return "t3", nil
	}, 2*time.Second)
	if err != nil {
		t.Fatalf("expected success despite completion during start, got %v", err)
	}
}

func TestMcpRunTransferStartError(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) { return "", fmt.Errorf("start failed") }, 2*time.Second)
	if err == nil || err.Error() != "start failed" {
		t.Fatalf("expected start error passthrough, got %v", err)
	}
}

func TestMcpRunTransferTimeout(t *testing.T) {
	a := newMcpTransferTestApp()
	err := a.mcpRunTransfer(func() (string, error) { return "t4", nil }, 30*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

// Non-complete events must not resolve a waiter.
func TestMcpDispatchIgnoresProgressEvents(t *testing.T) {
	a := newMcpTransferTestApp()
	a.mcpDispatchTransferEvent(map[string]any{"taskId": "t5", "event": "start", "status": "done"})
	if len(a.mcpTransferWaiters) != 0 {
		t.Fatalf("waiter map should stay empty without registered waiters, got %d", len(a.mcpTransferWaiters))
	}
}

// Sequential AI-style long commands against a real narrow ConPTY PowerShell —
// the flow that garbled returns: every paste's echo repaints over the previous
// command's leftovers, and each job renders through a fresh emulator fed by
// the shared tap. Reproduces the merged-fragment / missing-output returns the
// built-in panel produced when the emulator grid did not match the PTY size.
func TestForegroundSequentialLongCommands(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows ConPTY")
	}
	if os.Getenv("UNITERM_TEST_PTY") != "1" {
		t.Skip("set UNITERM_TEST_PTY=1 to test real local terminals")
	}
	s := session.NewLocalSession("test-seq")
	// Narrow docked-panel geometry, set before Connect so the ConPTY itself
	// starts at 54x30.
	s.SetPendingSize(54, 30)
	if err := s.Connect(session.ConnectionConfig{ShellPath: "powershell.exe"}); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()
	var rawMu sync.Mutex
	var raw []byte
	// No DSR injection here: ConPTY answers cursor-position probes itself and
	// a synthetic reply only corrupts PSReadLine's renderer.
	off := s.AddDataTap(func(data []byte) {
		rawMu.Lock()
		raw = append(raw, data...)
		rawMu.Unlock()
	})
	defer off()
	pty := ptyAdapter{s: s, taps: s}
	r := mcp.NewRegistry()

	marker := func(i, k int) string { return fmt.Sprintf("SEQ%d-MARKER-%d-ok", i, k) }
	commands := []string{
		fmt.Sprintf("$n=0; Write-Output '%s'; Get-Volume | Where-Object { $_.DriveLetter } | Sort-Object DriveLetter | Format-Table DriveLetter, FileSystemLabel, FileSystem, @{N='Size(GB)';E={[math]::Round($_.Size/1GB,2)}}, @{N='Free(GB)';E={[math]::Round($_.SizeRemaining/1GB,2)}} -AutoSize; Write-Output '%s'; Write-Output '%s'", marker(0, 0), marker(0, 1), marker(0, 2)),
		fmt.Sprintf("Get-Volume | Where-Object { $_.DriveLetter } | ForEach-Object { '{0}: [{1}] NTFS  Total {2} GB | Free {3} GB | Used {4} %%' -f $_.DriveLetter, $_.FileSystemLabel, [math]::Round($_.Size/1GB,2), [math]::Round($_.SizeRemaining/1GB,2), [math]::Round(($_.Size-$_.SizeRemaining)/$_.Size*100,1) }; Write-Output '%s'; Write-Output '%s'", marker(1, 0), marker(1, 1)),
		fmt.Sprintf("Get-PSDrive -PSProvider FileSystem | ForEach-Object { Write-Output (\"{0}: Total={1}GB Free={2}GB\" -f $_.Name, [math]::Round($_.Used/1GB + $_.Free/1GB,1), [math]::Round($_.Free/1GB,1)) }; Write-Output '%s'; Write-Output '%s'", marker(2, 0), marker(2, 1)),
		fmt.Sprintf("Get-PSDrive C | Select-Object Name, Used, Free | Format-List; Write-Output '%s'; Write-Output '%s'; Write-Output '%s'", marker(3, 0), marker(3, 1), marker(3, 2)),
	}

	for i, cmdText := range commands {
		cols, rows := sessionScreenSize(s)
		job, err := r.StartWithScreen(pty, s.ID(), cmdText, newlineFor(s), 15*time.Second, 0, cols, rows)
		if err != nil {
			t.Fatalf("command %d: %v", i, err)
		}
		deadline := time.Now().Add(20 * time.Second)
		var out []byte
		var jerr error
		for {
			var state string
			out, _, _, state, jerr = job.Snapshot()
			if state != mcp.StateRunning {
				t.Logf("command %d finished: state=%s err=%v outLen=%d", i, state, jerr, len(out))
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("command %d: completion not received, output=%q", i, out)
			}
			time.Sleep(50 * time.Millisecond)
		}
		for k := 0; k < 3; k++ {
			if k == 2 && len(commands[i]) < 400 {
				continue // commands 1 and 2 emit two markers only
			}
			want := marker(i, k)
			found := false
			for _, line := range strings.Split(string(out), "\n") {
				if strings.TrimSpace(line) == want {
					found = true
					break
				}
			}
			if !found {
				rawMu.Lock()
				t.Logf("raw len=%d tail=%q", len(raw), string(raw[max(0, len(raw)-1500):]))
				rawMu.Unlock()
				t.Fatalf("command %d: marker %q not intact in output (len=%d):\n%q", i, want, len(out), out)
			}
		}
		r.Interrupt(job.ID)
	}
}

// Opt in because this starts a real ConPTY shell. This checks the exact path
// used by the built-in assistant, including terminal control-sequence handling.
func TestForegroundCompletionOnRealWindowsTerminal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires Windows ConPTY")
	}
	if os.Getenv("UNITERM_TEST_PTY") != "1" {
		t.Skip("set UNITERM_TEST_PTY=1 to test real local terminals")
	}
	for _, tc := range []struct{ name, path, command, check string }{
		{"git-bash", `C:\Program Files\Git\bin\bash.exe`, `__ut_keep=kept; printf 'first\n'; sleep 1.1; printf 'last\n'; (exit 7)`, `printf '%s\n' "$__ut_keep"`},
		{
			// Realistic AI payloads: long enough that the wrapped echo spans
			// several screen rows and the SECOND paste repaints over the
			// first command's leftovers — the layout that garbled output.
			"powershell", "powershell.exe",
			`$__ut_keep='kept'; Write-Output 'first'; Start-Sleep -Milliseconds 1100; Get-Volume | Where-Object { $_.DriveLetter } | Sort-Object DriveLetter | Format-Table DriveLetter, FileSystemLabel, FileSystem, @{N='Size(GB)';E={[math]::Round($_.Size/1GB,2)}}, @{N='Free(GB)';E={[math]::Round($_.SizeRemaining/1GB,2)}} -AutoSize; Write-Output 'last'; cmd /c exit 7`,
			`Write-Output ('padding-padding-padding-padding-padding-padding-padding-padding-padding-padding-padding-padding-padding-padding-padding'); Write-Output $__ut_keep`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := session.NewLocalSession("test-" + tc.name)
			// A narrow terminal is the garbling reproduction: ConPTY echoes
			// rely on auto-wrap at the real width, so an emulator grid wider
			// than the PTY lays the stream out on different rows and the AI
			// receives merged fragments. Size is set before Connect so the
			// ConPTY itself starts narrow, like a real docked panel.
			s.SetPendingSize(54, 30)
			if err := s.Connect(session.ConnectionConfig{ShellPath: tc.path}); err != nil {
				t.Fatal(err)
			}
			defer s.Disconnect()
			var rawMu sync.Mutex
			var raw strings.Builder
			// A headless test has no xterm.js to answer cursor-position probes.
			off := s.AddDataTap(func(data []byte) {
				rawMu.Lock()
				raw.Write(data)
				rawMu.Unlock()
				if strings.Contains(string(data), "\x1b[6n") {
					_ = s.Write([]byte("\x1b[1;1R"))
				}
			})
			defer off()
			pty := ptyAdapter{s: s, taps: s}
			r := mcp.NewRegistry()
			started := time.Now()
			sCols, sRows := sessionScreenSize(s)
			job, err := r.StartWithScreen(pty, s.ID(), tc.command, newlineFor(s), 8*time.Second, 0, sCols, sRows)
			if err != nil {
				t.Fatal(err)
			}
			wait := func(job *mcp.Job, wanted int, text string) {
				t.Helper()
				defer r.Interrupt(job.ID)
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					out, _, code, state, err := job.Snapshot()
					if state != mcp.StateRunning {
						found := false
						for _, line := range strings.Split(string(out), "\n") {
							found = found || strings.TrimSpace(line) == text
						}
						if state != mcp.StateExited || code != wanted || !found {
							rawMu.Lock()
							rawText := raw.String()
							if len(rawText) > 2000 {
								rawText = rawText[len(rawText)-2000:]
							}
							t.Logf("raw tail=%q", rawText)
							rawMu.Unlock()
							t.Fatalf("state=%s code=%d err=%v output=%q", state, code, err, out)
						}
						return
					}
					time.Sleep(20 * time.Millisecond)
				}
				t.Fatal("completion not received")
			}
			wait(job, 7, "last")
			if time.Since(started) < time.Second {
				t.Fatal("returned during command pause")
			}
			// Dot/eval must preserve variables in the user's current shell.
			nCols, nRows := sessionScreenSize(s)
			next, err := r.StartWithScreen(pty, s.ID(), tc.check, newlineFor(s), 8*time.Second, 0, nCols, nRows)
			if err != nil {
				t.Fatal(err)
			}
			wait(next, 0, "kept")
		})
	}
}

// ── Live-shell probes for the foreground job emission pipeline ───────────
// Opt-in diagnostics that print the AI-visible stdout of real sessions.
// They skip unless UNITERM_TEST_PTY=1 (plus UNITERM_SSH_HOST/UNITERM_SSH_USER
// for the SSH one) so CI never opens real terminals.

// Dumps the AI-visible stdout of sequential local-shell jobs; mirrors the
// app's flow (capture emulator attached before Connect provides the
// prompt-line seed). Shell selectable via UNITERM_PROBE_SHELL.
func TestProbeDumpFirstLine(t *testing.T) {
	if runtime.GOOS != "windows" || os.Getenv("UNITERM_TEST_PTY") != "1" {
		t.Skip("set UNITERM_TEST_PTY=1 to probe real local terminals")
	}
	s := session.NewLocalSession("probe")
	s.SetPendingSize(120, 30)
	// Capture emulator attached BEFORE Connect, like the app does, so it sees
	// the shell banner + prompt.
	cols0, rows0 := sessionScreenSize(s)
	capScreen := mcp.NewScreen(cols0, rows0)
	s.AddDataTap(func(b []byte) {
		capScreen.Feed(b)
	})
	shell := "powershell.exe"
	if v := os.Getenv("UNITERM_PROBE_SHELL"); v != "" {
		shell = v
	}
	if err := s.Connect(session.ConnectionConfig{ShellPath: shell}); err != nil {
		t.Skipf("conpty unavailable: %v", err)
	}
	defer s.Disconnect()
	time.Sleep(2 * time.Second) // let the prompt settle

	// Replicate the app's panel-open sequence: snapshot the prompt row from
	// the old capture, replace the capture, then resize the PTY. The fresh
	// capture must recover the prompt line from ConPTY's post-resize repaint.
	carried := capScreen.PromptSeed()
	capScreen = mcp.NewScreen(54, 30)
	if err := s.Resize(54, 30); err != nil {
		t.Fatalf("resize: %v", err)
	}
	s.SetPendingSize(54, 30)
	time.Sleep(1 * time.Second)
	seed := capScreen.PromptSeed()
	t.Logf("carried=%q post-resize seed=%q", carried.Line, seed.Line)

	pty := ptyAdapter{s: s, taps: s}
	r := mcp.NewRegistry()
	cols, rows := sessionScreenSize(s)
	cmds := []string{
		`Write-Output 'first-cmd-marker'; Get-PSDrive C | Select-Object Name | Format-List`,
		`Write-Output 'second-cmd-marker'; Get-PSDrive C | Select-Object Used | Format-List`,
	}
	for i, cmd := range cmds {
		useSeed := seed
		if useSeed.Line == "" {
			useSeed = carried
			useSeed.Y = 0
		}
		job, err := r.StartWithScreen(pty, s.ID(), cmd, newlineFor(s), 10*time.Second, 0, cols, rows, useSeed)
		if err != nil {
			t.Fatal(err)
		}
		var stdout []byte
		waitJob(t, job, 10*time.Second, &stdout)
		fmt.Printf("===== CMD %d EMITTED STDOUT BEGIN =====\n%s===== CMD %d EMITTED STDOUT END =====\n", i, string(stdout), i)
		r.Interrupt(job.ID)
		time.Sleep(500 * time.Millisecond)
	}
}

// Real SSH session + job engine over the AI panel's long-Chinese-command
// flow: connect wide, fill the screen so the prompt sits at the bottom row,
// then resize to the docked-panel size before running the jobs. Requires a
// reachable host via UNITERM_SSH_HOST / UNITERM_SSH_USER.
func TestProbeSSHJobOutput(t *testing.T) {
	if os.Getenv("UNITERM_TEST_PTY") != "1" {
		t.Skip("set UNITERM_TEST_PTY=1 to probe real SSH sessions")
	}
	host, user := os.Getenv("UNITERM_SSH_HOST"), os.Getenv("UNITERM_SSH_USER")
	if host == "" || user == "" {
		t.Skip("set UNITERM_SSH_HOST and UNITERM_SSH_USER")
	}
	keyPath := filepath.Join(os.Getenv("USERPROFILE"), ".ssh", "uterm_debug", "id_ed25519")
	key, err := os.ReadFile(keyPath)
	if err != nil {
		t.Skipf("debug key not available: %v", err)
	}
	s := session.NewSSHSession("probe-ssh")
	cfg := session.ConnectionConfig{
		ID:         "probe-ssh",
		Type:       "ssh",
		Host:       host,
		Port:       22,
		User:       user,
		AuthType:   "keyText",
		KeyContent: string(key),
	}
	s.SetPendingSize(120, 30)
	// Capture emulator before Connect, mirroring the app; provides the
	// prompt-line seed for each job.
	capCols, capRows := sessionScreenSize(s)
	capScreen := mcp.NewScreen(capCols, capRows)
	s.AddDataTap(func(b []byte) {
		capScreen.Feed(b)
	})
	if err := s.Connect(cfg); err != nil {
		t.Skipf("ssh connect failed: %v", err)
	}
	defer s.Disconnect()
	time.Sleep(1 * time.Second)

	// Fill the screen so the prompt sits at the bottom row, like a used session.
	r0 := mcp.NewRegistry()
	j0, err := r0.StartWithScreen(ptyAdapter{s: s, taps: s}, s.ID(), `seq 1 40; echo filler-done`, newlineFor(s), 10*time.Second, 0, 120, 30)
	if err == nil {
		time.Sleep(2 * time.Second)
		r0.Interrupt(j0.ID)
	}
	time.Sleep(500 * time.Millisecond)

	// The AI panel docks: replicate the app's SessionResize — snapshot the
	// prompt row from the old capture, replace the capture, then resize.
	// SSH never repaints after a resize, so the fresh capture stays blank and
	// the job falls back to the carried seed.
	carried := capScreen.PromptSeed()
	capScreen = mcp.NewScreen(54, 30)
	if err := s.Resize(54, 30); err != nil {
		t.Fatalf("resize: %v", err)
	}
	s.SetPendingSize(54, 30)
	time.Sleep(1 * time.Second)
	seed := capScreen.PromptSeed()
	t.Logf("carried=%q post-resize seed=%q", carried.Line, seed.Line)

	pty := ptyAdapter{s: s, taps: s}
	r := mcp.NewRegistry()
	cols, rows := sessionScreenSize(s)
	cmds := []string{
		`echo "=== 磁盘 ==="; lsblk -d -o NAME,SIZE,MODEL,ROTA 2>/dev/null; echo; df -hT | grep -v tmpfs; echo; echo "=== PCI 设备 ==="; lspci 2>/dev/null | head -15; echo; echo "=== 虚拟化标识 ==="; systemd-detect-virt 2>/dev/null || (dmidecode -t system 2>/dev/null | grep -E "Manufacturer|Product"); echo; echo "=== 网卡 ==="; ip -br addr 2>/dev/null | head -5`,
		`echo "second-cmd-$(date +%s)"; uname -a; uptime`,
	}
	for i, cmd := range cmds {
		useSeed := seed
		if useSeed.Line == "" {
			useSeed = carried
			useSeed.Y = 0
		}
		job, err := r.StartWithScreen(pty, s.ID(), cmd, newlineFor(s), 15*time.Second, 0, cols, rows, useSeed)
		if err != nil {
			t.Fatal(err)
		}
		var stdout []byte
		waitJob(t, job, 15*time.Second, &stdout)
		fmt.Printf("===== SSH CMD %d EMITTED STDOUT BEGIN =====\n%s===== SSH CMD %d EMITTED STDOUT END =====\n", i, string(stdout), i)
		r.Interrupt(job.ID)
		time.Sleep(500 * time.Millisecond)
	}
}

// waitJob blocks until the job leaves the running state or the deadline
// passes, and captures the final stdout into out when non-nil.
func waitJob(t *testing.T, job *mcp.Job, timeout time.Duration, out *[]byte) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var stdout []byte
	for {
		var state string
		stdout, _, _, state, _ = job.Snapshot()
		if state != mcp.StateRunning || time.Now().After(deadline) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if out != nil {
		*out = stdout
	}
}

// ptyAdapter must forward the session's ExecFilterEncoding hint: the registry
// probes the adapter, not the session, and a dropped method silently disables
// the MSYS bash output filter (all jobs fall back to the plain wrapper).
type execFilterStub struct {
	session.Session
	filter string
}

func (s execFilterStub) ExecFilterEncoding() string { return s.filter }

func TestPtyAdapterForwardsExecFilterEncoding(t *testing.T) {
	p := ptyAdapter{s: execFilterStub{filter: "cp936"}}
	if got := p.ExecFilterEncoding(); got != "cp936" {
		t.Fatalf("adapter dropped the filter hint: %q", got)
	}
	if got := (ptyAdapter{s: execFilterStub{}}).ExecFilterEncoding(); got != "" {
		t.Fatalf("session without hint reported %q", got)
	}
}
