# uniterm v1.1 Audit · INDEX

**单元声明**：本目录是 v1.1 Audit milestone 的全部产物入口。所有 finding、矩阵、角色定义、任务提示词都在这里。

---

## 🚨 紧急必修（P0 / Security）

| Finding | 摘要 | 必修理由 |
|---|---|---|
| **REV-015** | SSH session 漏洞（P0）| 任何 SSH-family 功能发布前必须修 |

---

## 📊 总体数字

| 维度 | 数字 |
|---|---|
| **总 finding（有效）** | **97** |
| P0 / P1 / P2 / P3 | 1 / 23 / 41 / 16 |
| withdrawn / rejected | 5 / 1 |
| Lens 覆盖 | 8 / 8 |
| Subagent 自审通过率 | 100% |
| Verifier confirmed | 80 / likely 10 / disputed 0 |
| Backend modules 扫描 | 11/11 |
| Frontend modules 扫描 | 7/8 |

---

## 🗂️ 目录结构

```
.planning/audit/
├── INDEX.md                              ← 本文件
├── findings.md                           ← 97 条 finding 全部详情（1892 行）
├── prompts/                              ← 8 个任务提示词（subagent 调度用）
│   ├── 01-pm.md           (产品 · UX/文档/i18n/OSS)
│   ├── 02-architect.md    (架构 · 模块/OS 抽象/技术债)
│   ├── 03-developer.md    (研发 · 性能/内存/IPC/前端)
│   ├── 04-qa.md           (QA · 测试覆盖/边界/回归)
│   ├── 05-reviewer.md     (Reviewer · 6 维审查 + security)
│   ├── 06-debugger.md     (Debugger · bug 复现 + 最小修复)
│   ├── 07-mapper.md       (Mapper · 死代码/RTM)
│   └── 08-planner.md      (Planner · 调度，本里程碑 0 finding)
└── matrix/                               ← 6 个矩阵
    ├── coverage.md         (模块 × 视角 × 状态)
    ├── severity-category.md (Severity × Category)
    ├── risk-impact.md     (Risk × Impact × ROI 决策)
    ├── verification.md    (3 Verifier × Finding verdict)
    ├── milestone-map.md   (Finding → Milestone 路由)
    └── role-lens.md       (Lens × File 覆盖)
```

角色定义在 `.claude/agents/0X-name-audit.md`（8 个 lens 角色的稳定定义）。

---

## 🎯 Finding 路由（按 milestone）

| Milestone | 数量 | 必修紧急度 | 主要内容 |
|---|---|---|---|
| **v1.2.1 Emergency Security** | 12 | 🚨 **最高** | P0 SSH 漏洞 + 11 条 security |
| **v1.2 Bug Fixes** | 17 | 🚨 高 | IPv6/SQL/race/atomic 等核心 bug |
| v1.3 Performance | 17 | 中 | DEV 量化 perf 改进 |
| v1.4 Refactor | 13 | 中 | 死代码/RTM/重构 |
| v1.5 Deps | 4 | 低 | Go/npm 升级 |
| v1.6 OS Compat | 3 | 低 | 多 OS 兼容 |
| v1.7 Test | 16 | 中 | 测试覆盖 + fuzz |
| v1.8 Arch | 11 | 中 | 架构一致性 |
| v1.9 Docs / OSS | 10 | 低 | README/i18n/合规 |

---

## 🛣️ 路线图（修复优先级图）

### 阶段 1：紧急安全（v1.2.1 · ~2 周）

```
Week 1:
├── REV-015  SSH session 漏洞（P0）必修
├── REV-016  AI XSS（DOMPurify）
├── REV-017  k8s TLS verify
├── REV-018  local shell command injection
└── REV-019  SFTP path traversal

Week 2:
├── REV-020 ~ REV-026（剩余 7 条 security/correctness）
└── 全量 regression test
```

### 阶段 2：核心 bug 修复（v1.2 · ~4 周）

```
Week 3-4:
├── F-001  IPv6 dial helper（7 处）
├── F-006  SQL prepared statement（10 处）
├── F-010  k8s map race 加锁
├── F-012  store atomic_write helper
└── F-014  EventsOn composable 统一

Week 5-6:
├── F-008  错误吞掉 → log.Warn
├── F-009  goroutine 加 defer recover
├── ARCH-015 13 session 统一 log helper
├── ARCH-016 Session interface 加 ctx
└── DBG-015 ~ DBG-024（10 条可复现 bug）
```

### 阶段 3：性能（v1.3 · ~3 周）

```
Week 7-8:
├── DEV-029  DB 连接池复用（P99 -50%）
├── DEV-037  resize retry 7 → 1（每次 tab 切省 6 IPC）
├── DEV-019/020  前端响应式深度（5-10x perf）
├── DEV-015/017/018  read loop / save / k8s watch IPC
└── DEV-036  k8s log backpressure

Week 9:
├── DEV-040 mouse tracking scan（12% 单核）
├── DEV-039 useTerminal regex 守门
└── DEV-031 ~ DEV-034 其余 perf
```

### 阶段 4：测试覆盖（v1.7 · ~3 周，与 v1.2/v1.3 部分并行）

```
并行 v1.2：
├── QA-015 k8s race concurrent test
├── QA-017 sync changePassword atomicity
└── QA-023 6 个 FuzzXxx（parser / kubeconfig / ANSI）

并行 v1.3：
└── QA-024 6 个 store Save/Load round-trip

独立：
├── QA-016 update semver 边界
├── QA-018 LLM streaming 重组
├── QA-019 tunnel pipe panic recover
├── QA-020/021 container exec / parseFrontmatter
└── QA-022 CI 加 race / fuzz / mutation
```

### 阶段 5：架构 + 重构（v1.4 + v1.8 · ~4 周）

```
v1.4 Refactor:
├── MAP-016 + MAP-019 删 SetSortOrder / Bind wrapper
├── MAP-017 internal-only Bind cleanup
├── MAP-018 app:visibility emit / listen 修复
├── MAP-015 TabBar unused import
├── ARCH-024 retry 抽象
├── ARCH-025 container OS runner interface
├── ARCH-028 app.go 拆 god object（按领域）
├── ARCH-029 session status 状态机
├── ARCH-032 LLM streaming interface
└── ARCH-033/034 frontend data-testid / 命名一致性

v1.8 Arch:
├── ARCH-019 DB provider 抽 engine.go
├── ARCH-020 store 12 抽 atomic.go
├── ARCH-026 log rotation
├── ARCH-027 sync 用 go-git
├── ARCH-030 sync PBKDF2 → x/crypto
├── ARCH-031 k8s watch reconnect backoff
└── ARCH-035 frontend 错误处理统一
```

### 阶段 6：依赖 + OS（v1.5 + v1.6 · ~2 周）

```
v1.5 Deps:
├── F-002 Go deps ~80 minor 升级（分批）
├── F-003 npm deps pinia 2→4 + vite 5→6
├── F-004 npm audit CI 严格
└── ARCH-030 sync PBKDF2

v1.6 OS:
├── ARCH-018 平台路径硬编码 → shell abstraction
└── ARCH-025 container runner 多 OS
```

### 阶段 7：文档 / OSS（v1.9 · 持续）

```
├── PM-019  9 locale key 一致性（CI 检查）
├── PM-023  SECURITY.md + dependabot.yml
├── PM-024  THIRD_PARTY_NOTICES 补 LGPL（合规）
├── PM-015 ~ PM-022  README/i18n/UX 修复
└── F-005    rejected（文件已存在，无需修）
```

---

## 📈 修复 ROI 速查

**P0/P1 + 必修 ROI 高（首批做）**：
- REV-015 / F-001 / F-006 / F-007 / F-010 / F-012 / F-014
- ARCH-015 / ARCH-019 / ARCH-020 / ARCH-031
- DEV-015 / DEV-017 / DEV-018 / DEV-019 / DEV-020 / DEV-029 / DEV-036 / DEV-037
- QA-015 / QA-017 / QA-023 / QA-024
- REV-016 / REV-017 / REV-018 / REV-019
- DBG-017 / DBG-021
- PM-019 / PM-023 / PM-024

**P2 + 修复收益中性（排期）**：其余按 milestone 顺序处理

**Skipped / withdrawn**（5 条）：见 verification.md

---

## 🔄 后续流程（启动 milestone 时）

1. 从 `milestone-map.md` 抽本 milestone 涉及的 finding
2. 每条 finding 读 `findings.md` 完整 schema（location / code_block / caller_chain / fix_diff_hint / test_target / verification）
3. 写 plan：每条 finding 走 5 步（实现 → test → 验证 → review → commit）
4. 完成后回填 verification.md 的 verdict

---

## ⚠️ 已知 Gap（未来 milestone 可补）

- `frontend/src/services/`（agent / k8s client / zmodem）0 覆盖
- `frontend/wailsjs/`（Wails 自动生成）未独立审
- `main.go` / `go.mod` 整体未深扫
- `frontend/src/App.vue` 仅 1/8 lens 覆盖
- Planner lens 主场（本里程碑无 Wave 计划）

---

## 📚 参考资料

- 8 个角色定义：`.claude/agents/0X-name-audit.md`
- ADPM v2 来源：`/Users/coderstory/CodeSource/adpm-ai-team/docs/v2/03-roles/`
- CodeGraph: `.codegraph/`（项目级 code 索引，可加速后续扫描）
- CLAUDE.md：项目总体约定