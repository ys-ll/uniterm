package session

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRDPConnectionUsesWasmProxy(t *testing.T) {
	manager := NewSessionManager()
	s, err := manager.Create("rdp", ConnectionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close(s.ID())
	if _, ok := s.(*RDPSession); !ok || s.Type() != "rdp" {
		t.Fatal("RDP must use the WASM proxy with the canonical session type")
	}
}

func TestRDPSessionDefersUpstreamUntilWasmHandshake(t *testing.T) {
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	desktop := NewRDPSession("deferred-upstream")
	err = desktop.Connect(ConnectionConfig{Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port})
	if err != nil {
		t.Fatal(err)
	}
	defer desktop.Disconnect()
	if desktop.ProxyAddr() == "" {
		t.Fatal("local proxy did not start")
	}
	_ = listener.SetDeadline(time.Now().Add(100 * time.Millisecond))
	conn, err := listener.Accept()
	if err == nil {
		conn.Close()
		t.Fatal("session opened a disposable TCP probe before the WASM handshake")
	}
	if timeout, ok := err.(net.Error); !ok || !timeout.Timeout() {
		t.Fatalf("unexpected accept error: %v", err)
	}
}

func rdpProxyRequest(target string) []byte {
	x224 := []byte{3, 0, 0, 19, 14, 0xe0, 0, 0, 0, 0, 0, 1, 0, 8, 0, 1, 0, 0, 0}
	return derEncodeTLV(0x30, concatBytes(
		derExplicit(0, 0x02, []byte{1}),
		derExplicit(2, 0x0c, []byte(target)),
		derExplicit(3, 0x0c, []byte("proxy-lifecycle-test")),
		derExplicit(6, 0x04, x224),
	))
}

func TestRDPProxyStopCancelsHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	requestReceived := make(chan error, 1)
	peerClosed := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			requestReceived <- err
			return
		}
		defer conn.Close()
		_, err = readTPKTPDU(conn, 3*time.Second)
		requestReceived <- err
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var buf [1]byte
		_, err = conn.Read(buf[:])
		peerClosed <- err
	}()
	proxy := NewRDPProxy(listener.Addr().String(), nil, "proxy-lifecycle-test", RDPOptions{})
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()
	ws, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.WriteMessage(websocket.BinaryMessage, rdpProxyRequest(listener.Addr().String())); err != nil {
		t.Fatal(err)
	}
	if err := <-requestReceived; err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { proxy.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop left the upstream handshake running")
	}
	select {
	case err := <-peerClosed:
		if err != io.EOF {
			t.Fatalf("handshake connection was not closed cleanly: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream socket was leaked while stopping")
	}
}

func TestRDPProxyOwnsWebSocketDuringRelay(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	peer := make(chan net.Conn, 1)
	serverError := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverError <- err
			return
		}
		if _, err := readTPKTPDU(conn, 3*time.Second); err != nil {
			conn.Close()
			serverError <- err
			return
		}
		response := []byte{3, 0, 0, 19, 14, 0xd0, 0, 0, 0x12, 0x34, 0, 2, 0, 8, 0, 0, 0, 0, 0}
		if _, err := conn.Write(response); err != nil {
			conn.Close()
			serverError <- err
			return
		}
		_ = conn.SetDeadline(time.Time{})
		peer <- conn
	}()
	proxy := NewRDPProxy(listener.Addr().String(), nil, "proxy-lifecycle-test", RDPOptions{})
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()
	ws, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.WriteMessage(websocket.BinaryMessage, rdpProxyRequest(listener.Addr().String())); err != nil {
		t.Fatal(err)
	}
	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := ws.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	var conn net.Conn
	select {
	case conn = <-peer:
	case err := <-serverError:
		t.Fatal(err)
	case <-time.After(3 * time.Second):
		t.Fatal("mock RDP handshake timed out")
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("first")); err != nil {
		t.Fatal(err)
	}
	if _, data, err := ws.ReadMessage(); err != nil || string(data) != "first" {
		t.Fatalf("first client relay failed: %q, %v", data, err)
	}
	duplicate, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer duplicate.Close()
	_ = duplicate.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := duplicate.ReadMessage(); err == nil {
		t.Fatal("duplicate WebSocket was accepted during an active relay")
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		t.Fatal("duplicate WebSocket was left open")
	}
	if _, err := conn.Write([]byte("still alive")); err != nil {
		t.Fatal(err)
	}
	if _, data, err := ws.ReadMessage(); err != nil || string(data) != "still alive" {
		t.Fatalf("duplicate client interrupted original connection: %q, %v", data, err)
	}
}

func TestRDPProxyReportsHandshakeError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = readTPKTPDU(conn, 3*time.Second)
	}()
	proxy := NewRDPProxy(listener.Addr().String(), nil, "proxy-lifecycle-test", RDPOptions{})
	reported := make(chan error, 1)
	proxy.SetOnError(func(err error) { reported <- err })
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()
	ws, _, err := websocket.DefaultDialer.Dial(addr, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ws.Close()
	if err := ws.WriteMessage(websocket.BinaryMessage, rdpProxyRequest(listener.Addr().String())); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-reported:
		if !bytes.Contains([]byte(err.Error()), []byte("RDP server read failed")) {
			t.Fatalf("upstream failure lost its cause: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("upstream error was hidden behind a generic RDCleanPath error")
	}
}

func TestRDPConfigurationRoundTrip(t *testing.T) {
	const saved = `{"type":"rdp","host":"example","rdpEngine":"microsoft","rdpSmartSizing":true,"rdpDomain":"EXAMPLE","rdpFixedWidth":1600,"rdpFixedHeight":900,"rdpAdminSession":true,"rdpEnableNLA":true}`
	var config ConnectionConfig
	if err := json.Unmarshal([]byte(saved), &config); err != nil {
		t.Fatal(err)
	}
	if !config.RdpAdminSession || !config.RdpEnableNLA || config.RdpDomain != "EXAMPLE" || config.RdpFixedWidth != 1600 {
		t.Fatal("supported RDP settings were lost")
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("rdpEngine")) || bytes.Contains(encoded, []byte("rdpSmartSizing")) {
		t.Fatal("obsolete engine-specific settings were reserialized")
	}
}

// The RDCleanPath codec must produce PDUs structurally identical to what the
// WASM client's der-crate decode expects: outer SEQUENCE, explicit context
// tags over INTEGER/OCTET STRING/UTF8String/SEQUENCE OF, definite lengths.

func TestResponseStructure(t *testing.T) {
	fakeCert := []byte{0x30, 0x03, 0x02, 0x01, 0x05} // minimal DER for a fake cert
	x224 := []byte{0x00, 0x00, 0x00, 0x0F, 0x06, 0xD0, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}

	pdu, err := encodeRDCleanPathResponse(x224, [][]byte{fakeCert}, "10.0.0.5:3389")
	if err != nil {
		t.Fatal(err)
	}

	tag, _, err := derTLV(pdu, 0)
	if err != nil || tag != 0x30 {
		t.Fatalf("outer tag %x err %v", tag, err)
	}
	body := derTLVBody(pdu, 0)

	off := 0
	seen := map[int]bool{}
	for off < len(body) {
		fieldTag, n, err := derTLV(body, off)
		if err != nil {
			t.Fatal(err)
		}
		if fieldTag&0xA0 != 0xA0 {
			t.Fatalf("tag %x is not context-specific constructed", fieldTag)
		}
		wrapped := derTLVBody(body, off) // the inner TLV, e.g. `02 01 01`
		switch fieldTag & 0x1f {
		case 0: // version
			if !bytes.Equal(derTLVBody(wrapped, 0), derIntegerBytes(rdcleanPathVersion)) {
				t.Fatalf("version inner = % x", wrapped)
			}
			seen[0] = true
		case 6: // x224
			if wrapped[0] != 0x04 {
				t.Fatalf("x224 inner tag %x", wrapped[0])
			}
			if !bytes.Equal(derTLVBody(wrapped, 0), x224) {
				t.Fatalf("x224 mismatch")
			}
			seen[6] = true
		case 7: // chain: SEQUENCE { OCTET STRING cert }
			if wrapped[0] != 0x30 {
				t.Fatalf("chain tag %x", wrapped[0])
			}
			chainBody := derTLVBody(wrapped, 0)
			certTag, _, err := derTLV(chainBody, 0)
			if err != nil || certTag != 0x04 {
				t.Fatalf("cert tag %x err %v", certTag, err)
			}
			if !bytes.Equal(derTLVBody(chainBody, 0), fakeCert) {
				t.Fatal("cert bytes mismatch")
			}
			seen[7] = true
		case 9: // server_addr
			if string(derTLVBody(wrapped, 0)) != "10.0.0.5:3389" {
				t.Fatalf("server_addr %q", string(wrapped))
			}
			seen[9] = true
		}
		off += n
	}
	for _, want := range []int{0, 6, 7, 9} {
		if !seen[want] {
			t.Fatalf("missing field %d", want)
		}
	}
}

func TestRequestDecode(t *testing.T) {
	x224 := bytes.Repeat([]byte{0xAB}, 23)
	dest := "rdp.example.com"
	auth := "tok123"

	req := derEncodeTLV(0x30, concatBytes(
		derExplicit(0, 0x02, []byte{1}),
		derExplicit(2, 0x0C, []byte(dest)),
		derExplicit(3, 0x0C, []byte(auth)),
		derExplicit(6, 0x04, x224),
	))

	gotDest, gotAuth, gotX224, err := decodeRDCleanPathRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if gotDest != dest || gotAuth != auth || !bytes.Equal(gotX224, x224) {
		t.Fatalf("dest=%q auth=%q x224len=%d", gotDest, gotAuth, len(gotX224))
	}
}

func TestParseX224SelectedProtocol(t *testing.T) {
	build := func(li int, variable []byte) []byte {
		pdu := []byte{0x00, 0x00, 0x00, byte(5 + li), byte(li), 0xD0, 0, 0, 0, 0, 0}
		return append(pdu, variable...)
	}

	// No negotiation structure → standard RDP security.
	if got := parseX224SelectedProtocol(build(6, nil)); got != x224ProtoRDP {
		t.Fatalf("plain confirm got 0x%x, want RDP", got)
	}

	// NEG_RSP selecting HYBRID (NLA).
	variable := []byte{0x02, 0x00, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00}
	if got := parseX224SelectedProtocol(build(14, variable)); got != x224ProtoHybrid {
		t.Fatalf("NEG_RSP got 0x%x, want HYBRID", got)
	}

	// NEG_FAILURE.
	variable = []byte{0x03, 0x00, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00}
	if got := parseX224SelectedProtocol(build(14, variable)); got != x224ProtoFailure {
		t.Fatalf("NEG_FAILURE got 0x%x, want failure marker", got)
	}
}

// The NegotiationErr PDU must carry the mandatory error field (code 2) before
// the x224 confirm — the client's decode requires it to reach the
// NegotiationErr branch instead of failing with "missing error field".
func TestNegotiationErrStructure(t *testing.T) {
	// TPKT-framed NEG_FAILURE with failureCode 5 (HYBRID_REQUIRED_BY_SERVER).
	x224 := []byte{0x00, 0x00, 0x00, 0x13, 0x0E, 0xD0, 0, 0, 0, 0, 0,
		0x03, 0x00, 0x08, 0x00, 0x05, 0x00, 0x00, 0x00}

	pdu := encodeRDCleanPathNegotiationErr(x224)

	body := derTLVBody(pdu, 0)
	off := 0
	seen := map[int]bool{}
	var errCode []byte
	for off < len(body) {
		fieldTag, n, err := derTLV(body, off)
		if err != nil {
			t.Fatal(err)
		}
		if fieldTag&0xA0 != 0xA0 {
			t.Fatalf("tag %x is not context-specific constructed", fieldTag)
		}
		wrapped := derTLVBody(body, off)
		switch fieldTag & 0x1f {
		case 0: // version
			if !bytes.Equal(derTLVBody(wrapped, 0), derIntegerBytes(rdcleanPathVersion)) {
				t.Fatalf("version inner = % x", wrapped)
			}
			seen[0] = true
		case 1: // error: SEQUENCE { [0] INTEGER code }
			if wrapped[0] != 0x30 {
				t.Fatalf("error tag %x", wrapped[0])
			}
			errBody := derTLVBody(wrapped, 0)
			codeTag, _, err := derTLV(errBody, 0)
			if err != nil || codeTag != 0xA0 {
				t.Fatalf("error_code tag %x err %v", codeTag, err)
			}
			errCode = derTLVBody(derTLVBody(errBody, 0), 0)
			seen[1] = true
		case 6: // x224, TPKT stripped
			if wrapped[0] != 0x04 {
				t.Fatalf("x224 inner tag %x", wrapped[0])
			}
			got := derTLVBody(wrapped, 0)
			if bytes.Equal(got, x224) {
				t.Fatal("x224 still carries the TPKT header")
			}
			if !bytes.Equal(got, x224[4:]) {
				t.Fatalf("x224 mismatch: % x", got)
			}
			seen[6] = true
		}
		off += n
	}
	for _, want := range []int{0, 1, 6} {
		if !seen[want] {
			t.Fatalf("missing field %d", want)
		}
	}
	if !bytes.Equal(errCode, []byte{2}) {
		t.Fatalf("error_code = % x, want 2 (negotiation)", errCode)
	}

	if code, ok := parseX224FailureCode(x224); !ok || code != 5 {
		t.Fatalf("parseX224FailureCode = %d, %v; want 5", code, ok)
	}
}

func concatBytes(chunks ...[]byte) []byte {
	var out []byte
	for _, c := range chunks {
		out = append(out, c...)
	}
	return out
}

const rdpWasmHandshakeScript = `
// Integration-test fixture: send the installed WASM build's MCS handshake
// through the local proxy to a mock RDP server, stopping before logon.
import fs from 'node:fs'
import { pathToFileURL } from 'node:url'
const moduleURL = pathToFileURL(process.env.RDP_TEST_WASM_MODULE)
const { initSync, SessionBuilder, DesktopSize, Extension } = await import(moduleURL.href)

globalThis.Window = class Window {
  static [Symbol.hasInstance](value) { return value === globalThis }
}
globalThis.window = globalThis
const NativeWebSocket = globalThis.WebSocket
globalThis.WebSocket = class extends NativeWebSocket {
  send(data) {
    this.pending = (this.pending ?? Promise.resolve())
      .then(() => data instanceof Blob ? data.arrayBuffer() : data)
      .then(bytes => this.sendBytes(bytes))
  }
  sendBytes(data) {
    const bytes = Buffer.from(data)
    if (process.env.RDP_TEST_CREDSSP_ONLY === 'true' && bytes[0] === 0x30) {
      fs.writeFileSync(process.env.RDP_PROBE_CAPTURE, bytes)
      super.send(data)
      setTimeout(() => process.exit(0), 200)
      return
    }
    if (bytes[0] === 3) {
      for (let i = 0; i + 8 <= bytes.length; i++) {
        if (bytes.readUInt16LE(i) !== 0xc003) continue
        const length = bytes.readUInt16LE(i + 2)
        const count = bytes.readUInt32LE(i + 4)
        if (length !== 8 + count * 12 || i + length > bytes.length) continue
        if (process.env.RDP_PROBE_CAPTURE) fs.writeFileSync(process.env.RDP_PROBE_CAPTURE, bytes)
        super.send(data)
        // The test server does not answer MCS. Stop here, before logon.
        setTimeout(() => process.exit(0), 200)
        return
      }
      console.error('No channel definitions in first MCS packet')
      process.exit(2)
    }
    super.send(data)
  }
}

initSync({ module: fs.readFileSync(new URL('ironrdp_bg.wasm', moduleURL)) })
const builder = new SessionBuilder()
  .username('uniterm-probe')
  .password('')
  .destination(process.env.RDP_TEST_TARGET.split(':')[0])
  .proxyAddress(process.env.RDP_PROBE_PROXY)
  .authToken(new URL(process.env.RDP_PROBE_PROXY).pathname.slice(1))
  .desktopSize(new DesktopSize(800, 600))
  .renderCanvas({})
builder.extension(new Extension('enable_credssp', process.env.RDP_TEST_CREDSSP === 'true'))
builder.setCursorStyleCallback(() => {})
builder.setCursorStyleCallbackContext(null)
builder.remoteClipboardChangedCallback(() => {})
builder.forceClipboardUpdateCallback(() => {})
builder.connect().catch(error => {
  console.error(typeof error.backtrace === 'function' ? error.backtrace() : String(error))
  process.exit(1)
})
setTimeout(() => { console.error('Probe timed out'); process.exit(1) }, 10000)
`

func runWasmChannelHandshake(t *testing.T, target string, options RDPOptions, credSSPOnly bool) []byte {
	t.Helper()
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not installed")
	}
	modulePath, err := filepath.Abs("../../frontend/node_modules/@ys-ll/ironrdp-wasm/ironrdp.js")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(modulePath); err != nil {
		t.Skip("IronRDP WASM package not installed")
	}
	proxy := NewRDPProxy(target, nil, "wasm-channel-test", options)
	addr, err := proxy.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer proxy.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "handshake.mjs")
	if err := os.WriteFile(scriptPath, []byte(rdpWasmHandshakeScript), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, "node", scriptPath)
	capture := filepath.Join(dir, "mcs.bin")
	cmd.Env = append(os.Environ(), "RDP_PROBE_PROXY="+addr, "RDP_PROBE_CAPTURE="+capture, "RDP_TEST_WASM_MODULE="+modulePath,
		"RDP_TEST_TARGET="+target, fmt.Sprintf("RDP_TEST_CREDSSP=%t", options.EnableCredSSP),
		fmt.Sprintf("RDP_TEST_CREDSSP_ONLY=%t", credSSPOnly))
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("WASM handshake: %v\n%s", err, output)
	}
	data, err := os.ReadFile(capture)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// Exercise the installed WASM and the actual WebSocket -> TLS proxy together.
// The mock RDP server stops at MCS, so no credentials or desktop input is sent.
func TestRDPProxyInitializesWasmChannels(t *testing.T) {
	for _, options := range []RDPOptions{{}, {EnableCredSSP: true}, {AdminSession: true}, {EnableCredSSP: true, AdminSession: true}} {
		t.Run(fmt.Sprintf("credssp=%t/admin=%t", options.EnableCredSSP, options.AdminSession), func(t *testing.T) {
			testWasmProxyHandshake(t, options)
		})
	}
}

func testWasmProxyHandshake(t *testing.T, options RDPOptions) {
	certificateServer := httptest.NewTLSServer(nil)
	certificate := certificateServer.TLS.Certificates[0]
	certificateServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	stop := make(chan struct{})
	defer close(stop)
	received := make(chan []byte, 1)
	serverError := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverError <- err
			return
		}
		defer conn.Close()
		request, err := readTPKTPDU(conn, 5*time.Second)
		if err != nil {
			serverError <- err
			return
		}
		protocols := binary.LittleEndian.Uint32(request[len(request)-4:])
		if protocols&x224ProtoSSL == 0 {
			serverError <- fmt.Errorf("TLS login not offered: 0x%x", protocols)
			return
		}
		if (protocols&x224ProtoHybrid != 0) != options.EnableCredSSP {
			serverError <- fmt.Errorf("NLA setting not applied to negotiation: 0x%x", protocols)
			return
		}
		response := []byte{3, 0, 0, 19, 14, 0xd0, 0, 0, 0x12, 0x34, 0, 2, 0, 8, 0, 1, 0, 0, 0}
		if _, err := conn.Write(response); err != nil {
			serverError <- err
			return
		}
		secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		if err := secured.Handshake(); err != nil {
			serverError <- err
			return
		}
		packet, err := readTPKTPDU(secured, 5*time.Second)
		if err != nil {
			serverError <- err
			return
		}
		received <- packet
		<-stop
	}()
	original := runWasmChannelHandshake(t, listener.Addr().String(), options, false)
	select {
	case packet := <-received:
		definitions := clientStaticChannelDefinitions(packet)
		if len(definitions) != 12 || string(bytes.TrimRight(definitions[:8], "\x00")) != "cliprdr" {
			t.Fatalf("unexpected channel definitions: %x", definitions)
		}
		flags := binary.LittleEndian.Uint32(definitions[8:])
		if flags&channelOptionInitialized == 0 {
			t.Fatalf("server received uninitialized clipboard channel: 0x%x", flags)
		}
		initializeLegacyStaticChannels(original)
		if options.AdminSession {
			adminCluster(t, packet)
			original, err = requestRDPAdminSession(original)
			if err != nil {
				t.Fatal(err)
			}
		}
		if !bytes.Equal(packet, original) {
			t.Fatal("proxy changed bytes outside requested handshake options")
		}
	case err := <-serverError:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("mock RDP server did not receive MCS Connect Initial")
	}
}

func TestRDPProxyRelaysCredSSP(t *testing.T) {
	certServer := httptest.NewTLSServer(nil)
	certificate := certServer.TLS.Certificates[0]
	certServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan []byte, 1)
	serverError := make(chan error, 1)
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverError <- err
			return
		}
		defer conn.Close()
		request, err := readTPKTPDU(conn, 5*time.Second)
		if err != nil {
			serverError <- err
			return
		}
		if binary.LittleEndian.Uint32(request[len(request)-4:])&x224ProtoHybrid == 0 {
			serverError <- fmt.Errorf("CredSSP negotiation was downgraded")
			return
		}
		response := []byte{3, 0, 0, 19, 14, 0xd0, 0, 0, 0x12, 0x34, 0, 2, 0, 8, 0, 2, 0, 0, 0}
		if _, err = conn.Write(response); err != nil {
			serverError <- err
			return
		}
		secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		if err = secured.Handshake(); err != nil {
			serverError <- err
			return
		}
		_ = secured.SetReadDeadline(time.Now().Add(5 * time.Second))
		header := make([]byte, 2)
		if _, err = io.ReadFull(secured, header); err != nil {
			serverError <- err
			return
		}
		length := int(header[1])
		if length&0x80 != 0 {
			count := length & 0x7f
			if count == 0 || count > 4 {
				serverError <- fmt.Errorf("invalid CredSSP DER length")
				return
			}
			extra := make([]byte, count)
			if _, err = io.ReadFull(secured, extra); err != nil {
				serverError <- err
				return
			}
			header = append(header, extra...)
			length = 0
			for _, b := range extra {
				length = length<<8 | int(b)
			}
		}
		if length < 0 || length > 1<<20 {
			serverError <- fmt.Errorf("unexpected CredSSP token size")
			return
		}
		body := make([]byte, length)
		if _, err = io.ReadFull(secured, body); err != nil {
			serverError <- err
			return
		}
		received <- append(header, body...)
		<-stop
	}()
	original := runWasmChannelHandshake(t, listener.Addr().String(), RDPOptions{EnableCredSSP: true}, true)
	select {
	case packet := <-received:
		if !bytes.Equal(packet, original) || packet[0] != 0x30 || !bytes.Contains(packet, []byte("NTLMSSP")) {
			t.Fatal("CredSSP token did not reach the server unchanged")
		}
	case err := <-serverError:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not receive the WASM client's CredSSP negotiation")
	}
}

func perTestLength(length int) []byte {
	if length < 128 {
		return []byte{byte(length)}
	}
	return []byte{byte(length>>8) | 0x80, byte(length)}
}

func channelInitialFixture(options []uint32, legacyLength bool) []byte {
	network := make([]byte, 8+12*len(options))
	binary.LittleEndian.PutUint16(network, 0xc003)
	binary.LittleEndian.PutUint16(network[2:], uint16(len(network)))
	binary.LittleEndian.PutUint32(network[4:], uint32(len(options)))
	for i, flags := range options {
		copy(network[8+i*12:], []byte("cliprdr\x00"))
		binary.LittleEndian.PutUint32(network[16+i*12:], flags)
	}
	// A decoy name/options inside CS_CORE must never be patched.
	core := append([]byte{1, 0xc0, 16, 0}, []byte("cliprdr\x00\x00\x00\x80\x00")...)
	blocks := append(core, network...)
	prefix := []byte{0, 8, 0, 16, 0, 1, 0xc0, 0, 'D', 'u', 'c', 'a'}
	conference := append(prefix, perTestLength(len(blocks))...)
	conference = append(conference, blocks...)
	length := len(conference)
	if legacyLength {
		length = len(prefix) + len(blocks)
	}
	gcc := []byte{0, 5, 0, 20, 124, 0, 1}
	gcc = append(gcc, perTestLength(length)...)
	gcc = append(gcc, conference...)
	fields := []byte{4, 1, 1, 4, 1, 1, 1, 1, 0xff, 0x30, 0, 0x30, 0, 0x30, 0}
	fields = append(fields, derEncodeTLV(4, gcc)...)
	mcs := append([]byte{0x7f}, derEncodeTLV(0x65, fields)...)
	packet := append([]byte{3, 0, 0, 0, 2, 0xf0, 0x80}, mcs...)
	binary.BigEndian.PutUint16(packet[2:], uint16(len(packet)))
	return packet
}

func TestLegacyStaticChannelInitialization(t *testing.T) {
	for _, legacyLength := range []bool{false, true} {
		options := []uint32{0x00800000, 0, 0xc0a00000}
		packet := channelInitialFixture(options, legacyLength)
		before := bytes.Clone(packet)
		if changed := initializeLegacyStaticChannels(packet); changed != 2 {
			t.Fatalf("initialized=%d, want 2 (legacy GCC length=%t)", changed, legacyLength)
		}
		definitions := clientStaticChannelDefinitions(packet)
		if len(definitions) != len(options)*12 {
			t.Fatalf("bad definitions: %x", definitions)
		}
		for i, original := range options {
			got := binary.LittleEndian.Uint32(definitions[i*12+8:])
			if got != original|channelOptionInitialized {
				t.Fatalf("channel %d flags=%x", i, got)
			}
		}
		if initializeLegacyStaticChannels(packet) != 0 {
			t.Fatal("patch must be idempotent")
		}
		differences := 0
		for i := range packet {
			if packet[i] != before[i] {
				differences++
			}
		}
		if differences != 2 {
			t.Fatalf("changed %d bytes outside the two initialization flags", differences)
		}
	}
}

func TestLegacyStaticChannelsRejectTruncatedPackets(t *testing.T) {
	packet := channelInitialFixture([]uint32{0x00800000}, false)
	for i := 0; i < len(packet); i++ {
		truncated := bytes.Clone(packet[:i])
		before := bytes.Clone(truncated)
		if initializeLegacyStaticChannels(truncated) != 0 || !bytes.Equal(before, truncated) {
			t.Fatalf("modified truncated packet of length %d", i)
		}
	}
}

func TestLegacyStaticChannelsRejectInvalidCount(t *testing.T) {
	packet := channelInitialFixture([]uint32{0x00800000}, false)
	index := bytes.Index(packet, []byte{3, 0xc0, 20, 0})
	if index < 0 {
		t.Fatal("missing test network block")
	}
	binary.LittleEndian.PutUint32(packet[index+4:], 2)
	before := bytes.Clone(packet)
	if initializeLegacyStaticChannels(packet) != 0 || !bytes.Equal(before, packet) {
		t.Fatal("modified invalid network block")
	}
}

func TestLegacyStaticChannelsLongGCCLengths(t *testing.T) {
	options := make([]uint32, 20)
	packet := channelInitialFixture(options, true)
	if changed := initializeLegacyStaticChannels(packet); changed != len(options) {
		t.Fatalf("initialized %d of %d", changed, len(options))
	}
}

func adminCluster(t *testing.T, packet []byte) []byte {
	t.Helper()
	gcc := parseClientGCC(packet)
	if gcc == nil {
		t.Fatal("invalid GCC lengths after admin-session request")
	}
	for blocks := gcc.blocks; len(blocks) >= 4; {
		length := int(binary.LittleEndian.Uint16(blocks[2:]))
		if length < 4 || length > len(blocks) {
			t.Fatal("invalid GCC block")
		}
		if binary.LittleEndian.Uint16(blocks) == 0xc004 {
			return blocks[:length]
		}
		blocks = blocks[length:]
	}
	t.Fatal("admin cluster missing")
	return nil
}

func TestRDPAdminSessionGCC(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		for _, count := range []int{1, 8, 20} {
			original := channelInitialFixture(make([]uint32, count), legacy)
			packet, err := requestRDPAdminSession(bytes.Clone(original))
			if err != nil {
				t.Fatal(err)
			}
			cluster := adminCluster(t, packet)
			if binary.LittleEndian.Uint32(cluster[4:])&2 == 0 || binary.LittleEndian.Uint32(cluster[8:]) != 0 {
				t.Fatal("admin mode requires valid redirected session ID zero")
			}
			if !bytes.Equal(clientStaticChannelDefinitions(original), clientStaticChannelDefinitions(packet)) {
				t.Fatal("admin request changed clipboard channel data")
			}
			// An existing cluster must retain unrelated flags and packet lengths.
			binary.LittleEndian.PutUint32(cluster[4:], 0x41)
			binary.LittleEndian.PutUint32(cluster[8:], 25)
			again, err := requestRDPAdminSession(packet)
			if err != nil || len(again) != len(packet) {
				t.Fatal("existing cluster duplicated")
			}
			cluster = adminCluster(t, again)
			if binary.LittleEndian.Uint32(cluster[4:]) != 0x43 || binary.LittleEndian.Uint32(cluster[8:]) != 0 {
				t.Fatal("existing cluster incorrectly modified")
			}
		}
	}
}

func TestRDPAdminSessionRejectsTruncation(t *testing.T) {
	packet := channelInitialFixture([]uint32{0}, true)
	for length := 0; length < len(packet); length++ {
		if _, err := requestRDPAdminSession(bytes.Clone(packet[:length])); err == nil {
			t.Fatalf("accepted %d-byte truncated packet", length)
		}
	}
}

func TestRDPHandshakeAfterCredSSP(t *testing.T) {
	for _, admin := range []bool{false, true} {
		h := rdpClientHandshake{adminSession: admin}
		credssp := []byte{0x30, 3, 2, 1, 6}
		if got, err := h.rewrite(credssp); err != nil || !bytes.Equal(got, credssp) || h.done {
			t.Fatal("CredSSP consumed MCS rewrite slot")
		}
		packet := channelInitialFixture([]uint32{0}, true)
		for _, fragment := range [][]byte{packet[:2], packet[2:10]} {
			if got, err := h.rewrite(fragment); err != nil || len(got) != 0 {
				t.Fatal("partial MCS forwarded before lengths can be updated")
			}
		}
		suffix := []byte{3, 0, 0, 8, 2, 0xf0, 0x80, 4}
		got, err := h.rewrite(append(bytes.Clone(packet[10:]), suffix...))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(got, suffix) {
			t.Fatal("coalesced packet lost")
		}
		length := int(binary.BigEndian.Uint16(got[2:4]))
		definitions := clientStaticChannelDefinitions(got[:length])
		if len(definitions) != 12 || binary.LittleEndian.Uint32(definitions[8:])&channelOptionInitialized == 0 {
			t.Fatal("clipboard channel not initialized after NLA")
		}
		if admin {
			adminCluster(t, got[:length])
		}
		if next, err := h.rewrite(suffix); err != nil || !bytes.Equal(next, suffix) {
			t.Fatal("later RDP traffic modified")
		}
	}
}

func TestRDPCredSSPFragmentBoundaries(t *testing.T) {
	record := []byte{0x30, 0x81, 0x80}
	record = append(record, bytes.Repeat([]byte{3}, 128)...)
	for boundary := 1; boundary < len(record); boundary++ {
		h := rdpClientHandshake{adminSession: true}
		first, err := h.rewrite(record[:boundary])
		if err != nil || len(first) != 0 {
			t.Fatalf("partial CredSSP at %d: %v", boundary, err)
		}
		packet := channelInitialFixture([]uint32{0}, true)
		rest, err := h.rewrite(append(bytes.Clone(record[boundary:]), packet...))
		if err != nil || !bytes.HasPrefix(rest, record) {
			t.Fatalf("CredSSP changed at %d: %v", boundary, err)
		}
		adminCluster(t, rest[len(record):])
	}
}
