package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ys-ll/uniterm/backend/log"
)

// UpdateInfo is the result returned to the frontend.
type UpdateInfo struct {
	HasUpdate  bool   `json:"hasUpdate"`
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	ReleaseURL string `json:"releaseUrl"`
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Body    string `json:"body"`
}

type cacheEntry struct {
	Result    UpdateInfo `json:"result"`
	Source    string     `json:"source"`
	Timestamp time.Time  `json:"timestamp"`
	// ETag captured from the GitHub API response so a follow-up call
	// inside the disk-cache TTL window can send If-None-Match and let
	// GitHub/Gitee return 304 Not Modified, skipping the body decode.
	ETag string `json:"etag,omitempty"`
}

func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// parseSemver splits a version string into its numeric core parts and its
// optional pre-release identifiers, following SemVer 2.0.0. Build metadata
// (anything after '+') is discarded because it does not affect precedence.
func parseSemver(v string) (core []int, pre []string) {
	v = normalizeVersion(v)
	if idx := strings.Index(v, "+"); idx >= 0 {
		v = v[:idx]
	}
	var prePart string
	if idx := strings.Index(v, "-"); idx >= 0 {
		prePart = v[idx+1:]
		v = v[:idx]
	}
	for _, p := range strings.Split(v, ".") {
		n, _ := strconv.Atoi(p)
		core = append(core, n)
	}
	if prePart != "" {
		pre = strings.Split(prePart, ".")
	}
	return core, pre
}

// comparePre compares two pre-release identifier slices per SemVer 2.0.0 §11.4.
// Returns -1, 0, or 1. A version WITHOUT a pre-release has higher precedence
// than one WITH a pre-release; callers must handle that case before calling.
func comparePre(a, b []string) int {
	for i := 0; i < len(a) || i < len(b); i++ {
		if i >= len(a) {
			return -1
		}
		if i >= len(b) {
			return 1
		}
		an, aErr := strconv.Atoi(a[i])
		bn, bErr := strconv.Atoi(b[i])
		aNumeric := aErr == nil
		bNumeric := bErr == nil
		switch {
		case aNumeric && bNumeric:
			if an != bn {
				if an < bn {
					return -1
				}
				return 1
			}
		case aNumeric && !bNumeric:
			return -1
		case !aNumeric && bNumeric:
			return 1
		default:
			if a[i] != b[i] {
				if a[i] < b[i] {
					return -1
				}
				return 1
			}
		}
	}
	return 0
}

func versionGreater(latest, current string) bool {
	lc, lp := parseSemver(latest)
	cc, cp := parseSemver(current)
	for i := 0; i < len(lc) || i < len(cc); i++ {
		var ln, cn int
		if i < len(lc) {
			ln = lc[i]
		}
		if i < len(cc) {
			cn = cc[i]
		}
		if ln > cn {
			return true
		}
		if ln < cn {
			return false
		}
	}
	// Core versions equal: a version without a pre-release outranks one with.
	if len(lp) == 0 && len(cp) == 0 {
		return false
	}
	if len(lp) == 0 {
		return true
	}
	if len(cp) == 0 {
		return false
	}
	return comparePre(lp, cp) > 0
}

func shouldUpdate(current, latest string) bool {
	if current == "dev" {
		return true
	}
	return versionGreater(latest, current)
}

const cacheTTL = 5 * time.Minute

// sharedClient is a package-level *http.Client with keep-alive enabled.
// Reusing it avoids the per-call TCP+TLS handshake to api.github.com /
// gitee.com on every Check().
var (
	sharedClientOnce sync.Once
	sharedClient     *http.Client
)

func sharedHTTPClient() *http.Client {
	sharedClientOnce.Do(func() {
		sharedClient = &http.Client{Timeout: 10 * time.Second}
	})
	return sharedClient
}

func cachePath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "uniTerm", "update_cache.json")
}

func loadCache() *cacheEntry {
	path := cachePath()
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil
	}
	if time.Since(entry.Timestamp) > cacheTTL {
		return nil
	}
	return &entry
}

func saveCache(entry *cacheEntry) {
	path := cachePath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0755)
	data, _ := json.Marshal(entry)
	_ = os.WriteFile(path, data, 0600)
}

// Check compares the current version against the latest release from the given source.
func Check(currentVersion, source string) (*UpdateInfo, error) {
	if source == "" {
		source = "github"
	}

	if cached := loadCache(); cached != nil && cached.Source == source {
		result := cached.Result
		result.Current = currentVersion
		result.HasUpdate = shouldUpdate(currentVersion, result.Latest)
		log.Writef("[update] returning disk-cached result, source=%s, age=%s", source, time.Since(cached.Timestamp))
		return &result, nil
	}

	log.Writef("[update] Check called, current=%s, source=%s", currentVersion, source)

	apiURL := "https://api.github.com/repos/ys-ll/uniterm/releases/latest"
	if source == "gitee" {
		apiURL = "https://gitee.com/api/v5/repos/ys-l/uniterm/releases/latest"
	}

	// Reuse a package-level client with keep-alive; send the cached ETag
	// on the second-and-subsequent within-TTL call so GitHub/Gitee can
	// answer 304 without re-serializing the release.
	client := sharedHTTPClient()
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		log.Writef("[update] create request error: %v", err)
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "uniTerm")
	if prev := loadCache(); prev != nil && prev.Source == source && prev.ETag != "" {
		req.Header.Set("If-None-Match", prev.ETag)
	}

	resp, err := client.Do(req)
	if err != nil {
		log.Writef("[update] api request error: %v", err)
		return nil, fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()

	log.Writef("[update] %s API response status: %d", source, resp.StatusCode)

	// 304: nothing changed since our cached ETag. Refresh timestamp
	// so the disk TTL window slides forward and reuse the cached
	// payload.
	if resp.StatusCode == http.StatusNotModified {
		if prev := loadCache(); prev != nil && prev.Source == source {
			prev.Timestamp = time.Now()
			saveCache(prev)
			result := prev.Result
			result.Current = currentVersion
			result.HasUpdate = shouldUpdate(currentVersion, result.Latest)
			return &result, nil
		}
		// No prior cache to fall back to — fall through and treat as error.
		return nil, fmt.Errorf("unexpected status: 304 (no prior cache)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		log.Writef("[update] decode error: %v", err)
		return nil, fmt.Errorf("decode response: %w", err)
	}

	log.Writef("[update] latest=%s", release.TagName)

	releaseURL := "https://github.com/ys-ll/uniterm/releases/latest"
	if source == "gitee" {
		releaseURL = "https://gitee.com/ys-l/uniterm/releases/latest"
	}

	result := UpdateInfo{
		Current:    currentVersion,
		Latest:     release.TagName,
		ReleaseURL: releaseURL,
		HasUpdate:  shouldUpdate(currentVersion, release.TagName),
	}

	saveCache(&cacheEntry{
		Result:    result,
		Source:    source,
		Timestamp: time.Now(),
		ETag:      resp.Header.Get("ETag"),
	})

	return &result, nil
}
