package session

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func withServeSessionProxiesOverHTTP(t *testing.T, enabled bool) {
	t.Helper()
	saved := ServeSessionProxiesOverHTTP
	ServeSessionProxiesOverHTTP = enabled
	t.Cleanup(func() { ServeSessionProxiesOverHTTP = saved })
}

// Every proxy registers its upgrade handler under its auth token and drops
// it on Stop, in both address modes.
func TestSessionProxyRegistryLifecycle(t *testing.T) {
	for _, serverMode := range []bool{false, true} {
		withServeSessionProxiesOverHTTP(t, serverMode)

		rdp := NewRDPProxy("127.0.0.1:1", nil, "registry-rdp-token", RDPOptions{})
		addr, err := rdp.Start()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := LookupSessionProxyHandler("registry-rdp-token"); !ok {
			t.Fatal("rdp proxy did not register its handler")
		}
		rdp.Stop()
		if _, ok := LookupSessionProxyHandler("registry-rdp-token"); ok {
			t.Fatal("rdp proxy left its handler registered after Stop")
		}

		vnc := NewVNCProxy("127.0.0.1:1")
		addr, err = vnc.Start()
		if err != nil {
			t.Fatal(err)
		}
		if vnc.authToken == "" {
			t.Fatal("vnc proxy started without an auth token")
		}
		if _, ok := LookupSessionProxyHandler(vnc.authToken); !ok {
			t.Fatal("vnc proxy did not register its handler")
		}
		vnc.Stop()
		if _, ok := LookupSessionProxyHandler(vnc.authToken); ok {
			t.Fatal("vnc proxy left its handler registered after Stop")
		}

		spice := NewSPICEProxy("127.0.0.1:1")
		addr, err = spice.Start()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := LookupSessionProxyHandler(spice.authToken); !ok {
			t.Fatal("spice proxy did not register its handler")
		}
		spice.Stop()
		if _, ok := LookupSessionProxyHandler(spice.authToken); ok {
			t.Fatal("spice proxy left its handler registered after Stop")
		}

		if serverMode {
			if !strings.HasPrefix(addr, "/api/web"+SessionProxyRoutePrefix) {
				t.Fatalf("server-mode address %q is not a session-proxy path", addr)
			}
			if strings.Contains(addr, "127.0.0.1") {
				t.Fatalf("server-mode address %q leaks a loopback URL", addr)
			}
		} else if !strings.HasPrefix(addr, "ws://127.0.0.1:") {
			t.Fatalf("desktop address %q is not a loopback ws URL", addr)
		}
	}
}

// The RDP proxy's own mux still guards its token path when dispatched
// through the registry, so a wrong token must not upgrade.
func TestSessionProxyRegistryRejectsWrongPath(t *testing.T) {
	rdp := NewRDPProxy("127.0.0.1:1", nil, "registry-guard-token", RDPOptions{})
	if _, err := rdp.Start(); err != nil {
		t.Fatal(err)
	}
	defer rdp.Stop()

	h, ok := LookupSessionProxyHandler("registry-guard-token")
	if !ok {
		t.Fatal("handler not registered")
	}
	// Simulate the /api/web dispatch: path rewritten back to the proxy-local
	// token path. A hijack-capable recorder is not available, so only assert
	// the guard on a plain GET: the correct path attempts the upgrade (and
	// fails on the non-hijackable recorder), the wrong path 404s.
	rec := &discardRecorder{header: http.Header{}}
	req := &http.Request{URL: &url.URL{Path: "/wrong-token"}, Method: http.MethodGet}
	h.ServeHTTP(rec, req)
	if rec.code != http.StatusNotFound {
		t.Fatalf("wrong token path got %d, want 404", rec.code)
	}
}

type discardRecorder struct {
	header http.Header
	code   int
}

func (d *discardRecorder) Header() http.Header       { return d.header }
func (d *discardRecorder) Write([]byte) (int, error) { return 0, nil }
func (d *discardRecorder) WriteHeader(code int)      { d.code = code }
func (d *discardRecorder) WriteString(s string)      {}
