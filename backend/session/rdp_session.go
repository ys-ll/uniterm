package session

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"

	utls "github.com/refraction-networking/utls"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/ys-ll/uniterm/backend/log"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// DefaultRDPPort is used when a connection config omits the RDP port.
const DefaultRDPPort = 3389

// tlsProbeTimeout bounds each TLS handshake attempt against the RDP server.
// A reachable server answers the ClientHello within an RTT, so a silent
// server is detected quickly and the version fallback moves on instead of
// sitting out the full connection deadline.
const tlsProbeTimeout = time.Second

// IronRDPSession runs the local TCP→WebSocket proxy for the IronRDP WASM
// client. The RDP protocol itself runs entirely in the webview (WASM);
// Connect's job is: fail-fast reachability check → start the proxy → report
// connected with the proxy URL, mirroring the VNC/SPICE contract where
// CreateSession returns synchronously with ProxyAddr.
type RDPSession struct {
	baseSession
	proxy     *RDPProxy
	proxyAddr string
	lastError string
}

func NewRDPSession(id string) *RDPSession {
	return &RDPSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "rdp",
			status:      StatusDisconnected,
		},
	}
}

func (s *RDPSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)

	port := config.Port
	if port <= 0 {
		port = DefaultRDPPort
	}
	target := net.JoinHostPort(config.Host, strconv.Itoa(port))
	destination := target
	if config.TunnelTargetHost != "" {
		tunnelPort := config.TunnelTargetPort
		if tunnelPort <= 0 {
			tunnelPort = port
		}
		destination = net.JoinHostPort(config.TunnelTargetHost, strconv.Itoa(tunnelPort))
	}

	s.title = fmt.Sprintf("%s (RDP)", config.Host)

	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("rdp token: %w", err)
	}

	proxy := NewRDPProxy(target, config.Proxy, hex.EncodeToString(token), RDPOptions{
		EnableCredSSP: config.SecurityOffersCredSSP(),
		AdminSession:  config.RdpAdminSession,
		Destination:   destination,
	})
	proxy.SetOnError(func(err error) {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		s.setStatus(StatusError)
	})
	proxy.SetOnUpstreamClosed(func() {
		// The RDP server dropped the tunnel mid-session.
		if s.Status() == StatusConnected {
			s.setStatus(StatusDisconnected)
		}
	})
	url, err := proxy.Start()
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("rdp proxy start: %w", err)
	}
	s.proxy = proxy
	s.proxyAddr = url

	// Proxy is up; the actual RDP handshake happens between the WASM client
	// and the server through it.
	s.setStatus(StatusConnected)
	return nil
}

func (s *RDPSession) Disconnect() error {
	if s.proxy != nil {
		s.proxy.Stop()
		s.proxy = nil
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *RDPSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

// Resize is handled client-side via the display control channel; the backend
// has no terminal-size concept here.
func (s *RDPSession) Resize(cols, rows int) error { return nil }

// Write is a no-op: RDP data flows over the proxy's WebSocket.
func (s *RDPSession) Write(data []byte) error { return nil }

func (s *RDPSession) ProxyAddr() string { return s.proxyAddr }

func (s *RDPSession) LastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

// IronRDPProxy bridges WebSocket (the IronRDP WASM client in the webview) to
// TCP (the RDP server), dialing through the connection's proxy when set.
//
// The WASM client speaks the RDCleanPath handshake (the Devolutions gateway
// protocol used by ironrdp-web): its first WebSocket message is a DER-encoded
// RDCleanPathPdu wrapping the client's X.224 ConnectionRequest. The proxy
// dials the RDP server, forwards the X.224 request, reads the X.224 response,
// terminates TLS to the server and returns its certificate chain in a
// DER-encoded Response PDU. CredSSP runs in WASM, bound to that certificate.
//
// One instance per ironrdp session, bound to 127.0.0.1 on a random port and
// serving a single random path — both are part of the local-only guard: the
// webview's scripts can reach it, but nothing else on the machine can guess
// the endpoint while the session lives. The listener is closed with the
// session.
type RDPProxy struct {
	listener         net.Listener
	target           string // host:port used to dial upstream, post-tunnel rewrite
	destination      string // client-visible host:port before the tunnel rewrite
	upstream         *SocksProxy
	authToken        string
	stopCh           chan struct{}
	stopOnce         sync.Once
	wg               sync.WaitGroup
	mu               sync.Mutex
	wsConn           *websocket.Conn
	tcpConn          net.Conn
	onUpstreamClosed func() // fired when the RDP server drops the tunnel
	onError          func(error)
	ctx              context.Context
	cancel           context.CancelFunc
	options          RDPOptions
}

type RDPOptions struct {
	EnableCredSSP bool
	AdminSession  bool
	// Destination is the RDP target the WASM client puts in RDCleanPath.
	// It differs from target when an SSH jump-host tunnel rewrote the dial
	// address to 127.0.0.1:localPort.
	Destination string
}

func NewRDPProxy(target string, upstream *SocksProxy, authToken string, options RDPOptions) *RDPProxy {
	destination := options.Destination
	if destination == "" {
		destination = target
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &RDPProxy{
		target:      target,
		destination: destination,
		upstream:    upstream,
		authToken:   authToken,
		stopCh:      make(chan struct{}),
		options:     options,
		ctx:         ctx,
		cancel:      cancel,
	}
}

func (p *RDPProxy) SetOnError(fn func(error)) {
	p.mu.Lock()
	p.onError = fn
	p.mu.Unlock()
}

// SetOnUpstreamClosed registers the callback fired when the RDP server side
// of the tunnel closes, so the session can report StatusDisconnected.
func (p *RDPProxy) SetOnUpstreamClosed(fn func()) {
	p.mu.Lock()
	p.onUpstreamClosed = fn
	p.mu.Unlock()
}

// Start begins listening on a random local port under a random path and
// returns the full WebSocket URL including the token path.
func (p *RDPProxy) Start() (string, error) {
	if p.authToken == "" {
		return "", fmt.Errorf("rdp proxy: empty auth token")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("rdp proxy listen: %w", err)
	}
	p.mu.Lock()
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		ln.Close()
		return "", fmt.Errorf("rdp proxy stopped")
	}
	p.listener = ln
	p.wg.Add(1)
	p.mu.Unlock()

	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Only the exact token path upgrades; every other path is a 404.
		if r.URL.Path != "/"+p.authToken {
			http.NotFound(w, r)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Writef("[RDPProxy] WebSocket upgrade failed: %v", err)
			return
		}
		p.handleWebSocket(ws)
	})
	// Also reachable through the server build's /api/web mount: the gateway
	// resolves the internal listener below via the lookup endpoint and
	// relays the upgrade here (the asset server itself cannot upgrade).
	addr := ln.Addr().(*net.TCPAddr)
	registerSessionProxyHandler(p.authToken, mux, addr.String())

	go func() {
		defer p.wg.Done()
		_ = http.Serve(ln, mux)
	}()

	return sessionProxyAddress(p.authToken, fmt.Sprintf("ws://127.0.0.1:%d/%s", addr.Port, p.authToken)), nil
}

func (p *RDPProxy) handleWebSocket(ws *websocket.Conn) {
	p.mu.Lock()
	if p.ctx.Err() != nil {
		p.mu.Unlock()
		ws.Close()
		return
	}
	if p.wsConn != nil {
		p.mu.Unlock()
		log.Writef("[RDPProxy] Rejecting additional WebSocket connection")
		ws.Close()
		return
	}
	p.wsConn = ws
	p.wg.Add(1)
	p.mu.Unlock()
	defer p.wg.Done()
	ctx, cancel := context.WithCancel(p.ctx)
	defer cancel()
	var failed sync.Once
	reportError := func(err error) {
		if ctx.Err() != nil {
			return
		}
		failed.Do(func() {
			p.mu.Lock()
			fn := p.onError
			p.mu.Unlock()
			if fn != nil {
				fn(err)
			}
		})
	}

	defer func() {
		ws.Close()
		p.mu.Lock()
		if p.wsConn == ws {
			p.wsConn = nil
			p.tcpConn = nil
		}
		p.mu.Unlock()
	}()

	select {
	case <-p.stopCh:
		ws.Close()
		return
	default:
	}

	// First WebSocket message: DER-encoded RDCleanPath Request carrying the
	// client's X.224 ConnectionRequest (TPKT-framed, forwarded verbatim).
	_, reqDER, err := ws.ReadMessage()
	if err != nil {
		log.Writef("[RDPProxy] WebSocket read (rdcleanpath request) failed: %v", err)
		ws.Close()
		return
	}
	dest, proxyAuth, x224Req, err := decodeRDCleanPathRequest(reqDER)
	if err != nil {
		log.Writef("[RDPProxy] Invalid RDCleanPath request: %v", err)
		_ = p.writeRDCleanPathErr(ws, "invalid request PDU")
		ws.Close()
		return
	}
	if proxyAuth != p.authToken {
		log.Writef("[RDPProxy] RDCleanPath proxy_auth token mismatch")
		_ = p.writeRDCleanPathErr(ws, "authentication failed")
		ws.Close()
		return
	}
	if !p.acceptsDestination(dest) {
		// The session is pinned to one target; refuse anything else. The
		// client sends its builder destination (bare host), the proxy
		// destination carries the port — both spellings are accepted.
		log.Writef("[RDPProxy] RDCleanPath destination %q does not match session target %q", dest, p.destination)
		_ = p.writeRDCleanPathErr(ws, "destination mismatch")
		ws.Close()
		return
	}
	// Honor the connection setting; never silently downgrade an enabled NLA
	// request. Older servers can still be used with CredSSP explicitly off.
	if !p.options.EnableCredSSP {
		rewriteX224SslOnly(x224Req)
	}

	// In the RDCleanPath flow the WASM client never does TLS itself — the
	// gateway role owns the TLS leg to the RDP server (the client's CredSSP
	// TSRequest records travel raw through the tunnel, bound to the server
	// public key delivered out-of-band in the Response). The proxy therefore
	// dials, replays the client's X.224 request, terminates TLS to the server
	// and relays the decrypted plaintext.
	upstreamConn, selected, certChain, x224Resp, err := establishUpstream(ctx, p.target, p.destination, p.upstream, x224Req)
	if err != nil {
		reportError(err)
		log.Writef("[RDPProxy] Upstream establishment failed: %v", err)
		_ = p.writeRDCleanPathErr(ws, err.Error())
		ws.Close()
		return
	}
	defer upstreamConn.Close()
	if selected == x224ProtoFailure {
		_ = p.writeRDCleanPathNegotiationErr(ws, x224Resp)
		if code, ok := parseX224FailureCode(x224Resp); ok {
			log.Writef("[RDPProxy] RDP negotiation failed (server sent NEG_FAILURE, code=%d)", code)
		} else {
			log.Writef("[RDPProxy] RDP negotiation failed (server sent NEG_FAILURE)")
		}
		ws.Close()
		return
	}

	p.mu.Lock()
	if ctx.Err() != nil {
		p.mu.Unlock()
		upstreamConn.Close()
		return
	}
	p.tcpConn = upstreamConn
	p.mu.Unlock()

	resp, err := encodeRDCleanPathResponse(x224Resp, certChain, p.destination)
	if err != nil {
		log.Writef("[RDPProxy] RDCleanPath response encode failed: %v", err)
		ws.Close()
		upstreamConn.Close()
		return
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, resp); err != nil {
		log.Writef("[RDPProxy] RDCleanPath response write failed: %v", err)
		ws.Close()
		upstreamConn.Close()
		return
	}

	var relayWG sync.WaitGroup
	relayWG.Add(2)

	handshake := &rdpClientHandshake{adminSession: p.options.AdminSession}

	go func() {
		defer relayWG.Done()
		defer upstreamConn.Close()
		defer cancel()
		for {
			select {
			case <-p.stopCh:
				return
			default:
			}
			msgType, data, err := ws.ReadMessage()
			if err != nil {
				log.Writef("[RDPProxy] WebSocket read error: %v", err)
				return
			}
			if msgType == websocket.BinaryMessage {
				data, err = handshake.rewrite(data)
				if err != nil {
					reportError(fmt.Errorf("RDP client handshake failed: %w", err))
					log.Writef("[RDPProxy] Client handshake failed: %v", err)
					return
				}
				if len(data) == 0 {
					continue
				}
				if _, err := upstreamConn.Write(data); err != nil {
					reportError(fmt.Errorf("RDP server write failed: %w", err))
					log.Writef("[RDPProxy] TCP write error: %v", err)
					return
				}
			}
		}
	}()

	go func() {
		defer relayWG.Done()
		defer func() {
			ws.Close()
			// The RDP server dropped the tunnel (or we are stopping). The
			// stop path reports Disconnected itself via Disconnect(); only
			// surface the unexpected close here.
			select {
			case <-p.stopCh:
			default:
				if fn := p.upstreamClosedCallback(); fn != nil {
					fn()
				}
			}
		}()
		buf := make([]byte, 32768)
		for {
			select {
			case <-p.stopCh:
				return
			default:
			}
			n, err := upstreamConn.Read(buf)
			if n > 0 {
				if writeErr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					log.Writef("[RDPProxy] WebSocket write error: %v", writeErr)
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					reportError(fmt.Errorf("RDP server connection failed: %w", err))
				}
				log.Writef("[RDPProxy] TCP read error: %v", err)
				return
			}
		}
	}()
	relayWG.Wait()
}

func (p *RDPProxy) upstreamClosedCallback() func() {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.onUpstreamClosed
}

// acceptsDestination pins the RDCleanPath destination to the client-visible
// RDP target. An empty destination is allowed because the client omits it in
// some versions.
func (p *RDPProxy) acceptsDestination(dest string) bool {
	if dest == "" {
		return true
	}
	return dest == p.destination || dest == p.hostOnly()
}

// hostOnly strips the port from the destination for lenient destination checks.
func (p *RDPProxy) hostOnly() string {
	host, _, err := net.SplitHostPort(p.destination)
	if err != nil {
		return p.destination
	}
	return host
}

func (p *RDPProxy) writeRDCleanPathErr(ws *websocket.Conn, msg string) error {
	pdu := encodeRDCleanPathErr(rdcleanErrCodeGeneral)
	log.Writef("[RDPProxy] RDCleanPath error: %s", msg)
	return ws.WriteMessage(websocket.BinaryMessage, pdu)
}

func (p *RDPProxy) writeRDCleanPathNegotiationErr(ws *websocket.Conn, x224Resp []byte) error {
	pdu := encodeRDCleanPathNegotiationErr(x224Resp)
	return ws.WriteMessage(websocket.BinaryMessage, pdu)
}

// Stop closes all connections and waits for goroutines to exit (same close
// order as VNCProxy.Stop: connections first, then the listener before
// wg.Wait so a late accept cannot add waiters that never finish).
func (p *RDPProxy) Stop() {
	p.stopOnce.Do(func() {
		close(p.stopCh)
		p.cancel()
	})
	unregisterSessionProxyHandler(p.authToken)
	p.mu.Lock()
	if p.wsConn != nil {
		p.wsConn.Close()
	}
	if p.tcpConn != nil {
		p.tcpConn.Close()
	}
	p.mu.Unlock()
	if p.listener != nil {
		p.listener.Close()
	}
	p.wg.Wait()
}

// rewriteX224SslOnly clears the HYBRID/HYBRID_EX bits in the negotiation
// request's NEG_REQ (always the last 8 bytes of the X.224 variable part),
// making the server select plain SSL so the client skips CredSSP entirely.
func rewriteX224SslOnly(pdu []byte) {
	if len(pdu) < 20 {
		return
	}
	li := int(pdu[4])
	if li < 7 {
		return
	}
	end := 5 + li // TPKT + LI + LI-counted bytes
	if end > len(pdu) || end < 8 {
		return
	}
	off := end - 8 // NEG_REQ: type(1) flags(1) len(2) protocols(4)
	if pdu[off] != 0x01 {
		return
	}
	proto := binary.LittleEndian.Uint32(pdu[off+4 : off+8])
	proto &^= x224ProtoHybrid | x224ProtoHybridEx
	binary.LittleEndian.PutUint32(pdu[off+4:], proto)
}

// TLS leg strategies, tried in order on a fresh connection each. The first
// (utls12) mimics mstsc's ClientHello — see mstscClientHelloSpec — because
// Windows Server 2008-era Schannel both (a) requires TLS 1.2 for CredSSP and
// (b) RSTs Go's standard TLS 1.2 ClientHello (its signature_algorithms carry
// ed25519 and it offers encrypt-then-MAC, neither of which mstsc sends). The
// Go strategies remain as fallbacks for stacks the mstsc-shaped hello cannot
// reach.
const (
	tlsStrategyUtls12 = "utls-mstsc-tls12"
	tlsStrategyGo12   = "go-tls12"
	tlsStrategyGo10   = "go-tls10"
)

var tlsStrategyOrder = []string{tlsStrategyUtls12, tlsStrategyGo12, tlsStrategyGo10}

// tlsStrategyCache remembers, per real RDP target (host:port before any
// tunnel rewrite), the TLS strategy that last completed a handshake.
// Servers that reject a strategy would otherwise be re-probed (and re-stall)
// on every reconnect; with the cache the working strategy is tried first.
// Self-healing: a cached strategy that fails is dropped and probing resumes.
var (
	tlsStrategyCacheMu sync.Mutex
	tlsStrategyCache   = map[string]string{}
)

func cachedTLSStrategy(key string) (string, bool) {
	if key == "" {
		return "", false
	}
	tlsStrategyCacheMu.Lock()
	defer tlsStrategyCacheMu.Unlock()
	v, ok := tlsStrategyCache[key]
	return v, ok
}

func rememberTLSStrategy(key, strategy string) {
	if key == "" {
		return
	}
	tlsStrategyCacheMu.Lock()
	defer tlsStrategyCacheMu.Unlock()
	tlsStrategyCache[key] = strategy
}

func forgetTLSStrategy(key, strategy string) {
	if key == "" {
		return
	}
	tlsStrategyCacheMu.Lock()
	defer tlsStrategyCacheMu.Unlock()
	if tlsStrategyCache[key] == strategy {
		delete(tlsStrategyCache, key)
	}
}

// establishUpstream dials the RDP server, replays the client's X.224
// request (TPKT included), reads the Connection Confirm, and — when TLS was
// selected — terminates TLS to the server with the first strategy in
// tlsStrategyOrder that completes a handshake (mstsc-shaped TLS 1.2 first —
// see the strategy consts — then Go TLS 1.2, then Go TLS 1.0).
// cacheKey is the pre-tunnel target used for the strategy cache.
// Returns the plaintext-side stream to relay, the selected protocol, the
// server cert chain (nil for standard RDP security) and the x.224 confirm.
func establishUpstream(ctx context.Context, target, cacheKey string, upstream *SocksProxy, x224Req []byte) (net.Conn, uint32, [][]byte, []byte, error) {
	strategies := tlsStrategyOrder
	if known, ok := cachedTLSStrategy(cacheKey); ok {
		strategies = append([]string{known}, others(tlsStrategyOrder, known)...)
	}
	for i, strategy := range strategies {
		dialStart := time.Now()
		conn, err := dialFirstHopContext(ctx, target, upstream)
		dialDur := time.Since(dialStart).Round(time.Millisecond)
		if err != nil {
			log.Writef("[RDPProxy] dial %s failed after %s: %v", target, dialDur, err)
			return nil, 0, nil, nil, fmt.Errorf("cannot reach RDP server: %w", err)
		}
		stopClose := context.AfterFunc(ctx, func() { conn.Close() })
		defer stopClose()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		if _, err := conn.Write(x224Req); err != nil {
			conn.Close()
			return nil, 0, nil, nil, fmt.Errorf("RDP server write failed: %w", err)
		}
		resp, err := readTPKTPDU(conn, 5*time.Second)
		if err != nil {
			conn.Close()
			return nil, 0, nil, nil, fmt.Errorf("RDP server read failed: %w", err)
		}
		selected := parseX224SelectedProtocol(resp)
		if selected == x224ProtoFailure || selected == x224ProtoRDP {
			// Hand the confirm back verbatim: the caller relays a
			// NegotiationErr / plain tunnel respectively.
			_ = conn.SetDeadline(time.Time{})
			return conn, selected, nil, resp, nil
		}

		_ = conn.SetDeadline(time.Now().Add(tlsProbeTimeout))
		tlsStart := time.Now()
		tlsConn, chain, err := handshakeTLSLeg(strategy, conn, targetHostOf(target))
		if err != nil {
			forgetTLSStrategy(cacheKey, strategy)
			log.Writef("[RDPProxy] TLS handshake to %s failed after %s (strategy=%s): %v",
				target, time.Since(tlsStart).Round(time.Millisecond), strategy, err)
			conn.Close()
			if i == len(strategies)-1 {
				return nil, 0, nil, nil, fmt.Errorf("TLS to RDP server failed: %w", err)
			}
			continue
		}
		rememberTLSStrategy(cacheKey, strategy)
		_ = conn.SetDeadline(time.Time{})
		if len(chain) == 0 {
			conn.Close()
			tlsConn.Close()
			return nil, 0, nil, nil, fmt.Errorf("no server certificate presented")
		}
		return tlsConn, selected, chain, resp, nil
	}
	return nil, 0, nil, nil, fmt.Errorf("TLS to RDP server failed: exhausted retry attempts")
}

// others returns every element of order except except, preserving order.
func others(order []string, except string) []string {
	out := make([]string, 0, len(order))
	for _, s := range order {
		if s != except {
			out = append(out, s)
		}
	}
	return out
}

// targetHostOf strips the port from a host:port target for SNI purposes,
// falling back to the raw target when it carries no port.
func targetHostOf(target string) string {
	if h, _, err := net.SplitHostPort(target); err == nil && h != "" {
		return h
	}
	return target
}

// handshakeTLSLeg runs one TLS strategy against conn and returns the ready
// connection plus the server certificate chain as raw DER.
func handshakeTLSLeg(strategy string, conn net.Conn, host string) (net.Conn, [][]byte, error) {
	switch strategy {
	case tlsStrategyUtls12:
		u := utls.UClient(conn, &utls.Config{
			InsecureSkipVerify: true, //nolint:gosec // the proxy is local-only; the WASM client validates nothing here, CredSSP binds to the delivered public key
			ServerName:         host,
		}, utls.HelloCustom)
		if err := u.ApplyPreset(mstscClientHelloSpec(host)); err != nil {
			return nil, nil, err
		}
		if err := u.Handshake(); err != nil {
			return nil, nil, err
		}
		state := u.ConnectionState()
		chain := make([][]byte, 0, len(state.PeerCertificates))
		for _, c := range state.PeerCertificates {
			chain = append(chain, c.Raw)
		}
		return u, chain, nil
	case tlsStrategyGo12, tlsStrategyGo10:
		maxVersion := uint16(tls.VersionTLS12)
		if strategy == tlsStrategyGo10 {
			maxVersion = uint16(tls.VersionTLS10)
		}
		tlsConn := tls.Client(conn, &tls.Config{
			InsecureSkipVerify: true, //nolint:gosec // the proxy is local-only; the WASM client validates nothing here, CredSSP binds to the delivered public key
			MinVersion:         tls.VersionTLS10,
			MaxVersion:         maxVersion,
		})
		if err := tlsConn.Handshake(); err != nil {
			return nil, nil, err
		}
		state := tlsConn.ConnectionState()
		chain := make([][]byte, 0, len(state.PeerCertificates))
		for _, c := range state.PeerCertificates {
			chain = append(chain, c.Raw)
		}
		return tlsConn, chain, nil
	}
	return nil, nil, fmt.Errorf("unknown TLS strategy %q", strategy)
}

// mstscClientHelloSpec builds a ClientHello that mirrors what mstsc sends:
// Schannel's TLS 1.2 cipher list (minus the SHA-384 suites, which utls
// cannot negotiate), supported_versions 1.2/1.1/1.0, and no ed25519 or
// encrypt-then-MAC — the two things old Schannel servers RST on.
func mstscClientHelloSpec(host string) *utls.ClientHelloSpec {
	suites := []uint16{
		0xC02C, 0xC02B, 0xC030, 0xC02F, 0xC023,
		0xC027, 0xC00A, 0xC009, 0xC014, 0xC013,
		0x009C, 0x003C, 0x0035, 0x002F,
	}
	// server_name_list: [u16 list length][name_type=0][u16 name length][name].
	// Built raw because mstsc sends the dialed address (even a literal IP) as
	// SNI, which utls' SNIExtension filters out.
	name := []byte(host)
	entry := append([]byte{0x00, byte(len(name) >> 8), byte(len(name))}, name...)
	sniRaw := append([]byte{byte(len(entry) >> 8), byte(len(entry))}, entry...)
	return &utls.ClientHelloSpec{
		CipherSuites: suites,
		Extensions: []utls.TLSExtension{
			&utls.GenericExtension{Id: 0x0000, Data: sniRaw},
			&utls.StatusRequestExtension{},
			&utls.SupportedVersionsExtension{Versions: []uint16{
				utls.VersionTLS12, utls.VersionTLS11, utls.VersionTLS10,
			}},
			&utls.SignatureAlgorithmsExtension{SupportedSignatureAlgorithms: []utls.SignatureScheme{
				0x0804, 0x0805, 0x0806, 0x0401, 0x0501, 0x0201,
				0x0403, 0x0503, 0x0203, 0x0202, 0x0601, 0x0603,
			}},
			&utls.SupportedCurvesExtension{Curves: []utls.CurveID{
				utls.CurveX25519, utls.CurveP256, utls.CurveP384,
			}},
			&utls.SupportedPointsExtension{SupportedPoints: []byte{0}},
			&utls.ExtendedMasterSecretExtension{},
			&utls.RenegotiationInfoExtension{Renegotiation: utls.RenegotiateOnceAsClient},
		},
	}
}


// --- X.224 helpers -----------------------------------------------------------

const (
	x224ProtoRDP      uint32 = 0x00
	x224ProtoSSL      uint32 = 0x01
	x224ProtoHybrid   uint32 = 0x02
	x224ProtoHybridEx uint32 = 0x08
	x224ProtoFailure  uint32 = 0xFFFFFFFF // marker: NEG_FAILURE seen
)

// readTPKTPDU reads one TPKT-framed PDU from conn (4-byte TPKT header, then
// the payload of the length the header declares).
func readTPKTPDU(conn net.Conn, timeout time.Duration) ([]byte, error) {
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var hdr [4]byte
	if _, err := readFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	if hdr[0] != 3 {
		return nil, fmt.Errorf("bad TPKT version %d", hdr[0])
	}
	length := int(hdr[2])<<8 | int(hdr[3])
	if length < 7 || length > 65535 {
		return nil, fmt.Errorf("bad TPKT length %d", length)
	}
	pdu := make([]byte, length)
	copy(pdu, hdr[:])
	if _, err := readFull(conn, pdu[4:]); err != nil {
		return nil, err
	}
	_ = conn.SetReadDeadline(time.Time{})
	return pdu, nil
}

func readFull(conn net.Conn, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// parseX224SelectedProtocol extracts selectedProtocol from an RDP negotiation
// response embedded in a TPKT-framed X.224 Connection Confirm. Returns
// x224ProtoRDP when no negotiation structure is present (classic X.224
// confirm = standard RDP security) and x224ProtoFailure on NEG_FAILURE.
func parseX224SelectedProtocol(tpktPDU []byte) uint32 {
	// [TPKT 4][LI 1][type 1][dst 2][src 2][class 1] then variable part.
	if len(tpktPDU) < 12 {
		return x224ProtoRDP
	}
	li := int(tpktPDU[4])
	if li < 7 {
		return x224ProtoRDP
	}
	end := 4 + 1 + li // TPKT + LI byte + LI-counted bytes
	if end > len(tpktPDU) {
		end = len(tpktPDU)
	}
	off := 11 // start of the variable part
	for off+2 <= end {
		vtype := tpktPDU[off]
		if vtype == 0x02 || vtype == 0x03 { // RDP_NEG_RSP / RDP_NEG_FAILURE
			if off+8 > end {
				return x224ProtoRDP
			}
			if vtype == 0x03 {
				return x224ProtoFailure
			}
			return binary.LittleEndian.Uint32(tpktPDU[off+4 : off+8])
		}
		off++
	}
	return x224ProtoRDP
}

// parseX224FailureCode extracts the failureCode field from a NEG_FAILURE
// Connection Confirm (TPKT-framed), for diagnostics logging.
func parseX224FailureCode(tpktPDU []byte) (uint32, bool) {
	// Same layout as parseX224SelectedProtocol: [TPKT 4][LI 1][type 1]
	// [dst 2][src 2][class 1], variable part holds RDP_NEG_FAILURE.
	if len(tpktPDU) < 12 {
		return 0, false
	}
	li := int(tpktPDU[4])
	if li < 7 {
		return 0, false
	}
	end := 4 + 1 + li
	if end > len(tpktPDU) {
		end = len(tpktPDU)
	}
	off := 11
	for off+8 <= end {
		if tpktPDU[off] == 0x03 { // RDP_NEG_FAILURE
			return binary.LittleEndian.Uint32(tpktPDU[off+4 : off+8]), true
		}
		off++
	}
	return 0, false
}

// --- RDCleanPath DER codec ---------------------------------------------------
//
// Schema (der crate, tag_mode EXPLICIT, all definite lengths):
//
//	RDCleanPathPdu ::= SEQUENCE {
//	  version             [0] INTEGER,
//	  error               [1] SEQUENCE {      -- RDCleanPathErr
//	                        error_code        [0] INTEGER,
//	                        http_status_code  [1] INTEGER OPTIONAL,
//	                        wsa_last_error    [2] INTEGER OPTIONAL,
//	                        tls_alert_code    [3] INTEGER OPTIONAL } OPTIONAL,
//	  destination         [2] UTF8String OPTIONAL,
//	  proxy_auth          [3] UTF8String OPTIONAL,
//	  server_auth         [4] UTF8String OPTIONAL,
//	  preconnection_blob  [5] UTF8String OPTIONAL,
//	  x224_connection_pdu [6] OCTET STRING OPTIONAL,
//	  server_cert_chain   [7] SEQUENCE OF OCTET STRING OPTIONAL,
//	  server_addr         [9] UTF8String OPTIONAL }
//
// Only the fields this proxy consumes are modeled; other tags are skipped on
// decode.

const rdcleanErrCodeGeneral uint16 = 1

// Mirrors ironrdp-rdcleanpath's NEGOTIATION_ERROR_CODE: the client only
// decodes a NegotiationErr (and surfaces the X.224 failure code) when the
// error field carries this code.
const rdcleanErrCodeNegotiation uint16 = 2

// rdcleanPathVersion mirrors ironrdp-rdcleanpath's VERSION_1 = BASE_VERSION+1
// (3389 + 1). detect() rejects any other value — this burned us once as a
// hardcoded 1, which the client reported as "detection failed (invalid PDU)".
const rdcleanPathVersion uint64 = 3389 + 1

// derTLV returns the tag byte and total length of the TLV starting at off.
func derTLV(buf []byte, off int) (byte, int, error) {
	if off+2 > len(buf) {
		return 0, 0, fmt.Errorf("der: truncated tag/length")
	}
	tag := buf[off]
	l := int(buf[off+1])
	hdr := 2
	if l&0x80 != 0 {
		n := l & 0x7f
		if n == 0 || n > 4 || off+2+n > len(buf) {
			return 0, 0, fmt.Errorf("der: unsupported length")
		}
		l = 0
		for i := 0; i < n; i++ {
			l = l<<8 | int(buf[off+2+i])
		}
		hdr = 2 + n
	}
	if off+hdr+l > len(buf) {
		return 0, 0, fmt.Errorf("der: truncated value")
	}
	return tag, hdr + l, nil
}

// derTLVBody returns the value bytes of the TLV starting at off.
func derTLVBody(buf []byte, off int) []byte {
	hdr := 2
	if int(buf[off+1])&0x80 != 0 {
		hdr = 2 + int(buf[off+1])&0x7f
	}
	_, n, err := derTLV(buf, off)
	if err != nil {
		return nil
	}
	return buf[off+hdr : off+n]
}

func derEncodeTLV(tag byte, body []byte) []byte {
	if len(body) < 0x80 {
		return append([]byte{tag, byte(len(body))}, body...)
	}
	lenBytes := make([]byte, 0, 4)
	for v := len(body); v > 0; v >>= 8 {
		lenBytes = append([]byte{byte(v)}, lenBytes...)
	}
	out := []byte{tag, 0x80 | byte(len(lenBytes))}
	out = append(out, lenBytes...)
	return append(out, body...)
}

func derExplicit(tagNum int, innerTag byte, body []byte) []byte {
	return derEncodeTLV(byte(0xA0|tagNum), derEncodeTLV(innerTag, body))
}

func derIntegerBytes(v uint64) []byte {
	if v == 0 {
		return []byte{0}
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte(v)}, b...)
		v >>= 8
	}
	return b
}

// decodeRDCleanPathRequest parses a DER RDCleanPathPdu and returns the
// Request fields the proxy consumes.
func decodeRDCleanPathRequest(der []byte) (destination, proxyAuth string, x224 []byte, err error) {
	tag, _, err := derTLV(der, 0)
	if err != nil {
		return "", "", nil, fmt.Errorf("outer: %w", err)
	}
	if tag != 0x30 {
		return "", "", nil, fmt.Errorf("outer tag 0x%x, want SEQUENCE", tag)
	}
	body := derTLVBody(der, 0)
	off := 0
	for off < len(body) {
		fieldTag, n, err := derTLV(body, off)
		if err != nil {
			return "", "", nil, err
		}
		if fieldTag&0xA0 == 0xA0 {
			// Unwrap the explicit context tag, then take the inner
			// element's value (skip its tag/length header).
			wrapped := derTLVBody(body, off)
			if len(wrapped) == 0 {
				off += n
				continue
			}
			inner := derTLVBody(wrapped, 0)
			switch fieldTag & 0x1f {
			case 2: // destination
				destination = string(inner)
			case 3: // proxy_auth
				proxyAuth = string(inner)
			case 6: // x224_connection_pdu
				x224 = inner
			}
		}
		off += n
	}
	if x224 == nil {
		return "", "", nil, fmt.Errorf("missing x224_connection_pdu")
	}
	return destination, proxyAuth, x224, nil
}

// encodeRDCleanPathResponse builds the Response PDU: version 1, the x.224
// confirm (TPKT included, forwarded verbatim), the server cert chain as DER
// certificates (empty for standard RDP security — the client ignores it in
// that mode) and the server address string.
func encodeRDCleanPathResponse(x224 []byte, certChain [][]byte, serverAddr string) ([]byte, error) {
	var fields []byte
	fields = append(fields, derExplicit(0, 0x02, derIntegerBytes(rdcleanPathVersion))...)
	fields = append(fields, derExplicit(6, 0x04, x224)...)

	chainBody := make([]byte, 0, 64)
	for _, cert := range certChain {
		chainBody = append(chainBody, derEncodeTLV(0x04, cert)...)
	}
	// [7] EXPLICIT wrapping the SEQUENCE OF OCTET STRING itself.
	fields = append(fields, derExplicit(7, 0x30, chainBody)...)
	fields = append(fields, derExplicit(9, 0x0C, []byte(serverAddr))...)
	return derEncodeTLV(0x30, fields), nil
}

// encodeRDCleanPathErr builds the GeneralErr PDU (error_code only; the
// human-readable reason stays in the proxy log).
func encodeRDCleanPathErr(code uint16) []byte {
	errSeq := derExplicit(0, 0x02, derIntegerBytes(uint64(code)))
	pdu := derEncodeTLV(0x30, append(
		derExplicit(0, 0x02, derIntegerBytes(rdcleanPathVersion)),
		derExplicit(1, 0x30, errSeq)...,
	))
	return pdu
}

// encodeRDCleanPathNegotiationErr mirrors the client's NegotiationErr branch:
// a negotiation-error PDU carrying the raw X.224 confirm (a NEG_FAILURE)
// without the TPKT header. The error field is mandatory — the client's
// decode falls into the Error branch (no destination/server_addr present)
// and fails with "RDCleanPath is missing error field" when it is absent,
// hiding the server's actual failure code.
func encodeRDCleanPathNegotiationErr(x224Resp []byte) []byte {
	if len(x224Resp) > 4 {
		x224Resp = x224Resp[4:]
	}
	errSeq := derExplicit(0, 0x02, derIntegerBytes(uint64(rdcleanErrCodeNegotiation)))
	pdu := derEncodeTLV(0x30, append(append(
		derExplicit(0, 0x02, derIntegerBytes(rdcleanPathVersion)),
		derExplicit(1, 0x30, errSeq)...,
	), derExplicit(6, 0x04, x224Resp)...))
	return pdu
}

const channelOptionInitialized uint32 = 0x80000000

// clientGCCData points into a validated MCS Connect Initial envelope.
type clientGCCData struct {
	fieldsPrefix []byte
	blocks       []byte
	legacyLength bool
}

func parseClientGCC(packet []byte) *clientGCCData {
	if len(packet) < 11 || packet[0] != 3 || packet[1] != 0 ||
		int(binary.BigEndian.Uint16(packet[2:4])) != len(packet) ||
		!bytes.Equal(packet[4:9], []byte{2, 0xf0, 0x80, 0x7f, 0x65}) {
		return nil
	}
	// Connect Initial has a two-byte BER tag (7f 65). derTLV can parse the
	// remainder starting at the second tag byte, including the BER length.
	_, n, err := derTLV(packet, 8)
	if err != nil || 8+n != len(packet) {
		return nil
	}
	fields := derTLVBody(packet, 8)
	originalFields := fields
	for _, expected := range []byte{4, 4, 1, 0x30, 0x30, 0x30} {
		tag, n, err := derTLV(fields, 0)
		if err != nil || tag != expected {
			return nil
		}
		fields = fields[n:]
	}
	tag, n, err := derTLV(fields, 0)
	if err != nil || tag != 4 || n != len(fields) {
		return nil
	}
	gcc := derTLVBody(fields, 0)
	// T.124 object identifier followed by a PER-length ConferenceCreateRequest.
	if len(gcc) < 8 || !bytes.Equal(gcc[:7], []byte{0, 5, 0, 20, 124, 0, 1}) {
		return nil
	}
	conferenceLength, header := perLength(gcc[7:])
	if header == 0 {
		return nil
	}
	conference := gcc[7+header:]
	// Conference name "1", one H.221 non-standard user data block ("Duca").
	// This is the GCC envelope emitted by ironrdp-web. Unknown envelopes are
	// passed through instead of searching arbitrary packet bytes for CS_NET.
	prefix := []byte{0, 8, 0, 16, 0, 1, 0xc0, 0, 'D', 'u', 'c', 'a'}
	if len(conference) < len(prefix) || !bytes.Equal(conference[:len(prefix)], prefix) {
		return nil
	}
	blocks := perLengthBody(conference[len(prefix):])
	// IronRDP's connectPDU length excludes the nested user-data length
	// determinant. Validate both that encoding and the full-envelope length.
	if blocks == nil || (conferenceLength != len(conference) && conferenceLength != len(prefix)+len(blocks)) {
		return nil
	}
	return &clientGCCData{fieldsPrefix: originalFields[:len(originalFields)-len(fields)], blocks: blocks, legacyLength: conferenceLength != len(conference)}
}

// Return the CHANNEL_DEF array inside GCC CS_NET, aliasing the input packet.
func clientStaticChannelDefinitions(packet []byte) []byte {
	gcc := parseClientGCC(packet)
	if gcc == nil {
		return nil
	}
	blocks := gcc.blocks
	var definitions []byte
	for len(blocks) > 0 {
		if len(blocks) < 4 {
			return nil
		}
		kind := binary.LittleEndian.Uint16(blocks)
		length := int(binary.LittleEndian.Uint16(blocks[2:]))
		if length < 4 || length > len(blocks) {
			return nil
		}
		if kind == 0xc003 { // TS_UD_CS_NET
			if length < 8 || definitions != nil {
				return nil
			}
			count := binary.LittleEndian.Uint32(blocks[4:])
			if count > 31 || length != 8+int(count)*12 {
				return nil
			}
			definitions = blocks[8:length]
		}
		blocks = blocks[length:]
	}
	return definitions
}

// perLengthBody reads the one/two-byte aligned-PER length used by GCC.
func perLengthBody(data []byte) []byte {
	length, header := perLength(data)
	if header == 0 || header+length != len(data) {
		return nil
	}
	return data[header:]
}

func perLength(data []byte) (length, header int) {
	if len(data) == 0 {
		return 0, 0
	}
	header, length = 1, int(data[0])
	if length&0x80 != 0 {
		if len(data) < 2 {
			return 0, 0
		}
		header, length = 2, (length&0x7f)<<8|int(data[1])
	}
	return length, header
}

// initializeLegacyStaticChannels restores the flag Windows 2008 R2 and other
// older servers require to create a static channel. ironrdp-svc currently
// advertises compression options alone. Newer servers ignore INITIALIZED;
// older servers treat a channel without it as an uninitialized placeholder.
func initializeLegacyStaticChannels(packet []byte) int {
	definitions := clientStaticChannelDefinitions(packet)
	changed := 0
	for len(definitions) >= 12 {
		options := binary.LittleEndian.Uint32(definitions[8:12])
		if options&channelOptionInitialized == 0 {
			binary.LittleEndian.PutUint32(definitions[8:12], options|channelOptionInitialized)
			changed++
		}
		definitions = definitions[12:]
	}
	return changed
}

// The published WASM build has no administrator-session extension. Apply the
// standard TS_UD_CS_CLUSTER request in the proxy before sending MCS to the
// server. Session ID zero with REDIRECTED_SESSIONID_FIELD_VALID requests the
// administrative/console session, as mstsc /admin does.
func requestRDPAdminSession(packet []byte) ([]byte, error) {
	gcc := parseClientGCC(packet)
	if gcc == nil {
		return nil, fmt.Errorf("invalid RDP MCS Connect Initial")
	}
	var cluster []byte
	for blocks := gcc.blocks; len(blocks) > 0; {
		if len(blocks) < 4 {
			return nil, fmt.Errorf("truncated RDP GCC header")
		}
		length := int(binary.LittleEndian.Uint16(blocks[2:]))
		if length < 4 || length > len(blocks) {
			return nil, fmt.Errorf("invalid RDP GCC block length")
		}
		if binary.LittleEndian.Uint16(blocks) == 0xc004 {
			if length != 12 || cluster != nil {
				return nil, fmt.Errorf("invalid RDP client cluster data")
			}
			cluster = blocks[:length]
		}
		blocks = blocks[length:]
	}
	if cluster != nil {
		binary.LittleEndian.PutUint32(cluster[4:], binary.LittleEndian.Uint32(cluster[4:])|2)
		binary.LittleEndian.PutUint32(cluster[8:], 0)
		return packet, nil
	}
	blocks := append(append([]byte{}, gcc.blocks...), []byte{4, 0xc0, 12, 0, 2, 0, 0, 0, 0, 0, 0, 0}...)
	// Both GCC PER length determinants are limited to fifteen bits.
	if len(blocks)+14 > 0x7fff {
		return nil, fmt.Errorf("RDP GCC data too large")
	}
	prefix := []byte{0, 8, 0, 16, 0, 1, 0xc0, 0, 'D', 'u', 'c', 'a'}
	conference := append(prefix, encodeRDPPERLength(len(blocks))...)
	conference = append(conference, blocks...)
	conferenceLength := len(conference)
	if gcc.legacyLength {
		conferenceLength = len(prefix) + len(blocks)
	}
	userData := []byte{0, 5, 0, 20, 124, 0, 1}
	userData = append(userData, encodeRDPPERLength(conferenceLength)...)
	userData = append(userData, conference...)
	fields := append(append([]byte{}, gcc.fieldsPrefix...), derEncodeTLV(4, userData)...)
	mcs := append([]byte{0x7f}, derEncodeTLV(0x65, fields)...)
	result := append([]byte{3, 0, 0, 0, 2, 0xf0, 0x80}, mcs...)
	if len(result) > 0xffff {
		return nil, fmt.Errorf("RDP MCS packet too large")
	}
	binary.BigEndian.PutUint16(result[2:], uint16(len(result)))
	return result, nil
}

func encodeRDPPERLength(length int) []byte {
	if length < 128 {
		return []byte{byte(length)}
	}
	return []byte{byte(length>>8) | 0x80, byte(length)}
}

// CredSSP TSRequest records precede MCS when NLA is negotiated. Leave them
// untouched, then buffer the first TPKT so fragmented WebSocket messages and
// later/coalesced RDP packets cannot interfere with GCC changes.
type rdpClientHandshake struct {
	adminSession bool
	done         bool
	pending      []byte
}

func (h *rdpClientHandshake) rewrite(data []byte) ([]byte, error) {
	if h.done || len(data) == 0 {
		return data, nil
	}
	h.pending = append(h.pending, data...)
	var output []byte
	for len(h.pending) > 0 && h.pending[0] == 0x30 {
		// TSRequest is a DER SEQUENCE. Track its complete length so a token
		// fragment starting with 0x03 cannot be mistaken for an RDP TPKT.
		if len(h.pending) < 2 {
			return output, nil
		}
		length, header := int(h.pending[1]), 2
		if length&0x80 != 0 {
			count := length & 0x7f
			if count == 0 || count > 4 {
				return nil, fmt.Errorf("invalid CredSSP record length")
			}
			if len(h.pending) < 2+count {
				return output, nil
			}
			length, header = 0, 2+count
			for _, b := range h.pending[2:header] {
				length = length<<8 | int(b)
			}
		}
		if length < 0 || length > 8*1024*1024 {
			return nil, fmt.Errorf("CredSSP record too large")
		}
		if len(h.pending) < header+length {
			return output, nil
		}
		output = append(output, h.pending[:header+length]...)
		h.pending = h.pending[header+length:]
	}
	if len(h.pending) == 0 {
		h.pending = nil
		return output, nil
	}
	if h.pending[0] != 3 {
		output = append(output, h.pending...)
		h.pending = nil
		return output, nil
	}
	if len(h.pending) < 4 {
		return output, nil
	}
	length := int(binary.BigEndian.Uint16(h.pending[2:4]))
	if h.pending[1] != 0 || length < 11 {
		return nil, fmt.Errorf("invalid RDP TPKT header")
	}
	if len(h.pending) < length {
		return output, nil
	}
	packet := h.pending[:length]
	initializeLegacyStaticChannels(packet)
	if h.adminSession {
		var err error
		packet, err = requestRDPAdminSession(packet)
		if err != nil {
			return nil, err
		}
	}
	output = append(output, packet...)
	output = append(output, h.pending[length:]...)
	h.pending = nil
	h.done = true
	return output, nil
}
