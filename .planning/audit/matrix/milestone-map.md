# Milestone × Finding 矩阵

把 finding 按未来修复 milestone 分类。启动对应 milestone 时从这里抽列。

## 当前路由（v1.1 Audit 终态 · 97 条）

### v1.2.1 Emergency Security Patch（P0 + Security 必修 · 9 条）

| Finding | Severity | Category | Notes |
|---|---|---|---|
| REV-015 | P0 | security | **P0 必修，SSH 漏洞** |
| REV-016 | P1 | security | AI XSS（DOMPurify）|
| REV-017 | P1 | security | k8s TLS verify |
| REV-018 | P1 | security | local shell command injection |
| REV-019 | P1 | security | SFTP path traversal |
| REV-020 | P2 | security | SFTP client traversal |
| REV-021 | P2 | security | sync URL SSRF |
| REV-022 | P2 | security | docker volume traversal |
| REV-023 | P2 | security | markdown link noopener |
| REV-024 | P2 | security | markdown image src |
| REV-025 | P3 | security | shard path traversal |
| REV-026 | P2 | bug/correctness | ai_session 错误处理 |

### v1.2 Bug Fixes（紧急 · ~17 条）

| Finding | Severity | Category | Notes |
|---|---|---|---|
| F-001 IPv6 dial | P1 | bug | 7 处 + dialAddr helper |
| F-006 SQL Sprintf | P1 | bug/security | 走 prepared statement |
| F-008 error swallowed | P2 | bug | 加 log |
| F-009 goroutine no recover | P2 | bug | 加 defer recover |
| F-010 k8s map race | P1 | bug | 加锁覆盖 |
| F-012 store atomic_write | P1 | arch | 确认覆盖度 |
| F-014 EventsOn 泄漏 | P1 | bug | composable 统一 |
| ARCH-015 13 session 日志 | P1 | arch | 提 helper |
| ARCH-018 平台路径硬编码 | P2 | os-compat | shell abstraction |
| ARCH-022 store 8 处 _ = err | P2 | tech-debt | log.Warn |
| DBG-015 ~ DBG-024（10 条） | P2-P1 | bug | 可复现 |
| ARCH-016 Session interface ctx | P2 | arch/refactor | 改 interface |

### v1.3 Performance（14 条）

| Finding | Severity | Notes |
|---|---|---|
| DEV-015 ~ DEV-040（17 条 primary，撤回 4） | P1-P2 | 量化收益都有 |
| DEV-029 DB 连接池 | P1 | 性能影响最大 |
| DEV-037 resize 7 次 retry | P1 | 7 → 1 次 |
| DEV-019/020 前端响应式深度 | P1 | 5-10x perf |
| DEV-040 mouse tracking scan | P2 | 12% 单核 |

### v1.4 Refactor（~13 条）

| Finding | Severity | Notes |
|---|---|---|
| MAP-015 ~ MAP-019（5 条） | P2-P3 | 死代码/RTM |
| ARCH-024 retry 抽象 | P2 | 提 helper |
| ARCH-025 container OS runner 统一 | P2 | runner.go interface |
| ARCH-028 app.go 拆 god object | P2 | 按领域拆 |
| ARCH-029 session status 转换 | P2 | 状态机 |
| ARCH-032 LLM streaming 抽象 | P2 | interface |
| ARCH-033 frontend data-testid | P3 | 抽 util |
| ARCH-034 命名一致性 | P3 | session 命名 |

### v1.5 Dependency Updates（4 条）

| Finding | Severity | Notes |
|---|---|---|
| F-002 Go deps | P2 | ~80 minor 升级，分批 |
| F-003 npm deps | P2 | pinia 2→4 + vite 5→6 |
| F-004 npm audit | P3 | CI 严格 |
| ARCH-030 sync PBKDF2 | P2 | x/crypto/pbkdf2 |

### v1.6 OS Compatibility（3 条）

| Finding | Severity | Notes |
|---|---|---|
| ARCH-018 路径硬编码 | P2 | 已归 v1.2 |
| ARCH-025 container OS | P2 | 已归 v1.4 |
| ARCH-026 log rotation | P2 | lumberjack |

### v1.7 Test Coverage Boost（~16 条）

| Finding | Severity | Notes |
|---|---|---|
| F-007 sync 0 测试 | P1 | QA-017 给具体设计 |
| F-011 多包 0 测试 | P2 | 3 个包 |
| F-013 container 测试 | P2 | runner 多 OS |
| QA-015 ~ QA-024（10 条） | P1-P2 | 具体测试用例 |

### v1.8 Architecture（11 条）

| Finding | Severity | Notes |
|---|---|---|
| F-012 store atomic | P1 | 已归 v1.2 |
| ARCH-019 DB provider | P1 | 抽 engine.go |
| ARCH-020 store 重复 | P1 | 抽 atomic.go |
| ARCH-024 retry | P2 | |
| ARCH-025 runner | P2 | |
| ARCH-026 log | P2 | |
| ARCH-027 git CLI | P2 | |
| ARCH-028 god object | P2 | |
| ARCH-029 status | P2 | |
| ARCH-030 crypto | P2 | |
| ARCH-031 watch | P1 | |
| ARCH-032 LLM | P2 | |
| ARCH-033 data-testid | P3 | |
| ARCH-035 frontend 错误处理 | P2 | |

### v1.9 Documentation / OSS（10 条）

| Finding | Severity | Notes |
|---|---|---|
| F-005 OSS files | P2 | rejected（文件已存在）|
| PM-015 README Quick Start | P2 | |
| PM-016 README_zh-CN | P2 | |
| PM-017 AISidebar 硬编码 | P2 | |
| PM-018 StartTab 引导 | P2 | |
| PM-019 9 locale 一致 | P1 | |
| PM-020 i18n fallback | P2 | |
| PM-021 aria-label | P2 | |
| PM-022 i18n 漏译 | P3 | |
| PM-023 SECURITY.md + dependabot | P1 | |
| PM-024 THIRD_PARTY_NOTICES LGPL | P1 | 合规 |

## 累计进度

| Milestone | Total | Done | Verified | Open |
|---|---|---|---|---|
| v1.2.1 Emergency Security | 12 | 0 | 0 | 12 |
| v1.2 Bug Fixes | 17 | 0 | 0 | 17 |
| v1.3 Performance | 17 | 0 | 0 | 17 |
| v1.4 Refactor | 13 | 0 | 0 | 13 |
| v1.5 Deps | 4 | 0 | 0 | 4 |
| v1.6 OS Compat | 3 | 0 | 0 | 3 |
| v1.7 Test | 16 | 0 | 0 | 16 |
| v1.8 Arch | 11 | 0 | 0 | 11 |
| v1.9 Docs / OSS | 10 | 0 | 0 | 10 |
| **Total** | **97** | **0** | **0** | **97** |