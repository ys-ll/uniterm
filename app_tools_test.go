package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatJSONPretty(t *testing.T) {
	app := &App{}
	out, err := app.FormatJSON(`{"a":1,"b":[true,null,"x"]}`, "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "{\n  \"a\": 1,\n  \"b\": [\n    true,\n    null,\n    \"x\"\n  ]\n}"
	if out != want {
		t.Fatalf("got %q, want %q", out, want)
	}
	// Tab indent and 4-space indent.
	out, err = app.FormatJSON(`{"a":1}`, "tab")
	if err != nil || out != "{\n\t\"a\": 1\n}" {
		t.Fatalf("tab indent: out=%q err=%v", out, err)
	}
	out, err = app.FormatJSON(`{"a":1}`, "4")
	if err != nil || out != "{\n    \"a\": 1\n}" {
		t.Fatalf("4 indent: out=%q err=%v", out, err)
	}
}

func TestFormatJSONMinify(t *testing.T) {
	out, err := (&App{}).FormatJSON("{\n  \"a\": 1\n}", "min")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != `{"a":1}` {
		t.Fatalf("got %q, want %q", out, `{"a":1}`)
	}
}

func TestFormatJSONSyntaxError(t *testing.T) {
	_, err := (&App{}).FormatJSON("{\n  \"a\": ,\n}", "2")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected line info in error, got: %v", err)
	}
}

func TestFormatJSONUnicode(t *testing.T) {
	out, err := (&App{}).FormatJSON(`{"名称":"值"}`, "2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "名称") {
		t.Fatalf("expected CJK characters preserved, got: %q", out)
	}
}

func TestHashString(t *testing.T) {
	hashes, err := (&App{}).HashString("abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cases := map[string]string{
		"md5":    "900150983cd24fb0d6963f7d28e17f72",
		"sha1":   "a9993e364706816aba3e25717850c26c9cd0d89d",
		"sha256": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"sha512": "ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		"crc32":  "352441c2",
	}
	for algo, want := range cases {
		if got := hashes[algo]; got != want {
			t.Errorf("%s = %q, want %q", algo, got, want)
		}
	}
}

func TestHashStringEmptyAndUnicode(t *testing.T) {
	hashes, err := (&App{}).HashString("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hashes["md5"] != "d41d8cd98f00b204e9800998ecf8427e" {
		t.Fatalf("empty md5 = %q", hashes["md5"])
	}
	hashes, err = (&App{}).HashString("中文")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MD5 of UTF-8 bytes of "中文" (e4b8ad e69687).
	if hashes["md5"] != "a7bac2239fcdcb3a067903d8077c4a07" {
		t.Fatalf("unicode md5 = %q", hashes["md5"])
	}
}

func TestEncodeStringBase64RoundTrip(t *testing.T) {
	app := &App{}
	enc, err := app.EncodeString("你好 hello", "base64", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc != "5L2g5aW9IGhlbGxv" {
		t.Fatalf("base64 encode = %q", enc)
	}
	dec, err := app.EncodeString(enc, "base64", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != "你好 hello" {
		t.Fatalf("base64 round-trip = %q", dec)
	}
}

func TestEncodeStringHexRoundTrip(t *testing.T) {
	app := &App{}
	enc, err := app.EncodeString("abc", "hex", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enc != "616263" {
		t.Fatalf("hex encode = %q", enc)
	}
	dec, err := app.EncodeString(enc, "hex", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != "abc" {
		t.Fatalf("hex round-trip = %q", dec)
	}
}

func TestEncodeStringInvalidInput(t *testing.T) {
	app := &App{}
	if _, err := app.EncodeString("!!!not hex", "hex", true); err == nil {
		t.Fatal("expected error decoding invalid hex")
	}
	if _, err := app.EncodeString("x", "yaml", false); err == nil {
		t.Fatal("expected error for unsupported encoding")
	}
}

func TestEncodeStringTolerantDecode(t *testing.T) {
	app := &App{}
	// Whitespace inside the payload (e.g. line-wrapped base64).
	dec, err := app.EncodeString("5L2g5aW9\nIGhl\nbGxv", "base64", true)
	if err != nil || dec != "你好 hello" {
		t.Fatalf("wrapped base64: out=%q err=%v", dec, err)
	}
	// Missing padding (raw base64).
	dec, err = app.EncodeString("5L2g5aW9IGhlbGxv", "base64", true)
	if err != nil {
		// Padded form decodes via StdEncoding; this string is 16 chars, valid std.
		t.Fatalf("unexpected error: %v", err)
	}
	if dec != "你好 hello" {
		t.Fatalf("std decode = %q", dec)
	}
	dec, err = app.EncodeString("SGVsbG8", "base64", true)
	if err != nil || dec != "Hello" {
		t.Fatalf("raw (unpadded) decode: out=%q err=%v", dec, err)
	}
	// URL-safe variant with - and _.
	enc, err := app.EncodeString("subjects?_x", "base64", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dec, err = app.EncodeString(enc, "base64", true)
	if err != nil || dec != "subjects?_x" {
		t.Fatalf("url-safe round-trip: out=%q err=%v", dec, err)
	}
	// Hex with internal whitespace.
	dec, err = app.EncodeString("61 62\n63", "hex", true)
	if err != nil || dec != "abc" {
		t.Fatalf("hex with whitespace: out=%q err=%v", dec, err)
	}
}

func TestCronNextRuns(t *testing.T) {
	app := &App{}
	runs, err := app.CronNextRuns("*/5 * * * *", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runs) != 3 {
		t.Fatalf("want 3 runs, got %d", len(runs))
	}
	// Minutes must be multiples of 5.
	if !strings.HasSuffix(runs[0][:16], "0") && !strings.HasSuffix(runs[0][:16], "5") {
		t.Fatalf("unexpected minute in %q", runs[0])
	}
	if _, err := app.CronNextRuns("not a cron", 3); err == nil {
		t.Fatal("expected error for invalid cron")
	}
	// @-descriptor support.
	if _, err := app.CronNextRuns("@hourly", 1); err != nil {
		t.Fatalf("@hourly should parse: %v", err)
	}
}

func TestYamlToJson(t *testing.T) {
	app := &App{}
	out, err := app.YamlToJson("name: uni\nitems:\n  - 1\n  - two\nnested:\n  ok: true\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Map keys are sorted by json.Marshal, so compare parsed values.
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got["name"] != "uni" || got["nested"].(map[string]any)["ok"] != true {
		t.Fatalf("unexpected content: %v", got)
	}
	items, ok := got["items"].([]any)
	if !ok || len(items) != 2 || items[0] != float64(1) || items[1] != "two" {
		t.Fatalf("unexpected items: %v", got["items"])
	}
	if _, err := app.YamlToJson(":\n  - [bad"); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestJsonToYaml(t *testing.T) {
	app := &App{}
	out, err := app.JsonToYaml(`{"name":"uni","nested":{"ok":true,"n":3.5,"big":1234567890123}}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Note: yaml.v3 quotes the key "n" (a YAML 1.1 bool-like word).
	for _, want := range []string{"name: uni", "ok: true", `"n": 3.5`, "big: 1234567890123"} {
		if !strings.Contains(out, want) {
			t.Fatalf("want %q in output %q", want, out)
		}
	}
	if _, err := app.JsonToYaml("{bad json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
