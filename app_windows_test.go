//go:build windows
// +build windows

package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// detectThirdPartyShells must offer well-known third-party shell installs
// (Cygwin, MSYS2, Nushell) alongside the built-in shells, using only paths
// that actually exist on the host.
func TestDetectThirdPartyShells(t *testing.T) {
	existing := func(paths ...string) func(string) bool {
		set := make(map[string]bool)
		for _, p := range paths {
			set[strings.ToLower(p)] = true
		}
		return func(p string) bool { return set[strings.ToLower(p)] }
	}

	t.Run("cygwin fixed path", func(t *testing.T) {
		got := detectThirdPartyShells("",
			existing(`C:\cygwin64\bin\bash.exe`),
			func(string) (string, error) { return "", nil })
		want := []string{`C:\cygwin64\bin\bash.exe`}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	t.Run("cygwin registry rootdir", func(t *testing.T) {
		got := detectThirdPartyShells(`D:\cygwin`,
			existing(`D:\cygwin\bin\bash.exe`),
			func(string) (string, error) { return "", nil })
		if len(got) != 1 || got[0] != `D:\cygwin\bin\bash.exe` {
			t.Errorf("got %v, want [D:\\cygwin\\bin\\bash.exe]", got)
		}
	})

	t.Run("registry rootdir that does not exist is skipped", func(t *testing.T) {
		got := detectThirdPartyShells(`D:\gone`,
			existing(`C:\cygwin64\bin\bash.exe`),
			func(string) (string, error) { return "", nil })
		for _, p := range got {
			if strings.Contains(strings.ToLower(p), "d:\\gone") {
				t.Errorf("nonexistent registry rootdir leaked into %v", got)
			}
		}
		if len(got) != 1 || got[0] != `C:\cygwin64\bin\bash.exe` {
			t.Errorf("got %v, want [C:\\cygwin64\\bin\\bash.exe]", got)
		}
	})

	t.Run("msys2 usr bin bash", func(t *testing.T) {
		got := detectThirdPartyShells("",
			existing(`C:\msys64\usr\bin\bash.exe`),
			func(string) (string, error) { return "", nil })
		if len(got) != 1 || got[0] != `C:\msys64\usr\bin\bash.exe` {
			t.Errorf("got %v, want [C:\\msys64\\usr\\bin\\bash.exe]", got)
		}
	})

	t.Run("nushell via PATH", func(t *testing.T) {
		got := detectThirdPartyShells("", func(string) bool { return false },
			func(name string) (string, error) {
				if name == "nu.exe" {
					return `C:\Users\me\scoop\shims\nu.exe`, nil
				}
				return "", nil
			})
		if len(got) != 1 || got[0] != `C:\Users\me\scoop\shims\nu.exe` {
			t.Errorf("got %v, want [nu.exe resolved path]", got)
		}
	})

	t.Run("nothing installed yields empty", func(t *testing.T) {
		got := detectThirdPartyShells("", func(string) bool { return false },
			func(string) (string, error) { return "", nil })
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})

	t.Run("duplicates are removed", func(t *testing.T) {
		// Registry rootdir pointing at the same cygwin64 install as the fixed
		// path candidate must not yield the shell twice.
		got := detectThirdPartyShells(`C:\cygwin64`,
			existing(`C:\cygwin64\bin\bash.exe`),
			func(string) (string, error) { return "", nil })
		if len(got) != 1 {
			t.Errorf("got %v, want single entry", got)
		}
	})
}

func TestClipboardReadFileChunkRanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte{0, 1, 2, 3, 255}, 0600); err != nil {
		t.Fatal(err)
	}
	encoded, err := readClipboardFileChunk(path, 2, 10)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := base64.StdEncoding.DecodeString(encoded)
	if string(data) != string([]byte{2, 3, 255}) {
		t.Fatalf("unexpected bytes: %v", data)
	}
	for _, bounds := range [][2]int64{{-1, 1}, {0, -1}, {0, 8*1024*1024 + 1}} {
		if _, err := readClipboardFileChunk(path, bounds[0], int(bounds[1])); err == nil {
			t.Fatalf("accepted invalid range %v", bounds)
		}
	}
}

func TestClipboardGetFilesLive(t *testing.T) {
	if os.Getenv("UNITERM_CLIPBOARD_LIVE_TEST") != "1" {
		t.Skip("UNITERM_CLIPBOARD_LIVE_TEST not enabled")
	}
	a := &App{}
	files := a.ClipboardGetFiles()
	if len(files) == 0 {
		t.Skip("no files on the clipboard; copy a file first to run this test")
	}
	t.Logf("got %d files", len(files))
	for _, f := range files {
		t.Logf("file: %+v", f)
		if f.Size > 0 {
			b64, err := a.ClipboardReadFileChunk(0, 0, 16)
			t.Logf("chunk: %d bytes err=%v", len(b64), err)
		}
	}
}

// Regression test for issue #1426. findMainWindow used to register a fresh
// syscall callback on every call. Go's Windows runtime stores those in a
// fixed-size table (cb_max = 2000, entries never freed), so once the
// remote-desktop keyboard capture path fell back to an un-cached
// findMainWindow, every desktop-focus toggle leaked one entry until the
// process aborted with "fatal error: too many callback functions". 2100
// findMainWindow calls must stay well inside that budget: with the fix the
// callback is registered once and reused.
//
// Opt-in because the pre-fix failure mode is a fatal runtime throw that
// kills the whole test binary — exactly the crash users saw. Run with
// UNITERM_CALLBACK_STRESS=1 to reproduce the leak on old code.
func TestFindMainWindowReusesCallback(t *testing.T) {
	if os.Getenv("UNITERM_CALLBACK_STRESS") != "1" {
		t.Skip("fatal-throw stress test; set UNITERM_CALLBACK_STRESS=1 to run")
	}
	a := &App{}
	for i := 0; i < 2100; i++ {
		a.findMainWindow()
	}
}

// ensureMainWindowHwnd must hand back an already-resolved HWND without
// enumerating again: callers (remote-desktop keyboard capture, clipboard,
// open-with dialog) rely on repeated calls being stable and cheap.
func TestEnsureMainWindowHwndReturnsCachedValue(t *testing.T) {
	a := &App{mainHwnd: 4242}
	if got := a.ensureMainWindowHwnd(); got != 4242 {
		t.Fatalf("ensureMainWindowHwnd = %d, want cached 4242", got)
	}
	if got := a.ensureMainWindowHwnd(); got != 4242 {
		t.Fatalf("second ensureMainWindowHwnd = %d, want 4242", got)
	}
}

// The lazy resolution path must find a real "uniTerm" window of this
// process, cache it into mainHwnd, and install the WndProc subclass — the
// subclass drives the native window:resize-end events the terminal re-fit
// fix (#656) listens for. Before the fix this resolution ran eagerly at
// ServiceStartup, before the wails window existed, so it always failed and
// the subclass was silently never installed.
func TestEnsureMainWindowHwndResolvesAndSubclasses(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hwnd, err := createTestMainWindow()
	if err != nil {
		t.Fatalf("create test window: %v", err)
	}
	defer destroyTestMainWindow(hwnd)

	a := &App{}
	got := a.ensureMainWindowHwnd()
	if got == 0 {
		t.Fatal("ensureMainWindowHwnd = 0, want the test window HWND")
	}
	if a.mainHwnd != got {
		t.Fatalf("resolved HWND %d not cached in mainHwnd (%d)", got, a.mainHwnd)
	}
	if a.originalWndProc == 0 || a.wndProcCb == 0 {
		t.Fatal("WndProc subclass not installed after successful resolution")
	}
	if again := a.ensureMainWindowHwnd(); again != got {
		t.Fatalf("second ensureMainWindowHwnd = %d, want stable %d", again, got)
	}
}

// createTestMainWindow registers a window class and creates a real top-level
// window titled "uniTerm" so findMainWindow's EnumWindows sweep can discover
// it. Hidden windows are enumerated too, so it never needs to be shown.
func createTestMainWindow() (uintptr, error) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	procDefWindowProcW := user32.NewProc("DefWindowProcW")
	wndProc := windows.NewCallback(func(hwnd windows.HWND, msg uint32, wparam, lparam uintptr) uintptr {
		r1, _, _ := procDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), wparam, lparam)
		return r1
	})

	className, err := windows.UTF16PtrFromString("uniterm-test-window")
	if err != nil {
		return 0, err
	}
	var wc wndClassExW
	wc.cbSize = uint32(unsafe.Sizeof(wc))
	wc.lpfnWndProc = wndProc
	wc.lpszClassName = className
	procRegisterClassExW := user32.NewProc("RegisterClassExW")
	if atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); atom == 0 {
		return 0, fmt.Errorf("RegisterClassExW: %w", callErr)
	}

	title, err := windows.UTF16PtrFromString("uniTerm")
	if err != nil {
		return 0, err
	}
	hwnd, _, callErr := user32.NewProc("CreateWindowExW").Call(
		0,                                  // exStyle
		uintptr(unsafe.Pointer(className)), // class
		uintptr(unsafe.Pointer(title)),     // title
		wsOverlappedWindow,                 // style
		0, 0, 0, 0,                         // x, y, w, h (never shown)
		0, 0, 0, 0) // parent, menu, instance, param
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", callErr)
	}
	return hwnd, nil
}

func destroyTestMainWindow(hwnd uintptr) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	user32.NewProc("DestroyWindow").Call(hwnd)
	className, _ := windows.UTF16PtrFromString("uniterm-test-window")
	user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), 0)
}

// wndClassExW mirrors the user32 WNDCLASSEXW layout.
type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

const wsOverlappedWindow = 0x00CF0000
