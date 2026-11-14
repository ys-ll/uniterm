# 关于 uniTerm

uniTerm 是一款轻量级全能终端模拟器，支持 **30+** 种连接协议，覆盖远程终端、远程桌面、文件传输、数据库管理、容器和服务器监控。内置**自主 AI Agent**，可独立规划并执行多轮 Shell 命令。

支持的平台：**Windows / macOS / Linux / Android**。


## 核心特性

### 全能终端

覆盖您所有的远程访问需求，在一个应用中完成所有工作。

#### 终端

SSH / Telnet / Mosh / Raw TCP / 串口，外加本地 Shell（PowerShell / CMD / Git Bash / WSL）。

![新建连接](/imgs/new_connection_light.webp)

#### 文件传输

内置 **SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3** 双栏文件浏览器与文件边栏，支持 **WSL 文件管理**、**Zmodem**（`rz`/`sz`）与统一传输任务（失败重试、逐文件明细）。

![SFTP](/imgs/sftp_light.webp)

#### 远程桌面

集成 **RDP**（Windows 远程桌面）、**VNC**（Linux 远程控制）、**SPICE**（KVM/QEMU 虚拟机）和 **X11**（X Window 图形界面转发与完整桌面）协议，提供流畅的图形化远程桌面体验。

![RDP](/imgs/rdp_light.webp)

#### 数据库客户端

支持 **MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch**，行内编辑、查询历史、结果导出与脚本执行。

![数据库](/imgs/database_light.webp)

#### 容器

集成 **Kubernetes / Docker / Podman / nerdctl / WSLC**，可管理本机或 SSH 远程主机上的集群资源与容器镜像，支持生命周期操作、日志跟随、exec 进入容器终端等。

![Kubernetes](/imgs/kubernetes_light.webp)

#### 服务器监控

实时监控远程主机的 CPU / 内存 / 网络 / 磁盘与进程，支持端口、磁盘、网卡明细，以及 **服务管理、硬件设备、IPMI** 传感器与主机时钟偏差检测。

![服务器监控](/imgs/monitor_light.webp)

#### 边栏

文件、监控、隧道、快捷命令等伴随面板随行展示，文件边栏可跟随终端路径，支持 WSL 文件浏览。


### AI 助理

uniTerm 内置自主 AI Agent，可在终端中独立规划并执行多轮 Shell 命令。

- **自主多轮执行** — AI Agent 能够自主规划、执行、观察结果并迭代，在多轮 Shell 命令中无需人工干预即可完成复杂操作。
- **大模型集成** — 侧边栏对话，兼容 Anthropic / OpenAI 协议，支持 Claude、GPT 及其他兼容模型。
- **灵活的执行模式** — 提供免确认、仅高危确认、写操作确认、全部确认四种模式，自主权由你掌控。
- **终端智能协作** — AI 命令直接在当前终端标签页中执行，支持固定到指定标签页或跟随当前激活终端。分屏中人与 AI 各司其职，同屏协作互不干扰。
- **uniTerm MCP** — 内置 MCP 服务器，Claude Code、Codex、Gemini CLI 等外部 AI Agent 可安全地在已保存的连接上执行命令、传输文件，凭据不出应用。
- **技能与命令** — 可复用的 Skill 工作流与 Prompt 命令模板，输入 `/` 即可挂载触发，AI 也可自行沉淀新技能；也可接入外部 MCP 服务器，扩展 AI 的工具能力。

![AI 助理](/imgs/ai_assistant_light.webp)


### 个性化

#### 连接管理

分组、搜索、创建、批量管理服务器连接，常用连接可**收藏**置顶，支持从 Xshell、MobaXterm、DBeaver、Navicat、virt-viewer（.vv）等一键导入，一切触手可及。

#### 密钥库与代理

统一管理可复用的登录身份（密码 / 私钥）与 SOCKS5 / HTTP 代理，连接时直接引用，跨设备同步。

#### 工作区与分栏

将终端标签页拖放至内容区域自由分栏，组合为工作区；拖动面板边缘调整大小和排列，支持面板最大化与全应用输入广播。

![工作区](/imgs/workspace_light.webp)

#### 网络工具箱

内置**网络扫描**（发现子网内存活主机与开放端口）、**SSH 隧道**（本地 / 远程 / 动态端口转发）与**本地网络服务**（TFTP / FTP / HTTP / SSH/SFTP / Telnet / NFS / S3），一站式网络工具入口。

![SSH 隧道](/imgs/ssh_tunnel_light.webp)

#### 云同步

支持 **Git / WebDAV 云同步双模式**：通过您自己的 GitHub / GitLab / Gitee 私有仓库或 WebDAV 服务加密同步连接、收藏与 AI 配置，无需担心数据丢失或泄露，跨设备无缝衔接。

![云同步](/imgs/cloud_sync_light.webp)

#### 应用内更新

GitHub / Gitee 双源自动切换，应用内下载、校验并一键安装。

#### 主题

多款终端主题与界面主题，界面字号、字体与配色均可定制。

![终端设置](/imgs/terminal_settings_light.webp)

#### 多语言

11 种界面语言：简体中文、繁体中文、English、日本語、한국어、Deutsch、Español、Français、Русский、Português、Tiếng Việt。


## 下一步

- [快速开始](/zh/getting-started) — 下载安装，建立首次连接
- [连接协议](/zh/connections/terminal) — 了解各协议的使用方法
- [功能指南](/zh/features/ai-assistant) — 深入探索各项功能
