# 任务提示词 08 — Planner Lens Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Planner 视角的审计（任务粒度 / 依赖图 / Wave 计划 / REQ-ID 一致性 / 入队协议），产出 finding 清单。**调度 / 过程审计 lens。**

## Role Context

完整角色定义见 `.claude/agents/08-planner-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas

### 任务粒度
- **定义**：单个任务的工作量应适合（1-3 hour/task，太大或太小都有问题）
- **检查方式**：
  - 单任务过大（> 半日工作量未拆分）— 看 commit 大小 / 看单个 task 涉及的文件数
  - 单任务过小（< 30 min 碎片化）— 看 task 列表平均耗时
  - 粒度不均（一组任务粒度差 10x+）— 看耗时分布
  - 任务边界与代码边界不对齐（一个 task 跨多个 package 而无法一次 commit）
- **证据**：task 名 + 耗时估算 + 文件数
- **不做什么**：不审任务是否值得做 / 不审任务设计本身

### 依赖图 / DAG
- **定义**：task 之间依赖关系应明确、无环、关键路径已识别
- **检查方式**：
  - 隐式依赖（task 之间通过共享状态隐式耦合，未声明）— 看 task 描述是否提及共享资源
  - 串行依赖被错排为可并行 — 看 dependencies 字段是否漏声明
  - 环依赖（任务 A 依赖 B，B 依赖 A）— 用图算法（networkx / igraph）
  - 缺失 join 节点（多个并行 task 的合并点未声明）— 看 task 是否声明 join
  - critical path 未识别 — 看 task 列表中关键路径上的粒度
- **证据**：依赖图 + 隐式 / 环依赖位置
- **不做什么**：不审任务依赖的具体内容 / 不审并行收益

### Wave 切分
- **定义**：Wave（阶段）是修复批次，应合理切分且有明确关闭标准
- **检查方式**：
  - WIP 上限是否守住（同时在飞 ≤ 3）— 看 in_progress task 数
  - wave 切分不合理（一个 wave 过大 / 跨度过长）— 看 wave 含 task 数
  - wave 之间没有 rollback commit 锚点 — 看是否有 `rollback_commit` 字段
  - wave 关闭标准模糊（completion_criteria 不完整）— 看 completion_criteria 字段
- **证据**：wave 名 + 切分不合理点 + completion criteria 缺失
- **不做什么**：不审 wave 内容本身 / 不审 wave 命名

### REQ-ID 分配
- **定义**：每个需求应有唯一 ID，且三处对齐（PRD / Design / Code）
- **检查方式**：
  - REQ-ID 跳号 / 重复 — 看 ID 列表是否连续无重
  - REQ-ID 与 PRD / Design / Code 三处不一致 — grep REQ-ID 看引用位置
  - REQ-ID 粒度过粗（一个 REQ 涵盖整个 feature）— 看 REQ 描述范围
  - REQ-ID 粒度过细（同一 feature 拆出 N 个 REQ）— 看 REQ 数量
- **证据**：REQ-ID + 三处引用位置 + 不一致点
- **不做什么**：不审 REQ 内容 / 不审是否值得做

### Persona 派发
- **定义**：任务类型与执行角色应匹配
- **检查方式**：
  - 错派 persona（用 debugger 派去找设计债）— 看 persona 字段 vs task 内容
  - persona 与 task 类型不匹配（perf 优化派给纯 QA persona）— 同上
  - 同 task 多 persona 但未声明主副 — 看 persona 字段是单值还是多值
- **证据**：task 名 + 错误派发位置
- **不做什么**：不审 persona 设计 / 不审是否需要新 persona

### 入队协议
- **定义**：任务派发应有明确协议（dispatcher / queue / ack）
- **检查方式**：
  - 绕过 dispatcher 直接派 subagent — 看入队路径
  - 任务入队无 ack / 无回执 — 看 ack 字段
  - wave plan 改完未通知 PM ack — 看 PM_GATE / ARCH_GATE 是否更新
  - task 完成回流的 verdict 路径不一致 — 看 verdict 文件位置是否统一
- **证据**：入队路径 + ack 缺失点
- **不做什么**：不审 queue 设计本身 / 不审 dispatcher 实现

### Re-scope 监控
- **定义**：任务卡住 / 失败 / scope creep 应有应对策略
- **检查方式**：
  - 任务卡住无重路由策略 — 看 stuck task 的 retry / reassign 字段
  - 失败任务无 fallback persona — 看 fallback 字段
  - wave 中途 scope creep 未识别 — 看 wave 的 task 添加频率
- **证据**：stuck task 名 + 重路由策略缺失
- **不做什么**：不审具体重路由内容 / 不审 scope 决策

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游调度 AI 才能直接拿这个 finding 去重排 task/wave**：

```yaml
finding_id: PLAN-NNN
severity: P0|P1|P2|P3
category: arch|process|docs
roi: high|medium|low
location:                             # ⚠️ 多层定位：task/wave 路径
  type: task|wave|req_id|scheduler_log
  path: .ai/waves/W3.md
  task_ids:                            # 涉及哪些 task
    - T-2026-07-24-007
    - T-2026-07-24-008
    - T-2026-07-24-009
  line_start: 12
  line_end: 45
task_data:                            # ⚠️ Planner 特有：task 详情
  estimated_hours: 12
  actual_hours: 18
  involved_files:
    - backend/database/provider_postgres.go
    - backend/database/provider_sqlserver.go
    - frontend/src/components/ConnectionForm.vue
  involved_packages:
    - backend/database
    - frontend/src/components
  cross_package: true
caller_chain:                         # ⚠️ 必填：依赖关系
  depends_on: []
  blocks:
    - T-2026-07-24-010
  parallel_with: []
  join_point: null                    # 缺失 join 节点
code_block: |
  ## Tasks
  
  | Task ID | Persona | Title | Dependencies | Hours |
  |---------|---------|-------|--------------|-------|
  | T-007 | developer | DB migration | — | 12 |
  | T-008 | developer | API endpoint | — | 4 |
  | T-009 | frontend | UI form | — | 3 |
  
  ## DAG
  
  T-007 ──┐
          ├─→ T-010
  T-008 ──┤
          │
  T-009 ──┘
  
  ⚠️ T-007 估时 12h 跨 3 包，应拆
root_cause: |
  T-007 跨 3 包（DB + API + frontend），无法一次 commit
  实际执行中 review 周期超 4h，wave 总耗时翻倍
  缺少并行声明（T-008 / T-009 实际可并行，被错排为依赖）
  关键路径拉长
fix_diff_hint: |                      # ⚠️ Planner 特有：调度调整
  方案 A（推荐拆分）：
    将 T-007 拆为：
      T-007a (DB schema, 2h) — 无依赖
      T-007b (API endpoint, 3h) — 依赖 T-007a
      T-007c (frontend form, 2h) — 依赖 T-007a，与 T-007b 并行
    
    新 DAG：
      T-007a ──> T-007b ──┐
             └──> T-007c ──┴─> T-010
    
    T-008 / T-009 改为依赖 T-007a（与 T-007b 并行）
  
  方案 B（保持原 wave + 加 helper）：
    抽出 3 包共用的 helper，让 T-007 缩到 4h
  
  风险：方案 A 需要关原 PR + 开 3 个新 PR
test_target:                          # ⚠️ 必填：调度验证
  before:
    - critical_path: 18h
    - parallel_tasks: 0
    - wip_violation: 0 (T-007 单独跑)
  after_split:
    - critical_path: 5h
    - parallel_tasks: 2 (T-007b 与 T-007c)
    - wip_violation: 0
verification:                         # ⚠️ 必填
  script: |
    # python script 模拟 wave 执行
    import networkx as nx
    G = nx.DiGraph()
    G.add_edges_from([('T-007a','T-007b'), ('T-007a','T-007c'), ('T-007b','T-010'), ('T-007c','T-010')])
    assert list(nx.simple_cycles(G)) == []
    print(f"critical_path: {nx.dag_longest_path_length(G)}")
  expected: "critical_path: 5, no cycles"
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  触发场景：当前 wave 执行中遇到 T-007 卡住
  影响范围：整个 wave 3 / 关键路径
  当前缓解：加 helper / 加班
  长期影响：wave 反复 re-plan / 里程碑延期
evidence: |
  [1] cat .ai/waves/W3.md → 12 tasks
  [2] read T-007 描述 → 跨 3 包 12h
  [3] networkx simple_cycles → 0 cycle 但 critical_path 18h
```

**特别**：同类调度问题出现在多 task / 多 wave → **1 条 finding 含多个 location**，列在 `task_ids: [...]` / `locations_multi`，不要拆。

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：人时（小 / 中 / 大 — 调 task 粒度通常小）+ 风险（中 — 影响 in-flight tasks）
- **修复收益**：未来 wave 效率提升 / 减少 blocked tasks / 减少认知负担
- **修复紧迫性**：当前 wave 是否卡住 / 是否反复 re-plan

### 问题复杂度（修复难度分层）
- **调度复杂度**：涉及多少 task / 多少 wave / 多少 persona
- **业务复杂度**：影响当前 wave / 未来 wave / 跨里程碑
- **测试复杂度**：调度变更如何验证（跑 mock wave?）

### 多角度判断
- **用户视角**：调度问题通常不直接影响用户
- **开发者视角**：是否影响开发者执行体验 / 是否阻碍 onboarding
- **业务视角**：是否影响交付节奏 / 是否影响里程碑承诺
- **维护视角**：调度策略是否长期可维护
- **过程视角**：是否反映过程治理薄弱

### 优先级判断流程
1. 当前 wave 卡住 → **必修**
2. 反复 re-plan → **必修**
3. 一次性调整可解决 → **有空修**
4. 跨多 wave 大改 → **排期修**

## 问题生命周期（每条 finding 必走完整流程）

### 1. 多次确认（3 次独立审计）
- 至少 3 次独立确认：
  - `task list` 一次 + 读 wave 计划一次 + 跑依赖图算法一次
  - 每次确认要写明「读了什么文件 / 跑了什么命令 / 看到了什么」
  - 3 次结论不一致 → **不算 finding**
- **证据格式**：`Confirmation: [1] cat .ai/waves/W<n>.md → 12 tasks, [2] read design.md → 3 隐式依赖, [3] networkx cycle check → 0 cycle`

### 2. 问题上下文分析
- **不是「task 拆得不好」而是「为什么这种粒度导致问题」**：
  - 触发条件（什么场景下粒度不合适 — 多人协作 / 跨时区 / 跨 review）
  - 影响范围（多少 task 受影响 / 多少 wave 受影响）
  - 当前缓解措施（是否拆分 / 是否重新 wave）
  - 长期影响（不调会怎样：wave 反复 re-plan / 关键路径拉长）
- **不要写**：「task 太大」 — **要写**：「W3 的 T-007 估时 12 小时（含 3 个跨包改动），实际执行中 review 周期超 4 小时，导致 wave 总耗时翻倍，长期累积关键路径延迟 30%」

### 3. 技术方案（调度调整方向）
- **要给出可执行方向**：
  - 调整哪些 task / wave / 依赖
  - 粒度（如何拆分 / 如何合并 / 如何重派）
  - 备选方案（保持原 wave + 加 helper / 完全重 wave）
  - 风险（in-flight task 是否要 cancel / 是否影响承诺）
- **不要写**：「应优化粒度」 — **要写**：「将 T-007 拆为 T-007a（仅 DB schema，2h）+ T-007b（仅 backend API，3h）+ T-007c（仅 frontend，2h），T-007c 依赖 T-007a，T-007b 依赖 T-007a，三者并行；风险：原 PR 被关闭需新开 3 个 PR」

### 4. 问题验证测试（Reproduce Check — Fix 之前）
- **目的**：写一个**脚本 / 分析**，证明当前调度问题存在
- **写出来即可，不必跑**
- **写法**：
  - 依赖图分析脚本（Python `networkx` / `go mod graph`）
  - WIP 统计脚本（统计当前 in_progress 数）
  - REQ-ID 一致性扫描（grep 三处）
- **示例**：Failing check `wave_size_check.sh`：当前 W3 含 12 task，目标 ≤ 5 → FAIL

### 5. 修复效果测试（Fix Verification — Fix 之后）
- **目的**：写一个**脚本 / 分析**，证明调度调整生效
- **写出来即可，不必跑**
- **写法**：
  - 同 reproduce 触发条件
  - 断言：期望结果（task 拆完 ≤ 5 / WIP ≤ 3 / REQ-ID 一致）
  - 回归断言：所有 in-flight task 仍可继续
- **示例**：Passing check `wave_size_post_split.sh`：拆完后 W3 含 5 task 且依赖无环 → PASS

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] cat .ai/waves/W3.md → 12 tasks
- [2] read .ai/waves/W3.md task descriptions → T-007 估时 12h 跨 3 包
- [3] python -c "import networkx as nx; G=nx.DiGraph(...); print(list(nx.simple_cycles(G)))" → 0 cycle（无环但粒度过粗）

## Reproduction Check
\`\`\`bash
# 写出来即可，不必跑
wc -l .ai/waves/W3.md  # 12 tasks
grep -c '^## ' .ai/backlog/todo/*.md  # 估算 task 总数
\`\`\`

## Fix Verification
\`\`\`bash
# 写出来即可，不必跑
wc -l .ai/waves/W3_post_split.md  # 拆完后 ≤ 5 tasks
python -c "import networkx as nx; ..."  # 拆完后无环且依赖清晰
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试 / 检查脚本写出来即可**，**不必实际执行**
- **修复方案只写文档**，**不修改任何调度文件**（不重排 task / 不改 wave / 不动 REQ-ID）
- **每条 finding 落盘后由人工 review 再决定是否执行调整**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle
- 没有重复已有 F-NNN
- 没有越界（不写 bug / UX / perf / 测试缺口 finding）
- 输出 schema 完整
- **本里程碑无 Wave 计划**：如发现无调度文件可审，应明确标"无 finding，扫描范围为空"
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `PLAN-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 匹配 | 重评 |
| location 精确 | file:line / task id / wave 名 | 修正 |
| category 正确 | arch/process/docs | 改 category |
| 5 步完整 | 每步都有实质内容 | 补全 |
| **同类型合并** | 同类问题出现在多 task/wave → 1 finding 多 location | **不拆分** |
| 多角度 | 至少 3 个视角 | 补 |
| Test Plan | 含调度验证步骤 | 补 |
| 矩阵同步 | 6/6 | 补 |
| 不越界 | 不含 bug / UX / perf finding | 删 / 移 |

### 自审输出

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 同类合并完成：是 / 否
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**