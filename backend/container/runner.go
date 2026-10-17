package container

import (
	"context"
	"io"
)

// LineStream 是 logs -f / pull 这类流式命令的行通道。
type LineStream interface {
	Lines() <-chan string
	Wait() error
	Close() error
}

// RawStream 是二进制子流（docker cp 的 tar 流）：读毕 EOF 后调用 Wait 回收，
// 提前中止用 Close。
type RawStream interface {
	io.ReadCloser
	Wait() error
}

// PTYStream 是 exec 终端的双工通道。
type PTYStream interface {
	Data() <-chan []byte
	Write(p []byte) error
	Resize(cols, rows int) error
	Close() error
}

// readPTYOutput preserves backpressure while the terminal authority attaches.
// Close must unblock both Read (by closing r) and a send to a full output queue.
func readPTYOutput(r io.Reader, data chan<- []byte, done <-chan struct{}) {
	defer close(data)
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			cp := make([]byte, n)
			copy(cp, buf[:n])
			select {
			case data <- cp:
			case <-done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

type Runner interface {
	Run(ctx context.Context, argv []string) ([]byte, error)
	// RunStdin 带标准输入执行一次命令（docker login --password-stdin 用）。
	RunStdin(ctx context.Context, argv []string, stdin []byte) ([]byte, error)
	// RunStdinStream 带流式标准输入执行（本机 tar 流导入容器用），返回 stdout。
	RunStdinStream(ctx context.Context, argv []string, stdin io.Reader) ([]byte, error)
	// RunRaw 返回 stdout 的二进制流（docker cp 导出 tar 用）。
	RunRaw(ctx context.Context, argv []string) (RawStream, error)
	RunStream(ctx context.Context, argv []string) (LineStream, error)
	RunPTY(ctx context.Context, argv []string, cols, rows int) (PTYStream, error)
}
