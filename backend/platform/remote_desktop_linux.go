//go:build linux && !android && !server

package platform

import (
	"fmt"

	"github.com/ebitengine/purego"
)

type RDPKeyboardCapture struct {
	*desktopKeyboardCapture
}

type linuxDesktopKeyboard struct {
	invoke        func(func())
	window        uintptr
	seat          uintptr
	gtkMajor      func() uint32
	visible       func(uintptr) int32
	windowActive  func(uintptr) int32
	widgetWindow  func(uintptr) uintptr
	windowDisplay func(uintptr) uintptr
	defaultSeat   func(uintptr) uintptr
	grabSeat      func(uintptr, uintptr, uint32, int32, uintptr, uintptr, uintptr, uintptr) int32
	ungrabSeat    func(uintptr)
	nativeSurface func(uintptr) uintptr
	inhibit       func(uintptr, uintptr)
	restore       func(uintptr)
}

func NewRDPKeyboardCapture(emit func(RDPKeyEvent), dispatch ...func(func())) (*RDPKeyboardCapture, error) {
	if len(dispatch) == 0 || dispatch[0] == nil {
		return nil, fmt.Errorf("desktop keyboard capture requires a UI dispatcher")
	}
	capture := newDesktopKeyboardCapture(emit)
	b := &linuxDesktopKeyboard{invoke: dispatch[0]}
	if err := bindDesktopKeyboardFunctions(purego.RTLD_DEFAULT, map[string]any{
		"gtk_get_major_version":  &b.gtkMajor,
		"gtk_widget_get_visible": &b.visible,
		"gtk_window_is_active":   &b.windowActive,
	}); err != nil {
		return nil, err
	}
	functions := map[string]any{
		"gtk_native_get_surface":                &b.nativeSurface,
		"gdk_toplevel_inhibit_system_shortcuts": &b.inhibit,
		"gdk_toplevel_restore_system_shortcuts": &b.restore,
	}
	if b.gtkMajor() < 4 {
		functions = map[string]any{
			"gtk_widget_get_window":        &b.widgetWindow,
			"gdk_window_get_display":       &b.windowDisplay,
			"gdk_display_get_default_seat": &b.defaultSeat,
			"gdk_seat_grab":                &b.grabSeat,
			"gdk_seat_ungrab":              &b.ungrabSeat,
		}
	}
	if err := bindDesktopKeyboardFunctions(purego.RTLD_DEFAULT, functions); err != nil {
		return nil, err
	}
	capture.backend = b
	go capture.run()
	return &RDPKeyboardCapture{capture}, nil
}

func bindDesktopKeyboardFunctions(library uintptr, functions map[string]any) error {
	for name, target := range functions {
		symbol, err := purego.Dlsym(library, name)
		if err != nil {
			return fmt.Errorf("desktop keyboard API %s: %w", name, err)
		}
		purego.RegisterFunc(target, symbol)
	}
	return nil
}

func (b *linuxDesktopKeyboard) capture(window uintptr) bool {
	var enabled bool
	b.invoke(func() {
		b.releaseOnUI()
		if window == 0 || b.visible(window) == 0 || b.windowActive(window) == 0 {
			return
		}
		if b.grabSeat != nil {
			native := b.widgetWindow(window)
			if native == 0 {
				return
			}
			seat := b.defaultSeat(b.windowDisplay(native))
			if seat == 0 || b.grabSeat(seat, native, 8, 1, 0, 0, 0, 0) != 0 {
				return
			}
			b.seat = seat
		} else {
			surface := b.nativeSurface(window)
			if surface == 0 {
				return
			}
			b.inhibit(surface, 0)
		}
		b.window, enabled = window, true
	})
	return enabled
}

func (b *linuxDesktopKeyboard) focused() bool {
	var focused bool
	b.invoke(func() {
		focused = b.window != 0 && b.visible(b.window) != 0 && b.windowActive(b.window) != 0
	})
	return focused
}

func (b *linuxDesktopKeyboard) releaseOnUI() {
	if b.window == 0 {
		return
	}
	if b.grabSeat != nil {
		b.ungrabSeat(b.seat)
	} else if surface := b.nativeSurface(b.window); surface != 0 {
		b.restore(surface)
	}
	b.window, b.seat = 0, 0
}

func (b *linuxDesktopKeyboard) release() { b.invoke(b.releaseOnUI) }
func (b *linuxDesktopKeyboard) close()   { b.release() }
