package store

import (
	"errors"
	"sync"
	"testing"
)

func TestAIConversationsIndependentAndStaleWrites(t *testing.T) {
	s, err := NewAISessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := AISessionEntry{ID: "client-a", Messages: []AIMessageEntry{{ID: "a", Content: "alpha"}}}
	b := AISessionEntry{ID: "client-b", Messages: []AIMessageEntry{{ID: "b", Content: "beta"}}}
	var wg sync.WaitGroup
	for _, entry := range []AISessionEntry{a, b} {
		wg.Add(1)
		go func(entry AISessionEntry) {
			defer wg.Done()
			if _, err := s.Upsert(entry, ""); err != nil {
				t.Error(err)
			}
		}(entry)
	}
	wg.Wait()
	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sessions) != 2 {
		t.Fatalf("independent save lost history: %+v", loaded)
	}
	for _, entry := range loaded.Sessions {
		if entry.ID == a.ID {
			a = entry
		} else {
			b = entry
		}
	}
	stale := a
	a.Name = "newer"
	next, err := s.Upsert(a, a.Revision)
	if err != nil {
		t.Fatal(err)
	}
	stale.Name = "stale"
	if _, err := s.Upsert(stale, stale.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale upsert: %v", err)
	}
	if err := s.Delete(a.ID, stale.Revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err := s.Delete(a.ID, next); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Upsert(a, next); !errors.Is(err, ErrConflict) {
		t.Fatalf("deleted history resurrected: %v", err)
	}
	loaded, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Sessions) != 1 || loaded.Sessions[0].ID != b.ID || loaded.Sessions[0].Messages[0].Content != "beta" {
		t.Fatalf("delete touched another client: %+v", loaded)
	}
}
