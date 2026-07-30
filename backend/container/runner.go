package container

import "context"

// LineStream 是 logs -f / pull 这类流式命令的行通道。
type LineStream interface {
	Lines() <-chan string
	Wait() error
	Close() error
}

// PTYStream 是 exec 终端的双工通道。
type PTYStream interface {
	Data() <-chan []byte
	Write(p []byte) error
	Resize(cols, rows int) error
	Close() error
}

// Runner is the abstraction every container backend implements.
// LocalRunner (runner_local*.go via build tags) and SSHRunner
// (runner_ssh.go) are the two concrete adapters; Provider holds one
// of these and dispatches calls to it. New backends (e.g. a remote
// containerd) implement this same interface.
type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
	RunStream(ctx context.Context, argv []string) (LineStream, error)
	RunPTY(ctx context.Context, argv []string, cols, rows int) (PTYStream, error)
}

// Compile-time interface satisfaction checks — fail at build if a
// concrete runner drifts away from the Runner contract.
var (
	_ Runner = (*LocalRunner)(nil)
	_ Runner = (*SSHRunner)(nil)
)
