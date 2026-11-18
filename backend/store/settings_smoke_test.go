package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Smoke test for F-108 + F-109: Save/Load round-trip via the public API.
func TestSettingsStore_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Bypass NewSettingsStore (which uses os.UserConfigDir) — synthesize a
	// store pointing at our temp dir.
	s := &SettingsStore{configDir: dir}
	s.SetPasswordStore(nil)

	want := defaultSettings()
	want.Language = "zh-CN"
	want.Terminal.FontSize = 18
	want.Terminal.ZmodemDownloadDir = filepath.Join(dir, "downloads")
	want.Terminal.ZmodemIdleTimeoutSec = 60

	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "settings.json")); err != nil {
		t.Fatalf("expected settings.json to exist: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Language != want.Language {
		t.Errorf("Language: got %q want %q", got.Language, want.Language)
	}
	if got.Terminal.FontSize != want.Terminal.FontSize {
		t.Errorf("Terminal.FontSize: got %d want %d", got.Terminal.FontSize, want.Terminal.FontSize)
	}
	if got.Terminal.ZmodemDownloadDir != want.Terminal.ZmodemDownloadDir {
		t.Errorf("Terminal.ZmodemDownloadDir: got %q want %q", got.Terminal.ZmodemDownloadDir, want.Terminal.ZmodemDownloadDir)
	}
	if got.Terminal.ZmodemIdleTimeoutSec != want.Terminal.ZmodemIdleTimeoutSec {
		t.Errorf("Terminal.ZmodemIdleTimeoutSec: got %d want %d", got.Terminal.ZmodemIdleTimeoutSec, want.Terminal.ZmodemIdleTimeoutSec)
	}
	if got.Theme != want.Theme {
		t.Errorf("Theme: got %q want %q", got.Theme, want.Theme)
	}
}

// Regression: ShowTabShortcutHints / HostListMenuStyle / NewTabAction were
// missing from the AppSettings struct, so the JSON round-trip silently
// dropped them and the UI toggles reset to defaults on every restart.
func TestSettingsStore_PreservesTabAppearanceSettings(t *testing.T) {
	dir := t.TempDir()
	s := &SettingsStore{configDir: dir}

	settings := defaultSettings()
	settings.ShowTabShortcutHints = boolPtr(false)
	settings.HostListMenuStyle = "rightclick"
	settings.NewTabAction = "localTerminal"
	if err := s.Save(settings); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.ShowTabShortcutHints == nil || *got.ShowTabShortcutHints {
		t.Errorf("ShowTabShortcutHints: got %v, want preserved false", got.ShowTabShortcutHints)
	}
	if got.HostListMenuStyle != "rightclick" {
		t.Errorf("HostListMenuStyle: got %q, want %q", got.HostListMenuStyle, "rightclick")
	}
	if got.NewTabAction != "localTerminal" {
		t.Errorf("NewTabAction: got %q, want %q", got.NewTabAction, "localTerminal")
	}
}

func TestSettingsStore_LoadMissingFile(t *testing.T) {
	dir := t.TempDir()
	s := &SettingsStore{configDir: dir}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("expected no error on missing file, got %v", err)
	}
	if got.Theme == "" {
		t.Errorf("expected defaults to populate, got zero-value settings")
	}
	if got.AutoCheckUpdate == nil || !*got.AutoCheckUpdate {
		t.Errorf("AutoCheckUpdate: got %v, want true by default", got.AutoCheckUpdate)
	}
}

func TestSettingsStore_PreservesExplicitAutoCheckUpdate(t *testing.T) {
	for _, want := range []bool{false, true} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			dir := t.TempDir()
			s := &SettingsStore{configDir: dir}

			settings := defaultSettings()
			settings.AutoCheckUpdate = boolPtr(want)
			if err := s.Save(settings); err != nil {
				t.Fatalf("Save: %v", err)
			}

			got, err := s.Load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got.AutoCheckUpdate == nil || *got.AutoCheckUpdate != want {
				t.Errorf("AutoCheckUpdate: got %v, want preserved %v", got.AutoCheckUpdate, want)
			}
		})
	}
}

func TestSettingsStore_DefaultsMissingAutoCheckUpdateToTrue(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark"}`), 0600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	s := &SettingsStore{configDir: dir}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AutoCheckUpdate == nil || !*got.AutoCheckUpdate {
		t.Errorf("AutoCheckUpdate: got %v, want true for a missing field", got.AutoCheckUpdate)
	}
}

func TestSettingsStore_DefaultsMissingAIMaxTurns(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"theme":"dark","ai":{}}`), 0600); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	s := &SettingsStore{configDir: dir}

	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.AI.MaxTurns == nil || *got.AI.MaxTurns != defaultMaxTurns {
		t.Errorf("AI.MaxTurns: got %v, want %d for a missing field", got.AI.MaxTurns, defaultMaxTurns)
	}
}

func TestSettingsStore_LoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	s := &SettingsStore{configDir: dir}
	got, err := s.Load()
	if err != nil {
		t.Fatalf("Load should swallow corrupt-file error: %v", err)
	}
	if got.Theme == "" {
		t.Errorf("expected defaults, got zero-value")
	}

	// Quarantine sidecar must exist.
	entries, _ := os.ReadDir(dir)
	foundQuarantine := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "settings.json.corrupt-") {
			foundQuarantine = true
			break
		}
	}
	if !foundQuarantine {
		t.Errorf("expected quarantine file, dir=%v", entries)
	}
}

// F-109 regression: Load must release the store mutex before doing disk I/O
// so that a concurrent Save doesn't block on a slow os.ReadFile.
func TestSettingsStore_ConcurrentSaveLoad(t *testing.T) {
	dir := t.TempDir()
	s := &SettingsStore{configDir: dir}

	// Seed a valid file so Load has something to read.
	if err := s.Save(defaultSettings()); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	const writers = 4
	const readers = 8
	const iters = 20

	done := make(chan struct{}, writers+readers)
	for i := 0; i < writers; i++ {
		i := i
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iters; j++ {
				settings := defaultSettings()
				settings.Terminal.FontSize = 10 + i*2 + j
				if err := s.Save(settings); err != nil {
					t.Errorf("writer %d Save: %v", i, err)
					return
				}
			}
		}()
	}
	for i := 0; i < readers; i++ {
		i := i
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < iters; j++ {
				if _, err := s.Load(); err != nil {
					t.Errorf("reader %d Load: %v", i, err)
					return
				}
			}
		}()
	}
	for i := 0; i < writers+readers; i++ {
		<-done
	}
}

func mustSettingsStore(t *testing.T) *SettingsStore {
	t.Helper()
	s, err := NewSettingsStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewSettingsStore: %v", err)
	}
	s.SetPasswordStore(fakeCipherStore{})
	return s
}

// TestSettingsStoreSyncRoundTrip pins the sync exchange contract for the
// opt-in settings.json: the blob must carry plaintext apiKeys and survive a
// round-trip through a store on another directory.
func TestSettingsStoreSyncRoundTrip(t *testing.T) {
	a := mustSettingsStore(t)
	seed := defaultSettings()
	seed.Theme = "light"
	seed.Language = "zh-CN"
	seed.AI.Models = []AIModelConfig{{ID: "m1", Name: "model", APIKey: "sk-set"}}
	if err := a.Save(seed); err != nil {
		t.Fatalf("Save: %v", err)
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	if !contains(string(plain), "sk-set") || contains(string(plain), "enc:v1:") {
		t.Fatalf("blob must carry plaintext secrets: %s", plain)
	}
	b := mustSettingsStore(t)
	if err := b.ImportPlain(plain); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}
	got, err := b.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Theme != "light" || got.Language != "zh-CN" {
		t.Fatalf("settings lost: %+v", got)
	}
	if len(got.AI.Models) != 1 || got.AI.Models[0].APIKey != "sk-set" {
		t.Fatalf("settings ai models lost: %+v", got.AI.Models)
	}
}
