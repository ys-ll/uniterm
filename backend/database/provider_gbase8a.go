package database

// GBase 8a exposes a MySQL wire protocol on its query port; the shared
// mysqlProvider covers every operation. Only the fallback port differs
// from MySQL.
type gbase8aProvider struct {
	mysqlProvider
}

func init() {
	Register("gbase8a", &gbase8aProvider{})
}

func (p *gbase8aProvider) DSN(host string, port int, user, password, dbName string, extraParams map[string]string) string {
	if port <= 0 {
		port = 5258
	}
	return p.mysqlProvider.DSN(host, port, user, password, dbName, extraParams)
}
