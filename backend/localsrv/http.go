package localsrv

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

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

	id, ctx := m.newServer()
	m.log(ServiceHTTP, id, fmt.Sprintf("Starting HTTP server on port %d ...", cfg.Port))

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.log(ServiceHTTP, id, fmt.Sprintf("%s %s %s", r.RemoteAddr, r.Method, r.URL.Path))

		cleanPath := filepath.Clean(r.URL.Path)
		fullPath := filepath.Join(absRoot, cleanPath)

		// Prevent path traversal outside the root directory. The root itself
		// (rel == ".") and dotfiles inside the root are allowed; only paths
		// escaping above the root ("..") are denied.
		rel, err := filepath.Rel(absRoot, fullPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		fi, statErr := os.Stat(fullPath)
		if statErr == nil && fi.IsDir() {
			if cfg.ListDir {
				// Directory browsing enabled: list the directory. The
				// site's index.html stays reachable under its explicit
				// name, which is handled by the file path below.
				renderDirHTML(w, r, absRoot, fullPath)
				return
			}
			// Directory browsing disabled: serve index.html when present
			// so a static site still works, otherwise 404.
			indexPath := filepath.Join(fullPath, "index.html")
			if indexFi, indexErr := os.Stat(indexPath); indexErr == nil && !indexFi.IsDir() {
				// Redirect directory URLs to the trailing-slash form so
				// relative links inside index.html resolve correctly.
				if !strings.HasSuffix(r.URL.Path, "/") {
					http.Redirect(w, r, r.URL.Path+"/", http.StatusFound)
					return
				}
				http.ServeFile(w, r, indexPath)
				return
			}
			http.NotFound(w, r)
			return
		}

		// Serve files directly. http.FileServer is deliberately avoided: it
		// redirects any URL ending in "/index.html" back to the directory,
		// which would make the explicit name unreachable when browsing is
		// enabled. ServeContent keeps Content-Type sniffing and range support.
		f, err := os.Open(fullPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: handler,
	}

	ln, err := listen(ctx, "tcp", srv.Addr)
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to listen on port %d: %w", cfg.Port, err)
	}

	m.setStop(id, func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	go func() {
		defer m.finish(id)
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

// renderDirHTML renders a friendly MobaXterm-style directory listing for the
// directory at dirPath (rooted at absRoot) in response to the HTTP request r.
func renderDirHTML(w http.ResponseWriter, r *http.Request, absRoot, dirPath string) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, "Error reading directory: "+html.EscapeString(err.Error()), http.StatusInternalServerError)
		return
	}

	// Directories first, then by name.
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.IsDir() != b.IsDir() {
			return a.IsDir()
		}
		return a.Name() < b.Name()
	})

	displayPath := r.URL.Path
	if displayPath == "" {
		displayPath = "/"
	}
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
