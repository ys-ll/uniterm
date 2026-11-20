package database

import (
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
