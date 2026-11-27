package database

import (
	"database/sql"
	"os"
	"strings"
	"testing"
)

func TestGbase8sDSN(t *testing.T) {
	p := &gbase8sProvider{}
	if p.DriverName() != "gbase8s" {
		t.Errorf("DriverName = %q, want gbase8s", p.DriverName())
	}
	if placeholder(p.DriverName(), 1) != "?" {
		t.Errorf("placeholder mismatch")
	}

	dsn := p.DSN("h", 0, "u", "p", "db", nil)
	for _, want := range []string{
		"gbase8s://u:p@h:9088/db",
		"PROTOCOL=onsoctcp",
		"delimident=1",
	} {
		if !strings.Contains(dsn, want) {
			t.Errorf("DSN missing %q: %s", want, dsn)
		}
	}

	// No GBASEDBTSERVER is injected by default (a wrong name breaks auth);
	// an explicit one must pass through.
	if strings.Contains(dsn, "GBASEDBTSERVER") {
		t.Errorf("DSN should not inject GBASEDBTSERVER: %s", dsn)
	}
	got := p.DSN("h", 0, "u", "p", "db", map[string]string{"GBASEDBTSERVER": "ol_gbase_1", "PROTOCOL": "onsocssl"})
	for _, want := range []string{"GBASEDBTSERVER=ol_gbase_1", "PROTOCOL=onsocssl"} {
		if !strings.Contains(got, want) {
			t.Errorf("DSN override missing %q: %s", want, got)
		}
	}
}

func TestGbase8sQuote(t *testing.T) {
	p := &gbase8sProvider{}
	if got, want := p.Quote(`na"me`), `"na""me"`; got != want {
		t.Errorf("Quote = %q, want %q", got, want)
	}
}

func TestGbase8sPaging(t *testing.T) {
	p := &gbase8sProvider{}
	if got, want := p.DefaultTableQuery("db", "t", 10, 0), `SELECT FIRST 10 * FROM "db":"t"`; got != want {
		t.Errorf("DefaultTableQuery = %q, want %q", got, want)
	}
	if got, want := p.PagedTableQuery("db", "t", 10, 20), `SELECT SKIP 20 FIRST 10 * FROM "db":"t"`; got != want {
		t.Errorf("PagedTableQuery = %q, want %q", got, want)
	}
	// Empty dbName falls back to the bare quoted table.
	if got, want := p.PagedTableQuery("", "t", 10, 0), `SELECT FIRST 10 * FROM "t"`; got != want {
		t.Errorf("PagedTableQuery no-db = %q, want %q", got, want)
	}
}

func TestGbase8sColTypeName(t *testing.T) {
	tests := []struct {
		code      int
		collength int
		want      string
	}{
		{2, 0, "INTEGER"},
		{0, 10, "CHAR(10)"},
		{13, 255, "VARCHAR(255)"},
		{5, 10<<8 | 2, "DECIMAL(10,2)"},
		{52, 0, "BIGINT"},
		{41, 0, "BOOLEAN"},
	}
	for _, tt := range tests {
		if got, ok := gbase8sColTypeName(tt.code, tt.collength); !ok || got != tt.want {
			t.Errorf("gbase8sColTypeName(%d, %d) = %q, %v; want %q", tt.code, tt.collength, got, ok, tt.want)
		}
	}
}

// Live smoke test against a real GBase 8s server. Skipped unless GBASE8S_DSN
// is set:
//
//	GBASE8S_DSN="gbase8s://gbasedbt:pass@127.0.0.1:19088/testdb?GBASEDBTSERVER=gbase01&PROTOCOL=onsoctcp&delimident=1" \
//	  go test ./backend/database/ -run TestGbase8sLive -count=1 -v
func TestGbase8sLive(t *testing.T) {
	dsn := os.Getenv("GBASE8S_DSN")
	if dsn == "" {
		t.Skip("GBASE8S_DSN not set; live test skipped")
	}
	p := &gbase8sProvider{}
	db, err := sql.Open(p.DriverName(), dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}

	dbs, err := p.GetDatabases(db)
	if err != nil {
		t.Fatalf("GetDatabases: %v", err)
	}
	t.Logf("databases: %v", dbs)

	tables, err := p.GetTables(db, "testdb")
	if err != nil {
		t.Fatalf("GetTables: %v", err)
	}
	t.Logf("tables: %d", len(tables))

	const tbl = "uterm_smoke"
	_, _ = db.Exec("DROP TABLE " + p.Quote(tbl))
	if _, err := db.Exec(`CREATE TABLE uterm_smoke (
		id SERIAL PRIMARY KEY,
		name VARCHAR(64) NOT NULL,
		balance DECIMAL(10,2),
		created DATE,
		flag BOOLEAN
	)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	defer db.Exec("DROP TABLE uterm_smoke")

	schema, err := p.GetTableSchema(db, "testdb", tbl)
	if err != nil {
		t.Fatalf("GetTableSchema: %v", err)
	}
	for _, c := range schema.Columns {
		t.Logf("col %-8s type=%-14s null=%v pk=%v", c.Name, c.Type, c.Nullable, c.IsPrimary)
	}

	if err := p.InsertRow(db, "testdb", tbl, map[string]any{"name": "alice"}); err != nil {
		t.Fatalf("InsertRow: %v", err)
	}

	rows, err := db.Query(p.PagedTableQuery("testdb", tbl, 10, 0))
	if err != nil {
		t.Fatalf("paged query: %v", err)
	}
	n := 0
	for rows.Next() {
		n++
	}
	rows.Close()

	dump, err := p.DumpTable(db, "testdb", tbl, DumpOptions{Structure: true, Data: true})
	if err != nil {
		t.Fatalf("DumpTable: %v", err)
	}
	if !strings.Contains(dump, "CREATE TABLE") || !strings.Contains(dump, "INSERT INTO") {
		t.Errorf("dump incomplete:\n%s", dump)
	}
	t.Logf("dump ok (%d bytes), rows=%d", len(dump), n)
}
