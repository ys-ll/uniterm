package database

import (
	"database/sql"
	"fmt"
)

// openGauss (and GBase 8c, which shares the lineage) speaks the PostgreSQL
// wire protocol; the shared postgresProvider and its pg_catalog queries work
// as-is. The default port matches PostgreSQL, so no DSN override is needed.
// Authentication: openGauss ≥3.1 accepts SCRAM-SHA-256 (pgx-native); clusters
// storing passwords as md5 (password_encryption_type=1) also work since pgx
// speaks md5 too. Only the GaussDB-proprietary sha256 scheme is unreachable.
type openGaussProvider struct {
	postgresProvider
}

func init() {
	Register("opengauss", &openGaussProvider{})
}

// openGaussSchemas lists the vendor-internal namespaces openGauss seeds into
// every database, on top of the generic pg_* filter the PostgreSQL provider
// already applies.
var openGaussSchemas = []string{
	"blockchain", "cstore", "db4ai", "snapshot", "sqladvisor",
	"coverage", "pkg_service", "ca_catalog", "cshobj", "odbschema",
}

// GetSchemas narrows the PostgreSQL listing by openGauss's internal
// namespaces (dbe_perf, db4ai, cstore, …) so the tree shows user schemas.
func (p *openGaussProvider) GetSchemas(db *sql.DB, dbName string) ([]string, error) {
	results, err := queryStrings(db, `
		SELECT nspname AS schema_name
		FROM pg_namespace
		WHERE nspname NOT IN ('pg_catalog', 'information_schema', 'pg_toast', `+quoteStringList(openGaussSchemas)+`)
		  AND nspname NOT LIKE 'pg\_%'
		  AND nspname NOT LIKE 'dbe\_%'
		ORDER BY (nspname = 'public') DESC, nspname`)
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	names := make([]string, 0, len(results))
	for _, row := range results {
		names = append(names, row["schema_name"])
	}
	return names, nil
}

// tableListBase extends the PostgreSQL table listing by hiding openGauss's
// vendor-internal namespaces in the flat (bare-dbName) listing: the init
// phase seeds ~180 monitoring views into dbe_perf and friends, which would
// drown out the user's tables.
func (p *openGaussProvider) tableListBase(dbName string) (string, []any, int) {
	base, args, next := p.postgresProvider.tableListBase(dbName)
	base += `
		 AND n.nspname NOT IN (` + quoteStringList(openGaussSchemas) + `)
		 AND n.nspname NOT LIKE 'dbe\_%'`
	return base, args, next
}

// GetTables/GetTablePage are re-declared (not inherited) because Go method
// promotion is not virtual: the embedded PostgreSQL GetTables would resolve
// tableListBase against its own receiver and skip the openGauss filter.
func (p *openGaussProvider) GetTables(db *sql.DB, dbName string) ([]TableInfo, error) {
	base, args, _ := p.tableListBase(dbName)
	return pgListTables(db, base, args)
}

func (p *openGaussProvider) GetTablePage(db *sql.DB, dbName string, query string, offset, limit int) ([]TableInfo, int, error) {
	base, baseArgs, next := p.tableListBase(dbName)
	return pgListTablesPage(db, base, baseArgs, next, query, offset, limit)
}

// quoteStringList renders Go strings as a SQL literal list ('a', 'b', …).
func quoteStringList(items []string) string {
	out := ""
	for i, s := range items {
		if i > 0 {
			out += ", "
		}
		out += "'" + s + "'"
	}
	return out
}
