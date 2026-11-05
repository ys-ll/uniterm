package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/store"
	"golang.org/x/crypto/ssh"
)

// Only the entry hop can resolve the exit alias. Only the exit hop can
// resolve the destination alias. Direct or single-hop dialing cannot pass.
func sessionChainTestHop(t *testing.T, name, expectedHost, forwardAddr string) int {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if meta.User() != name || string(password) != name+"-password" {
			return nil, fmt.Errorf("wrong credentials")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer raw.Close()
				conn, channels, reqs, err := ssh.NewServerConn(raw, cfg)
				if err != nil {
					return
				}
				defer conn.Close()
				go ssh.DiscardRequests(reqs)
				for request := range channels {
					var dest struct {
						Host       string
						Port       uint32
						OriginHost string
						OriginPort uint32
					}
					if request.ChannelType() != "direct-tcpip" || ssh.Unmarshal(request.ExtraData(), &dest) != nil || dest.Host != expectedHost {
						request.Reject(ssh.ConnectionFailed, "unexpected destination")
						continue
					}
					ch, reqs, err := request.Accept()
					if err != nil {
						continue
					}
					go ssh.DiscardRequests(reqs)
					go func() {
						defer ch.Close()
						if forwardAddr == "" {
							io.Copy(ch, ch)
							return
						}
						target, err := net.DialTimeout("tcp", forwardAddr, time.Second)
						if err != nil {
							return
						}
						defer target.Close()
						go func() { io.Copy(ch, target); ch.Close() }()
						io.Copy(target, ch)
					}()
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestSessionJumpHostChainTraffic(t *testing.T) {
	exitPort := sessionChainTestHop(t, "exit", "target.internal.invalid", "")
	entryPort := sessionChainTestHop(t, "entry", "exit.internal.invalid", net.JoinHostPort("127.0.0.1", strconv.Itoa(exitPort)))
	connections, err := store.NewConnectionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	connections.SetPasswordStore(stubPasswordStore{})
	if err := connections.Save(session.ConnectionStoreData{Connections: []session.ConnectionConfig{
		{ID: "entry", Type: "ssh", Host: "127.0.0.1", Port: entryPort, User: "entry", AuthType: "password", Password: "entry-password"},
		{ID: "exit", Type: "ssh", Host: "exit.internal.invalid", Port: exitPort, User: "exit", AuthType: "password", Password: "exit-password", TunnelSSHConnID: "entry"},
	}}); err != nil {
		t.Fatal(err)
	}
	a := &App{connectionStore: connections, tunnelService: session.NewTunnelService()}
	t.Cleanup(a.tunnelService.Shutdown)
	config := session.ConnectionConfig{Type: "tcp", Host: "target.internal.invalid", Port: 2222, TunnelSSHConnID: "exit"}
	if err := a.setupJumpHostTunnel("test-chain", "tcp", &config); err != nil {
		t.Fatal(err)
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(config.Host, strconv.Itoa(config.Port)), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := io.WriteString(conn, "through-both-hops"); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, len("through-both-hops"))
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "through-both-hops" {
		t.Fatalf("forwarded data = %q, %v", buf, err)
	}
	if config.TunnelTargetHost != "target.internal.invalid" || config.TunnelTargetPort != 2222 {
		t.Fatal("lost original target")
	}
	if _, err := a.TestConnection(session.ConnectionConfig{Type: "tcp", Host: "target.internal.invalid", Port: 2222, TunnelSSHConnID: "exit"}); err != nil {
		t.Fatalf("test connection uses a different route: %v", err)
	}
}

func TestSessionJumpHostChainCredentials(t *testing.T) {
	connections, err := store.NewConnectionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	connections.SetPasswordStore(stubPasswordStore{})
	identities, err := store.NewIdentityStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identities.SetPasswordStore(stubPasswordStore{})
	if err := identities.Save(session.IdentityStoreData{Identities: []session.Identity{{ID: "identity", Username: "identity-user", AuthType: "password", Password: "identity-password"}}}); err != nil {
		t.Fatal(err)
	}
	a := &App{connectionStore: connections, identityStore: identities}
	for _, identity := range []bool{false, true} {
		t.Run(fmt.Sprintf("identity=%v", identity), func(t *testing.T) {
			authType := "password"
			if identity {
				authType = "identity"
			}
			if err := connections.Save(session.ConnectionStoreData{Connections: []session.ConnectionConfig{
				{ID: "entry", Type: "ssh", AuthType: authType, IdentityId: "identity"},
				{ID: "exit", Type: "ssh", User: "saved-user", Password: "saved-password", AuthType: "password", TunnelSSHConnID: "entry", TunnelSSHUser: "entry-user", TunnelSSHPassword: "entry-password"},
			}}); err != nil {
				t.Fatal(err)
			}
			chain, err := a.sessionJumpHostChain(session.ConnectionConfig{TunnelSSHConnID: "exit", TunnelSSHUser: "temporary", TunnelSSHPassword: "temporary"})
			if err != nil {
				t.Fatal(err)
			}
			wantUser, wantPassword := "entry-user", "entry-password"
			if identity {
				wantUser, wantPassword = "identity-user", "identity-password"
			}
			if chain[0].User != wantUser || chain[0].Password != wantPassword {
				t.Fatal("parent credentials resolved incorrectly")
			}
			if chain[1].User != "saved-user" || chain[1].Password != "saved-password" {
				t.Fatal("temporary credentials replaced saved credentials")
			}
		})
	}
}
