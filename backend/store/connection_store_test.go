package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ys-ll/uniterm/backend/credentials"
	"github.com/ys-ll/uniterm/backend/session"
)

func mustConnStore(t *testing.T) *ConnectionStore {
	t.Helper()
	return &ConnectionStore{configDir: t.TempDir(), passwordStore: fakeCipherStore{}}
}

// TestConnectionStoreSyncRoundTrip pins the sync exchange contract for
// connections.json: seed, ExportPlain, ImportPlain into a fresh store on a
// different directory, Load back. The data must survive through the plain
// bytes, and the blob must carry plaintext secrets — sync re-encrypts the
// whole blob with the sync key downstream.
func TestConnectionStoreSyncRoundTrip(t *testing.T) {
	a := mustConnStore(t)
	seed := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{{ID: "g1", Name: "grp"}},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "secret"}},
	}
	if err := a.Save(seed); err != nil {
		t.Fatalf("Save: %v", err)
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	if !contains(string(plain), "secret") || contains(string(plain), "enc:v1:") {
		t.Fatalf("blob must carry plaintext secrets: %s", plain)
	}
	b := mustConnStore(t)
	if err := b.ImportPlain(plain); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}
	got, err := b.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Groups) != 1 || got.Groups[0].ID != "g1" {
		t.Fatalf("groups lost: %+v", got.Groups)
	}
	if len(got.Connections) != 1 || got.Connections[0].ID != "c1" || got.Connections[0].Password != "secret" {
		t.Fatalf("connections lost: %+v", got.Connections)
	}
}

// TestConnectionStoreSyncRoundTripKeyText is the keyText regression guard:
// an inline private key (keyContent) is encrypted in place locally, so the
// export must carry plaintext — the whole blob is protected solely by the
// sync key at rest in the repo. A literal enc:v1: prefix escaping to another
// device breaks it.
func TestConnectionStoreSyncRoundTripKeyText(t *testing.T) {
	a := mustConnStore(t)
	seed := session.ConnectionStoreData{Connections: []session.ConnectionConfig{{
		ID: "c1", Type: "ssh", AuthType: "keyText", KeyContent: "-----BEGIN OPENSSH PRIVATE KEY-----",
	}}}
	if err := a.Save(seed); err != nil {
		t.Fatalf("Save: %v", err)
	}
	plain, err := a.ExportPlain()
	if err != nil {
		t.Fatalf("ExportPlain: %v", err)
	}
	if !contains(string(plain), "BEGIN OPENSSH PRIVATE KEY") || contains(string(plain), "enc:v1:") {
		t.Fatalf("blob must carry plaintext key content: %s", plain)
	}
	b := mustConnStore(t)
	if err := b.ImportPlain(plain); err != nil {
		t.Fatalf("ImportPlain: %v", err)
	}
	got, err := b.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Connections) != 1 || got.Connections[0].KeyContent != "-----BEGIN OPENSSH PRIVATE KEY-----" {
		t.Fatalf("key content lost: %+v", got.Connections)
	}
}

// TestConnectionStoreImportPlainRejectsGarbage: ImportPlain must surface
// unparseable input instead of persisting a zero value over good data.
func TestConnectionStoreImportPlainRejectsGarbage(t *testing.T) {
	a := mustConnStore(t)
	if err := a.Save(session.ConnectionStoreData{Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "p"}}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	b := mustConnStore(t)
	if err := b.ImportPlain([]byte("not json")); err == nil {
		t.Fatal("ImportPlain(garbage) = nil, want error")
	}
	if got, err := b.Load(); err != nil || len(got.Connections) != 0 {
		t.Fatalf("garbage import must not persist data: %+v, %v", got, err)
	}
}


func TestConnectionStoreDesktopRegions(t *testing.T) {
	s := mustConnStore(t)
	view := &session.DesktopView{
		Selected: "region:right",
		Regions: []session.DesktopRegion{
			{ID: "left", Name: "主屏", DesktopRect: session.DesktopRect{X: 0, Y: 0, Width: 1920, Height: 1080}},
			{ID: "right", Name: "副屏", DesktopRect: session.DesktopRect{X: 1920, Y: 0, Width: 2560, Height: 1440}},
		},
	}
	data := session.ConnectionStoreData{Connections: []session.ConnectionConfig{{ID: "desktop", Type: "vnc", AuthType: "password", Password: "secret", DesktopView: view}}}
	if err := s.Save(data); err != nil {
		t.Fatal(err)
	}
	// Load through a new store instance to exercise the file representation.
	reader := &ConnectionStore{configDir: s.configDir, passwordStore: fakeCipherStore{}}
	loaded, err := reader.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Connections[0].DesktopView, view) {
		t.Fatalf("saved regions changed: %+v", loaded.Connections[0].DesktopView)
	}
	if loaded.Connections[0].Password != "secret" {
		t.Fatal("connection credentials did not survive the region save")
	}
	loaded.Connections[0].DesktopView.Regions = loaded.Connections[0].DesktopView.Regions[:1]
	loaded.Connections[0].DesktopView.Selected = "all"
	if err := reader.Save(loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Connections[0].DesktopView.Regions) != 1 || loaded.Connections[0].DesktopView.Selected != "all" {
		t.Fatalf("region deletion did not persist: %+v", loaded.Connections[0].DesktopView)
	}
}


func TestConnectionStoreDesktopPreferences(t *testing.T) {
	for _, protocol := range []string{"vnc", "rdp", "spice"} {
		t.Run(protocol, func(t *testing.T) {
			s := mustConnStore(t)
			data := session.ConnectionStoreData{Connections: []session.ConnectionConfig{
				{ID: "remembered", Type: protocol, AuthType: "password", Password: "secret",
					DesktopPreferences: &session.DesktopPreferences{ScaleViewport: true, ViewOnly: true, ShowDotCursor: true}},
				{ID: "defaults", Type: protocol},
			}}
			if err := s.Save(data); err != nil {
				t.Fatal(err)
			}
			reader := &ConnectionStore{configDir: s.configDir, passwordStore: fakeCipherStore{}}
			loaded, err := reader.Load()
			if err != nil {
				t.Fatal(err)
			}
			remembered, defaults := loaded.Connections[0], loaded.Connections[1]
			if remembered.DesktopPreferences == nil || !remembered.DesktopPreferences.ScaleViewport ||
				!remembered.DesktopPreferences.ViewOnly || !remembered.DesktopPreferences.ShowDotCursor {
				t.Fatal("display preferences were not restored")
			}
			if remembered.Password != "secret" {
				t.Fatal("saving display preferences changed the credentials")
			}
			if defaults.DesktopPreferences != nil {
				t.Fatal("preferences leaked into another connection or changed legacy defaults")
			}
			// Turning an option back off must also survive closing and reopening.
			loaded.Connections[0].DesktopPreferences.ScaleViewport = false
			loaded.Connections[0].DesktopPreferences.ViewOnly = false
			loaded.Connections[0].DesktopPreferences.ShowDotCursor = false
			if err := reader.Save(loaded); err != nil {
				t.Fatal(err)
			}
			loaded, err = s.Load()
			if err != nil {
				t.Fatal(err)
			}
			remembered = loaded.Connections[0]
			if remembered.DesktopPreferences == nil || remembered.DesktopPreferences.ScaleViewport ||
				remembered.DesktopPreferences.ViewOnly || remembered.DesktopPreferences.ShowDotCursor {
				t.Fatal("turning display preferences off did not persist")
			}
		})
	}
}


// fakeLegacyKC is a LegacyPasswordSource keyed by connection ID, mimicking the
// pre-enc:v1 keychain (conn/<id>) holding plaintext passwords. DeletePassword
// mutates the shared map, so tests can assert move semantics through the
// original handle.
type fakeLegacyKC struct{ pw map[string]string }

func (f fakeLegacyKC) GetPassword(id string) (string, error) { return f.pw[id], nil }
func (fakeLegacyKC) GetModelAPIKey(string) (string, error)   { return "", nil }
func (f fakeLegacyKC) DeletePassword(id string) error {
	delete(f.pw, id)
	return nil
}

// TestLoadFallsBackToLegacyKeychain reproduces the upgrade regression: a
// connection saved under the old scheme has no enc:v1 field (the password
// lived in the keychain), and the one-shot migration never ran. Load() must
// recover the password from the legacy keychain and lazy-migrate it to enc:v1
// on the resulting save so it is not invisible.
func TestLoadFallsBackToLegacyKeychain(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	s.SetLegacyKeychain(fakeLegacyKC{pw: map[string]string{"c1": "secret-pw"}})

	// JSON with no password field — exactly the old keychain-mode shape.
	mustWriteConnections(t, dir, session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "password"},
		},
	})

	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := data.Connections[0].Password; got != "secret-pw" {
		t.Fatalf("password = %q, want %q (legacy keychain fallback)", got, "secret-pw")
	}
}

// TestLoadLegacyFallBackLazyMigrates verifies the recovered plaintext is
// written back as enc:v1 so subsequent loads don't depend on the keychain,
// and that the legacy entry is then deleted (move semantics).
func TestLoadLegacyFallBackLazyMigrates(t *testing.T) {
	dir := t.TempDir()
	kc := fakeLegacyKC{pw: map[string]string{"c1": "secret-pw"}}
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	s.SetLegacyKeychain(kc)

	mustWriteConnections(t, dir, session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "password"},
		},
	})
	if _, err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	raw, _ := os.ReadFile(filepath.Join(dir, storeFileName))
	var d session.ConnectionStoreData
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if !credentials.IsEncrypted(d.Connections[0].Password) {
		t.Fatalf("password not lazy-migrated to enc:v1: %q", d.Connections[0].Password)
	}
	// Move semantics: the encrypted copy is on disk, so the legacy entry must
	// be gone — a kept-forever copy is what let a user-cleared password
	// resurrect on the next load.
	if _, ok := kc.pw["c1"]; ok {
		t.Fatal("legacy keychain entry not deleted after lazy migration")
	}
}

// TestClearedPasswordDoesNotResurrectFromLegacy locks in the resurrection bug
// fix: backfill used to key on an empty JSON password field while the legacy
// keychain entry was kept forever, so a user-cleared password came back on the
// next start. With move semantics the entry is deleted once the encrypted copy
// is on disk, so clearing stays cleared across a restart.
func TestClearedPasswordDoesNotResurrectFromLegacy(t *testing.T) {
	dir := t.TempDir()
	kc := fakeLegacyKC{pw: map[string]string{"c1": "secret-pw"}}
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	s.SetLegacyKeychain(kc)

	// First load backfills from the legacy keychain and lazy-migrates.
	mustWriteConnections(t, dir, session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "password"},
		},
	})
	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Connections[0].Password != "secret-pw" {
		t.Fatalf("backfill = %q, want secret-pw", data.Connections[0].Password)
	}

	// The user clears the password.
	data.Connections[0].Password = ""
	if err := s.Save(data); err != nil {
		t.Fatalf("Save cleared: %v", err)
	}

	// "Restart": a fresh store over the same directory must not resurrect it.
	s2 := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	s2.SetLegacyKeychain(kc)
	fresh, err := s2.Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := fresh.Connections[0].Password; got != "" {
		t.Fatalf("cleared password resurrected on reload: %q", got)
	}
}

// TestLoadEncV1PreferredOverLegacy ensures an existing enc:v1 field is used
// and the legacy keychain is never consulted for that connection.
func TestLoadEncV1PreferredOverLegacy(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	enc, _ := fakeCipherStore{}.Encrypt("enc-pw")
	// Legacy holds a DIFFERENT value — it must be ignored.
	s.SetLegacyKeychain(fakeLegacyKC{pw: map[string]string{"c1": "stale-legacy"}})

	mustWriteConnections(t, dir, session.ConnectionStoreData{
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "password", Password: enc},
		},
	})
	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := data.Connections[0].Password; got != "enc-pw" {
		t.Fatalf("password = %q, want enc:v1 value %q", got, "enc-pw")
	}
}

func mustWriteConnections(t *testing.T, dir string, data session.ConnectionStoreData) {
	t.Helper()
	s := &ConnectionStore{configDir: dir}
	if err := writeJSONFile(s.filePath(), data); err != nil {
		t.Fatalf("writeJSONFile: %v", err)
	}
}


// enc returns the fakeCipherStore encoding of pw (used to seed encrypted fields).
func enc(pw string) string {
	return "enc:v1:" + base64.StdEncoding.EncodeToString([]byte(pw))
}

func TestConnectionStore_SaveEncryptsPassword(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	data := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "secret"}},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if contains(string(raw), "secret") {
		t.Fatalf("plaintext leaked: %s", raw)
	}
	if !contains(string(raw), "enc:v1:") {
		t.Fatalf("expected encrypted field on disk, got %s", raw)
	}
}

func TestConnectionStore_LoadDecryptsPassword(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	seed := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: enc("secret")}},
	}
	if err := writeJSONFile(s.filePath(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Connections[0].Password != "secret" {
		t.Fatalf("Load password = %q, want secret", data.Connections[0].Password)
	}
}

// TestConnectionStore_IdentityKeepsOverrideUser 锁定 issue #959 的语义：
// identity 连接的 User 是用户显式填写的覆盖用户名（非密文），保存时必须
// 原样保留；Password 仍照旧清空，凭据只从密钥库物化。
func TestConnectionStore_IdentityKeepsOverrideUser(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	data := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "identity", IdentityId: "id-1", User: "deploy", Password: enc("stale")}},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !contains(string(raw), "deploy") {
		t.Fatalf("override User lost on save: %s", raw)
	}
	if contains(string(raw), "stale") {
		t.Fatalf("stale Password leaked for identity connection: %s", raw)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Connections[0].User != "deploy" {
		t.Fatalf("Load User = %q, want deploy", loaded.Connections[0].User)
	}
}

func TestConnectionStore_LoadMigratesLegacyPlaintext(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	seed := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "legacy"}},
	}
	if err := writeJSONFile(s.filePath(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Connections[0].Password != "legacy" {
		t.Fatalf("Load should return plaintext, got %q", data.Connections[0].Password)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, storeFileName))
	if !contains(string(raw), "enc:v1:") {
		t.Fatalf("legacy plaintext not migrated to ciphertext: %s", raw)
	}
	if contains(string(raw), "\"password\": \"legacy\"") {
		t.Fatalf("legacy plaintext still on disk: %s", raw)
	}
}

// TestConnectionStore_AuthModeSwitchDropsStalePassword locks in that a login
// password does not survive an auth-mode switch to key/keyText. If it did, a
// later connect of the switched connection would use the stale password as a
// key passphrase and break unencrypted private keys.
func TestConnectionStore_AuthModeSwitchDropsStalePassword(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	// Save as a password connection.
	saved := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "pw"}},
	}
	if err := s.Save(saved); err != nil {
		t.Fatalf("Save password conn: %v", err)
	}

	// Switch the same connection to keyText — the password must be dropped.
	switched := session.ConnectionStoreData{
		Groups: []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{
			ID: "c1", Type: "ssh", AuthType: "keyText",
			KeyContent: "-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----",
		}},
	}
	if err := s.Save(switched); err != nil {
		t.Fatalf("Save keyText conn: %v", err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Connections[0].Password != "" {
		t.Fatalf("stale password survived auth-mode switch: %q", loaded.Connections[0].Password)
	}
}

// TestConnectionStore_SaveEncryptsKeyText and LoadDecryptsKeyText lock in that
// an inline private-key text (authType "keyText", KeyContent) is encrypted at
// rest in connections.json like a password, so plaintext keys can't leak to disk
// and are decrypted back on load (#720).
func TestConnectionStore_SaveEncryptsKeyText(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	const keyText = "-----BEGIN OPENSSH PRIVATE KEY-----\nfake\n-----END OPENSSH PRIVATE KEY-----"
	data := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "keyText", KeyContent: keyText, Password: "pp"}},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if contains(string(raw), keyText) {
		t.Fatalf("plaintext KeyContent leaked: %s", raw)
	}
	if !contains(string(raw), "enc:v1:") {
		t.Fatalf("expected encrypted KeyContent on disk: %s", raw)
	}
}

func TestConnectionStore_LoadDecryptsKeyText(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	seed := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "keyText", KeyContent: enc("PEM-KEY"), Password: enc("pp")}},
	}
	if err := writeJSONFile(s.filePath(), seed); err != nil {
		t.Fatalf("seed: %v", err)
	}
	data, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if data.Connections[0].KeyContent != "PEM-KEY" {
		t.Fatalf("Load KeyContent = %q, want PEM-KEY", data.Connections[0].KeyContent)
	}
}

// TestConnectionStore_SaveEncryptsKeyPassphrase locks in the field-based
// encryption rule: a key-type connection's private-key passphrase (stored in
// Password) is encrypted at rest exactly like a password-login credential —
// encryption is driven by the secret-field manifest, not by auth type.
func TestConnectionStore_SaveEncryptsKeyPassphrase(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}

	data := session.ConnectionStoreData{
		Groups: []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{
			{ID: "c1", Type: "ssh", AuthType: "key", KeyPath: "/tmp/id_ed25519", Password: "key-passphrase"},
			{ID: "c2", Type: "ssh", AuthType: "keyText", KeyContent: "-----BEGIN OPENSSH PRIVATE KEY-----", Password: "inline-key-passphrase"},
		},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if contains(string(raw), "key-passphrase") ||
		contains(string(raw), "inline-key-passphrase") ||
		contains(string(raw), "-----BEGIN OPENSSH PRIVATE KEY-----") {
		t.Fatalf("plaintext secret leaked to disk: %s", raw)
	}

	loaded, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Connections[0].Password != "key-passphrase" {
		t.Fatalf("key passphrase not restored: %q", loaded.Connections[0].Password)
	}
	if loaded.Connections[1].KeyContent != "-----BEGIN OPENSSH PRIVATE KEY-----" {
		t.Fatalf("keyContent not restored: %q", loaded.Connections[1].KeyContent)
	}
	if loaded.Connections[1].Password != "inline-key-passphrase" {
		t.Fatalf("keyText passphrase not restored: %q", loaded.Connections[1].Password)
	}
}

// refusingCipher accepts any Encrypt but refuses every Decrypt, standing in
// for a wrong or unavailable master key.
type refusingCipher struct{}

func (refusingCipher) Encrypt(p string) (string, error) { return fakeCipherStore{}.Encrypt(p) }
func (refusingCipher) Decrypt(string) (string, error) {
	return "", errors.New("wrong key")
}

// TestConnectionStore_LoadDecryptErrorSurfaces verifies a wrong-key decrypt
// failure propagates from Load instead of silently leaving ciphertext in
// memory (prerequisite for surfacing a re-unlock prompt on migrated data).
func TestConnectionStore_LoadDecryptErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	if err := s.Save(session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "key", Password: "secret"}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s.passwordStore = refusingCipher{}
	if _, err := s.Load(); err == nil {
		t.Fatal("expected Load to surface the decrypt error")
	}
}

// TestConnectionStore_SaveSkipsUnchangedContent locks in the F-105 skip:
// the comparison happens on the PRE-encryption input, so saving identical
// data must not touch the file at all (AES-GCM's random nonce would make a
// post-encryption comparison useless).
func TestConnectionStore_SaveSkipsUnchangedContent(t *testing.T) {
	dir := t.TempDir()
	s := &ConnectionStore{configDir: dir, passwordStore: fakeCipherStore{}}
	data := session.ConnectionStoreData{
		Groups:      []session.ConnectionGroup{},
		Connections: []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "secret"}},
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("Save: %v", err)
	}
	path := filepath.Join(dir, storeFileName)
	// Overwrite the on-disk copy with a sentinel: if the next Save rewrites
	// the file, the sentinel disappears and the test fails.
	if err := os.WriteFile(path, []byte("sentinel"), 0600); err != nil {
		t.Fatalf("seed sentinel: %v", err)
	}
	if err := s.Save(data); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(raw) != "sentinel" {
		t.Fatal("Save rewrote unchanged content — plaintext-level skip is broken")
	}
	// Changed content must go through.
	changed := data
	changed.Connections = []session.ConnectionConfig{{ID: "c1", Type: "ssh", AuthType: "password", Password: "other"}}
	if err := s.Save(changed); err != nil {
		t.Fatalf("Save changed: %v", err)
	}
	raw, _ = os.ReadFile(path)
	if string(raw) == "sentinel" {
		t.Fatal("Save skipped content that actually changed")
	}
}


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
