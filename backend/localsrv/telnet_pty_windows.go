//go:build windows
// +build windows

package localsrv

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"

	"github.com/UserExistsError/conpty"
	"golang.org/x/sys/windows"
)

// winTelnetPty wraps a Windows ConPTY for a local login shell. Data() streams
// the conpty output; Done() closes when the shell's output pipe reaches EOF;
// Close() tears the conpty down.
type winTelnetPty struct {
	cpty *conpty.ConPty
	data chan []byte
	done chan struct{}
	once sync.Once
}

func (p *winTelnetPty) Data() <-chan []byte  { return p.data }
func (p *winTelnetPty) Done() <-chan struct{} { return p.done }

func (p *winTelnetPty) Write(b []byte) error {
	_, err := p.cpty.Write(b)
	return err
}

// Resize resizes the ConPTY to the given column/row dimensions.
func (p *winTelnetPty) Resize(cols, rows int) error {
	return p.cpty.Resize(cols, rows)
}

func (p *winTelnetPty) Close() error {
	var err error
	p.once.Do(func() { err = p.cpty.Close() })
	return err
}

// windowsCommandLine joins argv back into a command-line string (conpty only
// accepts a command-line string).
func windowsCommandLine(argv []string) string {
	parts := make([]string, len(argv))
	for i, a := range argv {
		if strings.ContainsAny(a, " \t") {
			a = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
		}
		parts[i] = a
	}
	return strings.Join(parts, " ")
}

// Use cmd.exe as the telnet shell. PowerShell 5.1 drives its prompt/line
// editing with heavy CSI cursor-positioning redraws; stripping those for a
// remote telnet client leaves stale redraw text on screen and desyncs the
// shell's read state (e.g. "exit" → "exexiexit", stray ">>" continuations).
// cmd.exe's prompt editing is far simpler and survives control-sequence
// stripping cleanly, so telnet sessions get a reliable interactive shell.
func startLocalShell(ctx context.Context, cols, rows int) (telnetPty, error) {
	shell := "cmd.exe"
	_ = ctx // ConPTY has no context-cancellable start; lifetime is managed via Close().

	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	opts := []conpty.ConPtyOption{conpty.ConPtyDimensions(cols, rows)}
	// Start in the user's home directory instead of the exe's directory so the
	// prompt does not show the install/bin path.
	if home, err := os.UserHomeDir(); err == nil {
		opts = append(opts, conpty.ConPtyWorkDir(home))
	}

	cpty, err := conpty.Start(windowsCommandLine([]string{shell}), opts...)
	if err != nil {
		return nil, err
	}

	p := &winTelnetPty{cpty: cpty, data: make(chan []byte, 32), done: make(chan struct{})}
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := cpty.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				p.data <- cp
			}
			if err != nil {
				// ConPTY emits transient errors (e.g. ERROR_INVALID_HANDLE =
				// "The handle is invalid.") during the short window while the
				// child shell is still initialising, even though the shell goes
				// on to produce output. Treating any error as EOF caused the
				// telnet session to be torn down moments after connecting, so
				// only a true pipe-end / handle-closed condition ends the
				// stream; everything else is skipped and read again.
				if isConPtyEOF(err) {
					close(p.data)
					return
				}
				continue
			}
		}
	}()
	// Done() must reflect the shell process having actually exited, not the
	// transient errors above. conpty exposes no join API, but Wait polls the
	// process handle and returns once the process really terminates.
	go func() {
		_, _ = cpty.Wait(context.Background())
		close(p.done)
	}()
	return p, nil
}

// isConPtyEOF reports whether an error from a ConPTY read means the output
// stream genuinely ended (pipe broken / handle closed / EOF), as opposed to a
// transient error that can be retried.
func isConPtyEOF(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
		return true
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case windows.ERROR_BROKEN_PIPE,
			windows.ERROR_HANDLE_EOF,
			windows.ERROR_NO_DATA,
			windows.ERROR_PIPE_NOT_CONNECTED:
			return true
		}
	}
	return false
}
