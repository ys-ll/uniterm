package main

import (
	"reflect"
	"testing"
)

// Regression: ai:block_start payloads must be snapshots. The SSE parsers
// mutate the block map after emitting block_start (streamed deltas are
// accumulated into it), while Wails serializes the emitted payload on a
// separate goroutine. Handing the live map to emit crashed the app with
// "fatal error: concurrent map iteration and map write".
func TestSnapshotJSONMapIsolatesMutations(t *testing.T) {
	block := map[string]interface{}{
		"type":  "text",
		"text":  "",
		"input": map[string]interface{}{"a": []interface{}{float64(1), "x"}},
	}
	snap := snapshotJSONMap(block)

	// Mutate the original deeply, the way the parsers do after emitting.
	block["text"] = "streamed text"
	block["input"].(map[string]interface{})["a"] = "replaced"
	block["extra"] = map[string]interface{}{"k": "v"}

	if snap["text"] != "" {
		t.Fatalf("snapshot text mutated: %v", snap["text"])
	}
	want := map[string]interface{}{"a": []interface{}{float64(1), "x"}}
	if !reflect.DeepEqual(snap["input"], want) {
		t.Fatalf("snapshot input not isolated: %v", snap["input"])
	}
	if _, ok := snap["extra"]; ok {
		t.Fatal("snapshot gained a key added after snapshotting")
	}
}

func TestSnapshotJSONValueScalarsPassThrough(t *testing.T) {
	for _, c := range []any{nil, "s", float64(3), true} {
		if got := snapshotJSONValue(c); got != c {
			t.Fatalf("scalar changed: %v -> %v", c, got)
		}
	}
}
