---
title: 性能瓶颈与 Claude Code 兼容性分析
date: 2026-07-28
status: approved
scope: whole-app-sweep + claude-code-render-compat
---

# 性能瓶颈与 Claude Code 兼容性分析

## 1. 摘要

对 uniterm 全栈做一次**性能瓶颈 + Claude Code 渲染兼容性**审计,产出发现汇总文档、优先级化的修复批次建议,以及用于复现 / 验证的微基准与 pprof 命令清单。**不修改任何生产代码**。

Top 5 P0 在审计完成后填入;当前为占位(将由整合 agent 根据真实 findings 写入)。

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

执行完成后填入。模板:

| id | file:line | subsystem | severity | category | root_cause(短) |
|---|---|---|---|---|---|

## 4. P0 详述

执行完成后填入。每条 P0:根因 / 证据 / 影响 / 修复 / 验证。

## 5. P1 详述

执行完成后填入(密度更高)。

## 6. P2 列表

执行完成后填入(一句话一条)。

## 7. 建议的修复批次

参考仓库已有风格(`609ecc1 fix(session,k8s,frontend): batch 5 — ...`),按依赖关系拆分到 1-3 个 PR。

执行完成后填入。

## 8. 验证清单

### 8.1 微基准命令

```
# P0/P1 候选会写进对应包的 bench_test.go(新文件,不 git add)
go test -bench=. -benchmem ./backend/session/...
go test -bench=. -benchmem ./backend/store/...
go test -bench=. -benchmem ./backend/database/...
```

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

执行完成后填入。逐条列出 in-flight 文件中本应修但因 in-flight 限制未修的问题。

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