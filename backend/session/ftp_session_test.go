package session

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// setFTPCodec configures s with the encoding a Connect(config) would pick.
func setFTPCodec(s *FTPSession, name string) {
	if name == "" || name == "utf-8" {
		name = ""
	}
	s.encName = name
	s.enc = encodingByName(name)
}

// --- Name decoding (garbled Chinese directory names) ---

func TestFTPDecodeNameUTF8Fallback(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	// Server sends GBK bytes for "中文" — invalid UTF-8, must be decoded.
	if got := s.ftpDecodeName("\xd6\xd0\xce\xc4"); got != "中文" {
		t.Fatalf("GBK fallback decode: got %q, want 中文", got)
	}
	// Valid UTF-8 names must pass through untouched.
	if got := s.ftpDecodeName("日本語.txt"); got != "日本語.txt" {
		t.Fatalf("valid UTF-8 passthrough: got %q", got)
	}
}

func TestFTPDecodeNameExplicitGBK(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "gbk")

	if got := s.ftpDecodeName("\xd6\xd0\xce\xc4"); got != "中文" {
		t.Fatalf("GBK decode: got %q, want 中文", got)
	}
}

func TestFTPDecodeNameShiftJIS(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "shift-jis")

	if got := s.ftpDecodeName("\x93\xfa\x96{"); got != "日本" {
		t.Fatalf("Shift-JIS decode: got %q, want 日本", got)
	}
}

func TestFTPDecodeNameLatin1(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "latin-1")

	if got := s.ftpDecodeName("\xe9"); got != "é" {
		t.Fatalf("latin-1 decode: got %q, want é", got)
	}
}

// --- Path encoding (outgoing Retr/Stor/etc. must speak the server charset) ---

func TestFTPEncodePathGBK(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "gbk")

	want := "/a/\xd6\xd0\xce\xc4/x.txt"
	if got := s.ftpEncodePath("/a/中文/x.txt"); got != want {
		t.Fatalf("GBK encode: got %q, want %q", got, want)
	}
}

func TestFTPEncodePathPassthroughUTF8(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	in := "/a/中文/x.txt"
	if got := s.ftpEncodePath(in); got != in {
		t.Fatalf("UTF-8 passthrough: got %q, want %q", got, in)
	}
}

// A name recovered via the GBK fallback must round-trip: operations on it
// go out with the exact raw bytes the server originally sent.
func TestFTPEncodePathRestoresRawBytes(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	raw := "\xd6\xd0\xce\xc4"
	dec := s.ftpDecodeName(raw)
	if dec != "中文" {
		t.Fatalf("decode prerequisite failed: got %q", dec)
	}
	want := "/d/" + raw
	if got := s.ftpEncodePath("/d/" + dec); got != want {
		t.Fatalf("raw-byte restore: got %q, want %q", got, want)
	}
}

// --- Transfer cancellation (X button has no effect) ---

type blockingReader struct{ ch chan struct{} }

func (b *blockingReader) Read([]byte) (int, error) {
	<-b.ch
	return 0, errors.New("unreachable")
}

func TestFTPProgressReaderCancel(t *testing.T) {
	s := NewFTPSession("ftp-test")
	task := &TransferTask{ID: "t1"}
	task.start()
	defer task.done()
	task.cancel() // cancel before the first read

	pr := &progressReader{r: &blockingReader{ch: make(chan struct{})}, task: task, s: s}
	errCh := make(chan error, 1)
	go func() {
		_, err := pr.Read(make([]byte, 128))
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Read after cancel: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read blocked after cancellation")
	}
}

// The idle deadline must be re-armed before every read so a silent server
// cannot park the data-socket read forever.
type deadlineRecorder struct {
	calls []time.Time
}

func (d *deadlineRecorder) SetDeadline(t time.Time) error {
	d.calls = append(d.calls, t)
	return nil
}

func TestFTPProgressReaderIdleDeadline(t *testing.T) {
	s := NewFTPSession("ftp-test")
	task := &TransferTask{ID: "t2"}
	task.start()
	defer task.done()

	rec := &deadlineRecorder{}
	r := &sliceReader{data: []byte("ab"), errs: []error{nil, io.EOF}}
	pr := &progressReader{r: r, task: task, s: s, deadline: rec}

	buf := make([]byte, 8)
	for {
		if _, err := pr.Read(buf); err != nil {
			break
		}
	}
	if len(rec.calls) != 2 {
		t.Fatalf("SetDeadline calls: got %d, want 2", len(rec.calls))
	}
	for _, call := range rec.calls {
		delay := time.Until(call)
		if delay < ftpDataIdleTimeout-time.Second || delay > ftpDataIdleTimeout+time.Second {
			t.Fatalf("deadline delay %v, want ~%v", delay, ftpDataIdleTimeout)
		}
	}
}

type sliceReader struct {
	data []byte
	errs []error
	i    int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.errs) {
		return 0, io.EOF
	}
	e := r.errs[r.i]
	r.i++
	if e != nil {
		return 0, e
	}
	n := copy(p, r.data)
	return n, nil
}
