package container

import (
	"io"
	"testing"
	"time"
)

func TestPTYOutputCancellationWithoutAttachedReader(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	data := make(chan []byte, 32)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		readPTYOutput(reader, data, done)
	}()
	t.Cleanup(func() {
		_ = reader.Close()
		// Permit the old blocking-send implementation to exit after failure.
		for range data {
		}
	})
	written := make(chan error, 1)
	go func() {
		for i := 0; i <= cap(data); i++ {
			if _, err := writer.Write([]byte{byte(i)}); err != nil {
				written <- err
				return
			}
		}
		written <- nil
	}()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("PTY did not read the frame beyond its full output queue")
	}
	// io.Pipe.Write completes only after Read has consumed the extra frame.
	// No consumer has drained the full queue; cancellation must release its send.
	close(done)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("PTY output producer leaked after cancellation without a reader")
	}
	for i := 0; i < cap(data); i++ {
		if b := <-data; len(b) != 1 || b[0] != byte(i) {
			t.Fatalf("buffered pre-attachment output changed at %d: %v", i, b)
		}
	}
	if _, ok := <-data; ok {
		t.Fatal("cancelled producer left output open")
	}
}
