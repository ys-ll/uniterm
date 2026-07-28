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

全 6 个子系统(其中 terminal_io 含 Claude Code `render_compat` 子方向):

1. **terminal_io**(终端 I/O 路径)
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

- **P0**:每次会话都触发 / 中等负载就劣化 / Claude Code 跑起来就花屏
- **P1**:特定场景触发 / 高负载才暴露 / 特定主题下错位
- **P2**:理论隐患 / 不常见

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

执行完成后填入每个 agent 的完整 prompt(便于审计 / 复跑)。

---

## 元信息

- **作者**:Claude(claude-fable-5)
- **日期**:2026-07-28
- **审批状态**:approved(用户已认可方案 A 与三段设计)
- **下一步**:执行 5 个并行 agent → 整合 → 填表 → spec self-review → 用户复核 → writing-plans skill