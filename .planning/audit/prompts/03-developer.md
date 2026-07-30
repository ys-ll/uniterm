# 任务提示词 03 — Developer / Performance Lens Audit

本里程碑：uniterm v1.1 Audit

## 任务

执行 Developer 视角的审计（性能瓶颈 / 内存 / 重构机会 / 稳定性），产出 finding 清单。**每条 finding 必须量化收益**（消除 X allocs/min / P99 -Y ms / 省 Z 字节/次）。

## Role Context

完整角色定义见 `.claude/agents/03-developer-audit.md`（请先读）。

## Project Context

- 工作目录：`/Users/coderstory/CodeSource/uniterm`
- stack：Wails v2 + Vue 3 + Go
- 已有 finding **F-001 ~ F-014** 在 `.planning/audit/findings.md`（**不要重复**）
- 你的 finding 从 **F-015** 开始编号

## Focus Areas

### 性能热点（Backend Go）
- **定义**：热路径（read loop / emit / query / scrollback / streaming）上的不必要分配与重复计算
- **检查方式**（每项 grep / 读 hot path）：
  | 类型 | grep pattern | 关注 |
  |---|---|---|
  | 字符串拼接热路径 | `+=` 在 loop / `fmt.Sprintf` 在 hot path | 提 `strings.Builder` |
  | bytes ↔ string 反复转换 | `[]byte(s)` / `string(b)` | 单次转换 |
  | slice 反复 grow | `append` 无预分配容量 | 提 `make([]T, 0, n)` |
  | map 反复分配 | `make(map[..])` 在 hot path | 全局复用 |
  | 高频对象未对象池 | `bytes.Buffer` / `sync.Pool` 候选 | 复用 |
  | JSON 编解码热路径 | `json.Marshal/Unmarshal` | 提 `jsoniter` 或 `easyjson` |
- **证据**：hot path 函数 + 每次调用的 alloc 次数（用 `go test -bench -benchmem` 或 `pprof`）
- **不做什么**：不审冷路径（启动 / 一次性配置）

### I/O 效率（Backend）
- **定义**：文件 / DB / HTTP 操作走高效路径
- **检查方式**：
  | 操作 | 反模式 | 替代 |
  |---|---|---|
  | 读文件 | `ioutil.ReadFile`（全量读） | 大文件用 `os.Open + bufio.NewReader` |
  | DB 查询 | `db.Query`（每次重建） | `db.Prepare` + 复用 stmt |
  | HTTP client | 每次 `http.Get` 创建 client | `http.Client` 复用 + connection pool |
  | TLS handshake | 频繁 dial | 长连接 / session resumption |
- **证据**：具体函数 + 调用频次估算
- **不做什么**：不审冷启动 I/O / 不审日志 I/O

### 前端渲染（Vue 3）
- **定义**：渲染管线上的不必要开销
- **检查方式**：
  | 类型 | grep / 模式 | 关注 |
  |---|---|---|
  | 大列表未虚拟化 | `<div v-for="item in list">`（list > 1000） | 用 `vue-virtual-scroller` |
  | 不必要响应式深度 | `reactive(嵌套对象)` 全响应 | 浅响应 / `shallowRef` |
  | 重计算未 memoize | computed / method 内重计算 | `computed` 缓存 |
  | 事件监听未清理 | `addEventListener` 无 removeEventListener | onUnmounted 清理 |
  | Observer 未 disconnect | `IntersectionObserver` / `ResizeObserver` 创建后未 disconnect | onUnmounted disconnect |
  | DOM reflow thrashing | 多次读+写 DOM | 批处理 / `requestAnimationFrame` |
  | JSON 热路径 | `JSON.parse` / `JSON.stringify` 大对象在循环 | 提 schema |
- **证据**：组件 + 列表大小 + 渲染次数估算
- **不做什么**：不审视觉性能（CSS 动画流畅度）/ 不审首屏 SSR

### 内存使用
- **定义**：内存随时间增长无界，缓存无 TTL，长跑对象持有大资源
- **检查方式**：
  | 类型 | grep pattern | 关注 |
  |---|---|---|
  | 全局 map 无界增长 | `var xxx map[..]` at package level | 加 TTL / size cap |
  | 缓存无 TTL | `sync.Map` / `map[..]xxx` 无 expire | 加 evict 策略 |
  | 后台 goroutine 持有大对象 | `go func() { ... 闭包 ... }()` | 显式 release |
  | handle / socket 未关闭 | `resp.Body.Close()` 缺失 | defer Close |
  | 取消 / 超时 链路不完整 | `ctx.Done()` 未传到下游 | 透传 |
- **证据**：具体代码路径 + 估算增长速率
- **不做什么**：不审启动期内存 / 不审堆栈 dump 分析

### IPC Bridge（Wails 事件）
- **定义**：Go ↔ JS 事件通道的滥用
- **检查方式**：
  | 类型 | grep pattern | 关注 |
  |---|---|---|
  | EventsEmit 高频 | `EventsEmit(` 在 loop / 每次读字节 | 批处理 / 节流 |
  | 大 payload emit | payload > 100KB | 分块 / 压缩 |
  | EventsOn 未对应 EventsOff | `EventsOn(` 多于 `EventsOff` | composable 统一管理 |
  | Bind API 大对象 | `Bind:` 的方法返回大对象 | 拆 API |
- **证据**：emit 频次 + listener 数差异
- **不做什么**：不审 Wails 内部实现 / 不审 IPC 协议

### 稳定性
- **定义**：长跑 goroutine / 后台 task 不死 / 不泄漏 / 不 panic 传播
- **检查方式**：
  - `go func()` 无 `defer recover()`
  - retry 无 backoff / 无限重试
  - watchdog / 心跳缺失
- **证据**：goroutine 入口 + 风险
- **不做什么**：不审具体 bug 触发场景（Debugger 视角）

## Output

Append 到 `.planning/audit/findings.md`，**finding 必须含完整上下文，下游 AI 才能直接拿这个 finding 去修复**：

```yaml
finding_id: DEV-NNN
severity: P0|P1|P2|P3
category: perf|refactor|bug
roi: high|medium|low
location:                             # ⚠️ 多层定位
  file: backend/session/ssh_session.go
  function: connect
  line_start: 188
  line_end: 195
  symbol: fmt.Sprintf("%s:%d", host, port)
  locations_multi:                   # 7 处同类 → 列在这里
    - file: backend/session/mosh_session.go
      function: connect
      line_start: 46
      line_end: 49
    - file: backend/session/smb_session.go
      function: connect
      line_start: 47
      line_end: 50
    - file: backend/session/ssh_dial.go
      function: Dial
      line_start: 17
      line_end: 22
    - file: backend/session/tunnel_service.go
      function: dial
      line_start: 60
      line_end: 65
    - file: backend/session/telnet_session.go
      function: connect
      line_start: 61
      line_end: 64
    - file: app.go
      function: dialBackend
      line_start: 1680
      line_end: 1683
code_block: |
  func (s *SSHSession) connect(host string, port int) error {
      addr := fmt.Sprintf("%s:%d", host, port)  // ⚠️ IPv6 无方括号
      conn, err := net.Dial("tcp", addr)
      ...
  }
caller_chain:
  - app.Connect → session.SSHSession.connect
  - app.TunnelConnect → session.SSHSession.connect
root_cause: |
  fmt.Sprintf("%s:%d", host, port) 在 IPv6 主机（"::1"）上输出 "::1:8080"
  RFC 2732 要求 IPv6 字面量用方括号包裹（"[::1]:8080"）
  net.Dial 解析 "::1:8080" 失败 → IPv6 用户完全无法连接
fix_diff_hint: |
  方案 A（推荐）：在 backend/session/util.go 新增 helper：
    func dialAddr(host string, port int) string {
        return net.JoinHostPort(host, strconv.Itoa(port))
    }
  7 处统一替换 `fmt.Sprintf("%s:%d", host, port)` → `dialAddr(host, port)`
  风险：0（API 不变，行为对 IPv4 一致）
test_target:
  file: backend/session/util_test.go
  function: TestDialAddr
  cases:
    - { input: ["127.0.0.1", 22], expect: "127.0.0.1:22" }
    - { input: ["::1", 8080], expect: "[::1]:8080" }
    - { input: ["fe80::1", 80], expect: "[fe80::1]:80" }
verification:
  command: go test ./backend/session/ -run TestDialAddr -v
  expected: PASS
  regression: go test ./...
quantified_benefit:                 # ⚠️ Developer 特有，必填
  before: "7 处 IPv6 连接全部失败"
  after: "IPv6 全部正常"
  user_impact: "修复后 IPv6 用户数 +N%（取决于部署环境）"
lifecycle:
  confirmations_count: 3
  has_context: true
  has_fix_plan: true
  has_reproduce_test: true
  has_fix_verification: true
context: |
  触发场景：用户用 IPv6 连接 SSH / 任何 session
  影响范围：所有 IPv6 用户
  当前缓解：无（IPv6 完全不可用）
  长期影响：被客诉 / 流失 IPv6 用户
evidence: |
  grep -n 'fmt.Sprintf("%s:%d"' backend/session/*.go → 7 处
  go vet ./backend/... → 报 IP 格式警告
  read backend/session/ssh_session.go:189 → 确认无 JoinHostPort
```

每条 finding 同步更新 6 个矩阵。

## 评估维度（每条 finding 必填）

### ROI（修复投入产出）
- **修复成本**：人时（小 / 中 / 大）+ 风险（低 / 中 / 高：跨文件 / 破坏 API / 数据迁移）
- **修复收益**：用户价值（高频 / 中频 / 低频）+ 频次（每天 / 每周 / 偶尔）+ 不可替代性（独家卖点 / 通用能力）
- **修复紧迫性**：是否阻塞用户主流程 / 是否反复修 / 是否被客诉
- **量化收益**（Developer lens 特有）：消除 X allocs/min / P99 降低 Y ms / 省 Z 字节/次 — **必填**

### 问题复杂度（修复难度分层）
- **技术复杂度**：跨文件数 / 跨包数 / 是否需要 schema 迁移 / 是否需要新依赖
- **业务复杂度**：影响多少用户群 / 影响多少核心 user journey
- **测试复杂度**：需要多少 mock / fixture / 跨 OS / 跨平台

### 多角度判断（不同视角的修复必要性）
- **用户视角**：用户能否感知（启动快 0.1s 用户不感知 vs 启动快 1s 用户感知）/ 是否影响首次体验
- **开发者视角**：是否阻碍未来开发 / 是否影响 onboarding / 是否反复踩坑
- **业务视角**：是否影响市场推广（启动时间影响 SEO / 转化率）/ 是否影响付费转化
- **安全视角**：性能问题是否伴随资源泄漏 / 是否构成 DoS 攻击面
- **架构视角**：是否违反单一职责 / 是否造成循环依赖 / 是否增加未来维护成本

### 优先级判断流程（填到 finding 的 Context / Suggested Fix）
1. ROI 高 + 复杂度低 → **必修**（直接进 P1 修复）
2. ROI 高 + 复杂度高 → **排期修**（进对应 milestone）
3. ROI 低 + 复杂度低 → **有空修**（放 backlog）
4. ROI 低 + 复杂度高 → **谨慎评估**（可能不做）

每条 finding 在 Context 中说明属于上述哪个象限 + 原因 + 量化数字。

## 问题生命周期（每条 finding 必走完整流程）

**不要只写「找到 perf 瓶颈」就完事**。每条 finding 必须走完 5 步：

### 1. 多次确认（多次独立审计）
- **目的**：避免误报 / 量化不准 / 假阳性
- **做法**：
  - 至少 **3 次独立确认**：grep pattern 一次 + 读 hot path 代码一次 + 跑 benchmark 一次
  - 每次确认要写明「用什么命令 / 跑了哪个 benchmark / alloc 数 / ns/op」
  - 3 次结论不一致 → **不算 finding**，改写或撤回
- **证据格式**：`Confirmation: [1] grep 'fmt.Sprintf' in hot loop → matches 5, [2] read func Foo lines 100-130 → hot loop confirmed, [3] bench -benchmem → 12 allocs/op 5MB/op`

### 2. 问题上下文分析（必填 Context 段）
- **不是「哪里慢」而是「为什么这个 perf 问题值得修」**：
  - 触发条件（什么操作触发，频次多高 — 每次按键 / 每秒 N 次 / 每次连接）
  - 影响范围（多少用户能感知 — P99 延迟 / 启动时间 / 内存增长）
  - 当前缓解措施（用户能否通过配置绕过 / 是否有限流）
  - 长期影响（不修会怎样 — 内存增长 → OOM / P99 恶化 → 客诉）
- **量化收益必填**：消除 X allocs/min / P99 -Y ms / 省 Z 字节/次
- **不要写**：「这里有 perf 问题」「这里分配多」 — **要写**：「终端 read loop 每读 1KB 触发 1 次 `fmt.Sprintf`，每分钟 6000 次（终端用户高频），共 6000 allocs/min + 600KB/min 分配，改用 `strings.Builder` 可降至 60 allocs/min」

### 3. 技术方案（必填 Suggested Fix 段）
- **要给出可执行方向 + 量化目标**：
  - 修改哪些函数 / 改用什么 API
  - 预期改善（消除 N allocs/op / 减 P99 Y ms）
  - 备选方案（更激进 vs 更保守）
  - 风险（是否破坏 API / 是否需要 benchmark 对比）
- **不要写**：「优化」「减少分配」 — **要写**：「将 `fmt.Sprintf("host:%d", port)` 改为 `net.JoinHostPort(host, strconv.Itoa(port))`，预计从 8 allocs/op 降至 1 allocs/op（-87%），备选方案：引入 `xid` 包替代自实现 ID 生成」

### 4. 问题验证测试（Reproduce Test / Benchmark — Fix 之前）
- **目的**：写一个 benchmark，证明当前性能 **不达标**
- **写法**：
  - 用 `go test -bench -benchmem` 跑当前实现
  - 记录 allocs/op / ns/op / B/op
  - 断言目标值（如 allocs/op ≤ 1 — 当前 8 → FAIL）
- **示例**：Failing bench `BenchmarkSprintf`：当前 `-benchmem` 输出 `1000000 1200 ns/op 256 B/op 8 allocs/op`，目标 `allocs/op ≤ 1` → FAIL
- **不要跳过这步**：没有 baseline benchmark 的 finding = 没法验证 fix 真有效

### 5. 修复效果测试（Fix Verification — Fix 之后）
- **目的**：写一个 benchmark + 回归测试，证明 fix **生效且不破坏现有功能**
- **写法**：
  - 同 reproduce benchmark 触发条件
  - 断言：allocs/op / ns/op 达到目标
  - 回归断言：现有功能测试 + benchmark `BenchmarkRegression` 跑全量
  - pprof 对比：修复前 vs 修复后的火焰图
- **示例**：Passing bench `BenchmarkJoinHostPort`：修复后 `-benchmem` 输出 `5000000 240 ns/op 32 B/op 1 allocs/op`，达到目标 `allocs/op ≤ 1` → PASS；`go test ./...` 全绿

### 输出格式（追加到 finding 末尾）

```markdown
## Confirmations
- [1] grep 'fmt.Sprintf' in backend/session/*.go → 7 处
- [2] 读 backend/session/ssh_session.go:189 → read loop 内确认
- [3] bench -benchmem BenchmarkSprintf → 8 allocs/op 256 B/op

## Reproduction Benchmark
\`\`\`bash
go test -bench=BenchmarkSprintf -benchmem ./backend/session/
# 输出: 8 allocs/op 256 B/op → 目标 ≤ 1 → FAIL
\`\`\`

## Fix Verification
\`\`\`bash
go test -bench=BenchmarkJoinHostPort -benchmem ./backend/session/
# 输出: 1 allocs/op 32 B/op → PASS
go test ./...  # 回归全绿
\`\`\`
```

**没有 5 步完整 lifecycle 的 finding 不算合格。**

## 子任务产物自审（写完所有 finding 后必跑一遍）

**子任务完成后，必须重新审计自己产出的 finding**，确保：
- 所有 finding 都通过 5 步 lifecycle（确认 / 上下文 / 方案 / 验证测试 / 效果测试）
- 没有重复已有 F-NNN（去重扫描）
- 没有越界（不写 UX / 架构 / 死代码 / 测试缺口 finding）
- 输出 schema 完整
- **每条 finding 量化收益必填**（消除 X allocs/min / P99 -Y ms / 省 Z 字节/次）
- 6 个矩阵已同步更新
- 自审发现的问题（如有）→ 修后再交付

### 自审清单

| 检查项 | 标准 | 失败处理 |
|---|---|---|
| finding_id 唯一 | `DEV-NNN` 编号未与现有冲突 | 改编号 |
| severity 合理 | P0/P1/P2/P3 与实际严重程度匹配 | 重评 |
| location 精确 | file:line 真实存在 | 修正 |
| category 正确 | perf/refactor/bug | 改 category |
| 5 步完整 | 每步都有实质内容 | 补全 |
| **Quantified Benefit** | **必填，含具体数字** | **不达标必返工** |
| 多角度 | 至少 3 个视角 | 补 |
| Test Plan | 含 benchmark 步骤 | 补 |
| 矩阵同步 | 6/6 | 补 |
| 不越界 | 不含 UX / 架构 / 死代码 finding | 删 / 移 |

### 自审输出

```markdown
## 自审报告

- finding 总数：N 条
- 通过 5 步 lifecycle：N/N
- 含量化收益：N/N
- 越界 finding：0 条
- 重复 finding：0 条
- 矩阵同步完成：6/6
- 自审结论：✅ 合格 / ⚠️ 待修 / ❌ 重做
```

**自审不通过的 finding 不算交付物。**

## ⚠️ 重要约束（audit 模式铁律）

- **测试 / benchmark 写出来即可**，**不必实际跑**（避免长时间 bench / 避免误触发 perf regression）
- **修复方案只写文档**，**不实际改代码 / 不实际应用 perf patch**
- **每条 finding 落盘后由人工 review + 实际跑 benchmark 验证再决定是否修复**