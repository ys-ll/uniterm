# 任务提示词 06 — Debugger (Bug Hunt) Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Debugger 视角的 bug 调查（找真实**可复现**的 bug，写最小修复 plan — **不修代码**）。每条 finding 必须含复现步骤和修复方向。

## Role Context

完整角色定义见 `.claude/agents/06-debugger-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas（找可复现 bug）

### 空 / nil 输入 crash
- **定义**：函数接收空 / nil 输入时崩溃
- **检查方式**：
  - pointer 解引用前 nil check（`if p != nil { *p }`）
  - map 访问前 make（`make(map[..])`）
  - slice index 前 len check
  - interface{} type assertion 是否 panic-safe（`, ok`）
  - channel send / receive 前是否 close
  - 整数除以 0（不 panic，但 NaN / Inf 污染）
- **证据**：最小复现输入 + panic stack
- **不做什么**：不审 error 类型设计 / 不审 panic 替代方案

### 并发安全
- **定义**：共享状态在并发访问下行为正确
- **检查方式**：
  - `go func()` 访问共享变量是否加锁
  - 锁顺序是否一致（A 拿 lock1→lock2，B 拿 lock2→lock1 → 死锁）
  - `sync.Map` vs `map[..] + mutex` 选择
  - `atomic` vs `mutex` 适用场景
  - channel 是否会被多个 goroutine 关闭
  - `sync.Once` 是否复用 / 重复执行
- **证据**：并发场景 + 预期 race
- **不做什么**：不审 race detector 配置 / 不审 channel 缓冲大小

### 资源泄漏
- **定义**：file handle / socket / DB 连接 / 后台 task 不释放
- **检查方式**：
  - `defer resp.Body.Close()` 是否缺失
  - `defer file.Close()` 是否缺失
  - `defer rows.Close()` 是否缺失
  - 后台 goroutine 退出条件（context cancel / channel close）
  - 长跑 task 持有大对象不释放
  - DB connection pool 是否泄漏
- **证据**：泄漏路径 + 资源类型
- **不做什么**：不审具体资源释放时机 / 不审 pool 大小

### 异常路径缺恢复
- **定义**：长跑后台进程 panic 后传播 / 不退出
- **检查方式**：
  - `go func() { ... }()` 内无 `defer recover()`
  - watcher / reconnect 循环无 panic 兜底
  - io.Copy 桥接无 panic 兜底
- **证据**：goroutine 入口函数 + panic 传播路径
- **不做什么**：不审 panic 信息格式 / 不审 error wrap

### 错误被吞掉
- **定义**：error 被忽略，调用方不知情
- **检查方式**：
  - `if err != nil { /* nothing */ }`
  - `_ = someFunc()`
  - `if err != nil { log.Println(err) }` 仅 log 不返回
- **证据**：吞错位置 + 影响
- **不做什么**：不审 error 类型设计 / 不审错误日志格式

### Off-by-one / 边界错误
- **定义**：循环边界 / slice 索引 / 字符串切片错一位
- **检查方式**：
  - `< vs <=` / `i < len-1 vs i < len`
  - `for i := 0; i < n; i++` 边界
  - `s[:n]` / `s[n:]` 边界
  - range 0 次 vs 1 次
- **证据**：具体循环 + 错位结果
- **不做什么**：不审算法正确性 / 不审循环写法偏好

### 数值边界
- **定义**：整数溢出 / Float NaN / 除零
- **检查方式**：
  - `int64` 接近 `MaxInt64` 时加减
  - `uint` 减法下溢
  - `0 / x` 与 `x / 0`
  - `math.NaN` / `math.Inf` 传播
  - 时间戳溢出（year 2038 / 2106）
- **证据**：触发输入 + 错误结果
- **不做什么**：不审数学正确性 / 不审算法稳定性

### 死循环 / 死锁风险
- **定义**：retry / 循环无退出条件
- **检查方式**：
  - `for { ... }` 无 break / 无 context cancel
  - retry 无 backoff / 无 max retry
  - 无限等待 channel（无 select timeout）
  - 锁顺序错导致死锁
- **证据**：循环入口 + 永不退出场景
- **不做什么**：不审 backoff 算法 / 不审具体超时值

### 后台协程 + 同步机制泄漏
- **定义**：goroutine 启动后无退出 / WaitGroup 不 Wait
- **检查方式**：
  - `go func()` 启动后无 cancel signal
  - `sync.WaitGroup` Add 不 Wait
  - channel close 后仍有 sender
- **证据**：goroutine 入口 + 退出条件缺失
- **不做什么**：不审 goroutine 数量限制 / 不审调度策略

### 取消 / 超时 传播缺失
- **定义**：`context.Context` 未传到阻塞调用
- **检查方式**：
  - `http.NewRequest` 不用 `ctx`
  - `db.QueryContext` 不用 `ctx`
  - `time.Sleep` 不用 `time.NewTimer(ctx)`
  - goroutine 内不 select ctx.Done()
- **证据**：阻塞调用 + ctx 缺失
- **不做什么**：不审 ctx 派生策略 / 不审 timeout 数值

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游 AI 才能直接拿这个 finding 去修复**：

```yaml
finding_id: DBG-NNN
severity: P0|P1|P2|P3
category: bug
roi: high|medium|low
location:                             # ⚠️ 多层定位
  file: backend/store/ai_session_store.go
  function: LoadShard
  line_start: 120
  line_end: 135
  symbol: _ = err
  locations_multi:                   # 多处同类吞错
    - file: backend/store/connection_store.go
      function: LoadShard
      line_start: 88
      line_end: 95
    - file: backend/store/setting_store.go
      function: Load
      line_start: 40
      line_end: 50
code_block: |
  func (s *AISessionStore) LoadShard(shardID int) (*Session, error) {
      data, err := os.ReadFile(s.shardPath(shardID))
      if err != nil {
          _ = err  // ⚠️ 注释：fall through to scan
          return nil, nil  // ⚠️ 吞掉错误，返回 nil
      }
      ...
  }
caller_chain:
  - app.LoadSession → AISessionStore.LoadShard
  - app.ListSessions → AISessionStore.LoadShard
reproduction_steps:                  # ⚠️ Debugger 特有：可复现步骤
  1. 启动 app，导入一个 AI session
  2. 手动 `chmod 000` 关闭 shard 文件读权限
  3. 调用 LoadSession(id)
  4. 观察：返回 nil, nil（无错误）
  5. 期望：返回 error 或 fallback 到 scan
minimal_input: |
  - shard 文件存在但无读权限
  - 或 shard 文件 JSON 损坏
  - 或 shard 文件被截断
root_cause: |
  LoadShard 在 os.ReadFile 失败时直接 _ = err + 返回 nil
  上层调用方无法区分 "shard 不存在" vs "shard 损坏"
  静默 fallback 让数据丢失对用户不可见
fix_diff_hint: |                      # ⚠️ 最小改动（≤ 5 行）
  方案 A（推荐，改 2 行）：
    if err != nil {
        log.Warnf("shard %d load failed: %v, fall through to scan", shardID, err)
        return nil, nil  // 保留 fallback 行为
    }
  方案 B（更严格，改 4 行）：
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil  // 不存在是预期
        }
        return nil, fmt.Errorf("shard %d corrupted: %w", shardID, err)
    }
  风险：方案 B 可能让 caller 行为变化（之前默认 nil）
test_target:
  file: backend/store/ai_session_store_test.go
  function: TestLoadShard_PermissionDenied
  fixture: shard 文件 chmod 000
  assertion: 方案 B 应返回 error，方案 A 应返回 nil + log warning
verification:
  command: go test ./backend/store/ -run TestLoadShard -v
  expected: PASS
  regression: go test ./...
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  触发场景：shard 文件损坏 / 权限错 / 路径不存在
  影响范围：所有 store 加载逻辑
  当前缓解：注释说明 fallback
  长期影响：数据丢失静默 / 用户无法察觉
evidence: |
  grep -n '_ = err' backend/store/ → 3 处
  read ai_session_store.go:120-135 → 确认吞错
  手动 chmod 000 验证 → 复现成功
```

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：最小改动（通常 ≤ 5 行）+ 低风险
- **修复收益**：高频 crash / 不可逆副作用 / 数据丢失
- **修复紧迫性**：是否阻塞主流程 / 是否反复修 / 是否被客诉

### 问题复杂度（修复难度分层）
- **技术复杂度**：根因是否清楚 / 修复是否需要重构
- **业务复杂度**：触发场景是否高频 / 影响用户数
- **测试复杂度**：reproduce 是否容易 / mock 需要多少

### 多角度判断
- **用户视角**：崩溃频率 / 数据丢失风险 / 用户能否感知
- **开发者视角**：是否能 debug / 是否被反复警告
- **业务视角**：是否影响留存 / 是否被客诉
- **安全视角**：crash 路径是否暴露攻击面
- **维护视角**：是否能加 watchdog / 是否有现成兜底

### 优先级判断流程
1. P0（数据丢失 / 全局崩溃 / 安全 CVE）→ **必修**
2. P1（高频 crash / 不可逆副作用）→ **必修**
3. ROI 高 + 复杂度低 → **必修**
4. 其他 → **排期修**

## 问题生命周期（每条 finding 必走完整流程）

### 1. 多次确认（3 次独立审计）
- 至少 3 次独立确认：grep + 读代码 + 构造触发输入
- 不可复现的 bug → **不算 finding**，撤回或改写

### 2. 问题上下文分析
- 触发条件（具体输入 / 状态 / 时序）
- 影响范围（多少用户能撞到 / 触发概率）
- 当前缓解措施（panic recover / 用户 workaround）
- 长期影响（不修会怎样）

### 3. 技术方案（最小改动）
- 不修改架构，只修最小路径
- 1-5 行代码改动优先
- 备选方案（治标 vs 治本）
- 风险评估

### 4. 问题验证测试（Reproduce Test — Fix 之前）
- 写一个 test，**当前应当 FAIL**
- 触发条件：最小输入 + mock
- 断言：期望结果（应当不 panic / 应当返回 error）

### 5. 修复效果测试（Fix Verification — Fix 之后）
- 同 reproduce 触发条件
- 断言：现在应当 PASS
- 回归断言：现有合法输入仍正常

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] grep '_ = ' backend/ → 5 处吞错
- [2] read backend/store/ai_session_store.go:127 → 确认吞错
- [3] 构造 fixture: shard 文件损坏 → 触发吞错路径

## Reproduction Test
\`\`\`go
func Test_Reproduce_DBGN_NilDeref(t *testing.T) {
    // 触发 nil 解引用
    // 断言: 应当不 panic（当前 panic）→ FAIL
}
\`\`\`

## Fix Verification
\`\`\`go
func Test_Fix_DBGN_NilDeref(t *testing.T) {
    // 同触发条件
    // 断言: 修复后不 panic → PASS
    // 回归: 正常输入仍正常
}
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle
- 没有重复已有 F-NNN
- 没有越界（不写 UX / perf 优化 / 架构 finding）
- 输出 schema 完整
- **每个 finding 必须可复现**：reproduce test 当前应 FAIL
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `DBG-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 匹配 | 重评 |
| location 精确 | file:line 真实存在 | 修正 |
| category 正确 | bug | 改 category |
| 5 步完整 | 每步都有实质内容 | 补全 |
| **可复现** | **reproduce test 当前应 FAIL** | **不可复现必撤回** |
| Reproduction Steps | 含最小触发输入 | 补 |
| Root Cause | 含根因分析（非表面现象） | 补 |
| Fix Plan | 最小改动（≤ 5 行优先） | 补 |
| 多角度 | 至少 3 个视角 | 补 |
| 矩阵同步 | 6/6 | 补 |
| 不越界 | 不含 UX / perf 优化 / 架构 finding | 删 / 移 |

### 自审输出

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 可复现 finding：N/N
- 含最小改动 fix plan：N/N
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试代码写出来即可**，**不必实际执行 / 不必触发 panic**
- **修复方案只写文档**，**不实际修改代码 / 不实际 apply patch**
- **每条 finding 落盘后由人工 review 再决定是否修复**
- **Debugger lens 特别**：bug 复现脚本不直接跑（避免触发真实 crash）/ 不直接构造 fixture 注入生产路径

- **测试代码写出来即可**，**不必实际执行 / 不必触发 panic**
- **修复方案只写文档**，**不实际修改代码 / 不实际 apply patch**
- **每条 finding 落盘后由人工 review 再决定是否修复**
- **Debugger lens 特别**：bug 复现脚本不直接跑（避免触发真实 crash）/ 不直接构造 fixture 注入生产路径