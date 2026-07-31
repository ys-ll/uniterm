# uniterm

Wails v2 + Vue 3 (xterm.js 终端) + Go 后端。macOS / Windows / Linux 桌面端，覆盖 SSH/FTP/SFTP/SMB/WebDAV/S3/RDP/VNC/SPICE、本地与串口终端、Kubernetes、关系/非关系数据库，以及自主多轮执行的 AI Agent。

## 常用命令

前置工具（一次性安装，详见 `CONTRIBUTING.md`）：Go 1.23+、Node.js 20+、Wails CLI v2。

```bash
# 一次性：安装前端依赖
npm --prefix frontend install

# 开发：热重载 Go + 前端（GUI 窗口）
wails dev

# 仅校验前端改动（不启动 GUI，便宜）
npm --prefix frontend run build

# 前端改了但 wails dev 没看到效果：清 Vite 缓存再重建
rm -rf frontend/dist frontend/node_modules/.vite && npm --prefix frontend run build

# 生产构建（产物在 build/bin/）
wails build

# Go 测试（k8s / store / session 目录下有 _test.go）
go test ./backend/...

# 单跑某一个包
go test ./backend/k8s/...

# 单跑某一个测试函数（-run 走正则）
go test ./backend/session/ -run TestSessionConnect -v
```

## 架构

后端按职责分包，新增功能先去对应包里放：

- `main.go` — 程序入口；`//go:embed all:frontend/dist` 嵌入前端，`wails.Run` 启动；macOS 走原生 App+Edit 菜单以接管 Cmd+C/V/X/A/Z（issue #291），其他平台无菜单避免空 GtkMenuBar 闪白条。
- `app.go` + `app_*_*.go`（根目录）— Wails `Bind` 暴露的 API（AI/LLM 代理、SFTP 等）。`app_darwin.go` / `app_windows.go` / `app_not*.go` 是 build-tag 拆分；改这里要确认非本平台的 not* 文件不受牵连。
- `backend/session/` — 按协议拆：SSH/Telnet/Mosh/Local/Serial/SFTP/FTP/SMB/WebDAV/S3/RDP/VNC/SPICE/MongoDB/Redis 各自 `*_session.go`，由 `manager.go` 编排。`tunnel_*` / `mosh_session.go` / `zmodem_*` 是横切支持。Local 终端用 `local_session_unix.go` / `local_session_windows.go` build-tag 拆分。
- `backend/database/` — `provider_*.go` 各自实现 DSN + schema，`engine.go`/`executor.go` 是公共执行层。新增数据库只在这里加一个 provider。
- `backend/store/` — JSON 落盘的持久化：connections、ai/skills、settings、local state、terminal history、recent、tunnels、quick commands。
- `backend/k8s/` — kubeconfig 加载、client、rest 调用、watch、metrics、`Manager` 编排。
- `backend/sync/` — 加密云同步（GitHub/GitLab/Gitee 私有仓库），走 git + keyring。`crypto.go`/`git.go`/`keychain.go` 分层。
- `backend/platform/` — 字体处理，build-tag 拆分（`fonts_darwin.go`/`fonts_unix.go`/`fonts_windows.go`）。
- `backend/log/` — 文件日志，`main.go` 注册了 panic 兜底。
- `backend/update/` — 应用内更新检查。

前端：

- `frontend/src/App.vue` — 单文件壳。
- `frontend/src/components/` — Vue 组件（连接、终端、Tab、设置、AI 侧边栏等）。
- `frontend/src/composables/` — 终端相关：`useTerminal*`、`useKeyboardShortcuts`、`useSuggestions` 等。新增终端交互 hook 一般放这里。
- `frontend/wailsjs/` — **Wails 自动生成**的 Go ↔ JS 绑定（来自 `app.go`），不要手改；改 Go 端绑定后重新跑 `wails dev` 即可重生成。
- `frontend/src/stores/` — Pinia：tab/panel/session/connection/settings/sync/AI/k8s/zmodem 等。
- `frontend/src/services/` — AI Agent 循环（`agent.ts`/`terminalAgent.ts`）、LLM 客户端（`llm.ts`）、k8s 客户端（`k8s*.ts`）、zmodem。
- `frontend/src/i18n/`, `types/`, `utils/` — 翻译、TS 类型、工具函数。

## 约定

- **注释宜少不宜多**：只在意图不明处点一句，不为「改一行代码写五六行注释」。代码自解释优先。
- 提交：PR 标题用英文 conventional commit（`feat(scope):` / `fix(scope):`），正文英文 + 中文双语，中间 `---` 分隔；issue 用中文。
- 一分支一修复，基于最新 main。
- 前端改动后 `npm --prefix frontend run build` 验证；终端行为需 `wails dev` 实测。
- 改 `app*.go` 的绑定后，TypeScript 调用方通常不需要改类型（`wails dev` 重生成 `frontend/wailsjs/`），但新加方法要在前端显式调用一遍确认绑定到位。
