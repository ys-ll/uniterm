# Codebase Structure

**Analysis Date:** 2026-07-28

## Directory Layout

```text
uniterm/
├── main.go                       # Entry; wails.Run, panic recovery, embed dist
├── app.go                        # Wails-bound façade: ~204 exported methods
├── app_darwin.go                 # macOS-specific (key-repeat, app menu)
├── app_windows.go                # Windows HWND subclassing + WndProc defer
├── app_notdarwin.go              # Build-tag stub
├── app_notwindows.go             # Build-tag stub
├── go.mod / go.sum               # Go 1.23 module: github.com/ys-ll/uniterm
├── wails.json                    # Wails project metadata
│
├── backend/
│   ├── session/                  # 18 protocol session implementations
│   │   ├── session.go            # Session interface + baseSession + ConnectionConfig
│   │   ├── manager.go            # SessionManager (thread-safe map)
│   │   ├── ssh_session.go        # SSH + post-login hooks (embed baseSession)
│   │   ├── sftp_session.go       # SFTP file transfers (reuses SSH client)
│   │   ├── telnet_session.go
│   │   ├── mosh_session.go
│   │   ├── local_session_unix.go     # //go:build !windows
│   │   ├── local_session_windows.go
│   │   ├── serial_session.go
│   │   ├── ftp_session.go
│   │   ├── smb_session.go
│   │   ├── webdav_session.go
│   │   ├── s3_session.go
│   │   ├── rdp_session.go        # RDP over RDP client + WndProc embed
│   │   ├── rdp_session_stub.go   # Non-Windows stub
│   │   ├── vnc_session.go        # + vnc_proxy.go
│   │   ├── spice_session.go      # + spice_proxy.go (spice-html5 webview)
│   │   ├── redis_session.go
│   │   ├── mongodb_session.go
│   │   ├── database_session.go   # SQL via database.Provider
│   │   ├── monitor_session.go    # System monitor (CPU/mem/disk/net/processes)
│   │   ├── k8s_exec_session.go   # Pod exec WebSocket inside k8s.Manager
│   │   ├── ssh_auth.go           # makeSSHAuthMethods (key/password/agent)
│   │   ├── ssh_config.go         # ~/.ssh/config parser
│   │   ├── post_login_expect.go  # Interactive expect/send automation
│   │   ├── output_log.go         # Session output logger (Panel-scoped)
│   │   ├── zmodem_detect*.go     # ZMODEM header detection
│   │   ├── tunnel.go             # SSH jump-host / user tunnel types
│   │   ├── tunnel_service.go     # TunnelService manager
│   │   └── tunnel_forward.go     # Per-session + user tunnel forwarders
│   │
│   ├── database/                 # SQL provider registry
│   │   ├── provider.go           # Provider interface + Register / NewProvider
│   │   ├── engine.go             # BuildDSN, NewDB, queryStrings
│   │   ├── executor.go           # execPrepared wrapper
│   │   ├── schema.go             # TableInfo / SchemaResult / ColumnDef / IndexDef
│   │   ├── provider_mysql.go     # self-Register via init()
│   │   ├── provider_postgres.go
│   │   ├── provider_oracle.go
│   │   ├── provider_sqlserver.go
│   │   └── provider_rqlite.go
│   │
│   ├── k8s/                      # Kubernetes manager + REST + watch + log stream
│   │   ├── manager.go            # Manager (conns/watches/logs, EventEmitter)
│   │   ├── kubeconfig.go         # YAML parsing, ContextInfo
│   │   ├── client.go             # HTTP client / TLS / bearer-token build
│   │   ├── rest.go               # Do() — generic REST request
│   │   ├── watch.go              # startWatchStream
│   │   ├── logs.go               # startLogStream
│   │   └── server_addr.go        # API server URL resolution
│   │
│   ├── store/                    # JSON-file persistence (one file per concern)
│   │   ├── connection_store.go   # connections.json (+ PasswordStore hook)
│   │   ├── ai_session_store.go   # ai_sessions.json
│   │   ├── settings_store.go     # settings.json
│   │   ├── local_state_store.go  # local_state.json (window geom, title bar)
│   │   ├── quick_commands_store.go
│   │   ├── commands_store.go
│   │   ├── skills_store.go
│   │   ├── tunnel_store.go
│   │   ├── terminal_history_store.go
│   │   ├── recent_store.go
│   │   └── recent_store_test.go
│   │
│   ├── sync/                     # Cloud sync (GitHub/GitLab/Gitee private repo)
│   │   ├── sync_service.go       # SyncService façade
│   │   ├── sync_config.go        # SyncConfig persistence
│   │   ├── crypto.go             # Age / AES encryption for sync payload
│   │   ├── git.go                # Git CLI wrappers
│   │   └── keychain.go           # OS keychain (token + PasswordStore impl)
│   │
│   ├── platform/                 # OS-specific helpers (build tags)
│   │   ├── fonts.go              # Public GetFontFamilies
│   │   ├── fonts_darwin.go       # macOS CoreText
│   │   ├── fonts_unix.go         # Linux fontconfig
│   │   ├── fonts_windows.go      # Windows registry / DirectWrite
│   │   └── fonts_ttf.go + test   # Shared TTF enumeration
│   │
│   ├── log/                      # File logger + panic hookup
│   │   └── log.go                # Init / Writef / Close (mutex-guarded)
│   │
│   └── update/                   # In-app updater
│       └── checker.go            # GitHub release check
│
├── frontend/                     # Vue 3 + Vite + TypeScript
│   ├── package.json              # npm scripts; deps pin xterm, pinia, element-plus
│   ├── wails.json                # Wails frontend metadata
│   ├── tsconfig.json / tsconfig.node.json
│   ├── dist/                     # Vite build output (embedded by `main.go`)
│   ├── node_modules/             # npm deps (gitignored)
│   ├── wailsjs/                  # **GENERATED** — Go ↔ JS bindings, do not edit
│   │   ├── go/main/App.{js,d.ts,models.ts}    # The façade
│   │   └── runtime/{runtime.js,runtime.d.ts,package.json}
│   └── src/
│       ├── main.ts               # createApp(Pinia + ElementPlus); mount; global ctxmenu
│       ├── App.vue               # Single-file shell (header / sidebar / tab router)
│       ├── env.d.ts              # Ambient types
│       ├── style.css
│       │
│       ├── components/           # 53 .vue files (see Key File Locations)
│       │
│       ├── composables/          # Vue composables
│       │   ├── useTerminal.ts            # xterm lifecycle, theme, events
│       │   ├── useTerminalInput.ts       # Input handling, ZMODEM mode
│       │   ├── useTerminalMenu.ts        # Right-click context menu
│       │   ├── useTerminalThemeOptions.ts
│       │   ├── useTextCopyMenu.ts        # Global INPUT/TEXTAREA context menu
│       │   ├── useKeyboardShortcuts.ts   # Keybinding map (module-level)
│       │   ├── useSuggestions.ts         # History/quick-cmd/AI suggestions
│       │   ├── useHighlight.ts           # Command highlighter
│       │   ├── useFocusTerminal.ts
│       │   ├── useTunnelCredentials.ts
│       │   ├── useUpdateCheck.ts
│       │   └── itermcolorsParser.ts
│       │
│       ├── stores/               # Pinia stores (one per concern)
│       │   ├── tabStore.ts                # Tab/workspace router (large)
│       │   ├── panelStore.ts              # Panels + transferTasks + VNC cache
│       │   ├── sessionStore.ts            # Buffered chunks per sessionID
│       │   ├── connectionStore.ts         # Connections + groups
│       │   ├── settingsStore.ts           # All app settings
│       │   ├── aiStore.ts                 # AI Agent state (module-level eventOn)
│       │   ├── k8sStore.ts                # Resources tree / detail
│       │   ├── syncStore.ts               # Sync state
│       │   ├── tunnelStore.ts
│       │   ├── commandStore.ts
│       │   ├── quickCommandStore.ts
│       │   ├── localStateStore.ts
│       │   ├── skillStore.ts
│       │   ├── zmodemStore.ts
│       │   └── *.test.ts                   # Co-located Vitest specs
│       │
│       ├── services/             # Stateless helpers + side-effect clients
│       │   ├── agent.ts                   # AI agent loop
│       │   ├── terminalAgent.ts           # execute_command/watchOutput
│       │   ├── llm.ts                     # ChatCompletion wrapper
│       │   ├── message.ts                 # Anthropic-format message types
│       │   ├── k8sClient.ts               # High-level K8s façade
│       │   ├── k8sResources.ts            # Resource type mapping
│       │   ├── k8sActions.ts              # Mutating actions (delete/scale/etc.)
│       │   ├── k8sCrd.ts                  # CRD handling
│       │   ├── k8sMetrics.ts              # Metrics queries
│       │   ├── k8sQuantity.ts             # k8s quantity parsing
│       │   ├── terminalManager.ts         # Multi-terminal registry
│       │   ├── zmodemService.ts           # ZMODEM send/receive via base64
│       │   └── *.test.ts
│       │
│       ├── types/                # TS-only shared types
│       │   ├── workspace.ts               # Tab, Panel, PanelLayout, LayoutNode
│       │   ├── session.ts                 # ConnectionConfig, SessionStatus, …
│       │   ├── settings.ts                # All settings shapes
│       │   ├── ai.ts                      # AIMessage, AIConfig, AIAgentStatus
│       │   ├── database.ts                # DB query results
│       │   ├── redis.ts / mongodb.ts
│       │   ├── k8s.ts                     # K8sTab
│       │   ├── command.ts / skill.ts
│       │   └── zmodem.d.ts                # ambient types
│       │
│       ├── i18n/
│       │   ├── index.ts                   # t(), useI18n(), elLocale
│       │   └── locales/{de,en,es,fr,ja,ko,ru,zh-CN,zh-TW}.json
│       │
│       ├── utils/                # Small stateless helpers
│       │   ├── cursor.ts                  # stripCursorBlink
│       │   ├── formatFontFamily.ts
│       │   └── quickConnect.ts
│       │
│       └── vendor/
│           └── spice-html5.js             # Vendored SPICE JS client
│
├── build/                        # Platform-specific resources
│   ├── appicon.png
│   ├── darwin/Info.plist, Info.dev.plist
│   ├── linux/uniterm.desktop
│   └── windows/icon.ico + installer/
│
├── docs/guide/                   # VitePress site (zh + en)
│   ├── package.json / package-lock.json
│   ├── .vitepress/
│   ├── en/{getting-started,protocols,features/*,…}.md
│   └── zh/…
│
├── scripts/sync-release-to-gitee.sh
├── .github/workflows/{build,deploy-docs,update-homebrew,update-scoop}.yml
├── README.md, README_zh-CN.md
├── CHANGELOG.md, ROADMAP.md, CONTRIBUTING.md, CODE_OF_CONDUCT.md, LICENSE
├── .planning/codebase/           # ← this directory
└── .gitignore, wails.json
```

## Directory Purposes

**`backend/`:**
- Purpose: All non-`main` Go code, grouped by responsibility.
- Contains: Go packages (`session`, `database`, `k8s`, `store`, `sync`, `platform`, `log`, `update`); no `cmd/` subdirectory because the binary is at the repo root (`main.go`).
- Key files: `backend/session/session.go` (interface), `backend/session/manager.go` (dispatcher), `backend/k8s/manager.go` (K8s manager), `backend/database/provider.go` (DB registry), `backend/store/connection_store.go` (persistence façade).

**`backend/session/`:**
- Purpose: 18 protocol implementations + shared tunnel/post-login/zmodem/output-log helpers.
- Contains: One `*_session.go` per protocol; cross-cutting `*_test.go` for expect/zmodem/output_log/tunnel_forward/k8s_exec_channel; shared base types in `session.go`.
- Key files: `session.go` (interface + baseSession + ConnectionConfig + post-login helpers), `manager.go`, `output_log.go`, `tunnel_service.go`.

**`backend/database/`:**
- Purpose: Plug-in SQL providers with a shared `Provider` interface.
- Contains: `provider.go` registry; one `provider_<db>.go` per supported DB; shared `engine.go`/`executor.go`/`schema.go`.
- Adding a database = add `provider_<db>.go` that calls `Register(dbType, ...)` from `init()`.

**`backend/k8s/`:**
- Purpose: kubeconfig parsing, REST/watch/log-stream/exec over a single HTTP/WS client.
- Contains: `Manager` orchestrator + per-feature files (`kubeconfig.go`, `client.go`, `rest.go`, `watch.go`, `logs.go`, `server_addr.go`).

**`backend/store/`:**
- Purpose: Flat JSON persistence, one file per concern.
- Contains: 10 stores + `_test.go`. No indexes/migrations — pure JSON.

**`backend/sync/`:**
- Purpose: Encrypted cloud sync to a private git repo.
- Contains: Service + git/crypto/keychain layers + sync-config store.

**`backend/platform/`, `backend/log/`, `backend/update/`:**
- Purpose: OS-specific helpers, file logger, in-app update check.
- Contains: small focused packages; build-tag split for fonts.

**`frontend/src/`:**
- Purpose: Vue 3 SPA.
- Contains: `main.ts` (entry), `App.vue` (shell), then `components/`, `composables/`, `stores/`, `services/`, `types/`, `i18n/`, `utils/`, `vendor/`.

**`frontend/src/components/`:**
- Purpose: All UI surfaces — tabs, dialogs, panels.
- Contains: 53 `.vue` files. Naming convention: `<Feature>TabContent.vue` for tab bodies, `<Feature>Dialog.vue` for modals, `<Feature>Panel.vue` for sidebar panels, `BaseTerminal.vue` for the shared xterm host.

**`frontend/src/composables/`:**
- Purpose: Vue 3 composables (small reactive/logical units).
- Contains: Terminal setup (`useTerminal*`), input/menu/shortcuts/suggestions, tunnel creds, update check, parser.

**`frontend/src/stores/`:**
- Purpose: Pinia stores, one per concern.
- Contains: 14 stores + co-located Vitest specs (`*.test.ts`).

**`frontend/src/services/`:**
- Purpose: Side-effect-bearing helpers (LLM calls, K8s mutations, ZMODEM, terminal manager).
- Contains: `agent.ts`/`terminalAgent.ts`/`llm.ts` (AI); `k8s*.ts` (Kubernetes); `zmodemService.ts`; `terminalManager.ts`.

**`frontend/src/types/`, `i18n/`, `utils/`, `vendor/`:**
- Purpose: TypeScript-only types, translations, small helpers, third-party raw JS.

**`frontend/wailsjs/`:**
- Purpose: **Generated** Go ↔ JS bindings.
- Generated by: `wails dev` / `wails build`. Do not hand-edit.

**`build/`:**
- Purpose: Platform-specific resources copied into the final binary (Info.plist, .desktop, .ico).

**`docs/guide/`:**
- Purpose: VitePress user guide (zh + en); deployed via `.github/workflows/deploy-docs.yml`.

**`.github/workflows/`:**
- Purpose: CI/CD: `build.yml` (multi-OS build), `deploy-docs.yml`, `update-homebrew.yml`, `update-scoop.yml`.

## Key File Locations

**Entry Points:**
- `main.go` — process entry, Wails bootstrap.
- `app.go` — the single Wails-bound façade (`Bind: []interface{}{app}`).
- `frontend/src/main.ts` — Vue app entry.
- `frontend/src/App.vue` — Vue app shell.

**Configuration:**
- `wails.json` (root) — Wails project metadata, build flags.
- `frontend/wails.json` — frontend build config.
- `frontend/package.json` — npm scripts + deps.
- `frontend/tsconfig.json` — TypeScript config.
- `go.mod` — Go 1.23 module.

**Core Logic:**
- `app.go` — façade (4120 lines, 204 methods).
- `backend/session/session.go` — protocol-agnostic interface + baseSession (379 lines).
- `backend/session/manager.go` — protocol dispatcher.
- `backend/k8s/manager.go` — Kubernetes orchestrator.
- `backend/database/provider.go` — SQL DB registry interface.
- `backend/sync/sync_service.go` — cloud sync.

**Per-feature hubs:**
- AI/LLM: `frontend/src/stores/aiStore.ts`, `frontend/src/services/llm.ts`, `frontend/src/services/agent.ts`, `frontend/src/services/terminalAgent.ts`; backend at `app.go:ChatCompletion`/etc.
- Terminal UX: `frontend/src/components/BaseTerminal.vue` (1790 lines), `frontend/src/composables/useTerminal.ts` (731 lines).
- Tab/Panel router: `frontend/src/stores/tabStore.ts`, `frontend/src/stores/panelStore.ts`, `frontend/src/components/WorkspaceContent.vue`, `frontend/src/components/Panel.vue`, `frontend/src/components/PanelGrid.vue`, `frontend/src/components/PanelSplitter.vue`.

**Testing:**
- Backend: `**/*_test.go` (currently in `backend/k8s/`, `backend/session/`, `backend/platform/`, `backend/store/`).
- Frontend: `frontend/src/{stores,services}/**/*.test.ts` — Vitest specs co-located with source.

## Naming Conventions

**Go files:**
- `app*.go` (root) — façade + OS splits.
- `backend/<pkg>/<thing>.go` — package files.
- `backend/<pkg>/<thing>_test.go` — unit tests (no separate `tests/` dir).
- `provider_<db>.go`, `<protocol>_session.go` — protocol/plugin naming.
- `local_session_unix.go` / `local_session_windows.go` — OS split via build tags.
- `app_darwin.go` / `app_notdarwin.go` — empty stubs vs full impl.

**Go identifiers:**
- Types: PascalCase (`SessionManager`, `ConnectionConfig`, `EventEmitter`).
- Functions: PascalCase exported, camelCase unexported; receiver names 1–3 chars (`a *App`, `s *SSHSession`, `m *Manager`).
- Constants: PascalCase for exported (`StatusConnecting`), SCREAMING_SNAKE for tunables (`sshKeepAliveInterval`).
- Errors: package-level `var Err…` (`ErrWrongSyncPassword`); returned errors wrap with `fmt.Errorf("…: %w", err)`.

**TS/Vue files:**
- `App.vue` — single shell.
- `components/<Feature><Role>.vue` — `TerminalTabContent.vue`, `AddRepoDialog.vue`, `SettingsTab.vue`, `TunnelsPanel.vue`, `DBTreePanel.vue`, `BaseTerminal.vue`, `WindowControls.vue`.
- `composables/use<Capability>.ts` — Vue composable naming.
- `stores/<domain>Store.ts` — Pinia naming; tests `*.test.ts`.
- `services/<feature>.ts` — backend-interacting helpers.
- `types/<domain>.ts` — shared type modules.
- `i18n/locales/<lang>.json` — BCP-47-ish keys (`de`, `en`, `es`, `fr`, `ja`, `ko`, `ru`, `zh-CN`, `zh-TW`).

**TS identifiers:**
- Stores: `useXxxStore` (`useTabStore`, `useConnectionStore`).
- Composables: `useXxx` returning `UseXxxReturn`.
- Generated IDs: `${prefix}-${Date.now()}-${random}` (`panel-…`, `conn-…`).

## Where to Add New Code

**New protocol session (e.g. XYZ terminal):**
1. Add `<protocol>_session.go` in `backend/session/`; embed `baseSession`; implement `Session` interface methods.
2. Register the dispatch case in `backend/session/manager.go:Create` (`switch sessionType { case "xyz": s = NewXYZSession(...) }`).
3. Add a case in `App.CreateSession` (`app.go`) for any protocol-specific config wiring (encoding, jump host handling, etc.).
4. Frontend: add a tab type in `frontend/src/types/workspace.ts`; create `<Protocol>TabContent.vue`; mount in `App.vue`'s tab router; extend `frontend/src/components/ConnectionForm.vue`; add a `<Type>Connection` icon/path in `frontend/src/components/Sidebar.vue`.

**New SQL database:**
1. Add `backend/database/provider_<db>.go` implementing `Provider`; call `database.Register("<db>", p)` from `init()`.
2. Update `App.dbProvider` cast handling only if needed — most generic methods work via `database.Provider`.

**New Wails-bound method:**
1. Add `func (a *App) MethodName(...) (...)` to `app.go` (or `app_<os>.go` if OS-specific).
2. Run `wails dev` to regenerate `frontend/wailsjs/`.
3. Import and call from a Pinia store or composable.

**New Pinia store:**
1. Create `frontend/src/stores/<domain>Store.ts` using `defineStore('<domain>', () => { ... })`.
2. Co-locate `<domain>Store.test.ts` next to it.

**New composable:**
1. Create `frontend/src/composables/use<Capability>.ts` returning a stable API.

**New Vue component:**
1. Component file: `frontend/src/components/<Name>.vue` (PascalCase).
2. If tab body: name `<Feature>TabContent.vue`; add to `App.vue` router and `types/workspace.ts` `Tab` union.

**New persisted setting:**
1. Add the field to `backend/store/settings_store.go` (`AppSettings`).
2. Expose via `App.LoadSettings`/`App.SaveSettings` (already in `app.go`).
3. Read/write via `useSettingsStore` (`frontend/src/stores/settingsStore.ts`).

**New LLM-driven Agent tool:**
1. Add the tool definition in `aiStore.ts` (`SYSTEM_RULES` constant) and the handler in `services/agent.ts`.
2. Re-run `wails dev` if any new `App` method was added.

**Shared helpers:**
- Backend: add to the relevant package (`backend/session/`, `backend/k8s/`, …). No central `utils/` package.
- Frontend: add to `frontend/src/utils/` only if stateless and small; otherwise create a new composable or service.

## Special Directories

**`frontend/dist/`:**
- Purpose: Vite build output (HTML/JS/CSS/images).
- Generated: Yes — by `npm --prefix frontend run build` / `wails dev` / `wails build`.
- Committed: No (in `.gitignore`).

**`frontend/wailsjs/`:**
- Purpose: Auto-generated Go ↔ JS bindings.
- Generated: Yes — by `wails dev` / `wails build`.
- Committed: Yes (per repo state) so the frontend type-checks without running `wails dev`.
- Rule: Do not hand-edit. If you change a method signature in `app*.go`, regenerate.

**`frontend/node_modules/`:**
- Purpose: npm dependencies.
- Generated: Yes — by `npm --prefix frontend install`.
- Committed: No (gitignored).

**`build/bin/`:**
- Purpose: Wails-produced binaries (`uniTerm.app`, `.exe`, ELF).
- Generated: Yes — by `wails build`.
- Committed: No (gitignored).

**`build/{darwin,linux,windows}/`:**
- Purpose: Source resources that Wails copies into the bundle at build time (Info.plist, .desktop, .ico).
- Generated: No.
- Committed: Yes.

**`.planning/codebase/`:**
- Purpose: This codebase-map output (ARCHITECTURE.md, STRUCTURE.md, etc.).
- Generated: No (committed by GSD mapper).
- Committed: Yes.

**`.idea/` (project-local):**
- Purpose: JetBrains/GoLand workspace settings.
- Generated: Yes (by IDE).
- Committed: No (in `.gitignore`).

**`docs/guide/` (VitePress):**
- Purpose: User-facing documentation.
- Generated: No (committed source); deployed by `.github/workflows/deploy-docs.yml`.

---

*Structure analysis: 2026-07-28*