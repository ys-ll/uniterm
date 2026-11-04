package localsrv

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startTestHTTPServer starts the HTTP local service on an ephemeral port and
// returns its base URL plus a stop func.
func startTestHTTPServer(t *testing.T, root string, port int, listDir bool) (string, func()) {
	t.Helper()
	m := NewManager()
	id, err := m.StartHTTPServer(HTTPConfig{
		RootDir: root,
		Port:    port,
		ListDir: listDir,
	})
	if err != nil {
		t.Fatalf("StartHTTPServer: %v", err)
	}
	return fmt.Sprintf("http://127.0.0.1:%d", port), func() { m.Stop(id) }
}

// httpGet fetches a URL, following redirects, and returns status + body.
func httpGet(t *testing.T, url string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(body)
}

// TestHTTPServer_IndexFallbackWithoutListDir verifies that with directory
// browsing disabled, a directory request still serves index.html (static
// site mode) instead of 404, and 404s when no index.html exists.
func TestHTTPServer_IndexFallbackWithoutListDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>uniterm-home</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "index.html"), []byte("<h1>uniterm-sub</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "asset.txt"), []byte("asset"), 0o644); err != nil {
		t.Fatal(err)
	}

	base, stop := startTestHTTPServer(t, root, freePort(t), false)
	defer stop()

	// Root serves index.html.
	status, body := httpGet(t, base+"/")
	if status != 200 || !strings.Contains(body, "uniterm-home") {
		t.Fatalf("GET / = %d %q, want 200 with index content", status, body)
	}
	// Subdirectory serves its own index.html.
	status, body = httpGet(t, base+"/sub/")
	if status != 200 || !strings.Contains(body, "uniterm-sub") {
		t.Fatalf("GET /sub/ = %d %q, want 200 with sub index content", status, body)
	}
	// Directory without index.html still 404s.
	status, _ = httpGet(t, base+"/sub/empty/")
	if status != 404 {
		t.Fatalf("GET /sub/empty/ = %d, want 404 when no index.html", status)
	}
	// Regular files are still served.
	status, body = httpGet(t, base+"/asset.txt")
	if status != 200 || !strings.Contains(body, "asset") {
		t.Fatalf("GET /asset.txt = %d %q, want 200", status, body)
	}
	// Directory URL without trailing slash redirects to the slash form so
	// relative links inside index.html resolve correctly.
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Get(base + "/sub")
	if err != nil {
		t.Fatalf("GET /sub: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET /sub = %d, want 301 trailing-slash redirect", resp.StatusCode)
	}
}

// TestHTTPServer_ListDirBehavior verifies the listing mode: directory
// requests render the listing, and index.html stays reachable under its
// explicit name.
func TestHTTPServer_ListDirBehavior(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>uniterm-home</h1>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "noindex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "noindex", "child.txt"), []byte("child"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, stop := startTestHTTPServer(t, root, freePort(t), true)
	defer stop()

	// / shows the listing even when an index.html exists.
	status, body := httpGet(t, base+"/")
	if status != 200 || !strings.Contains(body, "Directory of") || !strings.Contains(body, "index.html") {
		t.Fatalf("GET / = %d %q, want directory listing", status, body)
	}
	// /index.html serves the page itself.
	status, body = httpGet(t, base+"/index.html")
	if status != 200 || !strings.Contains(body, "uniterm-home") {
		t.Fatalf("GET /index.html = %d %q, want index content", status, body)
	}
	// Directory without index.html lists too.
	status, body = httpGet(t, base+"/noindex/")
	if status != 200 || !strings.Contains(body, "Directory of") || !strings.Contains(body, "child.txt") {
		t.Fatalf("GET /noindex/ = %d %q, want directory listing", status, body)
	}
}
