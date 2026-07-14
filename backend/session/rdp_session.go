//go:build windows

package session

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/ys-ll/uniterm/backend/log"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	atlDll              = windows.NewLazySystemDLL("atl.dll")
	procAtlAxWinInit    = atlDll.NewProc("AtlAxWinInit")
	procAtlAxGetControl = atlDll.NewProc("AtlAxGetControl")

	user32Dll            = windows.NewLazySystemDLL("user32.dll")
	procSetWindowPos     = user32Dll.NewProc("SetWindowPos")
	procShowWindow       = user32Dll.NewProc("ShowWindow")
	procDestroyWindow    = user32Dll.NewProc("DestroyWindow")
	procFindWindowW      = user32Dll.NewProc("FindWindowW")
	procPeekMessage      = user32Dll.NewProc("PeekMessageW")
	procTranslateMessage = user32Dll.NewProc("TranslateMessage")
	procDispatchMessage  = user32Dll.NewProc("DispatchMessageW")
	procGetWindowRect    = user32Dll.NewProc("GetWindowRect")
	procGetClientRect    = user32Dll.NewProc("GetClientRect")
	procClientToScreen   = user32Dll.NewProc("ClientToScreen")
	procSetWindowLongPtr = user32Dll.NewProc("SetWindowLongPtrW")
	procPostMessageW     = user32Dll.NewProc("PostMessageW")
	procFindWindowExW    = user32Dll.NewProc("FindWindowExW")
	procSendMessageW     = user32Dll.NewProc("SendMessageW")
)

const (
	WM_CLOSE           = 0x0010
	SWP_SHOWWINDOW     = 0x0040
	SWP_HIDEWINDOW     = 0x0080
	SWP_NOMOVE         = 0x0002
	SWP_NOSIZE         = 0x0001
	SWP_NOACTIVATE     = 0x0010
	SWP_NOZORDER       = 0x0004
	SWP_ASYNCWINDOWPOS = 0x4000 // non-blocking: avoids freezing RDP COM thread
	WS_EX_TOOLWINDOW   = 0x00000080
	WS_EX_NOACTIVATE   = 0x08000000
	WS_POPUP           = 0x80000000
	WS_CLIPSIBLINGS    = 0x04000000
	PM_REMOVE          = 0x0001
	GWLP_HWNDPARENT    = ^uintptr(7) // -8 represented as uintptr for syscall compatibility
	SW_HIDE            = 0
	SW_SHOWNOACTIVATE  = 4
	WM_COMMAND         = 0x0111
	BM_CLICK           = 0x00F5
	IDYES              = 6
	IDOK               = 1
)

type RDPSession struct {
	baseSession
	parentHwnd uintptr
	hwnd       uintptr
	rdp        *ole.IDispatch
	config     ConnectionConfig
	mu         sync.Mutex
	shown      bool

	// Last known position, used by Show() after Hide()
	trackX, trackY int
}

func NewRDPSession(id string) *RDPSession {
	return &RDPSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "rdp",
			status:      StatusDisconnected,
		},
	}
}

// ClientAreaScreenRect returns the main window's client area in screen coordinates
// (physical pixels). Used by the frontend to position the RDP overlay precisely.
func (s *RDPSession) ClientAreaScreenRect() (x, y, w, h int) {
	if s.parentHwnd == 0 {
		return
	}
	var cr rect
	ret, _, _ := procGetClientRect.Call(s.parentHwnd, uintptr(unsafe.Pointer(&cr)))
	if ret == 0 {
		return
	}
	var origin point
	ret, _, _ = procClientToScreen.Call(s.parentHwnd, uintptr(unsafe.Pointer(&origin)))
	if ret == 0 {
		return
	}
	return int(origin.X), int(origin.Y), int(cr.Right), int(cr.Bottom)
}

func (s *RDPSession) SetParentHwnd(hwnd uintptr) {
	s.parentHwnd = hwnd
}

// autoDismissSecurityDialogs polls for RDP security warning dialogs (e.g.
// cert prompts or "do you want to connect" dialogs) and dismisses them.
func (s *RDPSession) autoDismissSecurityDialogs(stop <-chan struct{}) {
	dialogTitles := []string{
		"远程桌面连接",
		"远程桌面连接安全警告",
		"Remote Desktop Connection",
		"Remote Desktop Connection Security Warning",
		"Windows 安全",
		"Windows Security",
		"安全警告",
		"Security Warning",
	}
	clsName, _ := windows.UTF16PtrFromString("#32770")

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			for _, title := range dialogTitles {
				tPtr, _ := windows.UTF16PtrFromString(title)
				hwnd, _, _ := procFindWindowW.Call(
					uintptr(unsafe.Pointer(clsName)),
					uintptr(unsafe.Pointer(tPtr)),
				)
				if hwnd == 0 {
					continue
				}

				log.Writef("[RDP] found security dialog: %s hwnd=0x%x", title, hwnd)

				// Dismiss via standard dialog button IDs
				procPostMessageW.Call(hwnd, WM_COMMAND, IDYES, 0)
				procPostMessageW.Call(hwnd, WM_COMMAND, IDOK, 0)
				procPostMessageW.Call(hwnd, WM_COMMAND, IDYES+1, 0) // IDNO sometimes maps to 7

				// Also try clicking buttons directly (Windows 11 may use different labels)
				for _, btnText := range []string{
					"是(&Y)", "是", "Yes", "&Yes",
					"连接(&C)", "连接", "Connect", "&Connect",
					"确认", "确认(&Y)", "确认(&O)",
					"确定", "确定(&O)", "OK", "&OK",
					"继续", "继续(&C)", "Continue", "&Continue",
				} {
					btnPtr, _ := windows.UTF16PtrFromString(btnText)
					btnHwnd, _, _ := procFindWindowExW.Call(hwnd, 0, 0, uintptr(unsafe.Pointer(btnPtr)))
					if btnHwnd != 0 {
						procSendMessageW.Call(btnHwnd, BM_CLICK, 0, 0)
						log.Writef("[RDP] clicked button '%s' on dialog hwnd=0x%x", btnText, hwnd)
						break
					}
				}
			}
		}
	}
}

func (s *RDPSession) Connect(config ConnectionConfig) error {
	log.Writef("[RDP] starting connect to %s:%d as %s", config.Host, config.Port, config.User)

	defer func() {
		if r := recover(); r != nil {
			log.Writef("[RDP] PANIC in Connect: %v", r)
			s.setStatus(StatusError)
		}
	}()

	// Phase 1: quick state init (brief lock)
	s.mu.Lock()
	s.config = config
	s.title = fmt.Sprintf("%s@%s (RDP)", config.User, config.Host)
	s.setStatus(StatusConnecting)
	s.mu.Unlock()

	runtime.LockOSThread() // pin COM STA to a dedicated OS thread
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	defer func() {
		// Properly disconnect the RDP ActiveX control first.
		// This closes network sockets and stops internal threads —
		// skipping it leaks resources that cause progressive lag.
		s.mu.Lock()
		rdp := s.rdp
		s.rdp = nil
		s.mu.Unlock()
		if rdp != nil {
			rdp.CallMethod("Disconnect")
			rdp.Release()
		}

		s.mu.Lock()
		hwnd := s.hwnd
		s.hwnd = 0
		s.mu.Unlock()
		if hwnd != 0 {
			// Hide first to avoid visual flash during destruction
			procSetWindowPos.Call(hwnd, 0, 32000, 32000, 0, 0,
				SWP_NOSIZE|SWP_NOACTIVATE|SWP_NOZORDER|SWP_ASYNCWINDOWPOS)
			procDestroyWindow.Call(hwnd)
		}

		ole.CoUninitialize()
		runtime.UnlockOSThread()
	}()

	if s.parentHwnd == 0 {
		title, _ := windows.UTF16PtrFromString("uniTerm")
		hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(title)))
		if hwnd == 0 {
			log.Writef("[RDP] ERROR: cannot find main window")
			s.setStatus(StatusError)
			return fmt.Errorf("cannot find main window")
		}
		s.parentHwnd = hwnd
	}

	ret, _, _ := procAtlAxWinInit.Call()
	if ret == 0 {
		log.Writef("[RDP] ERROR: AtlAxWinInit failed")
		s.setStatus(StatusError)
		return fmt.Errorf("AtlAxWinInit failed")
	}

	progID := s.findRdpProgID()
	if progID == "" {
		log.Writef("[RDP] ERROR: no RDP ActiveX control found")
		s.setStatus(StatusError)
		return fmt.Errorf("no RDP ActiveX control found")
	}

	width := config.RdpFixedWidth
	height := config.RdpFixedHeight
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 600
	}

	// Create WS_POPUP off-screen
	name, _ := windows.UTF16PtrFromString(progID)
	className, _ := windows.UTF16PtrFromString("AtlAxWin")

	createWindowEx := windows.NewLazySystemDLL("user32.dll").NewProc("CreateWindowExW")
	hwnd, _, _ := createWindowEx.Call(
		uintptr(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE),
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(name)),
		uintptr(WS_POPUP|WS_CLIPSIBLINGS),
		32000, 32000,
		uintptr(width), uintptr(height),
		// Create without owner in CreateWindowEx to avoid COM initialization issues.
		// Owner is set immediately after via SetWindowLongPtr(GWLP_HWNDPARENT).
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		log.Writef("[RDP] ERROR: CreateWindowExW failed")
		s.setStatus(StatusError)
		return fmt.Errorf("CreateWindowEx failed")
	}

	// Make RDP window owned by uniTerm main window.
	// Owned windows naturally stay above their owner but below other top-level windows,
	// eliminating the need for manual HWND_TOPMOST/HWND_NOTOPMOST z-order management.
	procSetWindowLongPtr.Call(hwnd, GWLP_HWNDPARENT, s.parentHwnd)
	log.Writef("[RDP] hwnd=0x%x parentHwnd=0x%x (owned)", hwnd, s.parentHwnd)

	var unk *ole.IUnknown
	procAtlAxGetControl.Call(hwnd, uintptr(unsafe.Pointer(&unk)))
	if unk == nil {
		procDestroyWindow.Call(hwnd)
		s.setStatus(StatusError)
		return fmt.Errorf("AtlAxGetControl failed")
	}

	dispatch, err := unk.QueryInterface(ole.IID_IDispatch)
	unk.Release()
	if err != nil {
		procDestroyWindow.Call(hwnd)
		s.setStatus(StatusError)
		return fmt.Errorf("QI IDispatch: %w", err)
	}

	s.mu.Lock()
	s.hwnd = hwnd
	s.rdp = dispatch
	s.mu.Unlock()

	port := config.Port
	if port <= 0 {
		port = 3389
	}

	// Set the identity before the password so CredSSP receives the complete
	// account context. DOMAIN\\user and .\\user are accepted in the form field.
	user, domain := splitRDPUser(config.User)
	if _, err := dispatch.PutProperty("Server", config.Host); err != nil {
		return fmt.Errorf("set RDP server: %w", err)
	}
	if _, err := dispatch.PutProperty("UserName", user); err != nil {
		return fmt.Errorf("set RDP username: %w", err)
	}
	if _, err := dispatch.PutProperty("Domain", domain); err != nil {
		return fmt.Errorf("set RDP domain: %w", err)
	}
	dispatch.PutProperty("DesktopWidth", width)
	dispatch.PutProperty("DesktopHeight", height)
	dispatch.PutProperty("FullScreen", false)

	// AdvancedSettings2 is the scriptable interface documented for
	// ClearTextPassword. It must be set before Connect.
	advObj, err := dispatch.GetProperty("AdvancedSettings2")
	if err != nil || advObj == nil {
		if err == nil {
			err = fmt.Errorf("property is unavailable")
		}
		return fmt.Errorf("get RDP AdvancedSettings2: %w", err)
	}
	adv := advObj.ToIDispatch()
	if adv == nil {
		return fmt.Errorf("RDP AdvancedSettings2 is not an IDispatch")
	}

	if _, err := adv.PutProperty("RDPPort", port); err != nil {
		adv.Release()
		return fmt.Errorf("set RDP port: %w", err)
	}
	adv.PutProperty("RedirectClipboard", true)
	adv.PutProperty("RedirectDrives", true)
	adv.PutProperty("DisplayConnectionBar", false)
	adv.PutProperty("EnableAutoReconnect", true)
	adv.PutProperty("WarnOnDirectConnect", false)
	adv.PutProperty("ContainerHandledFullScreen", true)
	if config.RdpSmartSizing {
		adv.PutProperty("SmartSizing", true)
	}
	if config.Password != "" {
		if _, err := adv.PutProperty("ClearTextPassword", config.Password); err != nil {
			adv.Release()
			return fmt.Errorf("set RDP password: %w", err)
		}
		log.Writef("[RDP] AdvancedSettings2 ClearTextPassword accepted, nla=%v", config.RdpEnableNLA)
	} else {
		log.Writef("[RDP] no password supplied, ActiveX may show its credential UI")
	}
	adv.Release()

	// NonScriptable is a vtable-only COM interface. It owns the credential
	// prompt switches and is also the documented password injection path.
	if err := s.configureNonScriptable(config.Password, config.RdpEnableNLA); err != nil {
		return err
	}

	// AuthenticationLevel belongs to newer AdvancedSettings interfaces.
	if authObj, authErr := dispatch.GetProperty("AdvancedSettings4"); authErr == nil && authObj != nil {
		if auth := authObj.ToIDispatch(); auth != nil {
			if config.RdpEnableNLA {
				if _, err := auth.PutProperty("AuthenticationLevel", 2); err != nil {
					log.Writef("[RDP] AdvancedSettings4 AuthenticationLevel failed: %v", err)
				}
			} else {
				auth.PutProperty("AuthenticationLevel", 0)
			}
			auth.Release()
		}
	}
	if credObj, credErr := dispatch.GetProperty("AdvancedSettings6"); credErr == nil && credObj != nil {
		if cred := credObj.ToIDispatch(); cred != nil {
			if _, err := cred.PutProperty("EnableCredSspSupport", config.RdpEnableNLA); err != nil {
				log.Writef("[RDP] AdvancedSettings6 EnableCredSspSupport failed: %v", err)
			}
			cred.Release()
		}
	}

	// Suppress security prompts on all available AdvancedSettings versions
	for _, ver := range []int{9, 8, 7, 6, 5, 4, 3} {
		propName := fmt.Sprintf("AdvancedSettings%d", ver)
		advHigh, _ := dispatch.GetProperty(propName)
		if advHigh != nil {
			a := advHigh.ToIDispatch()
			if a != nil {
				a.PutProperty("ContainerHandledFullScreen", true)
				a.PutProperty("WarnOnDirectConnect", false)
				a.Release()
				log.Writef("[RDP] AdvancedSettings%d: security prompts suppressed", ver)
			}
		}
	}

	// Suppress server certificate warning at OS level
	setAuthLevelOverride()

	// Auto-dismiss any security dialogs that appear during Connect (e.g.
	// "网站正在尝试启动远程连接"). The goroutine polls for dialog windows
	// and clicks "Yes" to dismiss them.
	// On Windows 11, the dialog may appear after Connect succeeds, so keep
	// polling for a few seconds after connection.
	stopDismiss := make(chan struct{})
	go s.autoDismissSecurityDialogs(stopDismiss)
	defer func() {
		// Keep polling for dialogs after Connect completes. On Windows 11
		// the security warning may appear slightly after connection.
		go func() {
			time.Sleep(5 * time.Second)
			close(stopDismiss)
		}()
	}()

	log.Writef("[RDP] calling Connect...")
	_, err = dispatch.CallMethod("Connect")
	if err != nil {
		log.Writef("[RDP] Connect failed: %v", err)
		s.mu.Lock()
		s.hwnd = 0
		s.rdp = nil
		s.mu.Unlock()
		dispatch.Release()
		procDestroyWindow.Call(hwnd)
		s.setStatus(StatusError)
		return fmt.Errorf("RDP Connect: %w", err)
	}

	log.Writef("[RDP] Connect succeeded")
	// Immediate show-and-position to avoid white/black screen.
	// Frontend will refine via RDPSetPosition shortly after.
	s.positionFromMainWindow(width, height)

	s.setStatus(StatusConnected)

	s.runMessagePump()

	log.Writef("[RDP] COM thread exited")
	return nil
}

type rect struct {
	Left, Top, Right, Bottom int32
}

type msg struct {
	HWND    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      struct{ X, Y int32 }
}

func (s *RDPSession) runMessagePump() {
	log.Writef("[RDP] message pump started")
	var m msg
	pumpTick := 0
	noMsgCount := 0
	disconnectLogged := false
	for {
		s.mu.Lock()
		done := s.hwnd == 0
		s.mu.Unlock()
		if done {
			break
		}

		ret, _, _ := procPeekMessage.Call(
			uintptr(unsafe.Pointer(&m)),
			0, 0, 0,
			PM_REMOVE,
		)
		if ret != 0 {
			if m.Message == 0x0012 { // WM_QUIT
				log.Writef("[RDP-pump] WM_QUIT received, exiting")
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
			pumpTick++
			noMsgCount = 0
		} else {
			// No message available; sleep briefly to avoid busy-wait.
			// Check hwnd every ~1 second via heartbeat counter.
			time.Sleep(50 * time.Millisecond)
			noMsgCount++
			if noMsgCount%20 == 0 {
				log.Writef("[RDP-pump] heartbeat idle=%d, pumpMsgs=%d, hwnd=0x%x", noMsgCount, pumpTick, s.hwnd)
				// Check if RDP connection is still alive via ActiveX Connected property.
				// When the remote side drops or the connection is lost, this transitions
				// to 0 while the ActiveX window is still alive.
				if !disconnectLogged {
					s.mu.Lock()
					rdp := s.rdp
					s.mu.Unlock()
					if rdp != nil {
						connected, err := rdp.GetProperty("Connected")
						if err == nil && connected != nil {
							v := connected.Value()
							isDisconnected := false
							if b, ok := v.(bool); ok {
								isDisconnected = !b
							} else if v == nil || v == int16(0) || v == int32(0) || v == 0 {
								isDisconnected = true
							}
							if isDisconnected {
								discMsg := "RDP connection was lost"
								// Try to get the disconnect reason
								reason, reasonErr := rdp.GetProperty("DisconnectedReason")
								if reasonErr == nil && reason != nil {
									discMsg = fmt.Sprintf("RDP disconnected: %v", reason.Value())
								}
								log.Writef("[RDP-pump] connection lost: %s, signaling disconnected", discMsg)
								disconnectLogged = true
								s.setStatus(StatusDisconnected)
								// Post WM_QUIT to exit the pump and trigger cleanup
								s.mu.Lock()
								if s.hwnd != 0 {
									procPostMessageW.Call(s.hwnd, 0x0012, 0, 0)
								}
								s.mu.Unlock()
							}
						}
					}
				}
			}
		}
	}
	log.Writef("[RDP] message pump exited")
}

func (s *RDPSession) findRdpProgID() string {
	candidates := []string{
		"MsRdpClient12NotSafeForScripting",
		"MsRdpClient11NotSafeForScripting",
		"MsRdpClient10NotSafeForScripting",
		"MsRdpClient9NotSafeForScripting",
		"MsRdpClient8NotSafeForScripting",
		"MsTscAxNotSafeForScripting",
		"MsTscAx",
	}
	ole32 := windows.NewLazySystemDLL("ole32.dll")
	procCLSIDFromProgID := ole32.NewProc("CLSIDFromProgID")
	for _, id := range candidates {
		progID, _ := windows.UTF16PtrFromString(id)
		var clsid ole.GUID
		ret, _, _ := procCLSIDFromProgID.Call(
			uintptr(unsafe.Pointer(progID)),
			uintptr(unsafe.Pointer(&clsid)),
		)
		if ret == 0 {
			return id
		}
	}

	clsidCandidates := []string{
		"{9059F30F-4EB1-4BD2-9FDC-36F43A218F4A}",
		"{54D38BF7-B1EF-4479-9674-1BD6EA465258}",
		"{C0EFA91A-EEB7-41C7-97FA-F0ED645EFB24}",
		"{301B94BA-5F25-4A12-9FFE-3B274E75C7DE}",
		"{5F681803-2900-4C43-A1CC-CF405404A676}",
		"{1FB464C8-09BB-4017-A2F5-EB742F04392F}",
	}
	ole32Dll := windows.NewLazySystemDLL("ole32.dll")
	procCLSIDFromString := ole32Dll.NewProc("CLSIDFromString")
	for _, clsidStr := range clsidCandidates {
		wideStr, _ := windows.UTF16PtrFromString(clsidStr)
		var clsid ole.GUID
		ret, _, _ := procCLSIDFromString.Call(
			uintptr(unsafe.Pointer(wideStr)),
			uintptr(unsafe.Pointer(&clsid)),
		)
		if ret == 0 {
			return clsidStr
		}
	}

	return ""
}

// setAuthLevelOverride sets the system-wide RDP authentication level to 0,
// which suppresses the server certificate warning dialog.
func setAuthLevelOverride() {
	// AuthenticationLevelOverride = 0 disables server cert verification.
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Terminal Server Client`,
		registry.SET_VALUE)
	if err != nil {
		k, _, err = registry.CreateKey(registry.CURRENT_USER,
			`Software\Microsoft\Terminal Server Client`,
			registry.SET_VALUE)
		if err != nil {
			return
		}
	}
	defer k.Close()
	k.SetDWordValue("AuthenticationLevelOverride", 0)
	// Also disable the redirection warning dialog via the non-policy key
	k.SetDWordValue("ShowRedirectionWarningDialog", 0)

	// RedirectionWarningDialogVersion = 1 suppresses the "unknown remote
	// connection" security warning dialog. Check if already set first.
	if isRDWAlreadySet() {
		return
	}

	// Write to HKCU policy path (no elevation needed, works on Windows 11)
	rdwPath := `Software\Policies\Microsoft\Windows NT\Terminal Services\Client`
	if writeRegDWORD(registry.CURRENT_USER, rdwPath, "RedirectionWarningDialogVersion", 1) {
		return
	}

	// Fallback: try HKLM policy path (requires admin)
	rdwPathLM := `SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services\Client`
	if writeRegDWORD(registry.LOCAL_MACHINE, rdwPathLM, "RedirectionWarningDialogVersion", 1) {
		return
	}
	elevateRegWrite()
}

// isRDWAlreadySet returns true if RedirectionWarningDialogVersion is already 1.
func isRDWAlreadySet() bool {
	paths := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services\Client`},
		{registry.CURRENT_USER, `Software\Policies\Microsoft\Windows NT\Terminal Services\Client`},
	}
	for _, p := range paths {
		if readRegDWORD(p.root, p.path, "RedirectionWarningDialogVersion") == 1 {
			return true
		}
	}
	return false
}

// writeRegDWORD writes a DWORD value to the registry. Returns true on success.
func writeRegDWORD(root registry.Key, path, name string, value uint32) bool {
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		k, _, err = registry.CreateKey(root, path, registry.SET_VALUE)
		if err != nil {
			return false
		}
	}
	defer k.Close()
	return k.SetDWordValue(name, value) == nil
}

// readRegDWORD reads a DWORD value from the registry. Returns 0 if not found.
func readRegDWORD(root registry.Key, path, name string) uint32 {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return 0
	}
	defer k.Close()
	val, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0
	}
	return uint32(val)
}

// elevateRegWrite launches reg.exe with the "runas" verb to write the
// RedirectionWarningDialogVersion machine-policy key with admin rights.
func elevateRegWrite() {
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	procShellExecute := shell32.NewProc("ShellExecuteW")

	op, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString("reg.exe")
	params, _ := windows.UTF16PtrFromString(
		`add "HKLM\SOFTWARE\Policies\Microsoft\Windows NT\Terminal Services\Client" /v RedirectionWarningDialogVersion /t REG_DWORD /d 1 /f`,
	)

	ret, _, _ := procShellExecute.Call(
		0,
		uintptr(unsafe.Pointer(op)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		0, // SW_HIDE
	)
	if ret <= 32 {
		log.Writef("[RDP] ShellExecute runas failed: %d", ret)
	} else {
		log.Writef("[RDP] UAC elevation requested for RedirectionWarningDialogVersion")
	}
}

func splitRDPUser(value string) (user, domain string) {
	value = strings.TrimSpace(value)
	if i := strings.IndexByte(value, '\\'); i > 0 && i < len(value)-1 {
		return value[i+1:], value[:i]
	}
	return value, ""
}

func comInterface(obj *ole.IDispatch, iid *ole.GUID) (uintptr, error) {
	this := uintptr(unsafe.Pointer(obj))
	vtable := *(*uintptr)(unsafe.Pointer(this))
	queryInterface := *(*uintptr)(unsafe.Pointer(vtable))

	var result uintptr
	hr, _, _ := syscall.SyscallN(
		queryInterface,
		this,
		uintptr(unsafe.Pointer(iid)),
		uintptr(unsafe.Pointer(&result)),
	)
	if hr != 0 {
		return 0, ole.NewError(hr)
	}
	return result, nil
}

func comRelease(obj uintptr) {
	if obj == 0 {
		return
	}
	vtable := *(*uintptr)(unsafe.Pointer(obj))
	release := *(*uintptr)(unsafe.Pointer(vtable + 2*unsafe.Sizeof(uintptr(0))))
	syscall.SyscallN(release, obj)
}

func comHRESULT(obj uintptr, slot uintptr, args ...uintptr) error {
	vtable := *(*uintptr)(unsafe.Pointer(obj))
	method := *(*uintptr)(unsafe.Pointer(vtable + slot*unsafe.Sizeof(uintptr(0))))
	callArgs := make([]uintptr, 0, len(args)+1)
	callArgs = append(callArgs, obj)
	callArgs = append(callArgs, args...)
	hr, _, _ := syscall.SyscallN(method, callArgs...)
	if hr != 0 {
		return ole.NewError(hr)
	}
	return nil
}

func variantBool(value bool) uintptr {
	if value {
		return uintptr(0xffff) // VARIANT_TRUE is -1
	}
	return 0
}

func (s *RDPSession) configureNonScriptable(password string, nla bool) error {
	if s.rdp == nil {
		return fmt.Errorf("RDP control is unavailable")
	}

	// IMsTscNonScriptable is vtable-only. Its first method after IUnknown is
	// put_ClearTextPassword (slot 3); it must run before Connect.
	baseIID := ole.NewGUID("{C1E6743A-41C1-4A74-832A-0DD06C1C7A0E}")
	base, err := comInterface(s.rdp, baseIID)
	if err != nil {
		return fmt.Errorf("query IMsTscNonScriptable: %w", err)
	}
	defer comRelease(base)

	if password != "" {
		bstr := ole.SysAllocStringLen(password)
		if bstr == nil {
			return fmt.Errorf("allocate RDP password")
		}
		defer ole.SysFreeString(bstr)
		if err := comHRESULT(base, 3, uintptr(unsafe.Pointer(bstr))); err != nil {
			return fmt.Errorf("set IMsTscNonScriptable ClearTextPassword: %w", err)
		}
	}

	// IMsRdpClientNonScriptable3 inherits the password interface. Its vtable
	// slots 19 and 23 are put_PromptForCredentials and
	// put_EnableCredSspSupport respectively.
	ns3IID := ole.NewGUID("{B3378D90-0728-45C7-8ED7-B6159FB92219}")
	ns3, err := comInterface(s.rdp, ns3IID)
	if err != nil {
		return fmt.Errorf("query IMsRdpClientNonScriptable3: %w", err)
	}
	defer comRelease(ns3)

	if err := comHRESULT(ns3, 19, variantBool(false)); err != nil {
		return fmt.Errorf("disable RDP credential prompt: %w", err)
	}
	if err := comHRESULT(ns3, 23, variantBool(nla)); err != nil {
		return fmt.Errorf("set RDP CredSSP: %w", err)
	}

	// Newer controls expose two additional prompt gates. They are optional for
	// older controls, but disabling both prevents the ActiveX host from falling
	// back to a Windows credential dialog when the supplied credential fails.
	ns4IID := ole.NewGUID("{F50FA8AA-1C7D-4F59-B15C-A90CACAE1FCB}")
	if ns4, queryErr := comInterface(s.rdp, ns4IID); queryErr == nil {
		defer comRelease(ns4)
		if err := comHRESULT(ns4, 47, variantBool(false)); err != nil {
			log.Writef("[RDP] disable PromptForCredsOnClient failed: %v", err)
		}
		if err := comHRESULT(ns4, 45, variantBool(false)); err != nil {
			log.Writef("[RDP] disable credential saving failed: %v", err)
		}
	}

	ns5IID := ole.NewGUID("{4F6996D5-D7B1-412C-B0FF-063718566907}")
	if ns5, queryErr := comInterface(s.rdp, ns5IID); queryErr == nil {
		defer comRelease(ns5)
		if err := comHRESULT(ns5, 63, variantBool(false)); err != nil {
			log.Writef("[RDP] disable AllowPromptingForCredentials failed: %v", err)
		}
	}

	log.Writef("[RDP] NonScriptable configured: password=%v prompt=false credssp=%v", password != "", nla)
	return nil
}

type point struct{ X, Y int32 }

// positionFromMainWindow calculates the RDP window position and initializes tracking.
func (s *RDPSession) positionFromMainWindow(width, height int) {
	if s.parentHwnd == 0 || s.hwnd == 0 {
		return
	}
	var cr rect
	ret, _, _ := procGetClientRect.Call(s.parentHwnd, uintptr(unsafe.Pointer(&cr)))
	if ret == 0 {
		log.Writef("[RDP] GetClientRect failed, fallback to GetWindowRect")
		var wr rect
		ret2, _, _ := procGetWindowRect.Call(s.parentHwnd, uintptr(unsafe.Pointer(&wr)))
		if ret2 == 0 {
			log.Writef("[RDP] GetWindowRect also failed")
			return
		}
		cr = rect{0, 0, wr.Right - wr.Left, wr.Bottom - wr.Top}
	}
	var origin point
	ret2, _, _ := procClientToScreen.Call(s.parentHwnd, uintptr(unsafe.Pointer(&origin)))
	if ret2 == 0 {
		origin = point{0, 0}
	}
	clientLeft := int(origin.X)
	clientTop := int(origin.Y)
	clientWidth := int(cr.Right - cr.Left)
	clientHeight := int(cr.Bottom - cr.Top)

	topReserve := 80
	bottomReserve := 32
	sideMargin := 4

	x := clientLeft + sideMargin
	y := clientTop + topReserve
	w := clientWidth - sideMargin*2
	h := clientHeight - topReserve - bottomReserve
	log.Writef("[RDP] backend positioning: client=(%d,%d %dx%d) rdp=(x=%d y=%d w=%d h=%d)",
		clientLeft, clientTop, clientWidth, clientHeight, x, y, w, h)

	s.shown = true
	procSetWindowPos.Call(s.hwnd, 0,
		uintptr(x), uintptr(y),
		uintptr(w), uintptr(h),
		SWP_SHOWWINDOW|SWP_NOACTIVATE|SWP_ASYNCWINDOWPOS)

	s.trackX = x
	s.trackY = y
	log.Writef("[RDP] backend position done, shown=%v hwnd=0x%x", s.shown, s.hwnd)
}

func (s *RDPSession) SetPosition(x, y, w, h int) {
	s.mu.Lock()
	hwnd := s.hwnd
	if hwnd == 0 {
		log.Writef("[RDP-SetPos] SKIP hwnd=0")
		s.mu.Unlock()
		return
	}
	s.shown = true
	s.trackX = x
	s.trackY = y
	log.Writef("[RDP-SetPos] FROM FRONTEND x=%d y=%d w=%d h=%d", x, y, w, h)
	s.mu.Unlock()

	// Owned window: z-order is automatic. SWP_SHOWWINDOW handles tab-switch restore.
	procSetWindowPos.Call(hwnd, 0,
		uintptr(x), uintptr(y),
		uintptr(w), uintptr(h),
		SWP_SHOWWINDOW|SWP_NOACTIVATE|SWP_ASYNCWINDOWPOS)
}

// SetFocus adjusts the RDP window z-order when uniTerm gains or loses focus.
// With the owned-window model, z-order is fully automatic — the OS handles it.
// Kept as a no-op for API compatibility with the frontend.
func (s *RDPSession) SetFocus(focused bool) {
	log.Writef("[RDP-focus] SetFocus focused=%v (no-op, owned-window model)", focused)
}

func (s *RDPSession) Show() {
	s.mu.Lock()
	if s.shown {
		s.mu.Unlock()
		return
	}
	hwnd := s.hwnd
	tX := s.trackX
	tY := s.trackY
	s.shown = true
	s.mu.Unlock()
	log.Writef("[RDP-Show] trackX=%d trackY=%d hwnd=0x%x", tX, tY, hwnd)
	if hwnd != 0 {
		procShowWindow.Call(hwnd, SW_SHOWNOACTIVATE)
		procSetWindowPos.Call(hwnd, 0,
			uintptr(tX), uintptr(tY),
			0, 0,
			SWP_NOSIZE|SWP_NOACTIVATE|SWP_NOZORDER|SWP_ASYNCWINDOWPOS)
	}
}

func (s *RDPSession) Hide() {
	log.Writef("[RDP-Hide] called")
	s.mu.Lock()
	if !s.shown {
		s.mu.Unlock()
		return
	}
	hwnd := s.hwnd
	s.shown = false
	s.mu.Unlock()
	if hwnd != 0 {
		// SW_HIDE hides the window so the OS stops sending paint messages
		// and the ActiveX stops rendering in background.
		procShowWindow.Call(hwnd, SW_HIDE)
	}
}

func (s *RDPSession) Disconnect() error {
	// Post WM_QUIT to the COM STA message pump so it exits cleanly.
	// Do NOT zero s.hwnd here — the defer in Connect() needs it
	// to call DestroyWindow for proper cleanup.
	s.mu.Lock()
	hwnd := s.hwnd
	s.mu.Unlock()

	if hwnd != 0 {
		procPostMessageW.Call(hwnd, 0x0012, 0, 0) // WM_QUIT
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *RDPSession) Resize(cols, rows int) error {
	s.mu.Lock()
	if s.rdp != nil {
		s.rdp.PutProperty("DesktopWidth", cols)
		s.rdp.PutProperty("DesktopHeight", rows)
	}
	s.mu.Unlock()
	return nil
}

func (s *RDPSession) Write(_ []byte) error {
	return nil
}

func (s *RDPSession) IsConnected() bool {
	return s.Status() == StatusConnected
}
