# Phase 1 Audit — SUMMARY

**Date:** 2026-07-28
**Scope:** 6 module boundaries × parallel `gsd-code-reviewer` sub-agents
**Goal:** Severity-classified findings with file:line refs, ranked by severity × confidence.

---

## Aggregate by Severity

| Severity | Count |
|----------|-------|
| P0 — Critical | **15** |
| P1 — High    | **40** |
| P2 — Medium  | **63** |
| P3 — Low / Informational | **58** |
| **TOTAL**    | **176** |

| Module | P0 | P1 | P2 | P3 | Total |
|--------|----|----|----|----|-------|
| backend/session/ | 0 | 5 | 13 | 13 | 31 |
| backend/database/ | 5 | 7 | 10 | 10 | 32 |
| backend/store/ | 4 | 8 | 12 | 8 | 32 |
| backend/k8s/ | 0 | 2 | 5 | 7 | 14 |
| backend/sync/ | 2 | 11 | 9 | 13 | 35* |
| frontend/ | 4 | 6 | 15 | 7 | 32 |

*sync count 35 includes one finding retracted (false positive); audit summary reports 25 effective + 10 retracted/won't-fix; we keep the file as-is.

---

## Top P0 — Critical (all-module)

These warrant immediate Phase-2 verification before any triage decision:

1. **DB-01..05 (backend/database/)** — SQL injection in identifier quoting. MySQL `dbName` interpolated raw into backtick-wrapped DDL paths (`provider_mysql.go:53,152,177,213,245,250,258-364`); MySQL/Postgres/rqlite `Quote` does not escape backtick/quote chars. Reachable from WebView-bound `App.GetTables/CreateDatabase/DropDatabase` and from synced connection profiles.
2. **STORE-01 (backend/store/skills_store.go)** — `SkillsStore.GetBody` reads `metas[0].IsSystem` instead of the requested skill's `IsSystem`. User can read system skills (potential secret content) by querying a user skill name.
3. **STORE-02 (backend/store/skills_store.go)** — `SkillsStore.Delete` / `importToDir` follow symlinks in imported skills and remove arbitrary directories (path traversal + arbitrary file delete).
4. **STORE-03 (backend/store/)** — Every store writes with plain `os.WriteFile` — no temp-write + rename + fsync. Power loss corrupts the only copy of AI session history / settings / connections.
5. **STORE-04 (backend/store/connection_store.go)** — `populatePasswords` silently writes plaintext passwords to `connections.json` when keychain unavailable (operational variant of CONCERNS-noted plaintext-password issue).
6. **SYNC-P0-1 (backend/sync/sync_service.go:307-323)** — `isConfigDirEmpty` only inspects `connections.json`; user with zero connections but rich settings/AI/quick-commands gets silent destructive overwrite on first Sync.
7. **SYNC-P0-2 (backend/sync/sync_service.go)** — `ResolveConflict` does not take `s.mu`; concurrent Sync + Resolve yields worktree corruption via `.git/index.lock` collisions.
8. **FE-01 (frontend/src/components/AIMessage.vue)** — `v-html` XSS surface in homegrown markdown renderer.
9. **FE-02 (frontend/src/components/AISidebar.vue)** — MutationObserver memory leak (two `onMounted` blocks share one variable).
10. **FE-03 (frontend/src/)** — Widespread `EventsOn` registrations without matching `EventsOff` across 10+ files (Tab/Settings/Sidebar/AISidebar/Terminal etc.).
11. **FE-04 (frontend/src/composables/useUpdateCheck.ts)** — Module-level `setInterval` never torn down.

---

## Top P1 — High (selected — first 10)

1. **SESSION-01** — SSH `Disconnect`'s `sync.Once{ close(s.quit) }` permanently breaks Connect-then-Connect on the same `SSHSession` (keepalive exits immediately on second connect because `s.quit` is already closed).
2. **SESSION-02** — `kbCallback` closure reads `s.authAnswerCh` through the receiver while `Connect`'s defer nils it; race-detector hit and goroutine-leak risk.
3. **SESSION-03** — `FTPSession.Disconnect` calls `s.conn.Quit()` and `s.conn = nil` without holding `s.connMu` that transfer goroutines use — library-level panic risk on concurrent close.
4. **SESSION-04** — `VNCProxy.Stop` / `SPICEProxy.Stop` can leak goroutines between `listener.Close()` and `wg.Wait()`.
5. **SESSION-05** — Telnet `sendAutoLogin` hard-codes `time.Sleep(1500ms)` before sending the username; no prompt detection.
6. **DB-06..12** — Postgres `sslmode=disable` hard-coded; no connection-pool limits in `NewDB`; MySQL DDL on potentially different pool connections; `ExecuteQuery/ExecuteStatement` use `context.Background()` with no timeout; SQL Server CRUD bypasses `execPrepared`; Oracle `PrepareExec` case-insensitivity collision.
7. **STORE-05..12** — Skills migration atomicity, settings write race, terminal history unbounded growth, tunnels JSON parse crash, AI sessions no locking, quick commands path traversal, commands_store no rollback, local_state_store on every keystroke.
8. **K8S-01** — `authRoundTripper` has no 401 retry → broken for exec-plugin auth (GKE/EKS/AKS/kubelogin).
9. **K8S-02** — Watch & log streams never reconnect after apiserver bounce (goroutine exits via `onEnd`).
10. **SYNC-P1-1** — AES-GCM `Seal`/`Open` pass `nil` for `additionalData`, cross-file ciphertext swap undetectable.
11. **SYNC-P1-4** — `commitMsg` calls `os.Hostname()` and bakes it into every commit, leaking machine names.
12. **SYNC-P1-6** — `ChangePassword` re-uses the same salt for old+new keys, defeats password rotation.
13. **SYNC-P1-9** — `wt.Add(".")` stages *anything* dropped in the sync repo — SSH keys get published.
14. **FE-P1-1..6** — BaseTerminal disposal chain, xterm addon load order race, EventEmitter double-binding, scrollback cap not enforced, sessionStore buffer race, tab type-discrimination `as any`.

---

## Cross-Cutting Themes

1. **Missing atomic-write primitive.** Every backend store writes with `os.WriteFile` directly; no temp-write + rename + fsync. A single `atomicWriteFile` helper would resolve 4+ P0/P1 findings at once.
2. **Inconsistent mutex discipline.** `backend/sync/sync_service.go`: 4 of 5 mutators locked, 1 (`ResolveConflict`) not. `backend/store/`: only `RecentStore` has a mutex. Concurrent-mutation panics expected under load.
3. **Build-tag split files at risk.** `app_darwin.go` / `app_notdarwin.go`, `local_session_unix.go` / `local_session_windows.go`, `fonts_*.go` — fixes must avoid opportunistic changes; new build-tag issues surfaced only when findings explicitly apply.
4. **Identifier-quoting is provider-inconsistent.** Oracle/SQL Server escape properly; MySQL/Postgres/rqlite do not → SQL-injection surface in 5 of 5 SQL providers.
5. **WebView-bound surface area.** `main.go:110-112` binds every exported `App` method without per-call size limits; combined with `as any` casts in the frontend and missing TS validation at the Wails boundary, the WebView becomes an attack surface for any imported third-party widget.
6. **Memory leaks in long-lived components.** `AISidebar`, `Sidebar`, `SettingsTab`, all `BaseTerminal` consumers — EventsOn / MutationObserver / setInterval not torn down → unbounded growth on long sessions.
7. **xterm.js addon lifecycle.** Unicode11, fit, webgl, search addons loaded but not consistently disposed; load-order race documented.
8. **AI Agent tool input unvalidated.** `tu.input.command as string` — confused-deputy risk; no zod/valibot at boundary.

---

## Phase-2 Input

Phase 2 should focus verification effort on:
- **All P0 (15)** — every one needs a CONFIRM/REFUTE pass before any decision.
- **P1 with concrete repro path** (15+ items) — these are easiest to confirm.
- **CONCERNS-sharpenings** — SESSION-06 (kbCallback echo bug extension), SESSION-15 (FTP ChangeRemoteDir missing mutex).

Findings marked `PLAUSIBLE` (race conditions, leaks observable only at scale) can be deferred to Phase 4 verification once a repro is built.

---

## File Map

| Report | Findings | Notes |
|--------|----------|-------|
| `.planning/audit/phase-1/backend-session.md` | 31 | 0 P0 / 5 P1 / 13 P2 / 13 P3 |
| `.planning/audit/phase-1/backend-database.md` | 32 | 5 P0 (SQLi) / 7 P1 / 10 P2 / 10 P3 |
| `.planning/audit/phase-1/backend-store.md` | 32 | 4 P0 / 8 P1 / 12 P2 / 8 P3 |
| `.planning/audit/phase-1/backend-k8s.md` | 14 | 0 P0 / 2 P1 / 5 P2 / 7 P3 |
| `.planning/audit/phase-1/backend-sync.md` | 35 | 2 P0 / 11 P1 / 9 P2 / 13 P3 |
| `.planning/audit/phase-1/frontend.md` | 32 | 4 P0 / 6 P1 / 15 P2 / 7 P3 |
| **TOTAL** | **176** | **15 P0 / 40 P1 / 63 P2 / 58 P3** |

---

*Phase 1 audit complete. Phase 2 triage can now begin.*