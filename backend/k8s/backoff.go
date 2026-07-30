package k8s

import (
	"context"
	"time"
)

// backoff drives exponential sleep between reconnect attempts.
// Used by runWatchLoop / runLogLoop when the apiserver bounces (K8S-02).
// Package-private — there is no need to expose this beyond the k8s
// reconnect sites.
type backoff struct {
	cur time.Duration
	max time.Duration
}

func newBackoff(initial, max time.Duration) *backoff {
	return &backoff{cur: initial, max: max}
}

// next returns the current duration and doubles it for the following
// call, capped at max.
func (b *backoff) next() time.Duration {
	d := b.cur
	if b.cur < b.max {
		b.cur *= 2
		if b.cur > b.max {
			b.cur = b.max
		}
	}
	return d
}

// sleep blocks for the current duration or until ctx is cancelled.
// Returns ctx.Err() if cancelled mid-wait, nil otherwise.
func (b *backoff) sleep(ctx context.Context) error {
	t := time.NewTimer(b.next())
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
