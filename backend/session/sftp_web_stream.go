package session

import (
	"context"
	"io"
	"os"
	"path"
)

// Streaming adapters used by the headless web console (server_web_extension.go)
// to move bytes through a browser without staging whole files in memory.
//
// shortcut: only SFTP sessions stream; SCP/FTP keep the desktop transfer
// pipeline. Add adapters there if the web console must support them.

func (s *SFTPSession) resolveRemotePath(remotePath string) string {
	if path.IsAbs(remotePath) {
		return remotePath
	}
	return path.Join(s.cwd, remotePath)
}

// OpenFileReader opens a remote file for streaming read.
func (s *SFTPSession) OpenFileReader(ctx context.Context, remotePath string) (io.ReadCloser, int64, error) {
	if err := s.requireClient(); err != nil {
		return nil, 0, err
	}
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	f, err := s.sftpClient.Open(s.resolveRemotePath(remotePath))
	if err != nil {
		return nil, 0, err
	}
	// A failed STAT must not abort a stream that is otherwise readable; the
	// handler omits Content-Length when the size is negative.
	size := int64(-1)
	if info, statErr := f.Stat(); statErr == nil {
		size = info.Size()
	}
	return f, size, nil
}

// OpenFileWriter opens/creates a remote file for streaming write, creating
// missing parent directories the way PutContent does.
func (s *SFTPSession) OpenFileWriter(ctx context.Context, remotePath string, _ os.FileMode, _ int64) (io.WriteCloser, error) {
	if err := s.requireClient(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rp := s.resolveRemotePath(remotePath)
	if err := s.sftpClient.MkdirAll(path.Dir(rp)); err != nil {
		return nil, err
	}
	return s.sftpClient.Create(rp)
}

// MkdirAll creates remote directories recursively.
func (s *SFTPSession) MkdirAll(ctx context.Context, remoteDir string) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.sftpClient.MkdirAll(s.resolveRemotePath(remoteDir))
}

// WalkRemoteDir walks a remote tree in archive order, handing each entry a
// relative path (prefixed with the root's base name) and a lazy reader.
func (s *SFTPSession) WalkRemoteDir(ctx context.Context, remoteRoot string, fn func(relPath string, info os.FileInfo, openReader func() (io.ReadCloser, error)) error) error {
	if err := s.requireClient(); err != nil {
		return err
	}
	root := s.resolveRemotePath(remoteRoot)

	walker := s.sftpClient.Walk(root)
	for walker.Step() {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := walker.Err(); err != nil {
			return err
		}
		p := walker.Path()
		openReader := func() (io.ReadCloser, error) {
			return s.sftpClient.Open(p)
		}
		if err := fn(archiveRelPath(root, p), walker.Stat(), openReader); err != nil {
			return err
		}
	}
	// Step() stops on the first error (including a missing root) without
	// entering the loop body; surface it instead of returning an empty archive.
	return walker.Err()
}

// archiveRelPath maps a walked path to its tar entry name, prefixed with the
// root's base name so unpacking never scatters entries into the cwd.
func archiveRelPath(root, p string) string {
	base := path.Base(root)
	if base == "/" || base == "." || base == "" {
		base = "archive"
	}
	rel, err := path.Rel(root, p)
	if err != nil || rel == "." {
		return base
	}
	return path.Join(base, rel)
}
