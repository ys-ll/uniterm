package session

import (
	"context"
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

type ConnectionGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	// FTP-specific fields
	FtpEncryption string `json:"ftpEncryption,omitempty"` // "none"(default) | "auto" | "required"
	FtpPassive    bool   `json:"ftpPassive"`              // passive mode (default true)
	FtpEncoding   string `json:"ftpEncoding,omitempty"`   // "utf-8" | "gbk" | "shift-jis" | "latin-1"
	// SMB-specific fields
	SmbDomain string `json:"smbDomain,omitempty"`
	SmbShare  string `json:"smbShare,omitempty"`
	// WebDAV-specific fields
	WebdavURL   string `json:"webdavUrl,omitempty"`
	WebdavUseSSL bool  `json:"webdavUseSSL"` // default true
	// S3-specific fields
	S3Region string `json:"s3Region,omitempty"`
	S3Bucket string `json:"s3Bucket,omitempty"`
	// Terminal character encoding for ssh/telnet:
	// "" / "utf-8"(default) | "gbk" | "gb2312" | "gb18030" | "big5" | "shift-jis" | "euc-jp" | "euc-kr"
	Encoding string `json:"encoding,omitempty"`
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
	s.mu.RLock()
	cb := s.onDataCallback
	s.mu.RUnlock()
	if cb != nil {
		cb(data)
	}
}

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

func looksLikeZmodemHeader(data []byte) bool {
	for i := 0; i < len(data); i++ {
		if data[i] != '*' {
			continue
		}
		if i+1 >= len(data) || data[i+1] != '*' {
			continue
		}

		if i+3 < len(data) && data[i+2] == 0x18 && data[i+3] >= 'A' && data[i+3] <= 'C' {
			return true
		}

		hexCount := 0
		for j := i + 2; j < len(data); j++ {
			if isHexDigit(data[j]) {
				hexCount++
			} else {
				break
			}
		}
		if hexCount >= 14 {
			return true
		}
	}
	return false
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// RecordReadActivity updates the last-read timestamp for idle detection.
// Each session's readLoop should call this whenever data is received.
func (s *baseSession) RecordReadActivity() {
	s.lastReadTime.Store(time.Now().UnixNano())
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
