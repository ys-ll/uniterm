//go:build !windows

package packetcap

import (
	"io"
	"os"
)

// connectWorkerPipe is the worker (child) side of the elevated-worker split.
// On Unix the parent hands its connection over stdio (see linuxCaptureViaWorker
// in worker_linux.go): the parent pipes its stdout into the worker's stdin and
// reads the worker's stdout back. There is no named pipe, so pipeName is used
// only as the ServeWorker marker and ignored here.
func connectWorkerPipe(pipeName string) (io.ReadWriteCloser, error) {
	return &stdioPipe{in: os.Stdin, out: os.Stdout}, nil
}

// stdioPipe adapts the worker's stdin/stdout into a single ReadWriteCloser so
// the shared worker plumbing in worker.go (write hello, then stream JSON lines
// via bufio) is identical across platforms.
type stdioPipe struct {
	in  io.ReadCloser
	out io.WriteCloser
}

func (p *stdioPipe) Read(b []byte) (int, error)  { return p.in.Read(b) }
func (p *stdioPipe) Write(b []byte) (int, error) { return p.out.Write(b) }
func (p *stdioPipe) Close() error {
	_ = p.in.Close()
	return p.out.Close()
}
