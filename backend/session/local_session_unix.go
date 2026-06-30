//go:build !windows
// +build !windows

package session

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/creack/pty"
)

type LocalSession struct {
	baseSession
	cmd      *exec.Cmd
	pty      *os.File
	quit     chan struct{}
	quitOnce sync.Once
}

func NewLocalSession(id string) *LocalSession {
	s := &LocalSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "local",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
	// Set a generous default size so the PTY is unlikely to scroll before
	// the frontend sends its first Resize() with the real dimensions.
	s.SetPendingSize(200, 60)
	return s
}

func (s *LocalSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)

	shell := config.ShellPath
	if shell == "" {
		shell = defaultShell()
	}

	s.title = shellName(shell)

	s.cmd = exec.Command(shell)
	s.cmd.Env = os.Environ()

	ptyFile, err := pty.Start(s.cmd)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("start pty: %w", err)
	}
	s.pty = ptyFile

	go func() {
		_ = s.cmd.Wait()
		s.Disconnect()
	}()

	s.setStatus(StatusConnected)

	if cols, rows := s.GetPendingSize(); cols > 0 && rows > 0 {
		_ = pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
	}

	go s.readLoop()
	go s.runPostLoginScript(config.PostLoginScript)

	return nil
}

func (s *LocalSession) readLoop() {
	buf := make([]byte, 4096)
	for {
		select {
		case <-s.quit:
			return
		default:
		}

		n, err := s.pty.Read(buf)
		if n > 0 {
			s.RecordReadActivity()
			s.emitData(append([]byte(nil), buf[:n]...))
		}
		if err != nil {
			if err != io.EOF {
				s.emitData([]byte(fmt.Sprintf("\r\n[read error: %v]\r\n", err)))
			}
			s.Disconnect()
			return
		}
	}
}

func (s *LocalSession) Write(data []byte) error {
	if s.pty == nil {
		return fmt.Errorf("not connected")
	}
	_, err := s.pty.Write(data)
	return err
}

func (s *LocalSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
	if s.pty != nil {
		s.pty.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *LocalSession) Resize(cols, rows int) error {
	s.SetPendingSize(cols, rows)
	if s.pty == nil {
		return fmt.Errorf("session not connected")
	}
	return pty.Setsize(s.pty, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (s *LocalSession) IsConnected() bool {
	return s.Status() == StatusConnected
}

func (s *LocalSession) runPostLoginScript(script string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-s.quit:
			cancel()
		case <-ctx.Done():
		}
	}()

	send := func(data []byte) {
		if s.pty != nil {
			s.pty.Write(data)
		}
	}
	s.baseSession.RunPostLoginScript(ctx, script, send, s.IsConnected)
}

func defaultShell() string {
	switch runtime.GOOS {
	case "windows":
		if _, err := exec.LookPath("pwsh.exe"); err == nil {
			return "pwsh.exe"
		}
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			return "powershell.exe"
		}
		return "cmd.exe"
	default:
		if shell := os.Getenv("SHELL"); shell != "" {
			return shell
		}
		if _, err := exec.LookPath("bash"); err == nil {
			return "bash"
		}
		return "sh"
	}
}

func shellName(path string) string {
	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
	}
	return base
}
