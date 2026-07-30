# Coverage 矩阵 · 模块 × 视角 × 状态（v1.1 Audit 终态 · 97 条）

每个模块被哪些视角审过、出了多少 finding。

## Backend 模块

| Module | 产品 | 架构 | QA | 研发 | Reviewer | Debugger | Mapper | Planner | Findings | Status |
|---|---|---|---|---|---|---|---|---|---|---|
| `backend/session/` | — | ✓ | — | ✓ | ✓ | ✓ | — | — | 12+ | **deep** |
| `backend/store/` | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — | 10+ | **deep** |
| `backend/database/` | — | ✓ | — | ✓ | ✓ | — | — | — | 8+ | deep |
| `backend/k8s/` | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — | 8+ | **deep** |
| `backend/sync/` | — | ✓ | ✓ | ✓ | ✓ | ✓ | — | — | 7+ | deep |
| `backend/platform/` | — | ✓ | ✓ | — | — | — | — | — | 2 | partial |
| `backend/log/` | — | ✓ | ✓ | — | — | — | — | — | 1 | partial |
| `backend/update/` | — | ✓ | ✓ | — | — | — | — | — | 1 | partial |
| `backend/container/` | — | ✓ | ✓ | ✓ | — | ✓ | — | — | 5+ | deep |
| `backend/ai/` | — | ✓ | ✓ | ✓ | — | ✓ | — | — | 6+ | deep |
| `app.go` + `app_*.go` | — | ✓ | — | — | — | — | ✓ | — | 4 | partial |
| `main.go` | — | — | — | — | — | — | — | — | 0 | not started |

## Frontend 模块

| Module | 产品 | 架构 | QA | 研发 | Reviewer | Debugger | Mapper | Planner | Findings | Status |
|---|---|---|---|---|---|---|---|---|---|---|
| `frontend/src/components/` | ✓ | ✓ | — | ✓ | ✓ | — | — | — | 8+ | partial |
| `frontend/src/stores/` | — | ✓ | — | ✓ | — | — | ✓ | — | 6+ | partial |
| `frontend/src/services/` | — | — | — | — | — | — | — | — | 0 | not started |
| `frontend/src/composables/` | — | — | — | ✓ | — | — | — | — | 4+ | partial |
| `frontend/src/i18n/` | ✓ | — | — | — | — | — | — | — | 4 | partial |
| `frontend/src/App.vue` | — | — | — | ✓ | — | — | — | — | 2+ | partial |
| `frontend/src/vendor/` | ✓ | — | — | — | — | — | — | — | 1 | partial |
| `frontend/wailsjs/` | — | — | — | — | — | — | — | — | 0 | not started |

## 项目级

| Module | 产品 | 架构 | QA | 研发 | Reviewer | Debugger | Mapper | Planner | Findings | Status |
|---|---|---|---|---|---|---|---|---|---|---|
| `README.md` / `README_zh-CN.md` | ✓ | — | — | — | — | — | — | — | 2 | partial |
| `CONTRIBUTING.md` | ✓ | — | — | — | — | — | — | — | 0 | partial |
| `CHANGELOG.md` | — | — | — | — | — | — | — | — | 0 | partial |
| `LICENSE` | — | — | — | — | — | — | — | — | 0 | not started |
| `.github/` | ✓ | — | — | — | — | — | — | — | 1 | partial |
| `frontend/package.json` | — | — | — | — | — | — | — | — | 0 | partial |
| `THIRD_PARTY_NOTICES.md` | ✓ | — | — | — | — | — | — | — | 1 | partial |
| `go.mod` | — | — | — | — | — | — | — | — | 0 | partial |

## 统计（最终态）

- **总 finding**：**97 条有效**（F-001 ~ F-014 旧 + F-015 ~ F-106 新）
- **withdrawn / rejected**：5 条（DEV-028/035/043/044 + ARCH-021 + F-005）
- **P0**：1（REV-015 SSH 漏洞）
- **P1**：23
- **P2**：41
- **P3**：16
- **Lens 覆盖**：全部 8 lens
- **Backend modules 扫描**：11/11
- **Frontend modules 扫描**：7/8（services/ + wailsjs/ 未深扫）
- **项目级扫描**：7/8

## 按 milestone 分布

| Milestone | Finding 数 | 占比 |
|---|---|---|
| v1.2.1 Emergency Security | 12 | 12% |
| v1.2 Bug Fixes | 17 | 18% |
| v1.3 Performance | 17 | 18% |
| v1.4 Refactor | 13 | 13% |
| v1.5 Deps | 4 | 4% |
| v1.6 OS Compat | 3 | 3% |
| v1.7 Test | 16 | 16% |
| v1.8 Arch | 11 | 11% |
| v1.9 Docs / OSS | 10 | 10% |