<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>A modern cross-platform terminal emulator with a built-in autonomous AI Agent — capable of independently planning and executing multi-turn shell commands, like Claude Code for your terminal.</p>
  <p><a href="https://uniterm.net">🌐 Homepage</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a></p>
</div>

[简体中文](README_zh-CN.md)

[![GitHub release](https://img.shields.io/github/v/release/ys-ll/uniterm)](https://github.com/ys-ll/uniterm/releases)
[![GitHub release date](https://img.shields.io/github/release-date/ys-ll/uniterm)](https://github.com/ys-ll/uniterm/releases/latest)
[![GitHub downloads](https://img.shields.io/github/downloads/ys-ll/uniterm/total)](https://github.com/ys-ll/uniterm/releases)
[![GitHub latest release downloads](https://img.shields.io/github/downloads/ys-ll/uniterm/latest/total)](https://github.com/ys-ll/uniterm/releases/latest)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue)](https://github.com/ys-ll/uniterm)
[![License](https://img.shields.io/badge/license-Apache%202.0-green)](LICENSE)
[![Commit activity](https://img.shields.io/github/commit-activity/t/ys-ll/uniterm)](https://github.com/ys-ll/uniterm/commits)
[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm)](https://github.com/ys-ll/uniterm)

## Table of Contents

- [Features](#features)
- [Screenshots](#screenshots)
- [Download](#download)
- [Quick Workflows](#quick-workflows)
- [Tech Stack](#tech-stack)
- [Build from Source](#build-from-source)
- [Project Structure](#project-structure)
- [Star this Project](#star-this-project)
- [Feedback & Contributing](#feedback--contributing)
- [License](#license)

## Features

### AI Assistant

Autonomous AI Agent that works like Claude Code — independently plans and executes multi-turn shell commands directly in your terminal.

- **Autonomous Multi-Turn Execution** — The AI Agent can plan, execute, observe results, and iterate across multiple rounds of shell commands without manual intervention.
- **LLM Integration** — Sidebar chat with Anthropic/OpenAI-compatible API, supporting Claude, GPT and other compliant models.
- **Flexible Execution Modes** — Bypass, dangerous only, dangerous + write, or confirm all — you control how much oversight the AI Agent needs.
- **Persistent Conversations** — Chat history is saved per session, so conversations survive app restarts.
- **Terminal Integration** — AI commands execute directly in the active terminal tab, with optional pinning to a specific tab or following your active one. Collaborate side-by-side in split panes, each with its own terminal context.
- **Smart Completion** — While typing in SSH terminals, get real-time suggestions from your command history and AI-powered command rewrites.

### Full-Featured Terminal

Local terminal, SSH / Telnet / Mosh, file transfer (SFTP / FTP), RDP / VNC / SPICE, database, SSH tunnel, server monitor — covering all remote access needs.

- **Remote Terminal** — Supports SSH / Telnet / Mosh. Connect via password or private key authentication; SSH for general remote access, Telnet for legacy/embedded devices, and Mosh for high-latency mobile networks.
- **Local Terminal** — Supports PowerShell / CMD / Git Bash / WSL. Local shells share the same font, color, and behavior settings as SSH sessions.
- **Serial Terminal** — Scan available serial ports and connect with configurable baud rate, data bits, stop bits, and parity. Supports local echo and CR→CRLF normalization.
- **File Transfer** — Supports SFTP / FTP / FTPS / Zmodem. Dual-pane browser for local and remote files; Zmodem (`rz`/`sz`) supported in SSH terminals.
- **Remote Desktop** — Supports RDP / VNC / SPICE. Connect to Windows Remote Desktop, VNC, and SPICE.
- **Database Client** — Connect to MySQL, PostgreSQL, and rqlite databases. Execute SQL queries, browse table structures, and edit data rows inline — all from a unified interface.
- **SSH Tunnel** — Port forwarding. Any connection can use an existing SSH connection as a jump host. Auto-assigns local port, tunnels TCP through SSH to the target. Supports all TCP protocol connection types.
- **Server Monitor** — Real-time monitoring for connected servers. View performance metrics (CPU, memory, disk, network), process list with detail panel, listening ports, disk usage with mountpoint info, and network interfaces with bond/bridge detection.

| Category | Protocol | Description |
|----------|----------|-------------|
| Terminal | SSH | Remote server shell management |
| Terminal | Telnet | Remote terminal for legacy devices and embedded systems |
| Terminal | Mosh | Server connections over high-latency or intermittent networks |
| Terminal | Serial | Serial port terminal with configurable baud rate and other parameters |
| Terminal | Local | PowerShell, CMD, Git Bash, and other local shells |
| Terminal | WSL | Open installed WSL distributions via local terminal |
| File Transfer | SFTP | Server file management and transfer |
| File Transfer | FTP / FTPS | Website hosting, NAS file transfer |
| File Transfer | Zmodem | In-terminal file transfer via rz/sz commands |
| Remote Desktop | RDP | Windows server remote desktop management (Windows only) |
| Remote Desktop | VNC | Linux server remote control |
| Remote Desktop | SPICE | KVM/QEMU VM management |
| Database | MySQL | MySQL protocol: MySQL, MariaDB, TiDB, and more |
| Database | PostgreSQL | PostgreSQL protocol: PostgreSQL, CockroachDB, and more |
| Database | rqlite | Lightweight distributed DB built on SQLite with Raft consensus |
| Monitoring | Monitor | SSH-based real-time CPU, memory, disk monitoring |

### Customization

Connection management, workspace splits, cloud sync, themes — your terminal, your way.

- **Connection Manager** — Save, search, edit, group, and duplicate server connections. Drag-and-drop organization, multi-select or range-select for batch connect, batch delete, and more.
- **Workspace & Split Panes** — Merge terminal tabs into a workspace with horizontal or vertical splits. Drag panel edges or title bars to resize and rearrange freely.
- **Cloud Sync** — Build your own private sync repository via GitHub, GitLab, or Gitee. All configurations (connections, AI model keys, app settings) are encrypted with AES-256-GCM before syncing. Supports automatic sync, conflict resolution, master password changes, and repository management.
- **Themes** — Dark, Deep Blue, and Light themes with automatic system theme detection.
- **Internationalization** — 9-language UI: zh-CN, zh-TW, en, ja, ko, de, es, fr, ru.
- **Cross-Platform** — Built on Wails v2, runs natively on Windows, macOS, and Linux.

## Screenshots

<p align="center">
  <img src="docs/imgs/screenshot-ssh-ai.png" alt="SSH Terminal with AI" width="45%" />
  <img src="docs/imgs/screenshot-sftp.png" alt="SFTP File Manager" width="45%" />
</p>
<p align="center">
  <img src="docs/imgs/screenshot-ai-config.png" alt="AI Model Config" width="45%" />
</p>

## Download

Get the latest pre-built binaries from [GitHub Releases](https://github.com/ys-ll/uniterm/releases):

- **Windows**: installer `uniterm-windows-amd64-installer-*.exe`, or portable `uniterm-windows-amd64-portable-*.zip`
- **macOS**: Download `uniterm-darwin-universal-*.dmg`
- **Linux**: Download `uniterm-linux-amd64-*.tar.gz`

### Runtime Dependencies

- **Windows**: WebView2 runtime (included in Windows 10+; older versions need a one-time install)
- **macOS**: No extra dependencies (uses the system WebKit)
- **Linux**: `libgtk-3-0` and `libwebkit2gtk-4.1-0` (preinstalled on most desktop distros)

## Quick Workflows

### SSH Connection

1. Click **New Connection** in the Connection Manager
2. Fill in host, port, and authentication (password or private key)
3. Click **Connect** to open an SSH terminal session

### AI Assistant

1. Go to Settings and configure your **AI provider** (API endpoint, model, and key)
2. Open a terminal tab (SSH or local)
3. Open the AI sidebar chat — type your task, and the AI Agent executes commands directly in your terminal

### SFTP File Transfer

1. In the Connection Manager, **right-click** an SSH connection
2. Select **Connect SFTP**
3. Browse, upload, download, and drag-and-drop files in the dual-pane file manager

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Desktop Framework | Wails v2 |
| Backend | Go |
| Frontend | Vue 3 + Pinia + Element Plus |
| Terminal | xterm.js |
| AI Protocol | Anthropic Messages API / OpenAI Chat Completions API |

## Build from Source

Requires [Go](https://go.dev/dl/) 1.23+, [Node.js](https://nodejs.org/) 20+, and [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2. Additionally, macOS needs Xcode Command Line Tools, and Linux needs `libgtk-3-dev` and `libwebkit2gtk-4.1-dev`.

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails dev                   # Development
wails build                 # Production build
```

## Project Structure

```
uniTerm/
├── main.go                       # Entry point
├── app.go                        # Wails bindings, LLM API proxy, SFTP API
├── backend/
│   ├── session/                  # SSH/SFTP/database session management
│   ├── database/                 # SQL execution, schema introspection, DSN builders
│   ├── store/                    # Persistent config (connections, AI, settings)
│   └── log/                      # File-based logging
├── frontend/
│   └── src/
│       ├── components/           # Vue components
│       ├── composables/          # Terminal composables
│       ├── stores/               # Pinia stores
│       ├── services/             # AI agent loop, LLM client
│       ├── i18n/                 # Translations
│       └── types/                # TypeScript type definitions
└── wails.json
```

## Star this Project

If uniTerm is helpful to you, please consider giving it a ⭐ Star — it's the best encouragement for the project and helps more people discover it.

<a href="https://star-history.com/#ys-ll/uniterm&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=ys-ll/uniterm&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=ys-ll/uniterm&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=ys-ll/uniterm&type=Date" />
  </picture>
</a>

## Feedback &amp; Contributing

Issues, suggestions, and feedback are welcome at [GitHub Issues](https://github.com/ys-ll/uniterm/issues), and contributions via [Pull Request](https://github.com/ys-ll/uniterm/pulls) are always welcome.

Thanks to the following people for contributing code and improvements, and to everyone who reported issues and shared suggestions — you help make uniTerm better ❤️

- [@yuwei5380](https://github.com/yuwei5380)

## License

Apache 2.0
