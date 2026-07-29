package diag

import "testing"

func TestQueryFilters(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	Error("ssh.connect", "boom1", nil)
	Info("other", "ok", nil)
	Error("ssh.connect", "boom2", nil)
	Close()

	got, err := Query(QueryOpts{Tag: "ssh.connect", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 ssh.connect, got %d", len(got))
	}
	for _, e := range got {
		if e.Tag != "ssh.connect" {
			t.Fatal("filter leak")
		}
	}
}