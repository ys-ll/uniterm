package store

import "testing"

// TestNotesStoreSyncRoundTrip pins the sync exchange contract for
// notes.json, including the Version backfill ImportPlain applies.
func TestNotesStoreSyncRoundTrip(t *testing.T) {
	a := NewNotesStore(t.TempDir())
	seed := NotesData{
		Version: 1,
		Notes:   []Note{{ID: "n1", Title: "deploy", Category: "scripts", Content: "# deploy\n\n```bash\nssh prod-01\n```"}},
	}
	if err := a.Save(seed); err != nil {
		t.Fatalf("Save: %v", err)
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	b := NewNotesStore(t.TempDir())
	if err := b.ImportPlain(plain); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}
	got, err := b.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Notes) != 1 || got.Notes[0].Title != "deploy" || got.Notes[0].Content == "" {
		t.Fatalf("notes lost: %+v", got)
	}
}
