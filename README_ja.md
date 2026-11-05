<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>SSH、RDP、SFTP、データベース、Kubernetes など、30 以上のプロトコルに対応したオールインワンのターミナル<br>複数ターンのシェルコマンドを自律的に実行する AI エージェントを内蔵</p>
  <p><a href="https://uniterm.net">🌐 ホームページ</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 ユーザーガイド</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; 日本語

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

## 目次

- [機能](#機能)
- [対応プロトコル](#対応プロトコル)
- [スクリーンショット](#スクリーンショット)
- [ダウンロード](#ダウンロード)
- [クイックワークフロー](#クイックワークフロー)
- [技術スタック](#技術スタック)
- [ソースからのビルド](#ソースからのビルド)
- [プロジェクトの構成](#プロジェクトの構成)
- [ロードマップ](#ロードマップ)
- [このプロジェクトにスターを](#このプロジェクトにスターを)
- [フィードバックとコントリビュート](#フィードバックとコントリビュート)
- [ライセンス](#ライセンス)

## 機能

### フル機能のターミナル

リモートターミナル、ローカルとシリアルのターミナル、ファイル転送、リモートデスクトップ、データベース、コンテナ、サーバーの監視まで、リモートアクセスに必要なものをすべてカバーします。

- **リモートターミナル** — パスワードまたは鍵認証による SSH / Telnet / Mosh / Raw TCP。SSH トンネルのポートフォワーディングも備えており、どの接続も SSH の踏み台ホストを経由させられます。
- **ローカルとシリアルのターミナル** — PowerShell / CMD / Git Bash / WSL に加えて、ボーレート、データビット、ストップビット、パリティ、ローカルエコーを設定できるシリアル接続。
- **ファイル転送** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem。2 ペインでのブラウジングと、SSH のターミナルでの `rz`/`sz` に対応しています。
- **リモートデスクトップ** — RDP（Windows のリモートデスクトップ）、VNC（Linux のリモート操作）、SPICE（KVM/QEMU の仮想マシン）、X11（X Window のフォワーディング）
- **データベースのクライアント** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch。AI による自然言語でのクエリと、データの行の編集に対応しています。
- **Kubernetes とコンテナ** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC。クラスターのリソースとイメージの管理、Pod のログと exec のターミナル、CPU/メモリーのメトリクスに対応しています。
- **サーバーの監視** — CPU、メモリー、ディスク、ネットワーク、プロセス、ポート、ネットワークインターフェースをリアルタイムで表示します。

### AI エージェント

ターミナルの中で、複数ターンのシェルコマンドを自分で計画して実行する、自律的な AI エージェントです。

- **自律的な AI エージェント** — AI エージェントは、手動の介入なしに、複数ラウンドのシェルコマンドにわたって計画、実行、結果の観察、繰り返しを行えます。Anthropic/OpenAI 互換の API を使ったサイドバーのチャットで、Claude、GPT、その他の対応モデルに対応しています。
- **柔軟な実行モード** — バイパス、危険なもののみ、危険なもの + 書き込み、すべて確認から選べます。AI エージェントをどれだけ監督するかを、あなたが決められます。
- **ターミナルとの統合** — AI のコマンドはアクティブなターミナルのタブで直接実行され、特定のタブに固定するか、アクティブなタブに追従させるかを選べます。分割したペインで並べて協働でき、それぞれのペインが独自のターミナルのコンテキストを持ちます。
- **MCP サーバー** — 外部の AI エージェント（Claude Code、Codex、Gemini CLI など）が、保存した接続でコマンドを実行したりファイルを転送したりできる、MCP サーバーを内蔵しています。認証情報がアプリの外に出ることはありません。
- **スキルとコマンド** — 再利用できるスキルのワークフローと、プロンプトのテンプレートのコマンドを、AI の入力欄で `/` を使って添付できます。AI が自分で新しいスキルを保存することもできます。
- **スマートな補完** — コマンドの履歴と AI の能力に基づいて、入力中にリアルタイムで補完します。

### パーソナライズ

接続の管理、分割ペイン、クラウド同期、テーマと多言語対応。あなたのターミナルを、あなたのやり方で。

- **接続マネージャー** — サーバーへの接続をグループ化、すばやく検索、作成、一括操作できます。
- **ワークスペース** — ターミナルのタブをコンテンツの領域にドラッグして自由に分割し、ワークスペースとして組み合わせられます。パネルの端をドラッグして、サイズの変更や並べ替えもできます。
- **クラウド同期** — Git のリポジトリ、WebDAV、SFTP、S3、SMB の 5 つのモードで暗号化されたクラウド同期を行えます。データの消失や漏洩を心配することなく、デバイスをまたいでシームレスに作業を再開できます。
- **カスタムのキーバインド** — あらゆる操作にキーボードショートカットを自由に割り当てられ、キーボードから手を離すことなく、すべてをキーボードで操作できます。
- **テーマと国際化** — 28 種類のターミナルのテーマと 3 種類の UI のテーマ（ダーク / ディープブルー / ライト）、カスタマイズ可能な背景画像。UI は 11 言語に対応しています：簡体字中国語、繁体字中国語、英語、日本語、韓国語、ドイツ語、スペイン語、フランス語、ロシア語、ポルトガル語、ベトナム語。

## 対応プロトコル

| カテゴリ | プロトコル | 説明 |
|----------|----------|-------------|
| ターミナル | SSH | リモートサーバーのシェルの管理 |
| ターミナル | Telnet | レガシーなデバイスや組み込みシステム向けのリモートターミナル |
| ターミナル | Mosh | 遅延の大きい、または途切れがちなネットワークでのサーバーへの接続 |
| ターミナル | Serial | ボーレートなどのパラメーターを設定できるシリアルポートのターミナル |
| ターミナル | Raw TCP | プレーンなソケットを開いて生のバイトを送受信する Raw TCP のコンソール |
| ターミナル | Local | PowerShell、CMD、Git Bash などのローカルのシェル |
| ターミナル | WSL | ローカルのターミナルで、インストール済みの WSL のディストリビューションを開きます（Windows のみ） |
| ファイル転送 | SFTP | サーバーのファイルの管理と転送 |
| ファイル転送 | SCP | SFTP に対応していないレガシーな SSH サーバー向けのフォールバックのプロトコル |
| ファイル転送 | FTP / FTPS | Web サイトのホスティング、NAS とのファイル転送 |
| ファイル転送 | SMB | Windows の共有フォルダー、NAS のファイルへのアクセス |
| ファイル転送 | WebDAV | WebDAV サーバーのファイルの管理 |
| ファイル転送 | S3 | Amazon S3 互換のオブジェクトストレージ |
| ファイル転送 | Zmodem | rz/sz コマンドによるターミナル内でのファイル転送 |
| リモートデスクトップ | RDP | Windows サーバーのリモートデスクトップの管理 |
| リモートデスクトップ | VNC | Linux サーバーのリモート操作 |
| リモートデスクトップ | SPICE | KVM/QEMU の仮想マシンの管理 |
| リモートデスクトップ | X11 | X Window のフォワーディング |
| データベース | MySQL | MySQL プロトコル：MySQL、MariaDB、TiDB など |
| データベース | PostgreSQL | PostgreSQL プロトコル：PostgreSQL、CockroachDB など |
| データベース | Oracle Database | 純粋な Go のドライバーによる Oracle Database への接続 |
| データベース | SQL Server | 純粋な Go のドライバーによる SQL Server への接続 |
| データベース | SQLite | 組み込みのローカルデータベース。データベースのファイルを開いて接続します |
| データベース | rqlite | Raft の合意アルゴリズムを使い、SQLite の上に構築された軽量な分散 DB |
| データベース | Redis | キーの閲覧と編集を視覚的に行えるインメモリーのキーバリューストア |
| データベース | MongoDB | ツリーでの閲覧、クエリエディター、インラインでの編集に対応したドキュメントデータベース |
| データベース | Elasticsearch | インデックスとドキュメントの管理に対応した分散型の検索・分析エンジン |
| コンテナ | Kubernetes | クラスターのリソースの閲覧と管理、Pod のログと exec、パフォーマンスのメトリクス |
| コンテナ | Docker | ローカルのマシン、または SSH 経由のリモートホスト上のコンテナとイメージの管理 |
| コンテナ | Podman | Docker 互換のコンテナエンジン。ローカルのマシン、または SSH 経由のリモートホストで使えます |
| コンテナ | nerdctl (containerd) | 名前空間の切り替えに対応した containerd のコンテナの管理 |
| コンテナ | WSLC | Windows の WSL2 のコンテナランタイム（Windows のみ） |

Oracle Database への対応は、純粋な Go のドライバーで実装されています。uniTerm は、Oracle Database、Oracle Instant Client、OJDBC、ウォレットのファイル、Oracle のブランドのアセットを同梱していません。Oracle のライセンス、認証情報、データベースへのアクセスについては、ユーザー自身が責任を負います。

## スクリーンショット

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="スタートページ" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="新規接続" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="AI アシスタント付きの SSH ターミナル" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="ワークスペース" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="SFTP でのファイル転送" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="データベースのブラウザー" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Kubernetes の管理" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="ターミナルの背景画像" width="45%" loading="eager" />
</p>

## ダウンロード

ビルド済みの最新のバイナリは、[GitHub Releases](https://github.com/ys-ll/uniterm/releases) または [Gitee Releases](https://gitee.com/ys-l/uniterm/releases) から入手できます：

- **Windows**（amd64 / arm64）：インストーラー `uniterm-windows-*-installer-*.exe`、またはポータブル版 `uniterm-windows-*-portable-*.zip`
- **macOS**（Apple Silicon arm64 / Intel amd64）：`uniterm-darwin-*-*.dmg` をダウンロード
- **Linux**（amd64 / arm64）：`uniterm-linux-*-*.tar.gz`、`.deb`、または `.rpm` をダウンロード
- **Android**（arm64）：`uniterm-android-arm64-*.apk` をダウンロード
- **Docker**（amd64 / arm64）：Web のマルチテナントのデプロイ。ブラウザからアクセスするサーバーのデプロイで、各ユーザーに独立したワークスペースが割り当てられます。[Docker のデプロイガイド](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)をご覧ください

> **Windows のウイルス対策ソフトによる誤検知について**：このオープンソースのソフトウェアはコード署名の証明書を購入していないため、署名のない実行ファイルが、一部のウイルス対策エンジン（Windows Defender など）で誤検知されることがあります。これは Go/Wails のアプリケーションでよく知られている問題です（[wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308) を参照）。ウイルス対策ソフトに除外のルールを追加して許可できます。ダウンロードは、公式のオープンソースのチャネルである GitHub と Gitee からのみ行ってください。それでもマルウェアが心配な場合は、ソースコードをダウンロードして、ご自身でローカルにビルドして実行することもできます。

### パッケージマネージャー

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

### 実行時の依存関係

- **Windows**：WebView2 のランタイム（Windows 10 以降には含まれています。それより古いバージョンでは一度だけインストールが必要です）
- **macOS**：追加の依存関係はありません（システムの WebKit を使います）
- **Linux**：`libgtk-3-0` と `libwebkit2gtk-4.1-0`（ほとんどのデスクトップのディストリビューションにプリインストールされています）
- **Android**：Android 5.0 以降（システムの WebView を使い、追加の依存関係はありません）
- **Docker**：ホストに必要なのは Docker Engine だけです（compose でデプロイする場合は Docker Compose v2）。イメージには、実行時の依存関係がすべて含まれています

## クイックワークフロー

### SSH での接続

1. 接続マネージャーで **新規接続** をクリックします
2. ホスト、ポート、認証方法（パスワードまたは秘密鍵）を入力します
3. **接続** をクリックして、SSH のターミナルのセッションを開きます

### AI エージェント

1. 設定に移動して、**AI プロバイダー**（API のエンドポイント、モデル、キー）を設定します
2. ターミナルのタブ（SSH またはローカル）を開きます
3. AI のサイドバーのチャットを開き、タスクを入力すると、AI エージェントがターミナルで直接コマンドを実行します

### SFTP でのファイル転送

1. 接続マネージャーで、SSH の接続を **右クリック** します
2. **SFTP で接続** を選択します
3. 2 ペインのファイルマネージャーで、ファイルの閲覧、アップロード、ダウンロード、ドラッグ＆ドロップを行えます

## 技術スタック

| レイヤー | 技術 |
|-------|-----------|
| デスクトップのフレームワーク | Wails v3 |
| バックエンド | Go |
| フロントエンド | Vue 3 + Pinia + Element Plus |
| ターミナル | xterm.js |
| AI のプロトコル | Anthropic Messages API / OpenAI Chat Completions API |

## ソースからのビルド

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0（`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0` でインストール）
- macOS では Xcode Command Line Tools が必要です
- Linux では `libgtk-3-dev` と `libwebkit2gtk-4.1-dev` が必要です
- Android のビルドには、さらに JDK 21 と Android SDK/NDK が必要です（`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`）

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## プロジェクトの構成

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

## ロードマップ

現在のプロトコル / 機能の状況と、今後の予定については、[ROADMAP.md](ROADMAP.md) をご覧ください。リリースごとの履歴については、[CHANGELOG.md](CHANGELOG.md) をご覧ください。

## このプロジェクトにスターを

uniTerm が役に立ったら、ぜひ ⭐ Star を付けてください。プロジェクトにとって何よりの励みになり、より多くの人がこのプロジェクトを見つけられるようになります。

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## フィードバックとコントリビュート

これは作者が空き時間にメンテナンスしている個人の趣味のプロジェクトで、商業化やスポンサーの予定はありません。同じ興味を持つ方なら誰でも、参加、議論、共同開発を心から歓迎します。

Issue、提案、フィードバックは [GitHub Issues](https://github.com/ys-ll/uniterm/issues) でいつでも歓迎しており、[Pull Request](https://github.com/ys-ll/uniterm/pulls) によるコードのコントリビュートもいつでも歓迎です。ターミナルの環境や利用シーンは非常に多様なため、作者がテストであらゆるケースをカバーすることはできません。何か問題に遭遇したら、遠慮なくプロジェクトをフォークして、直接コントリビュートしてください。このプロジェクトは Vibe Coding を歓迎しています。

コードや改善を提供してくださった以下の方々、そして問題を報告し、提案を共有してくださったすべての方に感謝します。皆さんのおかげで uniTerm はより良くなっています ❤️

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

## ライセンス

Apache 2.0
