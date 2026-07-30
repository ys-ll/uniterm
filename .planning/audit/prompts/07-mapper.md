# 任务提示词 07 — Mapper (Codebase Cartographer) Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Mapper 视角的死代码 / 孤儿文件 / Test 盲区 / RTM 违规审计。**只标嫌疑，不删**。 每条 finding 必须含 last known usage / 证据 / manual review 提示。

## Role Context

完整角色定义见 `.claude/agents/07-mapper-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas

### 死代码候选
- **定义**：代码存在但不被调用 / 引用
- **检查方式**：
  - 函数无 caller：`codegraph callers <funcName>` / `grep -rn '<funcName>(' --include='*.go'`
  - 类型无实例化：`grep -rn 'new<TypeName>\|<TypeName>{}'`
  - 常量无引用：`grep -rn '<ConstName>'`
  - 不可达分支：`return` 之后的代码
  - exported 只在 test 用：`grep -rn '<funcName>(' --include='*_test.go'`（仅 test caller）
  - internal helper 只 1 个 caller：可能可直接 inline
- **证据**：last usage git blame + 调用链
- **不做什么**：不审 API 必要性 / 不审是否应该被外部用

### 孤儿文件
- **定义**：源文件存在但无引用 / 无入口
- **检查方式**：
  - 源文件无 package users：`codegraph callers <file>` 看 import 该文件的 package 数
  - 视图组件无 import：`grep -rn '<ComponentName>' frontend/src/`
  - 样式未引用：CSS 文件无 `import` / 无 `<link>`
  - 静态资源未用：`find frontend/src/assets -type f` 比对实际引用
- **证据**：孤儿文件路径 + 无引用证据
- **不做什么**：不审是否值得保留 / 不审归档策略

### Test 盲区
- **定义**：包内源文件无测试文件 / 关键 function 零测试
- **检查方式**：
  - `find <pkg> -name '*.go' ! -name '*_test.go'` 看源文件数
  - `find <pkg> -name '*_test.go'` 看测试文件数
  - 比对：源文件 vs 测试文件（1:N 比例）
  - 主要 exported function 是否有测试：`go test -coverprofile`
- **证据**：pkg 名 + 缺测函数列表
- **不做什么**：不审测试设计本身（QA 视角）

### RTM (Requirement Traceability Matrix)
- **定义**：公开元素是否被实际使用
- **检查方式**：
  - exported function 是否有 caller（`codegraph callers`）
  - 是否有 test（`go test` 跑通）
  - Bind 方法是否被前端调用（`grep -rn 'Backend.<MethodName>' frontend/`）
  - 框架事件是否被监听（`grep -rn 'EventsOn('<event>'`）
  - state store 是否被 dispatch（`grep -rn '<storeName>.<action>'`）
- **证据**：RTM 矩阵 + 缺链位置
- **不做什么**：不审是否应该被使用 / 不审是否值得保留

### 跨包 internal 访问
- **定义**：跨包访问未导出 / internal 元素
- **检查方式**：
  - test helpers 访问未导出 item
  - internal packages 从父 tree 外访问
  - test-only exports 在 production 残留（`export_test.go` 内容渗漏）
- **证据**：访问位置 + 应抽到 internal
- **不做什么**：不审 internal 包边界设计

### Public API Surface
- **定义**：注册的 API 是否被使用
- **检查方式**：
  - Wails Bind 方法（`app.go` 内）是否有前端 caller
  - 框架事件 emitted vs listened 不平衡（emit 多于 listen = 后端推无人接）
  - state store actions 是否被 dispatch
  - Pinia store getters 是否被组件用
- **证据**：未使用的 API / 事件 / store action
- **不做什么**：不审 API 设计 / 不审是否值得保留

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游 AI 才能直接拿这个 finding 去删除/迁移**：

```yaml
finding_id: MAP-NNN
severity: P0|P1|P2|P3
category: refactor|test|arch
roi: high|medium|low
location:                             # ⚠️ 多层定位
  file: backend/ai/legacy_provider.go
  function: OldLLMProvider
  line_start: 1
  line_end: 200
  symbol: "整个文件"
last_usage:                           # ⚠️ Mapper 特有：最后使用时间
  commit: abc123def
  date: 2024-03-15
  author: alice
  message: "feat: remove legacy LLM provider"
  note: "commit message 说删除，但文件实际仍在仓库"
caller_chain:                         # ⚠️ 必填：当前 caller 数
  caller_count: 0
  callers: []                          # codegraph callers 输出为空
  test_callers: 0                      # test 文件中的 caller
import_count:                         # 被多少 package import
  production: 0
  test: 0
code_block: |
  // Package legacy 标记为 deprecated 但仍存在
  package legacy
  
  type OldLLMProvider struct { ... }
  func (p *OldLLMProvider) Call(...) { ... }
root_cause: |
  历史重构遗留：commit abc123 标记删除但未实际 rm
  package import 链已断（0 caller），但文件仍在
  新开发者看到文件可能误用 / 浪费 onboarding 时间
fix_diff_hint: |                      # ⚠️ Mapper 特有：删除顺序 + 备选
  方案 A（推荐删除）：
    1. grep -rn 'legacy.OldLLMProvider' 确认 0 caller（已确认）
    2. 写删除前快照：git tag pre-delete-legacy
    3. rm backend/ai/legacy_provider.go
    4. go test ./... 验证编译通过
    5. 7 天后无回归 → git push
  
  方案 B（保守）：
    保留文件，在文件头加：
      // Deprecated: use NewLLMProvider instead. Removal planned 2026-Q3.
      //go:build legacy
  
  方案 C（迁移）：
    移到 internal/_legacy/ 目录（git history 保留但不影响 build）
  
  风险：误删（已通过 3 次确认 + caller 链 + git log 验证）
  ⚠️ 绝不自动执行删除 — 人工 review + 备份 + 分批执行
test_target:                          # ⚠️ 必填
  after_deletion:
    - go test ./... # 编译通过
    - grep -rn 'legacy' backend/ # 0 匹配
    - go vet ./...
verification:                         # ⚠️ 必填
  command: |
    rm backend/ai/legacy_provider.go
    go test ./...
    grep -rn 'legacy' backend/ frontend/src/
  expected: |
    - go test PASS
    - grep 输出空
  regression: git revert 路径明确（commit hash 保留）
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  为什么是死代码：重构遗留 / feature removed / 提前预留
  影响范围：误导新人 / 文档陈旧 / 文件膨胀
  当前缓解：无
  长期影响：维护负担 / 误用风险
evidence: |
  [1] codegraph callers OldLLMProvider → 0
  [2] grep -rn 'OldLLMProvider' --include='*.go' → 0 匹配
  [3] git log -p --all -S 'OldLLMProvider' → last usage 2024-03-15
```

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：人时（小 / 中 / 大）+ 风险（中 — 误删会破坏编译）
- **修复收益**：减少维护负担（少一个文件要维护）+ 减少认知负担（新人不用读 dead code）
- **修复紧迫性**：是否阻塞 onboarding / 是否被反复问到

### 问题复杂度（修复难度分层）
- **识别复杂度**：是否能静态证明死代码（caller = 0）
- **删除复杂度**：是否跨多文件 / 是否需同时删配置 / 是否需 migration
- **回归测试复杂度**：删除前需写哪些测试兜底

### 多角度判断
- **用户视角**：死代码不影响用户（间接价值）
- **开发者视角**：是否阻碍 onboarding / 是否被反复问到
- **业务视角**：是否反映已废弃的业务逻辑
- **安全视角**：dead code 是否有未打补丁的漏洞（dead but exploitable）
- **维护视角**：长期维护成本 / 文档陈旧风险

### 优先级判断流程
1. 死代码含已知 CVE → **必修**
2. dead but exploitable → **必修**
3. 简单删除 + 测试兜底 → **有空修**
4. 跨多文件删除 → **排期修**

## 问题生命周期（每条 finding 必走完整流程）

### 1. 多次确认（3 次独立审计）
- 至少 3 次独立确认：
  - `codegraph callers` 0 caller
  - `grep -rn '<funcName>' --include='*.go'` 0 匹配
  - `git log -p --all -S '<funcName>'` 看最后一次引用
- 3 次确认一致 → 才算死代码嫌疑
- **不确定的死代码不算 finding**（宁可漏报不误报）

### 2. 问题上下文分析
- 为什么是死代码（feature removed? refactor half-done? 提前预留?）
- 影响范围（多少文件 / 多少 package 见到）
- 替代品（是否被新实现替代 / 是否被 plugin 替代）
- 长期影响（不删会怎样：文档陈旧 / 误导新人 / 累积膨胀）

### 3. 技术方案（手动 review 方向）
- **绝不自动删**：列出候选 + 删除步骤 + 影响
- 给出删除顺序（先删 caller → 再删被调；先删 test → 再删 prod）
- 备选方案（保留 + 加 `// Deprecated:` / 移到 `_legacy/` / 归档）
- 风险（误删导致编译失败）

### 4. 问题验证测试（Reproduce Check — Fix 之前）
- **目的**：写一个**检查脚本 / 测试代码**，证明问题存在
- **写出来即可，不必跑**：测试 / 脚本作为规格说明落盘
- **写法**（Mapper lens 通常用工具）：
  - 静态检查脚本：`codegraph callers` / `grep` 反查
  - 集成测试：mock 触发场景，断言违反 RTM 规则
- **示例**：Failing check `codegraph_callers_<func>.sh`：跑后输出 `0 caller`，期望 ≥ 1 → FAIL

### 5. 修复效果测试（Fix Verification — Fix 之后）
- **目的**：写一个**检查脚本 / 测试代码**，证明 fix 生效
- **写出来即可，不必跑**
- **写法**：
  - 同 reproduce 触发条件
  - 断言：期望结果（caller 链现在存在 / dead code 已删）
  - 回归断言：所有现有功能编译通过 / 测试通过
- **示例**：Passing check `codegraph_callers_post_delete.sh`：修复后 caller 数为 0 且无编译错误 → PASS

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] `codegraph callers <funcName>` → 0 caller
- [2] `grep -rn '<funcName>' --include='*.go'` → 0 匹配
- [3] `git log -p --all -S '<funcName>'` → last usage 2024-03-15, commit abc123

## Reproduction Check
\`\`\`bash
# 写出来即可，不必实际跑
codegraph callers <funcName>
grep -rn '<funcName>' --include='*.go' --include='*.vue'
# 期望输出: 0 caller → 确认死代码 → FAIL（caller 期望 ≥ 1）
\`\`\`

## Fix Verification
\`\`\`bash
# 写出来即可，不必实际跑
go test ./...
# 修复后回归测试全绿
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试 / 检查脚本写出来即可**，**不必实际执行**（避免长时间运行 / 误触发副作用）
- **修复方案只写文档**，**不修改任何代码**（Mapper 误删会导致编译失败）
- **每条 finding 落盘后由人工 review 再决定是否执行修复**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle
- 没有重复已有 F-NNN
- 没有越界（不写 bug / perf / UX finding）
- 输出 schema 完整
- **绝不自动删**：所有删除建议都要人工 review 后才能执行
- **不确定的死代码不算 finding**（宁可漏报不误报）
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `MAP-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 匹配 | 重评 |
| location 精确 | file:line 真实存在 | 修正 |
| category 正确 | refactor/test/arch | 改 category |
| 5 步完整 | 每步都有实质内容 | 补全 |
| **last usage 必填** | **含 git blame / 最后 commit** | **缺失必返工** |
| **caller 链** | **codegraph 调用链列出** | **缺失必返工** |
| 多角度 | 至少 3 个视角 | 补 |
| Test Plan | 含回归测试 | 补 |
| 矩阵同步 | 6/6 | 补 |
| 不越界 | 不含 bug / perf / UX finding | 删 / 移 |
| 不自动删 | 任何删除建议都标"需人工 review" | 改写 |

### 自审输出

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 含 last usage：N/N
- 含 caller 链：N/N
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**