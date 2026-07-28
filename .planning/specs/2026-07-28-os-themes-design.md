# OS-Themed UI + Soft Gray 终端主题 设计文档

- 日期：2026-07-28
- 状态：待用户审阅

## 1. 目标

1. 新增两个 UI 主题：**Windows 11** 和 **macOS 26**。每个主题**自动跟随系统深浅模式**，但在选择器里只占一个条目。
2. 新增一个终端主题：`uniterm Soft Gray`，浅灰背景，ANSI 调色板针对字符易辨认度优化。
3. **不改**现有的 `dark` / `deep-blue` / `light` 三个 UI 主题 —— 零回归。

## 2. 不在范围内

- 不引入任何 npm / go 依赖。
- 不接 Win32 DWM / macOS NSVisualEffectView —— Mica 和 Liquid Glass **用 CSS 渐变模拟**，不调原生窗口 API。
- 不改 `dark` / `deep-blue` / `light` 三个现有 UI 主题。
- 不动现有 `system` 模式的 JS 逻辑（它本来就跟随系统深浅）。
- 不新增用户自定义主题机制（已存在 `customTerminalThemes`，不动）。
- 不改 xterm.js 字体加载机制（终端字体仍由 `settings.fontFamily` 决定）。

## 3. 架构

### 3.1 CSS 变量驱动的双层主题

现有主题系统全部基于 `<html data-theme="X">` 切换 CSS 变量。新主题沿用同机制，但**主题块内部用 `@media (prefers-color-scheme: dark)` 分支**：

```css
[data-theme="win11"] {
  --bg-base: #fafafa;
  /* ... light 变量 ... */
}

@media (prefers-color-scheme: dark) {
  [data-theme="win11"] {
    --bg-base: #202020;
    /* ... dark 变量覆盖 ... */
  }
}
```

效果：JS 把 `data-theme="win11"` 写到 `<html>` 后，深浅由浏览器/OS 的 `prefers-color-scheme` 媒体查询决定，JS 无需监听系统变化。

### 3.2 改动文件清单

| 文件 | 改动内容 |
|------|---------|
| `frontend/src/types/settings.ts` | `Theme` 联合类型扩展加 `'win11' \| 'macos26'`；`TERMINAL_THEMES` 数组加 `uniterm-soft-gray` 项 |
| `frontend/src/style.css` | 新增 `[data-theme="win11"]` 和 `[data-theme="macos26"]` 两个主题块（各含 light 变量 + `@media (prefers-color-scheme: dark)` dark 覆盖），并补全套组件层级样式覆盖 |
| `frontend/src/composables/useTerminal.ts` | `getXtermTheme()` 新增 `'uniterm-soft-gray'` 分支 |
| `frontend/src/components/Sidebar.vue` | 选择器 `<el-option>` 列表加 2 项 |
| `frontend/src/components/SettingsTab.vue` | 选择器 `<el-option>` 列表加 2 项 |
| `frontend/src/i18n/locales/*.json` | 所有 locale 加 `settings.themeWin11` 和 `settings.themeMacOS26` 两个 key |

### 3.3 不动的代码

- 把 `settings.theme` 写到 `<html data-theme>` 的逻辑（已在 `settingsStore` / `App.vue` 落地，无需改）
- `system` 模式的现有行为（CSS `@media` 已自动跟随）
- 现有 3 个 UI 主题的 CSS 块
- `TERMINAL_THEMES` 现有 27 项
- `getXtermTheme()` 现有分支

## 4. Win11 主题

### 4.1 字体

`--font-ui` 在 `[data-theme="win11"]` 内重定义：

```css
--font-ui: 'Segoe UI Variable Text', 'Segoe UI', system-ui, -apple-system, sans-serif;
```

`--font-mono` 不变（终端用，Win11 终端是 Cascadia Mono/Cascadia Code，但终端字体由 `fontFamily` 决定，主题不强制）。

### 4.2 配色

| 角色 | Light | Dark |
|------|-------|------|
| `--bg-base` | `#fafafa` | `#202020` |
| `--bg-elevated` | `#f3f3f3` | `#2c2c2c` |
| `--bg-surface` | `#ffffff` | `#2c2c2c` |
| `--bg-overlay` | `#ebebeb` | `#383838` |
| `--bg-hover` | `#e5e5e5` | `#3f3f3f` |
| `--bg-active` | `#d9d9d9` | `#4a4a4a` |
| `--text-primary` | `#1c1c1c` | `#f0f0f0` |
| `--text-secondary` | `#5c5c5c` | `#c8c8c8` |
| `--text-muted` | `#707070` | `#a0a0a0` |
| `--text-disabled` | `#9a9a9a` | `#6a6a6a` |
| `--accent` | `#0078d4` | `#4cc2ff` |
| `--accent-glow` | `rgba(0,120,212,0.22)` | `rgba(76,194,255,0.22)` |
| `--info` | `#006ab1` | `#5eb3f5` |
| `--success` | `#107c41` | `#34d399` |
| `--warning` | `#e07b00` | `#f59e0b` |
| `--error` | `#d13438` | `#f87171` |
| `--border-subtle` | `rgba(0,0,0,0.0578)` | `rgba(255,255,255,0.0837)` |
| `--border-hover` | `rgba(0,0,0,0.16)` | `rgba(255,255,255,0.16)` |
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,0.06)` | `0 1px 2px rgba(0,0,0,0.4)` |
| `--shadow-md` | `0 4px 12px rgba(0,0,0,0.08)` | `0 4px 12px rgba(0,0,0,0.5)` |
| `--shadow-lg` | `0 8px 24px rgba(0,0,0,0.10)` | `0 8px 24px rgba(0,0,0,0.5)` |
| `--radius-sm` | `4px` | `4px` |
| `--radius-md` | `8px` | `8px` |
| `--radius-lg` | `8px` | `8px` |
| `--scrim` | `rgba(255,255,255,0.6)` | `rgba(0,0,0,0.35)` |
| `--scrollbar-thumb` | `rgba(0,0,0,0.22)` | `rgba(255,255,255,0.18)` |
| `--scrollbar-thumb-hover` | `rgba(0,0,0,0.35)` | `rgba(255,255,255,0.3)` |

### 4.3 Mica 质感（CSS 模拟）

dark 模式 `--bg-base` 改为：

```css
background: linear-gradient(180deg, #2b2b2b 0%, #202020 100%);
```

light 模式 `--bg-base` 改为：

```css
background: linear-gradient(180deg, #fafbfc 0%, #f3f3f3 100%);
```

效果是顶部稍亮的微妙渐变，模拟 Mica 的明暗变化。

### 4.4 组件层级样式覆盖

所有覆盖用 `[data-theme="win11"]` 或 `[data-theme="win11"] .el-xxx` 选择器限定，不影响其他主题。

#### Tab（`.el-tabs__item`）

- **形状**：方正 8px 圆角，无下边框线
- **未激活**：透明背景，文字 `--text-primary`，hover 时背景 `--bg-hover`
- **激活**：背景 `--accent` 实色，文字 `#ffffff`，hover 不变色

```css
[data-theme="win11"] .el-tabs__item {
  border-radius: 8px;
  padding: 0 16px;
  height: 32px;
  line-height: 32px;
  color: var(--text-primary);
  transition: background-color 0.1s ease;
}
[data-theme="win11"] .el-tabs__item:hover {
  background: var(--bg-hover);
}
[data-theme="win11"] .el-tabs__item.is-active,
[data-theme="win11"] .el-tabs__item.is-active:hover {
  background: var(--accent);
  color: #ffffff;
  font-weight: 500;
}
[data-theme="win11"] .el-tabs__active-bar {
  display: none;
}
```

#### Button（`.el-button`）

- 主按钮：accent 实色 + 白字，圆角 4px
- 次按钮：浅灰底 + 1px 边框

```css
[data-theme="win11"] .el-button {
  border-radius: 4px;
  font-weight: 400;
}
[data-theme="win11"] .el-button--primary {
  background: var(--accent);
  border-color: var(--accent);
  color: #ffffff;
}
[data-theme="win11"] .el-button--primary:hover {
  filter: brightness(1.08);
}
[data-theme="win11"] .el-button:not(.el-button--primary):not(.el-button--text) {
  background: var(--bg-elevated);
  border-color: var(--border-subtle);
  color: var(--text-primary);
}
```

#### Input / Textarea（`.el-input__wrapper` / `.el-textarea__inner`）

- 仅底部 1px 边框，整体透明底
- 聚焦时整条 border 变 accent 色

```css
[data-theme="win11"] .el-input__wrapper {
  background: transparent;
  box-shadow: none;
  border-radius: 4px 4px 0 0;
  border-bottom: 1px solid var(--border-subtle);
}
[data-theme="win11"] .el-input__wrapper:hover {
  border-bottom-color: var(--border-hover);
}
[data-theme="win11"] .el-input__wrapper.is-focus {
  border-bottom-color: var(--accent);
  box-shadow: 0 1px 0 0 var(--accent);
}
```

#### Dialog / MessageBox（`.el-dialog` / `.el-message-box`）

- 圆角 8px，表面 `--bg-surface`，阴影更深

```css
[data-theme="win11"] .el-dialog,
[data-theme="win11"] .el-message-box {
  border-radius: 8px;
  background: var(--bg-surface);
  box-shadow: 0 16px 48px rgba(0,0,0,0.18), 0 0 0 1px rgba(0,0,0,0.04);
}
```

#### Menu / Sidebar 激活态

- 菜单项激活时左侧 3px accent 竖条

```css
[data-theme="win11"] .el-menu-item.is-active {
  background: var(--accent-subtle);
  color: var(--accent);
  position: relative;
}
[data-theme="win11"] .el-menu-item.is-active::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 3px;
  background: var(--accent);
}
```

#### Checkbox / Radio / Switch

- checkbox 角部 `border-radius: 2px`（Win11 风格）
- switch 用 Element Plus 默认胶囊 + 圆形滑块（已近似 Win11）

```css
[data-theme="win11"] .el-checkbox__inner,
[data-theme="win11"] .el-radio__inner {
  border-radius: 2px;
}
[data-theme="win11"] .el-checkbox__inner {
  border-width: 1px;
}
```

#### Scrollbar

- Win11 风格细滚动条

```css
[data-theme="win11"] ::-webkit-scrollbar { width: 8px; height: 8px; }
[data-theme="win11"] ::-webkit-scrollbar-thumb { background: var(--scrollbar-thumb); border-radius: 4px; }
[data-theme="win11"] ::-webkit-scrollbar-thumb:hover { background: var(--scrollbar-thumb-hover); }
```

#### 自定义组件

- **AppHeader**：底边用 `--border-subtle`，按钮 hover 改用 Win11 的轻量高亮
- **Sidebar**：personalization 面板行距用 `12px`，比现有稍紧凑
- **SettingsTab**：分组标题与项间距 8px，激活指示器用左侧 3px accent 竖条

所有自定义组件的覆盖用类选择器限定在 `[data-theme="win11"]` 下，不影响其他主题。

### 4.5 Element Plus 变量

完整覆盖 `--el-bg-color`、`--el-text-color-primary`、`--el-fill-color-*`、`--el-border-color-*` 等，保持和 `--bg-*` / `--text-*` 一致。

## 5. macOS26 主题

### 5.1 字体

```css
--font-ui: -apple-system, BlinkMacSystemFont, 'SF Pro Text', 'Helvetica Neue', sans-serif;
```

### 5.2 配色

| 角色 | Light | Dark |
|------|-------|------|
| `--bg-base` | `#ffffff` | `#1e1e1e` |
| `--bg-elevated`（sidebar） | `#ececef` | `#2c2c2e` |
| `--bg-surface` | `#ffffff` | `#1e1e1e` |
| `--bg-overlay` | `#f5f5f7` | `#3a3a3c` |
| `--bg-hover` | `#f0f0f3` | `#3a3a3c` |
| `--bg-active` | `#e5e5e8` | `#48484a` |
| `--text-primary` | `#1d1d1f` | `#f5f5f7` |
| `--text-secondary` | `#6e6e73` | `#aeaeb2` |
| `--text-muted` | `#86868b` | `#8e8e93` |
| `--text-disabled` | `#a1a1a6` | `#636366` |
| `--accent` | `#007aff` | `#0a84ff` |
| `--accent-glow` | `rgba(0,122,255,0.20)` | `rgba(10,132,255,0.22)` |
| `--info` | `#0064cd` | `#64d2ff` |
| `--success` | `#28a745` | `#30d158` |
| `--warning` | `#ff9500` | `#ff9f0a` |
| `--error` | `#ff3b30` | `#ff453a` |
| `--border-subtle` | `rgba(0,0,0,0.08)` | `rgba(255,255,255,0.08)` |
| `--border-hover` | `rgba(0,0,0,0.16)` | `rgba(255,255,255,0.18)` |
| `--shadow-sm` | `0 1px 2px rgba(0,0,0,0.04)` | `0 1px 2px rgba(0,0,0,0.3)` |
| `--shadow-md` | `0 4px 12px rgba(0,0,0,0.06)` | `0 4px 12px rgba(0,0,0,0.4)` |
| `--shadow-lg` | `0 8px 24px rgba(0,0,0,0.08)` | `0 8px 24px rgba(0,0,0,0.5)` |
| `--radius-sm` | `4px` | `4px` |
| `--radius-md` | `10px` | `10px` |
| `--radius-lg` | `14px` | `14px` |
| `--scrim` | `rgba(255,255,255,0.5)` | `rgba(0,0,0,0.4)` |
| `--scrollbar-thumb` | `rgba(0,0,0,0.20)` | `rgba(255,255,255,0.20)` |
| `--scrollbar-thumb-hover` | `rgba(0,0,0,0.32)` | `rgba(255,255,255,0.32)` |

### 5.3 Liquid Glass（CSS 模拟）

dark 模式：

```css
[data-theme="macos26"] body {
  background:
    radial-gradient(ellipse at top, rgba(255,255,255,0.04) 0%, transparent 60%),
    var(--bg-base);
}
```

light 模式：

```css
[data-theme="macos26"] body {
  background:
    radial-gradient(ellipse at top, rgba(255,255,255,0.6) 0%, transparent 50%),
    var(--bg-base);
}
```

sidebar 在两种模式下都用 `--bg-elevated`（明显深于主区），保持 macOS 经典观感。

### 5.4 组件层级差异（与 Win11 对照）

只列差异点，其余同 4.4。

#### Tab

- **形状**：圆滑 10px，无背景
- **激活**：底部 2px accent 实线 + 文字加粗
- 无激活背景块

```css
[data-theme="macos26"] .el-tabs__item {
  border-radius: 10px;
  padding: 0 12px;
  height: 32px;
  line-height: 32px;
  color: var(--text-secondary);
  background: transparent;
}
[data-theme="macos26"] .el-tabs__item:hover {
  color: var(--text-primary);
}
[data-theme="macos26"] .el-tabs__item.is-active {
  color: var(--accent);
  font-weight: 600;
  background: transparent;
}
[data-theme="macos26"] .el-tabs__active-bar {
  background: var(--accent);
  height: 2px;
}
```

#### Button

- 圆角 6px（比 Win11 略圆）
- 主按钮无边框，仅 accent 实色

```css
[data-theme="macos26"] .el-button {
  border-radius: 6px;
  font-weight: 500;
}
[data-theme="macos26"] .el-button--primary {
  background: var(--accent);
  border: none;
  color: #ffffff;
}
```

#### Input

- 全边框 1px，圆角 6px（与 Win11 的"仅下边框"不同）

```css
[data-theme="macos26"] .el-input__wrapper {
  background: var(--bg-surface);
  box-shadow: 0 0 0 1px var(--border-subtle);
  border-radius: 6px;
}
[data-theme="macos26"] .el-input__wrapper.is-focus {
  box-shadow: 0 0 0 2px var(--accent);
}
```

#### Dialog / MessageBox

- 圆角 14px，阴影更柔

```css
[data-theme="macos26"] .el-dialog,
[data-theme="macos26"] .el-message-box {
  border-radius: 14px;
  box-shadow: 0 20px 40px rgba(0,0,0,0.12);
}
```

#### Sidebar

- sidebar 容器背景强制用 `--bg-elevated`（macOS 标志性的稍深于主区）
- 通过 CSS 选择器 `[data-theme="macos26"] .sidebar` 覆盖现有 `.sidebar` 类 —— **无需改 Sidebar.vue**

#### Switch / Checkbox / Radio

- 角部圆度比 Win11 略大

```css
[data-theme="macos26"] .el-checkbox__inner,
[data-theme="macos26"] .el-radio__inner {
  border-radius: 4px;
}
```

### 5.5 Element Plus 变量

完整覆盖，和 4.5 类似但取 macOS26 的数值。

## 6. 终端主题 `uniterm Soft Gray`

### 6.1 元数据

```typescript
{ label: 'uniterm Soft Gray', value: 'uniterm-soft-gray', type: 'light' }
```

加入 `TERMINAL_THEMES` 数组，会自动出现在终端主题选择器的 "Light" 分组下。

### 6.2 ANSI 调色板

```typescript
{
  background:      '#e8e8e8',
  foreground:      '#1a1a1a',
  cursor:          '#1a1a1a',
  selection:       'rgba(0, 120, 212, 0.25)',

  black:           '#2d2d2d',
  red:             '#c1395b',
  green:           '#2c8c4f',
  yellow:          '#b07d00',
  blue:            '#1f6fcc',
  magenta:         '#8a44c9',
  cyan:            '#0d8a8a',
  white:           '#5a5a5a',

  brightBlack:     '#6a6a6a',
  brightRed:       '#e85a82',
  brightGreen:     '#4cb87a',
  brightYellow:    '#d99a1a',
  brightBlue:      '#4ca0e8',
  brightMagenta:   '#b06ee0',
  brightCyan:      '#33b3b3',
  brightWhite:     '#2d2d2d'  // light bg 上前景要深
}
```

**字符易辨认度优化点：**

- `0`（black `#2d2d2d`）和 `O`（white `#5a5a5a`）—— 亮度差大，颜色接近但权重不同
- `1`（亮色前景 + 加粗由程序控制）/ `l`（前景 `#1a1a1a`，深）—— 视觉重量不同
- `B`（blue `#1f6fcc`）vs `8`（magenta `#8a44c9`）—— 不同 hue，亮度差
- `red` `#c1395b`（偏粉）vs `magenta` `#8a44c9`（偏紫）—— hue 间隔大
- `green` `#2c8c4f` vs `cyan` `#0d8a8a` —— 饱和度差+亮度差

字体不在终端主题内强制 —— 用户在 `fontFamily` 选项里选 Consolas / Cascadia Code / JetBrains Mono 都能配这个调色板。

### 6.3 接入位置

在 `frontend/src/composables/useTerminal.ts` 的 `getXtermTheme()` 加一个 case 分支：

```typescript
if (name === 'uniterm-soft-gray') {
  return { background: '#e8e8e8', foreground: '#1a1a1a', /* ... */ }
}
```

参考已有的 `uniterm-light` 分支（同为 light 类型）实现。

## 7. 设置面板与 i18n

### 7.1 UI 主题选择器

`Sidebar.vue` 和 `SettingsTab.vue` 的 `<el-option>` 列表中，在现有 4 项（dark / deep-blue / light / system）之后加：

```vue
<el-option :label="t('settings.themeWin11')" value="win11" />
<el-option :label="t('settings.themeMacOS26')" value="macos26" />
```

### 7.2 翻译 key

所有 locale 文件（`frontend/src/i18n/locales/zh-CN.json`、`en.json`、`zh-TW.json`、`ja.json`、`ko.json`、`de.json`、`es.json`、`fr.json`、`ru.json`）加：

- `settings.themeWin11` —— 例如英文 `Windows 11`，中文 `Windows 11`
- `settings.themeMacOS26` —— 例如英文 `macOS 26`，中文 `macOS 26`

翻译值直接用主题名的本地化字符串（不深译"Windows 11"为别的）。

### 7.3 终端主题选择器

**不动**。`useTerminalThemeOptions.ts` 自动按 `type` 分组，新项加入 "Light" 分组。

## 8. 验收标准

### 8.1 视觉验收（手工，必做）

启动 `wails dev`，系统先切到 light 模式：

1. 选 `Windows 11`：
   - tab 是方角矩形激活态（accent 蓝实色块 + 白字）
   - 按钮圆角 4px
   - input 仅下边框
   - sidebar 偏深一档
   - 字体是 Segoe UI Variable / Segoe UI
2. 选 `macOS 26`：
   - tab 仅底线高亮（无背景块）
   - sidebar 明显深于主区
   - 整体更圆滑（圆角更大）
   - 字体是 SF Pro / -apple-system
3. 系统切到 dark 模式，两个主题应自动切到 dark 变体（颜色、对比度、阴影全部跟过去）
4. 切回 `dark` / `deep-blue` / `light`：和改动前**像素级一致**

### 8.2 功能验收

- 选 `Windows 11` 后重启 app，主题应保留（`settingsStore` 持久化）
- 选 `macOS 26` 后重启 app，主题应保留
- 选 `uniterm Soft Gray` 作为终端主题，背景 `#e8e8e8`，运行 `ls --color` / `git status` 输出颜色正常无错位
- `system` 模式行为和改动前一致

### 8.3 自动化

- `npm --prefix frontend run build` 通过（捕获未定义 token、SCSS 解析错误、TypeScript 类型错误）
- `npm --prefix frontend run build` 应同时作为 CI 门禁
- 新增主题名在 6 处对齐 —— 通过手工 grep 清单核对：
  - `frontend/src/types/settings.ts`（`Theme` 联合、`TERMINAL_THEMES` 数组）
  - `frontend/src/style.css`（`[data-theme="win11"]` / `[data-theme="macos26"]` 块）
  - `frontend/src/components/Sidebar.vue`（`<el-option>` 列表）
  - `frontend/src/components/SettingsTab.vue`（`<el-option>` 列表）
  - `frontend/src/composables/useTerminal.ts`（`getXtermTheme()` case 分支）
  - `frontend/src/i18n/locales/*.json`（所有 locale 的 `settings.themeWin11` 和 `settings.themeMacOS26`）

### 8.4 兼容性

- 现有用户的 `settings.theme` 值（`dark` / `deep-blue` / `light` / `system`）不受影响 —— 联合类型扩展是加法，向后兼容
- 现有用户的 `settings.terminal.theme` 值（27 个内置之一）不受影响
- 现有 `customTerminalThemes` 不受影响