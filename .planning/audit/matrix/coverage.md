# Coverage 矩阵 · 模块 × 视角 × 状态

每个模块被哪些视角审过、出了多少 finding。

## Backend 模块

| Module | 产品 | 架构 | QA | 研发 | Findings | Status |
|---|---|---|---|---|---|---|
| `backend/session/` | — | — | — | ✓ | 1 (F-001) | partial |
| `backend/store/` | — | — | — | — | — | not started |
| `backend/database/` | — | — | — | — | — | not started |
| `backend/k8s/` | — | — | — | — | — | not started |
| `backend/sync/` | — | — | — | — | — | not started |
| `backend/platform/` | — | — | — | — | — | not started |
| `backend/log/` | — | — | — | — | — | not started |
| `backend/update/` | — | — | — | — | — | not started |
| `backend/container/` | — | — | — | — | — | not started |
| `app.go` + `app_*.go` | — | — | — | ✓ | 1 (F-001) | partial |
| `main.go` | — | — | — | — | — | not started |

## Frontend 模块

| Module | 产品 | 架构 | QA | 研发 | Findings | Status |
|---|---|---|---|---|---|---|
| `frontend/src/components/` | — | — | — | — | — | not started |
| `frontend/src/stores/` | — | — | — | — | — | not started |
| `frontend/src/services/` | — | — | — | — | — | not started |
| `frontend/src/composables/` | — | — | — | — | — | not started |
| `frontend/src/i18n/` | — | — | — | — | — | not started |
| `frontend/src/App.vue` | — | — | — | — | — | not started |
| `frontend/wailsjs/` | — | — | — | — | — | not started |
| `frontend/vite.config.*` | — | — | — | — | — | not started |

## 项目级

| Module | 产品 | 架构 | QA | 研发 | Findings | Status |
|---|---|---|---|---|---|---|
| `README.md` / `README_zh-CN.md` | — | — | — | — | — | not started |
| `CONTRIBUTING.md` | — | — | — | — | — | not started |
| `CHANGELOG.md` | — | — | — | — | — | not started |
| `LICENSE` | — | — | — | — | — | not started |
| `.github/` | — | — | — | — | — | not started |
| `docs/guide/` | — | — | — | — | — | not started |

## 统计

- 总模块：20 个核心模块 + 6 个项目级
- 已被 1+ 视角审过：2/26 (8%)
- Findings so far: 5 (F-001 ~ F-005)
- 完成度：极低，需要继续读代码