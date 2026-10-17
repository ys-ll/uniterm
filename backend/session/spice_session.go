package session

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

type SPICESession struct{ remoteDesktopSession }

func NewSPICESession(id string) *SPICESession {
	return &SPICESession{remoteDesktopSession{baseSession: baseSession{id: id, sessionType: "spice", status: StatusDisconnected}}}
}

// Bare hosts retain the original WebSocket transport. tcp:// explicitly selects
// a SPICE TCP server; ws(s):// can include a gateway path/query. No migration of
// saved connections or guessing a protocol from its port number is necessary.
func spiceTarget(config ConnectionConfig) (string, error) {
	port := strconv.Itoa(remoteDesktopPort(config.Port))
	if !strings.Contains(config.Host, "://") {
		return "ws://" + net.JoinHostPort(strings.Trim(config.Host, "[]"), port) + "/", nil
	}
	u, err := url.Parse(config.Host)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return "", fmt.Errorf("invalid SPICE host URL")
	}
	if u.Port() != "" {
		port = u.Port()
	}
	switch u.Scheme {
	case "tcp":
		if u.RawQuery != "" || (u.Path != "" && u.Path != "/") {
			return "", fmt.Errorf("SPICE TCP targets cannot have a path or query")
		}
		return net.JoinHostPort(u.Hostname(), port), nil
	case "ws", "wss":
		if u.Port() == "" {
			u.Host = net.JoinHostPort(u.Hostname(), port)
		}
		return u.String(), nil
	default:
		return "", fmt.Errorf("SPICE host must use tcp://, ws:// or wss://")
	}
}

func (s *SPICESession) Connect(config ConnectionConfig) error {
	target, err := spiceTarget(config)
	if err != nil {
		return err
	}
	return s.connectProxy(config.Host, NewSPICEProxy(target).remoteDesktopProxy)
}
