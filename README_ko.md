<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>SSH, RDP, SFTP, 데이터베이스, Kubernetes 등 30개 이상의 프로토콜을 지원하는 올인원 터미널<br>여러 턴의 셸 명령을 자율적으로 실행하는 AI 에이전트 내장</p>
  <p><a href="https://uniterm.net">🌐 홈페이지</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 사용자 가이드</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; 한국어 &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## 목차

- [주요 기능](#주요-기능)
- [지원 프로토콜](#지원-프로토콜)
- [스크린샷](#스크린샷)
- [다운로드](#다운로드)
- [빠른 워크플로](#빠른-워크플로)
- [기술 스택](#기술-스택)
- [소스에서 빌드하기](#소스에서-빌드하기)
- [프로젝트 구조](#프로젝트-구조)
- [로드맵](#로드맵)
- [이 프로젝트에 스타를](#이-프로젝트에-스타를)
- [피드백 및 기여](#피드백-및-기여)
- [라이선스](#라이선스)

## 주요 기능

### 풀 기능 터미널

원격 터미널, 로컬 및 시리얼 터미널, 파일 전송, 원격 데스크톱, 데이터베이스, 컨테이너, 서버 모니터링까지 — 원격 접속에 필요한 모든 것을 다룹니다.

- **원격 터미널** — 비밀번호 또는 키 인증 방식의 SSH / Telnet / Mosh / Raw TCP를 지원하며, SSH 터널 포트 포워딩을 포함해 어떤 연결이든 SSH 점프 호스트를 통해 라우팅할 수 있습니다.
- **로컬 및 시리얼 터미널** — PowerShell / CMD / Git Bash / WSL 외에 보레이트, 데이터 비트, 스톱 비트, 패리티, 로컬 에코를 설정할 수 있는 시리얼 연결을 지원합니다.
- **파일 전송** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem을 지원하며, 듀얼 패인 브라우징과 SSH 터미널에서의 `rz`/`sz`를 지원합니다.
- **원격 데스크톱** — RDP(Windows 원격 데스크톱), VNC(Linux 원격 제어), SPICE(KVM/QEMU 가상 머신), X11(X Window 포워딩)
- **데이터베이스 클라이언트** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch를 지원하며, AI 기반 자연어 쿼리와 데이터 행 편집을 제공합니다.
- **Kubernetes 및 컨테이너** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC를 지원하며, 클러스터 리소스 및 이미지 관리, Pod 로그와 exec 터미널, CPU/메모리 메트릭을 제공합니다.
- **서버 모니터링** — CPU, 메모리, 디스크, 네트워크, 프로세스, 포트, 네트워크 인터페이스를 실시간으로 표시합니다.

### AI 에이전트

터미널 안에서 여러 턴의 셸 명령을 스스로 계획하고 실행하는 자율형 AI 에이전트입니다.

- **자율형 AI 에이전트** — AI 에이전트는 수동 개입 없이 여러 라운드의 셸 명령에 걸쳐 계획, 실행, 결과 관찰, 반복을 수행할 수 있습니다. Anthropic/OpenAI 호환 API를 사용한 사이드바 채팅으로 Claude, GPT 등 호환 모델을 지원합니다.
- **유연한 실행 모드** — 전부 우회, 위험한 것만, 위험 + 쓰기, 전부 확인 중에서 선택할 수 있습니다 — AI 에이전트를 얼마나 감독할지는 사용자가 결정합니다.
- **터미널 통합** — AI 명령은 활성 터미널 탭에서 직접 실행되며, 특정 탭에 고정하거나 활성 탭을 따라가도록 설정할 수 있습니다. 분할 패인에서 나란히 협업할 수 있고, 각 패인은 자체 터미널 컨텍스트를 가집니다.
- **MCP 서버** — 외부 AI 에이전트(Claude Code, Codex, Gemini CLI 등)가 저장된 연결에서 명령을 실행하고 파일을 전송할 수 있는 MCP 서버를 내장하고 있습니다 — 자격 증명은 앱 밖으로 나가지 않습니다.
- **스킬 및 명령** — 재사용 가능한 스킬 워크플로와 프롬프트 템플릿 명령을 AI 입력창에서 `/`로 붙여 사용할 수 있으며, AI가 새로운 스킬을 직접 저장할 수도 있습니다.
- **스마트 자동 완성** — 명령 히스토리와 AI 기능을 기반으로 입력하는 동안 실시간으로 자동 완성합니다.

### 개인화

연결 관리, 분할 패인, 클라우드 동기화, 테마 및 다국어 — 나만의 방식으로 나만의 터미널을.

- **연결 관리자** — 서버 연결을 그룹화하고 빠르게 검색, 생성, 일괄 작업할 수 있습니다.
- **워크스페이스** — 터미널 탭을 콘텐츠 영역으로 드래그해 자유롭게 분할하고 하나의 워크스페이스로 조합할 수 있습니다. 패인 가장자리를 드래그해 크기 조정과 재배치도 가능합니다.
- **클라우드 동기화** — Git 저장소, WebDAV, SFTP, S3, SMB의 다섯 가지 모드로 암호화된 클라우드 동기화를 지원합니다 — 데이터 유실이나 유출 걱정 없이 여러 기기에서 작업을 원활하게 이어갈 수 있습니다.
- **커스텀 키 바인딩** — 모든 동작에 단축키를 자유롭게 할당해 완전한 키보드 중심 조작이 가능하며, 손을 키보드에서 뗄 필요가 없습니다.
- **테마 및 국제화** — 28가지 터미널 테마, 3가지 UI 테마(다크 / 딥 블루 / 라이트), 커스텀 배경 이미지를 지원합니다. UI는 11개 언어를 지원합니다: 영어, 한국어, 일본어, 중국어(간체), 중국어(번체), 독일어, 스페인어, 프랑스어, 러시아어, 포르투갈어, 베트남어.

## 지원 프로토콜

| 분류 | 프로토콜 | 설명 |
|----------|----------|-------------|
| 터미널 | SSH | 원격 서버 셸 관리 |
| 터미널 | Telnet | 레거시 장비 및 임베디드 시스템용 원격 터미널 |
| 터미널 | Mosh | 지연이 크거나 불안정한 네트워크에서의 서버 연결 |
| 터미널 | Serial | 보레이트 등 파라미터를 설정할 수 있는 시리얼 포트 터미널 |
| 터미널 | Raw TCP | 일반 소켓을 열어 원시 바이트를 송수신하는 Raw TCP 콘솔 |
| 터미널 | Local | PowerShell, CMD, Git Bash 등 로컬 셸 |
| 터미널 | WSL | 로컬 터미널로 설치된 WSL 배포판을 엽니다 (Windows 전용) |
| 파일 전송 | SFTP | 서버 파일 관리 및 전송 |
| 파일 전송 | SCP | SFTP를 지원하지 않는 레거시 SSH 서버용 폴백 프로토콜 |
| 파일 전송 | FTP / FTPS | 웹사이트 호스팅, NAS 파일 전송 |
| 파일 전송 | SMB | Windows 공유 폴더, NAS 파일 접근 |
| 파일 전송 | WebDAV | WebDAV 서버 파일 관리 |
| 파일 전송 | S3 | Amazon S3 호환 오브젝트 스토리지 |
| 파일 전송 | Zmodem | rz/sz 명령을 통한 터미널 내 파일 전송 |
| 원격 데스크톱 | RDP | Windows 서버 원격 데스크톱 관리 |
| 원격 데스크톱 | VNC | Linux 서버 원격 제어 |
| 원격 데스크톱 | SPICE | KVM/QEMU 가상 머신 관리 |
| 원격 데스크톱 | X11 | X Window 포워딩 |
| 데이터베이스 | MySQL | MySQL 프로토콜: MySQL, MariaDB, TiDB 등 |
| 데이터베이스 | PostgreSQL | PostgreSQL 프로토콜: PostgreSQL, CockroachDB 등 |
| 데이터베이스 | Oracle Database | 순수 Go 드라이버를 통한 Oracle Database 연결 |
| 데이터베이스 | SQL Server | 순수 Go 드라이버를 통한 SQL Server 연결 |
| 데이터베이스 | SQLite | 임베디드 로컬 데이터베이스, 데이터베이스 파일을 열어 연결 |
| 데이터베이스 | rqlite | Raft 합의 기반으로 SQLite 위에 구축된 경량 분산 DB |
| 데이터베이스 | Redis | 시각적 키 탐색 및 편집이 가능한 인메모리 키-값 스토어 |
| 데이터베이스 | MongoDB | 트리 탐색, 쿼리 에디터, 인라인 편집을 지원하는 문서 데이터베이스 |
| 데이터베이스 | Elasticsearch | 인덱스 및 문서 관리를 지원하는 분산 검색·분석 엔진 |
| 컨테이너 | Kubernetes | 클러스터 리소스 탐색 및 관리, Pod 로그와 exec, 성능 메트릭 |
| 컨테이너 | Docker | 로컬 머신 또는 SSH를 통한 원격 호스트의 컨테이너 및 이미지 관리 |
| 컨테이너 | Podman | Docker 호환 컨테이너 엔진, 로컬 머신 또는 SSH를 통한 원격 호스트에서 사용 가능 |
| 컨테이너 | nerdctl (containerd) | 네임스페이스 전환을 지원하는 containerd 컨테이너 관리 |
| 컨테이너 | WSLC | Windows WSL2 컨테이너 런타임 (Windows 전용) |

Oracle Database 지원은 순수 Go 드라이버로 구현되었습니다. uniTerm은 Oracle Database, Oracle Instant Client, OJDBC, 월렛 파일 또는 Oracle 브랜드 자산을 번들로 제공하지 않으며, Oracle 라이선스, 자격 증명, 데이터베이스 접근에 대한 책임은 사용자에게 있습니다.

## 스크린샷

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="시작 페이지" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="새 연결" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="AI 어시스턴트가 있는 SSH 터미널" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="워크스페이스" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="SFTP 파일 전송" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="데이터베이스 브라우저" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Kubernetes 관리" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="터미널 배경 이미지" width="45%" loading="eager" />
</p>

## 다운로드

최신 빌드된 바이너리는 [GitHub Releases](https://github.com/ys-ll/uniterm/releases) 또는 [Gitee Releases](https://gitee.com/ys-l/uniterm/releases)에서 받을 수 있습니다:

- **Windows** (amd64 / arm64): 인스톨러 `uniterm-windows-*-installer-*.exe`, 또는 포터블 `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): `uniterm-darwin-*-*.dmg` 다운로드
- **Linux** (amd64 / arm64): `uniterm-linux-*-*.tar.gz`, `.deb`, 또는 `.rpm` 다운로드
- **Android** (arm64): `uniterm-android-arm64-*.apk` 다운로드
- **Docker** (amd64 / arm64): 웹 멀티테넌트 배포 — 브라우저로 접근하는 서버 배포 방식으로 각 사용자에게 격리된 워크스페이스가 제공됩니다. [Docker 배포 가이드](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)를 참고하세요

> **Windows 백신 오탐에 대해**: 이 오픈소스 소프트웨어는 코드 서명 인증서를 구매하지 않았기 때문에, 서명되지 않은 실행 파일이 일부 백신 엔진(예: Windows Defender)에서 오탐을 일으킬 수 있습니다. 이는 Go/Wails 애플리케이션의 알려진 문제입니다([wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308) 참고). 백신에 예외 규칙을 추가하여 허용할 수 있습니다. 공식 오픈소스 채널인 GitHub과 Gitee에서만 다운로드해 주세요. 여전히 악성코드가 걱정된다면 소스 코드를 내려받아 직접 로컬에서 빌드하고 실행할 수 있습니다.

### 패키지 매니저

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

### 런타임 의존성

- **Windows**: WebView2 런타임 (Windows 10+에는 포함되어 있으며, 그 이전 버전은 한 번만 설치하면 됩니다)
- **macOS**: 추가 의존성 없음 (시스템 WebKit 사용)
- **Linux**: `libgtk-3-0` 및 `libwebkit2gtk-4.1-0` (대부분의 데스크톱 배포판에 사전 설치되어 있음)
- **Android**: Android 5.0+ (시스템 WebView 사용, 추가 의존성 없음)
- **Docker**: 호스트에는 Docker Engine만 필요 (compose 배포에는 Docker Compose v2); 이미지에 모든 런타임 의존성이 포함되어 있습니다

## 빠른 워크플로

### SSH 연결

1. 연결 관리자에서 **새 연결**을 클릭합니다
2. 호스트, 포트, 인증 방식(비밀번호 또는 개인 키)을 입력합니다
3. **연결**을 클릭하여 SSH 터미널 세션을 엽니다

### AI 에이전트

1. 설정으로 이동해 **AI 공급자**(API 엔드포인트, 모델, 키)를 구성합니다
2. 터미널 탭(SSH 또는 로컬)을 엽니다
3. AI 사이드바 채팅을 열고 작업을 입력하면 AI 에이전트가 터미널에서 직접 명령을 실행합니다

### SFTP 파일 전송

1. 연결 관리자에서 SSH 연결을 **마우스 오른쪽 버튼으로 클릭**합니다
2. **SFTP 연결**을 선택합니다
3. 듀얼 패인 파일 관리자에서 파일을 탐색, 업로드, 다운로드, 드래그 앤 드롭할 수 있습니다

## 기술 스택

| 레이어 | 기술 |
|-------|-----------|
| 데스크톱 프레임워크 | Wails v3 |
| 백엔드 | Go |
| 프론트엔드 | Vue 3 + Pinia + Element Plus |
| 터미널 | xterm.js |
| AI 프로토콜 | Anthropic Messages API / OpenAI Chat Completions API |

## 소스에서 빌드하기

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`으로 설치)
- macOS에는 Xcode Command Line Tools가 필요합니다
- Linux에는 `libgtk-3-dev`와 `libwebkit2gtk-4.1-dev`가 필요합니다
- Android 빌드에는 추가로 JDK 21과 Android SDK/NDK가 필요합니다 (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## 프로젝트 구조

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

## 로드맵

현재 프로토콜 / 기능 상태와 향후 계획은 [ROADMAP.md](ROADMAP.md)를 참고하세요. 릴리스별 이력은 [CHANGELOG.md](CHANGELOG.md)를 참고하세요.

## 이 프로젝트에 스타를

uniTerm이 도움이 되었다면 ⭐ Star를 눌러 주세요 — 프로젝트에 가장 큰 격려가 되며, 더 많은 사람이 이 프로젝트를 발견하는 데 도움이 됩니다.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## 피드백 및 기여

이 프로젝트는 저자가 여가 시간에 유지 관리하는 개인 취미 프로젝트로, 상업화나 후원 계획은 없습니다 — 같은 관심사를 가진 분이라면 누구나 참여, 토론, 공동 개발을 환영합니다.

이슈, 제안, 피드백은 [GitHub Issues](https://github.com/ys-ll/uniterm/issues)에서 언제든 환영하며, [Pull Request](https://github.com/ys-ll/uniterm/pulls)를 통한 코드 기여도 언제든 환영합니다. 터미널 환경과 사용 시나리오가 매우 다양하기 때문에 저자가 테스트에서 모든 경우를 커버할 수는 없습니다 — 문제를 만나면 주저하지 말고 프로젝트를 포크하여 직접 기여해 주세요. 이 프로젝트는 바이브 코딩을 환영합니다.

코드와 개선에 기여한 아래 분들, 그리고 이슈를 보고하고 제안을 공유해 주신 모든 분들께 감사드립니다 — 여러분 덕분에 uniTerm은 더 나아집니다 ❤️

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

## 라이선스

Apache 2.0
