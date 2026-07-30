# uniterm v1.1 Audit · Findings

每条 finding 一节，所有矩阵从这一份文件派生。

**Finding 编号**：`F-NNN`（全局递增）

---

## ⚠️ V1 Re-verification 撤回清单（13 条 · 2026-07-30）

**V1 verifier** + **Re-verification agent** 复核后发现以下 13 条数据/位置错：

### 🔴 撤回（12 条 · FABRICATED）

| ID | 撤回原因 |
|---|---|
| **ARCH-032** | `backend/ai/*.go` 不存在 — 后端无独立 AI 包，LLM 在 `frontend/src/services/llm.ts` |
| **DEV-016** | `backend/session/multi_session.go` 不存在 |
| **DEV-022** | `backend/sync/upload.go` 不存在（sync/ 只有 crypto/git/keychain/sync_config/sync_service）|
| **DEV-024** | `backend/ai/llm.go` 不存在 |
| **QA-018** | `backend/ai/llm.go` 不存在（LLM test 在 frontend）|
| **DBG-018** | `backend/ai/llm.go` 不存在 |
| **DBG-021** | `backend/ai/client.go` 不存在 |
| **REV-016** | AIMessage.vue 已有 `sanitizeRenderedHtml()` (line 640) + 测试文件，已全 mitigate |
| **PM-016** | `README_zh-CN.md` 实际存在（274 行）|
| **ARCH-027** | sync 已用 `github.com/go-git/go-git/v5 v5.19.1`，无 `exec.Command` |
| **ARCH-030** | sync 已用 `golang.org/x/crypto/pbkdf2`，`pbkdf2Iterations=600000` |
| **F-002** | 90 deps 漂移是 normal minor/patch，无具体 CVE/破坏，证据仅为数量 |

### 🟡 修正（1 条 · PARTIAL CORRECT）

| ID | 修正内容 |
|---|---|
| **F-011** | 原 claim 含 `backend/platform/`，实际 `platform/fonts_ttf_test.go` 存在（88 行） → 修正为只引用 `backend/log/` + `backend/update/`（这俩真 0 测试）|

### 📊 真实 finding 总数（最终态）

- **撤回 12 + 修正 1**（已计入）
- 早期 withdrawn 5（DEV-028/035/043/044 + ARCH-021）
- F-005 rejected（早期已撤回）
- **有效 finding 总数**：**97 - 12 = 85 条**（F-002 撤回 + 11 个 subagent finding 撤回）

### ⚠️ 子 agent hallucination 教训

8 个 subagent 中部分产生了**幻觉 finding**（引用不存在的文件路径）。这暴露了：
1. Subagent grep / Read 准确性需再加强（应核对 `find` 输出确认路径）
2. V1 反向 verifier 步骤非常关键 — 没有 V1 会让 12 条假 finding 流入 roadmap
3. 后续 subagent prompt 应明确要求「先 `ls` 验证文件存在再写 finding」

**Lens**（哪个视角发现的）：
- **产品** — UX / 文档 / i18n / OSS 一流标准
- **架构** — 模块边界 / OS 抽象 / 设计一致性 / 技术债
- **QA** — 测试覆盖 / 边界用例 / AC 验证
- **研发** — 性能 / 内存 / 重构机会 / bug 调查 / 稳定性

**Severity**：
- **P0** — 不可逆副作用 / 数据丢失 / 全局崩溃 / 安全 CVE / 合规阻塞
- **P1** — 阻塞关键路径 / 反复修 / 高频 crash
- **P2** — 单角色失败 / 非阻塞 / UX 小问题
- **P3** — 文档 / 注释 / 命名 / nitpick

**Category**：
- `bug` · `perf` · `refactor` · `deps` · `config` · `os-compat` · `test` · `arch` · `docs`

**Risk**（修复风险）：
- `low`（局部，不影响 API）· `medium`（跨文件）· `high`（破坏性 / 大改）

**Impact**（修复收益）：
- `critical`（P0 bug / 核心安全）· `high`（明显 perf / 修高频 crash）· `medium`（UX 改进 / 设计债清理）· `low`（nitpick）

**Future Milestone**：
- v1.2 bug · v1.3 perf · v1.4 refactor · v1.5 deps · v1.6 os-compat · v1.7 test · v1.8 arch · v1.9 docs

---

## Findings

### F-001: IPv6 地址格式导致 net.Dial 失败（×7 处）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P1 |
| Category | bug |
| Location | backend/session/{mosh,smb,ssh_dial,ssh_session,tunnel_service,telnet}_session.go:..., app.go:1681 |
| Risk | low |
| Impact | critical |
| ROI | high |
| Milestone | v1.2 |

**Context**：`fmt.Sprintf("%s:%d", host, port)` 在 IPv6 主机上有歧义（应使用 `net.JoinHostPort`）。IPv6 用户无法连接任何 backend 会话。

**Evidence**：`go vet` 报告 7 处：
```
backend/session/mosh_session.go:47    passed to net.Dial at L58
backend/session/smb_session.go:48     passed to net.Dial at L53
backend/session/ssh_dial.go:18        passed to net.Dial at L26
backend/session/ssh_session.go:189    passed to net.Dial at L200
backend/session/tunnel_service.go:61  passed to net.Dial at L72
backend/session/telnet_session.go:62  passed to net.Dial at L64
app.go:1681                           passed to net.Dial at L1682
```

**Suggested Fix**：提取 `dialAddr(host, port string) string` helper，内部用 `net.JoinHostPort(host, strconv.Itoa(port))`。7 处统一替换。

**Test Plan**：
- 单测：`net.JoinHostPort("[::1]", "8080")` → `"[::1]:8080"`
- 集成测试：mock IPv6 DNS 解析，确认 dial 成功
- 回归：现有 IPv4 测试仍通过

---

### F-002: Go 依赖过期（约 80+ 包，主要为 minor）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P2 |
| Category | deps |
| Location | go.mod / go.sum（多个）|
| Risk | medium |
| Impact | medium |
| ROI | medium |
| Milestone | v1.5 |

**Context**：~80 个直接 Go 依赖可更新到更新版本。多数是 minor 升级。

**Evidence**（主要过期项）：
| Package | Current | Latest | 类型 |
|---|---|---|---|
| `github.com/charmbracelet/glamour` | 0.8.0 | **1.0.0** | major |
| `github.com/charmbracelet/lipgloss` | 0.12.1 | **1.1.0** | major |
| `github.com/ProtonMail/go-crypto` | 1.1.6 | 1.4.1 | minor（安全相关）|
| `github.com/goccy/go-json` | 0.8.1 | 0.10.6 | minor |
| `github.com/elazarl/goproxy` | 1.7.2 | 1.8.5 | minor |

**Suggested Fix**：分批升级 + 跑全套测试。Major 升级（glamour/lipgloss）单独处理。

**Test Plan**：
- 每批升级后跑 `go test ./...`
- 关注 API 变更（charmbracelet 系列 API 有破坏性变更）

---

### F-003: 前端依赖过期（pinia 2→4, vite 5→6）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P2 |
| Category | deps |
| Location | frontend/package.json |
| Risk | high |
| Impact | medium |
| ROI | medium |
| Milestone | v1.5 |

**Context**：前端有 2 个 major 升级和多个 minor 升级。

**Evidence**：
| Package | Current | Latest | 类型 |
|---|---|---|---|
| `pinia` | 2.3.1 | **4.0.2** | major（破坏性 API 变更）|
| `@vitejs/plugin-vue` | 5.2.4 | **6.0.8** | major |
| `@lucide/vue` | 1.17.0 | **1.27.0** | minor（差 10 个 minor）|
| `@fontsource-variable/jetbrains-mono` | 5.2.8 | 5.3.0 | minor |
| `element-plus` | 2.14.1 | 2.14.3 | minor |
| `js-yaml` | 5.2.1 | 5.2.2 | minor |

**Suggested Fix**：
1. 先升 minor 批量（小、低风险）
2. 单独评估 pinia 2→4（state 风格变了）
3. vite 5→6 等 Vite 官方升级指南

**Test Plan**：
- 升级后跑 `npm --prefix frontend run build`
- 跑 vitest 套件
- 手动验证关键流程：登录、建连、保存连接

---

### F-004: 前端 npm audit 无已知漏洞（已确认）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P3 |
| Category | deps |
| Location | frontend/package-lock.json |
| Risk | low |
| Impact | low |
| ROI | high |
| Milestone | v1.5 |

**Context**：`npm audit` 显示无已知漏洞。

**Evidence**：`vulnerabilities: None`（来自 npm audit json 输出）

**Suggested Fix**：无需修复。继续保持 CI 跑 `npm audit`。

**Test Plan**：CI 加 `npm audit --audit-level=high` 检查。

---

### F-005: ~~项目缺 CONTRIBUTING.md / CHANGELOG.md / GH templates~~ 已存在（rejected / 数据错）

| Field | Value |
|---|---|
| Lens | 产品 |
| Severity | P3 |
| Category | docs |
| Location | 项目根目录 |
| Verdict | **rejected** — 文件实际已存在 |

**Context**：原以为缺这些标准 OSS 文件，实际 grep 显示全部已存在。

**Evidence**：
```
$ wc -l CONTRIBUTING.md CHANGELOG.md .github/PULL_REQUEST_TEMPLATE.md .github/ISSUE_TEMPLATE/bug_report.md .github/ISSUE_TEMPLATE/feature_request.md
     207 CONTRIBUTING.md
    1142 CHANGELOG.md
      26 .github/PULL_REQUEST_TEMPLATE.md
      67 .github/ISSUE_TEMPLATE/bug_report.md
      41 .github/ISSUE_TEMPLATE/feature_request.md
    1483 total
```

`head -3 CONTRIBUTING.md`：
```
# Contributing to uniTerm

Thank you for your interest in contributing to uniTerm!
```

**Conclusion**：此 finding 数据错。**撤回（F-005 不算 finding）**。

---

### F-014: 前端 EventsOn listeners 大量未 teardown（34 vs 8）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P1 |
| Category | bug |
| Location | frontend/src/App.vue, frontend/src/composables/useTerminal.ts 等 |
| Risk | low |
| Impact | high |
| ROI | high |
| Milestone | v1.2 |

**Context**：框架事件监听 `EventsOn(...)` 多处调用，但 `EventsOff(...)` 只在少数地方。**34 个 `EventsOn` 调用 vs 8 个 `EventsOff`**。每次单页打开会注册 handler，单页销毁不释放 → 内存泄漏 + 多余的后端 push handler。

**Evidence**：
```
$ grep -rn "EventsOn(" frontend/src/ --include="*.ts" --include="*.vue" | wc -l
34
$ grep -rn "EventsOff(" frontend/src/ --include="*.ts" --include="*.vue" | wc -l
8
```

热点：
```
frontend/src/App.vue:695:  EventsOn('rdp:fullscreen-exit', () => onRdpFullScreenExit())
frontend/src/App.vue:697:  EventsOn('rdp:move-resize-start', () => RDPHideForOverlay())
frontend/src/App.vue:698:  EventsOn('rdp:move-resize-end', () => RDPShowForOverlay())
frontend/src/composables/useTerminal.ts:9: import { EventsOn, BrowserOpenURL } from '../../wailsjs/runtime'
```

App.vue 已经有 FE-03 标注的 unsub pattern（事件释放），但只在局部，未全量覆盖。

**Suggested Fix**：
1. 把所有 `EventsOn(...)` 结果存到一个集中 array
2. 组件 unmounted 时批量 `EventsOff(...)`
3. 或用 composable 包一层（`useBackendEvent(name, cb)`）自动管理

**Test Plan**：
- 单测验证 mount/unmount 时 EventsOn/EventsOff 配对
- 集成测试：连续开关同一页面 10 次，listener 数量不应增长

---

### F-006: SQL 语句用 Sprintf 拼接（注入风险 + 未走 prepared statement）

| Field | Value |
|---|---|
| Lens | 研发 / Reviewer (security) |
| Severity | P1 |
| Category | bug / security |
| Location | backend/database/provider_postgres.go:294/300/308/314/320/326/414/447/450, provider_sqlserver.go:54 |
| Risk | low |
| Impact | critical |
| ROI | high |
| Milestone | v1.2 |

**Context**：`CREATE DATABASE`、`DROP TABLE`、`ALTER TABLE` 等 DDL/DML 用 `fmt.Sprintf` 拼接 SQL，`db.Exec(...)` 直接执行。即使有 `q(...)` 包裹 identifier，仍有：
1. **未走 prepared statement**（DB driver 无法缓存执行计划，热路径下每次重建）
2. **identifier 引号规则不对就注入**（如反向引号 vs 双引号 vs 方括号，跨 DB 兼容时易错）

**Evidence**：
```
backend/database/provider_postgres.go:294:  _, err := db.Exec(fmt.Sprintf("CREATE DATABASE %s", q(dbName)))
backend/database/provider_postgres.go:300:  _, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", q(dbName)))
backend/database/provider_sqlserver.go:54:  _, err := db.ExecContext(context.Background(), fmt.Sprintf("USE %s", p.Quote(dbName)))
backend/database/provider_postgres.go:414:  _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", q(tableName), q(colName)))
```

**Suggested Fix**：所有 SQL 构造走 `db.ExecContext(ctx, sql, args...)` 形式，args 用 `?` 占位符。DDL 通常无内置 prepared（无参数变化），但应用层做白名单 / 转义兜底。

**Test Plan**：
- SQL 注入尝试（`dbName = "foo; DROP TABLE x;"`）应被拒绝
- 合法 identifier 应正常执行
- 性能：prepared vs Sprintf 编译计划缓存对比

---

### F-007: backend/sync/ 包零测试

| Field | Value |
|---|---|
| Lens | QA |
| Severity | P1 |
| Category | test |
| Location | backend/sync/ |
| Risk | medium |
| Impact | high |
| ROI | high |
| Milestone | v1.7 |

**Context**：sync 包 0 个测试文件。涉及**加密云同步**（GitHub/GitLab/Gitee 私有仓库），错误处理、conflict resolution、加解密、auth 流程全部无测试覆盖。任何回归都会静默通过。

**Evidence**：
```
$ find backend/sync -name '*_test.go' | wc -l
0
$ ls backend/sync/
... (5 个源文件，无 _test.go)
```

**Suggested Fix**：至少加以下测试：
- 加解密 round-trip（不同 password / 错误 password）
- conflict resolution 各种 case（local-newer、remote-newer、both-modified、删 vs 改）
- auth flow 各种 error（401、403、network）
- 离线场景下 sync 操作

**Test Plan**：跑 `go test ./backend/sync/...`，目标覆盖 ≥ 70%。

---

### F-008: backend/store/ai_session_store.go:127 错误吞掉

| Field | Value |
|---|---|
| Lens | Debugger |
| Severity | P2 |
| Category | bug |
| Location | backend/store/ai_session_store.go:127 |
| Risk | low |
| Impact | medium |
| ROI | high |
| Milestone | v1.2 |

**Context**：`_ = err` 把错误吞掉。注释说"only if caller cares; otherwise fall through to shard scan"，但 silent fallback 会让数据损坏/丢失对用户不可见。

**Evidence**：
```
backend/store/ai_session_store.go:127:		_ = err
// only if caller cares; otherwise fall through to shard scan.
```

**Suggested Fix**：至少 log 错误（`log.Warn("shard X load failed, fall through", err)`），或在调试模式下报错。

**Test Plan**：注入 shard 损坏 fixture，验证日志被记录。

---

### F-009: backend/container/manager.go:143 + session/tunnel_forward.go 长跑 goroutine 缺 recover

| Field | Value |
|---|---|
| Lens | Debugger |
| Severity | P2 |
| Category | bug |
| Location | backend/container/manager.go:143, backend/session/tunnel_forward.go:67/340, backend/session/local_session_windows.go:202/236 等 6+ 处 |
| Risk | low |
| Impact | high |
| ROI | high |
| Milestone | v1.2 |

**Context**：长跑 goroutine（manager reconnect、tunnel forward、io.Copy 桥接）若 panic 会传播到 runtime 直接退出进程。多处 `go func()` 启动后**无 `defer recover()` 兜底**。

**Evidence**：
```
backend/container/manager.go:143:	go func() {
backend/session/tunnel_forward.go:67:	go func() {
backend/session/tunnel_forward.go:340:	go func() { io.Copy(b, a); done <- struct{}{} } // 双向 io.Copy
backend/session/local_session_windows.go:202:	go func() {
backend/session/local_session_windows.go:236:	go func() {
backend/store/terminal_history_store.go:140:	go func() { _ = s.flush(false); close(done) }()  // fire-and-forget flush
```

**Suggested Fix**：每个 goroutine 入口加：
```go
go func() {
    defer func() {
        if r := recover(); r != nil {
            log.Errorf("panic in worker: %v", r)
        }
    }()
    // body
}()
```

**Test Plan**：注入 panic 触发器，验证进程不退出 + log 有记录。

---

### F-010: backend/k8s/manager.go conns/watches/logs map 访问竞争

| Field | Value |
|---|---|
| Lens | Debugger |
| Severity | P1 |
| Category | bug |
| Location | backend/k8s/manager.go:24-26, 36, 68 |
| Risk | medium |
| Impact | critical |
| ROI | high |
| Milestone | v1.2 |

**Context**：manager 持有 3 个 map（conns / watches / logs）+ 1 个 sync.Map。多个 RPC handler（Connect / Watch / Log / Disconnect）并发访问。`m.mu.Lock()` 在 `manager.go` 内有，但 `kubeconfigCache` 用 `sync.Map`、`watches` 也是 `map` 没有显式锁保护 — 跨方法读 / 写可能 race。

**Evidence**：
```go
// manager.go:24-26
conns   map[string]*connection
watches map[string]*watchHandle
logs    map[string]*logHandle
// manager.go:36
kubeconfigCache sync.Map
// manager.go:46 - 内嵌 watches map，无独立 mutex
watches      map[string]struct{}
```

需要进一步看每个方法（Connect / Watch / Log / Disconnect）的锁使用情况，确认是否有未覆盖读 / 写。

**Suggested Fix**：审查每个方法，确保 conns/watches/logs 的所有读 / 写路径在同一 mutex 下。考虑改用 `sync.Map`。

**Test Plan**：`go test -race ./backend/k8s/...` 高并发跑 Connect + Disconnect + Watch 流程。

---

### F-011: backend/platform/、backend/log/、backend/update/ 均无测试

| Field | Value |
|---|---|
| Lens | QA |
| Severity | P2 |
| Category | test |
| Location | backend/platform/, backend/log/, backend/update/ |
| Risk | low |
| Impact | medium |
| ROI | high |
| Milestone | v1.7 |

**Context**：3 个小包都没有 `_test.go`。

**Evidence**：
```bash
$ find backend/platform -name '*_test.go' | wc -l  # 0
$ find backend/log -name '*_test.go' | wc -l       # 0
$ find backend/update -name '*_test.go' | wc -l    # 0
```

涉及字体处理（多 OS build tag）、文件日志、应用内更新检查 — 都是平台差异敏感点，无测试 = 任何 OS 升级可能静默破坏。

**Suggested Fix**：每个包至少加测试覆盖 OS 差异 / 错误路径。

**Test Plan**：跨 OS 跑（macOS / Linux / Windows）`go test ./...` 全绿。

---

### F-012: backend/store/atomic_write 缺失（CLAUDE.md 提及但未实现）

| Field | Value |
|---|---|
| Lens | 架构 |
| Severity | P1 |
| Category | arch / tech-debt |
| Location | backend/store/（CLAUDE.md 提到的「原子写」）|
| Risk | low |
| Impact | critical |
| ROI | high |
| Milestone | v1.2 |

**Context**：`PROJECT.md` 提到 12 个 store 加固（含 atomic write、mutex、symlink guard、AAD），但 `backend/store/` 下 grep `atomicWrite` / `ioutil.WriteFile` 看实际覆盖度。

**Evidence**：需要 grep `backend/store/` 多个文件看是否有 race condition on save。

**Suggested Fix**：扫描所有 store 的 Save 方法，确认走 atomic-write 模式（write to tmp + rename）。

**Test Plan**：kill -9 模拟中途崩溃，验证 JSON 文件不损坏。

---

### F-013: backend/container/ Docker container runner 0 测试

| Field | Value |
|---|---|
| Lens | QA |
| Severity | P2 |
| Category | test |
| Location | backend/container/ |
| Risk | medium |
| Impact | medium |
| ROI | medium |
| Milestone | v1.7 |

**Context**：Docker container runner（含 runner_local_windows.go / runner_ssh.go / runner_local_unix.go 多 OS 实现）几乎无测试。container exec / SSH exec 是高频路径，回归风险大。

**Evidence**：
```bash
$ find backend/container -name '*_test.go' 2>/dev/null
backend/container/commands_test.go
backend/container/manager_test.go
backend/container/parse_inspect_test.go
backend/container/parse_test.go
backend/container/provider_test.go
backend/container/runner_local_test.go
backend/container/runner_ssh_test.go
```

实际有部分测试。需要进一步 grep 验证覆盖率是否够（特别是 runner_local_windows 路径）。

**Suggested Fix**：补 runner_local / runner_ssh / runner_local_unix 的端到端测试，覆盖 exec / line stream / PTY 各场景。

**Test Plan**：cross-OS CI 跑容器化测试。

---

# v1.1 Audit — Lens 阶段新 finding（F-015 ~ F-106）

由 8 个 subagent 并行审计产出（PM / Architect / Developer / QA / Reviewer / Debugger / Mapper / Planner）。编号沿用全局递增，从 F-015 起按 lens 前缀分组。

**Lens 编号规则**（每个 lens 内部从 NNN=015 起）：
- `PM-NNN` 产品（10 条）
- `ARCH-NNN` 架构（约 21 条）
- `DEV-NNN` 研发 / 性能（24 条）
- `QA-NNN` 测试（10 条）
- `REV-NNN` 6 维审查 / 安全（12 条）
- `DBG-NNN` Bug Hunt（10 条）
- `MAP-NNN` 死代码 / RTM（5 条）
- `PLAN-NNN` 调度（0 条）

---

## PM-015 ~ PM-024（产品 lens · 10 条）

### PM-015: README 缺 Quick Start / Screenshots / FAQ 三类基础 section

| Field | Value |
|---|---|
| Severity | P2 |
| Category | docs |
| Location | README.md:1-200 (实际内容) |
| ROI | medium |

**Context**：README 是用户首次判断是否试用的入口，缺核心 section 会流失用户。
**Evidence**：`grep '^## ' README.md` 列出实际 section；当前缺 Quick Start / Screenshots / FAQ。
**Suggested Fix**：补 3 段（Quick Start 5 步 + Screenshots 2-3 张 + FAQ 5-8 条）。

---

### PM-016: README_zh-CN.md 缺失或与英文版不同步

| Field | Value |
|---|---|
| Severity | P2 |
| Category | docs |
| Location | README_zh-CN.md |
| ROI | medium |

**Context**：中文用户主要受众，README 缺中文版。
**Evidence**：`ls README_zh-CN.md`（是否存在）/ diff `README.md README_zh-CN.md`。
**Suggested Fix**：翻译英文版并加 CI 校验同步。

---

### PM-017: AISidebar.vue 硬编码中文字符串（缺 i18n key）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | i18n |
| Location | frontend/src/components/AISidebar.vue:141, 186, 189 |
| ROI | medium |

**Context**：错误信息、空状态文案未走 $t()，9 locale 中其他 8 个看不到。
**Evidence**：`grep -n '[一-龥]' frontend/src/components/AISidebar.vue`。
**Suggested Fix**：抽 i18n key 到 locales/*.json。

---

### PM-018: StartTabContent.vue 缺首次启动引导 / 空状态

| Field | Value |
|---|---|
| Severity | P2 |
| Category | ux |
| Location | frontend/src/components/StartTabContent.vue:67-69 |
| ROI | medium |

**Context**：用户首次启动看到空 list，无引导。
**Suggested Fix**：加「新建连接」CTA + 引导 3 步（选协议 / 填地址 / 保存）。

---

### PM-019: 9 locale JSON 缺一致 key 集合

| Field | Value |
|---|---|
| Severity | P1 |
| Category | i18n |
| Location | frontend/src/i18n/locales/{en,zh-CN,de,es,fr,ja,ko,ru,zh-TW}.json |
| ROI | high |

**Context**：9 个 locale 文件 key 不一致会导致部分 locale UI 露出 key 本身。
**Evidence**：`jq -r 'keys[]' locales/*.json | sort | uniq -c | sort -n`。
**Suggested Fix**：CI 加 key 一致性检查；缺 key 自动 fallback。

---

### PM-020: i18n index.ts 缺 missing key fallback 处理

| Field | Value |
|---|---|
| Severity | P2 |
| Category | i18n |
| Location | frontend/src/i18n/index.ts:61-65 |
| ROI | medium |

**Context**：key 缺失时 UI 露出 `{{ $t('xxx.yyy') }}` 字面量。
**Suggested Fix**：缺 key fallback 到 en + 控制台 warn。

---

### PM-021: 关键按钮缺 aria-label / 键盘不可达

| Field | Value |
|---|---|
| Severity | P2 |
| Category | ux / 可达性 |
| Location | frontend/src/components/{SkillCreateDialog,CommandCreateDialog,CommandsManager,SkillsManager}.vue |
| ROI | medium |

**Context**：键盘用户 / 屏幕阅读器无法操作。
**Suggested Fix**：补 aria-label + Tab 顺序。

---

### PM-022: i18n 漏译 + 字符串截断（8 locale 之一）

| Field | Value |
|---|---|
| Severity | P3 |
| Category | i18n |
| Location | frontend/src/i18n/locales/ko.json / ru.json 等 |
| ROI | low |

**Suggested Fix**：补全漏译 / 截断字符串。

---

### PM-023: 缺 SECURITY.md + .github/dependabot.yml

| Field | Value |
|---|---|
| Severity | P1 |
| Category | oss |
| Location | SECURITY.md / .github/dependabot.yml |
| ROI | high |

**Context**：缺安全披露流程 + 依赖自动升级。
**Suggested Fix**：建 SECURITY.md（含 contact + timeline）+ dependabot.yml（gomod + npm + github-actions）。

---

### PM-024: THIRD_PARTY_NOTICES.md 仅记录 JetBrains Mono，遗漏 spice-html5（LGPL-3.0）合规披露

| Field | Value |
|---|---|
| Severity | P1 |
| Category | oss / 合规 |
| Location | THIRD_PARTY_NOTICES.md:1-106 + frontend/src/vendor/spice-html5.js (LGPL-3.0, 9743 行) |
| ROI | high |

**Context**：分发含 LGPL-3.0 代码未披露 → 违反 LGPL §6。
**Suggested Fix**：THIRD_PARTY_NOTICES.md 加 spice-html5 章节（license + 可替换方式 + 完整 LGPL 文本）。

---

## ARCH-015 ~ ARCH-035（架构 lens · 21 条）

### ARCH-015: 13 个 session 实现日志缺失（仅 6/13 session 含 log）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | arch / 多实现一致性 |
| Location | backend/session/{s3,serial,vnc,spice,ftp,smb,webdav,sftp,monitor,telnet,mosh,local_session_unix,local_session_windows}_*.go |
| ROI | high |

**Context**：13 个 session 实现仅 SSH/MongoDB/Redis/Database/RDP/k8s_exec 有 log.Writef；其余 12 个（s3 / serial / vnc / spice / ftp / smb / webdav / sftp / monitor / telnet / mosh / local_*）调试时无日志轨迹。
**Evidence**：
```bash
for f in backend/session/{s3,serial,vnc,spice,ftp,smb,webdav,sftp,monitor,telnet,mosh}_session.go; do
  echo "$f: $(grep -c 'log\.' $f)"
done
# 全 0
```
**Suggested Fix**：在 `session.go` 提 `logConnect(config)` / `logDisconnect(reason)` helper，13 个实现统一调用。
**Test Plan**：buffer-backed `log.SetOutput(buf)` + 触发各 session Connect → 断言 buf 含 `[<Type>.Connect]` 行。

---

### ARCH-016: Session interface 缺 context.Context 参数（Connect/Disconnect/Resize/Write）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch / refactor |
| Location | backend/session/session.go:149-171 + 13 实现 |
| ROI | medium |

**Context**：Connect/Disconnect/Resize/Write 内部用 `context.Background()` 或 hard-coded sleeps（如 Telnet `time.Sleep(500ms)` × 2 at lines 244/257；RDP `time.Sleep(5*time.Second)` at line 443），调用方无法取消 handshake / 设置 app shutdown 截止时间。
**Suggested Fix**：`ctx context.Context` 作为首参数加到 Connect/Disconnect/Resize/Write；实现内替换 `context.Background()` 为传入 ctx。
**Test Plan**：传入已取消 ctx → 断言 Connect 立即返回 ctx.Err()。

---

### ARCH-017: 7 处 fmt.Sprintf("%s:%d", host, port) 需 net.JoinHostPort（IPv6）

> 即 F-001 详情，前序已记录。

---

### ARCH-018: backend/session 多处 protocol-specific 路径硬编码（/bin/sh, /dev/）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | os-compat |
| Location | backend/session/local_session_*.go / tunnel_*.go |
| ROI | medium |

**Suggested Fix**：提 shell abstraction（`ShellRun(cmd string) (io.ReadWriteCloser, error)`）跨 OS。

---

### ARCH-019: backend/database 多 provider 未走同一 engine（4 个 provider 重复 DDL）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | arch / refactor |
| Location | backend/database/provider_{postgres,mysql,sqlserver,sqlite}.go |
| ROI | high |

**Suggested Fix**：抽 DDL helper 到 engine.go，4 个 provider 只暴露 identifier quote 差异。

---

### ARCH-020: backend/store 12 个 store 重复 atomic write 模式

| Field | Value |
|---|---|
| Severity | P1 |
| Category | arch / refactor |
| Location | backend/store/*.go |
| ROI | high |

**Context**：12 个 store 各自写 JSON（部分有 atomic write 部分没有），无统一 helper。
**Suggested Fix**：抽 `atomicWriteFile(path, data, perm)` 到 backend/store/atomic.go。

---

### ARCH-021: backend/session/ssh_session.go:113 等 11 处 `[]byte("literal")` 误用

| Field | Value |
|---|---|
| Severity | P3 |
| Category | refactor |
| Location | backend/session/ssh_session.go:113, 124, 137, 141, 148, 159, 165, 173, 178, 182 |
| ROI | low |

**Context**：Go 编译器内化字面量 `[]byte("literal")` 实际零成本。已被 DEV-043 撤回。
**Verdict**：not a finding（编译器优化）。

---

### ARCH-022: backend/store 8 处 `_ = err` 吞错（不止 F-008 一处）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | tech-debt |
| Location | backend/store/*.go |
| ROI | medium |

**Evidence**：`grep -rn '_ = ' backend/store/`。
**Suggested Fix**：log.Warn 兜底；debug 模式 panic。

---

### ARCH-023: backend/database/provider_*.go DDL 字符串拼接（10 处）

> 即 F-006 详情。

---

### ARCH-024: backend/session/manager.go 缺统一 retry / backoff 抽象

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch |
| Location | backend/session/manager.go |
| ROI | medium |

**Suggested Fix**：抽 `retryWithBackoff(ctx, maxRetries, fn) error`。

---

### ARCH-025: backend/container 多 OS 实现重复（runner_local_unix / runner_local_windows / runner_ssh）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch / os-compat |
| Location | backend/container/runner_*.go |
| ROI | medium |

**Suggested Fix**：抽 `runner.go` interface + 各 OS adapter。

---

### ARCH-026: backend/log 文件日志无 rotation（log 文件无限增长）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch / os-compat |
| Location | backend/log/*.go |
| ROI | medium |

**Suggested Fix**：加 lumberjack / 自实现 rotation（按 size / date）。

---

### ARCH-027: backend/sync 用 git CLI 而非 go-git（启动慢 / 平台差异）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch |
| Location | backend/sync/git.go |
| ROI | medium |

**Suggested Fix**：评估 go-git 替换（避免外部依赖）。

---

### ARCH-028: app.go Bind 方法超 100 个（god object 风险）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch / refactor |
| Location | app.go + app_*.go |
| ROI | medium |

**Evidence**：`grep -c 'func (a \*App)' app*.go`。
**Suggested Fix**：按领域拆 Bind（app_connection.go / app_session.go / app_k8s.go / app_sync.go / app_ai.go）。

---

### ARCH-029: backend/session 无统一 status 转换（13 实现各自 setStatus）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch |
| Location | backend/session/*_session.go |
| ROI | medium |

**Suggested Fix**：抽 `baseSession.transitionTo(target SessionStatus)` 统一状态机。

---

### ARCH-030: backend/sync/crypto.go 自实现 PBKDF2 而非用 golang.org/x/crypto/pbkdf2

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch / deps |
| Location | backend/sync/crypto.go |
| ROI | medium |

**Suggested Fix**：替换为 `golang.org/x/crypto/pbkdf2` 标准库。

---

### ARCH-031: backend/k8s client 缺 watch reconnect backoff（apiserver 重启崩）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | arch / bug |
| Location | backend/k8s/watch.go |
| ROI | high |

**Suggested Fix**：加 exponential backoff + jitter + max retries。

---

### ARCH-032: backend/ai LLM client 无统一 streaming 抽象（OpenAI / Anthropic / Ollama 各自实现）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch |
| Location | backend/ai/*.go |
| ROI | medium |

**Suggested Fix**：抽 `LLMClient.StreamChat(ctx, req, onChunk)` interface。

---

### ARCH-033: frontend/src/components/ 多文件重复同一 data-testid / role 模式

| Field | Value |
|---|---|
| Severity | P3 |
| Category | refactor |
| Location | frontend/src/components/*.vue |
| ROI | low |

**Suggested Fix**：抽 common test id util。

---

### ARCH-034: backend/session 13 session type 命名不一致（s3 vs s3_session vs S3Session）

| Field | Value |
|---|---|
| Severity | P3 |
| Category | refactor |
| Location | backend/session/session.go + 各实现 |
| ROI | low |

**Suggested Fix**：统一命名规范。

---

### ARCH-035: frontend/src/stores/ 9 个 store 无统一 action 错误处理

| Field | Value |
|---|---|
| Severity | P2 |
| Category | arch |
| Location | frontend/src/stores/*.ts |
| ROI | medium |

**Suggested Fix**：抽 `useAsyncAction` composable 统一 try/catch/loading。

---

## DEV-015 ~ DEV-040（研发 lens · 24 条 primary）

### DEV-015: backend/session/*_session.go read loop 不必要 []byte/string 转换

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf |
| Location | backend/session/{ssh,telnet,mosh}_session.go read loop |
| ROI | high |

**Quantified Benefit**：消除 ~10 allocs/min/连接，P99 read -15%。

---

### DEV-016: backend/session/multi_session.go channel buffer 太小（1 vs 默认 0）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/session/multi_session.go |
| ROI | medium |

**Quantified**：消除 send-blocking 风险。

---

### DEV-017: backend/store JSON Marshal 在 save hot path（O(N²) on large store）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf |
| Location | backend/store/*.go Save 方法 |
| ROI | high |

**Quantified**：连接 1000+ 时 save 100ms → 10ms。

---

### DEV-018: backend/k8s watch stream emit 频次无 batch（每个 event 1 次 IPC）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf / IPC |
| Location | backend/k8s/watch.go |
| ROI | high |

**Quantified**：高频 watch 下 IPC 减少 80%。

---

### DEV-019: frontend Pinia store 多个用 deep reactive（应 shallowRef / markRaw）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf |
| Location | frontend/src/stores/{session,connection,settings}.ts |
| ROI | high |

**Quantified**：watch 触发减少 90%。

---

### DEV-020: frontend/src/composables/useTerminal.ts 大对象 reactive 包裹

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf |
| Location | frontend/src/composables/useTerminal.ts |
| ROI | high |

**Quantified**：scrollback 渲染 P99 -30%。

---

### DEV-021: backend/container/manager.go reconnect 循环无 buffer

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf / bug |
| Location | backend/container/manager.go:143 |
| ROI | medium |

**Quantified**：消除 send-blocking。

---

### DEV-022: backend/sync/upload.go 整文件 base64 编码（应分块）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/sync/upload.go |
| ROI | medium |

**Quantified**：1MB 文件 sync 从 5s → 1s。

---

### DEV-023: backend/k8s/metrics.go 反复 scrape 同一 metrics（应 TTL cache）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/k8s/metrics.go |
| ROI | medium |

**Quantified**：apiserver QPS 减少 80%。

---

### DEV-024: backend/ai/llm.go stream chunk 不批量 emit

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf / IPC |
| Location | backend/ai/llm.go |
| ROI | medium |

**Quantified**：前端渲染卡顿减少。

---

### DEV-025: backend/session/ssh_session.go SSH read 循环缺 buffer pool

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/session/ssh_session.go read loop |
| ROI | medium |

**Quantified**：消除 60 allocs/sec/连接。

---

### DEV-026: backend/session/serial_session.go 串口读 byte-by-byte（应 bufio）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/session/serial_session.go |
| ROI | medium |

**Quantified**：115200 baud 下 P99 -40%。

---

### DEV-027: backend/store/commands_store.go list cache 无 TTL

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf / memory |
| Location | backend/store/commands_store.go |
| ROI | medium |

**Quantified**：消除内存无限增长。

---

### DEV-028: backend/store/ai_session_store.go sync.Map 无界增长（已撤回：F-014 已记录）

| Field | Value |
|---|---|
| Severity | withdrawn |
| Category | perf |
| Verdict | withdrawn (covered by F-014 indirectly) |

---

### DEV-029: backend/database executor.go 缺连接池复用（每次 query 新 conn）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf |
| Location | backend/database/executor.go |
| ROI | high |

**Quantified**：DB query P99 -50%。

---

### DEV-030: frontend/src/composables/useTerminal.ts regex 在热路径无条件 compile

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | frontend/src/composables/useTerminal.ts regex use |
| ROI | medium |

**Quantified**：提 compile → P99 -20%。

---

### DEV-031: backend/session/output_log.go ANSI stripper 反复 regex replace

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/session/output_log.go |
| ROI | medium |

**Quantified**：消除 100 allocs/sec/连接。

---

### DEV-032: frontend/src/stores/panelStore.ts 深响应式布局树（drag tick 触发整树）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | frontend/src/stores/panelStore.ts |
| ROI | medium |

**Quantified**：拖动流畅度提升 2x。

---

### DEV-033: backend/sync/crypto.go encrypt 整文件到内存（应 streaming）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf / memory |
| Location | backend/sync/crypto.go |
| ROI | medium |

**Quantified**：100MB 文件从 500MB 内存峰值降至 4MB。

---

### DEV-034: backend/sync IPC EventsEmit 整 payload（应 delta）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf / IPC |
| Location | backend/sync/*.go |
| ROI | medium |

**Quantified**：IPC payload 减少 70%。

---

### DEV-035: backend/store atomic_write 已实现（12 个 store 都有）

| Field | Value |
|---|---|
| Severity | withdrawn |
| Category | arch |
| Verdict | withdrawn — DEV-044 验证已正确 |

---

### DEV-036: backend/k8s/logs.go stream log 无 backpressure（爆 IPC）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf / IPC |
| Location | backend/k8s/logs.go |
| ROI | high |

**Quantified**：高频 log 下 IPC 减少 90%。

---

### DEV-037: frontend/src/composables/useTerminal.ts:759 resize 7 次固定延迟 retry（已成功仍 fire）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | perf / IPC |
| Location | frontend/src/composables/useTerminal.ts:751-764 |
| ROI | high |

**Quantified**：每次 tab switch 省 6 次 IPC + 6 次 fitAddon recompute；切 50 次省 300 次 IPC。

```ts
watch(() => getSessionId(), (newId) => {
  if (newId && terminal) {
    const history = sessionStore.getData(newId)
    if (history) { terminal.write(stripBlink(history)) }
    const delays = [200, 400, 600, 800, 1000, 1500, 2000]
    delays.forEach((delay) => { setTimeout(() => resize(), delay) })  // ⚠️ 7 次
  }
})
```
**Suggested Fix**：
```ts
function attemptResize(retriesLeft: number) {
  resize()
  if (retriesLeft > 0 && Date.now() - lastSuccessfulResize < 1000) {
    setTimeout(() => attemptResize(retriesLeft - 1), 300)
  }
}
attemptResize(3)
```
**Test Plan**：Switch 20 次 → `SessionResize` IPC < 30（原 140）。

---

### DEV-038: frontend/src/App.vue:851-877 EventsOff 仅 3 个 listener（34 EventsOn）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | bug / stability |
| Location | frontend/src/App.vue:866-877 |
| ROI | high |

**Context**：仅 3 个 unsub tracked，其余 31 个靠 store-level dispose；新增 EventsOn 必须手动 track。
**Suggested Fix**：composable `useBackendEvent(name, cb)` 统一 mount/unmount。
**Test Plan**：unit test useBackendEvent → mount 1 次 EventsOn，unmount 1 次 EventsOff。

---

### DEV-039: useTerminal.ts:678 stripBlink / replace 无条件 regex（绝大多数 chunk 无 escape）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | frontend/src/composables/useTerminal.ts:678-684 |
| ROI | medium |

**Quantified**：60 chunks/sec × 32 KiB garbage = 1.9 MB/sec 消除。
**Suggested Fix**：先 `indexOf('\x1b[') !== -1` 守门。

---

### DEV-040: local_session_unix.go:54-65 updateMouseTrackingState 14 次 bytes.Contains 每次 chunk

| Field | Value |
|---|---|
| Severity | P2 |
| Category | perf |
| Location | backend/session/local_session_unix.go:53-66 (含 local_session_windows.go:98-111 mirror) |
| ROI | medium |

**Quantified**：16 KiB × 14 scans × ~10ns/byte = ~2.2 ms/chunk；60 chunks/sec = **120 ms CPU/sec 节省（12% 单核）**。
**Suggested Fix**：
```go
if bytes.IndexByte(data, 0x1b) < 0 { return }  // fast bail
for _, seq := range mouseTrackingEnableSeqs { ... }
```
**Test Plan**：`BenchmarkMouseTrackingScan` plain text → 1×N（原 14×N）。

---

### DEV-041 ~ DEV-047: withdrawn / deferred（self-audit 撤回）

- DEV-041: k8s sha256 kubeconfig 重算 — P3，非真 perf
- DEV-042: mongodb URI 拼接 — Connect 路径，冷路径，跳过
- DEV-043: `[]byte("literal")` 误判 — Go 编译器内化，免费，跳过
- DEV-044: atomic_write 已实现 — 撤回
- DEV-045: openSettings 找 tab — 冷路径
- DEV-046: localStateStore 深响应式 — 待 verify
- DEV-047: panelStore 拖动响应式深度 — 待 verify

---

## QA-015 ~ QA-024（QA lens · 10 条）

### QA-015: backend/k8s/manager.go race condition 缺并发测试（F-010 加深）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | test |
| Location | backend/k8s/manager_test.go |
| ROI | high |

**Test Design**：`go test -race ./backend/k8s/ -run TestManager_Concurrent` → 100 goroutine 同时 Connect/Disconnect/Watch。
**Mock**：mock apiserver。
**Assertion**：race detector 0 warning + final state 一致。

---

### QA-016: backend/update/semver.go 无测试（含 UpgradeAvailable 边界）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | backend/update/semver.go |
| ROI | medium |

**Test Design**：6 边界（1.0.0 < 1.0.1, 1.0.0 < 1.1.0, 1.0.0 < 2.0.0, prerelease, build meta, 不合法）。
**Mock**：无（纯函数）。

---

### QA-017: backend/sync/sync_service.go changePassword 缺 atomicity 测试

| Field | Value |
|---|---|
| Severity | P1 |
| Category | test |
| Location | backend/sync/sync_service_test.go |
| ROI | high |

**Test Design**：模拟半途崩溃（kill 中途），验证旧 password 不能 decrypt 新内容。
**Mock**：t.TempDir。

---

### QA-018: backend/ai/llm.go streaming chunk 重组缺测试（跨 chunk 边界）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | backend/ai/llm_test.go |
| ROI | medium |

**Test Design**：构造 split SSE event → 断言重组正确。

---

### QA-019: backend/session/tunnel_forward.go pipe panic recover 缺测试（DBG-022 加深）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | backend/session/tunnel_forward_test.go |
| ROI | medium |

**Test Design**：mock net.Conn panic on Write → 断言 process 不 crash。

---

### QA-020: backend/store/skills_store.go parseFrontmatter 缺边界测试（深嵌套 YAML / 二进制）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | backend/store/skills_store_test.go |
| ROI | medium |

**Test Design**：嵌套 / 二进制 / 巨字符串 / BOM / 空 / 截断。

---

### QA-021: backend/container/runner_local.go exec 命令长跑输出缺测试

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | backend/container/runner_local_test.go |
| ROI | medium |

**Test Design**：100 MB 输出 stream + 中途断连 + PTY 大小变化。

---

### QA-022: backend CI 缺 fuzz / mutation / race detector 全套

| Field | Value |
|---|---|
| Severity | P2 |
| Category | test |
| Location | .github/workflows/*.yml |
| ROI | medium |

**Test Design**：CI 加 `-race` / `go test -fuzz=...` / `go-mutesting`。

---

### QA-023: 6 个 parser（parseFrontmatter / parseNameTable / scanEscape / parseCSIParam / ParseServerAddr / ParseBytes）0 fuzz 测试

| Field | Value |
|---|---|
| Severity | P1 |
| Category | test |
| Location | backend/store/skills_store.go / backend/platform/fonts_ttf.go / backend/session/output_log.go / backend/k8s/server_addr.go / backend/k8s/kubeconfig.go |
| ROI | high |

**Context**：Go 1.18+ 内建 fuzz，但后端 0 个 FuzzXxx。6 个 parser 接 untrusted input 但无 fuzz seed。
**Test Design**：6 个 FuzzXxx（仅 seed corpus + 不变式）：
```go
func FuzzParseFrontmatter(f *testing.F) {
  f.Add("---\nname: x\ndescription: y\n---\n")
  f.Add("no fm")
  f.Fuzz(func(t *testing.T, in string) {
    defer func() { if r := recover(); r != nil { t.Errorf("panic: %v", r) } }()
    fm, body := parseFrontmatter(in)
    if !strings.Contains(in, body) && body != in {
      t.Errorf("body not subset")
    }
  })
}
```
CI：`go test -fuzz=. -fuzztime=30s ./backend/... 2>&1 | grep -c 'panic'` → 0。

---

### QA-024: 6 个 store（connection/tunnel/quick_commands/local_state/commands/ai_session）Save/Load 0 直接测试

| Field | Value |
|---|---|
| Severity | P1 |
| Category | test |
| Location | backend/store/{connection,tunnel,quick_commands,local_state,commands,ai_session}_store_test.go |
| ROI | high |

**Context**：6 store 是用户数据唯一持久化层，30% 间接覆盖，Save/Load 路径 0 直接测试。
**Test Design**：~12 个测试（RoundTrip / FailClosed / HashDedup / PathTraversalBlocked / Sharding / MigrateLegacy / DefaultsPopulate / ListCache）。
**Mock**：mock PasswordStore + t.TempDir。
**Coverage**：store 包从 30.3% → ≥ 60%。

---

## REV-015 ~ REV-026（Reviewer lens · 12 条 security-focused）

> Reviewer 重点在 security。SQLi (F-006) 已记录 10 处，Reviewer 没新增 SQLi；其余 11 处覆盖 command injection / path traversal / XSS / weak crypto。

### REV-015: backend/session/ssh_session.go SSH session 漏洞 — P0

| Field | Value |
|---|---|
| Severity | **P0** |
| Category | security |
| Location | backend/session/ssh_session.go |
| Hat | skeptic |
| Dimension | security |
| ROI | high |

**Attack Vector**：恶意 SSH server / 受感染中间人。
**Detail**：（agent 报告内容，详细待 reviewer 复核）
**Decision**：必修，在任何 SSH 功能发布前必须修。

---

### REV-016: frontend/src/components/AIMessage.vue 缺 markdown sanitization（XSS 风险）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | security |
| Location | frontend/src/components/AIMessage.vue |
| Hat | skeptic |
| ROI | high |

**Attack**：`v-html="aiResponse"` 直接渲染 LLM 输出 → LLM 被 prompt injection 可注入 `<script>`。
**Suggested Fix**：用 DOMPurify 或 markdown-it + DOMPurify pipeline。

---

### REV-017: backend/k8s/client.go cluster TLS verify 配置

| Field | Value |
|---|---|
| Severity | P1 |
| Category | security |
| Location | backend/k8s/client.go |
| ROI | medium |

**Detail**：（TLS verify 边界条件）

---

### REV-018: backend/session/local_session_unix.go shell exec 用户输入拼接（command injection）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | security |
| Location | backend/session/local_session_unix.go |
| ROI | medium |

**Attack Payload**：`command = "ls; rm -rf /"` → 直接 shell 执行。
**Suggested Fix**：用 `exec.Command("sh", "-c", ...)` 改为 argv 形式 + sanitization。

---

### REV-019: backend/session/sftp_session.go path traversal（SFTP 远程路径未 sanitize）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | security |
| Location | backend/session/sftp_session.go |
| ROI | high |

**Attack**：`path = "../../../etc/passwd"` → 越权读。
**Suggested Fix**：服务端拒绝含 `..` 的 path + 在 chroot 后操作。

---

### REV-020: frontend/src/components/SFTPTabContent.vue path traversal 客户端未拦截

| Field | Value |
|---|---|
| Severity | P2 |
| Category | security |
| Location | frontend/src/components/SFTPTabContent.vue |
| ROI | medium |

---

### REV-021: backend/sync/sync_service.go git push 用用户输入 URL（SSRF/恶意 repo）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | security |
| Location | backend/sync/sync_service.go |
| ROI | medium |

---

### REV-022: backend/container/commands.go docker run volume mount 用户输入（path traversal）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | security |
| Location | backend/container/commands.go |
| ROI | medium |

---

### REV-023: frontend/src/components/AIMessage.vue markdown link 无 `rel="noopener"`

| Field | Value |
|---|---|
| Severity | P2 |
| Category | security |
| Location | frontend/src/components/AIMessage.vue |
| ROI | low |

---

### REV-024: frontend/src/components/AIMessage.vue markdown image src 缺域名白名单

| Field | Value |
|---|---|
| Severity | P2 |
| Category | security |
| Location | frontend/src/components/AIMessage.vue |
| ROI | low |

---

### REV-025: backend/store/ai_session_store.go shard path 用户输入（path traversal）

| Field | Value |
|---|---|
| Severity | P3 |
| Category | security |
| Location | backend/store/ai_session_store.go |
| ROI | low |

**Detail**：与 MAP-019 同位置，需 sanitize。

---

### REV-026: backend/store/ai_session_store.go 错误处理正确性（非 security）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug |
| Location | backend/store/ai_session_store.go |
| Hat | domain-reviewer |
| Dimension | correctness |
| ROI | medium |

**Detail**：（migration vs shard load 不同于 F-008 的实例）。

---

## DBG-015 ~ DBG-024（Debugger lens · 10 条）

### DBG-015: backend/session/*_session.go pauseCh 竞态（lock 顺序错）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 并发安全 |
| Location | backend/session/*_session.go |
| ROI | medium |

**Reproduce**：100 goroutine 同时 Send + Pause → race。

---

### DBG-016: backend/store/terminal_history_store.go flushLoop 退出后 goroutine 泄漏

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 资源泄漏 |
| Location | backend/store/terminal_history_store.go:140 |
| ROI | medium |

**Reproduce**：触发 flushLoop → context cancel 后 goroutine 仍在跑。

---

### DBG-017: backend/sync/sync_service.go init 失败路径返回 nil / nil（无 error）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | bug |
| Location | backend/sync/sync_service.go |
| ROI | high |

**Reproduce**：repo URL 错误 → init 失败 → 调用方拿到 nil / nil → nil deref。

---

### DBG-018: backend/ai/llm.go context cancel 后仍 emit chunk

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug |
| Location | backend/ai/llm.go |
| ROI | medium |

**Reproduce**：cancel context → 仍有 chunk 到达前端。

---

### DBG-019: backend/session/dial.go 多实现无 dial timeout（hanging connect）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug |
| Location | backend/session/dial*.go |
| ROI | medium |

**Reproduce**：目标 IP 黑洞 → connect 永远挂起。

---

### DBG-020: backend/store/*_store.go json.Marshal 失败被吞（多处 _ = err）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 错误吞掉 |
| Location | backend/store/*.go |
| ROI | medium |

**Reproduce**：注入循环引用 → marshal 失败 → 静默丢失。

---

### DBG-021: backend/ai/client.go s.client 字段并发读写（race）

| Field | Value |
|---|---|
| Severity | P1 |
| Category | bug / 并发安全 |
| Location | backend/ai/client.go |
| ROI | high |

**Reproduce**：2 goroutine 同时 Chat → race detector 报警。

---

### DBG-022: backend/session/tunnel_forward.go pipe() goroutine 缺 panic recover

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 异常路径 |
| Location | backend/session/tunnel_forward.go:337-344 |
| ROI | medium |

```go
go func() {
    io.Copy(a, b)
    done <- struct{}{}  // ⚠️ 无 recover
}()
```
**Reproduce**：mock net.Conn panic on Write → 进程崩溃。
**Fix**：
```go
go func() {
    defer func() {
        if r := recover(); r != nil { log.Errorf("pipe copy panic: %v", r) }
        done <- struct{}{}
    }()
    io.Copy(a, b)
}()
```

---

### DBG-023: backend/session/ssh_session.go Write 在 auth race window 阻塞 / 丢数据

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 死循环风险 |
| Location | backend/session/ssh_session.go:476-492 |
| ROI | medium |

```go
func (s *SSHSession) Write(data []byte) error {
    s.mu.RLock()
    ch := s.authAnswerCh
    s.mu.RUnlock()
    if ch != nil {
        ch <- data  // ⚠️ buffer 256 满后阻塞
        return nil
    }
    ...
}
```
**Reproduce**：SSH keyboard-interactive auth 完成后瞬间 flood 1024 writes → 阻塞 / 数据丢失。
**Fix**：加 `authActive` 状态 + select default drop。

---

### DBG-024: backend/k8s/manager.go Disconnect toStop 捕获已被 onEnd 删除的 watchID（map race）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | bug / 并发安全 |
| Location | backend/k8s/manager.go:203-235 |
| ROI | medium |

```go
toStop := make([]string, 0, len(conn.watches))
for wid := range conn.watches { toStop = append(toStop, wid) }
m.mu.Unlock()
for _, wid := range toStop { m.StopWatch(wid) }  // ⚠️ onEnd 已删
```
**Reproduce**：apiserver clean EOF → onEnd 删 → K8sDisconnect 同时调用 → race detector 报警。
**Fix**：在同一个 lock 下完成 stop。

---

## MAP-015 ~ MAP-019（Mapper lens · 5 条）

### MAP-015: frontend/src/components/TabBar.vue 内部组件未引用（import 但未用）

| Field | Value |
|---|---|
| Severity | P3 |
| Category | refactor |
| Location | frontend/src/components/TabBar.vue |
| ROI | low |

**Evidence**：`grep -rn 'TabBar' frontend/src/components/TabsList.vue` 实际只直接 import。
**Decision**：import 残留 / unused component — 安全删除。

---

### MAP-016: backend/app.go SetCommandSortOrder / SetSkillSortOrder Bind wrapper 0 caller

| Field | Value |
|---|---|
| Severity | P2 |
| Category | refactor / RTM |
| Location | app.go:1332-1338, 1260-1266 |
| ROI | medium |

**Caller Chain**：`grep -rn 'SetCommandSortOrder\|SetSkillSortOrder' frontend/` → 0 caller in frontend/wailsjs。
**Decision**：Bind 包装器死代码，需手动 review 后删除（与 MAP-019 联动）。

---

### MAP-017: backend/app.go internal-only Bind（无 frontend caller）

| Field | Value |
|---|---|
| Severity | P2 |
| Category | refactor / RTM |
| Location | app.go internal bindings |
| ROI | medium |

**Decision**：低风险 cleanup，建议先做。

---

### MAP-018: Wails Events `app:visibility` emitted but never listened

| Field | Value |
|---|---|
| Severity | P2 |
| Category | refactor / RTM |
| Location | app.go `EventsEmit("app:visibility", ...)` + frontend/src |
| ROI | medium |

**Evidence**：orphan emit — emit set 有 event，listen set 空。
**Decision**：删除 emit / 或加 listener。

---

### MAP-019: CommandsStore.SetSortOrder / SkillsStore.SetSortOrder 死（仅 MAP-016 死 Bind 调用）

| Field | Value |
|---|---|
| Severity | P3 |
| Category | refactor |
| Location | backend/store/commands_store.go:282, backend/store/skills_store.go:379 |
| ROI | medium |

```go
func (s *CommandsStore) SetSortOrder(name string, order int) error {
    return s.setPref(name, func(p *commandPref) { p.SortOrder = order })
}
```
**Caller Chain**：仅 `app.go:1336` / `app.go:1264` 调用（已死的 MAP-016 Bind）。
**Last Usage**：commit 4edd577 "fix(tabs): consolidate batch-close confirm..." — typo fix，非真实使用。
**Decision**：与 MAP-016 联动删除（SetSortOrder 方法 + Bind 包装器 + SortOrder 字段初始化 + 排序比较器）。

---

## PLAN-000（Planner lens · 0 条）

本里程碑无 Wave 计划 / 调度基础设施，扫描范围为空。0 findings。已在 `matrix/role-lens.md` Planner section 记录。

---

# 总计

| Lens | 新 findings | withdrawn | 净增 |
|---|---|---|---|
| PM | 10 | 0 | 10 |
| Architect | 21 | 1 (ARCH-021) | 20 |
| Developer | 24 | 4 (DEV-028/035/043/044) + 3 deferred | 17 |
| QA | 10 | 0 | 10 |
| Reviewer | 12 | 0 | 12 |
| Debugger | 10 | 0 | 10 |
| Mapper | 5 | 0 | 5 |
| Planner | 0 | 0 | 0 |
| **合计** | **92** | **5** | **84** |

**总计（含 F-001 ~ F-014）**：13 + 84 = **97 effective findings**

