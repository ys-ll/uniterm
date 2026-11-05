package localsrv

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// StartSSHServer starts an SSH server on the given port serving an interactive
// local shell, TCP forwarding and an SFTP subsystem restricted to rootDir.
// Authentication is by username/password; both are required — the shell
// grants full access to this machine. Returns the server id.
func (m *Manager) StartSSHServer(cfg SSHConfig) (string, error) {
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return "", fmt.Errorf("invalid port: %d", cfg.Port)
	}
	if cfg.Username == "" || cfg.Password == "" {
		return "", fmt.Errorf("username and password are required")
	}
	absRoot, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return "", fmt.Errorf("invalid root directory: %w", err)
	}

	id, ctx := m.newServer()
	m.log(ServiceSSH, id, fmt.Sprintf("Starting SSH/SFTP server on port %d ...", cfg.Port))

	// Generate a host key (RSA 2048) for this server instance.
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to generate host key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to create signer: %w", err)
	}

	sshCfg := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			// Require an exact credential match.
			if c.User() == cfg.Username && string(pass) == cfg.Password {
				return nil, nil
			}
			return nil, fmt.Errorf("authentication failed")
		},
	}
	sshCfg.AddHostKey(signer)

	ln, err := listen(ctx, "tcp", fmt.Sprintf(":%d", cfg.Port))
	if err != nil {
		m.finish(id)
		return "", fmt.Errorf("failed to listen on port %d: %w", cfg.Port, err)
	}

	// Track every accepted connection so shutdown (Stop/StopAll) can close
	// them. This unblocks the SSH handshake in handleSSHConn and any open
	// session, which lets the accept goroutine's WaitGroup actually drain and
	// the server goroutine terminate — instead of leaking connection
	// goroutines forever when the service or app is stopped.
	var connMu sync.Mutex
	active := make(map[net.Conn]struct{})
	closeConns := func() {
		connMu.Lock()
		for c := range active {
			_ = c.Close()
		}
		connMu.Unlock()
	}

	// Close already-accepted connections as soon as the context is cancelled
	// (belt-and-braces in case Stop is never called directly).
	go func() {
		<-ctx.Done()
		closeConns()
	}()

	m.setStop(id, closeConns)

	go func() {
		defer m.finish(id)
		m.state(ServiceSSH, id, true)
		m.log(ServiceSSH, id, "SSH/SFTP server started")

		var wg sync.WaitGroup
		for {
			conn, err := ln.Accept()
			if err != nil {
				if ctx.Err() != nil {
					break
				}
				continue
			}
			connMu.Lock()
			active[conn] = struct{}{}
			connMu.Unlock()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					connMu.Lock()
					delete(active, conn)
					connMu.Unlock()
				}()
				defer conn.Close()
				m.handleSSHConn(ctx, id, conn, sshCfg, absRoot)
			}()
		}
		wg.Wait()
		m.state(ServiceSSH, id, false)
		m.log(ServiceSSH, id, "SSH/SFTP server stopped")
	}()

	m.startTimeout(id, ServiceSSH, cfg.TimeoutSeconds)
	return id, nil
}

// handleSSHConn serves session and direct-tcpip channels after authentication.
func (m *Manager) handleSSHConn(ctx context.Context, id string, conn net.Conn, sshCfg *ssh.ServerConfig, rootDir string) {
	sConn, chans, reqs, err := ssh.NewServerConn(conn, sshCfg)
	if err != nil {
		m.log(ServiceSSH, id, fmt.Sprintf("SSH handshake failed from %s: %v", conn.RemoteAddr(), err))
		return
	}
	defer sConn.Close()
	ctx, cancel := context.WithCancel(ctx)
	var forwards sync.WaitGroup
	defer func() {
		cancel()
		forwards.Wait()
	}()
	m.log(ServiceSSH, id, fmt.Sprintf("SSH login from %s (user: %s)", conn.RemoteAddr(), sConn.User()))

	// Discard global requests (required by the ssh package).
	go ssh.DiscardRequests(reqs)

	for newChannel := range chans {
		if newChannel.ChannelType() == "direct-tcpip" {
			forwards.Add(1)
			go func() {
				defer forwards.Done()
				m.handleSSHForward(ctx, id, newChannel)
			}()
			continue
		}
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "unsupported channel type")
			continue
		}
		ch, reqs, err := newChannel.Accept()
		if err != nil {
			continue
		}
		go m.handleSSHSession(ctx, id, ch, reqs, rootDir)
	}
}

// directTCPIPRequest is the SSH direct-tcpip channel payload (RFC 4254).
type directTCPIPRequest struct {
	Host       string
	Port       uint32
	OriginHost string
	OriginPort uint32
}

func (m *Manager) handleSSHForward(ctx context.Context, id string, request ssh.NewChannel) {
	var dest directTCPIPRequest
	if err := ssh.Unmarshal(request.ExtraData(), &dest); err != nil || dest.Host == "" || dest.Port == 0 || dest.Port > 65535 {
		_ = request.Reject(ssh.ConnectionFailed, "invalid direct-tcpip destination")
		return
	}
	addr := net.JoinHostPort(dest.Host, fmt.Sprint(dest.Port))
	target, err := (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, "tcp", addr)
	if err != nil {
		m.log(ServiceSSH, id, fmt.Sprintf("SSH forwarding to %s failed: %v", addr, err))
		_ = request.Reject(ssh.ConnectionFailed, "failed to connect to destination")
		return
	}
	defer target.Close()
	ch, reqs, err := request.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	closeBoth := func() {
		_ = target.Close()
		_ = ch.Close()
	}
	stopClose := context.AfterFunc(ctx, closeBoth)
	defer stopClose()
	// The request stream ends on channel close, including while the target
	// is idle. EOF on channel data alone may only be a write half-close.
	go func() {
		ssh.DiscardRequests(reqs)
		cancel()
	}()
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := io.Copy(ch, target); err != nil {
			closeBoth()
			return
		}
		_ = ch.CloseWrite()
	}()
	if _, err := io.Copy(target, ch); err != nil {
		closeBoth()
	} else {
		_ = target.(*net.TCPConn).CloseWrite()
	}
	<-done
}

// handleSSHSession handles a session channel: interactive local shell,
// SFTP subsystem and simple exec commands.
func (m *Manager) handleSSHSession(ctx context.Context, id string, ch ssh.Channel, reqs <-chan *ssh.Request, rootDir string) {
	defer ch.Close()
	// Requested terminal size, carried by pty-req and window-change. The
	// shell is started on the "shell" request using the last pty-req size.
	cols, rows := 0, 0
	for req := range reqs {
		switch req.Type {
		case "subsystem":
			if len(req.Payload) >= 4 && string(req.Payload[4:]) == "sftp" {
				req.Reply(true, nil)
				m.serveSFTP(id, ch, rootDir)
				return
			}
			req.Reply(false, nil)
		case "exec":
			// Minimal exec support: echo back the command (not a real shell).
			req.Reply(true, nil)
			fmt.Fprintf(ch, "uniTerm local SSH server — command execution is not supported; use SFTP or an interactive shell\r\n")
			ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
			return
		case "pty-req":
			req.Reply(true, nil)
			cols, rows = parseWinsize(req.Payload)
		case "shell":
			req.Reply(true, nil)
			// Serve the shell synchronously (blocks until the session ends —
			// same lifetime model as serveSFTP). Requests that arrive while
			// the shell runs (window-change, env, signals) are serviced on a
			// side goroutine inside serveShell.
			m.serveShell(ctx, id, ch, reqs, cols, rows)
			return
		default:
			if req.WantReply {
				req.Reply(false, nil)
			}
		}
	}
}

// serveShell bridges the session channel to a local interactive shell PTY,
// started with the terminal size from pty-req. It runs until the shell exits,
// the client disconnects, or the server is stopped. Requests that arrive
// while the shell runs are serviced on a side goroutine; window-change
// resizes the PTY.
func (m *Manager) serveShell(ctx context.Context, id string, ch ssh.Channel, reqs <-chan *ssh.Request, cols, rows int) {
	defer ch.Close()

	sh, err := startLocalShell(ctx, cols, rows)
	if err != nil {
		m.log(ServiceSSH, id, fmt.Sprintf("shell start failed: %v", err))
		fmt.Fprintf(ch, "Failed to start shell: %v\r\n", err)
		_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 1})
		for range reqs { // drain so the connection teardown is not blocked
		}
		return
	}
	defer sh.Close()
	m.log(ServiceSSH, id, fmt.Sprintf("local shell started for session (%dx%d)", cols, rows))

	// clientGone is closed when the channel's input side ends; whichever side
	// ends first tears the whole session down (same pattern as the Telnet
	// bridge, so a client disconnect never leaves the shell running).
	clientGone := make(chan struct{})

	// Channel → shell: raw keystrokes.
	go func() {
		defer close(clientGone)
		buf := make([]byte, 8192)
		for {
			n, err := ch.Read(buf)
			if n > 0 {
				_ = sh.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Shell → channel: raw passthrough. Unlike the Telnet bridge there is no
	// control-sequence stripping — SSH terminal clients understand ANSI, and
	// ConPTY/PTY output is already UTF-8.
	go func() {
		for data := range sh.Data() {
			if len(data) > 0 {
				_, _ = ch.Write(data)
			}
		}
	}()

	// Service the remaining requests while the shell runs.
	go func() {
		for req := range reqs {
			switch req.Type {
			case "window-change":
				c, r := parseWinsize(req.Payload)
				if c > 0 && r > 0 {
					_ = sh.Resize(c, r)
				}
				if req.WantReply {
					req.Reply(true, nil)
				}
			default:
				if req.WantReply {
					req.Reply(false, nil)
				}
			}
		}
	}()

	select {
	case <-clientGone:
	case <-sh.Done():
	case <-ctx.Done():
	}
	_, _ = ch.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
	m.log(ServiceSSH, id, "local shell session ended")
}

// parseWinsize extracts cols/rows from a pty-req or window-change payload:
// string(term) followed by cols, rows, width px and height px as uint32s.
// Returns (0, 0) when the payload is malformed.
func parseWinsize(p []byte) (int, int) {
	if len(p) < 4 {
		return 0, 0
	}
	n := binary.BigEndian.Uint32(p[:4])
	if uint32(len(p)) < 4+n+16 {
		return 0, 0
	}
	cols := int(binary.BigEndian.Uint32(p[4+n : 4+n+4]))
	rows := int(binary.BigEndian.Uint32(p[4+n+4 : 4+n+8]))
	return cols, rows
}

// serveSFTP runs an SFTP server rooted at rootDir over the given channel.
func (m *Manager) serveSFTP(id string, ch ssh.Channel, rootDir string) {
	handlers := sftp.Handlers{
		FileGet:  sftpFileGet{root: rootDir},
		FilePut:  sftpFilePut{root: rootDir},
		FileCmd:  sftpFileCmd{root: rootDir},
		FileList: sftpFileList{root: rootDir},
	}

	srv := sftp.NewRequestServer(ch, handlers)
	if err := srv.Serve(); err != nil && err != io.EOF {
		m.log(ServiceSSH, id, fmt.Sprintf("SFTP session error: %v", err))
	}
}

// sftpFileGet implements sftp.FileReader for the GET method.
type sftpFileGet struct{ root string }

func (h sftpFileGet) Fileread(r *sftp.Request) (io.ReaderAt, error) {
	p, err := sftpResolve(h.root, r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

// sftpFilePut implements sftp.FileWriter for the PUT/OPEN methods.
type sftpFilePut struct{ root string }

func (h sftpFilePut) Filewrite(r *sftp.Request) (io.WriterAt, error) {
	p, err := sftpResolve(h.root, r.Filepath)
	if err != nil {
		return nil, err
	}
	return os.Create(p)
}

// sftpFileCmd implements sftp.FileCmder for Setstat/Rename/Rmdir/Mkdir/Remove.
type sftpFileCmd struct{ root string }

func (h sftpFileCmd) Filecmd(r *sftp.Request) error {
	p, err := sftpResolve(h.root, r.Filepath)
	if err != nil {
		return err
	}
	switch r.Method {
	case "Mkdir":
		return os.MkdirAll(p, 0o755)
	case "Rmdir", "Remove":
		return os.RemoveAll(p)
	case "Rename":
		tp, err := sftpResolve(h.root, r.Target)
		if err != nil {
			return err
		}
		return os.Rename(p, tp)
	default:
		// Setstat, Link, Symlink: no-op for a read-only-ish local file server.
		return nil
	}
}

// sftpFileList implements sftp.FileLister for List/Stat/Readlink methods.
type sftpFileList struct{ root string }

func (h sftpFileList) Filelist(r *sftp.Request) (sftp.ListerAt, error) {
	p, err := sftpResolve(h.root, r.Filepath)
	if err != nil {
		return nil, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	// Stat/Lstat (and a List of a plain file) must report the path's own
	// FileInfo. Never enumerate a directory's contents here: pkg/sftp's
	// filestat path takes the first returned entry as the Stat result, so
	// enumerating would report an arbitrary child's attributes (and error
	// out on empty directories entirely).
	if r.Method != "List" || !fi.IsDir() {
		return &fileListerAt{infos: []os.FileInfo{fi}}, nil
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	infos := make([]os.FileInfo, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	return &fileListerAt{infos: infos}, nil
}

// sftpResolve resolves a client-supplied path against rootDir and rejects any
// path that escapes the root directory. Accessing the root itself (rel == ".")
// is allowed; anything escaping above root ("..") is denied.
func sftpResolve(rootDir, clientPath string) (string, error) {
	cleanPath := filepath.Clean("/" + strings.TrimPrefix(clientPath, "/"))
	full := filepath.Join(rootDir, cleanPath)
	rel, err := filepath.Rel(rootDir, full)
	if err != nil {
		return "", fmt.Errorf("access denied: path outside root")
	}
	if rel == "." {
		return full, nil
	}
	if strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("access denied: path outside root")
	}
	return full, nil
}

// fileListerAt adapts a single FileInfo or a slice of FileInfos to the
// sftp.ListerAt interface.
type fileListerAt struct {
	infos []os.FileInfo
}

func (l *fileListerAt) ListAt(into []os.FileInfo, offset int64) (int, error) {
	if offset >= int64(len(l.infos)) {
		return 0, io.EOF
	}
	n := copy(into, l.infos[offset:])
	if n < len(into) {
		return n, io.EOF
	}
	return n, nil
}

// ensure fileListerAt satisfies sftp.ListerAt at compile time.
var _ sftp.ListerAt = (*fileListerAt)(nil)
