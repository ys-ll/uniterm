package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLocalOpsContainmentBlocksSymlinkEscape guards the regression where
// LocalPutContent / LocalRemove / LocalMkdir / LocalCopy / LocalMove /
// LocalRename would follow a symlink planted inside localCwd (e.g. by
// malicious remote content downloaded via FTP/SFTP/SMB/WebDAV/S3) and
// operate on files outside the user's chosen local directory.
//
// Without this guard, a downloaded symlink `evil -> /tmp/target` would let
// LocalPutContent overwrite /tmp/target and LocalRemove delete it.
func TestLocalOpsContainmentBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	// The decoy target lives outside root — any successful write/read/delete
	// here is a containment failure.
	outside := t.TempDir()
	decoy := filepath.Join(outside, "decoy.txt")
	if err := os.WriteFile(decoy, []byte("keep me safe"), 0644); err != nil {
		t.Fatalf("seed decoy: %v", err)
	}

	// Plant a symlink inside root pointing outside.
	link := filepath.Join(root, "sneaky")
	if err := os.Symlink(decoy, link); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	o := localFSOps{localCwd: root}

	t.Run("LocalPutContent", func(t *testing.T) {
		err := o.LocalPutContent("sneaky", []byte("pwned"))
		if err == nil {
			t.Fatal("LocalPutContent through symlink succeeded; should have been blocked")
		}
		if !strings.Contains(err.Error(), "escapes local directory") {
			t.Fatalf("unexpected error: %v", err)
		}
		got, rerr := os.ReadFile(decoy)
		if rerr != nil {
			t.Fatalf("read decoy after blocked write: %v", rerr)
		}
		if string(got) != "keep me safe" {
			t.Fatalf("decoy contents changed to %q; containment failed", got)
		}
	})

	t.Run("LocalRemove", func(t *testing.T) {
		err := o.LocalRemove("sneaky", false)
		if err == nil {
			t.Fatal("LocalRemove through symlink succeeded; should have been blocked")
		}
		if _, statErr := os.Stat(decoy); statErr != nil {
			t.Fatalf("decoy removed despite blocked call: %v", statErr)
		}
	})

	t.Run("LocalMkdir", func(t *testing.T) {
		err := o.LocalMkdir("sneaky")
		if err == nil {
			t.Fatal("LocalMkdir through symlink succeeded; should have been blocked")
		}
		if _, statErr := os.Stat(decoy); statErr != nil {
			t.Fatalf("decoy changed despite blocked call: %v", statErr)
		}
	})

	t.Run("LocalCopy", func(t *testing.T) {
		// Source inside root, destination is the symlink — would copy on top of /tmp/target.
		src := filepath.Join(root, "src.txt")
		if err := os.WriteFile(src, []byte("source"), 0644); err != nil {
			t.Fatalf("seed src: %v", err)
		}
		err := o.LocalCopy("src.txt", "sneaky")
		if err == nil {
			t.Fatal("LocalCopy destination through symlink succeeded; should have been blocked")
		}
	})

	t.Run("LocalRename", func(t *testing.T) {
		// Move a real file inside root to a name that, via a planted symlink
		// inside root pointing outside, lands on the outside dir.
		src := filepath.Join(root, "victim.txt")
		if err := os.WriteFile(src, []byte("payload"), 0644); err != nil {
			t.Fatalf("seed src: %v", err)
		}
		err := o.LocalRename("victim.txt", "sneaky")
		if err == nil {
			t.Fatal("LocalRename through symlink succeeded; should have been blocked")
		}
	})
}

// TestLocalOpsAllowLegitOperations confirms the containment guard rejects
// only escapes — ordinary operations inside localCwd (including into
// subdirectories) keep working.
func TestLocalOpsAllowLegitOperations(t *testing.T) {
	root := t.TempDir()
	o := localFSOps{localCwd: root}

	// Write a file in a subdirectory (creates the dir on demand).
	if err := o.LocalMkdir("sub"); err != nil {
		t.Fatalf("LocalMkdir sub: %v", err)
	}
	if err := o.LocalPutContent(filepath.Join("sub", "ok.txt"), []byte("hello")); err != nil {
		t.Fatalf("LocalPutContent: %v", err)
	}
	got, err := o.LocalGetContent(filepath.Join("sub", "ok.txt"))
	if err != nil {
		t.Fatalf("LocalGetContent: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	if err := o.LocalRemove(filepath.Join("sub", "ok.txt"), false); err != nil {
		t.Fatalf("LocalRemove: %v", err)
	}
}