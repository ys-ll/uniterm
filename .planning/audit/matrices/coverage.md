# Coverage 矩阵 · 模块 × 审计状态

每个角色审计完一个包/模块后追加 1 行。

## Backend 包

| Package | PM | Architect | Developer | QA | Reviewer | Debugger | Mapper | Findings | Last Update |
|---|---|---|---|---|---|---|---|---|---|
| `backend/session/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/store/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/database/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/k8s/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/sync/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/platform/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/log/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `backend/update/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `app.go` + `app_*.go` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `main.go` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |

## Frontend 包/层

| Module | PM | Architect | Developer | QA | Reviewer | Debugger | Mapper | Findings | Last Update |
|---|---|---|---|---|---|---|---|---|---|
| `frontend/src/components/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/stores/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/services/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/composables/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/i18n/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/types/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/utils/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/src/App.vue` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/wailsjs/` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/vite.config.*` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |
| `frontend/package.json` | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | ☐ | — | — |

## 项目级

| Module | PM | Architect | Developer | QA | Reviewer | Debugger | Mapper | Findings | Last Update |
|---|---|---|---|---|---|---|---|---|---|
| `README.md` / `README_zh-CN.md` | ☐ | — | — | — | — | — | — | — | — |
| `CONTRIBUTING.md` | ☐ | — | — | — | — | — | — | — | — |
| `CHANGELOG.md` | ☐ | — | — | — | — | — | — | — | — |
| `LICENSE` | ☐ | — | — | — | — | — | — | — | — |
| `.github/` | ☐ | — | — | — | — | — | — | — | — |
| `docs/guide/` | ☐ | — | — | — | — | — | — | — | — |

## 自动扫描

| Tool | Status | Result File |
|---|---|---|
| `go vet ./...` | ✅ done | see findings/auto-scan.md |
| `go list -m -u all` | ✅ done | see findings/auto-scan.md |
| `npm outdated` | ✅ done | see findings/auto-scan.md |
| `npm audit` | ✅ done | see findings/auto-scan.md |

## 统计

- Total backend packages: 9 + app code
- Total frontend modules: 11
- Total role × module cells: 7 × 20 = 140 (audit coverage slots)
- Filled: 0/140 (0%)
- Findings so far: 0