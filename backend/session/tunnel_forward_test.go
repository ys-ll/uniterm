package session

import (
	"io"
	"net"
	"testing"
)

func TestBuildHopChain(t *testing.T) {
	conns := map[string]ConnectionConfig{
		"exit": {ID: "exit", Host: "exit.h", Port: 22, TunnelSSHConnID: "edge"},
		"edge": {ID: "edge", Host: "edge.h", Port: 22},
		"solo": {ID: "solo", Host: "solo.h", Port: 22},
		"a":    {ID: "a", Host: "a.h", Port: 22, TunnelSSHConnID: "b"},
		"b":    {ID: "b", Host: "b.h", Port: 22, TunnelSSHConnID: "a"},
	}
	resolve := func(id string) (ConnectionConfig, bool) { c, ok := conns[id]; return c, ok }

	// Multi-hop: outermost first.
	chain, err := buildHopChain("exit", resolve)
	if err != nil {
		t.Fatalf("multi-hop: %v", err)
	}
	if len(chain) != 2 || chain[0].ID != "edge" || chain[1].ID != "exit" {
		t.Fatalf("multi-hop chain = %v, want [edge exit]", ids(chain))
	}

	// Single hop (no jump) → direct to exit.
	chain, err = buildHopChain("solo", resolve)
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	if len(chain) != 1 || chain[0].ID != "solo" {
		t.Fatalf("single chain = %v, want [solo]", ids(chain))
	}

	// Loop → error.
	if _, err := buildHopChain("a", resolve); err == nil {
		t.Fatal("loop: expected error, got nil")
	}

	// Missing connection → error.
	if _, err := buildHopChain("nope", resolve); err == nil {
		t.Fatal("missing: expected error, got nil")
	}

	// Empty exit → error.
	if _, err := buildHopChain("", resolve); err == nil {
		t.Fatal("empty: expected error, got nil")
	}
}

func ids(cs []ConnectionConfig) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func TestSocks5Handshake(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()

	go func() {
		// Greeting: version 5, 1 method, no-auth.
		client.Write([]byte{0x05, 0x01, 0x00})
		reply := make([]byte, 2)
		io.ReadFull(client, reply) // 0x05 0x00
		// CONNECT to a domain "example.com:443".
		host := "example.com"
		req := []byte{0x05, 0x01, 0x00, 0x03, byte(len(host))}
		req = append(req, []byte(host)...)
		req = append(req, 0x01, 0xBB) // port 443
		client.Write(req)
	}()

	got, err := socks5Handshake(server)
	if err != nil {
		t.Fatalf("socks5Handshake: %v", err)
	}
	if got != "example.com:443" {
		t.Fatalf("target = %q, want %q", got, "example.com:443")
	}
}
