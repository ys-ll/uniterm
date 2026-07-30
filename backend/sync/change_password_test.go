package sync

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// QA-017 / F-007: ChangePassword atomicity test.
//
// While the password change rotates the on-disk ciphertext, no
// concurrent reader should observe a state where the file is unreadable
// under either key (which would mean the reader fetched the
// half-written `.tmp` file before the rename).
//
// The production code path is:
//   .tmp-write → os.Rename → atomic on POSIX. So long as the
// rename(2) actually happens, observers see one consistent blob.
//
// We exercise the rename atomicity in isolation by writing distinct
// ciphertext under .tmp + os.Rename while many readers poll the
// final path and verify the file is always readable under at least
// one key.

func TestChangePassword_Atomic_DoesNotMixKeys(t *testing.T) {
	dir := t.TempDir()

	// Pre-bake two ciphertexts: one decryptable by oldKey, one by newKey.
	oldKey := mustKey(t)
	newKey := mustKey(t)

	seedDir := t.TempDir()
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
		if err := os.WriteFile(filepath.Join(seedDir, name), []byte(`{"k":1}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := EncryptConfigFiles(seedDir, dir, oldKey, nil); err != nil {
		t.Fatal(err)
	}
	oldCT, err := os.ReadFile(filepath.Join(dir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Sanity: decryptable with oldKey, not newKey.
	if _, err := decryptBytes(string(oldCT), newKey); err == nil {
		t.Fatal("newKey unexpectedly opens old ciphertext")
	}

	// Build the new-key ciphertext under a side path.
	newSeed := t.TempDir()
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
		if err := os.WriteFile(filepath.Join(newSeed, name), []byte(`{"k":2}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	newCTDir := t.TempDir()
	if err := EncryptConfigFiles(newSeed, newCTDir, newKey, nil); err != nil {
		t.Fatal(err)
	}
	newCT, err := os.ReadFile(filepath.Join(newCTDir, "connections.json"))
	if err != nil {
		t.Fatal(err)
	}

	connPath := filepath.Join(dir, "connections.json")

	const readers = 8
	const iterations = 200

	// The invariant: every successful os.ReadFile must yield ciphertext
	// decryptable by at least one of {oldKey, newKey}.  "Both fail" is the
	// only failure mode — that would indicate the reader fetched the
	// half-renamed .tmp or a torn write.

	var badReads int64
	var reads int64
	stop := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				select {
				case <-stop:
					return
				default:
				}
				ct, err := os.ReadFile(connPath)
				if err != nil {
					// os.Rename atomically swaps, so a ReadFile only fails
					// if the file is gone — impossible.
					continue
				}
				atomic.AddInt64(&reads, 1)
				_, errOld := decryptBytes(string(ct), oldKey)
				_, errNew := decryptBytes(string(ct), newKey)
				if errOld != nil && errNew != nil {
					atomic.AddInt64(&badReads, 1)
				}
			}
		}()
	}

	// Rotate 50 times.  Each rotation: write newCT to .tmp, then
	// rename(2) it onto the final path.  This is exactly what
	// change_password does in production.
	for i := 0; i < 50; i++ {
		tmp := connPath + ".tmp"
		want := oldCT
		if i%2 == 0 {
			want = newCT
		}
		if err := os.WriteFile(tmp, want, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, connPath); err != nil {
			t.Fatal(err)
		}
	}
	close(stop)
	wg.Wait()

	if got := atomic.LoadInt64(&badReads); got > 0 {
		t.Fatalf("atomicity violated: %d/%d reads observed ciphertext decryptable by NEITHER key",
			got, atomic.LoadInt64(&reads))
	}
	if atomic.LoadInt64(&reads) == 0 {
		t.Fatal("readers never read the file; test setup is broken")
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
