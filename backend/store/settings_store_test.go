package store

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDefaultKeyboardUsesPrimaryModifier(t *testing.T) {
	binding := defaultKeyboard()["nextTab"]
	if !binding.Primary || binding.Ctrl || binding.Meta {
		t.Fatalf("nextTab should use the platform primary modifier: %+v", binding)
	}
}

func TestKeyBindingPrimaryJSONRoundTrip(t *testing.T) {
	want := KeyBinding{Primary: true, Shift: true, Key: "enter"}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}

	var got KeyBinding
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("KeyBinding JSON round trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestKeyBindingExplicitFalsePrimaryIsPersisted(t *testing.T) {
	data, err := json.Marshal(KeyBinding{Ctrl: true, Primary: false, Key: "k"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(`"primary":false`)) {
		t.Fatalf("explicit false primary modifier was omitted: %s", data)
	}
}

func TestMigrateLegacyPrimaryBindings(t *testing.T) {
	keyboard := map[string]KeyBinding{
		"newConnection":     {Ctrl: true, Shift: true, Key: "n"},
		"openQuickCommands": {Meta: true, Key: "k"},
	}
	data := []byte(`{"keyboard":{"newConnection":{"ctrl":true,"shift":true,"key":"n"},"openQuickCommands":{"meta":true,"key":"k"}}}`)
	if !migrateLegacyPrimaryBindings(data, keyboard) {
		t.Fatal("expected legacy bindings to be migrated")
	}
	if !keyboard["newConnection"].Primary || keyboard["newConnection"].Ctrl {
		t.Fatalf("newConnection was not migrated: %+v", keyboard["newConnection"])
	}
	if !keyboard["openQuickCommands"].Primary || keyboard["openQuickCommands"].Meta {
		t.Fatalf("openQuickCommands was not migrated: %+v", keyboard["openQuickCommands"])
	}
}

func TestMigrateLegacyPrimaryBindingsPreservesExplicitFalse(t *testing.T) {
	keyboard := map[string]KeyBinding{
		"newConnection":     {Ctrl: true, Primary: false, Shift: true, Key: "n"},
		"openQuickCommands": {Meta: true, Primary: false, Key: "k"},
	}
	data := []byte(`{"keyboard":{"newConnection":{"ctrl":true,"primary":false,"shift":true,"key":"n"},"openQuickCommands":{"meta":true,"primary":false,"key":"k"}}}`)
	if migrateLegacyPrimaryBindings(data, keyboard) {
		t.Fatal("explicit modifiers must not be migrated")
	}
	if !keyboard["newConnection"].Ctrl || keyboard["newConnection"].Primary {
		t.Fatalf("explicit Control binding changed: %+v", keyboard["newConnection"])
	}
	if !keyboard["openQuickCommands"].Meta || keyboard["openQuickCommands"].Primary {
		t.Fatalf("explicit Meta binding changed: %+v", keyboard["openQuickCommands"])
	}
}
