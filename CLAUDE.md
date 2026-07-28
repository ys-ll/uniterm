# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

# uniterm

Wails v2 + Vue 3 (xterm.js) + Go 桌面终端，覆盖 SSH/FTP/SFTP/SMB/WebDAV/S3/RDP/VNC/SPICE、本地/串口终端、Kubernetes、关系/非关系数据库，以及自主多轮执行的 AI Agent。macOS / Windows / Linux 三端。

前置工具：Go 1.23+、Node 20+、Wails CLI v2（macOS 还要 Xcode CLT；Linux 要 `libgtk-3-dev libwebkit2gtk-4.1-dev`；Windows 要 WebView2）。详见 `CONTRIBUTING.md`。

## 常用命令

前置工具（一次性安装，详见 `CONTRIBUTING.md`）：Go 1.23+、Node.js 20+、Wails CLI v2。

```bash
# 一次性
npm --prefix frontend install

# 开发（GUI 热重载）
wails dev

# 仅校验前端改动（不起 GUI，便宜）
npm --prefix frontend run build

# 前端改了但 wails dev 没看到效果 — 清 Vite 缓存
rm -rf frontend/dist frontend/node_modules/.vite && npm --prefix frontend run build

# 生产构建（产物 build/bin/）
wails build

# Go 测试
go test ./backend/...
go test ./backend/k8s/...                       # 某个包
go test ./backend/session/ -run TestSessionConnect -v   # 某个用例（-run 走正则）

# 前端测试（vitest，watch 模式）
npm --prefix frontend run test
npm --prefix frontend run test -- --run useTerminal   # 单跑某文件
```

## 架构

后端按职责分包，新功能先去对应包放；前端按角色拆 composable / store / service / component。

### 后端 — `backend/`

- `backend/session/` — 按协议拆的 session：`ssh_session.go` / `telnet_session.go` / `mosh_session.go` / `local_session_unix.go` / `local_session_windows.go` / `serial_session.go` / `sftp_session.go` / `ftp_session.go` / `smb_session.go` / `webdav_session.go` / `s3_session.go` / `rdp_session.go` / `vnc_session.go` / `spice_session.go` / `mongodb_session.go` / `redis_session.go`。横切支持：`manager.go`（编排）/ `tunnel_*` / `zmodem_*`。Local 走 build-tag 拆分；新建协议一般加一个 `*_session.go` + 在 `manager.go` 注册。
- `backend/database/` — 关系/非关系数据库统一层：`provider_<db>.go` 各自实现 DSN + schema，`engine.go`/`executor.go` 是公共执行层。新增数据库只在这里加一个 provider。
- `backend/k8s/` — kubeconfig 加载、client、rest 调用、watch、metrics，`Manager` 编排。
- `backend/store/` — JSON 落盘的持久化：connections、ai/skills、settings、local state、terminal history、recent、tunnels、quick commands。
- `backend/sync/` — 加密云同步（GitHub/GitLab/Gitee 私有仓库），git + keyring 三层：`crypto.go` / `git.go` / `keychain.go`。
- `backend/platform/` — 字体处理，build-tag 拆分（`fonts_darwin.go` / `fonts_unix.go` / `fonts_windows.go`）。
- `backend/log/` / `backend/update/` — 文件日志、应用内更新检查。

### 后端 — 根目录 `app*.go`

- `main.go` — 入口，`//go:embed all:frontend/dist` 嵌入前端，`wails.Run` 启动。macOS 走原生 App+Edit 菜单接管 Cmd+C/V/X/A/Z（issue #291）；其他平台无菜单避免空 GtkMenuBar 闪白条。
- `app.go` + `app_*_*.go` — `Bind` 给前端的 API（AI/LLM 代理、SFTP、窗口状态、settings load/save 等）。`app_darwin.go` / `app_windows.go` / `app_not*.go` 是 build-tag 拆分。**改这里要确认非本平台的 not* 文件不受牵连。**

### 前端 — `frontend/src/`

- `App.vue` — 单文件壳。
- `components/` — Vue 组件（连接、终端、Tab、设置、AI 侧边栏等）。
- `composables/` — 终端相关：`useTerminal*` / `useKeyboardShortcuts` / `useSuggestions` 等。**新增终端交互 hook 一般放这里**。
- `stores/` — Pinia：tab / panel / session / connection / settings / sync / AI / k8s / zmodem 等。
- `services/` — AI Agent 循环（`agent.ts` / `terminalAgent.ts`）、LLM 客户端（`llm.ts`）、k8s 客户端（`k8s*.ts`）、zmodem。
- `wailsjs/` — **Wails 自动生成**的 Go ↔ JS 绑定（来自 `app.go`）。**不要手改**；改 Go 端绑定后重跑 `wails dev` 即可重生成。
- `i18n/` / `types/` / `utils/` — 翻译、TS 类型、工具函数。

### 前后端状态怎么流动

`settingsStore`（前端 Pinia）→ `app.SaveSettings` / `LoadSettings`（Wails 绑定）→ `backend/store`（JSON 落盘）。**目前 terminal.theme 只存在前端；后端启动 PTY 时拿不到**。这是 Claude Code 主题色 bug 的根因之一（见下方 gotchas）。

## Gotchas

- **xterm.js 的 theme.background 必须是真实 hex/rgb，不能是 `var(--bg-base)`**。xterm.js 不解析 CSS 变量，会默认成黑色（`rgb:0/0/0`），然后经 OSC 11 报告给后台进程，TUI 应用（Claude Code / lazygit / btop）据此判断深浅主题就全错了。`frontend/src/composables/useTerminal.css-var.test.ts` 守住这个约束；自定义背景图路径要走 `useTerminalTheme.ts:resolveXtermBackground` 把 `--bg-base` 解出 hex 再塞回去。
- **Claude Code 用 OSC 11（有时也看 `COLORFGBG`）探测背景色**。uniterm 只在 PTY 启动时设 `TERM=xterm-256color`（`local_session_unix.go:ensureTerminalEnv`），从未设 `COLORFGBG`；且 xterm.js 不会主动广播主题变更。**用户切换主题 → 已运行的 Claude Code 不会感知，必须重启**。这是上游 Claude Code 已知 bug（`anthropics/claude-code` issues #77394 / #56848 / #49839 / #47705）。
- **背景图分支的 `theme.background='rgba(0,0,0,0)'` 坑**：xterm.js 解析后存成 `rgb:0/0/0`，OSC 11 报黑色。`useTerminalTheme.ts` 头注释点名了 Claude Code — 一旦开启自定义背景图，主题切换语义就不对了。
- **`frontend/wailsjs/` 是生成产物**。提交时一起提交，但不要手改。
- **build-tag 拆分**：`app_darwin.go` / `app_notdarwin.go` / `app_windows.go` / `app_notwindows.go` / `local_session_unix.go` / `local_session_windows.go` / `fonts_*.go`。改一个平台时记得 grep 一下 `not*` 镜像文件，避免漏改。

## 约定

- **注释宜少不宜多**：只在意图不明处点一句，不为"改一行代码写五六行注释"。代码自解释优先。
- **提交**：PR 标题用英文 conventional commit（`feat(scope):` / `fix(scope):`），正文英文 + 中文双语，中间 `---` 分隔；issue 用中文。
- **一分支一修复**，基于最新 main。
- 前端改动后 `npm --prefix frontend run build` 验证；终端行为需 `wails dev` 实测。
- 改 `app*.go` 的绑定后，TypeScript 调用方通常不需要改类型（`wails dev` 重生成 `frontend/wailsjs/`），但新加方法要在前端显式调用一遍确认绑定到位。