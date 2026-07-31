# Testing Patterns

**Analysis Date:** 2026-07-28

## Test Framework

**Go:**
- Standard library `testing` package only — no `testify`, `gomega`, or third-party assertion libraries
- Run command: `go test ./backend/...` (per `CLAUDE.md`); per-package: `go test ./backend/k8s/...`
- Coverage tooling: `go test -cover ./...` (standard); no coverage gate enforced
- Test files exist only under `backend/` packages; root `app.go` and `main.go` are uncovered

**TypeScript:**
- `vitest@^4.1.8` (declared in `frontend/package.json`)
- No separate config file (`vitest.config.ts`) — uses Vite defaults; `vitest` discovers `*.test.ts` next to source
- Run command: `npx vitest run` (no `npm` script alias yet; invoke via `npx`)
- No `@vue/test-utils`, no `jsdom` env override — stores/services only (no component tests observed)
- `vue-tsc` used for type-checking (separate from test runs)

**Wails bindings in tests:**
- Tests stub `../../wailsjs/runtime` (`EventsOn`) and `../../wailsjs/go/main/App` (`SessionWrite`, `LoadAIConfig`, etc.) to avoid the Wails runtime
- Pattern: `vi.mock('../../wailsjs/runtime', () => ({ EventsOn: vi.fn(() => () => {}) }))`

## Test File Organization

**Location:** co-located next to source under the same package.

**Go layout:**
```
backend/
├── k8s/
│   ├── client_test.go          # tests for client.go
│   ├── kubeconfig_test.go
│   ├── logs_test.go
│   ├── manager_test.go
│   ├── rest_test.go
│   ├── server_addr_test.go
│   └── watch_test.go
├── session/
│   ├── k8s_exec_channel_test.go
│   ├── output_log_test.go       # 27 test functions, biggest suite
│   ├── post_login_expect_test.go
│   ├── tunnel_forward_test.go
│   └── zmodem_detect_test.go
├── store/
│   └── recent_store_test.go
└── platform/
    └── fonts_ttf_test.go        # mirrored //go:build windows || darwin
```

**TypeScript layout:**
```
frontend/src/
├── services/
│   ├── k8sActions.test.ts       # next to k8sActions.ts
│   ├── k8sCrd.test.ts
│   ├── k8sMetrics.test.ts
│   ├── k8sQuantity.test.ts
│   ├── k8sResources.test.ts
│   ├── terminalAgent.test.ts
│   └── zmodemService.test.ts
└── stores/
    ├── aiStore.test.ts
    ├── k8sStore.test.ts
    └── sessionStore.test.ts
```

**Naming:**
- `<source>_test.go` for both languages (no `spec.ts`, no `_integration_test.go` split)
- One test file per source file (not per function)

## Test Structure

**Go suite organization:**
- No table-driven wrapper helpers; some tests use anonymous `[]struct{...}` slices (see `server_addr_test.go:6`, `zmodem_detect_test.go:9`)
- Helper functions declared at file scope: `ids(...)` in `tunnel_forward_test.go:53`, `buildNameTable(...)` and `utf16BE(...)` in `fonts_ttf_test.go:9`
- Setup via `t.TempDir()` for filesystem tests, `httptest.NewTLSServer` for HTTP tests
- No `TestMain` or suite bootstrap; tests are independent

**Example patterns observed:**
```go
func TestParseBytesBasic(t *testing.T) {
    raw := []byte(fmt.Sprintf(basicKubeconfig, caData, certData, keyData))
    kc, err := ParseBytes(raw)
    if err != nil {
        t.Fatalf("ParseBytes: %v", err)
    }
    if kc.CurrentContext != "dev" {
        t.Errorf("CurrentContext = %q, want dev", kc.CurrentContext)
    }
}
// backend/k8s/kubeconfig_test.go:42
```

```go
func TestRecentStore_Record_Deduplicates(t *testing.T) {
    dir := t.TempDir()
    s := NewRecentStore(dir)
    s.Record("a"); s.Record("b"); s.Record("a")
    ids := s.GetAll()
    if len(ids) != 2 {
        t.Fatalf("expected 2 unique ids, got %d: %v", len(ids), ids)
    }
}
// backend/store/recent_store_test.go:9
```

**TypeScript suite organization:**
- `describe('<unit behavior>', () => { it('<scenario>', () => {...}) })`
- `beforeEach(() => { setActivePinia(createPinia()) })` for store tests
- Constants declared at file scope (`MAX_CHUNKS = 2000`, `TRIM_TO = 1000` in `sessionStore.test.ts`)
- Factories for fixtures (`mkPod(...)` in `k8sStore.test.ts:16`)

```ts
describe('sessionStore replay tracking', () => {
  beforeEach(() => { setActivePinia(createPinia()) })
  it('getChunkCount is a monotonic sequence, not the buffer length', () => {
    const store = useSessionStore()
    const id = 'seq-monotonic'
    store.initSession(id)
    for (let i = 0; i < total; i++) store.appendData(id, `c${i}\n`)
    expect(store.getChunkCount(id)).toBe(total)
  })
})
// frontend/src/stores/sessionStore.test.ts:15
```

## Mocking

**Go mocking:**
- No mocking framework; tests use real implementations + `httptest`
- `httptest.NewTLSServer` + `startTLSServer(t, handler)` helper in `backend/k8s/client_test.go:13` — returns `(server, ca)` for k8s client certs
- Real tempdirs via `t.TempDir()`; real filesystem; real goroutines
- `net.Pipe()` for in-process network tests (`tunnel_forward_test.go:62` for SOCKS5 handshake)
- `time.After` + channels for async assertions (`logs_test.go:43`)

**TypeScript mocking (vitest):**
- `vi.mock('<module-path>', () => ({ ... }))` at top of file — declares module replacement BEFORE the SUT import
- `vi.hoisted(() => ({ ...vi.fn()... }))` for mock factories shared across `vi.mock` factory and tests (see `zmodemService.test.ts:3`, `terminalAgent.test.ts:4`)
- `vi.fn().mockResolvedValue(value)` for promise-returning stubs
- `vi.fn().mockImplementation(() => ({ ... }))` for object-returning stubs (e.g. `vi.fn(() => mockTabStore)`)
- `vi.clearAllMocks()` / `mockReset()` in `beforeEach` to isolate state

**What gets mocked (TS):**
- `../../wailsjs/runtime` — `EventsOn: vi.fn(() => () => {})` to no-op subscriptions
- `../../wailsjs/go/main/App` — Wails-generated bindings (e.g. `SessionWrite`, `LoadAIConfig`)
- `../services/k8sClient` — `requestJSON` and `startWatch` (used to verify URL/method/body shape without HTTP)
- `../i18n` — `t: (k: string) => k` to pass-through keys
- Pinia stores via `use<X>Store: vi.fn(() => mockStore)` (e.g. `useZmodemStore`, `useTabStore`)

**What does NOT get mocked:**
- Pure parsers / formatters (`parseCpu`, `formatMemory`, `parseCRD`, `evalJsonPath`, `parsePodMetricsList`) — tested directly
- Real `lineProcessor`, `ansiStripper`, `socks5Handshake` — exercised on literal byte inputs
- Real `RecentStore`, `OutputLogger` against `t.TempDir()` — verified persistence behavior

**What to mock vs what NOT to mock (guideline):**
- Mock the Wails bridge and stores (boundary)
- Test the pure logic (parsers, helpers) by direct invocation
- Test the integration seam (k8sActions ↔ k8sClient) by mocking the client and asserting URL/method/body
- Test network code by hitting a real `httptest.NewTLSServer`

## Fixtures and Factories

**Test data (TS):**
- Inline objects (k8s pods, CRDs) — no JSON fixtures on disk
- `mkPod(name, ns?, uid?)` factory in `k8sStore.test.ts:16` for compact pod fixtures
- Hardcoded literal k8s responses in each test

**Test data (Go):**
- `basicKubeconfig` const in `kubeconfig_test.go:9` — raw YAML template substituted with `fmt.Sprintf`
- Hardcoded server URL / cert / token for `startTLSServer` consumers
- ANSI escape sequences as raw bytes (`[]byte("\x1b[31mred\x1b[0m")`)

**Location:**
- Co-located with test files; no `testdata/` directory used in `backend/`
- TypeScript tests inline literal fixtures in each `describe`

## Coverage

**Requirements:** none enforced. No CI step explicitly runs coverage. The repository has no `Makefile` target for coverage.

**View coverage:**
```bash
# Go
go test -cover ./backend/...

# TS (vitest has built-in coverage if installed; not currently wired)
npx vitest run --coverage
```

**What's covered:**
- `backend/k8s/` — kubeconfig parsing, client, REST, watch, logs, manager (~14 tests)
- `backend/session/` — output log (largest, ~27), post-login expect, tunnel forward, k8s exec framing, zmodem detect
- `backend/store/` — recent store (~5)
- `backend/platform/` — fonts TTF parsing (~mirrored build tag)
- `frontend/src/stores/` — session, k8s, ai store message queue
- `frontend/src/services/` — k8s parsing, formatting, CRUD action shape, terminalAgent, zmodemService

**What's NOT covered:**
- `app.go` (root) — Wails bindings are exercised manually via `wails dev`
- `backend/session/*_session.go` — SSH/SFTP/RDP/VNC/SPICE/MongoDB/Redis/S3/WebDAV live session drivers (require real servers)
- `backend/sync/` — git push/pull, keychain — requires real GitHub/GitLab/Gitee
- `backend/update/` — in-app updater
- `backend/database/` — DSN providers, schema introspection
- `frontend/src/components/` — no Vue component tests (no @vue/test-utils dep installed)
- `frontend/src/composables/` — no test files present

## Test Types

**Unit tests (both languages):**
- Scope: pure functions, parsers, helpers, individual struct methods
- Approach: direct call, literal inputs, expected outputs
- Examples: `TestParseBytesBasic`, `TestLooksLikeZmodemHeader`, `parseCpu`, `formatMemory`, `evalJsonPath`, `parseCRD`

**Integration tests (Go):**
- Scope: k8s client over `httptest.NewTLSServer` (real TLS handshake, real HTTP, real auth headers)
- Approach: start TLS server, hand cert to `BuildClient`, exercise `Do` / `startWatchStream` / `LogStream`
- Coverage of streaming behavior with `time.Sleep` + `time.After` timeouts

**Integration tests (TS):**
- Scope: store ↔ service interactions (k8sStore ↔ k8sClient mocks)
- Approach: `setActivePinia(createPinia())`, then drive `subscribe('c1', 'pods', 'default')` and assert on `getItems` / `getError`
- `terminalAgent.test.ts` exercises `watchOutput` against a faked `Date.now` / `Math.random` / `EventsOn` to test cancellation, idle detection, timeout

**End-to-end tests:** Not present. Manual QA via `wails dev` is the workflow (see `CLAUDE.md`).

## Common Patterns

**Async testing (Go):**
```go
errCh := make(chan error, 1)
go func() { errCh <- runPostLoginExpectAutomation(ctx, cfg) }()
output.Append([]byte("Welcome\r\n"))
select {
case err := <-errCh:
    if err != nil { t.Fatalf("...: %v", err) }
case <-time.After(time.Second):
    t.Fatal("... did not finish")
}
// backend/session/post_login_expect_test.go:14
```

**Async testing (TS):**
```ts
it('subscribe(pods, default) lists then getItems returns items', async () => {
  requestJSON.mockResolvedValue({ status: 200, data: { items: [...] }, raw: '' })
  const s = useK8sStore()
  await s.subscribe('c1', 'pods', 'default')
  expect(s.getItems('c1', 'pods', 'default').length).toBe(2)
  expect(requestJSON).toHaveBeenCalledWith('c1', 'GET', '/api/v1/namespaces/default/pods?limit=500')
})
// frontend/src/stores/k8sStore.test.ts:33
```

**Fake timers:**
- `vi.useFakeTimers()` in `zmodemService.test.ts:68` for testing chunked download batching
- `Date.now = vi.fn(() => MOCK_TIMESTAMP)` in `terminalAgent.test.ts:79` to control timestamps deterministically

**Error testing (Go):**
```go
if got != want {
    t.Errorf("Strip = %q, want %q", got, want)
}
if _, _, ok := decodeFrame([]byte{}); ok {
    t.Fatalf("empty frame should not decode")
}
```

**Error testing (TS):**
```ts
expect(s.getError('c1', 'no-such-thing', 'default')).toMatch(/unknown resource/i)
expect(requestJSON).not.toHaveBeenCalled()
// frontend/src/stores/k8sStore.test.ts:72
```

**Regression tests:**
- Tagged with `// Regression test for issue #242` style comments above the test function
- Pin the exact bytes/strings that previously caused the bug

**Table-driven tests (Go):**
```go
tests := []struct {
    name string
    data []byte
    want bool
}{
    {"real ZRQINIT", []byte("**\x18B00000000000000"), true},
    {"vim content stars+hex (no ZDLE)", []byte("value: **B0a1b2c3d4e5f60718"), false},
    // ...
}
for _, tt := range tests {
    if got := looksLikeZmodemHeader(tt.data); got != tt.want {
        t.Errorf("%s: ...", tt.name)
    }
}
// backend/session/zmodem_detect_test.go:8
```

**Test assertion style:**
- Go: `t.Errorf` for assertion mismatches, `t.Fatalf` to stop the test on unrecoverable setup error
- TS: `expect(...).toBe(...)`, `toEqual(...)`, `toHaveBeenCalledWith(...)`, `toContain(...)`, `toMatch(/.../)`, `not.toHaveBeenCalled()`

## Conventions Summary

- **Test the public surface** — exported functions / store actions
- **Pure functions get pure tests**; the SUT receives literal input and produces literal output
- **Stores get `setActivePinia(createPinia())` reset** between tests
- **Wails boundary is always mocked** in TS tests so the runtime never tries to spin up
- **HTTP boundary is real** in Go tests via `httptest.NewTLSServer` (k8s client)
- **Race conditions and timing** are tested with channels + `time.After` (Go) or fake timers (TS)
- **Bug fixes** add a regression test tagged with `// Regression test for issue #NNN`

---

*Testing analysis: 2026-07-28*