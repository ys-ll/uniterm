package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func tarGzBytes(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: name, Mode: 0755, Size: int64(len(content)), Typeflag: tar.TypeReg}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func useTestUpdateBase(t *testing.T, base string) {
	t.Helper()
	t.Setenv("UNITERM_UPDATE_API_BASE", base)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate the release disk cache
	t.Setenv("XDG_CACHE_HOME", t.TempDir())  // isolate download staging
}

// TestDownloadFailsClosedWithoutChecksum is the regression test for the
// fail-open update path: a release asset with no checksums.txt entry used to
// download and stage without any integrity check.
func TestDownloadFailsClosedWithoutChecksum(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()
	useTestUpdateBase(t, srv.URL)

	m := NewManager()
	cand := UpdateAsset{
		Name:   "uniterm-windows-amd64-installer-v9.9.9.exe",
		URL:    srv.URL + "/uniterm-windows-amd64-installer-v9.9.9.exe",
		Source: "github",
		// SHA256 intentionally empty: the release has no checksums entry.
	}
	pend, err := m.Download([]UpdateAsset{cand}, nil)
	if err == nil {
		if pend != nil {
			_ = os.RemoveAll(pend.StageDir)
		}
		t.Fatal("Download accepted an asset without a sha256; want a fail-closed error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "sha256") &&
		!strings.Contains(strings.ToLower(err.Error()), "checksum") {
		t.Errorf("error should explain the missing checksum, got: %v", err)
	}
	if !strings.Contains(err.Error(), "install it manually") {
		t.Errorf("error should offer a manual link, got: %v", err)
	}
	if m.Pending() != nil {
		t.Errorf("no pending update must remain, got %+v", m.Pending())
	}
	if got := atomic.LoadInt32(&hits); got != 0 {
		t.Errorf("network must not be touched before checksum rejection, got %d requests", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(os.Getenv("XDG_CACHE_HOME"), "uniTerm", "updates")); len(entries) != 0 {
		t.Errorf("no staging directory must be created, found %d entries", len(entries))
	}
}

func TestDownloadRejectsMismatchedChecksum(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()
	useTestUpdateBase(t, srv.URL)

	m := NewManager()
	cand := UpdateAsset{
		Name:   "uniterm-windows-amd64-installer-v9.9.9.exe",
		URL:    srv.URL + "/uniterm-windows-amd64-installer-v9.9.9.exe",
		SHA256: strings.Repeat("0", 64),
		Source: "github",
	}
	pend, err := m.Download([]UpdateAsset{cand}, nil)
	if err == nil {
		if pend != nil {
			_ = os.RemoveAll(pend.StageDir)
		}
		t.Fatal("Download accepted a payload whose sha256 does not match")
	}
	if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("want sha256 mismatch error, got: %v", err)
	}
	if m.Pending() != nil {
		t.Errorf("no pending update must remain, got %+v", m.Pending())
	}
}

func TestDownloadRejectsOffHostFirstHop(t *testing.T) {
	m := NewManager()
	cand := UpdateAsset{
		Name:   "uniterm-windows-amd64-installer-v9.9.9.exe",
		URL:    "https://evil.example/uniterm-windows-amd64-installer-v9.9.9.exe",
		SHA256: strings.Repeat("a", 64),
		Source: "github",
	}
	if _, err := m.Download([]UpdateAsset{cand}, nil); err == nil {
		t.Fatal("Download accepted an off-host first hop")
	} else if !strings.Contains(err.Error(), "official release download") {
		t.Errorf("want official-host rejection, got: %v", err)
	}
	if m.Pending() != nil {
		t.Errorf("no pending update must remain, got %+v", m.Pending())
	}
}

func TestDownloadRejectsOffHostRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example/payload", http.StatusFound)
	}))
	defer srv.Close()
	useTestUpdateBase(t, srv.URL)

	m := NewManager()
	cand := UpdateAsset{
		Name:   "uniterm-windows-amd64-installer-v9.9.9.exe",
		URL:    srv.URL + "/releases/download/v9.9.9/uniterm-windows-amd64-installer-v9.9.9.exe",
		SHA256: strings.Repeat("a", 64),
		Source: "github",
	}
	if _, err := m.Download([]UpdateAsset{cand}, nil); err == nil {
		t.Fatal("Download followed a redirect to an off-host URL")
	} else if !strings.Contains(err.Error(), "official host") {
		t.Errorf("want redirect-host rejection, got: %v", err)
	}
}

func TestDownloadHappyPathStagesVerifiedPayload(t *testing.T) {
	binaryContent := []byte("new-uniterm-binary")
	payload := tarGzBytes(t, "uniterm", binaryContent)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	useTestUpdateBase(t, srv.URL)

	m := NewManager()
	cand := UpdateAsset{
		Name:   "uniterm-" + runtime.GOOS + "-" + runtime.GOARCH + "-v9.9.9.tar.gz",
		URL:    srv.URL + "/payload.tar.gz",
		SHA256: sha256Hex(payload),
		Source: "github",
	}
	pend, err := m.Download([]UpdateAsset{cand}, nil)
	if err != nil {
		t.Fatalf("verified download failed: %v", err)
	}
	defer os.RemoveAll(pend.StageDir)
	if pend.NewBinary == "" {
		t.Fatal("payload was not extracted")
	}
	if want := sha256Hex(binaryContent); pend.NewBinarySHA256 != want {
		t.Errorf("NewBinarySHA256 = %q, want %q", pend.NewBinarySHA256, want)
	}
	if got, err := os.ReadFile(pend.NewBinary); err != nil || !bytes.Equal(got, binaryContent) {
		t.Errorf("extracted binary content mismatch: %q, err=%v", got, err)
	}
}

func TestFetchChecksumsAbsentVsFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid.txt":
			_, _ = w.Write([]byte(strings.Repeat("a", 64) + "  uniterm-linux-amd64-v1.tar.gz\n"))
		case "/garbage.txt":
			_, _ = w.Write([]byte("not a checksum file\n"))
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	t.Setenv("UNITERM_UPDATE_API_BASE", srv.URL)

	absent, err := fetchChecksums(&release{})
	if err != nil {
		t.Errorf("absent checksums must not be an error: %v", err)
	}
	if len(absent) != 0 {
		t.Errorf("absent checksums must be empty, got %v", absent)
	}

	if _, err := fetchChecksums(&release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/boom.txt"}}}); err == nil {
		t.Error("failed checksums fetch must return an error")
	}
	if _, err := fetchChecksums(&release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/garbage.txt"}}}); err == nil {
		t.Error("malformed checksums payload must return an error")
	}
	if _, err := fetchChecksums(&release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: "https://evil.example/checksums.txt"}}}); err == nil {
		t.Error("off-host checksums URL must be rejected")
	}

	sums, err := fetchChecksums(&release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/valid.txt"}}})
	if err != nil {
		t.Fatalf("valid checksums failed: %v", err)
	}
	if got := sums["uniterm-linux-amd64-v1.tar.gz"]; got != strings.Repeat("a", 64) {
		t.Errorf("parsed checksum = %q", got)
	}
}

func TestChecksumsForFallsBackToMirror(t *testing.T) {
	hash := strings.Repeat("b", 64)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/valid.txt":
			_, _ = w.Write([]byte(hash + "  uniterm-linux-amd64-v1.tar.gz\n"))
		default:
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	t.Setenv("UNITERM_UPDATE_API_BASE", srv.URL)

	valid := &release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/valid.txt"}}}
	failed := &release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/boom.txt"}}}

	// Primary fetch failure falls back to the mirror.
	if got := checksumsFor(failed, valid); got["uniterm-linux-amd64-v1.tar.gz"] != hash {
		t.Errorf("expected mirror checksums on primary failure, got %v", got)
	}
	// Primary absent falls back to the mirror.
	if got := checksumsFor(&release{}, valid); got["uniterm-linux-amd64-v1.tar.gz"] != hash {
		t.Errorf("expected mirror checksums when primary is absent, got %v", got)
	}
	// Primary present wins over the mirror.
	primary := &release{Assets: []releaseAsset{{Name: "checksums.txt", BrowserDownloadURL: srv.URL + "/valid.txt"}}}
	if got := checksumsFor(primary, failed); got["uniterm-linux-amd64-v1.tar.gz"] != hash {
		t.Errorf("primary checksums should win, got %v", got)
	}
	// No usable checksums anywhere yields an empty map, never a panic.
	if got := checksumsFor(&release{}, nil); len(got) != 0 {
		t.Errorf("no checksums anywhere should be empty, got %v", got)
	}
}

func TestCheckKeepsAssetsForReleaseWithChecksums(t *testing.T) {
	payload := []byte("package-payload")
	payloadSum := sha256Hex(payload)
	assetName := "uniterm-" + runtime.GOOS + "-" + runtime.GOARCH + "-v99.0.0.tar.gz"
	debName := "uniterm-linux-" + runtime.GOARCH + "-v99.0.0.deb"

	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/releases/latest":
			rel := release{
				TagName: "v99.0.0",
				Assets: []releaseAsset{
					{Name: "checksums.txt", BrowserDownloadURL: base + "/checksums.txt"},
					{Name: assetName, BrowserDownloadURL: base + "/payload"},
					{Name: debName, BrowserDownloadURL: base + "/payload"},
				},
			}
			_ = json.NewEncoder(w).Encode(rel)
		case "/checksums.txt":
			_, _ = w.Write([]byte(payloadSum + "  " + assetName + "\n" + payloadSum + "  " + debName + "\n"))
		case "/payload":
			_, _ = w.Write(payload)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	base = srv.URL
	useTestUpdateBase(t, srv.URL)

	info, err := Check("v1.0.0", "github")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !info.HasUpdate {
		t.Fatal("expected an update")
	}
	if len(info.Assets) == 0 {
		t.Fatal("a release with checksums.txt must still yield downloadable assets")
	}
	for _, a := range info.Assets {
		if a.SHA256 != payloadSum {
			t.Errorf("asset %s has sha256 %q, want %q", a.Name, a.SHA256, payloadSum)
		}
	}
	if err := allowedDownloadURL(info.Assets[0].URL); err != nil {
		t.Errorf("candidate URL should be allowed for the configured base: %v", err)
	}
}

func TestAllowedDownloadURLRules(t *testing.T) {
	ok := []string{
		"https://github.com/ys-ll/uniterm/releases/download/v1.0.0/uniterm-linux-amd64.tar.gz",
		"https://gitee.com/ys-l/uniterm/releases/download/v1.0.0/uniterm-linux-amd64.tar.gz",
	}
	for _, u := range ok {
		if err := allowedDownloadURL(u); err != nil {
			t.Errorf("allowedDownloadURL(%q) = %v, want nil", u, err)
		}
	}
	bad := []string{
		"http://github.com/ys-ll/uniterm/releases/download/v1/x",
		"https://github.com/evil/releases/download/v1/x",
		"https://github.com/ys-ll/uniterm/releases/latest",
		"https://gitee.com/ys-l/other/releases/download/v1/x",
		"https://evil.example/x",
		"https://github.com.evil.example/ys-ll/uniterm/releases/download/v1/x",
	}
	for _, u := range bad {
		if err := allowedDownloadURL(u); err == nil {
			t.Errorf("allowedDownloadURL(%q) = nil, want rejection", u)
		}
	}

	redirects := []string{
		"https://release-assets.githubusercontent.com/github-production-release-asset/1/x",
		"https://objects.githubusercontent.com/x",
		"https://foruda.gitee.com/x",
		"https://gitee.com/x",
	}
	for _, u := range redirects {
		if err := allowedRedirectURL(u); err != nil {
			t.Errorf("allowedRedirectURL(%q) = %v, want nil", u, err)
		}
	}
	if err := allowedRedirectURL("https://evil.example/x"); err == nil {
		t.Error("off-host redirect must be rejected")
	}
	if err := allowedRedirectURL("http://release-assets.githubusercontent.com/x"); err == nil {
		t.Error("plain-HTTP redirect must be rejected")
	}
}

func TestAllowedURLTestBaseLoopbackException(t *testing.T) {
	t.Setenv("UNITERM_UPDATE_API_BASE", "http://127.0.0.1:18765")
	for _, u := range []string{
		"http://127.0.0.1:18765/releases/download/v1/x",
		"http://127.0.0.1:18765/checksums.txt",
	} {
		if err := allowedDownloadURL(u); err != nil {
			t.Errorf("configured test base should allow %q: %v", u, err)
		}
		if err := allowedRedirectURL(u); err != nil {
			t.Errorf("configured test base redirect should allow %q: %v", u, err)
		}
	}
	if err := allowedDownloadURL("http://127.0.0.2:18765/releases/download/v1/x"); err == nil {
		t.Error("a different HTTP host must not inherit the exception")
	}
	// With an HTTPS base the plain-HTTP exception is not granted.
	t.Setenv("UNITERM_UPDATE_API_BASE", "https://mirror.example")
	if err := allowedDownloadURL("http://mirror.example/releases/download/v1/x"); err == nil {
		t.Error("HTTP must be rejected when the configured base is HTTPS")
	}
	if err := allowedDownloadURL("https://mirror.example/releases/download/v1/x"); err != nil {
		t.Errorf("HTTPS configured base should be allowed: %v", err)
	}
}

func TestApplyBinaryRejectsTamperedExtractedBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "uniterm")
	if err := os.WriteFile(bin, []byte("verified-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	want := sha256Hex([]byte("verified-binary"))

	// Happy path: the verified descriptor is rewound and exposes the bytes.
	f, err := openVerifiedBinary(bin, want)
	if err != nil {
		t.Fatalf("openVerifiedBinary: %v", err)
	}
	data, err := os.ReadFile(bin)
	f.Close()
	if err != nil || string(data) != "verified-binary" {
		t.Errorf("unexpected staged content: %q err=%v", data, err)
	}

	// Tamper after extraction: the pre-recorded hash no longer matches.
	if err := os.WriteFile(bin, []byte("tampered-binary"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := applyBinary(bin, want, nil); err == nil {
		t.Fatal("applyBinary accepted a binary changed after extraction")
	} else if !strings.Contains(err.Error(), "sha256 mismatch") {
		t.Errorf("want sha256 mismatch, got: %v", err)
	}

	// Fail closed with no recorded hash at all.
	if err := applyBinary(bin, "", nil); err == nil {
		t.Fatal("applyBinary accepted a binary without a recorded hash")
	}

	// A rejected apply must not have created a backup next to the running exe.
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(exe + ".uniterm-old"); statErr == nil {
		t.Error("rejected apply left a backup next to the running executable")
	}
}
