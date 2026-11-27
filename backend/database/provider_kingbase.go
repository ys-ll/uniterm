package database

// KingbaseES speaks the PostgreSQL wire protocol (PG-compatible mode); the
// shared postgresProvider and its pg_catalog/information_schema queries work
// as-is. Only the fallback port differs from PostgreSQL.
type kingbaseProvider struct {
	postgresProvider
}

func init() {
	Register("kingbase", &kingbaseProvider{})
}

func (p *kingbaseProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 54321
	}
	return p.postgresProvider.DSN(host, port, user, password, dbName, extraParams)
}
