# uniterm v1.1 Audit · Findings

每条 finding 一节，所有矩阵从这一份文件派生。

**Finding 编号**：`F-NNN`（全局递增）

**Lens**（哪个视角发现的）：
- **产品** — UX / 文档 / i18n / OSS 一流标准
- **架构** — 模块边界 / OS 抽象 / 设计一致性 / 技术债
- **QA** — 测试覆盖 / 边界用例 / AC 验证
- **研发** — 性能 / 内存 / 重构机会 / bug 调查 / 稳定性

**Severity**：
- **P0** — 不可逆副作用 / 数据丢失 / 全局崩溃 / 安全 CVE / 合规阻塞
- **P1** — 阻塞关键路径 / 反复修 / 高频 crash
- **P2** — 单角色失败 / 非阻塞 / UX 小问题
- **P3** — 文档 / 注释 / 命名 / nitpick

**Category**：
- `bug` · `perf` · `refactor` · `deps` · `config` · `os-compat` · `test` · `arch` · `docs`

**Risk**（修复风险）：
- `low`（局部，不影响 API）· `medium`（跨文件）· `high`（破坏性 / 大改）

**Impact**（修复收益）：
- `critical`（P0 bug / 核心安全）· `high`（明显 perf / 修高频 crash）· `medium`（UX 改进 / 设计债清理）· `low`（nitpick）

**Future Milestone**：
- v1.2 bug · v1.3 perf · v1.4 refactor · v1.5 deps · v1.6 os-compat · v1.7 test · v1.8 arch · v1.9 docs

---

## Findings

### F-001: IPv6 地址格式导致 net.Dial 失败（×7 处）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P1 |
| Category | bug |
| Location | backend/session/{mosh,smb,ssh_dial,ssh_session,tunnel_service,telnet}_session.go:..., app.go:1681 |
| Risk | low |
| Impact | critical |
| ROI | high |
| Milestone | v1.2 |

**Context**：`fmt.Sprintf("%s:%d", host, port)` 在 IPv6 主机上有歧义（应使用 `net.JoinHostPort`）。IPv6 用户无法连接任何 backend 会话。

**Evidence**：`go vet` 报告 7 处：
```
backend/session/mosh_session.go:47    passed to net.Dial at L58
backend/session/smb_session.go:48     passed to net.Dial at L53
backend/session/ssh_dial.go:18        passed to net.Dial at L26
backend/session/ssh_session.go:189    passed to net.Dial at L200
backend/session/tunnel_service.go:61  passed to net.Dial at L72
backend/session/telnet_session.go:62  passed to net.Dial at L64
app.go:1681                           passed to net.Dial at L1682
```

**Suggested Fix**：提取 `dialAddr(host, port string) string` helper，内部用 `net.JoinHostPort(host, strconv.Itoa(port))`。7 处统一替换。

**Test Plan**：
- 单测：`net.JoinHostPort("[::1]", "8080")` → `"[::1]:8080"`
- 集成测试：mock IPv6 DNS 解析，确认 dial 成功
- 回归：现有 IPv4 测试仍通过

---

### F-002: Go 依赖过期（约 80+ 包，主要为 minor）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P2 |
| Category | deps |
| Location | go.mod / go.sum（多个）|
| Risk | medium |
| Impact | medium |
| ROI | medium |
| Milestone | v1.5 |

**Context**：~80 个直接 Go 依赖可更新到更新版本。多数是 minor 升级。

**Evidence**（主要过期项）：
| Package | Current | Latest | 类型 |
|---|---|---|---|
| `github.com/charmbracelet/glamour` | 0.8.0 | **1.0.0** | major |
| `github.com/charmbracelet/lipgloss` | 0.12.1 | **1.1.0** | major |
| `github.com/ProtonMail/go-crypto` | 1.1.6 | 1.4.1 | minor（安全相关）|
| `github.com/goccy/go-json` | 0.8.1 | 0.10.6 | minor |
| `github.com/elazarl/goproxy` | 1.7.2 | 1.8.5 | minor |

**Suggested Fix**：分批升级 + 跑全套测试。Major 升级（glamour/lipgloss）单独处理。

**Test Plan**：
- 每批升级后跑 `go test ./...`
- 关注 API 变更（charmbracelet 系列 API 有破坏性变更）

---

### F-003: 前端依赖过期（pinia 2→4, vite 5→6）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P2 |
| Category | deps |
| Location | frontend/package.json |
| Risk | high |
| Impact | medium |
| ROI | medium |
| Milestone | v1.5 |

**Context**：前端有 2 个 major 升级和多个 minor 升级。

**Evidence**：
| Package | Current | Latest | 类型 |
|---|---|---|---|
| `pinia` | 2.3.1 | **4.0.2** | major（破坏性 API 变更）|
| `@vitejs/plugin-vue` | 5.2.4 | **6.0.8** | major |
| `@lucide/vue` | 1.17.0 | **1.27.0** | minor（差 10 个 minor）|
| `@fontsource-variable/jetbrains-mono` | 5.2.8 | 5.3.0 | minor |
| `element-plus` | 2.14.1 | 2.14.3 | minor |
| `js-yaml` | 5.2.1 | 5.2.2 | minor |

**Suggested Fix**：
1. 先升 minor 批量（小、低风险）
2. 单独评估 pinia 2→4（state 风格变了）
3. vite 5→6 等 Vite 官方升级指南

**Test Plan**：
- 升级后跑 `npm --prefix frontend run build`
- 跑 vitest 套件
- 手动验证关键流程：登录、建连、保存连接

---

### F-004: 前端 npm audit 无已知漏洞（已确认）

| Field | Value |
|---|---|
| Lens | 研发 |
| Severity | P3 |
| Category | deps |
| Location | frontend/package-lock.json |
| Risk | low |
| Impact | low |
| ROI | high |
| Milestone | v1.5 |

**Context**：`npm audit` 显示无已知漏洞。

**Evidence**：`vulnerabilities: None`（来自 npm audit json 输出）

**Suggested Fix**：无需修复。继续保持 CI 跑 `npm audit`。

**Test Plan**：CI 加 `npm audit --audit-level=high` 检查。

---

### F-005: 项目缺 CONTRIBUTING.md / CHANGELOG.md / GH templates — WITHDRAWN

| Field | Value |
|---|---|
| Lens | 产品 |
| Severity | P2 |
| Category | docs |
| Location | 项目根目录 |
| Risk | low |
| Impact | medium |
| ROI | high |
| Milestone | v1.9 |
| **Status** | **WITHDRAWN — premise incorrect** |

**Context**：原始 finding 声称项目缺少 `CONTRIBUTING.md`、`CHANGELOG.md`、`.github/ISSUE_TEMPLATE/`、`.github/PULL_REQUEST_TEMPLATE.md` 等 OSS 文件。审计于 v1.9 重启时复核，发现这些文件**全部已存在**，原始 evidence 是过时的。

**Verification (v1.9 audit)**：

```
$ ls CONTRIBUTING.md CHANGELOG.md LICENSE CODE_OF_CONDUCT.md SECURITY.md
CHANGELOG.md
CODE_OF_CONDUCT.md
CONTRIBUTING.md
LICENSE
SECURITY.md                  # added in v1.9 (PM-023)

$ ls .github/ISSUE_TEMPLATE .github/PULL_REQUEST_TEMPLATE.md
.github/PULL_REQUEST_TEMPLATE.md
.github/ISSUE_TEMPLATE:
bug_report.md
config.yml
docs_update.md
feature_request.md

$ ls .github/dependabot.yml   # added in v1.9 (PM-023)
.github/dependabot.yml
```

**Resolution**：
- F-005 的全部 suggested fix 在更早的 v1.x 周期内已落地，本次审计重新确认无遗漏。
- 新增的 OSS 缺口（SECURITY.md、Dependabot）已在 v1.9 单独以 **PM-023** 跟踪并修复，本 finding 关闭即可。
- F-005 状态：`WITHDRAWN`，不再计入 backlog。

---

## v1.9 docs milestone · PM findings (product lens)

产品视角在 v1.9 阶段追加的 10 条 PM-NNN finding，已在 v1.9 docs 分支全部处理。

| PM ID | Severity | Status | Fix summary |
|-------|----------|--------|-------------|
| PM-015 | P2 | FIXED    | README.md: 加 Quick Start + FAQ（SSH 密钥/字体/端口转发/AI/配置/Linux 依赖/上报渠道） |
| PM-016 | P2 | FIXED    | README_zh-CN.md: 与 PM-015 镜像同步，快速开始 + 常见问题两节到位 |
| PM-017 | P2 | FIXED    | AISidebar.vue: 3 处硬编码中文（已关联 / 引用终端 / Skill·命令）迁出到 i18n，9 locale 全翻译 |
| PM-018 | P2 | FIXED    | StartTabContent.vue: 首启动空状态（无连接时显示「创建连接 / 本地终端 / 阅读文档」） |
| PM-019 | P1 | FIXED    | 7 个 locale 缺 conn.ftpSkipVerify / conn.ftpSkipVerifyDesc，补齐后 9 locale 键完全一致 |
| PM-020 | P2 | FIXED    | i18n/index.ts: 缺键告警兜底；缺失 locale 命中 → 告警一次后回退 en；en 也无 → 告警后渲染 `[shortName]` 而不是原始 key 路径 |
| PM-021 | P2 | FIXED    | 13 个高频组件的图标按钮：把已有 `:title` 镜像成 `:aria-label`，不写空 aria-label |
| PM-022 | P3 | FIXED    | 审计 9 locale：键集合已一致（PM-019）、无空值、无未翻译键、3 条「短译」（ja「接続成功」、zh-TW「解析失敗」等）是日/汉语言的正常简写，非真截断 |
| PM-023 | P1 | FIXED    | 新增 SECURITY.md（私有上报渠道 / SLA / 加密说明 / 不受理范围）+ .github/dependabot.yml（go/npm/gha 周更，charmbracelet+wails 因 major 破坏性变更排除自动分组） |
| PM-024 | P1 | FIXED    | THIRD_PARTY_NOTICES.md 扩充：spice-html5 LGPL-3.0 重链接权、Space Grotesk OFL、所有 npm runtime 包、Go 直接依赖 + mosh-go GPL-3.0 影响；新增「How to Regenerate」指引 |

**Verification**

```
$ npm --prefix frontend run build
✓ 3690 modules transformed.
dist/assets/index-CktpQ9AS.js  2,758.22 kB
✓ built in 3.64s
```

`git log --oneline main..HEAD`（10 commits on top of main）：

```
0b6dbf8 docs(audit): mark F-005 withdrawn — OSS files all present (verified)
5829287 docs(readme-zh): add Quick Start and FAQ sections (PM-016 parity)
13d6d2b docs(readme): add Quick Start and FAQ sections (PM-015)
0cd7102 feat(start): add first-launch empty state with quick actions (PM-018)
3875422 feat(a11y): mirror :title to :aria-label on icon-only buttons (PM-021)
1731abb docs(oss): audit and expand THIRD_PARTY_NOTICES.md (PM-024)
3bca510 feat(oss): add SECURITY.md and .github/dependabot.yml (PM-023)
b072762 refactor(i18n): move AISidebar hardcoded Chinese strings to i18n (PM-017)
e3d2dc8 feat(i18n): warn-on-miss fallback that never renders raw key path (PM-020)
4bb5571 fix(i18n): add missing conn.ftpSkipVerify keys to 7 locales (PM-019)
```

---

（更多 finding 继续追加）