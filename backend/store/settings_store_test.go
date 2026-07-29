package store

import (
	"testing"
)

func TestTerminalSettingsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := NewSettingsStoreWithDir(dir)
	if err != nil {
		t.Fatal(err)
	}

	orig := defaultSettings()
	orig.Terminal.Theme = "solarized-dark"
	orig.Terminal.FontFamily = "Fira Code"
	orig.Terminal.FontSize = 16
	orig.Terminal.SelectionAction = "copy"
	orig.Terminal.RightClickAction = "paste"
	orig.Terminal.MiddleClickAction = "none"
	orig.Terminal.MaxHistoryLines = 9000
	orig.Terminal.SmartCompletion = boolPtr(false)
	orig.Terminal.HighlightEnabled = boolPtr(false)
	orig.Terminal.CursorBlink = boolPtr(false)
	orig.Terminal.SwallowWheelInAltScreen = boolPtr(false)
	orig.Terminal.SessionLogDir = "/tmp/x"

	if err := s.Save(orig); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}

	o, g := orig.Terminal, got.Terminal
	if o.Theme != g.Theme || o.FontFamily != g.FontFamily || o.FontSize != g.FontSize ||
		o.SelectionAction != g.SelectionAction || o.RightClickAction != g.RightClickAction ||
		o.MiddleClickAction != g.MiddleClickAction || o.MaxHistoryLines != g.MaxHistoryLines ||
		o.SessionLogDir != g.SessionLogDir {
		t.Fatalf("terminal string fields drift: orig=%+v got=%+v", o, g)
	}
	if !equalBoolPtr(o.SmartCompletion, g.SmartCompletion) ||
		!equalBoolPtr(o.HighlightEnabled, g.HighlightEnabled) ||
		!equalBoolPtr(o.CursorBlink, g.CursorBlink) ||
		!equalBoolPtr(o.SwallowWheelInAltScreen, g.SwallowWheelInAltScreen) {
		t.Fatalf("terminal *bool fields drift: orig=%+v got=%+v", o, g)
	}
}

func equalBoolPtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
