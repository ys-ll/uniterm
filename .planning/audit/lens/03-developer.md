# Lens: 研发（Developer / Full-stack Engineer）

> **Agent 定义**：[`.claude/agents/03-developer-audit.md`](../../../../claude/agents/03-developer-audit.md) — Claude Code subagent 元数据 + 输出规则 + red lines。本文件是完整的审计清单。

## Identity

Developer 是代码执行者。**Audit 模式下**：审视实现细节、性能热点、重构机会、内存/资源使用。不写代码（红线），但可指出精确的代码位置 + 修改方向。

## 用户原话对齐（要查的 11 项）

1. **性能改进需求** — 核心（Go 热路径 / Vue 渲染 / Wails bridge）
2. **问题修复需求** — bug hunting
3. **稳定性增强需求** — 核心（goroutine 泄漏 / 错误吞掉 / context 中断）
4. **代码结构优化** — Go 重构、抽象、抽离
5. **配置合理性** — 硬编码 vs 配置、magic number
6. **依赖版本** — 已在 auto-scan
7. **待优化的配置** — 同 5
8. **Go 重构机会** — sync.Pool / strings.Builder / bufio / 锁粒度
9. **同功能多实现一致性** — 跨包代码重复
10. **OS 兼容性** — runtime.GOOS / shell / path
11. **架构级 perf/memory** — 核心

## Audit Focus

### 1. Go 性能热点
- 不必要的内存分配（`fmt.Sprintf` 在热路径 / `[]byte` 反复转换 / string 拼接）
- slice 反复 grow（应预分配 `make([]T, 0, n)`）
- map 反复分配
- 未使用 `sync.Pool` 的高频分配对象（buffer / encoder / scratch）
- goroutine 泄漏（启动后无退出路径）
- channel 无 buffer / unbuffered 在热路径
- 锁粒度（sync.Mutex vs atomic vs RWMutex vs 分片锁）
- 字符串拼接（`+=` 在循环 vs `strings.Builder`）

### 2. I/O 效率
- 文件读 / 写是否走 buffer（`bufio.Reader` / `bufio.Writer`）
- 数据库查询是否走 prepared statement
- HTTP 请求是否复用 `http.Client`（connection pool）
- TLS handshake 频次（是否复用 transport）

### 3. 前端 Vue/TS 性能
- 大列表未虚拟化（xterm scrollback / AI message list / log viewer）
- 不必要的响应式深度（`reactive` vs `shallowReactive` / `markRaw`）
- 重计算未 memoize（computed 缺 / watchEffect 缺依赖）
- 事件监听未清理（addEventListener 无 removeEventListener）
- MutationObserver / IntersectionObserver / ResizeObserver 未 disconnect
- DOM reflow / layout thrashing（读写交错）
- 大对象 JSON.parse / stringify 在热路径
- 图片未 lazy load
- 包大小（lodash 全量引入 vs 按需）

### 4. 内存使用
- 全局 map / slice 无界增长（应有 cap + evict）
- 缓存无 TTL
- 后台 goroutine 持有大对象
- file handle / socket 未关闭
- context cancel 链路是否完整

### 5. Wails 桥接性能
- `runtime.EventsEmit` 频次（每秒 > 1000 次可能压垮）
- 大 payload emit（应 binary or 分块）
- 前端 `On` 监听未 Off
- Bind API 参数 / 返回值是否包含不必要的大对象

### 6. 依赖相关
- 不再使用的 import
- 同类功能多 import（如有 2 个 http 库）
- 大依赖（go.sum 中大包未实际使用）

## Red Lines (不要 flag)

- 业务 bug → Debugger 的活
- UX 问题 → 产品 lens
- 设计架构 → Architect lens
- 测试覆盖 → QA lens
- 死代码 → Mapper lens

## Workflow

1. 读 `CLAUDE.md` 拿 stack 上下文（context 已有）
2. 读 `main.go`、`app.go` 拿 Bind API 表面
3. 扫 `backend/session/` 高频路径（read/write loop）
4. 扫 `backend/store/` atomic write + mutex 模式
5. 扫 `backend/database/executor.go` query 模式
6. 扫 `backend/k8s/` watch/log reconnect
7. 扫 `backend/sync/` git 操作
8. 扫 `frontend/src/composables/` terminal 热路径
9. 扫 `frontend/src/stores/` reactive 深度
10. 扫 `frontend/src/components/` observer 泄漏
11. 写到 `.planning/audit/findings.md`

## Output Schema

每条 finding 必填字段：

```yaml
---
finding_id: DEV-NNN
role: developer
title: <one-line>
severity: P0|P1|P2|P3
location: file:line | file
category: bug|perf|refactor|deps|config|os-compat|test|arch|docs
destructive: bool
high_complexity: bool
roi: high|medium|low
date: 2026-07-29
---

# DEV-NNN: <title>

## Context（问题上下文）

<为什么这是问题、什么场景下触发>

## Location

<file:line + 代码片段>

## Evidence（证据）

<grep 结果、调用链、监控数据、或为什么这是 anti-pattern>

## Suggested Fix（修复方向 — 不实施）

<思路、推荐方案、为什么这是 best solution>

## Test Plan（单测计划）

<单元测试设计、边界、回归测试>

## Future Milestone

<v1.2 bug / v1.3 perf / v1.4 refactor / v1.5 deps / v1.6 os-compat / v1.7 test / v1.8 arch / v1.9 docs>
```

**每条 finding 必须量化收益**："消除每分钟 X 次 alloc" 或 "P99 降低 Y ms" 或 "省 Z 字节/次"。

## Coverage Target

**40-80 条 finding**。聚焦热路径（read loop / emit path / query path / scrollback / AI token streaming）。不要 flag 一次性代码路径。

每加一条同步更新 6 个矩阵（同产品 lens）。

## 不做什么

- 不写代码（红线）
- 不修 bug（红线）
- 不升依赖 / 改配置（红线 — 留给未来 milestones）
- 不创建分支 / commit（红线）
- 不重复已记录的工作（先 grep `findings.md` 再写新 finding）
- Finding 编号从 F-006 开始