<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>Terminal todo en uno compatible con más de 30 protocolos: SSH, RDP, SFTP, bases de datos, Kubernetes y más<br>Con un Agente de IA integrado que ejecuta comandos de shell de múltiples turnos de forma autónoma</p>
  <p><a href="https://uniterm.net">🌐 Sitio web</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 Guía de usuario</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; Español &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; <a href="README_pt.md">Português</a> &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## Índice

- [Características](#características)
- [Protocolos compatibles](#protocolos-compatibles)
- [Capturas de pantalla](#capturas-de-pantalla)
- [Descarga](#descarga)
- [Flujos de trabajo rápidos](#flujos-de-trabajo-rápidos)
- [Pila tecnológica](#pila-tecnológica)
- [Compilar desde el código fuente](#compilar-desde-el-código-fuente)
- [Estructura del proyecto](#estructura-del-proyecto)
- [Hoja de ruta](#hoja-de-ruta)
- [Dale una estrella a este proyecto](#dale-una-estrella-a-este-proyecto)
- [Comentarios y contribuciones](#comentarios-y-contribuciones)
- [Licencia](#licencia)

## Características

### Terminal con todas las funciones

Terminal remota, terminal local y de serie, transferencia de archivos, escritorio remoto, bases de datos, contenedores y monitorización de servidores: cubre todas las necesidades de acceso remoto.

- **Terminal remota** — SSH / Telnet / Mosh / Raw TCP con autenticación por contraseña o clave; incluye reenvío de puertos por túnel SSH, de modo que cualquier conexión puede pasar por un host de salto (jump host) SSH.
- **Terminal local y de serie** — PowerShell / CMD / Git Bash / WSL, además de conexiones serie con velocidad en baudios, bits de datos, bits de parada, paridad y eco local configurables.
- **Transferencia de archivos** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem, con exploración de doble panel y compatibilidad con `rz`/`sz` en terminales SSH.
- **Escritorio remoto** — RDP (escritorio remoto de Windows), VNC (control remoto de Linux), SPICE (máquinas virtuales KVM/QEMU), X11 (reenvío de X Window)
- **Cliente de bases de datos** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch, con consultas en lenguaje natural asistidas por IA y edición de filas de datos.
- **Kubernetes y contenedores** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC, con gestión de recursos e imágenes del clúster, terminales de logs y exec de Pods, y métricas de CPU/memoria.
- **Monitorización de servidores** — CPU, memoria, disco, red, procesos, puertos e interfaces de red en tiempo real.

### Agente de IA

Agente de IA autónomo que planifica y ejecuta por sí mismo comandos de shell de múltiples turnos directamente en tu terminal.

- **Agente de IA autónomo** — El Agente de IA puede planificar, ejecutar, observar resultados e iterar a lo largo de varias rondas de comandos de shell sin intervención manual; chat lateral con API compatibles con Anthropic/OpenAI, con soporte para Claude, GPT y otros modelos compatibles.
- **Modos de ejecución flexibles** — Omitir confirmaciones, solo operaciones peligrosas, peligrosas + escritura o confirmar todo: tú decides cuánta supervisión necesita el Agente de IA.
- **Integración con la terminal** — Los comandos de la IA se ejecutan directamente en la pestaña de terminal activa, con la opción de fijarlos a una pestaña específica o de seguir la pestaña activa. Colabora en paralelo en paneles divididos, cada uno con su propio contexto de terminal.
- **Servidor MCP** — Servidor MCP integrado que permite a agentes de IA externos (Claude Code, Codex, Gemini CLI, ...) ejecutar comandos y transferir archivos en las conexiones guardadas; las credenciales nunca salen de la aplicación.
- **Habilidades y comandos** — Flujos de trabajo de habilidades reutilizables y comandos basados en plantillas de prompt, que se adjuntan con `/` en la entrada de la IA; la IA también puede guardar nuevas habilidades por sí misma.
- **Autocompletado inteligente** — Autocompletado en tiempo real mientras escribes, basado en el historial de comandos y en las capacidades de la IA.

### Personalización

Gestión de conexiones, paneles divididos, sincronización en la nube, temas e internacionalización: tu terminal, a tu manera.

- **Gestor de conexiones** — Agrupa, busca rápidamente, crea y opera en lote conexiones de servidores.
- **Espacio de trabajo** — Arrastra las pestañas de terminal al área de contenido para dividirlas libremente y combinarlas en un espacio de trabajo; arrastra los bordes de los paneles para cambiar su tamaño y reorganizarlos.
- **Sincronización en la nube** — Sincronización cifrada en la nube en cinco modos: repositorio Git, WebDAV, SFTP, S3 y SMB; sin preocupaciones por pérdida o fugas de datos, y retoma tu trabajo sin problemas en todos tus dispositivos.
- **Atajos de teclado personalizados** — Asigna libremente atajos de teclado a cada acción para una operación totalmente controlada por teclado, sin que las manos abandonen el teclado.
- **Temas e internacionalización** — 28 temas de terminal más 3 temas de interfaz (Oscuro / Azul profundo / Claro) y una imagen de fondo personalizable; interfaz en 11 idiomas: chino simplificado, chino tradicional, inglés, japonés, coreano, alemán, español, francés, ruso, portugués, vietnamita.

## Protocolos compatibles

| Categoría | Protocolo | Descripción |
|----------|----------|-------------|
| Terminal | SSH | Gestión de shells de servidores remotos |
| Terminal | Telnet | Terminal remota para dispositivos antiguos y sistemas embebidos |
| Terminal | Mosh | Conexiones a servidores en redes con alta latencia o intermitentes |
| Terminal | Serial | Terminal de puerto serie con velocidad en baudios y otros parámetros configurables |
| Terminal | Raw TCP | Consola Raw TCP que abre un socket sencillo y transmite y recibe bytes en bruto |
| Terminal | Local | PowerShell, CMD, Git Bash y otras shells locales |
| Terminal | WSL | Abre las distribuciones WSL instaladas mediante la terminal local (solo Windows) |
| Transferencia de archivos | SFTP | Gestión y transferencia de archivos del servidor |
| Transferencia de archivos | SCP | Protocolo alternativo para servidores SSH antiguos que no admiten SFTP |
| Transferencia de archivos | FTP / FTPS | Alojamiento de sitios web, transferencia de archivos con NAS |
| Transferencia de archivos | SMB | Carpetas compartidas de Windows, acceso a archivos de NAS |
| Transferencia de archivos | WebDAV | Gestión de archivos en servidores WebDAV |
| Transferencia de archivos | S3 | Almacenamiento de objetos compatible con Amazon S3 |
| Transferencia de archivos | Zmodem | Transferencia de archivos dentro de la terminal mediante los comandos rz/sz |
| Escritorio remoto | RDP | Gestión de escritorio remoto de servidores Windows |
| Escritorio remoto | VNC | Control remoto de servidores Linux |
| Escritorio remoto | SPICE | Gestión de máquinas virtuales KVM/QEMU |
| Escritorio remoto | X11 | Reenvío de X Window |
| Base de datos | MySQL | Protocolo MySQL: MySQL, MariaDB, TiDB y más |
| Base de datos | PostgreSQL | Protocolo PostgreSQL: PostgreSQL, CockroachDB y más |
| Base de datos | Oracle Database | Conexiones a Oracle Database mediante un driver puro de Go |
| Base de datos | SQL Server | Conexiones a SQL Server mediante un driver puro de Go |
| Base de datos | SQLite | Base de datos local embebida; se conecta abriendo el archivo de la base de datos |
| Base de datos | rqlite | Base de datos distribuida ligera construida sobre SQLite con consenso Raft |
| Base de datos | Redis | Almacén clave-valor en memoria con navegación y edición visual de claves |
| Base de datos | MongoDB | Base de datos de documentos con navegación en árbol, editor de consultas y edición en línea |
| Base de datos | Elasticsearch | Motor distribuido de búsqueda y análisis con gestión de índices y documentos |
| Contenedores | Kubernetes | Navegación y gestión de recursos del clúster, logs y exec de Pods, métricas de rendimiento |
| Contenedores | Docker | Gestión de contenedores e imágenes, en la máquina local o en hosts remotos vía SSH |
| Contenedores | Podman | Motor de contenedores compatible con Docker, en la máquina local o en hosts remotos vía SSH |
| Contenedores | nerdctl (containerd) | Gestión de contenedores containerd con cambio de espacios de nombres |
| Contenedores | WSLC | Runtime de contenedores WSL2 de Windows (solo Windows) |

El soporte de Oracle Database está implementado con un driver puro de Go. uniTerm no incluye Oracle Database, Oracle Instant Client, OJDBC, archivos de wallet ni recursos de la marca Oracle; los usuarios son responsables de sus propias licencias de Oracle, credenciales y acceso a la base de datos.

## Capturas de pantalla

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="Página de inicio" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="Nueva conexión" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="Terminal SSH con asistente de IA" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="Espacio de trabajo" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="Transferencia de archivos SFTP" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="Explorador de bases de datos" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Gestión de Kubernetes" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="Imagen de fondo de la terminal" width="45%" loading="eager" />
</p>

## Descarga

Obtén los últimos binarios precompilados desde [GitHub Releases](https://github.com/ys-ll/uniterm/releases) o [Gitee Releases](https://gitee.com/ys-l/uniterm/releases):

- **Windows** (amd64 / arm64): instalador `uniterm-windows-*-installer-*.exe`, o versión portátil `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): descarga `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64): descarga `uniterm-linux-*-*.tar.gz`, `.deb` o `.rpm`
- **Android** (arm64): descarga `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64): despliegue multiusuario web — un despliegue de servidor accesible desde el navegador donde cada usuario obtiene un espacio de trabajo aislado, consulta la [guía de despliegue con Docker](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **Sobre los falsos positivos del antivirus en Windows**: como este software de código abierto no ha adquirido un certificado de firma de código, el ejecutable sin firmar puede provocar falsos positivos en algunos motores antivirus (p. ej. Windows Defender). Se trata de un problema conocido de las aplicaciones Go/Wails (véase [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). Puedes añadir una regla de exclusión en tu antivirus para permitirlo. Descarga únicamente desde los canales oficiales de código abierto: GitHub y Gitee. Si aún te preocupan los malwares, puedes descargar el código fuente y compilarlo y ejecutarlo localmente por ti mismo.

### Gestores de paquetes

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

### Dependencias en tiempo de ejecución

- **Windows**: runtime de WebView2 (incluido en Windows 10+; las versiones anteriores necesitan una instalación única)
- **macOS**: sin dependencias adicionales (usa el WebKit del sistema)
- **Linux**: `libgtk-3-0` y `libwebkit2gtk-4.1-0` (preinstaladas en la mayoría de las distribuciones de escritorio)
- **Android**: Android 5.0 o superior (usa el WebView del sistema, sin dependencias adicionales)
- **Docker**: solo Docker Engine en el host (Docker Compose v2 para despliegues con compose); la imagen incluye todas las dependencias en tiempo de ejecución

## Flujos de trabajo rápidos

### Conexión SSH

1. Haz clic en **Nueva conexión** en el Gestor de conexiones
2. Rellena el host, el puerto y la autenticación (contraseña o clave privada)
3. Haz clic en **Conectar** para abrir una sesión de terminal SSH

### Agente de IA

1. Ve a Ajustes y configura tu **proveedor de IA** (endpoint de la API, modelo y clave)
2. Abre una pestaña de terminal (SSH o local)
3. Abre el chat lateral de la IA — escribe tu tarea y el Agente de IA ejecutará los comandos directamente en tu terminal

### Transferencia de archivos SFTP

1. En el Gestor de conexiones, haz **clic derecho** en una conexión SSH
2. Selecciona **Conectar SFTP**
3. Explora, sube, descarga y arrastra y suelta archivos en el gestor de archivos de doble panel

## Pila tecnológica

| Capa | Tecnología |
|-------|-----------|
| Framework de escritorio | Wails v3 |
| Backend | Go |
| Frontend | Vue 3 + Pinia + Element Plus |
| Terminal | xterm.js |
| Protocolo de IA | Anthropic Messages API / OpenAI Chat Completions API |

## Compilar desde el código fuente

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (instalar con `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`)
- macOS necesita las Xcode Command Line Tools
- Linux necesita `libgtk-3-dev` y `libwebkit2gtk-4.1-dev`
- Las compilaciones para Android además necesitan JDK 21 y el Android SDK/NDK (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## Estructura del proyecto

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

## Hoja de ruta

Consulta [ROADMAP.md](ROADMAP.md) para ver el estado actual de los protocolos y funciones, así como lo que está previsto a continuación. Para el historial por versión, consulta [CHANGELOG.md](CHANGELOG.md).

## Dale una estrella a este proyecto

Si uniTerm te resulta útil, considera darle una ⭐ Star — es el mejor estímulo para el proyecto y ayuda a que más gente lo descubra.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## Comentarios y contribuciones

Este es un proyecto personal de afición mantenido por el autor en su tiempo libre, sin planes de comercialización ni patrocinio; cualquier persona que comparta el mismo interés es bienvenida para unirse, debatir y construir juntos.

Las issues, sugerencias y comentarios son bienvenidos en [GitHub Issues](https://github.com/ys-ll/uniterm/issues), y las contribuciones de código mediante [Pull Request](https://github.com/ys-ll/uniterm/pulls) son siempre bienvenidas. Debido a la gran variedad de entornos de terminal y escenarios de uso, el autor no puede cubrir todos los casos posibles en las pruebas; si te encuentras con algún problema, siéntete libre de hacer fork del proyecto y contribuir directamente. Este proyecto da la bienvenida al vibe coding.

Gracias a las siguientes personas por contribuir con código y mejoras, y a todos los que reportaron issues y compartieron sugerencias — ustedes ayudan a hacer uniTerm mejor ❤️

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

## Licencia

Apache 2.0
