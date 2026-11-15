//go:build windows

package packetcap

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var errElevationCancelled = errors.New("the UAC elevation prompt was cancelled")

// isProcessElevated reports whether the current process runs with an elevated
// (administrator) token.
func isProcessElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	var elevation uint32
	var returned uint32
	if err := windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &returned); err != nil {
		return false
	}
	return elevation != 0
}

// createWorkerPipeServer creates the parent-side named pipe the elevated
// worker connects to. The explicit security descriptor grants full access to
// the creating user only — the pipe carries raw packet capture output, so no
// other process on the machine may open it.
func createWorkerPipeServer(pipeName string) (windows.Handle, error) {
	token, err := windows.OpenCurrentProcessToken()
	if err != nil {
		return 0, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return 0, err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return 0, err
	}
	sa := &windows.SecurityAttributes{SecurityDescriptor: sd}
	namePtr, err := windows.UTF16PtrFromString(pipeName)
	if err != nil {
		return 0, err
	}
	return windows.CreateNamedPipe(
		namePtr,
		windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_FIRST_PIPE_INSTANCE|windows.FILE_FLAG_OVERLAPPED,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS,
		1, 65536, 65536, 0, sa)
}

// shellExecuteRunAs launches target with the "runas" verb (routes through UAC)
// and returns the spawned process handle, or errElevationCancelled when the
// user dismissed the prompt.
func shellExecuteRunAs(target, parameters string) (windows.Handle, error) {
	type shellExecuteInfo struct {
		cbSize         uint32
		fMask          uint32
		hwnd           windows.HWND
		lpVerb         uintptr
		lpFile         uintptr
		lpParameters   uintptr
		lpDirectory    uintptr
		nShow          int32
		hInstApp       uintptr
		lpIDList       uintptr
		lpClass        uintptr
		hkeyClass      windows.Handle
		dwHotKey       uint32
		hIconOrMonitor uintptr
		hProcess       windows.Handle
	}
	const (
		seeMaskNOCLOSEPROCESS = 0x00000040
		seeMaskNOASYNC        = 0x00001000
		swHide                = 0
	)
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(target)
	params, _ := windows.UTF16PtrFromString(parameters)
	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNOCLOSEPROCESS | seeMaskNOASYNC,
		lpVerb:       uintptr(unsafe.Pointer(verb)),
		lpFile:       uintptr(unsafe.Pointer(file)),
		lpParameters: uintptr(unsafe.Pointer(params)),
		nShow:        swHide,
	}
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteExW := shell32.NewProc("ShellExecuteExW")
	ret, _, _ := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		if errors.Is(windows.GetLastError(), windows.ERROR_CANCELLED) {
			return 0, errElevationCancelled
		}
		return 0, fmt.Errorf("ShellExecuteEx(runas) failed: %w", windows.GetLastError())
	}
	return info.hProcess, nil
}

// captureViaWorker is the parent-side path used when this process is NOT
// elevated: it creates a user-only named pipe, re-launches uniTerm elevated
// with the worker flags, readss the worker's streamed packetshopping them to
// onPacket. It returns when the pipe closes or ctx is cancelled.
func captureViaWorker(ctx context.Context, cfg CaptureConfig, onPacket func(*Packet)) error {
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return fmt.Errorf("pipe name: %w", err)
	}
	pipeName := `\\.\pipe\uniterm-pktcap-` + hex.EncodeToString(suffix)

	handle, err := createWorkerPipeServer(pipeName)
	if err != nil {
		return fmt.Errorf("create pipe: %w", err)
	}

	exe, err := executablePath()
	if err != nil {
		windows.CloseHandle(handle)
		return fmt.Errorf("locate executable: %w", err)
	}
	args := fmt.Sprintf("%s %s %s %q %s %q", WorkerArgPrefix, pipeName,
		WorkerFlagIface, cfg.InterfaceIP, WorkerFlagFilter, cfg.Filter)
	workerProc, err := shellExecuteRunAs(exe, args)
	if err != nil {
		windows.CloseHandle(handle)
		if errors.Is(err, errElevationCancelled) {
			return errors.New("提权被取消：抓包需要管理员（提权）进程，请在系统弹出提权确认时点击允许")
		}
		return err
	}

	// Wrap the handle so reads honour a read deadline (SetReadDeadline) used
	// to bound the initial handshake.
	f := newPipeFile(handle, pipeName)
	defer f.Close()

	workerGone := make(chan struct{})
	if workerProc != 0 {
		go func() {
			windows.WaitForSingleObject(workerProc, windows.INFINITE)
			close(workerGone)
		}()
	}

	// Bridge ctx cancellation (Stop) into a hard teardown so a Stop is prompt
	// and complete no matter what the worker is doing:
	//   - closing f cancels our own pending pipe read immediately;
	//   - TerminateProcess guarantees the elevated worker dies. The worker can
	//     be parked inside the raw-socket read while no packets are flowing,
	//     where closing our pipe alone would never reach it — it would linger
	//     forever (the leaked elevated worker you saw in Task Manager). Killing
	//     it also closes its pipe handle, which is what actually breaks our
	//     pending read out of ERROR_BROKEN_PIPE / ERROR_OPERATION_ABORTED, so
	//     readWorkerStream always returns and the stopped event always fires.
	// Cleanup note: the deferred f.Close() (and the OS reclaiming workerProc)
	// make this idempotent on the normal-exit path.
	go func() {
		<-ctx.Done()
		f.Close()
		if workerProc != 0 {
			windows.TerminateProcess(workerProc, 1)
		}
	}()

	// Read through a single bufio.Reader: the handshake consumes the worker's
	// hello line, and the same reader keeps draining the packet stream so no
	// bytes are ever dropped.
	r := bufio.NewReaderSize(f, 64*1024)

	// Bounded handshake: the worker connects (hello) within 30s or we bail.
	// Between spawn and connect the pipe is in listening state and reads fail
	// with ERROR_PIPE_LISTENING. If the user stopped during elevation, the
	// pending read is aborted by our own pipe close (ctx), which is a clean
	// manual stop — not a capture error.
	if err := awaitWorkerHello(r, f, workerGone, 30*time.Second); err != nil {
		if ctx.Err() != nil {
			return errStopped
		}
		return fmt.Errorf("worker handshake: %w", err)
	}

	err = readWorkerStream(r, onPacket)
	if err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		return err
	}
	return errStopped
}

// awaitWorkerHello reads the worker's hello line while bound by a deadline and
// the worker's liveness. On success the (possibly buffered) hello line is
// consumed and r is left positioned at the start of the packet stream.
func awaitWorkerHello(r *bufio.Reader, f *pipeFile, workerGone <-chan struct{}, timeout time.Duration) error {
	_ = f.SetReadDeadline(time.Now().Add(timeout))
	defer f.SetReadDeadline(time.Time{})

	deadline := time.After(timeout)
	for {
		line, err := r.ReadString('\n')
		if err == nil || len(line) > 0 {
			if strings.TrimSpace(line) == workerHello {
				return nil
			}
		}
		switch {
		case err == nil && len(line) == 0:
			// spurious empty read; keep waiting
		case errors.Is(err, windows.ERROR_PIPE_LISTENING):
			select {
			case <-workerGone:
				return errors.New("elevated worker exited before connecting")
			case <-time.After(50 * time.Millisecond):
			}
		case err != nil:
			// read deadline exceeded, worker alive but slow, or read failed
			select {
			case <-workerGone:
				return errors.New("elevated worker exited before connecting")
			case <-deadline:
				return errors.New("timed out waiting for worker to connect")
			default:
			}
			// keep waiting unless the error is terminal for the pipe
			if !errors.Is(err, os.ErrDeadlineExceeded) &&
				!errors.Is(err, windows.WAIT_TIMEOUT) &&
				!errors.Is(err, windows.ERROR_SEM_TIMEOUT) {
				return err
			}
		}
	}
}
