package database

// GBase 8c is openGauss-based and speaks the PostgreSQL wire protocol; it
// embeds openGaussProvider so the vendor-internal schema/view filters apply
// unchanged. The default port matches PostgreSQL, so no DSN override is
// needed.
type gbase8cProvider struct {
	openGaussProvider
}

func init() {
	Register("gbase8c", &gbase8cProvider{})
}
