# Roadmap

This document tracks the protocols and core features uniTerm already ships, and what we plan to build next. For per-release details, see [CHANGELOG.md](CHANGELOG.md).

Legend: ✅ Shipped, 🚧 Planned

## Platform Support

| Platform | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| Windows | ✅ | initial release | amd64 / arm64 |  |
| macOS | ✅ | initial release | Apple Silicon / Intel |  |
| Linux | ✅ | initial release | TAR.GZ / DEB / RPM, amd64 / arm64 |  |
| Android | ✅ | v1.10.0 | Touch and narrow-viewport adaptation for phones |  |
| Docker Web | ✅ | v1.12.0 | Browser-accessible multi-tenant deployment | [@NatureLProokie](https://github.com/NatureLProokie) |
| HarmonyOS | 🚧 | planned | |  |
| iOS | 🚧 | planned | |  |

## Protocol Support

### Terminal

| Protocol | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| SSH | ✅ | initial release | Remote shell over SSH |  |
| Local | ✅ | v2026.05.23-alpha | Local shell (PowerShell / CMD / Git Bash / bash / zsh) |  |
| Telnet | ✅ | v2026.06.02-alpha | Remote terminal for legacy devices |  |
| Mosh | ✅ | v2026.06.02-alpha | Roaming-friendly terminal over UDP |  |
| Serial | ✅ | v1.1.0 | Serial port terminal |  |
| WSL | ✅ | v1.1.0 | Open installed WSL distributions |  |
| Raw TCP | ✅ | v1.9.0 | Raw TCP console (plain socket byte streaming) |  |

### File Transfer

| Protocol | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| SFTP | ✅ | v2026.05.16-alpha | Dual-pane file manager over SSH |  |
| Zmodem | ✅ | v2026.06.12-alpha | In-terminal file transfer (`rz` / `sz`) |  |
| FTP / FTPS | ✅ | v2026.06.14-alpha | FTP and FTPS file transfer |  |
| SMB | ✅ | v1.2.2 | Windows shared folders and NAS |  |
| WebDAV | ✅ | v1.2.2 | WebDAV file management |  |
| S3 | ✅ | v1.2.2 | Amazon S3 compatible object storage |  |
| SCP | ✅ | v1.9.3 | Fallback for legacy SSH servers without SFTP support |  |
| WSL File | ✅ | v1.9.4 | WSL distribution file browser |  |
| Local File | ✅ | v1.12.4 | Dual-pane browser over the local filesystem |  |

### Remote Desktop

| Protocol | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| RDP (client on Windows) | ✅ | v2026.05.22-alpha | Windows Remote Desktop |  |
| VNC | ✅ | v2026.05.23-alpha | VNC remote control |  |
| SPICE | ✅ | v2026.06.08-alpha | KVM / QEMU VM console |  |
| X11 Forwarding | ✅ | v1.7.0 | Forward remote X applications over SSH |  |
| RDP (client on macOS / Linux) | ✅ | v1.11.0 | Windows Remote Desktop on macOS / Linux |  |

### Database

| Protocol | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| MySQL | ✅ | v2026.05.27-alpha | Also covers MariaDB, TiDB, etc. |  |
| PostgreSQL | ✅ | v2026.05.27-alpha | Also covers CockroachDB, etc. |  |
| rqlite | ✅ | v2026.05.27-alpha | Distributed SQLite with Raft |  |
| Oracle | ✅ | v1.2.0 | Oracle Database | [@yuwei5380](https://github.com/yuwei5380) |
| SQL Server | ✅ | v1.2.0 | Microsoft SQL Server |  |
| Redis | ✅ | v1.2.1 | Key-value store with visual browser |  |
| MongoDB | ✅ | v1.4.1 | Document database |  |
| Elasticsearch | ✅ | v1.9.0 | Distributed search and analytics engine | [@iCarrear](https://github.com/iCarrear) |
| SQLite | ✅ | v1.10.1 | Embedded local database (open by file path) |  |

### Container

| Protocol | Status | Since | Notes | Contributor |
|----------|--------|-------|-------|-------------|
| Kubernetes | ✅ | v1.6.0 | Kubernetes container orchestration |  |
| Docker | ✅ | v1.6.0 | Docker container runtime |  |
| nerdctl (containerd) | ✅ | v1.6.0 | containerd container runtime |  |
| Podman | ✅ | v1.6.0 | Podman container runtime |  |
| WSLC | ✅ | v1.9.0 | Windows WSL2 Container runtime | [@boltomli](https://github.com/boltomli) |

## Core Features

### AI Assistant

| Feature | Status | Since | Notes | Contributor |
|---------|--------|-------|-------|-------------|
| Anthropic Provider | ✅ | v2026.06.13-alpha | Anthropic-compatible Messages API |  |
| OpenAI Provider | ✅ | v1.1.1 | OpenAI-compatible Chat Completions API |  |
| Message Queue | ✅ | v1.4.1 | Queue messages while the agent is running |  |
| NL Database Query | ✅ | v1.4.1 | Natural-language database query generation |  |
| Multi-Panel AI Lock | ✅ | v1.5.0 | Control multiple terminals simultaneously |  |
| Skill Support | ✅ | v1.6.0 | Packaged, reusable workflows the agent can invoke | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |
| Prompt Library | ✅ | v1.6.0 | Manage and trigger reusable AI prompts | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |
| MCP Server | ✅ | v1.10.0 | Expose uniTerm as an MCP server for external AI agents | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |
| MCP Client | ✅ | v1.10.1 | Connect external MCP servers to extend uniTerm's AI | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |
| File Attachments | ✅ | v1.10.1 | Attach files to AI prompts for the agent to read | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |

### Personalization

| Feature | Status | Since | Notes | Contributor |
|---------|--------|-------|-------|-------------|
| UI Themes | ✅ | initial release | Dark / Deep Blue / Light |  |
| Terminal Themes | ✅ | initial release | Built-in terminal color themes |  |
| Internationalization | ✅ | v2026.06.10-alpha | Multi-language UI |  |
| Custom Keybindings | ✅ | v1.1.1 | Rebindable keyboard shortcuts |  |
| Background Image | ✅ | v1.5.2 | Customizable application background |  |

### Productivity

| Feature | Status | Since | Notes | Contributor |
|---------|--------|-------|-------|-------------|
| Workspace | ✅ | v2026.05.13-alpha | Split panes and broadcast input |  |
| Broadcast Input | ✅ | v2026.05.22-alpha | Send keyboard input to all or selected panels |  |
| Cloud Sync | ✅ | v2026.05.24-alpha | Encrypted sync via your own private repo |  |
| Terminal Search | ✅ | v2026.05.27-alpha | In-terminal text search |  |
| Smart Completion | ✅ | v2026.05.29-alpha | Real-time command suggestions |  |
| Server Monitor | ✅ | v2026.05.29-alpha | Real-time server metrics |  |
| Text Highlighting | ✅ | v2026.06.02-alpha | Terminal text highlighting |  |
| SSH Jump Host | ✅ | v2026.06.14-alpha | Route connections through an SSH jump host |  |
| Session Logging | ✅ | v1.4.0 | Log terminal output to file | [@wangxufeng](https://github.com/wangxufeng) |
| Connection Import / Export | ✅ | v1.8.0 | Import and export connection configurations |  |
| Session Recording | ✅ | v1.12.0 | Record a terminal session to asciinema cast v2 and replay it | [@laigcss](https://github.com/laigcss) |

### Network Toolbox

| Tool | Status | Since | Notes | Contributor |
|------|--------|-------|-------|-------------|
| SSH Tunnel Manager | ✅ | v1.4.1 | Local / remote / dynamic port forwarding | [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu) |
| Network Scan | ✅ | v1.11.2 | IP / port scan | [@etanman](https://github.com/etanman) |
| Local Network Services | ✅ | v1.12.0 | Temporary local servers | [@etanman](https://github.com/etanman) |
| Traffic Capture | ✅ | v1.12.4 | Local / remote packet capture | [@etanman](https://github.com/etanman) |
| Firewall Management | ✅ | v1.12.4 | firewalld / ufw over SSH | [@etanman](https://github.com/etanman) |

### Sidebar Panels

| Panel | Status | Since | Notes | Contributor |
|-------|--------|-------|-------|-------------|
| History | ✅ | v2026.05.29-alpha | Searchable terminal command history |  |
| Quick Commands | ✅ | v1.0.1 | Send saved command snippets to the terminal |  |
| Files | ✅ | v1.9.0 | File management following the current connection | [@iCarrear](https://github.com/iCarrear) |
| Monitor | ✅ | v1.9.0 | Real-time resource metrics of the current host | [@iCarrear](https://github.com/iCarrear) |
| Command Send | ✅ | v1.12.3 | Multi-line editor for current terminal / workspace / all tabs |  |
| Notes | ✅ | v1.12.4 | Markdown notes with categories and cloud sync |  |

## Contributing

Have a feature to propose or a planned item you'd like to pick up? Open an issue or PR on [GitHub](https://github.com/ys-ll/uniterm/issues) — we discuss roadmap items there.
