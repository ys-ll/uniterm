package session

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// buildX224ConnectionRequest must produce the frame layout the proxy's own
// helpers (rewriteX224SslOnly / parseX224SelectedProtocol) expect — the probe
// runs the same negotiation the session will.
func TestBuildX224ConnectionRequestLayout(t *testing.T) {
	pdu := buildX224ConnectionRequest(x224ProtoHybrid)
	if len(pdu) != 19 {
		t.Fatalf("expected 19-byte X.224 request, got %d", len(pdu))
	}
	if pdu[0] != 3 || pdu[4] != 14 || pdu[5] != 0xE0 || pdu[11] != 0x01 || pdu[13] != 8 {
		t.Fatalf("unexpected request layout: %v", pdu)
	}
	if got := binary.LittleEndian.Uint32(pdu[15:19]); got != x224ProtoHybrid {
		t.Fatalf("expected HYBRID requestedProtocols, got 0x%x", got)
	}
	// rewriteX224SslOnly only fires on frames >= 20 bytes (real client
	// requests carry more variable part), so it does not apply to the
	// synthesized 19-byte probe request.
}

// vncAuthResponse: deterministic 16-byte response keyed by the first 8
// password chars (VNC passwords are 8-char truncated).
func TestVncAuthResponse(t *testing.T) {
	challenge := make([]byte, 16)
	a := vncAuthResponse("longpassword", challenge)
	b := vncAuthResponse("longpass", challenge)
	if !bytes.Equal(a, b) {
		t.Fatalf("passwords sharing the first 8 chars must match")
	}
	if len(a) != 16 {
		t.Fatalf("expected 16-byte response, got %d", len(a))
	}
	c := vncAuthResponse("otherpass", challenge)
	if bytes.Equal(a, c) {
		t.Fatalf("different passwords must produce different responses")
	}
}

// SpiceLinkMess must be encoded exactly as the wire format specifies so the
// probe keeps interop with real servers.
func TestSpiceLinkMessEncoding(t *testing.T) {
	msg := spiceProbeLinkMess()
	if string(msg[0:4]) != "REDQ" {
		t.Fatalf("bad magic: %q", msg[0:4])
	}
	if binary.LittleEndian.Uint32(msg[4:8]) != 2 || binary.LittleEndian.Uint32(msg[8:12]) != 2 {
		t.Fatalf("expected protocol version 2.2")
	}
	if binary.LittleEndian.Uint32(msg[12:16]) != 18 {
		t.Fatalf("size must cover the 14-byte body plus one cap word, got %d", binary.LittleEndian.Uint32(msg[12:16]))
	}
	if msg[20] != spiceChannelMain {
		t.Fatalf("channel_type must be main(1)")
	}
}

// ProbeConnection must route the four desktop types to their desktop probes
// (closed-port dial errors name the protocol), not the unsupported default.
func TestDesktopProbeDispatchMentionsType(t *testing.T) {
	for _, typ := range []string{"rdp", "vnc", "spice", "x11-desktop"} {
		t.Run(typ, func(t *testing.T) {
			config := ConnectionConfig{Type: typ, Host: "127.0.0.1", Port: 1}
			_, err := ProbeConnection(config)
			if err == nil {
				t.Fatalf("expected error probing closed port")
			}
			if strings.Contains(err.Error(), "does not support connection testing") {
				t.Fatalf("type %q was not dispatched to a desktop probe: %v", typ, err)
			}
			if !strings.Contains(err.Error(), typ) {
				t.Fatalf("error should mention the probed type %q, got: %v", typ, err)
			}
		})
	}
}
