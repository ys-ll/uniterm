package session

import (
	"fmt"
	"os"
	osUser "os/user"
	"path/filepath"
	"strings"
	"time"
)

// localFSOps provides reusable local filesystem operations for file-transfer
// session types (FTP, SMB, WebDAV, S3). Embed it and set localCwd before use.
type localFSOps struct {
	localCwd string
}

func newLocalFSOps() localFSOps {
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		// Stripped environments (some sandboxes / minimal containers) can
		// fail UserHomeDir. Fall back to TempDir so safeAbsPath's containment
		// check still has an absolute prefix to compare against — without
		// this, an empty root would let every relative path through.
		homeDir = os.TempDir()
	}
	return localFSOps{localCwd: homeDir}
}

// safeAbsPath returns the absolute path for a user-provided path, refusing
// any relative path that resolves (after following symlinks) outside
// localCwd. Absolute paths are trusted and returned cleaned.
//
// Containment matters because remote content (downloaded via FTP/SFTP/etc.)
// can plant symlinks or `..` segments in the local directory; without this
// guard, LocalPutContent / LocalRemove / LocalMkdir / LocalCopy / LocalMove
// / LocalRename would follow the symlink and operate on files the user never
// intended to touch (e.g. overwrite ~/.ssh/authorized_keys or recursively
// delete a parent directory).
func (o *localFSOps) safeAbsPath(p string) (string, error) {
	if filepath.IsAbs(p) {
		return filepath.Clean(p), nil
	}
	abs := filepath.Clean(filepath.Join(o.localCwd, p))

	// Resolve symlinks so a planted link inside localCwd can't escape.
	// EvalSymlinks fails when the leaf doesn't exist yet (we're about to
	// create it); in that case resolve the parent and re-join the leaf so
	// a symlink in the parent still trips the check.
	resolved := abs
	if r, err := filepath.EvalSymlinks(abs); err == nil {
		resolved = r
	} else {
		parent, base := filepath.Split(abs)
		rp, perr := filepath.EvalSymlinks(parent)
		if perr != nil {
			return "", perr
		}
		resolved = filepath.Join(rp, base)
	}

	// Resolve localCwd too so a symlink at the root doesn't cause false
	// escapes (user picked ~/downloads which is itself a symlink).
	root := o.localCwd
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes local directory: %s", p)
	}
	return abs, nil
}

func (o *localFSOps) ListLocal(dir string) (FileListResult, error) {
	if dir == "" {
		dir = o.localCwd
	} else if !filepath.IsAbs(dir) {
		dir = filepath.Join(o.localCwd, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return FileListResult{}, err
	}
	files := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		fi, _ := e.Info()
		var size int64
		var mode os.FileMode
		var modTime time.Time
		if fi != nil {
			size = fi.Size()
			mode = fi.Mode()
			modTime = fi.ModTime()
		}
		owner := ""
		if currentUser, err := osUser.Current(); err == nil {
			owner = currentUser.Username
		}
		isDir := e.IsDir()
		if fi != nil && fi.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = target.IsDir()
			}
		}
		isHidden := e.Name() != "" && e.Name()[0] == '.'
		if !isHidden {
			isHidden = isPathHidden(filepath.Join(dir, e.Name()))
		}
		files = append(files, FileItem{
			Name:     e.Name(),
			Size:     size,
			ModTime:  modTime.Format(time.RFC3339),
			Mode:     mode.String(),
			IsDir:    isDir,
			IsHidden: isHidden,
			Owner:    owner,
		})
	}
	return FileListResult{Files: files, Dir: dir}, nil
}

func (o *localFSOps) ListLocalDrives() ([]FileItem, error) {
	var drives []FileItem
	for _, letter := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		root := string(letter) + ":\\"
		fi, err := os.Stat(root)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			drives = append(drives, FileItem{
				Name:    root,
				Size:    0,
				ModTime: fi.ModTime().Format(time.RFC3339),
				Mode:    fi.Mode().String(),
				IsDir:   true,
			})
		}
	}
	return drives, nil
}

func (o *localFSOps) ChangeLocalDir(dir string) (FileListResult, error) {
	target := dir
	if !filepath.IsAbs(dir) {
		target = filepath.Join(o.localCwd, dir)
	}
	fi, err := os.Stat(target)
	if err != nil {
		return FileListResult{}, fmt.Errorf("no such directory: %s", target)
	}
	if !fi.IsDir() {
		return FileListResult{}, fmt.Errorf("not a directory: %s", target)
	}
	abs, _ := filepath.Abs(target)
	o.localCwd = abs
	return o.ListLocal(abs)
}

func (o *localFSOps) LocalRemove(p string, recursive bool) error {
	p, err := o.safeAbsPath(p)
	if err != nil {
		return err
	}
	if recursive {
		return os.RemoveAll(p)
	}
	fi, err := os.Stat(p)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		entries, err := os.ReadDir(p)
		if err != nil {
			return err
		}
		if len(entries) > 0 {
			return fmt.Errorf("directory not empty (%d items)", len(entries))
		}
	}
	return os.Remove(p)
}

func (o *localFSOps) LocalRename(oldName, newName string) error {
	old, err := o.safeAbsPath(oldName)
	if err != nil {
		return err
	}
	newPath, err := o.safeAbsPath(newName)
	if err != nil {
		return err
	}
	return os.Rename(old, newPath)
}

func (o *localFSOps) LocalMkdir(dir string) error {
	p, err := o.safeAbsPath(dir)
	if err != nil {
		return err
	}
	return os.MkdirAll(p, 0755)
}

func (o *localFSOps) LocalGetContent(localPath string) ([]byte, error) {
	p, err := o.safeAbsPath(localPath)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func (o *localFSOps) LocalPutContent(localPath string, content []byte) error {
	p, err := o.safeAbsPath(localPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}
	return os.WriteFile(p, content, 0644)
}

func (o *localFSOps) LocalCopy(oldPath, newPath string) error {
	old, err := o.safeAbsPath(oldPath)
	if err != nil {
		return err
	}
	n, err := o.safeAbsPath(newPath)
	if err != nil {
		return err
	}
	return localCopyRecursive(old, n)
}

func (o *localFSOps) LocalMove(oldPath, newPath string) error {
	old, err := o.safeAbsPath(oldPath)
	if err != nil {
		return err
	}
	n, err := o.safeAbsPath(newPath)
	if err != nil {
		return err
	}
	return os.Rename(old, n)
}