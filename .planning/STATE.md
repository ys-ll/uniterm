---
milestone: v1.0 PR Submission
milestone_name: PR Submission
status: planning
progress:
  current_phase: 1
  total_phases: 21
  completed_phases: 0
---

# STATE — uniterm v1.0 PR Submission

## Current Position

Phase: 1 (defining requirements + roadmap complete; awaiting user approval)
Plan: —
Status: Planning artifacts written; awaiting go-ahead to start Phase 1 (PR-01)
Last activity: 2026-07-28 — Milestone v1.0 PR Submission started; PROJECT/REQUIREMENTS/ROADMAP written

## Pending Decisions

- [ ] User confirms `/gsd-discuss-phase 1` (PR-01 Terminal render-compat) ready to plan+execute
- [ ] gh CLI authentication (`gh auth login`) — required before `gh pr create` in any phase

## Context

This milestone is non-standard GSD: instead of feature development, the "phase" unit is a single GitHub PR submitted from `coderstory/uniterm` (fork) → `ys-ll/uniterm` (upstream).

Per-phase execution involves:
1. Worktree isolation per PR
2. Cherry-pick commits from main fork
3. Strip `Co-Authored-By: Claude ...` trailers
4. Substantive code review + tests + refactor
5. Push branch + `gh pr create --repo ys-ll/uniterm`

Quality gates (per PR): `npm build` + `go test` must pass; tests added; review notes captured.

## Decisions

- **2026-07-28**: Adopt 21-PR split (per `/tmp/pr_split_plan.md`).
- **2026-07-28**: Reference only 5 strong-match upstream issues (#288/#312/#415/#418/#424).
- **2026-07-28**: Strip all `Co-Authored-By: Claude ...` trailers.
- **2026-07-28**: Per-PR quality gate (review + tests + refactor + local build/test).
- **2026-07-28**: Drop fork commits that overlap with already-merged upstream PRs (#302/#303/#414).

## Blockers

None.

## Todos

(None yet — generated per phase via `/gsd-discuss-phase N`.)