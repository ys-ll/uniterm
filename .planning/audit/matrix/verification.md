# Verification 矩阵 · 3 Verifier × Finding（最终态 · v1.1 Audit 收口）

## Verdict 聚合规则

- **confirmed** — 3/3 verifier 同意「真实 + 必要 + ROI 合理」
- **likely** — 2/3 verifier 同意
- **disputed** — 1/3 verifier 同意
- **rejected** — 0/3 verifier 同意

## Verifier 立场（防 bias）

- **V1**：同 lens 实例 → 主动 refute，**关键路径验证**（最严格，发现 12 条 hallucination）
- **V2**：相邻 lens → 用户影响 / workaround 评估
- **V3**：反向 lens（skeptic） → ROI / 修复成本反驳

## V1 Re-verification 结果（2026-07-30）

V1 主动反驳了 13 条 finding，派 1 个 re-verification agent 复核，结论：

### 🔴 RETRACT（12 条 · FABRICATED）

| ID | 撤回原因 |
|---|---|
| ARCH-032 | `backend/ai/*.go` 不存在 |
| DEV-016 | `backend/session/multi_session.go` 不存在 |
| DEV-022 | `backend/sync/upload.go` 不存在 |
| DEV-024 | `backend/ai/llm.go` 不存在 |
| QA-018 | `backend/ai/llm.go` 不存在 |
| DBG-018 | `backend/ai/llm.go` 不存在 |
| DBG-021 | `backend/ai/client.go` 不存在 |
| REV-016 | `sanitizeRenderedHtml()` 已存在 (AIMessage.vue:640) |
| PM-016 | `README_zh-CN.md` 已存在（274 行）|
| ARCH-027 | sync 已用 `go-git v5.19.1` |
| ARCH-030 | sync 已用 `x/crypto/pbkdf2` |
| F-002 | 90 deps 是 normal minor/patch，无 CVE/破坏 |

### 🟡 CORRECT（1 条）

| ID | 修正内容 |
|---|---|
| F-011 | 去掉 `platform/`（有 fonts_ttf_test.go），保留 `log/` + `update/` |

## 最终 verdict 分布

| Verdict | 数量 | 来源 |
|---|---|---|
| **confirmed** | ~70 | 严格 3-verifier 通过 + V1 已回扫 |
| **likely** | ~10 | 部分 verifier 不同意 |
| **disputed** | 0 | — |
| **rejected** | 1 | F-005（OSS 文件已存在）|
| **withdrawn** | 5 | DEV-028/035/043/044 + ARCH-021（早期 subagent 自审撤回）|
| **retracted by V1** | 12 | hallucination / inverted claim |
| **corrected by V1** | 1 | F-011 |
| **Total** | **97** | |

**有效 finding**：97 - 5 withdrawn - 12 retracted = **80 effective**

## Verifier 立场分布（最终态）

| Verifier | 立场 | 关键发现 |
|---|---|---|
| **V1**（real?）| 同 lens 反查 | **14 条需复核**（12 撤回 + 1 修正 + 1 重复确认）|
| **V2**（necessary?）| 相邻 lens | 22 necessary / 59 optional / 16 unnecessary |
| **V3**（ROI?）| 反向 lens | 30 high / 41 medium / 19 low / 7 skip |

## V2 必要性裁决（22 条必修）

必修（V2 verdict = necessary）：
- F-001, F-006, F-009, F-010, F-012, F-014
- PM-019, PM-023, PM-024
- ARCH-017 (dup F-001), ARCH-023 (dup F-006), ARCH-031
- DEV-038 (dup F-014)
- REV-015 (P0), REV-016 (已 retract), REV-017 (TLS), REV-018 (cmd inj), REV-019 (SFTP)
- DBG-017 (nil/nil), DBG-019 (dial hang), DBG-021 (已 retract), DBG-022 (recover)

去重后：**18 条独立必修**（REV-016/DBG-021 已 retract，ARCH-017/023/DEV-038 是 dup）

## V3 ROI 裁决（30 条高 ROI）

high ROI（V3 verdict = high）：
- F-001, F-006, F-008, F-009, F-010, F-011 (corrected), F-014
- PM-019, PM-023, PM-024
- ARCH-015, ARCH-031
- DEV-015, DEV-017, DEV-018, DEV-036, DEV-037
- QA-015, QA-023, QA-024
- REV-015 (P0), REV-018, REV-019
- DBG-017, DBG-021 (已 retract), DBG-022

去重 + retract 后：**~22 条独立 high ROI**

## 流程说明

每条 finding 经历 3 层审查：
1. **Subagent 自审** — 10 项 checklist（finding_id / severity / location / category / 5 步 lifecycle / ROI / 多角度 / Test Plan / 矩阵同步 / 不越界）
2. **V1 反查** — 同 lens 实例主动 refute，找 hallucination / inverted claim
3. **V2 + V3** — 必要性 + ROI 评估

**最终通过率**：
- 97 条 → 80 effective（撤 12 + 5 withdrawn + 1 rejected）
- 0 条 disputed
- 12 条 hallucination 发现（V1 价值证明）

## Rejected / Retracted 完整原因

| ID | 原因 | Verifier |
|---|---|---|
| F-005 | OSS 文件已存在 | V1 早期 |
| DEV-028 | 与 F-014 重叠 | subagent 自审 |
| DEV-035 | 已正确实现 | subagent 自审 |
| DEV-043 | Go 编译器优化免费 | subagent 自审 |
| DEV-044 | 已正确实现 | subagent 自审 |
| ARCH-021 | Go 编译器优化免费 | subagent 自审 |
| **ARCH-032** | `backend/ai/` 不存在 | V1 |
| **DEV-016** | `multi_session.go` 不存在 | V1 |
| **DEV-022** | `upload.go` 不存在 | V1 |
| **DEV-024** | `backend/ai/llm.go` 不存在 | V1 |
| **QA-018** | `backend/ai/llm.go` 不存在 | V1 |
| **DBG-018** | `backend/ai/llm.go` 不存在 | V1 |
| **DBG-021** | `backend/ai/client.go` 不存在 | V1 |
| **REV-016** | `sanitizeRenderedHtml()` 已存在 | V1 |
| **PM-016** | `README_zh-CN.md` 已存在 | V1 |
| **ARCH-027** | sync 已用 `go-git` | V1 |
| **ARCH-030** | sync 已用 `x/crypto/pbkdf2` | V1 |
| **F-002** | 90 deps 是 normal 漂移 | V1 |