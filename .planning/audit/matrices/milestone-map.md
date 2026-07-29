# Future Milestone × Finding 矩阵

把审计 finding 按未来修复 milestone 分类。每个 milestone 启动时从对应列抽 finding 即可。

## Milestone 计划

| Milestone | Version | Focus | 预期 Finding 数量 |
|---|---|---|---|
| **v1.2 Bug Fixes** | v1.2 | 修所有 P0/P1 bugs、稳定性问题 | — |
| **v1.3 Performance Fixes** | v1.3 | Go/Vue perf 优化、内存 | — |
| **v1.4 Code Refactoring** | v1.4 | 重构债、设计一致性 | — |
| **v1.5 Dependency Updates** | v1.5 | 升过期依赖、修 dep 安全 | — |
| **v1.6 OS Compatibility** | v1.6 | 跨平台抽象隔离 | — |
| **v1.7 Test Coverage Boost** | v1.7 | 测试补全 | — |
| **v1.8 Architecture Improvements** | v1.8 | 架构层 perf / 设计 | — |
| **v1.9 Documentation / OSS** | v1.9 | 文档 / OSS 一流标准 | — |

## 矩阵（Synthesis 阶段填充）

| Finding ID | Severity | Category | Milestone | ROI | Notes |
|---|---|---|---|---|---|
| (e.g.) DBG-005 | P0 | bug | v1.2 | critical | nil deref |
| (e.g.) DEV-015 | P1 | perf | v1.3 | high | sync.Pool |
| (e.g.) ARCH-022 | P2 | refactor | v1.4 | medium | session manager |
| (e.g.) DEP-003 | P2 | deps | v1.5 | medium | pinia 2→4 |
| (e.g.) ARCH-008 | P1 | os-compat | v1.6 | high | _windows split |
| (e.g.) QA-001 | P1 | test | v1.7 | high | backend/session coverage |
| (e.g.) ARCH-040 | P2 | arch | v1.8 | medium | API redesign |
| (e.g.) PM-002 | P2 | docs | v1.9 | medium | README install steps |

## 类别映射规则

| Finding `category` | → Milestone |
|---|---|
| `bug` | v1.2 Bug Fixes |
| `perf` | v1.3 Performance Fixes |
| `refactor` | v1.4 Code Refactoring |
| `deps` | v1.5 Dependency Updates |
| `config` | v1.5 或 v1.4（看内容） |
| `os-compat` | v1.6 OS Compatibility |
| `test` | v1.7 Test Coverage Boost |
| `arch` | v1.8 Architecture Improvements |
| `docs` | v1.9 Documentation / OSS |

## Milestone 启动模板

每个未来 milestone 启动时（`/gsd-new-milestone`）：

```bash
# 1. 读 matrices/milestone-map.md，抽对应列
grep '| v1.2 ' matrices/milestone-map.md > v1.2-input.md

# 2. 按 severity 排序，生成 REQUIREMENTS.md
# 3. 走标准 GSD 流程：discuss → plan → execute
# 4. 修一条 finding 勾一行
```

## 累计进度（每个 milestone 完成后更新）

| Milestone | Total Findings | Fixed | Verified | Open |
|---|---|---|---|---|
| v1.2 Bug Fixes | — | — | — | — |
| v1.3 Performance Fixes | — | — | — | — |
| v1.4 Code Refactoring | — | — | — | — |
| v1.5 Dependency Updates | — | — | — | — |
| v1.6 OS Compatibility | — | — | — | — |
| v1.7 Test Coverage Boost | — | — | — | — |
| v1.8 Architecture Improvements | — | — | — | — |
| v1.9 Documentation / OSS | — | — | — | — |