package session

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type SessionStatus string

const (
	StatusConnecting   SessionStatus = "connecting"
	StatusConnected    SessionStatus = "connected"
	StatusDisconnected SessionStatus = "disconnected"
	StatusError        SessionStatus = "error"
)

// disconnectNotice formats a red disconnect prompt with the local time it
// happened, so users can tell when a remote host dropped them. See issue #367.
func disconnectNotice(msg string) []byte {
	ts := time.Now().Format("2006-01-02 15:04:05")
	return []byte(fmt.Sprintf("\r\n\x1b[31m%s (%s) Press Enter to reconnect.\x1b[0m\r\n", msg, ts))
}

type ConnectionGroup struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentId *string `json:"parentId,omitempty"`
}

type ConnectionConfig struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	AuthType string `json:"authType"`
	// Password is stored in plaintext JSON. Will be migrated to OS keychain in a future iteration.
	Password string  `json:"password,omitempty"`
	KeyPath  string  `json:"keyPath,omitempty"`
	GroupId  *string `json:"groupId,omitempty"`
	// RDP-specific fields
	RdpFixedWidth  int  `json:"rdpFixedWidth,omitempty"`
	RdpFixedHeight int  `json:"rdpFixedHeight,omitempty"`
	RdpSmartSizing bool `json:"rdpSmartSizing"`
	RdpEnableNLA  bool `json:"rdpEnableNLA"`
	// Local terminal shell path
	ShellPath string `json:"shellPath,omitempty"`
	// Working directory for local terminal (defaults to user home directory if empty)
	Cwd string `json:"cwd,omitempty"`
	// Serial port configuration
	SerialPort     string  `json:"serialPort,omitempty"`
	SerialBaudRate int     `json:"serialBaudRate,omitempty"`
	SerialDataBits int     `json:"serialDataBits,omitempty"`
	SerialStopBits float64 `json:"serialStopBits,omitempty"`
	SerialParity   string  `json:"serialParity,omitempty"`
	// Database-specific fields
	DBType   string `json:"dbType,omitempty"`   // "mysql", "postgres", "rqlite", "oracle", "sqlserver"
	DBName   string `json:"dbName,omitempty"`   // default database name
	DBParams string `json:"dbParams,omitempty"` // extra DSN query parameters, e.g. "sslmode=require&connect_timeout=30"
	// Redis Sentinel fields (only used when RedisMode == "sentinel")
	RedisMode        string `json:"redisMode,omitempty"`        // ""/"standalone"(default) | "sentinel"
	RedisMasterName  string `json:"redisMasterName,omitempty"`  // Sentinel primary group name, e.g. "mymaster"
	RedisSentinels   string `json:"redisSentinels,omitempty"`   // comma-separated sentinel host:port list
	SentinelUser     string `json:"sentinelUser,omitempty"`     // Sentinel ACL user (optional)
	SentinelPassword string `json:"sentinelPassword,omitempty"` // Sentinel requirepass (optional)
	// SSH post-login script: commands to execute after successful login
	PostLoginScript string `json:"postLoginScript,omitempty"`
	// Post-login expect/send automation: interactive steps executed after login.
	PostLoginExpectSteps []PostLoginExpectStep `json:"postLoginExpectSteps,omitempty"`
	// SSH tunnel: reference to an existing SSH connection used as a jump host.
	// When set, the connection goes through local port forwarding:
	//   127.0.0.1:auto-port → tunnel SSH → target Host:Port
	TunnelSSHConnID   string `json:"tunnelSSHConnId,omitempty"`
	TunnelSSHUser     string `json:"tunnelSSHUser,omitempty"`
	TunnelSSHPassword string `json:"tunnelSSHPassword,omitempty"`
	// SFTP max concurrent transfers (0 = unlimited)
	SftpMaxConcurrency int `json:"sftpMaxConcurrency,omitempty"`
	// Initial terminal size reported by the frontend BEFORE the
	// SSH/local PTY is created. Without this the backend starts the
	// remote shell with the default 80x24 and Claude Code draws tables
	// at that width; by the time the frontend's fitAddon measures the
	// actual xterm cols and sends SessionResize, several lines are
	// already wrapped at 80 cols and the rest at the real cols — table
	// borders drift apart. 0/0 means "use default"; otherwise fed into
	// SetPendingSize before Connect so getInitialSize picks them up.
	InitialCols int `json:"initialCols,omitempty"`
	InitialRows int `json:"initialRows,omitempty"`
	// DeferConnect tells the App layer to skip the auto-Connect goroutine
	// when CreateSession is called. The frontend then calls SessionStart
	// after mounting the xterm terminal and writing InitialCols/InitialRows
	// via SetPendingSize — guarantees the PTY starts at the real xterm
	// size, not the 80x24 default that would otherwise wrap Claude Code
	// tables at the wrong column count.
	DeferConnect bool `json:"deferConnect,omitempty"`
	// FTP-specific fields
	FtpEncryption string `json:"ftpEncryption,omitempty"` // "none"(default) | "auto" | "required"
	FtpPassive    bool   `json:"ftpPassive"`              // passive mode (default true)
	FtpEncoding   string `json:"ftpEncoding,omitempty"`   // "utf-8" | "gbk" | "shift-jis" | "latin-1"
	// FtpSkipVerify opts in to tls.Config.InsecureSkipVerify for FTPS connections.
	// Defaults to false (verify enabled). Off by default preserves backwards
	// compatibility for users today who rely on it for self-signed certs —
	// but the toggle now exists so the choice is explicit, and a one-shot
	// session-log warning fires on connect when it is enabled.
	FtpSkipVerify bool `json:"ftpSkipVerify,omitempty"`
	// SMB-specific fields
	SmbDomain string `json:"smbDomain,omitempty"`
	SmbShare  string `json:"smbShare,omitempty"`
	// S3-specific fields
	S3Region string `json:"s3Region,omitempty"`
	S3Bucket string `json:"s3Bucket,omitempty"`
	// Terminal character encoding for ssh/telnet:
	// "" / "utf-8"(default) | "gbk" | "gb2312" | "gb18030" | "big5" | "shift-jis" | "euc-jp" | "euc-kr"
	Encoding string `json:"encoding,omitempty"`
	// K8s-specific fields
	K8sConfigPath   string `json:"k8sConfigPath,omitempty"`   // File 模式：kubeconfig 文件路径
	K8sConfigInline string `json:"k8sConfigInline,omitempty"` // Inline 模式：kubeconfig YAML 全文（明文存储，同其他连接密码策略）
	K8sContext      string `json:"k8sContext,omitempty"`      // 选中的 context 名，为空则用 current-context
	K8sNamespace    string `json:"k8sNamespace,omitempty"`    // 默认 namespace，"" = all
	K8sInsecureTls  bool   `json:"k8sInsecureTls,omitempty"`  // 覆盖 kubeconfig 中的 insecure-skip-tls-verify
	// LogOnConnect, when true, tells the App layer to enable the
	// session output log automatically the first time this panel binds
	// a session. It has no effect on later reconnects — a manually
	// stopped log stays stopped for the life of the panel.
	LogOnConnect bool `json:"logOnConnect,omitempty"`
}

// ConnectionStoreData is the top-level structure persisted to connections.json.
type ConnectionStoreData struct {
	Groups      []ConnectionGroup  `json:"groups"`
	Connections []ConnectionConfig `json:"connections"`
}

type SessionInfo struct {
	ID     string        `json:"id"`
	Type   string        `json:"type"`
	Title  string        `json:"title"`
	Status SessionStatus `json:"status"`
}

type Session interface {
	ID() string
	Type() string
	Title() string
	Status() SessionStatus

	Connect(config ConnectionConfig) error
	Disconnect() error
	IsConnected() bool
	Resize(cols, rows int) error
	// SetPendingSize stashes cols/rows to be applied at Connect time, for
	// the deferred-start flow where the frontend mounts the xterm and
	// measures the real size AFTER CreateSession has already created the
	// session object but BEFORE SessionStart runs Connect.
	SetPendingSize(cols, rows int)

	Write(data []byte) error
	SetOnDataCallback(cb func([]byte))
	SetOnBinaryCallback(cb func([]byte))
	SetOnStatusChangeCallback(cb func(SessionStatus))
	SetZmodemMode(bool)
	IsZmodemMode() bool
}

type baseSession struct {
	id               string
	sessionType      string
	title            string
	status           SessionStatus
	onDataCallback   func([]byte)
	onBinaryCallback func([]byte)
	onStatusCallback func(SessionStatus)
	mu               sync.RWMutex
	pendingCols      int
	pendingRows      int
	zmodemMode       bool
	lastReadTime     atomic.Int64
	// outputLogWriter, if non-nil, receives a copy of every byte emitted
	// via emitData. It is set by the App layer and lives longer than any
	// single session — a reconnect re-uses the same underlying logger by
	// installing the same writer on the new session.
	outputLogWriter func([]byte)
	outputLogMu     sync.RWMutex
	// logOnConnect mirrors ConnectionConfig.LogOnConnect so the App
	// layer can query it via AutoLogOnConnect() and decide whether to
	// enable the log the first time this session binds to a panel.
	logOnConnect bool
}

func (s *baseSession) ID() string            { return s.id }
func (s *baseSession) Type() string          { return s.sessionType }
func (s *baseSession) Title() string         { return s.title }
func (s *baseSession) Status() SessionStatus { s.mu.RLock(); defer s.mu.RUnlock(); return s.status }

func (s *baseSession) SetOnDataCallback(cb func([]byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onDataCallback = cb
}

func (s *baseSession) SetOnStatusChangeCallback(cb func(SessionStatus)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onStatusCallback = cb
}

func (s *baseSession) setStatus(st SessionStatus) {
	s.mu.Lock()
	s.status = st
	cb := s.onStatusCallback
	s.mu.Unlock()
	if cb != nil {
		cb(st)
	}
}

func (s *baseSession) emitData(data []byte) {
	s.outputLogMu.RLock()
	w := s.outputLogWriter
	s.outputLogMu.RUnlock()
	if w != nil {
		w(data)
	}
	s.mu.RLock()
	cb := s.onDataCallback
	s.mu.RUnlock()
	if cb != nil {
		cb(data)
	}
}

// SetOutputLogWriter installs (or clears with nil) the sink that
// receives each byte emitted via emitData. Ownership of the underlying
// logger lives at the App layer so it can outlive any single session
// and survive reconnects.
func (s *baseSession) SetOutputLogWriter(w func([]byte)) {
	s.outputLogMu.Lock()
	s.outputLogWriter = w
	s.outputLogMu.Unlock()
}

// SetLogOnConnect records the per-connection auto-log preference so
// the App layer can read it via AutoLogOnConnect(). Each session type
// calls this from Connect based on ConnectionConfig.LogOnConnect.
func (s *baseSession) SetLogOnConnect(v bool) { s.logOnConnect = v }

// AutoLogOnConnect reports whether this session was created from a
// connection configured to start logging automatically.
func (s *baseSession) AutoLogOnConnect() bool { return s.logOnConnect }

func (s *baseSession) SetPendingSize(cols, rows int) {
	s.mu.Lock()
	s.pendingCols = cols
	s.pendingRows = rows
	s.mu.Unlock()
}

func (s *baseSession) GetPendingSize() (cols, rows int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pendingCols, s.pendingRows
}

func (s *baseSession) getInitialSize(defCols, defRows int) (int, int) {
	cols, rows := s.GetPendingSize()
	if cols <= 0 {
		cols = defCols
	}
	if rows <= 0 {
		rows = defRows
	}
	return cols, rows
}

func (s *baseSession) SetZmodemMode(v bool) {
	s.mu.Lock()
	s.zmodemMode = v
	s.mu.Unlock()
}

func (s *baseSession) IsZmodemMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.zmodemMode
}

func (s *baseSession) SetOnBinaryCallback(cb func([]byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onBinaryCallback = cb
}

func (s *baseSession) emitBinary(data []byte) {
	s.mu.RLock()
	cb := s.onBinaryCallback
	s.mu.RUnlock()
	if cb != nil {
		cb(data)
	}
}

// looksLikeZmodemHeader reports whether data contains a ZMODEM frame header.
// A real header is ZPAD ZPAD ZDLE frame-type: `**` `\x18` `[A-C]`. The ZDLE
// (0x18) control byte is mandatory and effectively never appears in ordinary
// terminal output, so requiring it avoids false positives — e.g. vim rendering
// a file whose content contains `**` followed by a long hex string, which used
// to trip a looser heuristic and flip the session into binary mode, crashing
// the remote shell (issue #242).
func looksLikeZmodemHeader(data []byte) bool {
	for i := 0; i+3 < len(data); i++ {
		if data[i] == '*' && data[i+1] == '*' && data[i+2] == 0x18 &&
			data[i+3] >= 'A' && data[i+3] <= 'C' {
			return true
		}
	}
	return false
}

// RecordReadActivity updates the last-read timestamp for idle detection.
// Each session's readLoop should call this whenever data is received.
func (s *baseSession) RecordReadActivity() {
	s.lastReadTime.Store(time.Now().UnixNano())
}

// idleSince reports how long since the last read activity. Returns -1 if no
// read has ever been recorded. Used for disconnect diagnostics.
func (s *baseSession) idleSince() time.Duration {
	ns := s.lastReadTime.Load()
	if ns == 0 {
		return -1
	}
	return time.Since(time.Unix(0, ns))
}

// waitIdle blocks until no read activity has occurred for the given idle
// duration, or the overall timeout expires. It returns true on idle detection.
func (s *baseSession) waitIdle(timeout, idle time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		last := time.Unix(0, s.lastReadTime.Load())
		if !last.IsZero() && time.Since(last) >= idle {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// RunPostLoginScript sends each non-empty line of script after the terminal
// output goes idle, and waits for idle between commands. Stops early if ctx
// is cancelled or isConnected returns false.
func (s *baseSession) RunPostLoginScript(ctx context.Context, script string, send func([]byte), isConnected func() bool) {
	if strings.TrimSpace(script) == "" {
		return
	}
	// Wait for shell to finish initialization.
	if !s.waitIdle(5*time.Second, 300*time.Millisecond) {
		return
	}
	lines := strings.Split(script, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !isConnected() {
			return
		}
		send([]byte(line + "\r"))
		// Wait for command output to settle.
		if !s.waitIdle(3*time.Second, 300*time.Millisecond) {
			return
		}
	}
}
