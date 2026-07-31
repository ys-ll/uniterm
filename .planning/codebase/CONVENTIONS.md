# Coding Conventions

**Analysis Date:** 2026-07-28

## Naming Patterns

**Go files:**
- Snake case lowercase: `ssh_session.go`, `post_login_expect.go`, `output_log.go`
- Build-tag-split files end with platform: `_windows.go`, `_unix.go`, `_darwin.go`, `_notdarwin.go`, `_notwindows.go`, `_stub.go` (e.g. `rdp_session_stub.go` for `!windows`)
- Single `_test.go` suffix; no separate `_integration_test.go` split observed
- One primary type per file when feasible (`session.go` holds `Session`, `manager.go` holds `SessionManager`)

**Go functions / types / vars:**
- Exported identifiers: PascalCase (`SaveConnections`, `ConnectionConfig`, `SessionStatus`)
- Unexported helpers: camelCase (`runPostLoginExpectAutomation`, `pushChunk`, `newPostLoginOutputBuffer`)
- Constructors: `New<Type>(...)` returning `*Type` (e.g. `NewSessionManager`, `NewApp`, `NewRDPSession`)
- Constants: PascalCase with `Status`/`Type` prefix for typed string enums (`StatusConnecting`, `StatusError`) — see `backend/session/session.go:14`
- Setters/getters: `Set<X>` / `Get<X>` / `Is<X>` (e.g. `SetEventEmitter`, `IsConnected`, `GetStatus`)

**TypeScript files:**
- Service files: PascalCase for module name + lowercase extension (`k8sClient.ts`, `terminalAgent.ts`, `llm.ts`)
- Pinia stores: `<thing>Store.ts` (`sessionStore.ts`, `k8sStore.ts`, `aiStore.ts`, `tabStore.ts`)
- Vue components: PascalCase with descriptive suffix (`BaseTerminal.vue`, `K8sResourceList.vue`, `StartTabContent.vue`)
- Composables: `use<Camel>` (`useTerminal*` family in `frontend/src/composables/`)

**TypeScript identifiers:**
- Functions: camelCase (`startWatch`, `requestJSON`, `parsePodMetricsList`)
- Types / interfaces: PascalCase (`K8sContextInfo`, `WatchHandle`, `ParsedCRD`)
- Type-only imports use `import type { ... }` — see `frontend/src/services/k8sCrd.ts:2` (`import type { ParsedCRD } from '../types/k8s'`)
- File-private state for stores: module-level `reactive({ ... })` plus a `defineStore(...)` factory (see `frontend/src/stores/sessionStore.ts:32`)

**Vue components:**
- Two-word names preferred; feature prefix indicates area: `K8s*`, `SFTP*`, `RDP*`, `DB*`, `AI*`, `QuickCommand*`, `Sync*`
- Dialog components end in `Dialog` (`AddRepoDialog`, `ChangePasswordDialog`, `K8sCreateDialog`)
- Tab content views end in `TabContent` (`TerminalTabContent`, `K8sTabContent`, `MongoDBTabContent`)
- Panel components end in `Panel` (`Sidebar`, `QuickCommandsPanel`, `TunnelsPanel`)

## Code Style

**Go formatting / linting:**
- No `.golangci.yml`, `.gofmt` config, or pre-commit hook present — assume `gofmt` defaults
- Imports grouped stdlib then third-party, no blank line between groups inside parens (see `app.go:3`)
- `gofmt` style: tabs, aligned struct field comments, tabbed `case` blocks

**TypeScript formatting / linting:**
- No ESLint/Prettier config found; `frontend/tsconfig.json` enforces: `strict`, `noUnusedLocals`, `noUnusedParameters`, `noFallthroughCasesInSwitch`
- `frontend/vite.config.ts` defines `import.meta.env.VITE_VERSION`
- Use `as` casts sparingly; prefer typed bindings (`as K8sContextInfo[]` only at Wails boundary)
- `let` and `const` only; no `var` (look at any source file for the pattern)

**Comments / docs:**
- Go doc-comments above exported funcs/types: `// SaveConnections persists the ...` style — sparse but present
- Inline comments explain WHY (workarounds, race conditions, platform quirks) more than WHAT
- Frontend comments are bilingual EN/CN acceptable; some source files contain Chinese-only comments inside `services/` (`k8sStore.ts`, `k8sMetrics.ts`) — these describe intent, not mechanics
- Avoid noisy self-explanatory comments; one-liners only when intent is non-obvious (project rule per CLAUDE.md: "注释宜少不宜多")

## Import Organization

**Go:**
1. Standard library
2. Third-party packages
3. Internal packages (`github.com/ys-ll/uniterm/backend/...`)
- Long imports may use line-continuation; `app.go:3` has all imports in a single `import (...)` block

**TypeScript:**
1. `vue` / `pinia` / framework (`import { defineStore } from 'pinia'`, `import { ref } from 'vue'`)
2. Generated Wails bindings (`from '../../wailsjs/go/main/App'`, `from '../../wailsjs/runtime'`)
3. Project types (`import type { ... } from '../types/...'`)
4. Project stores / services / composables (`from '../stores/...'`, `from '../services/...'`)
- Mocks for tests go to the TOP of the test file, BEFORE the SUT import — see `frontend/src/services/terminalAgent.test.ts:9-67`

**Path aliases:**
- None configured; all imports are relative (`./`, `../`, `../../`)
- `@` aliases are not used — verified absence in `tsconfig.json`

## Error Handling

**Go (backend):**
- Functions return `error` as the last value; idiomatic `(T, error)` or `error` only
- Errors constructed with `fmt.Errorf("context: %w", err)` (wrapping) or `fmt.Errorf("static message %s", val)` (format)
- Examples:
  - `backend/session/ftp_session.go:72` — `return fmt.Errorf("ftp dial (TLS required): %w", err)`
  - `backend/session/session.go` (const block) — sentinel `SessionStatus` string types instead of typed errors for status
- Sentinel / static errors: minimal — typed string enums serve most cases (`SessionStatus`, `TunnelState`)
- Logging: `backend/log/log.go` exposes `log.Writef(format, args...)` — used everywhere for non-fatal errors (`log.Writef("Failed to init connection store: %v", err)`)
- Init errors at startup: `fmt.Printf("WARN: ...")` followed by `return` to skip feature (see `app.go:100-102`)
- Tests use `t.Fatalf("X: %v", err)` for unrecoverable and `t.Errorf("X = %q, want %q", got, want)` for assertion mismatches

**TypeScript (frontend):**
- Pure helpers throw `new Error(\`HTTP ${status}: ${raw?.slice(0, 300) || ''}\`)` — see `frontend/src/services/k8sActions.ts:10`
- Custom error classes for expected control flow: `ChatCancelledError`, `ChatTimeoutError` in `frontend/src/services/llm.ts:16-28`
- Stores use try/catch with `console.error('Failed to <action>:', e)` — pattern across `connectionStore.ts`, `commandStore.ts`, `tunnelStore.ts`, `skillStore.ts`, `quickCommandStore.ts`, `syncStore.ts`
- Wails bindings surface as `error` returns — caught at store call sites; UI shows i18n toast
- Async service functions return `Promise<T>` and reject; no Result-style wrappers

## Logging

**Backend (`backend/log/log.go`):**
- Single package with `Init()`, `Writef()`, `Close()`
- File-based, mutex-guarded writes (no leveled logger / slog used directly)
- Use `log.Writef("[Tag] message %v", err)` format — bracketed tag is conventional (e.g. `[CreateSession]` in `app.go:980`)
- Wails `runtime.LogInfo` / `runtime.LogError` are NOT used in `app.go`; everything goes through `log.Writef`
- `fmt.Printf` reserved for init-stage warnings before `log.Init()` succeeds (`app.go:101`)

**Frontend:**
- `console.error('Failed to <action>:', e)` is the de-facto pattern in every Pinia store
- `console.log` / `console.warn` not observed in source files outside store error handling
- User-visible errors route through i18n toast (`t('error.xxx')`)

## Comments

**When to comment:**
- Workaround rationale (e.g. session log migration, ANSI stripper split-chunk handling)
- Race-condition guards ("竞态守卫：期间被 unsubscribe 就丢弃结果" — `k8sStore.ts:68`)
- Platform-specific quirks (Windows code page, ConPTY, hidden console — `local_session_windows.go:32`)
- Issue references (`// Regression test for issue #242` — `backend/session/zmodem_detect_test.go:5`)

**JSDoc/TSDoc:**
- Sparse; functions mostly rely on signature
- Where used, `/** ... */` describes invariants (e.g. `estimateTokens` in `aiStore.ts:9-13`)
- File-top banner comments are bilingual (path + intent): `// frontend/src/services/k8sActions.ts`

## Function Design

**Go:**
- Receivers are 1-2 letters: `func (a *App)`, `func (sm *SessionManager)`, `func (s *SSHSession)`
- Method receivers consistent across a type's methods (all `(s *SSHSession)` or all `(s SSHSession)` for value types)
- Public methods short; private helpers absorb complexity
- Constructors return `*T`; clone semantics via `&T{...}` literal

**TypeScript:**
- Async functions use `async/await` exclusively; `.then().catch()` chains appear only when interfacing with Wails event subscriptions
- `function` declarations for module exports; arrow functions reserved for inline callbacks
- Currying / decorators not used
- Test functions: `it('does X', () => { ... })` inside `describe('<unit>', () => { ... })`

## Module Design

**Go exports:**
- Package-level exported names serve as the API
- Companion types colocated in the same file or a `types.go` (`backend/session/session.go`)
- Interfaces prefixed by capability (`Provider`, `execer` in `backend/database/provider.go`); small and role-focused

**TypeScript exports:**
- `export function name(...)` for plain functions (services)
- `export interface TypeName` for shapes
- `export const useStore = defineStore(...)` for Pinia
- `export default class` only for Vue components / module-style exports
- No barrel `index.ts` re-export pattern observed; consumers import directly from source files

**Barrel files:**
- Not used; verified absence in `frontend/src/services/` and `frontend/src/stores/`

## Specific Patterns Observed

**Pinia store wiring:**
- `defineStore('<name>', () => { ... return { ...actions, ...refs } })` setup style — see `k8sStore.ts:15`, `sessionStore.ts:57`
- Module-level `EventsOn(...)` subscription registered at import time for lifecycle events — see `sessionStore.ts:39` (`session:status`, `session:data`)
- Test setup: `setActivePinia(createPinia())` in `beforeEach` to reset state per test

**Service layer pattern:**
- Thin wrapper modules that call `wailsjs/go/main/App` and shape responses (`k8sClient.ts` -> `k8sActions.ts`, `k8sMetrics.ts`, `k8sResources.ts`, `k8sCrd.ts`)
- Pure parsers/formaters extracted from async wrappers (`k8sMetrics.parsePodMetricsList` vs `fetchPodMetrics`)
- All network functions take a `connId: string` as first arg to identify the active backend session

**Resource descriptor pattern (k8s):**
- `ResourceDescriptor` table in `frontend/src/services/k8sResources.ts` registers every K8s resource; columns, paths, and detail sections are data, not component code
- Adding a new resource is one entry in `RESOURCES` array — no new Vue component

**State mutability guards:**
- `states.value.get(key) !== st` checks guard against unsubscribe-during-fetch races (`k8sStore.ts:69`)
- `bump(key, st)` re-creates Map slot to force reactivity (`k8sStore.ts:37`)

**Build-tag splitting (Go):**
- `//go:build windows` — `local_session_windows.go`, `fonts_windows.go`
- `//go:build !windows` — `local_session_unix.go`, `rdp_session_stub.go`
- `//go:build darwin` — `fonts_darwin.go`
- `//go:build !windows && !darwin` — `fonts_unix.go`
- Tests mirror the build tags when exercising platform code: `backend/platform/fonts_ttf_test.go:1 //go:build windows || darwin`

**Naming for fallback variants:**
- Files that exist on platforms OTHER than the tagged one use `_not<platform>.go` (e.g. `app_notdarwin.go`, `app_notwindows.go`); they hold the `//go:build !<platform>` form of the equivalent code

## Vue / TypeScript Conventions

**Component props / emits:**
- `defineProps<{ ... }>()` and `defineEmits<{ ... }>()` typed forms
- Avoid `any`; use `unknown` + narrowing at boundaries

**Reactive state inside components:**
- Prefer `ref()` for primitives, `reactive()` for grouped state
- `computed()` for derived values
- Watchers (`watch`, `watchEffect`) used for event subscriptions; cleanup in `onUnmounted` or via returned cleanup callback

**i18n:**
- `t('key.path')` from `frontend/src/i18n/` — keys mirror UI structure
- Component templates use `{{ t('foo.bar') }}` for user-facing strings

---

*Convention analysis: 2026-07-28*