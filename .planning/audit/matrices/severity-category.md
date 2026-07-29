# Severity × Category 矩阵

每个 finding 落在一个 severity × category 单元。Synthesis 阶段填充。

## Severity 定义

| Severity | 定义 |
|---|---|
| P0 | 不可逆副作用 / 数据丢失 / 全局崩溃 / 安全 CVE / 合规阻塞 |
| P1 | 阻塞 wave / 关键路径失败 / 反复修 / 持续 dev TDD 红 |
| P2 | 单角色失败 / 非阻塞 / UX 小问题 |
| P3 | 文档错 / 注释错 / 命名错 / nitpick |

## Category 定义

| Category | 含义 |
|---|---|
| `bug` | 真实存在的 bug |
| `perf` | 性能问题 |
| `refactor` | 重构机会（设计债） |
| `deps` | 依赖版本 / 安全公告 |
| `config` | 配置合理性 |
| `os-compat` | OS 抽象 / 跨平台 |
| `test` | 测试覆盖 |
| `arch` | 架构 / 接口 / 模块边界 |
| `docs` | 文档 / UX 文案 / OSS 标准 |

## 矩阵（Synthesis 阶段填充）

|  | bug | perf | refactor | deps | config | os-compat | test | arch | docs | Total |
|---|---|---|---|---|---|---|---|---|---|---|
| **P0** | — | — | — | — | — | — | — | — | — | 0 |
| **P1** | — | — | — | — | — | — | — | — | — | 0 |
| **P2** | — | — | — | — | — | — | — | — | — | 0 |
| **P3** | — | — | — | — | — | — | — | — | — | 0 |
| **Total** | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | 0 | **0** |

## 模板（每个 finding 填写时同步更新）

```
P0 bug: — 
P0 perf: —
P0 arch: —
P1 bug: <count>
...
```

## 阈值告警

- **P0 总数 > 5** → 必须在 v1.2 紧急修，不能进 backlog
- **P1 bug > 20** → v1.2 bug fixes milestone 资源吃紧，需拆分
- **P2 总数 > 50** → 考虑批量修 vs 选择性修
- **P3 占比 > 30%** → 审计过细，需修剪