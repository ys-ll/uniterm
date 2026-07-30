# Severity × Category 矩阵（V1 Re-verification 后 · 最终态）

## 当前 finding 分布（80 effective findings）

|  | bug | perf | refactor | deps | os-compat | test | arch | security | docs | **Total** |
|---|---|---|---|---|---|---|---|---|---|---|
| **P0** | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **1** | 0 | **1** |
| **P1** | 4 | 2 | 0 | 0 | 0 | 5 | 1 | 6 | 2 | **20** |
| **P2** | 6 | 9 | 3 | 0 | 2 | 2 | 6 | 3 | 4 | **33** |
| **P3** | 0 | 1 | 4 | 1 | 0 | 1 | 4 | 1 | 1 | **13** |
| **Total** | **10** | **12** | **7** | **1** | **2** | **8** | **11** | **11** | **7** | **80** |

> 注：从 97 → 80（撤回 12 + withdrawn 5）

## P0 紧急必修（1 条）

- REV-015 SSH session 漏洞（skeptic hat）

## 详细列表（80 条 effective）

### P0（1）
| Finding | Severity | Category |
|---|---|---|
| REV-015 | P0 | security |

### P1（20）
| Finding | Severity | Category |
|---|---|---|
| F-001 | P1 | bug |
| F-006 | P1 | bug/security |
| F-007 | P1 | test |
| F-010 | P1 | bug |
| F-012 | P1 | arch |
| F-014 | P1 | bug |
| DEV-015 | P1 | perf |
| DEV-017 | P1 | perf |
| DEV-018 | P1 | perf |
| DEV-019 | P1 | perf |
| DEV-020 | P1 | perf |
| DEV-029 | P1 | perf |
| DEV-036 | P1 | perf |
| DEV-037 | P1 | perf/IPC |
| DEV-038 | P1 | bug/stability |
| QA-015 | P1 | test |
| QA-017 | P1 | test |
| QA-023 | P1 | test |
| QA-024 | P1 | test |
| ARCH-015 | P1 | arch |
| ARCH-019 | P1 | arch |
| ARCH-020 | P1 | arch |
| ARCH-031 | P1 | arch |
| REV-017 | P1 | security |
| REV-018 | P1 | security |
| REV-019 | P1 | security |
| PM-019 | P1 | i18n |
| PM-023 | P1 | oss |
| PM-024 | P1 | oss/compliance |

### P2（33）— 详见 findings.md（撤 REV-016 / DEV-016 / DEV-022 / DEV-024 / QA-018 / DBG-018 / DBG-021 后）

### P3（13）— 详见 findings.md（撤 ARCH-032 / PM-016 / ARCH-027 / ARCH-030 / F-002 后）

## 类别 → milestone 路由

| Category | → Milestone | 当前 finding 数 |
|---|---|---|
| bug | v1.2 | 10 |
| security (P0+P1) | v1.2.1 紧急 patch | 7 |
| test | v1.7 | 8 |
| arch | v1.8 | 11 |
| perf | v1.3 | 12 |
| refactor | v1.4 | 7 |
| deps | v1.5 | 1 (F-002 已撤回 → 0；v1.5 重新评估) |
| os-compat | v1.6 | 2 |
| docs / oss / i18n | v1.9 | 7 |