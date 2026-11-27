package localsrv

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const outsideSecret = "TOP-SECRET-OUTSIDE-ROOT"

// mustSymlink creates a symlink or skips the test on Windows, where creating
// symlinks requires SeCreateSymbolicLinkPrivilege (Developer Mode or an
// elevated process). The containment code is still cross-compiled and vetted
// for Windows; only the fixture cannot be created on a stock Windows host.
func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires SeCreateSymbolicLinkPrivilege; covered by GOOS=windows go vet")
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

// writeOutsideRoot creates a sibling directory next to root (both under a
// shared parent) holding a secret file, and returns the secret directory.
func writeOutsideRoot(t *testing.T, root string) string {
	t.Helper()
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte(outsideSecret), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "leak.txt"), []byte("LEAKED-FILENAME"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "index.html"), []byte("<h1>OUTSIDE-INDEX</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return outside
}

// TestHTTPServer_SymlinkEscapeOutsideRoot is the regression test for the
// containment gap: a symlink inside the served root that resolves outside it
// must not expose the target's bytes or directory listing.
func TestHTTPServer_SymlinkEscapeOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := writeOutsideRoot(t, root)
	mustSymlink(t, outside, filepath.Join(root, "outdir"))
	mustSymlink(t, filepath.Join(outside, "secret.txt"), filepath.Join(root, "secret.txt"))

	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	status, body := httpGet(t, base+"/secret.txt")
	if status != http.StatusForbidden {
		t.Fatalf("GET /secret.txt = %d %q, want 403", status, body)
	}
	if strings.Contains(body, outsideSecret) {
		t.Fatalf("GET /secret.txt leaked outside bytes: %q", body)
	}

	status, body = httpGet(t, base+"/outdir/")
	if status != http.StatusForbidden {
		t.Fatalf("GET /outdir/ = %d %q, want 403", status, body)
	}
	if strings.Contains(body, "leak.txt") || strings.Contains(body, "LEAKED-FILENAME") {
		t.Fatalf("GET /outdir/ leaked outside listing: %q", body)
	}

	status, body = httpGet(t, base+"/outdir/secret.txt")
	if status != http.StatusForbidden {
		t.Fatalf("GET /outdir/secret.txt = %d %q, want 403", status, body)
	}
	if strings.Contains(body, outsideSecret) {
		t.Fatalf("GET /outdir/secret.txt leaked outside bytes: %q", body)
	}
}

// TestHTTPServer_SymlinkEscapeViaIndex verifies that a directory symlink
// pointing outside the root cannot be used to serve an outside index.html
// when directory listings are disabled.
func TestHTTPServer_SymlinkEscapeViaIndex(t *testing.T) {
	root := t.TempDir()
	outside := writeOutsideRoot(t, root)
	mustSymlink(t, outside, filepath.Join(root, "evil"))

	base, stop := startTestHTTPServer(t, root, freePort(t), false)
	defer stop()

	status, body := httpGet(t, base+"/evil/")
	if status != http.StatusForbidden {
		t.Fatalf("GET /evil/ = %d %q, want 403", status, body)
	}
	if strings.Contains(body, "OUTSIDE-INDEX") {
		t.Fatalf("GET /evil/ leaked outside index bytes: %q", body)
	}
}

// TestHTTPServer_SymlinkInsideRoot verifies that symlinks resolving inside
// the served root keep working for files and directories.
func TestHTTPServer_SymlinkInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("inside-real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "child.txt"), []byte("inside-child"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(root, "real.txt"), filepath.Join(root, "link.txt"))
	mustSymlink(t, filepath.Join(root, "sub"), filepath.Join(root, "sublink"))

	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	status, body := httpGet(t, base+"/link.txt")
	if status != http.StatusOK || body != "inside-real" {
		t.Fatalf("GET /link.txt = %d %q, want 200 inside-real", status, body)
	}
	status, body = httpGet(t, base+"/sublink/child.txt")
	if status != http.StatusOK || body != "inside-child" {
		t.Fatalf("GET /sublink/child.txt = %d %q, want 200 inside-child", status, body)
	}
	status, body = httpGet(t, base+"/sublink/")
	if status != http.StatusOK || !strings.Contains(body, "child.txt") {
		t.Fatalf("GET /sublink/ = %d %q, want listing with child.txt", status, body)
	}
}

// TestHTTPServer_SymlinkChain verifies chains that stay inside the root and
// chains that hop outside it.
func TestHTTPServer_SymlinkChain(t *testing.T) {
	root := t.TempDir()
	outside := writeOutsideRoot(t, root)
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("chain-inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, "real.txt", filepath.Join(root, "hop1"))
	mustSymlink(t, "hop1", filepath.Join(root, "hop2"))
	mustSymlink(t, "hop2", filepath.Join(root, "hop3"))
	mustSymlink(t, filepath.Join(outside, "secret.txt"), filepath.Join(root, "out-hop1"))
	mustSymlink(t, "out-hop1", filepath.Join(root, "out-hop2"))

	base, stop := startTestHTTPServer(t, root, freePort(t), false)
	defer stop()

	status, body := httpGet(t, base+"/hop3")
	if status != http.StatusOK || body != "chain-inside" {
		t.Fatalf("GET /hop3 = %d %q, want 200 chain-inside", status, body)
	}
	status, body = httpGet(t, base+"/out-hop2")
	if status != http.StatusForbidden {
		t.Fatalf("GET /out-hop2 = %d %q, want 403", status, body)
	}
	if strings.Contains(body, outsideSecret) {
		t.Fatalf("GET /out-hop2 leaked outside bytes: %q", body)
	}
}

// TestHTTPServer_DanglingSymlink verifies a dangling symlink reports 404.
func TestHTTPServer_DanglingSymlink(t *testing.T) {
	root := t.TempDir()
	mustSymlink(t, "missing-target", filepath.Join(root, "dead"))

	base, stop := startTestHTTPServer(t, root, freePort(t), false)
	defer stop()

	status, _ := httpGet(t, base+"/dead")
	if status != http.StatusNotFound {
		t.Fatalf("GET /dead = %d, want 404", status)
	}
}

// TestHTTPServer_DotfilesAndRange verifies dotfiles stay reachable and range
// requests keep working through the containment path.
func TestHTTPServer_DotfilesAndRange(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("dotfile-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "data.bin"), []byte("0123456789abcdefghij"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	status, body := httpGet(t, base+"/.env")
	if status != http.StatusOK || body != "dotfile-content" {
		t.Fatalf("GET /.env = %d %q, want 200 dotfile-content", status, body)
	}

	req, err := http.NewRequest(http.MethodGet, base+"/data.bin", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-4")
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("GET /data.bin: %v", err)
	}
	defer resp.Body.Close()
	partial, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(partial) != "01234" {
		t.Fatalf("GET /data.bin Range = %d %q, want 206 01234", resp.StatusCode, partial)
	}
}

// TestHTTPServer_RootIsSymlink verifies that pointing the served root at a
// symlink (the macOS /var -> /private/var pattern) still works.
func TestHTTPServer_RootIsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires SeCreateSymbolicLinkPrivilege; covered by GOOS=windows go vet")
	}
	realRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(realRoot, "page.txt"), []byte("root-via-link"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkRoot := filepath.Join(t.TempDir(), "rootlink")
	mustSymlink(t, realRoot, linkRoot)

	base, stop := startTestHTTPServer(t, linkRoot, freePort(t), false)
	defer stop()

	status, body := httpGet(t, base+"/page.txt")
	if status != http.StatusOK || body != "root-via-link" {
		t.Fatalf("GET /page.txt = %d %q, want 200 root-via-link", status, body)
	}
}

// TestHTTPServer_RejectsTraversal verifies raw and encoded traversal never
// returns outside bytes.
func TestHTTPServer_RejectsTraversal(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeOutsideRoot(t, root)

	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	paths := []string{
		"/../outside/secret.txt",
		"/%2e%2e/outside/secret.txt",
		"/..%2foutside%2fsecret.txt",
		"/%2e%2e%2foutside%2fsecret.txt",
		"/%00",
		"/%00/secret.txt",
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, p := range paths {
		u, err := url.Parse(base + p)
		if err != nil {
			t.Fatalf("parse %q: %v", p, err)
		}
		req := &http.Request{Method: http.MethodGet, URL: u, Host: u.Host, Header: http.Header{}}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %q: %v", p, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %q = %d, want 403 or 404", p, resp.StatusCode)
		}
		if strings.Contains(string(body), outsideSecret) {
			t.Fatalf("GET %q leaked outside bytes: %q", p, body)
		}
	}
}

// TestHTTPServer_ConcurrentOpenAndRemove exercises the open/stat path while
// files and symlink targets change underneath it. A panic in any handler
// crashes the test binary; a leaked outside byte fails the test.
func TestHTTPServer_ConcurrentOpenAndRemove(t *testing.T) {
	root := t.TempDir()
	outside := writeOutsideRoot(t, root)
	if err := os.WriteFile(filepath.Join(root, "inside.txt"), []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, filepath.Join(root, "inside.txt"), filepath.Join(root, "swap"))

	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	client := &http.Client{Timeout: 5 * time.Second}
	var wg sync.WaitGroup
	const workers, rounds = 8, 25

	// Flip the symlink between an inside file and the outside secret while
	// requests are in flight: os.Root must either follow the inside target or
	// refuse the escape, never leak.
	wg.Add(1)
	go func() {
		defer wg.Done()
		link := filepath.Join(root, "swap")
		for i := 0; i < rounds; i++ {
			_ = os.Remove(link)
			if i%2 == 0 {
				_ = os.Symlink(filepath.Join(outside, "secret.txt"), link)
			} else {
				_ = os.Symlink(filepath.Join(root, "inside.txt"), link)
				_ = os.WriteFile(filepath.Join(root, "volatile.txt"), []byte("volatile"), 0o644)
			}
			_ = os.Remove(filepath.Join(root, "volatile.txt"))
		}
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				for _, p := range []string{"/swap", "/volatile.txt", "/", "/inside.txt"} {
					resp, err := client.Get(base + p)
					if err != nil {
						continue
					}
					body, _ := io.ReadAll(resp.Body)
					resp.Body.Close()
					if strings.Contains(string(body), outsideSecret) {
						t.Errorf("GET %s leaked outside bytes", p)
					}
				}
			}
		}()
	}
	wg.Wait()
}

// TestHTTPServer_NotFoundForMissingFile pins the 404 mapping.
func TestHTTPServer_NotFoundForMissingFile(t *testing.T) {
	root := t.TempDir()
	base, stop := startTestHTTPServer(t, root, freePort(t), false)
	defer stop()
	if status, body := httpGet(t, base+"/nope.txt"); status != http.StatusNotFound {
		t.Fatalf("GET /nope.txt = %d %q, want 404", status, body)
	}
}
