package database

// MariaDB speaks the MySQL wire protocol; the shared mysqlProvider covers
// every operation. Registered as a separate dbType so the connection UI can
// list it under its own name (and evolve dialect overrides later if needed).
func init() {
	Register("mariadb", &mysqlProvider{})
}
