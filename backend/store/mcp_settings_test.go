package store

import "testing"

func TestMCPRemoteAccessSetting(t *testing.T) {
	if DefaultMCPSettings().AllowRemote {
		t.Fatal("remote access must default to off")
	}
	s := NewLocalStateStore(t.TempDir())
	for _, allow := range []bool{true, false} {
		if err := s.Save(LocalState{MCP: &MCPSettings{Enabled: true, AllowRemote: allow}}); err != nil {
			t.Fatal(err)
		}
		state, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if state.MCP == nil || state.MCP.AllowRemote != allow {
			t.Fatalf("allowRemote=%v did not round-trip: %+v", allow, state.MCP)
		}
	}
}

func TestMCPConcurrencySettingRoundTrip(t *testing.T) {
	if DefaultMCPSettings().MaxConcurrentCommands != 8 {
		t.Fatal("concurrency default must remain 8")
	}
	s := NewLocalStateStore(t.TempDir())
	// Zero represents an older config with no concurrency field.
	for _, limit := range []int{0, 1, 16, 64} {
		if err := s.Save(LocalState{MCP: &MCPSettings{MaxConcurrentCommands: limit}}); err != nil {
			t.Fatal(err)
		}
		state, err := s.Load()
		if err != nil {
			t.Fatal(err)
		}
		if state.MCP == nil || state.MCP.MaxConcurrentCommands != limit {
			t.Fatalf("limit %d did not round-trip: %+v", limit, state.MCP)
		}
	}
}
