# Role × File 覆盖矩阵

每个 subagent 写完 finding 后，列出自己审了哪些文件/包。Synthesis 阶段确保每个文件至少被 2 个不同角色审过。

## PM (Product Manager)

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**: (PM 没扫但其他角色应该补的)

## Architect

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

## Developer (Full-stack)

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

## QA

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

## Reviewer

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

## Debugger

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

## Mapper

- **Files scanned**: (填充)
- **Findings count**: 0
- **P0/P1/P2/P3**: —/—/—/—
- **Coverage gaps**:

---

## 交叉覆盖矩阵（Synthesis 阶段填充）

每个核心文件被哪些角色审计过：

| File | PM | Architect | Developer | QA | Reviewer | Debugger | Mapper | Coverage |
|---|---|---|---|---|---|---|---|---|
| (e.g.) `backend/session/ssh_session.go` | — | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | 6/7 |
| | | | | | | | | |

## 目标

每个核心文件至少被 3 个不同角色审过。如果某文件只被 1 个角色审过，需在 Synthesis 阶段补审。