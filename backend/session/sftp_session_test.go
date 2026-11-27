package session

import (
"bytes"
"encoding/binary"
"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
"sync"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

// newTestSFTPSession serves a temp directory over an in-process SFTP server
// (pkg/sftp speaks over a plain io.ReadWriteCloser; no SSH involved) and
// returns a session wired to a connected client. Tests address files by
// absolute path inside the returned root.
//
// Note: pkg/sftp v1.13.10 only offers NewServer(rwc io.ReadWriteCloser, ...);
// there is no NewServerWithFS / listener-based constructor, so the server
// serves the real filesystem and the tests confine themselves to temp paths.
func newTestSFTPSession(t *testing.T) (*SFTPSession, string) {
	t.Helper()
	root := t.TempDir()
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lst.Close() })

	c1, err := net.DialTimeout("tcp", lst.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := lst.Accept()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := sftp.NewServer(c2)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	client, err := sftp.NewClientPipe(c1, c1)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close(); c1.Close(); c2.Close() })

	s := NewSFTPSession("test-sftp")
	s.sftpClient = client
	s.sshClient = nil // not needed for transfers
	return s, root
}

// captureTransferEvents installs a TransferEventSink recorder for the
// duration of the test and returns the event stream. Needed because done
// tasks are deleted from s.transfers, so the map alone cannot be polled for
// the outcome of a fast transfer.
func captureTransferEvents(t *testing.T) <-chan map[string]any {
	t.Helper()
	ch := make(chan map[string]any, 512)
	prev := TransferEventSink
	TransferEventSink = func(sessionID string, payload map[string]any) {
		select {
		case ch <- payload:
		default:
		}
	}
	t.Cleanup(func() { TransferEventSink = prev })
	return ch
}

// waitForTask drains the event stream until the given task emits its
// "complete" event, returning that payload.
func waitForTask(t *testing.T, events <-chan map[string]any, id string) map[string]any {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev["taskId"] == id && ev["event"] == "complete" {
				return ev
			}
		case <-deadline:
			t.Fatal("transfer did not finish in time")
			return nil
		}
	}
}

func TestDirDownloadPerFileTrackingAndRetrySkip(t *testing.T) {
	s, root := newTestSFTPSession(t)
	events := captureTransferEvents(t)
	remote := filepath.Join(root, "src")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(remote, "a.txt"), []byte("AAA"), 0o644)

	os.WriteFile(filepath.Join(remote, "b.txt"), []byte("BB"), 0o644)
	os.MkdirAll(filepath.Join(remote, "sub"), 0o755)
	os.WriteFile(filepath.Join(remote, "sub", "c.txt"), []byte("C"), 0o644)

	local := filepath.Join(root, "out")
	id, err := s.startDirTransfer("download", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitForTask(t, events, id)
	if ev["status"] != "done" {
		t.Fatalf("status = %v, failed=%v", ev["status"], ev["failedFiles"])
	}
	if ev["fileCount"] != 3 || ev["completedFiles"] != 3 {
		t.Fatalf("fileCount=%v completed=%v", ev["fileCount"], ev["completedFiles"])
	}
	b, _ := os.ReadFile(filepath.Join(local, "sub", "c.txt"))
	if string(b) != "C" {
		t.Fatalf("content = %q", b)
	}

	// Retry with skip: a.txt is now read-only locally, so a re-attempt to
	// create it would fail - a skipped file must be considered complete
	// without touching it at all.
	ro := filepath.Join(local, "a.txt")
	if err := os.Chmod(ro, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(ro, 0o644) })
	id2, err := s.startDirTransfer("download", local, remote, []string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	ev2 := waitForTask(t, events, id2)
	if ev2["status"] != "done" || ev2["completedFiles"] != 3 {
		t.Fatalf("retry-skip: status=%v completed=%v failed=%v",
			ev2["status"], ev2["completedFiles"], ev2["failedFiles"])
	}
}

func TestDirDownloadContinuesAfterPerFileFailure(t *testing.T) {
	s, root := newTestSFTPSession(t)
	events := captureTransferEvents(t)
	remote := filepath.Join(root, "src")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(remote, "a.txt"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(remote, "b.txt"), []byte("B"), 0o644)
	os.WriteFile(filepath.Join(remote, "c.txt"), []byte("C"), 0o644)

	local := filepath.Join(root, "out")
	os.MkdirAll(filepath.Join(local, "b.txt"), 0o755) // blocks b.txt's os.Create

	id, err := s.startDirTransfer("download", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitForTask(t, events, id)
	if ev["status"] != "error" {
		t.Fatalf("status = %v, want error", ev["status"])
	}
	failed, _ := ev["failedFiles"].([]FileFailure)
	if len(failed) != 1 || !strings.HasSuffix(failed[0].Path, "b.txt") {
		t.Fatalf("failedFiles = %v", ev["failedFiles"])
	}
	if ev["completedFiles"] != 2 {
		t.Fatalf("completed = %v, want 2 (failure must not abort)", ev["completedFiles"])
	}
	// The failed task must be RETAINED for retry.
	s.mu.RLock()
	_, kept := s.transfers[id]
	s.mu.RUnlock()
	if !kept {
		t.Fatal("failed task was deleted from transfers map")
	}
}
// waitTaskRemoved polls until the task id is gone from s.transfers (deletion
// happens right after the complete event, so a short deadline-bounded wait is
// deterministic).
func waitTaskRemoved(t *testing.T, s *SFTPSession, id string) {
	t.Helper()
	for i := 0; i < 500; i++ {
		s.mu.RLock()
		_, ok := s.transfers[id]
		s.mu.RUnlock()
		if !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("task was not removed from transfers map in time")
}

// TestDirDownloadCancelDuringPool cancels a download while its worker pool is
// running: the single concurrency slot is held so every worker blocks
// deterministically at semaphore acquire after the walk has set fileCount.
func TestDirDownloadCancelDuringPool(t *testing.T) {
	s, root := newTestSFTPSession(t)
	events := captureTransferEvents(t)
	remote := filepath.Join(root, "src")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	const n = 8
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(remote, fmt.Sprintf("f%d.txt", i)), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	local := filepath.Join(root, "out")

	s.SetMaxConcurrency(1)
	s.sem <- struct{}{} // hold the only slot: workers park at acquire

	id, err := s.startDirTransfer("download", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Deterministic gate: fileCount is set when the pool has started.
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.RLock()
		task := s.transfers[id]
		fc := task.fileCount()
		s.mu.RUnlock()
		if fc == n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("worker pool did not start in time")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := s.CancelTransfer(id); err != nil {
		t.Fatal(err)
	}

	ev := waitForTask(t, events, id)
	if ev["status"] != "cancelled" {
		t.Fatalf("status = %v, want cancelled", ev["status"])
	}
	waitTaskRemoved(t, s, id)
}

func TestDirUploadHappyPath(t *testing.T) {
	s, root := newTestSFTPSession(t)
	events := captureTransferEvents(t)
	local := filepath.Join(root, "in")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(local, "a.txt"), []byte("AAA"), 0o644)
	os.WriteFile(filepath.Join(local, "b.txt"), []byte("BB"), 0o644)
	os.MkdirAll(filepath.Join(local, "sub"), 0o755)
	os.WriteFile(filepath.Join(local, "sub", "c.txt"), []byte("C"), 0o644)

	remote := filepath.Join(root, "out")
	id, err := s.startDirTransfer("upload", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitForTask(t, events, id)
	if ev["status"] != "done" {
		t.Fatalf("status = %v, failed=%v", ev["status"], ev["failedFiles"])
	}
	if ev["fileCount"] != 3 || ev["completedFiles"] != 3 {
		t.Fatalf("fileCount=%v completed=%v", ev["fileCount"], ev["completedFiles"])
	}
	waitTaskRemoved(t, s, id) // done tasks are deleted

	readRemote := func(rel string) string {
		t.Helper()
		f, err := s.sftpClient.Open(path.Join(remote, rel))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if got := readRemote("a.txt"); got != "AAA" {
		t.Fatalf("a.txt = %q", got)
	}
	if got := readRemote("b.txt"); got != "BB" {
		t.Fatalf("b.txt = %q", got)
	}
	if got := readRemote("sub/c.txt"); got != "C" {
		t.Fatalf("sub/c.txt = %q", got)
	}
}

// TestRetryTransferSkipsCompletedFiles drives the retry path end to end: a
// failed download retains its task, a retry with a skip list counts finished
// files as done without re-transferring, and dismiss drops the retained task.
func TestRetryTransferSkipsCompletedFiles(t *testing.T) {
	s, root := newTestSFTPSession(t)
	events := captureTransferEvents(t)
	remote := filepath.Join(root, "src")
	os.MkdirAll(remote, 0o755)
	os.WriteFile(filepath.Join(remote, "a.txt"), []byte("A"), 0o644)
	os.WriteFile(filepath.Join(remote, "b.txt"), []byte("B"), 0o644)
	local := filepath.Join(root, "out")
	os.MkdirAll(filepath.Join(local, "b.txt"), 0o755) // b.txt fails

	id, _ := s.RetryTransfer(TransferSpec{Type: "download", LocalPath: local, RemotePath: remote, Recursive: true}, nil)
	ev := waitForTask(t, events, id)
	if ev["status"] != "error" || ev["completedFiles"] != 1 {
		t.Fatalf("first attempt: status=%v completed=%v", ev["status"], ev["completedFiles"])
	}
	// Fix the blocker and retry, skipping a.txt (already done).
	os.Remove(filepath.Join(local, "b.txt"))
	id2, err := s.RetryTransfer(TransferSpec{Type: "download", LocalPath: local, RemotePath: remote, Recursive: true},
		[]string{"a.txt"})
	if err != nil {
		t.Fatal(err)
	}
	ev2 := waitForTask(t, events, id2)
	if ev2["status"] != "done" || ev2["completedFiles"] != 2 {
		t.Fatalf("retry: status=%v completed=%v", ev2["status"], ev2["completedFiles"])
	}
	// Dismiss removes a retained failed task.
	if err := s.DismissTransfer(id); err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	s.mu.RLock()
	_, kept := s.transfers[id]
	s.mu.RUnlock()
	if kept {
		t.Fatal("dismissed task still present")
	}
}


// newDelayedSFTPSession is newTestSFTPSession with a transparent proxy between
// the SFTP client and server that delays every protocol frame by `delay` in
// each direction, simulating an RTT of 2×delay. Frames are forwarded via
// time.AfterFunc (asynchronously), so arbitrarily many requests may be in
// flight at once: the delay penalises stop-and-wait request loops but not
// pipelined ones. This is what makes request pipelining observable in a unit
// test: a stop-and-wait transfer of N chunks needs N round trips, a pipelined
// one needs about N/64.
func newDelayedSFTPSession(t *testing.T, delay time.Duration) (*SFTPSession, string) {
	t.Helper()
	root := t.TempDir()
	a1, a2 := net.Pipe() // a1: client end
	b1, b2 := net.Pipe() // b2: server end
	t.Cleanup(func() { a1.Close(); a2.Close(); b1.Close(); b2.Close() })

	srv, err := sftp.NewServer(b2)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	t.Cleanup(func() { srv.Close() })

	go forwardFrames(a2, b1, delay) // client -> server
	go forwardFrames(b1, a2, delay) // server -> client

	// Configure the client exactly as production does (sftpClientOptions),
	// otherwise ReadFrom silently falls back to sequential writes.
	client, err := sftp.NewClientPipe(a1, a1, sftpClientOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })

	s := NewSFTPSession("test-sftp")
	s.sftpClient = client
	s.sshClient = nil
	return s, root
}

// forwardFrames relays length-prefixed SFTP frames from src to dst, each after
// an asynchronous delay. The SFTP wire format is uint32 length + payload, so
// no parsing beyond the frame header is needed.
func forwardFrames(src io.Reader, dst io.Writer, delay time.Duration) {
	var hdr [4]byte
	for {
		if _, err := io.ReadFull(src, hdr[:]); err != nil {
			return
		}
		n := binary.BigEndian.Uint32(hdr[:])
		frame := make([]byte, 4+n)
		copy(frame, hdr[:])
		if _, err := io.ReadFull(src, frame[4:]); err != nil {
			return
		}
		time.AfterFunc(delay, func() { _, _ = dst.Write(frame) })
	}
}

// waitCompleteLong waits for the task's "complete" event with a deadline long
// enough to also accommodate a failed (non-pipelined) transfer, so the tests
// below fail via their latency assertion, not via a wait timeout.
func waitCompleteLong(t *testing.T, events <-chan map[string]any, id string) map[string]any {
	t.Helper()
	deadline := time.After(60 * time.Second)
	for {
		select {
		case ev := <-events:
			if ev["taskId"] == id && ev["event"] == "complete" {
				return ev
			}
		case <-deadline:
			t.Fatal("transfer did not finish in time")
			return nil
		}
	}
}

// pipelineProbeSize is 128 chunks of 32 KiB. At a simulated RTT of 80ms a
// stop-and-wait transfer takes ~10s (upload, one round trip per 32 KiB chunk)
// or ~5s (download, one per 64 KiB chunk); a pipelined one takes a couple of
// round trips. The tests fail above 2s, far below the stop-and-wait floor and
// far above the pipelined time.
const pipelineProbeSize = 4 << 20

func pipelineProbeData() []byte {
	data := make([]byte, pipelineProbeSize)
	for i := range data {
		data[i] = byte(i * 7)
	}
	return data
}

func assertPipelinedElapsed(t *testing.T, start time.Time) {
	t.Helper()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("transfer took %v: SFTP requests are not pipelined", elapsed)
	}
}

func TestUploadTransferPipelinesRequests(t *testing.T) {
	s, root := newDelayedSFTPSession(t, 40*time.Millisecond)
	events := captureTransferEvents(t)
	local := filepath.Join(root, "in")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(local, "big.bin"), pipelineProbeData(), 0o644); err != nil {
		t.Fatal(err)
	}
	remote := filepath.Join(root, "out")

	start := time.Now()
	id, err := s.startDirTransfer("upload", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitCompleteLong(t, events, id)
	if ev["status"] != "done" {
		t.Fatalf("status = %v, failed=%v", ev["status"], ev["failedFiles"])
	}
	assertPipelinedElapsed(t, start)

	got, err := os.ReadFile(filepath.Join(remote, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pipelineProbeData()) {
		t.Fatal("uploaded content mismatch")
	}
}

func TestDownloadTransferPipelinesRequests(t *testing.T) {
	s, root := newDelayedSFTPSession(t, 40*time.Millisecond)
	events := captureTransferEvents(t)
	remote := filepath.Join(root, "src")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "big.bin"), pipelineProbeData(), 0o644); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(root, "out")

	start := time.Now()
	id, err := s.startDirTransfer("download", local, remote, nil)
	if err != nil {
		t.Fatal(err)
	}
	ev := waitCompleteLong(t, events, id)
	if ev["status"] != "done" {
		t.Fatalf("status = %v, failed=%v", ev["status"], ev["failedFiles"])
	}
	assertPipelinedElapsed(t, start)

	got, err := os.ReadFile(filepath.Join(local, "big.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pipelineProbeData()) {
		t.Fatal("downloaded content mismatch")
	}
}


// TestPauseResumeEvents checks the dedicated paused/resumed event payloads.
func TestPauseResumeEvents(t *testing.T) {
	var events []string
	TransferEventSink = func(sid string, p map[string]any) {
		if sid != "s1" {
			t.Fatalf("sessionId = %q, want s1", sid)
		}
		events = append(events, p["event"].(string))
	}
	defer func() { TransferEventSink = nil }()

	s := &baseSession{id: "s1", sessionType: "sftp", status: StatusConnected}
	task := &TransferTask{ID: "t1", Type: "download", Status: "running"}
	task.start()
	defer task.done()

	task.Status = "paused"
	s.emitTransferPaused(task)
	task.Status = "running"
	s.emitTransferResumed(task)

	if len(events) != 2 || events[0] != "paused" || events[1] != "resumed" {
		t.Fatalf("events = %v, want [paused resumed]", events)
	}
}

// pauseResumer matches the Pause/Resume surface shared by the transfer
// backends under test.
type pauseResumer interface {
	PauseTransfer(taskID string) error
	ResumeTransfer(taskID string) error
}

// putTransferTask registers task in a session's transfers map so the
// Pause/Resume entry points can find it.
func putTransferTask(s any, task *TransferTask) {
	switch v := s.(type) {
	case *SFTPSession:
		v.mu.Lock()
		v.transfers[task.ID] = task
		v.mu.Unlock()
	case *SCPSession:
		v.mu.Lock()
		v.transfers[task.ID] = task
		v.mu.Unlock()
	default:
		panic(fmt.Sprintf("putTransferTask: unsupported session %T", s))
	}
}

// TestPauseResumeTransferLifecycle drives PauseTransfer/ResumeTransfer through
// both backends: pausing must emit the dedicated "paused" event (not a fake
// "complete"), resuming must emit "resumed", and resuming a task that is not
// paused (e.g. already done) must fail without emitting anything.
func TestPauseResumeTransferLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name string
		sess pauseResumer
	}{
		{"sftp", NewSFTPSession("s1")},
		{"scp", NewSCPSession("s2")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []string
			TransferEventSink = func(sid string, p map[string]any) {
				events = append(events, p["event"].(string))
			}
			defer func() { TransferEventSink = nil }()

			task := &TransferTask{ID: "t-live", Type: "download", Status: "running"}
			task.start()
			defer task.done()
			putTransferTask(tc.sess, task)

			// Pause: honest "paused" event, status flipped, no fake "complete".
			if err := tc.sess.PauseTransfer("t-live"); err != nil {
				t.Fatalf("PauseTransfer: %v", err)
			}
			if task.Status != "paused" {
				t.Fatalf("status after pause = %q, want paused", task.Status)
			}
			if len(events) != 1 || events[0] != "paused" {
				t.Fatalf("events after pause = %v, want [paused]", events)
			}

			// Resume: back to running with a "resumed" event.
			if err := tc.sess.ResumeTransfer("t-live"); err != nil {
				t.Fatalf("ResumeTransfer: %v", err)
			}
			if task.Status != "running" {
				t.Fatalf("status after resume = %q, want running", task.Status)
			}
			if len(events) != 2 || events[1] != "resumed" {
				t.Fatalf("events after resume = %v, want [..., resumed]", events)
			}

			// Resuming a finished task must be rejected and stay silent.
			task.Status = "done"
			events = nil
			if err := tc.sess.ResumeTransfer("t-live"); err == nil {
				t.Fatal("ResumeTransfer on done task: want error, got nil")
			}
			if task.Status != "done" {
				t.Fatalf("status after rejected resume = %q, want done", task.Status)
			}
			if len(events) != 0 {
				t.Fatalf("rejected resume emitted %v, want none", events)
			}
		})
	}
}


func TestTransferTaskFileState(t *testing.T) {
	task := &TransferTask{ID: "dl-1", Type: "download"}
	task.SetSkip([]string{"already/done.txt"})
	if !task.shouldSkip("already/done.txt") || task.shouldSkip("other.txt") {
		t.Fatal("skip set mismatch")
	}
	task.setFileCount(3)
	if task.fileCount() != 3 {
		t.Fatal("fileCount")
	}
	task.beginFile("a.txt")
	task.finishFile("a.txt")
	task.failFile("b.txt", errors.New("denied"))
	if task.completedCount() != 1 {
		t.Fatalf("completed = %d", task.completedCount())
	}
	if len(task.FailedFiles) != 1 || task.FailedFiles[0].Path != "b.txt" || task.FailedFiles[0].Error != "denied" {
		t.Fatalf("FailedFiles = %v", task.FailedFiles)
	}
}

func TestTransferTaskConcurrentProgress(t *testing.T) {
	task := &TransferTask{ID: "dl-2"}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				task.addProgress(1)
			}
		}()
	}
	wg.Wait()
	if task.loadProgress() != 8000 {
		t.Fatalf("progress = %d, want 8000", task.loadProgress())
	}
}
