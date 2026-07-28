# uniterm — Project

## What This Is

uniterm 是一个 Wails v2 + Vue 3 的桌面终端应用，fork 自 [ys-ll/uniterm](https://github.com/ys-ll/uniterm)。当前 fork (`coderstory/uniterm`) 在 `main` 分支上有 **170 个 commits ahead of upstream main**，覆盖：

- 99 个 perf-audit 性能 / 内存修复（F-001..F-413 系列）
- 8 个 OS-themes / Soft Gray / Win11 终端主题增强
- 12 个 store 加固（原子写、互斥、对称链接守卫、AAD）
- 6 个 database 安全修复（identifier escape、pool 硬化、query timeout）
- AI/agent 强化（XSS sanitize、风险枚举、tool 输入校验、SSE 类型化）
- K8s auth round-tripper + watch/log reconnect
- Sync 安全（白名单、解密失败守卫、ChangePassword salt）
- Session 安全 / 性能（SSH buffer、local PTY 死锁、FTP connMu）

**Project Code:** uniterm-pr-milestone

## Core Value

把 fork 的改进**干净、聚焦、可审**地回写到 upstream。每个 PR 只解决一个问题，每个 PR 都附带单元测试和针对性 review，避免一次性提交巨量 commit 让 upstream 维护者无法 review。

## Constraints

1. **Third-party submission** — PRs go to a maintainer who will scrutinize. **No new bugs allowed.**
2. **PR scope** — small enough to review in <30 minutes; touches related code only.
3. **Tests required** — 每个 PR 都要有对应单元测试覆盖新代码路径。
4. **Refactor before push** — clean code, remove dead code, run linter.
5. **No Co-Authored-By AI trailers** — strip all `Co-Authored-By: Claude ...` trailers.
6. **Reference upstream issues** — only the 5 strong-match open issues (#288/#312/#415/#418/#424). Other PRs are unprefixed.
7. **Strip plan/audit docs** — `docs(planning): *` and `docs:` commits stay in fork, never go to PR.
8. **No rebase-of-stolen work** — PRs that conflict with already-merged upstream (#302/#303/#414) drop the conflicting commits cleanly.

## Architecture

Fork → upstream submission pipeline:

```
git log origin/main..HEAD         # 170 commits
        │
        ▼
group by topic → 21 PRs           # /tmp/pr_split_plan.md
        │
        ▼
map to upstream issues            # /tmp/upstream_issues.md
        │
        ▼
create worktree per PR            # branch: pr/<topic>-<n>
        │
        ▼
cherry-pick commits onto branch
        │
        ▼
quality gate: review + tests + refactor + npm build + go test
        │
        ▼
push branch to coderstory/uniterm
        │
        ▼
gh pr create --repo ys-ll/uniterm  # cross-fork PR
        │
        ▼
upstream maintainer review
```

## Current Milestone: v1.0 PR Submission

**Goal:** 把 170 个 fork commits 拆成 21 个聚焦 PR，提交到 upstream `ys-ll/uniterm`，每个 PR 通过质量门（review + tests + refactor + 本地 build/test 全绿）。

**Target deliverables:**
- 21 个 PR 在 GitHub 上 open (cross-fork: coderstory → ys-ll)
- 每个 PR 关联正确的 upstream issue (5 个强匹配 + 16 个无 issue)
- 每个 PR 自带单元测试
- 每个 PR 经过实质性 review 和必要的重构
- 不引入新 bug，所有本地 build/test 通过

**Tools used:**
- `git worktree` per PR (隔离 workspace)
- `gh pr create` 自动化 PR 创建
- `go test ./...` 验证后端
- `npm --prefix frontend run build` 验证前端

Last updated: 2026-07-28

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

## Key Decisions

- **2026-07-28**: Adopt 21-PR split (vs. 12-PR condensed) — smaller scope wins for upstream review.
- **2026-07-28**: Reference only 5 strong-match upstream issues (#288/#312/#415/#418/#424); other 16 PRs are unprefixed.
- **2026-07-28**: Strip all `Co-Authored-By: Claude ...` trailers from final PR commits.
- **2026-07-28**: Per-PR quality gate: review + tests + refactor + `npm build` + `go test` must pass.
- **2026-07-28**: Drop fork commits that overlap with already-merged upstream PRs (#302, #303, #414). Affected commits: `567e1e4`, `4bb5afa` (Linux flash — covered by #303), `main.go` macOS Edit menu (covered by #414).
- **2026-07-28**: Keep `Co-Authored-By` strip default ON for all 21 PRs unless user overrides per-PR.