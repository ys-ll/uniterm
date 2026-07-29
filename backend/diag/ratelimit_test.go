package diag

import (
	"testing"
	"time"
)

func TestTokenBucketBurstThenLimit(t *testing.T) {
	tb := newBucket(10, 5) // 10/s, burst 5
	for i := 0; i < 5; i++ {
		if !tb.Allow() {
			t.Fatalf("burst slot %d denied", i)
		}
	}
	if tb.Allow() {
		t.Fatal("6th in burst window should be denied")
	}
	time.Sleep(120 * time.Millisecond) // refill ~1 token
	if !tb.Allow() {
		t.Fatal("after refill, should allow")
	}
}

func TestGlobalRateLimitDrops(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	if err := Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1000; i++ {
		Info("flood.tag", "x", nil)
	}
	Close()
	// We don't assert exact count (rate limiter is approximate) — just that
	// some lines were dropped, evidenced by the dropped counter > 0.
	if getDroppedForTest() == 0 {
		t.Fatal("expected some lines to be rate-limited")
	}
}
