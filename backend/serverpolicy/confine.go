package serverpolicy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrPathDenied is the sentinel wrapped by every tenant path confinement
// failure.
var ErrPathDenied = errors.New("path outside the tenant sandbox")

// Confiner confines local paths to a fixed set of resolved roots.
//
// Roots and targets are canonicalized (absolute, cleaned, symlinks resolved
// through the deepest existing ancestor), so a symlink inside the tenant home
// that points at /etc cannot escape: the resolved target is checked against
// the resolved roots. A dangling symlink or an unresolvable component fails
// closed.
type Confiner struct {
	roots []string
}

// NewConfiner resolves the given roots. Empty roots are ignored; at least one
// usable root is required.
func NewConfiner(roots ...string) (*Confiner, error) {
	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		canonical, err := canonicalize(root)
		if err != nil {
			return nil, fmt.Errorf("%w: resolve root %q: %v", ErrPathDenied, root, err)
		}
		resolved = append(resolved, canonical)
	}
	if len(resolved) == 0 {
		return nil, fmt.Errorf("%w: no confinement roots configured", ErrPathDenied)
	}
	return &Confiner{roots: resolved}, nil
}

// ConfineLocalPath resolves path and verifies that it stays inside one of the
// given roots. It returns the canonical path to use for the actual filesystem
// operation, so the check and the operation see the same target.
func ConfineLocalPath(roots []string, path string) (string, error) {
	confiner, err := NewConfiner(roots...)
	if err != nil {
		return "", err
	}
	return confiner.Confine(path)
}

// Confine resolves and checks one path.
func (c *Confiner) Confine(path string) (string, error) {
	if c == nil {
		return "", fmt.Errorf("%w: nil confiner", ErrPathDenied)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%w: empty path", ErrPathDenied)
	}
	target, err := canonicalize(path)
	if err != nil {
		return "", fmt.Errorf("%w: resolve %q: %v", ErrPathDenied, path, err)
	}
	for _, root := range c.roots {
		if within(root, target) {
			return target, nil
		}
	}
	return "", fmt.Errorf("%w: %q resolves outside the allowed tenant roots", ErrPathDenied, path)
}

// Roots returns a copy of the resolved roots (tests and diagnostics).
func (c *Confiner) Roots() []string {
	if c == nil {
		return nil
	}
	return append([]string(nil), c.roots...)
}

// canonicalize returns an absolute, clean path with symlinks resolved through
// the deepest existing ancestor. Components that do not exist yet are rejoined
// after resolution, so a write to a new file inside the tenant home still
// resolves its existing parents (and any symlinked parent) before containment
// is checked.
func canonicalize(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)

	existing := absolute
	var suffix []string
	for {
		if _, err := os.Lstat(existing); err == nil {
			break
		}
		parent := filepath.Dir(existing)
		if parent == existing {
			break
		}
		suffix = append([]string{filepath.Base(existing)}, suffix...)
		existing = parent
	}

	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	for _, part := range suffix {
		resolved = filepath.Join(resolved, part)
	}
	return filepath.Clean(resolved), nil
}

// within reports whether target is root itself or a descendant of root.
func within(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
