# Risk × Impact 矩阵 · ROI 决策

每条 finding 评估 **修复风险 × 修复收益**，决定要不要修、什么时候修。

## 当前 finding 分布

| Finding | Risk | Impact | ROI | Decision |
|---|---|---|---|---|
| F-001 IPv6 dial | low | critical | **high** | ✅ 必修（v1.2 优先） |
| F-002 Go deps | medium | medium | medium | ✅ 修（v1.5） |
| F-003 npm deps | high | medium | medium | 🤔 大改时 |
| F-004 npm audit | low | low | high | ✅ 加 CI 检查（v1.5） |
| F-005 OSS files | low | medium | high | ✅ 修（v1.9） |

## 决策规则

|  | Low Impact | Medium Impact | High Impact | Critical |
|---|---|---|---|---|
| **Low Risk** | ✅ Quick win | ✅ 修 | ✅ 必修 | 🚨 紧急 |
| **Medium Risk** | 🤔 看时间 | ✅ 修 | 🚨 优先 | 🚨 紧急 |
| **High Risk** | ❌ Skip | 🤔 大改时 | 🚨 优先 | 🚨 需 plan |

- ✅ 修：放进对应未来 milestone
- 🚨 优先 / 紧急：必进 v1.2，不能拖
- 🤔 看时间：可选修
- ❌ Skip：记 Rejected，不修

## 修复 plan（按优先级）

### Critical × Low Risk（紧急必修）
- F-001 IPv6 dial — 7 处都是同一个 fix 模式，提取 helper + 替换

### High Impact × Low Risk（perf wins）
- (待审计发现)

### High Impact × Medium Risk（cross-file）
- (待审计发现)

## Rejected（不修）

(无)