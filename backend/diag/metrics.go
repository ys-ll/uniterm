package diag

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type opCounters struct {
	count    atomic.Int64
	errCount atomic.Int64
	lastErr  atomic.Value // string
	samples  []time.Duration
	sampleIx int
	mu       sync.Mutex
}

var metricsMap sync.Map // map[string]*opCounters

func Record(name string, elapsed time.Duration, err error) {
	v, _ := metricsMap.LoadOrStore(name, &opCounters{samples: make([]time.Duration, 100)})
	oc := v.(*opCounters)
	oc.count.Add(1)
	if err != nil {
		oc.errCount.Add(1)
		oc.lastErr.Store(err.Error())
	}
	oc.mu.Lock()
	oc.samples[oc.sampleIx%len(oc.samples)] = elapsed
	oc.sampleIx++
	oc.mu.Unlock()
}

func Snapshot() Summary {
	s := Summary{Levels: map[Level]int64{}, Ops: []OpStat{}}
	metricsMap.Range(func(k, v any) bool {
		oc := v.(*opCounters)
		oc.mu.Lock()
		n := oc.sampleIx
		if n > len(oc.samples) {
			n = len(oc.samples)
		}
		samples := append([]time.Duration(nil), oc.samples[:n]...)
		oc.mu.Unlock()
		sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
		stat := OpStat{Name: k.(string), Count: oc.count.Load()}
		if le, ok := oc.lastErr.Load().(string); ok {
			stat.LastErr = le
		}
		if len(samples) > 0 {
			stat.P50Ms = ms(samples[len(samples)*50/100])
			stat.P95Ms = ms(samples[len(samples)*95/100])
			stat.P99Ms = ms(samples[len(samples)*99/100])
		}
		s.Ops = append(s.Ops, stat)
		return true
	})
	sort.Slice(s.Ops, func(i, j int) bool { return s.Ops[i].Name < s.Ops[j].Name })
	return s
}

func ms(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// FillLevels is called from write() so the snapshot reflects levels too.
func FillLevels(level Level) {
	key := "__levels__." + string(level)
	v, _ := metricsMap.LoadOrStore(key, &opCounters{samples: nil})
	oc := v.(*opCounters)
	oc.count.Add(1)
}

func resetMetricsForTest() {
	metricsMap = sync.Map{}
}
