# Codebase Concerns

**Analysis Date:** 2026-07-28

## Tech Debt

**SSH host key verification disabled everywhere:**
- Issue: Every SSH-style session sets `ssh.InsecureIgnoreHostKey()` as the `HostKeyCallback`, accepting any host key without warning or persistence. The codebase explicitly enables legacy/insecure KEX algorithms alongside this.
- Files: `backend/session/ssh_session.go:176`, `backend/session/sftp_session.go:83`, `backend/session/mosh_session.go:52`, `backend/session/tunnel_forward.go:230`, `backend/session/tunnel_service.go:61`, `backend/session/monitor_session.go:136`; `backend/session/ssh_config.go:7` includes `InsecureKeyExchangeDH1SHA1`, `InsecureKeyExchangeDH14SHA1`, `InsecureKeyExchangeDHGEXSHA1` to talk to legacy servers.
- Impact: Man-in-the-middle attacks on SSH/SFTP/tunnel traffic; legacy KEX algorithms permit downgrade attacks.
- Fix approach: Prompt on first connect to trust-and-pin host key to local storage (`~/.ssh/known_hosts` style), default to known_hosts lookup, gate legacy KEX behind a per-connection "Allow legacy algorithms" toggle in the connection form.

**TLS verification bypassed by configuration / hard-coded:**
- Issue: `InsecureSkipVerify: true` is wired straight through user config without a separate user-facing warning.
- Files: `backend/k8s/client.go:65` (`cfg := &tls.Config{InsecureSkipVerify: cluster.InsecureSkipTLSVerify}`), `backend/session/ftp_session.go:58` (FTP dial hard-codes `InsecureSkipVerify: true`).
- Impact: Active MITM on Kubernetes API servers when the user toggles the kubeconfig setting; passive MITM on FTP servers (no per-connection toggle exists).
- Fix approach: Surface a banner/warning before save; offer one-time trust pinning (cert SHA-256 fingerprint) as an alternative to global skip-verify.

**Plaintext password storage accepted by connection schema:**
- Issue: `ConnectionConfig.Password` is a `json:"password,omitempty"` field. The `connection_store.go` only migrates to OS keychain **after** the user installs the `keyring` store; older installs (or those that fail to initialize the keychain) continue writing plaintext passwords to `connections.json`.
- Files: `backend/session/session.go:43` (schema comment: "Password is stored in plaintext JSON. Will be migrated to OS keychain in a future iteration."), `backend/store/connection_store.go:52-85` (only clears after keychain write), `backend/store/skills_store.go`.
- Impact: Backup copies, sync, and accidental JSON dumps leak credentials. Sync encryption keys are also cached in the OS keychain as a hex-encoded derived key — losing the master password leaves the synced blob unrecoverable.
- Fix approach: Make keychain mandatory; fail-closed on init failure. Drop the plaintext `Password` field from JSON serialization entirely (`json:"-"`) once migration is verified.

**Plaintext sync config backup / temp directories persist decrypted blobs:**
- Issue: Sync flow writes decrypted JSON to temp dirs (`sync-compare`, `sync-cmp`, `sync-verify`) and only removes them via `defer os.RemoveAll`. Crashes leave plaintext connection JSON on disk.
- Files: `backend/sync/sync_service.go:294-298`, `684-688`, `712-716`.
- Impact: Plaintext credentials survive an unclean shutdown under the OS temp directory.
- Fix approach: Decrypt in-memory only, or use `os.CreateTemp` with `O_EXCL` then `os.Remove` immediately after read; ensure files are `0600` and not world-readable.

**Migration safety net relies on error swallowing:**
- Issue: Migration / cache-save code paths intentionally drop errors (`jsonData, _ := json.MarshalIndent(...)`, `_ = os.WriteFile(...)`, `_ = s.passwordStore.SetPassword(...)`). A bad marshal or full disk silently corrupts user data without surfacing to the UI.
- Files: `backend/store/connection_store.go:154`, `backend/store/settings_store.go:204-206`, `backend/sync/crypto.go:78,113,189,227`.
- Impact: Silent data loss / partial migration that the user doesn't see until next launch.
- Fix approach: Surface migration errors to the App layer (log + non-blocking Wails event) so the UI can warn.

## Known Bugs

**Sync repo rewrite-on-any-non-equal-no-commit:**
- Issue: `Sync()` skips the local→remote commit only when `compareLocalWithRepo` returns `same == true`; the comparison is via `compareConfigDirs`, which compares JSON files. Any non-deterministic JSON key order or whitespace mismatch counts as drift and triggers a fresh commit, even when nothing changed semantically.
- Files: `backend/sync/sync_service.go:100-113`, `compareConfigDirs` (line 462).
- Impact: Empty or cosmetic-only edits create spurious commits in the user's sync repo, pollute history.
- Workaround: none in app — fix by normalizing key order before comparison.

**Sync can silently nuke the local config when remote is empty:**
- Issue: When `localEmpty` and `!remoteEmpty` path is taken, decryption of remote overwrites local. There is no dry-run/confirm; if the local user thinks they have data, the empty path silently nukes it.
- Files: `backend/sync/sync_service.go:307-322`.
- Impact: Local-only connections/settings lost on first connect to remote repo if local config is somehow empty.
- Workaround: prompt user before destructive pull.

**Force-push in conflict resolution overwrites remote with no merge step:**
- Issue: `ResolveConflict(useLocal=true)` calls `repo.ForcePush` immediately after re-encrypting local — no attempt at three-way merge or per-field diff.
- Files: `backend/sync/sync_service.go:201-213`.
- Impact: The losing side loses all unique entries.
- Workaround: user manually exports settings.json before pressing "use local" / "use remote".

**k8s `authRoundTripper` does not implement `io.Closer` / connection draining:**
- Issue: The custom `RoundTripper` holds a base `http.Transport` but never closes idle connections on auth changes (e.g. token rotation). P2 mention ("处理 exec provider 时可以在此扩展") was never finished; current code silently leaves idle conns open.
- Files: `backend/k8s/client.go:133-144`.
- Impact: Connection pool grows until the kube API server rate-limits the client.
- Workaround: restart app to flush.

**SSH keyboard-interactive prompt silently disables echo for password input but uses ASCII printable for echo-on prompts:**
- Issue: `kbCallback` echoes input only when `echos[i]` is true; the standard `kbCallback` early-return path emits `"\r\n" + q + " "` but if the server fails to advertise prompts the local `shouldPromptForSSHPassword` loop captures raw bytes (e.g. ANSI control sequences from remote echoing) into `answer`.
- Files: `backend/session/ssh_session.go:94-168`.
- Impact: Password gets corrupted if remote sends ANY bytes alongside the prompt (e.g. MOTD).
- Workaround: none — user enters password blindly.

**`RDPSession` ActiveX control is Windows-only and unstable:**
- Issue: `RDPSession` requires Windows + ActiveX + atl.dll; non-Windows stubs return `RDP is only supported on Windows`. The ActiveX embedding via `AtlAxWinInit` / `AtlAxGetControl` is fragile (depends on user having the RDP ActiveX registered), crashes leave the COM apartment in unknown state.
- Files: `backend/session/rdp_session.go:1-942` (entire Windows file), `backend/session/rdp_session_stub.go`.
- Impact: RDP works only on Windows with proper MSTSC plugins; users on other platforms can't use RDP feature at all.
- Workaround: full RDP rewrite using FreeRDP / rdr-desktop (significant effort).

**`output_log.go` flushes on every write:**
- Issue: `WriteOutput` calls `file.Sync()` after every write to maximize durability, but `Enable` also `Sync`s immediately. On long sessions with thousands of writes per second this stalls on disk I/O.
- Files: `backend/session/output_log.go:522-524,461-463`.
- Impact: Terminal throughput drops / latency spikes on slow disks (HDD or network mounts).
- Workaround: temporarily disable session log via settings.

**`disconnectNotice` is appended even on EOF where the user might have already seen the disconnect:**
- Issue: `readLoop` emits a red disconnect banner on EOF after the connection has been silent; on slow networks this sometimes races with the keepalive diagnostics.
- Files: `backend/session/ssh_session.go:325`.
- Impact: Cosmetic duplicate banners.

## Security Considerations

**No Content-Length / max-size guard on incoming Wails payloads:**
- Issue: App.go exports `Bind: []interface{}{app}` — every exported method is reachable from the WebView. Methods accepting JSON blobs (e.g. `SaveConnections`, `UpdateSettings`, `LoadKubeconfig(kubeconfigYAML []byte)`) have no per-call size limit; an attacker controlling the WebView (e.g. via XSS in a future renderer) could push arbitrarily large blobs.
- Files: `main.go:110-112`, `backend/k8s/manager.go:64` (`ListContexts(kubeconfigYAML []byte)`).
- Mitigation: Wails runs the WebView locally with no remote attacker, but extensions or imported third-party widgets share the WebView.
- Recommendations: add a request-size guard or chunked parsing limit at each entry point.

**Git sync auth uses `username` field literally as Basic-Auth user; token may leak in error messages:**
- Issue: `buildAuth(username, token)` is `BasicAuth{Username, Password}`. If `username` is empty, the Basic-Auth request still uses `:` + token — some servers reject, but logs from `git.go` print the username+token pair on failure when surfacing remote errors.
- Files: `backend/sync/git.go:267-272`, `backend/sync/sync_service.go:97-99` (raw error is wrapped and forwarded to the frontend `SyncResult`).
- Mitigation: none — error from go-git propagates.
- Recommendations: strip credentials from error messages before surfacing to the UI; ensure logs do not print auth tokens.

**AES-GCM without authentication tag verification context:**
- Issue: `encryptBytes` / `decryptBytes` use AES-256-GCM with a fresh nonce per file; good. But the encryption key is derived from a single password using PBKDF2 (`pbkdf2Iterations = 600000`). The salt is stored in the repo at `.sync-salt`. There is no second factor — anyone with both the Git repo URL (public if accidentally made public) AND the master password can decrypt.
- Files: `backend/sync/keychain.go:17-19`, `backend/sync/crypto.go:263-303`.
- Mitigation: relies on PBKDF2 iteration count being high enough (600k).
- Recommendations: add argon2id derivation; consider envelope encryption with a per-record data key.

**Frontend `localStorage` use is implicit but untracked — see `console.*` comments:**
- Issue: Frontend doesn't appear to use `localStorage`/`sessionStorage` directly (no matches found), but the Wails runtime may write the WebView's localStorage to `os.TempDir() + webviewDataPath` (see `main.go:42`). Sensitive data placed in any `localStorage` field would persist across launches.
- Files: `main.go:42-43` (`webviewDataPath := filepath.Join(os.TempDir(), fmt.Sprintf("uniTerm-webview2-%d", os.Getpid()))`).
- Mitigation: nil today; risk if future code uses storage.
- Recommendations: audit any future localStorage / IndexedDB usage; consider clearing `webviewDataPath` on shutdown.

**AI Agent risk classification defaults to `write` when missing or unknown:**
- Issue: `getRisk` returns `write` if `tu.input.risk` is missing or any unknown string; `shouldConfirm` then prompts unless user set `bypass`. The model can omit `risk` to bypass default read-only mode silently. No second-line validation.
- Files: `frontend/src/services/agent.ts:65-79`.
- Impact: Bypass prompt for dangerous ops if model sends a typo'd risk string.
- Recommendations: validate `risk` against a strict enum; treat invalid as `dangerous`.

**AI Agent tool input is cast through `as any`/`as string` with no validation:**
- Issue: `tu.input.command as string` — if the model returns `command` as an object/array the string cast yields `"[object Object]"` and the command runs through `terminalAgent.executeCommand` unchanged.
- Files: `frontend/src/services/agent.ts:466-468,526`, `frontend/src/services/llm.ts:87` (`let json: any`).
- Impact: Confused-deputy: malformed model outputs execute arbitrary shell input.
- Recommendations: runtime type-check (zod/valibot) before invoking execution paths.

**Frontend log lines sent to AI may contain secrets (passwords, tokens) echoed by terminal:**
- Issue: AI captures terminal output for context; SSH / DB prompts can echo secrets. No scrubbing layer.
- Files: `frontend/src/services/terminalAgent.ts`, `frontend/src/services/agent.ts`.
- Impact: Credentials leak to third-party LLM endpoints.
- Recommendations: pre-filter captured output for known password-prompt patterns (e.g. `password:`, `passphrase:`).

## Performance Bottlenecks

**`app.go` is 4120 lines — single file holding the entire Wails binding surface:**
- Issue: Every App method is a top-level Go function; many (e.g. settings/sync/AI/store helpers) are colocated rather than in subpackages. Compilation is slower; refactors require touching one huge file.
- Files: `/Users/coderstory/CodeSource/uniterm/app.go`.
- Improvement path: split into `app_bindings_*.go` (already partially done for darwin/windows); consider per-area subpackages (`app_settings.go`, `app_sync.go`) — careful because Wails binds via reflection and any sub-struct must still be in scope.

**`output_log.go` Sync-per-write stalls I/O:**
- See Known Bugs. Improvement: switch to `bufio.Writer` + periodic `Sync`, only `Sync` on flush boundaries.

**`App.vue` is 1869 lines, `Sidebar.vue` 2336, `AISidebar.vue` 2087:**
- Issue: Vue SFC hotspots are enormous; renders pull in everything. Hot reload on edits is slow.
- Files: `frontend/src/App.vue`, `frontend/src/components/Sidebar.vue`, `frontend/src/components/AISidebar.vue`.
- Improvement path: extract feature subcomponents; split template / script with `<script setup>` factoring.

**`monitor_session.go` (1398 lines) handles many concurrent collectors:**
- Issue: Single file holds `df`, `iostat`, `mpstat`, `network`, `process` collectors all sharing one goroutine; serial throughput.
- Files: `backend/session/monitor_session.go`.
- Improvement path: parallelize collectors with bounded worker pool.

**`sync_service.go` Sync uses a single mutex; long file IO blocks other operations:**
- Issue: `s.mu.Lock()` covers the whole sync (clone, commit, fetch, push). UI cannot trigger SaveConfig concurrently.
- Files: `backend/sync/sync_service.go:74-180`.
- Improvement path: split into per-phase locks; offload heavy IO to goroutines with progress reporting.

## Fragile Areas

**RDP ActiveX embedding (Windows):**
- Files: `backend/session/rdp_session.go` (entire 943-line file), `backend/session/rdp_session_stub.go`.
- Why fragile: COM lifecycle, ATL host, unsafe `syscall` window proc modifications. Crashes inside `Disconnect` can leave the Wails app in a wedged state.
- Safe modification: never call `Disconnect` twice without `quitOnce` reset; always `runtime.LockOSThread` around COM calls; never expose `unsafe.Pointer` to JS.
- Test coverage: zero tests for this file.

**`output_log.go` (`lineProcessor`, `ansiStripper`):**
- Files: `backend/session/output_log.go`, `backend/session/output_log_test.go` (27 tests — best-tested in repo).
- Why fragile: chunk-boundary ANSI parsing is stateful; tiny logic errors corrupt logs silently.
- Safe modification: add test cases for every CSI variant before tweaking.

**`sync_service.go` conflict-resolution logic:**
- Files: `backend/sync/sync_service.go`.
- Why fragile: silently destructive — `ForcePush` / `ResetToRemote` overwrite uncommitted state. No undo.
- Safe modification: never expose `ForcePush` without `ResolveConflict` UI confirmation; always log pre-state.
- Test coverage: zero tests.

**`App.startup` and `NewApp` initialization sequence:**
- Files: `app.go:82-300+`.
- Why fragile: store init errors are logged but ignored (`log.Writef` then `return`), leaving `a.connectionStore = nil` while `Bind` is already wired. Frontend calls then NPE / panic on first invocation.
- Safe modification: propagate init errors through a single `error` channel before `wails.Run`; gate `Bind` on `startup` completion.

**`session.Manager.Create` switch:**
- Files: `backend/session/manager.go:21-83`.
- Why fragile: type-based switch must be kept in sync with every new `*_session.go` file. Adding a protocol requires editing this switch AND `app.go` switch.
- Safe modification: registry pattern (`session.Register("ssh", NewSSHSession)`), one source of truth.

**`agent.ts` tool dispatch loop:**
- Files: `frontend/src/services/agent.ts`.
- Why fragile: long switch over tool names; missing tool name silently drops the call; tools with `dangerous` flag bypass confirmation if `risk` is missing.
- Safe modification: typed tool registry; unknown tools → throw.

## Scaling Limits

**`SessionManager` map grows unbounded:**
- Issue: `sm.sessions map[string]Session` is never size-capped; tabs accumulating without `CloseSession` calls leak memory and goroutines.
- Files: `backend/session/manager.go:10-19`.
- Current capacity: appears unbounded by design.
- Limit: a few thousand tabs likely OOMs the desktop app.
- Scaling path: weak-ref cache + LRU; surface UI warning when tab count > N.

**`SyncService.Sync` mutex held for entire network round-trip:**
- See Performance Bottlenecks.

**AI Agent turns hard-capped at 20 by default:**
- Issue: `defaultSettings().AI.MaxTurns = intPtr(20)`. Long tasks abort after 20 iterations.
- Files: `backend/store/settings_store.go:225`.
- Current capacity: 20 turns.
- Limit: complex multi-step automations cannot complete.
- Scaling path: expose per-task turn count; add resumable checkpoints.

**`monitor_session.go` single-pass collector polling:**
- Issue: Polls every N seconds; metrics broadcast via single WebSocket per connection; high-cardinality labels bloat payload.
- Limit: ~50 hosts before frontend render lag becomes noticeable.

## Dependencies at Risk

**`github.com/zalando/go-keyring`:**
- Issue: `keychain.go` reads/writes OS keychain on macOS/Linux/Windows. If `keyring.Set` fails (sandbox, headless Linux), `keychainService = "uniTerm"` entries silently fall through, and `passwordStore` is left nil. There is no fallback to encrypted file — older configs remain plaintext.
- Files: `backend/sync/keychain.go`.
- Migration plan: add an encrypted-file fallback when keyring unavailable; surface UI error.

**`github.com/cloudsoda/go-smb2` (recent, dated 2026-07):**
- Issue: Brand-new fork in go.mod; less battle-tested than `github.com/hirochimanada/smb2`.
- Files: `go.mod` line 5.
- Risk: SMB bugs may surface as connection drops.
- Migration plan: pin version, add smoke tests against a real SMB share.

**`github.com/wailsapp/wails/v2 v2.13.0`:**
- Issue: Wails v2 is the only option; v3 still alpha. `webviewDataPath` workaround in `main.go:42-43` indicates v2 limitation for Windows WebView2 cleanup.
- Files: `main.go`.
- Migration plan: monitor v3 stable, plan upgrade once.

**`github.com/rqlite/gorqlite` (dated 2026-05):**
- Issue: Custom fork; small user base.
- Files: `go.mod`.
- Migration plan: fork if abandoned.

**Frontend `spice-html5-bower@1.7.3` vendored:**
- Issue: Vendored `spice-html5.js` (10476 lines) ships TODO/FIXME markers inside the bundle; not type-checked.
- Files: `frontend/src/vendor/spice-html5.js`.
- Risk: Cannot apply modern lint to vendored JS; security advisories not auto-tracked.
- Migration plan: replace with maintained `@spice-project/spice-html5` or track upstream via npm + patch-package.

## Missing Critical Features

**End-to-end test harness:**
- Problem: `go test ./backend/...` covers k8s helpers, recent_store, output_log, post_login_expect, zmodem_detect, tunnel_forward, platform/fonts_ttf. There are zero tests for any `*_session.go` Connect/Disconnect path; zero tests for `app.go` bindings; zero tests for `sync_service.go` conflict logic.
- Blocks: refactors in those areas are pure risk; the team relies on manual `wails dev` runs.

**Race-detector integration in CI:**
- Problem: No CI configuration found; `go test -race ./...` not in any script. Concurrent code paths in `ssh_session.go` (`authAnswerCh`, `expectOutput`, `decoder`, `decodeLeftover`, `lastRecv`, `lastSent`), `k8s/manager.go`, `session/manager.go` are likely racey under contention.
- Blocks: confidence in concurrent correctness.

**Frontend component tests:**
- Problem: `vitest` installed (`package.json:35`) but zero `.test.ts` for components; no `npm run test` script. Stores / services tests exist (10 files).
- Blocks: regression confidence on UI logic.

**`CSP` / `Permissions-Policy` for embedded WebView:**
- Problem: No CSP set; the WebView loads `frontend/dist` and may run third-party widgets (`@novnc/novnc`, `@xterm/*`).
- Blocks: defense-in-depth against injected JS.

**RDP cross-platform support:**
- Problem: `RDPSession` is Windows-only; non-Windows stub returns error.
- Blocks: Linux/macOS users from using RDP.

**Mosh support requires Unix-shells fork:**
- Problem: `github.com/unixshells/mosh-go v0.5.2` (custom fork, dated).
- Blocks: mosh on Windows + arm64 macOS.

## Test Coverage Gaps

**`session/*_session.go` (Connect/Disconnect paths):**
- What's not tested: `ssh_session.go`, `sftp_session.go`, `telnet_session.go`, `mosh_session.go`, `serial_session.go`, `webdav_session.go`, `smb_session.go`, `s3_session.go`, `ftp_session.go`, `redis_session.go`, `mongodb_session.go`, `rdp_session.go`, `spice_session.go`, `vnc_session.go`, `monitor_session.go` — 14 of the largest session files have no test coverage at all.
- Files: see above.
- Risk: regressions in keyboard-interactive auth, transfer concurrency limits, pause/resume, proxy bridging will go unnoticed.
- Priority: High.

**`backend/sync/*.go` (sync / conflict / encryption):**
- What's not tested: `sync_service.go`, `crypto.go`, `git.go` — all of the encryption + git orchestration logic. `git.go:1` reports `1` test but the grep above shows `_test.go` for git in tests list; verify.
- Files: `backend/sync/sync_service.go`, `backend/sync/crypto.go`.
- Risk: Encryption downgrade, conflict-data-loss bugs will reach production.
- Priority: High.

**`backend/store/*_store.go` (most stores):**
- What's not tested: `ai_session_store.go`, `commands_store.go`, `connection_store.go`, `local_state_store.go`, `quick_commands_store.go`, `settings_store.go`, `skills_store.go`, `terminal_history_store.go`, `tunnel_store.go`. Only `recent_store_test.go` exists.
- Files: see above.
- Risk: JSON migration logic, password migration, keychain fallback paths unverified.
- Priority: High.

**`app.go` (Wails binding surface):**
- What's not tested: All 4120 lines, including `launchConnectGoroutine`, RDP pre-check, `SessionStart` deferred connect.
- Files: `app.go`.
- Risk: regressions in startup sequence, RDP edge cases uncaught.
- Priority: Medium.

**`backend/database/*.go` (provider implementations):**
- What's not tested: `provider_*.go`, `executor.go`, `engine.go`, `schema.go`.
- Files: `backend/database/`.
- Risk: SQL builders, identifier quoting, prepared-statement handling regressions.
- Priority: Medium.

**`frontend/src/services/agent.ts` (AI agent main loop):**
- What's not tested: risk classification, multi-turn loop, queue draining, approval flow.
- Files: `frontend/src/services/agent.ts` (~900 lines).
- Risk: silent bypass of confirmation prompts, runaway loops.
- Priority: High.

**`frontend/src/composables/useTerminal.ts`:**
- What's not tested: xterm integration, resize measurement, fitAddon lifecycle.
- Files: `frontend/src/composables/useTerminal.ts` (731 lines).
- Risk: terminal rendering regressions on size change.
- Priority: Medium.

---

*Concerns audit: 2026-07-28*