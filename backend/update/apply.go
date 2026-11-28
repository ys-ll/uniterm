package update

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/selfupdate"
	"github.com/ys-ll/uniterm/backend/log"
)

// Apply installs the staged update in place and reports progress. The caller
// is responsible for relaunching/restarting the app afterwards. The
// platform-specific logic lives in apply_<os>.go (applyPlatform).
func (m *Manager) Apply(onProgress func(Progress)) error {
	pend := m.Pending()
	if pend == nil {
		return errors.New("no staged update")
	}
	return m.applyPlatform(pend, onProgress)
}

// openVerifiedBinary opens newBinary, re-hashes it, and rewinds it for
// selfupdate.Apply. Verifying the same file descriptor that is handed to
// Apply removes the path-swap window between the hash and the replacement:
// the bytes read by Apply are exactly the bytes hashed here.
func openVerifiedBinary(newBinary, wantSHA string) (*os.File, error) {
	if wantSHA == "" {
		return nil, fmt.Errorf("refusing to apply %s: no recorded binary sha256", newBinary)
	}
	f, err := os.Open(newBinary)
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		f.Close()
		return nil, err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(got, wantSHA) {
		f.Close()
		log.Writef("[update] extracted binary sha256 mismatch: got %s want %s", got, wantSHA)
		return nil, fmt.Errorf("sha256 mismatch for extracted binary %s", newBinary)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

// applyBinary replaces the running executable with newBinary using
// minio/selfupdate (which handles the Windows locked-file rename dance and
// Unix permission semantics). The extracted binary is re-hashed immediately
// before the replacement; a backup copy of the current binary is kept at
// <exe>.uniterm-old and removed on success or used for a best-effort rollback.
func applyBinary(newBinary, wantSHA string, onProgress func(Progress)) error {
	if onProgress != nil {
		onProgress(Progress{Phase: "applying", Total: -1})
	}

	f, err := openVerifiedBinary(newBinary, wantSHA)
	if err != nil {
		return err
	}
	defer f.Close()

	exe, err := os.Executable()
	if err != nil {
		return err
	}

	backupPath := exe + ".uniterm-old"
	backedUp := false
	if data, err := os.ReadFile(exe); err == nil {
		if werr := os.WriteFile(backupPath, data, 0755); werr == nil {
			backedUp = true
		}
	}

	if err := selfupdate.Apply(f, selfupdate.Options{}); err != nil {
		log.Writef("[update] apply failed: %v", err)
		// Best-effort rollback: selfupdate may have already renamed the
		// running exe away, so restoring the backup makes the app runnable.
		if backedUp {
			if rerr := os.Rename(backupPath, exe); rerr == nil {
				log.Writef("[update] rolled back from backup")
			}
		}
		return err
	}

	if backedUp {
		_ = os.Remove(backupPath)
	}
	log.Writef("[update] binary replaced: %s", exe)
	return nil
}
