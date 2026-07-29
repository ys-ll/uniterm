# Verification 矩阵 · 3 Verifier × Finding

每条 finding 走 3 verifier：**real?** · **necessary?** · **ROI reasonable?** · **destructive?** · **high complexity?**

| Verdict | 含义 |
|---|---|
| **confirmed** | 3/3 ✓ |
| **likely** | 2/3 ✓ |
| **disputed** | 1/3 ✓ |
| **rejected** | 0/3 ✓ |

## 当前 verification 状态

| Finding | V1 (real?) | V2 (necessary?) | V3 (ROI?) | Verdict | Notes |
|---|---|---|---|---|---|
| F-001 | ✓ | ✓ | ✓ | confirmed | IPv6 失败可复现 |
| F-002 | ✓ | ✓ | ✓ | confirmed | deps outdated 客观事实 |
| F-003 | ✓ | ✓ | ⚠ | likely | pinia 2→4 ROI 待评估 |
| F-004 | ✓ | ✓ | ✓ | confirmed | npm audit 0 vuln |
| F-005 | ✓ | ⚠ | ✓ | likely | OSS 标准是软要求 |

## Verifier 选择（防止 bias）

每个 finding 找 3 个 verifier 子代理：

- **V1**：同 lens 实例（如 F-001 是研发视角的 bug，找另一个研发视角 verifier）
- **V2**：相邻 lens（研发 bug 找 QA verifier，看测试角度）
- **V3**：反向 lens（研发 bug 找架构 verifier，质疑设计假设）

## Rejected 原因记录

(无)

## 流程

1. 找到 finding 后，先写到 findings.md
2. 跑 3 verifier 反驳
3. 根据 verdict 决定保留 / 进 backlog / reject
4. 更新本矩阵的 verdict 列