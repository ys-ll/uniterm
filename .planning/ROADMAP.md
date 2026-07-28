# ROADMAP — Milestone v0.1: Refactor (Audit + Conservative Fix)

**Milestone:** v0.1
**Phases:** 4
**Requirements mapped:** 19 (5 AUDIT + 4 VERIFY + 6 FIX + 4 VAL)
**Coverage:** All v0.1 requirements covered ✓

---

## Phase 1 — Comprehensive Audit

**Goal:** 通过多个模块维度的子代理审计，输出一份带严重程度与置信度分级的全量问题清单。

**Requirements:** AUDIT-01, AUDIT-02, AUDIT-03, AUDIT-04, AUDIT-05

**Success criteria:**
1. ≥6 parallel sub-agent reports exist under `.planning/audit/phase-1/` (one per module boundary).
2. Each report has severity tags (P0–P3) and concrete file/line references — zero vague items.
3. `.planning/audit/phase-1/SUMMARY.md` ranks all findings by severity × confidence, surfaces cross-cutting concerns, and is committed.

**Module boundaries (suggested, may adjust):**
- `backend/session/` — all protocol sessions + manager + tunnel/mosh/zmodem support
- `backend/database/` — providers + engine + executor
- `backend/store/` — JSON persistence (connections, ai, settings, history, etc.)
- `backend/k8s/` — kubeconfig + client + watch + metrics + manager
- `backend/sync/` — crypto + git + keychain + remote sync orchestration
- `frontend/` — Vue components, composables, stores, services, types

---

## Phase 2 — Re-audit + ROI Triage

**Goal:** 另派子代理独立验证 Phase 1 的发现是否真实存在，剔除伪阳性，按 ROI 分级排队待修。

**Requirements:** VERIFY-01, VERIFY-02, VERIFY-03, VERIFY-04

**Success criteria:**
1. ≥1 sub-agent per Phase-1 module report has re-examined each finding with verdict `CONFIRMED` / `PLAUSIBLE` / `FALSE_POSITIVE`.
2. Every `CONFIRMED` finding has an ROI tag (`high` / `medium` / `low`).
3. `.planning/audit/phase-2/TRIAGE.md` separates the fix queue (CONFIRMED + high|medium ROI) from the discard pile (FALSE_POSITIVE / low ROI) with reasoning, and is committed.

---

## Phase 3 — Conservative Fix

**Goal:** 仅修复 Phase 2 确证的高 ROI 问题，偏保守、最小改动、不引入新 BUG。

**Requirements:** FIX-01, FIX-02, FIX-03, FIX-04, FIX-05, FIX-06

**Success criteria:**
1. Fix commit count equals the fix-queue length from Phase 2 (no fix escapes the queue, no extra fix is invented).
2. Each fix commit's diff is minimal — verified by `git show --stat` review: small, focused, no incidental churn.
3. No commit touches build-tag split files opportunistically; no commit adds a dependency; no commit changes public-facing UX.
4. `go test ./backend/...` and `npm --prefix frontend run build` pass after the full Phase 3 batch.
5. All fix commits reference their originating finding ID in the message.

---

## Phase 4 — Verification

**Goal:** 对每个修复点重新审计，确认问题已修复、未引入回归。

**Requirements:** VAL-01, VAL-02, VAL-03, VAL-04

**Success criteria:**
1. `.planning/audit/phase-4/REPORT.md` ties every fix commit back to its originating finding with pre/post/regression status.
2. Final test pass: `go test ./backend/...` + `npm --prefix frontend run build` green.
3. Any failing fix has been reverted (`git revert <commit>`) and the finding is re-queued with note "fix_attempt_failed".
4. Phase 4 commit lands and branch is ready for review / merge into `main`.

---

## Phase Dependencies

```
Phase 1 ──► Phase 2 ──► Phase 3 ──► Phase 4
```

Strict order: each phase's outputs are inputs to the next.

---

*Last updated: 2026-07-28 — Milestone v0.1 roadmap created (4 phases).*