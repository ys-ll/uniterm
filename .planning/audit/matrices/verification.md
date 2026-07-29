# Verification 矩阵 · 3 个独立 Verifier × Finding

每个 finding 必须被 3 个独立 verifier 子代理反驳。**≥2 verifier 同意「真实存在 + 必要 + ROI 合理」** 才进入最终清单。

## Verifier 协议

每个 verifier 收到一个 finding，回答：

1. **真实存在**？阅读代码 / grep 验证
2. **必要**？vs 接受现状的 ROI
3. **ROI 合理**？修复成本 vs 收益
4. **破坏性**？API/行为变更影响范围
5. **复杂度**？实现难度

每个维度 ✓ / ✗ / ⚠️（部分同意）。

## Verdict 聚合

- **3/3 ✓** → `confirmed`（高置信，进最终清单）
- **2/3 ✓** → `likely`（中置信，进清单但标记 reviewer 备注）
- **1/3 ✓** → `disputed`（低置信，进 backlog 备查）
- **0/3 ✓** → `rejected`（不进清单，理由写这里）

## 矩阵（Synthesis 阶段填充）

| Finding ID | Verifier A | Verifier B | Verifier C | Verdict | Notes |
|---|---|---|---|---|---|
| (e.g.) PM-001 | ✓✓✓✓✓ | ✓✓✓⚠⚠ | ✓✓✓✓⚠ | confirmed | — |
| | | | | | |

## Verifier 选择（去 bias）

为了让 3 verifier 独立思考：

- **Verifier A**：同角色（如 PM finding 给另一个 PM-audit 实例）
- **Verifier B**：相邻角色（PM finding 给 Architect-audit 实例）
- **Verifier C**：反向角色（PM finding 给 Debugger-audit 实例，反驳更狠）

## Finding 列表（Synthesis 阶段从 findings/*.md 聚合）

```
- ROLE-NNN: title (severity, category)
  - Verifier A: ...
  - Verifier B: ...
  - Verifier C: ...
  - Verdict: confirmed | likely | disputed | rejected
  - Notes: ...
```

## 拒绝原因（rejected 项记录）

避免下次重复提出同样的 finding。

| Finding | Rejected Reason |
|---|---|
| (e.g.) DEV-005 | "Already mitigated by upstream sync; ROI too low" |