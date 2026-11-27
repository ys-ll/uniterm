package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// writeSkill creates a skill directory under configDir/skills/<dir>/SKILL.md
// with the given frontmatter. Returns the directory name used.
func writeSkill(t *testing.T, configDir, name, description, body string) {
	t.Helper()
	dir := filepath.Join(configDir, "skills", name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body + "\n"
	mdPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(mdPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile(%s): %v", mdPath, err)
	}
}

// F-107: verify SkillsStore.List cache actually works. The cache is intended
// to coalesce rapid re-reads (per-token AI / terminal code) by reusing the
// merged result for up to skillsListCacheTTL, but only as long as no mutator
// invalidates it.
func TestSkillsStore_List_CachesAndInvalidates(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	writeSkill(t, dir, "alpha", "first skill", "alpha body")
	writeSkill(t, dir, "beta", "second skill", "beta body")

	// First call: populates the cache.
	first, err := s.List()
	if err != nil {
		t.Fatalf("first List: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(first))
	}

	// Cache populated — should not be nil and should be within TTL.
	s.listMu.Lock()
	populated := s.listCache != nil && time.Since(s.listCachedAt) < skillsListCacheTTL
	s.listMu.Unlock()
	if !populated {
		t.Fatalf("expected cache populated after first List, got cache=%v cachedAt=%v",
			s.listCache == nil, s.listCachedAt)
	}

	// Second call: serves from cache. We verify by adding a new skill
	// to disk AFTER the cache is populated — the cached result must NOT
	// see it (this is the correctness property of the cache).
	writeSkill(t, dir, "gamma", "third skill", "gamma body")
	second, err := s.List()
	if err != nil {
		t.Fatalf("second List: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("expected cached slice to keep 2 skills (gamma not yet visible), got %d", len(second))
	}

	// Mutator invalidates the cache; the next List picks up the new skill.
	s.invalidateListCache()
	third, err := s.List()
	if err != nil {
		t.Fatalf("third List: %v", err)
	}
	if len(third) != 3 {
		t.Fatalf("expected 3 skills after cache invalidation, got %d", len(third))
	}
}

// F-107: SetEnabled (and the other setPref mutators) must invalidate the
// cache so the next List reflects the change. Without invalidation the UI
// would show stale enabled state until the 2s TTL expired.
func TestSkillsStore_List_MutatorsInvalidate(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	writeSkill(t, dir, "delta", "fourth skill", "delta body")

	if _, err := s.List(); err != nil {
		t.Fatalf("seed List: %v", err)
	}
	s.listMu.Lock()
	cacheBefore := s.listCache != nil
	s.listMu.Unlock()
	if !cacheBefore {
		t.Fatalf("cache not populated before mutate")
	}

	if err := s.SetEnabled("delta", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	s.listMu.Lock()
	cacheAfter := s.listCache != nil
	s.listMu.Unlock()
	if cacheAfter {
		t.Fatalf("cache should be invalidated after SetEnabled")
	}

	out, err := s.List()
	if err != nil {
		t.Fatalf("post-mutate List: %v", err)
	}
	if len(out) != 1 || out[0].Enabled {
		t.Fatalf("expected disabled delta, got %+v", out)
	}
}

// F-107: repeated concurrent List calls must not corrupt the cache or
// return inconsistent results. The cache mutex serialises cache reads and
// store writes; List should be safe to call from many goroutines.
func TestSkillsStore_List_ConcurrentReads(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	for i := 0; i < 5; i++ {
		writeSkill(t, dir, "skill"+string(rune('a'+i)), "desc", "body")
	}

	var calls atomic.Int64
	const goroutines = 16
	done := make(chan struct{}, goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 8; j++ {
				out, err := s.List()
				if err != nil {
					t.Errorf("List: %v", err)
					return
				}
				if len(out) != 5 {
					t.Errorf("expected 5 skills, got %d", len(out))
					return
				}
				calls.Add(1)
			}
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	if got := calls.Load(); got != goroutines*8 {
		t.Fatalf("expected %d List calls, got %d", goroutines*8, got)
	}
}

// ---- 同步契约（SyncableStore）：skills.json blob = 偏好 + 目录内容捆绑包 ----

// ExportPlain 必须携带手拷进目录、尚无偏好条目的 skill（issue #1180 的核心
// 场景），否则该 skill 永远上不了远端。
func TestSkillsStore_ExportPlain_IncludesDirOnlySkills(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	writeSkill(t, dir, "alpha", "first", "alpha body")
	// 手拷进目录、从未经过 Create/Import —— 没有 prefs 记录。
	writeSkill(t, dir, "manual-copy", "copied in by hand", "body")

	plain, err := s.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	var bundle skillSyncBundle
	if err := json.Unmarshal(plain, &bundle); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}
	names := map[string]bool{}
	for _, sk := range bundle.Skills {
		names[sk.Name] = true
	}
	for _, name := range []string{"alpha", "manual-copy"} {
		if !names[name] {
			t.Fatalf("export missing skill %q: %s", name, plain)
		}
	}
}

// 空仓库（无 skill、无偏好文件）导出 fs.ErrNotExist，同步层据此跳过。
func TestSkillsStore_ExportPlain_NotExist(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)
	_, err := s.ExportPlain()
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

// 捆绑包内容校验：references/scripts 子文件、可执行位、正文都在。
func TestSkillsStore_ExportPlain_CarriesNestedFiles(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)
	writeSkill(t, dir, "alpha", "first", "body")
	refDir := filepath.Join(dir, "skills", "alpha", "references")
	if err := os.MkdirAll(refDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(refDir, "notes.md"), []byte("ref"), 0644); err != nil {
		t.Fatal(err)
	}
	scriptDir := filepath.Join(dir, "skills", "alpha", "scripts")
	if err := os.MkdirAll(scriptDir, 0755); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(scriptDir, "run.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	plain, err := s.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	var bundle skillSyncBundle
	if err := json.Unmarshal(plain, &bundle); err != nil {
		t.Fatal(err)
	}
	if len(bundle.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(bundle.Skills))
	}
	byPath := map[string]skillSyncFile{}
	for _, f := range bundle.Skills[0].Files {
		byPath[f.Path] = f
	}
	ref, ok := byPath["references/notes.md"]
	if !ok || string(ref.Content) != "ref" {
		t.Fatalf("references/notes.md not carried: %+v", byPath)
	}
	script, ok := byPath["scripts/run.sh"]
	if !ok {
		t.Fatalf("scripts/run.sh not carried: %+v", byPath)
	}
	// Windows filesystems carry no exec bit, so the exported mode is 666 there.
	if runtime.GOOS != "windows" && script.Mode&0100 == 0 {
		t.Fatalf("scripts/run.sh missing exec bit: mode %o", script.Mode)
	}
	if md, ok := byPath["SKILL.md"]; !ok || !strings.Contains(string(md.Content), "name: alpha") {
		t.Fatalf("SKILL.md not carried: %+v", byPath)
	}
}

// ImportPlain 后：远端 skill 目录整体落盘、偏好生效；本地独有 skill 保留
// 并由扫描补齐默认偏好；远端删除的 skill 不清本地目录（删除不传播）。
func TestSkillsStore_ImportPlain_Reconciles(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	writeSkill(t, dir, "alpha", "first", "alpha body")
	writeSkill(t, dir, "local-only", "never synced", "body")
	if _, err := s.List(); err != nil {
		t.Fatalf("seed List: %v", err)
	}
	// 关闭 alpha，模拟本地偏好与远端不同的场景。
	if err := s.SetEnabled("alpha", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}

	incoming := skillSyncBundle{Version: 1,
		Prefs: []skillPref{
			{Name: "alpha", Origin: "created", Locked: false, Enabled: true, SortOrder: 0},
			{Name: "new-skill", Origin: "imported", Locked: true, Enabled: true, SortOrder: 1},
		},
		Skills: []skillSyncSkill{
			{Name: "alpha", Files: []skillSyncFile{
				{Path: "SKILL.md", Content: []byte("---\nname: alpha\ndescription: first\n---\n\nalpha body v2\n"), Mode: 0644},
			}},
			{Name: "new-skill", Files: []skillSyncFile{
				{Path: "SKILL.md", Content: []byte("---\nname: new-skill\ndescription: from remote\n---\n\nremote body\n"), Mode: 0644},
			}},
		},
	}
	raw, err := json.Marshal(incoming)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ImportPlain(raw); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}

	out, err := s.List()
	if err != nil {
		t.Fatalf("post-import List: %v", err)
	}
	byName := map[string]SkillMeta{}
	for _, m := range out {
		byName[m.Name] = m
	}
	// 远端启用了 alpha，且内容是远端版本。
	if m, ok := byName["alpha"]; !ok || !m.Enabled {
		t.Fatalf("alpha should be enabled per remote prefs, got %+v", m)
	}
	body, err := s.GetBody("alpha")
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	if !strings.Contains(body, "alpha body v2") {
		t.Fatalf("alpha body should be the remote copy, got %q", body)
	}
	// 远端新 skill：目录已落盘。
	if m, ok := byName["new-skill"]; !ok || !m.Enabled {
		t.Fatalf("new-skill should be imported enabled, got %+v", m)
	}
	// 本地独有 skill：目录保留，偏好由扫描补齐（enabled）。
	if m, ok := byName["local-only"]; !ok || !m.Enabled {
		t.Fatalf("local-only should survive with default prefs, got %+v", m)
	}
}

// ImportPlain 拒绝不可解析的 blob —— 损坏数据绝不覆盖本地偏好。
func TestSkillsStore_ImportPlain_RejectsCorrupt(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	writeSkill(t, dir, "alpha", "first", "body")
	if _, err := s.List(); err != nil {
		t.Fatalf("seed List: %v", err)
	}
	if err := s.ImportPlain([]byte("{not json")); err == nil {
		t.Fatal("expected error for corrupt blob")
	}
	out, err := s.List()
	if err != nil {
		t.Fatalf("List after corrupt import: %v", err)
	}
	if len(out) != 1 || !out[0].Enabled {
		t.Fatalf("local prefs must survive corrupt import, got %+v", out)
	}
}

// ImportPlain 拒绝带路径穿越的捆绑包（".." / 绝对路径）。
func TestSkillsStore_ImportPlain_RejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)

	incoming := skillSyncBundle{Version: 1, Skills: []skillSyncSkill{
		{Name: "evil", Files: []skillSyncFile{
			{Path: "../../escape.txt", Content: []byte("x"), Mode: 0644},
		}},
	}}
	raw, _ := json.Marshal(incoming)
	if err := s.ImportPlain(raw); err == nil {
		t.Fatal("expected error for traversal path")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape.txt")); !os.IsNotExist(err) {
		t.Fatal("file must not escape the skills root")
	}
}

// A 导出 → B 导入：B 的 skill 集与偏好与 A 一致（round-trip 稳定）。
func TestSkillsStore_SyncRoundTripIsStable(t *testing.T) {
	aDir := t.TempDir()
	a := NewSkillsStore(aDir)
	writeSkill(t, aDir, "alpha", "first", "body")
	writeSkill(t, aDir, "beta", "second", "body")
	if _, err := a.List(); err != nil {
		t.Fatalf("seed List: %v", err)
	}
	if err := a.SetEnabled("beta", false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}

	bDir := t.TempDir()
	b := NewSkillsStore(bDir)
	if err := b.ImportPlain(plain); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}

	out, err := b.List()
	if err != nil {
		t.Fatalf("B List: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 skills on B, got %d", len(out))
	}
	byName := map[string]SkillMeta{}
	for _, m := range out {
		byName[m.Name] = m
	}
	if byName["beta"].Enabled {
		t.Fatalf("beta should be disabled per A's prefs, got %+v", byName["beta"])
	}
	if !byName["alpha"].Enabled {
		t.Fatalf("alpha should be enabled, got %+v", byName["alpha"])
	}
}

// 带 UTF-8 BOM 的 SKILL.md（Windows 记事本产物）必须能被识别，否则拷进
// 目录的 skill 被静默跳过（issue #1180）。
func TestSkillsStore_List_StripsBOM(t *testing.T) {
	dir := t.TempDir()
	s := NewSkillsStore(dir)
	skillDir := filepath.Join(dir, "skills", "bom-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := []byte("\uFEFF---\nname: bom-skill\ndescription: written on windows\n---\n\nbody")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), content, 0644); err != nil {
		t.Fatal(err)
	}
	out, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(out) != 1 || out[0].Name != "bom-skill" {
		t.Fatalf("BOM skill not recognized: %+v", out)
	}
}
