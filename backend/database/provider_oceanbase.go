package database

// OceanBase (MySQL mode) speaks the MySQL wire protocol; the shared
// mysqlProvider covers every operation. Only the fallback port differs
// from MySQL (the OBProxy listener port).
type oceanbaseProvider struct {
	mysqlProvider
}

func init() {
	Register("oceanbase", &oceanbaseProvider{})
}

func (p *oceanbaseProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 2881
	}
	return p.mysqlProvider.DSN(host, port, user, password, dbName, extraParams)
}
