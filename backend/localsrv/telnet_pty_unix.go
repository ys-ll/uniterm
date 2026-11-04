//go:build !windows
// +build !windows

package localsrv

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/creack/pty"
)

// unixTelnetPty wraps a creack/pty pseudo-terminal for a local login shell.
// Data() streams the shell's PTY output; Done() closes when the shell exits;
// Close() tears the PTY down and reaps the child.
type unixTelnetPty struct {
	cmd  *exec.Cmd
	ptmx *os.File
	data chan []byte
	done chan struct{}
	once sync.Once
}

func (p *unixTelnetPty) Data() <-chan []byte  { return p.data }
func (p *unixTelnetPty) Done() <-chan struct{} { return p.done }

func (p *unixTelnetPty) Write(b []byte) error {
	_, err := p.ptmx.Write(b)
	return err
}

// Resize resizes the PTY to the given column/row dimensions.
func (p *unixTelnetPty) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return pty.Setsize(p.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (p *unixTelnetPty) Close() error {
	var err error
	p.once.Do(func() {
		_ = p.ptmx.Close()
		if p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
		_ = p.cmd.Wait()
	})
	return err
}

// startLocalShell launches the user's interactive shell on a PTY. On Unix we
// prefer $SHELL, then bash, then sh (Android/Harmony fall back to
// /system/bin/sh) and export a valid TERM so line-editing works.
func startLocalShell(ctx context.Context, cols, rows int) (telnetPty, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		if runtime.GOOS == "android" {
			shell = "/system/bin/sh"
		} else if p, err := exec.LookPath("bash"); err == nil {
			shell = p
		} else {
			shell = "sh"
		}
	}

	cmd := exec.CommandContext(ctx, shell)
	// Ensure an interactive/editing-capable TERM when the parent (a GUI app
	// launched from a desktop) did not export one.
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	if cols <= 0 {
		cols = 120
	}
	if rows <= 0 {
		rows = 30
	}
	ws := &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
	ptmx, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, err
	}

	p := &unixTelnetPty{
		cmd:  cmd,
		ptmx: ptmx,
		data: make(chan []byte, 32),
		done: make(chan struct{}),
	}
	go func() {
		defer close(p.done)
		_ = cmd.Wait()
	}()
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				p.data <- cp
			}
			if err != nil {
				close(p.data)
				return
			}
		}
	}()
	return p, nil
}
