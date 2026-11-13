//go:build linux && !android && !server

package platform

import "testing"

func TestLinuxDesktopKeyboardGTK3Capture(t *testing.T) {
	grabs, releases := 0, 0
	b := &linuxDesktopKeyboard{
		invoke:  func(fn func()) { fn() },
		visible: func(uintptr) int32 { return 1 }, windowActive: func(uintptr) int32 { return 1 },
		widgetWindow:  func(window uintptr) uintptr { return window + 1 },
		windowDisplay: func(uintptr) uintptr { return 3 }, defaultSeat: func(uintptr) uintptr { return 4 },
		grabSeat: func(seat, window uintptr, capabilities uint32, ownerEvents int32, _, _, _, _ uintptr) int32 {
			if seat != 4 || window != 2 || capabilities != 8 || ownerEvents != 1 {
				t.Fatal("grab must target the application keyboard while preserving webview event delivery")
			}
			grabs++
			return 0
		},
		ungrabSeat: func(seat uintptr) {
			if seat != 4 {
				t.Fatal("wrong seat released")
			}
			releases++
		},
	}
	if !b.capture(1) || !b.focused() || grabs != 1 {
		t.Fatal("keyboard not captured")
	}
	b.release()
	b.close()
	if b.focused() || releases != 1 {
		t.Fatal("keyboard not released exactly once")
	}
}

func TestLinuxDesktopKeyboardFailedGrabDoesNotReleaseAnotherOwner(t *testing.T) {
	b := &linuxDesktopKeyboard{
		invoke:  func(fn func()) { fn() },
		visible: func(uintptr) int32 { return 1 }, windowActive: func(uintptr) int32 { return 1 },
		widgetWindow: func(uintptr) uintptr { return 2 }, windowDisplay: func(uintptr) uintptr { return 3 },
		defaultSeat: func(uintptr) uintptr { return 4 },
		grabSeat:    func(uintptr, uintptr, uint32, int32, uintptr, uintptr, uintptr, uintptr) int32 { return 1 },
		ungrabSeat:  func(uintptr) { t.Fatal("failed grab must not ungrab another owner") },
	}
	if b.capture(1) {
		t.Fatal("failed grab reported success")
	}
	b.close()
}

func TestLinuxDesktopKeyboardGTK4InhibitsAndRestores(t *testing.T) {
	inhibited, restored := 0, 0
	b := &linuxDesktopKeyboard{
		invoke:  func(fn func()) { fn() },
		visible: func(uintptr) int32 { return 1 }, windowActive: func(uintptr) int32 { return 1 },
		nativeSurface: func(uintptr) uintptr { return 9 },
		inhibit: func(surface, event uintptr) {
			if surface != 9 || event != 0 {
				t.Fatal("wrong surface")
			}
			inhibited++
		},
		restore: func(surface uintptr) {
			if surface != 9 {
				t.Fatal("wrong surface")
			}
			restored++
		},
	}
	if !b.capture(1) || inhibited != 1 {
		t.Fatal("shortcuts not inhibited")
	}
	b.close()
	if restored != 1 {
		t.Fatal("shortcuts not restored")
	}
}
