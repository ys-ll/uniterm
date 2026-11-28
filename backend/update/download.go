package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ys-ll/uniterm/backend/log"
)

// downloadHTTPClient tolerates large assets: no overall timeout, but bounded
// connect/TLS phases. Callers own their cancellation semantics.
var (
	downloadClientOnce sync.Once
	downloadClient     *http.Client
)

func downloadHTTPClient() *http.Client {
	downloadClientOnce.Do(func() {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSHandshakeTimeout = 20 * time.Second
		downloadClient = &http.Client{
			Transport: transport,
			Timeout:   0, // stream large assets without an overall deadline
			// Every redirect hop must stay on an official host; a tampered
			// API payload cannot bounce the download off-site.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return errors.New("stopped after 10 redirects")
				}
				return allowedRedirectURL(req.URL.String())
			},
		}
	})
	return downloadClient
}

// updateCacheDir returns (and creates) the per-user staging directory for
// update downloads.
func updateCacheDir() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "uniTerm", "updates")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// packageStageDir creates a staging directory for a .deb/.rpm payload that the
// package manager can read. The per-user cache (~/.cache) is commonly 0700,
// and apt hands local files to its unprivileged _apt sandbox user, so system
// packages are staged under the system temp dir with world-readable modes
// instead. The directory stays writable only by the current user.
func packageStageDir() (string, error) {
	stage, err := os.MkdirTemp("", "uniterm-update-*")
	if err != nil {
		return "", err
	}
	if err := os.Chmod(stage, 0755); err != nil {
		_ = os.RemoveAll(stage)
		return "", err
	}
	return stage, nil
}

// isSystemPackage reports whether an asset kind is installed through the
// system package manager rather than replaced in place.
func isSystemPackage(kind string) bool {
	return kind == "deb" || kind == "rpm"
}

// parseChecksums parses a GNU `sha256sum`-style checksums.txt into
// filename → lowercase hex digest.
func parseChecksums(data []byte) map[string]string {
	sums := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(fields[0])
		name := strings.TrimPrefix(fields[len(fields)-1], "*")
		if len(hash) != 64 {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		sums[name] = hash
	}
	return sums
}

// Download tries each candidate in order until one downloads and verifies.
// Progress is reported through onProgress (may be nil).
func (m *Manager) Download(candidates []UpdateAsset, onProgress func(Progress)) (*PendingUpdate, error) {
	m.Clear()
	var lastErr error
	for i, cand := range candidates {
		log.Writef("[update] trying candidate %d/%d: %s (%s)", i+1, len(candidates), cand.Name, cand.Source)
		pend, err := m.downloadOne(cand, onProgress)
		if err == nil {
			return pend, nil
		}
		lastErr = err
		log.Writef("[update] candidate failed: %v", err)
	}
	if lastErr == nil {
		lastErr = errors.New("no download candidates")
	}
	return nil, lastErr
}

func (m *Manager) downloadOne(cand UpdateAsset, onProgress func(Progress)) (_ *PendingUpdate, err error) {
	// Fail closed before touching the network or creating a staging directory:
	// a release without a checksum entry is never installable.
	if strings.TrimSpace(cand.SHA256) == "" {
		return nil, fmt.Errorf("refusing to install %s: the release provides no sha256 checksum; install it manually from %s", cand.Name, sourceReleaseURL(cand.Source))
	}
	if err := allowedDownloadURL(cand.URL); err != nil {
		return nil, fmt.Errorf("refusing to download %s: %w", cand.Name, err)
	}

	kind := classifyAsset(cand.Name)
	var stage string
	if isSystemPackage(kind) {
		stage, err = packageStageDir()
	} else {
		var root string
		root, err = updateCacheDir()
		if err == nil {
			stage, err = os.MkdirTemp(root, "stage-*")
		}
	}
	if err != nil {
		return nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }

	filePath := filepath.Join(stage, filepath.Base(cand.Name))
	if err := downloadToFile(cand.URL, filePath, cand.Name, onProgress); err != nil {
		cleanup()
		return nil, fmt.Errorf("download %s: %w", cand.Name, err)
	}

	// Verification is unconditional: the SHA256 was required up front, and a
	// missing one must never slip through a defensive check.
	if onProgress != nil {
		onProgress(Progress{Phase: "verifying", Total: -1, Message: cand.Name})
	}
	if strings.TrimSpace(cand.SHA256) == "" {
		cleanup()
		return nil, fmt.Errorf("refusing to install %s: no sha256 checksum", cand.Name)
	}
	got, err := sha256File(filePath)
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("hash %s: %w", cand.Name, err)
	}
	if !strings.EqualFold(got, cand.SHA256) {
		log.Writef("[update] sha256 mismatch for %s: got %s want %s", cand.Name, got, cand.SHA256)
		cleanup()
		return nil, fmt.Errorf("sha256 mismatch for %s", cand.Name)
	}
	log.Writef("[update] sha256 verified for %s", cand.Name)

	pend := &PendingUpdate{
		Asset:    cand,
		Kind:     kind,
		StageDir: stage,
		FilePath: filePath,
	}

	// Extract payloads now so Apply() only performs the replacement.
	switch pend.Kind {
	case "portable":
		pend.NewBinary, err = extractPortableZip(stage, filePath)
	case "binary-tar.gz":
		pend.NewBinary, err = extractTarGzBinary(stage, filePath)
	case "installer":
		// nothing to extract; the downloaded installer is the payload
	case "deb", "rpm":
		// the package itself is the payload; root reads it via pkexec
		err = os.Chmod(filePath, 0644)
	}
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("extract %s: %w", cand.Name, err)
	}

	// Record the extracted binary's hash so Apply can detect any change
	// between extraction and replacement. The archive SHA256 above remains
	// the authenticity anchor; this only closes the local TOCTOU window.
	if pend.NewBinary != "" {
		pend.NewBinarySHA256, err = sha256File(pend.NewBinary)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("hash extracted %s: %w", cand.Name, err)
		}
	}

	m.setPending(pend)
	return pend, nil
}

// downloadToFile streams url into dest, emitting progress when onProgress is
// set. Non-200 responses are errors (the fallback loop then tries the mirror).
// An overall 30-minute deadline bounds stalled connections.
func downloadToFile(url, dest, label string, onProgress func(Progress)) error {
	if err := allowedDownloadURL(url); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "uniTerm")

	resp, err := downloadHTTPClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	var received int64
	total := resp.ContentLength
	if total <= 0 {
		total = -1
	}
	buf := make([]byte, 64*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			received += int64(n)
			if onProgress != nil {
				p := Progress{Phase: "downloading", Received: received, Total: total, Message: label}
				if total > 0 {
					p.Percent = float64(received) / float64(total) * 100
				}
				onProgress(p)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	return nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractTarGzBinary extracts the first regular file from a .tar.gz payload
// (CI packages the bare executable at the archive root) and returns its path.
func extractTarGzBinary(stage, tarPath string) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	var binaryPath string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if strings.EqualFold(name, "uniterm") || strings.EqualFold(name, "uniterm.exe") {
			binaryPath = filepath.Join(stage, name)
			out, err := os.OpenFile(binaryPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			break
		}
	}
	if binaryPath == "" {
		return "", fmt.Errorf("no executable found in %s", tarPath)
	}
	return binaryPath, nil
}

// extractPortableZip extracts the Windows portable zip: uniTerm.exe plus the
// plugins/ directory (VcXsrv). Returns the extracted exe path.
func extractPortableZip(stage, zipPath string) (string, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	var binaryPath string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := filepath.Base(f.Name)
		dest := filepath.Join(stage, f.Name)
		// archive/zip names use forward slashes; guard against traversal.
		if strings.Contains(f.Name, "..") {
			continue
		}
		// "uniterm.exe" is the Windows payload; a bare "uniterm" covers the
		// macOS zip and the executable inside a fallback .app-bundle zip.
		if strings.EqualFold(base, "uniterm.exe") || strings.EqualFold(base, "uniterm") {
			binaryPath = dest
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return "", err
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return "", err
		}
	}
	if binaryPath == "" {
		return "", fmt.Errorf("uniterm executable not found in %s", zipPath)
	}
	return binaryPath, nil
}
