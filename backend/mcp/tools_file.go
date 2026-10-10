package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultMCPDirEntries caps list_remote_dir rows.
const DefaultMCPDirEntries = 500

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

// File tools: streaming uploads/downloads and listing between the user's
// allowed local directories and remote hosts. Transfers ride the session's
// companion file-transfer backend — SFTP or SCP, selected by the
// connection's fileTransferProto setting (SCP covers hosts whose SSH server
// provides no SFTP subsystem, common on embedded devices). File contents
// never pass through the model context — file inspection goes through
// exec_command (cat/head/Get-Content …), which also keeps shell-dialect
// choice with the AI.

type listDirIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	RemotePath string `json:"remotePath" jsonschema:"remote directory, e.g. /var/log"`
}
type listDirOut struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

type uploadIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	LocalPath  string `json:"localPath" jsonschema:"local file path, must be inside an allowed directory"`
	RemotePath string `json:"remotePath" jsonschema:"remote destination path"`
}
type uploadOut struct {
	Bytes int64 `json:"bytes"`
}

type downloadIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	RemotePath string `json:"remotePath" jsonschema:"remote file path"`
	LocalPath  string `json:"localPath" jsonschema:"local destination, must be inside an allowed directory"`
}
type downloadOut struct {
	Bytes int64 `json:"bytes"`
}

// registerFileTools installs the file tools (read-only listing/reading plus
// policy-gated transfers — upload/download go through gateExec as write risk).
func (s *Server) registerFileTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_remote_dir",
		Description: "List one remote directory (name, size, isDir, mtime, mode) on a connected SSH session, using the connection's file transfer protocol (SFTP or SCP). Bounded to 500 entries.",
	}, s.toolListRemoteDir)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "upload_file",
		Description: "Upload a local file to a remote host over the connection's file transfer protocol (SFTP or SCP, per the connection's fileTransferProto setting). Streams disk to disk — content never passes through the model. The local path must be inside a directory allowed in uniTerm's SFTP bookmarks. Requires user approval.",
	}, s.toolUploadFile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "download_file",
		Description: "Download a remote file to a local directory over the connection's file transfer protocol (SFTP or SCP, per the connection's fileTransferProto setting). Streams disk to disk. The local path must be inside a directory allowed in uniTerm's SFTP bookmarks.",
	}, s.toolDownloadFile)
}

func (s *Server) toolListRemoteDir(ctx context.Context, req *mcp.CallToolRequest, in listDirIn) (*mcp.CallToolResult, listDirOut, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, listDirOut{}, fmt.Errorf("sessionId is required")
	}
	fe, err := s.env.FileSession(in.SessionID)
	if err != nil {
		return nil, listDirOut{}, err
	}
	entries, err := fe.MCPListDir(in.RemotePath)
	if err != nil {
		s.audit(ctx, req, "list_remote_dir", in.SessionID, in.RemotePath, nil, err)
		return nil, listDirOut{}, err
	}
	s.audit(ctx, req, "list_remote_dir", in.SessionID, in.RemotePath, nil, nil)
	return nil, listDirOut{Path: in.RemotePath, Entries: entries}, nil
}

func (s *Server) toolUploadFile(ctx context.Context, req *mcp.CallToolRequest, in uploadIn) (*mcp.CallToolResult, uploadOut, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, uploadOut{}, fmt.Errorf("sessionId is required")
	}
	// Local path must be inside an allowed directory (and exist — the copy
	// below will fail with a clear error if not).
	resolved, err := s.env.ResolveLocalPath(in.LocalPath)
	if err != nil {
		return nil, uploadOut{}, err
	}
	fe, err := s.env.FileSession(in.SessionID)
	if err != nil {
		return nil, uploadOut{}, err
	}
	// Upload mutates the remote host: write-level risk.
	if err := s.gateExec(ctx, req, in.SessionID, "upload "+in.LocalPath+" → "+in.RemotePath, RiskWrite); err != nil {
		return nil, uploadOut{}, err
	}
	n, err := fe.MCPWriteFile(resolved, in.RemotePath)
	s.audit(ctx, req, "upload_file", in.SessionID, in.LocalPath+" → "+in.RemotePath, nil, err)
	return nil, uploadOut{Bytes: n}, err
}

func (s *Server) toolDownloadFile(ctx context.Context, req *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, downloadOut{}, fmt.Errorf("sessionId is required")
	}
	resolved, err := s.env.ResolveLocalPath(in.LocalPath)
	if err != nil {
		return nil, downloadOut{}, err
	}
	fe, err := s.env.FileSession(in.SessionID)
	if err != nil {
		return nil, downloadOut{}, err
	}
	// Download only reads the remote side; the local destination is already
	// constrained to the user's bookmarked directories, so it grades as
	// read — same as cat-ing the file through exec_command.
	if err := s.gateExec(ctx, req, in.SessionID, "download "+in.RemotePath+" → "+in.LocalPath, RiskRead); err != nil {
		return nil, downloadOut{}, err
	}
	n, err := fe.MCPReadRemoteToFile(in.RemotePath, resolved)
	s.audit(ctx, req, "download_file", in.SessionID, in.RemotePath+" → "+in.LocalPath, nil, err)
	return nil, downloadOut{Bytes: n}, err
}
