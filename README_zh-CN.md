<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>一站式终端，覆盖 SSH、RDP、VNC、SFTP、数据库、Kubernetes 等 30 余种协议<br>内置自主执行多轮命令的 AI Agent</p>
  <p><a href="https://uniterm.net">🌐 软件首页</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/zh/introduction">📖 用户手册</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README.md">English</a> &nbsp;|&nbsp; 简体中文 &nbsp;|&nbsp; <a href="README_ja.md">日本語</a>

<br>

<a href="https://github.com/ys-ll/uniterm/releases/latest"><img src="https://img.shields.io/github/v/release/ys-ll/uniterm" alt="GitHub release" /></a>
<a href="https://github.com/ys-ll/uniterm"><img src="https://img.shields.io/badge/platform-Windows%20%7C%20macOS%20%7C%20Linux%20%7C%20Android%20%7C%20Docker-blue" alt="Platform" /></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-green" alt="License" /></a>
<a href="https://github.com/ys-ll/uniterm/releases"><img src="https://img.shields.io/github/downloads/ys-ll/uniterm/total" alt="GitHub downloads" /></a>
<a href="https://github.com/ys-ll/uniterm"><img src="https://img.shields.io/github/stars/ys-ll/uniterm?style=social" alt="GitHub stars" /></a>
<a href="https://gitee.com/ys-l/uniterm"><img src="https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee" alt="Gitee stars" /></a>
<br>
<a href="https://trendshift.io/repositories/109042?utm_source=repository-badge&amp;utm_medium=badge&amp;utm_campaign=badge-repository-109042" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/repositories/109042" alt="ys-ll%2Funiterm | Trendshift" width="250" height="55"/></a>
<a href="https://alternativeto.net/software/uniterm/about/?utm_source=badge&amp;utm_medium=referral" target="_blank">
  <img src="https://alternativeto.net/static/badges/badge-compact-color.svg"
       alt="uniTerm | AlternativeTo"
       width="171" height="55"
       style="width: 171px; height: 55px;" />
</a>

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
- [路线图](#路线图)
- [欢迎 Star](#欢迎-star)
- [反馈与贡献](#反馈与贡献)
- [开源协议](#开源协议)

## 功能特性

### 全功能终端

远程终端、本地 & 串口终端、文件传输、远程桌面、数据库、容器、服务器监控 —— 覆盖全部远程访问场景。

- **远程终端** — SSH / Telnet / Mosh / Raw TCP，密码/私钥认证；含 SSH 隧道端口转发（任意连接可经 SSH 跳板访问）
- **本地 & 串口终端** — PowerShell / CMD / Git Bash / WSL，以及串口连接（波特率等参数、本地回显）
- **文件传输** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem，双栏浏览、鼠标拖拽上传下载，SSH 内 `rz`/`sz`
- **远程桌面** — RDP（Windows 远程桌面）、VNC（Linux 远程控制）、SPICE（KVM/QEMU 虚拟机）、X11（X Window 图形界面转发）
- **数据库客户端** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch，支持 AI 自然语言生成查询与数据行编辑
- **Kubernetes & 容器** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC，集群资源与容器镜像管理、Pod 日志与 exec 终端、CPU/内存指标
- **服务器监控** — CPU/内存/磁盘/网络、进程、端口、网卡实时监控

### AI Agent

自主执行的 AI Agent，独立规划并执行多轮 Shell 命令，直接在终端中完成复杂任务。

- **AI Agent 自主执行** — AI Agent 自主规划、执行、观察结果并迭代，在多轮 Shell 命令中无需人工干预即可完成复杂操作；侧边栏对话，兼容 Anthropic / OpenAI 协议，支持 Claude、GPT 及其他兼容模型。
- **灵活的执行模式** — 提供免确认、仅高危确认、写操作确认、全部确认四种模式，自主权由你掌控。
- **终端智能协作** — AI 命令直接在当前终端标签页中执行，支持固定到指定标签页或跟随当前激活终端。分屏中人与 AI 各司其职，同屏协作互不干扰。
- **MCP Server** — 内置 MCP 服务器，Claude Code、Codex、Gemini CLI 等外部 AI Agent 可安全地在已保存的连接上执行命令、传输文件，凭据不出应用。
- **技能与命令** — 可复用的 Skill 工作流与 Prompt 命令模板，输入 `/` 即可挂载触发，AI 也可自行沉淀新技能。
- **智能提示** — 输入时按历史命令与 AI 能力实时补全。

### 个性化能力

连接管理、自由分屏、云端同步、主题与国际化 —— 你的终端由你掌控。

- **连接管理器** — 分组管理服务器连接，快速搜索、一键新建连接，支持批量操作。
- **工作区** — 将终端标签拖动到内容区即可自由分屏，任意组合成工作区，并可拖拽面板边缘调整大小与布局。
- **云端同步** — 支持 Git 仓库、WebDAV、SFTP、S3、SMB 五种模式加密云同步，无需担心数据丢失泄露，多设备无缝衔接、随处接续工作。
- **自定义快捷键** — 自由绑定各项操作的键盘快捷键，实现全键盘操作，双手不离键盘。
- **主题与国际化** — 28 款终端主题、3 款界面主题（深色 / 深蓝 / 浅色）、自定义背景图片；支持简中、繁中、英、日、韩、德、西、法、俄、葡、越等 11 种语言界面。

## 支持的协议

| 类别 | 协议 | 说明 |
|------|------|------|
| 终端 | SSH | 远程服务器命令行管理 |
| 终端 | Telnet | 老旧设备、嵌入式系统的远程终端 |
| 终端 | Mosh | 高延迟、断续网络下的服务器连接 |
| 终端 | Serial | 串口终端连接，支持波特率等参数配置 |
| 终端 | Raw TCP | 原始 TCP 控制台会话，直连主机端口收发原始字节流 |
| 终端 | Local | PowerShell、CMD、Git Bash 等本地 Shell |
| 终端 | WSL | 通过本地终端打开已安装的 WSL 发行版（仅 Windows） |
| 文件传输 | SFTP | 服务器文件管理与传输 |
| 文件传输 | SCP | 兼容旧版 SSH 服务器的降级方案，适用于不支持 SFTP 协议的服务器 |
| 文件传输 | FTP / FTPS | 网站空间、NAS 文件传输 |
| 文件传输 | SMB | Windows 共享文件夹、NAS 文件访问 |
| 文件传输 | WebDAV | WebDAV 服务器文件管理 |
| 文件传输 | S3 | 兼容 Amazon S3 的对象存储 |
| 文件传输 | Zmodem | SSH 终端内 rz/sz 命令传输文件 |
| 远程桌面 | RDP | Windows 服务器远程桌面管理 |
| 远程桌面 | VNC | Linux 服务器远程控制 |
| 远程桌面 | SPICE | KVM/QEMU 虚拟机管理 |
| 远程桌面 | X11 | X Window 图形界面转发 |
| 数据库 | MySQL | 兼容 MySQL 协议：MySQL、MariaDB、TiDB 等 |
| 数据库 | PostgreSQL | 兼容 PostgreSQL 协议：PostgreSQL、CockroachDB 等 |
| 数据库 | Oracle Database | 通过纯 Go 驱动连接 Oracle Database |
| 数据库 | SQL Server | 通过纯 Go 驱动连接 SQL Server |
| 数据库 | SQLite | 本地嵌入式数据库，直接打开数据库文件 |
| 数据库 | rqlite | 基于 SQLite、Raft 共识的轻量分布式数据库 |
| 数据库 | Redis | 内存键值数据库，可视化键值浏览与编辑 |
| 数据库 | MongoDB | 文档数据库，树形浏览、查询编辑与行内编辑 |
| 数据库 | Elasticsearch | 分布式搜索与分析引擎，支持索引与文档管理 |
| 容器 | Kubernetes | 集群资源浏览与管理、Pod 日志与 exec 终端、性能指标 |
| 容器 | Docker | 容器与镜像管理，支持本机或 SSH 远程主机 |
| 容器 | Podman | 兼容 Docker 的容器引擎，支持本机或 SSH 远程主机 |
| 容器 | nerdctl (containerd) | containerd 容器管理，支持切换 namespace |
| 容器 | WSLC | Windows WSL2 Container 运行时（仅 Windows） |

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
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Kubernetes 管理" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="终端背景图片" width="45%" loading="eager" />
</p>

## 下载安装

前往 [GitHub Releases](https://github.com/ys-ll/uniterm/releases) 或 [Gitee Releases](https://gitee.com/ys-l/uniterm/releases) 下载最新版本：

- **Windows** (amd64 / arm64): 安装包 `uniterm-windows-*-installer-*.exe`，或便携版 `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): 下载 `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64): 下载 `uniterm-linux-*-*.tar.gz`、`.deb` 或 `.rpm`
- **Android** (arm64): 下载 `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64): Web 多租户部署模式——浏览器访问的服务端部署，每位用户拥有隔离的工作区，详见 [Docker 部署指南](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README_zh-CN.md)

> **关于 Windows 杀软误报**：由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

### 包管理器安装

```bash
# Windows
scoop bucket add uniterm https://github.com/ys-ll/scoop-uniterm && scoop install uniterm

# macOS
brew install --cask ys-ll/uniterm/uniterm

# Linux (deb)
curl -sLo uniterm.deb https://github.com/ys-ll/uniterm/releases/latest/download/uniterm-linux-amd64-*.deb
sudo dpkg -i uniterm.deb

# Linux (rpm)
curl -sLo uniterm.rpm https://github.com/ys-ll/uniterm/releases/latest/download/uniterm-linux-amd64-*.rpm
sudo rpm -i uniterm.rpm
```

### 运行依赖

- **Windows**: WebView2 运行时（Windows 10+ 已内置，更老的系统需安装）
- **macOS**: 无需额外依赖（使用系统自带 WebKit）
- **Linux**: `libgtk-3-0` 与 `libwebkit2gtk-4.1-0`（多数桌面发行版已自带）
- **Android**: Android 5.0+（使用系统自带 WebView，无需额外依赖）
- **Docker**: 宿主机仅需 Docker Engine（compose 部署需 Docker Compose v2），镜像内已内置全部运行时依赖

## 使用流程

### SSH 连接

1. 在连接管理器中点击**新建连接**
2. 填入主机、端口和认证信息（密码或私钥）
3. 点击**连接**打开 SSH 终端会话

### AI Agent

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
| 桌面框架 | Wails v3 |
| 后端 | Go |
| 前端 | Vue 3 + Pinia + Element Plus |
| 终端引擎 | xterm.js |
| AI 协议 | Anthropic Messages API / OpenAI Chat Completions API |

## 从源码构建

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0（安装：`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`）
- macOS 需 Xcode Command Line Tools
- Linux 需 `libgtk-3-dev` 与 `libwebkit2gtk-4.1-dev`
- Android 构建另需 JDK 21 与 Android SDK/NDK（`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`）

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                   # 开发模式运行
wails3 build                 # 构建生产版本
```

## 项目结构

```
uniTerm/
├── main.go                       # 入口文件
├── app.go                        # Wails 绑定、LLM API 代理、SFTP API
├── app_*.go                      # 平台相关实现
├── backend/
│   ├── session/                  # SSH/Telnet/Serial/SFTP/数据库 会话管理
│   ├── database/                 # SQL 执行、表结构查询、DSN 构建
│   ├── container/                # Docker/Podman/nerdctl 容器管理
│   ├── k8s/                      # Kubernetes 集群管理
│   ├── store/                    # 持久化配置（连接、AI、设置）
│   ├── sync/                     # 云端同步（GitHub/GitLab/Gitee）
│   ├── update/                   # 自动更新
│   ├── platform/                 # 平台抽象层
│   └── log/                      # 文件日志
├── frontend/
│   └── src/
│       ├── components/           # Vue 组件
│       ├── composables/          # 终端组合式函数
│       ├── stores/               # Pinia 状态管理
│       ├── services/             # AI 代理循环、LLM 客户端
│       ├── i18n/                 # 国际化翻译
│       ├── types/                # TypeScript 类型定义
│       ├── utils/                # 工具函数
│       └── vendor/               # 第三方库
├── plugins/                      # 插件目录
├── docs/                         # 文档
├── Taskfile.yml                  # wails3 构建任务
└── build/
    └── config.yml                # 构建配置（应用信息、开发模式）
```

## 路线图

已实现的协议 / 功能与后续计划见 [ROADMAP.md](ROADMAP.md)，逐版本的更新记录见 [CHANGELOG.md](CHANGELOG.md)。

## 欢迎 Star

如果 uniTerm 对你有帮助，欢迎点一个 ⭐ Star，这是对项目最大的鼓励，也能让更多人发现它。

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## 反馈与贡献

本项目为纯个人业余兴趣项目，无商业化及接收赞助计划，欢迎志同道合的朋友一起交流，参与共建。

欢迎通过 [GitHub Issues](https://github.com/ys-ll/uniterm/issues) 提交问题、建议或使用反馈，也欢迎通过 [Pull Request](https://github.com/ys-ll/uniterm/pulls) 贡献代码。由于终端访问场景与环境各异，作者难以覆盖所有场景的验证，如遇到相关问题，欢迎 fork 项目代码参与共建。本项目欢迎 vibe coding。

感谢以下朋友为 uniTerm 贡献代码与改进，以及每一位提交 issue 和建议的朋友，是你们让 uniTerm 变得更好 ❤️

<table>
  <tr>
    <td align="center"><a href="https://github.com/yuwei5380">@yuwei5380</a></td>
    <td align="center"><a href="https://github.com/surenwuyuwuqiu">@surenwuyuwuqiu</a></td>
    <td align="center"><a href="https://github.com/wangxufeng">@wangxufeng</a></td>
    <td align="center"><a href="https://github.com/coderstory">@coderstory</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/jiayunora">@jiayunora</a></td>
    <td align="center"><a href="https://github.com/iCarrear">@iCarrear</a></td>
    <td align="center"><a href="https://github.com/boltomli">@boltomli</a></td>
    <td align="center"><a href="https://github.com/kxn">@kxn</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/windtear">@windtear</a></td>
    <td align="center"><a href="https://github.com/Sunshow">@Sunshow</a></td>
    <td align="center"><a href="https://github.com/feuvan">@feuvan</a></td>
    <td align="center"><a href="https://github.com/zhangsir1211">@zhangsir1211</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/sonnartliao">@sonnartliao</a></td>
    <td align="center"><a href="https://github.com/Teejer">@Teejer</a></td>
    <td align="center"><a href="https://github.com/IBRAHIMELAMRABET">@IBRAHIMELAMRABET</a></td>
    <td align="center"><a href="https://github.com/chris-edstrom">@chris-edstrom</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/andreafunicelli">@andreafunicelli</a></td>
    <td align="center"><a href="https://github.com/dungvv">@dungvv</a></td>
    <td align="center"><a href="https://github.com/zhongqian97">@zhongqian97</a></td>
    <td align="center"><a href="https://github.com/laigcss">@laigcss</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/lutzr400">@lutzr400</a></td>
    <td align="center"><a href="https://github.com/Merrcurys">@Merrcurys</a></td>
    <td align="center"><a href="https://github.com/etanman">@etanman</a></td>
    <td align="center"><a href="https://github.com/wuqiang57">@wuqiang57</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/lijinxiao1982">@lijinxiao1982</a></td>
    <td align="center"><a href="https://github.com/mikemikimike">@mikemikimike</a></td>
    <td align="center"><a href="https://github.com/macoladik">@macoladik</a></td>
    <td align="center"><a href="https://github.com/NatureLProokie">@NatureLProokie</a></td>
  </tr>
  <tr>
    <td align="center"><a href="https://github.com/kevin9327">@kevin9327</a></td>
    <td align="center"><a href="https://github.com/gezerui888">@gezerui888</a></td>
    <td align="center"><a href="https://github.com/eltociear">@eltociear</a></td>
  </tr>
</table>

## 开源协议

Apache 2.0
