//go:build !server

package main

import "testing"

// TestDesktopServiceOptionsMethodFilterIsNil proves the desktop build keeps
// every App method bound: no filter is installed, so the desktop feature set is
// unchanged.
func TestDesktopServiceOptionsMethodFilterIsNil(t *testing.T) {
	options := appServiceOptions()
	if options.MethodFilter != nil {
		t.Fatal("desktop appServiceOptions().MethodFilter must be nil")
	}
	if options.Name != "" || options.Route != "" || options.MarshalError != nil {
		t.Fatalf("desktop appServiceOptions() must be empty, got %+v", options)
	}
}
