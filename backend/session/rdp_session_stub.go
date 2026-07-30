//go:build !windows

package session

import "fmt"

// RdpSession is a stub for non-Windows platforms.
// It satisfies the Session interface but Connect always returns an error.
type RdpSession struct {
	baseSession
}

func NewRdpSession(id string) *RdpSession {
	return &RdpSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "rdp",
			status:      StatusDisconnected,
		},
	}
}

func (s *RdpSession) Connect(_ ConnectionConfig) error {
	return fmt.Errorf("RDP is only supported on Windows")
}

func (s *RdpSession) Disconnect() error { return nil }

func (s *RdpSession) IsConnected() bool { return false }

func (s *RdpSession) Resize(_, _ int) error { return nil }

func (s *RdpSession) Write(_ []byte) error { return nil }

// Stub methods called from app.go

func (s *RdpSession) ClientAreaScreenRect() (x, y, w, h int) { return }

func (s *RdpSession) SetParentHwnd(_ uintptr) {}

func (s *RdpSession) SetPosition(_, _, _, _ int) {}

func (s *RdpSession) Show() {}

func (s *RdpSession) Hide() {}

func (s *RdpSession) SetFullScreen(_ bool) {}

func (s *RdpSession) SetOnFullScreenExit(_ func()) {}
