# Multi-Panel AI Lock Design

## 概述

当前 AI 通过单个锁定按钮绑定到一个终端面板，所有命令都发到该面板。本设计将锁定从单值扩展为集合，允许 AI 同时操作多个面板。交互方式不变——现有锁定按钮改为多选，用户无学习成本。

## 1. 数据模型

### tabStore.ts

`aiLockedPanelId: string | null` 改为 `aiLockedPanelIds: Set<string>`。

```ts
// 状态
aiLockedPanelIds: new Set<string>()

// Getter
getAILockedPanels(): string[]     // 返回所有锁定面板 ID
isPanelAILocked(id: string): boolean

// Setter
addAILockedPanel(id: string)
removeAILockedPanel(id: string)
clearAILockedPanels()
```

**向后兼容**：保留 `aiLockedPanelId` computed，返回集合中第一个（或 null）。现有单值和 toggle 风格的代码无需修改。

### tab 关闭清理

`closeTab` 中，遍历已关闭面板并逐个从集合中移除。其他清理逻辑同上。

## 2. 锁定按钮（Panel.vue + TabItem.vue）

### 交互

外观不变——`✨` Sparkles 按钮。toggle 逻辑从"点 A 取消 B"改为纯粹的集合 add/delete：

- 点未选中面板 → 加入集合
- 点已选中面板 → 从集合移出
- 集合为空 → AI 跟随活跃面板（和现在没有锁定一样）

Toggle 事件和 handler（App.vue / TerminalTabContent.vue / WorkspaceContent.vue / TabBar.vue）同步改动。

### 视觉

- 未选中：灰色 `✨`
- 已选中：金色 `✨`（和现在锁定态一致）
- 面板标题栏左侧金色边框在选中时保持

TabItem.vue 上，只要 tab 里任一 panel 被锁定就亮。

## 3. AI 侧边栏标签区域（AISidebar.vue）

### 位置

消息列表下方、输入框上方。

### 结构

标签区域使用 Element Plus Tag 组件，右侧固定 `[+]` 下拉按钮。

### 三种状态

**默认**（无锁定面板，当前 tab 是终端）：

```
┌─ 关联终端 ──────────────────────────┐
│ [当前终端]                    [+] ▼  │
└──────────────────────────────────────┘
```

`[当前终端]` 是不可删除的 tag，表示 AI 跟随活跃面板。

**已关联**（有锁定面板）：

```
┌─ 关联终端 ──────────────────────────┐
│ [Server-A ✕] [Server-B ✕]    [+] ▼  │
└──────────────────────────────────────┘
```

一旦添加了面板，`[当前终端]` 消失，替换为面板名 tag。每个可 `✕` 删除。全部删除后恢复为默认。

**无终端**（无锁定面板 且 当前 tab 非终端）：

灰色文字："请切换到终端标签页，或点击 [+] 添加关联终端。"

### 交互

- `[+]` 下拉：列出所有终端/SSH 面板（标题 + shell 类型），勾选即锁定，取消即解锁
- tag 上的 `✕`：取消该面板的锁定
- 和 Panel.vue 锁定按钮双向同步——侧边栏操作 → 面板按钮亮，反之亦然

下拉列表每个面板显示标题（重名时追加 id 后缀，如 `Server (panel-a1)`），对应 shell 类型。

## 4. 终端工具 panel 参数

### Schema（llm.ts）

6 个终端工具全部加可选 `panel` 字段：

```ts
panel: {
  type: 'string',
  description: 'Target panel by its title. Omit to use the default panel (first locked or active).'
}
```

不涉及面板的工具（`ask_user`）不加。

### resolveActiveSession（terminalAgent.ts）

```ts
function resolveActiveSession(panelTitle?: string): {
  sessionId: string
  shellPath?: string
}
```

匹配规则：
1. 如果有 `panelTitle`，在所有已加载面板中按标题精确匹配。支持 `title (id: xxx)` 后缀精确区分同名面板。
2. 如果无 `panelTitle` 但有锁定面板集，返回第一个锁定面板。
3. 无锁定面板，返回当前活跃面板。
4. 都没匹配到，throw Error。

### 调度（agent.ts）

每个终端工具 dispatch 从 `tu.input.panel` 取目标，传给 `resolveActiveSession()`。

## 5. 动态上下文

### buildDynamicContext（agent.ts）

从单面板信息改为面板列表。只展示 AI 可见的面板（有锁定时列所有锁定面板，无锁定时列活跃面板）。

```
AVAILABLE PANELS:
  1. "Server-A" [Bash] [SSH: root@192.168.1.100]
  2. "Server-B" [Bash]
  3. "Local"    [PowerShell]
```

重名面板标注 id 后缀：
```
  1. "Server (panel-a1)" [Bash]
  2. "Server (panel-b2)" [SSH: root@10.0.0.1]
```

面板来源规则：
- 有锁定面板 → 列出所有锁定面板
- 无锁定面板 → 只列出活跃面板

SSH 信息从 `panel.config.host/user/port` 读取。非 SSH 面板不显示。

## 6. 文件清单

| 文件 | 改动 |
|---|---|
| `tabStore.ts` | `aiLockedPanelId` → `aiLockedPanelIds: Set`；新增 add/remove/clear/isLocked；保留 `aiLockedPanelId` computed |
| `Panel.vue` | `isAILocked` 改为 check set |
| `TabItem.vue` | locked 判断改为 any match |
| `AISidebar.vue` | 新增关联终端标签区域 |
| `terminalAgent.ts` | `resolveActiveSession` 加可选 `panelTitle` 参数 |
| `llm.ts` | 6 个终端工具 schema 加可选 `panel` |
| `agent.ts` | `buildDynamicContext` 列所有可用面板；dispatch 传 `panel` |
| `App.vue` | toggle handler 改为 add/remove |
| `TerminalTabContent.vue` | 同上 |
| `WorkspaceContent.vue` | 同上 |
| `TabBar.vue` | 同上 |

## 7. 向后兼容

- `aiLockedPanelId` computed 返回第一个锁定面板或 null，依赖它的代码无需改动
- `panel` 参数可选——省略时走默认逻辑（和现在完全一致）
- 不勾任何面板 = 现在的不锁定行为
- 勾一个面板 = 现在的锁定行为
- 勾多个面板 = 新功能
