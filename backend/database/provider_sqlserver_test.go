package database

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// TestSqlserverDSN covers DSN building for plain hosts and named instances
// (issue: host containing "\" must not end up in url.URL.Host, where
// url.URL.String() escapes it to %5C and the driver's url.Parse rejects it
// as an invalid URL escape).
func TestSqlserverDSN(t *testing.T) {
	cases := []struct {
		name         string
		host         string
		port         int
		wantHost     string // expected u.Host in the parsed DSN
		wantInstance string // expected u.Path ("/instance") or ""
		wantRawQuery string // substring that must appear in RawQuery
		noPort       bool   // DSN must not carry an explicit port
	}{
		{
			name:         "plain host with default port",
			host:         "localhost",
			port:         0,
			wantHost:     "localhost:1433",
			wantInstance: "",
			wantRawQuery: "database=mydb",
		},
		{
			name:         "plain host with explicit port",
			host:         "db.example.com",
			port:         11433,
			wantHost:     "db.example.com:11433",
			wantInstance: "",
			wantRawQuery: "database=mydb",
		},
		{
			name:         "named instance without port omits port for SQL Browser",
			host:         `localhost\SQLEXPRESS`,
			port:         0,
			wantHost:     "localhost",
			wantInstance: "/SQLEXPRESS",
			wantRawQuery: "database=mydb",
			noPort:       true,
		},
		{
			name:         "named instance with explicit port keeps port",
			host:         `192.168.1.5\SQL2019`,
			port:         21433,
			wantHost:     "192.168.1.5:21433",
			wantInstance: "/SQL2019",
			wantRawQuery: "database=mydb",
		},
	}

	p := &sqlserverProvider{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dsn := p.DSN(tc.host, tc.port, "sa", "secret", "mydb", nil)
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("driver-side url.Parse failed on DSN %q: %v", dsn, err)
			}
			if u.Scheme != "sqlserver" {
				t.Errorf("scheme = %q, want sqlserver", u.Scheme)
			}
			if u.Host != tc.wantHost {
				t.Errorf("host = %q, want %q", u.Host, tc.wantHost)
			}
			if u.Path != tc.wantInstance {
				t.Errorf("path = %q, want %q", u.Path, tc.wantInstance)
			}
			if !strings.Contains(u.RawQuery, tc.wantRawQuery) {
				t.Errorf("raw query %q does not contain %q", u.RawQuery, tc.wantRawQuery)
			}
			if tc.noPort && strings.Contains(u.Host, ":") {
				t.Errorf("DSN %q must omit port for browser resolution", dsn)
			}
			if got, ok := u.User.Password(); !ok || got != "secret" {
				t.Errorf("password not preserved")
			}
		})
	}
}

// Schemas must group tables. The tree passes composite
// "db.schema" node names as dbName and schema-qualified "schema.table"
// strings as tableName; these helpers carry that encoding.

// Regression tableListBase's views half must reference
// the sys.views alias only. The first cut reused the tables-half name
// expression (t.name) in the UNION's second SELECT, so every schema-filtered
// listing failed with "multi-part identifier t.name could not be bound".
func TestSqlserverTableListBaseAliases(t *testing.T) {
	p := &sqlserverProvider{}
	base, args, next := p.tableListBase("shop.sales")
	parts := strings.Split(base, "UNION ALL")
	if len(parts) != 2 {
		t.Fatalf("tableListBase produced %d UNION parts, want 2:\n%s", len(parts), base)
	}
	if !strings.Contains(parts[0], "sys.tables t") || strings.Contains(parts[0], "sys.views") {
		t.Errorf("tables half wrong:\n%s", parts[0])
	}
	if !strings.Contains(parts[1], "sys.views v") || strings.Contains(parts[1], "sys.tables") {
		t.Errorf("views half wrong:\n%s", parts[1])
	}
	if strings.Contains(parts[1], "t.") {
		t.Errorf("views half references the tables alias t.:\n%s", parts[1])
	}
	if !strings.Contains(base, "s.name + N'.' + t.name") || !strings.Contains(base, "s.name + N'.' + v.name") {
		t.Errorf("qualified names missing:\n%s", base)
	}
	if len(args) != 1 || args[0] != "sales" {
		t.Errorf("args = %v, want [sales]", args)
	}
	if next != 2 {
		t.Errorf("next placeholder index = %d, want 2", next)
	}

	bareBase, bareArgs, bareNext := p.tableListBase("shop")
	if bareArgs != nil && len(bareArgs) != 0 {
		t.Errorf("bare node args = %v, want none", bareArgs)
	}
	if bareNext != 1 {
		t.Errorf("bare node next = %d, want 1", bareNext)
	}
	if strings.Contains(bareBase, "@p1") || strings.Contains(bareBase, "N'.' +") {
		t.Errorf("bare node must list flat unqualified names:\n%s", bareBase)
	}
}

func TestSqlserverSplitQualifiedName(t *testing.T) {
	cases := []struct {
		in       string
		wantQual string
		wantBase string
	}{
		{"sales.employees", "sales", "employees"},
		{"employees", "", "employees"},
		{"", "", ""},
		{"shop.sales", "shop", "sales"},
		// last-dot rule: a database name containing a dot keeps it
		{"my.db.sales", "my.db", "sales"},
		{"a.b.c", "a.b", "c"},
	}
	for _, tc := range cases {
		qual, base := splitQualifiedName(tc.in)
		if qual != tc.wantQual || base != tc.wantBase {
			t.Errorf("splitQualifiedName(%q) = (%q, %q), want (%q, %q)",
				tc.in, qual, base, tc.wantQual, tc.wantBase)
		}
	}
}

func TestSqlserverQualifyTable(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"employees", "[employees]"},
		{"sales.employees", "[sales].[employees]"},
		{"my.db.sales", "[my.db].[sales]"},
	}
	p := &sqlserverProvider{}
	for _, tc := range cases {
		if got := p.QualifyTable(tc.in); got != tc.want {
			t.Errorf("QualifyTable(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSqlserverQualifyTableIsDefaultForOtherProviders(t *testing.T) {
	// Other dialects never receive qualified table names, so QualifyTable
	// must behave exactly like Quote there (zero behavior change).
	sp := &sqliteProvider{}
	if got, want := sp.QualifyTable("weird.name"), sp.Quote("weird.name"); got != want {
		t.Errorf("sqlite QualifyTable = %q, want same as Quote %q", got, want)
	}
}

func TestSqlserverQualifyWithNodeSchema(t *testing.T) {
	// New tables created inside a "db.schema" node must land in that schema.
	cases := []struct {
		dbName    string
		tableName string
		want      string
	}{
		{"shop.sales", "newtbl", "sales.newtbl"},
		{"shop.sales", "dbo.explicit", "dbo.explicit"}, // already qualified wins
		{"shop", "newtbl", "newtbl"},                   // bare db: legacy behavior
		{"", "newtbl", "newtbl"},
	}
	p := &sqlserverProvider{}
	for _, tc := range cases {
		if got := p.qualifyWithNodeSchema(tc.dbName, tc.tableName); got != tc.want {
			t.Errorf("qualifyWithNodeSchema(%q, %q) = %q, want %q",
				tc.dbName, tc.tableName, got, tc.want)
		}
	}
}

func TestSqlserverWithUseCompositeDbName(t *testing.T) {
	p := &sqlserverProvider{}
	if got, want := p.withUse("shop.sales", "SELECT 1"), "USE [shop];\nSELECT 1"; got != want {
		t.Errorf("withUse composite = %q, want %q", got, want)
	}
	if got, want := p.withUse("shop", "SELECT 1"), "USE [shop];\nSELECT 1"; got != want {
		t.Errorf("withUse bare = %q, want %q", got, want)
	}
	if got, want := p.withUse("", "SELECT 1"), "SELECT 1"; got != want {
		t.Errorf("withUse empty = %q, want %q", got, want)
	}
}

func TestSqlserverSplitDbNode(t *testing.T) {
	cases := []struct {
		in         string
		wantDb     string
		wantSchema string
	}{
		{"shop.sales", "shop", "sales"},
		{"shop", "shop", ""},
		// trailing dot = explicit "no schema" (create-database wire format),
		// keeps dot-named database names unambiguous
		{"shop.", "shop", ""},
		{"my.db.", "my.db", ""},
		{"", "", ""},
		// last-dot rule keeps dot-named databases intact
		{"my.db.sales", "my.db", "sales"},
	}
	for _, tc := range cases {
		db, schema := splitDbNode(tc.in)
		if db != tc.wantDb || schema != tc.wantSchema {
			t.Errorf("splitDbNode(%q) = (%q, %q), want (%q, %q)",
				tc.in, db, schema, tc.wantDb, tc.wantSchema)
		}
	}
}

// fakeExecer records the last statement so PrepareExec's USE routing can be
// asserted without a live server.
type fakeExecer struct {
	lastQuery string
}

func (f *fakeExecer) ExecContext(_ context.Context, q string, _ ...any) (sql.Result, error) {
	f.lastQuery = q
	return nil, nil
}

func (f *fakeExecer) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, errors.New("not implemented")
}

func TestSqlserverPrepareExecCompositeDbName(t *testing.T) {
	p := &sqlserverProvider{}
	fe := &fakeExecer{}
	if err := p.PrepareExec(fe, "shop.sales"); err != nil {
		t.Fatalf("PrepareExec failed: %v", err)
	}
	if fe.lastQuery != "USE [shop]" {
		t.Errorf("PrepareExec sent %q, want %q", fe.lastQuery, "USE [shop]")
	}
}

// TestSqlserverRowNumberPaging guards the SQL Server 2008/2005 compatibility
// fix: every SQL Server pagination path must use ROW_NUMBER wrapping instead
// of OFFSET ... FETCH NEXT (SQL Server 2012+ syntax that errors on 2008).
func TestSqlserverRowNumberPaging(t *testing.T) {
	p := &sqlserverProvider{}

	assertNoOffsetFetch := func(name, sql string) {
		t.Helper()
		if strings.Contains(sql, "OFFSET") || strings.Contains(sql, "FETCH NEXT") {
			t.Errorf("%s still uses OFFSET/FETCH: %s", name, sql)
		}
		if !strings.Contains(sql, "ROW_NUMBER() OVER") {
			t.Errorf("%s missing ROW_NUMBER() OVER wrap: %s", name, sql)
		}
	}

	// Browse wrap: user sorts are carried by the OVER clause and the paging
	// bounds cover offset+1 .. offset+limit (limit includes the extra
	// has-more row the caller fetches).
	sorts := BrowseOptions{Sorts: []TableSort{{Column: "id", Desc: true}}, Limit: 20, Offset: 40}
	query, _, err := buildBrowseSQL(p, "items", map[string]bool{"id": true}, sorts)
	if err != nil {
		t.Fatal(err)
	}
	assertNoOffsetFetch("buildBrowseSQL", query)
	if !strings.Contains(query, "OVER (ORDER BY [id] DESC)") {
		t.Errorf("browse wrap lost the user sort ordering: %s", query)
	}
	if !strings.Contains(query, "__RN__ > 40 AND __RN__ <= 61") {
		t.Errorf("browse bounds wrong (offset=40,limit=20): %s", query)
	}

	// DefaultTableQuery / PagedTableQuery emit the wrap unsorted.
	assertNoOffsetFetch("DefaultTableQuery", p.DefaultTableQuery("", "items", 20, 5))
	if !strings.Contains(p.DefaultTableQuery("", "items", 20, 5), "__RN__ > 5 AND __RN__ <= 25") {
		t.Errorf("DefaultTableQuery bounds wrong (limit=20,offset=5): %s", p.DefaultTableQuery("", "items", 20, 5))
	}
	assertNoOffsetFetch("PagedTableQuery", p.PagedTableQuery("", "items", 20, 5))
}
