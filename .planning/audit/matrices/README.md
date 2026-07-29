# Audit Matrices — uniterm v1.1 Audit

6 个矩阵用于管控审计产出的 finding：

| 矩阵 | 目的 | 文件 |
|---|---|---|
| 1. Coverage | 模块 × 审计状态（哪些被审了，出了多少 finding）| [coverage.md](coverage.md) |
| 2. Severity × Category | 严重度 × 类别（P0-P3 × bug/perf/refactor/deps/config/os-compat/test/arch/docs） | [severity-category.md](severity-category.md) |
| 3. Role × File | 每个角色覆盖了哪些文件 / 包 | [role-coverage.md](role-coverage.md) |
| 4. Verification | 3 verifier × finding 反驳结果 | [verification.md](verification.md) |
| 5. Risk × Impact | ROI 决策矩阵（破坏性 × 复杂度 × 收益） | [risk-impact.md](risk-impact.md) |
| 6. Future Milestone | finding × 哪个未来 milestone 消化 | [milestone-map.md](milestone-map.md) |

## 使用流程

1. **审计阶段**：每个角色 subagent 写完 `findings/{role}.md` 后，追加 1 行到 coverage.md 和 role-coverage.md
2. **Synthesis**：合成时把所有 finding 聚合到 severity-category.md 和 milestone-map.md
3. **Verification**：每个 finding 走 3 个独立 verifier，写到 verification.md
4. **Final review**：risk-impact.md 给出「该不该修」的最终建议
5. **Future milestones 启动**：每个 milestone 从 milestone-map.md 抽对应类别