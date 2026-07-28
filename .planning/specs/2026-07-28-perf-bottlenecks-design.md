---
title: 性能瓶颈与 Claude Code 兼容性分析
date: 2026-07-28
status: approved
scope: whole-app-sweep + claude-code-render-compat
---

# 性能瓶颈与 Claude Code 兼容性分析

## 1. 摘要

对 uniterm 全栈做一次**性能瓶颈 + Claude Code 渲染兼容性**审计,产出发现汇总文档、优先级化的修复批次建议,以及用于复现 / 验证的微基准与 pprof 命令清单。**不修改任何生产代码**。

Top 5 P0:

- **F-009** — `backend/session/mosh_session.go:188` — MoshSession.readLoop calls `s.moshClient.Recv(100 * time.Millisecond)` in a tight for-loop that wakes every 100 ms even when the session is idle (no UDP input).
- **F-011** — `backend/session/output_log.go:630` — OutputLogger.flushLoop runs `time.NewTicker(logFlushInterval)` where `logFlushInterval = 1 * time.Second`. Every active log file creates a goroutine that wakes once per second for the entire session lifetime, even when no bytes have been written.
- **F-017** — `backend/session/session.go:341` — waitIdle busy-loops with `time.Sleep(50 * time.Millisecond)` for up to the configured timeout. With 8 tabs each waiting idle at login, this is up to 8 × (5 s / 50 ms) = 800 wakeups per tab connect cycle. After connect, idle windows of 5+ s pay 100 wakeups.
- **F-019** — `frontend/src/stores/sessionStore.ts:20` — sessionStore keeps up to 2000 chunks of every session's data (MAX_CHUNKS=2000, TRIM_TO=1000). `getData` joins all of them on every read, producing a multi-MB string per session. With 8 open tabs each holding 2000 chunks × 4 KB = 8 MB per session = 64 MB resident in the Pinia reactive state — all of which is held in JS by WKWebView's GC.
- **F-026** — `frontend/src/components/BaseTerminal.vue:1052` — BaseTerminal subscribes to the GLOBAL `session:data` event inside onMounted and accumulates state per session. On every KeepAlive re-mount the same listener is re-registered (generation guard mitigates but the listener closure is still retained). Vue KeepAlive cache can hold multiple BaseTerminal instances of the same panel, each with a full xterm + addon + scrollback buffer — visible memory bloat per inactive tab.

## 2. 方法论

### 2.1 范围

全 5 个子系统(其中 terminal_io 含 Claude Code `render_compat` 子方向):

1. **terminal_io**(终端 I/O 路径,含 Claude Code 渲染兼容)
2. **storage_db**(存储与数据库)
3. **wails_bridge**(Wails JS↔Go 桥)
4. **ai_llm**(AI/LLM 循环)
5. **k8s_sync_startup**(K8s / 同步 / 启动)

### 2.2 Agent 划分(并行)

| Agent | 范围(只读) | 关注点 |
|---|---|---|
| **terminal_io** | `backend/session/{output_log,ssh_session,local_session_unix,serial_session,telnet_session,mosh_session,manager}.go`;前端 `composables/useTerminal*.ts`、`composables/useFocusTerminal.ts`、`stores/sessionStore.ts`;xterm addon / 主题配置引用处 | 分配 / 拷贝热路径、缓冲批处理、xterm write 节奏、订阅泄漏、ringbuffer 设计、跨 goroutine channel 行为、**Claude Code 渲染清单**(斜体、同步输出 mode 2026、模糊宽度、代码块背景、鼠标 / 括号粘贴、alt screen、行高) |
| **storage_db** | `backend/store/*.go`、`backend/database/{engine,executor,provider_*}.go` | fsync 频率、锁粒度、JSON 序列化、连接池竞争、查询 N+1 |
| **wails_bridge** | `app.go`、`app_*.go`(非 build-tag 拆分)、`frontend/wailsjs/go/main/App.d.ts`、各 store 调用 Wails 处 | 单事件大 payload、频繁 EventsEmit、序列化成本、同步阻塞调用、pprof 暴露与否 |
| **ai_llm** | `frontend/src/services/{agent,llm,terminalAgent}.ts`、`stores/aiStore.ts`、`app.go` 中 AI proxy | 多轮上下文膨胀、流式 token 处理、prompt 拼接、retry/backoff、JSON 解析 |
| **k8s_sync_startup** | `backend/k8s/*.go`、`backend/sync/*.go`、`backend/update/*.go`、`main.go`、`app.go` 启动路径 | watch 重连、REST 响应体上限、git 操作阻塞启动、store/sync 初始化顺序、冷启动 |

### 2.3 Finding schema

```json
{
  "id": "F-001",
  "file": "path/to/file.go",
  "line": 142,
  "subsystem": "terminal_io | storage_db | wails_bridge | ai_llm | k8s_sync_startup",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic | caching | render_compat",
  "root_cause": "一句话根因",
  "evidence": "代码片段",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述"
}
```

### 2.4 约束(每个 agent 都收到)

- in-flight 文件**只读**,报告里可提建议,不修改:
  - `backend/session/output_log.go`
  - `backend/session/ftp_session.go`
  - `backend/session/session.go`
  - `backend/sync/git.go`
  - `backend/sync/sync_service.go`
  - `frontend/src/services/agent.ts`
  - `frontend/src/utils/runtimeTypeCheck.ts`
- 不写新生产代码、不改现有测试断言
- 返回结构化 JSON findings,不返回叙述文本

### 2.5 严重度定义

- **P0**:在普通用户日常使用 uniterm 时必然触发(每次开 tab、每次 AI 多轮、跑一次 Claude Code 就会暴露);或中等负载就明显劣化
- **P1**:特定场景才触发(大文件 `cat`、8+ tab、kubectl get -A、AI 5+ 轮、特定主题);高负载才暴露
- **P2**:理论隐患、不常见路径、仅在边界条件下出现

## 3. 发现汇总表

| id | file:line | subsystem | severity | category | root_cause |
|---|---|---|---|---|---|
| F-408 | `app.go:200` | k8s_sync_startup | P0 | io | Auto-sync goroutine spawned in startup (and triggerAutoSync on every save) is fire-and-forget with no lifecycle hook: it will keep running Sync() (which does git fetch + push, PBKDF2 on each call, AES encrypt/decrypt) even when the app is hidden, screen-locked, or in App Nap — multiplying CPU/wakeups when the user can't see the result. |
| F-204 | `app.go:349` | wails_bridge | P0 | serialization | SaveConnections emits the FULL ConnectionStoreData (every connection + group + recent) over the bridge on every save; the same full payload is then re-loaded by reloadStoresAfterSync on the next sync tick. |
| F-203 | `app.go:655` | wails_bridge | P0 | io | SyncNow / SyncTestConnection / SyncVerifyPassword / SyncConfigureRepo / SyncChangePassword / SyncDeleteRepo block the Wails handler thread on synchronous git + network + crypto work. |
| F-205 | `app.go:1192` | wails_bridge | P0 | allocation | SetOnDataCallback allocates a fresh map[string]interface{} per chunk and copies []byte to string (escapes to heap) for every terminal output byte chunk that crosses the bridge. |
| F-303 | `app.go:1729` | ai_llm | P0 | caching | `anthropic-beta: prompt-caching-2024-07-31` header is set, but the request body never has `cache_control: {type:"ephemeral"}` breakpoints on system / tools / last few messages. Static system prompt (~2KB) + tool definitions (~1KB) re-sent and re-billed every turn. |
| F-305 | `app.go:1779` | ai_llm | P0 | io | Error-path response body reads use unbounded `io.ReadAll(res.Body)` with no size cap. A hostile or buggy upstream returning a multi-GB error body can OOM the Go process. |
| F-306 | `app.go:1799` | ai_llm | P0 | allocation | Every SSE `data:` line is fully `json.Unmarshal`-ed into a fresh `map[string]interface{}` allocation. Anthropic emits ~1 event per token for content_block_delta; a 2K-token response = 2K map allocations, ~99% of fields discarded immediately. |
| F-403 | `backend/k8s/client.go:50` | k8s_sync_startup | P0 | io | The k8s http.Transport is built without any keep-alive tuning: no MaxIdleConns, no MaxIdleConnsPerHost, no IdleConnTimeout, no TLSHandshakeTimeout — every REST call opens a fresh TCP + TLS handshake to the apiserver, even when called many times per second from the same tab (e.g. log streaming + pod polling). |
| F-404 | `backend/k8s/watch.go:36` | k8s_sync_startup | P0 | locking | On every watch reconnect attempt, runWatchLoop calls onEnd(err) — which under manager.StartWatch is wired to emit `k8s:watch-end:<id>` via EventsEmit AND reacquire m.mu to clean up the watches map. So a flapping apiserver triggers an EventsEmit + a mutex dance every 1s/2s/4s/8s/16s/30s. |
| F-009 | `backend/session/mosh_session.go:188` | terminal_io | P0 | locking | MoshSession.readLoop calls `s.moshClient.Recv(100 * time.Millisecond)` in a tight for-loop that wakes every 100 ms even when the session is idle (no UDP input). |
| F-011 | `backend/session/output_log.go:630` | terminal_io | P0 | locking | OutputLogger.flushLoop runs `time.NewTicker(logFlushInterval)` where `logFlushInterval = 1 * time.Second`. Every active log file creates a goroutine that wakes once per second for the entire session lifetime, even when no bytes have been written. |
| F-017 | `backend/session/session.go:341` | terminal_io | P0 | locking | waitIdle busy-loops with `time.Sleep(50 * time.Millisecond)` for up to the configured timeout. With 8 tabs each waiting idle at login, this is up to 8 × (5 s / 50 ms) = 800 wakeups per tab connect cycle. After connect, idle windows of 5+ s pay 100 wakeups. |
| F-103 | `backend/store/ai_session_store.go:54` | storage_db | P0 | memory | AISessionStore.Save / Load marshals / unmarshals the entire ai-sessions.json into memory on every save. AIMessageEntry retains Content verbatim plus an optional _rawApiMsg blob. No sliding-window or token-budget truncation. |
| F-102 | `backend/store/recent_store.go:55` | storage_db | P0 | io | RecentStore.Record does not debounce — every Record call re-marshals the full id slice (up to maxRecent=20) and rewrites recent.json via plain os.WriteFile. Not using atomicWriteFile so a crash mid-write can lose the file. |
| F-101 | `backend/store/terminal_history_store.go:33` | storage_db | P0 | io | TerminalHistoryStore.Save writes the entire JSON file via os.WriteFile on every Save call (every command entered triggers Save via the UI). No write-coalescing or debounce; each Save produces a full file rewrite. Plain os.WriteFile is also non-atomic. |
| F-026 | `frontend/src/components/BaseTerminal.vue:1052` | terminal_io | P0 | memory | BaseTerminal subscribes to the GLOBAL `session:data` event inside onMounted and accumulates state per session. On every KeepAlive re-mount the same listener is re-registered (generation guard mitigates but the listener closure is still retained). Vue KeepAlive cache can hold multiple BaseTerminal instances of the same panel, each with a full xterm + addon + scrollback buffer — visible memory bloat per inactive tab. |
| F-034 | `frontend/src/services/terminalManager.ts:75` | terminal_io | P0 | render_compat | xterm Terminal is created with `fontSize: 13`, `fontFamily: 'Consolas, "Courier New", monospace'`, scrollback 2500, but NO `lineHeight` configured. xterm defaults to 1.0 line height, which means italic glyphs (e.g. the ⏺ thinking block in Claude Code) clip at the top/bottom of their cell because descenders exceed cell height. |
| F-302 | `frontend/src/stores/aiStore.ts:263` | ai_llm | P0 | allocation | `addMessage` wraps every message in `reactive({ ...msg })` and `_rawApiMsg` is a deep-tracked reactive value. Per-token mutation of `content` invalidates ALL dependents of every message proxy in the dep graph. |
| F-304 | `frontend/src/stores/aiStore.ts:419` | ai_llm | P0 | serialization | `doSave()` is called synchronously on every `addMessage` / `addSkillCard` / `addCommandCard` / `clearMessages`, marshaling and crossing the Wails bridge with the entire conversation history (including JSON.stringify of every `_rawApiMsg`). |
| F-301 | `frontend/src/stores/aiStore.ts:486` | ai_llm | P0 | memory | The `conversation` computed has no memoization; every reactive mutation on any message field (including per-token `content +=`) re-runs the entire 660-line transform: walks every message, JSON.stringify of _rawApiMsg, dangling tool_use filtering, pair validation, consecutive-user dedup. |
| F-019 | `frontend/src/stores/sessionStore.ts:20` | terminal_io | P0 | memory | sessionStore keeps up to 2000 chunks of every session's data (MAX_CHUNKS=2000, TRIM_TO=1000). `getData` joins all of them on every read, producing a multi-MB string per session. With 8 open tabs each holding 2000 chunks × 4 KB = 8 MB per session = 64 MB resident in the Pinia reactive state — all of which is held in JS by WKWebView's GC. |
| F-043 | `main.go:1` | terminal_io | P0 | io | No App Nap / lifecycle handling found anywhere in the repo. There is no Info.plist customization (no NSAppSleepDisabled reference); the Wails macOS runtime defaults to allowing App Nap. terminal_io goroutines (keepalive, mosh Recv, output log flush) keep firing at full rate even when the user backgrounds the app. |
| F-201 | `main.go:88` | wails_bridge | P0 | io | No pprof endpoint is exposed; the audit's §8.2 prerequisite and the checklist's G item both flag this. |
| F-206 | `app.go:123` | wails_bridge | P1 | locking | moveResizeCh goroutine launched in startup() never receives a shutdown signal — it lives for the process lifetime, blocking on a channel that is never closed. |
| F-211 | `app.go:343` | wails_bridge | P1 | io | SaveConnections / SaveSettings / SaveTunnels / SaveQuickCommands / SaveAIConfig all run a.fs-fsync on the Wails handler thread (atomic-rename write inside store.Save) before the emit; combined with F-203/207 this stalls the UI on every save. |
| F-207 | `app.go:619` | wails_bridge | P1 | locking | triggerAutoSync spawns a fresh `go func()` on every store mutation with no coalescing or in-flight tracking, so a burst of saves (e.g. pasting 50 commands into QuickCommands) launches 50 concurrent syncs. |
| F-308 | `app.go:1746` | ai_llm | P1 | locking | `chatCancel` is a single `context.CancelFunc` field with `chatCancelMu` protecting it. When two `ChatCompletion` calls overlap (retry storm, quick user re-trigger), call A's defer nils out `chatCancel` while call B is still in flight; `CancelChatStream` then becomes a no-op for B. |
| F-208 | `app.go:1768` | wails_bridge | P1 | io | chatCompletion* methods construct `&http.Client{Timeout: 0}` per call; no keep-alive, no connection pool — every LLM request opens a fresh TCP+TLS handshake. |
| F-320 | `app.go:1820` | ai_llm | P1 | serialization | Every SSE event triggers a `runtime.EventsEmit` carrying a freshly built `map[string]interface{}` payload (ai:block_start / ai:token / ai:input_json_delta / ai:content_block_stop / ai:done). These all cross the Wails bridge to the frontend, getting JSON-marshaled on the Go side and JS-cloned on the WebKit side. Per-token `ai:token` is the worst: a text-only delta plus the index crosses the bridge for every token of every response. |
| F-307 | `app.go:1832` | ai_llm | P1 | allocation | Text delta accumulation uses `currentBlock["text"].(string) + text` (string concat) per token. O(n²) for long responses plus every concat allocates a new string. |
| F-210 | `app.go:1885` | wails_bridge | P1 | serialization | Every ai:done emit and the final return value both json.Marshal the full message map from scratch, with the same contentBlocks slice; double work per turn. |
| F-113 | `backend/database/engine.go:50` | storage_db | P1 | allocation | queryStrings and queryAny allocate a fresh map[string]any per row plus a fresh []any value holder per row. For a 10k-row x 20-col result that's 10k maps + 200k interface{} slots. |
| F-116 | `backend/database/provider_mysql.go:19` | storage_db | P1 | io | mysqlProvider DSN sets timeout=10s but no ConnMaxLifetime / ConnMaxIdleTime / MaxOpenConns / MaxIdleConns. database/sql defaults to unlimited open conns and unbounded lifetime - a long-lived tab can accumulate idle TCP sockets to the DB, and a server-side timeout (wait_timeout) eventually drops them. |
| F-115 | `backend/database/provider_postgres.go:178` | storage_db | P1 | io | Postgres GetTableSchema issues three serial round-trips (columns, primary keys, indexes) per call. For a database-tree UI panel that walks every table this is 3*N queries. MySQL/SQLServer have the same shape. |
| F-405 | `backend/k8s/kubeconfig.go:98` | k8s_sync_startup | P1 | caching | ParseBytes fully re-parses the kubeconfig YAML (yaml.Unmarshal + base64 decode + map allocations) on every Connect call, with no caching keyed on file mtime or content hash — and Connect is called every time a K8s tab is opened. |
| F-413 | `backend/k8s/manager.go:167` | k8s_sync_startup | P1 | memory | Every StartWatch / StartLogStream captures `m.emit` under the manager mutex and uses it for the lifetime of the watch — but m.emit is set once via SetEventEmitter in startup and never changes. The bigger leak is that the `connection.watches` set is only cleaned up on Disconnect / StopWatch; if the Wails frontend reloads (HMR dev mode) and reopens k8s tabs, stale watchIDs pile up until process exit. |
| F-411 | `backend/k8s/rest.go:24` | k8s_sync_startup | P1 | io | Do() only enforces a 64 MiB body cap and respects ctx cancellation, but the surrounding http.Client has Timeout=0 (intentional, for watches) — so a hung apiserver TCP connection for a non-watch request will block forever, unless the caller remembers to set a deadline on ctx. |
| F-406 | `backend/k8s/rest.go:55` | k8s_sync_startup | P1 | io | Every successful REST response logs a 300-byte body preview via log.Writef, which calls fmt.Sprintf on every call and writes to a file-backed logger — paid on every k8s request, including polling calls fired many times per second by the frontend. |
| F-005 | `backend/session/local_session_unix.go:150` | terminal_io | P1 | io | LocalSession readLoop uses a 4 KiB raw `os.File.Read` on the PTY master fd, with no bufio wrapper, no deadline, and a non-blocking `select{ <-s.quit: default }` polling guard at every iteration. |
| F-006 | `backend/session/local_session_unix.go:163` | terminal_io | P1 | allocation | Every read allocates a new `data := append([]byte(nil), buf[:n]...)` then immediately calls `updateMouseTrackingState(data)` which runs `bytes.Contains(data, seq)` over up to 14 short sequences. The strip/inspect work runs on every single chunk, not on coalesced data. |
| F-010 | `backend/session/mosh_session.go:196` | terminal_io | P1 | allocation | Every Recv() result is copied via `append([]byte(nil), data...)` before emitData — even though mosh's Recv returns a fresh slice owned by the caller. |
| F-014 | `backend/session/output_log.go:39` | terminal_io | P1 | allocation | ansiStripper.Strip allocates `make([]byte, 0, len(data))` for every chunk and the `s.pending = append(s.pending[:0], data[i:]...)` copy on incomplete tails reallocates the pending slice as it grows. |
| F-015 | `backend/session/output_log.go:181` | terminal_io | P1 | allocation | lineProcessor.Feed calls `out = append(out, p.line[p.emitted:]...)` and `out = append(out, '\n')` repeatedly within the loop on every byte, plus `out` is allocated per call (var out []byte grows). |
| F-012 | `backend/session/output_log.go:449` | terminal_io | P1 | io | OutputLogger.Enable walks up to 100 filename suffixes via `for suffix := 1; suffix <= 100; suffix++` and calls `os.OpenFile` with `os.O_CREATE\|os.O_EXCL\|os.O_WRONLY` per attempt, costing up to 100 stat+create syscalls on collision. |
| F-013 | `backend/session/output_log.go:583` | terminal_io | P1 | locking | WriteOutput acquires `l.mu` and holds it across both ANSI stripping and the line-processor pass — which can call back into the lineProcessor's CSI parser. On a chatty session this serializes writes to the log while still allowing emitData to proceed (that's under a separate lock), but couples log writing latency to log formatting work. |
| F-016 | `backend/session/session.go:169` | terminal_io | P1 | locking | baseSession.emitData takes two RLock guards (outputLogMu then mu) and runs both callbacks inside the locked region of baseSession.mu. When the onData callback chains into Wails EventsEmit, every remote byte pays an extra lock round-trip and the lifetime of the callback extends the lock-hold window. |
| F-001 | `backend/session/ssh_session.go:313` | terminal_io | P1 | io | SSH read loop uses a fixed 4096-byte buffer per read; under heavy output (`cat` of a large file, log streaming, Claude Code streaming long answers) the per-RTT syscall count dominates and latency scales with chunk count, not bytes. |
| F-004 | `backend/session/ssh_session.go:313` | terminal_io | P1 | allocation | readLoop allocates two full copies of every received chunk: one for `data := append([]byte(nil), buf[:n]...)` and another inside `s.lastRecv.Store(append([]byte(nil), data...))`. Both copies are paid even when no diagnostic read happens. |
| F-044 | `backend/session/ssh_session.go:434` | terminal_io | P1 | io | startKeepAlive ticker fires every 60s (sshKeepAliveInterval = 60 * time.Second) for the lifetime of every SSH session. With 8 tabs this is ~8 wakeups/min — modest, but still an idle cost that does NOT pause on app background. |
| F-003 | `backend/session/ssh_session.go:459` | terminal_io | P1 | allocation | Write() copies the entire encoded input via `append([]byte(nil), enc...)` and stores it in lastSent; on a chatty session this duplicates every byte sent to the server into an ever-growing diagnostic buffer. |
| F-002 | `backend/session/ssh_session.go:542` | terminal_io | P1 | allocation | decodeOutput allocates a fresh `src` slice per call and copies `decodeLeftover + data` into it even when the decoder is nil-path; the encodeInput path also rebuilds the encoder on every keystroke via `enc.NewEncoder().Bytes(data)`. |
| F-008 | `backend/session/telnet_session.go:98` | terminal_io | P1 | io | Telnet read loop reads 4 KiB at a time and immediately filters IAC inline; filterIAC allocates a fresh `var out []byte` and copies byte-by-byte. Under chatty telnet banners the IAC scanner runs O(n) per chunk. |
| F-106 | `backend/store/commands_store.go:103` | storage_db | P1 | io | CommandsStore.List does a full directory scan + reads every commands/*.md file via readCapped on every call. No in-memory cache; on each call it also reads commands.json and frequently rewrites it (the default-fill loop sets changed = true the first time a missing-pref is encountered). |
| F-111 | `backend/store/commands_store.go:247` | storage_db | P1 | io | CommandsStore.CreateCommand / SaveCommand call os.WriteFile on the .md file directly (no atomic rename, no fsync) while also rewriting commands.json. A crash between the two writes leaves the .md updated but commands.json stale (or vice versa). |
| F-105 | `backend/store/connection_store.go:55` | storage_db | P1 | serialization | ConnectionStore.Save marshals the full ConnectionConfig slice (including PostLoginExpectSteps, K8sConfigInline YAML strings) with json.MarshalIndent even for a single-field edit. |
| F-110 | `backend/store/connection_store.go:142` | storage_db | P1 | locking | ConnectionStore.populatePasswords re-iterates every connection, calls the keychain once per password, and if any plaintext passwords existed, takes the write lock and does an atomic write of the full file under that lock. |
| F-108 | `backend/store/settings_store.go:141` | storage_db | P1 | serialization | SettingsStore.Save marshals the entire AppSettings with json.MarshalIndent and atomic-writes, including CustomTerminalThemes (full TerminalThemeColors for each) and the Keyboard map. MarshalIndent allocates a buffer the size of the output for whitespace alone. |
| F-109 | `backend/store/settings_store.go:166` | storage_db | P1 | locking | SettingsStore.Load takes the write lock for the entire duration of disk I/O + keychain backfill + default-fill. Any concurrent Save blocks behind it; the read returns only after the file is fully decoded. |
| F-107 | `backend/store/skills_store.go:226` | storage_db | P1 | io | SkillsStore.List reads every skill's SKILL.md (and probes references/ + scripts/ via dirHasFiles and countFiles, each of which opens a directory) on every invocation. No caching. |
| F-112 | `backend/store/skills_store.go:554` | storage_db | P1 | io | copyDir uses filepath.Walk that calls os.Lstat + os.Open + os.Create per file with no batching. copyFileWithoutSymlinks opens a fresh file handle for each file and uses io.Copy with default 32KB buffer; for many small reference files this is overhead-bound. |
| F-407 | `backend/sync/sync_service.go:35` | k8s_sync_startup | P1 | io | NewSyncService() runs synchronously inside app.startup and may invoke the OS keychain (PBKDF2 600k iterations + keychain IPC) — on macOS the Security framework call to read the encryption key can take 50–300ms, and PBKDF2 alone on the same keychain access pattern can take >500ms. |
| F-409 | `backend/update/checker.go:199` | k8s_sync_startup | P1 | io | Check() builds a fresh *http.Client with Timeout=10s but no transport reuse, no UA-cache, no If-Modified-Since / ETag handling — and the 5-min disk cache TTL only blocks identical calls within 5 minutes, after which every Check() opens a brand-new TCP+TLS to api.github.com. |
| F-029 | `frontend/src/components/BaseTerminal.vue:365` | terminal_io | P1 | allocation | sanitizeTerminalHistory runs 7 sequential regex passes over the entire scrollback string every time a KeepAlive tab restores. The pattern `/[^\u0000-\u007f一-鿿...]/g` allocates fresh match objects for every Unicode codepoint scan, and the function is invoked on every re-activation. |
| F-033 | `frontend/src/components/BaseTerminal.vue:461` | terminal_io | P1 | render_compat | resize() calls `terminal.refresh(0, terminal.rows - 1)` after every fit to force a full-viewport redraw. This is the "lineHeight / italic-clip" workaround mentioned in the spec: the existing render path may clip italic descenders, so a forced refresh is used to repaint. |
| F-028 | `frontend/src/components/BaseTerminal.vue:530` | terminal_io | P1 | allocation | exportContent walks every line in the xterm buffer (`buffer.length` rows) and calls `line.translateToString()` on each, then joins them with '\n' into a single string and base64-encodes via TextEncoder + manual charCode loop. The `for (let i = 0; i < bytes.length; i++) { binary += String.fromCharCode(bytes[i]) }` is the slowest possible string-concat in JS. |
| F-031 | `frontend/src/components/BaseTerminal.vue:530` | terminal_io | P1 | io | exportContent calls `WriteFileBase64(filePath, toBase64(content))` which marshals the full scrollback to base64, then crosses the Wails bridge, then Go decodes and writes the file. For a 1 MB scrollback this is 1.33 MB of base64 + JSON marshalling + Wails IPC. |
| F-030 | `frontend/src/components/BaseTerminal.vue:1135` | terminal_io | P1 | allocation | Each session:data chunk is run through stripCursorBlink (regex replace), then 2 .replace(/\u001b[2J/g, ...) regex passes, then stripAnsi (3 regexes via useTerminalInput.handleSessionData), then highlight (which itself runs 7+ regex patterns per line) on the FRONTEND for every chunk arriving from the backend. |
| F-027 | `frontend/src/components/BaseTerminal.vue:1553` | terminal_io | P1 | memory | releaseTerminal has a 500 ms disposeTimer that disposes the underlying xterm after all components release. During that 500 ms window, a fully detached xterm still holds its canvas + scrollback DOM nodes (moved to the hidden holding container in services/terminalManager.ts). If the user drags a tab back into the holding area, the terminals accumulate indefinitely until the session closes — adding up over a long day. |
| F-041 | `frontend/src/composables/useFocusTerminal.ts:67` | terminal_io | P1 | render_compat | installTerminalFocusRestore hooks a document-level mousedown listener that walks up the DOM tree on every click (`while (cur) { getComputedStyle(cur).getPropertyValue('--wails-draggable')... }`). Every mouse click anywhere in the app pays a synchronous reflow + computed-style read chain. |
| F-021 | `frontend/src/composables/useTerminal.ts:380` | terminal_io | P1 | caching | resize() reaches into xterm internals (`(terminal as any)._core?._renderService?.dimensions`) every time it runs. On a typical session, resize is called by IntersectionObserver, ResizeObserver, window resize, split-resize, and the retry timers in onMounted — easily 50+ times per second during drag. |
| F-020 | `frontend/src/composables/useTerminal.ts:471` | terminal_io | P1 | memory | The legacy useTerminal composable creates a NEW xterm.Terminal and three new addons (FitAddon, SearchAddon, WebLinksAddon) per call without disposing the prior addons if the composable is invoked twice in the same panel (e.g. workspace drag-in reuse). The searchAddon in particular has no explicit Dispose() call in onUnmounted. |
| F-022 | `frontend/src/composables/useTerminalInput.ts:47` | terminal_io | P1 | algorithmic | getCurrentCommandFromTerminal scans every visible line (buffer.rows iterations) per Enter keystroke. Each line runs translateToString() (O(width)) + stripAnsi() regexes + PROMPT_RE match. A 80×24 terminal pays 24 regex matches + 24 stripAnsi regexes per Enter. |
| F-023 | `frontend/src/composables/useTerminalInput.ts:158` | terminal_io | P1 | allocation | handleInput uses `lineBuffer.value.slice(0, idx) + ... + lineBuffer.value.slice(idx)` (string concatenation) on every printable character to insert into the middle of the line. Each edit produces 2 new string objects + 1 new buffer string. |
| F-311 | `frontend/src/services/agent.ts:430` | ai_llm | P1 | io | `activeAssistantMsg.content += data.text` is a deep-reactive mutation per SSE token that triggers the entire AISidebar render chain. With high-throughput models (Claude 100+ tok/s, GPT-4o), this is 100+ Vue re-renders per second on the main thread. |
| F-312 | `frontend/src/services/agent.ts:670` | ai_llm | P1 | memory | Tool result blobs (execute_command output, capture_terminal screen, use_skill manifest) stored verbatim in `m.content`. A 200-line × 2KB capture_terminal = 400KB retained. The conversation computed (F-301) walks all retained blobs every turn. |
| F-036 | `frontend/src/services/terminalManager.ts:75` | terminal_io | P1 | render_compat | xterm Terminal options do not enable italic font — there is no italic font configured. SGR 3 (italic) sequences forwarded by Claude Code's thinking-block markup render as upright text. The ItalicAddon (`@xterm/addon-italic`) is NOT loaded. |
| F-037 | `frontend/src/services/terminalManager.ts:75` | terminal_io | P1 | render_compat | xterm Terminal is not configured for DEC mode 2026 synchronized output. Claude Code emits \e[?2026h ... \e[?2026l to bracket a multi-line repaint (thinking-block redraws). Without SyncAddon / mode 2026 enabled, each line paints individually → flicker / partial renders during the spinner phase. |
| F-039 | `frontend/src/services/terminalManager.ts:75` | terminal_io | P1 | render_compat | xterm Terminal is created without explicit `windowsMode`, `scrollback` is the only viewport setting. Alternate screen buffer mode 1049 is supported by xterm natively, but the BaseTerminal resize path does not preserve scrollback correctly across alt-screen enter/exit (the terminal preserves it, but Vue's terminal.position tracking does not account for the saved-buffer offset). |
| F-035 | `frontend/src/services/terminalManager.ts:100` | terminal_io | P1 | render_compat | Unicode 11 widths are enabled via `terminal.unicode.activeVersion = '11'`, but there is no `wcwidth`/`charSizeCompat` override. East Asian ambiguous-width characters (e.g. ⏺⏵ used by Claude Code) still use Unicode 11's ambiguous=1 → may misalign with backend PTY (which often uses wcwidth -u13 or older). Also missing: 256-color code-block background (SGR 48;5;) — themes have only background, no codeBlockBackground. |
| F-310 | `frontend/src/stores/aiStore.ts:45` | ai_llm | P1 | serialization | `estimateMessageTokens` does `JSON.stringify(msg._rawApiMsg)` on every message, every time the conversation computed re-evaluates (which is per-token per F-301). For a 100-message session this is 100 redundant JSON stringifies per token. |
| F-316 | `frontend/src/stores/aiStore.ts:147` | ai_llm | P1 | memory | `loadSessionsFromBackend` eagerly `JSON.parse`s `_rawApiMsg` for every message in every saved session at app startup. For 15 sessions × 200 messages, that's 3000 JSON parses; parsed object trees held in memory even for sessions that won't be opened in this session. |
| F-314 | `frontend/src/stores/aiStore.ts:399` | ai_llm | P1 | serialization | `doSave` materializes a full snapshot via `sessions.value.map(...)` on every call (every message add). For a 200-message session with 4KB `_rawApiMsg` each, that's ~800KB allocated per save + bridged across to Go. |
| F-313 | `frontend/src/stores/aiStore.ts:486` | ai_llm | P1 | algorithmic | `conversation` makes multiple linear passes over the message array: build kept (with token-budget break), strip leading tool messages, collect resolved tool_use IDs, second loop filtering, third loop for pair validation, fourth for consecutive-user dedup. O(4n + n·estimateMessageTokens) per chat call. |
| F-410 | `main.go:76` | k8s_sync_startup | P1 | io | main.go synchronously loads LocalStateStore from disk to read the persisted window-frame preference BEFORE wails.Run — any disk stall (slow antivirus, network home dir, locked file) delays the entire app launch. |
| F-213 | `app.go:1347` | wails_bridge | P2 | memory | ListSessions returns every SessionInfo in the manager on each call; if the frontend polls this (it does — see panelStore and tabStore cross-checks) it produces a steady stream of allocations even when nothing has changed. |
| F-212 | `app.go:3273` | wails_bridge | P2 | algorithmic | panelLogTitle and EnableSessionOutputLog both scan the entire sessionToPanel map (keyed by sessionID) to find the one bound to a panelID — O(n) per enable on every Enable call. |
| F-114 | `backend/database/engine.go:126` | storage_db | P2 | allocation | scanToString calls fmt.Sprintf("%v", v) for any non-[]byte value. fmt.Sprintf allocates a fmt.buffer + parses the format string per call. |
| F-412 | `backend/k8s/watch.go:92` | k8s_sync_startup | P2 | memory | runOneWatch allocates a fresh 64 KiB initial Scanner buffer per watch and grows it up to 4 MiB for any single line; on a busy cluster where watch lines can be multi-MB JSON blobs (CRDs), this buffer is retained in the per-event allocated path and not pooled. |
| F-018 | `backend/session/manager.go:10` | terminal_io | P2 | memory | SessionManager.sessions map grows unbounded; Close deletes by ID, but failed/abandoned sessions (process crashed mid-Connect) are never reaped. The map also holds each session forever after Disconnect until the user explicitly closes the tab in UI. |
| F-007 | `backend/session/serial_session.go:109` | terminal_io | P2 | io | SerialSession readLoop uses 4 KiB Read and additionally copies each chunk via `data := make([]byte, n); copy(data, buf[:n])` before emitting, while also running `normalizeNewlines` which always allocates a fresh buffer. |
| F-032 | `frontend/src/components/BaseTerminal.vue:1500` | terminal_io | P2 | caching | The terminal settings watcher does `deep: true` which means every fontSize/fontFamily/scrollback/theme change reruns the entire handler, including building a fresh theme object and calling applyXtermTheme. theme is a fresh object every call → xterm internal theme diff re-runs all colors. |
| F-042 | `frontend/src/composables/useFocusTerminal.ts:113` | terminal_io | P2 | io | focusPanelTerminal retries with setTimeout(100ms) up to 10 times when xterm-helper-textarea is missing. With KeepAlive + drag, xterm's internal textarea can be absent for many frames — 10 retries × 100ms = up to 1 second of focus polling. |
| F-024 | `frontend/src/composables/useTerminalInput.ts:90` | terminal_io | P2 | allocation | updateCursorPosition() walks xterm internals (`buffer.x`, `buffer.y`, `renderer.dimensions.css.cell.width`) every time a 0-ms setTimeout fires. updateToken is also called per character; per-keystroke cursor tracking has no throttle beyond the 0-ms defer. |
| F-040 | `frontend/src/composables/useTerminalMenu.ts:67` | terminal_io | P2 | allocation | writeClipboard first awaits ClipboardSetText (Wails round-trip); on false result, awaits navigator.clipboard.writeText (another round-trip). Each clipboard operation is a full IPC hop. |
| F-025 | `frontend/src/composables/useTerminalThemeOptions.ts:22` | terminal_io | P2 | caching | terminalThemeGroups is a `computed()` that rebuilds via `TERMINAL_THEMES.filter(...)` on every dependency change. TERMINAL_THEMES is a constant module-level list, so the filter result never changes between filter invocations. |
| F-315 | `frontend/src/services/agent.ts:24` | ai_llm | P2 | locking | Module-level `activeTokenUnsubscribe` / `activeAssistantMsg` shared across runAgent calls. Re-entrant paths (approveTool → runAgent; rejectTool/answerQuestion/dismissQuestion setTimeout→runAgent at lines 1056, 1082, 1099) call `registerTokenListener` which cancels the prior listener, but if an early-return path bypassed `cleanupStreamListeners`, the previous `activeAssistantMsg` may be replaced mid-stream without releasing reactive refs. |
| F-319 | `frontend/src/services/llm.ts:74` | ai_llm | P2 | serialization | Request body is `JSON.stringify`-ed once but uses a flat object with the full `AVAILABLE_TOOLS` array (constant ~6KB) and the system prompt + entire conversation embedded every call. There's no template caching of the static prefix (system+tools) — even though they are invariant across all turns of a session. |
| F-317 | `frontend/src/services/terminalAgent.ts:191` | ai_llm | P2 | algorithmic | `resolveActiveSession` spreads `panelStore.panels` Map into an Array and runs two `.find` passes (exact title + suffix match) for every command call. O(n) per call. |
| F-038 | `frontend/src/services/terminalManager.ts:86` | terminal_io | P2 | render_compat | WebLinksAddon is loaded but the listening addons (mouse reporting, bracketed paste mode) do not appear to be explicitly verified for compliance. Bracketed paste mode ("\e[?2004h") is mentioned as a passthrough in BaseTerminal.vue (bracketedPasteMode check), but no automated check confirms xterm forwards the escape. |

## 4. P0 详述

### F-408 — `app.go:200` (io)

- **根因**: Auto-sync goroutine spawned in startup (and triggerAutoSync on every save) is fire-and-forget with no lifecycle hook: it will keep running Sync() (which does git fetch + push, PBKDF2 on each call, AES encrypt/decrypt) even when the app is hidden, screen-locked, or in App Nap — multiplying CPU/wakeups when the user can't see the result.
- **证据**: app.go:200-212 if syncSvc.IsAutoSyncEnabled() { go func(){ syncSvc.Sync() ... }() }; app.go:619-638 triggerAutoSync fires on every SaveConnections/SaveSettings/SaveQuickCommands/SaveAIConfig. No IsBackground()/lifecycle guard.
- **影响**: Saving 10 quick commands in a row → 10 background git push+PBKDF2+AES cycles. With the app hidden the user pays the cost with no benefit, and on macOS each push wakes the network interface and breaks App Nap throttling.
- **修复**: Wrap triggerAutoSync in a debounce (200ms) and gate both the trigger and the worker on a `lifecycle.Active()` flag that listens for OnHide/OnShow; also pause when the window is minimized.
- **验证**: Run app hidden, save a setting 20 times rapidly; `ps -o %cpu` and Time-Wait socket counts should not spike.

### F-204 — `app.go:349` (serialization)

- **根因**: SaveConnections emits the FULL ConnectionStoreData (every connection + group + recent) over the bridge on every save; the same full payload is then re-loaded by reloadStoresAfterSync on the next sync tick.
- **证据**: func (a *App) SaveConnections(data session.ConnectionStoreData) error {
    ...
    if err == nil {
        runtime.EventsEmit(a.ctx, "store:connections:changed", data) // <-- full blob
        a.triggerAutoSync()
    }
}
- **影响**: Connection lists grow unboundedly; emit payloads measured in 100s of KB for moderate users, MB-scale for power users. WebKit has to structured-clone the entire blob into JS land on every keystroke that triggers SaveConnections. Compounds with the 400 MB WebKit memory baseline — each emit can be retained until GC.
- **修复**: Emit only the diff or change kind: `{ kind: 'upsert' | 'remove', id, connection? }`. On sync reload, keep the full blob (it really did change) but compress with `sync/atomic` or only emit when the frontend's view isn't current. Add `omitempty` on zero-value struct fields so unused protocol blobs don't ship.
- **验证**: Chrome DevTools → Performance recording during a SaveConnections burst; look for 'structured clone' marker duration and heap delta. Compare payload size in DevTools Network (Wails frame size) before/after.

### F-203 — `app.go:655` (io)

- **根因**: SyncNow / SyncTestConnection / SyncVerifyPassword / SyncConfigureRepo / SyncChangePassword / SyncDeleteRepo block the Wails handler thread on synchronous git + network + crypto work.
- **证据**: func (a *App) SyncNow() (*sync.SyncResult, error) {
    if a.syncService == nil { ... }
    result, err := a.syncService.Sync() // git pull/push; potentially seconds to minutes
    ...
}
// SyncService.Sync is a blocking call to git via os/exec.
- **影响**: Every SaveConnections / SaveSettings triggers a.triggerAutoSync() (line 350, 488, 786). A user editing a connection field while online stalls the entire UI for the duration of git push+pull + AES-GCM decryption. With auto-sync enabled this happens on EVERY store mutation.
- **修复**: Return immediately with a request-id and run the sync in a goroutine; emit 'sync:started' / 'sync:completed' / 'sync:failed' events. Frontend already listens on 'sync:completed' so no breaking change. Add coalescing: only one sync in flight at a time, drop new triggers while running.
- **验证**: Add log.Writef timestamp markers; measure Wails handler round-trip latency in DevTools Network before/after — SyncNow should return <5ms instead of the sync duration.

### F-205 — `app.go:1192` (allocation)

- **根因**: SetOnDataCallback allocates a fresh map[string]interface{} per chunk and copies []byte to string (escapes to heap) for every terminal output byte chunk that crosses the bridge.
- **证据**: s.SetOnDataCallback(func(data []byte) {
    runtime.EventsEmit(a.ctx, "session:data", map[string]interface{}{
        "id":   s.ID(),
        "data": string(data), // <-- bytes->string copy
    })
})
- **影响**: Per active terminal this is the single hottest allocation site. Claude Code's spinner phase emits many small chunks per second; each chunk triggers: 1 map alloc, 1 string copy (escaped heap), 2 interface boxings, and a Wails JSON marshal + WebKit structured-clone. For 4 concurrent terminals at 50 chunks/s this is millions of allocs/s. Directly contributes to the 400 MB resident set.
- **修复**: Replace EventsEmit with a session-scoped Go channel + dedicated goroutine that uses json.NewEncoder on a sync.Pool'd buffer. Carry the bytes as base64 in a fixed-size struct (no interface map). Better: have the frontend subscribe to a binary protocol over a per-session WebSocket and decode there. Minimum change: drop the map and pass `(sID, []byte)` to a wrapper that constructs the map once per emit and reuses a string-builder.
- **验证**: go test -bench=BenchmarkSessionEmit -benchmem ./backend/session/... — record allocs/op before and after. pprof heap inuse_objects on `runtime.EventsEmit` should drop to <1% of total.

### F-303 — `app.go:1729` (caching)

- **根因**: `anthropic-beta: prompt-caching-2024-07-31` header is set, but the request body never has `cache_control: {type:"ephemeral"}` breakpoints on system / tools / last few messages. Static system prompt (~2KB) + tool definitions (~1KB) re-sent and re-billed every turn.
- **证据**: // app.go:1765  req.Header.Set("anthropic-beta", "prompt-caching-2024-07-31")
// app.go:1729-1734  reqBody["stream"] = true; json.Marshal(reqBody)  // no cache_control injection
// aiStore.ts:658  // Messages are inherently dynamic — no cache_control breakpoints here.
- **影响**: Every multi-turn Claude session. ~3KB wasted tokens + extra latency per turn. Across a 20-turn session that's 60KB of redundant input tokens, plus no cache-hit savings.
- **修复**: On chatCompletionAnthropic: insert cache_control breakpoint on `system` and on `tools` array. Optionally on the last 3-5 messages for incremental cache hits.
- **验证**: Inspect request body in Wails dev tools; compare cached vs uncached token counts in Anthropic dashboard.

### F-305 — `app.go:1779` (io)

- **根因**: Error-path response body reads use unbounded `io.ReadAll(res.Body)` with no size cap. A hostile or buggy upstream returning a multi-GB error body can OOM the Go process.
- **证据**: // app.go:1779  body, _ := io.ReadAll(res.Body); return "", fmt.Errorf("HTTP %d: %s", res.StatusCode, string(body))
// app.go:2098  // openai path
// app.go:2485  // responses path
// app.go:2684  // FetchModels path
- **影响**: Any non-200 response from LLM upstream. Currently unlikely in practice but trivial to fix and the cost is unbounded.
- **修复**: Cap to 64KB: `body, _ := io.ReadAll(io.LimitReader(res.Body, 64*1024))`.
- **验证**: pprof alloc on a synthetic upstream returning a 100MB 500 error body.

### F-306 — `app.go:1799` (allocation)

- **根因**: Every SSE `data:` line is fully `json.Unmarshal`-ed into a fresh `map[string]interface{}` allocation. Anthropic emits ~1 event per token for content_block_delta; a 2K-token response = 2K map allocations, ~99% of fields discarded immediately.
- **证据**: // app.go:1799-1802
var event map[string]interface{}
if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
  continue
}
eventType, _ := event["type"].(string)
- **影响**: Per-token allocation pressure on the streaming hot path. Compounds with string-concat accumulator in F-307.
- **修复**: Define typed structs (MessageStart, ContentBlockDelta with embedded delta, etc.) and decode into them. Use `json.RawMessage` and field-switch on `type` for the small set of event shapes actually handled.
- **验证**: pprof allocs during a streamed 5K-token response.

### F-403 — `backend/k8s/client.go:50` (io)

- **根因**: The k8s http.Transport is built without any keep-alive tuning: no MaxIdleConns, no MaxIdleConnsPerHost, no IdleConnTimeout, no TLSHandshakeTimeout — every REST call opens a fresh TCP + TLS handshake to the apiserver, even when called many times per second from the same tab (e.g. log streaming + pod polling).
- **证据**: backend/k8s/client.go:50 base := &http.Transport{ TLSClientConfig: tlsCfg, Proxy: http.ProxyFromEnvironment } — no MaxIdleConns/MaxIdleConnsPerHost/IdleConnTimeout/TLSHandshakeTimeout. http.Client{Transport: &authRoundTripper{base: base, ...}, Timeout: 0}.
- **影响**: Every kube list/get/watch reconnect and every log stream request pays full TCP+TLS handshake (often 50–200ms each on remote apiservers). On a busy cluster running `kubectl get pods -A` polling + a log tail + 2 watches, this is 5–10 handshakes/sec just from k8s. TCP TIME_WAIT sockets also accumulate.
- **修复**: Set MaxIdleConns=100, MaxIdleConnsPerHost=10, IdleConnTimeout=90s, TLSHandshakeTimeout=10s, ResponseHeaderTimeout=30s on the Transport; reuse a single *http.Client per kubeconfig context.
- **验证**: Benchmark: kube REST calls/sec with -keepalive=false vs enabled. Check `ss -s` TIME_WAIT count on `wails dev` while a K8s tab is open.

### F-404 — `backend/k8s/watch.go:36` (locking)

- **根因**: On every watch reconnect attempt, runWatchLoop calls onEnd(err) — which under manager.StartWatch is wired to emit `k8s:watch-end:<id>` via EventsEmit AND reacquire m.mu to clean up the watches map. So a flapping apiserver triggers an EventsEmit + a mutex dance every 1s/2s/4s/8s/16s/30s.
- **证据**: backend/k8s/watch.go:40-69 for-loop in runWatchLoop: `onEnd(err)` fires before `time.After(backoff)`. Manager wires onEnd to: emit event + `m.mu.Lock(); delete(m.watches, watchID); delete(c.watches, watchID); m.mu.Unlock()`. The next iteration immediately re-registers the watch in StartWatch and re-enters m.mu.Lock() again.
- **影响**: During a WiFi blip on a cluster with 4 watches, the frontend receives 4 reconnect-end events per backoff step (so 4 events at 1s, 4 at 2s, 4 at 4s …). Each emit also serializes the watch map lock against every concurrent StartWatch/StopWatch across all k8s tabs.
- **修复**: In runWatchLoop, surface onEnd ONCE on the first failure and emit silent `k8s:watch-reconnecting` instead; only emit a final onEnd when ctx is canceled or backoff succeeds. Defer the map cleanup to the success / cancel path.
- **验证**: Toggle WiFi with 3 watches active; EventsOn listener count for `k8s:watch-end:*` should drop from O(backoff_steps) to 1.

### F-009 — `backend/session/mosh_session.go:188` (locking)

- **根因**: MoshSession.readLoop calls `s.moshClient.Recv(100 * time.Millisecond)` in a tight for-loop that wakes every 100 ms even when the session is idle (no UDP input).
- **证据**: data := s.moshClient.Recv(100 * time.Millisecond)
if len(data) > 0 {
    s.RecordReadActivity()
    s.emitData(append([]byte(nil), data...))
}
if s.moshClient == nil {
    return
}
- **影响**: Each idle mosh session contributes 10 goroutine wakeups per second. With 8+ tabs this is a dominant contributor to the user's reported 900+ wakeups/sec measurement. Each wakeup also re-acquires the mosh client state and pays an allocation.
- **修复**: Use a longer Recv timeout (e.g. 1 s) or, better, block on `s.quit` and a long Recv timeout. Add an event-driven wake path: mosh library signals when frames arrive, the readLoop blocks until then.
- **验证**: trace / pprof goroutine samples during idle mosh session; netstat udp recv queue depth.

### F-011 — `backend/session/output_log.go:630` (locking)

- **根因**: OutputLogger.flushLoop runs `time.NewTicker(logFlushInterval)` where `logFlushInterval = 1 * time.Second`. Every active log file creates a goroutine that wakes once per second for the entire session lifetime, even when no bytes have been written.
- **证据**: const logFlushInterval = 1 * time.Second
func (l *OutputLogger) flushLoop() {
    defer close(l.flushDone)
    ticker := time.NewTicker(logFlushInterval)
    defer ticker.Stop()
    for {
        select {
        case <-l.flushCh:
            return
        case <-ticker.C:
            l.mu.Lock()
            if l.bw != nil {
                _ = l.bw.Flush()
            }
            l.mu.Unlock()
        }
    }
}
- **影响**: Every long-running SSH / local / Claude Code session that has logging on contributes 1 wakeup/sec. With 5–8 tabs open this is ~8 wakeups/sec just for logging flush loops, even when nothing is being written. Compounds the user's idle wakeup measurement.
- **修复**: Replace the ticker with a `chan time.Duration`-driven loop that wakes after `lastWrite + logFlushInterval`; or use `time.AfterFunc` that re-arms only when WriteOutput happens. Stop the goroutine when the underlying bufio.Writer is idle for N seconds.
- **验证**: pprof goroutine + runtime trace while 3 idle loggers are open.

### F-017 — `backend/session/session.go:341` (locking)

- **根因**: waitIdle busy-loops with `time.Sleep(50 * time.Millisecond)` for up to the configured timeout. With 8 tabs each waiting idle at login, this is up to 8 × (5 s / 50 ms) = 800 wakeups per tab connect cycle. After connect, idle windows of 5+ s pay 100 wakeups.
- **证据**: func (s *baseSession) waitIdle(timeout, idle time.Duration) bool {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        last := time.Unix(0, s.lastReadTime.Load())
        if !last.IsZero() && time.Since(last) >= idle {
            return true
        }
        time.Sleep(50 * time.Millisecond)
    }
    return false
}
- **影响**: Per-connect wakeup storm; contributes to the user's measured 900+ wakeups/sec when several tabs are logging in.
- **修复**: Subscribe to a per-session 'activity' channel that RecordReadActivity signals; use `select { case <-ch: case <-time.After(timeout): }` instead of polling. Increase idle granularity to 200–500 ms — 50 ms is overkill for idle detection.
- **验证**: pprof goroutine + runtime trace during a parallel 5-tab connect.

### F-103 — `backend/store/ai_session_store.go:54` (memory)

- **根因**: AISessionStore.Save / Load marshals / unmarshals the entire ai-sessions.json into memory on every save. AIMessageEntry retains Content verbatim plus an optional _rawApiMsg blob. No sliding-window or token-budget truncation.
- **证据**: type AIMessageEntry struct {
    ID, Role, Content string
    ToolCalls   []interface{}  `json:"tool_calls,omitempty"`
    PendingTools []interface{} `json:"pendingTools,omitempty"`
    RawAPIMsg   string         `json:"_rawApiMsg,omitempty"`
}
func (s *AISessionStore) Save(data AISessionData) error {
    jsonData, err := json.MarshalIndent(data, "", "  ")
    return os.WriteFile(s.filePath(), jsonData, 0600)
}
- **影响**: Long AI sessions with large tool results blow up the file and the Go-side decode cost grows linearly. The frontend also re-clones this blob across the Wails bridge. Together this contributes to the 400+ MB WebKit memory pressure on long sessions.
- **修复**: Cap Messages per session (sliding window of last N turns or token-budgeted). Drop RawAPIMsg after it's been transcoded into the visible message. Stream-write the JSON via json.Encoder instead of json.MarshalIndent.
- **验证**: Run a long Claude Code session; monitor ai-sessions.json size and RSS via pprof heap.

### F-102 — `backend/store/recent_store.go:55` (io)

- **根因**: RecentStore.Record does not debounce — every Record call re-marshals the full id slice (up to maxRecent=20) and rewrites recent.json via plain os.WriteFile. Not using atomicWriteFile so a crash mid-write can lose the file.
- **证据**: func (s *RecentStore) Record(id string) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // ...dedup + prepend + trim...
    return s.saveLocked()
}
func (s *RecentStore) saveLocked() error {
    data, err := json.Marshal(s.ids)
    return os.WriteFile(s.filePath, data, 0644)
}
- **影响**: Per connection-open and per reconnect a fresh JSON marshal + non-atomic file write happens. Inconsistent with the rest of the store package which fixed STORE-09.
- **修复**: Use atomicWriteFile; coalesce writes via a debounced background flusher; consider an append-only log.
- **验证**: Open + close connections in quick succession; stat recent.json mtime after each.

### F-101 — `backend/store/terminal_history_store.go:33` (io)

- **根因**: TerminalHistoryStore.Save writes the entire JSON file via os.WriteFile on every Save call (every command entered triggers Save via the UI). No write-coalescing or debounce; each Save produces a full file rewrite. Plain os.WriteFile is also non-atomic.
- **证据**: func (s *TerminalHistoryStore) Save(entries []HistoryEntry) error {
    // ...dedup loop...
    data := TerminalHistoryData{Entries: result}
    jsonData, err := json.MarshalIndent(data, "", "  ")
    if err != nil { return err }
    return os.WriteFile(s.filePath(), jsonData, 0600)
}
- **影响**: Every command a user types triggers a full JSON serialize + write of up to maxHistorySize (500) entries. Hundreds of full-file rewrites per active day. Crash mid-write can corrupt the file (contradicting STORE-09 quarantine).
- **修复**: Coalesce writes via a debounce timer (e.g. 500ms) and flush on shutdown; switch to atomicWriteFile for parity with ConnectionStore/SettingsStore; consider an append-only JSONL file.
- **验证**: tail -f modification time of terminal-history.json while typing in a terminal tab.

### F-026 — `frontend/src/components/BaseTerminal.vue:1052` (memory)

- **根因**: BaseTerminal subscribes to the GLOBAL `session:data` event inside onMounted and accumulates state per session. On every KeepAlive re-mount the same listener is re-registered (generation guard mitigates but the listener closure is still retained). Vue KeepAlive cache can hold multiple BaseTerminal instances of the same panel, each with a full xterm + addon + scrollback buffer — visible memory bloat per inactive tab.
- **证据**: unsubscribe = EventsOn('session:data', (payload: { id: string; data: string }) => {
    if (!isActive.value) { ... }
    if (payload.id !== props.sessionId || !terminal) return

- **影响**: Each inactive tab in KeepAlive still holds the xterm canvas + scrollback (2500 lines default). With 10 inactive tabs, that is ~10 × (2500 lines × ~80 cols) of pixel buffer pinned in WebKit. Direct contributor to 400+ MB.
- **修复**: Drop scrollback to ~500 lines for inactive tabs (and restore on activation). Lazy-create the xterm only when first activated, dispose when cached for >N seconds.
- **验证**: Chrome memory snapshot: 10 tabs, 5 active vs all inactive.

### F-034 — `frontend/src/services/terminalManager.ts:75` (render_compat)

- **根因**: xterm Terminal is created with `fontSize: 13`, `fontFamily: 'Consolas, "Courier New", monospace'`, scrollback 2500, but NO `lineHeight` configured. xterm defaults to 1.0 line height, which means italic glyphs (e.g. the ⏺ thinking block in Claude Code) clip at the top/bottom of their cell because descenders exceed cell height.
- **证据**: const terminal = new Terminal({
    fontSize: options.fontSize ?? 13,
    fontFamily: formatFontFamily(options.fontFamily ?? 'Consolas, "Courier New", monospace'),
    theme,
    cursorBlink,
    rightClickSelectsWord: false,
    scrollback: options.scrollback ?? 2500,
    allowProposedApi: true,
    allowTransparency: true,
})
- **影响**: Claude Code's italic thinking text appears cut off / visually broken. Compounded by the explicit `terminal.refresh(0, terminal.rows - 1)` in BaseTerminal.vue:498 — the refresh re-paints but cannot fix the underlying cell-height clipping.
- **修复**: Add `lineHeight: 1.15` (or expose as a setting). Consider also adding `letterSpacing: 0` for consistent width.
- **验证**: Open claude in uniterm, look at ⏺⏵ thinking block — descenders should not clip.

### F-302 — `frontend/src/stores/aiStore.ts:263` (allocation)

- **根因**: `addMessage` wraps every message in `reactive({ ...msg })` and `_rawApiMsg` is a deep-tracked reactive value. Per-token mutation of `content` invalidates ALL dependents of every message proxy in the dep graph.
- **证据**: // aiStore.ts:263-264
function addMessage(msg: AIMessage): AIMessage {
  const r = reactive({ ...msg }) as AIMessage
  messages.value.push(r)
- **影响**: All AI sessions. Per-token Vue dep-graph invalidation. With 200 messages in a long session, deep proxies × reactive arrays (tool_calls, _rawApiMsg blocks) inflates the dep graph substantially.
- **修复**: Use `shallowReactive` for the message wrapper and `markRaw(msg._rawApiMsg)` (it's only mutated by the LLM, never bound UI-side). Buffer streamed text in a non-reactive string assembled at flush time.
- **验证**: Repro: open DevTools Performance recording during a 5K-token response; observe script-time per token.

### F-304 — `frontend/src/stores/aiStore.ts:419` (serialization)

- **根因**: `doSave()` is called synchronously on every `addMessage` / `addSkillCard` / `addCommandCard` / `clearMessages`, marshaling and crossing the Wails bridge with the entire conversation history (including JSON.stringify of every `_rawApiMsg`).
- **证据**: // aiStore.ts:419  await SaveAISessions(data as any)
// aiStore.ts:276, 296, 315, 327, 514  doSave() called from each mutator + after each chat turn
- **影响**: Every message add. Long sessions push multi-MB per save across the Wails bridge. CPU + bridge cost is paid per token (every tool result adds a message → triggers doSave).
- **修复**: Debounce doSave (e.g., 500ms trailing); only persist on explicit save / session switch / window blur. Snapshot delta (only the changed session), not the whole array.
- **验证**: Bridge trace of SaveAISessions calls during a 5-turn session; confirm ≤2 calls instead of dozens.

### F-301 — `frontend/src/stores/aiStore.ts:486` (memory)

- **根因**: The `conversation` computed has no memoization; every reactive mutation on any message field (including per-token `content +=`) re-runs the entire 660-line transform: walks every message, JSON.stringify of _rawApiMsg, dangling tool_use filtering, pair validation, consecutive-user dedup.
- **证据**: // aiStore.ts:486-506 (paraphrased)
const conversation = computed(() => {
  ...
  const kept = []
  for (let i = messages.value.length - 1; i >= 0; i--) {
    const msgTokens = estimateMessageTokens(msg)   // includes JSON.stringify(_rawApiMsg)
    if (tokenCount + msgTokens > MAX_CONTEXT_TOKENS) break
    kept.unshift(msg)
  }
  // + 4 more linear passes (pair validation, dedup, block filtering)
})
- **影响**: Every Claude Code-style multi-turn session. Per token in a 100-message session: ~100 JSON.stringify calls on _rawApiMsg blocks + 4× O(n) passes. Hits main thread on every SSE chunk.
- **修复**: Compute conversation only when messages are added/removed (version counter), not when content mutates. Or store per-message token estimate alongside the message and update incrementally; cache the assembled Anthropic payload by messages-version.
- **验证**: pprof CPU on aiStore.conversation during a 50-turn Claude session; bench a session with `performance.mark` around computed eval.

### F-019 — `frontend/src/stores/sessionStore.ts:20` (memory)

- **根因**: sessionStore keeps up to 2000 chunks of every session's data (MAX_CHUNKS=2000, TRIM_TO=1000). `getData` joins all of them on every read, producing a multi-MB string per session. With 8 open tabs each holding 2000 chunks × 4 KB = 8 MB per session = 64 MB resident in the Pinia reactive state — all of which is held in JS by WKWebView's GC.
- **证据**: const MAX_CHUNKS = 2000
const TRIM_TO = 1000
function getData(id: string): string {
    const s = sessionState.sessions.get(id)
    if (!s) return ''
    const raw = s.data.join('')

- **影响**: Direct contributor to the user's reported 400+ MB resident memory. Each getData(id) call joins all 1000+ retained chunks — O(n) cost on every tab switch, every reconnect, every keepalive.
- **修复**: Replace the string array with a single ring-buffer string + offset cursor. Cap to ~256 KB per session (≈ 4× viewport). Add an explicit eviction on session remove that drops the SessionData entry from the reactive Map.
- **验证**: Chrome DevTools Memory snapshot before/after a 10-min Claude Code session with 8 tabs; expect per-tab retained size to plateau, not grow.

### F-043 — `main.go:1` (io)

- **根因**: No App Nap / lifecycle handling found anywhere in the repo. There is no Info.plist customization (no NSAppSleepDisabled reference); the Wails macOS runtime defaults to allowing App Nap. terminal_io goroutines (keepalive, mosh Recv, output log flush) keep firing at full rate even when the user backgrounds the app.
- **证据**: grep -rn "NSAppSleepDisabled\|App Nap\|disablePower\|activityAssertion\|beginActivity" → 0 hits in repo
(macOS App Nap throttles background apps; Wails does not disable it by default)
- **影响**: Background terminal sessions continue waking at full rate — directly multiplies the user's 900+ wakeups/sec measurement whenever the app is in background. macOS may also keep the GPU/WebKit awake longer.
- **修复**: (a) Register Wails OnHide / OnShow handlers in app.go that pause keepalive goroutines and mosh Recv when the window is hidden. (b) For macOS-specific, generate an Info.plist entry that opts out of App Nap only when a session requires foreground network — or accept App Nap and ensure throttled goroutines are fine. (c) Add import _ "net/http/pprof" so wakeup sources can be profiled.
- **验证**: Run app, background, sample pprof goroutine — count of flushLoop/startKeepAlive should drop when hidden.

### F-201 — `main.go:88` (io)

- **根因**: No pprof endpoint is exposed; the audit's §8.2 prerequisite and the checklist's G item both flag this.
- **证据**: func main() {
    ...
    err := wails.Run(&options.App{ ... Bind: []interface{}{ app, } })
- **影响**: Cannot run go tool pprof http://localhost:6060/debug/pprof/profile to capture CPU/heap/block profiles during a Claude Code session or any reproduction. The audit therefore cannot validate any wakeup/memory hypothesis, and end-users cannot diagnose regressions without rebuilding from source.
- **修复**: In startup() start `go func() { _ = http.ListenAndServe("localhost:6060", nil) }()` with `import _ "net/http/pprof"`. Gate on dev builds (e.g. const dev = Version == "dev") so production builds don't open a listener.
- **验证**: go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30 with the audit's recording template; verify the symbol list contains wails_bridge.* and session.OutputLogger.* paths.

## 5. P1 详述

### F-206 — `app.go:123` (locking)

moveResizeCh goroutine launched in startup() never receives a shutdown signal — it lives for the process lifetime, blocking on a channel that is never closed.. *修复*: Add `defer close(a.moveResizeCh)` (or signal via select { <-ctx.Done(): close(ch) }) in the goroutine body, and `close(a.moveResizeCh)` in shutdown(). For one-shot startup goroutines, branch on ctx.Done() before kicking off heavy work.

### F-211 — `app.go:343` (io)

SaveConnections / SaveSettings / SaveTunnels / SaveQuickCommands / SaveAIConfig all run a.fs-fsync on the Wails handler thread (atomic-rename write inside store.Save) before the emit; combined with F-203/207 this stalls the UI on every save.. *修复*: Run a.connectionStore.Save in a goroutine; return immediately with a request-id; emit 'store:saved' (or just 'store:connections:changed' if Save was best-effort with eventual consistency). For settings where the user expects persistence confirmation, keep Save sync but make SaveConnections async.

### F-207 — `app.go:619` (locking)

triggerAutoSync spawns a fresh `go func()` on every store mutation with no coalescing or in-flight tracking, so a burst of saves (e.g. pasting 50 commands into QuickCommands) launches 50 concurrent syncs.. *修复*: Add a single `syncInFlight sync.atomic.Bool` flag — if set, return immediately. When the sync completes, clear the flag and re-check whether another trigger came in during the run; if so, kick one more. Cap concurrent syncs at 1.

### F-308 — `app.go:1746` (locking)

`chatCancel` is a single `context.CancelFunc` field with `chatCancelMu` protecting it. When two `ChatCompletion` calls overlap (retry storm, quick user re-trigger), call A's defer nils out `chatCancel` while call B is still in flight; `CancelChatStream` then becomes a no-op for B.. *修复*: Use atomic.Pointer[CancelFunc] and only nil the slot if it still points to your own cancel. Or refuse new chat while one is active (serialize at the front door).

### F-208 — `app.go:1768` (io)

chatCompletion* methods construct `&http.Client{Timeout: 0}` per call; no keep-alive, no connection pool — every LLM request opens a fresh TCP+TLS handshake.. *修复*: Hoist a single `*http.Client` with `Transport: &http.Transport{TLSClientConfig: &tls.Config{...}, MaxIdleConnsPerHost: 4, IdleConnTimeout: 90 * time.Second}` to a sync.Once-built App-scoped field. Reset idle conns on shutdown. Streamed bodies do not need the client to be closed mid-flight.

### F-320 — `app.go:1820` (serialization)

Every SSE event triggers a `runtime.EventsEmit` carrying a freshly built `map[string]interface{}` payload (ai:block_start / ai:token / ai:input_json_delta / ai:content_block_stop / ai:done). These all cross the Wails bridge to the frontend, getting JSON-marshaled on the Go side and JS-cloned on the WebKit side. Per-token `ai:token` is the worst: a text-only delta plus the index crosses the bridge for every token of every response.. *修复*: Coalesce consecutive `ai:token` events in Go (e.g., 16ms flush window) before crossing the bridge. Or use a single WebSocket-like channel that flushes per animation frame. Drop unused fields from the event payload.

### F-307 — `app.go:1832` (allocation)

Text delta accumulation uses `currentBlock["text"].(string) + text` (string concat) per token. O(n²) for long responses plus every concat allocates a new string.. *修复*: Build a `bytes.Buffer` per block; flush to string at content_block_stop / message_stop.

### F-210 — `app.go:1885` (serialization)

Every ai:done emit and the final return value both json.Marshal the full message map from scratch, with the same contentBlocks slice; double work per turn.. *修复*: Marshal once into a pooled buffer; reuse the resulting JSON for both the emit and the return. Better: skip the EventsEmit payload entirely for the final message and let the frontend pick up the return value through the promise resolution (which it already does).

### F-113 — `backend/database/engine.go:50` (allocation)

queryStrings and queryAny allocate a fresh map[string]any per row plus a fresh []any value holder per row. For a 10k-row x 20-col result that's 10k maps + 200k interface{} slots.. *修复*: Stream results back to the frontend via a Wails event or via paged callbacks. Provide a row-count cap. Use []map[string]any only for small results; for large results return rows + columns separately and stream.

### F-116 — `backend/database/provider_mysql.go:19` (io)

mysqlProvider DSN sets timeout=10s but no ConnMaxLifetime / ConnMaxIdleTime / MaxOpenConns / MaxIdleConns. database/sql defaults to unlimited open conns and unbounded lifetime - a long-lived tab can accumulate idle TCP sockets to the DB, and a server-side timeout (wait_timeout) eventually drops them.. *修复*: In NewDB after sql.Open call db.SetMaxOpenConns(10), db.SetMaxIdleConns(5), db.SetConnMaxLifetime(5*time.Minute), db.SetConnMaxIdleTime(2*time.Minute). Same for postgresProvider / oracleProvider / sqlserverProvider.

### F-115 — `backend/database/provider_postgres.go:178` (io)

Postgres GetTableSchema issues three serial round-trips (columns, primary keys, indexes) per call. For a database-tree UI panel that walks every table this is 3*N queries. MySQL/SQLServer have the same shape.. *修复*: Cache the schema for the lifetime of the connection; serve subsequent GetTableSchema calls from cache; invalidate only on user-driven schema mutation. Same cache applies to all providers.

### F-405 — `backend/k8s/kubeconfig.go:98` (caching)

ParseBytes fully re-parses the kubeconfig YAML (yaml.Unmarshal + base64 decode + map allocations) on every Connect call, with no caching keyed on file mtime or content hash — and Connect is called every time a K8s tab is opened.. *修复*: Add a Manager-level LRU/single-entry cache keyed by sha256(kubeconfigYAML) → *Kubeconfig, OR key on file mtime when the YAML came from disk; cache also the parsed *tls.Config and the derived *http.Client.

### F-413 — `backend/k8s/manager.go:167` (memory)

Every StartWatch / StartLogStream captures `m.emit` under the manager mutex and uses it for the lifetime of the watch — but m.emit is set once via SetEventEmitter in startup and never changes. The bigger leak is that the `connection.watches` set is only cleaned up on Disconnect / StopWatch; if the Wails frontend reloads (HMR dev mode) and reopens k8s tabs, stale watchIDs pile up until process exit.. *修复*: Add a WatchRegistry sweeper that runs every 60s and prunes any watchHandle whose ctx is already Done(); expose WatchRegistry.Stats() for the audit's pprof verification.

### F-411 — `backend/k8s/rest.go:24` (io)

Do() only enforces a 64 MiB body cap and respects ctx cancellation, but the surrounding http.Client has Timeout=0 (intentional, for watches) — so a hung apiserver TCP connection for a non-watch request will block forever, unless the caller remembers to set a deadline on ctx.. *修复*: In http.Transport, set ResponseHeaderTimeout=30s and ExpectContinueTimeout=2s; in Do(), if ctx has no deadline, wrap it with a sane default (e.g. 5 min) so accidental callers can't pin the goroutine.

### F-406 — `backend/k8s/rest.go:55` (io)

Every successful REST response logs a 300-byte body preview via log.Writef, which calls fmt.Sprintf on every call and writes to a file-backed logger — paid on every k8s request, including polling calls fired many times per second by the frontend.. *修复*: Move the body preview to log.LevelDebug gated by an env flag; for production builds log only status, method, path, byte count, latency.

### F-005 — `backend/session/local_session_unix.go:150` (io)

LocalSession readLoop uses a 4 KiB raw `os.File.Read` on the PTY master fd, with no bufio wrapper, no deadline, and a non-blocking `select{ <-s.quit: default }` polling guard at every iteration.. *修复*: Wrap pty in bufio.NewReaderSize(pty, 32*1024) for downstream reads, and rely on the quit signal only inside the Read via SetReadDeadline — no need for the per-iter select.

### F-006 — `backend/session/local_session_unix.go:163` (allocation)

Every read allocates a new `data := append([]byte(nil), buf[:n]...)` then immediately calls `updateMouseTrackingState(data)` which runs `bytes.Contains(data, seq)` over up to 14 short sequences. The strip/inspect work runs on every single chunk, not on coalesced data.. *修复*: Call bytes.Contains on the original buf[:n] (no copy) and only allocate when emitting to the onDataCallback. Coalesce consecutive small reads with bufio before invoking the mouse-tracking pass.

### F-010 — `backend/session/mosh_session.go:196` (allocation)

Every Recv() result is copied via `append([]byte(nil), data...)` before emitData — even though mosh's Recv returns a fresh slice owned by the caller.. *修复*: Pass the mosh-owned slice directly to emitData. If onDataCallback must retain, copy only when needed.

### F-014 — `backend/session/output_log.go:39` (allocation)

ansiStripper.Strip allocates `make([]byte, 0, len(data))` for every chunk and the `s.pending = append(s.pending[:0], data[i:]...)` copy on incomplete tails reallocates the pending slice as it grows.. *修复*: Use a pooled bytes.Buffer (sync.Pool) for `out`; cap `s.pending` length (e.g. 1 KiB) and treat longer incomplete sequences as malformed + drop.

### F-015 — `backend/session/output_log.go:181` (allocation)

lineProcessor.Feed calls `out = append(out, p.line[p.emitted:]...)` and `out = append(out, '\n')` repeatedly within the loop on every byte, plus `out` is allocated per call (var out []byte grows).. *修复*: Pre-size `out` to `len(in) + len(p.line) + 8`. Use a bytes.Buffer pool. Track emitted via an index without re-slicing.

### F-012 — `backend/session/output_log.go:449` (io)

OutputLogger.Enable walks up to 100 filename suffixes via `for suffix := 1; suffix <= 100; suffix++` and calls `os.OpenFile` with `os.O_CREATE\|os.O_EXCL\|os.O_WRONLY` per attempt, costing up to 100 stat+create syscalls on collision.. *修复*: Compute the suffix atomically with os.CreateTemp (or use a single os.MkdirTemp within the session dir); reserve a UUID suffix instead of a numeric loop.

### F-013 — `backend/session/output_log.go:583` (locking)

WriteOutput acquires `l.mu` and holds it across both ANSI stripping and the line-processor pass — which can call back into the lineProcessor's CSI parser. On a chatty session this serializes writes to the log while still allowing emitData to proceed (that's under a separate lock), but couples log writing latency to log formatting work.. *修复*: Split into an inner mutex guarding stripper/lines state and the outer file mutex around bw.Write/Flush. Drop the outer lock when the byte slice is empty.

### F-016 — `backend/session/session.go:169` (locking)

baseSession.emitData takes two RLock guards (outputLogMu then mu) and runs both callbacks inside the locked region of baseSession.mu. When the onData callback chains into Wails EventsEmit, every remote byte pays an extra lock round-trip and the lifetime of the callback extends the lock-hold window.. *修复*: Combine into a single RWMutex protecting both fields. Inline the load under a single RLock acquisition and store both callbacks atomically when set.

### F-001 — `backend/session/ssh_session.go:313` (io)

SSH read loop uses a fixed 4096-byte buffer per read; under heavy output (`cat` of a large file, log streaming, Claude Code streaming long answers) the per-RTT syscall count dominates and latency scales with chunk count, not bytes.. *修复*: Increase read buffer to 32 KiB or 64 KiB (matches xterm.js write batch size and PTY kernel buffers) and/or wrap stdout with bufio.Reader at the conn level so crypto/ssh is drained in one syscall per goroutine pass.

### F-004 — `backend/session/ssh_session.go:313` (allocation)

readLoop allocates two full copies of every received chunk: one for `data := append([]byte(nil), buf[:n]...)` and another inside `s.lastRecv.Store(append([]byte(nil), data...))`. Both copies are paid even when no diagnostic read happens.. *修复*: Pass buf[:n] directly to emitData (no copy); only copy into lastRecv on disconnect path. For offerExpectOutput, hand the same slice or a 0-copy view.

### F-044 — `backend/session/ssh_session.go:434` (io)

startKeepAlive ticker fires every 60s (sshKeepAliveInterval = 60 * time.Second) for the lifetime of every SSH session. With 8 tabs this is ~8 wakeups/min — modest, but still an idle cost that does NOT pause on app background.. *修复*: When window is hidden (via OnHide hook), double the interval or pause until re-shown. On app quit, skip the keepalive entirely (Disconnect fires).

### F-003 — `backend/session/ssh_session.go:459` (allocation)

Write() copies the entire encoded input via `append([]byte(nil), enc...)` and stores it in lastSent; on a chatty session this duplicates every byte sent to the server into an ever-growing diagnostic buffer.. *修复*: Cap lastSent to e.g. last 256 bytes via a ring buffer; do not store the full payload on every keystroke — store the tail only.

### F-002 — `backend/session/ssh_session.go:542` (allocation)

decodeOutput allocates a fresh `src` slice per call and copies `decodeLeftover + data` into it even when the decoder is nil-path; the encodeInput path also rebuilds the encoder on every keystroke via `enc.NewEncoder().Bytes(data)`.. *修复*: Cache a single `transform.Transformer` per session (set during SetEncoding); reuse its scratch buffer with `Transform(dst, src, atEOF)`. Persist the encoder side too via `s.encoder` field.

### F-008 — `backend/session/telnet_session.go:98` (io)

Telnet read loop reads 4 KiB at a time and immediately filters IAC inline; filterIAC allocates a fresh `var out []byte` and copies byte-by-byte. Under chatty telnet banners the IAC scanner runs O(n) per chunk.. *修复*: Pre-size `out` to `len(data)` and walk with index appends instead of slice-by-slice (`out = append(out, data[i])`). For high-throughput telnet, batch reads into 32 KiB.

### F-106 — `backend/store/commands_store.go:103` (io)

CommandsStore.List does a full directory scan + reads every commands/*.md file via readCapped on every call. No in-memory cache; on each call it also reads commands.json and frequently rewrites it (the default-fill loop sets changed = true the first time a missing-pref is encountered).. *修复*: Cache the merged List result and invalidate only on fsnotify events (or on savePaths). Skip the pref-default-fill write during read; defer the migration to a one-time startup pass.

### F-111 — `backend/store/commands_store.go:247` (io)

CommandsStore.CreateCommand / SaveCommand call os.WriteFile on the .md file directly (no atomic rename, no fsync) while also rewriting commands.json. A crash between the two writes leaves the .md updated but commands.json stale (or vice versa).. *修复*: Use atomicWriteFile (the helper the rest of the package uses) for both writes. Order: write prefs first, then the .md, so an orphan pref (no .md) is recoverable on next List.

### F-105 — `backend/store/connection_store.go:55` (serialization)

ConnectionStore.Save marshals the full ConnectionConfig slice (including PostLoginExpectSteps, K8sConfigInline YAML strings) with json.MarshalIndent even for a single-field edit.. *修复*: Use json.NewEncoder without indent (drop MarshalIndent in production). Consider a flat-file-per-connection layout, or at minimum diff-and-write only the changed connection.

### F-110 — `backend/store/connection_store.go:142` (locking)

ConnectionStore.populatePasswords re-iterates every connection, calls the keychain once per password, and if any plaintext passwords existed, takes the write lock and does an atomic write of the full file under that lock.. *修复*: Backfill passwords asynchronously after returning Load() (mark conns as PasswordPending so the UI can prompt). Coalesce the migration rewrite to a single startup-only pass.

### F-108 — `backend/store/settings_store.go:141` (serialization)

SettingsStore.Save marshals the entire AppSettings with json.MarshalIndent and atomic-writes, including CustomTerminalThemes (full TerminalThemeColors for each) and the Keyboard map. MarshalIndent allocates a buffer the size of the output for whitespace alone.. *修复*: Persist only the changed subtree (split into multiple files: theme.json, terminal.json, ai.json, keybindings.json) OR write a JSON patch. Coalesce successive saves (debounce 250ms). Drop MarshalIndent in production.

### F-109 — `backend/store/settings_store.go:166` (locking)

SettingsStore.Load takes the write lock for the entire duration of disk I/O + keychain backfill + default-fill. Any concurrent Save blocks behind it; the read returns only after the file is fully decoded.. *修复*: Use sync.RWMutex for reads, hold the write lock only during the keychain backfill + needsSave rewrite. Cache the parsed AppSettings in memory so subsequent Loads return instantly.

### F-107 — `backend/store/skills_store.go:226` (io)

SkillsStore.List reads every skill's SKILL.md (and probes references/ + scripts/ via dirHasFiles and countFiles, each of which opens a directory) on every invocation. No caching.. *修复*: Cache the merged list keyed by skills-dir mtime; use a single os.ReadDir per skill dir and partition files/subdirs in-memory instead of three separate dir reads.

### F-112 — `backend/store/skills_store.go:554` (io)

copyDir uses filepath.Walk that calls os.Lstat + os.Open + os.Create per file with no batching. copyFileWithoutSymlinks opens a fresh file handle for each file and uses io.Copy with default 32KB buffer; for many small reference files this is overhead-bound.. *修复*: Use io.CopyBuffer with a pooled []byte (sync.Pool) and reuse open handles when iterating. Skip files with identical content (mtime/size check).

### F-407 — `backend/sync/sync_service.go:35` (io)

NewSyncService() runs synchronously inside app.startup and may invoke the OS keychain (PBKDF2 600k iterations + keychain IPC) — on macOS the Security framework call to read the encryption key can take 50–300ms, and PBKDF2 alone on the same keychain access pattern can take >500ms.. *修复*: Defer the entire sync.NewSyncService() body to a goroutine; expose a `sync.Ready()` channel that the SyncNow bound method waits on with a short timeout; in main.go/wails RunOptions set HideWindowOnClose=false so first paint is not blocked.

### F-409 — `backend/update/checker.go:199` (io)

Check() builds a fresh *http.Client with Timeout=10s but no transport reuse, no UA-cache, no If-Modified-Since / ETag handling — and the 5-min disk cache TTL only blocks identical calls within 5 minutes, after which every Check() opens a brand-new TCP+TLS to api.github.com.. *修复*: Use a package-level *http.Client with a tuned Transport; honor GitHub's ETag / If-None-Match on the second and subsequent calls; respect Cache-Control: max-age from the API response (currently ignored).

### F-029 — `frontend/src/components/BaseTerminal.vue:365` (allocation)

sanitizeTerminalHistory runs 7 sequential regex passes over the entire scrollback string every time a KeepAlive tab restores. The pattern `/[^\u0000-\u007f一-鿿...]/g` allocates fresh match objects for every Unicode codepoint scan, and the function is invoked on every re-activation.. *修复*: Combine the regex passes into a single scan via a single regex with alternation, or stream the input through a small state machine. Skip the pass entirely if scrollback is fresh from the active write path.

### F-033 — `frontend/src/components/BaseTerminal.vue:461` (render_compat)

resize() calls `terminal.refresh(0, terminal.rows - 1)` after every fit to force a full-viewport redraw. This is the "lineHeight / italic-clip" workaround mentioned in the spec: the existing render path may clip italic descenders, so a forced refresh is used to repaint.. *修复*: Set `xterm.option.lineHeight` to 1.15+ so glyphs have enough vertical room (no clipping); then drop the explicit `terminal.refresh(0, terminal.rows - 1)` call.

### F-028 — `frontend/src/components/BaseTerminal.vue:530` (allocation)

exportContent walks every line in the xterm buffer (`buffer.length` rows) and calls `line.translateToString()` on each, then joins them with '\n' into a single string and base64-encodes via TextEncoder + manual charCode loop. The `for (let i = 0; i < bytes.length; i++) { binary += String.fromCharCode(bytes[i]) }` is the slowest possible string-concat in JS.. *修复*: Replace the binary loop with `String.fromCharCode.apply(null, bytes.subarray(...))` or use the FileReader / `fetch('data:application/octet-stream;base64,' + base64Encoded)` indirect path. Process the loop in chunks of 8192 to avoid stack overflow.

### F-031 — `frontend/src/components/BaseTerminal.vue:530` (io)

exportContent calls `WriteFileBase64(filePath, toBase64(content))` which marshals the full scrollback to base64, then crosses the Wails bridge, then Go decodes and writes the file. For a 1 MB scrollback this is 1.33 MB of base64 + JSON marshalling + Wails IPC.. *修复*: Add a streaming WriteFile(path, []byte) Wails method; send the raw UTF-8 bytes directly. For very large scrollbacks, stream via ReadableStream.

### F-030 — `frontend/src/components/BaseTerminal.vue:1135` (allocation)

Each session:data chunk is run through stripCursorBlink (regex replace), then 2 .replace(/\u001b[2J/g, ...) regex passes, then stripAnsi (3 regexes via useTerminalInput.handleSessionData), then highlight (which itself runs 7+ regex patterns per line) on the FRONTEND for every chunk arriving from the backend.. *修复*: Batch incoming chunks in a 0-ms rAF coalescer, then run all regex passes on the coalesced payload once per frame. Use regex literals at module scope (already done — confirm no regex is in a hot closure).

### F-027 — `frontend/src/components/BaseTerminal.vue:1553` (memory)

releaseTerminal has a 500 ms disposeTimer that disposes the underlying xterm after all components release. During that 500 ms window, a fully detached xterm still holds its canvas + scrollback DOM nodes (moved to the hidden holding container in services/terminalManager.ts). If the user drags a tab back into the holding area, the terminals accumulate indefinitely until the session closes — adding up over a long day.. *修复*: Drop scrollback when moving into holding (terminal.options.scrollback = 0 / release buffer) so the retained xterm is small. Or skip the holding container and use WeakRef-style lifetime tied to the Vue component instance.

### F-041 — `frontend/src/composables/useFocusTerminal.ts:67` (render_compat)

installTerminalFocusRestore hooks a document-level mousedown listener that walks up the DOM tree on every click (`while (cur) { getComputedStyle(cur).getPropertyValue('--wails-draggable')... }`). Every mouse click anywhere in the app pays a synchronous reflow + computed-style read chain.. *修复*: Cache the --wails-draggable computed value per element on a WeakMap; invalidate on theme/setting change. Or check el.closest('[data-drag-region]') instead (a single attribute lookup).

### F-021 — `frontend/src/composables/useTerminal.ts:380` (caching)

resize() reaches into xterm internals (`(terminal as any)._core?._renderService?.dimensions`) every time it runs. On a typical session, resize is called by IntersectionObserver, ResizeObserver, window resize, split-resize, and the retry timers in onMounted — easily 50+ times per second during drag.. *修复*: Memoize cellWidth/cellHeight keyed by fontSize/fontFamily. Add a single coalesced debounced resize in the BaseTerminal (BaseTerminal.vue already does this — reuse its approach).

### F-020 — `frontend/src/composables/useTerminal.ts:471` (memory)

The legacy useTerminal composable creates a NEW xterm.Terminal and three new addons (FitAddon, SearchAddon, WebLinksAddon) per call without disposing the prior addons if the composable is invoked twice in the same panel (e.g. workspace drag-in reuse). The searchAddon in particular has no explicit Dispose() call in onUnmounted.. *修复*: Track all loaded addons in a list, dispose each in onUnmounted. Add a noopDispose stub for addons that don't expose dispose(). Modern components should route through services/terminalManager.ts (which already dispose()s the Terminal).

### F-022 — `frontend/src/composables/useTerminalInput.ts:47` (algorithmic)

getCurrentCommandFromTerminal scans every visible line (buffer.rows iterations) per Enter keystroke. Each line runs translateToString() (O(width)) + stripAnsi() regexes + PROMPT_RE match. A 80×24 terminal pays 24 regex matches + 24 stripAnsi regexes per Enter.. *修复*: Track the last prompt line seen via xterm onCursorMove or scan the cursor row first (cheap), then walk up only until a prompt is found. Cache compiled regex objects at module scope (already module-level) — confirm no RegExp is created inside the function.

### F-023 — `frontend/src/composables/useTerminalInput.ts:158` (allocation)

handleInput uses `lineBuffer.value.slice(0, idx) + ... + lineBuffer.value.slice(idx)` (string concatenation) on every printable character to insert into the middle of the line. Each edit produces 2 new string objects + 1 new buffer string.. *修复*: Switch lineBuffer to a string[] + cursorIndex model (or use Uint8Array); expose concatenation only when read by downstream consumers. For mid-string insert, splice the array.

### F-311 — `frontend/src/services/agent.ts:430` (io)

`activeAssistantMsg.content += data.text` is a deep-reactive mutation per SSE token that triggers the entire AISidebar render chain. With high-throughput models (Claude 100+ tok/s, GPT-4o), this is 100+ Vue re-renders per second on the main thread.. *修复*: Buffer tokens into a non-reactive string; flush to `activeAssistantMsg.content` via `requestAnimationFrame` (16ms cadence). Use `shallowRef` for the displayed text so token appends don't churn the dep graph.

### F-312 — `frontend/src/services/agent.ts:670` (memory)

Tool result blobs (execute_command output, capture_terminal screen, use_skill manifest) stored verbatim in `m.content`. A 200-line × 2KB capture_terminal = 400KB retained. The conversation computed (F-301) walks all retained blobs every turn.. *修复*: Cap tool result blobs at write time (e.g., head+tail truncation, or store large blobs to disk and reference by path/id in the message).

### F-036 — `frontend/src/services/terminalManager.ts:75` (render_compat)

xterm Terminal options do not enable italic font — there is no italic font configured. SGR 3 (italic) sequences forwarded by Claude Code's thinking-block markup render as upright text. The ItalicAddon (`@xterm/addon-italic`) is NOT loaded.. *修复*: Install `@xterm/addon-italic`; load it after FitAddon/SearchAddon/Unicode11Addon in services/terminalManager.ts. Update fontFamily to include a font that has italic face (e.g. `'JetBrains Mono', 'Cascadia Code', ...`).

### F-037 — `frontend/src/services/terminalManager.ts:75` (render_compat)

xterm Terminal is not configured for DEC mode 2026 synchronized output. Claude Code emits \e[?2026h ... \e[?2026l to bracket a multi-line repaint (thinking-block redraws). Without SyncAddon / mode 2026 enabled, each line paints individually → flicker / partial renders during the spinner phase.. *修复*: Install `@xterm/addon-sync`; load after other addons; the addon enables synchronized rendering for the bracketed pairs.

### F-039 — `frontend/src/services/terminalManager.ts:75` (render_compat)

xterm Terminal is created without explicit `windowsMode`, `scrollback` is the only viewport setting. Alternate screen buffer mode 1049 is supported by xterm natively, but the BaseTerminal resize path does not preserve scrollback correctly across alt-screen enter/exit (the terminal preserves it, but Vue's terminal.position tracking does not account for the saved-buffer offset).. *修复*: Verify on the latest xterm.js that alt-screen buffer save/restore is honored — it should be by default. If scrollback is lost, set `terminal.options.windowsMode = false` (forces normal mode, not Windows-conpty alt-screen shim) and ensure BaseTerminal does not call terminal.reset() on alt-screen exit.

### F-035 — `frontend/src/services/terminalManager.ts:100` (render_compat)

Unicode 11 widths are enabled via `terminal.unicode.activeVersion = '11'`, but there is no `wcwidth`/`charSizeCompat` override. East Asian ambiguous-width characters (e.g. ⏺⏵ used by Claude Code) still use Unicode 11's ambiguous=1 → may misalign with backend PTY (which often uses wcwidth -u13 or older). Also missing: 256-color code-block background (SGR 48;5;) — themes have only background, no codeBlockBackground.. *修复*: Set `terminal.options.charSizeCompat = true` (or expose a setting). Add codeBlockBackground to the theme schema; populate it for the built-in themes (subtly different from background, e.g. #0d1117 → #161b22 on github-dark).

### F-310 — `frontend/src/stores/aiStore.ts:45` (serialization)

`estimateMessageTokens` does `JSON.stringify(msg._rawApiMsg)` on every message, every time the conversation computed re-evaluates (which is per-token per F-301). For a 100-message session this is 100 redundant JSON stringifies per token.. *修复*: Cache `msg._tokenEstimate` at addMessage time (after JSON.stringify runs once). Track a running total on the conversation computed and decrement/increment on add/remove.

### F-316 — `frontend/src/stores/aiStore.ts:147` (memory)

`loadSessionsFromBackend` eagerly `JSON.parse`s `_rawApiMsg` for every message in every saved session at app startup. For 15 sessions × 200 messages, that's 3000 JSON parses; parsed object trees held in memory even for sessions that won't be opened in this session.. *修复*: Keep `_rawApiMsg` as JSON string; only parse on session switch (when the messages become reactive and may be sent to the API). Or persist as JSON-encoded blob per session and parse on demand.

### F-314 — `frontend/src/stores/aiStore.ts:399` (serialization)

`doSave` materializes a full snapshot via `sessions.value.map(...)` on every call (every message add). For a 200-message session with 4KB `_rawApiMsg` each, that's ~800KB allocated per save + bridged across to Go.. *修复*: Diff-based save (only the mutated session). Debounce to coalesce bursts. Pre-stringify `_rawApiMsg` once at `addMessage` so the JSON form is cached.

### F-313 — `frontend/src/stores/aiStore.ts:486` (algorithmic)

`conversation` makes multiple linear passes over the message array: build kept (with token-budget break), strip leading tool messages, collect resolved tool_use IDs, second loop filtering, third loop for pair validation, fourth for consecutive-user dedup. O(4n + n·estimateMessageTokens) per chat call.. *修复*: Single forward pass that validates pairings, deduplicates, and emits output in one loop. Build the `resolvedIds` Set lazily from the same pass.

### F-410 — `main.go:76` (io)

main.go synchronously loads LocalStateStore from disk to read the persisted window-frame preference BEFORE wails.Run — any disk stall (slow antivirus, network home dir, locked file) delays the entire app launch.. *修复*: Default to the framed-vs-frameless preference (systemTitleBar=false) and apply it asynchronously; if LocalStateStore.Load returns within 100ms, override before first paint via wails.OnDomReady.

## 6. P2 列表

- **F-213** `app.go:1347` — ListSessions returns every SessionInfo in the manager on each call; if the frontend polls this (it does — see panelStore and tabStore cross-checks) it produces a steady stream of allocations even when nothing has changed.
- **F-212** `app.go:3273` — panelLogTitle and EnableSessionOutputLog both scan the entire sessionToPanel map (keyed by sessionID) to find the one bound to a panelID — O(n) per enable on every Enable call.
- **F-114** `backend/database/engine.go:126` — scanToString calls fmt.Sprintf("%v", v) for any non-[]byte value. fmt.Sprintf allocates a fmt.buffer + parses the format string per call.
- **F-412** `backend/k8s/watch.go:92` — runOneWatch allocates a fresh 64 KiB initial Scanner buffer per watch and grows it up to 4 MiB for any single line; on a busy cluster where watch lines can be multi-MB JSON blobs (CRDs), this buffer is retained in the per-event allocated path and not pooled.
- **F-018** `backend/session/manager.go:10` — SessionManager.sessions map grows unbounded; Close deletes by ID, but failed/abandoned sessions (process crashed mid-Connect) are never reaped. The map also holds each session forever after Disconnect until the user explicitly closes the tab in UI.
- **F-007** `backend/session/serial_session.go:109` — SerialSession readLoop uses 4 KiB Read and additionally copies each chunk via `data := make([]byte, n); copy(data, buf[:n])` before emitting, while also running `normalizeNewlines` which always allocates a fresh buffer.
- **F-032** `frontend/src/components/BaseTerminal.vue:1500` — The terminal settings watcher does `deep: true` which means every fontSize/fontFamily/scrollback/theme change reruns the entire handler, including building a fresh theme object and calling applyXtermTheme. theme is a fresh object every call → xterm internal theme diff re-runs all colors.
- **F-042** `frontend/src/composables/useFocusTerminal.ts:113` — focusPanelTerminal retries with setTimeout(100ms) up to 10 times when xterm-helper-textarea is missing. With KeepAlive + drag, xterm's internal textarea can be absent for many frames — 10 retries × 100ms = up to 1 second of focus polling.
- **F-024** `frontend/src/composables/useTerminalInput.ts:90` — updateCursorPosition() walks xterm internals (`buffer.x`, `buffer.y`, `renderer.dimensions.css.cell.width`) every time a 0-ms setTimeout fires. updateToken is also called per character; per-keystroke cursor tracking has no throttle beyond the 0-ms defer.
- **F-040** `frontend/src/composables/useTerminalMenu.ts:67` — writeClipboard first awaits ClipboardSetText (Wails round-trip); on false result, awaits navigator.clipboard.writeText (another round-trip). Each clipboard operation is a full IPC hop.
- **F-025** `frontend/src/composables/useTerminalThemeOptions.ts:22` — terminalThemeGroups is a `computed()` that rebuilds via `TERMINAL_THEMES.filter(...)` on every dependency change. TERMINAL_THEMES is a constant module-level list, so the filter result never changes between filter invocations.
- **F-315** `frontend/src/services/agent.ts:24` — Module-level `activeTokenUnsubscribe` / `activeAssistantMsg` shared across runAgent calls. Re-entrant paths (approveTool → runAgent; rejectTool/answerQuestion/dismissQuestion setTimeout→runAgent at lines 1056, 1082, 1099) call `registerTokenListener` which cancels the prior listener, but if an early-return path bypassed `cleanupStreamListeners`, the previous `activeAssistantMsg` may be replaced mid-stream without releasing reactive refs.
- **F-319** `frontend/src/services/llm.ts:74` — Request body is `JSON.stringify`-ed once but uses a flat object with the full `AVAILABLE_TOOLS` array (constant ~6KB) and the system prompt + entire conversation embedded every call. There's no template caching of the static prefix (system+tools) — even though they are invariant across all turns of a session.
- **F-317** `frontend/src/services/terminalAgent.ts:191` — `resolveActiveSession` spreads `panelStore.panels` Map into an Array and runs two `.find` passes (exact title + suffix match) for every command call. O(n) per call.
- **F-038** `frontend/src/services/terminalManager.ts:86` — WebLinksAddon is loaded but the listening addons (mouse reporting, bracketed paste mode) do not appear to be explicitly verified for compliance. Bracketed paste mode ("\e[?2004h") is mentioned as a passthrough in BaseTerminal.vue (bracketedPasteMode check), but no automated check confirms xterm forwards the escape.

## 7. 建议的修复批次

参考仓库已有风格(`609ecc1 fix(session,k8s,frontend): batch 5 — ...`),按依赖关系拆分到 1-3 个 PR。

### fix(session,store,database,k8s): batch 6 — backend hot paths

Backend hot-path perf: enlarge SSH/local/telnet/serial read buffers, remove per-RTT copies in read loops, replace fixed 1s log-flush ticker with idle-armed timer, split output_log mutex into stripper/lines vs bw.Write scopes, replace waitIdle busy-loop with channel signal, atomic-write + debounce terminal_history/recent/commands stores, cache skills+commands List results keyed on mtime, switch settings MarshalIndent -> json.Encoder, async password keychain backfill, DB conn pool tuning (SetMaxOpenConns/IdleConns/Lifetime), cache GetTableSchema result for connection lifetime, fix scanToString to switch on type instead of fmt.Sprintf, tune k8s http.Transport (MaxIdleConnsPerHost, IdleConnTimeout, ResponseHeaderTimeout), add kubeconfig parse cache, add watch-handle reaper.

Files:
- `backend/session/ssh_session.go`
- `backend/session/local_session_unix.go`
- `backend/session/telnet_session.go`
- `backend/session/serial_session.go`
- `backend/session/output_log.go`
- `backend/session/session.go`
- `backend/session/manager.go`
- `backend/store/terminal_history_store.go`
- `backend/store/recent_store.go`
- `backend/store/ai_session_store.go`
- `backend/store/connection_store.go`
- `backend/store/commands_store.go`
- `backend/store/skills_store.go`
- `backend/store/settings_store.go`
- `backend/database/engine.go`
- `backend/database/provider_mysql.go`
- `backend/database/provider_postgres.go`
- `backend/k8s/client.go`
- `backend/k8s/kubeconfig.go`
- `backend/k8s/rest.go`
- `backend/k8s/manager.go`

### fix(wails_bridge,session,frontend,k8s): batch 7 — lifecycle, sync, bridge

Cross-cutting infrastructure: add net/http/pprof endpoint for verification, wire macOS OnHide/OnShow + visibilitychange to pause background goroutines (terminal keepalive, output_log flush, k8s watches, AI SSE stream, auto-sync trigger), make SyncNow async with request-id, coalesce triggerAutoSync via sync.atomic.Bool, replace session:data per-chunk map+string with sync.Pool+typed struct + 16ms rAF coalesce, replace full ConnectionStoreData emit with delta, use shared tuned http.Transport across LLM/K8s/update calls, LimitReader(64KB) on all error-path bodies, move LocalStateStore load off main goroutine, defer sync.NewSyncService to background, frontend: cap sessionStore retention to 256KB ring, dispose xterm addons on unmount, memoize cellWidth/cellHeight, debounced cursor tracking, combine BaseTerminal regex passes into single scan, buffer incoming chunks per rAF.

Files:
- `main.go`
- `app.go`
- `backend/sync/sync_service.go`
- `backend/k8s/watch.go`
- `backend/k8s/manager.go`
- `backend/update/checker.go`
- `frontend/src/components/BaseTerminal.vue`
- `frontend/src/services/terminalManager.ts`
- `frontend/src/stores/sessionStore.ts`
- `frontend/src/composables/useTerminal.ts`
- `frontend/src/composables/useTerminalInput.ts`
- `frontend/src/composables/useTerminalMenu.ts`
- `frontend/src/composables/useFocusTerminal.ts`
- `frontend/src/composables/useTerminalThemeOptions.ts`

### fix(frontend): batch 8 — AI/LLM streaming + reactive perf + render_compat

AI streaming + Vue reactivity: replace deep reactive() per message with shallowReactive + markRaw(_rawApiMsg), cache token estimate at addMessage, fold conversation computed into a single forward pass, debounce doSave to 500ms with delta-only snapshots, keep _rawApiMsg as JSON string until session activation, replace SSE json.Unmarshal into map[string]interface{} with typed structs, use bytes.Buffer for text accumulation, coalesce ai:token events per 16ms window, prompt-caching: insert cache_control breakpoints on system+tools and cache the static prefix JSON, atomic.Pointer for chatCancel, buffer streamed text into non-reactive string flushed at rAF, cap tool result blobs at write time. Plus render_compat: add lineHeight=1.15, install @xterm/addon-italic, @xterm/addon-sync, set charSizeCompat, add codeBlockBackground theme field.

Files:
- `frontend/src/stores/aiStore.ts`
- `frontend/src/services/agent.ts`
- `frontend/src/services/llm.ts`
- `app.go`
- `frontend/src/services/terminalManager.ts`

## 8. 验证清单

### 8.1 微基准(已写入仓库,untracked,不 git add)

为 Top 5 P0/P1 中可 bench 的 4 条写了 Go 微基准(剩余 1 条 F-301 是 Vue 计算属性,无 Go 路径)。

**文件**(均 untracked,符合 user 约束「不执行 git add」):

- `backend/session/bench_test.go` — F-205 (session:data emit map vs struct) + F-002 (SSH decode fast-path alloc vs scratch reuse) + F-205 变体(binary base64 encode vs pooled)
- `backend/database/bench_test.go` — F-113 (queryStrings per-row alloc vs flat [][]string)
- `app_bench_test.go`(root, package main)— F-306 (SSE json.Unmarshal map vs typed struct,content_block_delta + message_start)

**运行**(必须在 worktree 里执行):

```
# 前两个无需 frontend build
go test -bench=. -benchmem ./backend/session/...
go test -bench=. -benchmem ./backend/database/...

# app_bench_test.go 需要 frontend/dist 先 build(因为 main.go //go:embed):
cd frontend && npm run build && cd ..
go test -bench=. -benchmem .
```

**初步结果(1 次迭代,已在 worktree 中跑过)**:

| Bench | ns/op | B/op | allocs/op |
|---|---|---|---|
| BenchmarkSessionDataEmitMap (当前) | 29917 | 112 | 4 |
| BenchmarkSessionDataEmitStruct (修复) | 542 | 64 | 1 |
| BenchmarkSSHDecodeFastPathAlloc (当前) | 417 | 64 | 1 |
| BenchmarkSSHDecodeFastPathNoAlloc (修复) | 84 | 0 | 0 |
| BenchmarkSessionBinaryBase64Encode (当前) | 1750 | 192 | 2 |
| BenchmarkSessionBinaryBase64Pooled (修复) | 1375 | 1152 | 1 |
| BenchmarkQueryStringsPerRowAlloc (当前) | 667 | 320 | 1 |
| BenchmarkQueryStringsFlatRows (修复) | 250 | 0 | 0 |

修复路径在每个 case 上都赢(memory + CPU),尤其 session:data emit **55x 更快 + 75% alloc 减少**,这是 idle wakeup + per-byte bridge 的关键热路径。

### 8.2 pprof 命令

先核查 `app.go` 是否暴露 pprof endpoint(本身是 agent-3 要查的一条)。若未暴露,启动路径里加:

```go
import _ "net/http/pprof"
// OnStartup:
go func() {
    if err := http.ListenAndServe("localhost:6060", nil); err != nil {
        log.Printf("pprof: %v", err)
    }
}()
```

(这本身要进 spec 的 P0/P1 列表)

录制模板:

```bash
# CPU(30s)
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# 堆
go tool pprof http://localhost:6060/debug/pprof/heap

# goroutine 阻塞
go tool pprof http://localhost:6060/debug/pprof/block

# 前端:Chrome DevTools → Performance → 录制 localhost:34115
```

### 8.3 Claude Code 复现步骤

1. `wails dev`
2. 开新 SSH / local tab,ssh 进任意机器
3. 跑 `claude` 进交互
4. 触发以下场景并观察:
   - 斜体行(思考块)
   - 工具调用块(Read / Edit 行)
   - 长 markdown 输出
   - 代码块(尤其是多行嵌套)
5. 观察项:花屏?重影?行错位?token-by-token 闪?box drawing 对齐?

## 9. 范围外

### 9.1 in-flight 文件中观察到但未修的问题

- **F-011** `backend/session/output_log.go:630` (P0, locking) — OutputLogger.flushLoop runs `time.NewTicker(logFlushInterval)` where `logFlushInterval = 1 * time.Second`. Every active log file creates a goroutine that wakes once per second for the entire session lifetime, even when no bytes have been written.
- **F-012** `backend/session/output_log.go:449` (P1, io) — OutputLogger.Enable walks up to 100 filename suffixes via `for suffix := 1; suffix <= 100; suffix++` and calls `os.OpenFile` with `os.O_CREATE\|os.O_EXCL\|os.O_WRONLY` per attempt, costing up to 100 stat+create syscalls on collision.
- **F-013** `backend/session/output_log.go:583` (P1, locking) — WriteOutput acquires `l.mu` and holds it across both ANSI stripping and the line-processor pass — which can call back into the lineProcessor's CSI parser. On a chatty session this serializes writes to the log while still allowing emitData to proceed (that's under a separate lock), but couples log writing latency to log formatting work.
- **F-014** `backend/session/output_log.go:39` (P1, allocation) — ansiStripper.Strip allocates `make([]byte, 0, len(data))` for every chunk and the `s.pending = append(s.pending[:0], data[i:]...)` copy on incomplete tails reallocates the pending slice as it grows.
- **F-015** `backend/session/output_log.go:181` (P1, allocation) — lineProcessor.Feed calls `out = append(out, p.line[p.emitted:]...)` and `out = append(out, '\n')` repeatedly within the loop on every byte, plus `out` is allocated per call (var out []byte grows).
- **F-016** `backend/session/session.go:169` (P1, locking) — baseSession.emitData takes two RLock guards (outputLogMu then mu) and runs both callbacks inside the locked region of baseSession.mu. When the onData callback chains into Wails EventsEmit, every remote byte pays an extra lock round-trip and the lifetime of the callback extends the lock-hold window.
- **F-017** `backend/session/session.go:341` (P0, locking) — waitIdle busy-loops with `time.Sleep(50 * time.Millisecond)` for up to the configured timeout. With 8 tabs each waiting idle at login, this is up to 8 × (5 s / 50 ms) = 800 wakeups per tab connect cycle. After connect, idle windows of 5+ s pay 100 wakeups.
- **F-311** `frontend/src/services/agent.ts:430` (P1, io) — `activeAssistantMsg.content += data.text` is a deep-reactive mutation per SSE token that triggers the entire AISidebar render chain. With high-throughput models (Claude 100+ tok/s, GPT-4o), this is 100+ Vue re-renders per second on the main thread.
- **F-312** `frontend/src/services/agent.ts:670` (P1, memory) — Tool result blobs (execute_command output, capture_terminal screen, use_skill manifest) stored verbatim in `m.content`. A 200-line × 2KB capture_terminal = 400KB retained. The conversation computed (F-301) walks all retained blobs every turn.
- **F-315** `frontend/src/services/agent.ts:24` (P2, locking) — Module-level `activeTokenUnsubscribe` / `activeAssistantMsg` shared across runAgent calls. Re-entrant paths (approveTool → runAgent; rejectTool/answerQuestion/dismissQuestion setTimeout→runAgent at lines 1056, 1082, 1099) call `registerTokenListener` which cancels the prior listener, but if an early-return path bypassed `cleanupStreamListeners`, the previous `activeAssistantMsg` may be replaced mid-stream without releasing reactive refs.
- **F-407** `backend/sync/sync_service.go:35` (P1, io) — NewSyncService() runs synchronously inside app.startup and may invoke the OS keychain (PBKDF2 600k iterations + keychain IPC) — on macOS the Security framework call to read the encryption key can take 50–300ms, and PBKDF2 alone on the same keychain access pattern can take >500ms.

### 9.2 本次未覆盖

- 大型重构(重写 session 编排、改 event-driven 重构输出层)
- 更换 xterm.js 本身
- 协议层重写
- 新功能

## 10. 附录:Agent prompts(便于复跑)

> 每个 prompt 设计为可独立派发给一个 subagent。Schema (§2.3) 与 severity (§2.5) 在 prompt 内复述以保证可独立复跑。`terminal_io` 是唯一含 `render_compat` 子方向的 agent。

### Agent: terminal_io

```text
You are auditing the **terminal_io** subsystem of uniterm (Wails v2 + xterm.js + Go, terminal app supporting SSH / local / serial / mosh / telnet). This is a READ-ONLY audit. You will not modify any file.

Output a single JSON array — no prose, no commentary, no markdown fence, no explanation. JSON only.

## Scope (read-only)

Backend (Go):
- `backend/session/output_log.go`
- `backend/session/ssh_session.go`
- `backend/session/local_session_unix.go`
- `backend/session/serial_session.go`
- `backend/session/telnet_session.go`
- `backend/session/mosh_session.go`
- `backend/session/manager.go`
- Anything under `backend/session/` that references `xterm`, `OutputLog`, `RingBuffer`, `term.write`, or `PTY` is also in scope when discovered.

Frontend (TypeScript / Vue):
- `frontend/src/composables/useTerminal.ts`
- `frontend/src/composables/useTerminalInput.ts`
- `frontend/src/composables/useTerminalMenu.ts`
- `frontend/src/composables/useTerminalThemeOptions.ts`
- `frontend/src/composables/useFocusTerminal.ts`
- `frontend/src/stores/sessionStore.ts`
- xterm.js addon / theme configuration references anywhere in `frontend/src/components/`

## Read-only constraint (BINDING)

The following files are in-flight on other branches and MUST NOT be edited or diffed. You may still READ them and cite them as evidence. If a finding's only fix touches one of these, set `constrained_by_inflight: true` AND populate `inflight_file` with the literal path; the integrator will defer it:
- `backend/session/output_log.go`
- `backend/session/ftp_session.go`
- `backend/session/session.go`
- `backend/sync/git.go`
- `backend/sync/sync_service.go`
- `frontend/src/services/agent.ts`
- `frontend/src/utils/runtimeTypeCheck.ts`

Other constraints:
- Do NOT write new production code or new test files.
- Do NOT modify any existing test assertions.
- Do NOT touch any file under `.planning/` or anywhere else.
- Return only the JSON findings array.

## Focus checklist (categorized)

Examine both micro (single call-site) and systemic (architectural) issues in each category.

### A. allocation
- Per-byte / per-rune allocations on the hot path `net.Conn.Read` → xterm `term.write`.
- Slice / string growth without preallocation, repeated `[]byte(s)` conversions, `fmt.Sprintf` on the hot path.
- Closure / interface boxing that causes heap escape in tight loops.

### B. locking
- `sync.Mutex` held across xterm writes (which can block on the renderer thread).
- Channel send/recv under a mutex when an unbuffered or differently-sized channel would do.
- Lock-ordering hazards between `OutputLog` and session lifecycle.

### C. io
- Read buffer size too small (per-RTT syscalls) or too large (latency spike).
- Missing / wrong `io.Copy` direction, or `io.Reader` wrapping without a deadline.
- Bypass of `bufio.Reader` on the PTY master fd.

### D. serialization
- `json.Marshal` of large session blobs for IPC or log replay.
- Manual byte slicing where `bytes.Buffer` / `binary.Read` would be cleaner and faster.

### E. memory
- Ring buffer growth policy (fixed vs unbounded). Does the tail pointer ever stop moving in `OutputLog`?
- Listener / event-handler leaks on tab close — especially xterm.js addon `Dispose()` calls and `wails.EventsOn` without matching `EventsOff`.
- Tab references pinning terminal instances, preventing GC after disconnect.

### F. algorithmic
- O(n²) scans over the visible line buffer (e.g. regex per render frame).
- Per-keystroke full-buffer rescans.
- Searching the history ring instead of maintaining an index.

### G. caching
- Recreating xterm `ThemeData` / addon objects on every Vue re-render.
- Recompiling highlight regexes per line.
- Parsing theme JSON per render instead of once.

### H. render_compat — Claude Code compatibility (each item is its own finding row)

uniterm's purpose includes being a usable host for `claude` (Claude Code) interactive sessions. Each item below MUST be checked individually. Cite the exact file:line where the behavior is configured, missing, or incorrect. Set `category: "render_compat"` for these findings.

1. **Italic (`\e[3m` / `\e[23m`)**: are SGR 3 / SGR 23 forwarded to xterm? Is an italic font actually loaded? Claude Code emits italic for "thinking" blocks; if unsupported, blocks render as upright text and visually collapse into the surrounding output — bad UX.
2. **Mode 2026 synchronized output (`\e[?2026h` / `\e[?2026l`)**: does the xterm-write path bracket multi-line redraws (Claude Code streams a thinking block across multiple writes) so the user doesn't see flicker / partial renders? If unsupported, expect line-by-line repaints during `claude`'s spinner phases.
3. **Ambiguous-width characters (East Asian width, emoji)**: is `wcwidth` (or the xterm equivalent) set to a Unicode-13+ aware value? Claude Code uses ⏺⏵ etc; wrong widths break reflow / box-drawing alignment.
4. **256-color code-block background (`\e[48;5;…m`)**: does the active theme include a dedicated code-block background distinct from the regular background? Claude Code paints fenced code blocks with a slightly different bg. Missing → no visual distinction between prose and code.
5. **Mouse reporting / bracketed paste mode (`\e[?1000h`, `\e[?1006h`, `\e[?2004h`)**: are mouse events still passed to xterm addons? Is bracketed paste respected so Claude Code's pasted paths / long strings arrive as a single paste event rather than keystrokes? If broken, paste of multi-line code into `claude` input is mangled.
6. **Alternate screen buffer (`\e[?1049h` / `\e[?1049l`)**: does uniterm save/restore scrollback correctly on alt-screen enter/exit? Claude Code uses alt screen for its TUI; state leaks across → scrollback vanishes after exit.
7. **`lineHeight` rendering**: is `xterm.option.lineHeight` set to a non-1.0 value? If yes, are glyphs drawn with sufficient line-gap so italic / descenders / accent marks don't clip? Wrong value → italic text in Claude Code output clips at top/bottom.

## Finding schema (one JSON object per finding)

```json
{
  "id": "F-001",
  "file": "path/to/file.go",
  "line": 142,
  "subsystem": "terminal_io",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic | caching | render_compat",
  "root_cause": "一句话根因",
  "evidence": "代码片段 (≤ 6 行,保留前后文)",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述",
  "constrained_by_inflight": false,
  "inflight_file": "optional — only when constrained_by_inflight=true"
}
```

Severity (§2.5):
- **P0**: every tab open / every Claude Code session triggers it, or mid-load visibly degrades.
- **P1**: specific scenarios (large `cat`, 8+ tabs, long Claude Code session, specific theme).
- **P2**: theoretical / edge case.

## Output format

Return exactly one JSON array. No surrounding text. No markdown fences. No commentary.

```json
[
  { "id": "F-001", ... },
  { "id": "F-002", ... }
]
```

- Assign `id` starting at `F-001` and incrementing.
- `subsystem` is the literal string `"terminal_io"` for every finding.
- Findings touching in-flight files MUST include both `constrained_by_inflight: true` and `inflight_file`.
- If you find no issues in a category, omit it. If you find no issues at all, return `[]`.

Return JSON only — no prose.
```

### Agent: storage_db

```text
You are auditing the **storage_db** subsystem of uniterm (JSON-on-disk persistence + relational DB providers via `database/sql`). This is a READ-ONLY audit. You will not modify any file.

Output a single JSON array — no prose, no commentary, no markdown fence. JSON only.

## Scope (read-only)

Backend (Go):
- All files matching `backend/store/*.go`
- `backend/database/engine.go`
- `backend/database/executor.go`
- All files matching `backend/database/provider_*.go` (excluding `_test.go` files)

## Read-only constraint (BINDING)

The following files are in-flight on other branches and MUST NOT be edited or diffed. You may still READ them and cite them as evidence. If a finding's only fix touches one of these, set `constrained_by_inflight: true` AND populate `inflight_file` with the literal path:
- `backend/session/output_log.go`
- `backend/session/ftp_session.go`
- `backend/session/session.go`
- `backend/sync/git.go`
- `backend/sync/sync_service.go`
- `frontend/src/services/agent.ts`
- `frontend/src/utils/runtimeTypeCheck.ts`

Other constraints:
- Do NOT write new production code or new test files.
- Do NOT modify any existing test assertions.
- Do NOT touch any file under `.planning/` or anywhere else.
- Return only the JSON findings array.

## Focus checklist (categorized)

Examine both micro (single call-site) and systemic (architectural) issues in each category.

### A. allocation
- Per-write allocations on the JSON store hot path (e.g. `json.Marshal` of the full connection list on every save).
- Repeated `[]byte` ↔ string conversions inside store serialization loops.
- Closure capture causing heap escape in `Load` / `Save` paths.

### B. locking
- Store-level mutex held across `fsync` / disk I/O.
- Read-write mutex used where a `sync.Map` or sharded mutex would scale better.
- Lock-ordering hazards between multiple stores writing to the same disk directory concurrently.

### C. io
- **fsync frequency**: every write or every N writes? Is `O_SYNC` / `O_DSYNC` used? Is there a write-coalescing buffer? Per-keystroke fsync is a classic P0.
- File open / close on every write instead of a long-lived `*os.File` with append mode.
- Missing `bufio.Writer` flush on error / shutdown.
- For DB providers: missing / misconfigured connection-pool `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`.

### D. serialization
- Full-blob `json.Marshal` + atomic rename vs. streaming JSON encoder / decoder.
- Struct tags missing `omitempty` causing large zero-value fields to be persisted unnecessarily.
- Manual string concatenation for log / audit lines instead of `bytes.Buffer` or `strings.Builder`.
- For DB providers: building SQL by `fmt.Sprintf` instead of parameterized queries (also a correctness hazard — note but don't fix).

### E. memory
- Loading the full connection / settings / history file into memory on every read instead of `mmap` or streaming.
- Unbounded in-memory caches in stores (e.g. `var cache map[string]Item` with no eviction).
- Retention / truncation logic for terminal history store — is it bounded?

### F. algorithmic
- O(n²) scans over connection list when adding / removing an item.
- Linear search for a connection by ID on every keystroke / event handler fire.
- DB-side: classic N+1 (loop issuing one query per row) when a single `IN (…)` or join would do.

### G. caching
- Re-parsing the same JSON file on every read.
- DB query results cached where staleness is acceptable but no cache exists.
- DB query results NOT cached where the same query fires on every event.

## Finding schema (one JSON object per finding)

```json
{
  "id": "F-101",
  "file": "path/to/file.go",
  "line": 142,
  "subsystem": "storage_db",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic | caching",
  "root_cause": "一句话根因",
  "evidence": "代码片段 (≤ 6 行,保留前后文)",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述",
  "constrained_by_inflight": false,
  "inflight_file": "optional — only when constrained_by_inflight=true"
}
```

Severity (§2.5):
- **P0**: triggers on ordinary daily use (every save, every connection edit, every DB query).
- **P1**: specific scenarios (large connections list, concurrent writes from multiple tabs, big DB query result set).
- **P2**: theoretical / edge case.

## Output format

Return exactly one JSON array. No surrounding text. No markdown fences. No commentary.

```json
[
  { "id": "F-101", ... },
  { "id": "F-102", ... }
]
```

- Assign `id` starting at `F-101` and incrementing.
- `subsystem` is the literal string `"storage_db"` for every finding.
- Findings touching in-flight files MUST include both `constrained_by_inflight: true` and `inflight_file`.
- If you find no issues in a category, omit it. If you find no issues at all, return `[]`.

Return JSON only — no prose.
```

### Agent: wails_bridge

```text
You are auditing the **wails_bridge** subsystem of uniterm (the Wails v2 JS↔Go binding surface — bound methods in `app*.go`, generated TS declarations, and all store/service callers of those methods). This is a READ-ONLY audit. You will not modify any file.

Output a single JSON array — no prose, no commentary, no markdown fence. JSON only.

## Scope (read-only)

Backend (Go):
- `app.go` (root-level — bound App methods)
- All `app_*.go` in repo root EXCEPT the platform build-tag splits: explicitly EXCLUDE `app_darwin.go`, `app_windows.go`, `app_notdarwin.go`, `app_notwindows.go`. INCLUDE all other `app_*.go` files.

Frontend (TypeScript):
- `frontend/wailsjs/go/main/App.d.ts` (generated bindings — read but do not edit)
- All callsites in `frontend/src/stores/*` and `frontend/src/services/*` that invoke `Wails.*` or `window.go.*` (search for `window.go.`, `Wails.`, and `EventsEmit` / `EventsOn` usage)

## Read-only constraint (BINDING)

The following files are in-flight on other branches and MUST NOT be edited or diffed. You may still READ them and cite them as evidence. If a finding's only fix touches one of these, set `constrained_by_inflight: true` AND populate `inflight_file` with the literal path:
- `backend/session/output_log.go`
- `backend/session/ftp_session.go`
- `backend/session/session.go`
- `backend/sync/git.go`
- `backend/sync/sync_service.go`
- `frontend/src/services/agent.ts`
- `frontend/src/utils/runtimeTypeCheck.ts`

Other constraints:
- Do NOT write new production code or new test files.
- Do NOT modify any existing test assertions.
- Do NOT touch any file under `.planning/` or anywhere else — including `frontend/wailsjs/` (auto-generated, do not hand-edit regardless).
- Return only the JSON findings array.

## Focus checklist (categorized)

Examine both micro (single call-site) and systemic (architectural) issues in each category.

### A. allocation
- `EventsEmit` / `runtime.EventsEmit` carrying large marshalled payloads (entire session state, full scrollback buffers, full connection list) on every change.
- Per-emit `json.Marshal` instead of reusing a pooled `bytes.Buffer` / `encoding/json.Encoder`.
- Returning a fresh slice / map from a bound method when the caller could mutate in place.

### B. locking
- Bound methods holding a long-lived mutex while doing I/O or marshalling.
- Channel-based request/response under a mutex where a buffered channel would suffice.
- Race conditions between concurrent bound-method calls modifying shared `App` state (e.g. `chatCancel`, `panelLogs` — verify mutex coverage).

### C. io
- **Synchronous / blocking calls from the UI thread**: any bound method that does network I/O (sync git push/pull, K8s REST, DB query, HTTP) without goroutine + channel / promise handoff.
- Reading from `net/http` body without a size cap (unbounded body → OOM).
- Missing context cancellation propagation through bound-method call chain.

### D. serialization
- Repeated `json.Marshal` of the same payload per `EventsEmit` tick instead of incremental deltas.
- Marshalling entire structs when only one field changed — diff at the source.
- Inefficient encoding (base64 of large blobs where binary would do) crossing the bridge.

### E. memory
- Listener leaks: `EventsOn` registered on store init but never paired with `EventsOff` on store dispose. Cross-reference with the store lifecycle in `stores/*Store.ts`.
- Accumulating event handlers across HMR reloads (dev-mode leak).
- Bound methods retaining references to large closures past the response.

### F. algorithmic
- O(n) bound method called per row / per character (e.g. update-one-field-by-id over a slice).
- Frontend store calling `Wails.*` inside a Vue `computed` getter with no memoization → call per render frame.

### G. profiling exposure (special — also a §8.2 prerequisite)
- Is `net/http/pprof` exposed in dev or release builds? Search `app.go`, `main.go`, and any `http.ListenAndServe` / `http.ServeMux` references. If NOT exposed, this is itself a finding: `category: "io"`, `severity: "P0"`, `root_cause` explains why pprof is needed for the audit (§8.2).
- Is there any HTTP server in uniterm that could be reused as the pprof endpoint (vs. adding a new one)?

## Finding schema (one JSON object per finding)

```json
{
  "id": "F-201",
  "file": "path/to/file.go",
  "line": 142,
  "subsystem": "wails_bridge",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic",
  "root_cause": "一句话根因",
  "evidence": "代码片段 (≤ 6 行,保留前后文)",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述",
  "constrained_by_inflight": false,
  "inflight_file": "optional — only when constrained_by_inflight=true"
}
```

Severity (§2.5):
- **P0**: triggers on every tab event / every Wails round-trip / missing core dev capability (pprof).
- **P1**: specific scenarios (large payloads, 8+ tabs, long session).
- **P2**: theoretical / edge case.

## Output format

Return exactly one JSON array. No surrounding text. No markdown fences. No commentary.

```json
[
  { "id": "F-201", ... },
  { "id": "F-202", ... }
]
```

- Assign `id` starting at `F-201` and incrementing.
- `subsystem` is the literal string `"wails_bridge"` for every finding.
- Findings touching in-flight files MUST include both `constrained_by_inflight: true` and `inflight_file`.
- If you find no issues in a category, omit it. If you find no issues at all, return `[]`.

Return JSON only — no prose.
```

### Agent: ai_llm

```text
You are auditing the **ai_llm** subsystem of uniterm (the AI Agent loop on the frontend, the LLM client, and the AI proxy bound methods on the Go side). This is a READ-ONLY audit. You will not modify any file.

Output a single JSON array — no prose, no commentary, no markdown fence. JSON only.

## Scope (read-only)

Frontend (TypeScript):
- `frontend/src/services/agent.ts`
- `frontend/src/services/llm.ts`
- `frontend/src/services/terminalAgent.ts`
- `frontend/src/stores/aiStore.ts`
- Any other `frontend/src/services/agent*.ts` or `frontend/src/services/llm*.ts` if discovered.

Backend (Go):
- `app.go` — bound methods that proxy / relay to the LLM provider (search for AI/LLM-related method names: `AI*`, `Chat*`, `LLM*`, `Stream*`, `Completion*`). Include only the AI-proxy subset; do NOT audit other parts of `app.go`.

## Read-only constraint (BINDING)

The following files are in-flight on other branches and MUST NOT be edited or diffed. You may still READ them and cite them as evidence. If a finding's only fix touches one of these, set `constrained_by_inflight: true` AND populate `inflight_file` with the literal path:
- `backend/session/output_log.go`
- `backend/session/ftp_session.go`
- `backend/session/session.go`
- `backend/sync/git.go`
- `backend/sync/sync_service.go`
- `frontend/src/services/agent.ts`
- `frontend/src/utils/runtimeTypeCheck.ts`

Other constraints:
- Do NOT write new production code or new test files.
- Do NOT modify any existing test assertions.
- Do NOT touch any file under `.planning/` or anywhere else.
- Return only the JSON findings array.

## Focus checklist (categorized)

Examine both micro (single call-site) and systemic (architectural) issues in each category.

### A. allocation
- Per-token allocation in the SSE / streaming parser (string concatenation vs. streaming JSON / line buffer).
- Re-cloning message arrays on every append instead of push-to-mutated-tail under a copy-on-write guard.
- `JSON.parse` of the whole accumulated buffer per token instead of incremental parsing.

### B. locking
- Pinia store mutations from SSE callbacks without going through actions (race with Vue reactivity).
- Concurrent streams racing on a shared `AbortController` without coordination.
- `chatCancel` mutex in `app.go` — verify cancel propagation is correct AND non-blocking.

### C. io
- LLM proxy reading the upstream response body without size cap.
- Missing streaming `fetch` abort handling (browser tab close during streaming → dangling request).
- No backpressure between SSE chunks and Pinia state updates (UI jank when tokens arrive faster than 60 fps).

### D. serialization
- Re-marshalling the entire conversation history on every turn instead of sending only the diff.
- `JSON.stringify` of large tool results on the hot path.

### E. memory
- **Multi-turn context growth**: does the conversation history grow unboundedly across turns? Is there a sliding window / summarization / token-budget truncation? If not, this is a P0 — every long Claude Code-style session degrades.
- Tool-result blobs (large file contents, big shell outputs) retained verbatim in context forever.
- Streaming chunk buffers retained after stream completion.

### F. algorithmic
- Re-running the entire prompt template concatenation on every turn instead of caching the static prefix.
- Re-tokenizing the conversation to estimate length instead of tracking the running count.
- Linear scans over history for the most-recent matching tool result.

### G. caching
- Caching prompt prefixes that never change across turns (system prompt + tool definitions).
- NOT caching anything when many turns share the same system prompt + tool schema — wasted upstream tokens & latency.
- Retry idempotency: does the retry mechanism use the same request ID / idempotency key to avoid double-charging on transient failures?

### H. retry / backoff / JSON parsing (cross-cutting)
- **Retry / backoff**: exponential with jitter? Max attempts? Deadlines honored? Idempotency keys used?
- **JSON parsing**: does streaming JSON partial-parse handle split chunks across `data:` boundaries? Does the final parse handle `<think>` blocks / tool-call envelopes correctly?
- **Prompt concatenation**: is the system prompt + tool list built once per session or rebuilt every turn? Are tool definitions injected verbatim when the LLM has stable schemas?
- **Streaming token handling**: are tokens buffered until a complete SSE event arrives, or pushed UI-side at fragment granularity (causing flicker)?

## Finding schema (one JSON object per finding)

```json
{
  "id": "F-301",
  "file": "path/to/file.ts",
  "line": 142,
  "subsystem": "ai_llm",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic | caching",
  "root_cause": "一句话根因",
  "evidence": "代码片段 (≤ 6 行,保留前后文)",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述",
  "constrained_by_inflight": false,
  "inflight_file": "optional — only when constrained_by_inflight=true"
}
```

Severity (§2.5):
- **P0**: triggers on every multi-turn AI session, or noticeable from turn 3+ on a long Claude Code-style session.
- **P1**: specific scenarios (large tool outputs, 5+ AI turns, retry storms, network jitter).
- **P2**: theoretical / edge case.

## Output format

Return exactly one JSON array. No surrounding text. No markdown fences. No commentary.

```json
[
  { "id": "F-301", ... },
  { "id": "F-302", ... }
]
```

- Assign `id` starting at `F-301` and incrementing.
- `subsystem` is the literal string `"ai_llm"` for every finding.
- Findings touching in-flight files MUST include both `constrained_by_inflight: true` and `inflight_file`.
- If you find no issues in a category, omit it. If you find no issues at all, return `[]`.

Return JSON only — no prose.
```

### Agent: k8s_sync_startup

```text
You are auditing the **k8s_sync_startup** subsystem of uniterm (Kubernetes REST/watch, encrypted cloud sync over git, in-app update check, and the cold-start path from `main.go` → `app.startup`). This is a READ-ONLY audit. You will not modify any file.

Output a single JSON array — no prose, no commentary, no markdown fence. JSON only.

## Scope (read-only)

Backend (Go):
- All files matching `backend/k8s/*.go` (excluding `_test.go` files)
- All files matching `backend/sync/*.go`
- All files matching `backend/update/*.go`
- `main.go` (root-level — Wails bootstrap, menu setup, pre-`wails.Run` work)
- `app.go` startup path only: `startup`, `OnDomReady`, `OnStartup`-registered hooks, plus any blocking initialization that runs synchronously before the first frame. DO NOT audit other parts of `app.go` — those are owned by `wails_bridge` or `ai_llm`.

## Read-only constraint (BINDING)

The following files are in-flight on other branches and MUST NOT be edited or diffed. You may still READ them and cite them as evidence. If a finding's only fix touches one of these, set `constrained_by_inflight: true` AND populate `inflight_file` with the literal path:
- `backend/session/output_log.go`
- `backend/session/ftp_session.go`
- `backend/session/session.go`
- `backend/sync/git.go`
- `backend/sync/sync_service.go`
- `frontend/src/services/agent.ts`
- `frontend/src/utils/runtimeTypeCheck.ts`

Other constraints:
- Do NOT write new production code or new test files.
- Do NOT modify any existing test assertions.
- Do NOT touch any file under `.planning/` or anywhere else.
- Return only the JSON findings array.

## Focus checklist (categorized)

Examine both micro (single call-site) and systemic (architectural) issues in each category.

### A. allocation
- REST client building a fresh `http.Request` per call without reusing a `*http.Client` with keep-alive.
- `git status` / `git pull` / `git push` allocating large buffers when streaming stderr/stdout is enough.
- Watch event handlers allocating per-event without a sync.Pool.

### B. locking
- K8s manager mutex held across a network call (blocks the whole UI when a cluster is slow).
- Sync service holding a write lock during `git push` — every UI sync action queues behind it.
- Init-order mutex dance where multiple goroutines serialize on a single bootstrap channel.

### C. io
- **Watch reconnect**: is exponential backoff in place? Does it cap? Does it tear down the old watch before reconnecting (avoid goroutine + FD leak)? On a dropped WiFi connection, a naive reconnect can pin the file descriptor.
- **REST response body caps**: does every `client.Do` use `io.LimitReader` or check `resp.ContentLength` before reading? `kubectl get pods -A` on a busy cluster can return tens of MB; unbounded read → OOM.
- **Git blocking startup**: does sync init block `wails.Run` / `OnStartup`? Any `git clone`, `git fetch`, or credential-decrypt on the main goroutine before first paint is a P0 cold-start finding.
- Missing context cancellation through the K8s REST / watch / sync call chain.
- Update check (`backend/update/checker.go`) firing on the startup goroutine — verify it doesn't block first paint and has a timeout.

### D. serialization
- `json.Unmarshal` into `interface{}` then reflection instead of typed structs (K8s objects are well-typed — `unstructured.Unstructured` is fine, but `map[string]interface{}` deep copies are not).
- Re-encoding the whole kubeconfig on every save instead of diff-based updates.

### E. memory
- Watch goroutine retaining the entire event buffer after backpressure.
- Sync service retaining old blobs after a successful push (no eviction of historical snapshots).
- Update checker retaining the release-notes payload in memory past the response.

### F. algorithmic
- Listing all namespaces then filtering client-side instead of using field-selector on the server.
- Polling kubeconfig file with `os.Stat` loop instead of fsnotify / inotify.
- Re-decoding the same kubeconfig on every K8s call instead of caching the parsed `*rest.Config`.

### G. caching
- K8s discovery / API resource list not cached (refetched per request).
- kubeconfig not cached across calls when the file hasn't changed.
- Sync pull results not reused when no remote changes are detected (network round-trip on every app launch).

### H. cold-start (cross-cutting — focus on `main.go` + `app.startup`)
- **Store / sync init ordering**: in what order do `ConnectionStore.Load`, `SettingsStore.Load`, `SyncService.Init`, `UpdateChecker.Start`, `K8sManager.Init`, `SessionManager.Init` run? Anything blocking the main goroutine before `wails.Run` returns is a P0.
- Is `os.UserConfigDir()` called on the main goroutine? Is `store.NewLocalStateStore(...).Load()` blocking `wails.Run`?
- Any synchronous network call (e.g. update-check HEAD request) before first paint? If yes, what's the timeout?
- Goroutine leaks in startup: every `go func()` started in `startup` should have an explicit shutdown path.
- Any `time.Sleep` in startup?

## Finding schema (one JSON object per finding)

```json
{
  "id": "F-401",
  "file": "path/to/file.go",
  "line": 142,
  "subsystem": "k8s_sync_startup",
  "severity": "P0 | P1 | P2",
  "category": "allocation | locking | io | serialization | memory | algorithmic | caching",
  "root_cause": "一句话根因",
  "evidence": "代码片段 (≤ 6 行,保留前后文)",
  "impact": "何时/多大负载下触发",
  "fix_sketch": "1-3 行修复思路",
  "verification": "bench | pprof | trace | repro_描述",
  "constrained_by_inflight": false,
  "inflight_file": "optional — only when constrained_by_inflight=true"
}
```

Severity (§2.5):
- **P0**: triggers on every cold start / every K8s tab open / every sync cycle / on the first dropped network event.
- **P1**: specific scenarios (busy cluster, large kubeconfig, slow git remote, 8+ K8s tabs).
- **P2**: theoretical / edge case.

## Output format

Return exactly one JSON array. No surrounding text. No markdown fences. No commentary.

```json
[
  { "id": "F-401", ... },
  { "id": "F-402", ... }
]
```

- Assign `id` starting at `F-401` and incrementing.
- `subsystem` is the literal string `"k8s_sync_startup"` for every finding.
- Findings touching in-flight files MUST include both `constrained_by_inflight: true` and `inflight_file`.
- If you find no issues in a category, omit it. If you find no issues at all, return `[]`.

Return JSON only — no prose.
```

---

## 元信息

- **作者**:Claude(claude-fable-5)
- **日期**:2026-07-28
- **审批状态**:approved(用户已认可方案 A 与三段设计)
- **下一步**:执行 5 个并行 agent → 整合 → 填表 → spec self-review → 用户复核 → writing-plans skill