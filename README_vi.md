<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>Terminal tất cả trong một hỗ trợ hơn 30 giao thức — SSH, RDP, SFTP, cơ sở dữ liệu, Kubernetes và nhiều hơn nữa<br>Kèm theo AI Agent tích hợp có thể tự động thực thi các chuỗi lệnh shell nhiều lượt</p>
  <p><a href="https://uniterm.net">🌐 Trang chủ</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 Hướng dẫn sử dụng</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; Tiếng Việt

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

## Mục lục

- [Tính năng](#tính-năng)
- [Giao thức được hỗ trợ](#giao-thức-được-hỗ-trợ)
- [Ảnh chụp màn hình](#ảnh-chụp-màn-hình)
- [Tải xuống](#tải-xuống)
- [Quy trình nhanh](#quy-trình-nhanh)
- [Công nghệ sử dụng](#công-nghệ-sử-dụng)
- [Biên dịch từ mã nguồn](#biên-dịch-từ-mã-nguồn)
- [Cấu trúc dự án](#cấu-trúc-dự-án)
- [Lộ trình phát triển](#lộ-trình-phát-triển)
- [Gắn sao cho dự án](#gắn-sao-cho-dự-án)
- [Phản hồi & Đóng góp](#phản-hồi--đóng-góp)
- [Giấy phép](#giấy-phép)

## Tính năng

### Terminal đầy đủ tính năng

Terminal từ xa, terminal cục bộ & cổng serial, truyền tệp, máy tính từ xa, cơ sở dữ liệu, container và giám sát máy chủ — đáp ứng mọi nhu cầu truy cập từ xa.

- **Terminal** — SSH / Telnet / Mosh / Raw TCP / Serial, cùng các shell cục bộ (PowerShell / CMD / Git Bash / WSL).
- **Truyền tệp** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem với duyệt tệp hai khung và hỗ trợ `rz`/`sz` trong terminal SSH.
- **Máy tính từ xa** — RDP (Remote Desktop của Windows), VNC (điều khiển từ xa Linux), SPICE (máy ảo KVM/QEMU), X11 (chuyển tiếp X Window)
- **Ứng dụng khách cơ sở dữ liệu** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch, với truy vấn ngôn ngữ tự nhiên hỗ trợ bởi AI và chỉnh sửa trực tiếp các dòng dữ liệu.
- **Kubernetes & Container** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC, với quản lý tài nguyên & image của cluster, log Pod và terminal exec, cùng chỉ số CPU/bộ nhớ.
- **Giám sát máy chủ** — CPU, bộ nhớ, ổ đĩa, mạng, tiến trình, cổng và giao diện mạng theo thời gian thực.

### AI Agent

AI Agent tự động lập kế hoạch và thực thi các chuỗi lệnh shell nhiều lượt ngay trong terminal của bạn.

- **AI Agent tự động** — AI Agent có thể lập kế hoạch, thực thi, quan sát kết quả và lặp lại qua nhiều vòng lệnh shell mà không cần can thiệp thủ công; chat trên thanh sidebar với API tương thích Anthropic/OpenAI, hỗ trợ Claude, GPT và các mô hình tương thích khác.
- **Chế độ thực thi linh hoạt** — Bỏ qua xác nhận, chỉ với lệnh nguy hiểm, lệnh nguy hiểm + ghi, hoặc xác nhận tất cả — bạn quyết định mức giám sát mà AI Agent cần.
- **Tích hợp terminal** — Lệnh của AI được thực thi trực tiếp trong tab terminal đang hoạt động, có thể ghim vào một tab cụ thể hoặc đi theo tab đang hoạt động. Phối hợp song song trong các khung chia đôi, mỗi khung có ngữ cảnh terminal riêng.
- **Máy chủ MCP** — Máy chủ MCP tích hợp cho phép các AI Agent bên ngoài (Claude Code, Codex, Gemini CLI, ...) chạy lệnh và truyền tệp trên các kết nối đã lưu — thông tin xác thực không bao giờ rời khỏi ứng dụng.
- **Kỹ năng & Lệnh** — Các quy trình kỹ năng (skill) và lệnh mẫu prompt có thể tái sử dụng, đính kèm bằng `/` trong ô nhập của AI; AI cũng có thể tự lưu kỹ năng mới.
- **Hoàn thành thông minh** — Gợi ý hoàn thành theo thời gian thực khi đang gõ, dựa trên lịch sử lệnh và khả năng của AI.

### Cá nhân hóa

Quản lý kết nối, chia khung, đồng bộ đám mây, chủ đề & đa ngôn ngữ — terminal của bạn, theo cách của bạn.

- **Trình quản lý kết nối** — Nhóm, tìm kiếm và quản lý hàng loạt các kết nối máy chủ; nhập/xuất mã hóa và nhập từ các ứng dụng terminal khác (MobaXterm, Xshell, v.v.).
- **Không gian làm việc** — Kéo tab terminal vào vùng nội dung để chia tách tự do và kết hợp chúng thành một không gian làm việc; kéo mép các khung để thay đổi kích thước và sắp xếp lại.
- **Bộ công cụ mạng** — Quét mạng, đường hầm chuyển tiếp cổng SSH và dịch vụ cục bộ để chạy máy chủ tạm thời cho các thiết bị khác.
- **Đồng bộ đám mây** — Đồng bộ đám mây mã hóa với năm chế độ: kho lưu trữ Git, WebDAV, SFTP, S3 và SMB — không lo mất dữ liệu hoặc rò rỉ, và tiếp tục công việc liền mạch trên nhiều thiết bị.
- **Chủ đề & Quốc tế hóa** — 28 chủ đề terminal cùng 3 chủ đề giao diện (Tối / Xanh đậm / Sáng) và hình nền có thể tùy chỉnh; giao diện 11 ngôn ngữ: tiếng Trung giản thể, tiếng Trung phồn thể, tiếng Anh, tiếng Nhật, tiếng Hàn, tiếng Đức, tiếng Tây Ban Nha, tiếng Pháp, tiếng Nga, tiếng Bồ Đào Nha, tiếng Việt.

## Giao thức được hỗ trợ

| Danh mục | Giao thức | Mô tả |
|----------|----------|-------------|
| Terminal | SSH | Quản lý shell máy chủ từ xa |
| Terminal | Telnet | Terminal từ xa cho thiết bị cũ và hệ thống nhúng |
| Terminal | Mosh | Kết nối máy chủ qua mạng có độ trễ cao hoặc gián đoạn thường xuyên |
| Terminal | Serial | Terminal cổng serial với baud rate và các tham số có thể cấu hình |
| Terminal | Raw TCP | Console Raw TCP mở socket thuần và truyền nhận byte thô |
| Terminal | Local | PowerShell, CMD, Git Bash và các shell cục bộ khác |
| Terminal | WSL | Mở các bản phân phối WSL đã cài đặt qua terminal cục bộ (chỉ Windows) |
| Truyền tệp | SFTP | Quản lý và truyền tệp trên máy chủ |
| Truyền tệp | SCP | Giao thức dự phòng cho các máy chủ SSH cũ không hỗ trợ SFTP |
| Truyền tệp | FTP / FTPS | Lưu trữ website, truyền tệp với NAS |
| Truyền tệp | SMB | Thư mục chia sẻ Windows, truy cập tệp NAS |
| Truyền tệp | WebDAV | Quản lý tệp trên máy chủ WebDAV |
| Truyền tệp | S3 | Lưu trữ đối tượng tương thích Amazon S3 |
| Truyền tệp | Zmodem | Truyền tệp trong terminal qua các lệnh rz/sz |
| Máy tính từ xa | RDP | Quản lý máy tính từ xa máy chủ Windows |
| Máy tính từ xa | VNC | Điều khiển từ xa máy chủ Linux |
| Máy tính từ xa | SPICE | Quản lý máy ảo KVM/QEMU |
| Máy tính từ xa | X11 | Chuyển tiếp X Window |
| Cơ sở dữ liệu | MySQL | Giao thức MySQL: MySQL, MariaDB, TiDB và nhiều hơn nữa |
| Cơ sở dữ liệu | PostgreSQL | Giao thức PostgreSQL: PostgreSQL, CockroachDB và nhiều hơn nữa |
| Cơ sở dữ liệu | Oracle Database | Kết nối Oracle Database qua driver thuần Go |
| Cơ sở dữ liệu | SQL Server | Kết nối SQL Server qua driver thuần Go |
| Cơ sở dữ liệu | SQLite | Cơ sở dữ liệu cục bộ nhúng, kết nối bằng cách mở tệp cơ sở dữ liệu |
| Cơ sở dữ liệu | rqlite | Cơ sở dữ liệu phân tán nhẹ được xây dựng trên SQLite với đồng thuận Raft |
| Cơ sở dữ liệu | Redis | Kho key-value trong bộ nhớ với duyệt và chỉnh sửa key trực quan |
| Cơ sở dữ liệu | MongoDB | Cơ sở dữ liệu tài liệu với duyệt dạng cây, trình soạn thảo truy vấn và chỉnh sửa trực tiếp |
| Cơ sở dữ liệu | Elasticsearch | Công cụ tìm kiếm và phân tích phân tán với quản lý index và tài liệu |
| Container | Kubernetes | Duyệt và quản lý tài nguyên cluster, log Pod và exec, chỉ số hiệu năng |
| Container | Docker | Quản lý container và image, trên máy cục bộ hoặc máy chủ từ xa qua SSH |
| Container | Podman | Container engine tương thích Docker, trên máy cục bộ hoặc máy chủ từ xa qua SSH |
| Container | nerdctl (containerd) | Quản lý container containerd với chuyển đổi namespace |
| Container | WSLC | Container runtime Windows WSL2 (chỉ Windows) |

Hỗ trợ Oracle Database được triển khai bằng driver thuần Go. uniTerm không đóng gói kèm Oracle Database, Oracle Instant Client, OJDBC, tệp wallet hay tài sản thương hiệu Oracle; người dùng tự chịu trách nhiệm về giấy phép Oracle, thông tin xác thực và quyền truy cập cơ sở dữ liệu của mình.

## Ảnh chụp màn hình

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="Trang khởi động" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="Kết nối mới" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="Terminal SSH với trợ lý AI" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="Không gian làm việc" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="Truyền tệp SFTP" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="Trình duyệt cơ sở dữ liệu" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Quản lý Kubernetes" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="Hình nền terminal" width="45%" loading="eager" />
</p>

## Tải xuống

Tải bản dựng sẵn mới nhất từ [GitHub Releases](https://github.com/ys-ll/uniterm/releases) hoặc [Gitee Releases](https://gitee.com/ys-l/uniterm/releases):

- **Windows** (amd64 / arm64): bộ cài `uniterm-windows-*-installer-*.exe`, hoặc bản di động `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): Tải `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64): Tải `uniterm-linux-*-*.tar.gz`, `.deb` hoặc `.rpm`
- **Android** (arm64): Tải `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64): triển khai web đa người dùng — triển khai máy chủ truy cập qua trình duyệt, mỗi người dùng có một không gian làm việc riêng biệt, xem [hướng dẫn triển khai Docker](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **Về báo động giả từ phần mềm diệt virus trên Windows**: Do phần mềm mã nguồn mở này chưa mua chứng chỉ ký mã (code signing), tệp thực thi chưa ký có thể gây báo động giả ở một số engine diệt virus (ví dụ Windows Defender). Đây là vấn đề đã biết với các ứng dụng Go/Wails (xem [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). Bạn có thể thêm quy tắc loại trừ trong phần mềm diệt virus để cho phép ứng dụng. Vui lòng chỉ tải xuống từ các kênh mã nguồn mở chính thức — GitHub và Gitee. Nếu bạn vẫn lo ngại về mã độc, bạn có thể tải mã nguồn và tự biên dịch, chạy trên máy của mình.

### Trình quản lý gói

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

### Phụ thuộc thời gian chạy

- **Windows**: runtime WebView2 (đã tích hợp trong Windows 10 trở lên; các phiên bản cũ hơn cần cài một lần)
- **macOS**: không có phụ thuộc thêm (sử dụng WebKit của hệ thống)
- **Linux**: `libgtk-3-0` và `libwebkit2gtk-4.1-0` (được cài sẵn trên hầu hết các bản phân phối desktop)
- **Android**: Android 5.0 trở lên (sử dụng WebView của hệ thống, không có phụ thuộc thêm)
- **Docker**: chỉ cần Docker Engine trên máy chủ (Docker Compose v2 cho triển khai compose); image đã bao gồm tất cả phụ thuộc thời gian chạy

## Quy trình nhanh

### Kết nối SSH

1. Nhấp **Kết nối mới** trong Trình quản lý kết nối
2. Điền máy chủ, cổng và thông tin xác thực (mật khẩu hoặc khóa riêng)
3. Nhấp **Kết nối** để mở phiên terminal SSH

### AI Agent

1. Vào Cài đặt và cấu hình **nhà cung cấp AI** (endpoint API, mô hình và khóa)
2. Mở một tab terminal (SSH hoặc cục bộ)
3. Mở khung chat AI trên thanh sidebar — nhập nhiệm vụ của bạn, và AI Agent sẽ thực thi lệnh trực tiếp trong terminal của bạn

### Truyền tệp SFTP

1. Trong Trình quản lý kết nối, **nhấp chuột phải** vào một kết nối SSH
2. Chọn **Kết nối SFTP**
3. Duyệt, tải lên, tải xuống và kéo-thả tệp trong trình quản lý tệp hai khung

## Công nghệ sử dụng

| Lớp | Công nghệ |
|-------|-----------|
| Framework Desktop | Wails v3 |
| Backend | Go |
| Frontend | Vue 3 + Pinia + Element Plus |
| Terminal | xterm.js |
| Giao thức AI | Anthropic Messages API / OpenAI Chat Completions API |

## Biên dịch từ mã nguồn

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (cài đặt bằng `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`)
- macOS cần Xcode Command Line Tools
- Linux cần `libgtk-3-dev` và `libwebkit2gtk-4.1-dev`
- Biên dịch cho Android cần thêm JDK 21 và Android SDK/NDK (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## Cấu trúc dự án

```
uniTerm/
├── main.go                       # Entry point
├── app.go                        # Wails bindings, LLM API proxy, SFTP API
├── app_*.go                      # Platform-specific implementations
├── backend/
│   ├── session/                  # SSH/Telnet/Serial/SFTP/database session management
│   ├── database/                 # SQL execution, schema introspection, DSN builders
│   ├── container/                # Docker/Podman/nerdctl container management
│   ├── k8s/                      # Kubernetes cluster management
│   ├── store/                    # Persistent config (connections, AI, settings)
│   ├── sync/                     # Cloud sync (Git / WebDAV)
│   ├── update/                   # Auto-update
│   ├── platform/                 # Platform abstraction layer
│   └── log/                      # File-based logging
├── frontend/
│   └── src/
│       ├── components/           # Vue components
│       ├── composables/          # Terminal composables
│       ├── stores/               # Pinia stores
│       ├── services/             # AI agent loop, LLM client
│       ├── i18n/                 # Translations
│       ├── types/                # TypeScript type definitions
│       ├── utils/                # Utility functions
│       └── vendor/               # Third-party libraries
├── plugins/                      # Plugin directory
├── docs/                         # Documentation
├── Taskfile.yml                  # wails3 build tasks
└── build/
    └── config.yml                # build config (app info, dev mode)
```

## Lộ trình phát triển

Xem [ROADMAP.md](ROADMAP.md) để biết trạng thái hiện tại của các giao thức / tính năng và những gì được kế hoạch tiếp theo. Xem [CHANGELOG.md](CHANGELOG.md) cho lịch sử theo từng bản phát hành.

## Gắn sao cho dự án

Nếu uniTerm hữu ích với bạn, hãy cân nhắc gắn cho dự án một ⭐ Star — đó là động lực lớn nhất cho dự án và giúp nhiều người hơn khám phá ra nó.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## Phản hồi &amp; Đóng góp

Đây là một dự án cá nhân theo sở thích được tác giả duy trì trong thời gian rảnh, không có kế hoạch thương mại hóa hay tài trợ — bất kỳ ai có cùng sở thích đều được chào đón nồng nhiệt để tham gia, thảo luận và cùng xây dựng.

Các issue, đề xuất và phản hồi được chào đón tại [GitHub Issues](https://github.com/ys-ll/uniterm/issues), và đóng góp mã nguồn qua [Pull Request](https://github.com/ys-ll/uniterm/pulls) luôn được chào đón. Do sự đa dạng rộng lớn của các môi trường terminal và tình huống sử dụng, tác giả không thể kiểm thử phủ hết mọi trường hợp — nếu bạn gặp bất kỳ vấn đề nào, đừng ngại fork dự án và đóng góp trực tiếp. Dự án này chào đón vibe coding.

Cảm ơn những người đã đóng góp mã nguồn và cải tiến dưới đây, và cảm ơn tất cả mọi người đã báo lỗi và chia sẻ đề xuất — các bạn giúp uniTerm ngày càng tốt hơn ❤️

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

## Giấy phép

Apache 2.0
