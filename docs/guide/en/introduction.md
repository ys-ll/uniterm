# About uniTerm

uniTerm is a lightweight all-in-one terminal emulator supporting **20+** connection protocols, covering remote terminals, remote desktops, file transfers, database management, containers, and server monitoring. It features a built-in **autonomous AI Agent** that can independently plan and execute multi-round shell commands.


## Core Features

### All-in-One Terminal

Cover all your remote access needs and get everything done in a single application.

#### Remote Terminal

Connect to **SSH** servers via password or key authentication, with SSH tunnel port forwarding support. Any connection can be routed through an SSH jump host. Also supports **Telnet** and **Mosh** (ideal for high-latency, intermittent network environments).

![New Connection](/imgs/new_connection_light.webp)

#### Local and Serial Terminal

Supports **PowerShell / CMD / Git Bash / WSL** local terminals, as well as **serial** connections with configurable baud rate, data bits, stop bits, and parity.

#### File Transfer

Built-in **SFTP / FTP / FTPS / SMB / WebDAV / S3** dual-pane file browser, with **Zmodem** (`rz`/`sz`) support for transferring files directly within SSH terminals.

![SFTP](/imgs/sftp_light.webp)

#### Remote Desktop

Integrated **RDP** (Windows Remote Desktop), **VNC** (Linux remote control), and **SPICE** (KVM/QEMU virtual machine) protocols provide a smooth graphical remote desktop experience.

![RDP](/imgs/rdp_light.webp)

#### Database Client

Supports **MySQL / PostgreSQL / Oracle / SQL Server / rqlite / Redis**, allowing you to browse table structures and execute queries directly within the terminal.

![Database](/imgs/database_light.webp)

#### Containers

Integrated **Kubernetes / Docker / Podman / nerdctl** management for cluster resources and container images, on the local machine or remote hosts over SSH — with lifecycle actions, log following, and exec into a container terminal.

![Kubernetes](/imgs/kubernetes_light.webp)


### AI Assistant

uniTerm features a built-in autonomous AI Agent that can independently plan and execute multi-round shell commands in the terminal.

- **Autonomous Multi-Round Execution** — The AI Agent can plan, execute, observe results, and iterate across multiple rounds without human intervention.
- **LLM Integration** — The sidebar chat supports Anthropic/OpenAI-compatible APIs, allowing use of Claude, GPT, and other compatible models.
- **Flexible Execution Modes** — Skip, dangerous commands only, dangerous + write, confirm all — you control the level of supervision over the AI Agent.
- **Persistent Conversations** — Conversation history is saved per session and can be resumed even after restarting the application.
- **In-Terminal Integration** — AI commands are executed directly in the active terminal tab, and can be pinned to a specific tab or always follow the active tab.
- **Skills & Commands** — Capture reusable workflows as **skills** (which the AI can invoke on its own) and parameterized prompts as **commands**, invoked quickly in the conversation via `/name`.
- **Smart Completion** — Receive real-time suggestions from command history and AI while typing in SSH terminals.

![AI Assistant](/imgs/ai_assistant_light.webp)


### Personalization

#### Connection Management

Group, search, create, and batch-manage server connections — everything at your fingertips.

#### Workspaces and Splits

Drag and drop terminal tabs into the content area to freely split panes, combining them into workspaces; drag panel edges to resize and rearrange.

![Workspace](/imgs/workspace_light.webp)

#### Cloud Sync

Encrypted settings sync via your own GitHub / GitLab / Gitee private repository — no worries about data loss or leaks, seamlessly transition across devices.

![Cloud Sync](/imgs/cloud_sync_light.webp)

#### Custom Keyboard Shortcuts

Freely bind keyboard shortcuts for nearly every operation, so your hands never need to leave the keyboard.

#### Themes

**28** terminal themes + **3** UI themes (Dark / Deep Blue / Light).

![Terminal Settings](/imgs/terminal_settings_light.webp)

#### Multi-Language

9 interface languages: English, Simplified Chinese, Traditional Chinese, Japanese, Korean, German, Spanish, French, Russian.


## Next Steps

- [Getting Started](/en/getting-started) — Download, install, and establish your first connection
- [Connection Protocols](/en/connections/remote-terminal) — Learn how to use each protocol
- [Feature Guide](/en/features/ai-assistant) — Explore features in depth
