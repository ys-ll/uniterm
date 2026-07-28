# uniterm — v1.0 PR Submission Requirements

## Active

### Category: PR Submission (上游 PR 提交)

每个 PR 必须满足：
- 只解决一个上游 issue 或一个独立主题
- 通过本地 `npm --prefix frontend run build` 和 `go test ./backend/...` 全绿
- 包含至少一个新增/修改的单元测试
- 经过实质性 review（不只是格式化）
- `git log` 中已 strip 所有 `Co-Authored-By: Claude ...` trailer

### Active PRs (21 total)

#### Foundational (2)

- [ ] **PR-01**: Submit `pr/terminal-claude-render-compat` — terminal render-compat (U+FFFD strip, code-fence brace highlight, box-drawing/braille preservation). Refs: none. Commits: 5. Risk: low. Depends on: none.
- [ ] **PR-02**: Submit `pr/terminal-jetbrains-mono` — bundle JetBrains Mono Variable font + prefer in default stack + OFL-1.1 attribution. Refs: none. Commits: 3. Risk: low.

#### Terminal polish (3)

- [ ] **PR-03**: Submit `pr/terminal-sizing-defer-connect` — seed backend PTY with frontend-measured cols/rows + DeferConnect + deps bump. Refs: none. Commits: 3. Risk: medium. Note: includes dep bumps; if upstream rejects, split into 3a/3b.
- [ ] **PR-04**: Submit `pr/terminal-xterm-polish` — selection re-read, clipboard fallback, addon dispose, cell measurement, resize refresh, Unicode 11 widths, brace command extraction. Refs: none. Commits: 10. Risk: low. **Excludes**: `567e1e4` + `4bb5afa` (covered by upstream #303).
- [ ] **PR-05**: Submit `pr/frontend-doc-xterm-v5-gaps` — comment-only doc for xterm v5.5 / 6.0 known gaps. Refs: none. Commits: 4. Risk: low (docs only).
- [ ] **PR-06**: Submit `pr/deps-xterm-6` — bump `@xterm/xterm` to 6.0.0 + addons. Refs: none. Commits: 1. Risk: low. Depends on: PR 1, 2, 4.

#### Themes (2)

- [ ] **PR-07**: Submit `pr/terminal-themes-softgray-win11` — Soft Gray + uniterm Windows 11 + Windows 11 Light + accurate xterm background. Refs: none. Commits: 4. Risk: low.
- [ ] **PR-08**: Submit `pr/ui-themes-win11-macos26` — Win11 + macOS26 CSS variables, component overrides, picker registration. Refs: none. Commits: 8. Risk: low–medium.

#### Frontend perf (1)

- [ ] **PR-09**: Submit `pr/frontend-micro-optimizations` — rAF coalescing, regex caching, sanitizer single-pass, clipboard writer init, focus retry backoff, scrollback bounds, KeepAlive deactivation. Refs: none. Commits: 16. Risk: low.

#### AI/LLM perf (2)

- [ ] **PR-10**: Submit `pr/aistore-perf` — aiStore memoization, shallowReactive, _rawApiMsg lazy parse + JSON cache, doSave debounce. Refs: none. Commits: 7. Risk: low.
- [ ] **PR-12**: Submit `pr/llm-streaming-perf` — shared http.Client, typed SSE shapes (OpenAI + Anthropic + Responses), pooled buffer for final message marshal, atomic.Pointer for CancelChatStream, error-body cap, JSON prefix cache + cache_control. **Closes**: #288 (claude-code memory pressure at idle — but better mapping is #418; coordinate). Commits: 12. Risk: medium.

#### AI/Agent hardening (1) — security-critical

- [ ] **PR-11**: Submit `pr/ai-agent-hardening` — markdown sanitize before v-html, strict risk enum + tool input validation, generation-counter guard for module-level listener, rAF-coalesce SSE token writes, cap tool result blobs. Refs: none. Commits: 5. Risk: medium (security). **Lands before PR 12.**

#### App core (1)

- [ ] **PR-13**: Submit `pr/app-core-observability-perf` — surface startup-init errors, foreground flag, emit optimizations (delta events, typed structs, pooled buffers), async Saves, panel→session inverse index, ListSessions memo, SyncNow coalesce. Refs: none. Commits: 10. Risk: medium–high.

#### Session hardening (1)

- [ ] **PR-14**: Submit `pr/session-perf-hardening` — SSH/Local/Serial/Telnet read buffer reuse 16K, encoder cache, mosh 1s timeout, lineProcessor pre-size, ansi stripper cap, emitData single mutex, output_log buffered writes, FTP TLS verify toggle, FTP connMu, log filename collision. **Closes**: #415 (SFTP/SSH reconnect), #424 (docker container connect). Commits: 20. Risk: medium.

#### Store hardening (2)

- [ ] **PR-15**: Submit `pr/store-hardening-security` — atomic write + per-store mutex + skills symlink guard + AES-GCM AAD + defensive Close + commands cache mtime + commands atomic Save. **Closes**: #88 (ssh compile log interrupted), #61 (SFTP directory freeze — partial). Commits: 5. Risk: high (security-critical). **Lands before PR 16.**
- [ ] **PR-16**: Submit `pr/store-perf-debounce-cache` — debounce terminal_history writes, shard ai_session by id (one-time migration), connection_store streaming encoder + no-op hash skip, settings streaming encoder + lock-release-before-IO, async keychain backfill + cache, os.CopyFS. Refs: none. Commits: 9. Risk: medium (migration one-way). Depends on: PR 15.

#### Database (1) — security-critical

- [ ] **PR-17**: Submit `pr/database-hardening-perf` — identifier escape via SafeMyIdent/SafePgIdent (5 P0 SQL-injection findings), pool race + query timeout + sqlserver cleanup, MySQL pool tuning, Postgres GetTableSchema parallel + cached, scanToString type-switch, QueryRowsStream. **Closes**: #123 (database functionality — but weak mapping, internal perf). Commits: 6. Risk: high (security). **Note**: combine security + perf in one PR per user decision.

#### Sync (1) — security-critical

- [ ] **PR-18**: Submit `pr/sync-hardening-perf` — stage whitelist (7 files), decrypt-fail guard, ChangePassword salt + .sync-salt rewrite, JSON canonical compare, AES-GCM AAD, mutex on ResolveConflict + isConfigDirEmpty, drop hostname leak, NewSyncServiceAsync + Ready, shared http.Client + ETag/304. Refs: none. Commits: 4. Risk: high (security + data integrity).

#### K8s (1)

- [ ] **PR-19**: Submit `pr/k8s-perf-hardening` — authRoundTripper clones + retries on 401, watch + log reconnect with backoff + 64MiB REST body cap, transport tuning, onEnd/onReconnect split, ParseBytes cache + atomic emit + sweeper, body-preview env gate + 5min deadline, Scanner 4K initial buffer. Refs: none. Commits: 7. Risk: medium.

#### Dev ergonomics (1)

- [ ] **PR-20**: Submit `pr/dev-ergonomics-frontend-cleanup` — pprof listener on localhost:6060 (dev builds only), EventsOn/EventsOff pairing, AISidebar observer leak fix, update-check timer dispose, LocalStateStore.Load race with 100ms timeout + frameless default, terminal-agent O(1) title→panelId. Refs: none. Commits: 6. Risk: low.

#### Hotfix bundle (1) — must land last

- [ ] **PR-21**: Submit `pr/hotfix-chat-json-local-terminal-deadlock` — close missing `}` in chat() request body (F-319 regression), local terminal deadlock on close + no output on new tab, comment trim per CLAUDE.md. **Closes**: #312 (UEFI shell display — partial via Unicode 11 widths; full mapping is unicode/braille), #418 (claude code misalignment — lineHeight + windowsMode). Commits: 3. Risk: high (regression hotfix). Depends on: PR 12 (F-319 + F-205 paths), PR 14 (session work).

### Issue-to-PR Mapping (5 strong matches)

| Upstream issue | PR(s) | Mapping strength |
|---|---|---|
| #288 (后台 tab 输出截断) | PR 10, PR 9 (F-019/026/027) | strong |
| #312 (UEFI shell display) | PR 4 (Unicode 11 widths + box-drawing), PR 21 (unicode guard) | strong |
| #415 (SFTP 不操作断连) | PR 14 (F-044 keepalive + SSH buffer) | strong |
| #418 (claude code 错位) | PR 4 (lineHeight + windowsMode + char preservation), PR 21 | strong |
| #424 (docker 容器连接) | PR 14 (deadlock + force-evict), PR 21 (deadlock fix) | strong |

## Future

(已识别但本期不提交的 commit / PR)

- **FUTURE-XTERM-7**: xterm 7.x 大版本升级（依赖官方 release + 重大 API 变更）
- **FUTURE-WAILS-3**: Wails v3 迁移（依赖官方 stable）
- **FUTURE-RDP**: RDP 协议相关改进（fork 没动 RDP）
- **FUTURE-EXPORT-IMPORT**: 连接 / 配置导出导入 UI（issue #378）
- **FUTURE-SIDEBAR-STATUS**: 左侧主机列表状态标识（issue #326）
- **FUTURE-EDITOR-ENHANCE**: 内置编辑器查找替换 / undo / 高亮（issue #173）

## Out of Scope

下列 commit **永不**进入任何 PR：

- `644ff94` `chore: remove .planning/` — internal cleanup, not a feature
- `071515f`, `7ca34cc`, `299a8cd`, `142f84b`, `231636b`, `856cd01`, `02d880d`, `3c8cf13`, `a3161ad`, `66c40d2`, `b6d30cd`, `4c2f8e7`, `b648be4`, `43e814f` — `docs(planning): *` (12 个 internal GSD 规划产物)
- `4809abd`, `8393ce8` — `docs:` CLAUDE.md 刷新 + codebase map (internal docs)
- `a4f0b96` (`fix(session): batch 3`), `609ecc1` (`fix(session,k8s,frontend): batch 5`) — superseded by per-F-* PRs; verify no behavior delta before final exclusion
- `567e1e4` (`fix(ui): kill black→white flash`), `4bb5afa` (`fix(ui): snap scroll + ship flash fix`) — covered by upstream #303 (linux menubar); drop from PR 4
- All `Co-Authored-By: Claude ...` trailers — stripped at commit time

## Traceability

> Filled by ROADMAP.md after roadmap creation. Each requirement maps to exactly one phase.

Last updated: 2026-07-28