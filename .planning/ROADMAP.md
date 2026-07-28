# uniterm — Roadmap: v1.0 PR Submission

## Overview

21 phases, one per PR. Each phase = one worktree + one branch + one PR. Phases ordered by dependency: foundational → security-critical → perf → hotfix.

| # | Phase | Goal | Reqs | Risk | Depends on |
|---|-------|------|------|------|-----------|
| 1 | PR-01 Terminal render-compat | U+FFFD strip + code-fence brace + box-drawing | PR-01 | low | — |
| 2 | PR-02 JetBrains Mono font | Bundle font + OFL attribution + default stack | PR-02 | low | — |
| 3 | PR-04 xterm polish | Selection, clipboard, addon dispose, Unicode 11, resize | PR-04 | low | — |
| 4 | PR-03 Terminal sizing | Seed PTY + DeferConnect + deps bump | PR-03 | medium | Phase 3 |
| 5 | PR-05 xterm v5.5 docs | Comment-only known gaps | PR-05 | low | — |
| 6 | PR-06 xterm 6.0.0 bump | Dep upgrade | PR-06 | low | Phase 1-4 |
| 7 | PR-07 Terminal themes | Soft Gray + Win11 + Win11 Light + accurate bg | PR-07 | low | — |
| 8 | PR-08 UI themes | Win11 + macOS26 CSS vars + picker | PR-08 | low–med | — |
| 9 | PR-09 Frontend micro-perf | rAF + cache + sanitize + scrollback | PR-09 | low | — |
| 10 | PR-10 aiStore perf | Memoize + shallowReactive + lazy parse | PR-10 | low | — |
| 11 | PR-11 AI agent hardening | Sanitize + risk enum + tool validation | PR-11 | medium | — |
| 12 | PR-15 Store hardening | Atomic write + mutex + symlink + AAD | PR-15 | high | — |
| 13 | PR-16 Store perf | Debounce + shard + cache + streaming | PR-16 | medium | Phase 12 |
| 14 | PR-17 Database hardening | Escape + pool + scan + QueryRowsStream | PR-17 | high | — |
| 15 | PR-18 Sync hardening | Whitelist + decrypt guard + canonical | PR-18 | high | — |
| 16 | PR-19 K8s perf | Auth retry + watch reconnect + transport | PR-19 | medium | — |
| 17 | PR-14 Session perf | SSH buffer + mosh + FTP + output_log | PR-14 | medium | — |
| 18 | PR-13 App core | Startup errors + foreground + emit + memo | PR-13 | med–high | — |
| 19 | PR-12 LLM streaming | http.Client + SSE types + atomic.Pointer | PR-12 | medium | Phase 18 |
| 20 | PR-20 Dev ergonomics | pprof + observer leaks + timer dispose | PR-20 | low | — |
| 21 | PR-21 Hotfix bundle | chat JSON + local deadlock + comment trim | PR-21 | high | Phase 17, 19 |

## Phase Details

### Phase 1: PR-01 — Terminal render-compat for Claude Code

**Goal:** Claude Code TUI renders contiguous box-drawing borders; brace-highlight SGR doesn't corrupt fenced code blocks; U+FFFD replacement chars stripped from live + history paths.

**Branch:** `pr/terminal-claude-render-compat`

**Commits:**
- `f3cb20d` drop U+FFFD replacement chars in live and history paths
- `8a8e44a` preserve box-drawing and braille chars in history restore
- `c07c19e` use U+FFFD escape in U+FFFD strip regex
- `1b93fc1` skip brace highlight inside code fences
- `5f3edba` skip brace highlight across full code-fence blocks

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] vitest `useHighlight.test.ts` covers fenced/tilde/info-string/indented cases
- [ ] visual: scrolling Claude Code session shows contiguous box-drawing borders

**Excluded:** none.

---

### Phase 2: PR-02 — Bundle JetBrains Mono Variable

**Goal:** New users see JetBrains Mono Variable at first launch; OFL-1.1 attribution bundled.

**Branch:** `pr/terminal-jetbrains-mono`

**Commits:**
- `b8c6753` docs: bundle OFL-1.1 attribution for JetBrains Mono
- `96c0e85` feat(terminal): bundle JetBrains Mono Variable font
- `8cbe771` feat(terminal): prefer JetBrains Mono Variable in default font stack

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] `THIRD_PARTY_NOTICES.md` cites OFL-1.1
- [ ] new users see JetBrains Mono Variable at first launch (existing users unchanged)

---

### Phase 3: PR-04 — xterm polish

**Goal:** Stable selection re-read, clipboard fallback when Wails returns false, addon dispose on unmount, cell-width memoization, full-viewport refresh after resize, Unicode 11 widths active.

**Branch:** `pr/terminal-xterm-polish`

**Commits:**
- `0e1c6f7` trust fitAddon.cols/rows to keep cell grid aligned
- `83ee94a` activate Unicode 11 widths at terminal creation
- `688a99d` set Unicode 11 activeVersion after loading the addon
- `83daa38` F-021 memoize cellWidth/cellHeight in resize
- `ab77a6e` F-020 dispose xterm addons on unmount
- `290a135` F-022 scan from cursor row first when extracting command
- `c214b67` F-023 use char-array for lineBuffer to avoid per-char concat
- `e6e34b9` full-viewport refresh after resize to clear canvas residue
- `1a7b8dd` re-read xterm selection at right-click menu click
- `06fafb2` fall back to navigator.clipboard when Wails returns false

**Excluded:** `567e1e4`, `4bb5afa` (covered by upstream #303).

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] right-click copy works when Wails returns false
- [ ] no canvas residue after rapid resize during spinner burst

---

### Phase 4: PR-03 — Terminal sizing + DeferConnect

**Goal:** Backend PTY seeded with frontend-measured cols/rows on first paint; duplicate tab and reconnect paths also use measured dims.

**Branch:** `pr/terminal-sizing-defer-connect`

**Commits:**
- `0f137a6` seed backend PTY with frontend-measured cols/rows
- `66d137d` wire DeferConnect into retry and duplicate connect paths
- `6c82592` chore(terminal): regenerate wailsjs bindings and bump deps

**Acceptance:**
- [ ] `go build` + `npm --prefix frontend run build` pass
- [ ] opening local / SSH / Mosh terminal first paint matches xterm-measured cols/rows
- [ ] duplicate tab and reconnect paths also use measured dims

**Note:** if upstream rejects dep bumps, split into 3a (code) + 3b (deps).

---

### Phase 5: PR-05 — Document xterm v5.5 / 6.0 gaps

**Goal:** Reviewers know which xterm features are intentionally deferred.

**Branch:** `pr/frontend-doc-xterm-v5-gaps`

**Commits:**
- `b679447` F-036 document italic handling is native to xterm v5.5
- `3bf3871` F-035 document xterm v5.5 limits for charSizeCompat / codeBlockBackground
- `6a2d8d8` F-038 document xterm mode-2004 / 1000/1006/1015 verification gap
- `e3965d9` F-037 document DEC mode 2026 sync gap in xterm v5.5

**Acceptance:**
- [ ] comments land in correct files
- [ ] no behavior change

---

### Phase 6: PR-06 — Bump @xterm/xterm to 6.0.0

**Goal:** Stay on current xterm; existing vitest useTerminal cases pass.

**Branch:** `pr/deps-xterm-6`

**Commits:**
- `4124a2c` chore(deps): bump @xterm/xterm to 6.0.0 + addons

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] existing vitest useTerminal tests green

---

### Phase 7: PR-07 — Terminal themes (Soft Gray + Win11)

**Goal:** New light-gray theme (Soft Gray) + Windows 11 terminal themes that match Win Terminal look; OSC-11 background query returns accurate color for TUI theme detection.

**Branch:** `pr/terminal-themes-softgray-win11`

**Commits:**
- `4f72c10` feat(terminal-theme): add uniterm Soft Gray (light gray, ANSI tuned)
- `ba7ad53` feat(theme): strengthen Win11 blue+white + add uniterm Windows 11 terminal
- `fe513f8` feat(terminal-theme): add uniterm Windows 11 Light (matches UI base color)
- `2ac5953` fix(terminal-theme): make xterm background accurate for TUI theme detection

**Acceptance:**
- [ ] all 3 new themes render correctly (vitest covers Soft Gray, Windows 11, Windows 11 Light, transparent-background, css-var)
- [ ] Claude Code picks light-mode colors when Soft Gray is active (OSC-11 test)

---

### Phase 8: PR-08 — UI themes (Win11 + macOS26)

**Goal:** Windows 11 + macOS 26 users see native-looking chrome via CSS variables and component overrides.

**Branch:** `pr/ui-themes-win11-macos26`

**Commits:**
- `9194f4f` feat(theme): add Win11 CSS variables (light + dark via @media)
- `a475568` feat(theme): add macOS26 CSS variables (light + dark via @media)
- `a2e6a9d` feat(theme): Win11 component overrides
- `d473940` feat(theme): macOS26 component overrides
- `5778ae6` fix(theme): add dark-mode scrollbar thumb overrides for Win11/macOS26
- `06b6534` fix(theme): make Win11/macOS26 controls visually distinct
- `f082688` feat(theme): register Win11/macOS26 UI themes in picker

**Excluded:** `24d9f63` (no App.go changes affecting it).

**Acceptance:**
- [ ] Win11 + macOS26 themes selectable in Settings + Sidebar across all 9 locales
- [ ] visual: Win11 Fluent buttons (32px / 4px radius / inset border); macOS26 capsule (30px / 14px)
- [ ] dark variants show white-tinted scrollbar thumbs

---

### Phase 9: PR-09 — Frontend micro-optimizations

**Goal:** Eliminate main-thread stalls during resize-drag, slider scrubbing, regex recompilation in hot path.

**Branch:** `pr/frontend-micro-optimizations`

**Commits:** (16 total)
- `5c86049` F-024 coalesce cursor position updates via rAF
- `933e275` F-025 precompute terminal theme dark/light partitions
- `d4ebc13` F-029 single-pass sanitize regex
- `6d4bb57` F-028 chunked base64 in toBase64
- `02d4c62` F-040 select clipboard writer once at init
- `6a8ea6e` F-041 cache --wails-draggable lookups per element
- `02318bb` F-042 bound focusPanelTerminal retry with backoff
- `75ca8ff` F-031 rAF-coalesce resizeObserver debounce
- `d6b2575` F-033 drop forced full-viewport refresh in resize()
- `d49c602` F-032 watch terminal settings keys individually, debounce resize
- `455f298` F-032 rAF-coalesce session:data → terminal.write
- `71093ac` F-030 hoist hot-path regexes to module scope
- `0d4775d` F-019 sessionStore 256KB ring + dispose on close
- `6db3878` F-027 shrink scrollback in holding container
- `90d4ca9` F-026 drop xterm scrollback on KeepAlive deactivation

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] no behavior change (visual + functional parity)
- [ ] manual: dragging fontSize slider shows no per-tick watcher churn

---

### Phase 10: PR-10 — aiStore perf

**Goal:** 660-line conversation transform doesn't re-run on every per-token mutation; eager _rawApiMsg parsing at startup doesn't blow up cold start.

**Branch:** `pr/aistore-perf`

**Commits:**
- `ae19266` F-301 memoize aiStore.conversation via version counter
- `fcf9ad6` F-302 shallowReactive + markRaw _rawApiMsg in aiStore
- `2d6feb1` F-310 cache estimateMessageTokens per AIMessage
- `621674f` F-313 fold conversation builder into two passes
- `bf84a8f` F-314 cache _rawApiMsg JSON serialization in aiStore
- `1bcfaab` F-316 lazy parse _rawApiMsg on session switch
- `bbf586a` F-304 debounce aiStore.doSave by 500ms

**Acceptance:**
- [ ] `npm --prefix frontend run build` passes
- [ ] fast-model streaming (≥100 tok/s) caps AISidebar re-renders at ~60 Hz
- [ ] cold start with 15 saved sessions is O(1) parses for non-active sessions

---

### Phase 11: PR-11 — AI agent hardening (security)

**Goal:** XSS via v-html blocked; model cannot bypass risk enum or tool validation; module-level listener leaks across re-entry guarded; per-token coalesce; tool result blobs capped.

**Branch:** `pr/ai-agent-hardening`

**Commits:**
- `b38968d` fix(ai): sanitize markdown output before v-html binding
- `23396b6` fix(ai-agent): strict risk enum + tool input validation
- `753d37b` F-315 generation-counter guard for module-level listener
- `5824eae` F-311 rAF-coalesce SSE token writes to assistant message
- `4c29aa3` F-312 cap tool result blobs at write time

**Acceptance:**
- [ ] `AIMessage.sanitize.test.ts` covers javascript:/data:/script/iframe/object/embed removal
- [ ] `agent.risk.test.ts` + `agent.toolInput.test.ts` pass
- [ ] fast-model streaming caps re-renders at ~60 Hz
- [ ] `capToolResult` leaves content unchanged under threshold; truncates with marker above

**Risk:** medium (security-critical).

---

### Phase 12: PR-15 — Store hardening (security)

**Goal:** Power-loss / kill -9 can't leave 0-byte user data; concurrent Save can't tear JSON; skills symlink traversal blocked; AES-GCM AAD prevents ciphertext cross-file swap; keychain missing returns error rather than writing plaintext.

**Branch:** `pr/store-hardening-security`

**Commits:**
- `18c77c2` fix(store): atomic write + per-store mutex + skills symlink guard
- `3ff7109` fix(sync): mutex + isConfigDirEmpty + drop hostname leak + AES-GCM AAD
- `701294d` defensive Close on terminal_history — bounded + idempotent
- `60d3ad2` F-106 commands cache (mtime invalidation)
- `46de8bc` F-111 commands SaveCommand atomic write

**Acceptance:**
- [ ] `go test ./backend/store/...` green
- [ ] regression test: kill -9 mid-save leaves file intact (or original unchanged)
- [ ] regression test: skills `Delete` refuses to traverse symlinks
- [ ] regression test: missing keychain `Save` returns error rather than writing plaintext

**Risk:** high (security-critical).

---

### Phase 13: PR-16 — Store perf

**Goal:** Eliminate per-call full-file os.WriteFile; shard ai-session; cache List() results; streaming encoder for settings; lock-release-before-IO for settings Load; async keychain backfill.

**Branch:** `pr/store-perf-debounce-cache`

**Commits:**
- `f76715a` F-101 debounce + atomic terminal_history writes
- `2c995da` F-102 debounce recent.Record
- `fc5658f` F-103 shard ai_session by id (with one-time migration)
- `d4cdf25` F-105 connection_store encoder + no-op hash skip
- `e8506cb` F-108 settings Save uses streaming json.Encoder
- `241e68c` F-109 settings Load releases lock before disk I/O
- `5ff4252` F-110 async keychain backfill + cache
- `cebea43` F-112 skills copyDir uses os.CopyFS
- `3bd83bd` test(store): F-107 verify SkillsStore.List cache works

**Acceptance:**
- [ ] `go test ./backend/store/...` green
- [ ] migration path from legacy ai-sessions.json to per-id shard is idempotent + reversible

**Depends on:** Phase 12 (uses atomicWriteFile + writeJSONLocked helpers).

---

### Phase 14: PR-17 — Database hardening + perf (security)

**Goal:** SQL injection via synced connection profiles blocked; MySQL USE + DDL session-state race fixed; Postgres sslmode=disable removed; queries honor 30s timeout; per-cell fmt.Sprintf eliminated.

**Branch:** `pr/database-hardening-perf`

**Commits:**
- `561a7c6` fix(database): escape identifiers via SafeMyIdent / SafePgIdent
- `7905655` fix(database): harden pool race, query timeout, sqlserver cleanup
- `3eb3e34` fix(database): F-116 mysql connection pool tuning
- `2714092` F-115 Postgres GetTableSchema parallel + cached
- `80ccfd7` F-114 scanToString type-switch
- `2216281` F-113 add QueryRowsStream to avoid per-row map alloc

**Acceptance:**
- [ ] `go test ./backend/database/...` green
- [ ] `safeident_test.go` covers backtick/double-quote/nul/path-traversal/length-cap payloads + DEFAULT literal whitelist
- [ ] `ExecuteQuery`/`ExecuteStatement` honor 30s context deadline

**Risk:** high (security-critical).

---

### Phase 15: PR-18 — Sync hardening (security + perf)

**Goal:** Sync repo only stages 7 whitelisted files; wrong-password sync aborts; ChangePassword rewrites salt + re-encrypts every repo file; canonical JSON compare avoids spurious commits; sync no longer blocks app startup; shared http.Client with conditional GET.

**Branch:** `pr/sync-hardening-perf`

**Commits:**
- `d43f275` fix(sync): stage whitelist + decrypt-fail guard + ChangePassword salt + JSON canonical compare
- `084e588` perf(sync): F-407 defer NewSyncService to goroutine via NewSyncServiceAsync + Ready()
- `9f8f41e` perf(update): F-409 reuse shared http.Client + conditional GET via ETag/304

**Acceptance:**
- [ ] sync repo only stages the 7 whitelisted files
- [ ] wrong-password sync aborts with `lastSyncStatus="password_mismatch"` (no push)
- [ ] ChangePassword rewrites `.sync-salt` and re-encrypts every repo file
- [ ] `go test ./backend/sync/...` green

**Risk:** high (security + data-integrity).

---

### Phase 16: PR-19 — K8s perf + hardening

**Goal:** exec-plugin kubeconfigs work on first use and after token rotation; apiserver restart doesn't silently kill watch/log streams; giant CRD OOMs prevented; per-stream ParseBytes / every-emit Map allocations eliminated.

**Branch:** `pr/k8s-perf-hardening`

**Commits:**
- `0b31751` fix(k8s): authRoundTripper clones request + retries once on 401
- `6dc0b07` fix(k8s): watch & log reconnect with backoff + cap REST response body
- `be0a5dd` F-403 tune http.Transport with keep-alive, idle conns, response-header timeout
- `70297c7` F-404 split onEnd (terminal) from onReconnect (transient) for watch+log
- `c948c54` F-405 cache ParseBytes + F-413 atomic emit + sweeper for stale handles
- `ef6c728` F-406 gate body preview on env var + F-411 default 5min deadline
- `d4ae5b9` F-412 shrink Scanner initial buffer 64K → 4K for watch+log streams

**Acceptance:**
- [ ] `go test ./backend/k8s/...` green; `TestWatchDeliversEvents` + `TestLogStreamDeliversLines` honor the original onEnd-once contract
- [ ] REST body limited to 64 MiB
- [ ] onEnd split from onReconnect so transient failures don't trigger terminal end

**Risk:** medium.

---

### Phase 17: PR-14 — Session perf + FTP TLS toggle

**Goal:** Per-call 4K readBuf allocs eliminated; SSH encoder cached; mosh readLoop 1s timeout; emitData single mutex; output_log buffered writes; FTP connMu; FTP TLS verify opt-in toggle.

**Branch:** `pr/session-perf-hardening`

**Commits:**
- `a9902b4` F-001 + F-004 SSH read buffer reuse + 16K size
- `288509c` F-002 SSH decodeOutput scratch buffer
- `cc3e2c3` F-003 SSH Write cached encoder
- `50d8b1c` F-005 + F-006 local PTY 16K readBuf + reuse
- `d2a90aa` F-007 serial 16K readBuf + reuse
- `b0f3f4c` F-008 telnet 16K readBuf + filter in handler
- `3eb3e34` fix(database): F-116 mysql connection pool tuning — **excluded; moved to PR 17**
- `feb4d0d` F-018 manager.sessions force-evict + startup validation
- `ef77752` F-044 SSH keepalive 60s -> 90s
- `a25bf08` F-009 + F-010 — mosh readLoop: 1s timeout, no alloc copy
- `3cff7e2` F-013 — explicit Unlock paths in WriteOutput
- `abd2baa` F-015 — pre-size lineProcessor.Feed output buffer
- `abd9041` F-012 — log filename collision in one syscall
- `543ac33` F-011 — event-driven flush loop, no idle 1 Hz wakeup
- `46eaf35` F-016 + F-017 — single mutex in emitData, channel signal in waitIdle
- `6b623ad` output_log buffered writes + FTP TLS verify toggle
- `27def5c` FTP connMu on Disconnect/ChangeRemoteDir + proxy Stop doc

**Closes:** #415, #424.

**Acceptance:**
- [ ] `go test ./backend/session/...` green
- [ ] FTP connMu covers Disconnect + ChangeRemoteDir paths
- [ ] output_log writes no longer call Sync per write
- [ ] FTP `InsecureSkipVerify` opt-in is gated by `ftpSkipVerify` config; default `false`

**Risk:** medium.

---

### Phase 18: PR-13 — App core observability + perf

**Goal:** Nil-store panic wedge on startup prevented; per-keystroke full-blob emit eliminated; O(n) panel lookups eliminated; unbounded session poll cost eliminated.

**Branch:** `pr/app-core-observability-perf`

**Commits:**
- `8544e8d` surface startup-init errors instead of leaving stores nil
- `9296584` F-043 foreground flag + WindowIsMinimised poll
- `a69a0fc` F-206 close moveResizeCh in shutdown so the emitter goroutine exits
- `34022e0` F-205 typed struct + pooled buffer for session:data emit
- `48ffe4e` F-204 emit connection deltas instead of full store on Save
- `6a157b0` F-211 async SaveConnections/SaveTunnels/SaveQuickCommands
- `6591233` F-212 O(1) panel->session inverse index for output-log lookups
- `6ec8976` F-213 memoize ListSessions so polling skips unchanged snapshots
- `415204e` F-203 partial — coalesce concurrent SyncNow calls
- `0d7d3c4` chore(wailsjs): regenerate bindings for IsForeground / SetAppVisibility

**Acceptance:**
- [ ] build green; emit `app:startup-error` on init failure; `StartupError()` queryable from frontend
- [ ] foreground flag drives pause/resume for keepalive / output_log flush / k8s watches / auto-sync / AI SSE
- [ ] SaveConnections emits `store:connections:delta` for adds/removes; full-blob only on reload-after-sync

**Risk:** medium–high.

---

### Phase 19: PR-12 — LLM streaming perf + typed SSE

**Goal:** Back-to-back ChatCompletion calls reuse single TCP+TLS conn; double-marshal on Anthropic final eliminated; unbounded error-body reads bounded; per-call map[string]interface{} replaced with typed structs.

**Branch:** `pr/llm-streaming-perf`

**Commits:**
- `1a90f80` F-208 shared http.Client with keep-alive for LLM + FetchModels
- `518a39f` F-307 bytes.Buffer per block for text/input accumulation
- `b7e024c` F-306 typed SSE shapes for OpenAI Chat Completions stream
- `2712321` F-306 typed SSE envelope for Anthropic stream
- `bdd4b3b` F-210 marshal Anthropic final message once via pooled buffer
- `ebecd3c` F-210 also marshal final message once on OpenAI + Responses paths
- `bfc02bd` F-305 cap error-body reads to 64 KiB via io.LimitReader
- `a05869f` F-320 typed aiTokenEvent struct on all 3 chat paths
- `31429d6` F-320 typed ai:* event payloads across all 3 chat paths
- `8c18c73` F-308 atomic.Pointer[CancelFunc] so CancelChatStream survives overlap
- `6465c1a` F-319 cache JSON prefix + cache_control on last tool
- `f5cebf6` fix(ai): close missing } in chat() request body after F-319 prefix optimization — **excluded; moved to PR-21**

**Acceptance:**
- [ ] `go build` + `npm --prefix frontend run build` pass
- [ ] back-to-back ChatCompletion calls reuse single TCP+TLS conn
- [ ] Anthropic + OpenAI + Responses paths share final-message marshal helper

**Risk:** medium (large app.go surface).

---

### Phase 20: PR-20 — Dev ergonomics + frontend cleanup

**Goal:** Dev builds expose pprof for hot-path diagnosis; module-level setInterval / MutationObserver / EventsOn handlers don't leak across remounts/HMR; LocalStateStore.Load doesn't block app startup on slow disk.

**Branch:** `pr/dev-ergonomics-frontend-cleanup`

**Commits:**
- `8ca4efe` fix(main): F-201 expose net/http/pprof listener on localhost:6060 in dev builds
- `f7fe6c3` fix(frontend): FE-03 EventsOn/EventsOff pairing + FE-02 AISidebar observer leak
- `e60cdd9` fix(update-check): stop timer on app teardown
- `a3885b7` perf(main): F-410 race LocalStateStore.Load() with 100ms timeout, default to frameless
- `5adc230` fix(terminal-agent): F-317 O(1) title→panelId index for resolveActiveSession

**Excluded:** `902dae6` (chore: gitignore — internal).

**Acceptance:**
- [ ] pprof listener only opens when `Version == "dev"` (-ldflags)
- [ ] onUnmounted in App.vue disconnects both AISidebar observers + disposes every store
- [ ] useUpdateCheck.dispose() called on beforeunload

**Risk:** low.

---

### Phase 21: PR-21 — Hotfix bundle

**Goal:** `chat()` no longer emits unterminated JSON body; `Disconnect` no longer self-deadlocks on local PTY teardown; session data no longer double-JSON-encoded by F-205 regression.

**Branch:** `pr/hotfix-chat-json-local-terminal-deadlock`

**Commits:**
- `f5cebf6` fix(ai): close missing } in chat() request body after F-319 prefix optimization
- `ca647a8` fix(session): local terminal deadlocks on close + no output on new tab
- `fd4e9df` refactor(session): trim comments added in ca647a8 per CLAUDE.md guidance

**Closes:** #312, #418, #424 (partial).

**Acceptance:**
- [ ] `go test ./backend/session/...` green; `TestLocalSessionConnectDisconnectNoDeadlock` + `TestLocalSessionWaitGoroutineDoesNotCallDisconnect` pass
- [ ] new local terminal tab shows output immediately on first paint
- [ ] CloseAll on shutdown returns within seconds, not forever

**Risk:** high (regression hotfix). **Depends on:** Phase 17 (PR-14 session work), Phase 19 (PR-12 F-319 + F-205 paths).

---

## Per-Phase Workflow

每个 phase 都按下列流程执行（在 worktree 隔离环境）：

1. **Setup worktree** — `git worktree add .worktrees/pr-XX-YY origin/main -b pr/XX-YY`
2. **Cherry-pick** — `git cherry-pick <commit1> <commit2> ...` (按 ROADMAP.md 列出的顺序)
3. **Strip trailers** — `git filter-branch -f --msg-filter 'sed -E "/^Co-Authored-By: Claude.*$/d"'` 或逐 commit `git commit --amend --no-edit`
4. **Substantive review** — 走 PR 描述的 Acceptance 清单，逐项人工/AI review
5. **Add/update tests** — 对每个非 trivial 改动添加单元测试
6. **Refactor** — 清理 dead code、合并重复、统一命名
7. **Build gate** — `npm --prefix frontend run build` + `go test ./backend/...`
8. **Push branch** — `git push 我的 pr/XX-YY:pr/XX-YY`
9. **Create PR** — `gh pr create --repo ys-ll/uniterm --base main --head coderstory:pr/XX-YY --title "..." --body-file /tmp/pr-XX-body.md`
10. **Mark complete** — update `.planning/PROJECT.md` and `.planning/STATE.md`

## Issue References Summary

Only these upstream issues are referenced (per user decision):

| Issue | PR(s) that close it |
|---|---|
| #288 (后台 tab 输出截断) | PR-09 (F-019/026/027) |
| #312 (UEFI shell display) | PR-04 (Unicode 11 widths + box-drawing), PR-21 |
| #415 (SFTP/SSH 不操作断连) | PR-14 (F-044 + SSH buffer) |
| #418 (claude code 错位) | PR-04 (lineHeight + windowsMode + char preservation), PR-21 |
| #424 (docker 容器连接) | PR-14 (deadlock + force-evict), PR-21 |

All other PRs are unprefixed — they describe motivation in the body, no `Closes #N`.

## Quality Gate (per PR)

Before any `gh pr create`:

- [ ] `npm --prefix frontend run build` exits 0
- [ ] `go test ./backend/...` exits 0
- [ ] No `Co-Authored-By: Claude ...` trailer in any commit on the branch
- [ ] At least one new/modified unit test in the diff
- [ ] Code review notes captured in `.planning/phases/NN-PR-XX/review.md`
- [ ] No new TODOs / FIXMEs left in production code
- [ ] PR body has: 1-line title, 3-5 sentence context, "What changed" bullets, specific test plan
- [ ] Branch pushed to `coderstory/uniterm`
- [ ] `gh pr create` succeeds; PR URL captured

## Coverage

- **100% of 21 Active PRs** mapped to phases 1–21.
- **5 strong-match upstream issues** mapped to relevant PRs.
- **16 unprefixed PRs** with body motivation only.
- **12 docs(planning) commits** + **2 batch commits** (a4f0b96, 609ecc1) + **2 flash commits** (567e1e4, 4bb5afa) + **main.go macOS Edit** → excluded per Out of Scope.

Last updated: 2026-07-283