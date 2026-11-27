package database

// HighGo Database (瀚高) is PostgreSQL-based and speaks the PostgreSQL wire
// protocol; the shared postgresProvider and its pg_catalog queries work
// as-is. Only the fallback port differs from PostgreSQL.
type highgoProvider struct {
	postgresProvider
}

func init() {
	Register("highgo", &highgoProvider{})
}

func (p *highgoProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 5866
	}
	return p.postgresProvider.DSN(host, port, user, password, dbName, extraParams)
}
