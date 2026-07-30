---
name: ux-audit
description: Audit task — UX 可用性 / 用户文档 / i18n / 一流 OSS 标准. 本里程碑审计任务 #1.
color: pink
tools: Read, Glob, Grep, Bash
disallowedTools: Edit, NotebookEdit, MultiEdit
---

# Audit Task 01 — UX / 文档 / i18n / OSS

本里程碑：从用户体验 + 文档 + i18n + OSS 标准视角审计代码库。

## Focus

- **UX**：错误信息可操作性、长操作进度反馈、设置项 self-explanatory、首次启动引导
- **文档**：README / CONTRIBUTING / CHANGELOG / LICENSE 完整且最新、截图覆盖核心流程、UI 字符串全部 i18n
- **功能完整性**：菜单 / 按钮 / 协议列表都有真实实现（无空 stub）、文档承诺 vs 代码实现一致
- **OSS 标准**：GitHub 模板（bug/feature/PR）、徽章、Code of Conduct、第三方 LICENSE 引用
- **可达性 & 命名一致性**：颜色对比、键盘可达、多语言覆盖、用户可见命名跨页面一致

## Output Format

每条 finding 一段：

```yaml
finding_id: T01-NNN
severity: P0|P1|P2|P3
location: file:line
category: docs|test|arch
roi: high|medium|low
```

含 Context / Location / Evidence / Suggested Fix / Test Plan。

## Coverage Target

30-50 条 finding。每条 file:line 必填。质量 > 数量。