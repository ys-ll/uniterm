package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// DefaultMCPDirEntries caps list_remote_dir rows.
const DefaultMCPDirEntries = 500

// File tools: streaming uploads/downloads and listing between the user's
// local filesystem and remote hosts. Transfers ride the session's
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
	LocalPath  string `json:"localPath" jsonschema:"local file path to upload"`
	RemotePath string `json:"remotePath" jsonschema:"remote destination path"`
}
type uploadOut struct {
	Bytes int64 `json:"bytes"`
}

type downloadIn struct {
	SessionID  string `json:"sessionId" jsonschema:"session id from list_sessions / connect"`
	RemotePath string `json:"remotePath" jsonschema:"remote file path"`
	LocalPath  string `json:"localPath" jsonschema:"local destination path (existing file at this path is overwritten)"`
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
		Description: "Upload a local file to a remote host over the connection's file transfer protocol (SFTP or SCP, per the connection's fileTransferProto setting). Streams disk to disk — content never passes through the model. Requires user approval.",
	}, s.toolUploadFile)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "download_file",
		Description: "Download a remote file to a local directory over the connection's file transfer protocol (SFTP or SCP, per the connection's fileTransferProto setting). Streams disk to disk; an existing file at the local destination is overwritten. Requires user approval.",
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
	fe, err := s.env.FileSession(in.SessionID)
	if err != nil {
		return nil, uploadOut{}, err
	}
	// Upload mutates the remote host: write-level risk.
	if err := s.gateExec(ctx, req, in.SessionID, "upload "+in.LocalPath+" → "+in.RemotePath, RiskWrite); err != nil {
		return nil, uploadOut{}, err
	}
	n, err := fe.MCPWriteFile(in.LocalPath, in.RemotePath)
	s.audit(ctx, req, "upload_file", in.SessionID, in.LocalPath+" → "+in.RemotePath, nil, err)
	return nil, uploadOut{Bytes: n}, err
}

func (s *Server) toolDownloadFile(ctx context.Context, req *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, downloadOut, error) {
	if strings.TrimSpace(in.SessionID) == "" {
		return nil, downloadOut{}, fmt.Errorf("sessionId is required")
	}
	fe, err := s.env.FileSession(in.SessionID)
	if err != nil {
		return nil, downloadOut{}, err
	}
	// Download overwrites a local file at an unconstrained path — that is a
	// local mutation, so it grades as write and goes through the approval
	// dialog like upload does.
	if err := s.gateExec(ctx, req, in.SessionID, "download "+in.RemotePath+" → "+in.LocalPath, RiskWrite); err != nil {
		return nil, downloadOut{}, err
	}
	n, err := fe.MCPReadRemoteToFile(in.RemotePath, in.LocalPath)
	s.audit(ctx, req, "download_file", in.SessionID, in.RemotePath+" → "+in.LocalPath, nil, err)
	return nil, downloadOut{Bytes: n}, err
}
