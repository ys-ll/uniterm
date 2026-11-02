package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStateRoundtrip(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	if loaded.CollapsedQuickCommandGroupIds == nil {
		t.Fatalf("default LocalState should init CollapsedQuickCommandGroupIds, got nil")
	}
	if len(loaded.CollapsedQuickCommandGroupIds) != 0 {
		t.Fatalf("default CollapsedQuickCommandGroupIds should be empty, got %v", loaded.CollapsedQuickCommandGroupIds)
	}

	loaded.CollapsedQuickCommandGroupIds = []string{"qcg-1", "__ungrouped__"}
	if err := s.Save(loaded); err != nil {
		t.Fatalf("save: %v", err)
	}

	again, err := s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(again.CollapsedQuickCommandGroupIds) != 2 ||
		again.CollapsedQuickCommandGroupIds[0] != "qcg-1" ||
		again.CollapsedQuickCommandGroupIds[1] != "__ungrouped__" {
		t.Fatalf("roundtrip mismatch: got %v", again.CollapsedQuickCommandGroupIds)
	}

	// Defaults must survive: older configs lack the new key entirely.
	if err := os.WriteFile(filepath.Join(s.configDir, localStateFileName), []byte(`{"sidebarVisible":false}`), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	legacy, err := s.Load()
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if legacy.CollapsedQuickCommandGroupIds == nil || len(legacy.CollapsedQuickCommandGroupIds) != 0 {
		t.Fatalf("legacy config should yield empty CollapsedQuickCommandGroupIds, got %v", legacy.CollapsedQuickCommandGroupIds)
	}
}

func TestLocalStateSftpHiddenColumns(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	if loaded.SftpHiddenColumns != nil {
		t.Fatalf("default SftpHiddenColumns should be nil (all visible), got %v", loaded.SftpHiddenColumns)
	}

	loaded.SftpHiddenColumns = []string{"permission", "owner", "group"}
	if err := s.Save(loaded); err != nil {
		t.Fatalf("save: %v", err)
	}

	again, err := s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(again.SftpHiddenColumns) != 3 || again.SftpHiddenColumns[0] != "permission" {
		t.Fatalf("roundtrip mismatch: got %v", again.SftpHiddenColumns)
	}

	// Older configs lack the key entirely; columns stay visible (nil set).
	if err := os.WriteFile(filepath.Join(s.configDir, localStateFileName), []byte(`{"sidebarVisible":true}`), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	legacy, err := s.Load()
	if err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if legacy.SftpHiddenColumns != nil {
		t.Fatalf("legacy config should yield nil SftpHiddenColumns, got %v", legacy.SftpHiddenColumns)
	}
}

func TestLocalStateAIPreferencesRoundtrip(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())
	state, err := s.Load()
	if err != nil || state.AI != nil {
		t.Fatalf("legacy/default state should not invent an AI selection: %v, %v", state.AI, err)
	}
	state.AI = &AILocalState{ActiveModelID: "second", ModelPreferences: map[string]AIReasoningPreference{
		"first":  {ReasoningEffort: "high"},
		"second": {ReasoningEffort: "low"},
	}}
	if err := s.Save(state); err != nil {
		t.Fatal(err)
	}
	restarted := NewLocalStateStore(s.configDir)
	got, err := restarted.Load()
	if err != nil || got.AI == nil || got.AI.ActiveModelID != "second" || got.AI.ModelPreferences["first"].ReasoningEffort != "high" || got.AI.ModelPreferences["second"].ReasoningEffort != "low" {
		t.Fatalf("lost local AI preferences: %+v, %v", got.AI, err)
	}
	got.SidebarVisible = false
	if err := restarted.Save(got); err != nil {
		t.Fatal(err)
	}
	again, _ := restarted.Load()
	if again.AI == nil || len(again.AI.ModelPreferences) != 2 {
		t.Fatal("unrelated local state save discarded AI preferences")
	}
}

func TestSaveGeometryPersistsAllFields(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())

	if err := s.SaveGeometry(120, 80, 1500, 900, true); err != nil {
		t.Fatalf("save geometry: %v", err)
	}
	again, err := s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.WindowX != 120 || again.WindowY != 80 ||
		again.WindowWidth != 1500 || again.WindowHeight != 900 || !again.WindowMaximised {
		t.Fatalf("geometry roundtrip mismatch: x=%d y=%d w=%d h=%d max=%t",
			again.WindowX, again.WindowY, again.WindowWidth, again.WindowHeight, again.WindowMaximised)
	}
}

func TestSaveGeometryKeepsOtherLocalState(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())

	state, err := s.Load()
	if err != nil {
		t.Fatalf("initial load: %v", err)
	}
	state.SidebarWidth = 321
	state.LastTabsSnapshot = json.RawMessage(`{"tabs":[1]}`)
	if err := s.Save(state); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := s.SaveGeometry(10, 20, 1300, 850, false); err != nil {
		t.Fatalf("save geometry: %v", err)
	}
	again, err := s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.SidebarWidth != 321 {
		t.Fatalf("geometry save clobbered sidebar width: got %d", again.SidebarWidth)
	}
	var tabs struct{ Tabs []int }
	if err := json.Unmarshal(again.LastTabsSnapshot, &tabs); err != nil || len(tabs.Tabs) != 1 || tabs.Tabs[0] != 1 {
		t.Fatalf("geometry save clobbered tabs snapshot: got %s (%v)", again.LastTabsSnapshot, err)
	}
	if again.WindowX != 10 || again.WindowWidth != 1300 {
		t.Fatalf("geometry not saved: x=%d w=%d", again.WindowX, again.WindowWidth)
	}
}

func TestSaveGeometryIgnoresTinySize(t *testing.T) {
	s := NewLocalStateStore(t.TempDir())

	if err := s.SaveGeometry(50, 60, 1200, 800, false); err != nil {
		t.Fatalf("save geometry: %v", err)
	}
	// A minimised/tearing-down window reports the off-screen position and the
	// taskbar thumbnail size; persisting it restores an invisible window.
	if err := s.SaveGeometry(-32000, -32000, 160, 28, false); err != nil {
		t.Fatalf("save tiny geometry: %v", err)
	}
	// Degenerate zero-size report must be skipped as well.
	if err := s.SaveGeometry(0, 0, 0, 0, false); err != nil {
		t.Fatalf("save zero geometry: %v", err)
	}

	again, err := s.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if again.WindowX != 50 || again.WindowY != 60 ||
		again.WindowWidth != 1200 || again.WindowHeight != 800 {
		t.Fatalf("tiny geometry was persisted: x=%d y=%d w=%d h=%d",
			again.WindowX, again.WindowY, again.WindowWidth, again.WindowHeight)
	}
}
