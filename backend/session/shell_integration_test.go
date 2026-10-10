package session

import (
	"bytes"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestOSC7ScannerSplitAcrossChunks(t *testing.T) {
	var sc osc7Scanner
	// partial sequence in first chunk, rest in second
	cwd1, cleaned1, found1 := sc.Feed([]byte("hello\x1b]7;file://myhost/ho"))
	cwd2, cleaned2, found2 := sc.Feed([]byte("me/user\x1b\\world"))
	if found1 || cwd1 != "" {
		t.Fatal("must not report before terminator")
	}
	if !found2 || cwd2 != "/home/user" {
		t.Fatalf("cwd=%q found=%v", cwd2, found2)
	}
	combined := string(cleaned1) + string(cleaned2)
	if combined != "helloworld" {
		t.Fatalf("cleaned = %q, want helloworld", combined)
	}
	if bytes.Contains([]byte(combined), []byte("\x1b]7;")) {
		t.Fatal("cleaned output still contains the OSC-7 sequence")
	}
}

func TestOSC7ScannerSplitTerminator(t *testing.T) {
	var sc osc7Scanner
	cwd1, _, found1 := sc.Feed([]byte("\x1b]7;file://h/a\x1b"))
	cwd2, cleaned2, found2 := sc.Feed([]byte("\\next"))
	if found1 || cwd1 != "" {
		t.Fatal("must not report before terminator")
	}
	if !found2 || cwd2 != "/a" {
		t.Fatalf("cwd=%q found=%v", cwd2, found2)
	}
	if string(cleaned2) != "next" {
		t.Fatalf("cleaned = %q, want next", cleaned2)
	}
}

func TestOSC7UrlDecoding(t *testing.T) {
	var sc osc7Scanner
	cwd, _, found := sc.Feed([]byte("\x1b]7;file://h/space%20dir\x07"))
	if !found || cwd != "/space dir" {
		t.Fatalf("cwd=%q found=%v", cwd, found)
	}
}

func TestOSC7ScannerEmptyHostBEL(t *testing.T) {
	var sc osc7Scanner
	// BEL-terminated sequence with an empty host part (file:///...)
	cwd, cleaned, found := sc.Feed([]byte("pre\x1b]7;file:///home/x\x07post"))
	if !found || cwd != "/home/x" {
		t.Fatalf("cwd=%q found=%v", cwd, found)
	}
	if string(cleaned) != "prepost" {
		t.Fatalf("cleaned = %q, want prepost", cleaned)
	}
}

// Reproduced in the field (dev log): real prompts emit the ST-terminated OSC-7
// immediately followed by a BEL-terminated OSC-0 title sequence. The scanner
// must terminate the OSC-7 payload at the FIRST terminator (ST here), not at
// the later BEL — swallowing "\x1b\\" plus the whole title into the payload
// produced a garbage cwd and disabled path following entirely.
func TestOSC7ScannerSTTerminatedBeforeFollowingOSC0(t *testing.T) {
	var sc osc7Scanner
	input := "\x1b]7;file:///root\x1b\\\x1b]0;root@localhost:~\x07"
	cwd, cleaned, found := sc.Feed([]byte(input))
	if !found || cwd != "/root" {
		t.Fatalf("cwd=%q found=%v, want /root", cwd, found)
	}
	if !strings.Contains(string(cleaned), "\x1b]0;root@localhost:~\x07") {
		t.Fatalf("following OSC-0 title must pass through to the display: %q", cleaned)
	}
	if strings.Contains(string(cleaned), "\x1b]7;") {
		t.Fatal("cleaned output still contains the OSC-7 sequence")
	}
}

func TestBuildStartupCwdHook(t *testing.T) {
	tests := []struct {
		name     string
		shell    string
		wantOK   bool
		mustHave []string
	}{
		{
			name:   "bash",
			shell:  "/bin/bash",
			wantOK: true,
			mustHave: []string{
				"__uniterm_osc7",
				"stty -echo",
				`\033[6n`,
				"stty echo",
				`printf '\033]7777;uniterm-ok\007'`,
			},
		},
		{
			name:   "zsh",
			shell:  "/usr/bin/zsh",
			wantOK: true,
			mustHave: []string{
				"__uniterm_osc7",
				"precmd_functions",
				"stty -echo",
				`\033[6n`,
				"stty echo",
				`printf '\033]7777;uniterm-ok\007'`,
			},
		},
		{
			name:   "fish",
			shell:  "/usr/bin/fish",
			wantOK: true,
			mustHave: []string{
				"__uniterm_osc7",
				`printf '\r\e[2K'`,
				`printf '\e]7777;uniterm-ok\a'`,
				"stty echo",
			},
		},
		{name: "unsupported ksh", shell: "/usr/bin/ksh", wantOK: false},
		{name: "windows cmd", shell: "cmd", wantOK: false},
		{name: "windows backslash path", shell: `C:\Program Files\Git\bin\bash.exe`, wantOK: false},
		{name: "empty", shell: "", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := buildStartupCwdHook(tt.shell)
			if ok != tt.wantOK {
				t.Fatalf("buildStartupCwdHook(%q) ok = %v, want %v", tt.shell, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if !strings.HasPrefix(got, " ") || !strings.HasSuffix(got, "\n") {
				t.Fatalf("snippet must start with a space and end with a newline: %q", got)
			}
			if !strings.Contains(got, "__uniterm_osc7") {
				t.Fatalf("snippet must define the __uniterm_osc7 hook: %q", got)
			}
			// bash/zsh inject two lines: the echo-off line must come first so
			// the hook line itself never renders.
			if tt.shell != "/usr/bin/fish" && !strings.HasPrefix(got, " stty -echo;") {
				t.Fatalf("bash/zsh snippet must start with the echo-off line: %q", got)
			}
			for _, want := range tt.mustHave {
				if !strings.Contains(got, want) {
					t.Fatalf("snippet for %s must contain %q: %q", tt.shell, want, got)
				}
			}
		})
	}
}

// Echo must be restored BEFORE the ready marker is printed: a missing stty
// then leaves the marker unsent, so the blind fallback restores echo instead
// of a confirmed hook leaving the terminal permanently silent. The line
// clear (erasing a prompt printed before the injection landed, so the shell's
// post-hook prompt renders exactly once) must also precede the marker.
func TestStartupCwdHookRestoresEchoBeforeMarker(t *testing.T) {
	for _, shell := range []string{"/bin/bash", "/usr/bin/zsh", "/usr/bin/fish"} {
		hook, ok := buildStartupCwdHook(shell)
		if !ok {
			t.Fatalf("shell %q must be supported", shell)
		}
		if strings.Index(hook, "stty echo") > strings.Index(hook, "7777;uniterm-ok") {
			t.Fatalf("stty echo must run before the ready marker for %s: %q", shell, hook)
		}
		if strings.Index(hook, "[2K") > strings.Index(hook, "7777;uniterm-ok") {
			t.Fatalf("line clear must run before the ready marker for %s: %q", shell, hook)
		}
	}
}

func TestGeneratedStartupCwdHookHasValidBashSyntax(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not available for generated-script syntax checks")
	}
	hook, ok := buildStartupCwdHook("/bin/bash")
	if !ok {
		t.Fatal("bash must be supported")
	}
	cmd := exec.Command(bash, "-n")
	cmd.Stdin = strings.NewReader(hook)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("startup cwd hook has invalid bash syntax: %v: %s\n%s", err, out, hook)
	}
}

func TestHookReadyScannerSingleChunk(t *testing.T) {
	sc := newHookReadyScanner(sshCwdHookReadyMarker)
	in := []byte("prompt stuff" + sshCwdHookReadyMarker + "more")
	cleaned, found := sc.Feed(in)
	if !found {
		t.Fatal("marker must be detected")
	}
	if string(cleaned) != "prompt stuffmore" {
		t.Fatalf("cleaned = %q, want %q", cleaned, "prompt stuffmore")
	}
}

func TestHookReadyScannerSplitAcrossChunks(t *testing.T) {
	m := []byte(sshCwdHookReadyMarker)
	sc := newHookReadyScanner(sshCwdHookReadyMarker)
	c1, found1 := sc.Feed(append([]byte("abc"), m[:7]...))
	if found1 || string(c1) != "abc" {
		t.Fatalf("first chunk found=%v cleaned=%q", found1, c1)
	}
	c2, found2 := sc.Feed(append(append([]byte{}, m[7:]...), []byte("xyz")...))
	if !found2 {
		t.Fatal("marker split across chunks must be detected")
	}
	if string(c2) != "xyz" {
		t.Fatalf("second chunk cleaned = %q, want xyz", c2)
	}
}

func TestHookReadyScannerNoMarkerPassthrough(t *testing.T) {
	sc := newHookReadyScanner(sshCwdHookReadyMarker)
	in := []byte("normal terminal output\r\n$ ")
	cleaned, found := sc.Feed(in)
	if found {
		t.Fatal("no marker must report found=false")
	}
	if string(cleaned) != string(in) {
		t.Fatalf("cleaned = %q, want input unchanged", cleaned)
	}
}

func TestHookReadyScannerStopsAfterDone(t *testing.T) {
	sc := newHookReadyScanner(sshCwdHookReadyMarker)
	if _, found := sc.Feed([]byte(sshCwdHookReadyMarker)); !found {
		t.Fatal("first marker must be detected")
	}
	sc.done = true
	in := []byte("everything passes through now \x1b]7777;uniterm-ok\x07")
	cleaned, found := sc.Feed(in)
	if found {
		t.Fatal("scanner must stop matching after done")
	}
	if string(cleaned) != string(in) {
		t.Fatalf("cleaned = %q, want input unchanged", cleaned)
	}
}

func TestSSHRunCommandTimesOutWhileOpeningSession(t *testing.T) {
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatalf("parse test host key: %v", err)
	}
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	releaseServer := make(chan struct{})
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		serverConn, err := listener.Accept()
		if err != nil {
			return
		}
		defer serverConn.Close()
		_, channels, requests, err := ssh.NewServerConn(serverConn, serverConfig)
		if err != nil {
			return
		}
		go ssh.DiscardRequests(requests)
		_, ok := <-channels
		if !ok {
			return
		}
		<-releaseServer
		// Closing the transport unblocks the pending channel-open without
		// depending on a channel rejection completing first.
		_ = serverConn.Close()
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial test server: %v", err)
	}
	conn, channels, requests, err := ssh.NewClientConn(clientConn, "pipe", &ssh.ClientConfig{
		User:            "test",
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("create SSH client: %v", err)
	}
	client := ssh.NewClient(conn, channels, requests)

	const timeout = 50 * time.Millisecond
	started := time.Now()
	_, err = sshRunCommand(client, "true", "", timeout)
	if err == nil || !strings.Contains(err.Error(), "timed out opening SSH session") {
		t.Fatalf("sshRunCommand error = %v, want channel-open timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("channel-open timeout took %s, want a bounded return", elapsed)
	}

	close(releaseServer)
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("test SSH server did not stop")
	}
	_ = client.Close()
}


// Regression: a post-login line queued while the injected cwd hook's
// cursor-position probe (read -t 1 < /dev/tty) was still pending was silently
// consumed by that read instead of reaching the shell. runPostLoginAutomation
// must hold its gate until the hook confirms before typing anything.
func TestPostLoginWaitsForCwdHookConfirm(t *testing.T) {
	s := NewSSHSession("test-post-login-gate")
	s.hookConfirmedCh = make(chan struct{})
	s.setStatus(StatusConnected)
	s.RecordReadActivity()

	stdin := &testWriteCloser{}
	s.stdin = stdin

	done := make(chan struct{})
	go func() {
		s.runPostLoginAutomation(ConnectionConfig{PostLoginScript: "echo hookgate"})
		close(done)
	}()

	// Gate must hold: nothing typed while the hook channel is open.
	time.Sleep(300 * time.Millisecond)
	if stdin.Len() != 0 {
		t.Fatalf("post-login typed before hook confirm: %q", stdin.String())
	}

	close(s.hookConfirmedCh)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("post-login automation did not finish after hook confirm")
	}
	if !strings.Contains(stdin.String(), "echo hookgate\r") {
		t.Fatalf("post-login line missing after gate release: %q", stdin.String())
	}
}

type testWriteCloser struct{ bytes.Buffer }

func (w *testWriteCloser) Close() error { return nil }

func TestParseShellProbe(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		shell   string
		hasStty bool
	}{
		{"bash with stty", "/bin/bash\n/usr/bin/stty\n", "/bin/bash", true},
		{"bash without stty", "/bin/bash\n", "/bin/bash", false},
		{"crlf output", "/bin/bash\r\n/usr/bin/stty\r\n", "/bin/bash", true},
		{"empty SHELL", "\n/usr/bin/stty\n", "", false},
		{"empty output", "", "", false},
		{"shell only no newline", "/bin/zsh", "/bin/zsh", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shell, hasStty := parseShellProbe(tc.out)
			if shell != tc.shell || hasStty != tc.hasStty {
				t.Fatalf("parseShellProbe(%q) = (%q, %v), want (%q, %v)", tc.out, shell, hasStty, tc.shell, tc.hasStty)
			}
		})
	}
}

func TestBuildTwoPhaseCwdHook(t *testing.T) {
	arm, body, ok := buildTwoPhaseCwdHook("/usr/bin/bash")
	if !ok || arm == "" || body == "" {
		t.Fatalf("bash: ok=%v arm=%q body=%q, want both non-empty", ok, arm, body)
	}
	if !strings.Contains(arm, "stty -echo") || !strings.Contains(arm, `\033]7777;e\007`) {
		t.Fatalf("bash arm line missing echo-off or armed marker: %q", arm)
	}
	// The phase-2 body must carry its own echo restore and ready marker so a
	// completed injection always leaves the terminal echo-on.
	if !strings.Contains(body, "stty echo") || !strings.Contains(body, "uniterm-ok") {
		t.Fatalf("bash body missing echo restore or ready marker: %q", body)
	}
	if arm2, body2, ok := buildTwoPhaseCwdHook("/usr/bin/zsh"); !ok || arm2 != arm || body2 == "" {
		t.Fatalf("zsh: ok=%v arm mismatch", ok)
	}
	// fish has no stty-based arm phase and keeps the single-line injection.
	armF, bodyF, ok := buildTwoPhaseCwdHook("/usr/bin/fish")
	if !ok || armF != "" || bodyF == "" {
		t.Fatalf("fish: ok=%v arm=%q, want empty arm and non-empty body", ok, armF)
	}
	if _, _, ok := buildTwoPhaseCwdHook("/bin/ash"); ok {
		t.Fatal("ash must stay unsupported")
	}
}

func TestTypedCwdHookState(t *testing.T) {
	h := newTypedCwdHookState()

	// Single-phase (SSH startup): no arm pending — the arm line never came
	// with a marker, so nothing arms; only the ready marker confirms.
	cleaned, armed, confirmed := h.onOutput([]byte(" stty -echo" + sshCwdHookReadyMarker))
	if armed {
		t.Fatal("single-phase output must not arm")
	}
	if !confirmed {
		t.Fatal("ready marker must confirm")
	}
	if string(cleaned) != " stty -echo" {
		t.Fatalf("marker not stripped: %q", cleaned)
	}
	// The read loop turns the confirmed event into install; the passthrough
	// only starts afterwards.
	h.confirm()
	if !h.isInstalled() {
		t.Fatal("state must be installed after confirm")
	}
	// Passthrough after install: nothing is withheld or reported anymore.
	cleaned, armed, confirmed = h.onOutput([]byte("more" + cwdHookEchoArmedMarker))
	if armed || confirmed || string(cleaned) != "more"+cwdHookEchoArmedMarker {
		t.Fatalf("passthrough broken: cleaned=%q armed=%v confirmed=%v", cleaned, armed, confirmed)
	}
}

func TestTypedCwdHookStateTwoPhase(t *testing.T) {
	h := newTypedCwdHookState()
	h.arm("echo body\n")

	// Before the armed marker nothing arms or confirms.
	cleaned, armed, confirmed := h.onOutput([]byte("prompt "))
	if armed || confirmed || string(cleaned) != "prompt " {
		t.Fatalf("pre-arm output changed: cleaned=%q armed=%v confirmed=%v", cleaned, armed, confirmed)
	}

	// The armed marker split across chunks: the partial first chunk is
	// withheld (cleaned empty), the event fires when the marker completes.
	c1, armed1, confirmed1 := h.onOutput([]byte("\x1b]7777;"))
	c2, armed2, confirmed2 := h.onOutput([]byte("e\x07body output"))
	if armed1 || !armed2 || confirmed1 || confirmed2 {
		t.Fatalf("arm events wrong: armed1=%v armed2=%v confirmed1=%v confirmed2=%v", armed1, armed2, confirmed1, confirmed2)
	}
	if string(c1) != "" || string(c2) != "body output" {
		t.Fatalf("arm marker not stripped: %q %q", c1, c2)
	}
	c3, armed3, confirmed3 := h.onOutput([]byte(sshCwdHookReadyMarker + " more"))
	if armed3 || !confirmed3 || string(c3) != " more" {
		t.Fatalf("ready events wrong: armed3=%v confirmed3=%v cleaned3=%q", armed3, confirmed3, c3)
	}
	if got := h.takeArmedBody(); got != "echo body\n" {
		t.Fatalf("armed body = %q", got)
	}

	// takeArmedBody clears the pending state; a second arm marker never
	// fires again.
	_, armed, _ = h.onOutput([]byte(cwdHookEchoArmedMarker))
	if armed {
		t.Fatal("armed fired twice")
	}
	h.confirm()
	if !h.isInstalled() {
		t.Fatal("confirm did not install")
	}
	h.disarm()
	if h.takeArmedBody() != "" {
		t.Fatal("disarm must drop the body")
	}
}
