package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ys-ll/uniterm/backend/credentials"
)

// This file is the single source of truth for which JSON config files carry
// secret fields and where those fields live. Every consumer of encrypted
// fields derives its behaviour from this manifest so the lists can never
// drift apart:
//
//   - store Save: encrypt every non-empty field on these paths
//     (backend/store/*_store.go)
//   - store Load: decrypt every enc:v1: field on these paths
//   - credential reset: blank these paths without decrypting (app.go
//     clearEncryptedFields)
//   - credential setup guard: detect residual enc:v1: ciphertext on these
//     paths (credentials.storeHasSecrets)
//   - cloud sync: normalize these paths to plaintext for upload and back for
//     comparison (crypto.go)
//
// Path syntax: dot-separated object keys; "*" fans out over the elements of
// an array (or the values of an object) at that level, e.g.
// "connections.*.password" addresses every connection's password. Only
// string leaves are addressed; missing segments resolve to nothing, so a
// path that doesn't fit the file's shape is a harmless no-op.
type SecretFile struct {
	File  string
	Paths []string
}

// SecretFiles declares every config file's secret fields.
//
// connections.json: password holds the login password for password auth and
// the private-key passphrase for key/keyText auth; keyContent holds the
// inline private key for keyText auth; totpSecret holds the per-host TOTP
// seed. All are secrets regardless of auth type.
var SecretFiles = []SecretFile{
	{File: "connections.json", Paths: []string{"connections.*.password", "connections.*.keyContent", "connections.*.totpSecret"}},
	{File: "identities.json", Paths: []string{"identities.*.password", "identities.*.keyContent"}},
	{File: "proxies.json", Paths: []string{"proxies.*.pass"}},
	{File: "ai.json", Paths: []string{"models.*.apiKey"}},
	{File: "mcp_clients.json", Paths: []string{"clients.*.headers.*", "clients.*.env.*"}},
	{File: "settings.json", Paths: []string{"ai.models.*.apiKey"}},
}

// ClearAllSecretFields blanks every manifest-declared secret field in the
// data dir's config files WITHOUT decrypting: the credential reset runs while
// the credential store is already torn down, when decrypt-then-save would
// fail on the very fields being cleared. Missing or corrupt files are left
// untouched (a corrupt file carries no readable ciphertext and is the store's
// business on next load); non-secret fields are preserved byte-for-value.
func ClearAllSecretFields(dataDir string) {
	for _, sf := range SecretFiles {
		path := filepath.Join(dataDir, sf.File)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			continue
		}
		if err := ClearSecretFields(sf.File, obj); err != nil {
			continue
		}
		out, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			continue
		}
		_ = os.WriteFile(path, out, 0600)
	}
}

// SecretFileByName returns the manifest entry for file, or nil when the file
// carries no secret fields.
func SecretFileByName(file string) *SecretFile {
	for i := range SecretFiles {
		if SecretFiles[i].File == file {
			return &SecretFiles[i]
		}
	}
	return nil
}

// ErrNoCipher is returned by EncryptSecretFields when a plaintext secret
// would be persisted but no cipher is wired. Every store fails closed: the
// save aborts rather than persisting plaintext.
var ErrNoCipher = errors.New("passwordStore not initialized; refusing to save plaintext secret")

// DecryptResult reports what a decrypt walk found.
type DecryptResult struct {
	// Decrypted is the number of enc:v1: fields successfully decrypted in
	// place.
	Decrypted int
	// Plaintext is the number of secret fields found holding legacy
	// plaintext (candidates for lazy re-encryption on the next save).
	Plaintext int
}

// EncryptSecretFields encrypts every non-empty, not-yet-encrypted secret
// field declared for file in SecretFiles, writing the encrypted values back
// into data. data is the config struct (or map) being persisted; it is
// round-tripped through JSON so the manifest paths can address the fields.
// Returns ErrNoCipher when a plaintext secret remains unencrypted because ps
// is nil (fail-closed signal for callers that must never persist plaintext).
// Unknown files are a no-op.
func EncryptSecretFields[T any](file string, data *T, ps PasswordStore) error {
	sf := SecretFileByName(file)
	if sf == nil {
		return nil
	}
	obj, err := marshalToObj(*data)
	if err != nil {
		return err
	}
	if _, err := encryptSecretPaths(sf.Paths, obj, ps); err != nil {
		return err
	}
	return reMarshalInto(obj, data)
}

// encryptSecretPaths walks paths against obj, encrypting plaintext leaves in
// place. Returns the number of fields encrypted.
func encryptSecretPaths(paths []string, obj map[string]interface{}, ps PasswordStore) (int, error) {
	n := 0
	for _, path := range paths {
		var refs []fieldRef
		collectFieldRefs(obj, strings.Split(path, "."), &refs)
		for _, r := range refs {
			v := r.parent[r.key].(string)
			if v == "" || credentials.IsEncrypted(v) {
				continue
			}
			if ps == nil {
				return n, ErrNoCipher
			}
			enc, err := ps.Encrypt(v)
			if err != nil {
				return n, err
			}
			r.parent[r.key] = enc
			n++
		}
	}
	return n, nil
}

// DecryptSecretFields decrypts every enc:v1: field declared for file in
// SecretFiles, writing the decrypted values back into data. Fields holding
// plaintext are counted in the result but left untouched (the caller
// lazy-migrates them by re-saving). With ps nil the walk is skipped entirely
// so encrypted values pass through untouched (backward compat: stores
// without a wired cipher). Unknown files are a no-op.
func DecryptSecretFields[T any](file string, data *T, ps PasswordStore) (DecryptResult, error) {
	sf := SecretFileByName(file)
	if sf == nil || ps == nil {
		return DecryptResult{}, nil
	}
	obj, err := marshalToObj(*data)
	if err != nil {
		return DecryptResult{}, err
	}
	res, err := decryptSecretPaths(sf.Paths, obj, ps)
	if err != nil {
		return res, err
	}
	return res, reMarshalInto(obj, data)
}

// decryptSecretPaths walks paths against obj, decrypting enc:v1: leaves in
// place. Stops at the first decrypt error so the caller can surface it
// (wrong key / corrupted field) instead of silently carrying ciphertext.
func decryptSecretPaths(paths []string, obj map[string]interface{}, ps PasswordStore) (DecryptResult, error) {
	var res DecryptResult
	for _, path := range paths {
		var refs []fieldRef
		collectFieldRefs(obj, strings.Split(path, "."), &refs)
		for _, r := range refs {
			v := r.parent[r.key].(string)
			if v == "" {
				continue
			}
			if !credentials.IsEncrypted(v) {
				res.Plaintext++
				continue
			}
			pt, err := ps.Decrypt(v)
			if err != nil {
				return res, err
			}
			r.parent[r.key] = pt
			res.Decrypted++
		}
	}
	return res, nil
}

// ClearSecretFields blanks (sets to "") every field declared for file in
// SecretFiles, mutating obj in place. Used by credential reset: it must work
// while the credential store is locked, so it never decrypts — it only
// empties. Unknown files are a no-op.
func ClearSecretFields(file string, obj map[string]interface{}) error {
	sf := SecretFileByName(file)
	if sf == nil {
		return nil
	}
	for _, path := range sf.Paths {
		var refs []fieldRef
		collectFieldRefs(obj, strings.Split(path, "."), &refs)
		for _, r := range refs {
			r.parent[r.key] = ""
		}
	}
	return nil
}

// ObjHasEncryptedSecrets reports whether any field declared for file in
// SecretFiles currently holds enc:v1: ciphertext. Unknown files return
// false.
func ObjHasEncryptedSecrets(file string, obj map[string]interface{}) bool {
	sf := SecretFileByName(file)
	if sf == nil {
		return false
	}
	for _, path := range sf.Paths {
		var refs []fieldRef
		collectFieldRefs(obj, strings.Split(path, "."), &refs)
		for _, r := range refs {
			if credentials.IsEncrypted(r.parent[r.key].(string)) {
				return true
			}
		}
	}
	return false
}

// fieldRef addresses one string leaf reached by a manifest path: the map
// holding it and the key it sits under, so walkers can mutate in place.
type fieldRef struct {
	parent map[string]interface{}
	key    string
}

// collectFieldRefs walks segs against node, appending a fieldRef for every
// string leaf at the end of the path. "*" fans out over array elements and
// object values; concrete segments must match a map key. Values of other
// types (including non-string leaves) are skipped.
func collectFieldRefs(node interface{}, segs []string, out *[]fieldRef) {
	if len(segs) == 0 {
		return
	}
	seg := segs[0]
	last := len(segs) == 1
	switch n := node.(type) {
	case []interface{}:
		// Arrays are reached mid-path (e.g. after the "connections"
		// segment): "*" fans out over the elements; concrete segments are
		// not addressable on an array.
		if seg == "*" {
			for _, v := range n {
				collectFieldRefs(v, segs[1:], out)
			}
		}
	case map[string]interface{}:
		if seg == "*" {
			for k, v := range n {
				if last {
					if _, isStr := v.(string); isStr {
						*out = append(*out, fieldRef{parent: n, key: k})
					}
					continue
				}
				collectFieldRefs(v, segs[1:], out)
			}
			return
		}
		v, ok := n[seg]
		if !ok {
			return
		}
		if last {
			if _, isStr := v.(string); isStr {
				*out = append(*out, fieldRef{parent: n, key: seg})
			}
			return
		}
		collectFieldRefs(v, segs[1:], out)
	}
}

// marshalToObj round-trips v through encoding/json into a generic object so
// the manifest-driven secret-field walkers can address fields by path.
func marshalToObj(v interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// reMarshalInto serializes the generic obj back into the typed out value
// after a manifest walk has mutated secret fields.
func reMarshalInto(obj map[string]interface{}, out interface{}) error {
	raw, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

// --- Shared file persistence for the config stores ---
//
// Every secret-carrying store persists with the same three-step envelope:
// hash the pre-encryption input, skip the write when unchanged, otherwise
// encrypt via the manifest and write atomically. The helpers below keep that
// envelope in one place; each store only contributes its file name, path and
// cipher policy.

// jsonHash returns the canonical content hash used by the skip-rewrites
// check (F-105). It must be computed on the PRE-encryption input: AES-GCM
// uses a fresh random nonce per call, so a post-encryption hash would never
// match and the skip would never fire.
func jsonHash(v interface{}) (string, error) {
	preview, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(preview)
	return hex.EncodeToString(sum[:]), nil
}

// writeJSONFile atomically writes v as indented JSON (0600). Callers that
// guard concurrent Save/Load must hold their own lock.
func writeJSONFile(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data, 0600)
}

// forceWrite seeds saveEncryptedFile's hash-skip with a value no computed
// hash can match, forcing the write through. Use when the content to persist
// equals what was just loaded (lazy re-encryption of legacy plaintext): the
// seeded hash would otherwise match and skip the write, so the re-encryption
// would never run (docs/dev 4.3).
const forceWrite = ""

// saveEncryptedFile is the shared Save envelope for every secret-carrying
// store: hash the pre-encryption input, skip when unchanged, otherwise
// encrypt data in place via the manifest and write it atomically. The
// envelope fails closed: ErrNoCipher (no cipher wired) aborts the save
// rather than persisting plaintext. On any error the caller's lastSavedHash
// is returned unchanged, so callers may assign the result unconditionally:
//
//	h, err := saveEncryptedFile(...)
//	s.lastSavedHash = h
//	return err
func saveEncryptedFile[T any](file, path string, data *T, ps PasswordStore, lastSavedHash string) (string, error) {
	h, err := jsonHash(*data)
	if err != nil {
		return lastSavedHash, err
	}
	if h == lastSavedHash {
		return lastSavedHash, nil
	}
	if err := EncryptSecretFields(file, data, ps); err != nil {
		return lastSavedHash, err
	}
	if err := writeJSONFile(path, *data); err != nil {
		return lastSavedHash, err
	}
	return h, nil
}

// --- Shared file loading for the config stores ---
//
// The load-side counterpart of the Save envelope: read the file, parse it,
// and decrypt the manifest secret fields. Per-store policy (what to do when
// the file is missing, corrupt, or won't decrypt) stays with each store.

// errUnparseableConfig marks a file whose bytes are not valid JSON for the
// store's data type. loadEncryptedFile quarantines such files; callers use
// errors.Is to distinguish "corrupt file" from "decrypt failure".
var errUnparseableConfig = errors.New("config file is not parseable")

// loadEncryptedData parses raw config bytes into T and decrypts the manifest
// secret fields. It is the parsing half of the Load envelope, exposed for
// stores that read the bytes themselves (legacy format detection).
func loadEncryptedData[T any](file string, raw []byte, ps PasswordStore) (T, DecryptResult, error) {
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, DecryptResult{}, fmt.Errorf("%w: %v", errUnparseableConfig, err)
	}
	res, err := DecryptSecretFields(file, &out, ps)
	return out, res, err
}

// loadEncryptedFile reads the config file at path and parses+decrypts it,
// returning (data, decrypt result, content hash, existed, error). The hash is
// the same pre-encryption form saveEncryptedFile hashes, so stores seed their
// lastSavedHash with it and an unchanged first Save after startup skips the
// rewrite; it is empty unless parsing and decryption both succeeded. Missing
// file → existed=false. Unparseable JSON → quarantine + error. A decrypt
// failure returns the parsed data untouched — the file itself is intact and
// may recover with the right key. The caller decides whether to propagate
// errors or fall back to defaults.
func loadEncryptedFile[T any](file, path string, ps PasswordStore) (T, DecryptResult, string, bool, error) {
	var zero T
	return loadEncryptedFileBase(file, path, zero, ps)
}

// loadEncryptedFileBase is loadEncryptedFile with a base value the raw JSON is
// unmarshalled over: fields missing from the file keep base's values instead
// of Go zero values. Pass the store's defaults (e.g. defaultLocalState()) to
// keep older configs well-formed.
func loadEncryptedFileBase[T any](file, path string, base T, ps PasswordStore) (T, DecryptResult, string, bool, error) {
	var zero T
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return zero, DecryptResult{}, "", false, nil
		}
		return zero, DecryptResult{}, "", false, err
	}
	out := base
	if err := json.Unmarshal(raw, &out); err != nil {
		quarantineCorrupt(path)
		// existed=true: the file was there, the caller's !existed branch
		// (missing file) must not swallow the quarantine error.
		return zero, DecryptResult{}, "", true, fmt.Errorf("%w: %v", errUnparseableConfig, err)
	}
	res, err := DecryptSecretFields(file, &out, ps)
	if err != nil {
		return out, res, "", true, err
	}
	h, err := jsonHash(out)
	if err != nil {
		return out, res, "", true, err
	}
	return out, res, h, true, nil
}

// atomicWriteFile writes data to path via a temp file in the same directory,
// fsyncs the file, then renames over the destination. On POSIX this is
// atomic and survives process kill or power loss between calls without
// leaving a half-written file at the target path.
//
// Fixes: STORE-03, STORE-09, STORE-10, STORE-12, STORE-19, STORE-21 (refs in
// .planning/audit/phase-2/TRIAGE.md).
func atomicWriteFile(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err = os.Rename(tmpName, path); err != nil {
		// Windows: os.Open omits FILE_SHARE_DELETE (golang/go#32088), so a
		// concurrent reader holding the target open makes the replace-rename
		// fail with ACCESS_DENIED. Readers hold the handle only for the
		// instant of a ReadFile, so a short bounded retry gets through.
		if runtime.GOOS != "windows" {
			return err
		}
		for i := 0; i < 20; i++ {
			time.Sleep(5 * time.Millisecond)
			if err = os.Rename(tmpName, path); err == nil {
				break
			}
		}
	}
	return err
}

// quarantineCorrupt renames a corrupt JSON file aside so the next Save
// can proceed without losing the user's prior data to a silent overwrite.
// Returns the new path, or empty string if the rename was unnecessary.
//
// Fixes: STORE-09 (silent unmarshal-failure data wipe in 4 stores).
func quarantineCorrupt(path string) string {
	ts := time.Now().UTC().Format("20060102T150405")
	target := path + ".corrupt-" + ts
	if err := os.Rename(path, target); err != nil {
		return ""
	}
	return target
}

// The sync exchange contract (SyncableStore, declared in
// backend/sync/backend.go) is implemented by each store in its own file.
// The interface lives on the consumer side: store → credentials → sync means
// sync → store would be an import cycle, so the stores satisfy sync's
// interface structurally and neither package imports the other — the
// registry is assembled in app.go.
//
// The contract:
//
//   - ExportPlain renders the store's canonical plaintext — the same shape
//     the on-disk JSON carries after decryption (Load applies the store's
//     full semantics: legacy backfill with move semantics, lazy re-encrypt,
//     defaults). Secret fields come out as plaintext; the sync layer
//     re-encrypts the whole blob with the sync key downstream. A cleared
//     secret exports as cleared — the store is the single source of truth.
//   - A store whose backing file was never written returns an error wrapping
//     fs.ErrNotExist. Sync skips such blobs instead of uploading an empty
//     document that would clobber the remote copy.
//   - ImportPlain parses the blob and persists it through Save, so every
//     save-time invariant (password clearing for identity-auth, manifest
//     encryption, hash-skip) applies unchanged.

// exportPlainStore is the shared ExportPlain body: a store whose backing file
// was never created reports fs.ErrNotExist (sync skips it); otherwise the
// data comes through the store's own Load (all semantics applied) and is
// marshalled in the canonical on-disk form.
func exportPlainStore[T any](blob, path string, load func() (T, error)) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%s: %w", blob, fs.ErrNotExist)
		}
		return nil, err
	}
	data, err := load()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(data, "", "  ")
}

// importPlainStore is the shared ImportPlain body: parse into the store's
// data type and persist through Save. Unparseable input surfaces as an error
// so a corrupt blob never persists a zero value over good local data.
func importPlainStore[T any](raw []byte, save func(T) error) error {
	var data T
	if err := json.Unmarshal(raw, &data); err != nil {
		return fmt.Errorf("parse sync blob: %w", err)
	}
	return save(data)
}
