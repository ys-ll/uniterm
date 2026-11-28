package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/ys-ll/uniterm/backend/log"
)

const deepLinkScheme = "uniterm"

// LaunchOptions holds parsed CLI flags and deep-link arguments from either
// initial startup (os.Args) or a second-instance handoff.
type LaunchOptions struct {
	StartInTray bool
	StartMCP    bool
	DeepLinkURL string
}

// DeepLinkAction is a validated, safe action decoded from a uniterm:// URL.
// External URLs are deliberately limited to non-destructive UI / lifecycle
// operations: they never execute shell commands or bypass MCP approval gates.
type DeepLinkAction struct {
	Action       string `json:"action"` // "start" | "mcp-start" | "open" | "settings"
	ConnectionID string `json:"connectionId,omitempty"`
	Section      string `json:"section,omitempty"`
	Tray         bool   `json:"tray,omitempty"`
}

// parseLaunchArgs extracts supported CLI flags and the first uniterm:// URL
// from a raw argument slice (os.Args[1:] or SecondInstanceData.Args).
func parseLaunchArgs(args []string) LaunchOptions {
	var opts LaunchOptions
	for _, raw := range args {
		arg := strings.TrimSpace(raw)
		if arg == "" {
			continue
		}
		lower := strings.ToLower(arg)
		switch lower {
		case "--minimized", "--tray":
			opts.StartInTray = true
		case "--start-mcp", "--mcp":
			opts.StartMCP = true
		default:
			if opts.DeepLinkURL == "" && strings.HasPrefix(lower, deepLinkScheme+"://") {
				opts.DeepLinkURL = arg
			}
		}
	}
	return opts
}

// parseDeepLink validates and normalizes a uniterm:// URL into a safe action.
//
// Supported forms:
//   - uniterm:// or uniterm://start[?tray=1|minimized=1]
//   - uniterm://mcp or uniterm://mcp/start[?tray=1|minimized=1]
//   - uniterm://mcp/settings
//   - uniterm://open?id=<connId> or uniterm://open/<connId>
//   - uniterm://connect?id=<connId> or uniterm://connect/<connId>
//   - uniterm://settings[?section=<name>] or uniterm://settings/<name>
func parseDeepLink(raw string) (DeepLinkAction, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return DeepLinkAction{}, fmt.Errorf("empty deep link URL")
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return DeepLinkAction{}, fmt.Errorf("invalid deep link URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, deepLinkScheme) {
		return DeepLinkAction{}, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}

	q := u.Query()
	tray := isTruthyQueryParam(q.Get("tray")) || isTruthyQueryParam(q.Get("minimized"))
	host := strings.ToLower(strings.TrimSpace(u.Host))
	path := strings.Trim(u.Path, "/")
	pathLower := strings.ToLower(path)

	switch host {
	case "", "start", "show":
		return DeepLinkAction{Action: "start", Tray: tray}, nil

	case "mcp":
		switch pathLower {
		case "", "start":
			return DeepLinkAction{Action: "mcp-start", Tray: tray}, nil
		case "settings":
			return DeepLinkAction{Action: "settings", Section: "mcp"}, nil
		default:
			return DeepLinkAction{}, fmt.Errorf("unsupported mcp deep link path %q", path)
		}

	case "open", "connect":
		id := strings.TrimSpace(q.Get("id"))
		if id == "" {
			id = strings.TrimSpace(path)
		}
		if id == "" {
			return DeepLinkAction{}, fmt.Errorf("missing connection id in %q", raw)
		}
		return DeepLinkAction{Action: "open", ConnectionID: id}, nil

	case "settings":
		section := strings.TrimSpace(q.Get("section"))
		if section == "" {
			section = strings.TrimSpace(path)
		}
		return DeepLinkAction{Action: "settings", Section: section}, nil

	default:
		return DeepLinkAction{}, fmt.Errorf("unsupported deep link host %q", host)
	}
}

func isTruthyQueryParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// applyLaunchOptions applies cold-start CLI flags and deep-link URLs to App
// before the Wails window is created and stores are initialized.
func (a *App) applyLaunchOptions(opts LaunchOptions) {
	if opts.StartInTray {
		a.startHidden = true
	}
	if opts.StartMCP {
		a.forceStartMCP = true
	}
	if opts.DeepLinkURL == "" {
		return
	}
	action, err := parseDeepLink(opts.DeepLinkURL)
	if err != nil {
		log.Writef("deeplink: ignoring launch URL %q: %v", opts.DeepLinkURL, err)
		return
	}
	log.Writef("deeplink: cold-start action=%s connId=%s section=%s tray=%v",
		action.Action, action.ConnectionID, action.Section, action.Tray)
	if action.Tray {
		a.startHidden = true
	}
	switch action.Action {
	case "mcp-start":
		a.forceStartMCP = true
	case "open", "settings":
		a.startHidden = false
		a.deepLinkMu.Lock()
		a.pendingDeepLink = &action
		a.deepLinkMu.Unlock()
	}
}

// handleSecondInstance routes CLI flags and uniterm:// URLs forwarded by a
// second GUI launch through Wails' SingleInstance lock.
func (a *App) handleSecondInstance(args []string) {
	opts := parseLaunchArgs(args)
	if opts.StartMCP {
		go a.ensureMCPRunningAsync()
	}
	if opts.DeepLinkURL != "" {
		a.handleDeepLinkURL(opts.DeepLinkURL)
		return
	}
	if !opts.StartInTray {
		showMainWindow(a.window)
	}
}

// handleDeepLinkURL executes a uniterm:// URL received while the app is
// already running (via SingleInstance or ApplicationLaunchedWithUrl).
func (a *App) handleDeepLinkURL(raw string) {
	action, err := parseDeepLink(raw)
	if err != nil {
		log.Writef("deeplink: ignoring URL %q: %v", raw, err)
		return
	}
	log.Writef("deeplink: handling action=%s connId=%s section=%s tray=%v",
		action.Action, action.ConnectionID, action.Section, action.Tray)

	switch action.Action {
	case "start":
		if !action.Tray {
			showMainWindow(a.window)
		}
	case "mcp-start":
		go a.ensureMCPRunningAsync()
		if !action.Tray {
			showMainWindow(a.window)
		}
	case "open", "settings":
		showMainWindow(a.window)
		a.dispatchDeepLinkToFrontend(action)
	}
}

func (a *App) ensureMCPRunningAsync() {
	if err := a.EnsureMCPRunning(); err != nil {
		log.Writef("deeplink: EnsureMCPRunning failed: %v", err)
		a.emit("app:startup-error", "MCP server: "+err.Error())
	}
}

func (a *App) dispatchDeepLinkToFrontend(action DeepLinkAction) {
	a.deepLinkMu.Lock()
	if !a.deepLinkReady {
		cp := action
		a.pendingDeepLink = &cp
		a.deepLinkMu.Unlock()
		return
	}
	a.deepLinkMu.Unlock()
	a.emit("app:deeplink", action)
}

// markDeepLinkReady is invoked once the frontend has mounted its app:deeplink
// listener and loaded connections, flushing any queued cold-start action.
func (a *App) markDeepLinkReady() {
	a.deepLinkMu.Lock()
	a.deepLinkReady = true
	pending := a.pendingDeepLink
	a.pendingDeepLink = nil
	a.deepLinkMu.Unlock()
	if pending != nil {
		a.emit("app:deeplink", *pending)
	}
}
