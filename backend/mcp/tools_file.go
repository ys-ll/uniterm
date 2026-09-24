package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// File tools: streaming uploads/downloads between the user's allowed local
// directories and remote hosts over the session's SFTP channel. File
// contents never pass through the model context (read_remote_file is the
// bounded exception for small files/offsets).

type listDirIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	RemotePath string `json:"remotePath" jsonschema:"remote directory, e.g. /var/log"`
}
type listDirOut struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

type readFileIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	RemotePath string `json:"remotePath" jsonschema:"remote file path"`
	Offset     int64  `json:"offset,omitempty" jsonschema:"byte offset to start reading from (default 0)"`
	MaxBytes   int    `json:"maxBytes,omitempty" jsonschema:"maximum bytes to return (default 262144)"`
}
type readFileOut struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated,omitempty"`
	Size      int64  `json:"size"`
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

// registerFileTools installs the file group (gated by ToolsEnabled().Files
// per call so toggling the group in settings takes effect without restart).
func (s *Server) registerFileTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_remote_dir",
		Description: "List one remote directory (name, size, isDir, mtime, mode) on a connected SSH session. Bounded to 500 entries.",
	}, s.toolListRemoteDir)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "read_remote_file",
		Description: "Read up to 256KB of a remote file (text) starting at an offset. For larger files use download_file.",
	}, s.toolReadRemoteFile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "upload_file",
		Description: "Upload a local file to a remote host over the session's SFTP channel. Streams disk to disk — content never passes through the model. The local path must be inside a directory allowed in uniTerm's SFTP bookmarks. Requires user approval.",
	}, s.toolUploadFile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "download_file",
		Description: "Download a remote file to a local directory over the session's SFTP channel. Streams disk to disk. The local path must be inside a directory allowed in uniTerm's SFTP bookmarks. Requires user approval.",
	}, s.toolDownloadFile)
}

func (s *Server) toolListRemoteDir(ctx context.Context, req *mcp.CallToolRequest, in listDirIn) (*mcp.CallToolResult, listDirOut, error) {
	if !s.env.envFilesEnabled() {
		return nil, listDirOut{}, fmt.Errorf("file tools are disabled in uniTerm settings")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, listDirOut{}, fmt.Errorf("sessionId is required")
	}
	fe, ok := s.env.FileSession(in.SessionID)
	if !ok {
		return nil, listDirOut{}, fmt.Errorf("session %s not found or not an SSH session", in.SessionID)
	}
	entries, err := fe.MCPListDir(in.RemotePath)
	if err != nil {
		s.audit(ctx, req, "list_remote_dir", in.SessionID, in.RemotePath, nil, err)
		return nil, listDirOut{}, err
	}
	s.audit(ctx, req, "list_remote_dir", in.SessionID, in.RemotePath, nil, nil)
	return nil, listDirOut{Path: in.RemotePath, Entries: entries}, nil
}

func (s *Server) toolReadRemoteFile(ctx context.Context, req *mcp.CallToolRequest, in readFileIn) (*mcp.CallToolResult, readFileOut, error) {
	if !s.env.envFilesEnabled() {
		return nil, readFileOut{}, fmt.Errorf("file tools are disabled in uniTerm settings")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, readFileOut{}, fmt.Errorf("sessionId is required")
	}
	fe, ok := s.env.FileSession(in.SessionID)
	if !ok {
		return nil, readFileOut{}, fmt.Errorf("session %s not found or not an SSH session", in.SessionID)
	}
	data, truncated, err := fe.MCPReadFile(in.RemotePath, in.Offset, in.MaxBytes)
	if err != nil {
		s.audit(ctx, req, "read_remote_file", in.SessionID, in.RemotePath, nil, err)
		return nil, readFileOut{}, err
	}
	s.audit(ctx, req, "read_remote_file", in.SessionID, in.RemotePath, nil, nil)
	return nil, readFileOut{
		Path:      in.RemotePath,
		Content:   string(data),
		Truncated: truncated,
		Size:      int64(len(data)),
	}, nil
}

func (s *Server) toolUploadFile(ctx context.Context, req *mcp.CallToolRequest, in uploadIn) (*mcp.CallToolResult, uploadOut, error) {
	if !s.env.envFilesEnabled() {
		return nil, uploadOut{}, fmt.Errorf("file tools are disabled in uniTerm settings")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, uploadOut{}, fmt.Errorf("sessionId is required")
	}
	// Local path must be inside an allowed directory (and exist — the copy
	// below will fail with a clear error if not).
	resolved, err := s.env.ResolveLocalPath(in.LocalPath)
	if err != nil {
		return nil, uploadOut{}, err
	}
	fe, ok := s.env.FileSession(in.SessionID)
	if !ok {
		return nil, uploadOut{}, fmt.Errorf("session %s not found or not an SSH session", in.SessionID)
	}
	// Transfers follow the same policy matrix as exec, graded as write-level
	// risk (they move files across the trust boundary).
	if err := s.gateExec(ctx, req, in.SessionID, "upload "+in.LocalPath+" → "+in.RemotePath, RiskWrite); err != nil {
		return nil, uploadOut{}, err
	}
	n, err := fe.MCPWriteFile(resolved, in.RemotePath)
	s.audit(ctx, req, "upload_file", in.SessionID, in.LocalPath+" → "+in.RemotePath, nil, err)
	return nil, uploadOut{Bytes: n}, err
}

func (s *Server) toolDownloadFile(ctx context.Context, req *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	if !s.env.envFilesEnabled() {
		return nil, downloadOut{}, fmt.Errorf("file tools are disabled in uniTerm settings")
	}
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, downloadOut{}, fmt.Errorf("sessionId is required")
	}
	resolved, err := s.env.ResolveLocalPath(in.LocalPath)
	if err != nil {
		return nil, downloadOut{}, err
	}
	fe, ok := s.env.FileSession(in.SessionID)
	if !ok {
		return nil, downloadOut{}, fmt.Errorf("session %s not found or not an SSH session", in.SessionID)
	}
	if err := s.gateExec(ctx, req, in.SessionID, "download "+in.RemotePath+" → "+in.LocalPath, RiskWrite); err != nil {
		return nil, downloadOut{}, err
	}
	n, err := fe.MCPReadRemoteToFile(in.RemotePath, resolved)
	s.audit(ctx, req, "download_file", in.SessionID, in.RemotePath+" → "+in.LocalPath, nil, err)
	return nil, downloadOut{Bytes: n}, err
}

// envFilesEnabled mirrors Env.ToolsEnabled().Files with a nil-safe default
// (off) so a partially-wired Env cannot accidentally enable transfers.
func (e Env) envFilesEnabled() bool {
	if e.ToolsEnabled == nil {
		return false
	}
	return e.ToolsEnabled().Files
}
