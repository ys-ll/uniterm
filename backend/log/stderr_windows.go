//go:build windows

package log

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetStdHandle         = kernel32.NewProc("GetStdHandle")
	procSetStdHandle         = kernel32.NewProc("SetStdHandle")
	procGetHandleInformation = kernel32.NewProc("GetHandleInformation")
)

const stdErrorHandle = ^uintptr(11) // STD_ERROR_HANDLE (-12) as unsigned param

// stderrRedirectCap bounds panic-stderr.log; an oversized log is truncated on
// the next start so a crash loop cannot grow the file without limit. The tail
// of a past crash may be lost to the truncation, but the newest crash always
// gets a clean trace.
const stderrRedirectCap = 16 << 20

// CaptureStderr redirects the process stderr (fd 2) to panic-stderr.log next
// to the main log when the process has no stderr handle. Release builds use
// -H windowsgui, so double-clicked launches have no stderr: the Go runtime
// writes panics and fatal errors (any goroutine, not just main) straight to
// fd 2, and without this they are silently lost — which is why crash reports
// from #1426 showed neither a dump nor a panic trace.
//
// Best-effort: any failure is a silent no-op. A console launch (dev) or an
// explicit `uniTerm.exe 2> file` keeps its original stderr and is untouched.
func CaptureStderr() {
	if stderrHandleValid() {
		return
	}

	path := stderrLogPath()
	if path == "" {
		return
	}

	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if info, err := os.Stat(path); err == nil && info.Size() > stderrRedirectCap {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return
	}
	if !setStdErrorHandle(syscall.Handle(f.Fd())) {
		f.Close()
		return
	}
	os.Stderr = f
}

func stderrLogPath() string {
	p := logPath()
	if filepath.Base(p) == "uniterm.log" && filepath.Dir(p) == "." {
		// Home dir unavailable; the main log falls back to the CWD file, so
		// do the same for the panic log.
		return "panic-stderr.log"
	}
	return filepath.Join(filepath.Dir(p), "panic-stderr.log")
}

// stderrHandleValid reports whether the process already owns a usable stderr
// handle (console, pipe or file redirection). NULL and the pseudo-handle
// sentinel both count as missing.
func stderrHandleValid() bool {
	r, _, _ := procGetStdHandle.Call(stdErrorHandle)
	if r == 0 || r == ^uintptr(0) {
		return false
	}
	// A bare validity check: GetHandleInformation fails on closed/invalid
	// handles and succeeds on anything the runtime could write to.
	var flags uint32
	ok, _, _ := procGetHandleInformation.Call(r, uintptr(unsafe.Pointer(&flags)))
	return ok != 0
}

// setStdErrorHandle publishes h as the process stderr so the Go runtime's
// panic/fatal writer (which goes straight to fd 2, not os.Stderr) lands in
// the redirected file. os.Stderr is updated by the caller.
func setStdErrorHandle(h syscall.Handle) bool {
	r, _, _ := procSetStdHandle.Call(stdErrorHandle, uintptr(h))
	return r != 0
}
