//go:build windows

package session

import (
	"runtime"
	"fmt"
	"sync"
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


	user32Dll              = windows.NewLazySystemDLL("user32.dll")
	procSetWindowPos       = user32Dll.NewProc("SetWindowPos")
	procShowWindow         = user32Dll.NewProc("ShowWindow")
	procDestroyWindow      = user32Dll.NewProc("DestroyWindow")
	procFindWindowW        = user32Dll.NewProc("FindWindowW")
	procPeekMessage        = user32Dll.NewProc("PeekMessageW")
	procTranslateMessage   = user32Dll.NewProc("TranslateMessage")
	procDispatchMessage    = user32Dll.NewProc("DispatchMessageW")
	procGetWindowRect      = user32Dll.NewProc("GetWindowRect")
	procGetClientRect      = user32Dll.NewProc("GetClientRect")
	procClientToScreen     = user32Dll.NewProc("ClientToScreen")
	procSetWindowLongPtr   = user32Dll.NewProc("SetWindowLongPtrW")
	procPostMessageW       = user32Dll.NewProc("PostMessageW")
	procFindWindowExW      = user32Dll.NewProc("FindWindowExW")
	procSendMessageW       = user32Dll.NewProc("SendMessageW")
)

const (
	WM_CLOSE       = 0x0010
	SWP_SHOWWINDOW = 0x0040
	SWP_HIDEWINDOW = 0x0080
	SWP_NOMOVE     = 0x0002
	SWP_NOSIZE     = 0x0001
	SWP_NOACTIVATE = 0x0010
	SWP_NOZORDER   = 0x0004
	SWP_ASYNCWINDOWPOS = 0x4000 // non-blocking: avoids freezing RDP COM thread
	WS_EX_TOOLWINDOW  = 0x00000080
	WS_EX_NOACTIVATE  = 0x08000000
	WS_POPUP          = 0x80000000
	WS_CLIPSIBLINGS   = 0x04000000
	PM_REMOVE         = 0x0001
	GWLP_HWNDPARENT   = ^uintptr(7) // -8 represented as uintptr for syscall compatibility
	SW_HIDE           = 0
	SW_SHOWNOACTIVATE = 4
	WM_COMMAND        = 0x0111
	BM_CLICK          = 0x00F5
	IDYES             = 6
	IDOK              = 1
)

type RDPSession struct {
	baseSession
	parentHwnd uintptr
	hwnd       uintptr
	rdp        *ole.IDispatch
	config     ConnectionConfig
	mu         sync.Mutex
	shown     bool

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
		uintptr(WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE),
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

	// NonScriptable.ClearTextPassword BEFORE AdvancedSettings.
	s.configureNonScriptable(config.Password)

	dispatch.PutProperty("Server", config.Host)
	dispatch.PutProperty("UserName", config.User)
	dispatch.PutProperty("Domain", "")
	dispatch.PutProperty("DesktopWidth", width)
	dispatch.PutProperty("DesktopHeight", height)
	dispatch.PutProperty("FullScreen", false)

	// AdvancedSettings2
	advObj, _ := dispatch.GetProperty("AdvancedSettings2")
	if advObj != nil {
		adv := advObj.ToIDispatch()
		if adv != nil {
			adv.PutProperty("RDPPort", port)
			adv.PutProperty("RedirectClipboard", true)
			adv.PutProperty("RedirectDrives", true)
			adv.PutProperty("DisplayConnectionBar", false)
			adv.PutProperty("EnableAutoReconnect", true)
			if config.RdpEnableNLA {
				adv.PutProperty("EnableCredSspSupport", true)
				adv.PutProperty("AuthenticationLevel", 2)
			} else {
				adv.PutProperty("EnableCredSspSupport", false)
				adv.PutProperty("AuthenticationLevel", 0)
			}
			adv.PutProperty("WarnOnDirectConnect", false)
			adv.PutProperty("ContainerHandledFullScreen", true)
			if config.RdpSmartSizing {
				adv.PutProperty("SmartSizing", true)
			}
			if config.Password != "" {
				adv.PutProperty("ClearTextPassword", config.Password)
			}
			adv.Release()
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

func (s *RDPSession) configureNonScriptable(password string) {
	if s.rdp == nil {
		return
	}
	nsGUIDs := []string{
		"{C1E6743A-41C1-4A74-832A-0DD06C0C9265}", // IMsTscNonScriptable (base)
		"{4F5331FB-42F5-48A2-9AFD-4743E3F6D3D7}", // IMsRdpClientNonScriptable5
		"{F50FA8AA-1C05-471B-9CB5-3BD7A6FD32BD}", // IMsRdpClientNonScriptable4
		"{B3378D90-0728-45C7-8ED7-B6159FB92219}", // IMsRdpClientNonScriptable3
	}
	unk, err := s.rdp.QueryInterface(ole.IID_IUnknown)
	if err != nil {
		log.Writef("[RDP] QI IUnknown for NonScriptable: %v", err)
		return
	}
	defer unk.Release()
	for i, guid := range nsGUIDs {
		nsGUID := ole.NewGUID(guid)
		nsUnk, err := unk.QueryInterface(nsGUID)
		if err != nil || nsUnk == nil {
			continue
		}
		if password != "" {
			nsUnk.PutProperty("ClearTextPassword", password)
		}
		nsUnk.Release()
		log.Writef("[RDP] NonScriptable[%d] configured, password=%v", i, password != "")
		break
	}
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
