package database

// Greenplum speaks the PostgreSQL wire protocol; the shared postgresProvider
// and its pg_catalog queries work as-is. The default port matches
// PostgreSQL, so no DSN override is needed.
type greenplumProvider struct {
	postgresProvider
}

func init() {
	Register("greenplum", &greenplumProvider{})
}
