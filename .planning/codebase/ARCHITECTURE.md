# Architecture

**Analysis Date:** 2026-07-28

## System Overview

```text
┌──────────────────────────────────────────────────────────────────────────┐
│                     Vue 3 Frontend (Element Plus + Pinia)                │
│  `frontend/src/App.vue` → `components/*` → `composables/useTerminal*`    │
│         ↕ Pinia stores (`stores/*`)  ↕ Wails-generated JS bindings      │
│              `frontend/wailsjs/go/main/App.{js,d.ts}` (auto-generated)  │
└────────────────────────────────────┬─────────────────────────────────────┘
                                     │ JSON-RPC / Wails runtime events
┌────────────────────────────────────▼─────────────────────────────────────┐
│                       Go Backend (Wails App)                             │
│   `main.go` ── wails.Run ── `app.go` (App struct, ~204 bound methods)    │
│   │                                                                     │
│   ├── `backend/session/`  SessionManager  ── Session interface           │
│   │       (SSH/SFTP/Telnet/Mosh/Local/Serial/FTP/SMB/WebDAV/S3/         │
│   │        RDP/VNC/SPICE/Redis/MongoDB/Database/Monitor)                │
│   ├── `backend/k8s/`     Manager (connections, watch, log stream, exec) │
│   ├── `backend/database/` Provider registry (mysql/pg/oracle/rqlite/    │
│   │                         sqlserver) + engine/executor                 │
│   ├── `backend/sync/`    SyncService (git + keychain + crypto)           │
│   ├── `backend/store/`   JSON-file persistence (conns, AI, settings,    │
│   │                      tunnels, history, skills, commands, recents)    │
│   ├── `backend/platform/` OS-specific font discovery (build tags)       │
│   ├── `backend/log/`     App-wide file logger (panic-captured)          │
│   └── `backend/update/`  GitHub-release in-app updater                  │
└──────────────────────────────────────────────────────────────────────────┘
```

## Component Responsibilities

| Component | Responsibility | File |
|-----------|----------------|------|
| `App` | Wails-bound façade: ~204 exported methods bridging UI ↔ backend subsystems; lifecycle (`startup`/`shutdown`); routes session, k8s, DB, SFTP, RDP/VNC/SPICE, AI, sync calls. | `app.go` |
| `App` (macOS) | Disables press-and-hold accent picker so key repeat works in terminal. | `app_darwin.go` |
| `App` (Windows) | Subclasses main window HWND, defers `EventsEmit` from WndProc to avoid blocking resize/move. | `app_windows.go` |
| `App` (non-mac) | Empty build-tag stub. | `app_notdarwin.go` |
| `App` (non-Windows) | Empty build-tag stub. | `app_notwindows.go` |
| `main` | Entry point; embeds `frontend/dist`; configures Wails window/menu/frameless/title-bar; registers `panic` recovery. | `main.go` |
| `SessionManager` | Owns the live map `sessionID → Session`; `Create` dispatches by `sessionType` string to a protocol constructor; thread-safe. | `backend/session/manager.go` |
| `Session` (interface) | Uniform protocol-agnostic contract (`Connect`/`Disconnect`/`Write`/`Resize`/status callbacks/zmodem). | `backend/session/session.go` |
| `baseSession` | Embeds common fields (id, status, callbacks, output-log writer, pending size, zmodem flag, post-login helpers). | `backend/session/session.go` |
| `TunnelService` | Two kinds of SSH tunnels — per-session jump-host forward and user-defined standalone port forwards; emits `tunnel:state` events. | `backend/session/tunnel_service.go`, `tunnel.go`, `tunnel_forward.go` |
| `k8s.Manager` | Holds kubeconfig-derived `http.Client` per conn, runs watches and log streams; emits `k8s:watch:<id>` / `k8s:log:<id>` events; `DialExec` opens WebSocket to pod exec. | `backend/k8s/manager.go`, `rest.go`, `watch.go`, `logs.go`, `client.go` |
| `database.Provider` | Per-DB DSN, quoting, schema discovery, CRUD/DDL helpers; implementations `Register` themselves via `init()`. | `backend/database/provider.go`, `provider_*.go`, `engine.go`, `executor.go` |
| `store.*` | JSON-file persistence under `os.UserConfigDir()/uniTerm/...`. Separate file per concern. | `backend/store/*` |
| `sync.SyncService` | Encrypts store payload, syncs to a private GitHub/GitLab/Gitee repo via git + OS keychain token. | `backend/sync/sync_service.go`, `git.go`, `crypto.go`, `keychain.go` |
| `platform` | Font enumeration (`GetFontFamilies`) split by OS via build tags. | `backend/platform/fonts*.go` |
| `log` | Append-only file logger under `os.UserConfigDir()/uniTerm/logs/`. | `backend/log/log.go` |
| `update` | Checks GitHub releases for newer versions. | `backend/update/checker.go` |
| `App.vue` | Single-file shell: header, sidebar, tab router, dialogs, background image overlay. | `frontend/src/App.vue` |
| `BaseTerminal.vue` | xterm.js host with search bar, context menu, drag-drop, suggestions overlay, zmodem panel — 1790 lines. | `frontend/src/components/BaseTerminal.vue` |
| `TabContent*` | Per-protocol tab bodies (RDP/VNC/SPICE/DB/Redis/MongoDB/K8s/SFTP/Monitor). | `frontend/src/components/*TabContent.vue` |
| `composables/useTerminal*` | Encapsulate xterm.js setup, theming, input/menu/shortcuts, suggestions, focus, copy, tunnel credentials, update check. | `frontend/src/composables/use*.ts` |
| `stores/*` (Pinia) | Reactive global state per concern: `tabStore`, `panelStore`, `sessionStore`, `connectionStore`, `settingsStore`, `aiStore`, `k8sStore`, `syncStore`, `tunnelStore`, `commandStore`, `quickCommandStore`, `localStateStore`, `skillStore`, `zmodemStore`. | `frontend/src/stores/*.ts` |
| `services/agent.ts` + `terminalAgent.ts` | Multi-turn AI Agent loop: shell tool execution with prompt-line detection, output capture, cancel polling. | `frontend/src/services/agent.ts`, `terminalAgent.ts` |
| `services/llm.ts` | OpenAI/Anthropic-compatible chat client over `App.ChatCompletion` / `App.CancelChatStream`. | `frontend/src/services/llm.ts` |
| `services/k8s*.ts` | REST/CRD/metrics wrappers calling App-level K8s methods. | `frontend/src/services/k8s*.ts` |
| `vendor/spice-html5.js` | Vendored SPICE client (raw JS, untouched). | `frontend/src/vendor/spice-html5.js` |
| `wailsjs/` | **Generated** Go ↔ JS bindings. Do not edit; regenerate with `wails dev`. | `frontend/wailsjs/**` |

## Pattern Overview

**Overall:** Layered façade with plugin-style dispatch — one **Façade** (`App`) exposes every backend capability as a flat set of Wails-bound methods, delegating to **Manager** types that hold per-resource state and call **Provider**/interface implementations per protocol. Frontend is a Vue 3 SPA whose stores consume those methods via generated bindings and receive streamed data via Wails events.

**Key Characteristics:**

- **Manager pattern** for stateful subsystems (`SessionManager`, `k8s.Manager`, `TunnelService`, `database.Provider` registry) — each owns its `map[id]→thing` with RW locks and emits structured events for the UI.
- **Single Session interface** in `backend/session/session.go:139` — all 18 protocol-specific sessions (`ssh_session.go`, `sftp_session.go`, `telnet_session.go`, `mosh_session.go`, `local_session_{unix,windows}.go`, `serial_session.go`, `ftp_session.go`, `smb_session.go`, `webdav_session.go`, `s3_session.go`, `rdp_session.go`, `rdp_session_stub.go`, `vnc_session.go`, `spice_session.go`, `redis_session.go`, `mongodb_session.go`, `database_session.go`, `monitor_session.go`, `k8s_exec_session.go`) embed `baseSession` for shared lifecycle/callback/output-log/zmodem handling.
- **Registry pattern** for databases: each `provider_*.go` calls `database.Register("mysql", …)` from its `init()`; `NewProvider(dbType)` looks it up.
- **Event-driven UI updates** — backend pushes `session:data`, `session:status`, `tunnel:state`, `k8s:watch:<id>`, `k8s:log:<id>`, `store:connections:changed`, `store:tunnels:changed`, `sync:conflict`, `ai:message_start`, `ai:token`, `ai:block_start`, `ai:content_block_stop`, `ai:input_json_delta`, `ai:done`, `rdp:move-resize-end`, etc. via `runtime.EventsEmit`; frontend subscribes with `EventsOn` at module level (e.g. `sessionStore.ts:39-55`).
- **Build-tag OS split** — `app_darwin.go`/`app_notdarwin.go` and `app_windows.go`/`app_notwindows.go`; `local_session_unix.go` (`//go:build !windows`) vs `local_session_windows.go`; `fonts_{darwin,unix,windows,ttf}.go`.
- **Defer-and-callback flow** for output logs (`App.panelLogs`/`sessionToPanel`/`panelAutoTriggered` maps in `app.go:71-79`) — output loggers live at App level so they outlive individual session objects across reconnects.
- **Generated boundary** — `frontend/wailsjs/` is produced by `wails dev` from `App`'s exported methods; only modify `app*.go` to change the API surface.

## Layers

**Façade Layer (Wails-bound):**
- Purpose: Sole entry point for the frontend. Every UI-bound Go function is a method on `App`.
- Location: `app.go` (root), `app_darwin.go`, `app_windows.go`, `app_notdarwin.go`, `app_notwindows.go`.
- Contains: ~204 exported methods grouped by concern (Session, SFTP, RDP, VNC, SPICE, Monitor, DB, Redis, Mongo, K8s, AI/ChatCompletion, Sync, Skills/Commands, Tunnel, Stores, File dialogs, Background).
- Depends on: every backend subsystem.
- Used by: `main.go` (via `Bind: []interface{}{app}`); the frontend via generated bindings.

**Session Subsystem:**
- Purpose: Uniform terminal/file/remote-desktop connections across 18 protocols.
- Location: `backend/session/`.
- Contains: `SessionManager`, `Session` interface, `baseSession`, `*_session.go` per protocol, plus `tunnel.go`, `tunnel_service.go`, `tunnel_forward.go`, `mosh_session.go`, `ssh_auth.go`, `ssh_config.go`, `post_login_expect.go`, `output_log.go`, `zmodem_detect*`.
- Depends on: `golang.org/x/crypto/ssh`, `creack/pty`, `go.bug.st/serial`, and per-protocol client libraries.
- Used by: `App` methods (`CreateSession`, `SessionStart`, `CloseSession`, `SessionWrite`, `SessionResize`, `Sftp*`, `RDP*`, `VNC*`, `SPICE*`, `Redis*`, `Mongo*`, `GetDatabases`/`GetTables`/etc., `Monitor*`).

**K8s Subsystem:**
- Purpose: kubeconfig loading, REST client + watch + log stream + exec WebSocket.
- Location: `backend/k8s/`.
- Contains: `Manager`, `kubeconfig.go`, `client.go`, `rest.go`, `watch.go`, `logs.go`, `server_addr.go`.
- Depends on: `gorilla/websocket`, `k8s.io/client-go` (via `go.mod`).
- Used by: `App` methods (`K8sListContexts`, `K8sConnect`, `K8sDisconnect`, `K8sRequest`, `K8sStartWatch`, `K8sStopWatch`, `K8sStartLogStream`, `K8sStopLogStream`, `K8sExecSession`).

**Database Subsystem:**
- Purpose: SQL/Redis/Mongo-like clients with capability-aware UI.
- Location: `backend/database/`.
- Contains: `provider.go` (interface + registry), `engine.go`, `executor.go`, `schema.go`, `provider_mysql.go`, `provider_postgres.go`, `provider_oracle.go`, `provider_sqlserver.go`, `provider_rqlite.go`.
- Depends on: `database/sql`, per-driver packages.
- Used by: `App` methods (`GetDatabases`, `GetTables`, `GetTableSchema`, `ExecuteQuery`, `ExecuteStatement`, `CreateDatabase`, `DropDatabase`, `CreateTable`, `DropTable`, `DropView`, `TruncateTable`, `DBInsertRow`, `DBUpdateRow`, `DBDeleteRow`, `AddColumn`, `ModifyColumn`, `DropColumn`, `AddIndex`, `DropIndexOp`, `GetDBCapabilities`).

**Store/Sync/AI subsystems:**
- See component table; all flat JSON persistence under `os.UserConfigDir()/uniTerm/`.

**Frontend:**
- Purpose: Vue 3 SPA with terminal rendering and per-feature tabs.
- Location: `frontend/src/`.
- Contains: `App.vue` shell, `main.ts` (Vue/Pinia/ElementPlus bootstrap), `components/`, `composables/`, `stores/`, `services/`, `types/`, `i18n/`, `utils/`, `vendor/`.
- Depends on: `frontend/wailsjs/go/main/App` (generated), `@xterm/xterm`, `pinia`, `element-plus`, `@fontsource-variable/jetbrains-mono`, `vue`.
- Used by: the user (desktop UI in WKWebView/WebView2/WebKitGTK).

## Data Flow

### Primary Request Path — User connects an SSH session

1. `frontend/src/App.vue` sidebar emits `@connect` → `onConnect(config)` (`App.vue`).
2. `onConnect` calls a Pinia store action that ultimately invokes the generated binding `CreateSession(sessionType, config)` (`frontend/wailsjs/go/main/App.js`).
3. `App.CreateSession` (`app.go:976`) logs, calls `sessionManager.Create(sessionType, config)` which dispatches `NewSSHSession(config.ID)` and stores it in `sm.sessions` (`backend/session/manager.go:21`).
4. App-level pending-size and encoding are applied (`app.go:1000-1008`). If `TunnelSSHConnID` is set, `tunnelService.Start` opens a local port forward.
5. Either the synchronous `Connect` goroutine (`launchConnectGoroutine` at `app.go:1201`) or a deferred `SessionStart` (when `DeferConnect`) runs `s.Connect(config)`.
6. `SSHSession.Connect` (`backend/session/ssh_session.go`) opens TCP, applies keepalive, runs `makeSSHAuthMethods` (`ssh_auth.go`), opens channel/shell/pty, sets encoding, launches `readLoop`.
7. `baseSession.emitData` (`backend/session/session.go:215`) calls `onDataCallback` set by App which calls `runtime.EventsEmit(a.ctx, "session:data", {id, data})`.
8. Frontend `sessionStore` (`frontend/src/stores/sessionStore.ts:48-55`) `EventsOn('session:data', …)` appends the chunk to `sessionState.sessions.get(id).data` and bumps `seq`.
9. `useTerminal` (`frontend/src/composables/useTerminal.ts`) is the `onSessionData` consumer; it calls `terminal.write(data)` into xterm.js.
10. User keystrokes flow back: `BaseTerminal.vue` onData → `SessionWrite(sessionID, data)` → `App.SessionWrite` (`app.go:1288`) → `sessionManager.Get(sessionID).Write(data)`.

### Secondary Flow — AI Agent tool call

1. User selects text in terminal, context menu → "Ask AI" → `useTerminalMenu.askAI` (`composables/useTerminalMenu.ts:50`).
2. `AISidebar.vue` invokes `useAiStore().send(text)` (`frontend/src/stores/aiStore.ts`).
3. `aiStore.send` calls `services/agent.ts` / `services/terminalAgent.ts` which uses `App.ChatCompletion` (`app.go` ~line 1850).
4. App method makes an HTTP SSE call to the LLM provider, converts OpenAI/Anthropic stream events into Wails events `ai:message_start`, `ai:block_start`, `ai:token`, `ai:content_block_stop`, `ai:input_json_delta`, `ai:done` (`app.go:2000-2200`).
5. AISidebar consumes events and renders streaming tokens. Tool calls (e.g. `execute_command`) round-trip back to terminal via `App.SessionWrite`.

### Secondary Flow — Kubernetes watch

1. `K8sConnect` → `k8s.Manager.ConnectWith` (`backend/k8s/manager.go:88`) builds an `http.Client` + base URL + bearer token from kubeconfig.
2. `K8sStartWatch(path)` (`backend/k8s/manager.go:167`) registers a `watchHandle`, starts `startWatchStream`, emits `k8s:watch:<watchID>` and `k8s:watch-end:<watchID>`.
3. Frontend `k8sStore` (`frontend/src/stores/k8sStore.ts`) subscribed via `EventsOn` updates reactive resources list.

**State Management:**
- Frontend global state lives in Pinia stores; many stores hold **module-level `reactive(...)`** state plus module-level `EventsOn` subscriptions (`sessionStore.ts:32-55`, `aiStore.ts`) so multiple consumers share the same event-driven cache.
- Backend session state lives in `SessionManager.sessions map[string]Session` (RW mutex); k8s state in `Manager.conns/watches/logs`; tunnels in `TunnelService.tunnels/userTunnels`.
- Persistence is per-concern JSON files under `os.UserConfigDir()/uniTerm/`.

## Key Abstractions

**`Session` interface (`backend/session/session.go:139`):**
- Purpose: Uniform protocol-agnostic terminal contract.
- Methods: `ID()`, `Type()`, `Title()`, `Status()`, `Connect()`, `Disconnect()`, `IsConnected()`, `Resize()`, `SetPendingSize()`, `Write()`, `SetOnDataCallback()`, `SetOnBinaryCallback()`, `SetOnStatusChangeCallback()`, `SetZmodemMode()`, `IsZmodemMode()`.
- Examples: every `*_session.go` in `backend/session/`.
- Pattern: Embed `baseSession` (struct embedding) — concrete sessions extend with protocol-specific fields (e.g. `SSHSession.client`, `LocalSession.pty`, `RDPSession.peer`, `MonitorSession`).

**`database.Provider` interface (`backend/database/provider.go:27`):**
- Purpose: Per-DB capability envelope.
- Methods: `DSN`, `DriverName`, `Quote`, `GetDatabases`, `GetTables`, `GetTableSchema`, `DefaultTableQuery`, `InsertRow`, `UpdateRow`, `DeleteRow`, `CreateDatabase`, `DropDatabase`, `CreateTable`, `DropTable`, `DropView`, `TruncateTable`, `AddColumn`, `ModifyColumn`, `DropColumn`, `AddIndex`, `DropIndex`, `GetCapabilities`, `PrepareExec`.
- Examples: `provider_mysql.go`, `provider_postgres.go`, `provider_oracle.go`, `provider_sqlserver.go`, `provider_rqlite.go`.
- Pattern: Registry — `Register(dbType, p)` in `init()`; looked up by `NewProvider(dbType)`.

**`k8s.EventEmitter` (`backend/k8s/manager.go:17`):**
- Purpose: Decouple k8s manager from Wails runtime.
- Pattern: Functional interface — App installs the emitter (`SetEventEmitter`) at startup; manager calls `emit(name, payload)` for `k8s:watch:<id>` / `k8s:log:<id>` / `k8s:watch-end:<id>` / `k8s:log-end:<id>`.

**`store.PasswordStore` interface (`backend/store/connection_store.go:15`):**
- Purpose: Pluggable external secret store; `nil` means legacy plaintext JSON.
- Pattern: Optional dependency — `ConnectionStore.SetPasswordStore(ps)` migrates passwords to the keychain via `sync.SyncService.PasswordStore()`.

**Frontend — `panelStore` + `tabStore` + `WorkspaceContent.vue`:**
- Purpose: Multi-panel, split-layout workspace model. A `Tab` (`frontend/src/types/workspace.ts:31`) is a discriminated union of `TerminalTab | SettingsTab | WorkspaceTab | SFTPTab | RDPTab | VNCTab | SPICETab | DBTab | MonitorTab | StartTab | K8sTab`. A `WorkspaceTab` holds a `PanelLayout` (a recursive `LayoutNode` of leaf panels and split nodes).
- Pattern: Single source of truth for layout; components are drop targets using HTML5 drag/drop (`TerminalTabContent.vue:66-162`).

**Frontend — `useTerminal` (`frontend/src/composables/useTerminal.ts`):**
- Purpose: Encapsulate xterm.js lifecycle, theming, fitAddon, searchAddon, webLinksAddon, and the bind to `SessionWrite`/`SessionResize`/`session:data`/`session:status` events.
- Pattern: Vue composable returning a stable API used by `BaseTerminal.vue` and downstream overlays.

## Entry Points

**`main` (`main.go`):**
- Location: `/Users/coderstory/CodeSource/uniterm/main.go`
- Triggers: Wails app launch.
- Responsibilities: `log.Init`, panic recovery, App construction, macOS App+Edit menu setup, Linux multi-monitor max-size workaround, frameless/title-bar configuration from `localStateStore`, `wails.Run(&options.App{...Bind: []interface{}{app}})`.

**Wails-bound surface (`App`):**
- Location: `/Users/coderstory/CodeSource/uniterm/app.go` (and `app_darwin.go`/`app_windows.go`/…).
- Triggers: any frontend call via `frontend/wailsjs/go/main/App.js`.
- Responsibilities: every UI-actionable behavior. Key entry methods: `CreateSession` (`app.go:976`), `SessionStart` (`app.go:1254`), `SessionWrite`/`SessionResize`/`CloseSession`, `SaveConnections`/`LoadConnections`, `SaveTunnels`/`LoadTunnels`/`StartTunnel`/`StopTunnel`, `SaveSettings`/`LoadSettings`, `SaveAIConfig`/`LoadAIConfig`, `ChatCompletion`/`CancelChatStream`, `Sync*`, `K8s*`, `Redis*`, `Mongo*`, `GetDatabases`/`GetTables`/`ExecuteQuery`/`ExecuteStatement`/`DB*`/`AddColumn`/`DropIndexOp`/etc., `Sftp*`, `RDP*`, `VNC*` (via `VncProxy`), `SPICE*` (via `SpiceProxy`), `Monitor*`.

**Frontend entry (`main.ts`):**
- Location: `/Users/coderstory/CodeSource/uniterm/frontend/src/main.ts`.
- Triggers: WebView loads `frontend/dist`.
- Responsibilities: create Vue app, install Pinia + Element Plus, await `settingsStore.init()`, mount `#app`, install global contextmenu dispatcher that routes to `useTextCopyMenu` (input/textarea) or hides it (everything else).

**`App.vue`:**
- Location: `/Users/coderstory/CodeSource/uniterm/frontend/src/App.vue`.
- Triggers: mount.
- Responsibilities: `el-config-provider` + background overlay + `AppHeader` + `Sidebar` + tab router that mounts the right `*TabContent` based on `activeTab.type`.

## Architectural Constraints

- **Threading:** Single process. Backend: long-lived I/O runs in goroutines (`SSHSession.readLoop`, `k8s.Manager.startWatchStream`, `LocalSession` pty reader, `TunnelService` per-tunnel accept loop, `OutputLogger` writer). Frontend: Wails runtime events delivered on main thread; Pinia stores are synchronous. `chatCancel` is guarded by `chatCancelMu` (`app.go:61-66`).
- **Global state:** `App` itself is a long-lived singleton (built once in `main.go:45`). It owns the `*session.SessionManager`, `*k8s.Manager`, all `*store.*Store`s, and several maps: `panelLogs`, `sessionToPanel`, `panelAutoTriggered` (gated by `panelLogMu`), `customLogDir` (gated by `customLogDirMu`), `chatCancel` (gated by `chatCancelMu`), `moveResizeCh` (Windows resize event deferral channel). `SessionManager.sessions`, `k8s.Manager.{conns,watches,logs}`, `TunnelService.{tunnels,userTunnels,states}`, and `database.providers` are also module-level singletons with their own mutexes.
- **Circular imports:** `backend/session/session.go` (defines `ConnectionConfig`) is imported by `backend/store/connection_store.go`; `app.go` imports both — no cycle observed at the type level. `frontend/wailsjs/go/main/App.js` is generated.
- **Wails binding regeneration:** Changing `app*.go` signatures forces a fresh `wails dev` to regenerate `frontend/wailsjs/` — the frontend's TypeScript types would otherwise go stale.
- **PTY sizing timing:** Recent fix introduced deferred `Connect` (`DeferConnect` field + `SetPendingSize`) so the frontend can measure xterm size BEFORE the backend PTY is created; this prevents Claude Code table wrapping at 80 cols. Any new protocol session that uses a PTY must call `getInitialSize` in its `Connect`.
- **OS split:** Code paths that touch OS-specific APIs (window HWND subclassing, fonts, local shell, serial) are split via build tags; touching one half means verifying the other is not also affected.
- **macOS menu:** `Menu: appMenu` is only set for `darwin` to avoid an empty GtkMenuBar appearing as a white sliver underneath the frameless window. Linux GTK builds deliberately leave `Menu` nil.

## Anti-Patterns

### Adding protocol-specific business logic in `App` methods

**What happens:** New `App` method does per-protocol work inline (e.g. opening an SSH connection from `app.go`) instead of routing through `SessionManager`/`k8s.Manager`/`database.Provider`.
**Why it's wrong:** Spreads protocol knowledge across the façade; duplicates the work `Session.Connect` already encapsulates; makes testing and parallel development harder.
**Do this instead:** Add a method to the relevant manager/session, then expose a thin `App` wrapper that resolves the session and forwards the call. See `app.go:1500-1570` for the canonical `getMonitorSession` pattern.

### Mutating per-call arrays returned from stores

**What happens:** Direct mutation of `connectionsStore.connections[i]` then calling `save()` (`frontend/src/stores/connectionStore.ts`).
**Why it's wrong:** Skips the deep-copy `ConnectionStore.Save` does (`backend/store/connection_store.go:54-55`); can leak plaintext passwords back into the JSON when `passwordStore` is set.
**Do this instead:** Use the store's `add`/`update` helpers (`connectionStore.ts:47-72`) which always go through the save path. Backend's `Save` deep-copies first.

### Hand-editing `frontend/wailsjs/`

**What happens:** Tweaking `frontend/wailsjs/go/main/App.js` or `App.d.ts` to "fix" a type mismatch.
**Why it's wrong:** Next `wails dev` regenerates and overwrites; the edit is lost.
**Do this instead:** Fix the `App` method signature in `app.go` (or `app_*.go`) and regenerate.

### Constructing a new xterm `Terminal` per re-render

**What happens:** Each `KeepAlive` switch or tab change calls `new Terminal(...)` and discards the old one.
**Why it's wrong:** xterm.js has its own DOM/scrollback state — recreating it loses cursor position and selection, and resubscribes `EventsOn('session:data', …)` causing duplicate handlers (`sessionStore.ts:48-55` is module-level precisely to avoid this).
**Do this instead:** Mount once via `useTerminal`; reuse the same instance across keep-alive cycles. Use `terminal.reset()/clear()` for state changes.

## Error Handling

**Strategy:** Layered. Each subsystem returns Go errors; `App` methods either surface them to the UI or convert to a typed error string the frontend renders (e.g. `AI_REQUEST_TIMEOUT` at `app.go:2025`). SFTP and S3 batch transfers accumulate per-task errors in the task record so one failure does not abort the rest.

**Patterns:**
- All `App` methods that touch a `*Store` first nil-check the store (`app.go:278-280`, `app.go:300-302`) and return `"<thing> not initialized"` rather than panic.
- `SessionManager.Get` returns `(Session, bool)`; `App` callers translate the `!ok` case to `"session not found: <id>"` (`app.go:1500`).
- K8s manager methods check `conns[connID]` under lock and return descriptive errors (`backend/k8s/manager.go:107`, `162`).
- Stream cancellation uses a `context.CancelFunc` guarded by a mutex (`chatCancel`/`chatCancelMu` in `app.go:60-66`); `App.CancelChatStream` (`app.go`) is the externally callable equivalent.
- Post-login expect steps (`backend/session/post_login_expect.go`) and SSH post-login scripts (`baseSession.RunPostLoginScript`, `backend/session/session.go:350`) use idle-detection + context cancel; never panic the caller.
- Frontend wraps backend calls with try/catch in stores (`connectionStore.ts:23-34`) and surfaces via console; AI Agent abort uses `App.CancelChatStream` and `terminalAgent.ts` `shouldCancel` poll.

## Cross-Cutting Concerns

**Logging:** `backend/log` — file appender under `os.UserConfigDir()/uniTerm/logs/`; global `Writef` guarded by mutex; `main.go:27-35` panic-recover captures top-level crashes; `App.FrontendLog` (`app.go` ~line 1770) lets the WebView send logs back to the same file.
**Validation:** Per-protocol config in `Session.Connect` / `k8s.Manager.ConnectWith`; DSN construction in `database.BuildDSN`; SQL identifier quoting per `database.Provider.Quote`; `app.go` validates `SerialStopBits`/`SerialParity` enums before forwarding.
**Authentication:** SSH key/password auth in `backend/session/ssh_auth.go` (`makeSSHAuthMethods`); AI provider keys via `sync.Keychain` (`backend/sync/keychain.go`) and `store.PasswordStore` interface; cloud-sync token persisted to OS keychain.
**Secret handling:** Connections: plaintext in `connections.json` (legacy) → keychain via `sync.SyncService.PasswordStore()` (`backend/sync/sync_service.go`) wired in `app.go:180-185`. API keys: same path. RDP/VNC passwords stay in JSON.
**Window state:** `localStateStore` (`backend/store/local_state_store.go`) persists `WindowX/Y/Width/Height/Maximised/SystemTitleBar`; restored in `app.go:209-223`, saved in `app.go:228-262` and via Windows WndProc deferral in `app.go:111-120`.
**i18n:** `frontend/src/i18n/` with `locales/{de,en,es,fr,ja,ko,ru,zh-CN,zh-TW}.json`.

---

*Architecture analysis: 2026-07-28*
