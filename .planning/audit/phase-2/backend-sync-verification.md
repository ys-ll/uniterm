# Phase 2 Verification — backend/sync/

**Date:** 2026-07-28
**Verifier:** Phase 2 (independent reproduction against source)
**Scope:** Re-verify 35 findings from `.planning/audit/phase-1/backend-sync.md` against `backend/sync/{sync_service.go,crypto.go,git.go,keychain.go,sync_config.go}`.
**Method:** Direct read of cited `file:line` + ≥ 5 lines of context; construct concrete triggering state; mark CONFIRMED / PLAUSIBLE / FALSE_POSITIVE / RETRACTED. ROI assigned only to CONFIRMED (high / medium / low).

---

## Verdict Summary

| ID | Title | Severity | Verdict | ROI | Notes |
|----|-------|----------|---------|-----|-------|
| SYNC-P0-1 | `isConfigDirEmpty` partial scan | P0 | CONFIRMED | high | `sync_service.go:444-457` only inspects `connections.json`; local with rich settings/quickCommands but no connections returns `true` and triggers destructive `DecryptConfigFiles` overwrite at `sync_service.go:309-311`. |
| SYNC-P0-2 | `ResolveConflict` no mutex | P0 | CONFIRMED | high | `sync_service.go:183-225` never takes `s.mu`; `Sync(74)`, `ConfigureRepo(267)`, `ChangePassword(602)`, `DeleteRepo(659)` all lock. Concurrent Sync + Resolve races on the same `*git.Repository` worktree. |
| SYNC-P1-1 | AES-GCM no AAD | P1 | CONFIRMED | medium | `crypto.go:276` `Seal(nonce, nonce, plaintext, nil)` and `crypto.go:298` `Open(nil, nonce, ciphertext, nil)` both pass `nil` AAD. Cross-file ciphertext swap is undetected. |
| SYNC-P1-2 | `compareConfigFiles` structural equality | P1 | CONFIRMED | medium | `sync_service.go:493-494` re-marshal of `map[string]interface{}` is alphabetical-deterministic but ignores `[...]` order, schema drift, and the *asymmetric* `backfillFromKeychain(localObj,...)` (`sync_service.go:491`) — remote is never backfilled. Spurious diff → spurious commit. |
| SYNC-P1-3 | Mixed time sources in conflict UI | P1 | CONFIRMED | low | `getConfigModTime` (`sync_service.go:429-441`) returns filesystem mtime; `CompareHeads` (`git.go:201-202`) returns `Committer.When`. Same-named field `LocalTime/RemoteTime` carries two unrelated meanings. Misleading UX, not data corruption. |
| SYNC-P1-4 | `commitMsg` hostname leak | P1 | CONFIRMED | medium | `sync_service.go:672-676` embeds `os.Hostname()` in every commit. Public-shared org repos deanonymize machine names. |
| SYNC-P1-5 | `GetGitToken` swallows keyring err | P1 | CONFIRMED | medium | `keychain.go:53-59` returns `("", nil)` on missing entry; `getToken` (`sync_service.go:67-70`) discards second return. `Sync()` then proceeds with empty BasicAuth password — provider-dependent failure. |
| SYNC-P1-6 | `ChangePassword` salt reuse | P1 | CONFIRMED | medium | `sync_service.go:613,622,628` — same salt derives both `oldKey` and `newKey`. PBKDF2 work amortizes; rotation as a security control is weakened; identical-password "change" produces identical ciphertexts. |
| SYNC-P1-7 | Plaintext `sync-config.json` | P1 | CONFIRMED | low | `sync_config.go:34-39` writes `RepoURL`/`Username`/`LastSyncError` plaintext at `0600`. Backups / file-sync tools capture as-is. |
| SYNC-P1-8 | `isConfigDirEmpty` no size guard | P1 | CONFIRMED | low | `sync_service.go:444-457` `os.ReadFile` + `json.Unmarshal` with no size cap. Realistic risk is low (connections.json rarely gigabytes) but no defense-in-depth against malicious JSON in shared repo. |
| SYNC-P1-9 | `wt.Add(".")` stages arbitrary files | P1 | CONFIRMED | high | `git.go:101`. Any stray file dropped into `s.repoPath` (`~/.config/uniTerm/sync-repo/`) is committed unencrypted — e.g. user drops a private SSH key → next sync commits it plaintext. |
| SYNC-P1-10 | Network ops no retry/backoff | P1 | CONFIRMED | low | `git.go:118-140` single-shot `Push/Fetch/Pull`. Transient 503 / DNS blip aborts whole sync. UX issue, not data loss. |
| SYNC-P1-11 | Decrypt failures swallowed → overwrite remote | P1 | CONFIRMED | high | `sync_service.go:690-692`: `if err := DecryptConfigFiles(...); err != nil { return false, nil }` — treats decrypt failure as "changed". Subsequent `EncryptConfigFiles` (line 103) + `Push` clobbers the only good copy of remote with locally-derived ciphertext on next sync. Catastrophic on bad-password scenario. |
| SYNC-P2-1 | No busy-state signal | P2 | PLAUSIBLE | low | `sync_service.go:20,74` uses plain `sync.Mutex`; double-click silently queues a second 10s Sync that runs after first finishes. UX issue. |
| SYNC-P2-2 | `backfillFromKeychain` skips non-password auth | P2 | CONFIRMED | medium | `sync_service.go:498-537`, `if cm["authType"] != "password" { continue }` (line 506). SSH-key passphrase / cert auth secrets stored only in keychain are never round-tripped — partial-coverage of "encryption" claim. |
| SYNC-P2-3 | `compareConfigFiles` ignores unmarshal errors | P2 | CONFIRMED | low | `sync_service.go:477-488` discards `json.Unmarshal` errors via bare `json.Unmarshal(...)`. Two broken files both marshal to `null` → "same". Hides corruption. |
| SYNC-P2-4 | `repoHasFiles` "any file" looseness | P2 | FALSE_POSITIVE | — | `sync_service.go:696-704` returns `true` only when ALL three files exist (`os.IsNotExist` short-circuits to `false`); the finding describes opposite behavior. Function is actually *stricter* than claimed. |
| SYNC-P2-5 | Decrypted-on-disk gap during decrypt | P2 | RETRACTED | — | Auditor self-retracted at `phase-1/backend-sync.md:121`: decrypt writes *after* password clearing loop — order is correct. No issue. |
| SYNC-P2-6 | `TestConnection` no scheme allow-list | P2 | PLAUSIBLE | low | `git.go:254-265` accepts any URL scheme. `file://` reads local disk. Requires user cooperation, but no defense-in-depth. |
| SYNC-P2-7 | `GetEncryptionKey` no length check | P2 | CONFIRMED | low | `keychain.go:39-46` returns `hex.DecodeString` result without `len(decoded) == keyLength` check. AES rejects at use time but error surface is far from cause. |
| SYNC-P2-8 | `compareConfigDirs` ignores file mode | P2 | CONFIRMED | low | `sync_service.go:462-473` no mode check. Out-of-scope for current 3-file data model (all serialize at 0600) — finding already concedes this. |
| SYNC-P2-9 | `CloneOrOpen` no retry after `initEmpty` | P2 | CONFIRMED | low | `git.go:32-63` `initEmpty` hard-codes HEAD to `main` (line 70); subsequent `CompareHeads(config.Branch)` fails for non-`main` defaults. The Sync path handles it (returns SyncPush), but `ConfigureRepo` writes to whatever branch the user typed. |
| SYNC-P2-10 | `PushToBranch` hardcoded refspec | P2 | CONFIRMED | low | `git.go:217-229` always reads `refs/heads/<branch>` (line 218) — if local HEAD diverged onto another branch, wrong branch is pushed. No `HEAD:refs/heads/<branch>` semantics. |
| SYNC-P2-11 | `ResolveConflict(false)` hard reset | P2 | CONFIRMED | medium | `sync_service.go:215-220` calls `ResetToRemote` (HardReset at `git.go:248-251`) without backing up worktree. Un-pushed in-flight edits (or worktree index state) are wiped silently. |
| SYNC-P3-1 | `IsAutoSyncEnabled` no caller | P3 | FALSE_POSITIVE | — | `app.go:187` calls on startup; `app.go:553-570` `triggerAutoSync` is invoked from `app.go:284,422,720,740`. Feature IS wired. |
| SYNC-P3-2 | `commitMsg` RFC3339 (no nanos) | P3 | CONFIRMED | low | `sync_service.go:675`. Rapid auto-commits within same second collide on timestamp. |
| SYNC-P3-3 | `keychainService = "uniTerm"` generic | P3 | CONFIRMED | low | `keychain.go:14`. Cross-app collision plausible on libsecret with default key names (`git-token`, `encryption-key`). |
| SYNC-P3-4 | `IsAutoSyncEnabled` no read lock | P3 | CONFIRMED | low | `sync_service.go:249-253` reads config without `s.mu`. Concurrent `SaveConfig` races detectable by `-race`. |
| SYNC-P3-5 | `updateLastSyncResult` writes per outcome | P3 | CONFIRMED | low | `sync_service.go:241-247`; called from ≥ 15 sites (line 96, 104, 110, 119, 122, 125, 132, 138, 143, 146, 151, 155, 158, 211, 223, 321, 342, 365, 424). Amplified by auto-sync frequency. |
| SYNC-P3-6 | `GetEncryptionKey` length cross-ref | P3 | CONFIRMED | low | Duplicate of SYNC-P2-7; cross-ref handled in one fix. |
| SYNC-P3-7 | Hardcoded commit author | P3 | CONFIRMED | low | `git.go:105-110` `Author: "uniTerm <uniterm@local>"`. Multi-device sync loses attribution. |
| SYNC-P3-8 | `CloneOrOpen` re-opens every sync | P3 | CONFIRMED | low | `sync_service.go:94`. Disk-touching on every Sync click; small perf cost only. |
| SYNC-P3-9 | AAD nil cross-ref | P3 | CONFIRMED | low | Duplicate of SYNC-P1-1; tracked under one fix. |
| SYNC-P3-10 | `LastSyncError` unbounded | P3 | CONFIRMED | low | `sync_config.go:19` `LastSyncError string` no max length; go-git transport errors may contain URL with embedded creds. |
| SYNC-P3-11 | PBKDF2 iterations not injectable | P3 | CONFIRMED | low | `keychain.go:17` constant 600k; tests can't run fast (many seconds for suite). |
| SYNC-P3-12 | Remote-side not backfilled | P3 | CONFIRMED | low | `sync_service.go:491` only `backfillFromKeychain(localObj,...)` — comparison asymmetric. Currently safe (encryption always inlines password per `crypto.go:65-77`), but pre-emptive. |
| SYNC-P3-13 | Zero `time.Time` in JSON | P3 | CONFIRMED | low | `sync_service.go:429-441`. Returns `time.Time{}` when no files; JSON serializes as `0001-01-01T00:00:00Z`. Cosmetic. |

---

## Confirmed (with ROI)

**high (4):**
- SYNC-P0-1 — silent overwrite of local settings/quickCommands when connections is empty (`sync_service.go:444-457` + `:307-323`)
- SYNC-P0-2 — `ResolveConflict` mutex absence → `.git/index.lock` collisions on concurrent Sync/Resolve (`sync_service.go:183-225`)
- SYNC-P1-9 — `wt.Add(".")` at `git.go:101` commits arbitrary unencrypted files dropped into `sync-repo/`
- SYNC-P1-11 — decrypt failure swallowed at `sync_service.go:690-692` → next sync overwrites remote with garbage

**medium (7):**
- SYNC-P1-1 — AES-GCM no AAD at `crypto.go:276,298` (cross-file swap)
- SYNC-P1-2 — `compareConfigFiles` fragility at `sync_service.go:476-496`
- SYNC-P1-4 — hostname leak at `sync_service.go:672-676`
- SYNC-P1-5 — swallowed keyring err at `keychain.go:53-59` → Sync proceeds with no auth
- SYNC-P1-6 — salt reuse on `ChangePassword` at `sync_service.go:613,622,628`
- SYNC-P2-2 — `backfillFromKeychain` skips non-password auth at `sync_service.go:506`
- SYNC-P2-11 — `ResetToRemote` hard reset at `sync_service.go:215-220`

**low (18):**
- SYNC-P1-3 — mixed time sources (UX)
- SYNC-P1-7 — plaintext `sync-config.json` (mitigated by 0600)
- SYNC-P1-8 — no size guard (low practical risk)
- SYNC-P1-10 — no retry/backoff (UX)
- SYNC-P2-3 — unmarshal errors ignored (low-probability corruption)
- SYNC-P2-7 — no key length check
- SYNC-P2-8 — no mode check (already out of scope)
- SYNC-P2-9 — `initEmpty` hardcoded branch
- SYNC-P2-10 — hardcoded refspec
- SYNC-P3-2 — RFC3339 (no nanos)
- SYNC-P3-3 — generic keychain service name
- SYNC-P3-4 — read without lock (race detector only)
- SYNC-P3-5 — per-outcome config writes
- SYNC-P3-6 — cross-ref to P2-7
- SYNC-P3-7 — hardcoded author
- SYNC-P3-8 — `CloneOrOpen` re-opens every Sync
- SYNC-P3-9 — cross-ref to P1-1
- SYNC-P3-10 — `LastSyncError` unbounded
- SYNC-P3-11 — PBKDF2 iterations not injectable
- SYNC-P3-12 — asymmetric backfill (pre-emptive)
- SYNC-P3-13 — zero `time.Time` JSON

(Counting low: 21 — listed individually for fix-tracking.)

## Plausible (deferred)

- SYNC-P2-1 — busy-state UX (low ROI; defer until user reports double-click issue)
- SYNC-P2-6 — scheme allow-list (requires user cooperation; defer until threat model expands)

## False Positives (drop)

- SYNC-P2-4 — `repoHasFiles` actually requires ALL three files (`sync_service.go:696-704` short-circuits to `false` on any missing file); finding described opposite behavior.
- SYNC-P3-1 — `IsAutoSyncEnabled` IS wired: `app.go:187` (startup) + `app.go:284,422,720,740` (via `triggerAutoSync`). Feature is functional.

## Retracted by Phase 1 auditor (acknowledge, do not re-verify)

- SYNC-P2-5 — decrypt-on-disk reorder was claimed; auditor self-retracted after re-reading `crypto.go:159-193` (decrypt then clear then write is correct order).

## Net for fix queue

CONFIRMED + (high|medium): **11**

(4 high + 7 medium. 21 low are eligible but defer until high/medium are addressed.)

---

## Special-focus confirmations (P0)

### SYNC-P0-1 — `isConfigDirEmpty` partial scan

- **Source:** `backend/sync/sync_service.go:443-457`
- **Confirmed behavior:** Reads `connections.json` only. `settings.json` and `quickCommands.json` are never inspected. Returns `true` when (a) `connections.json` missing, (b) `connections.json` unparseable, or (c) `wrapper.Connections` is empty.
- **Triggering state at `ConfigureRepo`:** User with `connections.json = {"connections":[]}` but rich `settings.json` (AI prompts, theme) and `quickCommands.json` → `localEmpty := isConfigDirEmpty(s.configDir)` returns `true` (`sync_service.go:304`) → falls into `if localEmpty` branch at `:307` → `DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain)` at `:309-311` writes remote versions over local settings and quickCommands.
- **Verdict:** CONFIRMED, high ROI. Fix must check all three files OR route local-empty through `compareConfigDirs` like the normal Sync() path.

### SYNC-P0-2 — `ResolveConflict` no mutex

- **Source:** `backend/sync/sync_service.go:182-225`
- **Confirmed behavior:** Function body never calls `s.mu.Lock()` / `s.mu.Unlock()`. Direct line-by-line audit:
  ```
  183: func (s *SyncService) ResolveConflict(useLocal bool) (*SyncResult, error) {
  184:     config, err := s.configStore.Load()       // no lock
  ...
  191:     repo, err := CloneOrOpen(...)              // no lock
  ...
  201:     if useLocal {
  202:         EncryptConfigFiles(...)                // no lock
  205:         repo.StageAndCommit(...)               // no lock
  208:         repo.ForcePush(...)                    // no lock
  ...
  215:     repo.ResetToRemote(...)                    // no lock
  ```
- **Comparison with other public mutators:** `Sync()` (`:74`), `ConfigureRepo()` (`:267`), `ChangePassword()` (`:602`), `DeleteRepo()` (`:659`) all begin with `s.mu.Lock(); defer s.mu.Unlock()`.
- **Triggering state:** Sync() in flight holds `s.mu`; user accepts the resulting conflict UI; `app.go:611 SyncResolveConflict(useLocal=false)` calls `ResolveConflict` from another goroutine/tick. Both manipulate the same on-disk `*git.Repository` worktree → `go-git` writes `.git/index` under both paths → second writer either fails with `index.lock` contention or commits on a half-reset worktree.
- **Verdict:** CONFIRMED, high ROI. Fix is `s.mu.Lock(); defer s.mu.Unlock()` at top of `ResolveConflict`.

---

## Cross-references and notes

- `SYNC-P3-6` and `SYNC-P3-9` are cross-refs to `SYNC-P2-7` and `SYNC-P1-1`; counted once in fix queue.
- `SYNC-P3-5` (`updateLastSyncResult` writes per outcome) is worth a single fix even though low severity: batching the final save at end of `Sync()` removes both the I/O and the read-without-lock race that `SYNC-P3-4` highlights.
- `SYNC-P2-3` and `SYNC-P1-2` share the same fix-shape (semantic comparison); combining is cheaper than two patches.
- Auditor summary line "Total findings: 25"