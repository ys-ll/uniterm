package database

// TiDB speaks the MySQL wire protocol; the shared mysqlProvider covers every
// operation. Only the fallback port differs from MySQL.
type tidbProvider struct {
	mysqlProvider
}

func init() {
	Register("tidb", &tidbProvider{})
}

func (p *tidbProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 4000
	}
	return p.mysqlProvider.DSN(host, port, user, password, dbName, extraParams)
}
