package k8s

import (
	"sync"
	"time"
)

// watchBatchInterval is the maximum age of a buffered batch before it
// is flushed via a single EventsEmit. Keeps an idle batch from sitting
// forever if a quiet period follows an event burst. 50 ms matches the
// natural human-perception flicker threshold so a busy view still feels
// live.
const watchBatchInterval = 50 * time.Millisecond

// watchBatchCap is the maximum events held per batch. Past this size the
// per-IPC savings disappear (the array grows past reasonable JSON size),
// so we flush eagerly instead of letting the buffer grow.
const watchBatchCap = 256

// watchBatcher coalesces WatchEvent callbacks into a single EventsEmit
// every watchBatchInterval (or sooner when full). The aggregated
// payload is an array — the frontend unwraps it.
//
// One batcher per StartWatch — its run goroutine is launched by
// StartWatch and stopped by the terminal onEnd closure.
type watchBatcher struct {
	emit     func(name string, payload any)
	event    string
	mu       sync.Mutex
	buf      []WatchEvent
	flushCh  chan struct{}
	stopCh   chan struct{}
	stopOnce sync.Once
}

func newWatchBatcher(emit func(name string, payload any), event string) *watchBatcher {
	return &watchBatcher{
		emit:    emit,
		event:   event,
		flushCh: make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
	}
}

// onEvent is the per-WatchEvent callback installed by StartWatch.
// Hot path: takes the mutex, appends to buf, and nudges the flush
// channel (non-blocking — the coalescing is what saves IPC). Even
// single-event flows need a timer-tick flush so a sparse event stream
// doesn't sit idle until the next event arrives.
func (b *watchBatcher) onEvent(ev WatchEvent) {
	b.mu.Lock()
	b.buf = append(b.buf, ev)
	b.mu.Unlock()
	select {
	case b.flushCh <- struct{}{}:
	default:
	}
}

// run owns the flush goroutine. Exits when stop is closed; StartWatch
// closes stop after onEnd fires so any final batched events are emitted
// before the watch handle is dropped.
func (b *watchBatcher) run(stop <-chan struct{}) {
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
		timer = time.NewTimer(watchBatchInterval)
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
// Multiple concurrent onEvent callers cannot interleave the append
// with the read thanks to the mutex.
func (b *watchBatcher) flush() {
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
func (b *watchBatcher) stop() {
	b.stopOnce.Do(func() { close(b.stopCh) })
}
