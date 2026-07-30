# 任务提示词 05 — Reviewer (6-Dimension) Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Reviewer 视角的 6 维审查（correctness / test_coverage / code_quality / **security** / performance / maintainability）。**Security findings 优先级最高 — SQLi / XSS / Command injection 即使轻微也要 flag。**

## Role Context

完整角色定义见 `.claude/agents/05-reviewer-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas

### correctness（正确性）
- **定义**：代码在边界 / 异常 / 并发条件下行为正确
- **检查方式**：
  - 并发安全（共享变量访问是否加锁 / 锁顺序是否一致）
  - 错误处理（每个 error 是否被检查 / 是否有 swallowed error）
  - 边界条件（空 / nil / MaxInt / 空字符串 / 空数组）
  - 类型转换（int ↔ int64 / string ↔ []byte / interface{} assertion 是否 panic-safe）
  - nil 检查（pointer / map / channel / slice 解引用前）
- **证据**：具体函数 + 触发输入
- **不做什么**：不审算法正确性 / 不审业务逻辑是否符合需求

### test_coverage（测试覆盖）
- **定义**：核心代码被测试覆盖
- **检查方式**：
  - 行覆盖 ≥ 80%
  - 分支覆盖 ≥ 70%
  - 公开 API 100% 覆盖
- **证据**：coverage report + 缺测函数名
- **不做什么**：不审测试设计本身（QA 视角）

### code_quality（代码质量）
- **定义**：代码风格 / 结构 / 命名 / 复杂度合理
- **检查方式**：
  - 圈复杂度 > 15（`gocyclo -over 15`）
  - 函数 > 50 行（`grep -c '' file`）
  - magic number（裸数字无命名常量）
  - 命名（统一驼峰 / 缩写一致 / 单词拼写）
  - 注释（公开 API 注释 / 复杂逻辑解释 / 链接到 issue）
- **证据**：具体行号 + 圈复杂度数字
- **不做什么**：不审代码格式化（gofmt 应已处理）/ 不审 license header

### security（安全 — 优先）
- **定义**：代码不存在已知攻击面
- **检查清单**（每项 grep / 读）：
  | 类型 | grep pattern | 风险 |
  |---|---|---|
  | SQL 注入 | `fmt.Sprintf` 内含 SQL 关键字 | SQLi |
  | 模板注入 | `template.HTML(userInput)` / `v-html="userInput"` | XSS |
  | 命令注入 | `exec.Command("sh", "-c", userInput)` / `os.system(userInput)` | RCE |
  | 反序列化 | `json.Unmarshal(userInput, &any)` / `gob.Decode` | RCE |
  | 路径拼接 | `path.Join(userInput, "file")` 无 sanitize | 任意文件读写 |
  | 凭据明文 | `password` / `secret` / `token` 在 log / store | 数据泄露 |
  | SSRF | HTTP client 用用户输入 URL | 内网访问 |
  | CSRF | Wails IPC 是否校验 origin | 跨站请求伪造 |
  | 弱加密 | `MD5` / `SHA1` / `DES` / `ECB` | 加密失效 |
  | 硬编码密钥 | grep `"-----BEGIN"\|password = ".*"` | 密钥泄露 |
- **证据**：具体行号 + 攻击 payload 示例
- **不做什么**：不审第三方依赖 CVE（deps lens 范畴）

### performance（性能）
- **定义**：热路径高效，不阻塞主流程
- **检查方式**：
  - N+1 查询（循环里查 DB）
  - 缺失索引（看 SQL + 表结构）
  - 热路径 copy（`copy(m1, m2)` 在循环）
  - 连接池未复用（每次 `sql.Open`）
  - 热路径缓存（可缓存但未缓存）
- **证据**：具体函数 + 调用频次
- **不做什么**：不审冷启动 / 不审 UI 渲染（Developer lens）

### maintainability（可维护性）
- **定义**：未来开发者能快速理解并安全修改
- **检查方式**：
  - 模块边界（依赖单向、无环）
  - 循环依赖（`go list` + 反查）
  - 抽象层次一致（接口 / 实现分层）
  - 配置 vs 硬编码（常量在 config 还是写死）
- **证据**：违反点 + 影响范围
- **不做什么**：不审命名风格 / 不审测试可维护性

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游 AI 才能直接拿这个 finding 去修复**：

```yaml
finding_id: REV-NNN
severity: P0|P1|P2|P3
category: bug|security|perf|arch
hat: arch-reviewer|skeptic|domain-reviewer|user-reviewer  # 4 帽子之一
dimension: correctness|test_coverage|code_quality|security|performance|maintainability  # 6 维之一
roi: high|medium|low
location:                             # ⚠️ 多层定位
  file: backend/database/provider_postgres.go
  function: CreateDatabase
  line_start: 294
  line_end: 305
  symbol: db.Exec(fmt.Sprintf("CREATE DATABASE %s", q(dbName)))
  locations_multi:                   # 多处同类安全漏洞
    - file: backend/database/provider_postgres.go
      function: DropDatabase
      line_start: 299
      line_end: 305
    - file: backend/database/provider_postgres.go
      function: AlterTable
      line_start: 412
      line_end: 416
    - file: backend/database/provider_sqlserver.go
      function: UseDatabase
      line_start: 52
      line_end: 56
code_block: |
  func (p *PostgresProvider) CreateDatabase(dbName string) error {
      db, err := p.connect()
      if err != nil { return err }
      defer db.Close()
      
      // ⚠️ SQL 拼接 + 未走 prepared statement
      _, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", q(dbName)))
      if err != nil { return err }
      ...
  }
caller_chain:
  - app.CreateConnection → Provider.CreateDatabase
  - app.MigrateConnection → Provider.CreateDatabase
attack_payload:                      # ⚠️ Security finding 特有
  vector: "dbName 参数可控"
  example: 'dbName = "foo; DROP TABLE users; --"'
  current_behavior: "执行恶意 SQL（identifier 注入 + DDL 拼接）"
  expected_behavior: "应被拒绝 / 转义"
root_cause: |
  fmt.Sprintf 拼接 identifier + 未走 prepared statement
  identifier 引号规则不对：PG 用 ", MySQL 用 `, MSSQL 用 []，跨 DB 兼容易错
  双重问题：1) SQL 注入 2) 未走 prepared（性能 + 缓存失效）
fix_diff_hint: |
  改 db.Exec 为 db.ExecContext(ctx, sql, args...) 形式
  args 用 ? 占位符
  DDL 无内置 prepared，应用层做白名单 / 转义兜底：
    func sanitizeIdent(s string) string {
        if !regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`).MatchString(s) {
            return ""
        }
        return s
    }
test_target:
  file: backend/database/provider_postgres_test.go
  function: TestCreateDatabase_SQLInjection
  fixture: dbName = "foo; DROP TABLE x;"
  assertion: 应当返回 error 而非执行恶意 SQL
  additional: TestCreateDatabase_ValidIdent（合法 name 应正常）
verification:
  command: go test ./backend/database/ -run TestCreateDatabase -v
  expected: PASS（恶意名拒绝，合法名成功）
  linter: gosec ./backend/database/ → 0 SQL injection warning
  regression: go test ./...
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  触发场景：恶意用户传特殊 dbName / 跨 DB 迁移时引号规则错
  影响范围：所有 10 处 DDL/DML
  当前缓解：仅 `q()` 包裹，但仍有注入 + 性能问题
  长期影响：被客诉 / 安全审计失败 / 合规问题
evidence: |
  grep -n 'fmt.Sprintf' backend/database/ | grep -i 'CREATE\|DROP\|ALTER' → 10 处
  gosec ./backend/database/ → 报 SQL injection warning
  read provider_postgres.go:294 → 确认拼接
```

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：人时（小 / 中 / 大）+ 风险（低 / 中 / 高）
- **修复收益**：影响用户数 + 出现频次 + 不可替代性
- **修复紧迫性**：是否阻塞主流程 / 是否反复修 / 是否被客诉

### 问题复杂度（修复难度分层）
- **技术复杂度**：跨文件数 / 跨包数 / 是否需新依赖
- **业务复杂度**：影响多少 user journey
- **测试复杂度**：需要多少 mock / fixture / 跨环境

### 多角度判断
- **攻击者视角**：漏洞窗口多大 / 利用难度 / 影响范围
- **用户视角**：用户能否感知 / 是否泄露隐私
- **开发者视角**：是否阻碍未来开发 / 是否被反复警告
- **合规视角**：是否触发 GDPR / SOC2 / 行业标准

### 优先级判断流程
1. Security finding（高危）→ **必修**
2. ROI 高 + 复杂度低 → **必修**
3. ROI 高 + 复杂度高 → **排期修**
4. ROI 低 + 复杂度低 → **有空修**
5. ROI 低 + 复杂度高 → **谨慎评估**

## 问题生命周期（每条 finding 必走完整流程）

### 1. 多次确认（3 次独立审计）
- 至少 3 次独立确认：grep + 读代码 + 调用链反查
- 3 次结论不一致 → **不算 finding**

### 2. 问题上下文分析
- 触发条件 / 影响范围 / 当前缓解 / 长期影响
- 不要写「有 bug」 — 写「在 X 场景输入 Y，触发 Z，导致 W，长期 V」

### 3. 技术方案
- 可执行方向：哪些文件 / 哪些函数 / 改动粒度 / 备选方案 / 风险
- 不要写「应加固」 — 写「在 `xxx.go:NNN` 把 `A` 改为 `B`，更新 5 个 caller」

### 4. 问题验证测试
- 写测试 / 跑静态检查 / 反查脚本，证明问题存在
- Security finding 用 `gosec` / `govulncheck` 跑出警告

### 5. 修复效果测试
- 同 reproduce 触发条件 + 回归断言
- `gosec` / `govulncheck` 修复后 0 warning

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] grep 'fmt.Sprintf' in backend/database/ → 10 处含 SQL
- [2] read backend/database/provider_postgres.go:294 → CREATE DATABASE 拼接
- [3] 反查 caller → app.CreateConnection → 无 escape

## Reproduction Check
\`\`\`bash
gosec ./backend/database/  # 应报 SQL injection warning
govulncheck ./backend/...   # 应报 DB driver 漏洞
\`\`\`

## Fix Verification
\`\`\`bash
gosec ./backend/database/  # 修复后 0 warning
go test ./...  # 回归全绿
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle
- 没有重复已有 F-NNN
- 没有越界（不写 UX / 死代码 / 测试缺口 finding）
- 输出 schema 完整（含 `hat` 字段：arch-reviewer/skeptic/domain-reviewer/user-reviewer）
- **Security finding 不放过**：SQLi / XSS / 命令注入即使轻微也必须 flag
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `REV-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 匹配 | 重评 |
| location 精确 | file:line 真实存在 | 修正 |
| category 正确 | bug/security/perf/arch | 改 category |
| **hat 字段** | **4 帽子之一** | **缺失必返工** |
| 5 步完整 | 每步都有实质内容 | 补全 |
| 6 维归属 | 每条 finding 标注哪个 6 维 | 补 |
| 多角度 | 至少 3 个视角（含安全视角） | 补 |
| Test Plan | 含可执行步骤 | 补 |
| 矩阵同步 | 6/6 | 补 |
| 不越界 | 不含 UX / 死代码 finding | 删 / 移 |
| Security 必检 | SQLi/XSS/Command injection 全部 flag | 补漏 |

### 自审输出

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 含 hat 字段：N/N
- 6 维覆盖：correctness X / test_coverage X / code_quality X / security X / performance X / maintainability X
- Security 漏检：0
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试 / 检查脚本写出来即可**，**不必实际跑**
- **修复方案只写文档**，**不实际改代码 / 不实际加 lint 配置 / 不实际跑 `gosec` 真实扫描**
- **每条 finding 落盘后由人工 review 再决定是否修复**
- **Security finding 特别**：漏洞利用 payload 不真触发 / 不发送恶意请求