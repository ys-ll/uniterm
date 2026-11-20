//go:build windows

package platform

import (
	"testing"
	"time"
)

func TestRDPKeyboardHookLifecycle(t *testing.T) {
	// Install a real hook with capture disabled. No user input is intercepted.
	capture, err := NewRDPKeyboardCapture(func(RDPKeyEvent) {})
	if err != nil {
		t.Fatal(err)
	}
	if capture.SetCapture("inactive", 0) {
		t.Fatal("zero HWND must never capture")
	}
	capture.Release("stale")
	capture.Close()
	capture.Close()
	if capture.SetCapture("closed", 1) {
		t.Fatal("closed hook must never capture")
	}
}

func TestRDPKeyboardHookLeaseAndStaleRelease(t *testing.T) {
	// A fabricated HWND cannot match the user's foreground window. Exercise
	// renewal and ownership without capturing any real desktop input.
	c := &RDPKeyboardCapture{wake: make(chan struct{}, 1)}
	if !c.SetCapture("first", 1) {
		t.Fatal("capture not activated")
	}
	c.state.key(0x38, true, true, false)
	c.expires = time.Now().Add(-time.Second)
	if !c.SetCapture("first", 1) || !c.expires.After(time.Now()) {
		t.Fatal("lease was not renewed")
	}
	if event, _ := c.state.key(0x0f, true, true, false); event.CaptureID != "first" {
		t.Fatal("renewal lost modifier state")
	}
	if len(c.queue) != 0 {
		t.Fatal("renewal must not release held keys")
	}
	c.SetCapture("second", 1)
	c.Release("first")
	if c.state.captureID != "second" {
		t.Fatal("stale blur disabled newly focused panel")
	}
	if len(c.queue) != 1 || !c.queue[0].Reset || c.queue[0].CaptureID != "first" {
		t.Fatal("panel change must release old remote inputs")
	}
}

func TestRDPKeyboardHookFullscreenEscapeFlagRefresh(t *testing.T) {
	c := &RDPKeyboardCapture{wake: make(chan struct{}, 1)}
	if !c.SetShortcutCapture("vnc", 1, true) {
		t.Fatal("capture not activated")
	}
	if !c.escCapture {
		t.Fatal("fullscreen Escape capture was not enabled")
	}
	if !c.SetShortcutCapture("vnc", 1, false) {
		t.Fatal("flag refresh failed")
	}
	if c.escCapture {
		t.Fatal("flag refresh did not clear fullscreen Escape capture")
	}
	if len(c.queue) != 0 || c.state.captureID != "vnc" {
		t.Fatal("flag refresh must keep the capture without releasing keys")
	}
}

func TestNormalizeScanStripsShiftE0Prefix(t *testing.T) {
	// Keyboards reporting Shift with an E0 prefix must still reach the remote
	// as plain set-1 Shift; 0xE02A/0xE036 are invalid on the wire.
	for _, tc := range []struct{ in, want uint16 }{
		{0xe036, 0x36}, {0xe02a, 0x2a}, // E0-prefixed Shift, both sides
		{0x2a, 0x2a}, {0x36, 0x36}, // ordinary Shift passes through
		{0xe01d, 0xe01d}, {0xe053, 0xe053}, {0x2f, 0x2f}, // real extended keys untouched
		{0x00, 0x00},
	} {
		if got := normalizeScan(tc.in); got != tc.want {
			t.Fatalf("normalizeScan(%#x) = %#x, want %#x", tc.in, got, tc.want)
		}
	}
}
