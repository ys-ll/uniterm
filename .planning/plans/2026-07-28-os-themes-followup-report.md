# OS Themes Followup Report — 2026-07-28

Branch: `refactor/codebase-audit`
Working dir: `/Users/coderstory/CodeSource/uniterm`
HEAD before: `06b6534`
HEAD after: <filled at commit time>

## Scope

Two small additions in one commit, both approved by the user:

1. Strengthen the Win11 light theme's "blue + white" feel in `frontend/src/style.css`.
2. Add a new `uniterm Windows 11` terminal color scheme (terminal theme picker).

No other CSS, no macOS26 blocks, no existing 3 themes, no existing 28 terminal
themes, no spec, and no plan were touched.

## Task 1 — Win11 light: cooler blue + white

File: `frontend/src/style.css`, light block under `[data-theme="win11"]`
(lines ~246–304) and the body Mica gradient (line ~360).

| Token                | Before      | After       |
| -------------------- | ----------- | ----------- |
| `--bg-base`          | `#fafafa`   | `#f3f7fc`   |
| `--bg-elevated`      | `#f3f3f3`   | `#eaf2fb`   |
| `--bg-overlay`       | `#ebebeb`   | `#e0eaf5`   |
| `--bg-hover`         | `#e5e5e5`   | `#d8e4f3`   |
| `--bg-active`        | `#d9d9d9`   | `#c5d8ec`   |
| `--accent`           | `#0078d4`   | `#0067c0`   |
| `--accent-glow`      | `rgba(0,120,212,0.22)` | `rgba(0,103,192,0.24)` |
| `--accent-subtle`    | `rgba(0,120,212,0.08)` | `rgba(0,103,192,0.10)` |
| `--info`             | `#006ab1`   | `#005ba1`   |
| body gradient        | `linear-gradient(180deg,#fafbfc 0%,#f3f3f3 100%)` | `linear-gradient(180deg,#e8f0fa 0%,#d8e4f3 100%)` |

The dark variant under `@media (prefers-color-scheme: dark) [data-theme="win11"]`
was left untouched, as were the macOS26 blocks and the existing 3 themes (`:root`,
`deep-blue`, `light`).

### Build verification (Task 1 alone)

`npm --prefix frontend run build` → succeeds. 3680 modules transformed, no
errors. Only pre-existing warnings (dynamic-vs-static import, chunk size).

## Task 2 — `uniterm Windows 11` terminal theme

Three code locations, plus one new vitest file.

### Type union (`frontend/src/types/settings.ts:8`)

`TerminalTheme` extended from a 28-id union to a 29-id union by inserting
`'uniterm-windows11'` immediately after `'uniterm-soft-gray'`.

### TERMINAL_THEMES array (`frontend/src/types/settings.ts:203`)

New entry inserted immediately after `uniterm-soft-gray`:

```ts
{ label: 'uniterm Windows 11', value: 'uniterm-windows11', type: 'dark' },
```

This places it in the "Dark" group of the terminal theme picker, alongside the
other uniterm-* entries.

### `getXtermTheme` resolver (`frontend/src/composables/useTerminal.ts`)

New `case 'uniterm-windows11':` branch inserted **immediately after** the
`uniterm-soft-gray` case (i.e. before the `uniterm-light` case), returning the
Windows Terminal 11 default color scheme:

- bg `#0c0c0c`, fg `#cccccc`, cursor `#ffffff`
- selectionBackground `rgba(255,255,255,0.4)`
- black `#0c0c0c`, red `#e74856`, green `#16c60c`, yellow `#f9f1a5`,
  blue `#3b78ff` (signature Win Terminal accent blue), magenta `#b4009e`,
  cyan `#61d6d6`, white `#cccccc`
- brightBlack `#767676`, brightRed `#e74856`, brightGreen `#16c60c`,
  brightYellow `#f9f1a5`, brightBlue `#3b78ff`, brightMagenta `#b4009e`,
  brightCyan `#61d6d6`, brightWhite `#f2f2f2`

Shape: 20 fields (4 base + 16 ANSI), matching xterm.js's `ITheme` and matching
the `useTerminal.soft-gray.test.ts` assertions.

### Vitest coverage (`frontend/src/composables/useTerminal.windows11.test.ts`)

New file with the same 11 `vi.mock` calls as `useTerminal.soft-gray.test.ts`
(xterm addons, Wails bindings, stores, useHighlight, cursor util) so
`getXtermTheme` can be exercised in pure Node.

The single test asserts:
- bg/fg/cursor exact hex values
- `blue === '#3b78ff'` and `brightBlue === '#3b78ff'` (Win Terminal accent)
- object key set matches the 20-field ITheme shape exactly

### TDD log

| Step                                            | Result |
| ----------------------------------------------- | ------ |
| Add type union (no resolver yet)                | compile clean |
| Create `useTerminal.windows11.test.ts`, run     | **RED** — `theme.background` was `'var(--bg-base)'` (default branch) |
| Add `case 'uniterm-windows11'` branch + entry   | **GREEN** — 1/1 pass |
| Re-run `useTerminal.soft-gray.test.ts` alongside | **GREEN** — 2/2 pass (no regression) |

## Final verification

- `npx vitest run src/composables/useTerminal.windows11.test.ts src/composables/useTerminal.soft-gray.test.ts` → 2/2 pass
- `npm --prefix frontend run build` → succeeds. 3680 modules transformed.

## Commit

`feat(theme): strengthen Win11 blue+white + add uniterm Windows 11 terminal`

The commit body explains both changes, includes the Win11 brand-blue rationale,
mentions the new vitest coverage, and ends with the bilingual `---` separator
required by `CONTRIBUTING.md`.

## Concerns

None.