package store

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/ys-ll/uniterm/backend/session"
)

func TestConnectionTOTPVaultAndSync(t *testing.T) {
	const secret = "JBSWY3DPEHPK3PXP"
	a := mustConnStore(t)
	data := session.ConnectionStoreData{Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "identity", IdentityId: "i1", TOTPEnabled: true, TOTPSecret: secret}}}
	if err := a.Save(data); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(a.filePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) || !bytes.Contains(raw, []byte("enc:v1:")) {
		t.Fatal("TOTP secret must be encrypted on disk")
	}
	out, err := a.Load()
	if err != nil {
		t.Fatal(err)
	}
	if out.Connections[0].TOTPSecret != secret || !out.Connections[0].TOTPEnabled {
		t.Fatal("TOTP config lost")
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatal(err)
	}
	b := mustConnStore(t)
	if err := b.ImportPlain(plain); err != nil {
		t.Fatal(err)
	}
	out, err = b.Load()
	if err != nil || out.Connections[0].TOTPSecret != secret {
		t.Fatal("TOTP sync round trip failed", err)
	}
	raw, err = os.ReadFile(b.filePath())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(secret)) {
		t.Fatal("sync destination leaked TOTP secret")
	}
	var obj map[string]interface{}
	if err = json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	ClearSecretFields("connections.json", obj)
	if obj["connections"].([]interface{})[0].(map[string]interface{})["totpSecret"] != "" {
		t.Fatal("credential reset left TOTP secret")
	}
}

func TestConnectionTOTPRefusesPlaintextWithoutVault(t *testing.T) {
	s := &ConnectionStore{configDir: t.TempDir()}
	data := session.ConnectionStoreData{Connections: []session.ConnectionConfig{{ID: "c1", TOTPEnabled: true, TOTPSecret: "JBSWY3DPEHPK3PXP"}}}
	if err := s.Save(data); err == nil {
		t.Fatal("saved plaintext TOTP without a vault")
	}
	if _, err := os.Stat(s.filePath()); !os.IsNotExist(err) {
		t.Fatal("failed save must not create a file")
	}
	s.passwordStore = fakeCipherStore{}
	data.Connections[0].TOTPSecret = "invalid!"
	if err := s.Save(data); err == nil {
		t.Fatal("saved invalid enabled secret")
	}
}
