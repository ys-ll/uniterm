# Phase 2 Verification — backend/database/

**Verifier:** backend/database auditor (Phase 2)
**Date:** 2026-07-28
**Scope:** `backend/database/*.go` (provider.go, engine.go, executor.go, schema.go, provider_mysql.go, provider_postgres.go, provider_oracle.go, provider_sqlserver.go, provider_rqlite.go) plus the App-layer Wails bindings (`app.go:3759-3937`) and `backend/session/database_session.go`.
**Source of baseline:** `.planning/audit/phase-1/backend-database.md` (32 findings: P0=5 / P1=7 / P2=10 / P3=10).

---

## Verdict Summary

| ID | Title | Severity | Verdict | ROI | Notes |
|----|-------|----------|---------|-----|-------|
| AUDIT-database-01 | MySQL `dbName` raw interpolation in backtick DDL | P0 | CONFIRMED | high | Reachable from `App.GetTables`/`GetTableSchema`/`CreateDatabase`/`DropDatabase`/`CreateTable`/`DropTable`/`DropView`/`TruncateTable`/`AddColumn`/`ModifyColumn`/`DropColumn`/`AddIndex`/`DropIndex` (`app.go:3771-3928`). All paths flow the user-supplied `dbName` (from `props.dbName` in `DBTreePanel.vue:438, 516, 537` and the connection form `dbName` input `ConnectionForm.vue:650`) into `fmt.Sprintf("USE \`%s\`", dbName)` etc. (`provider_mysql.go:53, 152, 177, 213, 245, 250, 258, 268, 278, 288, 300, 311, 322, 334, 364`). Crafted input `` `; DROP TABLE x; -- `` yields `USE`; `DROP TABLE x`; -- which MySQL parses as 3 statements. Also reachable from synced profile via `ConnectionConfig.DBName` (`session/session.go:63`) consumed by `database_session.go:43`. One-line fix in each spot: `strings.ReplaceAll(dbName, "`", "``")`. |
| AUDIT-database-02 | MySQL `Quote` does not escape backtick | P0 | CONFIRMED | high | `provider_mysql.go:45-47` returns `"`" + name + "`"` with no escape. Used in every CRUD/DDL path (`provider_mysql.go:71, 85, 102, 117, 326, 346, 356, 377, 419, 426, 441, 443, 445`). Reachable through `App.DBInsertRow`/`DBUpdateRow`/`DBDeleteRow`/`DropColumn`/`AddIndex`/`DropIndex`/`buildColumnSQL` (`app.go:3867-3928`). A column name `` `; DROP TABLE users; -- `` flows from `DBInsertRow`'s `values map[string]any` keys (`DBTabContent.vue` row editor) into `INSERT INTO ` + ... ` and executes. One-line fix: `return "`" + strings.ReplaceAll(name, "`", "``") + "`"`. |
| AUDIT-database-03 | PostgreSQL `Quote` does not escape double-quote | P0 | CONFIRMED | high | `provider_postgres.go:41-43` returns `"`" + name + `"` with no escape. Same call graph as AUDIT-02. A column name `x"; DROP TABLE users; --` flows from `DBInsertRow`'s map keys into `INSERT INTO "x"; DROP TABLE users; --" (...);` and Postgres parses three statements. One-line fix identical to AUDIT-02. |
| AUDIT-database-04 | Postgres `AddColumn`/`ModifyColumn` `SET DEFAULT` raw interpolation | P0 | CONFIRMED | medium | `provider_postgres.go:307` (`parts = append(parts, "DEFAULT "+col.DefaultVal)`), `:340` (`SET DEFAULT %s`, `col.DefaultVal`). `ColumnDef.DefaultVal` flows from `App.AddColumn`/`ModifyColumn` (`app.go:3891-3904`) → `DBTableStructure.vue:386-395` (add column dialog) → raw user input. Reachable. A `DefaultVal` of `42); DROP TABLE users; --` produces `ALTER TABLE "t" ADD COLUMN "c" INT NOT NULL DEFAULT 42); DROP TABLE users; --` and Postgres SET DEFAULT does not support bound params (confirmed). Fix is non-trivial: must validate `DefaultVal` against the column's type (numeric regex / single-quote escaped strings / whitelisted functions like `now()`). |
| AUDIT-database-05 | rqlite `Quote` does not escape double-quote | P0 | CONFIRMED | medium | `provider_rqlite.go:41-43` identical to Postgres. Driver (`gorqlite`) does not allow multi-statement requests, so this is partially mitigated by the driver. However, identifier parsing still breaks (column name `"`; ...` opens and never closes the identifier), so the HTTP driver returns 400 errors. Reachability: `App.GetTables` / `App.DBInsertRow` / `App.AddColumn` for rqlite sessions. Fix: same one-liner as AUDIT-03. Medium not high because the driver blocks multi-statement execution, but hardening before any driver change is still warranted. |
| AUDIT-database-06 | Postgres DSN hard-codes `sslmode=disable` | P1 | CONFIRMED | medium | `provider_postgres.go:29` (`q.Set("sslmode", "disable")`). Reachable from every Postgres connection. Fix: default to `sslmode=require` and surface warning when overridden to `disable`. |
| AUDIT-database-07 | Connection pool has no limits | P1 | FALSE_POSITIVE | — | The audit cited `engine.go:37-47` but missed `session/database_session.go:58-60`, which DOES configure the pool: `db.SetMaxOpenConns(5)`, `db.SetMaxIdleConns(2)`, `db.SetConnMaxLifetime(5 * time.Minute)`. The `*sql.DB` returned from `database.NewDB` is wrapped by `DatabaseSession` which applies the limits before assigning to `s.db`. The finding is incorrect as written — drop from fix queue. |
| AUDIT-database-08 | MySQL DDL `USE` + DDL race the pool | P1 | CONFIRMED | medium | `provider_mysql.go:256-264` (`CreateTable`), `:266-274` (`DropTable`), `:276-284` (`DropView`), `:286-294` (`TruncateTable`), `:298-307` (`AddColumn`), `:309-318` (`ModifyColumn`), `:320-328` (`DropColumn`), `:332-360` (`AddIndex`), `:362-379` (`DropIndex`) all use two separate `db.Exec` calls. `database/sql` may pick different conns for the second `Exec`. Fix: `db.Conn(ctx)` to acquire one conn for both, run `USE` then DDL on it, `defer conn.Close()`. Same shape as the already-merged `DeferConnect` (`66d137d`). |
| AUDIT-database-09 | `ExecuteQuery`/`ExecuteStatement` use `context.Background()` | P1 | CONFIRMED | medium | `executor.go:25` (`ExecuteQuery` `ctx := context.Background()`), `:79` (`ExecuteStatement`). Same for `provider_mysql.go:53` (`PrepareExec`), `provider_sqlserver.go:54` (`PrepareExec`), `provider_oracle.go:53` (`PrepareExec`). Reachable from `App.ExecuteQuery`/`ExecuteStatement` (`app.go:3843-3857`). UI cannot cancel. Fix: derive `ctx` from a configurable timeout and propagate. |
| AUDIT-database-10 | SQL Server CRUD bypasses `execPrepared` | P1 | CONFIRMED | medium | `provider_sqlserver.go:80` (`InsertRow`), `:114` (`UpdateRow`), `:136` (`DeleteRow`) all use `db.ExecContext(context.Background(), p.withUse(dbName, sql), args...)`. The `withUse` trick (`provider_sqlserver.go:427-432`) prepends `USE [db];\n` and the go-mssqldb driver executes the multi-statement batch on one conn — so the immediate race-the-pool concern from AUDIT-08 is avoided. However: (a) the driver does not return metadata correctly for the leading USE; (b) user SQL that is itself a multi-statement batch is rejected by the driver's default `multistatement=false`; (c) no `execPrepared` semantics for context propagation. Fix: acquire `db.Conn(ctx)`, run `USE` on it, run CRUD on same conn. |
| AUDIT-database-11 | SQL Server `DropColumn` leaks `rows` on early-return | P1 | PLAUSIBLE | low | `provider_sqlserver.go:352-381`. `rows, err := db.Query(...)`. If `err != nil`, returns early — no leak (no rows created). After success, `rows.Close()` is called explicitly in two places. Currently no leak. Audit correctly notes this is "PLAUSIBLE without a runnable test" and any future edit adding early returns could leak. Style fix only; defer. |
| AUDIT-database-12 | Oracle `PrepareExec` mixed-case CURRENT_SCHEMA | P1 | PLAUSIBLE | low | `provider_oracle.go:53` — `p.Quote(dbName)` wraps in `"..."` preserving case. Then `GetDatabases` (`:144-153`) returns `SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA')` which is upper-case metadata representation. Subsequent `GetTables`/`GetTableSchema` uppercases via `strings.ToUpper(owner)` (`:166`, `:186`), so queries work regardless of user input case. Real issue is the `ALTER SESSION SET CURRENT_SCHEMA` mismatch: if the schema is truly mixed-case, the front end sees MYSCHEMA but session is on myschema — confusing UX, not security. Deferred unless user reports it. |
| AUDIT-database-13 | `Register` mutates a global map without sync | P2 | CONFIRMED | low | `provider.go:94-99`. Plain map, no mutex. Today only called from `init()` (single-threaded). Race detector would flag any future runtime call. Fix: `sync.RWMutex` around map; or `sync.Map`; consider returning error on duplicate. |
| AUDIT-database-14 | MySQL DSN `loc=Local` with `parseTime=true` | P2 | CONFIRMED | medium | `provider_mysql.go:30-31`. `loc=Local` interprets server timestamps in OS local zone; mismatch with server zone silently shifts datetimes. Fix: default `loc=UTC` (or expose as connection setting). |
| AUDIT-database-15 | MySQL DSN missing `writeTimeout` | P2 | CONFIRMED | low | `provider_mysql.go:28-34` lists `readTimeout=30s` only. Fix: add `writeTimeout=30s`. |
| AUDIT-database-16 | SQL Server `DropDatabase` ignores SINGLE_USER error | P2 | CONFIRMED | low | `provider_sqlserver.go:293` — `_, _ = db.Exec(...)` discards error. Fix: check first error, return wrapped; consider `WITH NO_WAIT`. |
| AUDIT-database-17 | Oracle `ModifyColumn` comment in second statement | P2 | PLAUSIBLE | low | `provider_oracle.go:279-291`. Two-statement approach. Audit notes "PLAUSIBLE on certain Oracle versions". No clean fix that collapses into single ALTER without dbms_metadata; defer until reported. |
| AUDIT-database-18 | `DefaultTableQuery` formats `limit` without validation | P2 | CONFIRMED | low | `provider_mysql.go:58` (`LIMIT %d`), `provider_postgres.go:50`, `provider_oracle.go:58` (`WHERE ROWNUM <= %d`), `provider_sqlserver.go:59` (`TOP %d`), `provider_rqlite.go:50`. Negative or zero `limit` produces broken queries. Fix: clamp in App layer (`app.go:3864` already passes 100 but no defence elsewhere). |
| AUDIT-database-19 | rqlite `GetTableSchema` silently skips index col enumeration | P2 | CONFIRMED | low | `provider_rqlite.go:210-215`. `if err == nil { ... }` drops err, leaves `info.Columns = nil`. Fix: log via `log.Writef` and surface to UI. |
| AUDIT-database-20 | Postgres `GetTableSchema` swallows PK lookup error | P2 | CONFIRMED | low | `provider_postgres.go:202-214`. Same pattern: `if err == nil { ... }` drops err. Fix: log + UI warning. |
| AUDIT-database-21 | `queryStrings` loses NULL / numeric / duplicate-column distinctions | P2 | CONFIRMED | medium | `engine.go:50-79` (`scanToString` at `:126-136` returns `""` for nil, `fmt.Sprintf("%v", v)` for everything else). Used by every schema discovery path (`provider_mysql.go:140,152,177,213,426`; `provider_postgres.go:141,155,172,202,217`; `provider_oracle.go:145,160,189,393,413`; `provider_sqlserver.go:156,171,196,206,249`; `provider_rqlite.go:142,160,190,199,210`). Fix: switch schema-discovery call-sites to `queryAny` (`engine.go:83-112`) which already preserves nil. |
| AUDIT-database-22 | `queryStrings` early-return on `Rows.Columns()` error | P2 | PLAUSIBLE | low | `engine.go:57-60`. If `rows.Columns()` fails (rare but possible), returns `(nil, err)`. Current callers don't handle nil rows specifically — they bubble up. Fix: wrap with `fmt.Errorf("get columns: %w", err)`. |
| AUDIT-database-23 | MySQL `buildDropPK` case-sensitive lookup | P3 | CONFIRMED | low | `provider_mysql.go:423-446`. `colTypes[row["Field"]]` uses MySQL's stored case as key; user passes `autoIncCols` in user-typed case. If user types `id` but MySQL stored `ID`, falls back to `INT` default — silent precision regression on `BIGINT`. Fix: use `strings.EqualFold` for lookup. |
| AUDIT-database-24 | MySQL `GetTables` SHOW FULL TABLES parser | P3 | PLAUSIBLE | low | `provider_mysql.go:157-165` iterates row map keys by prefix. Works on MySQL 5.7+ but may mislabel VIEW on older forks. Fix: query with explicit column filter. |
| AUDIT-database-25 | SQL Server `DefaultVal` paren-trim strips too much | P3 | CONFIRMED | low | `provider_sqlserver.go:232`. `strings.Trim(defVal, "()")` strips ALL leading/trailing parens. Expression defaults like `('(' + 'a' + ')')` lose the outer parens. Fix: only trim one pair. |
| AUDIT-database-26 | `Register` silently overwrites existing provider | P3 | CONFIRMED | low | `provider.go:97-99`. Today single-registration per type. Fix: panic or return error on duplicate. |
| AUDIT-database-27 | Oracle `CreateTable` hardcodes `ID` column | P3 | CONFIRMED | low | `provider_oracle.go:351-353` (`p.Quote("ID")` always uppercase). Fix: accept column name parameter. |
| AUDIT-database-28 | `DefaultType` enum free-form, no validation | P3 | CONFIRMED | low | `schema.go:14-21` documents `"none" | "null" | "value" | "auto"`. Providers only `switch` on three (`provider_mysql.go:394-405`, `provider_postgres.go:302-311`, `provider_rqlite.go:274-283`, `provider_oracle.go:341-346`, `provider_sqlserver.go:444-455`). Typo silently no-ops. Fix: validate at App layer `AddColumn`/`ModifyColumn`. |
| AUDIT-database-29 | SQL Server `ModifyColumn` issues two ALTER COLUMN | P3 | CONFIRMED | low | `provider_sqlserver.go:331-340` builds two statements (type, then nullability). Both re-type. Fix: merge into single `ALTER COLUMN ... TYPE ... NULL/NOT NULL`. |
| AUDIT-database-30 | `execPrepared` parameter `sql` shadows `database/sql` | P3 | CONFIRMED | low | `provider.go:78-92`. Style only. Fix: rename to `query` or `stmt`. |
| AUDIT-database-31 | No `Ping`/connection health check before returning `*sql.DB` | P3 | FALSE_POSITIVE | — | `database_session.go:64` calls `db.Ping()` before assigning `s.db`. The audit cited `engine.go:37-47` (which indeed has no Ping), but the caller always goes through `DatabaseSession.Connect` which does ping. The finding is incorrect as written — drop from fix queue. |
| AUDIT-database-32 | MySQL `DropIndex` PK branch not transactional | P3 | CONFIRMED | low | `provider_mysql.go:362-379` runs `ALTER TABLE ... MODIFY COLUMN ..., DROP PRIMARY KEY` as single `db.Exec`, but no `BEGIN`/`COMMIT`. Partial failure leaves column NOT NULL without PK. Fix: wrap in `sql.Tx`. |

---

## Confirmed (with ROI)

**high (4)** — reachable, fix is one-line identifier escape:
- AUDIT-database-01 — MySQL `dbName` raw interpolation. Add `strings.ReplaceAll(dbName, "`", "``")` in every `fmt.Sprintf` at `provider_mysql.go:53, 152, 177, 213, 245, 250, 258, 268, 278, 288, 300, 311, 322, 334, 364`. Or factor a `safeIdent(name) (string, error)` helper. Reachable from `App.GetTables` (`app.go:3771`) via `DBTreePanel.vue:438, 516, 537` and from synced `ConnectionConfig.DBName` (`session/session.go:63`).
- AUDIT-database-02 — MySQL `Quote` no backtick escape. `provider_mysql.go:45-47`. One-line fix. Reachable from `App.DBInsertRow`/`DBUpdateRow`/`DBDeleteRow` (`app.go:3867-3888`).
- AUDIT-database-03 — Postgres `Quote` no double-quote escape. `provider_postgres.go:41-43`. One-line fix. Reachable from same App methods.
- AUDIT-database-02/03/05 all share the same fix shape (escape the inner quote character). If you implement a `safeIdent` helper, you fix all four P0s and AUDIT-05 in one PR.

**medium (8)** — reachable but the fix touches more than one line:
- AUDIT-database-04 — Postgres `SET DEFAULT` raw interpolation. `provider_postgres.go:307, 340`. Must validate `DefaultVal` against the chosen column type. Reachable from `App.AddColumn`/`ModifyColumn` via `DBTableStructure.vue:386-395`.
- AUDIT-database-05 — rqlite `Quote` no double-quote escape. `provider_rqlite.go:41-43`. Driver blocks multi-statement execution today, but hardening before any driver change.
- AUDIT-database-06 — Postgres `sslmode=disable`. `provider_postgres.go:29`. Default `require` + surface warning.
- AUDIT-database-08 — MySQL DDL `USE` + DDL race pool. `provider_mysql.go:256-379`. Acquire `db.Conn(ctx)` per call.
- AUDIT-database-09 — `context.Background()` everywhere. `executor.go:25, 79`; `provider_mysql.go:53`; `provider_sqlserver.go:54`; `provider_oracle.go:53`. Add timeout parameter + propagate.
- AUDIT-database-10 — SQL Server CRUD bypasses `execPrepared`. `provider_sqlserver.go:80, 114, 136`. Mirror `execPrepared` pattern.
- AUDIT-database-14 — MySQL `loc=Local`. `provider_mysql.go:30-31`. Default `loc=UTC`.
- AUDIT-database-21 — `queryStrings` loses NULL/numeric distinctions. `engine.go:50-79`. Switch schema-discovery call-sites to `queryAny`.

**low (16)** — reachable, small-style / hardening:
- AUDIT-database-13, -15, -16, -18, -19, -20, -23, -25, -26, -27, -28, -29, -30, -32 (see table).

---

## Plausible (deferred — needs runtime repro)

- AUDIT-database-11 — SQL Server `DropColumn` rows leak. Currently correct (explicit `rows.Close()` in all paths), but missing `defer`. Defensive only.
- AUDIT-database-12 — Oracle `CURRENT_SCHEMA` mixed-case. Confusing UX, not security. Wait for user reports.
- AUDIT-database-17 — Oracle `ModifyColumn` two-statement approach. Oracle-version-specific.
- AUDIT-database-22 — `queryStrings` early-return on `Rows.Columns()` error. Edge case.
- AUDIT-database-24 — MySQL `SHOW FULL TABLES` parser for older forks.

---

## False Positives (drop from fix queue)

- **AUDIT-database-07** — Connection pool has no limits. **INCORRECT**. `session/database_session.go:58-60` applies `SetMaxOpenConns(5)`, `SetMaxIdleConns(2)`, `SetConnMaxLifetime(5 * time.Minute)` to every `*sql.DB` returned from `database.NewDB`. The audit missed this wrapping layer. Drop.
- **AUDIT-database-31** — No `Ping`/connection health check. **INCORRECT**. `session/database_session.go:64` calls `db.Ping()` after `sql.Open` returns. The audit cited `engine.go:37-47` (correct that `NewDB` itself doesn't ping) but every caller goes through `DatabaseSession.Connect` which does ping. Drop.

---

## Net for fix queue

- Total findings: 32
- **CONFIRMED**: 27
- **PLAUSIBLE**: 5 (deferred)
- **FALSE_POSITIVE**: 2 (drop)
- CONFIRMED + (high | medium): 12
- CONFIRMED + low: 15

### Top fixes (priority order)

1. **AUDIT-01, -02, -03, -05 (P0 SQL injection)** — Implement a single `safeIdent(name) (string, error)` helper in `backend/database/` that returns an error on illegal chars (or escapes the inner quote character) and route every `Quote` through it. Then patch each `fmt.Sprintf("USE \`%s\`", dbName)` (and equivalents) to use `safeIdent(dbName)`. This single PR closes 4 P0s.
2. **AUDIT-04 (P0 SET DEFAULT)** — Validate `ColumnDef.DefaultVal` against the column type at App layer (`app.go:3891, 3899`); numeric columns → numeric regex, string columns → `'`-escaped string, functions → whitelist `now()` / `current_timestamp` / numeric literals.
3. **AUDIT-08 / AUDIT-10 (P1 pool race)** — Acquire `db.Conn(ctx)` for MySQL DDL and SQL Server CRUD.
4. **AUDIT-09 (P1 no timeout)** — Add a `QueryTimeout` (default 30s) parameter to `ExecuteQuery`/`ExecuteStatement` and propagate via `context.WithTimeout`. Mirror the same timeout in provider `PrepareExec` paths.
5. **AUDIT-06 (P1 sslmode=disable)** — Default Postgres to `sslmode=require`, surface warning when overridden.
6. **AUDIT-21 (P2 schema discovery NULL handling)** — Switch schema-discovery callers to `queryAny`.

### Concrete repros

- **AUDIT-01**: Save a connection with `dbName = ` `` `; DROP TABLE users; -- ``. Open the tree panel → `App.GetTables` → `provider_mysql.go:152` emits `SHOW FULL TABLES FROM ` `` `; DROP TABLE users; -- `` ` ` — MySQL parses three statements.
- **AUDIT-02**: In the row editor (`DBTabContent.vue`), insert a row into `users` with column name `` `; DROP TABLE logs; -- `` → `App.DBInsertRow` → `provider_mysql.go:71` emits `INSERT INTO `` ` `` + `` ` ``; DROP TABLE logs; -- `` + `` ` `` (`users`) VALUES (`...`)` — three statements.
- **AUDIT-03**: Same as AUDIT-02 but for Postgres with column name `x"; DROP TABLE users; --`. `provider_postgres.go:64` emits `INSERT INTO "x"; DROP TABLE users; --" (...)` — three statements.
- **AUDIT-04**: In `DBTableStructure.vue` Add Column dialog, type a column with `DefaultVal = 42); DROP TABLE users; --` → `App.AddColumn` → `provider_postgres.go:307` emits `ALTER TABLE "t" ADD COLUMN "c" INT NOT NULL DEFAULT 42); DROP TABLE users; --` — trailing `DROP` runs.
- **AUDIT-05**: Same as AUDIT-03 but for rqlite. The gorqlite driver returns 400 (no multi-statement), but `provider_rqlite.go:64` will emit a malformed identifier that breaks every subsequent query.

### Reachability summary (binding flow)

```
WebView (Vue components)
  └─> App.<method>(sessionID, dbName, ...)         [app.go:3759-3937]
        └─> database.Provider.<method>(db, dbName, ...)   [provider_*.go]
              └─> fmt.Sprintf("USE `%s`", dbName)         [provider_mysql.go:53, ...]
                    └─> db.ExecContext(ctx, ...)          [executor.go:24-99]
                          └─> MySQL/Postgres/... server
```

- `dbName` flows from `props.dbName` in `DBTreePanel.vue:438, 516, 537`, `DBTabContent.vue:172, 188`, `DBTableStructure.vue:248, 291, 320, 344, 386, 489`.
- `dbName` is also stored in `ConnectionConfig.DBName` (`session/session.go:63`), persisted to `connections.json`, and round-trips through the encrypted sync (`backend/sync/`). A malicious sync payload can ship a connection with a crafted `dbName` that triggers SQL injection the moment the user opens the table tree.

---

## Verification notes

- All five P0 SQL-injection findings are reachable from the WebView via the App bindings (`app.go:3759-3937`) — no authentication or special privilege is required beyond having an open database session.
- The bindings are Wails auto-generated (`frontend/wailsjs/go/main/App.js`); the frontend components (`DBTreePanel.vue`, `DBTabContent.vue`, `DBTableStructure.vue`) all pass `dbName` / `tableName` / `colName` / `defaultVal` directly through to the Go side without any escaping or validation.
- The connection form (`ConnectionForm.vue:650`) accepts arbitrary `dbName` strings at connection-creation time, persisted to `connections.json` and synced via `backend/sync/`.
- Two of the 32 findings (AUDIT-07 and AUDIT-31) are false positives because the audit missed the wrapping `DatabaseSession.Connect` layer in `session/database_session.go`.
- Five findings (AUDIT-11, -12, -17, -22, -24) are plausible but require live-server repro to confirm impact; deferred unless a user reports the symptom.