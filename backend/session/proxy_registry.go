package session

import (
	"net/http"
	"sync"
)

// ServeSessionProxiesOverHTTP switches RDP/VNC/SPICE session proxies from
// ws://127.0.0.1:<port> addresses to path-form addresses
// (/api/web/session-proxy/<token>). The server build (-tags server) sets it
// at startup: there the page is viewed in a remote browser, and a loopback
// URL advertised to that browser points at the wrong machine (the browser's
// own), which made remote desktop sessions unreachable in web deployments.
//
// The advertised path is resolved by the gateway in front of the worker: the
// wails asset server serving /api/web rejects WebSocket upgrades (501), so
// the gateway answers a session-proxy request by asking the worker — via the
// plain-HTTP lookup endpoint registered below — for the proxy's internal
// loopback listener, and relays the upgrade straight to it.
var ServeSessionProxiesOverHTTP bool

// SessionProxyRoutePrefix is the /api/web sub-path under which session
// proxies are addressed in server mode, and (without the /api/web part) the
// sub-path of the plain-HTTP lookup endpoint that maps a token back to the
// proxy's internal listener.
const SessionProxyRoutePrefix = "/session-proxy/"

// SessionProxyLookupRoutePrefix is the /api/web sub-path of the plain-HTTP
// endpoint the gateway calls to resolve a proxy token to the internal
// loopback listener address ("host:port").
const SessionProxyLookupRoutePrefix = "/session-proxy-lookup/"

// sessionProxyEntry pairs the WebSocket upgrade handler with the address of
// the proxy's own loopback listener.
type sessionProxyEntry struct {
	handler http.Handler
	addr    string
}

// sessionProxyHandlers maps a proxy's random auth token to its upgrade
// handler and internal listener. Proxies register on start and unregister on
// stop; lookup is by token so the browser never needs to know a port —
// everything rides the same origin as the web page.
var sessionProxyHandlers sync.Map // token(string) -> sessionProxyEntry

func registerSessionProxyHandler(token string, h http.Handler, addr string) {
	sessionProxyHandlers.Store(token, sessionProxyEntry{handler: h, addr: addr})
}

func unregisterSessionProxyHandler(token string) {
	sessionProxyHandlers.Delete(token)
}

// LookupSessionProxyHandler returns the upgrade handler registered under
// token; used by the server build's /api/web mux.
func LookupSessionProxyHandler(token string) (http.Handler, bool) {
	e, ok := lookupSessionProxyEntry(token)
	if !ok {
		return nil, false
	}
	return e.handler, true
}

// LookupSessionProxyAddr returns the loopback listener address
// ("127.0.0.1:<port>") of the proxy registered under token. The gateway's
// session-proxy relay dials it directly: the wails asset server in front of
// /api/web cannot upgrade WebSockets, so the relay must bypass it.
func LookupSessionProxyAddr(token string) (string, bool) {
	e, ok := lookupSessionProxyEntry(token)
	if !ok {
		return "", false
	}
	return e.addr, true
}

func lookupSessionProxyEntry(token string) (sessionProxyEntry, bool) {
	v, ok := sessionProxyHandlers.Load(token)
	if !ok {
		return sessionProxyEntry{}, false
	}
	e, ok := v.(sessionProxyEntry)
	return e, ok && e.handler != nil && e.addr != ""
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
