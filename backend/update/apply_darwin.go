//go:build darwin

package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ys-ll/uniterm/backend/log"
)

func (m *Manager) applyPlatform(pend *PendingUpdate, onProgress func(Progress)) error {
	if err := applyBinary(pend.NewBinary, pend.NewBinarySHA256, onProgress); err != nil {
		return err
	}
	// Replacing Contents/MacOS/uniTerm invalidates the bundle signature.
	// Releases ship ad-hoc signed, so re-apply an ad-hoc signature.
	reSignBundle()
	return nil
}

// reSignBundle ad-hoc re-signs the .app bundle containing the running binary.
// Best effort: signing is skipped when not running from a bundle or codesign
// is unavailable.
func reSignBundle() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	bundle := filepath.Dir(filepath.Dir(filepath.Dir(exe)))
	if !strings.HasSuffix(bundle, ".app") {
		return
	}
	cmd := exec.Command("codesign", "--force", "--deep", "--sign", "-", bundle)
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Writef("[update] codesign failed: %v (%s)", err, strings.TrimSpace(string(out)))
		return
	}
	log.Writef("[update] bundle re-signed (ad-hoc): %s", bundle)
}
