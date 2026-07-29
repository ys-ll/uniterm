package diag

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatorFileSizeRotation(t *testing.T) {
	dir := t.TempDir()
	r := &Rotator{Dir: dir, Keep: 3, FileSizeCap: 1024, DirSizeCap: 1 << 20}
	// Simulate 3 rotations.
	for i := 0; i < 3; i++ {
		if err := r.MaybeRotate(2048); err != nil {
			t.Fatal(err)
		}
		// Touch a fake "current" file so subsequent size checks work.
		_ = os.WriteFile(filepath.Join(dir, "uniterm.log"), make([]byte, 2048), 0o644)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "uniterm.log*"))
	if len(files) < 2 {
		t.Fatalf("expected rotated files, got %d", len(files))
	}
}

func TestRotatorDirSizeCap(t *testing.T) {
	dir := t.TempDir()
	r := &Rotator{Dir: dir, Keep: 3, FileSizeCap: 1 << 20, DirSizeCap: 4096}
	// Pre-create files totalling 10 KiB.
	for i := 0; i < 10; i++ {
		_ = os.WriteFile(filepath.Join(dir, "uniterm.log."+string(rune('a'+i))), make([]byte, 1024), 0o644)
	}
	if err := r.EnforceDirCap(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) >= 10 {
		t.Fatalf("dir cap not enforced: %d files", len(entries))
	}
}
