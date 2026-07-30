package util

import (
	"context"
	"math/rand"
	"time"
)

// RetryConfig controls exponential backoff between successive attempts.
//
// Initial is the wait before the second attempt (after the first failure).
// Each subsequent failure doubles the wait, capped at Max. JitterFraction
// in [0,1] adds proportional randomness so synchronized callers do not
// stampede a recovering backend.
type RetryConfig struct {
	Initial        time.Duration
	Max            time.Duration
	JitterFraction float64
	OnReconnect    func(error) // optional, called with the last error before each wait
}

// DefaultBackoff is the shared profile used by apiserver-facing reconnect
// loops: 1s → 2s → 4s → … capped at 30s, 25% jitter.
func DefaultBackoff() RetryConfig {
	return RetryConfig{
		Initial:        time.Second,
		Max:            30 * time.Second,
		JitterFraction: 0.25,
	}
}

// BackoffLoop runs fn in a loop, sleeping with exponential backoff after
// each non-nil error. It returns when:
//   - fn returns nil (caller decides whether to retry — usually stop)
//   - ctx is cancelled (returns ctx.Err())
//
// Between attempts, OnReconnect(err) (if non-nil) is fired so the caller
// can surface a transient reconnect signal without terminating the loop.
// Used by k8s watch.go and k8s logs.go.
func BackoffLoop(ctx context.Context, cfg RetryConfig, fn func() error) error {
	if cfg.Initial <= 0 {
		cfg.Initial = time.Second
	}
	if cfg.Max <= 0 {
		cfg.Max = 30 * time.Second
	}
	wait := cfg.Initial
	for {
		err := fn()
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if cfg.OnReconnect != nil {
			cfg.OnReconnect(err)
		}
		sleep := jitter(wait, cfg.JitterFraction)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(sleep):
		}
		wait *= 2
		if wait > cfg.Max {
			wait = cfg.Max
		}
	}
}

// BackoffWait returns the next sleep duration given the current value and
// the configured max. Exposed so callers that need to inspect the schedule
// (mostly tests) can mirror BackoffLoop's growth rule.
func BackoffWait(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		next = max
	}
	return next
}

func jitter(d time.Duration, frac float64) time.Duration {
	if frac <= 0 {
		return d
	}
	// Symmetric jitter: d ± d*frac/2. Avoid going below 0.
	delta := float64(d) * frac
	half := delta / 2
	return d - time.Duration(half) + time.Duration(rand.Float64()*delta)
}
