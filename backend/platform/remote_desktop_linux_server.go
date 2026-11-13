//go:build linux && server

package platform

// Headless server builds exclude the purego-based GTK keyboard capture
// (remote_desktop_linux.go): there is no display or GTK library to grab.
// Importing purego would also pull in its fakecgo runtime, whose
// cgo_import_dynamic pragmas make a CGO_ENABLED=0 binary dynamically
// linked against glibc — that broke the Alpine-based Docker image on
// cross-compiled architectures (worker exit 127, ld-linux-aarch64.so.1
// not found). Mirrors the android stub: no-op object, nil error.

type RDPKeyboardCapture struct{}

func NewRDPKeyboardCapture(emit func(RDPKeyEvent), _ ...func(func())) (*RDPKeyboardCapture, error) {
	return &RDPKeyboardCapture{}, nil
}
func (*RDPKeyboardCapture) SetCapture(id string, hwnd uintptr) bool { return false }
func (*RDPKeyboardCapture) SetShortcutCapture(id string, hwnd uintptr, fullscreenEsc bool) bool {
	return false
}
func (*RDPKeyboardCapture) Release(id string) {}
func (*RDPKeyboardCapture) Close()            {}
