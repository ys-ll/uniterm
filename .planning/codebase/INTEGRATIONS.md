# External Integrations

**Analysis Date:** 2026-07-28

## APIs & External Services

**AI / LLM Providers (user-configured, accessed via backend proxy):**
- Anthropic Messages API — `app.chatCompletionAnthropic` in `app.go`. Headers: `x-api-key`, `anthropic-version: 2023-06-01`, `anthropic-beta: prompt-caching-2024-07-31`. Endpoint: `<baseURL>/v1/messages` (SSE streaming).
- OpenAI Chat Completions — `app.chatCompletionOpenAI` in `app.go`. Endpoint: `<baseURL>/chat/completions`. Auth: `Authorization: Bearer <apiKey>`. Tool definitions are translated from Anthropic shape (`anthropicToolToOpenAI`).
- OpenAI Responses API — `app.chatCompletionResponses` in `app.go`. Endpoint: `<baseURL>/responses`. Translation helper `anthropicToolToResponses`.
- OpenAI-compatible `/v1/models` — `app.FetchModels` in `app.go` lists available models (used by the settings UI to populate model dropdowns).

The protocol is chosen per `AIModelConfig.Protocol` (`'anthropic' | 'openai' | 'responses'`, see `frontend/src/types/settings.ts`). `baseURL` is fully user-configurable, so any Anthropic-compatible or OpenAI-compatible endpoint (self-hosted, proxy, third-party) works.

**Update Channel:**
- GitHub Releases — `backend/update/checker.go` queries `https://api.github.com/repos/ys-ll/uniterm/releases/latest` when `source == "github"` (default). User-Agent header: `uniTerm`.
- Gitee Releases (mirror) — same file: `https://gitee.com/api/v5/repos/ys-l/uniterm/releases/latest` when `source == "gitee"`. Release page URL: `https://gitee.com/ys-l/uniterm/releases/latest`.

**Cloud Sync (encrypted Git repo as storage):**
- Generic HTTPS Git over `go-git` (`backend/sync/git.go`). The repo URL is user-supplied (`SyncConfig.RepoURL`) and is expected to be a private repo on GitHub, GitLab, Gitee, or any HTTPS-reachable Git host. Auth is HTTP Basic with username + Personal Access Token (PAT) read from the OS keychain.
- A sync repo mirror is published under `ys-l/uniterm` on Gitee; the script `scripts/sync-release-to-gitee.sh` uses `gh` + `curl` against `https://gitee.com/api/v5` (requires `GITEE_TOKEN` env var).

## Data Storage

**User-Configured Remote Databases (per `backend/database/provider_*.go`):**
- MySQL — `github.com/go-sql-driver/mysql`. Connection via `Provider.DSN`.
- PostgreSQL — `github.com/lib/pq` (`_ "github.com/lib/pq"`).
- Microsoft SQL Server — `github.com/microsoft/go-mssqldb`.
- Oracle — `github.com/sijms/go-ora/v2`.
- rqlite — `github.com/rqlite/gorqlite/stdlib` (HTTP-based distributed SQLite).
- MongoDB — `github.com/mongodb/mongo-driver` (used by `backend/session/mongodb_session.go`, not the SQL provider layer).
- Redis — `github.com/redis/go-redis/v9` (`backend/session/redis_session.go`).

All DSNs are constructed in the Go backend from user-entered credentials; nothing is baked in.

**User-Configured Remote Endpoints (per session):**
- SSH / Telnet / Mosh / Local / Serial — `backend/session/*_session.go`
- FTP — `github.com/jlaffaye/ftp`
- SFTP — `github.com/pkg/sftp` over SSH
- SMB — `github.com/cloudsoda/go-smb2`
- WebDAV — `github.com/studio-b12/gowebdav`
- S3 — `github.com/ys-ll/simples3` (forked via `replace`)
- RDP — `backend/session/rdp_session.go` (uses Windows ConPTY / native APIs on Windows; stub on other platforms in `rdp_session_stub.go`)
- VNC — `backend/session/vnc_proxy.go` bridges WebSocket ↔ TCP for the `@novnc/novnc` frontend client (`VNCTabContent.vue`)
- SPICE — `backend/session/spice_proxy.go` bridges WebSocket ↔ TCP for `spice-html5-bower` (`SPICETabContent.vue`)
- Kubernetes — `backend/k8s/` parses a kubeconfig (`gopkg.in/yaml.v3`) and talks to apiserver over HTTPS via `http.Client`, with optional `DialContext` injection to route through an SSH tunnel (`BuildClientWithDial`). Exec via WebSocket subprotocol `v4.channel.k8s.io` (`manager.go:321`).

**Local File Storage:**
- All persistence is local JSON under `os.UserConfigDir()/uniTerm/`. No remote database, no telemetry upload, no third-party storage service.
- Files: `connections.json`, `settings.json`, `ai-sessions.json`, `skills.json`, `commands.json`, `quick-commands.json`, `tunnels.json`, `local-state.json`, `recent.json`, `terminal-history.json`, `sync-config.json`.
- Sync repo (when configured) is cloned to `<configDir>/uniTerm/sync-repo/`.

**File Storage:**
- Local filesystem only (per-connection SFTP / FTP / SMB / WebDAV / S3 / local downloads / ZMODEM transfers go through the active session driver).

**Caching:**
- None. Update check uses a 6h disk cache (`backend/update/checker.go` `loadCache`), keyed by source.

## Authentication & Identity

**LLM API Keys:**
- Stored in the OS keychain via `backend/sync/keychain.go` under service `uniTerm`. The settings store migrates plaintext keys from `AppSettings.AI.Models[*].APIKey` into the keychain on save (`backend/store/settings_store.go`, `SetModelAPIKey`). Per-model, keyed by model ID.

**Connection Passwords / SSH Passphrases:**
- Migrated from JSON into the same keychain via `connectionStore.SetPasswordStore(syncSvc.PasswordStore())` (`app.go`). Same `PasswordStore` is used for both connections and AI model keys.

**Sync Encryption Key:**
- PBKDF2-derived (600000 iterations, 32-byte key, 16-byte salt) and stored in the OS keychain (`backend/sync/keychain.go` `StoreEncryptionKey` / `GetEncryptionKey`).

**Sync Git Token:**
- Stored in the OS keychain (`SetGitToken` / `GetGitToken` in `backend/sync/keychain.go`), passed to `go-git` as HTTP basic auth.

**OS Keychain Backends (via `github.com/zalando/go-keyring`):**
- macOS: Keychain
- Linux: Secret Service (GNOME Keyring / KWallet)
- Windows: wincred (`github.com/danieljoos/wincred` as indirect dep)

**SSH:**
- `pkg/sftp` + `golang.org/x/crypto/ssh` with password, keyboard-interactive, public-key, and SSH agent auth (`backend/session/ssh_auth.go`, `github.com/xanzy/ssh-agent`). `knownhosts` verification via `github.com/skeema/knownhosts`.

## Monitoring & Observability

**Error Tracking:**
- None. No Sentry / Crashlytics / Bugsnag integration. The only telemetry is the in-app panic handler in `main.go`, which writes to the file logger.

**Logs:**
- Local file logger (`backend/log/log.go`) writing to `<configDir>/uniTerm/uniTerm.log` via `log.Writef`. Initialised at `main()` and on every `App.startup` so log writes work both inside and outside Wails startup.
- Frontend has no console.log-based log shipping; debug output stays in the browser devtools.

**Session Output Logs:**
- Per-panel rolling file logs (`backend/session/output_log.go`), enabled on demand or via `LogOnConnect` setting. Configurable directory via `SetDefaultSessionLogDir`.

## CI/CD & Deployment

**Hosting:**
- No backend service. Distribution is via GitHub Releases at `https://github.com/ys-ll/uniterm/releases` and the Gitee mirror at `https://gitee.com/ys-l/uniterm/releases`.

**CI Pipeline:**
- None detected in repo (no `.github/workflows/`, no `.gitlab-ci.yml`, no `.gitea/workflows/`). Releases are published manually or via local `wails build` + `gh release create`; the `scripts/sync-release-to-gitee.sh` script then mirrors release assets to Gitee.

## Environment Configuration

**Required env vars:**
- None required at runtime. The app boots from local JSON stores and the OS keychain.

**Build-time env vars:**
- `VITE_VERSION` — injected into `import.meta.env.VITE_VERSION` via Vite define (`frontend/vite.config.ts`). Defaults to `'dev'`. Wails passes the version here automatically.

**Dev tooling env vars (scripts only):**
- `GITEE_TOKEN` — required by `scripts/sync-release-to-gitee.sh` to upload mirrored release assets.
- `GH_REPO`, `GITEE_REPO`, `GITEE_API`, `TARGET_COMMITISH` — optional overrides in the same script (defaults `ys-ll/uniterm`, `ys-l/uniterm`, `https://gitee.com/api/v5`, `main`).

**Secrets location:**
- OS keychain (service `uniTerm`) for: AI model API keys, connection passwords, sync encryption key, sync Git token. Plaintext secrets are never written to disk; `backend/store/settings_store.go` strips `APIKey` from JSON before persisting.

## Webhooks & Callbacks

**Incoming:**
- None. The desktop app does not expose an HTTP server. VNC/SPICE proxies (`vnc_proxy.go`, `spice_proxy.go`) bind a random local port to bridge the frontend WebSocket client to the remote TCP server, but these are loopback-only and not externally addressable.

**Outgoing:**
- SSH/SFTP, FTP, SMB, WebDAV, S3, RDP, VNC, SPICE, MongoDB, Redis, MySQL/Postgres/SQLServer/Oracle/rqlite, Kubernetes — all connections are outbound from the desktop client to user-specified remote hosts.
- LLM HTTP(S) calls to user-configured `baseURL` (Anthropic or OpenAI-compatible).
- Git HTTPS push/pull to user-configured `SyncConfig.RepoURL`.
- GitHub/Gitee release metadata GET on demand (`backend/update/checker.go`).

---

*Integration audit: 2026-07-28*