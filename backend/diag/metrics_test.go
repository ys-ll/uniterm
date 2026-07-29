package diag

import (
	"errors"
	"testing"
	"time"
)

func TestRecordAndSnapshot(t *testing.T) {
	resetMetricsForTest()
	for i := 0; i < 100; i++ {
		Record("op.test", time.Duration(i)*time.Millisecond, nil)
	}
	Record("op.err", 5*time.Millisecond, errors.New("boom"))
	s := Snapshot()
	var found *OpStat
	for i := range s.Ops {
		if s.Ops[i].Name == "op.test" {
			found = &s.Ops[i]
		}
	}
	if found == nil || found.Count != 100 {
		t.Fatalf("op.test count wrong: %+v", found)
	}
	if found.P50Ms <= 0 || found.P95Ms <= 0 {
		t.Fatalf("percentiles missing: %+v", found)
	}
}
