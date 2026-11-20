package session

import (
	"context"
	"net/http"
	"testing"
)

// TestProxyDialFuncNil covers the nil-propagation contract: every optional
// dialer field in the drivers must stay nil so the driver's default dialer
// (or the direct path) is used.
func TestProxyDialFuncNil(t *testing.T) {
	if ProxyDialFunc(nil) != nil {
		t.Fatal("ProxyDialFunc(nil) must return nil")
	}
	if ProxyHTTPTransportProxy(nil) != nil {
		t.Fatal("ProxyHTTPTransportProxy(nil) must return nil")
	}
}

func TestProxyHTTPTransportProxyURL(t *testing.T) {
	cases := []struct {
		name        string
		proxy       *SocksProxy
		wantURL     string
		wantHasPass bool
	}{
		{
			name:    "socks5",
			proxy:   &SocksProxy{Kind: "socks5", Host: "10.0.0.2", Port: 1080},
			wantURL: "socks5://10.0.0.2:1080",
		},
		{
			name:        "http with credentials",
			proxy:       &SocksProxy{Kind: "http", Host: "proxy.corp", Port: 3128, User: "u", Pass: "p:ss"},
			wantURL:     "http://u:p%3Ass@proxy.corp:3128",
			wantHasPass: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := ProxyHTTPTransportProxy(tc.proxy)
			if fn == nil {
				t.Fatal("expected non-nil proxy func")
			}
			u, err := fn(&http.Request{})
			if err != nil {
				t.Fatal(err)
			}
			if u.String() != tc.wantURL {
				t.Errorf("proxy url = %q, want %q", u.String(), tc.wantURL)
			}
			if tc.wantHasPass {
				if _, ok := u.User.Password(); !ok {
					t.Errorf("userinfo password missing in %q", u.String())
				}
			}
		})
	}
}

// TestProxyDialFuncSurfacesError verifies the closure routes through
// dialFirstHopContext: dialing through an unreachable proxy must fail with
// that dial error rather than falling back to a direct connection.
func TestProxyDialFuncSurfacesError(t *testing.T) {
	dial := ProxyDialFunc(&SocksProxy{Kind: "socks5", Host: "127.0.0.1", Port: 1})
	if dial == nil {
		t.Fatal("expected non-nil dial func")
	}
	_, err := dial(context.Background(), "tcp", "127.0.0.1:2")
	if err == nil {
		t.Fatal("expected dial error through unreachable proxy")
	}
}

// TestDBProxyDialNil verifies the database dial hook is nil without a proxy
// so NewDBWithDial keeps the plain sql.Open path.
func TestDBProxyDialNil(t *testing.T) {
	if dbProxyDial(ConnectionConfig{}) != nil {
		t.Fatal("dbProxyDial without proxy must be nil")
	}
	if dbProxyDial(ConnectionConfig{Proxy: &SocksProxy{Kind: "socks5", Host: "h", Port: 1}}) == nil {
		t.Fatal("dbProxyDial with proxy must be non-nil")
	}
}
