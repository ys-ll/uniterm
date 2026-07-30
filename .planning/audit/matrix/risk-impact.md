# Risk × Impact 矩阵 · ROI 决策

每个 finding 评估：修复风险 × 修复收益，决定要不要修、什么时候修。

## 当前 finding 分布（v1.1 Audit 终态 · 97 条）

| Finding | Risk | Impact | ROI | Decision | Milestone |
|---|---|---|---|---|---|
| REV-015 | low | critical | **high** | 🚨 P0 必修 | v1.2.1 |
| F-001 IPv6 dial | low | critical | **high** | ✅ 必修 | v1.2 |
| F-006 SQL Sprintf | low | critical | **high** | 🚨 优先（安全）| v1.2 |
| F-007 sync 0 测试 | medium | high | high | 🚨 必须补 | v1.7 |
| F-008 error swallowed | low | medium | high | ✅ 修 | v1.2 |
| F-009 goroutine no recover | low | high | high | ✅ 批量加 recover | v1.2 |
| F-010 k8s map race | medium | critical | **high** | 🚨 必须修 | v1.2 |
| F-012 store atomic_write | low | critical | **high** | 🚨 必修 | v1.2 |
| F-014 EventsOn 泄漏 | low | high | high | ✅ 修（批量）| v1.2 |
| ARCH-015 13 session 日志缺失 | low | high | high | ✅ 修 | v1.2 |
| ARCH-019 DB 多 provider 重复 | low | high | high | ✅ 修 | v1.8 |
| ARCH-020 store 12 atomic 重复 | low | high | high | ✅ 修 | v1.8 |
| ARCH-031 k8s watch reconnect | medium | high | high | ✅ 修 | v1.8 |
| DEV-015 ~ DEV-040（17 条 primary） | low-med | high | high | ✅ 修 | v1.3 |
| QA-015 ~ QA-024（10 条） | low-med | high | high | ✅ 修 | v1.7 |
| REV-016 ~ REV-026（11 条 security/correctness） | low-med | high | high | 🚨 必须修 | v1.2.1 |
| DBG-015 ~ DBG-024（10 条） | low-med | high | high | ✅ 修 | v1.2 |
| MAP-015 ~ MAP-019（5 条） | low | medium | high | ✅ 修（手动 review）| v1.4 |
| PM-015 ~ PM-024（10 条） | low | medium | high | ✅ 修 | v1.9 |
| F-002 Go deps | medium | medium | medium | 🤔 大改时分批 | v1.5 |
| F-003 npm deps | high | medium | medium | 🤔 大改时分批 | v1.5 |

## 决策分布

| 决策 | 数量 | Finding |
|---|---|---|
| 🚨 P0 必修 | 1 | REV-015 |
| 🚨 P1 必修 | 31 | F-001/006/007/010/012/014 + ARCH-015/019/020/031 + DEV 11 条 + QA-015/017/023/024 + REV 4 条 + DBG-021/017 + PM-019/023/024 |
| ✅ 修 | 49 | 其余 P1/P2 |
| 🤔 大改时 | 2 | F-002, F-003 |
| ❌ Skip | 0 | — |
| withdrawn | 5 | DEV-028/035/043/044 + ARCH-021（已撤回） |

## Critical × Low Risk（紧急必修）

- **REV-015** SSH session 漏洞 — P0，必须在下一个 SSH 发布前修
- **F-001** IPv6 dial（7 处）— 单文件 helper + 替换
- **F-006** SQL Sprintf（10 处）— 走 prepared statement
- **F-010** k8s map race — 加锁覆盖
- **F-012** store atomic_write — 确认覆盖度
- **ARCH-015** 13 session 日志缺失 — 提 helper

## 高 Impact × Medium Risk（cross-file）

- **F-009** goroutine 加 defer recover（6+ 处）
- **F-007** sync 包测试
- **DBG-022** pipe panic recover
- **ARCH-031** k8s reconnect backoff
- **REV-016** AI markdown XSS sanitize

## Rejected（已撤回 / 无 ROI）

- DEV-028 (covered by F-014)
- DEV-035 (verified already)
- DEV-043 (compiler optimization)
- DEV-044 (verified)
- ARCH-021 (compiler optimization)