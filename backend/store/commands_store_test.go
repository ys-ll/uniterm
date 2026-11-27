package store

import (
	"reflect"
	"testing"
)



// The commands.json sync blob carries prefs + full .md bodies: a round trip
// through ExportPlain/ImportPlain must reproduce every command (and its
// preference layer) on a fresh store.
func TestCommandsStoreSyncRoundTrip(t *testing.T) {
	a := NewCommandsStore(t.TempDir())
	if err := a.CreateCommand("alpha", "first", "", "alpha body"); err != nil {
		t.Fatal(err)
	}
	if err := a.CreateCommand("beta", "second", "hint", "beta body"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSortOrder("beta", 5); err != nil {
		t.Fatal(err)
	}

	blob, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}

	b := NewCommandsStore(t.TempDir())
	if err := b.ImportPlain(blob); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}

	metas, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 2 {
		t.Fatalf("got %d commands, want 2", len(metas))
	}
	byName := map[string]CommandMeta{}
	for _, m := range metas {
		byName[m.Name] = m
	}
	if got := byName["alpha"]; got.Description != "first" || got.ArgumentHint != "" || got.SortOrder != 0 || got.Origin != "created" {
		t.Fatalf("alpha meta = %+v, want description=first sortOrder=0 origin=created", got)
	}
	if got := byName["beta"]; got.Description != "second" || got.ArgumentHint != "hint" || got.SortOrder != 5 {
		t.Fatalf("beta meta = %+v, want description=second argumentHint=hint sortOrder=5", got)
	}
	body, err := b.GetBody("beta")
	if err != nil {
		t.Fatal(err)
	}
	if body != "beta body" {
		t.Fatalf("beta body = %q, want %q", body, "beta body")
	}
}

// Deletion does not propagate: a local command absent from the pulled blob
// survives the import (same guarantee as skills sync).
func TestCommandsStoreSyncImportKeepsLocalOnly(t *testing.T) {
	a := NewCommandsStore(t.TempDir())
	if err := a.CreateCommand("shared", "from peer", "", "peer body"); err != nil {
		t.Fatal(err)
	}
	blob, err := a.ExportPlain()
	if err != nil {
		t.Fatal(err)
	}

	b := NewCommandsStore(t.TempDir())
	if err := b.CreateCommand("shared", "local", "", "local body"); err != nil {
		t.Fatal(err)
	}
	if err := b.CreateCommand("local-only", "only here", "", "local body"); err != nil {
		t.Fatal(err)
	}
	if err := b.ImportPlain(blob); err != nil {
		t.Fatal(err)
	}

	metas, err := b.List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, m := range metas {
		names = append(names, m.Name)
	}
	if !reflect.DeepEqual(names, []string{"shared", "local-only"}) {
		t.Fatalf("names = %v, want [shared local-only] (local-only survives, shared overwritten)", names)
	}
	body, err := b.GetBody("shared")
	if err != nil {
		t.Fatal(err)
	}
	if body != "peer body" {
		t.Fatalf("shared body = %q, want the pulled %q", body, "peer body")
	}
}
