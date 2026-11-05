package localsrv

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func startForwardTestSSH(t *testing.T) (*Manager, string, string) {
	t.Helper()
	m := NewManager()
	port := freePort(t)
	id, err := m.StartSSHServer(SSHConfig{RootDir: t.TempDir(), Port: port, Username: "user", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Stop(id) })
	return m, id, net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
}

func TestSSHForwardJumpHost(t *testing.T) {
	_, _, jumpAddr := startForwardTestSSH(t)
	_, _, targetAddr := startForwardTestSSH(t)
	jump := dialTestSSH(t, jumpAddr, "user", "pass")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	raw, err := jump.DialContext(ctx, "tcp", targetAddr)
	if err != nil {
		t.Fatalf("dial through jump host: %v", err)
	}
	defer raw.Close()
	// Bound the inner handshake, which runs over an SSH channel without deadlines.
	stopClose := context.AfterFunc(ctx, func() { raw.Close() })
	defer stopClose()
	conn, chans, reqs, err := ssh.NewClientConn(raw, targetAddr, &ssh.ClientConfig{
		User: "user", Auth: []ssh.AuthMethod{ssh.Password("pass")}, HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	})
	if err != nil {
		t.Fatalf("target SSH handshake: %v", err)
	}
	target := ssh.NewClient(conn, chans, reqs)
	defer target.Close()
	session, err := target.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if _, err := session.CombinedOutput("test"); err != nil {
		t.Fatalf("target session through jump host: %v", err)
	}
}

func TestSSHForwardHalfClose(t *testing.T) {
	_, _, addr := startForwardTestSSH(t)
	client := dialTestSSH(t, addr, "user", "pass")
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		data, err := io.ReadAll(conn)
		if err == nil {
			_, err = conn.Write(append([]byte("reply:"), data...))
		}
		serverDone <- err
	}()
	ch, reqs, err := client.OpenChannel("direct-tcpip", ssh.Marshal(directTCPIPRequest{
		Host: "127.0.0.1", Port: uint32(ln.Addr().(*net.TCPAddr).Port), OriginHost: "127.0.0.1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()
	go ssh.DiscardRequests(reqs)
	if _, err := io.WriteString(ch, "hello"); err != nil {
		t.Fatal(err)
	}
	if err := ch.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(ch)
	if err != nil || string(data) != "reply:hello" {
		t.Fatalf("response after half-close = %q, %v", data, err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
}

func TestSSHForwardCleanup(t *testing.T) {
	for _, action := range []string{"channel", "client", "server"} {
		t.Run(action, func(t *testing.T) {
			m, id, addr := startForwardTestSSH(t)
			client := dialTestSSH(t, addr, "user", "pass")
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer ln.Close()
			forward, err := client.Dial("tcp", ln.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer forward.Close()
			_ = ln.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
			target, err := ln.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer target.Close()
			switch action {
			case "channel":
				forward.Close()
			case "client":
				client.Close()
			case "server":
				m.Stop(id)
			}
			_ = target.SetReadDeadline(time.Now().Add(5 * time.Second))
			if _, err := target.Read(make([]byte, 1)); err != io.EOF {
				t.Fatalf("forwarded connection remains open: %v", err)
			}
		})
	}
}

func TestSSHForwardRejectsInvalidDestination(t *testing.T) {
	_, _, addr := startForwardTestSSH(t)
	client := dialTestSSH(t, addr, "user", "pass")
	for _, payload := range [][]byte{
		{0},
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: 0}),
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: 65536}),
		ssh.Marshal(directTCPIPRequest{Port: 22}),
		ssh.Marshal(directTCPIPRequest{Host: "127.0.0.1", Port: uint32(freePort(t))}),
	} {
		ch, _, err := client.OpenChannel("direct-tcpip", payload)
		if err == nil {
			ch.Close()
			t.Fatal("invalid or unreachable destination accepted")
		}
		var openErr *ssh.OpenChannelError
		if !errors.As(err, &openErr) || openErr.Reason != ssh.ConnectionFailed {
			t.Fatalf("expected ConnectionFailed, got %v", err)
		}
	}
	if session, err := client.NewSession(); err != nil {
		t.Fatalf("failed forwarding damaged SSH connection: %v", err)
	} else {
		session.Close()
	}
}
