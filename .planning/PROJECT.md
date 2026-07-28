# PROJECT

## What This Is

**uniterm** — Wails v2 + Vue 3 (xterm.js 终端) + Go desktop terminal.
Cross-platform (macOS / Windows / Linux) supporting SSH/FTP/SFTP/SMB/WebDAV/S3/RDP/VNC/SPICE, local & serial terminals, Kubernetes, relational/non-relational databases, and an autonomous multi-turn AI Agent.

## Core Value

One desktop app that consolidates terminal, file transfer, remote desktop, k8s, database clients, and AI agent workflows — replacing a stack of separate tools.

## Current Milestone: v0.1 — Refactor (Audit + Conservative Fix)

**Goal:** 通过多轮审计找出技术债务与隐患，保守修复，零新增缺陷。

**Phases:**
1. **Phase 1 — Comprehensive Audit:** multi-agent audit by module (session/database/store/k8s/sync/platform/frontend); emit findings list.
2. **Phase 2 — Re-audit + ROI Triage:** second-pass agents verify Phase 1 findings, score ROI, drop false positives.
3. **Phase 3 — Conservative Fix:** address only Phase 2-confirmed, high-ROI issues; minimal diff, no new bugs.
4. **Phase 4 — Verification:** re-audit fixed sites to confirm resolution and zero regression.

**Constraints:**
- One branch per refactor milestone (`refactor/codebase-audit`, based on latest `main`).
- Fixes must be conservative, revertible, minimal.
- Each phase commits independently.
- No feature additions, no behavior changes, no dependency upgrades.

## Active Requirements

See `.planning/REQUIREMENTS.md` for REQ-IDs and traceability.

## Validated Requirements

None yet (this is the first GSD-tracked milestone; legacy features predate GSD and are out of scope for v0.1).

## Out of Scope

- Feature additions or behavior changes (audit-only + bug-class fixes).
- Dependency upgrades (Go modules, npm packages) unless a finding is *caused* by a known CVE / upstream fix.
- Refactors driven by style/preference with no concrete defect or ROI.
- Anything that touches public-facing UX in a user-visible way.
- Database migration of existing user data.

## Context

- Brownfield project brought into GSD via `/gsd-map-codebase`.
- `.planning/codebase/` (7 docs) is the audit baseline — STACK/INTEGRATIONS/ARCHITECTURE/STRUCTURE/CONVENTIONS/TESTING/CONCERNS.
- `gsd-tools.cjs` / `gsd-sdk` not installed locally → SDK queries replaced with direct file edits + `git` for commits (matches prior `/gsd-map-codebase` commit pattern).
- Working branch: `refactor/codebase-audit` (off `main @ c07c19e`).
- Recent commits concentrate on terminal/PTY plumbing — that area may be near-stable and audit can deprioritize it.

## Key Decisions

- **ROI-gated fixes:** only fix findings confirmed by Phase 2 re-audit with clear defect, security, or maintainability benefit.
- **Multi-agent audit by module:** one agent per backend sub-package + one for frontend; avoids token blow-up and yields module-specific reports.
- **Conservative-first fixes:** when in doubt, leave as-is; bias toward "no new BUG" over "more elegant."
- **Per-phase commits:** each phase ends with `git commit`, so any phase is independently revertible.

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---

*Last updated: 2026-07-28 — Milestone v0.1 started*