package database

// Vastbase G100 is built on openGauss and speaks the PostgreSQL wire
// protocol; embedding openGaussProvider inherits its vendor-namespace
// filtering (dbe_perf and friends), which Vastbase seeds as well. Only the
// default port matches PostgreSQL.
type vastbaseProvider struct {
	openGaussProvider
}

func init() {
	Register("vastbase", &vastbaseProvider{})
}
