package localsrv

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// rootEscapeMessage is the message os.Root reports when a name (usually via a
// symlink) resolves outside its root. The standard library keeps that sentinel
// unexported (os/file.go: errPathEscapes), so containment is recognized by
// message here.
const rootEscapeMessage = "path escapes from parent"

// errTraversal marks a request path that contains an explicit ".." segment.
// Such paths are rejected instead of being silently cleaned into a different
// file so traversal attempts are visible as 403s.
var errTraversal = errors.New("path traversal")

// errRootEscape mirrors the os.Root containment error for paths resolved by
// expandLinksOnce. errTooManyLinks bounds symlink expansion.
var (
	errRootEscape   = errors.New(rootEscapeMessage)
	errTooManyLinks = errors.New("too many levels of symbolic links")
)

// isRootEscape reports whether err is os.Root's containment error.
func isRootEscape(err error) bool {
	return err != nil && strings.Contains(err.Error(), rootEscapeMessage)
}

// writePathError maps a filesystem error to an HTTP status: containment
// violations and explicit traversal become 403; missing files, dangling
// symlinks, malformed names and other lookup failures become 404 so the
// server never discloses whether a path exists outside the root.
func writePathError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case isRootEscape(err), errors.Is(err, errTraversal):
		http.Error(w, "Forbidden", http.StatusForbidden)
	case errors.Is(err, fs.ErrNotExist):
		http.NotFound(w, r)
	default:
		http.NotFound(w, r)
	}
}

// rootRelPath converts a request URL path into a relative, slash-free name
// suitable for os.Root. Explicit ".." segments and NUL bytes are rejected
// rather than cleaned, so no request can be rewritten into a path outside or
// silently drop a component.
func rootRelPath(urlPath string) (string, error) {
	if strings.ContainsRune(urlPath, 0) {
		return "", fs.ErrInvalid
	}
	for _, seg := range strings.Split(urlPath, "/") {
		if seg == ".." {
			return "", errTraversal
		}
	}
	rel := strings.TrimPrefix(path.Clean("/"+urlPath), "/")
	if rel == "" {
		return ".", nil
	}
	return filepath.FromSlash(rel), nil
}

// openWithinRoot opens name through root. os.Root follows symlinks only while
// they stay inside the root, but it refuses absolute symlink targets outright
// - even ones that resolve inside. On a containment error we rewrite such
// links into root-relative names and retry once. The retry also goes through
// root.Open, so containment is still enforced atomically at open time even if
// a link changes between resolution and open.
func openWithinRoot(root *os.Root, rootBases []string, name string) (*os.File, error) {
	f, err := root.Open(name)
	if err == nil || !isRootEscape(err) {
		return f, err
	}
	rewritten, rerr := resolveLinksWithinRoot(root, rootBases, name)
	if rerr != nil || rewritten == name {
		return nil, err
	}
	return root.Open(rewritten)
}

const maxLinkResolutions = 255

// resolveLinksWithinRoot expands every symlink in name into a plain relative
// path, mapping absolute targets back under the root. It fails with
// errRootEscape when an absolute target resolves outside all accepted root
// spellings (the configured path and its canonical form).
func resolveLinksWithinRoot(root *os.Root, rootBases []string, name string) (string, error) {
	current := name
	for i := 0; i < maxLinkResolutions; i++ {
		next, changed, err := expandLinksOnce(root, rootBases, current)
		if err != nil {
			return "", err
		}
		if !changed {
			return next, nil
		}
		current = next
	}
	return "", errTooManyLinks
}

// expandLinksOnce rewrites the first symlink in name, returning the expanded
// path and whether a rewrite happened.
func expandLinksOnce(root *os.Root, rootBases []string, name string) (string, bool, error) {
	parts := splitPathParts(filepath.ToSlash(name))
	resolved := make([]string, 0, len(parts))
	for i, part := range parts {
		candidate := path.Join(append(append([]string{}, resolved...), part)...)
		if candidate == "" {
			candidate = "."
		}
		fi, err := root.Lstat(filepath.FromSlash(candidate))
		if err != nil {
			return "", false, err
		}
		if fi.Mode()&os.ModeSymlink == 0 {
			resolved = append(resolved, part)
			continue
		}
		target, err := root.Readlink(filepath.FromSlash(candidate))
		if err != nil {
			return "", false, err
		}
		var targetParts []string
		if filepath.IsAbs(target) {
			rel, ok := relativeToRoot(rootBases, target)
			if !ok {
				return "", false, errRootEscape
			}
			targetParts = splitPathParts(filepath.ToSlash(rel))
		} else {
			targetParts = splitPathParts(filepath.ToSlash(target))
		}
		combined := append([]string{}, resolved...)
		combined = append(combined, targetParts...)
		combined = append(combined, parts[i+1:]...)
		joined := path.Join(combined...)
		if joined == "" {
			joined = "."
		}
		return joined, true, nil
	}
	return name, false, nil
}

// relativeToRoot reports whether an absolute symlink target resolves inside
// one of the accepted root spellings and returns it relative to that base.
func relativeToRoot(rootBases []string, target string) (string, bool) {
	for _, base := range rootBases {
		if base == "" {
			continue
		}
		rel, err := filepath.Rel(base, target)
		if err != nil {
			continue
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return rel, true
	}
	return "", false
}

// splitPathParts splits a slash path into non-empty, non-dot components.
func splitPathParts(p string) []string {
	parts := strings.Split(p, "/")
	out := parts[:0]
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		out = append(out, part)
	}
	return out
}

// listen creates a net.Listener bound to addr on network, and registers a
// goroutine that closes it when ctx is cancelled. This lets Stop cancel
// blocking Accept calls cleanly.
func listen(ctx context.Context, network, addr string) (net.Listener, error) {
	ln, err := net.Listen(network, addr)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	return ln, nil
}

// StartHTTPServer starts an HTTP file server serving rootDir on the given
// port. With listDir enabled, directory requests render the directory
// listing (index.html stays reachable under its explicit name). With
// listDir disabled, directory requests serve index.html when one is
// present and 404 when not. Returns the server id.
func (m *Manager) StartHTTPServer(cfg HTTPConfig) (string, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("invalid port: %d", cfg.Port)
	}
	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return "", fmt.Errorf("invalid root directory: %w", err)
	}
	// Open the served root once and keep it for the handler lifetime. Every
	// lookup below goes through this *os.Root, which follows symlinks only
	// while they stay inside the root and refuses any escape.
	root, err := os.OpenRoot(absRoot)
	if err != nil {
		return "", fmt.Errorf("invalid root directory: %w", err)
	}
	// Accept both the configured spelling and its canonical form when
	// resolving absolute symlink targets, so a root reached through a
	// symlink (the macOS /var pattern) still recognizes in-root links.
	rootBases := []string{absRoot}
	if canonical, cerr := filepath.EvalSymlinks(absRoot); cerr == nil && canonical != absRoot {
		rootBases = append(rootBases, canonical)
	}
	var closeOnce sync.Once
	closeRoot := func() { closeOnce.Do(func() { _ = root.Close() }) }

	id, ctx := m.newServer()
	m.log(ServiceHTTP, id, fmt.Sprintf("Starting HTTP server on port %d ...", cfg.Port))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.log(ServiceHTTP, id, fmt.Sprintf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path))

		rel, err := rootRelPath(r.URL.Path)
		if err != nil {
			writePathError(w, r, err)
			return
		}

		// Open once and branch on the opened file's own metadata: no
		// stat-then-open window where the target can change underneath us.
		f, err := openWithinRoot(root, rootBases, rel)
		if err != nil {
			writePathError(w, r, err)
			return
		}
		defer f.Close()

		fi, err := f.Stat()
		if err != nil {
			writePathError(w, r, err)
			return
		}

		if fi.IsDir() {
			if cfg.ListDir {
				// Directory browsing enabled: list the directory. The
				// site's index.html stays reachable under its explicit
				// name, which is handled by the file path below.
				entries, err := f.ReadDir(-1)
				if err != nil {
					http.Error(w, "Error reading directory: "+html.EscapeString(err.Error()), http.StatusInternalServerError)
					return
				}
				renderDirHTML(w, entries, displayPath(r.URL.Path))
				return
			}
			// Directory browsing disabled: serve index.html when present
			// so a static site still works, otherwise 404.
			indexRel := filepath.Join(rel, "index.html")
			indexFile, err := openWithinRoot(root, rootBases, indexRel)
			if err != nil {
				writePathError(w, r, err)
				return
			}
			defer indexFile.Close()
			indexFi, err := indexFile.Stat()
			if err != nil || indexFi.IsDir() {
				http.NotFound(w, r)
				return
			}
			// Redirect directory URLs to the trailing-slash form so
			// relative links inside index.html resolve correctly.
			if !strings.HasSuffix(r.URL.Path, "/") {
				http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
				return
			}
			http.ServeContent(w, r, indexFi.Name(), indexFi.ModTime(), indexFile)
			return
		}

		// Serve files directly. http.FileServer is deliberately avoided: it
		// redirects any URL ending in "/index.html" back to the directory,
		// which would make the explicit name unreachable when browsing is
		// enabled. ServeContent keeps Content-Type sniffing and range support.
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	ln, err := listen(ctx, "tcp", srv.Addr)
	if err != nil {
		closeRoot()
		m.finish(id)
		return "", fmt.Errorf("failed to listen on port %d: %w", cfg.Port, err)
	}

	m.setStop(id, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		closeRoot()
	})

	go func() {
		defer m.finish(id)
		defer closeRoot()
		m.state(ServiceHTTP, id, true)
		m.log(ServiceHTTP, id, "HTTP server started")
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			m.log(ServiceHTTP, id, fmt.Sprintf("HTTP server error: %v", err))
		}
		m.state(ServiceHTTP, id, false)
		m.log(ServiceHTTP, id, "HTTP server stopped")
	}()

	m.startTimeout(id, ServiceHTTP, cfg.TimeoutSeconds)
	return id, nil
}

// displayPath returns the path shown in a directory listing. The root ("")
// is displayed as "/".
func displayPath(urlPath string) string {
	if urlPath == "" {
		return "/"
	}
	return urlPath
}

// renderDirHTML renders a friendly MobaXterm-style directory listing for the
// already-read entries. displayPath is the request path shown in the heading.
func renderDirHTML(w http.ResponseWriter, entries []fs.DirEntry, displayPath string) {
	// Directories first, then by name.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}
		return a.Name() < b.Name()
	})

	base := strings.TrimSuffix(displayPath, "/")

	var totalFiles int
	var totalBytes int64
	var rows strings.Builder
	for _, e := range entries {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		totalFiles++
		name := e.Name()
		linkName := name
		if e.IsDir() {
			linkName += "/"
		} else {
			totalBytes += fi.Size()
		}

		rows.WriteString("<tr>")
		rows.WriteString("<td class=\"perm\">" + html.EscapeString(permString(fi)) + "</td>")
		if e.IsDir() {
			rows.WriteString("<td class=\"size\"></td>")
		} else {
			rows.WriteString(fmt.Sprintf("<td class=\"size\">%d</td>", fi.Size()))
		}
		rows.WriteString("<td class=\"time\">" + html.EscapeString(fi.ModTime().Format("Jan 02, 2006  15:04:05")) + "</td>")
		href := base + "/" + url.PathEscape(name)
		rows.WriteString("<td><a href=\"" + html.EscapeString(href) + "\">" + html.EscapeString(linkName) + "</a></td>")
		rows.WriteString("</tr>")
	}

	fmt.Fprintf(w, `<!doctype html><html><head><meta charset="utf-8"><title>Directory of %s</title>
<style>
 body{font:14px/1.6 -apple-system,"Segoe UI",Roboto,sans-serif;margin:2rem;color:#111827}
 h1{font-size:18px;margin:0 0 1rem;font-weight:600}
 table{border-collapse:collapse;width:100%%;max-width:900px;font-family:Consolas,Menlo,monospace;font-size:13px}
 th,td{text-align:left;padding:.4rem .7rem;border-bottom:1px solid #e5e7eb;white-space:nowrap}
 th{color:#6b7280;font-weight:600;font-size:12px;background:#f9fafb}
 .perm{color:#374151}.size{color:#374151;text-align:right}.time{color:#6b7280}
 a{color:#2563eb;text-decoration:none}a:hover{text-decoration:underline}
 .total{margin-top:1rem;color:#6b7280;font-size:13px}
</style></head><body>
<h1>Directory of %s</h1>
<table><thead><tr><th>Permissions</th><th>Size</th><th>Modified</th><th>Name</th></tr></thead><tbody>%s</tbody></table>
<div class="total">Total: %d file(s), %d byte(s)</div>
</body></html>`, html.EscapeString(displayPath), html.EscapeString(displayPath), rows.String(), totalFiles, totalBytes)
}

// permString returns a compact 5-character permission descriptor similar to
// MobaXterm (e.g. "drw--" for a directory, "-rw--" for a readable file).
func permString(fi os.FileInfo) string {
	m := fi.Mode()
	b := []byte("-----")
	if fi.IsDir() {
		b[0] = 'd'
	}
	if m&0o400 != 0 {
		b[1] = 'r'
	}
	if m&0o200 != 0 {
		b[2] = 'w'
	}
	if m&0o100 != 0 {
		b[3] = 'x'
	}
	return string(b)
}
