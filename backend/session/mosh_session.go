package session

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	mosh "github.com/unixshells/mosh-go"
)

type MoshSession struct {
	baseSession
	moshClient *mosh.Client
	sshClient  *ssh.Client
	cancel     context.CancelFunc
	quit       chan struct{}
	quitOnce   sync.Once
}

func NewMoshSession(id string) *MoshSession {
	return &MoshSession{
		baseSession: baseSession{
			id:          id,
			sessionType: "mosh",
			status:      StatusDisconnected,
		},
		quit: make(chan struct{}),
	}
}

func (s *MoshSession) Connect(config ConnectionConfig) error {
	s.setStatus(StatusConnecting)
	s.title = fmt.Sprintf("%s@%s (mosh)", config.User, config.Host)

	// Step 1: SSH to remote and start mosh-server to get key + UDP port.
	authMethods := makeSSHAuthMethods(config, nil)
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	clientConfig := &ssh.ClientConfig{
		User:            config.User,
		Auth:            authMethods,
		Timeout:         30 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := net.DialTimeout("tcp", addr, clientConfig.Timeout)
	if err != nil {
		s.setStatus(StatusError)
		return fmt.Errorf("mosh ssh dial: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, clientConfig)
	if err != nil {
		conn.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("mosh ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)

	key, udpPort, err := startMoshServer(client)
	if err != nil {
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("mosh-server: %w", err)
	}

	s.sshClient = client

	// Step 2: Dial mosh server over UDP.
	moshClient, err := mosh.Dial(config.Host, udpPort, key)
	if err != nil {
		client.Close()
		s.setStatus(StatusError)
		return fmt.Errorf("mosh dial: %w", err)
	}

	s.moshClient = moshClient
	s.setStatus(StatusConnected)

	// Send the initial terminal size.  Transport().SetPending injects the
	// resize as the next state in the mosh protocol, bypassing the Client
	// action queue which would otherwise wait for a server ACK.
	cols, rows := s.getInitialSize(80, 24)
	resizeBytes := mosh.MarshalUserMessage([]mosh.UserInstruction{
		{Width: int32(cols), Height: int32(rows)},
	})
	moshClient.Transport().SetPending(resizeBytes)

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	go s.readLoop(ctx)
	go s.runPostLoginScript(ctx, config.PostLoginScript)

	return nil
}

func (s *MoshSession) runPostLoginScript(ctx context.Context, script string) {
	s.baseSession.RunPostLoginScript(ctx, script, func(data []byte) {
		if s.moshClient != nil {
			s.moshClient.Send(data)
		}
	}, s.IsConnected)
}

func startMoshServer(client *ssh.Client) (key string, udpPort int, err error) {
	session, err := client.NewSession()
	if err != nil {
		return "", 0, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return "", 0, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return "", 0, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start("mosh-server new -s"); err != nil {
		return "", 0, fmt.Errorf("start mosh-server: %w", err)
	}

	var output strings.Builder
	stdoutScanner := bufio.NewScanner(stdout)
	for stdoutScanner.Scan() {
		output.WriteString(stdoutScanner.Text())
		output.WriteByte('\n')
	}
	stderrScanner := bufio.NewScanner(stderr)
	for stderrScanner.Scan() {
		output.WriteString(stderrScanner.Text())
		output.WriteByte('\n')
	}
	session.Wait()

	out := output.String()
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "MOSH_KEY=") {
			key = strings.TrimPrefix(line, "MOSH_KEY=")
		}
		if strings.HasPrefix(line, "MOSH_PORT=") {
			fmt.Sscanf(strings.TrimPrefix(line, "MOSH_PORT="), "%d", &udpPort)
		}

		if strings.HasPrefix(line, "MOSH CONNECT ") {
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				fmt.Sscanf(parts[2], "%d", &udpPort)
				key = parts[3]
			}
		}
	}

	if key == "" || udpPort == 0 {
		return "", 0, fmt.Errorf("missing key or port in mosh-server output: %s", out)
	}

	return key, udpPort, nil
}

func (s *MoshSession) readLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data := s.moshClient.Recv(100 * time.Millisecond)
		if len(data) > 0 {
			s.RecordReadActivity()
			s.emitData(append([]byte(nil), data...))
		}

		if s.moshClient == nil {
			return
		}
	}
}

func (s *MoshSession) Write(data []byte) error {
	if s.moshClient == nil {
		return fmt.Errorf("not connected")
	}
	s.moshClient.Send(data)
	return nil
}

func (s *MoshSession) Disconnect() error {
	s.quitOnce.Do(func() {
		close(s.quit)
	})
	if s.cancel != nil {
		s.cancel()
	}
	if s.moshClient != nil {
		s.moshClient.Close()
	}
	if s.sshClient != nil {
		s.sshClient.Close()
	}
	s.setStatus(StatusDisconnected)
	return nil
}

func (s *MoshSession) Resize(cols, rows int) error {
	s.SetPendingSize(cols, rows)
	if s.moshClient == nil {
		return fmt.Errorf("session not connected")
	}
	s.moshClient.Resize(uint16(cols), uint16(rows))
	return nil
}

func (s *MoshSession) IsConnected() bool {
	return s.Status() == StatusConnected
}
