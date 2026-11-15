//go:build windows

package packetcap

import (
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// pipeFile wraps *os.File so the parent side can set a read deadline (os.File
// on an overlapped windows handle honours that through Go's async I/O
// machinery — the same mechanism the session package's adminPty uses).
type pipeFile struct {
	f *os.File
}

func newPipeFile(handle windows.Handle, name string) *pipeFile {
	return &pipeFile{f: os.NewFile(uintptr(handle), name)}
}

func (p *pipeFile) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *pipeFile) Write(b []byte) (int, error) { return p.f.Write(b) }
func (p *pipeFile) Close() error                { return p.f.Close() }
func (p *pipeFile) SetReadDeadline(t time.Time) error {
	return p.f.SetReadDeadline(t)
}

// executablePath returns the current executable's path for the runas
// re-launch. Queried at spawn time because argv[0] is not reliable across the
// elevated re-launch.
func executablePath() (string, error) {
	return os.Executable()
}

// connectWorkerPipe opens the parent's named pipe as a client (CreateFile).
// The parent creates the server instance before spawning us; if it is not yet
// ready (ERROR_PIPE_BUSY / not available) we retry briefly until it is.
func connectWorkerPipe(pipeName string) (io.ReadWriteCloser, error) {
	p, _ := windows.UTF16PtrFromString(pipeName)
	var lastErr error
	for i := 0; i < 200; i++ { // ~10s worst case
		h, err := windows.CreateFile(p,
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0, nil, windows.OPEN_EXISTING,
			windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return newPipeFile(h, pipeName), nil
		}
		lastErr = err
		if err != windows.ERROR_PIPE_BUSY && err != windows.ERROR_FILE_NOT_FOUND {
			return nil, err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil, lastErr
}
