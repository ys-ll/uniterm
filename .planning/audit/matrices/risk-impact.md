# Risk × Impact 矩阵 · ROI 决策

每个 finding 评估：**破坏性变更 × 复杂度 × 影响面**，决定该不该修、什么时候修。

## Risk（修复风险）

| Level | 定义 |
|---|---|
| **Low** | 局部修改，不影响 API，单文件可解，单测可守 |
| **Medium** | 跨文件，影响 1-2 个调用方，需要集成测试 |
| **High** | API 行为变更 / 破坏性 / 需要迁移 / 跨包改动 / 高耦合 |

## Impact（修复收益）

| Level | 定义 |
|---|---|
| **Critical** | 修一个 P0 bug 或核心安全问题，影响所有用户 |
| **High** | 性能明显提升 / 修高频 crash / 大幅降低内存 |
| **Medium** | 修偶发问题 / UX 改进 / 设计债清理 |
| **Low** | Nitpick / 注释 / 命名 / 文档微调 |

## 矩阵

|  | Low Impact | Medium Impact | High Impact | Critical |
|---|---|---|---|---|
| **Low Risk** | ✅ Quick win | ✅ **修** | ✅ **必修** | 🚨 **紧急** |
| **Medium Risk** | 🤔 看时间 | ✅ **修** | 🚨 **优先** | 🚨 **紧急** |
| **High Risk** | ❌ Skip | 🤔 大改时 | 🚨 **优先** | 🚨 需 plan |

## 决策规则

- ✅ **修**：放进未来对应 milestone
- 🤔 **看时间**：可选修，看其他工作进度
- 🚨 **优先 / 紧急**：必进 v1.2 bug fix milestone，不能拖
- ❌ **Skip**：记到「Rejected」备查，不修

## Finding 分类（Synthesis 阶段填充）

```
## Critical × Low Risk (P0/P1 bugs, easy fix)
- (e.g.) DBG-005: nil deref in session.go:42 — Low Risk, Critical Impact
- (e.g.) DBG-012: ...

## Critical × High Risk (P0/P1 bugs, destructive)
- (e.g.) ARCH-008: API signature change — High Risk, Critical Impact
- 修复 plan: 写到 findings/architect.md

## High Impact × Low Risk (perf wins, design debt cleanup)
- (e.g.) DEV-015: sync.Pool for ssh encoder — Low Risk, High Impact

## High Impact × Medium Risk (cross-file refactor)
- (e.g.) ARCH-022: session manager redesign — Medium Risk, High Impact
```

## 阈值

- Critical × 任意 Risk：**0 个未处理**（不能有 backlog）
- High Impact × Low Risk：**全部修**
- High Impact × Medium Risk：进对应 milestone 优先批
- Medium Impact × Low Risk：批量修或合并修
- Low Impact × Low Risk：写到 backlog，每季度 review