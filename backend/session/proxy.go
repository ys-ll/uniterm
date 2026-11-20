package session

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strconv"

	"github.com/ys-ll/uniterm/backend/database"
)

// Proxy is a reusable outbound proxy (SOCKS5/HTTP) stored in proxies.json,
// mirroring Identity in the credential vault. Pass is decrypted in memory and
// encrypted at rest. Enabled is a tri-state: nil (missing in old JSON) counts
// as enabled so existing stores keep working after upgrade.
type Proxy struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"` // "socks5" | "http"
	Host    string `json:"host"`
	Port    int    `json:"port"`
	User    string `json:"user,omitempty"`
	Pass    string `json:"pass,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// IsActive reports whether the proxy should be honored on connect. A disabled
// proxy is skipped silently: connections referencing it dial directly.
func (p *Proxy) IsActive() bool { return p.Enabled == nil || *p.Enabled }

// ProxyStoreData is the top-level shape of proxies.json.
type ProxyStoreData struct {
	Proxies []Proxy `json:"proxies"`
}

// ProxyResolver resolves a Proxy ID into the runtime SocksProxy dial struct
// (Kind/Host/Port/User/Pass), consumed directly by dialFirstHop.
type ProxyResolver func(id string) (SocksProxy, bool)

// ProxyIDSystem is the sentinel proxy reference selecting the OS system proxy
// (registry/system config, PAC scripts, HTTP(S)_PROXY env fallback), resolved
// at connect/request time against the actual target and never persisted as a
// concrete proxy. Saved proxy IDs are generated as "px-..." so it never
// collides. An empty reference stays "dial direct".
const ProxyIDSystem = "system"

// ProxyDialFunc adapts a SocksProxy into the driver-style context dialer that
// database/redis/mongo connectors accept. nil proxy → nil, so callers can
// assign the result directly to an optional dialer field. The network argument
// is ignored: every proxyable session dial is TCP (dialFirstHop's domain).
func ProxyDialFunc(upstream *SocksProxy) func(ctx context.Context, network, addr string) (net.Conn, error) {
	if upstream == nil {
		return nil
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialFirstHopContext(ctx, addr, upstream)
	}
}

// proxyContextDialer wraps ProxyDialFunc's closure into the named-method
// interface shape (ContextDialer) that mongo-driver's SetDialer requires.
type proxyContextDialer struct{ fn func(ctx context.Context, network, addr string) (net.Conn, error) }

func dialerAdapter(upstream *SocksProxy) proxyContextDialer {
	return proxyContextDialer{fn: ProxyDialFunc(upstream)}
}

func (d proxyContextDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.fn(ctx, network, addr)
}

// dbProxyDial converts a connection's proxy reference into the database
// package's dial hook. nil when no proxy is configured.
func dbProxyDial(config ConnectionConfig) database.DialContextFunc {
	if config.Proxy == nil {
		return nil
	}
	return database.DialContextFunc(ProxyDialFunc(config.Proxy))
}

// ProxyHTTPTransportProxy returns an http.Transport Proxy hook routing all
// requests through the upstream proxy. The http.Transport layer understands
// both http(s):// CONNECT and socks5:// URLs, matching dialFirstHop's two
// proxy kinds; proxy credentials ride in the URL userinfo. nil proxy → nil
// so callers can keep their existing Proxy assignment.
func ProxyHTTPTransportProxy(upstream *SocksProxy) func(*http.Request) (*url.URL, error) {
	if upstream == nil {
		return nil
	}
	u := &url.URL{Scheme: upstream.Kind, Host: net.JoinHostPort(upstream.Host, strconv.Itoa(upstream.Port))}
	if upstream.User != "" {
		u.User = url.UserPassword(upstream.User, upstream.Pass)
	}
	return func(*http.Request) (*url.URL, error) { return u, nil }
}
