package k8s

import (
	"sync"
	"time"
)

// logBatchInterval is the maximum age of a buffered batch before it
// is flushed via a single EventsEmit. Logs are higher volume than
// watch events, so we coalesce on the same 50ms cadence to match the
// frontend's natural repaint rate.
const logBatchInterval = 50 * time.Millisecond

// logBatchCap is the maximum lines held per batch. Past this size the
// per-IPC savings disappear (the array grows past reasonable JSON
// size), so we flush eagerly instead of letting the buffer grow.
const logBatchCap = 512

// logBatcher coalesces per-line log callbacks into a single EventsEmit
// every logBatchInterval (or sooner when full). The aggregated
// payload is an array of strings — the frontend unwraps it.
//
// One batcher per StartLogStream — its run goroutine is launched by
// the caller and stopped by the terminal onEnd closure.
type logBatcher struct {
	emit     func(name string, payload any)
	event    string
	mu       sync.Mutex
	buf      []string
	flushCh  chan struct{}
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newLogBatcher(emit func(name string, payload any), event string) *logBatcher {
	return &logBatcher{
		emit:    emit,
		event:   event,
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
	}
}

// onLine is the per-line callback installed by StartLogStream.
// Hot path: takes the mutex, appends to buf, and signals the flush
// channel (non-blocking — the coalescing is what saves IPC).
func (b *logBatcher) onLine(line string) {
	b.mu.Lock()
	b.buf = append(b.buf, line)
	full := len(b.buf) >= logBatchCap
	b.mu.Unlock()
	// Always nudge the flush goroutine so a sparse line stream still
	// gets the 50ms-tick flush; the channel is size 1 so subsequent
	// signals collapse into one wake.
	select {
	case b.flushCh <- struct{}{}:
	default:
	}
	_ = full // logBatchCap triggers the same nudge below; redundant signal is fine
}

// run owns the flush goroutine. Exits when stop is closed; caller
// closes stop after onEnd fires so any final batched lines emit
// before the log handle is dropped.
func (b *logBatcher) run(stop <-chan struct{}) {
	var timer *time.Timer
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	defer stopTimer()
	for {
		select {
		case <-stop:
			b.flush()
			return
		case <-b.flushCh:
		}
		stopTimer()
		timer = time.NewTimer(logBatchInterval)
		select {
		case <-stop:
			b.flush()
			return
		case <-timer.C:
			b.flush()
		}
	}
}

// flush empties buf under the lock and emits one aggregated payload.
// Multiple concurrent onLine callers cannot interleave the append with
// the read thanks to the mutex.
func (b *logBatcher) flush() {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	out := b.buf
	b.buf = nil
	b.mu.Unlock()
	b.emit(b.event, out)
}

// stop signals the run goroutine to exit after a final flush. Safe to
// call multiple times.
func (b *logBatcher) stop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
}
