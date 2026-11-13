<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>一站式終端機，涵蓋 SSH、RDP、SFTP、資料庫、Kubernetes 等 30 種以上協定<br>內建可自主執行多輪指令的 AI Agent</p>
  <p><a href="https://uniterm.net">🌐 軟體首頁</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/zh/introduction">📖 使用者手冊</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; 繁體中文 &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## 目錄

- [功能特色](#功能特色)
- [支援的協定](#支援的協定)
- [介面截圖](#介面截圖)
- [下載安裝](#下載安裝)
- [快速上手](#快速上手)
- [技術棧](#技術棧)
- [從原始碼建置](#從原始碼建置)
- [專案結構](#專案結構)
- [路線圖](#路線圖)
- [歡迎 Star](#歡迎-star)
- [回饋與貢獻](#回饋與貢獻)
- [授權條款](#授權條款)

## 功能特色

### 全功能終端機

遠端終端機、本地與序列埠終端機、檔案傳輸、遠端桌面、資料庫、容器與伺服器監控 —— 涵蓋所有遠端存取需求。

- **遠端終端機** — SSH / Telnet / Mosh / Raw TCP，支援密碼或金鑰認證；內含 SSH 隧道連接埠轉發，任何連線皆可經由 SSH 跳板機轉發。
- **本地與序列埠終端機** — PowerShell / CMD / Git Bash / WSL，以及序列埠連線（可設定鮑率、資料位元、停止位元、同位檢查與本地回顯）。
- **檔案傳輸** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem，雙窗格瀏覽，並在 SSH 終端機內支援 `rz`/`sz`。
- **遠端桌面** — RDP（Windows 遠端桌面）、VNC（Linux 遠端控制）、SPICE（KVM/QEMU 虛擬機器）、X11（X Window 轉發）
- **資料庫用戶端** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch，支援 AI 自然語言查詢與資料列編輯。
- **Kubernetes 與容器** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC，叢集資源與映像檔管理、Pod 日誌與 exec 終端機、CPU/記憶體指標。
- **伺服器監控** — 即時監控 CPU、記憶體、磁碟、網路、程序、連接埠與網路介面。

### AI Agent

自主執行的 AI Agent，可在終端機中獨立規劃並執行多輪 Shell 指令。

- **自主 AI Agent** — AI Agent 可自行規劃、執行、觀察結果並持續迭代，跨越多輪 Shell 指令無需人工介入；側邊欄對話相容 Anthropic / OpenAI 協定，支援 Claude、GPT 及其他相容模型。
- **彈性執行模式** — 提供免確認、僅高危確認、寫入操作確認、全部確認四種模式，自主程度由你掌控。
- **終端機整合** — AI 指令直接在目前終端機分頁中執行，可釘選至特定分頁或跟隨目前使用中的分頁。可在分割窗格中並肩協作，各自擁有獨立的終端機環境。
- **MCP Server** — 內建 MCP 伺服器，讓 Claude Code、Codex、Gemini CLI 等外部 AI Agent 能安全地在已儲存的連線上執行指令、傳輸檔案 —— 憑證不會離開應用程式。
- **技能與指令** — 可重複使用的 Skill 工作流程與提示詞範本指令，在 AI 輸入框輸入 `/` 即可掛載使用，AI 也能自行儲存新技能。
- **智慧補全** — 輸入時依據指令歷史紀錄與 AI 能力即時補全。

### 個人化

連線管理、自由分割窗格、雲端同步、主題與國際化 —— 你的終端機由你做主。

- **連線管理員** — 伺服器連線分組管理，快速搜尋、快速建立，支援批次操作。
- **工作區** — 將終端機分頁拖曳至內容區即可自由分割，任意組合成工作區；拖曳面板邊緣可調整大小與重新排列。
- **雲端同步** — 提供 Git 儲存庫、WebDAV、SFTP、S3 與 SMB 五種模式的加密雲端同步，不必擔心資料遺失或外洩，並能在多裝置間無縫接續工作。
- **自訂快速鍵** — 自由為各項操作綁定鍵盤快速鍵，實現全鍵盤操作，雙手不離鍵盤。
- **主題與國際化** — 28 種終端機主題、3 種介面主題（深色 / 深藍 / 淺色）以及可自訂的背景圖片；支援 11 種語言介面：繁體中文、簡體中文、英文、日文、韓文、德文、西班牙文、法文、俄文、葡萄牙文、越南文。

## 支援的協定

| 類別 | 協定 | 說明 |
|------|------|------|
| 終端機 | SSH | 遠端伺服器命令列管理 |
| 終端機 | Telnet | 適用於老舊設備與嵌入式系統的遠端終端機 |
| 終端機 | Mosh | 高延遲或不穩定網路下的伺服器連線 |
| 終端機 | Serial | 序列埠終端機，可設定鮑率等參數 |
| 終端機 | Raw TCP | 原始 TCP 主控台，開啟純 socket 連線並收發原始位元組 |
| 終端機 | Local | PowerShell、CMD、Git Bash 等本地 Shell |
| 終端機 | WSL | 透過本地終端機開啟已安裝的 WSL 發行版（僅限 Windows） |
| 檔案傳輸 | SFTP | 伺服器檔案管理與傳輸 |
| 檔案傳輸 | SCP | 不支援 SFTP 的舊版 SSH 伺服器之備援協定 |
| 檔案傳輸 | FTP / FTPS | 網站空間、NAS 檔案傳輸 |
| 檔案傳輸 | SMB | Windows 共用資料夾、NAS 檔案存取 |
| 檔案傳輸 | WebDAV | WebDAV 伺服器檔案管理 |
| 檔案傳輸 | S3 | 相容 Amazon S3 的物件儲存 |
| 檔案傳輸 | Zmodem | 在終端機內透過 rz/sz 指令傳輸檔案 |
| 遠端桌面 | RDP | Windows 伺服器遠端桌面管理 |
| 遠端桌面 | VNC | Linux 伺服器遠端控制 |
| 遠端桌面 | SPICE | KVM/QEMU 虛擬機器管理 |
| 遠端桌面 | X11 | X Window 轉發 |
| 資料庫 | MySQL | 相容 MySQL 協定：MySQL、MariaDB、TiDB 等 |
| 資料庫 | PostgreSQL | 相容 PostgreSQL 協定：PostgreSQL、CockroachDB 等 |
| 資料庫 | Oracle Database | 透過純 Go 驅動連線 Oracle Database |
| 資料庫 | SQL Server | 透過純 Go 驅動連線 SQL Server |
| 資料庫 | SQLite | 本地嵌入式資料庫，直接開啟資料庫檔案即可連線 |
| 資料庫 | rqlite | 以 SQLite 為基礎、採用 Raft 共識的輕量級分散式資料庫 |
| 資料庫 | Redis | 記憶體鍵值資料庫，提供視覺化鍵值瀏覽與編輯 |
| 資料庫 | MongoDB | 文件型資料庫，支援樹狀瀏覽、查詢編輯與行內編輯 |
| 資料庫 | Elasticsearch | 分散式搜尋與分析引擎，支援索引與文件管理 |
| 容器 | Kubernetes | 叢集資源瀏覽與管理、Pod 日誌與 exec、效能指標 |
| 容器 | Docker | 容器與映像檔管理，支援本機或透過 SSH 連線遠端主機 |
| 容器 | Podman | 相容 Docker 的容器引擎，支援本機或透過 SSH 連線遠端主機 |
| 容器 | nerdctl (containerd) | containerd 容器管理，支援切換 namespace |
| 容器 | WSLC | Windows WSL2 Container 執行環境（僅限 Windows） |

Oracle Database 支援以純 Go 驅動實作。uniTerm 不會隨附 Oracle Database、Oracle Instant Client、OJDBC、wallet 檔案或 Oracle 品牌素材；使用者需自行確保 Oracle 授權、憑證與資料庫存取權限。

## 介面截圖

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="開始頁" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="新增連線" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="SSH 終端機與 AI 助手" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="工作區" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="SFTP 檔案傳輸" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="資料庫瀏覽器" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Kubernetes 管理" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="終端機背景圖片" width="45%" loading="eager" />
</p>

## 下載安裝

前往 [GitHub Releases](https://github.com/ys-ll/uniterm/releases) 或 [Gitee Releases](https://gitee.com/ys-l/uniterm/releases) 下載最新版本：

- **Windows** (amd64 / arm64)：安裝包 `uniterm-windows-*-installer-*.exe`，或免安裝版 `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64)：下載 `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64)：下載 `uniterm-linux-*-*.tar.gz`、`.deb` 或 `.rpm`
- **Android** (arm64)：下載 `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64)：Web 多租戶部署模式 —— 透過瀏覽器存取的伺服器部署，每位使用者皆有隔離的工作區，詳見 [Docker 部署指南](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **關於 Windows 防毒軟體誤報**：由於本開源軟體未購買程式碼簽署憑證，未簽署的執行檔可能被部分防毒引擎（如 Windows Defender）誤報攔截。這是 Go/Wails 應用程式的已知問題（參見 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。您可以在防毒軟體中為其新增排除規則予以放行。請務必從 GitHub、Gitee 官方開源管道下載軟體。若仍擔心含有惡意程式，可自行下載原始碼在本地建置執行。

### 套件管理器

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

### 執行階段相依套件

- **Windows**：WebView2 執行階段（Windows 10+ 已內建，較舊版本需一次性安裝）
- **macOS**：無額外相依套件（使用系統內建 WebKit）
- **Linux**：`libgtk-3-0` 與 `libwebkit2gtk-4.1-0`（多數桌面發行版已預先安裝）
- **Android**：Android 5.0+（使用系統內建 WebView，無額外相依套件）
- **Docker**：宿主機僅需 Docker Engine（compose 部署需 Docker Compose v2），映像檔已內建所有執行階段相依套件

## 快速上手

### SSH 連線

1. 在連線管理員中點擊**新增連線**
2. 填入主機、連接埠與認證資訊（密碼或私密金鑰）
3. 點擊**連線**開啟 SSH 終端機工作階段

### AI Agent

1. 前往設定頁面，設定你的 **AI 供應商**（API 端點、模型與金鑰）
2. 開啟一個終端機分頁（SSH 或本地）
3. 開啟 AI 側邊欄對話 —— 輸入你的任務，AI Agent 即會直接在終端機中執行指令

### SFTP 檔案傳輸

1. 在連線管理員中對 SSH 連線按**右鍵**
2. 選擇**連線 SFTP**
3. 在雙窗格檔案管理器中瀏覽、上傳、下載或拖放檔案

## 技術棧

| 層級 | 技術 |
|------|------|
| 桌面框架 | Wails v3 |
| 後端 | Go |
| 前端 | Vue 3 + Pinia + Element Plus |
| 終端機引擎 | xterm.js |
| AI 協定 | Anthropic Messages API / OpenAI Chat Completions API |

## 從原始碼建置

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0（安裝：`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`）
- macOS 需安裝 Xcode Command Line Tools
- Linux 需安裝 `libgtk-3-dev` 與 `libwebkit2gtk-4.1-dev`
- Android 建置另需 JDK 21 與 Android SDK/NDK（`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`）

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # 開發模式
wails3 build                # 建置正式版本
```

## 專案結構

```
uniTerm/
├── main.go                       # 進入點
├── app.go                        # Wails 綁定、LLM API 代理、SFTP API
├── app_*.go                      # 各平台特定實作
├── backend/
│   ├── session/                  # SSH/Telnet/Serial/SFTP/資料庫 工作階段管理
│   ├── database/                 # SQL 執行、結構描述查詢、DSN 建構
│   ├── container/                # Docker/Podman/nerdctl 容器管理
│   ├── k8s/                      # Kubernetes 叢集管理
│   ├── store/                    # 持久化設定（連線、AI、偏好設定）
│   ├── sync/                     # 雲端同步（Git / WebDAV）
│   ├── update/                   # 自動更新
│   ├── platform/                 # 平台抽象層
│   └── log/                      # 檔案日誌
├── frontend/
│   └── src/
│       ├── components/           # Vue 元件
│       ├── composables/          # 終端機組合式函式
│       ├── stores/               # Pinia 狀態管理
│       ├── services/             # AI Agent 迴圈、LLM 用戶端
│       ├── i18n/                 # 翻譯檔案
│       ├── types/                # TypeScript 型別定義
│       ├── utils/                # 工具函式
│       └── vendor/               # 第三方函式庫
├── plugins/                      # 外掛目錄
├── docs/                         # 文件
├── Taskfile.yml                  # wails3 建置任務
└── build/
    └── config.yml                # 建置設定（應用程式資訊、開發模式）
```

## 路線圖

目前已支援的協定 / 功能與後續規劃請見 [ROADMAP.md](ROADMAP.md)；各版本的更新紀錄請見 [CHANGELOG.md](CHANGELOG.md)。

## 歡迎 Star

如果 uniTerm 對你有幫助，歡迎給它一個 ⭐ Star —— 這是對專案最大的鼓勵，也能讓更多人發現它。

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## 回饋與貢獻

本專案是作者於閒暇時間維護的個人興趣專案，沒有商業化或接受贊助的計畫 —— 誠摯歡迎志同道合的朋友一起交流、共同打造。

歡迎透過 [GitHub Issues](https://github.com/ys-ll/uniterm/issues) 提交問題、建議與使用回饋，也歡迎透過 [Pull Request](https://github.com/ys-ll/uniterm/pulls) 貢獻程式碼。由於終端機環境與使用場景繁多，作者難以在測試中涵蓋所有情況 —— 若你遇到任何問題，歡迎 fork 本專案並直接參與貢獻。本專案歡迎 vibe coding。

感謝以下朋友為 uniTerm 貢獻程式碼與改進，也感謝每一位回報問題與提供建議的朋友 —— 你們讓 uniTerm 變得更好 ❤️

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
    <td align="center"><a href="https://github.com/dajiaohuang">@dajiaohuang</a></td>
  </tr>
</table>

## 授權條款

Apache 2.0
