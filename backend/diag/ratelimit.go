package diag

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type tokenBucket struct {
	ratePerSec float64
	burst      float64
	tokens     float64
	last       time.Time
	mu         sync.Mutex
}

func newBucket(ratePerSec, burst float64) *tokenBucket {
	return &tokenBucket{
		ratePerSec: ratePerSec,
		burst:      burst,
		tokens:     burst,
		last:       time.Now(),
	}
}

func (b *tokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * b.ratePerSec
	if b.tokens > b.burst {
		b.tokens = b.burst
	}
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}

var (
	rateMu     sync.Mutex
	globalBkt  = newBucket(200, 400)
	perTagBkts = map[string]*tokenBucket{}
	dropped    atomic.Int64
)

func allowRate(tag string) bool {
	if !globalBkt.Allow() {
		dropped.Add(1)
		return false
	}
	rateMu.Lock()
	tb, ok := perTagBkts[tag]
	if !ok {
		tb = newBucket(20, 40)
		perTagBkts[tag] = tb
	}
	rateMu.Unlock()
	if tb.Allow() {
		return true
	}
	dropped.Add(1)
	return false
}

// resetForTest resets the rate-limit state. Test-only.
func resetForTest(t *testing.T) {
	t.Helper()
	rateMu.Lock()
	perTagBkts = map[string]*tokenBucket{}
	rateMu.Unlock()
	dropped.Store(0)
	globalBkt = newBucket(200, 400)
}

func getDroppedForTest() int64 { return dropped.Load() }
