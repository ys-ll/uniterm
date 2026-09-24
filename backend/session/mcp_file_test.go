package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveMcpLocalPath(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	for _, d := range []string{allowed, outside} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A file inside the allowed dir.
	f := filepath.Join(allowed, "f.txt")
	os.WriteFile(f, []byte("x"), 0644)
	// A file outside.
	g := filepath.Join(outside, "g.txt")
	os.WriteFile(g, []byte("y"), 0644)
	// A symlink inside allowed pointing outside (escape attempt).
	link := filepath.Join(allowed, "escape")
	os.Symlink(outside, link)

	cases := []struct {
		name    string
		path    string
		roots   []string
		wantErr bool
	}{
		{"inside allowed", f, []string{allowed}, false},
		{"allowed root itself", allowed, []string{allowed}, false},
		{"outside rejected", g, []string{allowed}, true},
		{"symlink escape rejected", filepath.Join(link, "g.txt"), []string{allowed}, true},
		{"no roots rejects everything", f, nil, true},
		{"empty path", "", []string{allowed}, true},
		{"relative resolves against cwd", "f.txt", []string{allowed}, true}, // cwd is the package dir, not allowed
		// Download destination: file does not exist yet, but its directory
		// does — must resolve through the symlinked ancestor (regression for
		// the macOS /tmp → /private/tmp case where plain EvalSymlinks failed
		// and every fresh download target was wrongly rejected).
		{"nonexistent file in allowed dir", filepath.Join(allowed, "download-dest.txt"), []string{allowed}, false},
		{"nonexistent file outside", filepath.Join(outside, "nope.txt"), []string{allowed}, true},
	}
	for _, c := range cases {
		_, err := ResolveMcpLocalPath(c.path, c.roots)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v wantErr=%v", c.name, err, c.wantErr)
		}
	}
}
