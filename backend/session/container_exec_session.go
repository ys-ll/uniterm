package session

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ys-ll/uniterm/backend/log"
)

// ExecPTY mirrors container.PTYStream; container already imports session,
// so a minimal interface here avoids an import cycle.
type ExecPTY interface {
	Data() <-chan []byte
	Write(p []byte) error
	Resize(cols, rows int) error
	Close() error
}

var _ Session = (*ContainerExecSession)(nil)

type ContainerExecSession struct {
	baseSession
	pty      ExecPTY
	writeMu  sync.Mutex
	quitOnce sync.Once
	readOnce sync.Once
}

func NewContainerExecSession(id string, pty ExecPTY) *ContainerExecSession {
	s := &ContainerExecSession{
		baseSession: baseSession{id: id, sessionType: "container-exec", status: StatusConnected},
		pty:         pty,
	}
	return s
}

// StartOutput is called after the App installed callbacks and the desktop
// authority attached. The protocol stream buffers/backpressures until then.
func (s *ContainerExecSession) StartOutput() {
	s.readOnce.Do(func() { s.startEmitLoop(); go s.readLoop() })
}

// Connect is a no-op: the exec stream is already established by the App layer.
func (s *ContainerExecSession) Connect(_ ConnectionConfig) error { return nil }

func (s *ContainerExecSession) readLoop() {
	for data := range s.pty.Data() {
		s.RecordReadActivity()
		s.emitData(data)
	}
	s.setStatus(StatusDisconnected)
	s.emitData(disconnectNotice("Container exec session closed"))
}

func (s *ContainerExecSession) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.pty.Write(data)
}

func (s *ContainerExecSession) Resize(cols, rows int) error { return s.pty.Resize(cols, rows) }

func (s *ContainerExecSession) Disconnect() error {
	var err error
	s.quitOnce.Do(func() {
		err = s.pty.Close()
		s.setStatus(StatusDisconnected)
	})
	return err
}

func (s *ContainerExecSession) IsConnected() bool { return s.Status() == StatusConnected }

// K8sExecStream 把 k8s exec 的 WebSocket 通道（v4.channel.k8s.io 帧协议）
// 适配为通用 PTYStream，使 k8s Pod 终端与容器终端共用 session.ContainerExecSession。
type k8sExecStream struct {
	conn    *websocket.Conn
	data    chan []byte
	writeMu sync.Mutex
	once    sync.Once
}

// NewK8sExecStream 启动读循环并返回适配后的 ExecPTY。
func NewK8sExecStream(conn *websocket.Conn) ExecPTY {
	s := &k8sExecStream{conn: conn, data: make(chan []byte, 32)}
	go s.readLoop()
	return s
}

func (s *k8sExecStream) readLoop() {
	defer close(s.data)
	for {
		_, msg, err := s.conn.ReadMessage()
		if err != nil {
			log.Writef("[k8s-exec] stream closed: %v", err)
			return
		}
		ch, payload, ok := decodeFrame(msg)
		if !ok {
			continue
		}
		switch ch {
		case execChStdout, execChStderr:
			if len(payload) > 0 {
				s.data <- payload
			}
		case execChError:
			if len(payload) > 0 {
				log.Writef("[k8s-exec] error channel: %s", string(payload))
			}
		}
	}
}

func (s *k8sExecStream) Data() <-chan []byte { return s.data }

func (s *k8sExecStream) Write(data []byte) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, encodeStdin(data))
}

func (s *k8sExecStream) Resize(cols, rows int) error {
	frame, err := encodeResize(cols, rows)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.BinaryMessage, frame)
}

func (s *k8sExecStream) Close() error {
	var err error
	s.once.Do(func() {
		err = s.conn.Close()
	})
	return err
}
