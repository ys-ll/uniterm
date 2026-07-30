package util

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestBackoffLoop_StopsOnSuccess(t *testing.T) {
	calls := 0
	err := BackoffLoop(context.Background(), RetryConfig{Initial: time.Millisecond, Max: time.Millisecond}, func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil err, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestBackoffLoop_RetriesUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- BackoffLoop(ctx, RetryConfig{Initial: time.Millisecond, Max: 4 * time.Millisecond}, func() error {
			calls++
			if calls >= 3 {
				cancel()
			}
			return errors.New("boom")
		})
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("BackoffLoop did not return after cancel")
	}
	if calls < 3 {
		t.Fatalf("expected at least 3 calls, got %d", calls)
	}
}

func TestBackoffLoop_FiresOnReconnect(t *testing.T) {
	// ROOT CAUSE: the original test wrote to a shared slice from the
	// OnReconnect callback (running on the BackoffLoop goroutine) and
	// read it from the test goroutine after time.Sleep, with no
	// synchronization. Replace the slice+time.Sleep with a buffered
	// channel handoff so the main goroutine observes a deterministic
	// happens-before; also wait for BackoffLoop to return so we don't
	// leak the goroutine.
	reconnect := make(chan error, 1)
	cfg := RetryConfig{
		Initial:        time.Millisecond,
		Max:            2 * time.Millisecond,
		JitterFraction: 0,
		OnReconnect: func(err error) {
			select {
			case reconnect <- err:
			default:
			}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callsMu sync.Mutex
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- BackoffLoop(ctx, cfg, func() error {
			callsMu.Lock()
			calls++
			n := calls
			callsMu.Unlock()
			if n >= 2 {
				cancel()
			}
			return errors.New("retry me")
		})
	}()

	select {
	case err := <-reconnect:
		if err == nil || err.Error() != "retry me" {
			t.Fatalf("OnReconnect fired with unexpected err: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("OnReconnect did not fire")
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("BackoffLoop did not return after cancel")
	}
}

func TestBackoffWait(t *testing.T) {
	if got := BackoffWait(time.Second, 4*time.Second); got != 2*time.Second {
		t.Fatalf("expected 2s, got %v", got)
	}
	if got := BackoffWait(4*time.Second, 4*time.Second); got != 4*time.Second {
		t.Fatalf("expected capped 4s, got %v", got)
	}
	if got := BackoffWait(8*time.Second, 4*time.Second); got != 4*time.Second {
		t.Fatalf("expected cap 4s, got %v", got)
	}
}
