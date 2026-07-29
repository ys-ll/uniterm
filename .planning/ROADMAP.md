# uniterm — Roadmap: v1.1 Audit

## Overview

**Audit-only milestone.** This milestone produces a verified, prioritized checklist of issues. **No code changes.**

## Phases

| # | Phase | Goal | Output |
|---|---|---|---|
| 1 | Infrastructure | Scaffold + matrices + auto-scan | `.planning/audit/{matrices,findings/auto-scan.md}` |
| 2 | Parallel Audits | 7 role-based subagents | `.planning/audit/findings/{role}.md` |
| 3 | Verification | 3-verifier per finding | `.planning/audit/matrices/verification.md` |
| 4 | Synthesis | INDEX.md + populate matrices | `.planning/audit/findings/INDEX.md` |

---

## Phase 1: Infrastructure ✅

**Goal:** Scaffold the audit infrastructure.

**Done:**
- 7 audit subagent definitions in `.claude/agents/`
- 6 matrices in `.planning/audit/matrices/`
- Auto-scan findings (go vet, npm outdated, npm audit) in `.planning/audit/findings/auto-scan.md`
- `.gitignore` whitelist for `.claude/agents/`
- PROJECT.md + STATE.md

**Output:** Audit infrastructure ready

---

## Phase 2: Parallel Audits 🔄

**Goal:** Run 7 role-based audits in parallel.

**Agents (all in `.claude/agents/`):**
- pm-audit: UX, docs, OSS standards (target 30-50 findings)
- architect-audit: module boundaries, OS abstraction, design consistency (40-80)
- developer-audit: Go/Vue perf, memory, hot paths (40-80)
- qa-audit: test coverage, boundary cases, AC verifiability (30-60)
- reviewer-audit: 6-dim matrix (correctness/quality/security/perf/maintainability/test) (50-100)
- debugger-audit: bug hunting, P0-P3 (30-60)
- mapper-audit: dead code, orphan, RTM (20-50)

**Acceptance:**
- [ ] All 7 agents complete
- [ ] Each writes findings to `.planning/audit/findings/{role}.md`
- [ ] Each appends row to `.planning/audit/matrices/role-coverage.md`
- [ ] Total findings: 240-580 (across all 7 roles)
- [ ] No role's findings are entirely empty (means audit was too shallow)

---

## Phase 3: Verification ☐

**Goal:** Adversarial verification — each finding challenged by 3 independent verifiers.

**Method:**
- For each finding, spawn 3 verifier subagents:
  - Verifier A: same role as the finding (e.g., PM finding → PM-audit)
  - Verifier B: adjacent role (PM finding → Architect-audit)
  - Verifier C: reverse role (PM finding → Debugger-audit, to be skeptical)
- Each verifier answers: real? necessary? ROI reasonable? destructive? complex?
- Verdict: confirmed (3/3), likely (2/3), disputed (1/3), rejected (0/3)

**Output:** `.planning/audit/matrices/verification.md`

**Acceptance:**
- [ ] All findings verified
- [ ] Rejected findings documented with reasons
- [ ] Confirmed + likely findings proceed to synthesis

---

## Phase 4: Synthesis ☐

**Goal:** Consolidate verified findings into INDEX.md and populate matrices.

**Tasks:**
1. Aggregate all role findings → master list
2. Populate `severity-category.md` (count by severity × category)
3. Populate `role-coverage.md` (which files covered by which roles)
4. Populate `risk-impact.md` (ROI decision per finding)
5. Populate `milestone-map.md` (which future milestone eats each finding)
6. Write `INDEX.md` — top-level summary by category, sorted by severity + ROI

**Output:** `.planning/audit/findings/INDEX.md`

**Acceptance:**
- [ ] INDEX.md has all confirmed+likely findings
- [ ] Sorted by future milestone → severity → ROI
- [ ] Each finding has clear evidence + suggested fix + test plan
- [ ] Rejected findings documented (avoid re-raise later)

---

## Coverage

- **7 audit roles** covering all 11 dimensions from user's original ask
- **~240-580 findings** expected across roles
- **6 matrices** for tracking state
- **8 future milestones** (v1.2-v1.9) to consume findings

Last updated: 2026-07-29