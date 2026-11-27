package database

// GreatSQL is a community fork of Percona Server for MySQL and speaks the
// MySQL wire protocol as-is; the shared mysqlProvider covers every operation.
// The default port matches MySQL, so no DSN override is needed.
type greatsqlProvider struct {
	mysqlProvider
}

func init() {
	Register("greatsql", &greatsqlProvider{})
}
