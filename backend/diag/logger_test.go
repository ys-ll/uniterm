package diag

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitInfoCloseRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	Info("test.tag", "hello", map[string]any{"k": "v"})
	Close()

	f, err := os.Open(filepath.Join(dir, "uniterm.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		t.Fatal("no line written")
	}
	var got map[string]any
	if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
		t.Fatalf("bad json: %v", err)
	}
	if got["level"] != "INFO" || got["tag"] != "test.tag" || got["msg"] != "hello" {
		t.Fatalf("unexpected entry: %+v", got)
	}
	fields, _ := got["fields"].(map[string]any)
	if fields["k"] != "v" {
		t.Fatalf("missing field: %+v", fields)
	}
	if got["dedup_count"] != float64(1) {
		t.Fatalf("dedup_count wrong")
	}
}
