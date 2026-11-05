<div align="center">
  <img src="build/appicon.png" alt="uniTerm" width="128" height="128" />
  <h1>uniTerm</h1>
  <p>Terminal tudo-em-um com suporte a mais de 30 protocolos — SSH, RDP, SFTP, bancos de dados, Kubernetes e mais<br>Com um Agente de IA integrado que executa comandos de shell em múltiplas rodadas de forma autônoma</p>
  <p><a href="https://uniterm.net">🌐 Site oficial</a> &nbsp;|&nbsp; <a href="https://uniterm.net/guide/en/introduction">📖 Guia do usuário</a> &nbsp;|&nbsp; <a href="https://github.com/ys-ll/uniterm">💻 GitHub</a> &nbsp;|&nbsp; <a href="https://gitee.com/ys-l/uniterm">💻 Gitee</a></p>
</div>

<div align="center">

<a href="README_zh-CN.md">简体中文</a> &nbsp;|&nbsp; <a href="README_zh-TW.md">繁體中文</a> &nbsp;|&nbsp; <a href="README.md">English</a> &nbsp;|&nbsp; <a href="README_ja.md">日本語</a> &nbsp;|&nbsp; <a href="README_ko.md">한국어</a> &nbsp;|&nbsp; <a href="README_de.md">Deutsch</a> &nbsp;|&nbsp; <a href="README_es.md">Español</a> &nbsp;|&nbsp; <a href="README_fr.md">Français</a> &nbsp;|&nbsp; <a href="README_ru.md">Русский</a> &nbsp;|&nbsp; Português &nbsp;|&nbsp; <a href="README_vi.md">Tiếng Việt</a>

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

## Sumário

- [Funcionalidades](#funcionalidades)
- [Protocolos Suportados](#protocolos-suportados)
- [Capturas de Tela](#capturas-de-tela)
- [Download](#download)
- [Fluxos de Trabalho Rápidos](#fluxos-de-trabalho-rápidos)
- [Stack Tecnológica](#stack-tecnológica)
- [Compilar a Partir do Código-Fonte](#compilar-a-partir-do-código-fonte)
- [Estrutura do Projeto](#estrutura-do-projeto)
- [Roadmap](#roadmap)
- [Dê uma Estrela a Este Projeto](#dê-uma-estrela-a-este-projeto)
- [Feedback e Contribuição](#feedback-e-contribuição)
- [Licença](#licença)

## Funcionalidades

### Terminal Completo

Terminal remoto, terminal local e serial, transferência de arquivos, área de trabalho remota, banco de dados, containers e monitoramento de servidores — cobrindo todas as necessidades de acesso remoto.

- **Terminal Remoto** — SSH / Telnet / Mosh / Raw TCP com autenticação por senha ou chave; inclui encaminhamento de portas via túnel SSH, permitindo que qualquer conexão passe por um host de salto (jump host) SSH.
- **Terminal Local e Serial** — PowerShell / CMD / Git Bash / WSL, além de conexões seriais com baud rate, bits de dados, bits de parada, paridade e eco local configuráveis.
- **Transferência de Arquivos** — SFTP / SCP / FTP / FTPS / SMB / WebDAV / S3 / Zmodem, com navegação em painéis duplos e suporte a `rz`/`sz` em terminais SSH.
- **Área de Trabalho Remota** — RDP (área de trabalho remota do Windows), VNC (controle remoto de Linux), SPICE (máquinas virtuais KVM/QEMU), X11 (encaminhamento do X Window)
- **Cliente de Banco de Dados** — MySQL / PostgreSQL / Oracle / SQL Server / SQLite / rqlite / Redis / MongoDB / Elasticsearch, com consultas em linguagem natural assistidas por IA e edição de linhas de dados.
- **Kubernetes e Containers** — Kubernetes / Docker / Podman / nerdctl (containerd) / WSLC, com gerenciamento de recursos e imagens do cluster, logs de Pods e terminais exec, e métricas de CPU/memória.
- **Monitoramento de Servidores** — CPU, memória, disco, rede, processos, portas e interfaces de rede em tempo real.

### Agente de IA

Agente de IA autônomo que planeja e executa, de forma independente, comandos de shell em múltiplas rodadas diretamente no seu terminal.

- **Agente de IA Autônomo** — O Agente de IA pode planejar, executar, observar os resultados e iterar ao longo de múltiplas rodadas de comandos de shell sem intervenção manual; chat na barra lateral com API compatível com Anthropic/OpenAI, com suporte a Claude, GPT e outros modelos compatíveis.
- **Modos de Execução Flexíveis** — Bypass, apenas operações perigosas, perigosas + escrita, ou confirmar tudo — você controla o nível de supervisão que o Agente de IA precisa.
- **Integração com o Terminal** — Os comandos da IA são executados diretamente na aba de terminal ativa, com opção de fixar em uma aba específica ou seguir a aba ativa. Colabore lado a lado em painéis divididos, cada um com seu próprio contexto de terminal.
- **Servidor MCP** — Servidor MCP integrado que permite a agentes de IA externos (Claude Code, Codex, Gemini CLI, ...) executar comandos e transferir arquivos nas conexões salvas — as credenciais nunca saem do aplicativo.
- **Skills e Comandos** — Fluxos de trabalho de skills reutilizáveis e comandos baseados em templates de prompt, anexados com `/` na entrada da IA; a IA também pode salvar novas skills por conta própria.
- **Preenchimento Inteligente** — Preenchimento em tempo real enquanto você digita, baseado no histórico de comandos e nas capacidades da IA.

### Personalização

Gerenciamento de conexões, painéis divididos, sincronização em nuvem, temas e i18n — seu terminal, do seu jeito.

- **Gerenciador de Conexões** — Agrupe, pesquise rapidamente, crie e realize operações em lote em conexões de servidores.
- **Workspace** — Arraste abas de terminal para a área de conteúdo para dividir livremente e combiná-las em um workspace; arraste as bordas dos painéis para redimensionar e reorganizar.
- **Sincronização em Nuvem** — Sincronização em nuvem criptografada em cinco modos: repositório Git, WebDAV, SFTP, S3 e SMB — sem se preocupar com perda ou vazamento de dados, e retome seu trabalho perfeitamente entre dispositivos.
- **Atalhos de Teclado Personalizados** — Vincule livremente atalhos de teclado para cada ação, para uma operação totalmente orientada ao teclado, sem tirar as mãos do teclado.
- **Temas e Internacionalização** — 28 temas de terminal, além de 3 temas de UI (Escuro / Azul Profundo / Claro) e uma imagem de fundo personalizável; UI em 11 idiomas: chinês simplificado, chinês tradicional, inglês, japonês, coreano, alemão, espanhol, francês, russo, português, vietnamita.

## Protocolos Suportados

| Categoria | Protocolo | Descrição |
|----------|----------|-------------|
| Terminal | SSH | Gerenciamento de shell em servidores remotos |
| Terminal | Telnet | Terminal remoto para dispositivos legados e sistemas embarcados |
| Terminal | Mosh | Conexões a servidores em redes de alta latência ou intermitentes |
| Terminal | Serial | Terminal de porta serial com baud rate e outros parâmetros configuráveis |
| Terminal | Raw TCP | Console Raw TCP que abre um socket simples e transmite/recebe bytes brutos |
| Terminal | Local | PowerShell, CMD, Git Bash e outros shells locais |
| Terminal | WSL | Abra distribuições WSL instaladas via terminal local (apenas Windows) |
| Transferência de Arquivos | SFTP | Gerenciamento e transferência de arquivos do servidor |
| Transferência de Arquivos | SCP | Protocolo alternativo para servidores SSH legados que não suportam SFTP |
| Transferência de Arquivos | FTP / FTPS | Hospedagem de sites, transferência de arquivos para NAS |
| Transferência de Arquivos | SMB | Pastas compartilhadas do Windows, acesso a arquivos em NAS |
| Transferência de Arquivos | WebDAV | Gerenciamento de arquivos em servidores WebDAV |
| Transferência de Arquivos | S3 | Armazenamento de objetos compatível com Amazon S3 |
| Transferência de Arquivos | Zmodem | Transferência de arquivos no terminal via comandos rz/sz |
| Área de Trabalho Remota | RDP | Gerenciamento de área de trabalho remota de servidores Windows |
| Área de Trabalho Remota | VNC | Controle remoto de servidores Linux |
| Área de Trabalho Remota | SPICE | Gerenciamento de máquinas virtuais KVM/QEMU |
| Área de Trabalho Remota | X11 | Encaminhamento do X Window |
| Banco de Dados | MySQL | Protocolo MySQL: MySQL, MariaDB, TiDB e mais |
| Banco de Dados | PostgreSQL | Protocolo PostgreSQL: PostgreSQL, CockroachDB e mais |
| Banco de Dados | Oracle Database | Conexões ao Oracle Database por meio de um driver Go puro |
| Banco de Dados | SQL Server | Conexões ao SQL Server por meio de um driver Go puro |
| Banco de Dados | SQLite | Banco de dados local embarcado, conecte-se abrindo o arquivo do banco de dados |
| Banco de Dados | rqlite | Banco de dados distribuído leve construído sobre SQLite com consenso Raft |
| Banco de Dados | Redis | Armazenamento chave-valor em memória com navegação e edição visual de chaves |
| Banco de Dados | MongoDB | Banco de dados de documentos com navegação em árvore, editor de consultas e edição inline |
| Banco de Dados | Elasticsearch | Motor de busca e análise distribuído com gerenciamento de índices e documentos |
| Containers | Kubernetes | Navegação e gerenciamento de recursos do cluster, logs de Pods e exec, métricas de desempenho |
| Containers | Docker | Gerenciamento de containers e imagens, na máquina local ou em hosts remotos via SSH |
| Containers | Podman | Engine de containers compatível com Docker, na máquina local ou em hosts remotos via SSH |
| Containers | nerdctl (containerd) | Gerenciamento de containers containerd com troca de namespaces |
| Containers | WSLC | Runtime de containers WSL2 do Windows (apenas Windows) |

O suporte ao Oracle Database é implementado com um driver Go puro. O uniTerm não inclui Oracle Database, Oracle Instant Client, OJDBC, arquivos de wallet ou ativos de marca da Oracle; os usuários são responsáveis por suas próprias licenças Oracle, credenciais e acesso ao banco de dados.

## Capturas de Tela

<p align="center">
  <picture>
    <source srcset="docs/imgs/start_tab.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/start_tab_light.webp" alt="Página Inicial" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/new_connection.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/new_connection_light.webp" alt="Nova Conexão" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/ai_assistant.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/ai_assistant_light.webp" alt="Terminal SSH com Assistente de IA" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/workspace.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/workspace_light.webp" alt="Workspace" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/sftp.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/sftp_light.webp" alt="Transferência de Arquivos via SFTP" width="45%" loading="eager" />
  </picture>
  <picture>
    <source srcset="docs/imgs/database.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/database_light.webp" alt="Navegador de Banco de Dados" width="45%" loading="eager" />
  </picture>
</p>
<p align="center">
  <picture>
    <source srcset="docs/imgs/kubernetes.webp" media="(prefers-color-scheme: dark)" />
    <img src="docs/imgs/kubernetes_light.webp" alt="Gerenciamento do Kubernetes" width="45%" loading="eager" />
  </picture>
  <img src="docs/imgs/background_image.webp" alt="Imagem de Fundo do Terminal" width="45%" loading="eager" />
</p>

## Download

Obtenha os binários pré-compilados mais recentes em [GitHub Releases](https://github.com/ys-ll/uniterm/releases) ou [Gitee Releases](https://gitee.com/ys-l/uniterm/releases):

- **Windows** (amd64 / arm64): instalador `uniterm-windows-*-installer-*.exe`, ou versão portátil `uniterm-windows-*-portable-*.zip`
- **macOS** (Apple Silicon arm64 / Intel amd64): baixe `uniterm-darwin-*-*.dmg`
- **Linux** (amd64 / arm64): baixe `uniterm-linux-*-*.tar.gz`, `.deb` ou `.rpm`
- **Android** (arm64): baixe `uniterm-android-arm64-*.apk`
- **Docker** (amd64 / arm64): implantação web multiusuário (multi-tenant) — uma implantação de servidor acessível pelo navegador, em que cada usuário recebe um workspace isolado; veja o [guia de implantação Docker](https://github.com/ys-ll/uniterm/blob/main/deploy/docker/README.md)

> **Sobre falsos positivos de antivírus no Windows**: como este software de código aberto não adquiriu um certificado de assinatura de código, o executável não assinado pode gerar falsos positivos em alguns antivírus (por exemplo, Windows Defender). Este é um problema conhecido de aplicações Go/Wails (veja [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). Você pode adicionar uma regra de exclusão no seu antivírus para permiti-lo. Faça o download apenas pelos canais oficiais de código aberto — GitHub e Gitee. Se ainda estiver preocupado com malware, você pode baixar o código-fonte e compilá-lo e executá-lo localmente por conta própria.

### Gerenciadores de Pacotes

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

### Dependências de Execução

- **Windows**: runtime do WebView2 (incluído no Windows 10+; versões mais antigas precisam de uma instalação única)
- **macOS**: sem dependências extras (usa o WebKit do sistema)
- **Linux**: `libgtk-3-0` e `libwebkit2gtk-4.1-0` (pré-instalados na maioria das distribuições com desktop)
- **Android**: Android 5.0+ (usa o WebView do sistema, sem dependências extras)
- **Docker**: apenas o Docker Engine no host (Docker Compose v2 para implantações com compose); a imagem já inclui todas as dependências de execução

## Fluxos de Trabalho Rápidos

### Conexão SSH

1. Clique em **Nova Conexão** no Gerenciador de Conexões
2. Preencha host, porta e autenticação (senha ou chave privada)
3. Clique em **Conectar** para abrir uma sessão de terminal SSH

### Agente de IA

1. Vá até as Configurações e configure seu **provedor de IA** (endpoint da API, modelo e chave)
2. Abra uma aba de terminal (SSH ou local)
3. Abra o chat na barra lateral da IA — digite sua tarefa, e o Agente de IA executa os comandos diretamente no seu terminal

### Transferência de Arquivos via SFTP

1. No Gerenciador de Conexões, **clique com o botão direito** em uma conexão SSH
2. Selecione **Conectar SFTP**
3. Navegue, envie, baixe e arraste e solte arquivos no gerenciador de arquivos de painéis duplos

## Stack Tecnológica

| Camada | Tecnologia |
|-------|-----------|
| Framework Desktop | Wails v3 |
| Backend | Go |
| Frontend | Vue 3 + Pinia + Element Plus |
| Terminal | xterm.js |
| Protocolo de IA | Anthropic Messages API / OpenAI Chat Completions API |

## Compilar a Partir do Código-Fonte

- [Go](https://go.dev/dl/) 1.26+
- [Node.js](https://nodejs.org/) 20+
- [wails3 CLI](https://wails.io/docs/) v3.1.0 (instale com `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`)
- O macOS precisa do Xcode Command Line Tools
- O Linux precisa de `libgtk-3-dev` e `libwebkit2gtk-4.1-dev`
- Compilações para Android precisam adicionalmente do JDK 21 e do Android SDK/NDK (`sdkmanager "platform-tools" "platforms;android-35" "build-tools;35.0.0" "ndk;26.3.11579264"`)

```bash
git clone https://github.com/ys-ll/uniterm.git
cd uniTerm
cd frontend && npm install && cd ..
wails3 dev                  # Development
wails3 build                # Production build
```

## Estrutura do Projeto

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

Consulte o [ROADMAP.md](ROADMAP.md) para ver o status atual de protocolos / funcionalidades e o que está planejado a seguir. Para o histórico por versão, consulte o [CHANGELOG.md](CHANGELOG.md).

## Dê uma Estrela a Este Projeto

Se o uniTerm é útil para você, considere dar-lhe uma ⭐ Estrela — é o melhor incentivo para o projeto e ajuda mais pessoas a descobri-lo.

[![GitHub stars](https://img.shields.io/github/stars/ys-ll/uniterm?style=social)](https://github.com/ys-ll/uniterm)
[![Gitee stars](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fgitee.com%2Fapi%2Fv5%2Frepos%2Fys-l%2Funiterm&query=%24.stargazers_count&label=Stars&style=social&logo=gitee)](https://gitee.com/ys-l/uniterm)

<a href="https://www.star-history.com/?repos=ys-ll%2Funiterm&type=date&legend=top-left">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&theme=dark&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
   <img alt="Star History Chart" src="https://api.star-history.com/chart?repos=ys-ll/uniterm&type=date&legend=top-left&sealed_token=EJf9Yr6aVGkBwIbfGQ6u9hRwpxdIl7vVpUJshd8jaRUCw-OkotyrQX9qzDP5cXNidgLvC0yq8ltQivK06MxiTJYOxkVbPCPuIuCBZj7aTkMxUA-mqJk2KA" />
 </picture>
</a>

## Feedback e Contribuição

Este é um projeto pessoal de hobby mantido pelo autor no tempo livre, sem planos de comercialização ou patrocínio — qualquer pessoa que compartilhe do mesmo interesse é bem-vinda para participar, discutir e construir juntos.

Issues, sugestões e feedback são bem-vindos no [GitHub Issues](https://github.com/ys-ll/uniterm/issues), e contribuições de código via [Pull Request](https://github.com/ys-ll/uniterm/pulls) são sempre bem-vindas. Devido à grande variedade de ambientes de terminal e cenários de uso, o autor não consegue cobrir todos os casos possíveis em testes — se você encontrar algum problema, sinta-se à vontade para fazer um fork do projeto e contribuir diretamente. Este projeto acolhe vibe coding.

Agradecemos às seguintes pessoas por contribuírem com código e melhorias, e a todos que reportaram issues e compartilharam sugestões — vocês ajudam a tornar o uniTerm melhor ❤️

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

## Licença

Apache 2.0
