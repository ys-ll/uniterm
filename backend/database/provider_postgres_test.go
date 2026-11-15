package database

import (
	"testing"
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
