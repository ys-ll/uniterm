# Technology Stack

**Analysis Date:** 2026-07-28

## Languages

**Primary:**
- Go 1.26.2 (`go.mod`) — backend, desktop shell, all session drivers, DB providers, sync, k8s client, AI proxy
- TypeScript ~5.3+ (`frontend/package.json`) — frontend source (`frontend/src/**/*.ts`)
- Vue 3 SFC (`.vue`) — single-file components under `frontend/src/components/`, `frontend/src/App.vue`

**Secondary:**
- JavaScript (ESM) — Vite build config, generated Wails bindings under `frontend/wailsjs/`
- CSS / HTML — `frontend/src/style.css`, `frontend/index.html`
- JSON — i18n locale files (`frontend/src/i18n/locales/*.json`), persisted settings, AI session logs
- Bash — `scripts/sync-release-to-gitee.sh`

## Runtime

**Environment:**
- Go 1.23+ toolchain (declared `go 1.26.2` in `go.mod`)
- Node.js 20+ for the Vite frontend toolchain
- Wails CLI v2 for `wails dev` / `wails build` orchestration (`wails.json`)

**Package Manager:**
- npm — `frontend/package-lock.json` committed, install via `npm --prefix frontend install`
- Go modules — `go.mod` / `go.sum` committed; two `replace` directives pin forks:
  - `github.com/unixshells/mosh-go` → `github.com/ys-ll/mosh-go`
  - `github.com/rhnvrm/simples3` → `github.com/ys-ll/simples3`

## Frameworks

**Core (Desktop Shell):**
- Wails v2.13.0 (`github.com/wailsapp/wails/v2`) — Go ↔ webview bridge; embeds `frontend/dist` via `//go:embed all:frontend/dist` in `main.go`; `Bind` exposes `*App` (`main.go`) to JS through the auto-generated `frontend/wailsjs/go/main/App.js`.

**Frontend:**
- Vue 3.4+ (`frontend/package.json`) with `<script setup>` SFCs
- Vite 5.0+ (`frontend/vite.config.ts`) — dev server on port 34115, builds target `esnext`
- Pinia 2.1+ — state stores under `frontend/src/stores/` (tab/panel/session/connection/settings/sync/AI/k8s/zmodem)
- Element Plus 2.5+ — UI component library (`frontend/src/main.ts`, `frontend/src/components/*.vue`)
- @xterm/xterm 5.5+ with addons (`addon-fit`, `addon-search`, `addon-unicode11`, `addon-web-links`) — terminal rendering in `BaseTerminal.vue`
- @novnc/novnc 1.5+ — VNC web client rendered inside `VNCTabContent.vue`
- spice-html5-bower 1.7.3 — SPICE client inside `SPICETabContent.vue`
- zmodem.js 0.1.10 + `zmodemService.ts` — ZMODEM file transfer over serial/SSH
- js-yaml — kubeconfig parsing on frontend
- lz-string — terminal history compression
- @fontsource-variable/jetbrains-mono, @fontsource-variable/space-grotesk — embedded fonts

**Backend layout (Go, all under `backend/`):**
- `session/` — protocol drivers (`ssh_session.go`, `sftp_session.go`, `ftp_session.go`, `smb_session.go`, `webdav_session.go`, `s3_session.go`, `rdp_session.go`, `vnc_session.go`, `spice_session.go`, `mosh_session.go`, `telnet_session.go`, `serial_session.go`, `local_session_*.go`, `mongodb_session.go`, `redis_session.go`, `k8s_exec_session.go`, `database_session.go`), plus cross-cutting `tunnel_*`, `output_log.go`, `manager.go`, `vnc_proxy.go`, `spice_proxy.go`
- `database/` — `engine.go` + per-DB providers (`provider_mysql.go`, `provider_postgres.go`, `provider_sqlserver.go`, `provider_oracle.go`, `provider_rqlite.go`)
- `store/` — JSON-backed persistence (connections, AI sessions, settings, skills, commands, quick commands, recent, terminal history, tunnels, local state)
- `k8s/` — kubeconfig parsing, REST client, exec channel, logs, watch, metrics, `Manager`
- `sync/` — encrypted Git-based cloud sync (`git.go`, `crypto.go`, `keychain.go`, `sync_config.go`, `sync_service.go`)
- `platform/` — font discovery (`fonts_*.go` build-tag split for darwin/unix/windows)
- `log/` — file logger with panic capture registered in `main.go`
- `update/` — GitHub/Gitee release check (`checker.go`)

**Testing:**
- Go `testing` stdlib — `_test.go` files under `backend/k8s/`, `backend/session/`, `backend/store/`; run via `go test ./backend/...`
- Vitest 4.1+ (`frontend/package.json` devDep) — `*.test.ts` next to services and stores (`frontend/src/services/*.test.ts`, `frontend/src/stores/*.test.ts`)
- vue-tsc 2.0+ — type-check SFCs

**Build/Dev:**
- Wails CLI v2 (`wails.json`) — orchestrates frontend `npm install` / `npm run build` and Go compile
- Vite plugin-vue — SFC compilation

## Key Dependencies

**Critical:**
- `github.com/wailsapp/wails/v2 v2.13.0` — desktop shell; without this there is no app
- `github.com/ys-ll/mosh-go` (forked via `replace`) — Mosh protocol (no pure-Go upstream at required version)
- `github.com/ys-ll/simples3` (forked via `replace`) — S3 protocol driver

**SSH / Crypto:**
- `golang.org/x/crypto v0.51.0` — SSH, PBKDF2 (sync crypto)
- `github.com/pkg/sftp v1.13.10` — SFTP subsystem over SSH
- `github.com/xanzy/ssh-agent v0.3.3` (indirect) — SSH agent forwarding
- `github.com/skeema/knownhosts v1.3.1` (indirect) — host key verification

**Protocol Drivers:**
- `github.com/creack/pty v1.1.24` — local PTY
- `github.com/UserExistsError/conpty v0.1.4` — Windows ConPTY
- `go.bug.st/serial v1.7.1` — serial port
- `github.com/gorilla/websocket v1.5.3` — VNC/SPICE WebSocket proxies (`vnc_proxy.go`, `spice_proxy.go`) and k8s exec channel
- `github.com/jlaffaye/ftp v0.2.1` — FTP
- `github.com/cloudsoda/go-smb2` (replaced 2026-07-01) — SMB
- `github.com/studio-b12/gowebdav v0.12.0` — WebDAV
- `go.mongodb.org/mongo-driver v1.17.9` — MongoDB
- `github.com/redis/go-redis/v9 v9.21.0` — Redis

**Database Drivers:**
- `github.com/go-sql-driver/mysql v1.10.0`
- `github.com/lib/pq v1.12.3`
- `github.com/microsoft/go-mssqldb v1.10.0`
- `github.com/sijms/go-ora/v2 v2.9.0`
- `github.com/rqlite/gorqlite v0.0.0-20260504155303-…` (stdlib driver)

**Sync / Storage:**
- `github.com/go-git/go-git/v5 v5.19.1` — git operations for sync
- `github.com/zalando/go-keyring v0.2.8` — OS keychain (used in `backend/sync/keychain.go`)
- `gopkg.in/yaml.v3 v3.0.1` — kubeconfig and iTerm colors

**Windows-specific (indirect under `wailsapp/go-webview2`):**
- `git.sr.ht/~jackmordaunt/go-toast/v2` — Windows toast
- `github.com/Microsoft/go-winio`, `github.com/jchv/go-winloader`, `github.com/danieljoos/wincred` — Win32

## Configuration

**Environment:**
- No `.env` files are committed; secrets live in the OS keychain via `backend/sync/keychain.go` (`keychainService = "uniTerm"`). API keys for AI models are migrated from `AppSettings.AI.Models[*].APIKey` (JSON) into the keychain on save (`backend/store/settings_store.go`).
- Frontend exposes only `import.meta.env.VITE_VERSION` (`frontend/vite.config.ts`, `frontend/src/env.d.ts`), sourced from `process.env.VITE_VERSION || 'dev'` at build time.

**Build:**
- `wails.json` — project name `uniTerm`, output binary `uniTerm`, frontend dir `./frontend`
- `go.mod` — module `github.com/ys-ll/uniterm`
- `frontend/vite.config.ts` — Vite config: dev port 34115, `strictPort`, target `esnext`
- `frontend/tsconfig.json` — strict TS, `target: ES2020`, `moduleResolution: bundler`, `noUnusedLocals` / `noUnusedParameters` enabled
- `frontend/package.json` — npm scripts: `dev` (vite), `build` (vite build), `preview`

**Persistence paths (from `main.go` / `app.go`):**
- Config root: `os.UserConfigDir()/uniTerm/` (e.g. `~/Library/Application Support/uniTerm/` on macOS)
- Stores: `connections.json`, `settings.json`, `ai-sessions.json`, `terminal-history.json`, `skills.json`, `commands.json`, `quick-commands.json`, `tunnels.json`, `local-state.json`, `recent.json`, `sync-config.json`
- Sync repo clone: `<configDir>/uniTerm/sync-repo/`
- Logs: `backend/log/log.go` writes to `<configDir>/uniTerm/uniTerm.log` (write function: `log.Writef`)
- WebView2 user data (Windows): `os.TempDir()/uniTerm-webview2-<pid>` (`main.go`)

## Platform Requirements

**Development:**
- macOS: Xcode Command Line Tools (CGO for `creack/pty`, ConPTY not needed)
- Linux: `libgtk-3-dev`, `libwebkit2gtk-4.1-dev`
- Windows: WebView2 runtime (preinstalled on Windows 10+)
- All: Go 1.23+, Node.js 20+, Wails CLI v2

**Production:**
- Cross-compiled desktop binaries produced by `wails build` into `build/bin/`
- macOS uses native App + Edit menus (`menu.NewMenuFromItems` in `main.go`) to wire Cmd+C/V/X/A/Z through WKWebView first responder (issue #291); Linux/Windows use frameless window with no menu to avoid GTK menu-bar artifacts
- Frameless window disabled on macOS and when user toggles `systemTitleBar` in local state
- Linux workaround: `MaxWidth/MaxHeight` set to 9999 to avoid Wails #2431 multi-monitor clamp