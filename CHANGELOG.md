# Changelog

## v1.12.1

### What's Changed

**Improvements**

- SSH: regular connections on macOS and Windows now support multi-hop jump chains — a jump host's own parent jump is resolved recursively (local → A → B → C), each hop with its own credentials, with loop detection and full-chain cleanup on failure or stop. (@surenwuyuwuqiu)
- Network scan: IP octet and port inputs accept digits only (a Chinese IME no longer leaks letters into the fields), typing a dot jumps to the next octet, inputs select their content on focus, and the port scan validates that the target is a valid IPv4 before starting. (@etanman)
- Database tree: schemas with tens of thousands of tables no longer freeze the app — table browsing is backend-paginated with virtual scrolling and server-side search, staying smooth with ~100k tables; table-name search is case-insensitive, truncated names show a full-name tooltip, schemas start collapsed with tables lazy-loaded, and schema / dictionary queries are capped at 60 s so a locked server can no longer freeze the whole app. (@etanman)

**Bug Fixes**

- RDP: the mouse wheel scrolls in the same direction as the local machine (also fixing near-zero scrolling with Firefox's line-mode deltas); server-side negotiation rejections now report the real error instead of an opaque decode failure; in web (Docker) deployments RDP / VNC / SPICE sessions are reachable from remote browsers again.
- Built-in SSH service: the SSH / SFTP local service can now act as a jump host — `direct-tcpip` forwarding is supported for authenticated clients, so chains like phone → uniTerm built-in SSH → internal machine work. (@surenwuyuwuqiu)
- MCP: opening a terminal no longer restarts the MCP service and drops active client sessions (requests no longer fail with "session not found") when the listener configuration is unchanged. (@surenwuyuwuqiu)
- Terminal (Windows): IME commits are deduplicated — picking a candidate with the mouse, or an IME whose commit keyup precedes the input event, no longer inserts the word twice. (@sonnartliao)
- Credentials: a missing OS keychain entry is now correctly recognized — the app shows the "cannot unlock credentials" explanation (possible causes such as a moved app directory, an OS reinstall, or a cleared keychain) instead of asking to set the credential store up again. (@gezerui888)
- Database: a `DELIMITER` command on the very first line of a `.sql` script is honored — MySQL routine dumps that start with `DELIMITER $$` execute correctly instead of being cut at every inner `;`. (@kevin9327)
- WSL: the cwd prompt hook is no longer typed at session start — a plain first login shows no handshake output at all; enabling "Follow terminal path" now asks once per session and types the hook on demand at an idle prompt.
- UI: the gear (row action) menu in the Skills and Commands settings pages never opened, leaving edit / delete unreachable; the menu also gains a "reveal in file manager" item.

**Notes**

- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.
- Android app: since version 1.11.0, the Android app uses a new application ID based on the uniterm domain (`net.uniterm.app`) — if a version before 1.11.0 is installed, uninstall it first before installing the new version.

Thanks to @surenwuyuwuqiu, @etanman, @sonnartliao, @gezerui888 and @kevin9327 for their contributions to this release.

### 更新内容

**改进**

- SSH：macOS 和 Windows 的普通连接支持多级跳板——跳板自身的上一级跳板会被递归解析（本机 → A → B → C），每一跳使用自己的凭据，带循环引用检测，失败或停止时清理整条链。(@surenwuyuwuqiu)
- 网络扫描：IP 各段与端口输入只接受数字（中文输入法不再把字母带进输入框），输入点号自动跳到下一段，输入框聚焦时全选已有内容；端口扫描启动前校验目标是合法 IPv4。(@etanman)
- 数据库树：几万张表的库不再卡死——表浏览改为后端分页 + 虚拟滚动 + 服务端搜索，约 10 万张表依然流畅；表名搜索不区分大小写、截断的表名悬停显示完整名称、库默认折叠按需加载表，schema / 字典查询增加 60 秒超时，锁住的服务器不再拖死整个应用。(@etanman)

**问题修复**

- RDP：鼠标滚轮方向与本地一致（同时修复 Firefox 行模式增量下几乎不滚动的问题）；服务端协商失败现在显示真实原因，而非不可读的解码错误；Web（Docker）部署下远程浏览器可以正常打开 RDP / VNC / SPICE 会话。
- 内置 SSH 服务：SSH / SFTP 本地服务可以作为跳板机使用——为已认证客户端支持 `direct-tcpip` 转发，手机 → uniTerm 内置 SSH → 内网机器 这类链路可用了。(@surenwuyuwuqiu)
- MCP：监听配置未变化时，打开终端不再重启 MCP 服务、不再丢失已有客户端会话（后续请求不再报 "session not found"）。(@surenwuyuwuqiu)
- 终端（Windows）：输入法上屏去重——鼠标点选候选词、或提交时序特殊的输入法，不再把同一个词插入两次。(@sonnartliao)
- 凭据：系统钥匙串条目丢失时能正确识别——显示「无法解锁凭证」说明及可能原因（如应用目录被移动、重装系统、清除了钥匙串），而不是直接要求重新初始化凭据库。(@gezerui888)
- 数据库：`.sql` 脚本第一行的 `DELIMITER` 命令现在会被识别——以 `DELIMITER $$` 开头的 MySQL 存储过程导出脚本可以正确执行，不再在每个内部 `;` 处被切开。(@kevin9327)
- WSL：会话启动时不再键入 cwd 提示钩子——普通首次登录不再出现任何握手输出；开启「跟随终端路径」时每个会话询问一次，并在空闲提示符处按需键入钩子。
- 界面：Skills 和 Commands 设置页的行操作菜单无法弹出，编辑 / 删除不可达；菜单新增「在文件管理器中打开」项。

**说明**

- 本软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报。这是 Go/Wails 应用的已知问题（见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行；请务必从官方开源渠道——GitHub 和 Gitee——下载。如仍有顾虑，可自行下载源码本地构建运行。
- Android：自 1.11.0 起，Android 应用改用基于 uniterm 域名的新 application ID（`net.uniterm.app`）——如果安装过 1.11.0 之前的版本，请先卸载旧版再安装新版。

感谢 @surenwuyuwuqiu, @etanman, @sonnartliao, @gezerui888, @kevin9327 对本版本发布的贡献。

## v1.12.0

### What's Changed

**New Features**

- Docker web multi-tenant deployment: uniTerm can now be deployed as a browser-accessible service (official Docker image, compose file and guide included) where each user gets an isolated workspace with their own connections, sessions and keys. (@NatureLProokie)
- Local services: seven new local services — HTTP, TFTP, SSH, FTP, Telnet, NFS and a directory-backed S3; all local services gain a unified auto-stop timeout, so a service left running by mistake stops itself after the configured time instead of running indefinitely. (@etanman)
- Terminal recording: sessions can be recorded and replayed (asciinema cast v2 format) — a REC badge with the live elapsed time sits in the terminal, and a recording list offers replay, delete and open-folder actions. (@laigcss)
- Batch execution: select multiple SSH connections and run a command on all of them concurrently, with configurable parallelism, timeout and failure strategy, a live per-host result table and CSV / JSON export — the entry lives in the connection context menu when two or more hosts are selected. (@laigcss)
- Terminal: inline suggestions from the login shell's history — a dimmed hint completes after the cursor and is accepted with the Right arrow, while Tab keeps reaching the shell's own completion; the hint and the suggestion popup each have their own switch in terminal settings. (@dungvv)
- SSH: host keys are now verified with trust-on-first-use — the server fingerprint is recorded on the first connect, and a changed key is rejected with a dialog showing both fingerprints, offering to save the new key, connect once without recording it, or cancel. (@laigcss)
- Remote desktop (RDP): quality tiers (high / balanced / low bandwidth) cut server-side graphics features on slow links; resolution and zoom controls move to the status bar and fullscreen toolbar for RDP / VNC / SPICE (adaptive, presets, physical-pixel zoom, fit-window, stretch) and are persisted per connection.
- UI: a bottom panel is added under the tab area — any sidebar view (quick commands, monitor, …) can dock there and be shown beside the left sidebar at the same time; views move and reorder between the two docks via the context menu, the panel title-bar button or drag & drop.
- System monitor: the items shown in the monitor sidebar are configurable — disabled items are hidden and no longer polled, reducing the load on the monitored node.
- Containers: image archives can be loaded and containers committed to images, and container creation gains stdin and TTY options. (@surenwuyuwuqiu)
- Database: the result grid can filter rows in place, and CSV import / export are written atomically. (@surenwuyuwuqiu)

**Improvements**

- AI: a local risk guard independently classifies every command the AI runs (read / write / dangerous) by pattern matching, so even if a prompt-injected model labels `rm -rf ~` as safe, the confirmation prompt is still enforced. (@laigcss)
- AI: auto-compaction is configurable — it can be turned off, and a trigger budget and a dedicated summary model can be set per device. (@surenwuyuwuqiu)
- Terminal: the shell history feeding inline suggestions reads only the tail of the history file (at most 4 MiB) instead of the whole file — a years-old multi-megabyte history no longer stalls the first keystroke. (@boltomli)
- Terminal: Ctrl+Backspace deletes the previous word. (@surenwuyuwuqiu)
- MCP: the MCP service gains opt-in remote access. (@surenwuyuwuqiu)
- Quick commands: a button display style (name-only chips tiled horizontally) joins the existing list style.
- Connections: a compact display style that hides the host subtitle.
- Files: F5 refreshes the file panel, and activating the panel restores the terminal's focus. (@surenwuyuwuqiu)
- macOS: the native window menu follows the system language. (@etanman)
- Tabs: the active tab can be renamed with F2. (@macoladik)
- Terminal: a new right-click mode "copy or paste" (Windows Terminal style) — right-clicking a selection copies it and right-clicking with no selection pastes.
- Local terminals: the connection form gains launch arguments and a working directory for local / WSL terminals — for cmd.exe the arguments are consumed by /k, enabling launching another terminal program with parameters.
- System tray: the tray icon can now be hidden in Settings → Interaction — the change applies immediately, and on macOS a hidden window stays reachable via the Dock or the global show / hide hotkey. (@surenwuyuwuqiu)

**Bug Fixes**

- Window: geometry is saved on every quit path (tray quit, Cmd+Q, OS close, relaunch) instead of only the in-app close button, and a restored window is kept on an attached screen with its position re-checked when shown — no more opening at the default size or off-screen. (@macoladik)
- Tabs: titles set via rename survive an app restart — restored tabs no longer revert to the connection name. (@macoladik)
- AI: tool_use / tool_result pairs stay consistent, so a dropped parallel tool call can no longer deadlock compaction and kill the session with a context-too-large error. (@lijinxiao1982)
- AI: tool input objects are no longer rendered as empty in a conversation. (@mikemikimike)
- AI: the foreground execution lock is released after Ctrl+C, and copy actions in a conversation use the platform clipboard so they keep working when the app window is not focused. (@surenwuyuwuqiu)
- AI: a conversation that outgrows its budget degrades gracefully — compaction runs on what it can, oversized tool outputs are trimmed for the request only (the UI keeps the full output), and the session self-recovers on the next turn instead of dying with a context-too-large error; the default compaction trigger is raised from 32K to 96K tokens.
- AI: output piped inside Git Bash (MSYS) with a legacy system codepage is re-encoded, so text no longer shows up as mojibake.
- Menu: opening a dialog no longer closes the menu underneath it, so AI session rename / delete can repeat without reopening the menu. (@wuqiang57)
- Quick commands: copying a command works on macOS (now routed through the platform clipboard). (@surenwuyuwuqiu)
- Network scan: closes a race that left the port scan stuck at "scanning" with a frozen progress bar on fast scans. (@etanman)
- Local: the post-login script never ran on elevated (admin) shells and could be silently dropped when the shell started slowly — it is now sent anyway and buffered until the shell is ready.
- Terminal: command input verified character-by-character survives hard wraps (long pasted commands are no longer corrupted at line breaks), and the line-number gutter reserves five digits so wide line numbers no longer shift the layout. (@surenwuyuwuqiu)
- Terminal: fixes a UI freeze caused by very long wrapped output lines — the line-registry scan on each write grew quadratically with the wrapped line length.
- SSH: credentials are persisted after an auth retry instead of being lost. (@surenwuyuwuqiu)
- MCP: when shell detection fails (hardened SSH targets), foreground jobs no longer wait out their full deadline and report a timeout after actually finishing, and the busy lock no longer wedges the session until its TTL.
- Remote desktop (RDP): the last working TLS version is remembered per target, so servers that silently drop the modern handshake no longer pay a 5 s penalty on every connect; a valid tunneled session is no longer rejected as a destination mismatch over a jump host.
- Remote desktop (RDP): servers whose self-signed certificate carries a negative serial number are reachable again — Go 1.23's stricter x509 parser rejected such certificates even with certificate verification disabled, and some older Windows servers ship exactly these.
- Remote desktop (VNC): the fullscreen cursor overlay stays visible when the panel is not focused.
- X11: starting VcXsrv no longer blocks the first SSH connect for up to 5 s. (@sonnartliao)
- Containers: commit targets are validated against Docker's reference grammar up front, and the SSH stdin pipe is created before registry login starts. (@boltomli)
- Database: Oracle 11g and other pre-12c servers can list schemas and browse table data again — two dictionary queries used 12c-only syntax, so the schema tree stayed empty and table browsing failed on older releases.
- macOS: restarting immediately after toggling the system title bar setting works again — the app used to exit without relaunching. (@surenwuyuwuqiu)
- Startup: the tab-restore prompt is no longer shown when it was disabled in settings. (@surenwuyuwuqiu)

**Notes**

- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.
- Android app: since version 1.11.0, the Android app uses a new application ID based on the uniterm domain (`net.uniterm.app`) — if a version before 1.11.0 is installed, uninstall it first before installing the new version.

Thanks to @NatureLProokie, @etanman, @laigcss, @dungvv, @surenwuyuwuqiu, @boltomli, @macoladik, @lijinxiao1982, @mikemikimike, @wuqiang57 and @sonnartliao for their contributions to this release.

### 更新内容

**新功能**

- Docker Web 多租户部署模式：uniTerm 现在可以作为浏览器访问的服务部署（官方提供 Docker 镜像、compose 文件与部署指南），每位用户拥有相互隔离的工作区（各自的连接、会话与密钥）。(@NatureLProokie)
- 本地服务：新增 HTTP、TFTP、SSH、FTP、Telnet、NFS、S3 七种本地服务；所有本地服务支持统一的自动停止超时，忘记手动停止的服务会在设定时间后自动停止，避免长期占用资源和安全风险。(@etanman)
- 终端录制：支持录制并回放终端会话（asciinema cast v2 格式）——终端内显示带实时时长的 REC 徽标；录制列表提供回放、删除、打开目录等操作。(@laigcss)
- 批量执行：选中多个 SSH 连接即可批量执行命令，支持并发数、超时与失败策略配置，提供实时逐主机结果表格和 CSV / JSON 导出——在连接右键菜单中选中两台及以上主机时出现入口。(@laigcss)
- 终端：基于登录 shell 历史的行内建议——光标后以暗色提示补全，按右方向键接受；Tab 键仍保留给 shell 自己的补全。提示与建议弹窗在终端设置中各有独立开关。(@dungvv)
- SSH：主机密钥改为 TOFU（首次信任）校验——首次连接记录服务器指纹，密钥变化时拒绝连接并弹出对话框显示新旧指纹，可选择保存新密钥、不记录仅连一次，或取消。(@laigcss)
- 远程桌面（RDP）：新增画质档位（高 / 均衡 / 低带宽），慢速链路下降低服务端图形特性；RDP / VNC / SPICE 的分辨率与缩放控制移入状态栏和全屏工具栏（自适应、预设、物理像素缩放、适应窗口、拉伸），按连接保存。
- 界面：标签区下方新增底部面板——任意侧边栏视图（快捷命令、系统监控等）都可停靠至此，与左侧边栏同时显示；视图可通过右键菜单、面板标题栏按钮或拖拽在两个停靠区之间移动和排序。
- 系统监控：监控边栏显示的监控项内容可配置，禁用的监控项隐藏且不再轮询，降低被监控节点的负载。
- 容器：支持导入镜像压缩包、将容器提交为镜像，创建容器时新增 stdin 与 TTY 选项。(@surenwuyuwuqiu)
- 数据库：结果表格支持就地筛选，CSV 导入 / 导出改为原子写入。(@surenwuyuwuqiu)

**改进**

- AI：新增本地风险护栏，独立对 AI 要执行的命令做模式匹配分级（读 / 写 / 危险）——即使被提示注入的模型把 `rm -rf ~` 标为安全，确认提示依然会被强制执行。(@laigcss)
- AI：自动压缩可配置——可以关闭，也可按设备设置触发预算和独立的摘要模型。(@surenwuyuwuqiu)
- 终端：供行内建议使用的 shell 历史改为只读历史文件尾部（最多 4 MiB）而非整个文件——积年膨胀到数十 MB 的历史文件不再卡顿首次按键。(@boltomli)
- 终端：Ctrl+Backspace 删除前一个词。(@surenwuyuwuqiu)
- MCP：MCP 服务支持可选开启的远程访问。(@surenwuyuwuqiu)
- 快捷命令：新增按钮显示样式（仅名称的按钮横向平铺），与原有列表样式并存。
- 连接列表：新增紧凑显示样式，隐藏主机副标题。
- 文件：F5 刷新文件面板，激活面板时恢复终端的输入焦点。(@surenwuyuwuqiu)
- macOS：原生窗口菜单跟随系统语言。(@etanman)
- 标签页：按 F2 即可重命名当前活动标签页。(@macoladik)
- 终端：新增「复制或粘贴」右键模式（Windows Terminal 风格）——右键选中文本即复制，无选中时右键即粘贴。
- 本地终端：连接表单为本地 / WSL 终端新增启动参数与工作目录字段——cmd.exe 的参数由 /k 接收，可以带参数启动其他终端程序。
- 系统托盘：托盘图标可在 设置 → 交互 中隐藏——修改即时生效，macOS 下隐藏的窗口仍可通过 Dock 或全局显示 / 隐藏快捷键找回。(@surenwuyuwuqiu)

**问题修复**

- 窗口：所有退出路径（托盘退出、Cmd+Q、系统关闭、重启）都会保存窗口几何信息，而不再只限于应用内关闭按钮；恢复的窗口会保留在已连接的显示器上并在显示时重新校验位置——不再出现以默认尺寸或屏幕外位置启动。(@macoladik)
- 标签页：手动重命名的标题在应用重启后保留，恢复的标签页不再回退为连接名。(@macoladik)
- AI：保持 tool_use / tool_result 配对一致，被丢弃的并行工具调用不再使压缩死锁、导致会话因上下文过大而不可用。(@lijinxiao1982)
- AI：修复会话中工具调用入参对象显示为空的问题。(@mikemikimike)
- AI：Ctrl+C 后释放前台执行锁；会话内的复制操作改用平台剪贴板，应用窗口失焦时也能正常复制。(@surenwuyuwuqiu)
- AI：会话超出上下文预算时优雅降级——尽量压缩，超大工具输出仅在请求中裁剪（界面保留完整输出），下一轮自动恢复，不再因 context-too-large 错误报废；默认压缩触发阈值从 32K 提升到 96K。
- AI：修复在 Git Bash（MSYS）中管道执行、系统代码页为旧字符集时输出乱码的问题。
- 菜单：弹出对话框不再连带关闭底下的菜单，AI 会话重命名 / 删除可连续操作而无需重新打开菜单。(@wuqiang57)
- 快捷命令：修复 macOS 下复制快捷命令无效的问题（改经平台剪贴板）。(@surenwuyuwuqiu)
- 网络扫描：修复快速扫描时端口扫描卡在「扫描中」、进度条冻结的问题。(@etanman)
- 本地终端：post-login 脚本在提权（管理员）会话下从不执行，且在 shell 启动缓慢时可能被静默丢弃——现在总会发送并缓冲到 shell 就绪后写入。
- 终端：逐字符校验过的命令输入在硬换行处不再损坏（长粘贴命令跨行不再出错）；行号栏预留五位数字，宽行号不再挤压布局。(@surenwuyuwuqiu)
- 终端：修复超长自动换行的输出行导致界面卡死的问题——每次写入时的行注册扫描随换行行长度呈平方级增长。
- SSH：认证重试后凭据会被持久化，不再丢失。(@surenwuyuwuqiu)
- MCP：shell 探测失败（加固的 SSH 目标）时，前台任务不再等满超时后才报超时（实际早已完成），会话也不再因 busy 锁被占住直到 TTL 而不可用。
- 远程桌面（RDP）：按目标记住上次成功的 TLS 版本，静默丢弃现代握手的服务器不再每次连接都付出 5 秒代价；经跳板机隧道时不再因目标不匹配拒绝有效会话。
- 远程桌面（RDP）：支持连接证书序列号为负的服务器（部分老版本 Windows 服务器的自签名证书属于这种情况）。
- 远程桌面（VNC）：全屏下面板未聚焦时光标覆盖层保持可见。
- X11：启动 VcXsrv 不再阻塞首次 SSH 连接最长 5 秒。(@sonnartliao)
- 容器：提交目标引用提前按 Docker reference 语法校验；SSH stdin 管道在 registry 登录开始前创建。(@boltomli)
- 数据库：Oracle 11g 及其他 12c 之前的服务器恢复 schema 列表与表数据浏览——两处字典查询使用了 12c 才有的语法，导致旧版本的 schema 树一直为空、表浏览报错。
- macOS：切换「使用系统标题栏」设置后点「立即重启」能正常重启了，不再只是退出。(@surenwuyuwuqiu)
- 启动：设置中关闭的标签恢复提示不再弹出。(@surenwuyuwuqiu)

**说明**

- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。
- 安卓 APP 从 1.11.0 版本起更换为基于 uniterm 域名的新应用 ID（`net.uniterm.app`），如果安装了该版本之前的版本，需要先卸载旧版后安装新版。

感谢 @NatureLProokie、@etanman、@laigcss、@dungvv、@surenwuyuwuqiu、@boltomli、@macoladik、@lijinxiao1982、@mikemikimike、@wuqiang57、@sonnartliao 对本次发布的贡献。

## v1.11.2

### What's Changed

**New Features**

- Network scan: new unified IP / port network scanner, from the top-right menu or the sidebar's new-connection menu — an IP range scan with service detection (SSH / Telnet / RDP / VNC / FTP / SMB / HTTP) on top and a port scan below. A result row opens a prefilled connection form in one click, and the IP range is prefilled with the local /24 subnet. (@etanman)
- Remote desktop (VNC): multi-display VNC servers now get a display selector in the toolbar (all displays or a single display), and a custom region editor lets you view only part of the desktop — drag, draw and resize a box on the desktop. The chosen display / region is saved per connection.
- Settings: a new "Local Data" tab holds the data directory, encryption mode and master password settings moved out of the general tab; it is grouped with cloud sync under a new "Configuration Management" section.

**Bug Fixes**

- Terminal: fixes sessions failing to reconnect — after a session ended, the terminal stayed stuck on the reconnecting prompt and never picked up the new session.
- File transfer: fixes the standalone file panel's transfer list staying empty while a transfer ran.
- Terminal (macOS): fixes Vietnamese Telex input methods garbling text — typing "tieengs vieejt" could come out as "tieến việt" because the keystrokes an input method emits for one keypress (a delete plus the replacement text) reached the page out of order; the keystrokes of one keypress are now reordered by their intent. (@dungvv)

**Notes**

- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.
- Android app: since version 1.11.0, the Android app uses a new application ID based on the uniterm domain (`net.uniterm.app`) — if a version before 1.11.0 is installed, uninstall it first before installing the new version.

Thanks to @etanman and @dungvv for their contributions to this release.

### 更新内容

**新功能**

- 网络扫描：新增统一的 IP / 端口网络扫描器，入口在右上角菜单或侧边栏「新建连接」菜单——上方为带服务识别（SSH / Telnet / RDP / VNC / FTP / SMB / HTTP）的 IP 段扫描，下方为端口扫描。扫描结果可一键打开预填好的连接表单，IP 段自动预填本机所在 /24 子网。(@etanman)
- 远程桌面（VNC）：多显示器的 VNC 服务器新增显示器选择器（全部显示器或单个显示器），并支持自定义区域编辑，只查看桌面的一部分——在桌面上拖动、绘制、调整框体即可。所选显示器 / 区域按连接保存。
- 设置：新增「本地数据」标签页，数据目录、加密模式、主密码等设置从通用页移入；它与云同步一起归入新的「配置管理」分组。

**问题修复**

- 终端：修复会话重连失败的问题——会话结束后终端一直停留在「重新连接中」提示，无法接续新会话。
- 文件传输：修复独立文件面板在传输进行时传输列表一直为空的问题。
- 终端（macOS）：修复越南文 Telex 输入法打字乱序错乱的问题——输入 "tieengs vieejt" 可能变成 "tieến việt"，原因是输入法一次按键产生的 keystroke（删除 + 替换文本）到达页面的顺序不保证；现在会按按键意图对同一次按键的 keystroke 重新排序。(@dungvv)

**说明**

- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。
- 安卓 APP 从 1.11.0 版本起更换为基于 uniterm 域名的新应用 ID（`net.uniterm.app`），如果安装了该版本之前的版本，需要先卸载旧版后安装新版。

感谢 @etanman、@dungvv 对本次发布的贡献。

## v1.11.1

### What's Changed

**Bug Fixes**

- Terminal: `clear` keeps the history again — the screen content scrolls into the scrollback instead of being wiped, and the clear-scrollback action now drops only the rows above the viewport, leaving the current screen untouched.
- Terminal (macOS): fixes Backspace stopping to work after pasting text into the prompt. (@surenwuyuwuqiu)
- AI: fixes garbled terminal output returned by foreground jobs in the AI panel — merged fragments, lost or blank rows, and a first line starting mid-row without the shell prompt, on PowerShell, git bash and SSH sessions alike.
- AI: sessions started from a terminal are now named after both the connection and the topic of the first message, instead of the connection name alone; manual renames are never overwritten.
- Tray: fixes the tray menu always showing English when the language is set to follow the system, and switching the language in the UI now updates the tray immediately without a restart.
- Remote desktop (VNC / SPICE): pressing Escape in fullscreen is now forwarded to the remote desktop instead of exiting fullscreen, and the mouse cursor no longer disappears after entering fullscreen.
- Remote desktop (SPICE): auto-scale now upscales a desktop smaller than the window to fill fullscreen, matching VNC and RDP behavior.

**Notes**

- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.
- Android app: since version 1.11.0, the Android app uses a new application ID based on the uniterm domain (`net.uniterm.app`) — if a version before 1.11.0 is installed, uninstall it first before installing the new version.

Thanks to @surenwuyuwuqiu for their contribution to this release.

### 更新内容

**问题修复**

- 终端：`clear` 不再清空历史——屏幕内容滚动进入回滚区保留下来；「清空回滚区」操作也修正为只删除视口上方的内容，当前屏幕不受影响。
- 终端（macOS）：修复粘贴文本后退格键失效的问题。(@surenwuyuwuqiu)
- AI：修复 AI 面板前台任务回传的终端输出错乱问题——PowerShell 下内容合并、丢行，git bash 下行间出现空白，以及首行从行中开始、缺少 shell 提示符等，SSH 会话同样受影响。
- AI：从终端新建的 AI 会话命名改为结合连接名与首条消息的主题，不再只取连接名；手动重命名的会话不会被覆盖。
- 托盘：修复语言设置为「跟随系统」时托盘菜单始终显示英文的问题；在界面中切换语言后托盘菜单立即更新，无需重启。
- 远程桌面（VNC / SPICE）：修复全屏下按 Esc 会退出全屏而非发送到远端的问题，以及进入全屏后鼠标光标消失的问题。
- 远程桌面（SPICE）：自动缩放现在会将小于窗口的远程桌面放大铺满全屏，与 VNC、RDP 行为一致。

**说明**

- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。
- 安卓 APP 从 1.11.0 版本起更换为基于 uniterm 域名的新应用 ID（`net.uniterm.app`），如果安装了该版本之前的版本，需要先卸载旧版后安装新版。

## v1.11.0

### What's Changed

**New Features**

- Remote desktop: RDP is rebuilt on the IronRDP engine, replacing the previous ActiveX approach — RDP remote desktops can now be reached from macOS and Linux as well, not just Windows, credentials can be saved for NLA authentication, and connections automatically fall back to TLS login when the server rejects the NLA (CredSSP) handshake.
- Remote desktop: RDP / VNC / SPICE share one unified set of desktop controls (fullscreen, shortcut capture, aligned status bars), render at physical pixels for a sharper picture, and support bidirectional clipboard sync.
- Remote desktop: SPICE gains SSH tunnel support, and the session reports success only after the desktop channels are actually ready.
- Database: the result grid supports cell copy — click to select a cell and press Ctrl/Cmd+C, or use the context menu's Copy Cell / Copy Row (TSV, pastes into Excel with columns) / Copy Column Name. (@laigcss)
- Database: the tree panel shows all databases the account can access, not only the default set. (@laigcss)
- AI: long conversations compact automatically — older exchanges are condensed into a summary and sent along with only the recent turns, keeping requests small and fast and avoiding failures from exceeding the model's context limit.
- Cloud sync: skills can sync across devices via an opt-in skills.json scope — a new device pulls complete skills including their enabled / locked / sort preferences; hand-copied skill directories are picked up without a restart, and remote deletions never propagate, protecting local skills. (@surenwuyuwuqiu)
- SSH: terminal directory following is rebuilt as a non-intrusive dedicated probe channel, replacing shell cwd command injection; SFTP views opened from the terminal land directly in the shell's current directory, hosts restricted to SCP (no SFTP subsystem) are supported as well, and the now-obsolete "directory follow injection" option is removed from the connection form.

**Improvements**

- AI: reasoning effort (thinking intensity) is selectable and persisted per device.
- AI: a preset dropdown in the model configuration dialog pre-fills the protocol and base URL for common providers, so only the API key needs to be entered. (@laigcss)
- AI: terminal command output capture now waits for a reliable shell completion signal (the command actually finishing and the prompt returning) before reporting back, so the AI no longer receives incomplete terminal output and misjudges the result.
- AI: a model card can be duplicated with one click, and each model can define an extra HTTP header (e.g. a session ID header) sent on every request — for gateways that key prompt caching or sticky routing on it. (@surenwuyuwuqiu)
- AI: a new session is named after the terminal connection it was started from (connection name, falling back to user@host) instead of the generic "New Session". (@surenwuyuwuqiu)
- Import: MobaXterm session importing now covers the session types newly added in uniTerm. (@Merrcurys)
- Shortcuts: configurable shortcuts for toggling the file panel and the system monitor. (@laigcss)
- Database: the Oracle object browser lists every schema the account can access alongside the login schema, and opens the first schema automatically when the service name matches no schema; the connection field is now labeled "service name", required and validated on save.
- SFTP: the upload and download pickers remember the last used directory, and fall back to home when the recorded path no longer exists.
- UI: new, duplicated and companion tabs (duplicate session / channel, open SFTP or monitor from a context menu) now open right beside the tab they were triggered from — the source tab when there is one, otherwise the active tab — instead of always appending at the far end of the bar; the tab bar also keeps the active tab scrolled into view.
- Android: file exports (connection export, AI session markdown, database results / SQL, themes, terminal content) are handed to the system save-to picker — previously export flows did nothing on Android.

**Bug Fixes**

- Terminal: fixes a batch of input and rendering bugs — clear-screen behaving inverted, CJK characters truncated at the right screen edge, backspace eating into the shell prompt on shells whose stty erase doesn't match, and the backspace that deletes the last typed character never reaching the shell. (@laigcss)
- UI: fixes the database tree panel staying empty on first connect and ignoring refresh (panel state reactivity), and a blank window at startup caused by a missing import. (@laigcss)
- MCP client: after an external server's long connection drops, tool calls now redial automatically and the pending call replays transparently, instead of failing with "connection closed" until an app restart. (@surenwuyuwuqiu)
- Remote desktop: fixes macOS fullscreen, keyboard capture on Unix systems, and Android connections (loopback WebSocket permission, application ID).
- SSH: fixes agent authentication failing when the configured key path points at a public key — the agent key matching that public key is now used. (@lutzr400)
- AI: fixes requests to the current OpenCode Go inference endpoint missing the conversation session header. (@zhongqian97)
- i18n: fixes the missing settings.cancel translations in all locales. (@Merrcurys)

**Notes**

- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.
- Android app: starting with version 1.11.0, the Android app uses a new application ID based on the uniterm domain (`net.uniterm.app`) — if a version before 1.11.0 is installed, uninstall it first before installing the new version.

Thanks to @laigcss, @surenwuyuwuqiu, @Merrcurys, @lutzr400 and @zhongqian97 for their contributions to this release.

### 更新内容

**新功能**

- 远程桌面：RDP 改用 IronRDP 引擎重构，替换原有 ActiveX 方案——macOS、Linux 也可连接 RDP 远程桌面（不再限于 Windows），NLA 认证模式下支持保存密码，服务器拒绝 NLA（CredSSP）握手时自动改用 TLS 登录重试。
- 远程桌面：RDP / VNC / SPICE 统一为一套桌面控制（全屏、快捷键捕获、一致的状态栏），按物理像素渲染，画面更加清晰，并支持双向剪贴板同步。
- 远程桌面：SPICE 支持 SSH 隧道，且仅在桌面通道就绪后才报告连接成功。
- 数据库：结果网格支持单元格复制——点击选中单元格后 Ctrl/Cmd+C 复制，或通过右键菜单复制单元格 / 复制整行（TSV 格式，可带列粘贴进 Excel）/ 复制列名。(@laigcss)
- 数据库：树形面板展示账号可访问的全部数据库，不再局限于默认集合。(@laigcss)
- AI：长会话自动压缩上下文——较早的对话被归纳为摘要，仅随最近对话一同发送，请求更小、响应更快，避免超出模型上下文失败。
- 云同步：技能可通过可选的 skills.json 同步项跨设备同步——新设备一次拉取完整技能（含启用 / 锁定 / 排序偏好）；手工拷入的技能目录无需重启即可识别，远端删除不会传播，保护本地技能。(@surenwuyuwuqiu)
- SSH：目录跟随检测功能重构，升级为无侵入的独立探测通路，替代 shell cwd 命令注入；从终端打开的 SFTP 视图直接定位到 shell 当前所在目录，仅允许 SCP（无 SFTP 子系统）的主机同样支持；连接表单中已无作用的「目录跟随注入」选项随之移除。

**改进**

- AI：支持选择思考强度（reasoning effort），并按设备本地持久化。
- AI：模型配置弹窗新增预设下拉，可预填常用提供商的协议、Base URL，只需填写 API Key。(@laigcss)
- AI：终端命令输出采集等待可靠的 shell 完成信号（命令真正结束、提示符返回）后才回传，避免把不完整的终端输出发给 AI 导致误判。
- AI：模型卡片支持一键复制，每个模型可配置额外的 HTTP 请求头（如会话 ID 头）随每个请求发送——适用于按该头做提示缓存或粘性路由的网关。(@surenwuyuwuqiu)
- AI：新建会话默认以其来源终端连接命名（连接名，缺省回退 user@host），不再显示通用的「新会话」。(@surenwuyuwuqiu)
- 导入：MobaXterm 会话导入补充了 uniTerm 新增的会话类型。(@Merrcurys)
- 快捷键：新增文件面板和系统监视器开关的可配置快捷键。(@laigcss)
- 数据库：Oracle 对象浏览器列出账号可访问的全部 schema（登录 schema 之外的授权 schema 一并展示），服务名匹配不到 schema 时自动展开并打开第一个；连接字段更名为「服务名」，必填并在保存时校验。
- SFTP：上传 / 下载选择器记住上次使用的目录，目录已不存在时回退到主目录。
- 界面：新建、复制及衍生标签页（复制会话 / 通道，右键打开 SFTP / 监视器）现在紧跟在触发它的标签页旁打开（有源标签跟源标签，否则跟当前活动标签），不再总是追加到标签栏最右侧；标签栏也会自动滚动，让活动标签保持可见。
- 安卓：文件导出（连接导出、AI 会话 Markdown、数据库结果 / SQL、主题、终端内容）改走系统「保存到」选择器——此前在安卓上导出没有任何反应。

**问题修复**

- 终端：修复一批输入与渲染问题——清屏行为反转、中日韩字符在屏幕右边界被截断、stty erase 不匹配的 shell（如 Euler OS）上退格键吞掉提示符、以及删除最后一个字符的退格键被拦截无法到达 shell。(@laigcss)
- 界面：修复首次连接时数据库树形面板一直为空、刷新无效（面板状态响应式问题），以及启动时因缺少导入导致的白屏。(@laigcss)
- MCP 客户端：外部服务器长连接断开后自动重连并透明重放未完成的调用，不再出现「connection closed」报错直到重启应用。(@surenwuyuwuqiu)
- 远程桌面：修复 macOS 全屏、Unix 系统键盘捕获以及 Android 连接（回环 WebSocket 权限、应用 ID）。
- SSH：修复配置的密钥路径指向公钥时 agent 认证失败的问题——现在会选用 agent 中与该公钥匹配的密钥。(@lutzr400)
- AI：修复当前 OpenCode Go 推理端点的请求缺少会话头的问题。(@zhongqian97)
- 国际化：补齐所有语言缺失的 settings.cancel 翻译。(@Merrcurys)

**说明**

- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。
- 安卓 APP 从 1.11.0 版本起更换为基于 uniterm 域名的新应用 ID（`net.uniterm.app`），如果安装了该版本之前的版本，需要先卸载旧版后安装新版。

感谢 @laigcss、@surenwuyuwuqiu、@Merrcurys、@lutzr400 和 @zhongqian97 对本版本发布的贡献。

## v1.10.1

### What's Changed

**New Features**
- AI: image and file attachments — an image can be pasted straight from the clipboard into a conversation, and files can be attached by picking them or dragging them into the conversation; the attachments are sent to the model along with the message. (@surenwuyuwuqiu)
- Database: add SQLite database type support — select a local database file in the connection form and open it directly; SQLite connections imported from Navicat / DBeaver are also recognized.
- Import: mRemoteNG connection files can be imported — the folder structure is preserved and per-connection attributes (protocol, host, port, username) are mapped; passwords are imported when the export file uses supported encryption. (@andreafunicelli)
- Import: MobaXterm private keys can be imported, and PuTTY `.ppk` keys are supported as authentication keys — keys from PuTTY-based tools can be selected directly, without converting them to OpenSSH format first. (@chris-edstrom)
- MCP Client: the built-in AI can use tools from external MCP servers over HTTP, stdio or SSE, with encrypted storage for sensitive configuration. (@surenwuyuwuqiu)
- MCP Server: foreground execution lets AI operate in the current terminal after interactive login through a jump host or bastion, reaching the actual target node when a separate background SSH session cannot. It shares the built-in AI's terminal tools, supports screen capture, keyboard input and asynchronous output collection, and retains approval policies; background SSH execution remains available. (@surenwuyuwuqiu)
- MCP: file tools now run through the connection's file-transfer session, following the connection's transfer protocol — SFTP or SCP — so hosts whose SSH server provides no SFTP subsystem (common on embedded devices) work by switching the connection to SCP.
- Android: local terminals are available from the start page and connection form, with configurable default shell and system-shell / PATH fallbacks. (@laigcss)
- Cloud sync: SFTP, S3 and SMB join Git and WebDAV as sync sources, with connection testing and a shared snapshot engine for the four file-based backends.
- i18n: Portuguese and Vietnamese locales — the UI now ships pt / vi translations selectable in settings.

**Improvements**
- AI: starting a conversation automatically locks the AI to the currently active terminal, so replies are grounded in the right session without a manual lock step first.
- MCP: client configurations use add/edit dialogs with connection testing, a compact list, localized transport names and HTTP as the default; server port and execution mode are locked while running. Foreground tool parameters now match the built-in AI, replacing the previous parameter names.
- UI: sidebar widths persist across restarts — resized sidebars no longer reset to the default width on every launch.
- Connections: advanced settings are grouped into protocol, terminal and network sections; terminal type is configurable per SSH, Telnet and local connection, defaulting to `xterm-256color`, with improved field layout and translated labels.
- SSH tunnels: configurable automatic reconnection with a retry interval and random jitter for local, remote and dynamic forwarding. (@zhongqian97)
- RDP: input wakes the message loop immediately, reducing keyboard and mouse latency and improving redraw responsiveness. (@IBRAHIMELAMRABET)
- Cloud sync: only files whose content actually changed are uploaded — a content comparison skips unchanged files, cutting sync traffic and write churn on the cloud source.
- Terminal: the timestamp column gains a millisecond option, useful for timing fast-producing output precisely.
- Shells: the Git Bash path is configurable in settings for non-default install locations, and scoop-installed Git Bash is detected automatically. (@chris-edstrom)
- Workspace: a group can be selected when saving / editing a workspace so workspaces can be organized into groups, and new creation entries make saving one reachable from more places.
- Mobile: options that only apply on desktop platforms are hidden on mobile, keeping the interface uncluttered on small screens; detail drawers no longer overflow the window bounds, and the database row dialog columns are rebalanced for narrow windows.
- Start page: sections (favorites / recent / groups) can be hidden and reordered from the top-right menu.
- S3: large objects download much faster with parallel ranged GETs, falling back to a single stream on endpoints that don't support ranges.
- Tabs: the tab bar's plus button and the new-tab shortcut get a configurable default action — the start page (default) or a local terminal using the default shell.

**Bug Fixes**
- AI: fixes requests to the OpenCode Zen Go endpoint missing the required `x-opencode-session` header — the session ID is now sent so the server can correctly track the conversation session. (@andreafunicelli)
- MCP: approval dialog text can be selected and copied.
- Terminal: fixes pasted multi-line text inserting LF line feeds that Windows local sessions mishandle (ConPTY expects CR) — newlines are converted to CR when bracketed paste is off, so pasting a multi-line command sequence now executes correctly.
- Terminal: fixes multi-character IME key rewrites not being delivered to the terminal — keyboards whose IME rewrites a key press into a string (such as Vietnamese Telex input) now work in the terminal. (@dungvv)
- SSH: fixes key-based authentication failing when a stale remembered password was left on the host — the leftover password no longer breaks the key attempt.
- SSH: fixes the cwd hook injection interfering with sessions that run post-login automation (commands or scripts started automatically at login); the injection is reworked, and WSL and SSH now share the same typed state machine.
- SSH: fixes GSSAPI / Kerberos authentication failing when `KRB5CCNAME` points at a Windows drive path — such paths are now treated as FILE ticket caches.
- RDP: fixes security dialogs blocking connections on operating systems using languages other than Chinese or English; credential prompts still require user input.
- Credentials: data-safety hardening — a recovered key is verified against the actual data before unlocking, so a bad recovery can no longer produce a half-unlocked store; the reset flow derives the affected field list from the store manifest instead of a hardcoded list; and the unlock dialog is reworded with an exit option.
- Import: fixes `.utm` exports depending on the exporting machine's credential store — exports are now self-contained (credentials travel inside the connections) and every secret field is encrypted with the export password.
- App: fixes nil-window crashes in headless/server mode; local file browsing falls back to the home directory when sandbox permissions prevent reading the root directory. (@laigcss)
- App: a fixed webview user-data folder is used instead of a per-process temp dir, so WebView data is no longer scattered across temp folders that accumulate; legacy per-pid temp dirs are swept at startup.
- App: refuses to migrate the data directory into itself (nesting it under its own subfolder), which previously corrupted the directory layout.
- Database: fixes binary Guid columns rendering as garbled bytes in the result grid — SQL Server `UNIQUEIDENTIFIER` and Oracle `RAW(16)` columns (the form EF Core / ABP write Guid keys in) now display as formatted GUID text, and other non-text binary values render as `0x`-prefixed hex. (@IBRAHIMELAMRABET)
- Shells: scoop-installed Git Bash is labeled separately from system Git Bash, so the two entries in the shell list are distinguishable. (@chris-edstrom)
- Terminal: fixes full-screen TUIs (ncurses menus, ...) rendering incorrectly under heavy load — no more corrupted output or leftover content after exiting.
- Credentials: fixes the credential store failing to unlock after the data directory is moved or copied, and a possible lockout after switching the encryption mode or changing the master password.
- UI: fixes a black flash at startup when a light theme is configured.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu, @dungvv, @andreafunicelli, @chris-edstrom, @IBRAHIMELAMRABET, @zhongqian97 and @laigcss for their contributions to this release.

### 更新内容

**新功能**
- AI：支持图片与文件附件——可直接从剪贴板把图片粘贴进会话，也可通过文件选择或拖拽附加文件，附件随消息一并发送给模型。(@surenwuyuwuqiu)
- 数据库：新增 SQLite 数据库类型支持——可在连接表单中选择本地数据库文件并直接打开；同时支持识别从 Navicat / DBeaver 导入的 SQLite 连接。
- 导入：支持导入 mRemoteNG 连接文件——保留文件夹层级结构，映射各连接的协议、主机、端口、用户名等属性；导出文件使用受支持加密方式时密码可一并导入。(@andreafunicelli)
- 导入：支持导入 MobaXterm 私钥，并支持使用 PuTTY `.ppk` 密钥认证——来自 PuTTY 系工具的密钥可直接选用，无需先转换为 OpenSSH 格式。(@chris-edstrom)
- MCP 客户端：内置 AI 可通过 HTTP、stdio 或 SSE 使用外部 MCP 服务器的工具，敏感配置加密存储。(@surenwuyuwuqiu)
- MCP 服务端：新增前台执行模式，支持在跳板机、堡垒机等场景下完成交互式登录后，让 AI 直接操作当前终端，解决独立后台 SSH 会话无法访问实际目标节点的问题。与内置 AI 共用终端工具，支持屏幕捕获、按键输入和异步输出收集，保留审批策略；同时保留后台 SSH 执行模式。 (@surenwuyuwuqiu)
- MCP：文件工具改走连接的文件传输会话，传输协议跟随连接设置——支持 SFTP 或 SCP，没有 SFTP 子系统的主机（嵌入式设备常见）把连接改为 SCP 即可使用。
- Android：开放起始页和连接表单中的本地终端入口，支持默认 Shell 设置，并补充系统 Shell 和 PATH 回退处理。(@laigcss)
- 云同步：新增 SFTP、S3、SMB 同步源，与 Git、WebDAV 一同可选；四种文件型后端共用快照同步引擎，并支持连接测试。
- 国际化：新增葡萄牙语、越南语——设置中现可选 pt / vi 界面语言。

**改进**
- AI：开始会话时自动将 AI 锁定到当前活动终端，回复自动关联正确的会话，无需先手动锁定。
- MCP：客户端配置改用新增 / 编辑弹窗，支持弹窗内测试连接，列表布局更紧凑，传输方式名称本地化并默认选择 HTTP；服务运行期间禁止修改端口和执行模式。前台工具参数与内置 AI 统一，替换原有参数名。
- 界面：侧边栏宽度在重启后保持，调整过的侧边栏不再每次启动都恢复默认宽度。
- 连接：高级设置按协议、终端、网络分组；SSH、Telnet 和本地连接可分别设置终端类型，默认 `xterm-256color`，并优化字段布局和翻译文案。
- SSH 隧道：支持配置自动重连、重试间隔和随机抖动，适用于本地、远程及动态转发。(@zhongqian97)
- RDP：输入事件立即唤醒消息循环，降低键盘和鼠标延迟，改善画面刷新响应。(@IBRAHIMELAMRABET)
- 云同步：只上传内容真正有变化的文件——通过内容比对跳过未变化的文件，减少同步流量和对云端的写入。
- 终端：时间戳列新增毫秒选项，可更精确地度量快速产出的输出。
- Shell：Git Bash 路径可在设置中配置以支持非默认安装位置，并自动检测 scoop 安装的 Git Bash。(@chris-edstrom)
- 工作区：保存 / 编辑工作区时可选择分组，便于将工作区归入分组管理；同时新增了创建入口，保存工作区更容易触达。
- 移动端：在移动平台上隐藏仅桌面适用的选项，小屏界面更清爽；详情抽屉不再超出窗口边界，数据库行对话框列宽为窄窗口重新分配。
- 起始页：各区块（收藏 / 最近 / 分组）可隐藏与排序，设置入口在起始页右上角。
- S3：大对象下载提速——改为并发分段下载，不支持的端点自动回退单流。
- 标签页：标签栏加号按钮与新标签快捷键的默认行为可配置——可选打开起始页（默认）或使用默认 Shell 的本地终端。

**修复**
- AI：修复请求 OpenCode Zen 的 Go 端点时缺少必需的 `x-opencode-session` 会话头的问题——现在会传入会话 ID，服务端可正确关联同一会话。(@andreafunicelli)
- MCP：审批弹窗中的文字支持选中复制。
- 终端：修复 Windows 本地会话中粘贴多行文本插入 LF 换行导致命令出错的问题（ConPTY 期望 CR）——关闭括号粘贴时换行符转换为 CR，粘贴多行命令序列可正确执行。
- 终端：修复多字符输入法按键改写未送达终端的问题——会把单次按键改写为字符串的输入法（如越南语 Telex）现在可正常使用。(@dungvv)
- SSH：修复主机残留旧登录密码导致密钥认证失败的问题——遗留密码不再破坏密钥认证。
- SSH：修复 cwd 钩子注入干扰登录后自动化（登录时自动执行的命令或脚本）的问题；注入逻辑重构，WSL 与 SSH 现共用同一状态机。
- SSH：修复 GSSAPI / Kerberos 认证在 `KRB5CCNAME` 指向 Windows 盘符路径时失败的问题——这类路径现在按 FILE 票据缓存处理。
- RDP：修复非中英文操作系统上的安全提示弹窗阻塞连接的问题；凭据输入弹窗仍需用户操作。
- 凭据：数据安全加固——恢复的密钥解锁前先对实际数据校验，错误的恢复不再产生半解锁的存储；重置流程按存储清单推导受影响字段，不再依赖硬编码列表；解锁对话框文案调整并新增退出选项。
- 导入：修复 `.utm` 导出依赖导出机器凭据存储的问题——导出现在自包含（凭据随连接一并写入），且所有机密字段均用导出密码加密。
- 应用：修复无窗口 / 服务端模式的空指针崩溃；沙箱权限禁止读取根目录时，本地文件浏览自动回退到用户主目录。(@laigcss)
- 应用：WebView 用户数据目录改为固定路径，不再按进程创建临时目录（避免临时目录不断累积）；启动时清理历史遗留的按进程临时目录。
- 应用：拒绝将数据目录迁移到其自身内部（嵌套在自身子目录下），此前会破坏目录结构。
- 数据库：修复二进制 Guid 列在结果网格中显示为乱码的问题——SQL Server 的 `UNIQUEIDENTIFIER` 与 Oracle 的 `RAW(16)` 列（EF Core / ABP 写入 Guid 主键的形式）现按格式化 GUID 文本显示，其他非文本二进制值以 `0x` 前缀的十六进制显示。(@IBRAHIMELAMRABET)
- Shell：scoop 安装的 Git Bash 与系统 Git Bash 分开标注，shell 列表中两者可区分。 (@chris-edstrom)
- 终端：修复负载高时全屏 TUI（ncurses 菜单等）界面显示错乱、退出后残留内容的问题。
- 凭据：修复数据目录移动或复制后凭据存储无法恢复解锁的问题，以及切换加密模式 / 修改主密码后可能锁死存储的问题。
- 界面：修复浅色主题下启动时闪黑的问题。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @surenwuyuwuqiu、@dungvv、@andreafunicelli、@chris-edstrom、@IBRAHIMELAMRABET、@zhongqian97 和 @laigcss 对本版本的贡献。

## v1.10.0

### What's Changed

**New Features**
- MCP Server: a built-in MCP (Model Context Protocol) server lets external AI agents (Claude Code, Cursor, Trae, ...) run commands on saved SSH connections and transfer files. (@surenwuyuwuqiu)
- Android: the first Android release — touch and narrow-viewport adaptations for phone use.
- Sync: WebDAV is now available as a sync source alongside Git — both can serve as cloud sync sources. The sync scope is now configurable.
- Containers: a container file browser, image management and an overview page. Four runtimes are supported — Docker / Podman / nerdctl (containerd) / WSLC — with WSLC contributed by @boltomli.
- Kubernetes: a container file browser and an overview page, with create templates completed for more resource types.
- AI: a collapsible thinking view — the Thinking indicator is clickable and expands into a live view streaming the model's reasoning, with a status indicator and elapsed timer; completed thinking is saved with the message as a collapsed block showing duration and timestamp. (@Teejer)

**Improvements**
- UI: the dark theme palette is refreshed and corner-radius tiers unified; dialog presentation is optimized with a capped height, and hover states use native cursors.
- Transfer: S3-compatible object storage gains streaming uploads (no whole-file buffering), listing pagination and per-file retry support.
- Remote desktop: RDP resolution now adapts to follow the window size.
- Connections: the login username can be overridden per connection when using identity auth, so one key works across servers with different users.
- Terminal: tabs can be restored at startup — a prompt at launch offers to reopen the tabs from the last session, enabled by default.
- Terminal: network-device highlighting rules — Cisco IOS / Huawei VRP interface names and config keywords are recognized in output.
- Terminal: the right/middle click actions gain a "No action" option, so mouse-tracking apps (vim, htop, ...) keep receiving those clicks. (#1088)
- Sync: Git sync repo history is auto-compacted to bound repo size.

**Bug Fixes**
- SSH: fixes connections failing with EOF on certain devices (dropbear on OpenWrt CPEs, for example) by detecting the shell before the shell channel opens. (@sonnartliao)
- SSH: fixes the injected cwd hook command line occasionally flashing in the terminal after connect; injection is hardened, and a new per-connection setting can defer it until directory follow is first enabled.
- SSH / WSL: fixes the login MOTD not displaying on SSH and WSL, and failed logins on minimal CLI-only appliances (the cwd-hook probe now uses its own short-lived connection).
- SMB: fixes transfers plateauing around 35MB/s on a typical LAN by pipelining large concurrent chunks instead of one round trip per 64KB — the link can now be saturated.
- FTP: fixes garbled directory listings and filenames by decoding names with the configured charset; cancelling a single-file transfer now takes effect immediately.
- S3: fixes creating an empty file / folder failing with an error; failures now surface a clear toast instead of going unnoticed.
- MongoDB: fixes connections failing over an SSH tunnel; a failed connect keeps the tab open with the concrete error instead of tearing it down, and the form gains default-database and extra-params fields.
- Database: fixes the duplicated extra-params field, the unusable test-connection button (mongodb / redis / elasticsearch) and the missing API Key input in the NoSQL / Elasticsearch connection forms.
- Containers: fixes silently failing when a container references an SSH host without saved credentials — the credential dialog now opens.
- Terminal: fixes the app freezing when a command floods the terminal with output.
- Terminal: fixes highlighting not applying to lines that span a soft wrap.
- SMB: fixes a session dying silently after idle — reconnect now restores the previous directory.
- UI: fixes progress bars and status indicators turning invisible in background-image mode.
- Linux: fixes the running window showing as an orphan with a generic icon on GNOME Wayland — the desktop file is now named after the Wayland app_id (`org.wails.uniterm.desktop`) so the window correctly matches its launcher. (@Teejer)

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu, @boltomli, @Teejer and @sonnartliao for their contributions to this release.

### 更新内容

**新功能**
- MCP Server：内置 MCP（Model Context Protocol）服务器，外部 AI Agent（Claude Code、Cursor、Trae 等）可在已保存的 SSH 连接上执行命令、传输文件。(@surenwuyuwuqiu)
- Android：新增安卓版发布——针对手机屏幕的触控与窄视口适配。
- 同步：新增 WebDAV 作为同步源，现支持 Git、WebDAV 两种云同步源。新增支持配置同步范围。
- 容器：新增容器文件浏览器、镜像管理和总览页。支持 Docker / Podman / nerdctl（containerd）/ WSLC 四种运行时，其中 WSLC 由 @boltomli 贡献。
- Kubernetes：新增容器文件浏览器和总览页，补全更多资源类型的创建模板。
- AI：新增可折叠的思考过程视图——点击"思考中"指示器可展开实时查看模型的思考内容，带状态指示与耗时计时；完成后随消息保存为可折叠的思考块，显示耗时与时间戳。(@Teejer)

**改进**
- 界面：深色主题配色焕新、圆角层级统一；对话框展示效果优化并限制最大高度，悬停使用原生光标。
- 文件传输：S3 兼容对象存储支持流式上传（不再整文件缓冲）、列表分页和单文件重试。
- 远程桌面：RDP 分辨率自适应窗口大小。
- 连接：身份认证下可按连接覆盖登录用户名，一把密钥可用于多台不同用户名的服务器。
- 终端：支持启动时恢复标签页——启动时询问是否恢复上次会话的标签，默认开启。
- 终端：新增网络设备高亮规则——识别 Cisco IOS / 华为 VRP 的接口名与配置关键字。
- 终端：右键、中键动作新增「无动作」选项，开启鼠标追踪的程序（vim、htop 等）可正常接收点击。(#1088)
- 同步：Git 同步仓库历史自动压缩，避免仓库无限膨胀。

**修复**
- SSH：修复部分设备（如 OpenWrt 上的 dropbear）连接报 EOF 失败的问题，现在先探测 shell 类型再打开 shell 通道。(@sonnartliao)
- SSH：修复连接后终端偶尔闪现 cwd 钩子注入命令行的问题；注入逻辑加固，并新增连接级设置，可选择仅在启用"目录跟随"时注入。
- SSH / WSL：修复 SSH 和 WSL 登录 MOTD 不显示、部分 CLI-only 设备登录失败（cwd 钩子探测改走独立短连接）的问题。
- SMB：修复传输吞吐卡在约 35MB/s 的问题，改为大块并发流水线传输、不再逐块往返等待，局域网内可跑满带宽。
- FTP：修复服务器目录列表与文件名乱码的问题，文件名按配置的字符集解码；单文件传输的取消现在可立即生效。
- S3：修复新建空文件 / 目录直接报错的问题，失败时也会给出明确提示，不再静默。
- MongoDB：修复 SSH 隧道下连接失败的问题；连接失败时标签页保留并显示具体错误、不再直接关闭，表单补充默认数据库与额外参数字段。
- 数据库：修复 NoSQL / Elasticsearch 连接表单的几处问题——额外参数字段重复显示、mongodb/redis/elasticsearch 的"测试连接"按钮不可用、API Key 输入框不出现。
- 容器：修复引用的 SSH 主机没有保存凭据时静默失败的问题，现在会弹出凭据录入对话框。
- 终端：修复命令大量输出时整个应用卡死的问题。
- 终端：修复跨软换行的行高亮不生效的问题。
- SMB：修复空闲后会话静默失效的问题——重连后恢复原目录。
- 界面：修复背景图模式下进度条、状态指示不可见的问题。
- Linux：修复 GNOME Wayland 下运行窗口显示为无图标孤立窗口的问题——桌面文件改为按 Wayland app_id 命名（`org.wails.uniterm.desktop`），窗口可正确关联启动器图标。(@Teejer)

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @surenwuyuwuqiu、@boltomli、@Teejer 和 @sonnartliao 对本版本的贡献。

## v1.9.5

### What's Changed

**New Features**
- Tray: a system tray icon with left-click show/hide and a menu (show window, hide to tray, reset window position, open settings, about, quit), an OS-global show/hide hotkey (default Ctrl+M, configurable in the keyboard settings), and single-instance locking — launching a second instance focuses the running window instead.
- Workspace overhaul: workspaces are now built by pointing and clicking — an "open in workspace" context-menu entry (multi-select aware) and an empty workspace with a drag-here placeholder; connections dragged onto a panel connect and split at the drop position. A workspace can be dissolved (members return to the tab bar as live tabs, broadcast and AI lock preserved) and saved as a "workspace"-type connection that rebuilds the live workspace on open. Connections can also be dragged onto the tab bar to open a tab at that position.
- SSH: per-connection algorithm modes — **compatible** (default, full support set with security-leaning ordering), **secure** (modern algorithms only: no SHA-1 exchanges, CBC ciphers or DSA host keys) and **custom** (four editable, ordered preference lists for key exchange, ciphers, MACs and host keys, validated against a closed candidate pool). Compatible mode completes the AES-CBC family (aes192-cbc / aes256-cbc) and prefers plain RSA host keys, so legacy devices and servers with malformed ECDSA P-521 keys still connect; negotiation failures carry mode-specific guidance.
- SSH: "duplicate channel" — open a new tab as a fresh session channel on the source session's already-authenticated connection: no re-dial, no re-authentication (handy behind a jump host with 2FA). (@surenwuyuwuqiu)
- Terminal: sixel and iTerm2 (OSC 1337) image support for remote terminals — images render directly in the terminal.
- Terminal: remote programs (vim, tmux, multi-hop SSH chains) can write to the local clipboard via OSC 52, and applications can drive a tab progress indicator via OSC 9;4, rendered as a thin bar along the tab's bottom edge (percentage / error / indeterminate / paused).
- Terminal: the text highlighting engine is rewritten — SGR re-injection is upgraded to overlay highlighting that recolors cells directly in the xterm buffer without touching the output stream, completely fixing cases where highlighting did not take effect and the garbled output caused by injection errors; highlighting also now applies to all terminal types (local, WSL, serial, k8s, container, telnet, SSH) instead of SSH only.
- Terminal: "reset terminal output" and "clear terminal scrollback" actions in the terminal / tab / panel context menus. Reset wipes the screen and the scrollback history and redraws the prompt (like MobaXterm's screen-cleaning); clear-scrollback drops only the rows above the viewport, leaving the screen and any half-typed command intact. Line numbers restart after either action. (@surenwuyuwuqiu)
- Terminal search: the first match is anchored at the bottom of the buffer (closest to the prompt) instead of the top of scrollback; Enter searches upward and Shift+Enter downward; the search bar gains case-sensitive, regex and whole-word toggles that persist across open/close.
- Import: SPICE/VNC `.vv` connection files used by Virt-Viewer (the one-shot desktop-session files downloaded from hypervisor VM console pages) can be imported, mapping type, host, port, username and password.

**Improvements**
- SSH: the silent cwd-reporting hook is now always injected invisibly after the shell starts, sessions start as plain shells again (sshd prints its own MOTD), and the per-connection toggle is removed — "follow terminal path" is a display-side switch.
- SFTP / SCP transfers are now pipelined — concurrent offset-addressed requests overlap round trips instead of stop-and-wait. A 4 MiB transfer over an 80 ms link drops from ~10.7 s to ~0.6 s (upload) and ~6.1 s to ~1.2 s (download); progress, pause and cancel semantics are unchanged.
- SFTP / SCP: standalone SFTP and SCP tabs gain an "open terminal" tab-context action, and SSH-derived file tabs gain it too — a file tab can now reach a shell on the same host and vice versa. (@surenwuyuwuqiu)
- Monitor: the process list now includes every process (previously the top 30 by CPU) with progressive loading as you scroll, so search finds all of them.
- Monitor: polling is robust against hosts whose script output leaves a variable empty (each numeric field defaults, raw output is logged on parse failure), df is guarded with a timeout so stale NFS mounts cannot wedge the poll loop, the disk list now includes every mounted filesystem (overlay, tmpfs, bind mounts) alongside block devices, and monitor table columns are drag-resizable.
- Native file pickers now open at a useful directory: the private-key picker starts at ~/.ssh, kubeconfig at ~/.kube, SFTP upload / "download to" at the local pane's directory — and hidden files are shown so dot-directories are reachable. (@surenwuyuwuqiu)
- Settings: the settings category sidebar is collapsible (auto-collapses on narrow windows) and setting cards stack vertically below phone widths, so settings are usable on small screens.
- New settings for the host list menu style (button or right-click only) and tab shortcut hints; the tab tooltip always shows the shortcut.
- Themes: a terminal theme submenu in the main menu (follow-app, built-in themes grouped dark/light, custom themes) applied live on click. The app theme now defaults to "system" (follows the OS color scheme) instead of dark, and theme / language / terminal-theme picks no longer close the menu, so options can be flipped through and previewed in place.
- Element-plus scrollbars stay visible and are styled like the native ones (no pointer cursor, instant hover highlight).
- UI: the app window gained a 1px outer frame with divider lines under the title bar and along both sidebar edges, staying visible on fractional-DPI displays; decorative gradient lines were removed from the title bar and sidebar, and the keyboard-settings table columns are unified.

**Bug Fixes**
- Terminal: pressing Enter in local sessions no longer resets mouse tracking, fixing wheel scrolling in fullscreen TUIs like vim. (@sonnartliao)
- Terminal: device query responses (cursor position, device status, window ops) are now passed through to the PTY. Apps that query the terminal and block on the answer — any `docker compose` confirmation prompt built on the survey library — previously froze the terminal completely, with no echo and a dead Ctrl-C.
- Terminal: select-copy and right-click paste are self-healing — clipboard writes pick the best path per platform with a timeout and fall back to the other path, reads retry once to ride out transient clipboard locks, and re-selecting the same text copies again after a failed copy.
- Terminal: MAC addresses no longer highlight as clock times (the HH:MM:SS rule now requires time-like boundaries), and the keyword rules gained fatal / critical / exception / panic / abort (error) and bare warn / caution (warning).
- SSH: wrong-password failures surface faster — a rejected password reaches the interactive prompt in a single handshake, each re-challenge prints a denial line (matching the OpenSSH client), and auth-type failures open the credential dialog immediately instead of after two more reconnects.
- AI: command output from background panels is read correctly. Keep-alive-frozen panels previously returned stale or empty output and timed out; a headless mirror terminal now parses the session stream continuously and serves as the read source while the panel is inactive.
- AI: a command confirmed by the user is replayed on the panel the model targeted — in multi-panel mode it could previously run on the wrong terminal — and the confirmation card shows the target panel. (@surenwuyuwuqiu)
- AI: fixed a compatibility issue where the AI sidebar composer could not be clicked or typed into on WebKit shipped with older macOS versions. (@zhangsir1211)
- UI: AI lock is signalled by turning the tab and panel icons warning-coloured instead of tinting the whole tab surface, which read muddy on dark themes.
- Tabs: dragging to reorder no longer steals the selection to the neighbouring tab, and the tab merge drop-zone indicator matches the workspace split style.
- WSL: sessions start in the distro user's home directory instead of a /mnt/c path under the app's own working directory.
- Windows: the WebView2 autofill-wallet feature is disabled — WebView2 152/153 runtimes call LogonUser with an invalid password once per environment creation at startup (a Chromium regression), producing a 4625 failed-logon event per launch that can trip account lockout policies.
- Settings: tab shortcut hints and host list menu style persist across restarts (they silently reset before), and the system title bar choice survives a relaunch.
- Linux: the .desktop launcher path matches the packaged binary, so launching from the application menu works after installing the deb/rpm package. (@surenwuyuwuqiu)
- SFTP: the max concurrent transfers setting is shown for standalone SFTP connections (the backend applied it, but the form hid the field).
- UI: the connection form resets only after the dialog's close transition finishes, so default values no longer flash while the dialog fades out.
- UI: batch fixes — custom window controls stay drawn until the native title bar setting takes effect after a relaunch, the chmod octal input is a plain text field again, multi-file SFTP drags show a drag ghost, and the tab strip end-drop indicator no longer pushes the add button to the far right.
- SSH: `~` in private key paths is expanded at import and connect time.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu, @zhangsir1211 and @sonnartliao for their contributions to this release.

### 更新内容

**新功能**
- 托盘：新增系统托盘图标，左键单击显示/隐藏窗口，菜单包含 显示窗口、隐藏到托盘、重置窗口位置、打开设置、关于、退出；支持系统级全局显隐快捷键（默认 Ctrl+M，可在键盘设置中修改）；应用单实例锁定——再次启动会唤起已运行的窗口而不是开新进程。
- 工作区大改版：工作区支持点选创建——右键菜单「在工作区中打开」（支持多选）以及带拖放占位的空工作区；连接拖入面板即连接并按落点位置分屏。工作区可解散（成员以实时终端标签回到标签栏，广播与 AI 锁定保留），也可保存为「工作区」类型连接，再次打开即重建整个工作区。连接还可拖到标签栏的任意位置直接在该处打开标签。
- SSH：连接级算法模式——**兼容**（默认，完整支持集、安全优先排序）、**安全**（仅现代算法：不含 SHA-1 交换、CBC 密码套件与 DSA 主机密钥）与**自定义**（密钥交换 / 密码套件 / MAC / 主机密钥四组可编辑的有序优先列表，限定封闭候选池）。兼容模式补全 AES-CBC 家族（aes192-cbc / aes256-cbc），并优先使用普通 RSA 主机密钥，老旧设备与 ECDSA P-521 主机密钥异常的服务器也能连上；协商失败时给出按模式定制的提示。
- SSH：新增「复制通道」——在已认证的源会话连接上开一个新的会话通道：无需重新拨号、无需重新认证（经 2FA 跳板机时尤其方便）。（@surenwuyuwuqiu）
- 终端：远程终端支持 sixel 与 iTerm2（OSC 1337）图片，图像直接在终端中渲染。
- 终端：远程程序（vim、tmux、多跳 SSH 链）可通过 OSC 52 写入本地剪贴板；应用可通过 OSC 9;4 驱动标签进度指示，沿标签底边渲染一条细进度条（百分比 / 错误 / 不定进度 / 暂停）。
- 终端：文本高亮方案重构——SGR 重注入升级为覆盖式高亮，直接对 xterm 缓冲区单元格重新着色，不触碰输出流，彻底解决部分场景高亮不生效以及注入错误导致的乱码问题；同时高亮从仅 SSH 扩展到所有终端类型（本地、WSL、串口、k8s、容器、telnet、SSH）。
- 终端：终端 / 标签 / 面板右键菜单新增「重置终端输出」与「清空终端回滚」。重置会清空屏幕与回滚历史并重绘提示符（类似 MobaXterm 的清屏）；清空回滚只删除视口上方的行，屏幕与正在输入的命令保持不动。两种操作后行号都从 1 重新开始。（@surenwuyuwuqiu）
- 终端搜索：首个匹配定位到缓冲区底部（紧邻提示符）而不是回滚区顶部；Enter 向上搜索，Shift+Enter 向下搜索；搜索栏新增 区分大小写、正则、全字匹配 三个开关，开关状态在关闭再打开后保留。
- 导入：支持导入 Virt-Viewer 的 SPICE/VNC `.vv` 连接文件（从虚拟化管理台控制台页下载的一次性桌面会话文件），映射类型、主机、端口、用户名与密码。

**改进**
- SSH：静默的 cwd 上报钩子改为 shell 启动后始终无感注入，会话恢复为普通 shell 启动（sshd 自行打印 MOTD 欢迎信息）；移除按连接开关，「跟随终端路径」变为纯显示侧开关。
- SFTP / SCP 传输改为流水线式——并发的按偏移寻址请求重叠往返，替代停止等待。80ms 时延链路上传输 4 MiB：上传从约 10.7s 降到约 0.6s，下载从约 6.1s 降到约 1.2s；进度、暂停与取消语义不变。
- SFTP / SCP：独立的 SFTP 与 SCP 标签页新增「打开终端」标签右键菜单项，SSH 衍生的文件标签页同样支持——文件页签与同一主机的 shell 互通。（@surenwuyuwuqiu）
- 监控：进程列表改为全量进程（原先只有 CPU 前 30），随滚动渐进加载，搜索可找到全部进程。
- 监控：脚本输出变量为空不再导致整个 JSON 失效（数值字段逐一兜底，解析失败时记录原始输出）；df 加了超时保护，陈旧的 NFS 挂载不再卡死轮询；磁盘列表涵盖所有挂载文件系统（overlay、tmpfs、bind 挂载）及块设备；监控表格支持拖拽调整列宽。
- 原生文件选择器从有用的目录打开：私钥选择器从 ~/.ssh、kubeconfig 从 ~/.kube、SFTP 上传 /「下载到」从本地面板当前目录开始，并显示隐藏文件，点目录可达。（@surenwuyuwuqiu）
- 设置：设置分类侧栏可折叠（窄窗口自动折叠），窄屏下设置卡片改为纵向堆叠，小屏幕上设置页也可正常使用。
- 新增 主机列表菜单样式（按钮或仅右键）与 标签快捷键提示 两个设置；标签 tooltip 始终显示快捷键。
- 主题：主菜单新增终端主题子菜单（跟随应用、内置主题按深浅色分组、自定义主题），点击即时生效。应用默认主题改为「跟随系统」（跟随操作系统配色，原为深色）；主题 / 语言 / 终端主题选择后不再关闭菜单，可连续切换预览。
- Element-plus 滚动条常驻显示，样式与原生滚动条一致（无 pointer 光标、悬停即时高亮）。
- 界面：应用窗口新增 1px 外边框，标题栏下方与两侧边栏带分隔线，分数缩放（高 DPI）下边框保持可见；移除标题栏与侧栏的装饰性渐变线，键盘设置表格列样式统一。

**Bug 修复**
- 终端：本地会话中按 Enter 不再重置鼠标追踪，修复 vim 等全屏 TUI 中滚轮退化为方向键的问题。（@sonnartliao）
- 终端：设备查询应答（光标位置、设备状态、窗口操作）现在会透传给 PTY。此前凡是查询终端并阻塞等待应答的程序——所有基于 survey 库的 `docker compose` 确认提示——都会把终端彻底卡死：无回显、Ctrl-C 失效。
- 终端：选中即复制与右键粘贴可自愈——剪贴板写入按平台选择最优路径并带超时，失败回退另一条路径；读取在空结果/出错后重试一次以渡过其他进程的瞬时剪贴板锁；复制失败后重新选中同一段文本也能再次复制。
- 终端：MAC 地址不再被当作时间高亮（HH:MM:SS 规则增加时间样式边界判定），关键词新增 fatal / critical / exception / panic / abort（错误级）与 warn / caution（警告级）。
- SSH：错误密码更快得到反馈——被拒绝的密码一次握手即到达交互提示；每次重新质询前先打印一条拒绝提示（与 OpenSSH 客户端一致）；认证方式类失败立即弹出凭据对话框，而不是再重连两次后才出现。
- AI：后台面板的命令输出读取正确。此前被 KeepAlive 冻结的面板会返回过期或空输出并超时；现在每个会话有一个持续解析会话流的无头镜像终端，面板不活跃时以它为读取来源。
- AI：用户确认的命令会回放到模型指定的面板——多面板模式下此前可能在错误的终端上执行——确认卡片上显示目标面板。（@surenwuyuwuqiu）
- AI：修复 AI 侧栏输入框在老版本 macOS 的 WebKit 上无法点击定位光标与输入的兼容性问题。（@zhangsir1211）
- 界面：AI 锁定改为将标签与面板图标染成警示色，取代原先将整个标签表面着色的方式（深色主题下显得浑浊）。
- 标签：拖拽排序不再把选中抢到相邻标签；标签合并的落区指示样式与工作区分屏保持一致。
- WSL：会话从发行版用户的主目录启动，而不是落到应用自身工作目录对应的 /mnt/c 路径。
- Windows：禁用 WebView2 自动填充钱包特性——WebView2 152/153 运行时在启动创建环境时会用当前账户和错误密码调用一次 LogonUser（Chromium 回归），每次启动产生一条 4625 失败登录事件，可能触发账户锁定策略。
- 设置：标签快捷键提示与主机列表菜单样式在重启后保留（此前每次重启静默重置）；系统标题栏选择在重启后生效。
- Linux：.desktop 启动器路径与安装的二进制一致，安装 deb/rpm 包后可从应用菜单正常启动。（@surenwuyuwuqiu）
- SFTP：独立 SFTP 连接显示最大并发传输数设置（后端已生效，但表单未显示该字段）。
- 界面：连接表单在对话框关闭过渡动画结束后才重置，淡出过程中不再闪烁默认值。
- 界面：批量修复——重启后原生标题栏设置生效前保留自绘窗口控件；chmod 八进制输入恢复为纯文本框；SFTP 多文件拖拽显示拖拽幻影；标签栏末尾落区指示不再把新增按钮挤到最右侧。
- SSH：私钥路径中的 `~` 在导入与连接时展开。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报。这是 Go/Wails 应用的已知问题（见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中添加排除规则放行。请仅从官方开源渠道 GitHub 与 Gitee 下载。如仍有顾虑，可下载源码自行构建运行。

感谢 @surenwuyuwuqiu、@zhangsir1211、@sonnartliao 在本版本的贡献。

## v1.9.4

### What's Changed

**New Features**
- Connections: favorites. Star any connection; a pinned Favorites group sits above the sidebar groups with drag-to-reorder, and the start page gains a favorites section. Synced across devices.
- SSH: Kerberos (GSSAPI) authentication, SSH agent authentication, and SSH agent forwarding. (@windtear)
- SCP: a new SCP protocol transfer type; SSH connections can set their file-transfer protocol to SFTP or SCP, compatible with hosts without an SFTP subsystem (embedded devices, minimal systems, etc.).
- SFTP overhaul:
  - File panes: back / forward / up navigation history, Explorer-style rubber-band selection (Ctrl-drag adds to the selection), a selection stats bar, a grouped flat toolbar, and copy-path context-menu actions.
  - Open with: a context-menu action that hands the file to the OS file-association picker; remote files reuse the external editor's temp copy and auto-upload watcher, so saves made in the picked app push back to the remote path.
  - Built-in editor: search/replace with match count, regex / case / whole-word toggles and `$N` group replacement, undo/redo, font size adjustment, and a toolbar (cut/copy/paste, wrap toggle).
  - Transfers: unified transfer tasks with retry (already-completed files are skipped), expandable per-file detail for directory transfers, dismiss, failed tasks marked on disconnect, auto-show on new tasks and a collapsible panel bar; SCP recursive transfers reach full parity.
  - Sidebar: a follow-terminal-path toggle — the file sidebar follows the active SSH/WSL terminal's cwd, reported via OSC-7.
  - Symbolic links: create links from the context menu, navigate directory links to their targets (SFTP / SCP / WSL).
- WSL (Windows): WSL file sidebar support — browse and manage distro files over the //wsl.localhost share; `/mnt/<drive>/...` paths map directly onto local Windows drives.
- Local shells (Windows): a "Command Prompt (Clink)" shell and administrator Command Prompt / PowerShell terminals launched through a UAC broker, embedded as ordinary tabs. (@kxn)
- Workspace: duplicate sessions within the current workspace and open SSH sessions into existing workspaces. (@windtear)
- Workspace: panels can be maximized to fill the workspace, with a configurable maximize shortcut. (@windtear)
- Shortcuts: numbered tab switching (Ctrl+1–9) and workspace panel switching (Alt+1–9), and a quick-commands shortcut. (@windtear, @kxn)
- Monitor: new Services, Hardware Devices and IPMI tabs; the system info tab gains a clock section showing host clock skew.
- Auto-update: in-app updates with GitHub/Gitee dual-source failover, streaming download progress, SHA256 verification and one-click install; the update source is selectable in Settings → About; release notes render as localized markdown. (@Sunshow)
- UI: a major display overhaul, especially on macOS — the entire interface now scales on a rem baseline, with a new "UI font size" setting that takes effect immediately (platform-aware defaults; the macOS baseline is raised to 14px), and the forced grayscale font smoothing is removed. This fixes text rendering too small and looking blurry on macOS.
- Zmodem: a default download directory setting, so received files land there without asking each time. (@windtear)

**Improvements**
- Connection form: RDP smart sizing defaults to off, the shell resets when switching local/WSL, credentials clear when switching auth type, identity auth is listed first, and validation errors surface instead of being swallowed.
- Connection form (SSH): a one-click "use default key" action for the key-path field — fills the first standard OpenSSH key that actually exists (`id_ed25519`/`id_rsa`/`id_ecdsa`/`id_dsa`), and repeated clicks cycle through the ones present. (@Sunshow)
- Terminal: the search bar follows the app theme colors.
- Terminal: input broadcast now works across all tabs and workspaces, not just within one workspace — right-click a terminal or workspace tab to add it as a broadcast target.
- Local shells (Windows): auto-detection of Cygwin, MSYS2 and Nushell.
- SSH keepalive for SFTP and SCP sessions, so idle transfer connections are no longer dropped by servers or NAT/firewall timeouts.
- Redis: keys grouped into a folder tree by a configurable separator, with a flat-list toggle. (@surenwuyuwuqiu)
- Database: "Run SQL File" against a database and "Copy Table" from the database tree context menus; script feedback shows failure details and affected rows. (@surenwuyuwuqiu)
- Shortcuts: the keyboard settings page is regrouped into titled sections, with a unified shortcut display.
- Shortcuts (macOS): shortcut hints now render with the UI font so modifier symbols keep their native shapes.
- Panel headers gain a right-click menu aligned with the tab menu (reconnect, copy address, AI lock, broadcast, close).
- AI: the syncable AI config is split into a dedicated `ai.json` (model catalog and agent turn limit); app personalization settings (theme, paths, shells, keybindings, etc.) are now device-local and no longer participate in cloud sync.
- Session logs: improved file naming, and charset escape sequences no longer pollute the log. (@windtear)
- UI: a connection locked by AI now tints the whole tab body and panel header with a warning colour, instead of a thin edge marker that competed with the selection ring. (@surenwuyuwuqiu)
- SFTP: the footer status bar is always visible (selection stats replace the entry count while a selection exists), select-all (Ctrl/Cmd+A) and Ctrl/Cmd+X/C/V clipboard shortcuts are added, and file-list column widths now follow the designed per-column sizes with single-line headers.
- Tunnels: starting a tunnel whose exit SSH connection has no saved credentials now prompts for them instead of failing the handshake.
- K8s: the inline kubeconfig field is now revealed on demand (like the private-key text field), and kubeconfig can be imported from file.
- SFTP: file entries are told apart at a glance by icon color — folders render as Finder-style light-blue filled glyphs, symlinks violet (name text stays neutral) — and non-name list columns (Type / Modified / Size / Permission / Owner / Group) can be hidden via a header right-click check menu, shared across panels and remembered across restarts. (@surenwuyuwuqiu)

**Bug Fixes**
- macOS IME: committed keystrokes are delivered directly to the terminal (no more dropped or duplicated characters under fast typing), duplicate input is prevented, and an IME-committed Enter no longer triggers app shortcuts (AI send, search jump, dialog confirm, ~25 handlers). (@surenwuyuwuqiu)
- Terminal: fixed copy-on-select writing to the clipboard when WKWebView lacks focus. (@surenwuyuwuqiu)
- Terminal: fixed terminal device attribute queries (DA) being swallowed, so remote TUI apps' capability probing now gets a response. (@windtear)
- Windows: rounded corners are restored on first launch, and the startup background matches the theme so it no longer flashes a black background.
- Local shells (Windows): opening a WSL terminal or browsing WSL files no longer flashes console windows; one-shot child process spawns go through a single hidden-window entry point.
- zsh now starts as a login shell so `.zprofile` is sourced before `.zshrc`. (@boltomli)
- Tunnel: start failures are surfaced with actionable hints, a config test and an SSH example; error toasts include the tunnel name and a localized prefix.
- SSH: the server's actual error is reported when keyboard-interactive auth is rejected cold. (@kxn)
- AI: markdown rendering escapes quotes and closes an XSS vector in slash-separated attributes. (@kxn)
- Shortcuts: rebinding no longer fires runtime handlers mid-capture; digit-shortcut conflicts between tabs and panels are resolved with fixed platform bindings. (@kxn)
- File transfer: fixed recursive directory retry, SCP fetch completion and the zmodem cancel path; file panels auto-reconnect when a refresh hits a dead session.
- File transfers: fixed transfer task lists breaking on terminal switches — background transfers now keep updating (no more frozen progress or un-cancellable tasks), and terminal tabs show a transfer indicator while their companion file panel has active transfers.
- Sync / update: automatic update check logic fixed; OSC-7 payloads terminate at the first terminator so crafted output can't pollute cwd reporting.
- Zmodem: the shell prompt returns on its own after an rz/sz transfer (no more pressing Enter), cancelling an in-flight sz download recovers cleanly instead of leaving the terminal swallowing all subsequent output, and stalled rz/sz transfers are prevented. (@windtear)
- macOS IME: fixed uppercase letters being dropped when the IME swallows the keypress (e.g. Doubao in English mode). (@surenwuyuwuqiu)
- AI: the model dropdown populated by "Fetch Models" now shows each model's display name but stores the actual model ID — previously the display name itself was saved as the model, so requests used the wrong model name. (@feuvan)
- UI: the "download to" overwrite/rename conflict prompt works again, and the AI sidebar search highlight no longer throws on every keystroke.
- Internal editor: saving files containing Chinese or other non-Latin1 characters no longer fails with a base64 encoding error; switching the encoding no longer discards unsaved edits — the encoding selector is now a two-level menu that separates "reopen with encoding" (re-decode from disk, with a confirmation when unsaved edits exist) from "save with encoding" (affects saving only).

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @windtear, @kxn, @surenwuyuwuqiu, @boltomli, @Sunshow, and @feuvan for their contributions to this release.

### 更新内容

**新功能**
- 连接：新增收藏功能。任意连接可加星标；侧边栏顶部出现可拖拽排序的「收藏」固定分组，起始页新增收藏区。收藏跨设备同步。
- SSH：新增 Kerberos（GSSAPI）认证、SSH agent 认证与 SSH agent 转发。（@windtear）
- SCP：新增 SCP 协议传输类型；SSH 连接可将文件传输协议配置为 sftp 或 scp，兼容无 SFTP 子系统的主机（嵌入式设备、精简系统等）。
- SFTP 全面改版：
  - 文件面板：新增 后退 / 前进 / 上级 导航历史、资源管理器式橡皮筋框选（Ctrl 拖拽为追加选择）、选中统计条、分组式扁平工具栏与复制路径右键菜单。
  - 打开方式：右键菜单项调用系统文件关联选择器打开文件；远程文件复用外部编辑器的临时副本与自动回传监听，在所选应用中保存后自动推回远端路径。
  - 内置编辑器：新增 查找/替换（匹配计数、正则 / 忽略大小写 / 全字匹配、`$N` 分组替换）、撤销/重做、字号调节与工具栏（剪切/复制/粘贴、换行开关）。
  - 传输：统一的传输任务支持失败重试（自动跳过已完成文件）、目录传输可展开查看每个文件明细、可移除、断线时任务标记失败、新任务自动展开、面板条可折叠；SCP 递归传输能力对齐 SFTP。
  - 文件边栏：新增「跟随终端路径」开关——跟随当前 SSH/WSL 终端的 cwd（通过 OSC-7 上报）。
  - 符号链接：右键菜单可创建链接，目录链接可点入并落到真实目标（SFTP / SCP / WSL）。
- WSL（Windows）：支持 WSL 文件边栏，通过 //wsl.localhost 共享浏览与管理发行版文件；`/mnt/<盘>/...` 路径直接映射到本地 Windows 盘符。
- 本地终端（Windows）：新增「命令提示符（Clink）」与管理员命令提示符 / PowerShell 终端，经 UAC broker 提权后以普通标签页嵌入。（@kxn）
- 工作区：支持在当前工作区内复制会话、将 SSH 会话打开到已有工作区。（@windtear）
- 工作区：支持面板最大化铺满工作区，最大化快捷键可配置。（@windtear）
- 快捷键：新增 数字键切换标签（Ctrl+1–9）与切换工作区面板（Alt+1–9）、快捷命令快捷键。（@windtear、@kxn）
- 监控：新增 服务、硬件设备、IPMI 三个标签页；系统信息新增时钟区，展示主机时钟与本机偏差。
- 自动更新：应用内更新，GitHub/Gitee 双源自动切换，流式下载进度、SHA256 校验与一键安装；设置 → 关于中可选择更新源；更新说明按语言渲染为 Markdown。（@Sunshow）
- 界面：界面显示整体优化（重点针对 macOS）——整个界面改为基于 rem 基准缩放，新增「界面字号」设置（即时生效，默认值随平台；macOS 默认基准字号提升为 14px），并移除强制灰度字体平滑，解决 macOS 下文本偏小、字体发虚的问题。
- Zmodem：新增默认下载目录设置，接收的文件直接保存到该目录，不再每次询问。（@windtear）

**改进**
- 连接表单：RDP 智能缩放默认关闭；切换 本地/WSL 时重置 Shell；切换认证方式时清除凭据；身份认证排到认证方式首位；表单校验错误如实提示不再吞掉。
- 连接表单（SSH）：密钥路径新增一键「使用默认密钥」——自动填入本机实际存在的标准 OpenSSH 密钥（`id_ed25519`/`id_rsa`/`id_ecdsa`/`id_dsa`），再次点击在存在的密钥间轮换。（@Sunshow）
- 终端：搜索栏颜色跟随应用主题。
- 终端：输入广播不再局限于单个工作区——右键任意终端或工作区标签即可加入广播目标，全应用内广播。
- 本地终端（Windows）：自动检测 Cygwin、MSYS2、Nushell。
- SFTP / SCP 会话增加 SSH 保活，空闲的传输连接不再被服务端或 NAT/防火墙超时断开。
- Redis：键名按可配置分隔符组装为文件夹树，可切换回平铺列表。（@surenwuyuwuqiu）
- 数据库：数据库树右键菜单新增「运行 SQL 文件」与「复制表」；脚本执行反馈显示失败详情与影响行数。（@surenwuyuwuqiu）
- 快捷键：键盘设置页重新分组，快捷键展示样式统一。
- 快捷键（macOS）：快捷键提示改用界面字体渲染，修饰键符号显示为系统原生字形。
- 面板标题栏新增右键菜单，与标签右键菜单保持一致（重连、复制地址、AI 锁定、广播、关闭）。
- AI：AI 配置拆分为独立的 `ai.json`（模型目录与 agent 轮次上限，跨设备同步）；应用个性化配置（主题、路径、Shell、快捷键等）改为仅本机生效，不再参与云同步。
- 会话日志：文件名更规范，不再混入字符集转义序列。（@windtear）
- 界面：AI 锁定的连接现在将整个标签与面板标题栏染上警示色背景，取代原先与选中高亮相互竞争的细边框标记。（@surenwuyuwuqiu）
- SFTP：底部状态栏常驻显示（有选中时改为显示选中统计与总大小）；新增全选（Ctrl/Cmd+A）与 Ctrl/Cmd+X/C/V 剪贴板快捷键；文件列表列宽按设计值生效，表头与排序箭头不再折行。
- 隧道：启动出口 SSH 连接未保存凭据的隧道时，改为提示输入凭据，而不是直接握手失败。
- K8s：内联 kubeconfig 改为按需展开显示（与私钥文本一致），并支持从文件导入。

**Bug 修复**
- macOS 输入法：提交后的按键直接送达终端（快速输入不再丢字/重复）、不再产生重复输入；输入法回车上屏不再触发应用快捷键（AI 发送、搜索跳转、对话框确认等约 25 处）。（@surenwuyuwuqiu）
- 终端：修复 WKWebView 失焦时选中文本即写入剪贴板的问题。（@surenwuyuwuqiu）
- 终端：修复终端设备属性查询（DA）应答被拦截的问题，远程 TUI 程序的能力探测现在能正常得到回应。（@windtear）
- Windows：首次启动恢复 Win11 圆角；启动背景与主题一致，不再闪黑色背景。
- zsh 改为以登录 shell 启动，`.zprofile` 在 `.zshrc` 之前生效。（@boltomli）
- 隧道：启动失败如实提示，附配置测试与 SSH 示例；错误提示包含隧道名与本地化前缀。
- SSH：keyboard-interactive 认证被服务端直接拒绝时，如实上报服务端错误。（@kxn）
- AI：Markdown 渲染转义引号，修复斜杠分隔属性处的 XSS 注入。（@kxn）
- 快捷键：录制改绑时不再误触发运行时快捷键；数字键在标签/面板间的冲突以固定平台绑定方式解决。（@kxn）
- 文件传输：修复递归目录重试、SCP 拉取完成判断与 zmodem 取消路径；刷新遇到已断开会话时自动重连。
- 文件传输：修复切换终端后传输任务列表失效的问题——后台传输继续更新（进度不再冻结、任务不再无法取消）；终端标签页在其文件面板有进行中的传输时显示传输指示。
- Zmodem：rz/sz 传输完成后 shell 提示符自动恢复，不再需要手动按回车；取消进行中的 sz 下载后终端干净恢复，不再吞掉后续所有输出；并防止 rz/sz 传输停滞。（@windtear）
- macOS 输入法：修复输入法吞掉按键时大写字母丢失的问题（如豆包英文模式）。（@surenwuyuwuqiu）
- AI：「拉取模型列表」后的模型下拉框改为显示模型展示名、保存真实模型 ID——此前会把展示名当作模型值保存，导致请求时模型名不正确。（@feuvan）
- 界面：恢复「下载到」覆盖/重命名冲突提示；修复 AI 边栏搜索高亮每次按键报错的问题。
- 内置编辑器：修复保存含中文等非 Latin1 字符的文件时报 base64 编码错误的问题；切换编码不再丢失未保存的修改——编码选择改为两级菜单，拆分「以此编码重新打开」（重读磁盘文件，有未保存修改时先确认）与「以此编码保存」（仅影响保存编码）。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @windtear、@kxn、@surenwuyuwuqiu、@boltomli、@Sunshow 和 @feuvan 对本版本的贡献。

## v1.9.2

### What's Changed

**New Features**
- Terminal: screen preview on scrollbar hover — hovering the scrollbar pops up a preview of the history output at that position. On by default; always enabled, display-only.
- Sidebar: the tunnels tab is back. Right-click the tab strip to toggle tab visibility; also configurable in Settings. Visible by default.
- AI: session management — sessions in the history dropdown can be renamed via a button; right-click a message bubble to export the conversation as Markdown, delete that message only, or delete it and everything after it (context rollback). (@surenwuyuwuqiu)
- Import: one-click import of connections from DBeaver workspaces and Navicat `.ncx` exports, with password decryption and folder / group restore. (@surenwuyuwuqiu)
- Database: export query results as CSV / TXT / JSON; drag to reorder doc tabs; new data-only table export option (besides structure-only and structure+data). (@surenwuyuwuqiu)
- AI models & SSH: new "system proxy" option that resolves the OS system proxy (registry / system config, PAC scripts, env fallback) against the actual target — for AI model requests (model list, connection test, chat) and SSH-family connections; the model form gains a quick-create "+" button for proxies.
- RDP: admin (console) session mode (mstsc /admin equivalent), editable domain field, custom desktop resolution, and new connections default to fullscreen.

**Improvements**
- User-facing backend errors (private key validation, SSH key auth, SFTP, sync) are now localized on the frontend instead of showing hardcoded Chinese text.
- Terminal: gutter line numbers / timestamps are now fixed logical-line numbers — wrapped rows don't consume numbers and they survive resizing; right-click the gutter to toggle numbers and timestamps.
- Upload: dragging OS files onto an SSH terminal (rz) or the SFTP sidebar uploads directly from the source file via native paths — no size cap, no webview memory overhead.
- File sidebar: clipboard support (copy / cut / paste with conflict handling) and a reworked toolbar with a "more" menu (hidden files, new file / directory).
- Header main menu: added import / export entries. (@surenwuyuwuqiu)
- Quick commands: collapsed groups are remembered across restarts.
- Editors: Mongo / ES / K8s / kubeconfig code & YAML editors unified on a shared CodeMirror component that follows the app theme.
- Proxy: per-proxy on/off toggle — a disabled proxy is skipped and connections referencing it dial directly instead of failing. (@surenwuyuwuqiu)
- Database: MongoDB / Elasticsearch views unified with the database style — row insert/edit dialogs redesigned as aligned tables, row actions moved to the result toolbar and footer bar, Elasticsearch gains multi-tab with per-index views and a cluster tab, wheel scrolling and an overflow menu on doc tab bars.

**Bug Fixes**
- Terminal: restored the Backspace default to 0x7F (DEL), fixing whole-word deletion in Windows 11 ConPTY PowerShell/CMD and `bash read` erasing nothing over SSH. Existing connections that already saved a backspace option need to be manually switched back to DEL in the connection settings.
- SFTP: the external editor menu in the file sidebar now works, with auto-upload on save; on Windows, editors no longer flash a console window and VS Code wait-for-close works.
- SFTP: the filename tooltip now shows only when the name is truncated.
- Identities: identities imported via key text now show the correct auth type label.
- AI: command output is now captured from the terminal screen buffer instead of the raw PTY stream, fixing duplicated/shifted fragments from ConPTY redraws.
- AI: the tool timeout badge now always shows the effective wait, and model-provided timeouts are honored as-is instead of being clamped.
- Terminal: text highlighting is now optimized based on MobaXterm's color scheme — improved IP coloring, curated keyword colors for errors / success / warnings / info; arbitrary numbers are no longer highlighted.
- Sync: the remote is fetched before committing, so two machines used alternately no longer hit the conflict dialog on every open; resolving a conflict no longer force-pushes, which previously re-broke the other machine on its next open.
- Sync: fixed synced-file-list drift — tunnel changes are now actually committed to the sync repo; AI sessions and skills stay local-only.
- Database: connection tests and reconnect for Redis / MongoDB / Elasticsearch now route to their dedicated probes instead of failing.
- SQL Server: named-instance hosts (server\instance) now connect; the port is resolved via SQL Browser when omitted.
- Connection test now goes through the configured jump-host tunnel for all connection types, not just k8s / container.
- AI: LLM requests route through the OS system proxy (with env fallback), so model list / test / chat work behind system-level proxies.
- SFTP: external editor lifecycle rework — reopening the same file keeps a stable temp path, file watchers no longer stack, auto-upload survives editor open/close; 25 s timeouts on content operations; fixed folder downloads silently transferring nothing.
- Zmodem: cancelling the file picker dialog now aborts cleanly instead of leaving the terminal stuck.
- Terminal: the screen preview popup now aligns with the scrollbar click position, follows the terminal theme, and reads as a distinct overlay with a pale-blue tinted background and no side borders; fixed the blank area below the terminal after maximize / restore; tooltips no longer block clicks or linger under the cursor.
- Settings: deleting an AI model now asks for confirmation; the "Second Font" option is renamed to "Fallback Font"; other copy fixes.
- macOS: the real app icon now shows in dev builds and packaged .app bundles. (@surenwuyuwuqiu)

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu for their contribution to this release.

### 更新内容

**新功能**
- 终端：滚动条悬停预览——悬停滚动条即弹出该位置历史输出的预览。默认开启，始终启用，仅供预览。
- 侧边栏：隧道标签页回归；右键标签栏可开关显示各标签页，设置中也可配置。默认显示。
- AI：会话管理——历史下拉列表中的会话支持按钮重命名；右键消息气泡可导出会话为 Markdown、仅删除该条消息，或删除它及其后所有消息（上下文回滚）。（@surenwuyuwuqiu）
- 导入：支持从 DBeaver 工作区和 Navicat `.ncx` 导出文件一键导入连接，含密码解密与文件夹 / 分组还原。（@surenwuyuwuqiu）
- 数据库：查询结果可导出为 CSV / TXT / JSON；文档标签页支持拖拽排序；表导出新增「仅数据」选项（原有「仅结构」「结构+数据」不变）。（@surenwuyuwuqiu）
- AI 模型与 SSH：新增「系统代理」选项——按实际目标解析操作系统系统代理（注册表 / 系统配置、PAC 脚本、环境变量兜底），适用于 AI 模型请求（模型列表、连接测试、对话）和 SSH 系连接；模型表单新增代理快速创建「+」按钮。
- RDP：新增管理员（console）会话模式（等价 mstsc /admin）、可编辑的域名字段、自定义桌面分辨率；新建 RDP 连接默认全屏。

**改进**
- 后端返回的用户可见报错（私钥校验、SSH 密钥认证、SFTP、云同步等）改为前端 i18n 本地化，不再显示写死的中文。
- 终端：行号 / 时间戳改为固定逻辑行号——折行不再占用行号，窗口缩放后保持不变；右键行号栏可开关行号与时间戳。
- 上传：拖拽系统文件到 SSH 终端（rz）或 SFTP 边栏，改为通过原生路径直接从源文件上传——无大小限制，不再占用 WebView 内存。
- 文件边栏：支持剪贴板（复制 / 剪切 / 粘贴，含冲突处理与自动重命名）；工具栏重构，新增「更多」菜单（显示隐藏文件、新建文件 / 目录）。
- 顶部主菜单：新增导入 / 导出入口。（@surenwuyuwuqiu）
- 快捷命令：分组折叠状态跨重启记忆。
- 编辑器：Mongo / ES / K8s / kubeconfig 的代码与 YAML 编辑器统一为共享 CodeMirror 组件，并跟随应用主题。
- 代理：每个代理支持启用/禁用开关——禁用的代理会被跳过，引用它的连接改为直连而不再报错。（@surenwuyuwuqiu）
- 数据库：MongoDB / Elasticsearch 界面统一为数据库风格——行插入/编辑弹窗重排为对齐表格，行操作移至结果工具栏与底部栏，Elasticsearch 新增多标签（每索引视图 + 集群标签），文档标签栏支持滚轮滚动与溢出菜单。

**Bug 修复**
- 终端：退格键默认值恢复为 0x7F（DEL），修复 Windows 11 ConPTY 下 PowerShell/CMD 退格删整词、SSH `bash read` 无法删除字符的问题。存量连接若已保存退格键设置，需手动在连接配置中改回 DEL。
- SFTP：文件边栏的「外部编辑器」菜单现已生效，保存后自动回传；Windows 上启动编辑器不再闪现控制台窗口，VS Code 等待关闭也恢复正常。
- SFTP：文件名提示框仅在名称被截断时显示。
- 身份（密钥库）：私钥文本导入的身份现在显示正确的认证类型标签。
- AI：命令输出改为从终端屏幕缓冲区捕获，修复 ConPTY 重绘导致的重复/错位片段。
- AI：工具超时徽标现在始终显示实际等待时间，模型提供的超时值按原值生效、不再被钳制。
- 终端：文本高亮基于 MobaXterm 配色方案进行优化——优化 IP 配色、错误/成功/警告/提示关键词配色；不再高亮任意数字。
- 云同步：改为先拉取远端再提交，两台机器交替使用不再每次打开都弹冲突对话框；解决冲突不再 force-push（此前会导致另一台机器下次打开再次冲突）。
- 云同步：修复同步文件清单不一致——隧道变更现在会真正提交到同步仓库；AI 会话与技能保持仅本地。
- 数据库：Redis / MongoDB / Elasticsearch 的连接测试与重连改为路由到各自的探测逻辑，不再报「不支持」。
- SQL Server：支持 `server\instance` 命名实例主机；未填端口时通过 SQL Browser 解析。
- 连接测试现在对所有连接类型都走配置的跳板机隧道（此前仅 k8s / 容器生效）。
- AI：LLM 请求走操作系统系统代理（环境变量兜底），系统级代理环境下模型列表 / 测试 / 对话不再失败。
- SFTP：重构外部编辑器生命周期——同一文件重复打开使用固定临时路径、文件监听不再叠加、自动上传在编辑器反复开关后依然有效；内容操作增加 25 秒超时；修复目录下载静默传输 0 个文件的问题。
- Zmodem：取消文件选择对话框现在会干净地中止，不再导致终端卡死。
- 终端：屏幕预览弹窗与滚动条点击位置对齐，颜色跟随终端主题，并以淡蓝色调背景、去除左右边框使其视觉上更独立；修复窗口最大化 / 还原后终端下方空白；tooltip 不再阻挡点击、不再滞留。
- 设置：删除 AI 模型增加确认；「第二字体」改名「回退字体」；其他文案修正。
- macOS：开发模式与打包 .app 显示真实应用图标。（@surenwuyuwuqiu）

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.9.1

### What's Changed

**New Features**
- Database: Elasticsearch support — browse and manage clusters, indexes and documents. (@iCarrear)
- File sidebar: a companion file-management panel for browsing and managing files, with drag-and-drop upload. (@iCarrear)
- Monitor sidebar: shows the current host's CPU, memory, and network monitoring info right in the sidebar. (@iCarrear)
- Database: query execution history — a panel that records executed SQL and re-runs it with one click. (@iCarrear)
- Database: inline cell editing with batch apply. (@iCarrear)
- Container: added the WSLC (Windows WSL2 Container) runtime. (@boltomli)
- SFTP: open and edit files in your own external editor with auto-upload on save; auto-detects a dozen-plus text editors (VSCode, Sublime Text, etc.) and supports custom launch commands.
- Terminal: raw TCP console session — connect a plain socket to host:port and send raw bytes, with encoding, newline mode, local echo, and backspace options.

**Improvements**
- SFTP: quick-locate files by typing the first letter of a name (case-insensitive, folders listed first; press again to cycle), in both SFTP panes and the file sidebar.
- SFTP: the file list now shows type / permission / owner / group columns.
- SFTP: file encoding auto-detect now strictly decodes the whole file, so valid UTF-8 is no longer misjudged as GBK when a multi-byte character crosses the sample boundary.
- SFTP: progressive loading for large directories to prevent freezing / crashing on thousands of entries.
- SFTP / Monitor: encrypted private keys now work for authentication.
- RDP: native window follows moves, clips to the client area, and no longer steals the foreground; menus / dialogs show a snapshot instead of a black area. (@boltomli)
- Updated to Wails v3 (frontend on Vite 8).
- Themes: added built-in Xshell, MobaXterm and FinalShell color schemes, now folded into the standard Dark / Light groups. (@iCarrear)
- Popup / context menus are no longer clipped at the window edge — submenus open in the correct direction and menus stay within the viewport.
- New connection: database category split into SQL / NoSQL filter groups (with an Elasticsearch entry).
- Config export: exporting connection configurations now always requires an export password.
- Tunnel management moved into the Settings page.
- Monitor: collapsible per-core / per-NIC / per-disk detail lists and a process kill button in the monitor tab; disk and network statistics are now more accurate (works on hosts without `ip` / `lsblk -j`, e.g. CentOS 7).
- Terminal: configurable middle-click action, Ctrl/Cmd+right-click always opens the context menu, and font zoom via shortcuts or Ctrl/Cmd+wheel.
- Quick commands: remark field and a larger edit dialog.
- Connection: "Connect Only" opens a session without saving it; new-connection forms prefill from the search box.
- SSH: new "keyText" inline private-key auth mode — paste or import the PEM private-key text directly (beside password and key-file), encrypted at rest and portable across machines.

**Bug Fixes**
- VNC / SPICE: fixed a race that could break session start. (@boltomli)
- macOS: fixed the title-bar layout. (@surenwuyuwuqiu)
- Settings: fixed the cursor-blink toggle showing the "text highlight" title and description, which duplicated the text-highlight entry. (@wangxufeng)
- Linux: desktop icon added for the deb / rpm packages.
- Terminal: suggestion popup no longer lingers when clicking outside.
- Terminal: pasting content now auto-scrolls to the newest output (the bottom).
- Terminal: highlighting now covers indented lines and paths with special characters.
- Terminal: after resizing the window, the terminal now refills the new size instead of leaving a large blank area.
- K8s: namespaces stay usable when list permission is restricted.
- History: wrapped command lines are joined so extraction isn't truncated.
- Connection: "Save & Connect" persists edits; test-connection error toast auto-dismisses.
- macOS: fixed a startup panic that crashed the app on launch, so uniTerm now starts normally again. (@surenwuyuwuqiu)
- Tunnel: jump-host credentials for K8s / container connections are now resolved by auth type (identity from the vault); inline credentials never override the resolved value.
- Windows installer: "Run" on the finish page now launches uniTerm unelevated so the first-run config isn't written into a then-unwritable Program Files dir.
- Sync: switching a connection to identity now clears the stale saved user/password, so a leftover enc:v1: field isn't propagated into cloud sync.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @iCarrear, @boltomli, @surenwuyuwuqiu, and @wangxufeng for their contributions to this release.

### 更新内容

**新功能**
- 数据库：新增 Elasticsearch 支持，可浏览与管理集群、索引与文档。（@iCarrear）
- 文件边栏：新增文件管理边栏，可浏览与管理文件，支持拖拽上传。（@iCarrear）
- 监控边栏：在边栏中展示当前主机的 CPU、内存、网络等监控信息。（@iCarrear）
- 数据库：新增查询执行历史面板，记录已执行 SQL，可一键重跑。（@iCarrear）
- 数据库：结果表格支持行内编辑与批量应用。（@iCarrear）
- 容器：容器管理新增 WSLC（Windows WSL2 Container）运行时支持。（@boltomli）
- SFTP：支持外部编辑器打开编辑文件并自动回传，支持自动检测 VSCode、Sublime Text 等 10 余种文本编辑器及自定义启动命令。
- 终端：新增原始 TCP 控制台会话，连接 `host:port` 后直接收发原始字节流，支持编码、换行模式、本地回显与退格键设置。

**改进**
- SFTP：支持按名称首字母快速定位文件（不区分大小写、目录优先，再按同一字母循环查找），SFTP 双栏与文件边栏均可用。
- SFTP：文件列表新增 类型 / 权限 / 属主 / 属组 等列。
- SFTP：编码自动检测改为对整份文件做严格 UTF-8 解码，避免多字节字符跨过采样边界时把合法 UTF-8 误判为 GBK 而显示乱码。
- SFTP：大目录采用渐进式加载，避免上千条目时卡死或崩溃。
- SFTP / 监控：加密私钥现在可用于认证。
- RDP：原生窗口随窗口移动、裁切在客户区内且不再抢占前台；菜单 / 弹窗打开时显示快照而非黑块。（@boltomli）
- 界面框架升级至 Wails v3（前端基于 Vite 8）。
- 主题：新增 Xshell、MobaXterm、FinalShell 内置配色方案，并入标准的 深色 / 浅色 分组。（@iCarrear）
- 修复弹出 / 右键菜单被窗口边缘遮挡的问题：子菜单朝正确方向弹出，菜单始终显示在视口内。
- 新建连接：数据库分类细分为 SQL / NoSQL 筛选分组（新增 Elasticsearch 入口）。
- 配置导出：导出连接配置时始终要求设置导出密码。
- 隧道管理移动到设置页中。
- 监控：监控 Tab 支持可折叠的 按核心 / 网卡 / 磁盘 明细列表与进程结束按钮；磁盘与网络统计更准确（支持无 `ip` / `lsblk -j` 的主机，如 CentOS 7）。
- 终端：中键动作可配置，Ctrl/Cmd + 右键始终打开右键菜单，并支持快捷键或 Ctrl/Cmd + 滚轮缩放字体。
- 快捷命令：新增备注字段，并放大编辑弹窗。
- 连接：新增「仅连接」操作，打开会话而不保存连接；新建连接表单支持从搜索框预填。
- SSH：新增「KeyText 内联私钥」认证模式——可直接粘贴或从文件导入 PEM 私钥文本（第三种认证方式），加密存储、跨机器可用，不受密钥文件路径影响。

**Bug 修复**
- VNC / SPICE：修复可能导致会话无法启动的竞态问题。（@boltomli）
- macOS：修复标题栏布局。（@surenwuyuwuqiu）
- 设置：修复「光标闪烁」开关误用「文本高亮」标题与描述、导致与高亮条目重复显示的问题。（@wangxufeng）
- Linux：修复 deb / rpm 缺少桌面图标的问题。
- 终端：修复补全建议弹窗点击外部后不消失的问题。
- 终端：粘贴内容后自动滚动到最新输出（底部）。
- 终端：文本高亮现在覆盖缩进行与含特殊字符的路径。
- 终端：调整窗口大小后，终端画面会自动重新铺满，不再留下大片空白。
- K8s：列表权限受限时命名空间仍可正常访问。
- 历史：换行的命令会拼回一行，命令提取不再截断。
- 连接：「保存并连接」会持久化编辑；测试连接错误提示自动关闭。
- macOS：修复启动即崩溃的启动 panic，应用在 macOS 上恢复正常启动。（@surenwuyuwuqiu）
- 隧道：K8s / 容器连接的跳板机凭据按认证方式解析（身份来自密钥库），内联凭据永不覆盖已解析值。
- Windows 安装包：完成页「运行」改为非提权启动，避免首次配置写入日后不可写的 Program Files。
- 同步：连接切到身份认证时清掉残留的用户名/密码，避免旧 enc:v1: 密文被云同步散布到其他设备。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @iCarrear、@boltomli、@surenwuyuwuqiu 和 @wangxufeng 对本版本的贡献。

## v1.8.0

### What's Changed

**New Features**
- SSH: vault (Identities). Save reusable SSH keys and passphrase-protected private keys, then reference them from SSH-family connections, with a keychain management page in Settings.
- SSH: SOCKS5 / HTTP proxy support, with a proxy list management page in Settings.
- Configuration: encrypted import / export of connection configurations, via the new-connection menu.
- Configuration: import connection configurations from third-party terminal software via the new-connection menu, including Xshell, MobaXterm, WindTerm, SecureCRT, and OpenSSH config.
- Configuration: configurable data directory and dual encryption modes.
- Credential: master-password encryption mode alongside the system keychain mode, for using the master password to recover saved passwords after switching machines or reinstalling the OS.
- Terminal: line-number and timestamp gutters, toggled via right-click or keyboard shortcuts.
- Database: multi-tab panels, open several database sessions at once and switch quickly between different databases / schemas within the same connection. (@surenwuyuwuqiu)
- Database: table-data pagination, browse large tables page by page instead of loading all rows at once. (@surenwuyuwuqiu)
- Database: import table data by running `.sql` scripts for MySQL / PostgreSQL / Oracle / SQL Server / RQLite. (@surenwuyuwuqiu)
- Database: export table data as `.sql` files for MySQL / PostgreSQL / Oracle / SQL Server / RQLite. (@surenwuyuwuqiu)
- Connection: "Test Connection" button in the new-connection dialog, to check connection reachability before saving.
- Connection: open-status indicator with locate-session.
- Connection: locate-to-connection items in tab/panel context menus.
- Tab: "Reconnect" item in the tab right-click menu, force-disconnects the current session and re-establishes the connection.

**Improvements**
- SSH: hardened credential handling with legacy fallback, auto-answer for keyboard-interactive auth using the saved password, and synchronous keychain loading so saved credentials are never lost. (@jiayunora)
- Terminal: JetBrains Mono Variable is now the default monospace font for terminal sessions.
- VNC: connect to RealVNC Server. Note: RealVNC Server must have its "Prefer on" option enabled and the toggle that allows legacy VNC software to connect turned on.
- VNC: fix blurry scaled rendering.
- SPICE: fix blurry scaled rendering.
- Terminal: configurable cursor style, with block / underline / bar selections.
- Terminal: configurable minimum text contrast, raised by default so text stays readable when foreground and background colors are close.
- Terminal: font options — a second fallback font, a monospace-only filter, and adjustable font weight, supporting independent configuration of East Asian fonts (e.g. Chinese).
- Settings: gear replaced with a dropdown menu for quick access to entries like the keychain and proxies.
- UI: context menus and header tooltips now show the configured keyboard shortcuts.
- Settings: new terminal "follow app theme" option, now the default theme setting.
- Tab: rename menu, repositioned close button, and copy-host-address item.
- AI: Windows OpenSSH detection so shell guidance and command execution adapt accordingly.
- SFTP: local download conflict handling — choose overwrite / rename / cancel.
- Shortcuts: Ctrl+Shift+C / Ctrl+Shift+V are now configurable terminal-scoped shortcuts.
- Connection: two-level type filter in the sidebar and start page; container connections are now distinguished by runtime (Docker / Podman / nerdctl) in the badge, card subtitle, and filter.

**Bug Fixes**
- SSH: fixed handshake EOF on older servers by negotiating CTR ciphers before GCM. (@jiayunora)
- Connection: fixed stale/leftover data leaking into the connection form when editing.
- Compatibility: fixed a lookbehind regex that black-screened WKWebView on macOS ≤ 12.3.
- Input: fixed numeric fields appending pasted numbers instead of overwriting them.
- Terminal: fixed macOS issues where pressing modifier keys (e.g. Ctrl) yanked the viewport to the bottom and Shift+Delete stopped working. (@surenwuyuwuqiu)
- Window: fixed the "system title bar" setting not actually relaunching the app. (@surenwuyuwuqiu)
- Sync: fixed stores not reloading after a startup auto-sync pull.
- SFTP: fixed connections failing on servers whose login shell prints to the stream in non-interactive sessions (noisy profile/bashrc output) — the client now falls back to exec'ing sftp-server with a marker and skips everything before it, surfacing an actionable hint if the fallback also fails. (@surenwuyuwuqiu)
- Session: fixed duplicated-session tabs appearing late; the duplicate entries (context menu and keyboard shortcut) now share one routine and the new tab opens immediately.
- Settings: fixed the settings page leaving large empty side columns on a maximized window; the content area is now sized and centered independently. Raised the window minimum size (400→600 wide, 300→450 tall).
- Highlight: fixed text highlighting overwriting already-colored spans (e.g. directory colors) — those spans are now skipped.
- Menu: fixed terminal and sidebar right-click menus clipping at the viewport bottom; they now measure real size and clamp to the window.
- Window: fixed a relaunched window (system-title-bar toggle) landing in the background on Windows, and fixed tab close-button jitter in right-side mode by reserving the button's width.
- Sync: fixed cloud sync dropping tunnel definitions; tunnels.json is now included in the synced config file set.
- SFTP: fixed drag upload only handling single files; folder and multi-selection drag upload are now supported.
- Broadcast: fixed pasted input (shortcut, middle-click, Cmd+V) not reaching synced panels unlike typed input; every paste path now broadcasts like keyboard input.
- Zmodem: fixed sz downloads stopping mid-file with "Zmodem transfer cancelled" — completion now waits until every write reaches disk, and received bytes are batched and flushed in 64KB blocks.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu and @jiayunora for their contributions to this release.

### 更新内容

**新功能**
- SSH：密钥库（身份钥匙）。可保存可复用的 SSH 密钥及带口令保护的私钥，供 SSH 系列连接引用；设置中新增密钥库管理页面。
- SSH：连接支持 SOCKS5 / HTTP 代理，设置中新增代理列表管理页面。
- 配置：新增加密导入导出连接配置功能，入口位于新建连接菜单。
- 配置：新增第三方终端软件连接配置导入功能，入口位于新建连接菜单，包括 Xshell、MobaXterm、WindTerm、SecureCRT、OpenSSH 配置。
- 配置：支持迁移到程序目录或自定义目录，满足便携需求。
- 凭据：在系统钥匙库加密模式基础上，新增主密码加密模式，便于更换机器或重装系统后使用主密码恢复记录的密码。
- 终端：新增行号与时间戳侧栏，支持右键开关和快捷键开关。
- 数据库：多标签面板，可同时打开多个数据库会话，在同一连接的不同库 / 表之间快速切换。（@surenwuyuwuqiu）
- 数据库：表数据分页，浏览大表时分页加载与查看，避免一次性拉取全部数据。（@surenwuyuwuqiu）
- 数据库：支持运行 `.sql` 脚本导入表数据（适配 MySQL / PostgreSQL / Oracle / SQL Server / RQLite）。（@surenwuyuwuqiu）
- 数据库：支持导出表数据为 `.sql` 文件（适配 MySQL / PostgreSQL / Oracle / SQL Server / RQLite）。（@surenwuyuwuqiu）
- 连接：新建连接对话框新增「测试连接」按钮，支持在创建时提前检查连接可达性。
- 连接：新增打开会话状态展示与定位会话功能。
- 连接：标签/面板右键菜单新增「定位到连接」。
- 连接：标签右键菜单新增「重新连接」选项，强制断开当前会话并重新建立连接。

**改进**
- SSH：加固凭据处理并加入旧版回退，自动用已保存密码应答 keyboard-interactive 认证。（@jiayunora）
- 终端：JetBrains Mono Variable 字体作为默认等宽字体。
- VNC：支持连接 RealVNC Server。注意：RealVNC Server 需开启「Prefer on」，并开启允许旧版（legacy）VNC 软件连接的开关。
- VNC：修复缩放后渲染模糊的问题。
- SPICE：修复缩放后渲染模糊的问题。
- 终端：新增光标样式配置，支持方块、下划线、竖线样式选择。
- 终端：新增高对比度文本配置，默认调高对比度，解决前景背景色相近时难以识别的问题。
- 终端：新增字体选项：第二字体、仅等宽字体过滤、可调字体粗细，支持中文等东亚字体单独配置需求。
- 设置：入口由齿轮图标改为下拉菜单，便于钥匙库、代理等入口快速进入。
- 界面：右键菜单与标题栏 tooltip 现在展示已配置的快捷键。
- 设置：终端新增「跟随应用主题」选项，作为默认主题设置。
- 标签：重命名菜单、关闭按钮位置调整、复制主机地址。
- AI：检测到 Windows OpenSSH 时，shell 指引与命令执行会自动适配。
- SFTP：本地下载冲突支持选择覆盖 / 重命名 / 取消。
- 快捷键：Ctrl+Shift+C / Ctrl+Shift+V 改为可配置的终端作用域快捷键。
- 连接：侧边栏与起始页新增两级类型筛选（分类 → 具体类型）；容器连接在徽标、卡片副标题与筛选中按运行时区分（Docker / Podman / nerdctl）。

**Bug 修复**
- SSH：修复旧服务器握手 EOF 问题：先协商 CTR 加密算法再协商 GCM。（@jiayunora）
- 连接：修复编辑连接时表单残留上一次编辑数据的问题。
- 兼容性：修复文本高亮中一处 lookbehind 正则导致 macOS ≤ 12.3 WKWebView 黑屏的问题。
- 输入框：修复数字输入框粘贴时数值被追加而不是覆盖替换的问题。
- 终端：修复 macOS 上按修饰键（如 Ctrl）时视口跳到底部、以及 Shift+Delete 删除键失效的问题。（@surenwuyuwuqiu）
- 窗口：修复「系统标题栏」设置切换后未真正重启应用的问题。（@surenwuyuwuqiu）
- 同步：修复启动时自动同步拉取后 store 未重载的问题。
- SFTP：修复服务端非交互会话中登录脚本输出（如 profile/bashrc 打印）污染协议流导致连接失败的问题：现改为带标记 fallback exec sftp-server，并跳过标记之前的输出；若 fallback 也失败则给出可操作的提示。（@surenwuyuwuqiu）
- 会话：修复右键菜单 / 快捷键复制会话时新标签显示延迟的问题；复制入口统一为同一套逻辑，新标签立即打开。
- 设置：修复窗口最大化时设置页左右空白过大的问题：内容区改为独立缩放并居中。同时提高窗口最小尺寸（宽 400→600，高 300→450）。
- 高亮：修复文本高亮覆盖已着色 span（如目录颜色）的问题，现跳过已着色 span。
- 菜单：修复终端与侧边栏右键菜单在视口底部被裁剪的问题，现会测量实际尺寸并限制在窗口内。
- 同步：修复云同步丢失隧道配置的问题；tunnels.json 现纳入同步配置文件集合。
- SFTP：修复拖拽上传仅支持单个文件的问题；现支持文件夹与多选拖拽上传。
- 广播：修复粘贴（快捷键 / 中键 / Cmd+V）与键盘输入不同步、无法到达已同步面板的问题；现在所有粘贴路径与键盘输入一样广播。
- Zmodem：修复 sz 下载中途停止并提示 "Zmodem transfer cancelled" 的问题；下载完成现在会等待所有写入落盘，接收字节按 64KB 块批量写入。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @surenwuyuwuqiu 和 @jiayunora 对本版本的贡献。

## v1.7.0

### What's Changed

**New Features**
- X11 forwarding for SSH connections. Run Linux GUI applications remotely and display them on your local machine. Windows builds bundle VcXsrv X Server with no extra components needed; macOS requires XQuartz (`brew install --cask xquartz`). Enable the toggle in the SSH connection form's advanced settings.
- X11 desktop support. Launch full Linux desktop environments (GNOME, KDE, XFCE, MATE, Cinnamon, Openbox) over SSH with X11 forwarding. A new `x11-desktop` session type with standalone SSH credentials and auto-launched X Server.
- VNC Require TLS toggle, shared session mode, and VNC Repeater ID support.
- S3 connections now support virtual-hosted–style endpoints (e.g. Alibaba Cloud OSS, Tencent COS, Huawei OBS).
- Telnet negotiation mode, local echo, send mode (character/line), newline mode (CR/CRLF), and character encoding options for compatibility with diverse telnet servers.
- Serial port connections now use the unified connection form (same as all other protocols) instead of a standalone dialog, with new support for character encoding, local echo, and newline mode.
- Per-connection backspace key option for SSH, Telnet, Serial, and Mosh sessions. Choose between ASCII Delete (DEL, 0x7F), ASCII Backspace (BS, 0x08), or VT220 Delete. Default changed from DEL to Backspace (^H) for out-of-the-box compatibility with Huawei, H3C, Cisco network gear and serial consoles.

**Improvements**
- xterm.js upgraded to 6.0.0 with all addons (search, fit, ligatures, Unicode 11, clipboard). (@coderstory)
- JetBrains Mono Variable font bundled as the default monospace font, replacing the previous system font stack. (@coderstory)
- Terminal text highlighting no longer highlights inside code fences or full code blocks, avoiding false positives in AI output and markdown. (@coderstory)
- Extensive performance and stability hardening across all subsystems: store (atomic writes, debounced I/O, sharded sessions), session (larger read buffers, event-driven flush loops, lock contention reduction), frontend (rAF-coalesced rendering, memoized computations, ring buffer for session data), K8s (HTTP transport tuning, cached parsing, watch/log reconnect with backoff), AI/LLM (SSE streaming buffers, shared HTTP client, typed event payloads), sync (async init, ETag conditional GETs, AES-GCM AAD binding), and database (connection pool tuning, parallel schema loading, identifier escaping). (@coderstory)
- Configurable terminal double-click word separator. Choose which characters act as word boundaries for double-click selection in terminal settings. (@wangxufeng)
- Refined the built-in uniTerm Dark and uniTerm Light terminal theme colors.
- macOS DMG now split into separate amd64 and arm64 packages instead of a universal binary, avoiding compatibility warnings on macOS 26+.
- Connection remark field for adding free-form notes to any connection.
- Redesigned database and MongoDB natural-language query layout: the NL input now sits to the left of the SQL/filter editor at matching height, with the generate button styled like the Execute button for better discoverability.

**Bug Fixes**
- Fixed cursor blink setting not persisting across restarts. (@wangxufeng)
- Fixed Claude Code and other TUI glyphs not rendering after terminal history restore. (@coderstory)
- Fixed AI CancelChatStream race condition that could leave stale cancellations across overlapping requests. (@coderstory)
- Fixed sync security: enforced file whitelist, improved password mismatch handling, removed hostname leak from config. (@coderstory)
- Fixed K8s auth not retrying on 401, watch/log streams not reconnecting after transient failures. (@coderstory)
- Fixed database connection pool race, query timeout handling, and SQL Server resource cleanup. (@coderstory)
- Fixed monitor trend chart colors not resolving CSS variables, causing invisible chart lines.
- Fixed container commands (docker/nerdctl) flashing a console window on Windows.
- Fixed K8s node operations: added confirmation prompts before cordon/uncordon and corrected the drain icon.
- Fixed macOS Caps Lock swallowing the first letter typed after toggling on macOS 26/27 beta. (@surenwuyuwuqiu)
- Fixed legacy SSH servers failing to connect due to missing CBC/3DES ciphers and hmac-sha1-96 MAC algorithms.
- Fixed terminal history recording reliability: restored bottom-up prompt scanning for compatibility with diverse shell prompts. Added independent AI transcription toggle so users can keep history suggestions while turning off AI-powered command rewrites.
- Fixed X11 forwarding failure on macOS when DISPLAY is empty (e.g. app launched from Finder/Dock) by probing /tmp/.X11-unix. (@surenwuyuwuqiu)
- Fixed SSH jump-host tunnel not working for terminal sessions. (@wangxufeng)
- Fixed AI sidebar allowing messages to be sent without an active terminal session, causing failed messages to persist in conversation history and later be sent to the LLM as valid context. The input box and send button are now disabled when no terminal panel is associated.
- Fixed NL-to-SQL prompt not including the currently selected table name, so the AI had no context about which table the user was viewing.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @coderstory, @wangxufeng, and @surenwuyuwuqiu for their contributions to this release.

### 更新内容

**新功能**
- SSH X11 转发。在 SSH 连接表单高级设置中开启后，可运行 Linux 远程 GUI 应用并在本地显示。Windows 版本内置 VcXsrv X Server，无需额外安装组件；macOS 需安装 XQuartz（`brew install --cask xquartz`）。
- X11 桌面支持。通过 SSH X11 转发启动完整 Linux 桌面环境（GNOME、KDE、XFCE、MATE、Cinnamon、Openbox）。新增 `x11-desktop` 会话类型，独立 SSH 凭据，自动启动 X Server。
- VNC 连接新增 TLS 开关、共享会话模式和 VNC Repeater ID 支持。
- S3 连接支持虚拟主机风格端点（如阿里云 OSS、腾讯 COS、华为 OBS）。
- Telnet 协商模式、本地回显、发送模式（字符/行）、换行模式（CR/CRLF）、字符编码选项，提升与不同 Telnet 服务器的兼容性。
- 串口连接统一使用标准连接表单（与其他协议一致），不再使用独立弹窗；新增字符编码、本地回显、换行模式支持。
- SSH、Telnet、Serial、Mosh 连接新增退格键选项：可选 ASCII Delete（DEL, 0x7F）、ASCII Backspace（BS, 0x08）或 VT220 Delete。默认值由 DEL 改为 Backspace（^H），兼容华为、H3C、Cisco 网络设备及串口控制台。

**改进**
- xterm.js 升级至 6.0.0，所有插件同步升级（search、fit、ligatures、Unicode 11、clipboard）。（@coderstory）

- 内置 JetBrains Mono Variable 字体作为默认等宽字体，取代之前的系统字体栈。（@coderstory）
- 终端文本高亮不再匹配代码块（code fence）内的内容，避免 AI 输出和 markdown 中的误高亮。（@coderstory）
- 全面性能与稳定性加固：store（原子写入、防抖 I/O、会话分片）、session（更大读缓冲、事件驱动刷新循环、减少锁竞争）、前端（rAF 合并渲染、缓存计算、会话数据环形缓冲）、K8s（HTTP 传输调优、缓存解析、watch/log 断线重连与退避）、AI/LLM（SSE 流缓冲、共享 HTTP 客户端、类型化事件负载）、sync（异步初始化、ETag 条件 GET、AES-GCM AAD 绑定）、database（连接池调优、并行 schema 加载、标识符转义）。（@coderstory）
- 终端双击选词分隔符可配置。在终端设置中可自定义哪些字符作为单词边界。（@wangxufeng）
- 优化内置 uniTerm Dark 和 uniTerm Light 终端主题配色。
- macOS DMG 拆分为独立的 amd64 和 arm64 安装包，避免通用二进制在 macOS 26+ 上的兼容性警告。
- 连接备注字段，可为任意连接添加自由文本备注。
- 重新设计数据库和 MongoDB 自然语言查询布局：NL 输入框移至 SQL/过滤编辑器左侧，高度一致，生成按钮样式与执行按钮对齐，更易发现和使用。

**Bug 修复**
- 修复光标闪烁设置在重启后丢失的问题。（@wangxufeng）
- 修复终端历史恢复后 Claude Code 等 TUI 字形无法渲染的问题。（@coderstory）
- 修复 AI CancelChatStream 在重叠请求下的竞态条件。（@coderstory）
- 修复同步安全：强制文件白名单校验、改进密码不匹配处理、移除配置中的主机名泄露。（@coderstory）
- 修复 K8s 认证 401 不重试、watch/log 流断线不重连的问题。（@coderstory）
- 修复数据库连接池竞态、查询超时处理、SQL Server 资源清理问题。（@coderstory）
- 修复监控趋势图颜色因 CSS 变量未解析导致折线不可见的问题。
- 修复容器命令（docker/nerdctl）在 Windows 上弹出控制台窗口的问题。
- 修复 K8s 节点操作：cordon/uncordon 增加确认提示，修正 drain 图标。
- 修复 macOS 26/27 beta 上切换 Caps Lock 后第一个字母被吞掉的问题。（@surenwuyuwuqiu）
- 修复旧版 SSH 服务器因缺少 CBC/3DES 加密算法和 hmac-sha1-96 MAC 算法导致连接失败的问题。
- 修复终端历史记录可靠性：恢复从底部向上扫描提示符的命令提取逻辑，兼容更多类型的 shell 提示符。新增独立的 AI 转录开关，用户可在保留历史补全建议的同时关闭 AI 命令改写。
- 修复 macOS 上从 Finder/Dock 启动应用时 DISPLAY 为空导致 X11 转发失败的问题，自动探测 /tmp/.X11-unix。（@surenwuyuwuqiu）
- 修复 SSH 跳板隧道对终端会话不生效的问题。（@wangxufeng）
- 修复 AI 侧边栏在无终端关联时仍可发送消息的问题，导致失败消息残留在对话历史中并被后续发送给 LLM 作为有效上下文。现在无终端面板关联时输入框和发送按钮自动禁用。
- 修复 NL-to-SQL 提示词不含当前选中表名，AI 无法感知用户正在操作哪张表的问题。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @coderstory、@wangxufeng 和 @surenwuyuwuqiu 对本版本的贡献。

## v1.6.0

### What's Changed

**New Features**
- AI skills. Create, import, and manage skills under Settings → Skills & Commands, then type `/` in the AI input to attach one to your request; the AI can also save new skills itself. (@surenwuyuwuqiu)
- AI commands: reusable prompt templates. Pick one from the `/` dropdown to attach it as a `/name` tag, optionally type arguments, and send; managed under Settings → Skills & Commands. (@surenwuyuwuqiu)
- Kubernetes management. Connect to clusters via kubeconfig with optional SSH tunnel; browse and manage resources, edit YAML, follow Pod logs, exec into containers in a terminal tab, and view Pod/Node CPU and memory metrics.
- Container management for Docker, Podman, and nerdctl — manage containers and images on the local machine or on remote hosts over SSH: live status, lifecycle actions, logs, exec, image pull, and more.
- Redis Sentinel (failover) connection mode. Choose Sentinel mode in the Redis connection form and supply a sentinel node list plus the master name; all existing Redis operations work unchanged. (@surenwuyuwuqiu)

**Improvements**
- OpenAI Responses API protocol support — a third AI protocol option for providers whose channel is natively the Responses API (e.g. codex-style channels).
- Drag-and-drop reordering for connection groups and connections in the sidebar, with the custom order persisted. Reordering is paused while search or type filters are active.
- Tab titles are now capped at a max width with ellipsis; hovering shows the full name in a tooltip. (@surenwuyuwuqiu)

**Bug Fixes**
- Fixed SSH keepalive silently stopping, which let the server drop the connection for idleness: the wait-for-reply loop could wedge on an internal lock and halt heartbeats entirely. Keepalive is now send-only with the interval aligned to OpenSSH defaults (60s); tunnel keepalive got the same fix.
- Fixed the group not being preserved when creating a second connection inside the same group.
- Fixed wrong OpenAI protocol message order when one turn carries both a tool_result and text.
- Fixed session logs capturing interactive-edit garbage: readline cursor/erase CSI sequences are now preserved so edited command lines reconstruct cleanly.
- Fixed the RDP fullscreen connection bar showing a minimize button (the window has no taskbar entry, so minimizing made it unrecoverable).
- Fixed RDP security dialog auto-dismiss failing on Chinese Windows due to a missing '连接(&N)' button caption.
- Fixed RDP keyboard focus not following mouse clicks in multi-monitor setups.
- Fixed macOS Cmd+C/V/X/A/Z not working in text fields app-wide: the native Edit menu was inadvertently removed when hiding Wails' default menus; it is now restored on macOS only. (@surenwuyuwuqiu)
- Fixed AI "fetch models" failing against Anthropic-compatible endpoints (404 or auth error); the model list request is now protocol-aware, using the `/v1/models` path with x-api-key auth for Anthropic.
- Fixed some remote apps (e.g. hermes CLI on Ubuntu 26.04) failing with a tcsetattr error: SSH PTY modes now use the standard baud rate 38400 instead of a non-standard value that polluted the remote termios.
- Fixed RPM packages failing to install on Fedora due to wrong dependency names.

**Notes**
- As this open-source software has not purchased a code-signing certificate, the unsigned executable may trigger false positives in some antivirus engines (e.g. Windows Defender). This is a known issue with Go/Wails applications (see [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)). You can add an exclusion rule in your antivirus to allow it. Please download only from the official open-source channels — GitHub and Gitee. If you are still concerned about malware, you can download the source code and build and run it locally yourself.

Thanks to @surenwuyuwuqiu for their contributions to this release.

### 更新内容

**新功能**
- AI 技能（Skills）。在设置 → 技能与命令中创建、导入和管理技能，AI 输入框输入 `/` 即可为本次请求挂载；AI 也可自行保存新技能。（@surenwuyuwuqiu）
- AI 命令（Commands）：可复用的 Prompt 模板。`/` 下拉选择后以 `/名称` 标签挂载，可附加参数后发送；在设置 → 技能与命令中管理。（@surenwuyuwuqiu）
- Kubernetes 管理。通过 kubeconfig 接入集群（支持 SSH 隧道），浏览与管理集群资源、编辑 YAML、跟随查看 Pod 日志、exec 进入容器终端，并展示 Pod/Node CPU、内存指标。
- 容器管理：支持 Docker、Podman、nerdctl，管理本机或 SSH 远程主机的容器与镜像，可查看状态、生命周期操作、日志、exec、镜像拉取等。
- Redis 哨兵连接模式。在 Redis 连接表单中选择哨兵模式，填写哨兵节点列表和主节点名即可；现有全部 Redis 操作无需改动。（@surenwuyuwuqiu）

**改进**
- 支持 OpenAI Responses API 协议——第三种 AI 协议选项，适用于原生 Responses API 通道的服务商（如 codex 类通道）。
- 侧边栏连接分组与连接支持拖拽排序，顺序持久化保存；搜索或类型过滤生效时暂停排序。
- 标签标题限制最大宽度，超出部分显示省略号，悬停 tooltip 显示完整名称。（@surenwuyuwuqiu）

**Bug 修复**
- 修复 SSH 保活静默失效、连接因空闲被服务端断开的问题：等待回复的保活分支可能卡死在内部锁上，导致心跳完全停发。保活改为只发不等回复，间隔对齐 OpenSSH 默认值（60 秒）；隧道保活同步修复。
- 修复在同一分组内新建第二个连接时分组未被保留的问题。
- 修复 OpenAI 协议同一轮同时携带 tool_result 和 text 时消息顺序错误的问题。
- 修复会话日志记录交互式编辑内容时出现乱码交错的问题：现在保留 readline 光标/擦除 CSI 序列，命令行可正确还原。
- 修复 RDP 全屏连接条显示最小化按钮的问题（窗口无任务栏入口，最小化后无法恢复）。
- 修复中文 Windows 下 RDP 安全对话框自动关闭失败的问题（缺少「连接(&N)」按钮文本）。
- 修复多显示器环境下 RDP 点击后键盘焦点不跟随的问题。
- 修复 macOS 下文本框内 Cmd+C/V/X/A/Z 全平台失效的问题：此前隐藏 Wails 默认菜单时误删了原生 Edit 菜单，现仅在 macOS 上恢复标准菜单。（@surenwuyuwuqiu）
- 修复 AI「拉取模型列表」在 Anthropic 兼容端点上失败（404 或鉴权错误）的问题：拉取请求改为协议感知，Anthropic 使用 `/v1/models` 路径与 x-api-key 鉴权。
- 修复部分远程程序（如 Ubuntu 26.04 上的 hermes CLI）报 tcsetattr 错误的问题：SSH PTY 模式改用标准波特率 38400，不再使用会污染远端 termios 的非标准值。
- 修复 RPM 安装包在 Fedora 上因依赖名错误无法安装的问题。

**说明**
- 由于本开源软件未购买代码签名证书，未签名的可执行文件可能被部分杀毒引擎（如 Windows Defender）误报拦截。这是 Go/Wails 应用的已知问题（参见 [wailsapp/wails#3308](https://github.com/wailsapp/wails/issues/3308)）。可在杀毒软件中为其添加排除规则以放行。请务必从 GitHub、Gitee 官方开源渠道下载软件。如仍担心存在病毒，可自行下载源代码在本地构建运行。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.5.2

### What's Changed

**New Features**
- Customizable application background image. Set an image under App Settings with adjustable opacity and blur.
- RDP full-screen support with a new "Fullscreen" resolution option and a fullscreen toggle button on the status bar.
- RDP now forwards the Windows key and combos (Win+E, Win+D, Alt+Tab, etc.) to the remote machine. Ctrl+Alt+Del still can't be forwarded — press Ctrl+Alt+End instead.
- Option to use the OS native title bar instead of the built-in one, toggleable under App Settings (requires restart).

**Improvements**
- Disconnect / reconnect notices now include the local timestamp so you can tell when a connection dropped.
- AI request timeout raised from 2 to 10 minutes for both Anthropic and OpenAI paths, so slower models can finish complex tasks; on timeout a friendly localized hint is shown instead of the raw `context deadline exceeded`.
- Anthropic base URL now accepts both suffix-free (`https://api.anthropic.com`) and `/v1`-suffixed (`https://api.anthropic.com/v1`) addresses.
- App icons: the "U" glyph is now centered, and the macOS icon has proper transparent padding so the Dock/Launchpad tile size matches other apps.

**Bug Fixes**
- Fixed the terminal text highlighter not highlighting numbers adjacent to letters and stripping upstream SGR spans on partially-styled lines.
- Fixed the PostgreSQL browser listing every database in the cluster and showing "no tables" for the ones the connection can't query; the browser is now scoped to the connected database, and the connection form requires a database name for PostgreSQL.
- Fixed the RDP native window covering modal dialogs (tunnel add/edit, close-tab / close-app confirm).
- Fixed the RDP dialog auto-dismiss loop repeatedly closing the system `mstsc.exe`'s own password/security prompts.

### 更新内容

**新功能**
- 应用背景图片自定义。可在应用设置中设置背景图片，支持调整不透明度和模糊度。
- RDP 全屏支持，新增“全屏”分辨率选项，新增全屏切换状态栏按钮。
- RDP 现在会将 Windows 键及组合键（Win+E、Win+D、Alt+Tab 等）转发到远端。Ctrl+Alt+Del 属于安全注意序列，任何 RDP 客户端都无法转发，请改用 Ctrl+Alt+End。
- 新增使用系统原生标题栏的选项，应用设置中可切换，重启后生效。

**改进**
- 断线 / 重连提示条附带本地时间戳，方便定位连接中断的具体时间。
- AI 请求超时时间从 2 分钟提升到 10 分钟（Anthropic 与 OpenAI 通道均生效），避免慢速模型跑长任务被打断；超时时不再显示原始的 `context deadline exceeded`，改为友好的多语言提示。
- Anthropic base URL 同时兼容不带 `/v1` 后缀（如 `https://api.anthropic.com`）和带 `/v1` 后缀（如 `https://api.anthropic.com/v1`）两种写法。
- 应用图标 “U” 字重新居中，macOS 图标补充透明外边距，Dock 与 Launchpad 中显示尺寸与其他应用一致。

**Bug 修复**
- 修复终端文本高亮无法高亮紧邻字母的数字，以及在部分带样式的行中会覆盖上游 SGR 样式的问题。
- 修复 PostgreSQL 数据库浏览器列出整个集群所有数据库、点开非当前连接的库时显示“无表”的问题；现在浏览器仅显示当前连接的数据库，新建 PostgreSQL 连接时数据库名为必填。
- 修复 RDP 原生窗口盖住模态弹窗（隧道新建/编辑、关闭标签确认、关闭应用确认）的问题。
- 修复 RDP 对话框自动关闭逻辑误关系统 `mstsc.exe` 自身密码/安全提示框，导致 mstsc 弹窗反复闪烁的问题。

## v1.5.1

### What's Changed

**Improvements**
- macOS Cmd+Q quits the app and Cmd+W closes the current tab, matching platform conventions. (@surenwuyuwuqiu)
- Right-click the copyable log-path toast (shown after starting a session log) to copy the path or select all. (@surenwuyuwuqiu)
- Batch-close confirmation for "close right / others / left" is now consolidated into a single dialog with a per-batch "don't show again" option, instead of one prompt per connected tab.
- Hovered tab is decluttered — the AI-lock button moved into the context menu, no more layout shift when hovering. (@surenwuyuwuqiu)
- The tab "+" button now stays visible next to the overflow (…) menu when tabs overflow, so it's still reachable with many tabs open. (@surenwuyuwuqiu)
- The "Copy as Markdown" button no longer shows under assistant messages that contain only tool calls (previously it appeared but the clipboard ended up empty).

**Bug Fixes**
- Fixed `Ctrl+V` being swallowed by the paste shortcut, so it now passes through to shell prompts (bash `Ctrl+V Tab` for literal characters) and alternate-screen apps such as vim visual block, k9s and less. Paste is bound to the platform shortcut only. (@wangxufeng, @surenwuyuwuqiu)
- Fixed the "close tab prompt" / "close app prompt" settings not being persisted — they were silently dropped by the backend, so the toggle reset to on after every restart.
- Fixed tab duplicate for terminal / SFTP / database tabs, which regressed after v1.5.0 (multi-minute black screen or missing session bind). (@surenwuyuwuqiu)
- Fixed the tab rename input being unable to take focus because the terminal focus guard immediately stole it back. (@surenwuyuwuqiu)

Thanks to @surenwuyuwuqiu and @wangxufeng for their contributions to this release.

### 更新内容

**改进**
- 支持 macOS 快捷键 Cmd+Q 退出应用、Cmd+W 关闭当前标签，与系统习惯保持一致。（@surenwuyuwuqiu）
- 会话日志路径提示框支持右键复制 / 全选，方便复制日志路径。（@surenwuyuwuqiu）
- "关闭右侧 / 其他 / 左侧"批量操作现在合并为一次确认弹窗，"不再提示"当批即生效，不再逐个标签弹框。
- Hover 标签栏简化：AI 锁定按钮移入右键菜单，hover 不再发生布局抖动。（@surenwuyuwuqiu）
- 标签数量溢出时，"+"新建按钮改为固定显示在溢出 (…) 菜单旁，方便快速新建。（@surenwuyuwuqiu）
- 只包含工具调用的 AI 消息不再显示"复制为 Markdown"按钮（此前会显示，但点击后复制的是空白内容）。

**Bug 修复**
- 修复 `Ctrl+V` 被粘贴快捷键截获的问题，现在在 shell 提示符（bash `Ctrl+V Tab` 输入字面字符）和终端全屏应用（vim 可视块、k9s、less 等）中都能正确透传。粘贴仅绑定到各平台自身的快捷键。（@wangxufeng、@surenwuyuwuqiu）
- 修复"关闭标签页提示"、"关闭应用提示"开关无法持久化的问题——之前后端未识别这两个字段，重启后总是回到默认开启状态。
- 修复 v1.5.0 后终端 / SFTP / 数据库标签"复制标签"功能失效的问题（表现为终端出现长时间黑屏或会话未绑定）。（@surenwuyuwuqiu）
- 修复标签重命名输入框无法获得焦点的问题——之前被终端焦点守卫立即抢回。（@surenwuyuwuqiu）

感谢 @surenwuyuwuqiu 和 @wangxufeng 对本版本的贡献。

## v1.5.0

### What's Changed

**New Features**
- Multi-panel AI lock. AI can now control multiple terminals at once, each shown with its own tag. Type `#<tag>` in the AI prompt to reference a specific terminal.
- AI `ask_user` tool. AI can now ask the user a question mid-run and wait for a reply before continuing, useful for confirmations and clarifications during autonomous runs.
- Session output log for terminal connections. Toggle recording from the workspace panel menu; a red REC dot on the panel header indicates an active recording. Logs are written to the configured session log directory.
- SSH passphrase-protected private keys are now supported.
- Configurable terminal cursor blink, default on. (@wangxufeng)
- Confirm before closing tabs/panels with active connections, with a "don't show again" option. (@wangxufeng)
- Tab lock. Right-click a tab to lock it, preventing accidental close and marking it with a lock indicator.
- Duplicate tab action extended to SFTP and database tabs (previously terminal-only).

**Improvements**
- AI tool chain: added completion markers, `start_command` risk control, and `collect_output` idle detection.
- AI `execute_command` / `collect_output` results now include the reappeared shell prompt line, so the AI can tell the command finished and see the current shell state (cwd, user, REPL, etc.).
- WebDAV connections now use a single URL field instead of separate host / port / SSL toggle.
- Broadcast input per-panel selection: Ctrl+click panel headers to include only the selected panels when broadcasting keyboard input.
- Tab styling: minimum tab width, close / more / AI-lock buttons now appear only on hover, and duplicate local terminal names are deduplicated.
- Sidebar Tunnels and QuickCommands panels no longer draw a divider between the search bar and list.
- Default `openSettings` shortcut bound to Ctrl+, (Cmd+, on macOS via alias). (@surenwuyuwuqiu)

**Bug Fixes**
- Fixed paste via Wails clipboard and added Cmd+A / Cmd+C / Cmd+V shortcuts for WKWebView. (@surenwuyuwuqiu)
- Fixed quick command auto-run using LF as line terminator; switched to CR so multi-line commands run reliably. (@wangxufeng)
- Fixed terminal scroll position not being restored after KeepAlive reactivation. (@wangxufeng)
- Fixed WebDAV `ChangeRemoteDir` failing on servers whose `Stat` on a directory does not return the expected metadata; switched to `ReadDir`.
- Fixed edits not being persisted when using Save & Connect for non-terminal connection types.
- Fixed accent color not following the current theme in some views.
- Fixed empty menu bar causing a white line at the top of frameless windows on Linux.
- Fixed the tab "+" button ignoring clicks when the event was swallowed by header drag detection on Windows.
- Fixed terminal losing focus after quick command execution, drag, or scrollbar interaction.

Thanks to @wangxufeng and @surenwuyuwuqiu for their contributions to this release.

### 更新内容

**新功能**
- 多面板 AI 锁定。AI 可同时控制多个终端，每个终端显示独立标签，AI 输入框中可用 `#<标签>` 引用特定终端。
- AI `ask_user` 工具。AI 可在自主运行过程中向用户提问并等待回复后继续，适用于确认和澄清场景。
- 终端会话输出日志。工作区面板菜单中可开关录制，录制期间面板标题栏显示红色 REC 圆点，日志保存至设置中配置的会话日志目录。
- SSH 支持带口令保护的私钥。
- 终端光标闪烁可配置，默认开启。（@wangxufeng）
- 关闭带活动连接的标签/面板时弹出确认，支持"不再提示"选项。（@wangxufeng）
- 标签页锁定。右键标签选择锁定后，标签显示锁定图标并防止误关闭。
- 复制标签功能扩展到 SFTP 和数据库标签（此前仅支持终端）。

**改进**
- AI 工具链优化:新增命令完成标记、`start_command` 风险控制、`collect_output` 空闲检测。
- AI `execute_command` / `collect_output` 结果现在保留末尾重新出现的 shell 提示符行，AI 可据此判断命令已结束并感知当前 shell 状态（cwd、用户、REPL 等）。
- WebDAV 连接改为单个 URL 字段，取代原来的主机 / 端口 / SSL 开关组合。
- 广播输入支持逐面板选择:Ctrl+点击面板标题栏,广播输入时仅发送到已选中的面板。
- 标签样式:限制最小宽度,关闭/更多/AI 锁定按钮仅在 hover 时显示,本地终端同名标签自动去重。
- 侧边栏 Tunnels 和 QuickCommands 面板取消搜索栏与列表之间的分隔线。
- 默认 `openSettings` 快捷键绑定为 Ctrl+,(macOS 通过别名映射为 Cmd+,)。(@surenwuyuwuqiu)

**Bug 修复**
- 修复通过 Wails 剪贴板粘贴的问题,并为 WKWebView 补充 Cmd+A / Cmd+C / Cmd+V 快捷键。(@surenwuyuwuqiu)
- 修复快捷命令自动执行使用 LF 换行导致多行命令无法可靠执行的问题,改用 CR。(@wangxufeng)
- 修复 KeepAlive 重新激活后终端滚动位置未恢复的问题。(@wangxufeng)
- 修复 WebDAV `ChangeRemoteDir` 在某些服务端目录 `Stat` 无法返回预期元信息时失败的问题,改用 `ReadDir`。
- 修复非终端类型连接使用"保存并连接"时编辑内容未持久化的问题。
- 修复部分视图强调色未跟随当前主题的问题。
- 修复 Linux 无边框窗口顶部因空菜单栏出现白色横线的问题。
- 修复 Windows 标签栏 "+" 按钮点击被标题栏拖拽逻辑吞掉的问题。
- 修复快捷命令执行、拖拽、滚动条交互后终端失焦的问题。

感谢 @wangxufeng 和 @surenwuyuwuqiu 对本版本的贡献。

## v1.4.2

### What's Changed

**New Features**
- Default local shell setting. Users can now configure the preferred shell (e.g. Git Bash, PowerShell, CMD) for local terminal sessions in Settings, instead of relying on the auto-detection fallback order.
- Linux ARM64 builds and DEB/RPM packaging. New build targets for Linux ARM64 with native `.deb` and `.rpm` package artifacts.
- Windows ARM64 build target.
- Multi-level nested connection groups. Groups now support unlimited nesting depth with tree-based rendering, drag-to-reparent, breadcrumb navigation, and auto-rename on name conflict.

**Improvements**
- AI autonomous interaction rounds are now configurable. Set the max conversation turns per AI request in Settings (0 = unlimited, default 20).
- Terminal search shortcut (Ctrl+F) is now configurable in the keyboard shortcuts settings and can be rebound or disabled.
- Inactive tab hover now shows a close button, allowing single-click close without activating first.
- Local terminal now defaults to the user's home directory instead of the application directory on Windows. New sessions correctly start in `$HOME`.
- Protocol selector in AI model dialog changed from dropdown to toggle buttons for quicker switching.
- AI sidebar model dropdown now includes an "Add Model" entry; shows "+ Add Model" button when no models are configured.

**Bug Fixes**
- Fixed raw OS pipe error message ("The pipe has been ended.") shown when a TUI application (e.g. opencode) exits without cleaning up on Windows ConPTY. Read errors after session shutdown are now silently suppressed.
- Fixed default macOS Edit and Window menus appearing as empty dropdowns. An empty app menu is now set to hide them.
- Fixed RDP connection type appearing on macOS and Linux where it is not supported. RDP is now only shown on Windows.
- Fixed smart suggestion selection being reset by a race condition between keyboard navigation and the debounced suggestion refresh timer.
- Fixed paste via Wails clipboard and added bracketed paste support, so pasting into vim no longer re-indents each line. Cmd/Ctrl+V now pastes correctly in WKWebView. (@surenwuyuwuqiu)
- Fixed SSH disconnect when full-screen apps (vim/less/tmux) query the terminal in alternate screen. Terminal query responses are now filtered in both screens, preventing stray ESC sequences from being forwarded to the remote app. (@surenwuyuwuqiu)
- Fixed model form dialog not clearing state between edits: clicking "Add Model" after editing now correctly creates a new model instead of updating the previous one, and test connection results no longer carry over across different model edits.

Thanks to @surenwuyuwuqiu for contributions to this release.

### 更新内容

**新功能**
- 默认本地终端 Shell 设置。用户可在设置中配置本地终端会话的首选 Shell（如 Git Bash、PowerShell、CMD）。
- Linux ARM64 构建与 DEB/RPM 打包。新增 Linux ARM64 构建目标，提供原生 `.deb` 和 `.rpm` 安装包。
- Windows ARM64 构建目标。
- 多级嵌套连接分组。分组支持无限层级嵌套，提供树状渲染、拖拽调整父子关系、面包屑导航，同层级名称冲突时自动重命名。

**改进**
- AI 自主交互最大轮次可配置。在设置中可调整单次 AI 请求的最大对话轮数（0 = 不限制，默认 20）。
- 终端搜索快捷键（Ctrl+F）加入可配置快捷键列表，支持改绑或关闭。
- 非激活标签 hover 时显示关闭按钮，无需先点击激活再关闭。
- Windows 本地终端默认工作目录改为用户主目录，新会话从 `$HOME` 启动。
- AI 模型编辑对话框中协议选择从下拉菜单改为 toggle 按钮，切换更快捷。
- AI 边栏模型下拉菜单新增「添加模型」入口，无模型时显示「+ 添加模型」按钮。

**Bug 修复**
- 修复 Windows ConPTY 下 TUI 应用（如 opencode）退出时未清理终端状态，导致显示原始 OS 管道错误信息（"The pipe has been ended."）的问题。会话关闭后的读取错误现在会被静默忽略。
- 修复 macOS 默认 Edit 和 Window 菜单显示为空下拉的问题。设置空菜单以隐藏它们。
- 修复 RDP 连接类型在 macOS 和 Linux 上显示的问题（RDP 仅 Windows 支持）。
- 修复智能提示选择因键盘导航与防抖刷新计时器之间的竞态条件而被重置的问题。
- 修复终端粘贴通过 Wails 剪贴板实现，新增 bracketed paste 支持，粘贴到 vim 不再逐行缩进。Cmd/Ctrl+V 在 WKWebView 下正常粘贴。（@surenwuyuwuqiu）
- 修复全屏应用（vim/less/tmux）在交替屏幕中查询终端时导致 SSH 断开的问题。终端查询响应现在在两个屏幕中均被过滤，防止 stray ESC 序列被转发到远程应用。（@surenwuyuwuqiu）
- 修复模型编辑表单在编辑和新建之间状态未清除的问题：编辑后点击新建不再更新旧模型，测试连接结果也不会在不同模型之间串扰。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.4.1

### What's Changed

**New Features**
- MongoDB database support. Browse databases, collections, and documents in a tree sidebar, run queries with the built-in editor, and edit documents inline.
- AI natural language query editor. Describe queries in plain language and AI generates the SQL or MongoDB filter, with automatic schema detection for relational databases.
- SSH Tunnel Manager with 3 forwarding modes: Local (-L), Remote (-R), and Dynamic (-D, SOCKS5). Tunnels are listed in a dedicated sidebar tab with status indicators, grouped by drag-and-drop, and support auto-start on launch. (@surenwuyuwuqiu)
- Serial port logging. Serial sessions can now log all received data to a file with save and toggle options, configurable in the serial connection dialog. (@wangxufeng)
- AI message queue. Messages can now be sent while the AI agent is running or awaiting confirmation. Queued messages are shown as removable chips above the input and are processed at the next turn boundary of the agent loop.

**Improvements**
- Ctrl+scroll wheel now adjusts terminal font size (±1px, range 8–32px), persisted to settings. (@surenwuyuwuqiu)
- Added persistent AI execution status indicator showing the current phase, e.g. Thinking, Outputting, Executing, Awaiting confirmation.
- AI command end detection now strips ANSI from captured prompts and uses an idle heuristic for dynamic prompts, improving reliability.
- AI sidebar toolbar optimized: send/stop replaced with icon buttons, model/mode selectors use ghost style, avatars removed with wider message layout, markdown rendered in `<p>` tags.
- AI user cancellation now shows a friendly "interrupted" hint instead of raw "API Error: context canceled".
- New and edit connection forms: host and port merged into a single row for a cleaner layout.
- Sidebar group collapse state is now persisted and restored across operations, no longer lost when creating or deleting a connection group.

**Bug Fixes**
- RDP blank screen on modern Windows. Added NLA (Network Level Authentication) toggle — enabled by default using CredSSP for modern Windows; when disabled, uses RDP standard security with password field for auto-login.
- Fixed system font names containing spaces (e.g. DejaVu Sans Mono, Fira Code) causing double-width character rendering in terminal due to CSS font-family parsing. (@surenwuyuwuqiu)
- Fixed window geometry not being clamped and saving incorrectly when minimized, which could cause the next launch to restore an off-screen or minuscule window. Added `MinWidth=400, MinHeight=300` and skip geometry save while minimized.
- Fixed database connection advanced configuration toggle not responding when creating or editing database connections. The SSH tunnel selector was incorrectly nested inside the FTP type template, causing it to not render for non-FTP connection types.

Thanks to @surenwuyuwuqiu and @wangxufeng for their contributions to this release.

### 更新内容

**新功能**
- MongoDB 数据库支持。树形侧边栏浏览数据库、集合和文档，内置编辑器运行查询，支持文档行内编辑。
- AI 自然语言数据库查询。用自然语言描述需求，AI 自动生成 SQL 或 MongoDB 查询，关系型数据库支持自动获取表结构。
- SSH 隧道管理器，支持三种转发模式：本地转发 (-L)、远程转发 (-R) 和动态转发 (-D, SOCKS5)。隧道在侧边栏独立标签页中展示，带状态指示灯，支持拖拽分组、启动时自动启动。（@surenwuyuwuqiu）
- 串口日志记录。串口会话可将所有接收数据记录到文件，支持保存和开关切换，在串口连接对话框中配置。（@wangxufeng）
- AI 消息队列。AI 运行中或等待确认时仍可发送消息，待处理消息显示为可移除的标签条，在 agent 循环的下一轮边界自动注入处理。

**改进**
- Ctrl+滚轮调整终端字体大小（±1px，范围 8–32px），设置自动持久化。（@surenwuyuwuqiu）
- AI 执行时增加运行状态提示，如：思考中、输出中、执行中、等待确认。
- AI 命令结束检测优化：从捕获的提示符中剥离 ANSI 转义序列，并为动态提示符增加空闲启发式检测，提升判断可靠性。
- AI 侧边栏工具栏优化：发送/停止改为图标按钮，模型/模式选择器使用 ghost 样式，移除头像并拓宽消息布局，markdown 用 `<p>` 标签包裹。
- AI 用户取消操作时显示友好的"已中断"提示，而非原始的 "API Error: context canceled"。
- 连接新建和编辑表单主机与端口合并为单行，布局更简洁。
- 侧边栏分组折叠状态持久化，新建或删除分组时不再丢失折叠状态。

**Bug 修复**
- 修复现代 Windows RDP 连接白屏问题。新增 NLA（网络级认证）开关 — 默认启用 CredSSP 兼容现代 Windows；关闭后使用 RDP 标准安全并显示密码字段用于自动登录。
- 修复系统字体名含空格（如 DejaVu Sans Mono、Fira Code）时，CSS font-family 解析错误导致终端字符占双格、比例失调的问题。（@surenwuyuwuqiu）
- 修复窗口无边界的尺寸和最小化时保存异常坐标导致的启动后窗口不可见或极小问题。新增 `MinWidth=400, MinHeight=300`，最小化时跳过保存窗口位置和尺寸。
- 修复新建或编辑数据库连接时，高级配置按钮点击无效的问题。原因：SSH 隧道选择器被错误地嵌套在 FTP 类型模板内部，导致非 FTP 连接类型无法正常渲染。

感谢 @surenwuyuwuqiu 和 @wangxufeng 对本版本的贡献。

## v1.3.3

### What's Changed

**New Features**
- Remember window size and position between sessions. The application now restores its previous window size and position on launch.

**Improvements**
- Database connection parameters are now customizable. DSN construction has been refactored to URL format, allowing users to specify additional database parameters.
- Linux build upgraded to webkit2gtk 4.1 for better compatibility with newer distributions.

**Bug Fixes**
- Fixed macOS key-repeat in terminal. Holding a key now produces continuous input instead of showing the press-and-hold accent picker. (@surenwuyuwuqiu)
- Fixed serial port busy error caused by duplicate Connect call when creating a serial session. (@wangxufeng)
- Fixed AI model field not showing as clickable dropdown after fetching model list from server. Changed from autocomplete to filterable select with allow-create. (@surenwuyuwuqiu)
- Fixed preset terminal fonts (Monaco, Menlo, Consolas, etc.) not appearing in the font picker when installed. Font scanning now returns all families and unions in well-known presets, regardless of the font file's isFixedPitch flag. (@surenwuyuwuqiu)
- Fixed CJK font family names (e.g. 幼圆, 隶书) regressing to ASCII names after adding Mac Roman encoding fallback for legacy macOS system fonts like Monaco. (@surenwuyuwuqiu)
- Fixed some issues that could cause RDP white screen. Added TCP pre-check for unreachable hosts, enabled NLA/CredSSP support, added periodic connection status detection with reconnect button, fixed reconnection panel positioning, and fixed background color in light mode.
- Fixed SSH connection failures with older servers by adding legacy key exchange algorithms (diffie-hellman-group1-sha1, diffie-hellman-group14-sha1, diffie-hellman-group-exchange-sha1).
- Fixed version comparison incorrectly treating pre-release versions (e.g. 1.3.3-alpha) as older than stable releases. (@wangxufeng)

Thanks to @surenwuyuwuqiu and @wangxufeng for their contributions to this release.

### 更新内容

**新功能**
- 窗口大小和位置记忆。应用启动时自动恢复上次关闭时的窗口大小和位置。

**改进**
- 数据库连接参数支持自定义。DSN 构建重构为 URL 格式，用户可指定额外数据库参数。
- Linux 构建升级至 webkit2gtk 4.1，提升新版发行版兼容性。

**Bug 修复**
- 修复 macOS 终端按键长按不重复的问题。长按按键现在产生连续输入，不再弹出重音符号选择器。（@surenwuyuwuqiu）
- 修复创建串口会话时重复调用 Connect 导致"串口被占用"错误。（@wangxufeng）
- 修复 AI 模型输入框从服务器拉取列表后不显示为可点击下拉框的问题。从 autocomplete 改为 filterable select。（@surenwuyuwuqiu）
- 修复 Monaco、Menlo、Consolas 等已安装的预设终端字体在字体选择器中不显示的问题。字体扫描现在返回所有字体族并合并知名预设字体，不受字体文件 isFixedPitch 标志影响。（@surenwuyuwuqiu）
- 修复新增 Mac Roman 编码回退（支持 Monaco 等 macOS 旧字体）后，CJK 字体名称（如幼圆、隶书）错误显示为 ASCII 名称的问题。（@surenwuyuwuqiu）
- 修复部分会导致 RDP 连接白屏的问题。增加 TCP 预检、启用 NLA/CredSSP 支持、增加断线检测及重连按钮、修复重连后面板定位错误、修复浅色模式背景色异常。
- 修复旧版 SSH 服务器连接失败问题，增加 legacy 密钥交换算法支持（diffie-hellman-group1-sha1、diffie-hellman-group14-sha1、diffie-hellman-group-exchange-sha1）。
- 修复版本比较错误将预发布版本（如 1.3.3-alpha）判定为低于正式版的问题。（@wangxufeng）

感谢 @surenwuyuwuqiu 和 @wangxufeng 对本版本的贡献。

## v1.3.2

### What's Changed

**New Features**
- Middle-click paste in terminal. Middle-click pastes clipboard content by default, configurable in settings.

**Improvements**
- Clearable search input on start tab. Built-in clear button appears when the search field has content.
- SSH tunnel moved to basic section in the connection dialog, no longer hidden under advanced settings.
- UI monospace font stack trimmed to system pre-installed fonts only.
- Font scanning now includes per-user install locations on Windows and reads macOS system font directories directly, fixing missing user-installed fonts in the picker. (@surenwuyuwuqiu)
- Connection dialog category icons reduced from 28px to 20px for a more refined look.
- All toast notifications now unified with close button, 5s auto-dismiss, and lowered position to avoid overlapping the tab bar.
- Suggestion delete button only appears on mouse hover, no longer on keyboard selection (prevents accidental deletion).

**Bug Fixes**
- Fixed terminal not resizing when window is maximized or dragged. SessionResize is now always called after container resize.
- Fixed SFTP bookmark dropdown overflow on right pane. Delete button is no longer clipped by the viewport edge.
- Fixed encoding detection and GBK save in built-in file editor. UTF-8 Chinese files are now correctly detected, and switching encoding properly re-decodes content. GBK encoding delegated to Go backend.
- Fixed horizontal scrollbar appearing on the start tab when the window is narrow. Content width calculation now matches the actual CSS padding.
- Removed drag-out-to-desktop download feature to eliminate confusing forbidden cursor icon.

Thanks to @surenwuyuwuqiu for their contribution to this release.

### 更新内容

**新功能**
- 终端中键粘贴。中键点击终端时粘贴剪贴板内容，默认启用，可在设置中配置。

**改进**
- 起始页搜索框支持一键清除。输入内容后末尾自动显示清除按钮。
- SSH 隧道移至连接对话框的基础配置区域，不再隐藏在高级设置中。
- 界面等宽字体栈精简为系统预装字体（macOS 用 SF Mono，Windows 用 Consolas）。
- 字体扫描现在包含 Windows 用户级安装目录，macOS 直接读取系统字体目录，修复用户安装字体无法显示在字体选择器中的问题。（@surenwuyuwuqiu）
- 连接对话框中分类图标从 28px 缩小到 20px，更克制。
- 所有提示消息统一有关闭按钮、5 秒自动消失、位置下移不再遮挡标签栏。
- 智能提示框中的删除按钮仅在鼠标悬停时显示，键盘选中时不再出现，防止误删。

**Bug 修复**
- 修复最大化或拖拽窗口时终端和 TUI 应用不跟随调整尺寸的问题。
- 修复 SFTP 右侧窗口收藏栏删除按钮被视口边缘裁剪的问题。
- 修复内置编辑器 UTF-8 中文文件误识别为 GBK、手动切换编码无效、GBK 保存实际为 UTF-8 的问题。
- 修复窗口缩窄时起始页出现横向滚动条的问题。
- 移除拖动远程文件到桌面自动下载的功能，消除禁止光标歧义。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.3.1

### What's Changed

**New Features**
- Hover "..." button on sidebar connections and start tab cards, clicking opens the context menu.
- Windows hidden file detection for both local files and remote SFTP Windows servers.

**Improvements**
- Unified change-group dialog: start tab cards now reuse the sidebar's full dialog with new-group support.
- Removed redundant Refresh and Upload items from SFTP context menus (available in toolbar).
- SFTP transfer progress bar: fixed row height to prevent jitter, unified icons and font sizes.
- Shortened "Duplicate Session" to "Duplicate" in context menus.
- Replaced all non-lucide icons (raw SVGs, emoji, text symbols) with lucide icon components.

**Bug Fixes**
- Fixed start tab card edit not persisting.

### 更新内容

**新功能**
- 侧边栏连接项和起始页卡片新增悬停"..."按钮，点击弹出右键菜单。
- 文件列表支持 Windows 隐藏文件检测，本地和远程 Windows SFTP 服务端均可识别。

**改进**
- 统一修改分组对话框：起始页卡片复用侧边栏的完整分组对话框。
- 去掉 SFTP 右键菜单中与工具栏重复的刷新和上传项。
- SFTP 传输进度条样式优化：固定行高防抖，统一图标与字号风格。
- 右侧菜单"复制会话"英文缩写为 Duplicate。
- 所有非 lucide 图标统一替换为 lucide 图标组件。

**Bug 修复**
- 修复起始页卡片编辑保存不生效的问题。

## v1.3.0

- **new** Redesigned start page (new tab) with quick connect parsing that auto-detects host, port, and username from input strings.
- **new** Redesigned new connection dialog with left sidebar category navigation + icon sub-type grid for a more intuitive experience.
- **new** Custom terminal color themes with .itermcolors import/export support. (@surenwuyuwuqiu)
- **new** Terminal content export to text file via right-click menu, tab menu, or panel menu.
- **new** Terminal search via Ctrl+F / Cmd+F with prev/next navigation.
- **new** SMB share browsing: leave share name empty to list all available shares, mount on first navigation. Breadcrumb organized as /share/path.
- **improve** Unified connection type icons across all UI components (connection form, sidebar, start page, tabs) with dedicated S3/SMB/WebDAV icons.
- **improve** S3 breadcrumb now includes bucket name, organized as /bucket/path.
- **improve** Connections sidebar and AI sidebar default to closed to prevent startup flash; only the start page is shown initially.
- **improve** Unified header button spacing and window control button sizing for a more compact look.
- **improve** Added macOS-style Option/Cmd + arrow keys for cursor word/line jumping (Option+Arrow = word jump, Cmd+Arrow = line start/end).
- **improve** Ctrl+Shift+C copies terminal selection to clipboard.
- **improve** Close confirmation dialog when active connection tabs exist, preventing accidental quit.
- **improve** Removed "Local"/"Remote" text labels from SFTP dual-pane breadcrumb for a cleaner look.
- **improve** Shortened SSH keepalive interval for better connection stability. (@surenwuyuwuqiu)
- **improve** Removed "Open Settings" keyboard shortcut binding to avoid conflict with terminal copy. Shortcut settings page reordered by usage frequency.
- **bugfix** Fixed Git Bash Chinese character display (mojibake). (@surenwuyuwuqiu)
- **bugfix** Fixed terminal rendering not filling the container after duplicating a session and resizing the window.
- **bugfix** Fixed mouse scroll wheel misbehavior after reactivating a background tab.
- **bugfix** Fixed extra newlines when pasting text into vim due to CRLF line endings.

Thanks to @surenwuyuwuqiu for their contribution to this release.

---

- **new** 重新设计起始页（新标签页），新增快速连接解析（quick connect），支持从输入字符串自动识别主机、端口、用户名。
- **new** 重新设计新建连接对话框，采用左侧分类导航 + 图标子类型网格布局，更直观易用。
- **new** 新增自定义终端颜色主题功能，支持导入/导出 .itermcolors 格式文件。（@surenwuyuwuqiu）
- **new** 终端支持导出内容到文本文件，可通过右键菜单、标签页菜单或面板菜单触发。
- **new** 新增终端搜索快捷键 Ctrl+F / Cmd+F，支持前后导航。
- **new** SMB 支持空共享名浏览所有可用共享，进入共享后才挂载，面包屑按 /共享名/路径 组织。
- **improve** 统一所有 UI 组件中的连接类型图标（连接表单、侧边栏、起始页、标签页），新增 S3/SMB/WebDAV 专属图标。
- **improve** S3 面包屑按 /存储桶名/路径 组织，路径中包含存储桶名称。
- **improve** 连接边栏和 AI 边栏默认关闭，避免启动时闪烁，首次启动仅显示起始页。
- **improve** 统一标题栏按钮间距和窗口控制按钮尺寸，视觉更紧凑。
- **improve** 新增 macOS 风格 Option/Cmd + 方向键光标跳转（Option+方向键按词跳转，Cmd+方向键跳行首行尾）。
- **improve** Ctrl+Shift+C 可复制终端选中文本。
- **improve** 关闭应用时，如果存在活动连接标签页，弹出确认对话框防止误关闭。
- **improve** SFTP 双栏窗口移除左上角"本地"/"远程"文字标签，面包屑更简洁。
- **improve** 缩短 SSH 保活间隔，提升连接稳定性。（@surenwuyuwuqiu）
- **improve** 移除"打开设置"快捷键绑定，避免与终端复制快捷键冲突。快捷键设置页按使用频率重新排序。
- **bugfix** 修复 Git Bash 下中文显示乱码的问题。（@surenwuyuwuqiu）
- **bugfix** 修复复制会话后拖动窗口调整尺寸时终端渲染无法满屏的问题。
- **bugfix** 修复重新激活后台标签页后鼠标滚轮异常的问题。
- **bugfix** 修复粘贴文本到 vim 时多余回车导致空行的问题。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.2.2

- **new** Added SMB file transfer support with remote file browsing, upload, and download.
- **new** Added WebDAV file transfer support for connecting to WebDAV servers.
- **new** Added S3 object storage support, compatible with Amazon S3 API. Browse buckets, list objects, upload and download files.
- **new** Added start tab page showing recent connections, groups, and all connections as cards with search and type filtering.
- **improve** Unified color system and button styles across all components for visual consistency. (@surenwuyuwuqiu)
- **improve** AI detects command completion via prompt reappearance, reducing unnecessary wait time. (@surenwuyuwuqiu)
- **improve** Terminal text highlighting now uses ANSI theme colors to adapt to different terminal themes.
- **improve** Optimized uniTerm light terminal theme colors.
- **improve** Simplified Chinese locale now detects version updates from Gitee Release.
- **bugfix** Fixed serial port local echo causing double keystrokes.
- **bugfix** Fixed background tab terminal output being truncated on buffer trim. (@surenwuyuwuqiu)
- **bugfix** Fixed macOS local terminal not starting as a login shell. (@surenwuyuwuqiu)
- **bugfix** Fixed Ctrl+Shift+N opening connection form instead of start tab.
- **bugfix** Fixed settings page column width being squeezed.

Thanks to @surenwuyuwuqiu for contributions to this release.

---

- **new** 新增 SMB 文件传输支持，可浏览、上传、下载远程共享文件。
- **new** 新增 WebDAV 文件传输支持，可连接 WebDAV 服务器进行文件管理。
- **new** 新增 S3 对象存储支持，兼容 Amazon S3 API，支持列出存储桶、文件浏览和传输。
- **new** 新增起始页（新标签页），展示最近连接、分组和全部连接卡片，支持搜索和类型筛选。
- **improve** 统一组件颜色系统和按钮样式，优化视觉一致性。（@surenwuyuwuqiu）
- **improve** AI 通过 prompt 重新出现检测命令完成，减少不必要的等待时间。（@surenwuyuwuqiu）
- **improve** 终端文本高亮改用 ANSI 主题颜色，适配不同终端主题。
- **improve** 优化 uniTerm 浅色终端主题配色。
- **improve** 简体中文环境下从 Gitee Release 检测版本更新。
- **bugfix** 修复串口本地回显导致按键重复的问题。
- **bugfix** 修复后台标签页终端输出被截断的问题。（@surenwuyuwuqiu）
- **bugfix** 修复 macOS 本地终端未以登录 shell 方式启动的问题。（@surenwuyuwuqiu）
- **bugfix** 修复新建标签页快捷键 Ctrl+Shift+N 打开连接表单而非起始页的问题。
- **bugfix** 修复设置页列宽被挤压的问题。

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.2.1

- **new** Added Redis connection support with visual key browser and value editor.
- **new** SFTP bookmark button for quick directory navigation.
- **new** Notification dot on inactive tabs when terminal receives new output.
- **improve** Added local terminal type in connection form with post-login script support.
- **improve** Added serial port connection type with custom baud rate input.
- **improve** AI `send_terminal_key` tool now supports `send_enter` parameter (default true) for auto-Enter after input.
- **improve** Enhanced AI conversation markdown rendering with broader syntax support and improved dark theme readability.
- **improve** Improved connection retry flow with auto-focus and pre-filled credential dialog.
- **bugfix** Fixed macOS title bar button misalignment. (@surenwuyuwuqiu)
- **bugfix** Fixed SFTP fallback to SFTP-level copy when remote cp command fails.

- **bugfix** Fixed garbled CJK input and broken editing keys (Backspace, etc.) in zsh on macOS local terminal. (@surenwuyuwuqiu)

Thanks to @surenwuyuwuqiu for contributions to this release.

---

- **new** 新增 Redis 连接支持，提供可视化键浏览器和值编辑器。
- **new** SFTP 新增书签按钮，可快速跳转到常用目录。
- **new** 非活跃标签页在终端有新输出时显示通知圆点。
- **improve** 连接表单新增本地终端类型，支持登录后执行脚本。
- **improve** 连接表单新增串口连接类型，支持自定义波特率。
- **improve** AI `send_terminal_key` 工具新增 `send_enter` 参数（默认 true），交互式回复自动追加回车。
- **improve** 增强 AI 对话 markdown 渲染，支持更多语法并优化深色主题下的可读性。
- **improve** 优化连接失败后的重试流程，凭据对话框自动聚焦并预填上次输入。
- **bugfix** 修复 macOS 标题栏按钮位置偏移的问题。（@surenwuyuwuqiu）
- **bugfix** 修复 Windows SFTP 服务器上远程复制文件失败的问题（cp 不可用时自动通过 SFTP 协议传输）。
- **bugfix** 修复 macOS 本地终端 zsh 中文输入乱码、退格键等编辑按键失效的问题。（@surenwuyuwuqiu）

感谢 @surenwuyuwuqiu 对本版本的贡献。

## v1.2.0

- **new** Added Oracle database support. (@yuwei5380)
- **new** Added SQL Server database support.
- **new** Moved database CRUD SQL logic to backend for unified management.
- **new** Database page now includes query/object tabs with view-aware actions.
- **new** Added Expect/Send auto-login support for jumphost and similar scenarios. (@yuwei5380)
- **new** Added file picker button for SSH key path input. (@yuwei5380)
- **new** SSH connections now support terminal character encoding configuration.
- **new** Added collapsible advanced section to connection form.
- **new** Added app config section to sidebar personalization panel.
- **new** Added 4px padding around terminal content.
- **improve** Dark loading mask for monitor refresh to avoid white flash.
- **bugfix** Fixed SSH key authentication failure. (@yuwei5380)
- **bugfix** Fixed OOM when processing large files in sz/rz. (@yuwei5380)
- **bugfix** Fixed macOS header double-click maximize not working. (@surenwuyuwuqiu)
- **bugfix** Fixed command history not recording with zsh/oh-my-zsh Unicode prompt glyphs.
- **bugfix** Fixed SFTP breadcrumb freezing on overflow boundary.
- **bugfix** Fixed database grid not refreshing immediately after inline edit/delete.
- **bugfix** Fixed missing key file picker logic in connection form.

Thanks to @yuwei5380 and @surenwuyuwuqiu for their contributions to this release.

---

- **new** 新增 Oracle 数据库支持。（@yuwei5380）
- **new** 新增 SQL Server 数据库支持。
- **new** 数据库 CRUD SQL 逻辑移至后端统一管理。
- **new** 数据库页面新增查询/对象标签页，支持视图感知操作。
- **new** 支持 Expect/Send 自动登录，可用于跳板机等场景。（@yuwei5380）
- **new** SSH 密钥路径新增文件选择器按钮。（@yuwei5380）
- **new** SSH 连接支持终端字符编码设置。
- **new** 连接表单新增可折叠高级设置区域。
- **new** 侧边栏个性化面板新增应用配置区域。
- **new** 终端内容四周增加 4px 内边距。
- **improve** 监控刷新使用深色加载遮罩，避免白色闪烁。
- **bugfix** 修复 SSH 密钥认证失败的问题。（@yuwei5380）
- **bugfix** 修复 sz/rz 处理大文件时内存溢出问题。（@yuwei5380）
- **bugfix** 修复 macOS 标题栏双击最大化不生效的问题。（@surenwuyuwuqiu）
- **bugfix** 修复 zsh/oh-my-zsh 等 Unicode 提示符下命令历史无法记录的问题。
- **bugfix** 修复 SFTP 面包屑在溢出边界时冻结的问题。
- **bugfix** 修复数据库表格行内编辑/删除后网格未立即刷新的问题。
- **bugfix** 修复连接表单密钥文件选择器逻辑缺失的问题。

感谢 @yuwei5380、@surenwuyuwuqiu 对本版本的贡献。

## v1.1.2

- **new** Added 22 terminal themes with a sidebar personalization panel for one-click theme switching.
- **new** SFTP built-in text editor with encoding & line ending configuration.
- **new** SFTP new file/folder creation and copy/paste/cut.
- **new** Prompt for credentials when connecting without saved username or password.
- **improve** SFTP chmod dialog now supports octal permission input.
- **improve** Connection-type icons in sidebar session list.
- **improve** Sidebar visibility now persisted to local state file across restarts.
- **bugfix** Fixed history panel not updating in real time.
- **bugfix** Fixed missing overwrite confirmation when uploading files with same name in SFTP.
- **bugfix** Fixed paste event not dispatched to all panels in broadcast mode due to SFTP overlay interference.
- **bugfix** Fixed `__AI_KEY_` marker residue in AI output.
- **bugfix** Fixed AI command heredoc syntax compatibility.
- **bugfix** Fixed SFTP drive dropdown closing on mousedown before click event fires.

---

- **new** 新增 22 款终端主题，侧边栏个性化面板支持一键切换主题。
- **new** SFTP 新增内置文本编辑器功能，支持编码和换行符设置。
- **new** SFTP 新增新建文件/文件夹、复制/粘贴/剪切功能。
- **new** 连接未保存用户名或密码时，连接前弹窗提示输入凭据。
- **improve** SFTP chmod 对话框新增八进制权限输入。
- **improve** 侧边栏连接列表新增连接类型图标。
- **improve** 侧边栏展开/收起状态持久化到本地文件，重启后保持。
- **bugfix** 修复历史命令面板未实时更新的问题。
- **bugfix** 修复 SFTP 上传同名文件时无覆盖确认提示。
- **bugfix** 修复广播模式下 SFTP 遮罩层干扰 paste 事件分发。
- **bugfix** 修复 AI 输出中残留 `__AI_KEY_` 标记。
- **bugfix** 修复 AI 命令 heredoc 语法兼容性问题。
- **bugfix** 修复 SFTP 驱动器下拉菜单因 mousedown 提前关闭、click 无法触发。

## v1.1.1

- **new** Prompt for SSH password directly in the terminal when no password is saved.
- **new** Detect system monospace fonts with live preview in the settings font selector.
- **new** AI model config supports API protocol switching, connection test, and custom User-Agent.
- **new** Customizable keyboard shortcuts for common actions.
- **bugfix** Fixed Linux multi-screen maximize using wrong screen dimensions.
- **bugfix** Fixed empty AI API message role causing request errors.
- **bugfix** Fixed brief UI freeze during maximize/restore on Windows 11.
- **bugfix** Fixed WSL local terminal console window flashing on startup.

---

- **new** SSH 密码未保存时，终端内直接弹出密码输入提示。
- **new** 系统等宽字体检测与预览，设置中字体选择器展示可用 monospace 字体。
- **new** AI 模型配置支持 API 协议切换、连接测试与自定义 User-Agent。
- **new** 自定义键盘快捷键，支持为常用操作绑定快捷键。
- **bugfix** 修复 Linux 多屏环境下窗口最大化使用错误屏幕尺寸。
- **bugfix** 修复 AI API 消息角色为空时导致请求报错。
- **bugfix** 修复 Windows 11 最大化/还原时 UI 短暂卡顿。
- **bugfix** 修复 WSL 本地终端启动时控制台窗口闪烁。

## v1.1.0

- **new** Added serial port terminal connection. Supports scanning available serial ports and connecting with configurable baud rate, data bits, stop bits, and parity.
- **new** Added WSL support to local terminal. The `New Local Terminal` sidebar menu now scans and lists installed WSL distributions (e.g. `WSL - Ubuntu`), which can be opened with one click.
- **new** Added Windows portable zip artifact to the build workflow.

---

- **new** 新增串口终端连接。支持扫描可用串口，配置波特率、数据位、停止位、校验位后连接。
- **new** 本地终端新增 WSL 支持。侧边栏 `New Local Terminal` 菜单自动扫描并列出已安装的 WSL 发行版（如 `WSL - Ubuntu`），点击即可打开对应 Linux shell。
- **new** 构建工作流新增 Windows 便携版 zip 产物。

## v1.0.1

- **new** Quick Commands management. Sidebar panel with drag-drop groups, search filtering, keyboard navigation (arrow keys + Enter), edit dialog, and full 9-language i18n support.
- **new** History Panel. New sidebar tab displaying all terminal command history with search and copy support.
- **new** Quick command suggestions. Smart completion popup now includes matching quick command suggestions in real time.
- **new** "Upload File (rz -be)" right-click menu option in SSH panels to trigger Zmodem upload.
- **improve** New-connection button moved to sidebar top; quick command toolbar menu unified with sidebar styling.
- **improve** Terminal command history is now always recorded regardless of the smart completion setting.
- **bugfix** Fixed right-click paste in broadcast mode only applying to the current panel instead of all panels in the workspace.
- **bugfix** Fixed double input after SSH reconnect via generation counter guard.
- **bugfix** Fixed history panel tooltip, button layout, and text brightness issues.
- **bugfix** Fixed session data replay on terminal reuse during panel merge/split.
- **bugfix** Fixed text highlight clearing terminal background colors.
- **bugfix** Fixed escape-sequence guard failing to skip TUI lines, causing highlight interference in vim/k9s.
- **bugfix** Fixed SSH keepalive switching to global request to prevent auto-disconnect on some servers.

---

- **new** 快捷命令管理。侧边栏新增快捷命令面板，支持拖拽排序分组、搜索过滤、键盘导航（上下选择、Enter 执行）、编辑弹窗，覆盖 9 种语言国际化。
- **new** 历史命令面板。侧边栏新增历史标签页，展示所有终端命令历史记录，支持搜索和复制。
- **new** 快捷命令建议。智能补全弹出框中融合快捷命令建议，输入时实时匹配。
- **new** SSH 面板右键菜单新增"上传文件 (rz -be)"选项，可直接触发 Zmodem 上传。
- **improve** 新建连接按钮移至侧边栏顶部，快捷命令工具栏菜单统一风格。
- **improve** 无论是否开启智能补全，始终记录终端命令历史。
- **bugfix** 修复广播模式下右键粘贴文本仅当前 panel 生效，未分发到工作区所有面板。
- **bugfix** 修复 SSH 重连后按键重复输入两次（generation counter 守卫）。
- **bugfix** 修复历史面板提示框、按钮布局、文字亮度问题。
- **bugfix** 修复面板合并/分离时终端复用导致历史数据重复回放。
- **bugfix** 修复文本高亮清除终端背景色的问题。
- **bugfix** 修复 escape 序列守卫未正确跳过 TUI 行导致高亮干扰 vim/k9s 等应用。
- **bugfix** 修复 SSH keepalive 改用 global request 防止部分服务器自动断开。

## v2026.06.17

- **bugfix** Fixed URL highlight causing all subsequent text to be underlined in the terminal.
- **bugfix** Fixed terminal canvas blocking window edge resize by adding 3px padding to the tab area.
- **improve** Tightened dialog padding and form item spacing; form labels now auto-expand row height when text wraps.
- **improve** All dialogs are now draggable by default.
- **improve** Unified select dropdown font size to 12px.
- **improve** Update notification link now opens in the system browser instead of the built-in WebView2 window.
- **improve** New connection button now shows "Save & Connect" consistently, same as the edit dialog.

---

- **bugfix** 修复终端中 URL 高亮后后续文字全部带下划线。
- **bugfix** 修复终端 canvas 贴边导致窗口边缘无法拖动调整大小。
- **improve** 统一收紧弹出框内边距和表单项间距，表单标签换行时行高自动撑开。
- **improve** 所有弹出框全局启用拖动。
- **improve** 下拉框字体统一为 12px。
- **improve** 检测到更新包时点击链接改用系统浏览器打开。
- **improve** 新建连接主按钮文案统一为"保存并连接"。

## v2026.06.16-alpha

- **bugfix** Fixed local terminal tab close causing entire app to crash. Root cause: multiple goroutines concurrently calling `ConPTY.Close()` → double `ClosePseudoConsole` on Windows triggers OS-level access violation unrecoverable by Go's `recover()`. Fix: wrap entire `Disconnect()` body in `sync.Once`.
- **bugfix** Fixed terminal output loss when switching tabs. Data arriving during KeepAlive deactivation was buffered in sessionStore but never replayed on reactivation. Track written chunk count and replay missed chunks in `onActivated`.
- **bugfix** Fixed Enter key not triggering reconnect after SSH disconnect occurred while tab was in background. `session:status` event was dropped by `isActive` guard, so `retryOnEnter` never got set. Sync from `sessionStore.getStatus()` on reactivation.
- **bugfix** Fixed suggestion popup stuck at top-left after SSH reconnect. `terminalInput` held stale reference to disposed terminal, cursor tracking returned `{0,0}`. Recreate `terminalInput` when session ID changes.
- **bugfix** Fixed terminal content being cleared after reconnect. Release + acquire created a new xterm.js instance. Use `transferTerminal()` to move the existing terminal entry to the new session ID, preserving scrollback.
- **bugfix** Fixed pressing Ctrl+G / Shift+G / PageDown in vim leaving rendering residue. `\x1b[2J` replacement with scrollClear was also applied in alternate screen buffer, corrupting vim's screen state. Now only applies to main buffer.
- **bugfix** Fixed sidebar resize handle blocking the connection list scrollbar. Moved activation area outside the sidebar edge via negative `right` offset.
- **improve** Text highlighting overhaul:
  - Highlight only resets foreground color (`\x1b[39m`) instead of all SGR (`\x1b[0m`), preserving vim's reverse video selection
  - Lines with display attributes (reverse video, bold, etc.) skip highlighting to avoid color mixup
  - File path regex now matches directories and files without extensions, anchored by `(^|\s)` to avoid false positives inside words
  - Added datetime formats: `HH:MM` without seconds, ISO 8601 `Z` suffix, syslog `Mon DD HH:MM:SS`, weekday+year `Wed Jan 21 HH:MM:SS YYYY`
  - Color palette extracted into named constants; number color 145→152, brace color 147→223 for better contrast on vim reverse video
  - Local terminal sessions no longer apply text highlighting
  - Suggestion popup no longer triggers on arrow key navigation when closed
- **refactor** Merged `WorkspaceTabItem` into `TabItem`, eliminating ~300 lines of duplicate code. All tab types (terminal, workspace, SFTP, RDP, VNC, etc.) handled by a single component.
- **improve** Tab close buttons replaced plain `×` text with Lucide `X` SVG icon, adjusted sizing and spacing to prevent text shift when switching tabs. AI lock button always visible. SFTP and Monitor context menu items hidden for local terminal panels.
- **bugfix** Fixed AI lock state being cleared when panel detached from workspace tab.

---

- **bugfix** 修复关闭本地终端 tab 导致程序崩溃。根因：多个 goroutine 并发调用 `ConPTY.Close()`，Windows 上 `ClosePseudoConsole` 被重复调用触发 OS 级访问违规，Go 的 `recover()` 无法捕获。修复：用 `sync.Once` 包裹完整 `Disconnect()` 体。
- **bugfix** 修复切换 tab 后后台终端输出丢失。KeepAlive 停用期间数据被 sessionStore 缓存，但切回时从未回放。追踪已写入 chunk 数，`onActivated` 中补写缺失数据。
- **bugfix** 修复后台 tab 中 SSH 断开后按 Enter 无法重连。`session:status` 事件被 `isActive` 守卫丢弃，`retryOnEnter` 从未设置。切回时从 `sessionStore.getStatus()` 同步。
- **bugfix** 修复重连后智能提示框定位在左上角。`terminalInput` 持有已销毁终端引用，光标追踪返回 `{0,0}`。sessionId 变更时重建 `terminalInput`。
- **bugfix** 修复重连后终端内容被清空。release + acquire 创建了全新 xterm.js 实例。改用 `transferTerminal()` 迁移终端条目到新 sessionId，保留 scrollback。
- **bugfix** 修复 vim 中按 Ctrl+G / Shift+G / PageDown 出现渲染残留。`\x1b[2J` 替换为 scrollClear 在交替屏幕中也会执行，破坏 vim 的屏幕状态。现在仅主屏生效。
- **bugfix** 修复 sidebar 分隔栏挡住连接列表滚动条。激活区域通过负 `right` 偏移移到 sidebar 外部。
- **improve** 文本高亮全面优化：
  - 高亮结束只用 `\x1b[39m` 重置前景色，不取消 vim 的反转视频
  - 有显示属性（反转视频、加粗等）的行跳过，避免颜色混叠
  - 文件路径正则匹配支持无扩展名文件和目录，用 `(^|\s)` 锚定避免词内误匹配
  - 新增日期格式：`HH:MM`（无秒）、ISO 8601 `Z` 后缀、syslog `Mon DD HH:MM:SS`、`Wed Jan 21 HH:MM:SS YYYY`
  - 颜色提取为命名常量；数字色 145→152，符号色 147→223，vim 反选下可辨
  - 本地终端不启用文本高亮
  - 智能提示框关闭时方向键不再触发弹出
- **refactor** 合并 `WorkspaceTabItem` 到 `TabItem`，消除约 300 行重复代码。所有 tab 类型统一由单一组件处理。
- **improve** Tab 关闭按钮替换为 Lucide `X` SVG 图标，调整尺寸和间距避免切 tab 时文字偏移。AI 锁按钮始终可见。本地终端隐藏 SFTP/监控菜单项。
- **bugfix** 修复面板从 workspace 拖出后 AI 锁定状态被清除。

## v2026.06.14-alpha

- **new** SSH tunnel (local port forwarding). Any connection can use an existing SSH connection as a jump host. Auto-assigns local port, tunnels TCP through SSH. VNC ports automatically adjusted for libvirt display numbers.
- **new** FTP/FTPS file transfer. New File Transfer category with FTP and FTPS (explicit TLS), passive/active mode, configurable character encoding. Reuses the SFTP two-pane file manager UI; Go backend uses shared fileTransferSession interface.
- **new** SFTP max concurrent transfers (per SSH connection, default 5). Semaphore-based concurrency control prevents bandwidth saturation and server MaxSessions limits.
- **new** Connection form now has four categories: Terminal / File Transfer / Remote Desktop / Database. SSH labeled as SSH (SFTP), appears under both Terminal and File Transfer.
- **improve** All notifications now have a close button and auto-dismiss after 5 seconds. Unified via `services/message.ts` wrapper.
- **improve** KeepAlive cache extended to all tab components (Settings/SFTP/RDP/VNC/SPICE). Switching tabs no longer rebuilds components.
- **improve** Fonts switched to system native font stack, removing Google Fonts CDN dependency. UI uses system interface fonts, monospace uses system-provided fixed-width fonts. CJK fallback covers Windows/macOS/Linux.
- **bugfix** Fixed KeepAlive-cached SFTP instances picking up global drag events, causing files to upload to the wrong connection. Document event listeners now managed via onActivated/onDeactivated.
- **bugfix** Fixed stale edit data leaking into the quick-new-connection form.
- **bugfix** Fixed duplicate task IDs from identical nanosecond timestamps in concurrent transfers causing jumbled progress bars. Switched to atomic counter.
- **bugfix** Fixed port input min=1 preventing value 0.
- **bugfix** Fixed 4px body padding preventing the titlebar from being flush with the window edge.
- **bugfix** Fixed 4px gap between local terminal submenu and its trigger causing submenu to close on mouse enter.
- **bugfix** Fixed AI confirmation level dropdown button missing ChevronDown icon import.
- **bugfix** Fixed default port not updating when switching between remote desktop types (e.g. RDP 3389 → VNC still showing 3389).

---

- **new** SSH 隧道（本地端口转发）。任何连接可选择已有 SSH 连接作为跳板，自动分配本地端口，通过隧道访问目标。VNC 自动处理 libvirt 端口偏移。
- **new** FTP/FTPS 文件传输。新增文件传输大类，支持 FTP 和 FTPS（显式 TLS），被动/主动模式、字符编码可选。复用 SFTP 两栏文件管理器 UI，Go 后端统一 fileTransferSession 接口。
- **new** SFTP 最大并发传输数配置（SSH 连接设置，默认 5），semaphore 控制同时传输文件数，避免带宽打满或触发服务器 MaxSessions 限制。
- **new** 连接表单分类调整为四类：终端 / 文件传输 / 远程桌面 / 数据库。SSH 标注为 SSH (SFTP)，同时出现在终端和文件传输下。
- **improve** 所有通知消息增加关闭按钮，5 秒自动消失。统一 `services/message.ts` 包装器。
- **improve** KeepAlive 缓存扩展至全部标签页组件（Settings/SFTP/RDP/VNC/SPICE），切标签不再重建组件。
- **improve** 字体改为系统原生字体栈，移除 Google Fonts CDN 依赖。UI 用系统界面字体，等宽用系统自带等宽字体，中文 fallback 覆盖 Windows/macOS/Linux。
- **bugfix** 修复 KeepAlive 下 SFTP 缓存实例监听全局拖拽事件导致文件误上传至其他连接的 bug。改由 onActivated/onDeactivated 管理事件。
- **bugfix** 修复快速新建连接时上次编辑的残留数据泄漏到新表单的问题。
- **bugfix** 修复并发传输时同一纳秒时间戳导致任务 ID 重复、进度条混乱的 bug。改用原子计数器。
- **bugfix** 修复连接端口 min 限制为 1 导致无法输入 0 的问题。
- **bugfix** 修复 body 4px padding 导致标题栏不贴顶的问题。
- **bugfix** 修复"本地终端"子菜单与主菜单之间 4px 缝隙导致鼠标划入子菜单消失的问题。
- **bugfix** 修复 AI 确认级别下拉按钮缺少 ChevronDown 图标 import 的问题。
- **bugfix** 修复切换连接类型时远程桌面类型间不更新默认端口的问题（如 RDP 3389 切到 VNC 仍为 3389）。

## v2026.06.13-alpha.1

- **new** Update checker. Manual check + auto-check for GitHub Releases. Settings About page shows current version, notification on new release with view details link.
- **fix** macOS rounded corners now use native Wails TitleBarHiddenInset, removing CSS border-radius workaround that caused a visible square frame.
- **fix** macOS traffic lights now use system native controls instead of custom simulated buttons.

---

- **new** 更新检查。设置关于页支持手动检查 + 后台自动检查 GitHub Releases，发现新版本弹出通知并可直接跳转查看详情。
- **fix** macOS 无边框窗口圆角改为原生实现。使用 Wails TitleBarHiddenInset，移除 CSS 圆角 hack，修复之前方形外框问题。
- **fix** macOS 窗口控制按钮改用系统原生红绿灯，移除自定义模拟按钮。

## v2026.06.13-alpha

- **new** AI terminal toolchain. 5 new tools: start_command (fire-and-forget), capture_terminal (read screen), collect_output (passive wait), send_terminal_key (interactive input), interrupt_command (cancel). execute_command gains configurable timeout and output truncation.
- **new** AI SSE streaming. Go backend proxies Anthropic SSE events, frontend renders tokens in real time via ai:token.
- **new** AI context management. Layered system prompt (static cached + dynamic injected), token-aware context window management for improved prompt cache hit rate.
- **new** AI IN boxes show tool type names with i18n and parsed parameters per tool type. Headers display tool name with timeout `[xxs]`, body shows command/params instead of raw JSON.
- **new** AI sidebar search. Highlight matches, navigate matches (Enter / Shift+Enter), match count, auto-scroll to active match.
- **bugfix** Fixed text search menu opening search bar in all terminal windows simultaneously. Event now targets the current panel.
- **improve** Rewritten AI system prompt with timeout guidelines, decision tree, interactive prompt handling, and clear-screen prohibition.

---

- **new** AI 终端工具链。新增 5 个工具：start_command（启动后台命令）、capture_terminal（读取终端屏幕）、collect_output（被动等待输出）、send_terminal_key（发送终端输入）、interrupt_command（中断命令）。execute_command 新增超时和输出截断参数，AI 可自主控制等待时长。
- **new** AI SSE 流式响应。Go 后端转发 Anthropic SSE 事件，前端 ai:token 实时渲染 token 输出。
- **new** AI 上下文管理优化。系统提示词分层（静态缓存 + 动态注入），token 感知的上下文窗口管理，提升 prompt cache 命中率。
- **new** AI 对话 IN 框按工具类型解析展示。头部显示工具中文名和超时 `[xxs]`，体部按类型展示命令/参数，不再显示原始 JSON。
- **new** AI 侧边栏搜索。支持高亮匹配文本、上下导航（Enter / Shift+Enter）、匹配计数，自动滚动到当前匹配。
- **bugfix** 修复文本搜索菜单在所有终端窗口同时弹出搜索框的问题。事件细化到当前面板。
- **improve** AI 系统提示词重写。增加超时指南、超时决策树、交互式提示处理说明，禁止清屏命令。

## v2026.06.12-alpha

- **new** Zmodem file transfer (rz/sz). Upload (including drag-and-drop onto terminal) and download files in SSH terminals via `rz -be` and `sz`, with real-time progress bars.
- **new** SSH panel header "..." dropdown menu and tab right-click menu now include Duplicate Session, Connect SFTP, Server Monitor, and Text Search.
- **improve** Refactored terminal instance management so xterm instances are reused across workspace panel and standalone tab drag-and-drop merge/detach, eliminating garbled text during transitions.
- **bugfix** Fixed double-click text selection not copying to clipboard. Replaced mousedown/mouseup tracking with xterm's native onSelectionChange event.

---

- **new** Zmodem 文件传输（rz/sz）。支持在 SSH 终端中使用 `rz -be` 上传（含直接拖拽文件到终端）、`sz` 下载文件，带实时进度条。
- **new** SSH 面板头部新增"..."菜单、标签右键菜单新增，包含复制会话、连接 SFTP、服务器监控、文本搜索。
- **improve** 重构终端实例管理，工作区面板和独立标签页拖拽合并/分离后不再重建 xterm 实例，消除切换过程中可能出现的乱码。
- **bugfix** 修复双击选中文字不复制到剪贴板。改用 xterm 原生 onSelectionChange 事件。

## v2026.06.10-alpha

- **new** AI model list sync. One-click fetch available models from the server in the model edit dialog, with autocomplete suggestions in the model input.
- **new** Sidebar search now supports filtering by connection type (Terminal / Remote Desktop / Database), combined with text search.
- **new** Multilingual support. Supports 9 languages (zh-CN, zh-TW, en, ja, ko, de, es, fr, ru) with real-time switching in settings.
- **improve** Simplified AI command markers (echo-only), removed self-check from system prompt, expanded run confirmation panel by default.
- **improve** Fixed AI confirm-write button to use primary color style.
- **improve** Window title simplified, showing only "uniTerm" without version number.
- **bugfix** Fixed terminal losing focus after paste, causing invisible cursor.
- **bugfix** Fixed CSI response sequences being echoed as garbage text by bash on tab switch.

---

- **new** AI 模型列表同步。在 AI 模型编辑弹窗中可一键从服务端拉取可用模型列表，模型输入框带下拉建议。
- **new** 侧边栏搜索支持按连接类型过滤（终端/远程桌面/数据库等），与文本搜索联合使用。
- **new** 多语言支持。支持简体中文、繁体中文、英文、日文、韩文、德文、西班牙文、法文、俄文 9 种语言，设置中切换实时生效。
- **improve** AI 命令标记简化，移除 `u='...'` 前缀仅保留 echo，移除自检提示词，运行确认面板默认展开。
- **improve** AI 写操作确认按钮配色修正为 primary 风格。
- **improve** 窗口标题去版本号，仅显示 uniTerm。
- **bugfix** 修复粘贴后终端失焦导致光标不显示的问题。
- **bugfix** 修复切换标签时 CSI 响应序列被 bash 回显为乱码的问题。

## v2026.06.08-alpha

- **new** SPICE remote desktop protocol support.
- **new** Panel duplicate, rename, drag image preview, and title synchronization.
- **new** Drag active terminal tab to adjacent tab with workspace merge.
- **improve** SSH keepalive changed from global request to session channel request (`keepalive@openssh.com`), matching OpenSSH `ServerAliveInterval` behavior; interval adjusted from 30s to 60s, max failures from 2 to 3.
- **improve** On Windows, prefer Git Bash over WSL and fix WSL bash argument passing.
- **improve** Suggestion popup position fixed in multi-panel workspace; SFTP scroll behavior improved.
- **bugfix** Fixed terminal size not updating after SSH reconnect. New sessions default to 80×24 PTY; now forces a `SessionResize` with the current terminal dimensions when reconnected, so apps like vim/k9s display at the correct size.
- **bugfix** Fixed `clear` command destroying scrollback history. Replaces ED2 (clear screen) with newline scrolling + home, pushing viewport content into scrollback before clearing.
- **bugfix** Fixed text highlighting disappearing after tab switch. Restored history now applies `highlight()` based on the current `highlightEnabled` setting.
- **bugfix** Fixed copy-on-select overwriting clipboard when switching panels or returning from another app. Copy now only triggers when the mouse selection actually started inside the same terminal.
- **bugfix** Fixed dropping panel/tab onto empty tab bar area not working in certain layouts.

---

- **new** SPICE 远程桌面协议支持。
- **new** 面板复制、重命名、拖拽图像预览、标题同步。
- **new** 将活动终端标签拖拽到相邻标签并合并工作区。
- **improve** SSH keepalive 从全局请求改为 session channel 请求（`keepalive@openssh.com`），对齐 OpenSSH `ServerAliveInterval` 行为；间隔从 30s 调整为 60s，最大失败次数从 2 调整为 3。
- **improve** Windows 上优先使用 Git Bash 而非 WSL，修复 WSL bash 参数传递问题。
- **improve** 多面板工作区中建议弹出框位置修复；SFTP 滚动行为优化。
- **bugfix** 修复终端重连后尺寸未更新。新 session 默认以 80×24 创建 PTY；现在重连后强制发送当前终端尺寸进行 `SessionResize`，vim/k9s 等全屏应用显示正确。
- **bugfix** 修复 `clear` 命令清除 scrollback 历史的问题。将 ED2（清屏）替换为换行滚动+归位，清屏前先将 viewport 内容推入 scrollback。
- **bugfix** 修复切换标签后文本高亮消失。恢复历史时根据当前 `highlightEnabled` 设置重新应用高亮。
- **bugfix** 修复选中复制在切换面板或从其他应用返回时误覆盖剪贴板。现在只有鼠标确实在本 terminal 内开始选择时才触发复制。
- **bugfix** 修复某些布局下将面板/标签拖放到空标签栏区域不生效的问题。

## v2026.06.02-alpha

- **new** Telnet and Mosh connection protocol support. Telnet provides IAC negotiation (binary mode, terminal type, window size); Mosh uses UDP-based SSP protocol for low-latency mobile connections.
- **new** Terminal text highlighting. Automatically highlights timestamps, IP addresses, URLs, file paths, keywords (ERROR/WARN/INFO), quoted strings, numbers, and punctuation in terminal output. Toggle in settings; lines containing ESC are skipped to avoid TUI interference.
- **new** xterm.js Unicode11 addon for correct emoji and wide character rendering (e.g. k9s dog icon).
- **improve** Merged tab bar into titlebar as a single row, saving ~40px vertical space. All buttons icon-only, new connection + local terminal merged into `+` dropdown, window controls styled consistently.
- **improve** New/Edit connection dialog restructured into two-level category selection (Terminal / Remote Desktop / Database) with radio-button toggle for protocol sub-type.
- **improve** Unified control sizing across the entire UI: 28px height, 12px font, consistent border-radius, background, and border colors for all controls (el-input, el-button, el-select, el-radio-button, el-switch, el-checkbox, etc.).
- **improve** Smart completion UX fixes: popup flips above/below intelligently to avoid covering input; mouse hover only activates after movement to prevent accidental selection; password (hidden) input is not saved to history and does not trigger suggestions.
- **improve** Terminal tabs restyled as buttons with accent border + background for active state, AI lock + active effects combined.
- **improve** AI sidebar defaults to new session on restart; empty sessions are not saved; max 15 sessions retained.
- **bugfix** Fixed terminal history capture logic. Scans visible buffer area (bottom to top) instead of relying on cursorY, which is unreliable after buffer scrolling.
- **bugfix** Fixed race condition where suggestion popup remained open after Enter (debounce timer cancellation + empty token check).
- **bugfix** Fixed WebView2 conflict causing input failure when opening multiple processes on Windows 11. UserDataFolder now uses a per-process PID-isolated path.
- **bugfix** Fixed garbled text on tab switch. xterm.js OSC color queries sent via onData were echoed back by the server as scrambled text; added OSC filtering to resolve.

---

- **new** Telnet 和 Mosh 连接协议支持。Telnet 提供 IAC 协商（二进制模式、终端类型、窗口大小）；Mosh 基于 UDP 的 SSP 协议实现低延迟移动连接。
- **new** 终端文本高亮。自动高亮终端输出中的时间戳、IP 地址、URL、文件路径、关键词（ERROR/WARN/INFO）、引号字符串、数字、括号等符号。设置中可开关，含 ESC 的行自动跳过避免干扰 TUI 应用。
- **new** xterm.js Unicode11 插件，正确渲染 emoji 等宽字符（如 k9s 小狗图标）。
- **improve** 标签栏与标题栏合并为一行，节省约 40px 垂直空间。按钮全部图标化，新建连接与本地终端合并为 `+` 下拉，窗口控制按钮风格统一。
- **improve** 新建/编辑连接界面重构为两级分类结构（终端 / 远程桌面 / 数据库），子级用 radio-button toggle 切换协议。
- **improve** 全界面控件风格统一：高 28px、字号 12px、圆角统一、边框和底色一致，涵盖 el-input、el-button、el-select、el-radio-button、el-switch、el-checkbox 等。
- **improve** 智能提示 UX 修复：提示框智能上下翻转避免遮挡输入行；鼠标静止时不会误选中提示项；密码隐藏输入不记入历史、不弹出提示。
- **improve** 终端标签改为按钮风格，活跃态有 accent 边框 + 底色，AI 锁定与选中效果叠加。
- **improve** AI 侧边栏默认新建会话，空会话不保存，最多保留 15 个会话。
- **bugfix** 修复终端历史记录读取逻辑。从可视区域末行扫描替代 cursorY，解决 buffer 滚动后无法读取 prompt 行命令的问题。
- **bugfix** 修复提示框 Enter 后未关闭的竞态条件（debounce 计时器取消 + 空 token 检查）。
- **bugfix** 修复 Windows 11 多进程 WebView2 冲突导致无法输入。UserDataFolder 改用进程 PID 隔离路径。
- **bugfix** 修复 Tab 切换时终端出现乱码。xterm.js allowProposedApi 开启后 OSC 颜色查询经 onData 被发往服务端回显为乱码，增加 OSC 过滤解决。

## v2026.05.29-alpha

- **new** Terminal smart completion. Real-time popup with history command and AI rewrite suggestions while typing in SSH terminals. Settings page adds a command history management section with search, select-all, and batch delete.
- **new** Server monitor. Real-time monitoring for connected servers. Supports performance metrics (CPU/memory/disk/network), process list with details, listening ports, disk usage and mount info, network interfaces with bond/bridge detection.
- **new** SSH post-login script execution. Configure a script to run automatically after SSH connection; supports idle detection to avoid executing during manual interaction.
- **new** SSH keepalive to prevent idle disconnect. Sends periodic keepalive packets and shows a reconnect prompt when the connection drops.
- **improve** Sidebar splitter visibility and terminal scrollbar contrast improved for easier interaction.
- **bugfix** Fixed MySQL multi-database race condition in database query capabilities.
- **bugfix** Unified connection type label rendering between grouped and ungrouped views in the sidebar.

---

- **new** 终端智能补全。SSH 终端输入时实时弹出历史命令和 AI 转写建议。设置页面新增历史命令管理栏目，支持搜索、全选、批量删除。
- **new** 服务器监控。实时监看已连接服务器的运行状态。支持 CPU/内存/磁盘/网络性能指标、进程列表及详情、监听端口、磁盘用量与挂载信息、网卡列表及 bond/bridge 识别。
- **new** SSH 登录后脚本执行。支持配置连接成功后自动执行脚本，支持空闲检测避免在用户手动操作时误执行。
- **new** SSH 保活机制防止空闲断开。定时发送保活包，连接断开时显示重连提示。
- **improve** 侧边栏分割条可见性和终端滚动条对比度优化，操作更便捷。
- **bugfix** 修复数据库 MySQL 多库查询竞态条件问题。
- **bugfix** 统一侧边栏分组与非分组视图中的连接类型标签渲染。

## v2026.05.27-alpha

- **new** Database connection and query. Supports MySQL, PostgreSQL, and rqlite. Provides SQL query execution, table schema browsing, CRUD on data rows, and tree navigation of databases/tables.
- **new** Terminal search bar. Press Ctrl+F to open the search bar; highlights matches and counts results using @xterm/addon-search.
- **new** Connection sidebar and AI sidebar visibility states are now persisted to localStorage, restoring expand/collapse state after restart.
- **improve** Scrollbar width increased from 5px to 8px for easier grabbing.
- **improve** AI sidebar maximize button now shows a shrink icon when expanded for clearer indication.
- **improve** Added transparent padding around window edges so the terminal can still be resized by dragging even when it fills the edge.
- **improve** Workspace tabs now display the LayoutDashboard icon.
- **bugfix** Fixed garbled text appearing when switching tabs. Session buffer truncation could fall in the middle of escape sequences (DA2, OSC color queries, etc.), leaving fragments without the \x1b prefix that xterm.js rendered as garbage. Fix: scan for \x1b before the first \n to determine a safe restart boundary.

---

- **new** 数据库连接与查询。支持 MySQL、PostgreSQL、rqlite 三种数据库，提供 SQL 查询执行、表结构浏览、数据行增删改查、数据库/表树形导航等功能。
- **new** 终端搜索栏。Ctrl+F 打开搜索栏，基于 @xterm/addon-search 实现匹配高亮和结果计数。
- **new** 连接侧边栏和 AI 侧边栏显示状态持久化到 localStorage，重启后保持上次的展开/收起状态。
- **improve** 滚动条宽度从 5px 增加到 8px，更容易抓取操作。
- **improve** AI 侧边栏最大化按钮在展开时显示缩回图标（Shrink），更直观。
- **improve** 窗口边缘增加透明内边距，终端填充边缘时仍可拖拽调整窗口大小。
- **improve** 工作区标签页增加 LayoutDashboard 图标。
- **bugfix** 修复标签页切换时终端出现乱码的问题。会话数据缓冲区裁剪点可能落在转义序列中间（DA2、OSC 颜色查询等），残缺片段缺少 \x1b 前缀被 xterm.js 渲染为乱码。修复方案：在第一个 \n 前扫描 \x1b 确定安全重启边界。

## v2026.05.25-alpha

- **bugfix** Editing and saving a connection caused passwords for other connections to be lost. Fixed an issue in the Go backend Save method where underlying array sharing inadvertently cleared passwords/APIKeys.

---

- **bugfix** 编辑连接保存后，其他连接的密码信息丢失。修复 Go 后端 Save 方法底层数组共享导致的密码/APIKey 被意外清空的问题。

## v2026.05.24-alpha

- **new** Cloud config sync. Build a private cloud sync repository based on GitHub, GitLab, or Gitee private repos. All configurations (connections, AI model keys, app settings) are encrypted with AES-256-GCM before being saved remotely. Supports auto-sync, conflict resolution, master password change, and repo binding management.

---

- **new** 云端配置同步。基于 GitHub、GitLab、Gitee 私有仓库构建专属私人云同步仓库，所有配置（连接信息、AI 模型密钥、应用设置）经 AES-256-GCM 加密后保存至远端，支持自动同步、冲突解决、主密码修改和仓库绑定管理。

## v2026.05.23-alpha

- **new** VNC remote desktop. Connect to VNC servers (TigerVNC, TightVNC, QEMU, etc.) via noVNC with a built-in WebSocket↔TCP proxy bridge. DOM remains alive across tab switches for zero-latency screen recovery. Supports auto-resize toggle and bidirectional clipboard sharing (Ctrl+Shift+V to paste local clipboard).
- **new** Local terminal support. Open a local shell directly (Windows PowerShell/CMD, macOS/Linux bash/zsh) without an SSH connection.
- **new** AI Shell awareness. AI can detect the current terminal's shell type to generate more accurate commands.
- **improve** Connection list now shows ports. Sidebar entries changed from `user@host` to `user@host:port`, displaying `host:port` when the username is empty.
- **improve** Username field is hidden when creating a new VNC connection (VNC authentication only requires a password).
- **improve** Icon unification.
- **bugfix** RDP windows now correctly hide/show when menus, dialogs, or window dragging occur, avoiding obstruction.
- **bugfix** Settings page state persistence issue fixed.
- **bugfix** Password input visibility toggle fixed.
- **bugfix** Windows 11 security dialog suppression fixed.

---

- **new** VNC 远程桌面。支持通过 noVNC 连接 VNC 服务器（TigerVNC、TightVNC、QEMU 等），内置 WebSocket↔TCP 代理桥接；标签页切换时 DOM 保活实现零延迟恢复画面，支持自动缩放开关、剪贴板双向共享（Ctrl+Shift+V 粘贴本地剪贴板）。
- **new** 本地终端支持。可直接打开本地 shell（Windows PowerShell/CMD、macOS/Linux bash/zsh），无需 SSH 连接。
- **new** AI Shell 感知。AI 可以感知当前终端的 shell 类型，生成更准确的命令。
- **improve** 连接列表显示端口。Sidebar 中连接条目从 `user@host` 改为 `user@host:port`，用户名为空时仅显示 `host:port`。
- **improve** VNC 新建连接时隐藏用户名字段（VNC 认证只需要密码）。
- **improve** 图标统一。
- **bugfix** RDP 窗口在弹出菜单、对话框、窗口拖拽时正确隐藏/显示，避免遮挡。
- **bugfix** 设置页状态持久化问题修复。
- **bugfix** 密码输入框可见性切换修复。
- **bugfix** Windows 11 安全对话框抑制修复。

## v2026.05.22-alpha

- **new** RDP remote desktop. Connect to Windows Remote Desktop via the Microsoft RDP ActiveX control. Native security dialogs are fully suppressed; the RDP window is seamlessly embedded into uniTerm tabs and smoothly follows window dragging and resizing.
- **new** AI sidebar maximize button. Expands the AI assistant panel to fill the entire main area; click again to restore original width.
- **new** AI message copy functionality. Added copy buttons next to tool-call IN/OUT expand boxes with a checkmark feedback on click. Added a "Copy as Markdown" button in the top-right corner of messages that expands on hover.
- **new** Broadcast input. Added a broadcast button to the workspace panel header to send keyboard input to all terminals in the workspace simultaneously.
- **new** About section in Settings page showing the app version.
- **new** Added "Write-operation confirmation" level to AI command execution confirmation. On top of the existing three levels (Off / Dangerous only / All), added a "Dangerous + Write" option that also requires confirmation for write operations like rm and mv, filling the granularity gap between Dangerous and All.
- **new** Added "New Connection" to the connection group context menu; the group is automatically preselected when creating.
- **improve** Unified sidebar selection state. Merged `selectedId` and `multiSelectedIds` into a single `selectedIds` set for consistent highlight logic; fixed the issue where previous selection was not cleared on right-click.
- **improve** Right-clicking a connection now automatically deselects others and selects only the current one if it wasn't already in the selection.
- **improve** Tab bar improvements: supports horizontal scrolling with mouse wheel, and dropdown selections auto-scroll into view.
- **improve** System prompt message role changed from `assistant` to `tool` to avoid polluting the LLM conversation context.
- **improve** AI session storage migrated from localStorage to Go backend file storage.
- **improve** README redesign: added Chinese version, landing page, categorized feature showcase, and screenshot carousel.
- **bugfix** Fixed garbled text when switching tabs (strip incomplete escape sequences).
- **bugfix** Fixed panel active state not syncing terminal focus in workspaces.
- **bugfix** Fixed edit button incorrectly grayed out when a single connection is selected.
- **bugfix** Fixed "New Connection..." virtual item not auto-selected when search yields no matches.
- **bugfix** Fixed tab title displaying with host suffix (should show connection name only).

---

- **new** RDP 远程桌面。支持通过 Microsoft RDP ActiveX 控件连接 Windows 远程桌面，原生安全对话框已完全抑制，RDP 窗口无缝嵌入 uniTerm 标签页，窗口拖拽和缩放时平滑跟随。
- **new** AI 边栏最大化按钮。可将 AI 助理窗口最大化至整个主区域，再次点击恢复原始宽度。
- **new** AI 消息复制功能。工具调用 IN/OUT 展开框旁增加复制按钮，点击后显示对勾反馈；消息右上角增加"复制为 Markdown"按钮，hover 后展开显示文字。
- **new** 广播输入功能。工作区面板标题栏增加广播按钮，可将键盘输入同时发送到工作区内所有终端。
- **new** 设置页关于栏目，显示应用版本号。
- **new** AI 命令执行确认模式新增"写操作确认"级别。原有三级（关闭/仅危险/全部）基础上增加"危险+写操作"选项，对 rm、mv 等写操作命令也需确认，填补危险与全部之间的安全粒度空缺。
- **new** 连接分组右键菜单增加"新建连接"，新建时自动预选该分组。
- **improve** 侧边栏选中状态统一。将 `selectedId` 与 `multiSelectedIds` 合并为单一 `selectedIds` 集合，选中高亮逻辑一致，修复右键选中时之前选中未清除的问题。
- **improve** 右键单击连接时，若该连接未在选中集合中，自动取消其他选中并仅选中当前连接。
- **improve** 标签栏优化：支持鼠标滚轮横向滚动，下拉菜单选中后自动滚动到可见位置。
- **improve** 系统提示消息角色从 `assistant` 改为 `tool`，避免污染 LLM 对话上下文。
- **improve** AI 会话存储从 localStorage 迁移至 Go 后端文件存储。
- **improve** README 重新设计：新增中文版本、介绍网站首页、功能分类展示和截图轮播。
- **bugfix** 修复标签页切换时终端出现乱码的问题（剥离不完整的转义序列）。
- **bugfix** 修复工作区中面板激活状态未同步终端焦点的问题。
- **bugfix** 修复选中单个连接时编辑按钮错误置灰的问题。
- **bugfix** 修复搜索无匹配结果时未自动选中"新建连接..."虚拟条目的问题。
- **bugfix** 修复标签标题显示带主机后缀的问题（应仅显示连接名称）。

## v2026.05.17-alpha

- **new** Connection grouping. Supports creating, renaming, and deleting connection groups. Connections can be collapsed by group; drag-and-drop to change group assignment; ungrouped connections are automatically placed in a "(No Group)" virtual group.
- **new** Batch connection selection and actions. Supports Ctrl+click multi-select and Shift+click range select. Context menu supports batch connect, batch SFTP connect, batch copy, and batch delete. Enter key also supports batch open.
- **new** A "New Connection..." virtual item appears at the bottom of the list while typing in the search bar. Double-click or press Enter to prefill the host field and open the new connection dialog.
- **new** Confirmation prompt before deleting connections, showing the number of items to be deleted.
- **change** Light theme redesigned with a Windows-style neutral gray palette, using blue accent color instead of the previous warm yellow.
- **improve** AI session names, workspace names, AI lock button tooltips, and more now support Chinese/English internationalization.
- **bugfix** Fixed AI assistant settings button navigating to General Settings instead of AI Settings.
- **bugfix** Fixed right-click menu appearing on the ".." parent directory row in the SFTP file list.
- **bugfix** Fixed selected count being one less than actual when multi-selecting connections.

---

- **new** 连接分组功能。支持创建、重命名、删除连接分组，连接可按分组折叠展示，支持拖拽调整连接所属分组，无分组连接自动归入"(无分组)"虚拟分组。
- **new** 连接批量选择与操作。支持 Ctrl+点击多选、Shift+点击范围选择，右键菜单可批量连接、批量连接 SFTP、批量复制、批量删除选中的连接，回车键同样支持批量打开。
- **new** 搜索栏输入时列表底部显示"新建连接..."虚拟条目，双击或回车可将搜索内容预填到主机字段并打开新建连接窗口。
- **new** 删除连接前弹出确认提示，显示待删除数量。
- **change** 浅色主题重新设计为 Windows 风格中性灰色调，使用蓝色强调色替代原来的暖黄色。
- **improve** AI 会话名称、工作区名称、AI 锁定按钮提示等支持中英文国际化。
- **bugfix** 修复 AI 助理设置按钮跳转到基础设置而非 AI 设置的问题。
- **bugfix** 修复 SFTP 文件列表中".."返回上级目录行弹出右键菜单的问题。
- **bugfix** 修复多选连接时选中数量总是少一个的问题。

## v2026.05.16-alpha

- **new** SFTP file manager with dual-pane browsing of local and remote files. Supports upload, download, rename, delete, and permission changes. Transfer tasks are tracked independently per tab.

---

- **new** SFTP 文件管理器，支持双栏浏览本地与远程文件，可进行文件上传、下载、重命名、删除、权限修改等操作，传输任务按标签页独立跟踪。

## v2026.05.15-alpha

- **new** Workspace panel system. Merge multiple terminal tabs into a workspace, displayed side-by-side or stacked within the same window. Drag panel headers to freely adjust panel position and size; drag tabs to panel edges to auto-create new splits.
- **new** Custom window title bar adapted for Windows and macOS platform window controls.
- **new** Terminal http/https links are auto-detected and underlined. Hover to see tooltip; Ctrl+click to open in default browser.
- **new** Windows installer now detects running processes and prompts to close the running application before installation.
- **improve** Dark theme contrast improved; both Default and Deep Blue color schemes have clearer background layers and more readable text.
- **bugfix** Selected text auto-copy to clipboard now works even if the mouse is released outside the terminal.
- **bugfix** Fixed errors caused by missing tool responses in AI multi-turn conversations.
- **bugfix** Fixed blank areas in child panels and unmovable split bars when panel splitting occurs.
- **bugfix** Fixed abnormal terminal content display when window or panel size changes.
- **bugfix** Fixed Ctrl+scroll wheel causing unexpected full-window zoom.

---

- **new** 工作区面板系统。支持将多个终端标签页合并为工作区，在同一个窗口内左右或上下分屏显示，拖拽面板标题栏可自由调整面板位置和大小，拖拽标签页到面板边缘自动创建新的分屏。
- **new** 自定义窗口标题栏，适配 Windows 和 macOS 平台窗口控制按钮。
- **new** 终端内 http/https 链接自动识别并显示下划线，鼠标悬停提示，Ctrl+点击在默认浏览器中打开。
- **new** Windows 安装包增加运行中进程检测，安装前提示关闭正在运行的程序。
- **improve** 暗色主题对比度提升，默认和深蓝两套配色背景层次更分明，文字更易读。
- **bugfix** 选中文字自动复制到剪贴板现在即使鼠标在终端外松开也能生效。
- **bugfix** 修复 AI 多轮对话中 tool 响应丢失导致的错误。
- **bugfix** 修复面板分屏时子面板出现空白区域、分割条无法拖动的问题。
- **bugfix** 修复窗口或面板尺寸变化时终端内容显示异常。
- **bugfix** 修复 Ctrl+滚轮导致整个窗口意外缩放的问题。

## v2026.05.13-alpha

- **new** Free panel splitting. Split windows left/right or top/bottom to open multiple terminals simultaneously. Drag borders to resize panels; tabs can also be dragged across panels.
- **new** AI window lock button. The "AI" button on each terminal tab locks AI to that terminal. After locking, AI command execution targets only that terminal; switching to other tabs won't send commands to the wrong place.
- **improve** Increased AI max consecutive interactions per conversation from 10 to 20. A "Continue" button appears when the limit is reached to continue the conversation; added history message length control to prevent sluggishness from overly long context.
- **new** Connection list supports up/down arrow key navigation; press Enter to connect directly without double-clicking.
- **change** Windows releases now provide installer (.exe) only, no additional zip archives. macOS releases now provide DMG only, no additional zip archives.
- **improve** Terminal default scrollback lines reduced from 5000 to 2500 to reduce memory usage.
- **bugfix** Fixed terminal content not resizing when the window or panel shrinks.

---

- **new** 支持自由分屏功能。窗口可以左右或上下拆分，同时打开多个终端，拖拽边界调整面板大小，标签页也可跨面板拖拽。
- **new** 支持AI窗口锁定按钮。每个终端标签页上的 "AI" 按钮可将 AI 锁定到该终端，锁定后 AI 执行命令只针对该终端，切换其他标签也不会跑错位置。
- **improve** 增加 AI 单次对话最多连续交互 20 轮（原来是 10 轮），达到上限后会出现 "继续" 按钮，点击可接着之前的话题继续聊；增加控制历史消息长度，防止上下文过长导致卡顿。
- **new** 左侧连接列表可用上下箭头切换选中项，按回车直接连接，不用鼠标双击。
- **change** Windows 只提供安装包（.exe），不再额外提供压缩包。macOS 只提供 DMG 镜像，不再额外提供压缩包。
- **improve** 终端默认保留的历史行数从 5000 行降到 2500 行，减少内存占用。
- **bugfix** 修复窗口或面板缩小时终端内容不跟随调整的问题。
