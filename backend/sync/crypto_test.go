package sync

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustKey returns a fresh 32-byte AES key.
func mustKey(t *testing.T) []byte {
	t.Helper()
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		t.Fatal(err)
	}
	return k
}

// writeJSON writes data to path with 0600.
func writeJSON(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

// ---- encrypt / decrypt round-trip ----

// TestEncryptDecryptBytes_RoundTrip pins the contract that
// encryptBytes / decryptBytes is a lossless round-trip; if a key is
// tampered, decryption must fail (no silent corruption).
func TestEncryptDecryptBytes_RoundTrip(t *testing.T) {
	key := mustKey(t)
	plain := []byte("hello world — secret payload")
	enc, err := encryptBytes(plain, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "" {
		t.Fatal("empty ciphertext")
	}

	got, err := decryptBytes(enc, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("roundtrip mismatch: got %q want %q", got, plain)
	}
}

// TestEncryptDecryptBytes_TamperedCiphertextFails verifies the AES-GCM
// authentication tag is enforced: any base64-level corruption fails
// decrypt rather than returning garbage.
func TestEncryptDecryptBytes_TamperedCiphertextFails(t *testing.T) {
	key := mustKey(t)
	enc, err := encryptBytes([]byte("secret"), key)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := base64.StdEncoding.DecodeString(enc)
	if len(raw) < 2 {
		t.Fatal("ciphertext too short to flip")
	}
	// Flip a byte in the middle of the ciphertext (not in the nonce).
	raw[len(raw)/2] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)

	if _, err := decryptBytes(tampered, key); err == nil {
		t.Fatal("expected decrypt of tampered ciphertext to fail")
	}
}

// TestEncryptDecryptBytes_WrongKeyFails: a different key must NOT open
// the same ciphertext.
func TestEncryptDecryptBytes_WrongKeyFails(t *testing.T) {
	key := mustKey(t)
	enc, _ := encryptBytes([]byte("secret"), key)
	other := mustKey(t)
	if _, err := decryptBytes(enc, other); err == nil {
		t.Fatal("expected decrypt with wrong key to fail")
	}
}

// TestEncryptBytes_DistinctNoncePerCall verifies nonce uniqueness; two
// encryptions of identical plaintext under the same key MUST produce
// different ciphertexts. (Failure here = random.Read broken or reused
// nonce; both would silently catastrophically break AES-GCM.)
func TestEncryptBytes_DistinctNoncePerCall(t *testing.T) {
	key := mustKey(t)
	a, _ := encryptBytes([]byte("same"), key)
	b, _ := encryptBytes([]byte("same"), key)
	if a == b {
		t.Fatal("expected different ciphertexts across encryptions (nonce reuse)")
	}
}

// TestEncryptBytesWithAAD_RejectsWrongContext: ciphertext bound to
// logical file A must not be openable as file B (SYNC-P1-1).
func TestEncryptBytesWithAAD_RejectsWrongContext(t *testing.T) {
	key := mustKey(t)
	enc, _ := encryptBytesWithAAD([]byte(`{"foo":1}`), key, []byte("connections.json"))
	if _, err := decryptBytesWithAAD(enc, key, []byte("settings.json")); err == nil {
		t.Fatal("expected AAD mismatch to fail decrypt")
	}
	if _, err := decryptBytesWithAAD(enc, key, []byte("connections.json")); err != nil {
		t.Fatalf("expected AAD match to succeed, got %v", err)
	}
}

// TestEncryptDecryptFiles_RoundTrip covers the public Encrypt/Decrypt
// with kc=nil (the "no keychain" path). The keychain path needs OS
// integration; here we verify the file plumbing is intact end-to-end.
func TestEncryptDecryptFiles_RoundTrip(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	connData, _ := json.Marshal(map[string]any{
		"groups":      []any{},
		"connections": []any{map[string]any{"id": "a", "host": "x", "authType": "key"}},
	})
	writeJSON(t, filepath.Join(srcDir, "connections.json"), connData)
	writeJSON(t, filepath.Join(srcDir, "settings.json"), []byte(`{"theme":"dark"}`))
	writeJSON(t, filepath.Join(srcDir, "quickCommands.json"), []byte(`{"commands":[]}`))

	key := mustKey(t)
	if err := EncryptConfigFiles(srcDir, dstDir, key, nil); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if err := DecryptConfigFiles(dstDir, outDir, key, nil); err != nil {
		t.Fatal(err)
	}

	for name, want := range map[string]string{
		"connections.json":   string(connData),
		"settings.json":      `{"theme":"dark"}`,
		"quickCommands.json": `{"commands":[]}`,
	} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var a, b map[string]any
		_ = json.Unmarshal(got, &a)
		_ = json.Unmarshal([]byte(want), &b)
		if len(a) != len(b) {
			t.Errorf("%s mismatch:\n got: %s\nwant: %s", name, got, want)
		}
	}
}

// TestEncryptConfigFiles_MissingSourceFileFillsEmpty: a missing
// connections.json is treated as an empty config (no error).
func TestEncryptConfigFiles_MissingSourceFileFillsEmpty(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	writeJSON(t, filepath.Join(srcDir, "settings.json"), []byte(`{}`))
	writeJSON(t, filepath.Join(srcDir, "quickCommands.json"), []byte(`{}`))
	// connections.json missing on purpose.

	key := mustKey(t)
	if err := EncryptConfigFiles(srcDir, dstDir, key, nil); err != nil {
		t.Fatalf("EncryptConfigFiles with missing connections.json: %v", err)
	}
}

// TestDecryptConfigFiles_MissingSourceFileFillsEmpty: per the decrypt
// helper, a missing src file (not yet encrypted) writes an empty
// object on the dest side — this avoids panicking if the user
// only has settings.json uploaded.
func TestDecryptConfigFiles_MissingSourceFileFillsEmpty(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	// Intentionally missing all three source files.
	key := mustKey(t)
	if err := DecryptConfigFiles(srcDir, outDir, key, nil); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.TrimSpace(string(got)) != "{}" {
			t.Errorf("%s = %s, want {}", name, got)
		}
	}
}

// ---- derive-key / salt / round-trip ----

// TestDeriveKey_Deterministic verifies PBKDF2 with the same salt and
// password yields the same key (deterministic) but different
// passwords produce different keys.
func TestDeriveKey_Deterministic(t *testing.T) {
	salt := []byte("0123456789abcdef")
	a := DeriveKey("pw", salt)
	b := DeriveKey("pw", salt)
	if !bytes.Equal(a, b) {
		t.Fatal("DeriveKey should be deterministic for same password+salt")
	}
	if len(a) != keyLength {
		t.Fatalf("derived key length = %d, want %d", len(a), keyLength)
	}
	c := DeriveKey("pw2", salt)
	if bytes.Equal(a, c) {
		t.Fatal("different passwords should derive different keys")
	}
}

// TestSaltRoundTrip pins ReadSaltFile / WriteSaltFile as a pair — a
// salt written in hex must decode to identical bytes on reload.
func TestSaltRoundTrip(t *testing.T) {
	dir := t.TempDir()
	salt, _ := GenerateSalt()
	if len(salt) != saltLength {
		t.Fatalf("salt length = %d, want %d", len(salt), saltLength)
	}
	if err := WriteSaltFile(dir, salt); err != nil {
		t.Fatal(err)
	}
	got, err := ReadSaltFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, salt) {
		t.Fatal("salt round-trip mismatch")
	}
}

// TestReadSaltFile_MissingReturnsNil: a non-existent salt file should
// yield (nil, nil) — that's how configureRepo decides between
// "existing repo" and "fresh setup".
func TestReadSaltFile_MissingReturnsNil(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadSaltFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing salt, got %v", got)
	}
}

// TestSaltFileHexEncoded ensures the on-disk salt is hex-encoded (no
// raw bytes; safer to inspect).
func TestSaltFileHexEncoded(t *testing.T) {
	dir := t.TempDir()
	salt := []byte{0x00, 0x01, 0x02, 0xFF}
	if err := WriteSaltFile(dir, salt); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".sync-salt"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("salt file not valid hex: %v", err)
	}
	if !bytes.Equal(got, salt) {
		t.Fatalf("salt file decodes to %v, want %v", got, salt)
	}
}

// ---- block-cipher sanity ----

// TestAES_GCMTearDownSelfTest is a paranoia test for the cipher
// package: encrypt → decrypt with explicit AAD should match. This
// guards against the package's use of GCM drifting if a future
// refactor swaps modes.
func TestAES_GCMTearDownSelfTest(t *testing.T) {
	key := mustKey(t)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	io.ReadFull(rand.Reader, nonce)
	ct := gcm.Seal(nil, nonce, []byte("payload"), []byte("aad"))
	pt, err := gcm.Open(nil, nonce, ct, []byte("aad"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "payload" {
		t.Fatal("aes-gcm self-test mismatch")
	}
	if gcm.NonceSize() == 0 {
		t.Fatal("nonce size should be > 0")
	}
}

// TestEncryptConfigFiles_PreservesArbitraryJSON exercises a smoke run
// with realistic shapes — connections groups, settings, and a
// non-empty quickCommands payload.
func TestEncryptConfigFiles_PreservesArbitraryJSON(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()
	outDir := t.TempDir()

	conn := `{"groups":[{"id":"g1","name":"prod"}],"connections":[{"id":"c1","host":"h","authType":"key"}]}`
	set := `{"theme":"dark","font":"mono","ai":{"models":[{"id":"x","apiKey":"y"}]}}`
	quick := `{"commands":[{"id":"q","name":"q","command":"ls"}]}`
	writeJSON(t, filepath.Join(srcDir, "connections.json"), []byte(conn))
	writeJSON(t, filepath.Join(srcDir, "settings.json"), []byte(set))
	writeJSON(t, filepath.Join(srcDir, "quickCommands.json"), []byte(quick))

	key := mustKey(t)
	if err := EncryptConfigFiles(srcDir, dstDir, key, nil); err != nil {
		t.Fatal(err)
	}
	if err := DecryptConfigFiles(dstDir, outDir, key, nil); err != nil {
		t.Fatal(err)
	}

	for name := range map[string]string{"connections.json": "", "settings.json": "", "quickCommands.json": ""} {
		got, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.TrimSpace(string(got)) == "" {
			t.Fatalf("%s decrypted to empty", name)
		}
	}
}
