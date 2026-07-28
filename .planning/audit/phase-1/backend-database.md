# Phase 1 Audit — backend/database/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** `backend/database/*.go` (provider.go, engine.go, executor.go, schema.go, provider_mysql.go, provider_postgres.go, provider_oracle.go, provider_sqlserver.go, provider_rqlite.go)

---

## Findings (by severity)

### P0 — Critical

**AUDIT-database-01: SQL injection in MySQL `dbName` interpolation across all DDL/schema paths**
- Files:
  - `backend/database/provider_mysql.go:53` — `fmt.Sprintf("USE \`%s\`", dbName)`
  - `backend/database/provider_mysql.go:152` — `fmt.Sprintf("SHOW FULL TABLES FROM \`%s\`", dbName)`
  - `backend/database/provider_mysql.go:177` — `fmt.Sprintf("SHOW FULL COLUMNS FROM \`%s\`.\`%s\`", dbName, tableName)`
  - `backend/database/provider_mysql.go:213` — `fmt.Sprintf("SHOW INDEX FROM \`%s\`.\`%s\`", dbName, tableName)`
  - `backend/database/provider_mysql.go:245` — `fmt.Sprintf("CREATE DATABASE \`%s\`", dbName)`
  - `backend/database/provider_mysql.go:250` — `fmt.Sprintf("DROP DATABASE \`%s\`", dbName)`
  - `backend/database/provider_mysql.go:258,268,278,288,300,311,322,334,364` — `fmt.Sprintf("USE \`%s\`", dbName)` inside every DDL function
- Severity: P0
- Failure scenario: User-supplied `dbName` from `App.GetTables`, `App.GetTableSchema`, `App.CreateDatabase`, `App.DropDatabase`, `App.CreateTable`, `App.DropTable`, `App.DropView`, `App.TruncateTable`, `App.AddColumn`, etc. (`app.go:3771-3836`, `app.go:3891-3912`) is interpolated directly between backticks without escaping backticks. A `dbName` value of `` `; DROP TABLE x; -- `` yields e.g. `` USE `; DROP TABLE x; --` `` which MySQL parses as `USE`, then `DROP TABLE x`, then a comment. Even a connection profile synced via the encrypted sync feature (`backend/sync/`) can carry a malicious `dbName` that triggers arbitrary SQL on the target server when the user opens the table tree.
- Fix category: validate `dbName`/`tableName` against an identifier regex (e.g. `^[A-Za-z0-9_$.-]+$`) or escape backticks in every spot; preferably pass `dbName` as a bound parameter via `PREPARE` for the `USE`/DDL calls MySQL supports.
- Evidence:
  ```go
  // provider_mysql.go:53
  _, err := db.ExecContext(context.Background(), fmt.Sprintf("USE `%s`", dbName))
  // provider_mysql.go:177
  colRows, err := queryStrings(db, fmt.Sprintf("SHOW FULL COLUMNS FROM `%s`.`%s`", dbName, tableName))
  ```

**AUDIT-database-02: SQL injection in MySQL `Quote` — backticks in identifiers not escaped**
- File: `backend/database/provider_mysql.go:45-47`
- Severity: P0
- Failure scenario: `p.Quote(name)` is `` "`" + name + "`" ``. A `name` containing `` ` `` (e.g. column name crafted via the connection profile / sync JSON) breaks out. Callsite risk: `provider_mysql.go:71` (`INSERT`), `:85` (`UPDATE SET`), `:117` (`DELETE WHERE`), `:326` (`ALTER TABLE ... DROP COLUMN`), `:346` (PK cols), `:356` (`CREATE INDEX cols`), `:377` (`DROP INDEX`), `:419,426,441,443,445` (column SQL builder). On every CRUD path, the user can name a column `` `; DROP TABLE users; -- `` and the resulting SQL executes the trailing statements.
- Fix category: escape internal backticks by doubling (`` "`" + strings.ReplaceAll(name, "`", "``") + "`" ``), and reject empty / overly long names.
- Evidence:
  ```go
  func (p *mysqlProvider) Quote(name string) string {
      return "`" + name + "`"
  }
  ```

**AUDIT-database-03: SQL injection in PostgreSQL `Quote` — double quotes not escaped**
- File: `backend/database/provider_postgres.go:41-43`
- Severity: P0
- Failure scenario: `p.Quote(name)` is `"` + name + `"` without escaping `"`. A `name` of `x"; DROP TABLE users; --` produces `"x"; DROP TABLE users; --"`, which Postgres parses as identifier `x`, then `DROP TABLE users`, then a comment. Callsite risk is identical to the MySQL provider — every CRUD/DDL path (`InsertRow:64`, `UpdateRow:94`, `DeleteRow:115`, DDL `CreateDatabase:253`, `DropDatabase:259`, `CreateTable:267`, `DropTable:273`, `DropView:279`, `TruncateTable:285`, `AddColumn:313`, `ModifyColumn:322-347`, `DropColumn:365`, `AddIndex:379,389`, `DropIndex:398,401`).
- Fix category: double internal `"` (`strings.ReplaceAll(name, `"`, `""`)`); reject illegal chars.
- Evidence:
  ```go
  func (p *postgresProvider) Quote(name string) string {
      return `"` + name + `"`
  }
  ```

**AUDIT-database-04: SQL injection in PostgreSQL `ModifyColumn`/`AddColumn` `SET DEFAULT` clause**
- Files:
  - `backend/database/provider_postgres.go:307` — `DEFAULT ` + col.DefaultVal
  - `backend/database/provider_postgres.go:339-344` — `SET DEFAULT %s` with `col.DefaultVal`
- Severity: P0
- Failure scenario: `ColumnDef.DefaultVal` flows from `App.AddColumn` / `App.ModifyColumn` (`app.go:3891-3904`) directly into the SQL string without quoting or escaping. If a user enters `42); DROP TABLE users; --` (or an array literal `'{1,2}'::int[]` that smuggles a closing `)` plus `;`), the trailing statement executes. Postgres `SET DEFAULT` does not support bound parameters.
- Fix category: validate `DefaultVal` against the chosen column type (numeric columns → numeric regex; string columns → escape `'`, wrap in `'`; functions → whitelist `now()`, `current_timestamp`, etc.); reject anything that contains `;`, `--`, `/*`, `\`.
- Evidence:
  ```go
  case "value":
      if col.DefaultVal != "" {
          parts = append(parts, "DEFAULT "+col.DefaultVal)
      ...
  stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s",
      q(tableName), q(col.Name), col.DefaultVal))
  ```

**AUDIT-database-05: SQL injection in rqlite `Quote` — double quotes not escaped**
- File: `backend/database/provider_rqlite.go:41-43`
- Severity: P0
- Failure scenario: Same shape as Postgres `Quote` (`"` + name + `"` without escape). Used at `provider_rqlite.go:50` (`SELECT * FROM`), `:64` (`INSERT`), `:94` (`UPDATE`), `:115` (`DELETE`), `:236,242,248,254,285,296,315,323`. The rqlite HTTP driver (`gorqlite`) does not allow multi-statement requests, so this is partially mitigated by the driver — but the same payload still breaks identifier parsing and the rqlite server logs / surfaces unexpected behaviour. Flag for hardening before the driver changes.
- Fix category: escape `"` by doubling, identical to Postgres.
- Evidence:
  ```go
  func (p *rqliteProvider) Quote(name string) string {
      return `"` + name + `"`
  }
  ```

---

### P1 — High

**AUDIT-database-06: PostgreSQL `DSN` hard-codes `sslmode=disable` (extraParams can override but default is insecure)**
- File: `backend/database/provider_postgres.go:18-35`
- Severity: P1
- Failure scenario: Any Postgres connection that does not explicitly set `sslmode` in `extraParams` opens a plaintext channel. Connection config persisted in `connections.json` (syncable across users) silently downgrades on first launch. Similar to the `backend/k8s/client.go:65` / `ftp_session.go:58` TLS-skip pattern already called out in CONCERNS.md, but unmitigated here.
- Fix category: default to `sslmode=require` (or `verify-full` if `sslrootcert` provided), surface a warning when overridden to `disable`; require user opt-in for the insecure default.
- Evidence:
  ```go
  q.Set("sslmode", "disable")
  for k, v := range extraParams {
      q.Set(k, v)
  }
  ```

**AUDIT-database-07: Connection pool has no limits — `sql.Open` is called with no `SetMaxOpenConns/SetMaxIdleConns/SetConnMaxLifetime`**
- File: `backend/database/engine.go:37-47`
- Severity: P1
- Failure scenario: `NewDB` returns an `*sql.DB` whose pool grows unbounded. A user that opens dozens of DB tabs and runs slow `ExecuteQuery` calls (no timeout — see AUDIT-09) exhausts the underlying DB server's `max_connections`, then every subsequent query hangs. Also impacts the desktop app's file descriptor / goroutine budget. Each tab likely opens a fresh `*sql.DB` via `ds.DB()` — let me re-check, the `App` likely reuses it (need to confirm in follow-up). Either way, the unbounded setting is wrong.
- Fix category: in `NewDB` call `db.SetMaxOpenConns(10)`, `db.SetMaxIdleConns(5)`, `db.SetConnMaxLifetime(30 * time.Minute)`; surface as a setting.
- Evidence:
  ```go
  func NewDB(dbType, dsn string) (*sql.DB, error) {
      p, err := NewProvider(dbType)
      if err != nil {
          return nil, err
      }
      db, err := sql.Open(p.DriverName(), dsn)
      if err != nil {
          return nil, fmt.Errorf("open %s: %w", dbType, err)
      }
      return db, nil
  }
  ```

**AUDIT-database-08: MySQL DDL paths emit `USE` and the actual DDL on potentially different pooled connections — `PrepareExec` is bypassed**
- Files:
  - `backend/database/provider_mysql.go:256-264` (`CreateTable`)
  - `backend/database/provider_mysql.go:266-274` (`DropTable`)
  - `backend/database/provider_mysql.go:276-284` (`DropView`)
  - `backend/database/provider_mysql.go:286-294` (`TruncateTable`)
  - `backend/database/provider_mysql.go:298-307` (`AddColumn`)
  - `backend/database/provider_mysql.go:309-318` (`ModifyColumn`)
  - `backend/database/provider_mysql.go:320-328` (`DropColumn`)
  - `backend/database/provider_mysql.go:332-360` (`AddIndex`)
  - `backend/database/provider_mysql.go:362-379` (`DropIndex`)
- Severity: P1
- Failure scenario: These functions call `db.Exec("USE \`db\`")` then `db.Exec("CREATE TABLE ...")` as two **separate** pool calls. `database/sql` may pick a different connection for the second Exec. If the second conn never executed `USE`, MySQL errors with `No database selected`. The bug is masked today only because most users stick to a single default DB, but cross-DB DDL operations (e.g. user opens `mysql.system` to inspect, then DDL on `app.users`) will intermittently fail. This is the same shape as the already-merged `DeferConnect` fix (`66d137d`).
- Fix category: hold a single `*sql.Conn` (`db.Conn(ctx)`) per call, run `USE` then DDL on it, `defer conn.Close()`. Or use `execPrepared` which already does this.
- Evidence:
  ```go
  // provider_mysql.go:256-264
  func (p *mysqlProvider) CreateTable(db *sql.DB, dbName, tableName string) error {
      if dbName != "" {
          if _, err := db.Exec(fmt.Sprintf("USE `%s`", dbName)); err != nil {
              return err
          }
      }
      _, err := db.Exec(fmt.Sprintf("CREATE TABLE `%s` (id INT AUTO_INCREMENT PRIMARY KEY)", tableName))
      return err
  }
  ```

**AUDIT-database-09: `ExecuteQuery`/`ExecuteStatement` use `context.Background()` with no timeout — UI can hang forever**
- File: `backend/database/executor.go:24-99`
- Severity: P1
- Failure scenario: `ExecuteQuery` and `ExecuteStatement` are the Wails-bound functions (`app.go:3843,3851`) reachable from the WebView. Both pin `ctx := context.Background()` and never derive a deadline. A slow / locked query (e.g. `SELECT * FROM huge_table` waiting on a row lock) leaves the UI spinner forever; closing the DB tab cannot cancel the underlying conn because the pool does not surface the `ctx`. Similar concern applies to every `db.ExecContext(context.Background(), ...)` in the provider files (`provider_mysql.go:53`, `provider_sqlserver.go:54,80,114,136`, `provider_oracle.go:53`).
- Fix category: add a `QueryTimeout` parameter (or constant, default 30s) and propagate via `context.WithTimeout`; expose cancellation when the tab closes.
- Evidence:
  ```go
  func ExecuteQuery(p Provider, db *sql.DB, dbName, sqlStr string) (*QueryResult, error) {
      ctx := context.Background()
      conn, err := db.Conn(ctx)
      ...
  }
  ```

**AUDIT-database-10: SQL Server `InsertRow`/`UpdateRow`/`DeleteRow` bypass `execPrepared` — `withUse` workaround races the pool**
- Files:
  - `backend/database/provider_sqlserver.go:68-82` (`InsertRow`)
  - `backend/database/provider_sqlserver.go:84-116` (`UpdateRow`)
  - `backend/database/provider_sqlserver.go:118-138` (`DeleteRow`)
- Severity: P1
- Failure scenario: Unlike other providers, these call `db.ExecContext(context.Background(), p.withUse(dbName, sql), args...)` directly. `withUse` prepends `USE [dbName];` to the user SQL and runs them as a multi-statement batch. The Go `database/sql` layer does call `Exec` on a single conn, so the multi-statement trick keeps `USE` and the actual SQL on the same wire — but two things break: (1) the driver may not return the last result set's metadata correctly when the leading USE produces no rows; (2) if the user SQL is itself a multi-statement batch (`SELECT 1; SELECT 2`), the mssql driver (`microsoft/go-mssqldb`) restricts to single-statement batches by default and silently fails. Result: ambiguous errors, missing affected-row counts, no context propagation.
- Fix category: mirror `execPrepared` pattern — acquire `db.Conn(ctx)`, run `USE` on it, then run the parameterised CRUD on the same conn, `defer conn.Close()`.
- Evidence:
  ```go
  // provider_sqlserver.go:78-82
  sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
      p.qualifiedTable(tableName), strings.Join(quotedCols, ", "), strings.Join(placeholders, ", "))
  _, err := db.ExecContext(context.Background(), p.withUse(dbName, sql), args...)
  ```

**AUDIT-database-11: SQL Server `DropColumn` leaks `Rows` on early-return paths**
- File: `backend/database/provider_sqlserver.go:352-381`
- Severity: P1
- Failure scenario: `db.Query` is called without `defer rows.Close()`. The function manually calls `rows.Close()` in three places (after the loop, in the scan-error path, and after constraints). However, if `db.Query` returns a non-nil error **before** `rows` is created (impossible here, but) or if the rows variable is somehow shadowed by an unreached branch, the connection's cursor leaks. More realistically: any future edit that adds an early `return` between line 357 and 370 will leak. PLAUSIBLE without a runnable test.
- Fix category: refactor to `rows, err := db.QueryContext(ctx, ...); if err != nil { return ... }; defer rows.Close()`. Standard Go pattern.
- Evidence:
  ```go
  rows, err := db.Query(p.withUse(dbName, `...`), tableName, colName)
  if err != nil {
      return fmt.Errorf("find default constraints: %w", err)
  }
  var constraints []string
  for rows.Next() {
      var name string
      if err := rows.Scan(&name); err != nil {
          rows.Close()
          return err
      }
      constraints = append(constraints, name)
  }
  rows.Close()
  ```

**AUDIT-database-12: Oracle `PrepareExec` `ALTER SESSION SET CURRENT_SCHEMA` does not handle quoted identifiers with mixed case**
- File: `backend/database/provider_oracle.go:49-55`
- Severity: P1
- Failure scenario: `dbName` arrives from the WebView (uppercase, lowercase, or quoted form). `p.Quote(dbName)` wraps in `"..."` and doubles internal `"`, which is correct. BUT: Oracle treats unquoted identifiers case-insensitively (always uppercased), so a lowercase `myschema` becomes `MYSCHEMA` and never matches the user's `myschema` schema. The connection's initial schema (set by the DSN or `ALTER SESSION SET CURRENT_SCHEMA = "MYSCHEMA"`) will not equal the user's intent. Subsequent queries on tables named in mixed case fail with "table or view does not exist".
- Fix category: either always uppercase `dbName` before quoting (and document this in the UI), or set `CURRENT_SCHEMA` via a parameterised `EXECUTE IMMEDIATE` that preserves case; failing that, warn the user in the connection panel.
- Evidence:
  ```go
  _, err := db.ExecContext(context.Background(), fmt.Sprintf("ALTER SESSION SET CURRENT_SCHEMA = %s", p.Quote(dbName)))
  ```

---

### P2 — Medium

**AUDIT-database-13: `Provider.Register` mutates a global map without synchronization — concurrent `Register` would race**
- File: `backend/database/provider.go:94-99`
- Severity: P2
- Failure scenario: `var providers = map[string]Provider{}` is a plain map. `Register` writes to it; `NewProvider` reads from it. Today all calls happen from `init()` (single-threaded), so this is benign. But the exported function signature invites third-party packages (or a future plugin system) to call `Register` at runtime, and Go's race detector will flag any concurrent write/read. Also, the function silently overwrites an existing entry — no error if two providers register the same type.
- Fix category: protect the map with `sync.RWMutex`, or use `sync.Map`; make `Register` idempotent / return an error on duplicate.
- Evidence:
  ```go
  var providers = map[string]Provider{}
  func Register(dbType string, p Provider) {
      providers[dbType] = p
  }
  ```

**AUDIT-database-14: MySQL DSN hard-codes `loc=Local` while `parseTime=true` — server-vs-local timezone mismatch silently shifts datetimes**
- File: `backend/database/provider_mysql.go:28-34`
- Severity: P2
- Failure scenario: `loc=Local` tells the driver to interpret `DATETIME`/`TIMESTAMP` columns in the OS local zone. If the MySQL server is UTC (or another zone) and the desktop client is JST/KST/CST, the rendered value is wrong by the timezone offset. Worse: `TIMESTAMP` columns are already stored as UTC instants, so the value gets converted twice. PLAUSIBLE — actual impact depends on the user's server config; no test catches this.
- Fix category: default `loc=UTC` (or expose a `loc` setting in the connection form), and surface a warning when `parseTime=true` is enabled without `loc` set explicitly.
- Evidence:
  ```go
  cfg.Params = map[string]string{
      "charset":      "utf8mb4",
      "parseTime":    "true",
      "loc":          "Local",
      "timeout":      "10s",
      "readTimeout":  "30s",
  }
  ```

**AUDIT-database-15: `MySQL DSN` drops `writeTimeout` (only `readTimeout` set)**
- File: `backend/database/provider_mysql.go:33-34`
- Severity: P2
- Failure scenario: DSN params list `readTimeout=30s` but no `writeTimeout`. A stuck `INSERT` on a locked row can block forever; user clicks "Cancel" but the goroutine still waits on the socket. The `go-sql-driver/mysql` driver supports both.
- Fix category: add `writeTimeout=30s` (or 10s) to default params; allow override via `extraParams`.
- Evidence:
  ```go
  "timeout":      "10s",
  "readTimeout":  "30s",
  // no writeTimeout
  ```

**AUDIT-database-16: SQL Server `DropDatabase` ignores the `ALTER DATABASE ... SET SINGLE_USER` failure then unconditionally drops**
- File: `backend/database/provider_sqlserver.go:292-296`
- Severity: P2
- Failure scenario: `_, _ = db.Exec(...SINGLE_USER...)` discards the error. If the `SINGLE_USER` transition fails (active sessions, permission denied, mirror), the `DROP DATABASE` will hang on locks or return a less informative error. Worse, the user sees a half-completed operation: SINGLE_USER not actually applied, DROP succeeds (or fails) without context.
- Fix category: check the first error, return wrapped error with both operation's status; consider switching to `WITH NO_WAIT` and explicit transaction.
- Evidence:
  ```go
  func (p *sqlserverProvider) DropDatabase(db *sql.DB, dbName string) error {
      _, _ = db.Exec(fmt.Sprintf("ALTER DATABASE %s SET SINGLE_USER WITH ROLLBACK IMMEDIATE", p.Quote(dbName)))
      _, err := db.Exec(fmt.Sprintf("DROP DATABASE %s", p.Quote(dbName)))
      return err
  }
  ```

**AUDIT-database-17: Oracle `ModifyColumn` rebuilds column without applying new comment / nullability in correct order**
- File: `backend/database/provider_oracle.go:279-291`
- Severity: P2
- Failure scenario: `ModifyColumn` issues `ALTER TABLE ... MODIFY (...)` then a separate `COMMENT ON COLUMN ...` statement. If the MODIFY fails (e.g. data does not fit the new size) the comment is not applied — fine. But if `col.Comment` is non-empty, Oracle can fail the MODIFY because COMMENT and the column-level spec conflict on certain Oracle versions (12c R2 vs 19c). The two-statement approach also opens a window where another session sees inconsistent metadata.
- Fix category: merge into a single `ALTER TABLE ... MODIFY (...)` with `MODIFY (col TYPE ... COMMENT '...')` is not supported; use `dbms_metadata` or run within a single transaction.
- Evidence:
  ```go
  func (p *oracleProvider) ModifyColumn(db *sql.DB, dbName, tableName string, col ColumnDef) error {
      table := p.qualifiedTable(dbName, tableName)
      stmts := []string{p.buildColumnSQL("MODIFY", table, col)}
      if col.Comment != "" {
          stmts = append(stmts, p.commentOnColumnSQL(dbName, tableName, col.Name, col.Comment))
      }
      ...
  }
  ```

**AUDIT-database-18: MySQL `DefaultTableQuery` / `OracleProvider.DefaultTableQuery` / etc. use `int` for limit without validation**
- Files:
  - `backend/database/provider_mysql.go:57-59`
  - `backend/database/provider_oracle.go:57-59`
  - `backend/database/provider_postgres.go:49-51`
  - `backend/database/provider_sqlserver.go:58-60`
  - `backend/database/provider_rqlite.go:49-51`
- Severity: P2
- Failure scenario: `limit int` is formatted with `%d`. A negative `limit` from the frontend (no validation on `App`-side either, presumably) renders `SELECT * FROM \`t\` LIMIT -1` (MySQL treats as no limit) or `SELECT * FROM "t" WHERE ROWNUM <= -1` (Oracle returns zero rows). Not a security bug but a UX bug — the grid silently returns nothing when user expects 100 rows.
- Fix category: clamp `limit` to `[1, maxLimit]` in `DefaultTableQuery` or in the App layer; default to 100 on zero/negative.
- Evidence:
  ```go
  return fmt.Sprintf("SELECT * FROM %s LIMIT %d", p.Quote(tableName), limit)
  ```

**AUDIT-database-19: rqlite `GetTableSchema` silently skips index column enumeration on error**
- File: `backend/database/provider_rqlite.go:209-216`
- Severity: P2
- Failure scenario: `colRows, err := queryStrings(db, fmt.Sprintf("PRAGMA index_info(%s)", q(idx["name"])))` — if `err != nil`, `info.Columns` is left as the zero value (nil). The function still returns an `IndexInfo` with `Columns = nil`, which the frontend renders as an empty index — masking the actual schema-introspection failure. User sees a partial schema.
- Fix category: propagate the error, or at minimum log via `log.Writef` and mark the entry as `IsPrimary=false`.
- Evidence:
  ```go
  for _, idx := range idxRows {
      info := IndexInfo{
          Name:   idx["name"],
          Unique: idx["unique"] == "1",
      }
      colRows, err := queryStrings(db, fmt.Sprintf("PRAGMA index_info(%s)", q(idx["name"])))
      if err == nil {
          for _, c := range colRows {
              info.Columns = append(info.Columns, c["name"])
          }
      }
      indexes = append(indexes, info)
  }
  ```

**AUDIT-database-20: PostgreSQL `GetTableSchema` swallows primary-key lookup errors silently**
- File: `backend/database/provider_postgres.go:202-214`
- Severity: P2
- Failure scenario: The PK lookup runs after the column lookup; if the PK query fails (e.g. user lacks `pg_class` SELECT privilege), `err == nil` branch is skipped, and every column is reported as `IsPrimary=false`. The user then sees a schema with no PK on a table that actually has one — silently misleading for data-modification safety decisions.
- Fix category: log the error via `log.Writef` and continue; or surface a warning to the UI ("primary key info unavailable").
- Evidence:
  ```go
  pkRows, err := queryStrings(db, ...)
  if err == nil {
      for _, row := range pkRows { ... columns[i].IsPrimary = true }
  }
  // err is dropped
  ```

**AUDIT-database-21: `queryStrings` returns `[]map[string]string` losing `NULL` distinction and mis-rendering numeric/bool types**
- File: `backend/database/engine.go:50-79`
- Severity: P2
- Failure scenario: `queryStrings` (used by every schema-discovery path across providers — `provider_mysql.go:140,152,177,213,426`, `provider_postgres.go:141,155,172,202,217`, `provider_oracle.go:145,160,189,393,413`, `provider_sqlserver.go:156,171,196,206,249`, `provider_rqlite.go:142,160,190,199,210`) loses three pieces of information: (a) NULL values become `""` instead of a distinct sentinel — column `c IS NULL` cannot be distinguished from empty string; (b) numeric columns are coerced via `fmt.Sprintf("%v", v)` — `1` and `"1"` become indistinguishable; (c) the rows map uses the column name as key — duplicate column names (e.g. joins) silently overwrite. `queryAny` was added as a sibling but the schema-discovery paths still use `queryStrings`, so NULLs are flattened in the very place where the user is most likely to inspect defaults and nullability.
- Fix category: migrate schema discovery to `queryAny` (already exists in `engine.go:83`), or add a typed scan helper. Keep `queryStrings` for places where stringy output is acceptable.
- Evidence:
  ```go
  func scanToString(v any) string {
      if v == nil {
          return ""
      }
      switch s := v.(type) {
      case []byte:
          return string(s)
      default:
          return fmt.Sprintf("%v", v)
      }
  }
  ```

**AUDIT-database-22: `queryStrings` returns no rows when `Rows.Columns()` fails — silently drops column list**
- File: `backend/database/engine.go:57-60`
- Severity: P2
- Failure scenario: If `rows.Columns()` returns an error mid-iteration (rare but possible with broken drivers), `queryStrings` returns `(nil, err)` — caller then iterates `len(nil)` rows. No rows, no clue. The issue is that `Columns()` is called before iterating, so the failure means every schema-discovery call returns nothing.
- Fix category: minor — wrap the error with context (`fmt.Errorf("get columns: %w", err)`) and consider falling back to `rows.Columns()` after a single `rows.Next()`.
- Evidence:
  ```go
  cols, err := rows.Columns()
  if err != nil {
      return nil, err
  }
  ```

---

### P3 — Low / Informational

**AUDIT-database-23: MySQL `buildDropPK` case-sensitive lookup of column types may miss columns**
- File: `backend/database/provider_mysql.go:423-446`
- Severity: P3
- Failure scenario: `colTypes[row["Field"]]` uses MySQL's `Field` (the actual stored case) as the map key. The caller passes `autoIncCols` in the case the user typed (e.g. `id` vs `ID`). If the user-typed case doesn't match the stored case, the `MODIFY COLUMN` falls back to `INT` default, silently dropping precision (`bigint` → `int`). Not a security bug, just a precision regression.
- Fix category: lookup using `strings.EqualFold`, or always uppercase/lowercase both sides.
- Evidence:
  ```go
  colTypes[row["Field"]] = row["Type"]
  ...
  ct, ok := colTypes[c]
  if !ok {
      ct = "INT"
  }
  ```

**AUDIT-database-24: `MySQL GetTables` does not handle views vs tables returned by `SHOW FULL TABLES` correctly on MySQL 5.7**
- File: `backend/database/provider_mysql.go:151-174`
- Severity: P3
- Failure scenario: `SHOW FULL TABLES` returns one row per table/view; the second column is the type (`BASE TABLE` or `VIEW`). The parser iterates the row map and assigns `tp` from whichever key is NOT `Tables_in_*`. On MySQL 5.7 the second column is `Table_type` (PascalCase) — fine. But on older MySQL 5.5 / MariaDB 10.1 the column header is lowercase. Since the loop reads by `HasPrefix`, only the type column is captured (the other key wins). PLAUSIBLE — the schema discovery works on modern MySQL but may mislabel `VIEW` as `BASE TABLE` on older forks.
- Fix category: query by column name (`SHOW FULL TABLES FROM ... WHERE Table_type = 'VIEW'`) and rely on result ordering instead of map keys.
- Evidence:
  ```go
  for key, val := range row {
      if strings.HasPrefix(key, "Tables_in_") {
          name = val
      } else {
          tp = val
      }
  }
  ```

**AUDIT-database-25: `DefaultVal` parser assumes PostgreSQL `column_default` strips parens — but function defaults start with parens**
- File: `backend/database/provider_sqlserver.go:232`
- Severity: P3
- Failure scenario: `defVal = strings.Trim(defVal, "()")` trims **all** leading/trailing parens. A SQL Server default like `('7')` becomes `'7'` (intended), but `((0))` becomes `0` (still works), and `('(' + 'a' + ')')` becomes `'(' + 'a' + ')'` (loses outer parens that make it a string expression). Mostly works in practice but breaks for expression defaults containing parens.
- Fix category: only trim a single leading/trailing paren pair, then validate.
- Evidence:
  ```go
  defVal = strings.Trim(defVal, "()")
  ```

**AUDIT-database-26: `Register` silently overwrites an existing provider**
- File: `backend/database/provider.go:97-99`
- Severity: P3
- Failure scenario: Two `init()` functions both calling `Register("mysql", ...)` would silently replace the first with the second. The order of `init()` is determined by import dependency, which is non-obvious across the codebase. A future refactor that accidentally double-registers would silently break the original.
- Fix category: panic on duplicate registration, or return an error from `Register` (which requires making it `Register() error`).
- Evidence:
  ```go
  func Register(dbType string, p Provider) {
      providers[dbType] = p
  }
  ```

**AUDIT-database-27: Oracle `Quote` is correct but used inconsistently — `CreateTable`/`DropTable` skip the schema qualifier when dbName is empty, but `CreateTable` SQL builder always wraps `ID` even when caller passes an empty column name**
- File: `backend/database/provider_oracle.go:351-353`
- Severity: P3
- Failure scenario: `createTableSQL` always uses `p.Quote("ID")` as the PK column name, hardcoding uppercase. A user creating a table with a non-English locale will get `ID` regardless of preference. Minor.
- Fix category: pass an explicit column-name argument.
- Evidence:
  ```go
  return fmt.Sprintf("CREATE TABLE %s (%s NUMBER PRIMARY KEY)", p.qualifiedTable(dbName, tableName), p.Quote("ID"))
  ```

**AUDIT-database-28: `ColumnInfo.DefaultType` enum is a free-form string, no validation in `AddColumn` / `ModifyColumn`**
- File: `backend/database/schema.go:14-21`, callers across all providers
- Severity: P3
- Failure scenario: `DefaultType` is documented as `"none" | "null" | "value" | "auto"` but the providers only `switch` on `"null"`, `"value"`, `"auto"`. A typo from the WebView (e.g. `"auto "`, `"AUTO"`, `"increment"`) falls through the switch with no effect — column is added without a default. The user sees a silent no-op rather than an error.
- Fix category: validate at the App layer (`App.AddColumn`) before invoking the provider; surface validation errors back to the UI.
- Evidence:
  ```go
  switch col.DefaultType {
  case "null":
      ...
  case "value":
      ...
  case "auto":
      ...
  }
  ```

**AUDIT-database-29: `sqlserver ModifyColumn` always issues the column-type alter twice (nullable + non-nullable branch)**
- File: `backend/database/provider_sqlserver.go:331-340`
- Severity: P3
- Failure scenario: `ModifyColumn` builds two `ALTER COLUMN` statements: one with the type and one with the nullability. The second one (with explicit `NULL` or `NOT NULL`) re-types the column, doubling the rewrite cost on large tables. `ALTER TABLE` in SQL Server is metadata-only when null/type both change and there's no data conversion, but in practice the second alter triggers an unnecessary scan. Not a correctness bug, just wasteful.
- Fix category: collapse into a single `ALTER TABLE ... ALTER COLUMN ... TYPE ... NULL/NOT NULL` statement.
- Evidence:
  ```go
  stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s",
      q(tableName), p.Quote(col.Name), col.Type))
  if col.Nullable {
      stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NULL", ...))
  } else {
      stmts = append(stmts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s %s NOT NULL", ...))
  }
  ```

**AUDIT-database-30: `execPrepared` parameter naming shadows the `sql` package**
- File: `backend/database/provider.go:78-92`
- Severity: P3
- Failure scenario: `func execPrepared(p Provider, db *sql.DB, dbName, sql string, args []any)` — the `sql` parameter name shadows the imported `database/sql` package within the function body. The function does not reference `sql` inside, so this works, but future maintainers adding e.g. `sql.IsolationLevel` will hit a name collision. Style only.
- Fix category: rename parameter to `query` or `stmt`.
- Evidence:
  ```go
  func execPrepared(p Provider, db *sql.DB, dbName, sql string, args []any) error {
      ...
      _, err = conn.ExecContext(ctx, sql, args...)
      return err
  }
  ```

**AUDIT-database-31: No `Ping`/connection health check before returning `*sql.DB`**
- File: `backend/database/engine.go:37-47`
- Severity: P3
- Failure scenario: `sql.Open` does not actually connect — it only validates the DSN format. `NewDB` returns a `*sql.DB` that the user later queries, and only then discovers the DSN is wrong (typo in host, wrong port, firewall block). Better UX would be `db.PingContext(ctx)` with a short timeout before returning. Performance/UX, not security.
- Fix category: add `db.PingContext(ctx)` after `sql.Open`, propagate the error.
- Evidence:
  ```go
  db, err := sql.Open(p.DriverName(), dsn)
  if err != nil {
      return nil, fmt.Errorf("open %s: %w", dbType, err)
  }
  return db, nil
  ```

**AUDIT-database-32: `MySQL DropIndex` primary-key branch ignores `autoIncCols` correctly but does not wrap MODIFY in a transaction**
- File: `backend/database/provider_mysql.go:362-379`
- Severity: P3
- Failure scenario: `DropIndex` for a primary key builds `ALTER TABLE ... MODIFY COLUMN ... NOT NULL, DROP PRIMARY KEY`. If `MODIFY` succeeds but `DROP PRIMARY KEY` fails (FK constraint referencing the PK), the table is left in an inconsistent state — column is now NOT NULL but PK still exists, and the AUTO_INCREMENT may have been silently dropped on some MariaDB versions. Run in a transaction (`db.Begin()`) so the MODIFY rolls back if DROP fails.
- Fix category: wrap in `sql.Tx`.
- Evidence:
  ```go
  sql, err := p.buildDropPK(db, tableName, autoIncCols)
  ...
  _, err = db.Exec(sql)
  ```

---

## Cross-cutting concerns within module

1. **Identifier-quoting discipline is inconsistent across providers.**
   - Oracle (`provider_oracle.go:45-47`) and SQL Server (`provider_sqlserver.go:46-48`) properly escape the inner quote character.
   - MySQL, PostgreSQL, and rqlite do not. They wrap in `` ` `` / `"` and trust the caller. Combined with the `dbName` interpolation at AUDIT-01 and the column-name flow at AUDIT-02/03/05, the entire CRUD + DDL surface is exposed.
   - Fix shape: a single helper `safeIdentifier(name) (string, error)` returning an error on illegal chars, called from every `Quote`.

2. **DSN construction is per-provider, with different defaults for TLS.**
   - Postgres hard-codes `sslmode=disable` (AUDIT-06).
   - SQL Server sets `encrypt=disable` (`provider_sqlserver.go:32`).
   - MySQL has no `tls=` parameter; falls back to driver's default.
   - Oracle uses go-ora's default which is plaintext unless TLS params set.
   - rqlite uses `http://` plaintext.
   - Fix shape: a single TLS-config matrix keyed by `dbType`, surfaced in the connection form.

3. **No `context.WithTimeout` anywhere.** Every query path uses `context.Background()`. There is no way for the UI to cancel a long-running query even if it had a Cancel button — see `DBQueryEditor.vue:303,306` invocation. The only escape hatch is closing the DB tab, which kills the pool, not the query.

4. **`PrepareExec` is asymmetric across providers.** MySQL/Oracle/SQL Server do something useful, Postgres/rqlite are no-ops, and SQL Server's "useful" thing is bolted onto every query via `withUse` rather than going through `execPrepared`. This shape is the root cause of AUDIT-08 and AUDIT-10.

5. **Schema-discovery paths share one buggy helper (`queryStrings`)** that flattens NULLs and confuses types (AUDIT-21). A `queryAny` variant exists but is unused by schema discovery.

6. **`sql.Open` returns a `*sql.DB` that is never pinged, never has pool limits configured, and is closed only when the App layer decides.** This is the root cause of AUDIT-07 (no pool limits) and AUDIT-31 (no ping).

7. **No tests exist for any provider.** This was already called out in CONCERNS.md (Test Coverage Gaps → `backend/database/*.go`), and every finding above is reproducible only by manual `wails dev` against a real database. The conservative-fix follow-up should add at minimum: an identifier-validation unit test, a DSN unit test for password-with-special-chars, and an integration smoke for each provider against a docker-compose service.

---

## Summary

- Total findings: 32 (P0: 5, P1: 7, P2: 10, P3: 10)
- Confidence: **medium-high** for P0/P1 (all reproducible by code reading); **medium** for P2 (some PLAUSIBLE marks on items requiring a live DB to confirm); **medium-low** for P3 (style / hardening).

The five P0 findings all share the same root cause: **identifier and dbName values are interpolated into SQL strings without escaping**, and three of the five providers (`mysql`, `postgres`, `rqlite`) do not even escape inner quote characters in their `Quote` helper. Combined with the fact that `dbName` arrives from WebView JSON (`App.GetTables`, `App.GetTableSchema`, `App.CreateDatabase`, etc.) and from the synced connection profile, this is reachable from the JS side and from a malicious synced profile.

The P1 layer is dominated by resource / lifecycle issues: pool unbounded (AUDIT-07), no timeouts (AUDIT-09), and DDL paths that race the connection pool by issuing `USE` and the DDL on separate `db.Exec` calls (AUDIT-08, AUDIT-10).

---

Found 32 findings: P0=5, P1=7, P2=10. Top concern: **AUDIT-database-01 — MySQL `dbName` interpolated raw into backtick-wrapped SQL across every DDL/schema path, allowing arbitrary SQL execution via the WebView-bound `App` methods or a synced connection profile.**