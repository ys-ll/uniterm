<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>All-in-One-Terminal mit Unterstützung für über 30 Protokolle — SSH, RDP, SFTP, Datenbanken, Kubernetes und mehr<br>Mit einem integrierten KI-Agenten, der Shell-Befehle über mehrere Runden hinweg autonom ausführt</p>
  <p><a href="https://uniterm.net">🌐 Webseite</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 Benutzerhandbuch</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; Deutsch &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## Inhaltsverzeichnis

- [Funktionen](#funktionen)
- [Unterstützte Protokolle](#unterstützte-protokolle)
- [Screenshots](#screenshots)
- [Download](#download)
- [Schnellstarts](#schnellstarts)
- [Technologie-Stack](#technologie-stack)
- [Build aus dem Quellcode](#build-aus-dem-quellcode)
- [Projektstruktur](#projektstruktur)
- [Roadmap](#roadmap)
- [Dieses Projekt unterstützen](#dieses-projekt-unterstützen)
- [Feedback & Mitwirken](#feedback--mitwirken)
- [Lizenz](#lizenz)

## Funktionen

### Vollwertiges Terminal

Remote-Terminal, lokale und serielle Terminals, Dateiübertragung, Remote-Desktop, Datenbanken, Container und Serverüberwachung — deckt sämtliche Anforderungen an den Remote-Zugriff ab.

- **Terminal** — SSH / Telnet / Mosh / Raw TCP / Seriell, plus lokale Shells (PowerShell / CMD / Git Bash / WSL).
- **Dateiübertragung** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem mit Zweifenster-Browsing und `rz`/`sz`-Unterstützung in SSH-Terminals.
- **Remote-Desktop** — RDP (Windows-Remote-Desktop), VNC (Fernsteuerung von Linux), SPICE (KVM/QEMU-VMs), X11 (X-Window-Weiterleitung)
- **Datenbank-Client** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch, mit KI-gestützten Abfragen in natürlicher Sprache und Bearbeitung von Datenzeilen.
- **Kubernetes & Container** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC, mit Verwaltung von Cluster-Ressourcen und Images, Pod-Protokollen und Exec-Terminals sowie CPU-/Speicher-Metriken.
- **Serverüberwachung** — CPU, Speicher, Festplatten, Netzwerk, Prozesse, Ports und Netzwerkschnittstellen in Echtzeit.

### KI-Agent

Autonomer KI-Agent, der selbstständig Shell-Befehle über mehrere Runden hinweg plant und direkt in Ihrem Terminal ausführt.

- **Autonomer KI-Agent** — Der KI-Agent kann über mehrere Runden von Shell-Befehlen hinweg planen, ausführen, Ergebnisse auswerten und iterieren — ohne manuelles Eingreifen; Sidebar-Chat mit Anthropic/OpenAI-kompatibler API, unterstützt Claude, GPT und andere kompatible Modelle.
- **Flexible Ausführungsmodi** — Umgehung, nur gefährliche, gefährliche + schreibende oder alle Befehle bestätigen — Sie bestimmen, wie viel Kontrolle der KI-Agent benötigt.
- **Terminal-Integration** — KI-Befehle werden direkt im aktiven Terminal-Tab ausgeführt, optional an einen bestimmten Tab angeheftet oder dem aktiven Tab folgend. Zusammenarbeit Seite an Seite in geteilten Bereichen, jeder mit seinem eigenen Terminal-Kontext.
- **MCP-Server** — Integrierter MCP-Server, mit dem externe KI-Agenten (Claude Code, Codex, Gemini CLI, ...) Befehle ausführen und Dateien auf gespeicherten Verbindungen übertragen können — Zugangsdaten verlassen die App nie.
- **Skills & Befehle** — Wiederverwendbare Skill-Workflows und Befehle auf Prompt-Vorlagenbasis, angehängt mit `/` in der KI-Eingabe; die KI kann auch selbst neue Skills speichern.
- **Intelligente Vervollständigung** — Vervollständigung in Echtzeit während der Eingabe, basierend auf dem Befehlsverlauf und KI-Fähigkeiten.

### Personalisierung

Verbindungsverwaltung, geteilte Bereiche, Cloud-Sync, Themes & Internationalisierung — Ihr Terminal, Ihre Art.

- **Verbindungsmanager** — Serververbindungen gruppieren, durchsuchen und im Stapel verwalten; verschlüsselter Import/Export sowie Import aus anderen Terminal-Apps (MobaXterm, Xshell usw.).
- **Workspace** — Terminal-Tabs per Drag & Drop in den Inhaltsbereich ziehen, um sie frei zu teilen und zu einem Workspace zu kombinieren; Panelränder ziehen, um Größe und Anordnung anzupassen.
- **Netzwerk-Toolbox** — Netzwerkscan, SSH-Port-Weiterleitungs-Tunnel und lokale Dienste zum Betreiben temporärer Server für andere Geräte.
- **Cloud-Sync** — Verschlüsselte Cloud-Synchronisierung in fünf Modi: Git-Repository, WebDAV, SFTP, S3 und SMB — keine Sorge um Datenverlust oder Datenlecks, und nahtloses Weiterarbeiten über Geräte hinweg.
- **Themes & Internationalisierung** — 28 Terminal-Themes sowie 3 UI-Themes (Dark / Deep Blue / Light) und ein anpassbares Hintergrundbild; UI in 11 Sprachen: Englisch, Deutsch, Spanisch, Französisch, Russisch, Portugiesisch, Japanisch, Koreanisch, vereinfachtes Chinesisch, traditionelles Chinesisch, Vietnamesisch.

## Unterstützte Protokolle

| Kategorie | Protokoll | Beschreibung |
|----------|----------|-------------|
| Terminal | SSH | Shell-Verwaltung entfernter Server |
| Terminal | Telnet | Remote-Terminal für ältere Geräte und Embedded-Systeme |
| Terminal | Mosh | Serververbindungen über Netzwerke mit hoher Latenz oder instabiler Verbindung |
| Terminal | Seriell | Serielles Port-Terminal mit konfigurierbarer Baudrate und anderen Parametern |
| Terminal | Raw TCP | Raw-TCP-Konsole, die einen reinen Socket öffnet und rohe Bytes sendet und empfängt |
| Terminal | Lokal | PowerShell, CMD, Git Bash und andere lokale Shells |
| Terminal | WSL | Installierte WSL-Distributionen über das lokale Terminal öffnen (nur Windows) |
| Dateiübertragung | SFTP | Dateiverwaltung und -übertragung auf Servern |
| Dateiübertragung | SCP | Fallback-Protokoll für ältere SSH-Server ohne SFTP-Unterstützung |
| Dateiübertragung | FTP / FTPS | Website-Hosting, Dateiübertragung auf NAS |
| Dateiübertragung | SMB | Windows-Freigaben, Dateizugriff auf NAS |
| Dateiübertragung | WebDAV | Dateiverwaltung auf WebDAV-Servern |
| Dateiübertragung | S3 | Amazon-S3-kompatibler Objektspeicher |
| Dateiübertragung | Zmodem | Dateiübertragung im Terminal über rz/sz-Befehle |
| Remote-Desktop | RDP | Remote-Desktop-Verwaltung von Windows-Servern |
| Remote-Desktop | VNC | Fernsteuerung von Linux-Servern |
| Remote-Desktop | SPICE | Verwaltung von KVM/QEMU-VMs |
| Remote-Desktop | X11 | X-Window-Weiterleitung |
| Datenbank | MySQL | MySQL-Protokoll: MySQL, MariaDB, TiDB und mehr |
| Datenbank | PostgreSQL | PostgreSQL-Protokoll: PostgreSQL, CockroachDB und mehr |
| Datenbank | Oracle Database | Oracle-Database-Verbindungen über einen reinen Go-Treiber |
| Datenbank | SQL Server | SQL-Server-Verbindungen über einen reinen Go-Treiber |
| Datenbank | SQLite | Eingebettete lokale Datenbank, Verbindung durch Öffnen der Datenbankdatei |
| Datenbank | rqlite | Leichtgewichtige verteilte DB auf SQLite-Basis mit Raft-Konsens |
| Datenbank | Redis | In-Memory-Schlüssel-Wert-Speicher mit visueller Schlüsselverwaltung und -bearbeitung |
| Datenbank | MongoDB | Dokumentendatenbank mit Baumansicht, Abfrage-Editor und Inline-Bearbeitung |
| Datenbank | Elasticsearch | Verteilte Such- und Analyse-Engine mit Index- und Dokumentenverwaltung |
| Container | Kubernetes | Durchsuchen und Verwaltung von Cluster-Ressourcen, Pod-Protokolle und Exec, Leistungsmetriken |
| Container | Docker | Container- und Image-Verwaltung, lokal oder auf Remote-Hosts über SSH |
| Container | Podman | Docker-kompatible Container-Engine, lokal oder auf Remote-Hosts über SSH |
| Container | nerdctl (containerd) | containerd-Containerverwaltung mit Namespace-Wechsel |
| Container | WSLC | Windows-WSL2-Container-Runtime (nur Windows) |

Die Oracle-Database-Unterstützung ist mit einem reinen Go-Treiber implementiert. uniTerm bündelt weder Oracle Database, Oracle Instant Client, OJDBC, Wallet-Dateien noch Oracle-Marken-Assets; die Oracle-Lizenzen, Zugangsdaten und der Datenbankzugriff liegen in der Verantwortung der Nutzer.

## Screenshots

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="Startseite" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="Neue Verbindung" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="SSH-Terminal mit KI-Assistent" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="Workspace" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="SFTP-Dateiübertragung" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="Datenbank-Browser" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Kubernetes-Verwaltung" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="Terminal-Hintergrundbild" width="45%" loading="eager" />
</p>

## Download

Die neuesten vorgebauten Binärdateien gibt es auf [GitHub Releases](https://github.com/ys-ll/uniterm/releases) oder [Gitee Releases](https://gitee.com/ys-l/uniterm/releases):

- **Windows** (amd64 / arm64): Installer `uniterm-windows-*-installer-*.exe` oder portable Version `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): `uniterm-darwin-*-*.dmg` herunterladen
- **Linux** (amd64 / arm64): `uniterm-linux-*-*.tar.gz`, `.deb` oder `.rpm` herunterladen
- **Android** (arm64): `uniterm-android-arm64-*.apk` herunterladen
- **Docker** (amd64 / arm64): Web-basierte Multi-Tenant-Bereitstellung — eine serverseitige Bereitstellung, die über den Browser zugänglich ist und bei der jeder Nutzer einen isolierten Workspace erhält, siehe die [Docker-Bereitstellungsanleitung](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **Zu falschen Viruswarnungen unter Windows**: Da diese Open-Source-Software kein Code-Signing-Zertifikat erworben hat, kann die unsignierte ausführbare Datei bei manchen Antivirus-Engines (z. B. Windows Defender) Fehlalarme auslösen. Dies ist ein bekanntes Problem von Go-/Wails-Anwendungen (siehe [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). Sie können in Ihrer Antivirus-Software eine Ausnahmeregel hinzufügen, um sie zuzulassen. Bitte laden Sie ausschließlich über die offiziellen Open-Source-Kanäle herunter — GitHub und Gitee. Falls Sie weiterhin Bedenken wegen Schadsoftware haben, können Sie den Quellcode herunterladen und selbst lokal bauen und ausführen.

### Paketmanager

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

### Laufzeitabhängigkeiten

- **Windows**: WebView2-Runtime (in Windows 10+ enthalten; ältere Versionen benötigen eine einmalige Installation)
- **macOS**: Keine zusätzlichen Abhängigkeiten (nutzt das System-WebKit)
- **Linux**: `libgtk-3-0` und `libwebkit2gtk-4.1-0` (auf den meisten Desktop-Distributionen vorinstalliert)
- **Android**: Android 5.0+ (nutzt die System-WebView, keine zusätzlichen Abhängigkeiten)
- **Docker**: nur Docker Engine auf dem Host (Docker Compose v2 für Compose-Bereitstellungen); das Image enthält alle Laufzeitabhängigkeiten

## Schnellstarts

### SSH-Verbindung

1. Im Verbindungsmanager auf **Neue Verbindung** klicken
2. Host, Port und Authentifizierung (Passwort oder privater Schlüssel) eintragen
3. Auf **Verbinden** klicken, um eine SSH-Terminal-Sitzung zu öffnen

### KI-Agent

1. Zu den Einstellungen gehen und den **KI-Anbieter** konfigurieren (API-Endpunkt, Modell und Schlüssel)
2. Einen Terminal-Tab öffnen (SSH oder lokal)
3. Den KI-Sidebar-Chat öffnen — Aufgabe eintippen, und der KI-Agent führt die Befehle direkt in Ihrem Terminal aus

### SFTP-Dateiübertragung

1. Im Verbindungsmanager eine SSH-Verbindung **mit der rechten Maustaste anklicken**
2. **Mit SFTP verbinden** auswählen
3. Dateien im Zweifenster-Dateimanager durchsehen, hochladen, herunterladen und per Drag & Drop verschieben

## Technologie-Stack

| Schicht | Technologie |
|-------|-----------|
| Desktop-Framework | Wails v3 |
| Backend | Go |
| Frontend | Vue 3 + Pinia + Element Plus |
| Terminal | xterm.js |
| KI-Protokoll | Anthropic Messages API / OpenAI Chat Completions API |

## Build aus dem Quellcode

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (Installation mit `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`)
- macOS benötigt die Xcode Command Line Tools
- Linux benötigt `libgtk-3-dev` und `libwebkit2gtk-4.1-dev`
- Android-Builds benötigen zusätzlich JDK 21 und das Android SDK/NDK (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## Projektstruktur

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

## Roadmap

Den aktuellen Stand der Protokolle / Funktionen und was als Nächstes geplant ist, finden Sie in [ROADMAP.md](ROADMAP.md). Die Historie der einzelnen Releases finden Sie in [CHANGELOG.md](CHANGELOG.md).

## Dieses Projekt unterstützen

Wenn uniTerm Ihnen weiterhilft, gönnen Sie ihm gerne einen ⭐ Star — das ist die beste Ermutigung für das Projekt und hilft mehr Menschen, es zu entdecken.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## Feedback &amp; Mitwirken

Dies ist ein privates Hobby-Projekt, das der Autor in seiner Freizeit pflegt, ohne kommerzielle Ambitionen oder Sponsoring-Pläne — alle, die dieselbe Begeisterung teilen, sind herzlich willkommen mitzumachen, zu diskutieren und gemeinsam zu bauen.

Issues, Vorschläge und Feedback sind jederzeit willkommen unter [GitHub Issues](https://github.com/ys-ll/uniterm/issues), und Code-Beiträge über [Pull Requests](https://github.com/ys-ll/uniterm/pulls) sind immer gerne gesehen. Aufgrund der großen Vielfalt an Terminal-Umgebungen und Nutzungsszenarien kann der Autor nicht jeden erdenklichen Fall in Tests abdecken — falls Sie auf Probleme stoßen, forken Sie das Projekt gerne und tragen Sie direkt bei. Dieses Projekt begrüßt Vibe Coding.

Dank an die folgenden Personen für Code-Beiträge und Verbesserungen sowie an alle, die Issues gemeldet und Vorschläge geteilt haben — Sie helfen, uniTerm besser zu machen ❤️

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

## Lizenz

Apache 2.0
