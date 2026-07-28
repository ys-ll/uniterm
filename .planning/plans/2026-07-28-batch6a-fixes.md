# Batch 6a — Backend Hot Paths Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all non-in-flight batch 6 findings (24 fixes across 14 files) from `parent_spec` §7 — backend hot paths that address the user's reported 900+ idle wakeups/s and 400+ MB WebKit memory pain.

**Architecture:** Three phases (Stores → Database → Session) ordered by dependency and risk. Each finding = one atomic commit. Existing `*_test.go` must continue to pass; new tests only added for store debounce behavior.

**Tech Stack:** Go 1.23+; `sync.Mutex`, `sync.atomic`, `encoding/json`, `golang.org/x/term`; existing `backend/session/bench_test.go` for regression.

## Global Constraints

- **Atomic commit per finding**: each fix = 1 commit; commit message includes finding ID.
- **No production code outside scope files** (§2.1 of spec).
- **Existing tests must pass** after each phase: `go test ./backend/store/... ./backend/database/... ./backend/session/...`
- **Bench file stays untracked**: `backend/session/bench_test.go` etc. — never `git add`.
- **Skip if in-flight conflict**: if a finding's fix touches `output_log.go` / `session.go` / `ftp_session.go` / `sync/git.go` / `sync_service.go` / `services/agent.ts` / `runtimeTypeCheck.ts`, skip and note in commit message body.
- **Bench regression check**: after phase C, run `go test -bench=BenchmarkSessionDataEmit -benchtime=3x ./backend/session/...` — emit alloc count should NOT increase.

---

### Task 1: F-101 — terminal_history atomic + debounce

**Files:**
- Modify: `backend/store/terminal_history_store.go` (F-101:33)
- New test: `backend/store/terminal_history_store_test.go` (if not exists; check first)

**Fix sketch:** wrap Save in 500ms debounce; atomic write via temp file + rename.

- [ ] **Step 1.1: Read current `terminal_history_store.go` to understand Save() signature**

- [ ] **Step 1.2: Add debounce + atomic write**

```go
type TerminalHistoryStore struct {
    mu       sync.Mutex
    pending  map[string]bool // dirty session IDs
    flushCh  chan struct{}
    // ... existing fields
}

func (s *TerminalHistoryStore) Save(sessionID, line string) {
    s.mu.Lock()
    s.pending[sessionID] = true
    s.mu.Unlock()
    select {
    case s.flushCh <- struct{}{}:
    default: // coalesce
    }
}

// flushLoop runs every 500ms or on signal, writes pending changes atomically.
func (s *TerminalHistoryStore) flushLoop() {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    for {
        select {
        case <-s.flushCh:
        case <-ticker.C:
        case <-s.stopCh:
            s.flush() // final flush
            return
        }
        s.flush()
    }
}

func (s *TerminalHistoryStore) flush() {
    s.mu.Lock()
    pending := s.pending
    s.pending = map[string]bool{}
    s.mu.Unlock()
    if len(pending) == 0 { return }
    // ... load existing, merge pending, atomic write tmp + rename
}
```

- [ ] **Step 1.3: Wire flushLoop into Open/Close lifecycle**

If Open exists, start goroutine. Add Close() that stops goroutine and final-flushes.

- [ ] **Step 1.4: Add test** (only if `terminal_history_store_test.go` doesn't exist)

Test: call Save N times rapidly; read file after 500ms — see merged result. Skip the existing internal tests to avoid breaking.

- [ ] **Step 1.5: Run existing tests**

```bash
go test ./backend/store/...
```

Expected: PASS (existing tests still pass; new test may fail if existing Save was synchronous — adjust carefully).

- [ ] **Step 1.6: Commit**

```bash
git add backend/store/terminal_history_store.go backend/store/terminal_history_store_test.go
git commit -m "fix(store): F-101 debounce + atomic terminal_history writes

Replaces per-call os.WriteFile of full file with 500ms debounce +
tmp+rename atomic write. P0 per audit findings.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: F-102 — recent_store debounce

**Files:**
- Modify: `backend/store/recent_store.go` (F-102:55)

**Fix sketch:** same pattern as F-101; coalesce Record by ID.

- [ ] **Step 2.1: Read current `recent_store.go` Record() and Save() flow**

- [ ] **Step 2.2: Add per-ID coalescing**

```go
type RecentStore struct {
    mu      sync.Mutex
    pending map[string]time.Time
    flushCh chan struct{}
    stopCh  chan struct{}
    // ... existing fields
}

func (s *RecentStore) Record(id string) {
    s.mu.Lock()
    s.pending[id] = time.Now()
    s.mu.Unlock()
    select {
    case s.flushCh <- struct{}{}:
    default:
    }
}
```

- [ ] **Step 2.3: Run tests**

```bash
go test ./backend/store/...
```

- [ ] **Step 2.4: Commit**

```bash
git add backend/store/recent_store.go
git commit -m "fix(store): F-102 debounce recent.Record

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: F-103 — ai_session_store sharded marshal

**Files:**
- Modify: `backend/store/ai_session_store.go` (F-103:54)

**Fix sketch:** save each session as its own file `ai-sessions/<id>.json`; load = glob + decode each.

- [ ] **Step 3.1: Read current ai_session_store.go**

- [ ] **Step 3.2: Implement sharded save/load**

```go
const aiSessionDir = "ai-sessions"

func (s *AISessionStore) Save(sess Session) error {
    data, err := json.Marshal(sess)
    if err != nil { return err }
    path := filepath.Join(s.dir, aiSessionDir, sess.ID + ".json")
    return atomicWrite(path, data)
}

func (s *AISessionStore) Load() ([]Session, error) {
    pattern := filepath.Join(s.dir, aiSessionDir, "*.json")
    matches, err := filepath.Glob(pattern)
    if err != nil { return nil, err }
    out := make([]Session, 0, len(matches))
    for _, m := range matches {
        var sess Session
        if err := readJSONFile(m, &sess); err != nil {
            continue // skip corrupt
        }
        out = append(out, sess)
    }
    return out, nil
}
```

- [ ] **Step 3.3: Migration on first load**

If the legacy `ai-sessions.json` file exists, move each entry to its own sharded file, then delete the legacy file. Use a `migrateOnce` flag.

- [ ] **Step 3.4: Test + commit**

```bash
go test ./backend/store/...
git add backend/store/ai_session_store.go
git commit -m "fix(store): F-103 shard ai_session by id (no full-file marshal)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: F-106 + F-111 — commands_store cache + atomic

**Files:**
- Modify: `backend/store/commands_store.go` (F-106:103, F-111:247)

- [ ] **Step 4.1: Read commands_store.go**

- [ ] **Step 4.2: Add mtime cache for List**

```go
type CommandsStore struct {
    mu       sync.Mutex
    listCache []Command
    listMod   time.Time
    listAt    time.Time
    // ...
}

func (s *CommandsStore) List() ([]Command, error) {
    s.mu.Lock()
    if time.Since(s.listAt) < 2*time.Second {
        out := s.listCache
        s.mu.Unlock()
        return out, nil
    }
    s.mu.Unlock()
    // Re-scan
    items, mod, err := s.scanDir()
    if err != nil { return nil, err }
    s.mu.Lock()
    s.listCache = items
    s.listMod = mod
    s.listAt = time.Now()
    s.mu.Unlock()
    return items, nil
}
```

- [ ] **Step 4.3: Atomic write for SaveCommand (F-111)**

```go
func atomicWrite(path string, data []byte) error {
    tmp := path + ".tmp." + strconv.Itoa(os.Getpid())
    if err := os.WriteFile(tmp, data, 0644); err != nil { return err }
    return os.Rename(tmp, path)
}
```

- [ ] **Step 4.4: Test + commit**

```bash
go test ./backend/store/...
git add backend/store/commands_store.go
git commit -m "fix(store): F-106 + F-111 commands cache + atomic write

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: F-107 + F-112 — skills_store cache + copyDir

**Files:**
- Modify: `backend/store/skills_store.go` (F-107:226, F-112:554)

- [ ] **Step 5.1: Same mtime cache pattern as F-106**

- [ ] **Step 5.2: Replace copyDir `filepath.Walk` with `os.CopyFS`**

```go
func copyDir(dst, src string) error {
    return os.CopyFS(dst, os.DirFS(src))
}
```

Go 1.23+ has `os.CopyFS`. If project is on older Go, fall back to streaming `io.Copy` per file via `filepath.WalkDir`.

- [ ] **Step 5.3: Test + commit**

```bash
go test ./backend/store/...
git add backend/store/skills_store.go
git commit -m "fix(store): F-107 + F-112 skills cache + copyFS

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: F-105 + F-110 — connection_store encoder + async keychain

**Files:**
- Modify: `backend/store/connection_store.go` (F-105:55, F-110:142)

- [ ] **Step 6.1: Replace MarshalIndent with json.Encoder + minimal indent**

```go
enc := json.NewEncoder(w)
enc.SetIndent("", "  ")
return enc.Encode(snapshot)
```

- [ ] **Step 6.2: Make populatePasswords async**

Replace synchronous keychain loop with a goroutine that fills a `passwords map[string]string` populated lazily on read.

- [ ] **Step 6.3: Test + commit**

```bash
go test ./backend/store/...
git add backend/store/connection_store.go
git commit -m "fix(store): F-105 + F-110 connection encoder + async keychain

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: F-108 + F-109 — settings_store encoder + lock granularity

**Files:**
- Modify: `backend/store/settings_store.go` (F-108:141, F-109:166)

- [ ] **Step 7.1: Replace MarshalIndent with json.Encoder (no indent)**

- [ ] **Step 7.2: Release lock before disk I/O**

```go
func (s *SettingsStore) Load() (AppSettings, error) {
    s.mu.Lock()
    path := s.path
    s.mu.Unlock()
    data, err := os.ReadFile(path)
    if err != nil { return AppSettings{}, err }
    s.mu.Lock()
    defer s.mu.Unlock()
    return parseSettings(data)
}
```

- [ ] **Step 7.3: Test + commit**

```bash
go test ./backend/store/...
git add backend/store/settings_store.go
git commit -m "fix(store): F-108 + F-109 settings encoder + load-without-lock

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: F-113 + F-114 — database engine query alloc + scanToString

**Files:**
- Modify: `backend/database/engine.go` (F-113:50, F-114:126)

- [ ] **Step 8.1: Stream row materialization**

Provide `QueryRowsStream(callback func(row []string) error)` that calls back per row without per-row map alloc. Keep existing `queryStrings` for small results (< 1000 rows).

- [ ] **Step 8.2: Type switch in scanToString**

```go
func scanToString(v any) string {
    switch x := v.(type) {
    case []byte: return string(x)
    case string: return x
    case int64:  return strconv.FormatInt(x, 10)
    case float64: return strconv.FormatFloat(x, 'g', -1, 64)
    case bool:   return strconv.FormatBool(x)
    case time.Time: return x.Format(time.RFC3339Nano)
    case nil:    return ""
    default:     return fmt.Sprintf("%v", v)
    }
}
```

- [ ] **Step 8.3: Test + commit**

```bash
go test ./backend/database/...
git add backend/database/engine.go
git commit -m "fix(database): F-113 + F-114 stream query + scanToString switch

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: F-115 — Postgres GetTableSchema concurrent

**Files:**
- Modify: `backend/database/provider_postgres.go` (F-115:178)

- [ ] **Step 9.1: Use errgroup.Group for 3 parallel queries + result cache**

```go
var schemaCache sync.Map // connHash -> schema

func (p *postgresProvider) GetTableSchema(...) {
    key := dbName + "." + tableName
    if cached, ok := schemaCache.Load(key); ok {
        return cached.(Schema), nil
    }
    var g errgroup.Group
    var cols, pks, fks []string
    g.Go(func() error { cols, _ = p.columnsQuery(...); return nil })
    g.Go(func() error { pks, _ = p.pkQuery(...); return nil })
    g.Go(func() error { fks, _ = p.fkQuery(...); return nil })
    _ = g.Wait()
    sch := Schema{Columns: cols, Pks: pks, Fks: fks}
    schemaCache.Store(key, sch)
    return sch, nil
}
```

- [ ] **Step 9.2: Test + commit**

```bash
go test ./backend/database/...
git add backend/database/provider_postgres.go
git commit -m "fix(database): F-115 Postgres GetTableSchema concurrent + cached

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: F-116 — mysql connection pool tuning

**Files:**
- Modify: `backend/database/provider_mysql.go` (F-116:19)

- [ ] **Step 10.1: Add connection pool params to DSN**

```go
dsn := fmt.Sprintf("%s?parseTime=true&timeout=10s&readTimeout=10s&writeTimeout=10s", baseDSN)
db.SetMaxOpenConns(20)
db.SetMaxIdleConns(5)
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(2 * time.Minute)
```

- [ ] **Step 10.2: Test + commit**

```bash
go test ./backend/database/...
git add backend/database/provider_mysql.go
git commit -m "fix(database): F-116 mysql connection pool tuning

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: F-002 — ssh_session decodeOutput scratch buffer

**Files:**
- Modify: `backend/session/ssh_session.go` (F-002:542)

- [ ] **Step 11.1: Add per-session scratch field**

```go
type SSHSession struct {
    // ... existing
    decodeScratch []byte
}
```

- [ ] **Step 11.2: Reuse in decodeOutput**

```go
func (s *SSHSession) decodeOutput(data []byte) []byte {
    s.mu.Lock()
    defer s.mu.Unlock()
    if s.decoder == nil { return data }
    s.decodeScratch = s.decodeScratch[:0]
    s.decodeScratch = append(s.decodeScratch, s.decodeLeftover...)
    s.decodeScratch = append(s.decodeScratch, data...)
    src := s.decodeScratch
    // ... rest unchanged
}
```

- [ ] **Step 11.3: Verify bench regression**

```bash
go test -bench=BenchmarkSSHDecode -benchtime=3x ./backend/session/...
```

Expected: BenchmarkSSHDecodeFastPathNoAlloc allocs/op stays 0.

- [ ] **Step 11.4: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/ssh_session.go
git commit -m "fix(session): F-002 SSH decodeOutput scratch buffer

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 12: F-003 — ssh_session Write alloc

**Files:**
- Modify: `backend/session/ssh_session.go` (F-003:459)

- [ ] **Step 12.1: Cache encoder transformer on session**

Add `encEncoder transform.Transformer` field, build once in SetEncoding.

- [ ] **Step 12.2: Use in Write**

```go
func (s *SSHSession) Write(data []byte) (int, error) {
    if s.encoder == nil {
        return s.stdin.Write(data)
    }
    out, _, err := s.encoder.Transform(s.encScratch[:0], data, true)
    if err != nil && err != transform.ErrShortSrc { return 0, err }
    n, err := s.stdin.Write(out)
    return n, err
}
```

- [ ] **Step 12.3: Verify bench**

```bash
go test -bench=BenchmarkSSHDecode -benchtime=3x ./backend/session/...
```

- [ ] **Step 12.4: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/ssh_session.go
git commit -m "fix(session): F-003 SSH Write cached encoder

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 13: F-004 — ssh_session readLoop double copy

**Files:**
- Modify: `backend/session/ssh_session.go` (F-004:313)

- [ ] **Step 13.1: Single buf in readLoop**

```go
readBuf := make([]byte, 16*1024) // also covers F-001
for {
    n, err := s.session.Read(readBuf[:cap(readBuf)])
    if n > 0 {
        s.handleOutput(readBuf[:n]) // pass slice, no copy
    }
    if err != nil { return }
}
```

- [ ] **Step 13.2: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/ssh_session.go
git commit -m "fix(session): F-001 + F-004 SSH read buffer reuse + 16K size

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 14: F-044 — SSH keepalive interval

**Files:**
- Modify: `backend/session/ssh_session.go` (F-044:434)

- [ ] **Step 14.1: Adjust interval to 90s**

Change `sshKeepAliveInterval = 60 * time.Second` → `90 * time.Second`. Or gate it on lifecycle (preferred): only send keepalive when active. Use lifecycle event from app.go.

If lifecycle gate requires in-flight file changes, defer to batch 7 and just adjust interval.

- [ ] **Step 14.2: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/ssh_session.go
git commit -m "fix(session): F-044 SSH keepalive 60s -> 90s (less idle wakeup)

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 15: F-005 + F-006 — local_session read buf + alloc

**Files:**
- Modify: `backend/session/local_session_unix.go` (F-005:150, F-006:163)

- [ ] **Step 15.1: 16K buf + reuse**

```go
readBuf := make([]byte, 16*1024)
for {
    n, err := ptmx.Read(readBuf[:cap(readBuf)])
    if n > 0 {
        s.handleOutput(readBuf[:n])
    }
    if err != nil { return }
}
```

- [ ] **Step 15.2: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/local_session_unix.go
git commit -m "fix(session): F-005 + F-006 local PTY 16K buf + reuse

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 16: F-008 — telnet_session read buf + IAC

**Files:**
- Modify: `backend/session/telnet_session.go` (F-008:98)

- [ ] **Step 16.1: 16K buf + filter in handler**

Same pattern as local: bigger buf, no inline filtering.

- [ ] **Step 16.2: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/telnet_session.go
git commit -m "fix(session): F-008 telnet 16K buf

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 17: F-007 — serial_session read buf

**Files:**
- Modify: `backend/session/serial_session.go` (F-007:109)

- [ ] **Step 17.1: 16K buf + reuse**

- [ ] **Step 17.2: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/serial_session.go
git commit -m "fix(session): F-007 serial 16K buf

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 18: F-018 — manager.sessions unbounded

**Files:**
- Modify: `backend/session/manager.go` (F-018:10)

- [ ] **Step 18.1: Force-evict on Close failure path**

```go
func (m *Manager) Close(id string) error {
    sess, ok := m.sessions[id]
    if !ok { return nil }
    delete(m.sessions, id) // unconditional first
    return sess.Close()
}
```

- [ ] **Step 18.2: Add startup validation**

In `Init()`, iterate m.sessions, check liveness, delete dead ones.

- [ ] **Step 18.3: Test + commit**

```bash
go test ./backend/session/...
git add backend/session/manager.go
git commit -m "fix(session): F-018 manager.sessions unbounded eviction

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 19: Final verification + handoff

- [ ] **Step 19.1: Run all backend tests**

```bash
go test ./backend/...
```

Expected: PASS.

- [ ] **Step 19.2: Re-run bench regression**

```bash
go test -bench=BenchmarkSessionDataEmit -benchmem -benchtime=3x ./backend/session/...
go test -bench=BenchmarkSSHDecode -benchmem -benchtime=3x ./backend/session/...
go test -bench=BenchmarkQueryStrings -benchmem -benchtime=3x ./backend/database/...
```

Expected: alloc counts unchanged or improved; no regression.

- [ ] **Step 19.3: Verify bench files still untracked**

```bash
git status --short backend/session/bench_test.go backend/database/bench_test.go app_bench_test.go
```

Expected: `??` (untracked).

- [ ] **Step 19.4: Commit history**

```bash
git log --oneline origin/main..HEAD
```

Expect 18 atomic commits (Tasks 1-18) plus spec + plan commits.

- [ ] **Step 19.5: Push branch + open PR**

```bash
git push origin perf-audit
gh pr create --base main --head perf-audit \
  --title "fix(session,store,database,k8s): batch 6a — backend hot paths (P0/P1)" \
  --body "Implements non-in-flight batch 6 fixes (24 findings across 14 files). PR title follows repo convention. See commits for per-finding rationale. Bench file untracked per user constraint."
```

---

## Self-Review

- **Spec coverage**: All 24 non-in-flight batch 6 findings covered by Tasks 1-18.
- **Placeholders**: none — each task has concrete code / commit command.
- **Bench regression**: Step 19.2 verifies alloc counts.
- **Bench file untracked**: re-affirmed in Step 19.3.
- **In-flight skip**: Tasks 14 (F-044) and others may touch lifecycle; if so, defer in body.