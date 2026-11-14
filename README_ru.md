<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>Универсальный терминал «всё в одном» с поддержкой более 30 протоколов — SSH, RDP, SFTP, базы данных, Kubernetes и другие<br>Со встроенным AI-агентом, который автономно выполняет многошаговые команды shell</p>
  <p><a href="https://uniterm.net">🌐 Сайт</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 Руководство пользователя</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; Русский &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## Содержание

- [Возможности](#возможности)
- [Поддерживаемые протоколы](#поддерживаемые-протоколы)
- [Скриншоты](#скриншоты)
- [Загрузка](#загрузка)
- [Быстрые сценарии работы](#быстрые-сценарии-работы)
- [Технологический стек](#технологический-стек)
- [Сборка из исходников](#сборка-из-исходников)
- [Структура проекта](#структура-проекта)
- [План развития](#план-развития)
- [Поставьте звёздочку проекту](#поставьте-звёздочку-проекту)
- [Обратная связь и участие в разработке](#обратная-связь-и-участие-в-разработке)
- [Лицензия](#лицензия)

## Возможности

### Полнофункциональный терминал

Удалённый терминал, локальный и последовательный терминал, передача файлов, удалённый рабочий стол, базы данных, контейнеры и мониторинг серверов — всё для любых задач удалённого доступа.

- **Терминал** — SSH / Telnet / Mosh / Raw TCP / Serial, а также локальные оболочки (PowerShell / CMD / Git Bash / WSL).
- **Передача файлов** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem с двухпанельным просмотром и поддержкой `rz`/`sz` в SSH-терминалах.
- **Удалённый рабочий стол** — RDP (удалённый рабочий стол Windows), VNC (удалённое управление Linux), SPICE (виртуальные машины KVM/QEMU), X11 (проброс X Window)
- **Клиент баз данных** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch, с запросами на естественном языке на базе AI и редактированием строк данных.
- **Kubernetes и контейнеры** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC, с управлением ресурсами кластера и образами, логами Pod и exec-терминалами, а также метриками CPU/памяти.
- **Мониторинг серверов** — CPU, память, диски, сеть, процессы, порты и сетевые интерфейсы в реальном времени.

### AI-агент

Автономный AI-агент, который самостоятельно планирует и выполняет многошаговые команды shell прямо в вашем терминале.

- **Автономный AI-агент** — AI-агент может планировать, выполнять, наблюдать результаты и повторять итерации на протяжении нескольких раундов команд shell без вмешательства человека; чат в боковой панели через API, совместимый с Anthropic/OpenAI, с поддержкой Claude, GPT и других совместимых моделей.
- **Гибкие режимы выполнения** — Без ограничений, только опасные команды, опасные + запись или подтверждение всего — вы сами решаете, насколько строго контролировать AI-агента.
- **Интеграция с терминалом** — Команды AI выполняются прямо в активной вкладке терминала, при желании их можно закрепить за конкретной вкладкой или следовать за активной. Совместная работа бок о бок в разделённых панелях, каждая со своим контекстом терминала.
- **MCP-сервер** — Встроенный MCP-сервер, позволяющий внешним AI-агентам (Claude Code, Codex, Gemini CLI, ...) выполнять команды и передавать файлы на сохранённых подключениях — учётные данные никогда не покидают приложение.
- **Навыки и команды** — Переиспользуемые рабочие процессы-навыки и команды на основе шаблонов промптов, прикрепляемые через `/` в поле ввода AI; AI также может сохранять новые навыки самостоятельно; также можно подключать внешние MCP-серверы, расширяя набор инструментов ИИ.
- **Умное автозавершение** — Автодополнение в реальном времени во время набора, на основе истории команд и возможностей AI.

### Персонализация

Управление подключениями, разделённые панели, облачная синхронизация, темы и локализация — ваш терминал, ваши правила.

- **Менеджер подключений** — Группировка, поиск и массовое управление серверными подключениями; шифрованный импорт/экспорт и импорт из других терминальных приложений (MobaXterm, Xshell и др.).
- **Рабочее пространство** — Перетаскивайте вкладки терминала в область содержимого, чтобы свободно разделять экран и объединять их в рабочее пространство; перетаскивайте края панелей, чтобы менять размер и расположение.
- **Сетевой инструментарий** — Сканирование сети, туннели переадресации портов SSH и локальные сервисы для запуска временных серверов для других устройств.
- **Облачная синхронизация** — Зашифрованная облачная синхронизация в пяти режимах: Git-репозиторий, WebDAV, SFTP, S3 и SMB — никаких переживаний о потере данных или утечках, и без труда продолжайте работу на другом устройстве.
- **Темы и локализация** — 28 тем терминала плюс 3 темы интерфейса (Тёмная / Тёмно-синяя / Светлая) и настраиваемое фоновое изображение; интерфейс на 11 языках: упрощённый китайский, традиционный китайский, английский, японский, корейский, немецкий, испанский, французский, русский, португальский, вьетнамский.

## Поддерживаемые протоколы

| Категория | Протокол | Описание |
|----------|----------|-------------|
| Терминал | SSH | Управление shell на удалённых серверах |
| Терминал | Telnet | Удалённый терминал для устаревших устройств и встраиваемых систем |
| Терминал | Mosh | Подключение к серверам в сетях с высокой задержкой или нестабильной связью |
| Терминал | Serial | Терминал последовательного порта с настраиваемой скоростью передачи и другими параметрами |
| Терминал | Raw TCP | Консоль Raw TCP, открывающая обычный сокет и передающая «сырые» байты |
| Терминал | Local | PowerShell, CMD, Git Bash и другие локальные оболочки |
| Терминал | WSL | Открытие установленных дистрибутивов WSL через локальный терминал (только Windows) |
| Передача файлов | SFTP | Управление файлами сервера и передача файлов |
| Передача файлов | SCP | Резервный протокол для старых SSH-серверов, не поддерживающих SFTP |
| Передача файлов | FTP / FTPS | Хостинг сайтов, передача файлов на NAS |
| Передача файлов | SMB | Общие папки Windows, доступ к файлам NAS |
| Передача файлов | WebDAV | Управление файлами на серверах WebDAV |
| Передача файлов | S3 | Объектное хранилище, совместимое с Amazon S3 |
| Передача файлов | Zmodem | Передача файлов внутри терминала командами rz/sz |
| Удалённый рабочий стол | RDP | Управление удалённым рабочим столом серверов Windows |
| Удалённый рабочий стол | VNC | Удалённое управление серверами Linux |
| Удалённый рабочий стол | SPICE | Управление виртуальными машинами KVM/QEMU |
| Удалённый рабочий стол | X11 | Проброс X Window |
| Базы данных | MySQL | Протокол MySQL: MySQL, MariaDB, TiDB и другие |
| Базы данных | PostgreSQL | Протокол PostgreSQL: PostgreSQL, CockroachDB и другие |
| Базы данных | Oracle Database | Подключение к Oracle Database через чистый драйвер на Go |
| Базы данных | SQL Server | Подключение к SQL Server через чистый драйвер на Go |
| Базы данных | SQLite | Встроенная локальная база данных, подключение путём открытия файла базы данных |
| Базы данных | rqlite | Лёгкая распределённая СУБД на основе SQLite с консенсусом Raft |
| Базы данных | Redis | Хранилище «ключ–значение» в памяти с визуальным просмотром и редактированием ключей |
| Базы данных | MongoDB | Документная база данных с просмотром в виде дерева, редактором запросов и редактированием на месте |
| Базы данных | Elasticsearch | Распределённый поисковый и аналитический движок с управлением индексами и документами |
| Контейнеры | Kubernetes | Просмотр и управление ресурсами кластера, логи Pod и exec, метрики производительности |
| Контейнеры | Docker | Управление контейнерами и образами — локально или на удалённых хостах по SSH |
| Контейнеры | Podman | Совместимый с Docker движок контейнеров, локально или на удалённых хостах по SSH |
| Контейнеры | nerdctl (containerd) | Управление контейнерами containerd с переключением пространств имён |
| Контейнеры | WSLC | Среда выполнения контейнеров WSL2 в Windows (только Windows) |

Поддержка Oracle Database реализована с помощью чистого драйвера на Go. uniTerm не поставляет в комплекте Oracle Database, Oracle Instant Client, OJDBC, файлы кошельков или брендовые материалы Oracle; ответственность за лицензии Oracle, учётные данные и доступ к базе данных несут пользователи.

## Скриншоты

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="Стартовая страница" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="Новое подключение" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="SSH-терминал с AI-ассистентом" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="Рабочее пространство" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="Передача файлов по SFTP" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="Обозреватель баз данных" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Управление Kubernetes" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="Фоновое изображение терминала" width="45%" loading="eager" />
</p>

## Загрузка

Последние готовые сборки можно скачать со страницы [GitHub Releases](https://github.com/ys-ll/uniterm/releases) или [Gitee Releases](https://gitee.com/ys-l/uniterm/releases):

- **Windows** (amd64 / arm64): установщик `uniterm-windows-*-installer-*.exe` или портативная версия `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): скачайте `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64): скачайте `uniterm-linux-*-*.tar.gz`, `.deb` или `.rpm`
- **Android** (arm64): скачайте `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64): веб-развёртывание с поддержкой нескольких пользователей — серверное развёртывание, доступное через браузер, где каждый пользователь получает изолированное рабочее пространство; см. [руководство по развёртыванию в Docker](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **О ложных срабатываниях антивирусов в Windows**: поскольку это открытое ПО не приобретало сертификат подписи кода, неподписанный исполняемый файл может вызывать ложные срабатывания в некоторых антивирусных движках (например, Windows Defender). Это известная проблема приложений Go/Wails (см. [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). Вы можете добавить правило исключения в антивирус, чтобы разрешить его. Пожалуйста, скачивайте только из официальных открытых источников — GitHub и Gitee. Если вы всё же опасаетесь вредоносного кода, вы можете скачать исходный код и собрать и запустить его локально самостоятельно.

### Менеджеры пакетов

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

### Зависимости среды выполнения

- **Windows**: среда выполнения WebView2 (входит в состав Windows 10 и новее; в старых версиях требуется однократная установка)
- **macOS**: дополнительных зависимостей нет (используется системный WebKit)
- **Linux**: `libgtk-3-0` и `libwebkit2gtk-4.1-0` (предустановлены в большинстве настольных дистрибутивов)
- **Android**: Android 5.0+ (используется системный WebView, дополнительных зависимостей нет)
- **Docker**: на хосте нужен только Docker Engine (Docker Compose v2 для развёртывания через compose); образ содержит все зависимости среды выполнения

## Быстрые сценарии работы

### Подключение по SSH

1. Нажмите **Новое подключение** в менеджере подключений
2. Заполните хост, порт и данные аутентификации (пароль или приватный ключ)
3. Нажмите **Подключиться**, чтобы открыть SSH-сессию терминала

### AI-агент

1. Перейдите в настройки и сконфигурируйте **AI-провайдера** (конечная точка API, модель и ключ)
2. Откройте вкладку терминала (SSH или локальный)
3. Откройте чат в боковой панели AI — введите задачу, и AI-агент выполнит команды прямо в вашем терминале

### Передача файлов по SFTP

1. В менеджере подключений **щёлкните правой кнопкой мыши** по SSH-подключению
2. Выберите **Подключить SFTP**
3. Просматривайте, загружайте, скачивайте и перетаскивайте файлы в двухпанельном файловом менеджере

## Технологический стек

| Уровень | Технология |
|-------|-----------|
| Настольный фреймворк | Wails v3 |
| Бэкенд | Go |
| Фронтенд | Vue 3 + Pinia + Element Plus |
| Терминал | xterm.js |
| Протокол AI | Anthropic Messages API / OpenAI Chat Completions API |

## Сборка из исходников

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (установка: `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`)
- На macOS требуются Xcode Command Line Tools
- На Linux требуются `libgtk-3-dev` и `libwebkit2gtk-4.1-dev`
- Для сборки под Android дополнительно требуются JDK 21 и Android SDK/NDK (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## Структура проекта

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

## План развития

Текущий статус протоколов и возможностей, а также ближайшие планы см. в [ROADMAP.md](ROADMAP.md). Историю по релизам см. в [CHANGELOG.md](CHANGELOG.md).

## Поставьте звёздочку проекту

Если uniTerm вам полезен, пожалуйста, поставьте проекту ⭐ Star — это лучшая поддержка для проекта и помогает большему числу людей узнать о нём.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## Обратная связь и участие в разработке

Это личный хобби-проект, который автор поддерживает в свободное время, без планов коммерциализации или спонсорства — все, кто разделяет этот интерес, с радостью приглашаются присоединиться, обсуждать и развивать проект вместе.

Замечания, предложения и отзывы приветствуются в [GitHub Issues](https://github.com/ys-ll/uniterm/issues), а вклад кодом через [Pull Request](https://github.com/ys-ll/uniterm/pulls) всегда приветствуется. Из-за огромного разнообразия терминальных сред и сценариев использования автор не может протестировать все возможные случаи — если вы столкнулись с проблемой, смело форкайте проект и вносите изменения напрямую. Этот проект приветствует vibe coding.

Спасибо следующим людям за вклад в код и улучшения, а также всем, кто сообщал о проблемах и делился предложениями — вы помогаете делать uniTerm лучше ❤️

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

## Лицензия

Apache 2.0
