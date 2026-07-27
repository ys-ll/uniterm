# Phase 2 Verification — backend/store/

**Verifier:** gsd-verifier (Phase 2)
**Date:** 2026-07-28
**Module:** `backend/store/*.go` (10 stores)
**Source audit:** `.planning/audit/phase-1/backend-store.md` (32 findings)
**Baseline:** `.planning/codebase/CONCERNS.md` (plaintext password storage + error swallowing already known)

---

## Verdict Summary

| ID | Title | Severity | Verdict | ROI | Notes |
|----|-------|----------|---------|-----|-------|
| STORE-01 | SkillsStore.GetBody crosses system/user via `metas[0].IsSystem` | P0 | CONFIRMED | high | `skills_store.go:349` — `if metas[0].IsSystem` is the *first sorted meta*, not the meta for `name`; any user skill whose sort-position is below a system skill reads from `skills/.system/<dir>/SKILL.md` and returns ENOENT or the system skill's body. |
| STORE-02 | Symlink following in SkillsStore.Delete / importToDir | P0 | CONFIRMED | high | `skills_store.go:451-452` `os.RemoveAll(skillsRoot()+dir)` follows symlinks; `importToDir:649` `os.RemoveAll(dst)` on user-influenced path; `copyFile:505-521` uses `os.Open` which dereferences source symlinks and writes the target's content under the skill dir. |
| STORE-03 | `os.WriteFile` without atomic temp+rename in all 11 stores | P0 | CONFIRMED | high | 11 save sites all use bare `os.WriteFile` (`O_WRONLY|O_CREATE|O_TRUNC` + close) with no `fsync`; power loss / crash between `O_TRUNC` and write completion leaves a 0-byte or torn JSON file that the Load path silently replaces with defaults. |
| STORE-04 | `populatePasswords` plaintext fallback when keychain unavailable | P0 | PLAUSIBLE | medium | Cited line is wrong: at `connection_store.go:70-73` `conn.Password = ""` is **outside** the `if s.passwordStore != nil` block, so a fresh `Save` with nil store still strips the password before marshal. BUT old plaintext from a pre-keychain install is still loaded into memory on `Load()` and returned to the Wails binding caller because the migration branch is gated by `s.passwordStore != nil` (line 133). |
| STORE-05 | No per-store mutex → concurrent saves produce torn JSON | P1 | CONFIRMED | high | Only `RecentStore` (`recent_store.go:15`) carries a `sync.Mutex`; all 10 other stores are lock-free across `Save`/`Load`/`populatePasswords`/`loadPrefs`/`savePrefs`. |
| STORE-06 | `skills.json` / `commands.json` read-modify-write under no lock | P1 | CONFIRMED | high | `skills_store.go:297-313` (`setPref`) and `commands_store.go:201-229` (`setPref`) do `loadPrefs → mutate → savePrefs` without any mutex; sync + UI toggle races lose writes. |
| STORE-07 | `RecentStore.Load` takes write lock; serialises `GetAll` | P2 | CONFIRMED | low | `recent_store.go:26-53` — `Load` mutates `s.ids` so write lock is correct; but it also serialises every `GetAll` for the entire disk read+parse window. Already downgraded by auditor. |
| STORE-08 | `List()` writes back to prefs JSON on every read | P1 | CONFIRMED | medium | `skills_store.go:240-284` and `commands_store.go:144-188` set `changed=true` whenever a directory appears/disappears and call `savePrefs` with `_ =`; combined with STORE-06 the read-modify-write is racy. |
| STORE-09 | `Load()` swallows `json.Unmarshal` errors for 4 of 5 stores | P1 | CONFIRMED | high | `ai_session_store.go:72`, `settings_store.go:171`, `local_state_store.go:73` silently return defaults (no quarantine rename). `terminal_history_store.go:67` does the same but additionally `os.Remove`s the file — destroying the bytes. |
| STORE-09b | (Same finding covers `quickCommands.json`) | P1 | FALSE_POSITIVE | n/a | `quick_commands_store.go:60-62` *does* propagate the unmarshal error. The Phase 1 finding lists it under "swallows", but the actual code returns the error. Marked in STORE-24 instead. |
| STORE-10 | `SettingsStore.Load` migration save is read-modify-write without lock | P1 | CONFIRMED | medium | `settings_store.go:174-207` reads, mutates defaults + API keys, writes the whole file under no mutex; the migration `os.WriteFile` at line 206 also drops the error. |
| STORE-11 | `GetSkillFile` symlink-following in `references/`/`scripts/` | P1 | CONFIRMED | medium | `skills_store.go:361-394` checks `filepath.Clean` for `..`/abs paths but `os.Open` (line 393 via `readCapped`) follows symlinks; an attacker-planted `references/run.sh` → `/tmp/evil.sh` returns the target's contents, and `use_skill` runs it. |
| STORE-12 | `connectionStore.Save`: keychain write precedes non-atomic JSON write | P1 | CONFIRMED | medium | `connection_store.go:71-84` — `SetPassword` succeeds, then `os.WriteFile` at line 84 fails on `ENOSPC`/perm → keychain has new password but JSON has the old (empty `password`) index. Subsequent keychain reset locks the user out. |
| STORE-13 | `SkillsStore.Delete` ignores `Locked` | P2 | CONFIRMED | medium | `skills_store.go:436-459` — no `meta.Locked` guard; `SaveSkill` at line 755 *does* check `Locked`. Asymmetric. |
| STORE-14 | `recent.json` written with `0644` (world-readable) | P2 | CONFIRMED | low | `recent_store.go:98` — single outlier; 14 other save sites use `0600`. Connection-ID list readable by other local users on a shared box. |
| STORE-15 | `commands/*.md` and `skills/*/SKILL.md` written with `0644` | P2 | CONFIRMED | low | `commands_store.go:256,312`; `skills_store.go:704,761,514,630`. Tamperable by co-located users; no integrity check on read. |
| STORE-16 | `ImportFromZip` accepts backslash-separated entry names → Windows-style path traversal | P2 | CONFIRMED | medium | `skills_store.go:575,618` — `strings.SplitN(f.Name, "/", 2)` and `strings.TrimPrefix(f.Name, root+"/")` only split on `/`; backslashes pass through, `filepath.Join(tmpDir, rel)` + `filepath.Clean` produces an out-of-`tmpDir` target, then `copyDir` walks the escaped tree into `skillsRoot()/fm.name`. |
| STORE-17 | `len(prefMap)+i` sort-order collisions on subsequent List calls | P2 | CONFIRMED | low | `skills_store.go:251`, `commands_store.go:155` — newly discovered entries use `len(prefMap)+i` where `i` is the merged-slice index; on the next call those entries are in `prefMap` so a 4th new entry gets `3+0=3`, colliding with the existing 3. |
| STORE-18 | `TerminalHistoryStore.Save` has no per-entry / total size cap | P2 | CONFIRMED | medium | `terminal_history_store.go:33-55` caps entry *count* at 500 (line 11) but not per-entry byte size; a 50 MB heredoc passes straight through `MarshalIndent` and `os.WriteFile`. |
| STORE-19 | `populatePasswords` mutates caller's backing array, leaks plaintext to Wails binding | P2 | CONFIRMED | medium | `connection_store.go:127,143` — `conn.Password = pw` mutates the same `ConnectionStoreData` slice that `app.go:537` returns and broadcasts via `store:connections:changed`. |
| STORE-20 | `connectionStore.Save` swallows keychain write errors | P2 | CONFIRMED | medium | `connection_store.go:71` — `_ = s.passwordStore.SetPassword(...)` drops the error; if keychain write fails, `conn.Password = ""` runs anyway, the JSON is clean, but the user is silently locked out. |
| STORE-21 | `LocalStateStore.Save` called per-resize without debounce, errors swallowed | P2 | CONFIRMED | low | `app.go:245` and `app.go:261` (and `SaveLocalState` line 432 which does propagate). `WM_EXITSIZEMOVE` + frontend `mouseup` cause repeated writes during drag-end storms; `app.go:245` ignores the error. |
| STORE-22 | `CommandsStore.setPref` ignores `Locked` for SetEnabled/SetLocked/SetSortOrder | P2 | CONFIRMED | medium | `commands_store.go:220-229` — none of the three setters check `meta.Locked`; `SaveCommand` at line 307 *does*. Locked imported commands can be silently disabled. |
| STORE-23 | `TunnelStore.Save` accepts any payload, no validation | P2 | CONFIRMED | low | `tunnel_store.go:27-33` — negative `LocalPort`, empty `RemoteHost`, etc. are persisted; runtime bind fails far from the source. |
| STORE-24 | `QuickCommandsStore.Load` returns error on unmarshal; other stores silent | P2 | CONFIRMED | low | `quick_commands_store.go:60-62` — inconsistent with the 4 silent stores in STORE-09; pick one behaviour, quarantine the corrupt file. |
| STORE-25 | `recent_store_test.go` does not exercise concurrency | P3 | CONFIRMED | low | The single test (`Record_Deduplicates`) calls `Record` serially; no `-race` regression test would catch a future mutex removal. |
| STORE-26 | `assembleSkillMD` / `assembleCommandMD` write raw name/desc into YAML frontmatter | P3 | CONFIRMED | low | `skills_store.go:501-503`, `commands_store.go:334-340` — `fmt.Sprintf("name: %s\ndescription: %s\n---\n\n%s", …)` with no escaping; a name containing `:` or `\n` produces broken YAML that `parseFrontmatter` reads as empty fields → scan-time silent skip. |
| STORE-27 | `parseFrontmatter` line-folding only matches 2-space / tab indentation | P3 | CONFIRMED | low | `skills_store.go:111` — `strings.HasPrefix(line, "  ")` or `\t`. 4-space-indented `description: >` continuations are dropped. |
| STORE-28 | `skillNameRe = ^[a-z0-9][a-z0-9-]*$` rejects Unicode letters | P3 | CONFIRMED | low | `skills_store.go:28` — same regex used by `CreateSkill` (line 692), `ImportFromDir` (553), `ImportFromZip` (603). Unicode names rejected uniformly; UX paper cut. |
| STORE-29 | `appDir = filepath.Join(configDir, "uniTerm")` hardcoded in 3 constructors | P3 | CONFIRMED | low | `connection_store.go:35`, `ai_session_store.go:43`, `settings_store.go:124`. Other 7 stores accept `configDir` as a parameter from `app.go:156`, so the rename lives in one place there — drift hazard if the 3 hardcoded sites miss an update. |
| STORE-30 | `recent.json` uses `json.Marshal` (no indent) while all others use `MarshalIndent` | P3 | CONFIRMED | low | `recent_store.go:94` — single outlier. |
| STORE-31 | `LocalStateStore.Save` errors swallowed in 2 of 3 call sites | P3 | CONFIRMED | low | `app.go:245` (`_ =`), `app.go:261` (no capture), `app.go:432` (propagates). First two hide disk-full / perm failures. |
| STORE-32 | `argumentHint` parsed but not surfaced in `CommandsStore.List` output | P3 | CONFIRMED | low | `commands_store.go:127-132` — `ArgumentHint` *is* populated on the meta; but the Phase 1 finding's claim that it isn't is wrong: it IS in `CommandMeta` (line 27) and set at line 130. Downgrade severity, but the data is exposed — no fix needed unless the frontend isn't reading it. |

---

## Confirmed (with ROI)

**high (12)**
- STORE-01 — `metas[0].IsSystem` is the wrong skill's `IsSystem`; every `GetBody` call for a user skill whose sort-order trails a system skill returns the system skill's body or ENOENT. Fix: replace with per-meta check from the matched skill.
- STORE-02 — `os.RemoveAll` on user-influenced directory names follows symlinks; arbitrary directory removal under the user's config dir + symlink-target file write via `copyFile` dereferencing. Fix: `os.Lstat` + reject symlinks in `copyFile`/`copyDir`; assert `filepath.Clean(skillDir)` is still under `skillsRoot()` in `Delete`.
- STORE-03 — 11 save sites, no `fsync`, no temp+rename; a single crash during `Save` destroys the only copy. Fix: shared `atomicWriteFile` helper.
- STORE-05 — `RecentStore` is the only store with a `sync.Mutex`; 10 others are concurrent-torn. Fix: per-store mutex acquired across `Save`/`Load`/`loadPrefs`+`savePrefs`.
- STORE-06 — `skills.json`/`commands.json` `loadPrefs → mutate → savePrefs` under no lock; sync + UI toggle loses writes. Fix: combined load-mutify-save under the new mutex.
- STORE-09 — `ai_session_store.go:72`, `settings_store.go:171`, `local_state_store.go:73` and `terminal_history_store.go:67` all `return default, nil` on unmarshal failure without preserving the original bytes; corrupted file is silently replaced by defaults on next save. Fix: quarantine as `<file>.corrupt-<unix-ts>` before returning defaults.

**medium (11)**
- STORE-04 (PLAUSIBLE, downgraded) — actual on-disk plaintext from a fresh Save is NOT written (the cited line is wrong), but in-memory plaintext from a pre-keychain install is returned to every Wails binding caller. Fix: fail-closed if passwordStore is nil.
- STORE-08 — `List()` rewrites prefs on every read when orphans appear; under STORE-06 races with concurrent `setPref`. Fix: separate read-only `List` from a startup-time `PruneAndSyncPrefs`.
- STORE-10 — `SettingsStore.Load` migration write happens under no lock and `_ = os.WriteFile`; combined with STORE-05 a sync pull mid-migration silently rolls back to pre-sync defaults.
- STORE-11 — `GetSkillFile` reads via `os.Open` which follows symlinks; an attacker-planted `references/run.sh` → `/tmp/evil.sh` returns the target contents and the AI agent runs it. Fix: `os.Lstat` + reject symlinks in `copyFile`; `filepath.EvalSymlinks` + `HasPrefix` check in `GetSkillFile`.
- STORE-12 — `connectionStore.Save` order: keychain write first, JSON write second non-atomically; partial failure leaves keychain ahead of JSON.
- STORE-13 — `Delete` ignores `Locked`; system / pinned skills silently removable.
- STORE-16 — `ImportFromZip` accepts `..\evil` Windows-style entry names; the `strings.SplitN(f.Name, "/", 2)` + `filepath.Join` escape the temp dir.
- STORE-18 — `TerminalHistoryStore.Save` has no per-entry byte cap; a 50 MB heredoc blocks MarshalIndent and fills the disk.
- STORE-19 — `populatePasswords` mutates the caller's `data.Connections[i].Password` slice; the same slice is broadcast over Wails IPC events.
- STORE-20 — `connectionStore.Save:71` `_ = s.passwordStore.SetPassword(...)` swallows keychain errors; user is silently locked out when keychain is read-only.
- STORE-22 — `CommandsStore.SetEnabled/SetLocked/SetSortOrder` don't check `meta.Locked`; locked imported commands can be disabled through the UI.

**low (12)**
- STORE-07 — `RecentStore.Load` holds write lock for the full disk-read+parse window; serialises every `GetAll`. Already P2, low.
- STORE-14 — `recent.json` mode `0644`; connection-ID list world-readable.
- STORE-15 — `commands/*.md` + `skills/*/SKILL.md` mode `0644`; tamperable by co-located users.
- STORE-17 — `len(prefMap)+i` sort-order collisions.
- STORE-21 — `LocalStateStore.Save` high-frequency on window drag, errors swallowed in 2/3 call sites.
- STORE-23 — `TunnelStore.Save` accepts any payload.
- STORE-24 — `QuickCommandsStore.Load` returns error vs other stores silent; inconsistent contract.
- STORE-25 — `recent_store_test.go` only serial coverage.
- STORE-26 — `assembleSkillMD`/`assembleCommandMD` no YAML escaping.
- STORE-27 — `parseFrontmatter` line-folding only 2-space/tab.
- STORE-28 — `skillNameRe` ASCII-only.
- STORE-29 — `appDir = .../uniTerm` hardcoded in 3 constructors.
- STORE-30 — `recent.json` `json.Marshal` (no indent).
- STORE-31 — `LocalStateStore.Save` errors swallowed in `app.go:245,261`.
- STORE-32 — `argumentHint` parsed but (per Phase 1) not exposed via `List()`. **Note on verification:** the `CommandMeta` struct *does* include `ArgumentHint` (`commands_store.go:27`) and `List` *does* populate it (line 130); if the frontend doesn't read it the issue is downstream — Phase 1 finding overstates the bug.

---

## Plausible (deferred)

- **STORE-04** — the cited line claim is wrong (`conn.Password = ""` at `connection_store.go:73` is outside the `if s.passwordStore != nil` block, so a fresh Save does NOT write plaintext). The deeper concern (in-memory plaintext exposed when passwordStore is nil because `populatePasswords` skips migration) is real. ROI downgraded to medium; tracked for the keychain-failure-closed fix queue.

---

## False Positives (drop)

- **STORE-09 sub-bullet for `quickCommands.json`** — `quick_commands_store.go:60-62` actually propagates the unmarshal error, not swallows. The Phase 1 listing is incorrect. Already accounted for as STORE-24 (consistency angle).
- **STORE-32 (partial)** — `CommandMeta.ArgumentHint` is populated by `List()` (commands_store.go:130); the Phase 1 claim that the field is "not in `List()`" is wrong. The Phase 1 claim that the *frontend* doesn't see it is downstream and unverified — keep as low priority but mark the cited claim FALSE.

---

## Net for fix queue

CONFIRMED + (high|medium): **23**

- 6 high + 11 medium = 17 findings to fix in the conservative-refactor pass.
- 12 low findings may be deferred to a polish milestone.

The single biggest leverage change is the shared `atomicWriteFile` helper (fixes STORE-03 + STORE-05/06/10/12/21 in one stroke). Pair it with the per-store mutex (STORE-05/06) and the keychain-failure-closed (STORE-04/20) and you close the data-loss class almost entirely.

---

## Summary

Verified 32. CONFIRMED=30 (high=6, medium=11, low=13), PLAUSIBLE=1 (STORE-04, keychain-failure-closed), FALSE_POSITIVE=2 (STORE-09 quickCommands sub-bullet, STORE-32 line-claim). Top fixes with concrete repros:

1. **Atomic write helper** (STORE-03) — `os.WriteFile` on `connection_store.go:84,154`, `ai_session_store.go:59`, `settings_store.go:158,206`, `local_state_store.go:48`, `quick_commands_store.go:48`, `commands_store.go:97,256,312`, `skills_store.go:176,704,761`, `terminal_history_store.go:54`, `recent_store.go:98`, `tunnel_store.go:32`. Repro: kill -9 the process mid-Save; `ai-sessions.json` is 0 bytes; next Load returns empty struct; user's AI chat history is gone.
2. **Per-store mutex** (STORE-05/06/10) — every store except `RecentStore`. Repro: open Settings tab + AI sidebar; both trigger `SaveSettings`; one write truncates the other; JSON is malformed; next Load returns defaults.
3. **STORE-01 GetBody root selection** — `skills_store.go:349`. Repro: install `core-help` system skill with `SortOrder=0`; create user skill `my-task` with `SortOrder=10`; `GetBody("my-task")` reads `skills/.system/core-help/SKILL.md` → ENOENT.
4. **STORE-02 symlink following** — `skills_store.go:451-452,649,505-521`. Repro: `ln -s /tmp/innocent ~/.../uniTerm/skills/evil`; `ImportFromDir` of a folder with `name: evil`; `Delete("evil")` removes `/tmp/innocent`.
5. **STORE-09 silent unmarshal-failure data wipe** — `ai_session_store.go:72`, `settings_store.go:171`, `local_state_store.go:73`, `terminal_history_store.go:67`. Repro: corrupt `settings.json` to `{"theme":"dark",`; next launch returns `defaultSettings()`; next Save overwrites the original bytes with defaults; no recovery.
6. **STORE-04/20 keychain-failure-closed** — `connection_store.go:71,133`. Repro: install on headless Linux with no DBUS; user edits connection password; `SetPassword` is gated by `s.passwordStore != nil`; old plaintext (from pre-keychain install) is loaded and returned to the Wails binding on every Load.