//go:build !windows && !linux

package update

// detectChannel is the default for macOS and the other non-Windows,
// non-Linux platforms: self-update always replaces the binary in place.
func detectChannel() Channel {
	return ChannelPortable
}
