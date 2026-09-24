package session

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/sftp"
)

// MCP SFTP support: file tools ride the same authenticated client as the
// terminal (sftp.NewClient opens a dedicated subsystem channel), so uploads
// and downloads never re-authenticate. Streams go disk↔disk; file contents
// never enter the model context (read_remote_file is the bounded exception).

const (
	// DefaultMCPReadFileMax caps read_remote_file content returned inline.
	DefaultMCPReadFileMax = 256 * 1024
	// DefaultMCPDirEntries caps list_remote_dir rows.
	DefaultMCPDirEntries = 500
)

// sftpClientPool lazily opens and caches one SFTP client per SSH session for
// MCP file tools. Cached clients are closed when the session disconnects.
var sftpClientPool = struct {
	mu    sync.Mutex
	bySID map[string]*sftp.Client
}{bySID: make(map[string]*sftp.Client)}

// mcpSFTPClient returns a cached SFTP client opened on the session's shared
// authenticated SSH client.
func (s *SSHSession) mcpSFTPClient() (*sftp.Client, error) {
	s.mu.RLock()
	ref := s.clientRef
	s.mu.RUnlock()
	if ref == nil || ref.client == nil || s.Status() != StatusConnected {
		return nil, fmt.Errorf("session %s is not connected", s.id)
	}
	sftpClientPool.mu.Lock()
	defer sftpClientPool.mu.Unlock()
	if sc, ok := sftpClientPool.bySID[s.id]; ok {
		return sc, nil
	}
	// Hold a clientRef for the SFTP client's lifetime so closing the source
	// tab doesn't tear down the transport mid-transfer.
	ref.acquire()
	sc, err := sftp.NewClient(ref.client, sftpClientOptions()...)
	if err != nil {
		ref.release()
		return nil, fmt.Errorf("sftp client: %w", err)
	}
	sftpClientPool.bySID[s.id] = sc
	go func() {
		<-s.quit
		sftpClientPool.mu.Lock()
		delete(sftpClientPool.bySID, s.id)
		sftpClientPool.mu.Unlock()
		sc.Close()
		ref.release()
	}()
	return sc, nil
}

// MCPFileEntry is one remote dir listing row.
type MCPFileEntry struct {
	Name    string `json:"name"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"isDir"`
	ModTime int64  `json:"modTime,omitempty"`
	Mode    string `json:"mode,omitempty"`
}

// MCPListDir lists one remote directory (bounded rows).
func (s *SSHSession) MCPListDir(remotePath string) ([]MCPFileEntry, error) {
	sc, err := s.mcpSFTPClient()
	if err != nil {
		return nil, err
	}
	infos, err := sc.ReadDir(remotePath)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", remotePath, err)
	}
	sort.Slice(infos, func(i, j int) bool {
		if infos[i].IsDir() != infos[j].IsDir() {
			return infos[i].IsDir()
		}
		return infos[i].Name() < infos[j].Name()
	})
	if len(infos) > DefaultMCPDirEntries {
		infos = infos[:DefaultMCPDirEntries]
	}
	entries := make([]MCPFileEntry, 0, len(infos))
	for _, fi := range infos {
		entries = append(entries, MCPFileEntry{
			Name:    fi.Name(),
			Size:    fi.Size(),
			IsDir:   fi.IsDir(),
			ModTime: fi.ModTime().UnixMilli(),
			Mode:    fi.Mode().String(),
		})
	}
	return entries, nil
}

// MCPReadFile returns up to max bytes of a remote file starting at offset.
func (s *SSHSession) MCPReadFile(remotePath string, offset int64, max int) ([]byte, bool, error) {
	sc, err := s.mcpSFTPClient()
	if err != nil {
		return nil, false, err
	}
	fi, err := sc.Stat(remotePath)
	if err != nil {
		return nil, false, fmt.Errorf("stat %s: %w", remotePath, err)
	}
	if fi.IsDir() {
		return nil, false, fmt.Errorf("%s is a directory", remotePath)
	}
	if max <= 0 {
		max = DefaultMCPReadFileMax
	}
	f, err := sc.Open(remotePath)
	if err != nil {
		return nil, false, fmt.Errorf("open %s: %w", remotePath, err)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, false, err
		}
	}
	buf := make([]byte, max)
	n, err := io.ReadFull(f, buf)
	truncated := false
	if err == io.ErrUnexpectedEOF || n == max {
		truncated = fi.Size()-offset > int64(n)
	} else if err != nil && err != io.EOF {
		return nil, false, err
	}
	return buf[:n], truncated, nil
}

// MCPWriteFile writes localPath's content to remotePath (streams, never
// buffered whole). remotePath's parent must exist.
func (s *SSHSession) MCPWriteFile(localPath, remotePath string) (int64, error) {
	sc, err := s.mcpSFTPClient()
	if err != nil {
		return 0, err
	}
	src, err := os.Open(localPath)
	if err != nil {
		return 0, fmt.Errorf("open local %s: %w", localPath, err)
	}
	defer src.Close()
	dst, err := sc.Create(remotePath)
	if err != nil {
		return 0, fmt.Errorf("create remote %s: %w", remotePath, err)
	}
	n, copyErr := io.Copy(dst, src)
	cerr := dst.Close()
	if copyErr != nil {
		return n, fmt.Errorf("upload %s → %s: %w", localPath, remotePath, copyErr)
	}
	if cerr != nil {
		return n, fmt.Errorf("close remote %s: %w", remotePath, cerr)
	}
	return n, nil
}

// MCPReadRemoteToFile downloads remotePath to localPath (streams).
func (s *SSHSession) MCPReadRemoteToFile(remotePath, localPath string) (int64, error) {
	sc, err := s.mcpSFTPClient()
	if err != nil {
		return 0, err
	}
	src, err := sc.Open(remotePath)
	if err != nil {
		return 0, fmt.Errorf("open remote %s: %w", remotePath, err)
	}
	defer src.Close()
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return 0, err
	}
	dst, err := os.Create(localPath)
	if err != nil {
		return 0, fmt.Errorf("create local %s: %w", localPath, err)
	}
	n, copyErr := io.Copy(dst, src)
	cerr := dst.Close()
	if copyErr != nil {
		return n, fmt.Errorf("download %s → %s: %w", remotePath, localPath, copyErr)
	}
	if cerr != nil {
		return n, fmt.Errorf("close local %s: %w", localPath, cerr)
	}
	return n, nil
}

// MCPRemoteStat returns one remote file's size/mtime.
func (s *SSHSession) MCPRemoteStat(remotePath string) (MCPFileEntry, error) {
	sc, err := s.mcpSFTPClient()
	if err != nil {
		return MCPFileEntry{}, err
	}
	fi, err := sc.Stat(remotePath)
	if err != nil {
		return MCPFileEntry{}, fmt.Errorf("stat %s: %w", remotePath, err)
	}
	return MCPFileEntry{
		Name:    fi.Name(),
		Size:    fi.Size(),
		IsDir:   fi.IsDir(),
		ModTime: fi.ModTime().UnixMilli(),
		Mode:    fi.Mode().String(),
	}, nil
}

// ResolveMcpLocalPath validates a local path against the allowed directory
// list: the resolved path must live under one of the roots. Symlinks are
// resolved (filepath.EvalSymlinks) then containment is re-checked, so
// symlink escapes are caught. A not-yet-existing path (the normal case for
// download destinations) resolves its nearest existing ancestor instead —
// plain EvalSymlinks fails on missing files, which on macOS (/tmp →
// /private/tmp) would wrongly reject every fresh download target.
// An empty allowed list rejects everything.
func ResolveMcpLocalPath(path string, allowedRoots []string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("local path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		// Missing leaf (download destination): resolve the deepest existing
		// ancestor, then re-append the non-existing tail.
		dir, tail := filepath.Split(abs)
		var parts []string
		for tail != "" {
			if r, derr := filepath.EvalSymlinks(filepath.Clean(dir)); derr == nil {
				resolved = filepath.Join(r, tail, strings.Join(parts, string(filepath.Separator)))
				break
			}
			d, t := filepath.Split(filepath.Clean(dir))
			if d == dir {
				resolved = abs
				break
			}
			parts = append([]string{tail}, parts...)
			dir, tail = d, t
		}
		if resolved == "" {
			resolved = abs
		}
	}
	for _, root := range allowedRoots {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		rootResolved, err := filepath.EvalSymlinks(rootAbs)
		if err != nil {
			rootResolved = rootAbs
		}
		if rootResolved == resolved || strings.HasPrefix(resolved, rootResolved+string(filepath.Separator)) {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("local path %s is outside the allowed directories (configure them in SFTP bookmarks)", path)
}
