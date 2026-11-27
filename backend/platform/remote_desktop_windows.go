//go:build windows

package platform

import (
	"fmt"
	"runtime"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var rdpKeyboardUser32 = windows.NewLazySystemDLL("user32.dll")
var rdpSetKeyboardHook = rdpKeyboardUser32.NewProc("SetWindowsHookExW")
var rdpUnhookKeyboard = rdpKeyboardUser32.NewProc("UnhookWindowsHookEx")
var rdpNextKeyboardHook = rdpKeyboardUser32.NewProc("CallNextHookEx")
var rdpGetMessage = rdpKeyboardUser32.NewProc("GetMessageW")
var rdpPeekMessage = rdpKeyboardUser32.NewProc("PeekMessageW")
var rdpPostThreadMessage = rdpKeyboardUser32.NewProc("PostThreadMessageW")
var rdpGetForegroundWindow = rdpKeyboardUser32.NewProc("GetForegroundWindow")

type rdpKeyboardData struct {
	VKCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}

type rdpKeyboardMessage struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	X, Y    int32
	Private uint32
}

// The hook runs on a dedicated Windows message thread. Its callback only
// updates state and enqueues events; Wails dispatch never blocks the hook.
type RDPKeyboardCapture struct {
	mu            sync.Mutex
	state         rdpKeyboardState
	hwnd          uintptr
	threadID      uint32
	callback      uintptr
	closed        bool
	shortcutsOnly bool
	// escCapture additionally swallows Escape while active: in fullscreen the
	// browser consumes Escape to leave fullscreen before the page sees it, so
	// VNC/SPICE need it captured like a shortcut and forwarded to the remote.
	escCapture bool
	expires    time.Time
	queue      []RDPKeyEvent
	wake       chan struct{}
	done       chan struct{}
	emit       func(RDPKeyEvent)
}

func NewRDPKeyboardCapture(emit func(RDPKeyEvent), _ ...func(func())) (*RDPKeyboardCapture, error) {
	c := &RDPKeyboardCapture{wake: make(chan struct{}, 1), done: make(chan struct{}), emit: emit}
	ready := make(chan error, 1)
	go c.run(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	go c.dispatch()
	return c, nil
}

func (c *RDPKeyboardCapture) enqueue(event RDPKeyEvent) {
	c.queue = append(c.queue, event)
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *RDPKeyboardCapture) reset() {
	if c.state.captureID != "" {
		c.enqueue(RDPKeyEvent{CaptureID: c.state.captureID, Reset: true})
	}
	c.state.activate("")
}

func (c *RDPKeyboardCapture) SetCapture(id string, hwnd uintptr) bool {
	return c.setCapture(id, hwnd, false, false)
}

func (c *RDPKeyboardCapture) SetShortcutCapture(id string, hwnd uintptr, fullscreenEsc bool) bool {
	return c.setCapture(id, hwnd, true, fullscreenEsc)
}

func (c *RDPKeyboardCapture) setCapture(id string, hwnd uintptr, shortcutsOnly, escCapture bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || hwnd == 0 {
		return false
	}
	// A crashed/reloaded WebView must not keep swallowing local input.
	c.expires = time.Now().Add(3 * time.Second)
	if c.state.captureID == id && c.hwnd == hwnd {
		// Flag-only refreshes (e.g. fullscreen toggling) must not reset the
		// capture: a reset emits a spurious remote key-reset event.
		c.shortcutsOnly, c.escCapture = shortcutsOnly, escCapture
		return true
	}
	c.reset()
	c.hwnd = hwnd
	c.shortcutsOnly = shortcutsOnly
	c.escCapture = escCapture
	c.state.activate(id)
	return true
}

func (c *RDPKeyboardCapture) Release(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.captureID == id {
		c.reset()
	}
}

func (c *RDPKeyboardCapture) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.reset()
	threadID := c.threadID
	c.mu.Unlock()
	rdpPostThreadMessage.Call(uintptr(threadID), 0x12, 0, 0) // WM_QUIT
	<-c.done
}

func (c *RDPKeyboardCapture) dispatch() {
	for {
		select {
		case <-c.done:
			return
		case <-c.wake:
		}
		c.mu.Lock()
		batch := c.queue
		c.queue = nil
		c.mu.Unlock()
		for _, event := range batch {
			c.emit(event)
		}
	}
}

// normalizeScan maps non-standard scancodes to their XT set-1 equivalents.
// Some USB keyboards report Shift with an E0 prefix; set-1 defines no
// extended Shift and remote servers reject 0xE02A/0xE036, which killed
// Shift (and every Shift chord) for users of such keyboards.
func normalizeScan(scan uint16) uint16 {
	if scan == 0xe02a || scan == 0xe036 {
		return scan & 0xff
	}
	return scan
}

// hookStructPtr reinterprets a hook-provided uintptr as a pointer. lParam in a
// low-level keyboard hook already IS a KBDLLHOOKSTRUCT pointer handed over by
// Windows, so the conversion is sound at runtime; going through the variable's
// address instead of a direct uintptr→unsafe.Pointer conversion keeps
// `go vet`'s unsafeptr check from flagging provenance it cannot see.
func hookStructPtr(p uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&p))
}

func (c *RDPKeyboardCapture) run(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(c.done)
	c.threadID = windows.GetCurrentThreadId()
	var message rdpKeyboardMessage
	// Create the thread message queue before Close can post WM_QUIT.
	rdpPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 0)
	c.callback = windows.NewCallback(func(code int32, wParam, lParam uintptr) uintptr {
		if code >= 0 && (wParam == 0x100 || wParam == 0x101 || wParam == 0x104 || wParam == 0x105) {
			data := (*rdpKeyboardData)(hookStructPtr(lParam))
			scan := uint16(data.ScanCode & 0xff)
			if scan != 0 && data.Flags&1 != 0 {
				scan |= 0xe000
			}
			scan = normalizeScan(scan)
			down := data.Flags&0x80 == 0
			foreground, _, _ := rdpGetForegroundWindow.Call()
			c.mu.Lock()
			active := !c.closed && foreground == c.hwnd && time.Now().Before(c.expires)
			if !active {
				c.reset()
			}
			local := data.VKCode == 0x7b
			if c.shortcutsOnly && !(c.escCapture && scan == keyScanEscape) && !c.state.shortcutKey(scan, data.Flags&0x20 != 0) {
				local = true
			}
			event, swallow := c.state.key(scan, down, active, local) // F12
			if event.CaptureID != "" {
				event.VirtualKey = data.VKCode
				c.enqueue(event)
			}
			c.mu.Unlock()
			if swallow {
				return 1
			}
		}
		result, _, _ := rdpNextKeyboardHook.Call(0, uintptr(code), wParam, lParam)
		return result
	})
	hook, _, err := rdpSetKeyboardHook.Call(13, c.callback, 0, 0) // WH_KEYBOARD_LL
	if hook == 0 {
		ready <- fmt.Errorf("install RDP keyboard hook: %w", err)
		return
	}
	defer rdpUnhookKeyboard.Call(hook)
	ready <- nil
	for {
		result, _, _ := rdpGetMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		if int32(result) <= 0 {
			return
		}
	}
}
