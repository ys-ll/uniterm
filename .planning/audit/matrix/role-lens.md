# Role × File 覆盖矩阵

每个 subagent lens 覆盖了哪些文件 / 出了哪些 finding。

## 产品 (PM) — 10 findings

- **Files scanned**: README, CONTRIBUTING, .github, frontend/src/components/, frontend/src/i18n/, THIRD_PARTY_NOTICES.md, frontend/src/vendor/
- **Findings count**: 10 (PM-015 ~ PM-024)
- **P0/P1/P2/P3**: 0/3/5/2
- **Coverage gaps**: Wails IPC UX 流 / 错误码规范 / 中文/英文以外 locale 实际体验

## 架构 (Architect) — 21 findings（撤回 1）

- **Files scanned**: backend/* (全部 packages) + 13 session 实现一致性 + 多 OS build tag + import graph
- **Findings count**: 21 (ARCH-015 ~ ARCH-035)；1 withdrawn (ARCH-021)
- **P0/P1/P2/P3**: 0/4/13/3
- **Coverage gaps**: 公共库封装一致性 / 跨平台 native binding / 国际化架构

## QA — 10 findings

- **Files scanned**: 各 package 测试覆盖 / fuzz 缺口 / CI 缺失
- **Findings count**: 10 (QA-015 ~ QA-024)
- **P0/P1/P2/P3**: 0/4/5/1
- **Coverage gaps**: 端到端 e2e 框架 / mutation test 集成 / 跨 OS test matrix

## 研发 (Developer / Perf) — 24 findings（撤回 4 + deferred 3）

- **Files scanned**: backend read loops / IPC / store / k8s / ai；frontend composables / stores / components
- **Findings count**: 24 primary（DEV-015 ~ DEV-040），4 withdrawn + 3 deferred
- **P0/P1/P2/P3**: 0/7/12/1
- **Coverage gaps**: 前端 bundle size / memory profile / pprof 实战数据

## Reviewer (6-dim) — 12 findings（10 security）

- **Files scanned**: security 全维度（SQLi / Command injection / XSS / SSRF / weak crypto / path traversal / deserialization / CSRF）
- **Findings count**: 12 (REV-015 ~ REV-026)
- **P0/P1/P2/P3**: **1/3/7/1**（**唯一 P0**）
- **Coverage gaps**: test_coverage / code_quality / performance / maintainability 4 维较薄

## Debugger — 10 findings

- **Files scanned**: nil deref / 并发安全 / 资源泄漏 / panic recover / 错误吞掉 / 数值边界
- **Findings count**: 10 (DBG-015 ~ DBG-024)
- **P0/P1/P2/P3**: 0/2/8/0
- **Coverage gaps**: 反复修的 bug 历史深查 / 线上 crash report 分析

## Mapper — 5 findings

- **Files scanned**: Bind methods / Events / Store actions / RTM
- **Findings count**: 5 (MAP-015 ~ MAP-019)
- **P0/P1/P2/P3**: 0/0/3/2
- **Coverage gaps**: codegraph 全量 callers 反查 / test-only exports / build tag 切换可达性

## Planner — 0 findings

- **Files scanned**: 任务系统 / Wave 计划 / 调度日志（本里程碑 0 文件）
- **Findings count**: 0
- **Coverage gaps**: 任务粒度 / DAG / WIP / REQ-ID / persona 派发 — **全部留待未来 milestone**

## 交叉覆盖矩阵（最终态）

| File | PM | Arch | QA | Dev | Rev | Dbg | Map | Plan | Coverage |
|---|---|---|---|---|---|---|---|---|---|
| `backend/session/ssh_session.go` | — | ✓ | — | ✓ | ✓ | ✓ | — | — | 4/8 |
| `backend/session/local_session_*.go` | — | ✓ | — | ✓ | ✓ | ✓ | — | — | 4/8 |
| `backend/session/tunnel_forward.go` | — | — | — | ✓ | — | ✓ | — | — | 2/8 |
| `backend/store/*.go` | — | ✓ | ✓ | ✓ | — | ✓ | — | — | 4/8 |
| `backend/store/ai_session_store.go` | — | — | — | — | ✓ | ✓ | — | — | 2/8 |
| `backend/database/provider_*.go` | — | ✓ | — | ✓ | ✓ | — | — | — | 3/8 |
| `backend/k8s/*.go` | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — | 5/8 |
| `backend/sync/*.go` | — | — | ✓ | ✓ | ✓ | ✓ | — | — | 4/8 |
| `backend/container/*.go` | — | ✓ | ✓ | ✓ | — | ✓ | — | — | 4/8 |
| `backend/ai/*.go` | — | ✓ | — | ✓ | — | ✓ | — | — | 3/8 |
| `app.go` + `app_*.go` | — | ✓ | — | — | — | — | ✓ | — | 2/8 |
| `backend/platform/`, `log/`, `update/` | — | ✓ | ✓ | — | — | — | — | — | 2/8 |
| `frontend/src/composables/useTerminal.ts` | — | — | — | ✓ | — | — | — | — | 1/8 |
| `frontend/src/App.vue` | — | — | — | ✓ | — | — | — | — | 1/8 |
| `frontend/src/components/` | ✓ | ✓ | — | ✓ | ✓ | — | — | — | 4/8 |
| `frontend/src/stores/` | — | ✓ | — | ✓ | — | — | ✓ | — | 3/8 |
| `frontend/src/i18n/` | ✓ | — | — | — | — | — | — | — | 1/8 |
| `README.md` / CONTRIBUTING / CHANGELOG | ✓ | — | — | — | — | — | — | — | 1/8 |
| `THIRD_PARTY_NOTICES.md` | ✓ | — | — | — | — | — | — | — | 1/8 |
| `frontend/src/vendor/` | ✓ | — | — | — | — | — | — | — | 1/8 |
| `.github/` | ✓ | — | — | — | — | — | — | — | 1/8 |

## 目标：每个核心文件至少 3 lens 审计

**当前覆盖率最低**：
- `frontend/src/App.vue` 1/8
- `frontend/src/composables/useTerminal.ts` 1/8
- `frontend/src/i18n/` 1/8
- `app.go` 2/8

**未来 milestone 可加深**：frontend 深度扫描（VM render / 响应式 profile）/ AI/LLM 安全深扫 / k8s reconnect 性能 profile。