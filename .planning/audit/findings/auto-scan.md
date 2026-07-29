# Auto-Scan · 自动化扫描结果

时间：2026-07-29
工具：`go vet`, `go list -m -u all`, `npm outdated`, `npm audit`

---

## 1. `go vet ./...` · 后端静态分析

### 1.1 IPv6 地址格式问题（7 处）

`fmt.Sprintf("%s:%d", host, port)` 在 IPv6 主机上有歧义（应使用 `net.JoinHostPort`）：

```
backend/session/mosh_session.go:47:22      passed to net.Dial at L58
backend/session/smb_session.go:48:22       passed to net.Dial at L53
backend/session/ssh_dial.go:18:22          passed to net.Dial at L26
backend/session/ssh_session.go:189:22      passed to net.Dial at L200
backend/session/tunnel_service.go:61:22    passed to net.Dial at L72
backend/session/telnet_session.go:62:22    passed to net.Dial at L64
app.go:1681:24                             passed to net.Dial at L1682
```

**类别**：bug（IPv6 用户无法连接）
**严重度**：P1（影响 IPv6 用户，但 IPv4 用户不受影响）
**ROI**：高（7 处都是同一个 fix 模式）
**Future Milestone**：v1.2 Bug Fixes + v1.4 Refactor（提取 `dialAddr` helper）

---

## 2. `go list -m -u all` · Go 依赖过期

**统计**：约 80+ 个直接依赖可更新。

### 2.1 主要过期项（major 版本）

| Package | Current | Latest | 类型 |
|---|---|---|---|
| `github.com/charmbracelet/glamour` | 0.8.0 | **1.0.0** | major bump |
| `github.com/charmbracelet/lipgloss` | 0.12.1 | **1.1.0** | major bump |
| `github.com/elazarl/goproxy` | 1.7.2 | 1.8.5 | minor |
| `github.com/goccy/go-json` | 0.8.1 | 0.10.6 | minor |
| `github.com/ProtonMail/go-crypto` | 1.1.6 | 1.4.1 | minor（安全相关）|
| `github.com/AzureAD/microsoft-authentication-library-for-go` | 1.6.0 | 1.8.0 | minor |

### 2.2 安全相关

- `github.com/ProtonMail/go-crypto` — 加密库，需关注是否有 CVE 修复
- `github.com/goccy/go-json` — JSON 解析，需关注是否有性能/安全更新

**类别**：deps
**严重度**：P2（无直接 CVE 公告，但 minor 升级有风险）
**ROI**：中（依赖多，需要逐个测试）
**Future Milestone**：v1.5 Dependency Updates

---

## 3. `npm outdated` · 前端依赖过期

### 3.1 Major 升级（破坏性）

| Package | Current | Latest | 备注 |
|---|---|---|---|
| `pinia` | 2.3.1 | **4.0.2** | Vue 3 状态管理有破坏性变更 |
| `@vitejs/plugin-vue` | 5.2.4 | **6.0.8** | Vite 6 大版本 |
| `@lucide/vue` | 1.17.0 | **1.27.0** | minor 升级多 |

### 3.2 Minor 升级（安全/小特性）

| Package | Current | Latest |
|---|---|---|
| `@fontsource-variable/jetbrains-mono` | 5.2.8 | 5.3.0 |
| `@fontsource-variable/space-grotesk` | 5.2.10 | 5.3.0 |
| `element-plus` | 2.14.1 | 2.14.3 |
| `js-yaml` | 5.2.1 | 5.2.2 |

**类别**：deps
**严重度**：P2
**ROI**：中（pinia 2→4 / vite 5→6 是大改；其它可批量升）
**Future Milestone**：v1.5 Dependency Updates

---

## 4. `npm audit` · 前端安全审计

**结果**：**无已知漏洞**

```
vulnerabilities: None
total deps: 0 (审计异常，未跑全，详见备注)
```

**备注**：`npm audit` 返回 totalDependencies 为 None，怀疑 npm 配置或网络问题。需在后续 verification 阶段重跑确认。

---

## 5. 自动化扫描未覆盖项（需 LLM 审计补）

- [ ] `staticcheck` / `golangci-lint` 未安装（可考虑装上做更深入的 Go 静态分析）
- [ ] 前端 `eslint` 配置未发现
- [ ] 前端 `vitest` 覆盖率报告未生成
- [ ] `go test -cover` 各包覆盖率未统计

---

## 6. 已知外部依赖重大版本变更（背景信息）

- **Wails v3** — 官方 stable 未发布（fork 仍在 Wails v2）
- **xterm 7.x** — 未发布
- **Vue 3.5+** — 已发布但未升

## 7. 总结

| 类别 | 条目数 | 严重度分布 |
|---|---|---|
| bug (IPv6) | 7 | 全部 P1 |
| deps Go | ~80 (含 minor) | 多数 P2 |
| deps npm | ~7 (major + minor) | 多数 P2 |
| security npm | 0 | — |
| **合计需要修** | ~94 | P1: 7 / P2: 87 |