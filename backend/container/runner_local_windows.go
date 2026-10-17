//go:build windows

package container

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"

	"github.com/UserExistsError/conpty"

	"github.com/ys-ll/uniterm/backend/platform"
)

// setHideWindow delegates to the shared platform helper so every console
// child spawn site funnels through the same CREATE_NO_WINDOW entry point.
func setHideWindow(cmd *exec.Cmd) {
	platform.HideConsoleWindow(cmd)
}

type localPTY struct {
	cpty *conpty.ConPty
	data chan []byte
	done chan struct{}
	mu   sync.Mutex
	once sync.Once
}

// windowsCommandLine 把 argv 拼回命令行（conpty 只接受命令行字符串）。
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

func (r *LocalRunner) RunPTY(_ context.Context, argv []string, cols, rows int) (PTYStream, error) {
	if p, err := resolveBinary(argv[0]); err == nil {
		argv[0] = p
	}
	cpty, err := conpty.Start(windowsCommandLine(argv), conpty.ConPtyDimensions(cols, rows))
	if err != nil {
		return nil, err
	}
	p := &localPTY{cpty: cpty, data: make(chan []byte, 32), done: make(chan struct{})}
	go func() {
		readPTYOutput(cpty, p.data, p.done)
		// ConPTY can flush output while Close waits, before closing its handles.
		_, _ = io.Copy(io.Discard, cpty)
	}()
	return p, nil
}

func (p *localPTY) Data() <-chan []byte { return p.data }
func (p *localPTY) Write(b []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, err := p.cpty.Write(b)
	return err
}
func (p *localPTY) Resize(cols, rows int) error { return p.cpty.Resize(cols, rows) }
func (p *localPTY) Close() error {
	var err error
	p.once.Do(func() { close(p.done); err = p.cpty.Close() })
	return err
}
