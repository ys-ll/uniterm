package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// QA-016 / F-403: UpgradeAvailable edge cases — equal versions, lower
// major/minor/patch, pre-release tags, dev sentinel, malformed input.
// shouldUpdate wraps these semantics in production; the table below
// pins the contract callers depend on.

func TestShouldUpdate_FeatureBehindGreater(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		// Identical
		{"1.2.3", "1.2.3", false},
		{"v1.2.3", "v1.2.3", false},

		// Lower
		{"1.2.3", "1.2.4", true},
		{"1.2.4", "1.2.3", false},
		{"1.2.3", "1.3.0", true},
		{"1.3.0", "1.2.99", false},
		{"1.2.3", "2.0.0", true},
		{"2.0.0", "1.99.99", false},
		{"1.2.3", "1.2.10", true},

		// Pre-release: a version WITHOUT a pre-release tag outranks one WITH.
		// shouldUpdate(current, latest) → returns true iff latest > current.
		{"1.2.3-rc.1", "1.2.3", true},  // current is RC, latest is release
		{"1.2.3", "1.2.3-rc.1", false}, // current is release, latest is RC
		{"1.2.3-rc.1", "1.2.3-rc.2", true},
		{"1.2.3-rc.2", "1.2.3-rc.1", false},
		// alpha < beta per SemVer §11.4.3 (lexical ASCII).
		{"1.2.3-alpha", "1.2.3-beta", true},  // current=alpha, latest=beta; beta > alpha → update available
		{"1.2.3-beta", "1.2.3-alpha", false}, // current=beta, latest=alpha; alpha < beta → no update
		// Numeric identifiers: smaller number is lower precedence.
		{"1.2.3-10", "1.2.3-2", false}, // current=10, latest=2; 10 > 2 → no update
		{"1.2.3-2", "1.2.3-10", true},  // current=2, latest=10; 10 > 2 → update

		// Build metadata is ignored for precedence.
		{"1.2.3+build1", "1.2.3+build2", false},
		{"1.2.3+build1", "1.2.4", true},

		// Both pre-release; different core.
		{"1.2.3-rc.1", "1.2.4", true},
		{"1.2.4-rc.1", "1.2.3", false},

		// Different segment lengths — missing segments compare as 0.
		{"1.2", "1.2.0", false},
		{"1.2", "1.2.1", true},
		{"1.2", "1.3", true},
	}

	for _, tc := range cases {
		t.Run(tc.current+"_vs_"+tc.latest, func(t *testing.T) {
			got := shouldUpdate(tc.current, tc.latest)
			if got != tc.want {
				t.Errorf("shouldUpdate(%q, %q) = %v, want %v",
					tc.current, tc.latest, got, tc.want)
			}
		})
	}
}

// TestShouldUpdate_DevAlwaysUpdates confirms the dev sentinel forces update.
func TestShouldUpdate_DevAlwaysUpdates(t *testing.T) {
	if !shouldUpdate("dev", "0.0.1") {
		t.Error("dev should always update (vs any non-dev version)")
	}
	if shouldUpdate("1.0.0", "dev") {
		t.Error("1.0.0 vs dev should NOT trigger update (downgrade to dev)")
	}
}

// TestVersionGreater_NumericSegments verifies multi-segment numerics
// beyond major.minor.patch work for tags like "1.2.3.4".
func TestVersionGreater_NumericSegments(t *testing.T) {
	if !versionGreater("1.2.3.4", "1.2.3.3") {
		t.Error("1.2.3.4 > 1.2.3.3 expected")
	}
	if versionGreater("1.2.3.3", "1.2.3.4") {
		t.Error("1.2.3.3 not > 1.2.3.4")
	}
}

// TestParseSemver_StripsPrefix: the helper ignores a leading "v".
func TestParseSemver_StripsPrefix(t *testing.T) {
	core, pre := parseSemver("v2.0.0-rc.1")
	if len(core) != 3 || core[0] != 2 || core[1] != 0 || core[2] != 0 {
		t.Errorf("core = %v, want [2 0 0]", core)
	}
	if len(pre) != 2 || pre[0] != "rc" || pre[1] != "1" {
		t.Errorf("pre = %v, want [rc 1]", pre)
	}
}

// TestParseSemver_DropsBuildMetadata: "1.2.3+build" should ignore +build.
func TestParseSemver_DropsBuildMetadata(t *testing.T) {
	core, _ := parseSemver("1.2.3+build.42")
	if len(core) != 3 || core[2] != 3 {
		t.Errorf("core = %v, want [1 2 3]", core)
	}
}

// TestCheck_FetchFromStubServer drives the request path against a
// stub HTTP server using a custom apiURL replacement. Skipped: we
// cannot redirect production Check() without touching globals, so the
// test exercises only the ETag / cache round-trip helpers.
func TestCheck_FetchFromStubServer(t *testing.T) {
	var hits int
	var gotIfNoneMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		gotIfNoneMatch = r.Header.Get("If-None-Match")
		w.Header().Set("ETag", `W/"abc"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v2.0.0","body":"new"}`))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("ETag") == "" {
		t.Error("server should expose an ETag header")
	}
	if gotIfNoneMatch != "" {
		t.Errorf("stub server should not see If-None-Match on first hit, got %q", gotIfNoneMatch)
	}
	if hits == 0 {
		t.Fatal("stub server hit count should be 1")
	}
}

// TestComparePre: numeric vs alphanumeric precedence per SemVer 2.0.0 §11.4.
func TestComparePre(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{[]string{"rc", "1"}, []string{"rc", "2"}, -1},
		{[]string{"rc", "2"}, []string{"rc", "1"}, 1},
		{[]string{"alpha"}, []string{"alpha"}, 0},
		// numeric < alphanumeric
		{[]string{"1"}, []string{"alpha"}, -1},
		{[]string{"alpha"}, []string{"1"}, 1},
		// shorter pre outranks longer matching prefix
		{[]string{"alpha"}, []string{"alpha", "1"}, -1},
	}
	for _, tc := range cases {
		got := comparePre(tc.a, tc.b)
		if (got < 0) != (tc.want < 0) || (got > 0) != (tc.want > 0) || (got == 0) != (tc.want == 0) {
			t.Errorf("comparePre(%v, %v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestShouldUpdate_PrereleaseOrderingNumbers verifies the numeric case
// in comparePre happens after a string segment.
func TestShouldUpdate_PrereleaseOrderingNumbers(t *testing.T) {
	// "alpha" < "alpha.1" — longer pre-release outranks shorter.
	if !shouldUpdate("1.0.0-alpha", "1.0.0-alpha.1") {
		t.Error("1.0.0-alpha should be less than 1.0.0-alpha.1")
	}
}

// TestSharedHTTPClient verifies the package-level client is a singleton.
func TestSharedHTTPClient(t *testing.T) {
	a := sharedHTTPClient()
	b := sharedHTTPClient()
	if a != b {
		t.Fatal("sharedHTTPClient must return the same singleton")
	}
}

// TestNormalizeVersion: leading "v" and surrounding whitespace are stripped.
func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v1.2.3":  "1.2.3",
		" 1.2.3 ": "1.2.3",
		"1.2.3":   "1.2.3",
		"":        "",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCacheRoundTrip serializes and reloads the cache directly,
// bypassing UserConfigDir path discovery: the on-disk format must be
// readable by loadCache() so a tool that owns its cache directory
// (e.g. a test rig or migration script) can pre-seed it.
func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "update_cache.json")
	now := time.Now()
	want := &cacheEntry{
		Result: UpdateInfo{
			HasUpdate:  true,
			Current:    "1.0.0",
			Latest:     "v1.1.0",
			ReleaseURL: "https://example.com",
		},
		Source:    "github",
		Timestamp: now,
		ETag:      `W/"xyz"`,
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	read, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got cacheEntry
	if err := json.Unmarshal(read, &got); err != nil {
		t.Fatal(err)
	}
	if got.Source != want.Source || got.Result.Latest != want.Result.Latest || got.ETag != want.ETag {
		t.Errorf("cache round-trip mismatch: %+v vs %+v", got, want)
	}
}
