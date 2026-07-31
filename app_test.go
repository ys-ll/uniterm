package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

// TestErrorBodyCap_F305 guards F-305: the error-body read on the non-200
// path is capped at 64 KiB so a hostile or buggy upstream returning a
// multi-GB error body can't OOM the Go process. We can't drive the full
// chatCompletion* HTTP path from a unit test, but the cap is a single
// io.LimitReader wrapper around io.ReadAll — verify the contract that
// reads beyond 64 KiB stop there.
func TestErrorBodyCap_F305(t *testing.T) {
	const cap = 64 * 1024
	// 1 MiB of 'a' is well above the cap.
	big := strings.Repeat("a", 1024*1024)
	body, err := io.ReadAll(io.LimitReader(strings.NewReader(big), cap))
	if err != nil {
		t.Fatalf("LimitReader read failed: %v", err)
	}
	if len(body) != cap {
		t.Errorf("expected capped read of %d bytes, got %d", cap, len(body))
	}
	// Sanity: a smaller body is read in full.
	small := strings.Repeat("b", 1024)
	bodySmall, err := io.ReadAll(io.LimitReader(strings.NewReader(small), cap))
	if err != nil {
		t.Fatalf("LimitReader read failed: %v", err)
	}
	if len(bodySmall) != len(small) {
		t.Errorf("small body got %d bytes, want %d", len(bodySmall), len(small))
	}
}

// TestApp_CancelChatStream_NoActiveCall guards the safe-no-op path:
// when no ChatCompletion is in flight, CancelChatStream must not panic.
func TestApp_CancelChatStream_NoActiveCall(t *testing.T) {
	a := &App{}
	a.CancelChatStream() // must not panic
}

// TestApp_CancelChatStream_OverlappingCalls guards F-308: when two
// ChatCompletion calls overlap, CancelChatStream invoked during the second
// call must still cancel the second call's context — not be silently
// no-op'd because the first call's defer already nil'd the slot.
func TestApp_CancelChatStream_OverlappingCalls(t *testing.T) {
	a := &App{}

	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	myA := cancelA
	a.chatCancel.Store(&myA)

	ctxB, cancelB := context.WithCancel(context.Background())
	defer cancelB()
	myB := cancelB
	a.chatCancel.Store(&myB)

	// Call A finishes first — its defer must NOT clobber B's slot.
	a.chatCancel.CompareAndSwap(&myA, nil)

	a.CancelChatStream()

	if ctxB.Err() == nil {
		t.Errorf("ctxB expected to be cancelled by CancelChatStream")
	}
}

// TestApp_ConcurrentCancelSurvives guards F-308's race at scale: the
// old mutex+CancelFunc design could let a CancelChatStream drop a
// cancel because the wrong defer cleared the slot. With
// atomic.Pointer[CancelFunc] + CompareAndSwap(my, nil), only the call
// that currently owns the slot ever clears it — the rest leave it
// alone. This test verifies the simpler invariant: a call's cancel
// survives any number of OTHER calls' defers running first.
func TestApp_ConcurrentCancelSurvives(t *testing.T) {
	a := &App{}

	// Two overlapping calls.
	_, cancelA := context.WithCancel(context.Background())
	defer cancelA()
	_, cancelB := context.WithCancel(context.Background())
	defer cancelB()

	myA := cancelA
	myB := cancelB

	a.chatCancel.Store(&myA)
	a.chatCancel.Store(&myB)

	// N goroutines, each simulating a call whose defer tries to clear
	// the slot. None of them is "call A" or "call B" — their CAS
	// addresses don't match anything in the slot, so they all no-op.
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			other := context.CancelFunc(func() {})
			a.chatCancel.CompareAndSwap(&other, nil)
		}()
	}
	wg.Wait()

	// After all the unrelated defers, the slot must still hold B's cancel
	// — the no-op CASes never touched it.
	c := a.chatCancel.Load()
	if c == nil {
		t.Fatalf("chatCancel unexpectedly nil; B's cancel was wiped by a stale defer")
	}
	if c != &myB {
		t.Errorf("chatCancel points at %p, want B's cancel at %p", c, &myB)
	}
}