package session

import (
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/ys-ll/uniterm/backend/log"
)

type SessionManager struct {
	sessions map[string]Session
	mu       sync.RWMutex
}

func NewSessionManager() *SessionManager {
	return &SessionManager{
		sessions: make(map[string]Session),
	}
}

func (sm *SessionManager) Create(sessionType string, config ConnectionConfig) (Session, error) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	config.ID = uuid.New().String()

	var s Session
	switch sessionType {
	case "ssh":
		s = NewSSHSession(config.ID)
	case "sftp":
		s = NewSFTPSession(config.ID)
	case "scp":
		s = NewSCPSession(config.ID)
	case "rdp":
		s = NewRDPSession(config.ID)

	case "vnc":
		s = NewVNCSession(config.ID)

	case "local":
		s = NewLocalSession(config.ID)

	case "wsl":
		s = NewWSLSession(config.ID)

	case "wsl-file":
		s = NewWSLFileSession(config.ID)

	case "database":
		s = NewDatabaseSession(config.ID)

	case "monitor":
		s = NewMonitorSession(config.ID)

	case "telnet":
		s = NewTelnetSession(config.ID)

	case "mosh":
		s = NewMoshSession(config.ID)

	case "spice":
		s = NewSPICESession(config.ID)

	case "ftp":
		s = NewFTPSession(config.ID)

	case "smb":
		s = NewSMBSession(config.ID)

	case "webdav":
		s = NewWebDAVSession(config.ID)

	case "s3":
		s = NewS3Session(config.ID)

	case "serial":
		s = NewSerialSession(config.ID)

	case "tcp":
		s = NewTCPSession(config.ID)

	case "redis":
		s = NewRedisSession(config.ID)

	case "mongodb":
		s = NewMongoSession(config.ID)

	case "elasticsearch":
		s = NewElasticsearchSession(config.ID)

	case "x11-desktop":
		s = NewX11DesktopSession(config.ID)

	default:
		return nil, fmt.Errorf("unsupported session type: %s", sessionType)
	}

	// trackKeepalive promotes the app to an Android foreground service while
	// any session is connected (see keepalive.go). AddStatusListener (not
	// SetOnStatusChangeCallback) so the App layer's own callback wiring can
	// never overwrite it. All session types embed baseSession.
	trackKeepalive := func(s Session) {
		if l, ok := s.(interface {
			AddStatusListener(func(SessionStatus))
		}); ok {
			l.AddStatusListener(func(st SessionStatus) {
				onSessionStatus(s.ID(), st)
			})
		}
	}

	sm.sessions[config.ID] = s
	trackKeepalive(s)
	return s, nil
}

func (sm *SessionManager) Add(s Session) {
	sm.mu.Lock()
	sm.sessions[s.ID()] = s
	sm.mu.Unlock()
	if l, ok := s.(interface {
		AddStatusListener(func(SessionStatus))
	}); ok {
		l.AddStatusListener(func(st SessionStatus) {
			onSessionStatus(s.ID(), st)
		})
	}
}

// Init evicts dead sessions from the map at startup. A previous run may
// have left behind sessions in StatusDisconnected / StatusError state
// (failed/abandoned entries) that the bounded in-memory map would
// otherwise carry forward forever.
func (sm *SessionManager) Init() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	for id, s := range sm.sessions {
		switch s.Status() {
		case StatusDisconnected, StatusError:
			delete(sm.sessions, id)
		}
	}
}

func (sm *SessionManager) Close(sessionID string) error {
	sess, err := sm.Detach(sessionID)
	if err != nil {
		return err
	}
	return sess.Disconnect()
}

// Detach transfers teardown to a caller that still owns in-flight setup.
func (sm *SessionManager) Detach(sessionID string) (Session, error) {
	sm.mu.Lock()
	sess, ok := sm.sessions[sessionID]
	delete(sm.sessions, sessionID)
	sm.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	return sess, nil
}

func (sm *SessionManager) Get(sessionID string) (Session, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[sessionID]
	return s, ok
}

func (sm *SessionManager) List() []SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	infos := make([]SessionInfo, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		infos = append(infos, SessionInfo{
			ID:     s.ID(),
			Type:   s.Type(),
			Title:  s.Title(),
			Status: s.Status(),
		})
	}
	return infos
}

func (sm *SessionManager) CloseAll() {
	sm.mu.Lock()
	sessions := make([]Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		sessions = append(sessions, s)
	}
	clear(sm.sessions)
	sm.mu.Unlock()

	for _, s := range sessions {
		if err := s.Disconnect(); err != nil {
			log.Writef("session %s disconnect error: %v", s.ID(), err)
		}
	}
}
