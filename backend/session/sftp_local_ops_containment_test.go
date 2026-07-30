package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSFTPSessionLocalPutContentBlocksSymlinkEscape guards the regression
// where SFTPSession defined its own LocalXxx methods that bypassed the
// safeAbsPath containment guard added to localFSOps. The original 5a3a925
// commit fixed SMB/WebDAV/S3/FTP (which embed localFSOps) but missed SFTP,
// leaving a real path-traversal attack surface: a malicious remote SFTP
// server can plant a symlink in the user's local download dir, and a
// subsequent LocalPutContent / LocalMkdir / LocalGetContent would follow
// it to write/read outside localCwd.
//
// Fix: SFTPSession now embeds localFSOps and the LocalXxx methods delegate
// to it, so they share the same symlink-containment check.
func TestSFTPSessionLocalPutContentBlocksSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	decoy := filepath.Join(outside, "decoy.txt")
	if err := os.WriteFile(decoy, []byte("keep me safe"), 0644); err != nil {
		t.Fatalf("seed decoy: %v", err)
	}
	link := filepath.Join(root, "sneaky")
	if err := os.Symlink(decoy, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	s := NewSFTPSession("sftp-ctn-test")
	s.localFSOps.localCwd = root

	if err := s.LocalPutContent("sneaky", []byte("pwned")); err == nil {
		t.Fatal("SFTPSession.LocalPutContent through symlink succeeded; should have been blocked")
	} else if !strings.Contains(err.Error(), "escapes local directory") {
		t.Fatalf("unexpected error: %v", err)
	}
	got, rerr := os.ReadFile(decoy)
	if rerr != nil {
		t.Fatalf("read decoy: %v", rerr)
	}
	if string(got) != "keep me safe" {
		t.Fatalf("decoy contents changed to %q; containment failed", got)
	}
}