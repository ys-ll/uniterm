# Phase 2 Triage — Milestone v0.1

**Date:** 2026-07-28
**Goal:** Confirmed/Refuted verdicts with ROI tags. Separate fix queue (CONFIRMED + high|medium ROI) from discard pile (FALSE_POSITIVE / low ROI).

---

## Verification Totals

| Module | Verified | CONFIRMED (high\|med\|low) | PLAUSIBLE | FALSE_POSITIVE |
|--------|----------|----------------------------|-----------|----------------|
| backend/session/   | 31 | 26 (1\|14\|11) | 5 | 4 |
| backend/database/  | 32 | 27 (4\|8\|15)  | 5 | 2 |
| backend/store/     | 32 | 30 (6\|11\|13) | 1 | 2 |
| backend/k8s/       | 14 | 14 (2\|4\|8)   | 0 | 0 |
| backend/sync/      | 35 | 31 (4\|7\|20)  | 2 | 2 |
| frontend/          | 39 | 28+1partial (2\|11\|15+1part) | 8 | 5+1partial |
| **TOTAL** | **183** | **156 (19\|55\|82)** | **21** | **15** |

---

## Fix Queue — CONFIRMED + (high | medium ROI)

> Each row: ID • title • ROI • file:line • fix shape. Batched by cross-cutting helper where one fix kills many findings.

### Bundle A — `atomicWriteFile` helper (kills STORE-03, -05, -06, -09, -10, -12, -13, -19, -21)
- **STORE-03** *(P0, high)* `backend/store/*.go` (11 sites) — bare `os.WriteFile` on settings/connections/AI sessions/skills/history. Power loss / kill -9 mid-write → 0-byte file → data gone. **Fix:** shared `backend/store/atomic.go` with `atomicWriteFile(path, data, mode)` (temp + fsync + rename).
- **STORE-05** *(P0, high)* Only `RecentStore` has a mutex. Concurrent `SaveConnections` + `SaveSettings` → JSON torn. **Fix:** per-store `sync.Mutex` on each store struct; `atomicWriteFile` covers this since the temp-write-rename pair is itself atomic on POSIX.
- **STORE-09** *(P1, medium)* 4 stores silently wipe data on `json.Unmarshal` failure. **Fix:** surface the error to the App layer (already in CONCERNS); plus: when unmarshal fails, rename the corrupt file to `<file>.corrupt-<ts>` before the new write.
- **STORE-10/12/19/21** — JSON race / lock-contention variants; killed by per-store mutex + atomic write.

### Bundle B — DB `safeIdent` helper (kills AUDIT-01..05)
- **AUDIT-database-01** *(P0, high)* `provider_mysql.go:53,152,177,213,245,250,258-364` — `dbName` interpolated raw into backtick-wrapped DDL. **Fix:** new `backend/database/safeident.go` `SafeQuoteMyIdent(name) (string, error)`; reject names containing `\x00` or length-out-of-range; replace backtick with two backticks.
- **AUDIT-database-02** *(P0, high)* `provider_mysql.go:45-47` — `Quote` doesn't escape backtick. **Fix:** route through `SafeQuoteMyIdent`.
- **AUDIT-database-03** *(P0, high)* `provider_postgres.go:41-43` — `Quote` doesn't escape `"`. **Fix:** `SafeQuotePgIdent` (double-up `"`).
- **AUDIT-database-05** *(P0, high)* `provider_rqlite.go:41-43` — same as Postgres. **Fix:** reuse `SafeQuotePgIdent`.
- **AUDIT-database-04** *(P0, medium)* `provider_postgres.go:307,339-344` — `col.DefaultVal` raw-injected. **Fix:** validate against allowed literal forms (number / string / `NULL` / `CURRENT_TIMESTAMP` etc.) before interpolation; reject otherwise.

### Bundle C — SkillsStore P0 fixes
- **STORE-01** *(P0, high)* `backend/store/skills_store.go:349` — `GetBody` reads `metas[0].IsSystem` instead of the requested skill's `IsSystem`. **Fix:** index into `metas` by the matched index returned from `findMeta`, not `[0]`.
- **STORE-02** *(P0, high)* `backend/store/skills_store.go` (`Delete`/`importToDir`) — symlink following removes arbitrary directories. **Fix:** `os.Lstat` first, refuse if `Mode()&os.ModeSymlink != 0`; on `copyFile`, use `os.OpenFile` with `O_NOFOLLOW`; on `Delete`, evaluate `filepath.Clean` and reject paths that escape the skills root.

### Bundle D — Sync P0/P1 fixes
- **SYNC-P0-1** *(P0, high)* `sync_service.go:444-457` (`isConfigDirEmpty`) + `:307-323` (`ConfigureRepo`) — silent overwrite on first Sync when local has only settings/AI/quick-cmds but no connections. **Fix:** `isConfigDirEmpty` checks all known config files, not just `connections.json`.
- **SYNC-P0-2** *(P0, high)* `sync_service.go:183-225` (`ResolveConflict`) — no `s.mu`. **Fix:** acquire `s.mu.Lock()` at top of `ResolveConflict`.
- **SYNC-P1-1** *(P1, medium)* `crypto.go:276,298` — AES-GCM with `nil` AAD. **Fix:** pass `[]byte(filePath)` as `additionalData`; same on `Open`.
- **SYNC-P1-9** *(P1, medium)* `git.go:101` `wt.Add(".")` stages anything in sync dir. **Fix:** stage each known file explicitly: `wt.Add("connections.json.enc"); wt.Add("settings.json.enc"); ...`.
- **SYNC-P1-11** *(P1, medium)* `sync_service.go:690-692` — decrypt failures swallowed → next sync overwrites remote with garbage. **Fix:** return error from `compareLocalWithRepo` when decrypt fails; surface to UI.
- **SYNC-P1-4** *(P1, medium)* `sync_service.go` `commitMsg` calls `os.Hostname()` and bakes it in. **Fix:** redact to `<redacted>` or omit; commit message doesn't need host identity.
- **SYNC-P1-6** *(P1, medium)* `keychain.go` `ChangePassword` reuses salt. **Fix:** generate a new random salt on password change; store alongside new password verifier.

### Bundle E — K8S 401-retry + reconnect (kills K8S-01, -07, -09, -02, -06, -04, -05)
- **K8S-01** *(P1, high)* `client.go:138-144` `authRoundTripper.RoundTrip` doesn't retry on 401. **Fix:** on 401, refresh the token (re-run exec plugin / reload kubeconfig) and retry once.
- **K8S-07** *(P2, medium)* `client.go:58` ignores `user.Exec`. **Fix:** call exec plugin if `user.Exec` is set; cache token until expiry.
- **K8S-09** *(P2, medium)* `client.go:139-140` mutates `req.Header` in place. **Fix:** `req = req.Clone(req.Context())` before mutating.
- **K8S-02** *(P1, medium)* `watch.go:48-60` and `logs.go:37-41` never reconnect. **Fix:** in `onEnd`, schedule reconnect with exponential backoff (1s, 2s, 4s, 8s, max 30s) bounded by ctx.
- **K8S-04** *(P2, medium)* `manager.go:288` `DialExec` no context. **Fix:** add `ctx context.Context` arg, pass to `dialer.Dial`.
- **K8S-05** *(P2, medium)* `rest.go:38` `io.ReadAll` unbounded. **Fix:** `io.LimitReader(resp.Body, maxResponseBytes)` (e.g. 64 MiB).

### Bundle F — Session fixes
- **SESSION-03** *(P1, high)* `backend/session/ftp_session.go` — `Disconnect` calls `s.conn.Quit()` + `s.conn = nil` without `s.connMu`. **Fix:** acquire `s.connMu` for the duration of the close.
- **SESSION-15** *(P2, medium)* Same — `ChangeRemoteDir` lacks `s.connMu`. **Fix:** acquire before mutating `s.conn`.
- **SESSION-04** *(P1, high)* `backend/session/vnc_proxy.go` / `spice_proxy.go` — `Stop` race between `listener.Close()` and `wg.Wait()`. **Fix:** close listener BEFORE `wg.Add`; ensure `wg.Wait()` runs after all `wg.Add`s in the goroutines.
- **SESSION-13** *(P1, medium)* `backend/session/mosh_session.go` — `bufio.Scanner` uses default 64 KiB buffer and never checks `scanner.Err()`. **Fix:** explicit buffer `scanner.Buffer(make([]byte, 64*1024), 1024*1024)`; check `scanner.Err()` after loop.
- **SESSION-19** *(P1, medium)* `backend/session/local_session_unix.go` — Disconnect lifecycle. **Fix:** ensure PTY `Wait()` goroutine is joined on disconnect.
- **SESSION-28** *(P2, medium)* `backend/session/output_log.go:parseCSIParam` — int overflow → negative `p.pos` → panic. **Fix:** cap to `maxScrollback`.
- **SESSION-17** *(P2, medium)* `backend/session/ssh_auth.go` — key errors swallowed. **Fix:** surface key parse failures to user via UI.
- **SESSION-09** *(P2, medium)* `backend/session/post_login_expect.go` `trimPostLoginOutput` can split UTF-8 mid-rune. **Fix:** use `utf8.DecodeLastRuneInString` to find safe boundary.
- **SESSION-08** *(P2, medium)* `backend/session/output_log.go:applyCSI` ICH doesn't update `p.emitted`. **Fix:** update `p.emitted += n` after insert.
- **SESSION-07** *(P2, medium)* `output_log.go` Sync-per-write. **Fix:** `bufio.Writer` + periodic `Sync` (existing CONCERNS item).
- **SESSION-05** *(P1, medium)* `backend/session/telnet_session.go` `sendAutoLogin` — hard-coded `time.Sleep(1500ms)`. **Fix:** wait for prompt pattern with timeout instead.
- **SESSION-01** *(P1, medium — downgraded from PLAUSIBLE)* `ssh_session.go` `Disconnect`'s `sync.Once{ close(s.quit) }` permanently breaks reuse. **Fix:** only close `s.quit` if it hasn't been closed; or document the single-use nature and add a `Reset()` method.
- **SESSION-12** *(P2, medium)* `tunnel_service.go` parallel shutdown races. **Fix:** serialize shutdown via `s.mu` or done channel.
- **SESSION-14** *(P2, medium)* `serial_session.go` readLoop doesn't exit on `s.quit`. **Fix:** select on `s.quit` + `Read`.
- **SESSION-10** *(P2, medium)* `monitor_session.go` systemInfo retry. **Fix:** bounded retry with backoff.

### Bundle G — Frontend P0/XSS/lifecycle
- **FE-01** *(P0, high)* `frontend/src/components/AIMessage.vue:18,401-403,590-605` — `v-html` XSS via model output. **Fix:** sanitize URL scheme (allow only `http`, `https`, `mailto`); escape `on*=` attrs; strip `data:` URLs from `<img src>`; better yet — render markdown → AST → safe HTML (e.g. via `marked` + `DOMPurify`).
- **FE-03** *(P0, high)* `frontend/src/App.vue:679,681,682` + module-level `EventsOn` in 6 stores (`aiStore.ts:662`, `syncStore.ts:210/218`, `connectionStore.ts:309`, `settingsStore.ts:176`, `tunnelStore.ts:77/81`, `sessionStore.ts:39/48`) — return values discarded; no `EventsOff`. **Fix:** store the return callback, call `EventsOff(event, callback)` in `onUnmounted` (component) or wrap module-level listeners in a singleton teardown helper.
- **FE-04** *(P0, medium)* `frontend/src/composables/useUpdateCheck.ts:33-37,41,83,115-125` — `setInterval(24*60*60*1000)` never cleared. **Fix:** store the interval ID; clear it on `onUnmounted` and on `autoCheck = false`.
- **FE-02** *(P0, medium)* `frontend/src/components/AISidebar.vue:524` — `let mutationObserver` shared by two `onMounted` blocks; only one `onUnmounted` disconnects. **Fix:** rename to two distinct vars (`editableObserver`, `messagesObserver`); disconnect both in `onUnmounted`.

### Bundle H — Frontend P1 (selected)
- **FE-P1-2 / P2-7** *(P1, medium)* `frontend/src/services/llm.ts:65` — hardcoded `max_tokens: 4096` truncates long turns. **Fix:** raise to 16384 OR surface truncation as a soft error.
- **FE-P1-3** *(P1, medium)* `BaseTerminal.vue` disposal chain. **Fix:** ensure every addon (`fit`, `search`, `unicode11`, `webgl`) has matching `.dispose()` in `onUnmounted`.

### Bundle I — Database P1 (non-SQLi)
- **DB-06** *(P1, medium)* `provider_postgres.go:29` — hardcoded `sslmode=disable`. **Fix:** derive from connection form setting.
- **DB-07** *(P1, medium — but already addressed in `session/database_session.go:58-60`)* Mark as **deferred** (false positive).
- **DB-08** *(P1, medium)* MySQL DDL pool race. **Fix:** use `sql.Conn` to pin DDL to one conn.
- **DB-09** *(P1, medium)* `executor.go` `context.Background()` no timeout. **Fix:** `context.WithTimeout(ctx, queryTimeout)`.
- **DB-10** *(P1, medium)* SQL Server CRUD bypasses `execPrepared`. **Fix:** route through `execPrepared`.
- **DB-11** *(P1, medium)* SQL Server `DropColumn` leaks `Rows`. **Fix:** `defer rows.Close()`.

### Bundle J — Store medium (non-Bundle-A)
- **STORE-04** *(P0, medium — downgraded to PLAUSIBLE)* `connection_store.go` `populatePasswords` writes plaintext when keychain unavailable. **Fix:** fail-closed: if `passwordStore == nil`, refuse to save connections and surface an error.
- **STORE-08** *(P2, medium)* Connection store migration race. **Fix:** atomic move via temp file + rename.
- **STORE-11** *(P2, medium)* Settings store on every keystroke. **Fix:** debounce 500ms.
- **STORE-13** *(P2, medium)* `tunnel_store.go` JSON parse crash. **Fix:** wrap `json.Unmarshal` with try/recover? — no, just propagate the error.
- **STORE-16/18/22** *(P2, medium)* — covered by atomic write helper.

---

## Discard Pile (FALSE_POSITIVE / low ROI / deferred)

### FALSE_POSITIVE (drop from fix queue)
- SESSION-02, -06, -23, -30 (kbCallback races that don't exist; S3 bucket clear)
- DB-07 (pool limits — already implemented in session/database_session.go:58-60)
- DB-31 (no Ping — already at line 64)
- STORE-32 partial (ArgumentHint is populated)
- SYNC-P2-4 (repoHasFiles is stricter than claimed)
- SYNC-P3-1 (IsAutoSyncEnabled IS wired)
- FE-P1-5 (gen counter captured as const)
- FE-P2-3 (SettingsTab surfaces lastResult)
- FE-P2-10 (zmodem lock state machine correct)
- FE-P2-12 (no live bug)
- FE-P3-10 (most fields commit on @change, not @input)

### PLAUSIBLE (deferred — needs runtime repro)
- SESSION-01 latent (single-use; API unsafe but not currently triggered)
- SESSION-25, -26, -27, -29, -31 (perf/UX claims unobservable by inspection)
- DB-11, -12, -17, -22, -24 (provider-specific edge cases)
- STORE-04 (partial — the cited line is wrong, broader behavior real)
- SYNC-P2-6, -P3-5 (need real GitHub test)
- FE-P2-1, -2, -8, -11, -13, -14, -15, -16 (perf/UX)

### low ROI (defer or accept)
- All `P3` items (~58 findings).
- All `P2` items without ROI≥medium.

---

## Net Fix Queue

| Category | Items |
|----------|-------|
| CONFIRMED + high | **19** |
| CONFIRMED + medium | **55** |
| **Total in queue** | **74** |
| PLAUSIBLE (deferred) | 21 |
| FALSE_POSITIVE (dropped) | 15 |
| Low ROI / P3 (deferred) | ~73 |

---

## Bundle Strategy for Phase 3

10 bundles, ~30-35 atomic commits (one commit per finding OR one per bundle — preference is **per-bundle** when the helper covers many findings, **per-finding** when the fix is local).

Commit-prefix rules from CLAUDE.md:
- Go: `fix(scope): description — addresses AUDIT-X-NN`
- TS: `fix(scope): description — addresses FE-X-NN`
- Each commit: `go test ./backend/...` green, `npm --prefix frontend run build` green (verified post-bundle, not per-commit, since fix-per-finding may share a helper).

---

*Phase 2 triage complete. Phase 3 can begin applying fixes in bundle order.*