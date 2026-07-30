package util

import (
	"context"
	"errors"
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
	got := []error{nil, nil}
	cfg := RetryConfig{
		Initial:        time.Millisecond,
		Max:            2 * time.Millisecond,
		JitterFraction: 0,
		OnReconnect: func(err error) {
			if got[0] == nil {
				got[0] = err
			} else {
				got[1] = err
			}
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go BackoffLoop(ctx, cfg, func() error {
		calls++
		if calls >= 2 {
			cancel()
		}
		return errors.New("retry me")
	})
	time.Sleep(20 * time.Millisecond)
	if got[0] == nil || got[0].Error() != "retry me" {
		t.Fatalf("OnReconnect not fired: %v", got)
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
