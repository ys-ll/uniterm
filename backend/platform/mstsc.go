package platform

// MstscOptions describes the RDP target handed to the external mstsc
// launcher (see LaunchMstsc).
type MstscOptions struct {
	Host   string
	Port   int
	User   string
	Domain string
	// Admin connects to the server console session (mstsc /admin).
	Admin bool
}
