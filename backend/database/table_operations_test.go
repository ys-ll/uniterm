package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testTableDB(t *testing.T) (*sqliteProvider, *sql.DB) {
	t.Helper()
	p := &sqliteProvider{}
	db, err := NewDB("sqlite", filepath.Join(t.TempDir(), "table.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, name TEXT UNIQUE, qty INTEGER)`); err != nil {
		t.Fatal(err)
	}
	return p, db
}

func TestBrowseTableFiltersBeforePagination(t *testing.T) {
	p, db := testTableDB(t)
	for _, row := range []struct {
		id   int
		name string
	}{{1, "plain"}, {2, "a_%"}, {3, "a_% second"}, {4, "another"}} {
		if _, err := db.Exec(`INSERT INTO items (id, name) VALUES (?, ?)`, row.id, row.name); err != nil {
			t.Fatal(err)
		}
	}
	opts := BrowseOptions{Filters: []TableFilter{{Column: "name", Op: "contains", Value: "a_%"}}, Sorts: []TableSort{{Column: "id", Desc: true}}, Limit: 1}
	result, err := BrowseTable(p, db, "main", "items", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || !result.HasMore || result.Rows[0]["id"] != int64(3) {
		t.Fatalf("first page: %+v", result)
	}
	opts.Offset = 1
	result, err = BrowseTable(p, db, "main", "items", opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 || result.HasMore || result.Rows[0]["id"] != int64(2) {
		t.Fatalf("second page: %+v", result)
	}
	opts.Filters[0].Column = `name" OR 1=1 --`
	if _, err := BrowseTable(p, db, "main", "items", opts); err == nil {
		t.Fatal("expected invalid column to be rejected")
	}
}

func TestBrowseSQLDialects(t *testing.T) {
	cols := map[string]bool{"id": true, "name": true}
	opts := BrowseOptions{Filters: []TableFilter{{Column: "name", Op: "contains", Value: "50%_!"}}, Sorts: []TableSort{{Column: "id", Desc: true}}, Limit: 20, Offset: 40}
	for _, test := range []struct {
		name, placeholder, pagination string
		provider                      Provider
	}{
		{"mysql", "?", "LIMIT 21 OFFSET 40", &mysqlProvider{}},
		{"postgres", "$1", "LIMIT 21 OFFSET 40", &postgresProvider{}},
		{"sqlite", "?", "LIMIT 21 OFFSET 40", &sqliteProvider{}},
		{"sqlserver", "@p1", "OFFSET 40 ROWS FETCH NEXT 21 ROWS ONLY", &sqlserverProvider{}},
		{"oracle", ":1", "ROWNUM", &oracleProvider{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, args, err := buildBrowseSQL(test.provider, "items", cols, opts)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(query, test.placeholder) || !strings.Contains(query, test.pagination) || !strings.Contains(query, "ORDER BY") {
				t.Fatalf("unexpected SQL: %s", query)
			}
			if len(args) != 1 || args[0] != "%50!%!_!!%" {
				t.Fatalf("literal filter argument: %v", args)
			}
		})
	}
}

func TestUpdateRowsAtomicRollback(t *testing.T) {
	p, db := testTableDB(t)
	_, _ = db.Exec(`INSERT INTO items VALUES (1, 'one', 1), (2, 'two', 2)`)
	updates := []RowUpdate{
		{Set: map[string]any{"qty": 10}, Where: map[string]any{"id": 1}},
		{Set: map[string]any{"name": "one"}, Where: map[string]any{"id": 2}},
	}
	if err := UpdateRowsAtomic(p, db, "main", "items", updates); err == nil {
		t.Fatal("expected unique constraint failure")
	}
	var qty int
	if err := db.QueryRow(`SELECT qty FROM items WHERE id=1`).Scan(&qty); err != nil || qty != 1 {
		t.Fatalf("first update was not rolled back: qty=%d err=%v", qty, err)
	}
	updates[1].Set["name"] = "changed"
	if err := UpdateRowsAtomic(p, db, "main", "items", updates); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT qty FROM items WHERE id=1`).Scan(&qty); err != nil || qty != 10 {
		t.Fatalf("batch was not committed: qty=%d err=%v", qty, err)
	}
}

func TestImportCSVMappingAndRollback(t *testing.T) {
	p, db := testTableDB(t)
	path := filepath.Join(t.TempDir(), "items.csv")
	write := func(content string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("\ufeffLabel,Number,Quantity\n\"first, item\",1,3\nsecond,2,4\n")
	preview, err := PreviewCSV(path)
	if err != nil || preview.Headers[0] != "Label" || preview.Rows[0][0] != "first, item" {
		t.Fatalf("preview: %+v, %v", preview, err)
	}
	mapping := []CSVColumnMapping{{SourceIndex: 0, Target: "name"}, {SourceIndex: 1, Target: "id"}, {SourceIndex: 2, Target: "qty"}}
	count, err := ImportCSV(p, db, "main", "items", path, mapping)
	if err != nil || count != 2 {
		t.Fatalf("imported %d rows: %v", count, err)
	}
	write("Label,Number,Quantity\nthird,3,5\nduplicate,2,6\n")
	if _, err := ImportCSV(p, db, "main", "items", path, mapping); err == nil || !strings.Contains(err.Error(), "CSV row 3") {
		t.Fatalf("expected row 3 failure, got %v", err)
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM items`).Scan(&total); err != nil || total != 2 {
		t.Fatalf("partial import persisted: count=%d err=%v", total, err)
	}
}
