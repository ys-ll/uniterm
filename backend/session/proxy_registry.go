package session

import (
	"net/http"
	"sync"
)

// ServeSessionProxiesOverHTTP switches RDP/VNC/SPICE session proxies from
// ws://127.0.0.1:<port> addresses to path-form addresses
// (/api/web/session-proxy/<token>) served through the main HTTP server's
// /api/web mount. The server build (-tags server) sets it at startup: there
// the page is viewed in a remote browser, and a loopback URL advertised to
// that browser points at the wrong machine (the browser's own), which made
// remote desktop sessions unreachable in uniterm-web deployments.
var ServeSessionProxiesOverHTTP bool

// SessionProxyRoutePrefix is the /api/web sub-path under which the server
// build's WebStreamService dispatches to registered session proxies.
const SessionProxyRoutePrefix = "/session-proxy/"

// sessionProxyHandlers maps a proxy's random auth token to the WebSocket
// upgrade handler that drives the tunnel. Proxies register on start and
// unregister on stop; lookup is by token so the browser never needs to know
// a port — everything rides the same origin as the web page.
var sessionProxyHandlers sync.Map // token(string) -> http.Handler

func registerSessionProxyHandler(token string, h http.Handler) {
	sessionProxyHandlers.Store(token, h)
}

func unregisterSessionProxyHandler(token string) {
	sessionProxyHandlers.Delete(token)
}

// LookupSessionProxyHandler returns the upgrade handler registered under
// token; used by the server build's /api/web mux.
func LookupSessionProxyHandler(token string) (http.Handler, bool) {
	v, ok := sessionProxyHandlers.Load(token)
	if !ok {
		return nil, false
	}
	h, _ := v.(http.Handler)
	return h, h != nil
}

// sessionProxyAddress returns the advertised address for a started proxy:
// a path served by the main HTTP server in server mode, or the proxy's own
// loopback URL otherwise.
func sessionProxyAddress(token string, loopbackURL string) string {
	if ServeSessionProxiesOverHTTP {
		return "/api/web" + SessionProxyRoutePrefix + token
	}
	return loopbackURL
}
