package session

import (
	"fmt"
	"io"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Require an interactive password and OTP. A companion cannot authenticate
// again from the source config, which deliberately contains no credentials.
func fileTestSSHSource(t *testing.T, rejectSFTP bool) (*SSHSession, *atomic.Int32) {
	t.Helper()
	var authentications atomic.Int32
	config := &ssh.ServerConfig{KeyboardInteractiveCallback: func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		authentications.Add(1)
		answers, err := challenge("", "", []string{"Password:", "OTP:"}, []bool{false, false})
		if err != nil {
			return nil, err
		}
		if len(answers) != 2 || answers[0] != "password" || answers[1] != "123456" {
			return nil, fmt.Errorf("invalid answers")
		}
		return nil, nil
	}}
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							switch request.Type {
							case "subsystem":
								if rejectSFTP {
									request.Reply(false, nil)
									continue
								}
								request.Reply(true, nil)
								server, err := sftp.NewServer(channel)
								if err != nil {
									return
								}
								defer server.Close()
								_ = server.Serve()
								return
							case "exec":
								request.Reply(true, nil)
								_, _ = io.WriteString(channel, "/\n")
								_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							default:
								request.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{
		User: "tester", Timeout: 5 * time.Second, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Auth: []ssh.AuthMethod{ssh.KeyboardInteractive(func(string, string, []string, []bool) ([]string, error) {
			return []string{"password", "123456"}, nil
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := NewSSHSession("interactive-source")
	source.clientRef = newSSHClientRef(client)
	source.setStatus(StatusConnected)
	t.Cleanup(func() { source.Disconnect() })
	return source, &authentications
}

func TestSFTPConnectSSHInteractiveTransfer(t *testing.T) {
	source, authentications := fileTestSSHSource(t, false)
	files := NewSFTPSession("files")
	if err := files.ConnectSSH(source); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { files.Disconnect() })
	remote := filepath.Join(t.TempDir(), "transfer.txt")
	if err := files.PutContent(remote, []byte("uploaded over authenticated SSH")); err != nil {
		t.Fatal(err)
	}
	content, err := files.GetContent(remote)
	if err != nil || string(content) != "uploaded over authenticated SSH" {
		t.Fatalf("download = %q, %v", content, err)
	}
	if authentications.Load() != 1 {
		t.Fatalf("authenticated %d times", authentications.Load())
	}
	files.Disconnect()
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
	source.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 0 {
		t.Fatalf("remaining refs after final disconnect = %d, want 0", refs)
	}
	if channel, err := source.clientRef.client.NewSession(); err == nil {
		channel.Close()
		t.Fatal("transport survived final disconnect")
	}
}

func TestSFTPConnectSSHFailurePreservesTerminal(t *testing.T) {
	source, _ := fileTestSSHSource(t, true)
	files := NewSFTPSession("unsupported-sftp")
	if err := files.ConnectSSH(source); err == nil {
		t.Fatal("expected subsystem failure")
	}
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
}

func TestSCPConnectSSHInteractive(t *testing.T) {
	source, authentications := fileTestSSHSource(t, false)
	files := NewSCPSession("scp-files")
	if err := files.ConnectSSH(source); err != nil {
		t.Fatal(err)
	}
	if _, err := files.runCommand("pwd", time.Second); err != nil {
		t.Fatal(err)
	}
	if authentications.Load() != 1 {
		t.Fatalf("authenticated %d times", authentications.Load())
	}
	files.Disconnect()
	files.Disconnect()
	if refs := source.clientRef.refs.Load(); refs != 1 {
		t.Fatalf("remaining refs = %d, want 1", refs)
	}
	assertFileTestSSHAlive(t, source)
}

func TestFileConnectSSHRequiresConnectedSource(t *testing.T) {
	source := NewSSHSession("disconnected")
	if err := NewSFTPSession("sftp").ConnectSSH(source); err == nil {
		t.Fatal("SFTP accepted disconnected source")
	}
	if err := NewSCPSession("scp").ConnectSSH(source); err == nil {
		t.Fatal("SCP accepted disconnected source")
	}
}

func assertFileTestSSHAlive(t *testing.T, source *SSHSession) {
	t.Helper()
	channel, err := source.clientRef.client.NewSession()
	if err != nil {
		t.Fatalf("terminal transport closed: %v", err)
	}
	defer channel.Close()
	if _, err := channel.Output("pwd"); err != nil {
		t.Fatalf("terminal exec: %v", err)
	}
}
