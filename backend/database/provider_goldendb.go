package database

// GoldenDB (ZTE's distributed financial database) speaks the MySQL wire
// protocol on its proxy nodes; the shared mysqlProvider covers every
// operation. The default port matches MySQL, so no DSN override is needed.
type goldenDBProvider struct {
	mysqlProvider
}

func init() {
	Register("goldendb", &goldenDBProvider{})
}
