package importer

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ys-ll/uniterm/backend/session"
)

func sampleStore() session.ConnectionStoreData {
	return session.ConnectionStoreData{
		Groups: []session.ConnectionGroup{{ID: "g1", Name: "prod"}},
		Connections: []session.ConnectionConfig{
			{ID: "c1", Name: "db", Type: "ssh", Host: "10.0.0.5", Port: 22, User: "root",
				AuthType: "password", Password: "hunter2", GroupId: strptr("g1")},
		},
	}
}

func TestUnitermRoundTripWithPassword(t *testing.T) {
	b, err := ExportUniterm(sampleStore(), "pass123")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	res, err := parseUniterm(b, ParseOptions{Password: "pass123"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(res.Connections) != 1 || res.Connections[0].Password != "hunter2" {
		t.Fatalf("password not restored: %+v", res.Connections)
	}
	if res.Connections[0].ID == "c1" {
		t.Fatalf("connection id not regenerated")
	}
	if res.Groups[0].ID == "g1" {
		t.Fatalf("group id not regenerated")
	}
}

func TestUnitermEncryptedNoKDFFails(t *testing.T) {
	body := []byte(`{"format":"uniterm","version":1,"encrypted":true,"groups":[],"connections":[]}`)
	if _, err := parseUniterm(body, ParseOptions{Password: "pass"}); err == nil {
		t.Fatal("expected error for encrypted file missing kdf block")
	}
}

func TestExportUnitermEncryptsKeyPassphrase(t *testing.T) {
	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Name: "db", Type: "ssh", Host: "h", Port: 22, User: "root",
				AuthType: "key", KeyPath: "/k", Password: "keypass"},
		},
	}
	b, err := ExportUniterm(data, "pass123")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f utmFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p := f.Connections[0].Password; p == "keypass" || !strings.HasPrefix(p, "enc:v1:") {
		t.Fatalf("key passphrase not encrypted: %q", p)
	}
	res, err := parseUniterm(b, ParseOptions{Password: "pass123"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Connections[0].Password != "keypass" {
		t.Fatalf("key passphrase not restored: %q", res.Connections[0].Password)
	}
}

func TestUnitermWrongPasswordFails(t *testing.T) {
	b, _ := ExportUniterm(sampleStore(), "pass123")
	if _, err := parseUniterm(b, ParseOptions{Password: "wrong"}); err == nil {
		t.Fatal("expected error for wrong password")
	}
	if _, err := parseUniterm(b, ParseOptions{}); err == nil {
		t.Fatal("expected error for missing password")
	}
}

// TestMaterializeForExport pins the export materialization: every
// identity-referencing connection is resolved into a plain credential
// connection, so the exported file is self-contained — no authType
// "identity" survives and nothing references the source machine's identity
// store. A dangling reference degrades to a connection without credentials.
func TestMaterializeForExport(t *testing.T) {
	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", AuthType: "identity", IdentityId: "id1", Host: "h", Port: 22},
			{ID: "c2", AuthType: "identity", IdentityId: "gone", Host: "h2", Port: 22, User: "override"},
			{ID: "c3", AuthType: "keyText", Host: "h3", Port: 22, KeyContent: "PEM"},
		},
	}
	idents := map[string]session.Identity{
		"id1": {ID: "id1", Name: "prod", Username: "deploy", AuthType: "password", Password: "secret"},
	}
	out, err := MaterializeForExport(data, func(id string) (session.Identity, bool) {
		i, ok := idents[id]
		return i, ok
	})
	if err != nil {
		t.Fatal(err)
	}
	cs := out.Connections
	if cs[0].AuthType != "password" || cs[0].User != "deploy" || cs[0].Password != "secret" || cs[0].IdentityId != "" {
		t.Fatalf("c1 not materialized: %+v", cs[0])
	}
	// A dangling reference degrades to no credentials; the connection's own
	// username override is kept (it never came from the identity).
	if cs[1].AuthType != "password" || cs[1].User != "override" || cs[1].Password != "" || cs[1].IdentityId != "" {
		t.Fatalf("c2 dangling reference not degraded: %+v", cs[1])
	}
	if cs[2].AuthType != "keyText" || cs[2].KeyContent != "PEM" {
		t.Fatalf("non-identity connection must pass through: %+v", cs[2])
	}
}

// TestExportUnitermEncryptsAllSecretFields: the export encrypts every field
// the save path treats as secret — the manifest list — so a private key never
// rides along in cleartext inside an "encrypted" export.
func TestExportUnitermEncryptsAllSecretFields(t *testing.T) {
	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "keyText", Host: "h", Port: 22,
				KeyContent: "-----BEGIN OPENSSH PRIVATE KEY-----", Password: "keypass"},
		},
	}
	b, err := ExportUniterm(data, "pass123")
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f utmFile
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	if p := f.Connections[0].KeyContent; !strings.HasPrefix(p, "enc:v1:") {
		t.Fatalf("key content exported in cleartext: %q", p)
	}
	res, err := parseUniterm(b, ParseOptions{Password: "pass123"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if res.Connections[0].KeyContent != "-----BEGIN OPENSSH PRIVATE KEY-----" || res.Connections[0].Password != "keypass" {
		t.Fatalf("round-trip lost secrets: %+v", res.Connections[0])
	}
}

// TestExportUnitermRejectsInPlaceCiphertext: a value still encrypted under
// the source machine's credential store would make the export depend on that
// machine — the export refuses instead of silently carrying it.
func TestExportUnitermRejectsInPlaceCiphertext(t *testing.T) {
	data := session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "password", Host: "h", Port: 22, Password: "enc:v1:residue"},
		},
	}
	if _, err := ExportUniterm(data, "pass123"); err == nil {
		t.Fatal("in-place ciphertext must abort the export")
	}
}

// TestExportUnitermRequiresPassword: the UI always provides one; the
// password-less branch (which cleared every credential) is gone.
func TestExportUnitermRequiresPassword(t *testing.T) {
	if _, err := ExportUniterm(sampleStore(), ""); err == nil {
		t.Fatal("an export without a password must be refused")
	}
}

func TestUnitermTOTPEncryptedExport(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	data := sampleStore()
	data.Connections[0].TOTPEnabled = true
	data.Connections[0].TOTPSecret = secret
	raw, err := ExportUniterm(data, "export-password")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("export leaked plaintext TOTP secret")
	}
	res, err := parseUniterm(raw, ParseOptions{Password: "export-password"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Connections[0].TOTPEnabled || res.Connections[0].TOTPSecret != secret {
		t.Fatal("export lost TOTP configuration")
	}
}
