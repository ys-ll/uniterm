package database

// StarRocks exposes a MySQL wire protocol on its FE query port; the shared
// mysqlProvider covers every operation. Only the fallback port differs
// from MySQL.
type starRocksProvider struct {
	mysqlProvider
}

func init() {
	Register("starrocks", &starRocksProvider{})
}

func (p *starRocksProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 9030
	}
	return p.mysqlProvider.DSN(host, port, user, password, dbName, extraParams)
}
