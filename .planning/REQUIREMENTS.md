# REQUIREMENTS — Milestone v0.1: Refactor (Audit + Conservative Fix)

> Requirements for the audit-and-fix cycle. No new features, no behavior changes.
> Each REQ-ID maps to exactly one phase. See `.planning/ROADMAP.md` for traceability.

---

## Audit (Phase 1)

- [ ] **AUDIT-01**: Phase 1 dispatches ≥4 parallel `gsd-code-reviewer` (or equivalent) sub-agents covering distinct module boundaries — minimum: `backend/session/`, `backend/database/`, `backend/store/`, `backend/k8s/`, `backend/sync/`, `frontend/src/` — each with fresh context.
- [ ] **AUDIT-02**: Each agent produces a module report under `.planning/audit/phase-1/<module>.md` with severity-classified findings (P0 critical / P1 high / P2 medium / P3 low / informational).
- [ ] **AUDIT-03**: Each agent reads `.planning/codebase/CONCERNS.md` and the relevant module section of `ARCHITECTURE.md` / `STRUCTURE.md` as baseline; does not duplicate concerns already documented unless it can sharpen them.
- [ ] **AUDIT-04**: Findings include file path, line range, concrete failure scenario, suggested fix category — no vague "consider refactoring" items.
- [ ] **AUDIT-05**: Aggregator summary `.planning/audit/phase-1/SUMMARY.md` ranks all findings by severity × confidence and surfaces cross-cutting concerns (build-tag split issues, shared types, etc.).

## Re-audit + ROI Triage (Phase 2)

- [ ] **VERIFY-01**: Phase 2 dispatches a second batch of sub-agents — at least one per Phase-1 module report — whose sole job is to refute, sharpen, or confirm each finding independently.
- [ ] **VERIFY-02**: Each finding gets one of: `CONFIRMED`, `PLAUSIBLE`, `FALSE_POSITIVE`. Refutations must cite a reason (not just "looks fine").
- [ ] **VERIFY-03**: Each `CONFIRMED` finding receives an ROI score (`high` / `medium` / `low`) considering: defect severity, fix cost (lines / risk), blast radius, test coverage available.
- [ ] **VERIFY-04**: Aggregator `.planning/audit/phase-2/TRIAGE.md` lists the queue to fix (`CONFIRMED` + `high|medium ROI`) and the discard pile (`FALSE_POSITIVE` / `low ROI`) with reasoning.

## Conservative Fix (Phase 3)

- [ ] **FIX-01**: Only Phase-2-`CONFIRMED` findings with `high` or `medium` ROI enter Phase 3. `low` ROI and unconfirmed findings are deferred or discarded.
- [ ] **FIX-02**: Each fix is minimal diff — no opportunistic refactoring, no formatting churn, no comment rewrites outside the change.
- [ ] **FIX-03**: No behavior changes. No public-facing UX changes. No new exports / no removed exports. No dependency upgrades unless the fix targets a known CVE or upstream regression explicitly.
- [ ] **FIX-04**: Build-tag split files (`*_darwin.go`, `*_windows.go`, `*_not*.go`, `*_unix.go`) are touched only when the finding explicitly applies to the non-target platform — never opportunistically.
- [ ] **FIX-05**: Each fix is one atomic commit; commit message references the originating finding ID (e.g. `fix(session): resolve SSH reconnect race — addresses AUDIT-session-12`).
- [ ] **FIX-06**: `frontend/wailsjs/` regenerated bindings stay in sync with any `app*.go` change (`wails dev` rebuild); no manual edits to generated files.

## Verification (Phase 4)

- [ ] **VAL-01**: Phase 4 re-audits each fixed site — confirms the original finding no longer reproduces and the surrounding behavior is intact.
- [ ] **VAL-02**: Regression sweep: existing Go tests (`go test ./backend/...`) and frontend build (`npm --prefix frontend run build`) pass after each fix batch.
- [ ] **VAL-03**: `.planning/audit/phase-4/REPORT.md` ties each fix back to its originating finding and records: pre-state, post-state, regression status.
- [ ] **VAL-04**: Any fix that fails VAL-01/VAL-02 is reverted (atomic `git revert`) and the finding returns to the Phase-2 triage queue with note "fix_attempt_failed".

---

## Future Requirements

(Deferred to v0.2+ — explicitly out of scope for v0.1.)

- Dependency hygiene pass: bump Go modules / npm packages, retire abandoned ones.
- Test coverage uplift for under-tested packages (`backend/database/`, `backend/sync/`).
- Architectural overhaul of any subsystem that the audit surfaces as fundamentally fragile (separate milestone).

## Out of Scope

- Adding features, fixing cosmetic issues, renaming for taste.
- Schema or data-format migrations.
- macOS-only / Windows-only behavior changes driven by platform taste.
- Performance work without a concrete profiling signal.

## Traceability

| REQ-ID | Phase | Status |
|--------|-------|--------|
| AUDIT-01..05 | Phase 1 — Comprehensive Audit | pending |
| VERIFY-01..04 | Phase 2 — Re-audit + ROI Triage | pending |
| FIX-01..06 | Phase 3 — Conservative Fix | pending |
| VAL-01..04 | Phase 4 — Verification | pending |

*Last updated: 2026-07-28 — Milestone v0.1 requirements defined.*