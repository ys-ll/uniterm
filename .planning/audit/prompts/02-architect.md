# 任务提示词 02 — Architect Lens Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Architect 视角的审计（模块边界 / 多实现一致性 / OS 抽象 / 依赖方向 / 技术债），产出 finding 清单。**最大 lens。**

## Role Context

完整角色定义见 `.claude/agents/02-architect-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas

### 模块边界
- **定义**：同类功能不应被复制到多个实现；跨 package 引用应符合层级
- **检查方式**：
  - 13 种 session 实现（SSH/Telnet/Mosh/Local/Serial/FTP/SMB/WebDAV/S3/RDP/VNC/SPICE/MongoDB/Redis）是否共享同一组 helper
  - 多个 DB provider（postgres / mysql / sqlserver / sqlite）是否走同一 engine
  - 多个 store（connection/ai/skill/setting/...）是否走同一 persistence 层
- **证据**：重复代码片段 + 应抽取的 helper 路径
- **不做什么**：不审 package 内命名 / 不审 API 命名风格

### 多实现一致性
- **定义**：同类操作（连接 / 重连 / 心跳 / 断开 / 验证 / 序列化）在不同实现里走相同路径
- **检查方式**：
  - 事件 emit 数据格式跨模块统一（grep `EventsEmit(` 看 payload schema）
  - 错误返回风格统一（`error` vs `panic` vs `(T, error)`）
  - 配置加载在所有 store 走相同路径（JSON unmarshal + validate）
  - 重连逻辑在 k8s / SSH / DB 是否走同一抽象
- **证据**：跨实现差异点 + 应统一方向
- **不做什么**：不审实现性能差异 / 不审协议特定行为

### OS 兼容性抽象
- **定义**：平台相关代码走 build tag 拆分，而非运行时分支
- **检查方式**：
  - 文件命名带 `_darwin.go` / `_unix.go` / `_windows.go` 后缀
  - 平台分支常量硬编码（grep `runtime.GOOS == "darwin"` vs `//go:build darwin`）
  - 路径分隔符硬编码（grep `"/"` vs `filepath.Join`）
  - shell 命令硬编码（grep `"/bin/sh"` / `"cmd.exe"` vs shell abstraction）
  - 文件权限 / 行结束符 / encoding 差异
- **证据**：应改用 build tag 的位置 + 缺失的文件
- **不做什么**：不审各 OS 实现差异细节 / 不审特定 OS API 错误处理

### 接口签名 & 类型系统
- **定义**：公共 API 稳定，同类函数签名一致，错误类型明确
- **检查方式**：
  - public function 参数顺序一致（先 ctx 再 data vs 先 data 再 ctx）
  - 错误类型用 `errors.New` vs 自定义 error 类型 vs `fmt.Errorf` 一致性
  - 取消 / 超时信号 `context.Context` 是否传到所有阻塞调用
  - 公开 API 是否稳定（看 git log / changelog）
- **证据**：签名不一致位置 + 类型定义缺失
- **不做什么**：不审 Go 风格细节 / 不审 struct 字段顺序

### 依赖方向（关键 — 此项最常被误审）
- **定义**：包间 import 关系应单向、无环、底层不依赖上层
- **正确理解**：
  - **单向**：高层依赖底层，反之不行（如 `session/` 不能 import `main.go`；`store/` 不应 import `session/`）
  - **无环**：A→B→C→A 是循环依赖，必须打破
  - **层级清晰**：`main` → `app` → `backend/*` → `vendor`
- **检查方式**：
  - `go list -f '{{ join .Deps "\n" }}' ./backend/...` → 看 import graph
  - 找循环依赖：`go list -f '{{ join .Imports "\n" }}' ./backend/... | sort | uniq -c | awk '$1 > 1'`
  - `main` 包不应被 import（除 test）：`grep -r "import" backend/ | grep '"main"'`
  - `internal/` 包只在父 tree 内被引用：超出范围即违规
  - 底层 utility 不应依赖上层 UI / session：grep 反向 import
- **证据**：违反规则的 import 路径 + 触发场景
- **不做什么**：不审运行时依赖注入 / DI 容器设计 / 微服务拆分

### 技术债（关键 — 此项最常被泛泛而谈）
- **定义**：代码中标记"知道但暂不修"的痕迹 + 长期遗留的设计债
- **检查清单**（每项 grep 验证）：
  | 类型 | grep pattern | 含义 |
  |---|---|---|
  | TODO 注释 | `grep -rn 'TODO' backend/ frontend/src/` | 待办，未来应清 |
  | FIXME 注释 | `grep -rn 'FIXME' backend/ frontend/src/` | 待修 |
  | HACK 注释 | `grep -rn 'HACK' backend/ frontend/src/` | 临时方案 |
  | XXX 注释 | `grep -rn 'XXX' backend/ frontend/src/` | 危险 / 需重审 |
  | Deprecated 标记 | `grep -rn '// Deprecated:' backend/` | 公开 API 弃用 |
  | Production panic | `grep -rn 'panic(' backend/ \| grep -v _test.go` | library 不应 panic |
  | log.Fatal in library | `grep -rn 'log.Fatal\|os.Exit' backend/ \| grep -v main.go` | library 不应终止进程 |
  | time.Sleep in prod | `grep -rn 'time.Sleep' backend/ \| grep -v _test.go` | 生产路径不应 sleep |
  | 吞错 | `grep -rn '_ = ' backend/` | `_ = someFunc()` 隐藏错误 |
  | Magic number | 扫描裸数字（`3600` / `86400` / `1024` 等无命名常量）| 应提 const |
  | 绕过 lint | `grep -rn '// nolint:' backend/` | 抑制 lint |
  | 空函数 / 空分支 | `func XXX() {}` / `if x { }` | 无实现 |
  | 仅 panic stub | `func XXX() { panic("not implemented") }` | 占位 stub |
  | 临时调试输出 | `grep -rn 'fmt.Println\|console.log' backend/ frontend/src/ \| grep -v main\|debug` | 调试残留 |
- **证据**：grep 输出 + 文件分布
- **不做什么**：不审注释风格 / 不审 license header / 不审代码格式化（gofmt 应已处理）

### 依赖版本 lockfile
- **定义**：go.sum / package-lock.json / go.mod 完整性
- **检查方式**：
  - `go.sum` 缺失 / 不一致
  - `package-lock.json` 与 `package.json` 不一致
  - 已知 CVE（grep `go.mod` 比对 NVD）
  - 主版本升级（v1 → v2）需手动 review
- **证据**：缺哪个 / 不一致 / CVE 列表
- **不做什么**：不审具体升级路径（留给 v1.5 milestone）

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游 AI 才能直接拿这个 finding 去修复**：

```yaml
finding_id: ARCH-NNN                  # 新 finding 用 lens 前缀
severity: P0|P1|P2|P3
category: arch|refactor|os-compat|test
roi: high|medium|low
location:                             # ⚠️ 多层定位，不能只给 file:line
  file: backend/store/connection_store.go
  function: Save
  line_start: 87
  line_end: 95
  symbol: ioutil.WriteFile(path, data, 0644)
  locations_multi:                   # 同类 hack 多文件 → 列在这里（不拆 finding）
    - file: backend/store/ai_session_store.go
      function: Save
      line_start: 145
      line_end: 155
    - file: backend/store/setting_store.go
      function: Save
      line_start: 95
      line_end: 110
code_block: |                         # ⚠️ 必填：当前实际代码（5-20 行）
  func (s *ConnectionStore) Save(conn *Connection) error {
      data, err := json.MarshalIndent(conn, "", "  ")
      if err != nil { return err }
      // ⚠️ 直接写，无 atomic write
      return ioutil.WriteFile(s.path, data, 0644)
  }
caller_chain:                         # ⚠️ 必填：哪些路径依赖此处
  - app.go:SaveConnection → ConnectionStore.Save
  - app.go:ImportConnections → ConnectionStore.Save
  - sync.ExportConnections → ConnectionStore.Save
root_cause: |
  Save 方法直接 ioutil.WriteFile 覆盖原文件，无 write-temp + rename 模式
  中途崩溃（kill -9 / 断电）→ 文件截断 / 数据损坏
  多个 store 重复同一反模式，未抽取公共 helper
fix_diff_hint: |                      # ⚠️ 必填：具体改什么
  方案 A（推荐）：在 backend/store/atomic.go 新增 helper：
    func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
        dir := filepath.Dir(path)
        tmp, err := os.CreateTemp(dir, ".tmp-*")
        ...
        return os.Rename(tmp.Name(), path)
    }
  方案 B（轻）：改 Save 用 ioutil.WriteFile 前先写 .tmp 再 rename
  风险：rename 跨 OS 行为差异（Windows 需先 Remove 再 Rename）
test_target:                          # ⚠️ 必填
  file: backend/store/atomic_test.go
  function: TestAtomicWriteFile
  fixture: kill -9 中途崩溃模拟
  assertion: 文件不损坏 / 原文件保留
verification:                         # ⚠️ 必填
  command: go test ./backend/store/ -run TestAtomicWriteFile -v
  expected: PASS
  regression: go test ./...
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  触发场景：用户在 app 内保存连接，进程突然崩溃
  影响范围：所有 12 个 store
  当前缓解：用户手动从 backup 恢复
  长期影响：数据丢失 / 用户流失
evidence: |
  grep -n 'ioutil.WriteFile' backend/store/ → 12 处无 atomic
  read backend/store/connection_store.go:87 → 确认直接写
  反查 caller → app.SaveConnection
```

**特别**：同类 hack 出现在 2+ 文件 → **1 条 finding 包含多个 location**，列在 `locations_multi`，不要拆成多条。

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：人时（小 / 中 / 大）+ 风险（低 / 中 / 高：跨文件 / 破坏 API / 数据迁移）
- **修复收益**：用户价值（高频 / 中频 / 低频）+ 频次（每天 / 每周 / 偶尔）+ 不可替代性（独家卖点 / 通用能力）
- **修复紧迫性**：是否阻塞用户主流程 / 是否反复修 / 是否被客诉

### 问题复杂度（修复难度分层）
- **技术复杂度**：跨文件数 / 跨包数 / 是否需要 schema 迁移 / 是否需要新依赖
- **业务复杂度**：影响多少用户群 / 影响多少核心 user journey
- **测试复杂度**：需要多少 mock / fixture / 跨 OS / 跨平台

### 多角度判断（不同视角的修复必要性）
- **用户视角**：用户能否感知 / 是否影响首次体验 / 是否影响留存
- **开发者视角**：是否阻碍未来开发 / 是否影响 onboarding / 是否反复踩坑
- **业务视角**：是否影响市场推广 / 是否影响付费转化 / 是否影响合规
- **安全视角**：是否构成攻击面 / 是否泄露用户数据
- **架构视角**：是否违反单一职责 / 是否造成循环依赖 / 是否增加未来维护成本

### 优先级判断流程（填到 finding 的 Context / Suggested Fix）
1. ROI 高 + 复杂度低 → **必修**（直接进 P1 修复）
2. ROI 高 + 复杂度高 → **排期修**（进对应 milestone）
3. ROI 低 + 复杂度低 → **有空修**（放 backlog）
4. ROI 低 + 复杂度高 → **谨慎评估**（可能不做）

每条 finding 在 Context 中说明属于上述哪个象限 + 原因。

## 问题生命周期（每条 finding 必走完整流程）

**不要只写「找到问题」就完事**。每条 finding 必须走完 5 步：

### 1. 多次确认（多次独立审计）
- **目的**：避免误报 / 假阳性 / 误解
- **做法**：
  - 至少 **3 次独立确认**：grep 一次 + 读代码一次 + 调用链反查一次
  - 每次确认要写明「用什么命令 / 读了哪些行 / 反查到什么 caller」
  - 3 次结论不一致 → **不算 finding**，改写或撤回
- **证据格式**：`Confirmation: [1] grep pattern → matches N, [2] read lines X-Y → confirms Z, [3] reverse caller check → A→B→C`

### 2. 问题上下文分析（必填 Context 段）
- **不是「哪里出错」而是「为什么是问题」**：
  - 触发条件（什么场景下用户会撞到）
  - 影响范围（多少用户 / 多少流程 / 多少设备）
  - 当前缓解措施（用户如何 workaround / 是否已部分修复）
  - 长期影响（不修会怎样：技术债积累 / 用户流失 / 合规风险）
- **不要写**：「这里有循环依赖」「这里有重复代码」 — **要写**：「backend/store 与 backend/database 当前互相 import，导致 X 场景下编译时间增加 Y ms，重构时无法单独提取 W 模块」

### 3. 技术方案（必填 Suggested Fix 段）
- **要给出可执行方向，不要只说「应改进」**：
  - 修改哪些文件 / 哪些函数 / 哪些 import
  - 改动的粒度（单行 / 单函数 / 单文件 / 跨文件 / 跨包）
  - 备选方案（如果主方案不行，可以走哪条路）
  - 依赖与风险（是否需要新库 / 是否破坏 API / 是否要 migration）
- **不要写**：「应抽象」「应统一」「应解耦」 — **要写**：「在 `backend/store/` 新建 `internal/db.go`，将 `xxx` 函数从 `backend/database/` 迁入，删除 `backend/store/store.go` 中的对应 import，更新 5 个 caller，备选方案：保留原路径但加 `// sync.RWMutex` 兜底」

### 4. 问题验证测试（Reproduce Test — Fix 之前）
- **目的**：写一个测试 / 静态检查 / grep，证明问题 **存在**
- **写法**（Architect lens 通常用工具而非单元测试）：
  - 静态检查：`go vet` / `staticcheck` / `golangci-lint` 跑出 warning
  - 集成测试：mock 触发场景，断言违反架构规则
  - 反查脚本：`codegraph callers` 找出循环 import
- **示例**：Failing check `go list -f '{{ join .Imports "\n" }}' ./backend/store/...` 应输出 0 个 `database` import，当前输出 3 个
- **不要跳过这步**：没有 reproduce 检查的 finding = 没有证据链完整

### 5. 修复效果测试（Fix Verification — Fix 之后）
- **目的**：写一个测试 / 静态检查 / 跑构建，证明 fix **生效**
- **写法**：
  - 同 reproduce 触发条件
  - 断言：期望结果（架构规则现在满足）
  - 回归断言：所有现有功能编译通过 / 测试通过
- **示例**：Passing check `go test ./...` 全绿 + `golangci-lint` 0 warning + 受影响 caller 编译通过

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] `go list -f '{{ join .Imports "\n" }}' ./backend/store/...` → 3 个 database import
- [2] 读 backend/store/store.go:23 → 确认 `import "backend/database"`
- [3] codegraph callers store → database → store 形成环

## Reproduction Check
\`\`\`bash
go list -f '{{ join .Imports "\n" }}' ./backend/store/...
# 期望输出为空，实际输出 3 行 → FAIL（违反架构）
\`\`\`

## Fix Verification Check
\`\`\`bash
go test ./...
golangci-lint run
# 期望全绿 → PASS（架构合规）
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle（确认 / 上下文 / 方案 / 验证测试 / 效果测试）
- 没有重复已有 F-NNN（去重扫描）
- 没有越界（不写 UX / 死代码 / 性能细节 / 测试缺口 finding）
- 输出 schema 完整（`finding_id` / `severity` / `location` / `category` / `roi` / Context / Location / Evidence / Suggested Fix / Test Plan）
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单（每条 finding 必查）

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `ARCH-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 与实际严重程度匹配 | 重评 |
| location 精确 | file:line 真实存在 | 修正 |
| category 正确 | 在 Architect 允许类别内（arch/refactor/os-compat） | 改 category |
| 5 步完整 | 每步都有实质内容 | 补全 |
| ROI 评估 | ROI 矩阵有具体值 | 具体化 |
| 多角度 | 至少 3 个视角有判断（含架构视角） | 补 |
| Test Plan | 含可执行步骤 | 补 |
| 矩阵同步 | 6 个矩阵的 finding 都已登记 | 补 |
| 不越界 | 不包含 bug / perf / 死代码 / 测试 finding | 删 / 移 |

### 自审输出（追加到任务结尾）

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 输出 schema 完整：N/N
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试 / 检查脚本写出来即可**，**不必实际执行**
- **修复方案只写文档**，**不实际修改代码 / 不实际做 import 重排 / 不实际跑重构**
- **每条 finding 落盘后由人工 review 再决定是否执行**