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

### F-005: 项目缺 CONTRIBUTING.md / CHANGELOG.md / GH templates

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

**Context**：项目要成为「一流开源项目」，需要标准 OSS 文件。

**Evidence**：
```
$ ls CONTRIBUTING.md CHANGELOG.md
ls: CONTRIBUTING.md: No such file or directory
ls: CHANGELOG.md: No such file or directory
$ ls .github/ISSUE_TEMPLATE .github/PULL_REQUEST_TEMPLATE 2>/dev/null
(空)
```

**Suggested Fix**：
1. 写 `CONTRIBUTING.md`：开发流程 / PR 规范 / 本地构建命令
2. 写 `CHANGELOG.md`：从 git log 生成（已有 conventional commits）
3. 加 `.github/ISSUE_TEMPLATE/bug_report.md` 和 `feature_request.md`
4. 加 `.github/PULL_REQUEST_TEMPLATE.md`

**Test Plan**：CI 可加 `action-lint` 或类似工具验证模板。

---

（更多 finding 继续追加）