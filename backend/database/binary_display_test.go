package database

import (
	"bytes"
	"testing"
)

// .NET: new Guid("00112233-4455-6677-8899-aabbccddeeff").ToByteArray()
var dotnetBytes = []byte{0x33, 0x22, 0x11, 0x00, 0x55, 0x44, 0x77, 0x66, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff}

const dotnetGuid = "00112233-4455-6677-8899-aabbccddeeff"

func TestFormatDotNetGuid(t *testing.T) {
	if got := FormatDotNetGuid(dotnetBytes); got != dotnetGuid {
		t.Fatalf("FormatDotNetGuid = %s, want %s", got, dotnetGuid)
	}
}

func TestParseDotNetGuidRoundTrip(t *testing.T) {
	for _, in := range []string{dotnetGuid, "{" + dotnetGuid + "}", "00112233-4455-6677-8899-AABBCCDDEEFF"} {
		b, ok := ParseDotNetGuid(in)
		if !ok || !bytes.Equal(b, dotnetBytes) {
			t.Fatalf("ParseDotNetGuid(%q) = %x, %v", in, b, ok)
		}
	}
	for _, bad := range []string{"", "0x00112233", "00112233445566778899aabbccddeeff", "zz112233-4455-6677-8899-aabbccddeeff"} {
		if _, ok := ParseDotNetGuid(bad); ok {
			t.Fatalf("ParseDotNetGuid(%q) accepted", bad)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		in   []byte
		guid bool
		want string
	}{
		{dotnetBytes, true, dotnetGuid},
		{dotnetBytes, false, "0x33221100554477668899AABBCCDDEEFF"},
		{[]byte{0x01, 0x02}, true, "0x0102"},                        // RAW column but not 16 bytes
		{[]byte("plain text\nline 2"), false, "plain text\nline 2"}, // MySQL text arrives as []byte
		{[]byte("Café crème, déjà vu"), true, "Café crème, déjà vu"},
		{[]byte{0x00, 'a'}, false, "0x0061"}, // control byte means binary
	}
	for _, c := range cases {
		if got := formatBytes(c.in, c.guid); got != c.want {
			t.Errorf("formatBytes(%x, %v) = %q, want %q", c.in, c.guid, got, c.want)
		}
	}
}

func TestBytesFromDisplayRoundTrip(t *testing.T) {
	if b, ok := bytesFromDisplay(formatBytes(dotnetBytes, true), true).([]byte); !ok || !bytes.Equal(b, dotnetBytes) {
		t.Fatalf("guid round trip failed: %v", b)
	}
	raw := []byte{0xde, 0xad, 0x00, 0xbe, 0xef}
	if b, ok := bytesFromDisplay(formatBytes(raw, false), false).([]byte); !ok || !bytes.Equal(b, raw) {
		t.Fatalf("hex round trip failed: %v", b)
	}
	if v := bytesFromDisplay("plain", true); v != "plain" {
		t.Fatalf("text changed: %v", v)
	}
	if v := bytesFromDisplay(int64(5), true); v != int64(5) {
		t.Fatalf("number changed: %v", v)
	}
}

func TestApplyRaw(t *testing.T) {
	raw := map[string]bool{"Id": true, "Blob": false}
	in := map[string]any{"Id": dotnetGuid, "Blob": "0xDEAD", "Name": dotnetGuid, "Gone": nil}
	out := applyRaw(raw, in)
	if b, ok := out["Id"].([]byte); !ok || !bytes.Equal(b, dotnetBytes) {
		t.Fatalf("Id not converted: %v", out["Id"])
	}
	if b, ok := out["Blob"].([]byte); !ok || !bytes.Equal(b, []byte{0xde, 0xad}) {
		t.Fatalf("Blob not converted: %v", out["Blob"])
	}
	if out["Name"] != dotnetGuid || out["Gone"] != nil {
		t.Fatalf("non-RAW columns changed: %v", out)
	}
}
