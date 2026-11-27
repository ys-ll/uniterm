package database

// YugabyteDB (YSQL) speaks the PostgreSQL wire protocol; the shared
// postgresProvider and its pg_catalog queries work as-is. Only the fallback
// port differs from PostgreSQL.
type yugabyteDBProvider struct {
	postgresProvider
}

func init() {
	Register("yugabytedb", &yugabyteDBProvider{})
}

func (p *yugabyteDBProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 5433
	}
	return p.postgresProvider.DSN(host, port, user, password, dbName, extraParams)
}
