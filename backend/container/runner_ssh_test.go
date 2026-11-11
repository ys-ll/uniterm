package container

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// 编译期接口合规断言：SSHRunner 必须实现冻结的 Runner 接口。
var _ Runner = (*SSHRunner)(nil)

// JoinShellCommand 已在 Task 2 测；这里测 detectArgs 经 shell 的形态。
func TestDetectArgs(t *testing.T) {
	got := JoinShellCommand(detectArgs(RuntimePodman))
	want := "sh -c 'command -v podman'"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Exercise the real SSH transport: login must send the password and EOF on
// stdin, keep it out of the command, and preserve registry error messages.
func TestSSHRunnerRegistryLogin(t *testing.T) {
	for _, stderr := range []string{"", "unauthorized: authentication required"} {
		name := "success"
		if stderr != "" {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			type request struct{ command, stdin string }
			received := make(chan request, 1)
			client := newRunnerTestSSHClient(t, func(ch ssh.Channel, command string) {
				data, err := io.ReadAll(ch)
				if err != nil {
					t.Errorf("read stdin: %v", err)
				}
				received <- request{command, string(data)}
				status := uint32(0)
				if stderr != "" {
					status = 1
					_, _ = io.WriteString(ch.Stderr(), stderr+"\n")
				}
				_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
			})
			p := NewProvider(RuntimeDocker, "", NewSSHRunner(client))
			err := p.Login(context.Background(), LoginOptions{
				Registry: "harbor.example", Username: "admin", Password: "test secret ' $()",
			})
			if stderr == "" && err != nil {
				t.Fatalf("login: %v", err)
			}
			if stderr != "" && (err == nil || err.Error() != stderr) {
				t.Fatalf("login error = %v, want %q", err, stderr)
			}
			select {
			case got := <-received:
				// SSHRunner prepends a PATH extension (non-interactive sshd
				// PATH may miss /usr/local/bin, e.g. Synology DSM); the
				// docker command itself must stay unchanged after it.
				if !strings.HasSuffix(got.command, "docker login --password-stdin -u admin harbor.example") ||
					!strings.HasPrefix(got.command, "PATH=$PATH:") {
					t.Errorf("command = %q", got.command)
				}
				if got.stdin != "test secret ' $()\n" {
					t.Errorf("stdin = %q", got.stdin)
				}
			case <-time.After(time.Second):
				t.Fatal("server did not receive stdin EOF")
			}
		})
	}
}

func newRunnerTestSSHClient(t *testing.T, execute func(ssh.Channel, string)) *ssh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	done := make(chan struct{})
	t.Cleanup(func() { <-done })
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		for incoming := range channels {
			if incoming.ChannelType() != "session" {
				_ = incoming.Reject(ssh.UnknownChannelType, "session required")
				continue
			}
			ch, requests, err := incoming.Accept()
			if err != nil {
				return
			}
			func() {
				defer ch.Close()
				for req := range requests {
					var payload struct{ Command string }
					if req.Type != "exec" || ssh.Unmarshal(req.Payload, &payload) != nil {
						_ = req.Reply(false, nil)
						continue
					}
					_ = req.Reply(true, nil)
					execute(ch, payload.Command)
					return
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	})
	if err != nil {
		// Unblock Accept if dialing failed before establishing a connection.
		_ = listener.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}
