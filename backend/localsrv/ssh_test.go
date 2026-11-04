package localsrv

import (
	"encoding/binary"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// freePort grabs an ephemeral TCP port for a test server. There is an
// inherent TOCTOU window between close and reuse; acceptable for tests.
func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestSSHServer_RejectsEmptyCredentials(t *testing.T) {
	m := NewManager()
	if _, err := m.StartSSHServer(SSHConfig{RootDir: t.TempDir(), Port: freePort(t)}); err == nil {
		t.Fatal("expected error for empty credentials, got nil")
	}
	if _, err := m.StartSSHServer(SSHConfig{RootDir: t.TempDir(), Port: freePort(t), Username: "u"}); err == nil {
		t.Fatal("expected error for empty password, got nil")
	}
	if _, err := m.StartSSHServer(SSHConfig{RootDir: t.TempDir(), Port: freePort(t), Password: "p"}); err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
}

func TestTelnetServer_RejectsEmptyPassword(t *testing.T) {
	m := NewManager()
	if _, err := m.StartTelnetServer(TelnetConfig{Port: freePort(t)}); err == nil {
		t.Fatal("expected error for empty password, got nil")
	}
}

// dialTestSSH authenticates and returns a live client connection.
func dialTestSSH(t *testing.T, addr, user, pass string) *ssh.Client {
	t.Helper()
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		t.Fatalf("ssh.Dial: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// ptyReqPayload builds a pty-req / window-change payload: string(term) then
// cols, rows, width px, height px as uint32s.
func ptyReqPayload(term string, cols, rows uint32) []byte {
	p := make([]byte, 0, 4+len(term)+16)
	p = binary.BigEndian.AppendUint32(p, uint32(len(term)))
	p = append(p, term...)
	p = binary.BigEndian.AppendUint32(p, cols)
	p = binary.BigEndian.AppendUint32(p, rows)
	p = binary.BigEndian.AppendUint32(p, 0) // width px
	p = binary.BigEndian.AppendUint32(p, 0) // height px
	return p
}

// TestSSHServer_PtyReqDoesNotCloseChannel verifies the #1381 regression: a
// pty-req followed by an exec on the same channel used to abort at pty-req
// (the shared branch closed the channel), surfacing to clients as
// "shell: EOF". The channel must stay open and serve subsequent requests.
func TestSSHServer_PtyReqDoesNotCloseChannel(t *testing.T) {
	m := NewManager()
	port := freePort(t)
	id, err := m.StartSSHServer(SSHConfig{
		RootDir:  t.TempDir(),
		Port:     port,
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("StartSSHServer: %v", err)
	}
	defer m.Stop(id)

	client := dialTestSSH(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "user", "pass")
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if err := session.RequestPty("xterm-256color", 40, 120, nil); err != nil {
		t.Fatalf("RequestPty: %v — channel must stay open after pty-req", err)
	}
	out, err := session.CombinedOutput("ls")
	if err != nil {
		t.Fatalf("exec after pty-req: %v", err)
	}
	if want := "command execution is not supported"; !strings.Contains(string(out), want) {
		t.Fatalf("exec output = %q, want it to contain %q", out, want)
	}
}

// TestSSHServer_ShellStartsLocalShell connects, requests a PTY and a shell,
// and expects the local shell's prompt output to arrive on the channel. On
// Unix this exercises the creack/pty path; on Windows the ConPTY path.
func TestSSHServer_ShellStartsLocalShell(t *testing.T) {
	m := NewManager()
	port := freePort(t)
	id, err := m.StartSSHServer(SSHConfig{
		RootDir:  t.TempDir(),
		Port:     port,
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("StartSSHServer: %v", err)
	}
	defer m.Stop(id)

	client := dialTestSSH(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "user", "pass")
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer session.Close()

	if err := session.RequestPty("xterm-256color", 40, 120, nil); err != nil {
		t.Fatalf("RequestPty: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}
	if err := session.Shell(); err != nil {
		t.Fatalf("Shell: %v", err)
	}

	// Collect output while typing a command; the shell must echo and answer.
	var got []byte
	done := make(chan struct{})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdout.Read(buf)
			got = append(got, buf[:n]...)
			if err != nil {
				close(done)
				return
			}
		}
	}()

	if _, err := stdin.Write([]byte("echo uniterm_ssh_test\r\n")); err != nil {
		t.Fatalf("stdin write: %v", err)
	}

	deadline := time.After(15 * time.Second)
	for !strings.Contains(string(got), "uniterm_ssh_test") {
		select {
		case <-done:
			t.Fatalf("shell closed before marker output; got %q", got)
		case <-deadline:
			t.Fatalf("timed out waiting for marker output; got %q", got)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// startTestSFTPClient starts the local SSH server and connects a real SFTP
// client over the sftp subsystem.
func startTestSFTPClient(t *testing.T) (*sftp.Client, func()) {
	t.Helper()
	m := NewManager()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "emptydir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "fulldir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fulldir", "nested.txt"), []byte("nested"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	port := freePort(t)
	id, err := m.StartSSHServer(SSHConfig{
		RootDir:  root,
		Port:     port,
		Username: "user",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("StartSSHServer: %v", err)
	}
	stop := func() { m.Stop(id) }

	client := dialTestSSH(t, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), "user", "pass")
	session, err := client.NewSession()
	if err != nil {
		stop()
		t.Fatalf("NewSession: %v", err)
	}
	pipe, err := session.StdoutPipe()
	if err != nil {
		stop()
		t.Fatalf("StdoutPipe: %v", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		stop()
		t.Fatalf("StdinPipe: %v", err)
	}
	if err := session.RequestSubsystem("sftp"); err != nil {
		stop()
		t.Fatalf("RequestSubsystem: %v", err)
	}
	sftpClient, err := sftp.NewClientPipe(pipe, stdin)
	if err != nil {
		stop()
		t.Fatalf("sftp.NewClientPipe: %v", err)
	}
	return sftpClient, func() {
		sftpClient.Close()
		session.Close()
		stop()
	}
}

// TestSFTP_StatDirectory verifies the SFTP Stat path: Stat must report the
// directory's own attributes. The server used to enumerate a directory's
// contents for Stat, which made Stat of an empty directory fail entirely and
// Stat of a non-empty directory report its first child — breaking directory
// navigation in the file browser.
func TestSFTP_StatDirectory(t *testing.T) {
	cl, stop := startTestSFTPClient(t)
	defer stop()

	for _, dir := range []string{"emptydir", "fulldir"} {
		fi, err := cl.Stat("/" + dir)
		if err != nil {
			t.Fatalf("Stat /%s: %v", dir, err)
		}
		if !fi.IsDir() {
			t.Fatalf("Stat /%s: mode %v, want a directory", dir, fi.Mode())
		}
	}
	fi, err := cl.Stat("/file.txt")
	if err != nil {
		t.Fatalf("Stat /file.txt: %v", err)
	}
	if fi.IsDir() {
		t.Fatal("Stat /file.txt: unexpected directory mode")
	}
	entries, err := cl.ReadDir("/")
	if err != nil {
		t.Fatalf("ReadDir /: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("ReadDir / returned %d entries, want 3", len(entries))
	}
}
