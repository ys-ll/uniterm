package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCompletionScannerEveryReadBoundary(t *testing.T) {
	const token = "fg-test"
	prefix := "__UT_" + token + ";"
	wire := "中文 output\r\n" + prefix + "23__$ next prompt"
	for split := 0; split <= len(wire); split++ {
		sc := completionScanner{prefix: []byte(prefix)}
		a, code, done := sc.feed([]byte(wire[:split]))
		if !done {
			b, status, finished := sc.feed([]byte(wire[split:]))
			a = append(a, b...)
			code, done = status, finished
		}
		if string(a) != "中文 output\r\n" || !done || code != 23 {
			t.Fatalf("split %d: %q %d %v", split, a, code, done)
		}
	}
}

func TestCompletionCommandsInRealShells(t *testing.T) {
	for _, shell := range []string{"bash", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if runtime.GOOS == "windows" && shell == "bash" {
				gitBash := `C:\Program Files\Git\bin\bash.exe`
				if _, statErr := os.Stat(gitBash); statErr == nil {
					path, err = gitBash, nil
				}
			}
			if err != nil {
				t.Skip("shell unavailable")
			}
			const token = "fg-test-real-shell"
			command := "printf '%s\\n' \"a'b\"; sleep 0.15; printf 'last line\\n'; (exit 7)"
			if shell == "powershell" {
				command = `Write-Output "a'b"; Start-Sleep -Milliseconds 150; Get-Item .; Write-Output 'last line'; cmd /c exit 7`
			}
			wrapped, _, ok := CompletionCommand(command, shell, "", token, "\n")
			if !ok {
				t.Fatal("supported shell rejected")
			}
			var cmd *exec.Cmd
			if shell == "bash" {
				cmd = exec.Command(path, "--noprofile", "--norc", "-c", wrapped)
			} else {
				cmd = exec.Command(path, "-NoProfile", "-NonInteractive", "-Command", wrapped)
			}
			started := time.Now()
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("wrapper failed: %v %q", err, out)
			}
			sc := completionScanner{prefix: []byte("__UT_" + token + ";")}
			cleaned, code, done := sc.feed(out)
			if !done || code != 7 || !strings.Contains(string(cleaned), "a'b") || !strings.Contains(string(cleaned), "last line") {
				t.Fatalf("missing output/exit: %q code=%d done=%v", out, code, done)
			}
			if time.Since(started) < 150*time.Millisecond {
				t.Fatal("returned before command finished")
			}
		})
	}
}

// The GBK filter path (see session.LocalSession.ExecFilterEncoding) runs the
// command's output through a fifo + Perl line filter inside the real shell.
func TestCompletionCommandGBKFilter(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if runtime.GOOS == "windows" {
		gitBash := `C:\Program Files\Git\bin\bash.exe`
		if _, statErr := os.Stat(gitBash); statErr == nil {
			bashPath, err = gitBash, nil
		}
	}
	if err != nil {
		t.Skip("bash unavailable")
	}
	const token = "fg-test-gbk"
	command := "printf 'GBK中文测试\n' | iconv -f utf-8 -t gbk; echo 中文直出"
	wrapped, _, ok := CompletionCommand(command, "bash", "cp936", token, "\n")
	if !ok {
		t.Fatal("supported shell rejected")
	}
	out, err := exec.Command(bashPath, "--noprofile", "--norc", "-c", wrapped).CombinedOutput()
	if err != nil {
		t.Fatalf("wrapper failed: %v %q", err, out)
	}
	sc := completionScanner{prefix: []byte("__UT_" + token + ";")}
	cleaned, code, done := sc.feed(out)
	if !done || code != 0 {
		t.Fatalf("missing completion: %q", cleaned)
	}
	if !strings.Contains(string(cleaned), "GBK中文测试") || !strings.Contains(string(cleaned), "中文直出") {
		t.Fatalf("filter output wrong: %q", cleaned)
	}
}

func TestTimeoutKeepsPartialLineAndOriginalSessionLock(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	j, err := r.StartWithScreen(p, "s", "read", "\n", 40*time.Millisecond, 0, 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	p.emit("partial")
	waitState(t, j, StateTimeout)
	out, _, _, _, _ := j.Snapshot()
	if string(out) != "partial" {
		t.Fatalf("partial output lost: %q", out)
	}
	// The wait timeout keeps the session lock, but a NEW command supersedes
	// the timed-out job (see TestTakeOverSupersedesTimedOutJob); this test
	// exercises the no-new-command path: the tap stays installed, so output
	// emitted after the timeout is still collected when Collect resumes.
	p.emit(" continued\r\nlate line\r\n")
	if resumed, ok := r.Collect("s", time.Second); !ok || resumed != j {
		t.Fatal("did not resume original job")
	}
	p.complete(j, 0)
	waitDone(t, j, time.Second)
	out, _, _, _, _ = j.Snapshot()
	if string(out) != "partial continued\nlate line\n" {
		t.Fatalf("lost/duplicated partial line: %q", out)
	}
	// Completion frees the lock immediately, without depending on watcher scheduling.
	next, err := r.Start(p, "s", "next", "\n", time.Second, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.complete(next, 0)
}

// The echo fallback for shells outside the known list: the acknowledgement is
// two bare echo lines after the command. Detection must anchor on line start
// so the terminal echo of the typed marker line (preceded by "echo ") can
// never impersonate it, must tolerate the command's own output ending without
// a trailing newline (the filler echo guarantees the marker's leading
// newline), and reports no exit code.
func TestEchoFallbackCompletionForUnknownShell(t *testing.T) {
	wire, sc, ok := CompletionCommand("uname -a", "", "", "fg-x", "\n")
	if !ok || sc == nil || !sc.plain {
		t.Fatalf("unknown shell must get the plain echo fallback: ok=%v sc=%v", ok, sc)
	}
	if want := "uname -a\necho\necho __UT_fg-x__"; wire != want {
		t.Fatalf("wire=%q want %q", wire, want)
	}

	// The terminal echo of the typed lines must not complete the job.
	if _, _, done := sc.feed([]byte("$ uname -a\r\necho\r\necho __UT_fg-x__\r\n")); done {
		t.Fatal("typed echo impersonated the acknowledgement")
	}
	// A marker appearing directly after a newline is indistinguishable from a
	// genuine acknowledgement — and that is fine: only the typed echo carries
	// "echo " before the token, which the line-start anchor excludes.

	// Real acknowledgement: command output ends without a trailing newline,
	// the filler echo re-establishes the line boundary before the marker.
	sc = &completionScanner{prefix: []byte("\n__UT_fg-x__"), plain: true}
	cleaned, code, done := sc.feed([]byte("partial"))
	if done {
		t.Fatal("completed on keep-back chunk")
	}
	b, code, done := sc.feed([]byte(" output\r\nECHO is on.\r\n__UT_fg-x__\r\n$ "))
	cleaned = append(cleaned, b...)
	if !done || code != -1 {
		t.Fatalf("done=%v code=%d", done, code)
	}
	// The next prompt after the marker is intentionally not part of the result.
	if string(cleaned) != "partial output\r\nECHO is on.\r\n" {
		t.Fatalf("marker not stripped from output: %q", cleaned)
	}

	// Arbitrary read boundaries, like the eval-mode scanner.
	wire = "out\r\n" + "\n__UT_fg-x__\r\n$ "
	for split := 0; split <= len(wire); split++ {
		sc := &completionScanner{prefix: []byte("\n__UT_fg-x__"), plain: true}
		a, code, done := sc.feed([]byte(wire[:split]))
		if !done {
			b, status, finished := sc.feed([]byte(wire[split:]))
			a = append(a, b...)
			code, done = status, finished
		}
		if !done || code != -1 || string(a) != "out\r\n\n" {
			t.Fatalf("split %d: %q %d %v", split, a, code, done)
		}
	}
}

func TestUnknownShellWaitsForTimeoutDespitePromptAndForeignMarker(t *testing.T) {
	r := NewRegistry()
	p := &fakePty{alive: true}
	// The interface wrapper intentionally omits ForegroundShell.
	j, err := r.Start(struct{ Pty }{p}, "s", "cmd", "\n", 150*time.Millisecond, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.emit(fmt.Sprintf("$ done?\r\n__UT_%s;0__", j.ID))
	time.Sleep(50 * time.Millisecond)
	_, _, _, state, _ := j.Snapshot()
	if state != StateRunning {
		t.Fatalf("unverified completion: %s", state)
	}
	waitState(t, j, StateTimeout)
	_ = r.Interrupt(j.ID)
}
