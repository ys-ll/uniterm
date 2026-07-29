package diag

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"sort"
	"sync"
	"time"
)

type pending struct {
	count int
	first time.Time
}

type ringBuffer struct {
	mu      sync.Mutex
	window  time.Duration
	entries map[string]*pending
	maxSize int
}

func newRingBuffer(window time.Duration) *ringBuffer {
	return &ringBuffer{
		window:  window,
		entries: map[string]*pending{},
		maxSize: 4096,
	}
}

func (rb *ringBuffer) Merge(level Level, tag, msg string, fields map[string]any) (int, bool) {
	key := fingerprint(level, tag, msg, fields)
	rb.mu.Lock()
	defer rb.mu.Unlock()
	now := time.Now()
	if p, ok := rb.entries[key]; ok && now.Sub(p.first) <= rb.window {
		p.count++
		return p.count, true
	}
	if len(rb.entries) >= rb.maxSize {
		var oldestKey string
		var oldest time.Time
		for k, v := range rb.entries {
			if oldestKey == "" || v.first.Before(oldest) {
				oldestKey = k
				oldest = v.first
			}
		}
		delete(rb.entries, oldestKey)
	}
	rb.entries[key] = &pending{count: 1, first: now}
	return 1, false
}

func fingerprint(level Level, tag, msg string, fields map[string]any) string {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf []byte
	buf = append(buf, string(level)...)
	buf = append(buf, '|')
	buf = append(buf, tag...)
	buf = append(buf, '|')
	buf = append(buf, msg...)
	buf = append(buf, '|')
	for _, k := range keys {
		b, _ := json.Marshal(fields[k])
		buf = append(buf, k...)
		buf = append(buf, '=')
		buf = append(buf, b...)
		buf = append(buf, ',')
	}
	h := sha1.Sum(buf)
	return hex.EncodeToString(h[:])
}
