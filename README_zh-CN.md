<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>一款轻量级一站式终端软件，支持 SSH、RDP、VNC、SFTP、数据库 等 20 余种协议<br>内置可自主执行的 AI Agent，规划并执行多轮 Shell 命令</p>
  <p><a href="https://uniterm.net">🌐 软件首页</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/zh/introduction">📖 用户手册</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README.md">English</a> &nbsp;|&nbsp; 简体中文

<br>

<a href="https://github.com/ys-ll/uniterm/releases/latest"><img src="https://img.shields.io/github/v/release/ys-ll/uniterm" alt="GitHub release" /></a>
<a href="https://github.com/ys-ll/uniterm"><img src="https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux-blue" alt="Platform" /></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-green" alt="License" /></a>
<a href="https://github.com/ys-ll/uniterm"><img src="https://img.shields.io/github/stars/ys-ll/uniterm?style=social" alt="GitHub stars" /></a>
<a href="https://gitee.com/ys-l/uniterm"><img src="https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee" alt="Gitee stars" /></a>

</div>

## 目录

- [功能特性](#功能特性)
- [支持的协议](#支持的协议)
- [界面截图](#界面截图)
- [下载安装](#下载安装)
- [使用流程](#使用流程)
- [技术栈](#技术栈)
- [从源码构建](#从源码构建)
- [项目结构](#项目结构)
- [欢迎 Star](#欢迎-star)
- [反馈与贡献](#反馈与贡献)
- [开源协议](#开源协议)

## 功能特性

### 全功能终端

远程终端（SSH / Telnet / Mosh）、本地 & 串口终端（PowerShell / CMD / Git Bash / WSL）、文件传输、远程桌面、数据库、服务器监控 —— 覆盖全部远程访问场景。

- **远程终端** — SSH / Telnet / Mosh，密码/私钥认证；含 SSH 隧道端口转发（任意连接可经 SSH 跳板访问）
- **本地 & 串口终端** — PowerShell / CMD / Git Bash / WSL，以及串口连接（波特率等参数、本地回显）
- **文件传输** — SFTP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem，双栏浏览、鼠标拖拽上传下载，SSH 内 `rz`/`sz`
- **远程桌面** — RDP（Windows 远程桌面）、VNC（Linux 远程控制）、SPICE（KVM/QEMU 虚拟机）
- **数据库客户端** — MySQL / PostgreSQL / Oracle / SQL Server / rqlite / Redis / MongoDB
- **服务器监控** — CPU/内存/磁盘/网络、进程、端口、网卡实时监控

### AI 助理

自主执行的 AI Agent，独立规划并执行多轮 Shell 命令，直接在终端中完成复杂任务。

- **自主多轮执行** — AI Agent 能够自主规划、执行、观察结果并迭代，在多轮 Shell 命令中无需人工干预即可完成复杂操作。
- **大模型集成** — 侧边栏对话，兼容 Anthropic / OpenAI 协议，支持 Claude、GPT 及其他兼容模型。
- **灵活的执行模式** — 提供免确认、仅高危确认、写操作确认、全部确认四种模式，自主权由你掌控。
- **对话持久化** — 会话聊天记录按标签页保存，重新打开应用后历史记录仍然保留。
- **终端智能协作** — AI 命令直接在当前终端标签页中执行，支持固定到指定标签页或跟随当前激活终端。分屏中人与 AI 各司其职，同屏协作互不干扰。
- **智能补全** — SSH 终端输入时，根据历史命令和 AI 能力实时提供命令补全建议。

### 个性化能力

连接管理、自由分屏、云端同步、主题定制 —— 你的终端由你掌控。

- **连接管理器** — 分组管理服务器连接，快速搜索、一键新建连接，支持批量操作。
- **自由分屏** — 将终端标签拖动到内容区即可自由分屏，任意组合成工作区，并可拖拽面板边缘调整大小与布局。
- **云端同步** — 基于 GitHub、GitLab、Gitee 去中心化个人私有仓库加密自动同步配置，无需担心数据丢失泄露，多设备无缝衔接、随处接续工作。
- **自定义快捷键** — 自由绑定各项操作的键盘快捷键，实现全键盘操作，双手不离键盘。
- **主题** — 28 款终端主题、3 款界面主题（暗色 / 深蓝 / 浅色）。
- **国际化** — 支持简中、繁中、英、日、韩、德、西、法、俄等 9 种语言界面。

## 支持的协议

| 类别 | 协议 | 说明 |
|------|------|------|
| 终端 | SSH | 远程服务器命令行管理 |
| 终端 | Telnet | 老旧设备、嵌入式系统的远程终端 |
| 终端 | Mosh | 高延迟、断续网络下的服务器连接 |
| 终端 | Serial | 串口终端连接，支持波特率等参数配置 |
| 终端 | Local | PowerShell、CMD、Git Bash 等本地 Shell |
| 终端 | WSL | 通过本地终端打开已安装的 WSL 发行版 |
| 文件传输 | SFTP | 服务器文件管理与传输 |
| 文件传输 | FTP / FTPS | 网站空间、NAS 文件传输 |
| 文件传输 | SMB | Windows 共享文件夹、NAS 文件访问 |
| 文件传输 | WebDAV | WebDAV 服务器文件管理 |
| 文件传输 | S3 | 兼容 Amazon S3 的对象存储 |
| 文件传输 | Zmodem | SSH 终端内 rz/sz 命令传输文件 |
| 远程桌面 | RDP | Windows 服务器远程桌面管理（仅 Windows） |
| 远程桌面 | VNC | Linux 服务器远程控制 |
| 远程桌面 | SPICE | KVM/QEMU 虚拟机管理 |
| 数据库 | MySQL | 兼容 MySQL 协议：MySQL、MariaDB、TiDB 等 |
| 数据库 | PostgreSQL | 兼容 PostgreSQL 协议：PostgreSQL、CockroachDB 等 |
| 数据库 | Oracle Database | 通过纯 Go 驱动连接 Oracle Database |
| 数据库 | SQL Server | 通过纯 Go 驱动连接 SQL Server |
| 数据库 | rqlite | 基于 SQLite、Raft 共识的轻量分布式数据库 |
| 数据库 | Redis | 内存键值数据库，可视化键值浏览与编辑 |
| 数据库 | MongoDB | 文档数据库，树形浏览、查询编辑与行内编辑 |

Oracle Database 支持基于纯 Go 驱动实现。uniTerm 不随安装包分发 Oracle Database、Oracle Instant Client、OJDBC、Wallet 文件或 Oracle 品牌素材；用户需自行确保其 Oracle 授权、凭据和数据库访问权限合规。

## 界面截图

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="开始页" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="新建连接" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="SSH 终端与 AI 对话" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="工作区" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="SFTP 文件传输" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="数据库浏览器" width="45%" loading="eager" />
  </picture>
</p>
  </picture>
</p>

## 下载安装

前往 [GitHub Releases](https://github.com/ys-ll/uniterm/releases) 或 [Gitee Releases](https://gitee.com/ys-l/uniterm/releases) 下载最新版本：

- **Windows**: 安装包 `uniterm-windows-amd64-installer-*.exe`，或便携版 `uniterm-windows-amd64-portable-*.zip`
- **macOS**: 下载 `uniterm-darwin-universal-*.dmg`
- **Linux**: 下载 `uniterm-linux-amd64-*.tar.gz`、`.deb` 或 `.rpm`

### 包管理器安装

```bash
# Windows
winget install ys-ll.uniTerm
scoop bucket add uniterm https://github.com/ys-ll/scoop-uniterm && scoop install uniterm
choco install uniterm

# macOS
brew install ys-ll/uniterm/uniterm

# Linux (deb)
curl -sLo uniterm.deb https://github.com/ys-ll/uniterm/releases/latest/download/uniterm-linux-amd64-*.deb
sudo dpkg -i uniterm.deb
```

### 运行依赖

- **Windows**: WebView2 运行时（Windows 10+ 已内置，更老的系统需安装）
- **macOS**: 无需额外依赖（使用系统自带 WebKit）
- **Linux**: `libgtk-3-0` 与 `libwebkit2gtk-4.1-0`（多数桌面发行版已自带）

## 使用流程

### SSH 连接

1. 在连接管理器中点击**新建连接**
2. 填入主机、端口和认证信息（密码或私钥）
3. 点击**连接**打开 SSH 终端会话

### AI 助理

1. 进入设置页面，配置你的 **AI 大模型**（API 地址、模型和密钥）
2. 打开一个终端标签页（SSH 或本地）
3. 打开 AI 侧边栏对话，输入需求 —— AI Agent 直接在终端中执行命令

### SFTP 文件传输

1. 在连接管理器中**右键**一个 SSH 连接
2. 选择**连接 SFTP**
3. 在双栏文件管理器中浏览、上传、下载或拖拽文件

## 技术栈

| 层级 | 技术 |
|------|------|
| 桌面框架 | Wails v2 |
| 后端 | Go |
| 前端 | Vue 3 + Pinia + Element Plus |
| 终端引擎 | xterm.js |
| AI 协议 | Anthropic Messages API / OpenAI Chat Completions API |

## 从源码构建

需要 [Go](https://go.dev/dl/) 1.23+、[Node.js](https://nodejs.org/) 20+ 和 [Wails CLI](https://wails.io/docs/gettingstarted/installation) v2。此外，macOS 需 Xcode Command Line Tools，Linux 需 `libgtk-3-dev` 与 `libwebkit2gtk-4.1-dev`。

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails dev                   # 开发模式运行
wails build                 # 构建生产版本
```

## 项目结构

```
uniTerm/
├── main.go                       # 入口文件
├── app.go                        # Wails 绑定、LLM API 代理、SFTP API
├── backend/
│   ├── session/                  # SSH/SFTP/数据库 会话管理
│   ├── database/                 # SQL 执行、表结构查询、DSN 构建
│   ├── store/                    # 持久化配置（连接、AI、设置）
│   └── log/                      # 文件日志
├── frontend/
│   └── src/
│       ├── components/           # Vue 组件
│       ├── composables/          # 终端组合式函数
│       ├── stores/               # Pinia 状态管理
│       ├── services/             # AI 代理循环、LLM 客户端
│       ├── i18n/                 # 国际化翻译
│       └── types/                # TypeScript 类型定义
└── wails.json
```

## 欢迎 Star

如果 uniTerm 对你有帮助，欢迎点一个 ⭐ Star，这是对项目最大的鼓励，也能让更多人发现它。

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

## 反馈与贡献

欢迎通过 [GitHub Issues](https://github.com/ys-ll/uniterm/issues) 提交问题、建议或使用反馈，也欢迎通过 [Pull Request](https://github.com/ys-ll/uniterm/pulls) 参与共建。

感谢以下朋友为 uniTerm 贡献代码与改进，以及每一位提交 issue 和建议的朋友，是你们让 uniTerm 变得更好 ❤️

- [@yuwei5380](https://github.com/yuwei5380)
- [@surenwuyuwuqiu](https://github.com/surenwuyuwuqiu)
- [@wangxufeng](https://github.com/wangxufeng)

## 开源协议

Apache 2.0
