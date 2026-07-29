# 关于 uniTerm

uniTerm 是一款轻量级全能终端模拟器，支持 **20+** 种连接协议，覆盖远程终端、远程桌面、文件传输、数据库管理、容器和服务器监控。内置**自主 AI Agent**，可独立规划并执行多轮 Shell 命令。


## 核心特性

### 全能终端

覆盖您所有的远程访问需求，在一个应用中完成所有工作。

#### 远程终端

通过密码或密钥认证连接 **SSH** 服务器，支持 SSH 隧道端口转发，任何连接都能通过 SSH 跳板机路由。同时支持 **Telnet** 和 **Mosh**（适合高延迟、间歇性网络环境）。

![新建连接](/imgs/new_connection_light.webp)

#### 本地与串口终端

支持 **PowerShell / CMD / Git Bash / WSL** 本地终端，以及可配置波特率、数据位、停止位、校验位的**串口**连接。

#### 文件传输

内置 **SFTP / FTP / FTPS / SMB / WebDAV / S3** 双栏文件浏览器，支持 **Zmodem**（`rz`/`sz`）在 SSH 终端中直接传输文件。

![SFTP](/imgs/sftp_light.webp)

#### 远程桌面

集成 **RDP**（Windows 远程桌面）、**VNC**（Linux 远程控制）和 **SPICE**（KVM/QEMU 虚拟机）协议，提供流畅的图形化远程桌面体验。

![RDP](/imgs/rdp_light.webp)

#### 数据库客户端

支持 **MySQL / PostgreSQL / Oracle / SQL Server / rqlite / Redis**，在终端内直接浏览表结构和执行查询。

![数据库](/imgs/database_light.webp)

#### 容器

集成 **Kubernetes / Docker / Podman / nerdctl**，可管理本机或 SSH 远程主机上的集群资源与容器镜像，支持生命周期操作、日志跟随、exec 进入容器终端等。

![Kubernetes](/imgs/kubernetes_light.webp)


### AI 助理

uniTerm 内置自主 AI Agent，可在终端中独立规划并执行多轮 Shell 命令。

- **自主多轮执行** — AI Agent 能够规划、执行、观察结果，并跨多轮迭代，无需人工干预。
- **LLM 集成** — 侧边栏聊天支持 Anthropic/OpenAI 兼容 API，可使用 Claude、GPT 及其他兼容模型。
- **灵活的执行模式** — 跳过、仅危险命令、危险+写入、全部确认 — 由您控制 AI Agent 的监督级别。
- **持久化对话** — 对话历史按会话保存，应用重启后仍可继续。
- **终端内集成** — AI 命令直接在活动终端标签页中执行，可固定到特定标签页或始终跟随活动标签页。
- **技能与命令** — 将可复用的工作流沉淀为**技能**（AI 可自主调用），将带参数的提示词沉淀为**命令**，在对话中通过 `/名称` 快速唤起。
- **智能补全** — 在 SSH 终端中输入时，从命令历史和 AI 中获得实时建议。

![AI 助理](/imgs/ai_assistant_light.webp)


### 个性化

#### 连接管理

分组、搜索、创建、批量管理服务器连接，一切触手可及。

#### 工作区与分栏

将终端标签页拖放至内容区域自由分栏，组合为工作区；拖动面板边缘调整大小和排列。

![工作区](/imgs/workspace_light.webp)

#### 云同步

通过您自己的 GitHub / GitLab / Gitee 私有仓库加密同步设置，无需担心数据丢失或泄露，跨设备无缝衔接。

![云同步](/imgs/cloud_sync_light.webp)

#### 自定义快捷键

为几乎所有操作自由绑定快捷键，双手无需离开键盘。

#### 主题

**28** 款终端主题 + **3** 款界面主题（Dark / Deep Blue / Light）。

![终端设置](/imgs/terminal_settings_light.webp)

#### 多语言

9 种界面语言：简体中文、繁体中文、English、日本語、한국어、Deutsch、Español、Français、Русский。


## 下一步

- [快速开始](/zh/getting-started) — 下载安装，建立首次连接
- [连接协议](/zh/connections/remote-terminal) — 了解各协议的使用方法
- [功能指南](/zh/features/ai-assistant) — 深入探索各项功能
