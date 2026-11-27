package database

// Apache Cloudberry is the Apache incubating continuation of Greenplum and
// speaks the PostgreSQL wire protocol; the shared postgresProvider and its
// pg_catalog queries work as-is. The default port matches PostgreSQL, so no
// DSN override is needed.
type cloudberryProvider struct {
	greenplumProvider
}

func init() {
	Register("cloudberry", &cloudberryProvider{})
}
