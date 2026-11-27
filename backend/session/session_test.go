package session

import (
"bytes"
	"net"
"sync"
	"testing"
	"time"
)

func TestEffectiveTermType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty falls back to default", "", DefaultTermType},
		{"valid passthrough xterm", "xterm", "xterm"},
		{"valid passthrough vt100", "vt100", "vt100"},
		{"valid passthrough vt320", "vt320", "vt320"},
		{"invalid value falls back to default", "foo-bar", DefaultTermType},
		{"injection attempt falls back to default", "xterm; rm -rf /", DefaultTermType},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EffectiveTermType(tc.in); got != tc.want {
				t.Fatalf("EffectiveTermType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExplicitTermType(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty means no explicit choice", "", ""},
		{"valid explicit value", "vt100", "vt100"},
		{"invalid value is ignored", "foo-bar", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ExplicitTermType(tc.in); got != tc.want {
				t.Fatalf("ExplicitTermType(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// ttypeReply runs handleSubNegotiation against a TTYPE SEND subnegotiation and
// returns the IS reply the session writes back.
func ttypeReply(t *testing.T, termType string) []byte {
	t.Helper()

	s := NewTelnetSession("test")
	s.termType = termType

	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	s.conn = client

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := []byte{telnetIAC, telnetSB, telnetOptTerminalType, telnetTTYPESend, telnetIAC, telnetSE}
		s.handleSubNegotiation(buf, 0)
	}()

	server.SetReadDeadline(time.Now().Add(2 * time.Second))
	reply := make([]byte, 64)
	n, err := server.Read(reply)
	if err != nil {
		t.Fatalf("read TTYPE reply: %v", err)
	}
	<-done
	return reply[:n]
}

func TestTelnetTTYPEUsesConfiguredTermType(t *testing.T) {
	reply := ttypeReply(t, "vt100")
	want := append([]byte{telnetIAC, telnetSB, telnetOptTerminalType, telnetTTYPEIs}, []byte("vt100")...)
	want = append(want, telnetIAC, telnetSE)
	if string(reply) != string(want) {
		t.Fatalf("TTYPE reply = % x, want % x", reply, want)
	}
}

func TestTelnetTTYPEDefaultTermType(t *testing.T) {
	reply := ttypeReply(t, DefaultTermType)
	want := append(append([]byte{telnetIAC, telnetSB, telnetOptTerminalType, telnetTTYPEIs}, DefaultTermType...), telnetIAC, telnetSE)
	if string(reply) != string(want) {
		t.Fatalf("TTYPE reply = % x, want default term %s", reply, DefaultTermType)
	}
}


func TestTapRemovalKeepsOtherObservers(t *testing.T) {
	s := &baseSession{}
	var data, status [3]int
	d0 := s.AddDataTap(func([]byte) { data[0]++ })
	d1 := s.AddDataTap(func([]byte) { data[1]++ })
	d2 := s.AddDataTap(func([]byte) { data[2]++ })
	s0 := s.AddStatusTap(func(SessionStatus) { status[0]++ })
	s1 := s.AddStatusTap(func(SessionStatus) { status[1]++ })
	s2 := s.AddStatusTap(func(SessionStatus) { status[2]++ })
	d0()
	d1()
	s0()
	s1()
	s.emitData([]byte("test"))
	s.setStatus(StatusConnected)
	if data != [3]int{0, 0, 1} || status != [3]int{0, 0, 1} {
		t.Fatalf("wrong observers removed: data=%v status=%v", data, status)
	}
	// Reuse slots, then invoke an old unsubscribe again.
	n := 0
	s.AddDataTap(func([]byte) { n++ })
	s.AddStatusTap(func(SessionStatus) { n++ })
	d0()
	s0()
	d2()
	s2()
	s.emitData([]byte("again"))
	s.setStatus(StatusConnected)
	if n != 2 || data[2] != 1 || status[2] != 1 {
		t.Fatalf("unsubscribe removed a reused slot: n=%d data=%v status=%v", n, data, status)
	}
}


// Tests for the async, bounded emit path (issue #1053): a flood of terminal
// output must never block the session's readLoop against the synchronous
// Wails event emit, and high-frequency chunks should be coalesced before they
// cross the IPC boundary.
//
// Overflow policy is two-tier: while the consumer is merely slow the producer
// blocks (flow control — a full-screen TUI only sends diffs, so dropped bytes
// would desync the screen), and only after a watchdog expires does it degrade
// to drop-oldest so a wedged frontend cannot freeze the readLoop.

func TestEmitDataSyncWhenLoopNotStarted(t *testing.T) {
	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnDataCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})

	s.emitData([]byte("hello"))

	mu.Lock()
	defer mu.Unlock()
	if string(got) != "hello" {
		t.Fatalf("emitData without emitLoop delivered %q synchronously, want %q", got, "hello")
	}
}

func TestEmitDataBatchesWhenLoopStarted(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 40 * time.Millisecond
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	calls := 0
	s.SetOnDataCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
		calls++
	})
	s.startEmitLoop()
	t.Cleanup(s.stopEmitLoop)

	const n = 50
	for i := 0; i < n; i++ {
		s.emitData([]byte("x"))
	}
	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != n {
		t.Fatalf("delivered %d bytes, want %d (no loss)", len(got), n)
	}
	if bytes.Count(got, []byte("x")) != n {
		t.Fatalf("delivered content %q is corrupted", got)
	}
	if calls >= n {
		t.Fatalf("callback ran %d times for %d chunks; batching did not happen", calls, n)
	}
}

func TestEmitDataDropsOldestWhenQueueFull(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 5 * time.Second
	t.Cleanup(func() { emitFlushInterval = oldInterval })
	oldWatchdog := emitBlockWatchdog
	emitBlockWatchdog = 100 * time.Millisecond
	t.Cleanup(func() { emitBlockWatchdog = oldWatchdog })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnDataCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.startEmitLoop()

	// Enqueue well past the queue capacity before the first flush window
	// opens: the oldest chunks must be dropped, keeping the newest.
	for i := 0; i < emitQueueCapacity+10; i++ {
		s.emitData([]byte{byte(i)})
	}
	// stopEmitLoop force-flushes whatever survived in the queue.
	s.stopEmitLoop()

	mu.Lock()
	defer mu.Unlock()
	want := make([]byte, 0, emitQueueCapacity)
	for i := 10; i < emitQueueCapacity+10; i++ {
		want = append(want, byte(i))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("delivered %v, want chunks %d..%d in order (drop-oldest)", got, 10, emitQueueCapacity+9)
	}
}

func TestEmitDataSyncDuringZmodem(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 5 * time.Second
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnDataCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.startEmitLoop()
	t.Cleanup(s.stopEmitLoop)
	s.SetZmodemMode(true)

	s.emitData([]byte("zmodem payload"))

	mu.Lock()
	defer mu.Unlock()
	if string(got) != "zmodem payload" {
		t.Fatalf("emitData during zmodem delivered %q synchronously, want %q", got, "zmodem payload")
	}
}

func TestStopEmitLoopFlushesPending(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 5 * time.Second
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnDataCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.startEmitLoop()

	s.emitData([]byte("tail"))
	s.stopEmitLoop()

	mu.Lock()
	if string(got) != "tail" {
		mu.Unlock()
		t.Fatalf("stopEmitLoop delivered %q, want pending %q flushed", got, "tail")
	}
	mu.Unlock()

	// After the loop stopped, emitData must fall back to the synchronous path.
	s.emitData([]byte("sync"))
	mu.Lock()
	if string(got) != "tailsync" {
		mu.Unlock()
		t.Fatalf("emitData after stop delivered %q, want %q", got, "tailsync")
	}
	mu.Unlock()
}

func TestEmitDataDoesNotBlockOnSlowConsumer(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 40 * time.Millisecond
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	blocked := make(chan struct{})
	release := make(chan struct{})
	signalOnce := &sync.Once{}
	s.SetOnDataCallback(func(data []byte) {
		signalOnce.Do(func() { close(blocked) })
		<-release
	})
	s.startEmitLoop()
	t.Cleanup(s.stopEmitLoop)

	s.emitData([]byte("first"))

	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatal("callback was never invoked")
	}

	done := make(chan struct{})
	go func() {
		s.emitData([]byte("second"))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("emitData blocked on a stuck consumer; readLoop would freeze")
	}
	close(release)
}

// Flow control: while the consumer is merely slow (but still draining), the
// producer must block instead of dropping bytes. A full-screen TUI such as
// menuconfig only sends diffs after its initial paint, so losing a chunk
// desyncs the screen permanently.
func TestEmitDataNoLossWhenConsumerSlow(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 5 * time.Millisecond
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnDataCallback(func(data []byte) {
		time.Sleep(10 * time.Millisecond) // simulate a busy frontend
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.startEmitLoop()
	t.Cleanup(s.stopEmitLoop)

	const n = emitQueueCapacity + 16
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			s.emitData([]byte{byte(i)})
		}
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("producer never finished; flow control wedged the readLoop")
	}
	// Let the emit loop drain the tail of the queue.
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(got) != n {
		t.Fatalf("delivered %d bytes, want %d (transient consumer lag must not lose bytes)", len(got), n)
	}
	for i := 0; i < n; i++ {
		if got[i] != byte(i) {
			t.Fatalf("byte %d corrupted: got %d", i, got[i])
		}
	}
	s.emitMu.Lock()
	dropped := s.emitDropped
	s.emitMu.Unlock()
	if dropped != 0 {
		t.Fatalf("dropped %d chunk(s); transient consumer lag must not lose bytes", dropped)
	}
}

// Watchdog: when the consumer is wedged (not draining at all), the producer
// must degrade to drop-oldest instead of blocking forever — the freeze
// protection for a wedged frontend stays intact.
func TestEmitDataWedgeFallsBackToDropOldest(t *testing.T) {
	oldWatchdog := emitBlockWatchdog
	emitBlockWatchdog = 100 * time.Millisecond
	t.Cleanup(func() { emitBlockWatchdog = oldWatchdog })
	oldInterval := emitFlushInterval
	emitFlushInterval = 5 * time.Second
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	release := make(chan struct{})
	s.SetOnDataCallback(func(data []byte) {
		<-release
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.startEmitLoop()
	t.Cleanup(s.stopEmitLoop)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < emitQueueCapacity+10; i++ {
			s.emitData([]byte{byte(i)})
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("producer never degraded to drop-oldest; readLoop would freeze")
	}

	// Read the drop counter before stopEmitLoop: its force-flush drains the
	// queue and resets the counter (after logging the overflow).
	s.emitMu.Lock()
	dropped := s.emitDropped
	s.emitMu.Unlock()
	if dropped != 10 {
		t.Fatalf("dropped %d chunks, want 10", dropped)
	}
	close(release)

	s.stopEmitLoop()

	mu.Lock()
	defer mu.Unlock()
	want := make([]byte, 0, emitQueueCapacity)
	for i := 10; i < emitQueueCapacity+10; i++ {
		want = append(want, byte(i))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("delivered %v, want chunks %d..%d (drop-oldest after watchdog)", got, 10, emitQueueCapacity+9)
	}
}

func TestEmitBinaryDeliversWithoutExplicitStart(t *testing.T) {
	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnBinaryCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})

	// Unlike the data emit loop, the binary coalescer starts itself on the
	// first chunk — a session that only ever emits binary (zmodem) never
	// needs a startEmitLoop call.
	s.emitBinary([]byte("abc"))

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("emitBinary self-started loop never delivered the chunk")
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if string(got) != "abc" {
		t.Fatalf("emitBinary delivered %q, want %q", got, "abc")
	}
}

func TestEmitBinaryBatchesChunksWithoutLoss(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 40 * time.Millisecond
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	calls := 0
	s.SetOnBinaryCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
		calls++
	})
	t.Cleanup(s.stopBinaryEmitLoop)

	// Concurrent producers, interleaved with the flush window, like a
	// readLoop racing the coalescer during a fast transfer.
	const producers = 4
	const perProducer = 200
	var wg sync.WaitGroup
	wg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(p int) {
			defer wg.Done()
			for i := 0; i < perProducer; i++ {
				s.emitBinary([]byte{byte(p), byte(i)})
			}
		}(p)
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(got)
		mu.Unlock()
		if n >= producers*perProducer*2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for delivery: got %d bytes, want %d", n, producers*perProducer*2)
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != producers*perProducer*2 {
		t.Fatalf("delivered %d bytes, want %d (no loss)", len(got), producers*perProducer*2)
	}
	seen := make(map[[2]byte]int)
	for i := 0; i < len(got); i += 2 {
		seen[[2]byte{got[i], got[i + 1]}]++
	}
	for p := 0; p < producers; p++ {
		for i := 0; i < perProducer; i++ {
			if seen[[2]byte{byte(p), byte(i)}] != 1 {
				t.Fatalf("chunk (producer=%d, seq=%d) seen %d times", p, i, seen[[2]byte{byte(p), byte(i)}])
			}
		}
	}
	if calls >= producers*perProducer {
		t.Fatalf("callback ran %d times for %d chunks; batching did not happen", calls, producers*perProducer)
	}
}

func TestEmitBinaryStopFlushesTail(t *testing.T) {
	oldInterval := emitFlushInterval
	emitFlushInterval = 30 * time.Second // window never fires on its own
	t.Cleanup(func() { emitFlushInterval = oldInterval })

	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnBinaryCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	t.Cleanup(s.stopBinaryEmitLoop)

	s.emitBinary([]byte("tail"))
	s.stopBinaryEmitLoop() // must flush the pending chunk despite the long window

	mu.Lock()
	defer mu.Unlock()
	if string(got) != "tail" {
		t.Fatalf("stop left %d bytes undelivered (got %q)", 4-len(got), got)
	}
}

func TestEmitBinaryPostStopFallsBackToSync(t *testing.T) {
	s := &baseSession{}
	var mu sync.Mutex
	var got []byte
	s.SetOnBinaryCallback(func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, data...)
	})
	s.emitBinary([]byte("a")) // starts the loop
	s.stopBinaryEmitLoop()
	s.emitBinary([]byte("b")) // after stop: synchronous fallback

	mu.Lock()
	defer mu.Unlock()
	if string(got) != "ab" {
		t.Fatalf("post-stop emitBinary delivered %q, want %q", got, "ab")
	}
}
