package main

import (
	"testing"
)

func TestParseLaunchArgs(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want LaunchOptions
	}{
		{
			name: "empty",
			args: nil,
			want: LaunchOptions{},
		},
		{
			name: "tray and mcp flags",
			args: []string{"--tray", "--start-mcp"},
			want: LaunchOptions{StartInTray: true, StartMCP: true},
		},
		{
			name: "minimized and mcp aliases case-insensitive",
			args: []string{"--MINIMIZED", "--MCP"},
			want: LaunchOptions{StartInTray: true, StartMCP: true},
		},
		{
			name: "deep link url with unrelated args",
			args: []string{"--unknown", "uniterm://mcp/start?tray=1", "uniterm://open?id=second"},
			want: LaunchOptions{DeepLinkURL: "uniterm://mcp/start?tray=1"},
		},
		{
			name: "uppercase scheme preserved",
			args: []string{"UNITERM://open?id=abc"},
			want: LaunchOptions{DeepLinkURL: "UNITERM://open?id=abc"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLaunchArgs(tc.args)
			if got != tc.want {
				t.Fatalf("parseLaunchArgs(%q) = %+v, want %+v", tc.args, got, tc.want)
			}
		})
	}
}

func TestParseDeepLink(t *testing.T) {
	valid := []struct {
		raw  string
		want DeepLinkAction
	}{
		{"uniterm://", DeepLinkAction{Action: "start"}},
		{"uniterm://start", DeepLinkAction{Action: "start"}},
		{"uniterm://start?tray=1", DeepLinkAction{Action: "start", Tray: true}},
		{"uniterm://show?minimized=true", DeepLinkAction{Action: "start", Tray: true}},
		{"uniterm://mcp", DeepLinkAction{Action: "mcp-start"}},
		{"uniterm://mcp/start", DeepLinkAction{Action: "mcp-start"}},
		{"uniterm://mcp/start?tray=1", DeepLinkAction{Action: "mcp-start", Tray: true}},
		{"uniterm://mcp/settings", DeepLinkAction{Action: "settings", Section: "mcp"}},
		{"uniterm://open?id=conn-123", DeepLinkAction{Action: "open", ConnectionID: "conn-123"}},
		{"uniterm://open/conn-456", DeepLinkAction{Action: "open", ConnectionID: "conn-456"}},
		{"uniterm://connect?id=conn-789", DeepLinkAction{Action: "open", ConnectionID: "conn-789"}},
		{"uniterm://connect/conn-000", DeepLinkAction{Action: "open", ConnectionID: "conn-000"}},
		{"uniterm://settings", DeepLinkAction{Action: "settings"}},
		{"uniterm://settings/mcp", DeepLinkAction{Action: "settings", Section: "mcp"}},
		{"uniterm://settings?section=about", DeepLinkAction{Action: "settings", Section: "about"}},
		{"UNITERM://MCP/START?minimized=yes", DeepLinkAction{Action: "mcp-start", Tray: true}},
	}
	for _, tc := range valid {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := parseDeepLink(tc.raw)
			if err != nil {
				t.Fatalf("parseDeepLink(%q) unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("parseDeepLink(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}

	invalid := []string{
		"",
		"https://example.com",
		"ssh://root@127.0.0.1",
		"uniterm://open",
		"uniterm://connect",
		"uniterm://exec?cmd=whoami",
		"uniterm://mcp/exec",
		"uniterm://unknown/path",
	}
	for _, raw := range invalid {
		t.Run("invalid_"+raw, func(t *testing.T) {
			if got, err := parseDeepLink(raw); err == nil {
				t.Fatalf("parseDeepLink(%q) = %+v, want error", raw, got)
			}
		})
	}
}

func TestApplyLaunchOptions(t *testing.T) {
	t.Run("mcp start in tray", func(t *testing.T) {
		a := NewApp("")
		a.applyLaunchOptions(parseLaunchArgs([]string{"uniterm://mcp/start?tray=1"}))
		if !a.startHidden || !a.forceStartMCP {
			t.Fatalf("got startHidden=%v forceStartMCP=%v, want true/true", a.startHidden, a.forceStartMCP)
		}
		if a.pendingDeepLink != nil {
			t.Fatalf("unexpected pendingDeepLink: %+v", a.pendingDeepLink)
		}
	})

	t.Run("open connection overrides tray and queues pending action", func(t *testing.T) {
		a := NewApp("")
		a.applyLaunchOptions(parseLaunchArgs([]string{"--tray", "uniterm://open?id=c1"}))
		if a.startHidden {
			t.Fatal("open deep link should show main window")
		}
		if a.pendingDeepLink == nil || a.pendingDeepLink.Action != "open" || a.pendingDeepLink.ConnectionID != "c1" {
			t.Fatalf("pendingDeepLink = %+v, want open c1", a.pendingDeepLink)
		}
	})

	t.Run("cli tray and mcp flags", func(t *testing.T) {
		a := NewApp("")
		a.applyLaunchOptions(parseLaunchArgs([]string{"--minimized", "--start-mcp"}))
		if !a.startHidden || !a.forceStartMCP {
			t.Fatalf("got startHidden=%v forceStartMCP=%v, want true/true", a.startHidden, a.forceStartMCP)
		}
	})
}

func TestDeepLinkQueueAndFlush(t *testing.T) {
	a := NewApp("")
	// Before frontend signals ready, dispatch queues the action.
	a.dispatchDeepLinkToFrontend(DeepLinkAction{Action: "open", ConnectionID: "c42"})
	if a.pendingDeepLink == nil || a.pendingDeepLink.ConnectionID != "c42" {
		t.Fatalf("expected queued action c42, got %+v", a.pendingDeepLink)
	}
	// markDeepLinkReady flips ready=true and drains pendingDeepLink.
	a.markDeepLinkReady()
	if !a.deepLinkReady {
		t.Fatal("expected deepLinkReady=true")
	}
	if a.pendingDeepLink != nil {
		t.Fatalf("expected pendingDeepLink drained, got %+v", a.pendingDeepLink)
	}
}
