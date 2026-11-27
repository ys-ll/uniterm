package database

// CockroachDB speaks the PostgreSQL wire protocol; the shared
// postgresProvider and its pg_catalog queries work as-is. Only the fallback
// port differs from PostgreSQL.
type cockroachDBProvider struct {
	postgresProvider
}

func init() {
	Register("cockroachdb", &cockroachDBProvider{})
}

func (p *cockroachDBProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 26257
	}
	return p.postgresProvider.DSN(host, port, user, password, dbName, extraParams)
}
