package store

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrConflict = errors.New("CONFIG_CONFLICT: changed in another client; your edits were not saved. Reload before retrying, or save a copy")

// Revision hashes decoded content, not ciphertext, so re-encryption alone does
// not invalidate an editor. No revision fields are added to existing files.
func Revision(data any) (string, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}
