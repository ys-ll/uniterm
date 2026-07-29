# uniterm — Project

## What This Is

uniterm is a Wails v2 + Vue 3 desktop terminal application, forked from [ys-ll/uniterm](https://github.com/ys-ll/uniterm). The fork (`coderstory/uniterm`) has **179 commits ahead of upstream main**, covering:

- 99 perf-audit performance / memory fixes (F-001..F-413 series)
- 8 OS-themes / Soft Gray / Win11 terminal theme enhancements
- 12 store hardening (atomic writes, mutex, symlink guards, AAD)
- 6 database security fixes (identifier escape, pool hardening, query timeout)
- AI/agent hardening (XSS sanitize, risk enum, tool input validation, SSE typing)
- K8s auth round-tripper + watch/log reconnect
- Sync security (whitelist, decrypt-fail guard, ChangePassword salt)
- Session security / performance (SSH buffer, local PTY deadlock, FTP connMu)

The fork's goal is to evolve into a **first-class open source project** with high code quality, comprehensive test coverage, and up-to-date dependencies.

## Core Value

Build a polished, well-tested, well-documented, first-class open source terminal application. Code quality, stability, and OSS standards matter as much as features.

## Constraints

1. **First-class OSS** — README, CONTRIBUTING, CHANGELOG, LICENSE, GH templates all present and complete
2. **Security-first** — No SQLi / XSS / command injection / unsafe deserialization
3. **Test coverage** — Each fix/add comes with unit tests
4. **Cross-platform** — Windows / macOS / Linux all supported; abstraction isolated
5. **Dependency hygiene** — No stale deps beyond minor; no security advisories
6. **Documentation** — i18n 9 locales; code comments explain WHY

## Architecture

Same as CLAUDE.md — Wails v2 + Vue 3 frontend, Go backend by protocol/package, JSON store, encrypted git sync.

---

## Current Milestone: v1.1 Audit

**Goal:** Produce a comprehensive, verified checklist of issues across the codebase. **Audit only — no code changes in this milestone.**

**Deliverables:**
- 7 role-based audit reports in `.planning/audit/findings/{role}.md`
- 6 matrices in `.planning/audit/matrices/` (coverage / severity-category / role-coverage / verification / risk-impact / milestone-map)
- Each finding verified by 3 independent verifier subagents
- INDEX.md synthesizing findings by category + future milestone

**Methodology:**
- 7 audit roles (PM / Architect / Developer / QA / Reviewer / Debugger / Mapper)
- Automated scanners (go vet / npm audit / outdated)
- 3-verifier adversarial verification per finding
- Severity × Category × ROI × Risk × Future Milestone matrices

**Quality bar:** Every finding must be:
- Real (verified by 3 independent verifiers)
- Necessary (vs accepting status quo)
- ROI-justified (cost vs benefit)
- Risk-assessed (destructive? high-complexity?)
- Routable to a specific future milestone

---

## Future Milestones (consume audit findings)

| Milestone | Version | Focus | Source of work |
|---|---|---|---|
| v1.2 | Bug Fixes | All P0/P1 bugs, stability, error handling | audit findings: category=bug, severity P0/P1 |
| v1.3 | Performance Fixes | Go/Vue perf, memory, hot paths | audit findings: category=perf |
| v1.4 | Code Refactoring | Design consistency, duplication, abstraction | audit findings: category=refactor |
| v1.5 | Dependency Updates | Stale deps, security advisories | audit findings: category=deps |
| v1.6 | OS Compatibility | Cross-platform abstraction, build tags | audit findings: category=os-compat |
| v1.7 | Test Coverage Boost | Missing tests, boundary cases | audit findings: category=test |
| v1.8 | Architecture Improvements | Module boundaries, API design | audit findings: category=arch |
| v1.9 | Documentation / OSS | README, i18n, templates, license | audit findings: category=docs |

Each future milestone is launched via `/gsd-new-milestone` and consumes the corresponding slice from `audit/matrices/milestone-map.md`.

---

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition:**
1. Findings invalidated? → Move to Out of Scope
2. Findings validated? → Track in milestone-map.md
3. New findings emerged? → Add to next audit cycle
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone:**
1. Full review of all sections
2. Core Value check — still right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

## Key Decisions

- **2026-07-29**: Adopt 7-role audit framework (PM / Architect / Developer / QA / Reviewer / Debugger / Mapper) per ADPM v2
- **2026-07-29**: Audit-only v1.1 milestone; no code changes; future milestones consume audit findings
- **2026-07-29**: 3-verifier adversarial verification for every finding (≥2/3 confirmed)
- **2026-07-29**: 6 matrices for tracking (coverage / severity-category / role-coverage / verification / risk-impact / milestone-map)
- **2026-07-29**: Whitelist `.claude/agents/` in `.gitignore` so audit subagent definitions are tracked
- **2026-07-29**: 8 future milestones split by category (v1.2 bug / v1.3 perf / v1.4 refactor / v1.5 deps / v1.6 os-compat / v1.7 test / v1.8 arch / v1.9 docs)

Last updated: 2026-07-29