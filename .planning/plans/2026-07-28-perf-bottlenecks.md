# Performance Bottleneck + Claude Code Compatibility Audit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run 5 parallel static-audit agents + consolidator against the uniterm codebase, fill the spec's findings tables (P0/P1/P2 + Claude Code render-compat), and produce Go micro-benchmarks for the top candidates — without modifying any production code or in-flight files.

**Architecture:** Five read-only parallel agents (terminal_io / storage_db / wails_bridge / ai_llm / k8s_sync_startup) each scan their subsystem and emit a JSON findings file. A consolidator agent deduplicates, ranks severity, and groups fixes into PR-style batches. Top 3-5 P0/P1 findings get Go micro-benchmarks (untracked). The orchestrator (us) validates schemas, applies consolidator output to spec sections 3-7 / 9.1 / 10, and commits the updated spec.

**Tech Stack:** Go (static analysis + `testing.B` benchmarks), Vue 3 / TypeScript / xterm.js (frontend read-only), Wails v2 (bridge surface read-only).

## Global Constraints

- **No production code changes** — agents read only; orchestrator edits spec markdown + creates new `bench_test.go` files only.
- **In-flight files are read-only** — agents may report findings about them but must not modify:
  - `backend/session/output_log.go`
  - `backend/session/ftp_session.go`
  - `backend/session/session.go`
  - `backend/sync/git.go`
  - `backend/sync/sync_service.go`
  - `frontend/src/services/agent.ts`
  - `frontend/src/utils/runtimeTypeCheck.ts`
- **Bench files stay untracked** — `*_bench_test.go` files are written but `git add` is **NEVER** called on them. (User preference: "不执行 git add".) They appear in `git status` as untracked.
- **Spec IS committed** — `.planning/specs/2026-07-28-perf-bottlenecks-design.md` updates are committed normally.
- **Finding schema is enforced** — every finding must match the JSON schema in spec §2.3 exactly. Consolidator rejects malformed entries.
- **Finding category** — values are: `allocation | locking | io | serialization | memory | algorithmic | caching | render_compat`.
- **Severity** — P0 = everyday use exposes it; P1 = specific scenarios / high load; P2 = theoretical. See spec §2.5.
- **Claude Code `render_compat` findings** — must include concrete reproduction trigger (which escape sequence / mode / theme breaks).

---

### Task 1: Write agent prompts and commit to spec §10

**Files:**
- Modify: `.planning/specs/2026-07-28-perf-bottlenecks-design.md` §10 (replace placeholder with the 5 agent prompts)

**Interfaces:**
- Produces: 5 fully-formed agent prompts, one per subsystem. Each prompt is the exact text a subagent will receive when dispatched.

- [ ] **Step 1.1: Draft `terminal_io` prompt**

The prompt must include:
- Scope files (spec §2.2 row 1)
- Special Claude Code checklist (italic `\e[3m`, mode 2026 synchronized output, ambiguous-width chars, code-block bg `\e[48;5;...m`, mouse / bracketed paste, alt screen `\e[?1049h`, lineHeight)
- Category list including `render_compat`
- Read-only constraint including in-flight file list
- Output: JSON array, no prose

- [ ] **Step 1.2: Draft `storage_db` prompt**

Scope: `backend/store/*.go`, `backend/database/{engine,executor,provider_*}.go`. Focus: fsync frequency, lock granularity, JSON marshalling, pool contention, query N+1.

- [ ] **Step 1.3: Draft `wails_bridge` prompt**

Scope: `app.go`, `app_*.go` (non build-tag split), `frontend/wailsjs/go/main/App.d.ts`, store calls into Wails. Focus: large event payloads, EventsEmit frequency, marshal cost, sync-blocking calls, whether `net/http/pprof` is exposed.

- [ ] **Step 1.4: Draft `ai_llm` prompt**

Scope: `frontend/src/services/{agent,llm,terminalAgent}.ts`, `stores/aiStore.ts`, AI proxy in `app.go`. Focus: multi-turn context growth, streaming token handling, prompt concatenation, retry/backoff, JSON parsing.

- [ ] **Step 1.5: Draft `k8s_sync_startup` prompt**

Scope: `backend/k8s/*.go`, `backend/sync/*.go`, `backend/update/*.go`, `main.go`, `app.go` startup path. Focus: watch reconnect, REST response body caps, git blocking startup, store/sync init ordering, cold-start.

- [ ] **Step 1.6: Replace §10 placeholder with the 5 prompts**

Edit `.planning/specs/2026-07-28-perf-bottlenecks-design.md` §10. Use fenced code blocks per prompt with a header like `### Agent: terminal_io`.

- [ ] **Step 1.7: Commit**

```bash
git add .planning/specs/2026-07-28-perf-bottlenecks-design.md
git commit -m "docs(planning): add 5 audit agent prompts to perf-bottlenecks spec

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Dispatch `terminal_io` audit agent

**Files:**
- Create: `.planning/findings/terminal_io.json`

**Interfaces:**
- Consumes: prompt from spec §10 (`### Agent: terminal_io`)
- Produces: JSON array of findings, schema per spec §2.3

- [ ] **Step 2.1: Dispatch the agent**

Use the Agent tool with `subagent_type: "general-purpose"` and pass the `terminal_io` prompt verbatim from spec §10. Set `run_in_background: true` if dispatching alongside Tasks 3-6 in the same orchestrator call.

- [ ] **Step 2.2: Wait for completion**

Block on the agent notification.

- [ ] **Step 2.3: Validate output**

Write the agent's JSON output to `.planning/findings/terminal_io.json`. Validate:
- Top-level is a JSON array
- Every entry has all schema fields (`id`, `file`, `line`, `subsystem`, `severity`, `category`, `root_cause`, `evidence`, `impact`, `fix_sketch`, `verification`)
- `subsystem === "terminal_io"` for every entry
- `category` is in the allowed list

If validation fails, re-dispatch with the validation error appended.

- [ ] **Step 2.4: Sanity-check the file**

Read the file back. Confirm no findings reference in-flight files as "should modify" — they should only appear in `evidence` / `impact` notes.

- [ ] **Step 2.5: No commit (intermediate file)**

`.planning/findings/terminal_io.json` stays untracked. (Or committed at end as a single batch — your choice; document the choice.)

---

### Task 3: Dispatch `storage_db` audit agent

**Files:**
- Create: `.planning/findings/storage_db.json`

**Interfaces:**
- Consumes: prompt from spec §10 (`### Agent: storage_db`)
- Produces: JSON array per spec §2.3

- [ ] **Step 3.1: Dispatch with `subagent_type: "general-purpose"`, run_in_background if batching**

- [ ] **Step 3.2: Wait for completion**

- [ ] **Step 3.3: Validate**

Same as Task 2.3, but `subsystem === "storage_db"`.

- [ ] **Step 3.4: Sanity-check**

Confirm no findings propose edits to in-flight files.

---

### Task 4: Dispatch `wails_bridge` audit agent

**Files:**
- Create: `.planning/findings/wails_bridge.json`

**Interfaces:**
- Consumes: prompt from spec §10 (`### Agent: wails_bridge`)
- Produces: JSON array per spec §2.3

- [ ] **Step 4.1: Dispatch**

- [ ] **Step 4.2: Wait**

- [ ] **Step 4.3: Validate (`subsystem === "wails_bridge"`)**

- [ ] **Step 4.4: Sanity-check**

Pay particular attention: agent should explicitly check whether `app.go` imports `net/http/pprof` and exposes an endpoint. If not, that's likely a P0/P1 finding.

---

### Task 5: Dispatch `ai_llm` audit agent

**Files:**
- Create: `.planning/findings/ai_llm.json`

**Interfaces:**
- Consumes: prompt from spec §10 (`### Agent: ai_llm`)
- Produces: JSON array per spec §2.3

- [ ] **Step 5.1: Dispatch**

- [ ] **Step 5.2: Wait**

- [ ] **Step 5.3: Validate (`subsystem === "ai_llm"`)**

- [ ] **Step 5.4: Sanity-check**

`agent.ts` is in-flight — confirm no proposed edits, only observations.

---

### Task 6: Dispatch `k8s_sync_startup` audit agent

**Files:**
- Create: `.planning/findings/k8s_sync_startup.json`

**Interfaces:**
- Consumes: prompt from spec §10 (`### Agent: k8s_sync_startup`)
- Produces: JSON array per spec §2.3

- [ ] **Step 6.1: Dispatch**

- [ ] **Step 6.2: Wait**

- [ ] **Step 6.3: Validate (`subsystem === "k8s_sync_startup"`)**

- [ ] **Step 6.4: Sanity-check**

`backend/sync/git.go` and `backend/sync/sync_service.go` are in-flight — observations only.

---

### Task 7: Dispatch consolidator agent

**Files:**
- Create: `.planning/findings/consolidated.json`
- Modify: `.planning/specs/2026-07-28-perf-bottlenecks-design.md` §3, §4, §5, §6, §7, §9.1

**Interfaces:**
- Consumes: 5 files from Tasks 2-6
- Produces: structured consolidated output (see step 7.3)

- [ ] **Step 7.1: Draft consolidator prompt**

Prompt must instruct the consolidator to:
- Load all 5 `.planning/findings/*.json` files (excluding `consolidated.json`)
- Deduplicate: same root_cause across files → merge into one finding, list all files in evidence
- Cross-subsystem associations (e.g. AI context bloat ↔ bridge serialization)
- Validate severity assignment against spec §2.5
- Group fixes into PR-style batches referencing the existing batch naming (`fix(...): batch N — ...`)
- Output JSON with sections: `findings` (deduped), `cross_subsystem`, `pr_batches`, `top_for_bench` (top 3-5 P0/P1 by expected impact)

- [ ] **Step 7.2: Dispatch with `subagent_type: "general-purpose"`**

Pass the 5 file paths and the consolidator prompt.

- [ ] **Step 7.3: Validate consolidator output**

`consolidated.json` must contain `findings`, `cross_subsystem`, `pr_batches`, `top_for_bench` keys.

- [ ] **Step 7.4: Apply §1 (Top 5 P0) + §3 (summary table)**

Replace §1 placeholder `Top 5 P0 ...` with the top 5 P0 entries from `consolidated.findings` (one-line each).

Replace §3 placeholder with a markdown table built from `consolidated.findings`. Columns: `id | file:line | subsystem | severity | category | root_cause`. Sort P0 first, then by file:line.

- [ ] **Step 7.5: Apply §4 (P0 detail)**

Replace §4 placeholder. For each P0 in `consolidated.findings`, write: `### F-NNN — short title` then `根因 / 证据 / 影响 / 修复 / 验证` from the JSON fields.

- [ ] **Step 7.6: Apply §5 (P1 detail)**

Same shape as §4, denser formatting.

- [ ] **Step 7.7: Apply §6 (P2 list)**

Replace §6 placeholder with one bullet per P2 from `consolidated.findings`.

- [ ] **Step 7.8: Apply §7 (PR batches)**

Replace §7 placeholder. For each entry in `consolidated.pr_batches`, write a `### Batch N: <title>` subsection with the batched files + 1-line description per file.

- [ ] **Step 7.9: Apply §9.1 (in-flight file observations)**

Replace §9.1 placeholder. List each in-flight file with the observations noted by agents but explicitly NOT being addressed in this round.

---

### Task 8: Write micro-benchmarks for top 3-5 findings

**Files:**
- Create: `backend/<package>/bench_test.go` — one per benchmarked finding. **NEVER `git add` these.**

**Interfaces:**
- Consumes: `consolidated.top_for_bench` list (3-5 entries with file:line + root_cause)
- Produces: `BenchmarkXxx` functions that prove severity

- [ ] **Step 8.1: Determine benchmark locations**

For each `top_for_bench` entry, identify the function/struct under test. Decide the package:
- `backend/session/bench_test.go` for terminal_io / session hot paths
- `backend/store/bench_test.go` for store contention
- `backend/database/bench_test.go` for query / pool hot paths

If a finding has no obvious micro-bench target (e.g. xterm render, Claude Code render_compat), skip it and note in spec §8.

- [ ] **Step 8.2: For each chosen finding, write a `BenchmarkXxx`**

Template:

```go
package session

import (
    "testing"
)

func BenchmarkSuspectHotPath(b *testing.B) {
    // Setup mirroring real workload (small buffer / many writes)
    var buf []byte
    data := bytes.Repeat([]byte("a"), 64)
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // The actual code path under suspicion
        buf = append(buf, data...)
    }
    _ = buf
}
```

Each benchmark must:
- Call `b.ReportAllocs()`
- Mirror realistic input size (small terminal chunks / store records)
- Avoid `b.N` external dependencies

- [ ] **Step 8.3: Verify each benchmark compiles**

```bash
go test -bench=. -benchtime=1x ./backend/<package>/...
```

Expected: PASS (or at minimum compile + run the 1-iteration case).

- [ ] **Step 8.4: Confirm files are untracked**

```bash
git status --short backend/*/bench_test.go
```

Expected: `?? backend/<package>/bench_test.go` (untracked, never staged).

- [ ] **Step 8.5: Document each bench in spec §8.1**

Append to `.planning/specs/2026-07-28-perf-bottlenecks-design.md` §8.1:

```
- `BenchmarkSuspectHotPath` (backend/session/bench_test.go) — 验证 F-NNN 的 <短描述>
  运行: go test -bench=BenchmarkSuspectHotPath -benchmem ./backend/session/...
```

---

### Task 9: Spec self-review pass 2

**Files:**
- Modify: `.planning/specs/2026-07-28-perf-bottlenecks-design.md` (any inline fixes)

- [ ] **Step 9.1: Placeholder scan**

Read the spec. Any "TBD", "TODO", "执行完成后填入" left behind? Fix.

- [ ] **Step 9.2: Internal consistency**

Do findings tables match P0/P1/P2 detail sections? Do PR batch files match the findings' files? Fix any drift.

- [ ] **Step 9.3: Scope check**

Did the audit pull in unrelated items? Trim.

- [ ] **Step 9.4: Ambiguity check**

Any finding that could be interpreted two ways? Tighten the wording.

---

### Task 10: Commit updated spec + handoff

**Files:**
- Modify: `.planning/specs/2026-07-28-perf-bottlenecks-design.md`
- (Possibly) commit `.planning/findings/*.json` for traceability

- [ ] **Step 10.1: Decide on findings JSON**

Ask user whether `.planning/findings/*.json` should be committed (audit trail) or stay untracked. Default: commit for traceability.

- [ ] **Step 10.2: Commit**

```bash
git add .planning/specs/2026-07-28-perf-bottlenecks-design.md
git add .planning/findings/   # only if Step 10.1 says commit
git commit -m "docs(planning): fill perf-bottlenecks audit findings

- 5 subsystem agents returned N findings (P0: x, P1: y, P2: z)
- Consolidated into M deduped items
- PR batches: <list>
- Top findings backed by Go micro-benchmarks (untracked)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

- [ ] **Step 10.3: Verify bench files remain untracked**

```bash
git status --short backend/*/bench_test.go
```

Expected: still `??` (untracked).

- [ ] **Step 10.4: User review handoff**

Tell the user: "Spec + findings committed at `<commit>`. Bench files at `<paths>` (untracked). Review §3-7 for the consolidated bottleneck list. Confirm before we move to writing-plans for the actual fix batches (if you want me to plan the fixes next)."

---

## Self-Review Notes (post-write)

- **Spec coverage**: §3-7, §9.1 all filled by Task 7. §8 enriched by Task 8. §10 from Task 1.
- **Placeholders**: none in final spec after Tasks 7-9.
- **Type consistency**: N/A — no code types cross tasks; only JSON shapes.
- **Bench files**: explicitly untracked per user constraint; reaffirmed in Task 8.4 and 10.3.
- **In-flight files**: enforced in Tasks 2-6 sanity checks.