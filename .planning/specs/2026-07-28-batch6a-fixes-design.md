---
title: Batch 6a — Backend hot paths(非 in-flight 子集)
date: 2026-07-28
status: approved
parent_spec: .planning/specs/2026-07-28-perf-bottlenecks-design.md
parent_batch: Batch 6(24 of 32 findings; 7 in-flight skipped)
---

# Batch 6a — Backend hot paths

实施审计推荐的 batch 6 中所有**非 in-flight** 的修复。非 in-flight 的 24 个 finding 见 `parent_spec` §7。

## 1. 目标

按依赖顺序实施 batch 6 所有非 in-flight 修复,每个 finding 一个原子 commit。完成后:

- P0 store debouncing(F-101, F-102, F-103)消除全量文件写和重复 marshal
- P1 DB 连接池/查询/序列化优化(F-113, F-114, F-115, F-116)降低 query 分配和握手
- P1 session 读路径(F-001, F-002, F-003, F-004, F-005, F-006, F-008)消除每字节分配
- P1 store 缓存(F-106, F-107, F-112)避免无谓文件系统读
- P1 connection_store / settings_store 加锁粒度优化(F-109, F-110, F-111)
- P1 SSH keepalive(F-044)调整间隔
- P2 修补(F-007, F-018)顺手做

## 2. 范围

### 2.1 文件清单(14 个)

```
backend/store/terminal_history_store.go   # F-101
backend/store/recent_store.go            # F-102
backend/store/ai_session_store.go        # F-103
backend/store/connection_store.go        # F-105, F-110
backend/store/settings_store.go          # F-108, F-109
backend/store/commands_store.go          # F-106, F-111
backend/store/skills_store.go            # F-107, F-112
backend/database/engine.go               # F-113, F-114
backend/database/provider_postgres.go    # F-115
backend/database/provider_mysql.go       # F-116
backend/session/ssh_session.go           # F-001, F-002, F-003, F-004, F-044
backend/session/local_session_unix.go    # F-005, F-006
backend/session/serial_session.go        # F-007
backend/session/telnet_session.go        # F-008
backend/session/manager.go               # F-018
```

### 2.2 不在本次范围

- output_log.go 全部 finding(F-011~F-016)— in-flight
- session.go 全部 finding(F-016, F-017)— in-flight
- ftp_session.go — in-flight,无 finding
- store/connection_store.go 中 in-flight 触及的部分 —— 见下节

### 2.3 约束

- 每个 finding 一个原子 commit,commit message 含 finding ID(F-XXX)
- 已有的 `*_test.go` 不改断言;只新增测试文件
- `backend/session/bench_test.go` 已在仓(由 audit 阶段写入),可作为回归证据
- bench 文件继续 untracked(用户约束)
- 若实施时发现某 finding 与 in-flight 文件冲突,登记在 §6,跳过

## 3. 修复策略(per finding)

引用 `parent_spec` §4-§6 详述。每条 fix 的具体 sketch:

### 3.1 Stores(P0 / P1)

| ID | 简述 | 修复 |
|---|---|---|
| F-101 | terminal_history Save 写全文件 | 加 debounce(500ms)+ atomic rename + 按 session 增量写 |
| F-102 | recent.Record 不 debounce | 加 debounce(500ms),同一 ID 合并 |
| F-103 | ai_session Save/Load 全量 marshal | 改用 `json.NewEncoder` + 单条消息文件,按 session id 分片 |
| F-105 | connection_store.Save 全量 marshal | 改 `json.Encoder` + 检查是否真改 |
| F-106 | commands.List 全目录扫描 | 缓存 list 结果,mtime-based invalidation |
| F-107 | skills.List 全 SKILL.md 读 | 同 F-106,mtime 缓存 |
| F-108 | settings.MarshalIndent | 改 `json.NewEncoder` + 减少 indentation |
| F-109 | settings.Load 持锁做 IO | 加载完再 Lock;或 atomic.Value |
| F-110 | populatePasswords 重迭代 + 同步 keychain | 后台 goroutine 异步 backfill,加载时不阻塞 |
| F-111 | commands.SaveCommand 直接 WriteFile | atomic rename + parent dir mkdir |
| F-112 | copyDir filepath.Walk 慢 | 改 `os.CopyFS` 或 streaming io.Copy |

### 3.2 Database

| ID | 简述 | 修复 |
|---|---|---|
| F-113 | queryStrings/queryAny 每行 alloc | stream via chan 或 flat [][]string;小结果用 map |
| F-114 | scanToString fmt.Sprintf | type switch |
| F-115 | Postgres GetTableSchema 三次往返 | 并发 errgroup + 缓存 |
| F-116 | mysql DSN 缺 ConnMaxLifetime | DSN 加 parseTime + 连接池参数 |

### 3.3 Session

| ID | 简述 | 修复 |
|---|---|---|
| F-001 | SSH read 4096 固定 buf | 16K 起步或按需扩 |
| F-002 | decodeOutput fresh src | per-session scratch buffer |
| F-003 | Write 全量 copy | 直接 write enc.Bytes()(transform 输出已分配) |
| F-004 | readLoop 双拷贝 | 复用单一 buf |
| F-005 | Local PTY 4K Read | 16K |
| F-006 | Local data := append | 复用 buf |
| F-007 | Serial 4K Read + 拷贝 | 同 F-005/F-006 |
| F-008 | Telnet 4K Read + IAC filter inline | 16K buf + 先 read 后 filter |
| F-044 | SSH keepalive 60s | 调整或按 lifecycle 启停 |
| F-018 | SessionManager.sessions map unbounded | Close 时强删 + 启动时校验 |

## 4. 实施顺序(依赖关系)

```
Phase A: stores (10 fixes, 5 files)
  1. terminal_history_store (F-101) - 最 straightforward
  2. recent_store (F-102)
  3. ai_session_store (F-103)
  4. commands_store (F-106, F-111) - cache + atomic
  5. skills_store (F-107, F-112) - cache + copyFS
  6. connection_store (F-105, F-110) - encoder + async keychain
  7. settings_store (F-108, F-109) - encoder + lock

Phase B: database (4 fixes, 3 files)
  8. engine.go (F-113) - queryStrings stream
  9. engine.go (F-114) - scanToString switch
  10. provider_postgres (F-115)
  11. provider_mysql (F-116)

Phase C: session hot paths (10 fixes, 4 files)
  12. ssh_session (F-002) - decode scratch
  13. ssh_session (F-003) - write alloc
  14. ssh_session (F-004) - read reuse
  15. ssh_session (F-001) - read buffer size
  16. ssh_session (F-044) - keepalive
  17. local_session_unix (F-005, F-006)
  18. telnet_session (F-008)
  19. serial_session (F-007)
  20. manager.go (F-018)
```

20 个原子 commit,每个 cross-验证已有 test + bench(`backend/session/bench_test.go`)。

## 5. 测试

- 现有 `*_test.go` 必须继续通过(`go test ./backend/...`)
- 新增 store debounce 测试:写后立即读应看到 stale;debounce 窗口后看到新值
- Bench 文件(`backend/session/bench_test.go`)继续 untracked;每次 commit 前跑一次 `go test -bench=Benchmark -benchmem ./backend/session/...` 记录 baseline 对比

## 6. 风险 / 推迟

- 若某 finding 的实现触及 in-flight 文件,推迟到后续 batch
- store debounce 改动若影响其他代码路径,登记回归
- bench 数据若变差(出现 ns/op 上升)立即停止并复核

## 7. 不在范围(明确)

- Frontend 改动(batch 8)
- Wails bridge 改动(batch 7)
- output_log.go / session.go 改动(in-flight 解除后单独 batch)