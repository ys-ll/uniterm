package session

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestBuildHopChain(t *testing.T) {
	for _, tc := range []struct {
		name        string
		connections map[string]ConnectionConfig
		wantErr     string
	}{
		{"ordered", map[string]ConnectionConfig{"b": {ID: "b", Type: "ssh", TunnelSSHConnID: "a"}, "a": {ID: "a", Type: "ssh"}}, ""},
		{"missing", map[string]ConnectionConfig{"b": {ID: "b", TunnelSSHConnID: "a"}}, "a not found"},
		{"cycle", map[string]ConnectionConfig{"b": {ID: "b", TunnelSSHConnID: "a"}, "a": {ID: "a", TunnelSSHConnID: "b"}}, "loop"},
		{"wrong type", map[string]ConnectionConfig{"b": {ID: "b", Type: "rdp"}}, "not an SSH connection"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chain, err := BuildHopChain("b", func(id string) (ConnectionConfig, bool) { c, ok := tc.connections[id]; return c, ok })
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil || len(chain) != 2 || chain[0].ID != "a" || chain[1].ID != "b" {
				t.Fatalf("chain = %+v, %v", chain, err)
			}
		})
	}
}

func TestSessionTunnelChain(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		t.Run(fmt.Sprintf("shutdown=%v", shutdown), func(t *testing.T) {
			var chain []ConnectionConfig
			var connected []<-chan *ssh.ServerConn
			for _, name := range []string{"entry", "exit"} {
				cfg, conns := tunnelTestServerConfig(t, &ssh.ServerConfig{PasswordCallback: func(meta ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
					if meta.User() != name || string(password) != name+"-password" {
						return nil, fmt.Errorf("wrong hop credentials")
					}
					return nil, nil
				}})
				cfg.ID, cfg.User, cfg.Password = name, name, name+"-password"
				chain = append(chain, cfg)
				connected = append(connected, conns)
			}
			echo, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer echo.Close()
			go func() {
				c, err := echo.Accept()
				if err == nil {
					defer c.Close()
					io.Copy(c, c)
				}
			}()
			ts := NewTunnelService()
			defer ts.Shutdown()
			port, err := ts.StartChain("session", chain, "127.0.0.1", echo.Addr().(*net.TCPAddr).Port, nil)
			if err != nil {
				t.Fatal(err)
			}
			var servers []*ssh.ServerConn
			for _, conns := range connected {
				servers = append(servers, tunnelTestReceive(t, conns))
			}
			local, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			defer local.Close()
			_ = local.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err := io.WriteString(local, "chain"); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 5)
			if _, err := io.ReadFull(local, buf); err != nil || string(buf) != "chain" {
				t.Fatalf("echo = %q, %v", buf, err)
			}
			if shutdown {
				ts.Shutdown()
			} else {
				ts.Stop("session")
			}
			for _, server := range servers {
				done := make(chan struct{})
				go func() { server.Wait(); close(done) }()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Fatal("hop survived tunnel shutdown")
				}
			}
			if _, err := local.Read(buf); err == nil {
				t.Fatal("forwarded connection survived shutdown")
			}
		})
	}
}

func TestSessionTunnelChainFailedHopClosesEarlierHops(t *testing.T) {
	entry, connected := tunnelTestServer(t)
	exit, _ := tunnelTestServerConfig(t, &ssh.ServerConfig{PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, fmt.Errorf("rejected") }})
	ts := NewTunnelService()
	defer ts.Shutdown()
	if _, err := ts.StartChain("session", []ConnectionConfig{entry, exit}, "127.0.0.1", 22, nil); err == nil {
		t.Fatal("expected failed hop authentication")
	}
	server := tunnelTestReceive(t, connected)
	done := make(chan struct{})
	go func() { server.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("earlier hop leaked after failed authentication")
	}
	if len(ts.tunnels) != 0 {
		t.Fatal("failed chain registered a session tunnel")
	}
}
