# Severity × Category 矩阵

每条 finding 落在一个 severity × category 单元。Synthesis 阶段从这里出 backlog 优先级。

## 当前 finding 分布

|  | bug | perf | refactor | deps | config | os-compat | test | arch | docs | **Total** |
|---|---|---|---|---|---|---|---|---|---|---|
| **P0** | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **0** |
| **P1** | 1 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **1** |
| **P2** | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 1 | **3** |
| **P3** | 0 | 0 | 0 | 1 | 0 | 0 | 0 | 0 | 0 | **1** |
| **Total** | 1 | 0 | 0 | 3 | 0 | 0 | 0 | 0 | 1 | **5** |

## 详细列表

| Finding | Severity | Category |
|---|---|---|
| F-001 | P1 | bug |
| F-002 | P2 | deps |
| F-003 | P2 | deps |
| F-004 | P3 | deps |
| F-005 | P2 | docs |

## 阈值告警（执行时关注）

- P0 总数 > 5 → 紧急修
- P1 bug > 20 → v1.2 milestone 资源吃紧
- 总数 > 100 → 考虑批量修

## 类别 → 未来 milestone 映射

| Category | → Milestone |
|---|---|
| `bug` | v1.2 Bug Fixes |
| `perf` | v1.3 Performance Fixes |
| `refactor` | v1.4 Code Refactoring |
| `deps` | v1.5 Dependency Updates |
| `config` | v1.5 Dependency Updates（config）|
| `os-compat` | v1.6 OS Compatibility |
| `test` | v1.7 Test Coverage Boost |
| `arch` | v1.8 Architecture Improvements |
| `docs` | v1.9 Documentation / OSS |