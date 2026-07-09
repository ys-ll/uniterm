# Changelog

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
