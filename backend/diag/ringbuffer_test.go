package diag

import (
	"testing"
	"time"
)

func TestRingBufferMergesRepeated(t *testing.T) {
	rb := newRingBuffer(50 * time.Millisecond)
	for i := 0; i < 5; i++ {
		n, merged := rb.Merge(LevelInfo, "tag", "msg", map[string]any{"k": "v"})
		if merged && i == 0 {
			t.Fatalf("first call should NOT merge: n=%d merged=%v", n, merged)
		}
		if merged && n < 2 {
			t.Fatalf("after second call merged count must be >=2: got %d", n)
		}
	}
	time.Sleep(80 * time.Millisecond)
	n, merged := rb.Merge(LevelInfo, "tag", "msg", map[string]any{"k": "v"})
	if merged {
		t.Fatalf("after window expired, must emit fresh entry: got merged=%v n=%d", merged, n)
	}
	if n != 1 {
		t.Fatalf("fresh entry count must be 1, got %d", n)
	}
}
