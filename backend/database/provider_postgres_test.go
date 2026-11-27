package database

import (
"context"
"database/sql"
"database/sql/driver"
"errors"
"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The PostgreSQL tree reuses the MSSQL composite-node encoding
// ("db.schema" nodes, schema-qualified table names); these tests lock the
// quoting and node-splitting behavior.

func TestPostgresQualifyTable(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"employees", `"employees"`},
		{"sales.employees", `"sales"."employees"`},
		{"my.db.sales", `"my.db"."sales"`},
	}
	p := &postgresProvider{}
	for _, tc := range cases {
		if got := p.QualifyTable(tc.in); got != tc.want {
			t.Errorf("QualifyTable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPostgresQualifyWithNodeSchema(t *testing.T) {
	cases := []struct {
		dbName    string
		tableName string
		want      string
	}{
		{"shop.sales", "newtbl", "sales.newtbl"},
		{"shop.sales", "dbo.explicit", "dbo.explicit"},
		{"shop", "newtbl", "newtbl"},
		{"", "newtbl", "newtbl"},
	}
	p := &postgresProvider{}
	for _, tc := range cases {
		if got := p.qualifyWithNodeSchema(tc.dbName, tc.tableName); got != tc.want {
			t.Errorf("qualifyWithNodeSchema(%q, %q) = %q, want %q",
				tc.dbName, tc.tableName, got, tc.want)
		}
	}
}

// TestPostgresDSNParsesWithPgx locks the DSN shape the provider generates to
// what the pgx stdlib driver accepts. lib/pq was lenient about unknown URL
// query parameters; pgx treats them as runtime parameters, but a handful of
// lib/pq-era keywords are rejected outright — a regression here would break
// every postgres connection at sql.Open time.
func TestPostgresDSNParsesWithPgx(t *testing.T) {
	cases := []struct {
		name        string
		extraParams string
	}{
		{"plain", ""},
		{"sslmode override", "sslmode=require"},
		{"connect timeout", "connect_timeout=10"},
		{"runtime param", "search_path=public,app"},
		{"combined", "sslmode=verify-full&connect_timeout=5&application_name=uniterm"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dsn, err := BuildDSN("postgres", "pg.example.com", "user", "pass", "appdb", tc.extraParams, 5433)
			if err != nil {
				t.Fatalf("BuildDSN: %v", err)
			}
			cfg, err := pgx.ParseConfig(dsn)
			if err != nil {
				t.Fatalf("pgx rejected generated DSN %q: %v", dsn, err)
			}
			if cfg.Host != "pg.example.com" || cfg.Port != 5433 {
				t.Errorf("host/port mismatch: host=%q port=%d", cfg.Host, cfg.Port)
			}
		})
	}
}


// TestPostgresSchemaCacheKey verifies the cache is keyed by "dbName.tableName"
// so two tables in the same database have distinct entries, and the same
// (dbName, tableName) tuple maps to the same SchemaResult pointer.
func TestPostgresSchemaCacheKey(t *testing.T) {
	// Use distinct keys to avoid stomping on entries left by other tests.
	const dbName = "test_cache_db_key"
	const tbl1 = "users_key"
	const tbl2 = "orders_key"

	// Wipe any prior state for these keys so the test is hermetic.
	postgresSchemaCache.Delete(dbName + "." + tbl1)
	postgresSchemaCache.Delete(dbName + "." + tbl2)

	r1 := &SchemaResult{Columns: []ColumnInfo{{Name: "id", Type: "int"}}}
	r2 := &SchemaResult{Columns: []ColumnInfo{{Name: "qty", Type: "int"}}}

	postgresSchemaCache.Store(dbName+"."+tbl1, r1)
	postgresSchemaCache.Store(dbName+"."+tbl2, r2)

	// Same key returns same pointer.
	if got, ok := postgresSchemaCache.Load(dbName + "." + tbl1); !ok || got != r1 {
		t.Errorf("cache miss for %s.%s after Store", dbName, tbl1)
	}
	if got, ok := postgresSchemaCache.Load(dbName + "." + tbl2); !ok || got != r2 {
		t.Errorf("cache miss for %s.%s after Store", dbName, tbl2)
	}

	// Different tables do not collide.
	if r1 == r2 {
		t.Fatal("r1 and r2 must be distinct pointers")
	}

	// Clean up so we don't leak across runs.
	postgresSchemaCache.Delete(dbName + "." + tbl1)
	postgresSchemaCache.Delete(dbName + "." + tbl2)
}

// TestPostgresSchemaCacheParallel exercises the cache under concurrent
// Store / Load from many goroutines. With -race, this catches any
// non-atomic access to the sync.Map itself or any unsynchronised
// mutation of shared SchemaResult.
func TestPostgresSchemaCacheParallel(t *testing.T) {
	const dbName = "test_cache_db_par"
	const tbl = "users_par"
	key := dbName + "." + tbl
	postgresSchemaCache.Delete(key)

	expected := &SchemaResult{Columns: []ColumnInfo{{Name: "id", Type: "int"}}}
	postgresSchemaCache.Store(key, expected)

	const N = 64
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if got, ok := postgresSchemaCache.Load(key); !ok || got != expected {
					t.Errorf("concurrent Load returned %v ok=%v, want %v", got, ok, expected)
					return
				}
			}
		}()
	}
	wg.Wait()
	postgresSchemaCache.Delete(key)
}

// TestPostgresSchemaCacheSameTableParallel verifies the design intent of
// F-115: parallel callers requesting the same (dbName, tableName) get
// the same SchemaResult pointer. We simulate the post-GetTableSchema
// cache write and confirm a parallel Load picks it up.
func TestPostgresSchemaCacheSameTableParallel(t *testing.T) {
	const dbName = "test_cache_db_same"
	const tbl = "users_same"
	key := dbName + "." + tbl
	postgresSchemaCache.Delete(key)

	expected := &SchemaResult{Indexes: []IndexInfo{{Name: "pk", IsPrimary: true}}}

	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)

	// Half the goroutines write, half read; they must converge.
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				postgresSchemaCache.Store(key, expected)
			} else {
				if got, ok := postgresSchemaCache.Load(key); ok && got != expected {
					t.Errorf("got unexpected pointer %p (want %p)", got, expected)
				}
			}
		}()
	}
	wg.Wait()

	// Final value must be the one we stored.
	if got, ok := postgresSchemaCache.Load(key); !ok || got != expected {
		t.Errorf("final cache state: got=%v ok=%v, want %v", got, ok, expected)
	}
	postgresSchemaCache.Delete(key)
}

// captureDriver records statements at the driver boundary. Issue #1621 was a
// bug in the generated SQL text, not in server-side behaviour, so asserting
// on the exact statement is the only regression guard that would have caught
// it: testing QualifyTable alone does not prove the CRUD callers use it.
// No external mock dependency or live PostgreSQL server is required.
type captureDriver struct {
	mu      sync.Mutex
	queries []string
	args    [][]driver.NamedValue
}

func (d *captureDriver) Open(string) (driver.Conn, error) { return &captureConn{d: d}, nil }

func (d *captureDriver) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queries = nil
	d.args = nil
}

func (d *captureDriver) record(query string, args []driver.NamedValue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queries = append(d.queries, query)
	d.args = append(d.args, append([]driver.NamedValue(nil), args...))
}

func (d *captureDriver) snapshot() ([]string, [][]driver.NamedValue) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.queries...), append([][]driver.NamedValue(nil), d.args...)
}

type captureConn struct{ d *captureDriver }

func (c *captureConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("capture driver does not implement Prepare")
}
func (c *captureConn) Close() error { return nil }
func (c *captureConn) Begin() (driver.Tx, error) {
	return nil, errors.New("capture driver does not implement Begin")
}

func (c *captureConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.d.record(query, args)
	return driver.RowsAffected(1), nil
}

var crudCapture = &captureDriver{}

func init() {
	sql.Register("uniterm-pg-crud-capture", crudCapture)
}

func newCaptureDB(t *testing.T) *sql.DB {
	t.Helper()
	crudCapture.reset()
	db, err := sql.Open("uniterm-pg-crud-capture", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestPostgresCRUDQualifiesSchemaTable locks the fix for issue #1621: the
// schema-grouped tree hands out "schema.table" names, which must be quoted as
// "schema"."table". Quoting the whole string as a single identifier produces
// DELETE FROM "public.user_log", i.e. SQLSTATE 42P01 on PostgreSQL/GaussDB.
// Bare names (the pre-1.12.4 flat tree) must keep working unchanged.
func TestPostgresCRUDQualifiesSchemaTable(t *testing.T) {
	p := &postgresProvider{}

	tables := []struct {
		name      string
		tableName string
		wantIdent string
	}{
		{"schema_qualified", "public.user_log", `"public"."user_log"`},
		{"bare_legacy", "user_log", `"user_log"`},
		{"dotted_database", "my.db.sales", `"my.db"."sales"`},
	}

	operations := []struct {
		name     string
		run      func(db *sql.DB, table string) error
		wantSQL  func(ident string) string
		wantArgs int
	}{
		{
			name: "insert",
			run: func(db *sql.DB, table string) error {
				return p.InsertRow(db, "shop", table, map[string]any{"id": int64(7)})
			},
			wantSQL: func(ident string) string {
				return `INSERT INTO ` + ident + ` ("id") VALUES ($1)`
			},
			wantArgs: 1,
		},
		{
			name: "update",
			run: func(db *sql.DB, table string) error {
				return p.UpdateRow(db, "shop", table,
					map[string]any{"name": "alice"}, map[string]any{"id": int64(7)})
			},
			wantSQL: func(ident string) string {
				return `UPDATE ` + ident + ` SET "name" = $1 WHERE "id" = $2`
			},
			wantArgs: 2,
		},
		{
			name: "delete",
			run: func(db *sql.DB, table string) error {
				return p.DeleteRow(db, "shop", table, map[string]any{"id": int64(7)})
			},
			wantSQL: func(ident string) string {
				return `DELETE FROM ` + ident + ` WHERE "id" = $1`
			},
			wantArgs: 1,
		},
		{
			name: "drop_view",
			run: func(db *sql.DB, table string) error {
				return p.DropView(db, "shop", table)
			},
			wantSQL: func(ident string) string {
				return `DROP VIEW ` + ident
			},
			wantArgs: 0,
		},
	}

	for _, tc := range tables {
		for _, op := range operations {
			t.Run(tc.name+"/"+op.name, func(t *testing.T) {
				db := newCaptureDB(t)
				if err := op.run(db, tc.tableName); err != nil {
					t.Fatalf("%s: %v", op.name, err)
				}
				queries, args := crudCapture.snapshot()
				if len(queries) != 1 {
					t.Fatalf("got %d statements, want 1: %q", len(queries), queries)
				}
				want := op.wantSQL(tc.wantIdent)
				if queries[0] != want {
					t.Errorf("SQL = %q\nwant %q", queries[0], want)
				}
				if len(args) != 1 || len(args[0]) != op.wantArgs {
					t.Errorf("bound args = %v, want %d values", args, op.wantArgs)
				}
			})
		}
	}
}
