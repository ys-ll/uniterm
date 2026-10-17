package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/session"
	"golang.org/x/crypto/ssh"
)

// A real SSH handshake and PTY: no Connect/Write substitutes in the auth test.
type authSSHStats struct {
	shells, cols, rows atomic.Int32
}

func authSSHServer(t *testing.T, otp bool) (int, *authSSHStats) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{}
	if otp {
		config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			if _, err := challenge("", "", []string{"Password:"}, []bool{false}); err != nil {
				return nil, err
			}
			answers, err := challenge("", "", []string{"OTP:"}, []bool{false})
			if err != nil {
				return nil, err
			}
			if len(answers) != 1 || answers[0] != "fixture-answer" {
				return nil, fmt.Errorf("incorrect fixture answer")
			}
			return nil, nil
		}
	} else {
		config.PasswordCallback = func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "fixture-answer" {
				return nil, fmt.Errorf("incorrect fixture answer")
			}
			return nil, nil
		}
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	stats := &authSSHStats{}
	var connections sync.Map
	var wg sync.WaitGroup
	acceptDone := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			connections.Store(conn, true)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer conn.Close()
				defer connections.Delete(conn)
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						_ = incoming.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						return
					}
					wg.Add(2)
					go func() {
						defer wg.Done()
						_, _ = io.Copy(channel, channel)
					}()
					go func() {
						defer wg.Done()
						defer channel.Close()
						for request := range requests {
							if request.Type == "shell" {
								stats.shells.Add(1)
							}
							if request.Type == "pty-req" {
								var size struct {
									Term                      string
									Cols, Rows, Width, Height uint32
									Modes                     string
								}
								if ssh.Unmarshal(request.Payload, &size) == nil {
									stats.cols.Store(int32(size.Cols))
									stats.rows.Store(int32(size.Rows))
								}
							}
							_ = request.Reply(true, nil)
						}
					}()
				}
			}()
		}
	}()
	t.Cleanup(func() {
		listener.Close()
		<-acceptDone
		connections.Range(func(conn, _ any) bool { conn.(net.Conn).Close(); return true })
		wg.Wait()
	})
	return listener.Addr().(*net.TCPAddr).Port, stats
}

func TestWebConnectSSHAuthenticationInput(t *testing.T) {
	for _, otp := range []bool{false, true} {
		for _, caller := range []string{"legacy-desktop", "shared-desktop", "browser"} {
			t.Run(fmt.Sprintf("%s/otp=%t", caller, otp), func(t *testing.T) {
				port, stats := authSSHServer(t, otp)
				a := webTestApp(t)
				ctx := context.Background()
				if caller == "browser" {
					ctx = webTestBrowserContext(t, a)
				}
				prompt, connected, echoed := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)
				a.webTransport.native = func(e *application.CustomEvent) {
					data, _ := e.Data.(map[string]any)
					if e.Name == "web:terminal-attach" {
						go a.AcknowledgeWebTerminal(context.Background(), data["requestId"].(string))
					}
					if e.Name == "session:data" && (strings.Contains(data["data"].(string), "Password:") || strings.Contains(data["data"].(string), "OTP:")) {
						prompt <- struct{}{}
					}
					if e.Name == "session:status" && data["status"] == "connected" {
						connected <- struct{}{}
					}
					if e.Name == "session:data" && strings.Contains(data["data"].(string), "POST_AUTH") {
						echoed <- struct{}{}
					}
				}
				config := session.ConnectionConfig{Type: "ssh", Host: "127.0.0.1", Port: port, User: "fixture", AuthType: "password", CwdHookMode: session.CwdHookModeFollow, InitialCols: 80, InitialRows: 24}
				if otp {
					config.Password = "saved-fixture-password"
				}
				info, err := a.CreateSession("ssh", config)
				if err != nil {
					t.Fatal(err)
				}
				s, _ := a.sessionManager.Get(info.ID)
				// Also release the pre-fix blocked Connect after the red assertion.
				defer func() {
					if !s.IsConnected() {
						_ = s.Write([]byte{3})
					}
					_ = a.CloseSession(info.ID)
				}()
				if err := a.SessionStart(ctx, info.ID, config); err != nil {
					t.Fatal(err)
				}
				select {
				case <-prompt:
				case <-time.After(3 * time.Second):
					t.Fatal("SSH did not display an authentication prompt")
				}
				written := make(chan error, 1)
				go func() {
					if caller == "legacy-desktop" {
						written <- a.SessionWrite(info.ID, "fixture-answer\r")
					} else {
						written <- a.WebTerminalInput(info.ID, caller, "fixture-answer\r", 101, 31, false)
					}
				}()
				select {
				case err := <-written:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("authentication input blocked behind Connect")
				}
				select {
				case <-connected:
				case <-time.After(3 * time.Second):
					t.Fatal("terminal input did not authenticate SSH")
				}
				for i := 0; i < 5; i++ {
					if err := a.SessionStart(context.Background(), info.ID, config); err != nil {
						t.Fatal(err)
					}
				}
				if stats.shells.Load() != 1 {
					t.Fatalf("created %d shells instead of one", stats.shells.Load())
				}
				cols, rows := int32(101), int32(31)
				if caller == "legacy-desktop" {
					cols, rows = 80, 24
				}
				if stats.cols.Load() != cols || stats.rows.Load() != rows {
					t.Fatal("PTY did not inherit the auth-time grid")
				}
				terminal, _ := a.webTerminalFor(info.ID)
				waitWebConnectDone(t, terminal)
				if err := a.SessionWrite(info.ID, "POST_AUTH\r"); err != nil {
					t.Fatal(err)
				}
				select {
				case <-echoed:
				case <-time.After(time.Second):
					t.Fatal("normal input did not reach the authenticated PTY")
				}
			})
		}
	}
}

func TestWebConnectCloseDuringSSHAuthentication(t *testing.T) {
	for _, otp := range []bool{false, true} {
		t.Run(fmt.Sprintf("otp=%t", otp), func(t *testing.T) {
			port, stats := authSSHServer(t, otp)
			a := webTestApp(t)
			prompt := make(chan struct{}, 1)
			requested := make(chan struct{}, 1)
			var closedEvent, lateEvent atomic.Bool
			a.webTransport.native = func(e *application.CustomEvent) {
				if e.Name == "session:closed" {
					closedEvent.Store(true)
				} else if closedEvent.Load() && (e.Name == "session:status" || e.Name == "session:data") {
					lateEvent.Store(true)
				}
				if e.Name == "session:data" {
					text := e.Data.(map[string]any)["data"].(string)
					if strings.Contains(text, "Password:") || strings.Contains(text, "OTP:") {
						prompt <- struct{}{}
					}
				}
				if e.Name == "web:snapshot-request" {
					requested <- struct{}{}
				}
			}
			config := session.ConnectionConfig{Host: "127.0.0.1", Port: port, User: "fixture", AuthType: "password", CwdHookMode: session.CwdHookModeFollow}
			if otp {
				config.Password = "saved-fixture-password"
			}
			info, err := a.CreateSession("ssh", config)
			if err != nil {
				t.Fatal(err)
			}
			terminal, _ := a.webTerminalFor(info.ID)
			if err := a.SessionStart(context.Background(), info.ID, config); err != nil {
				t.Fatal(err)
			}
			select {
			case <-prompt:
			case <-time.After(3 * time.Second):
				t.Fatal("SSH did not prompt")
			}
			// Inventory and snapshots must remain available while Connect waits.
			if sessions := a.GetWebSessions(); len(sessions) != 1 || sessions[0].Status != session.StatusConnecting {
				t.Fatalf("connecting inventory: %+v", sessions)
			}
			snapshot := make(chan error, 1)
			go func() {
				_, err := a.GetWebTerminalSnapshot(context.Background(), info.ID)
				snapshot <- err
			}()
			select {
			case <-requested:
			case <-time.After(time.Second):
				t.Fatal("snapshot blocked behind authentication")
			}
			closed := make(chan error, 1)
			go func() { closed <- a.CloseSession(info.ID) }()
			select {
			case err := <-closed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("close blocked behind authentication")
			}
			select {
			case err := <-snapshot:
				if err == nil {
					t.Fatal("closed snapshot succeeded")
				}
			case <-time.After(time.Second):
				t.Fatal("close did not cancel snapshot")
			}
			waitWebConnectDone(t, terminal)
			if terminal.session.IsConnected() || terminal.session.(*session.SSHSession).Authenticating() || stats.shells.Load() != 0 {
				t.Fatal("closed authentication created a shell or kept accepting answers")
			}
			if len(a.ListSessions()) != 0 || len(a.GetWebSessions()) != 0 {
				t.Fatal("closed authentication remained in inventory")
			}
			if !closedEvent.Load() || lateEvent.Load() {
				t.Fatal("late connection callbacks recreated closed frontend state")
			}
			if a.SessionStart(context.Background(), info.ID, config) == nil || a.SessionWrite(info.ID, "late\r") == nil || a.WebTerminalInput(info.ID, "browser", "late\r", 80, 24, false) == nil {
				t.Fatal("closed authentication accepted retry or input")
			}
		})
	}
}

func waitWebConnectDone(t *testing.T, terminal *webTerminal) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		terminal.mu.Lock()
		connecting := terminal.connecting
		terminal.mu.Unlock()
		if !connecting {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Connect did not finish after authentication or cancellation")
		}
		time.Sleep(time.Millisecond)
	}
}

type blockedConnectTerminal struct {
	*controlledTerminal
	entered, release, disconnected chan struct{}
	disconnectOnce                 sync.Once
	onConnected                    func()
}

func (s *blockedConnectTerminal) Connect(session.ConnectionConfig) error {
	s.connects.Add(1)
	s.connected.Store(true) // Some sessions announce ready before setup returns.
	if s.onConnected != nil {
		s.onConnected()
	}
	close(s.entered)
	<-s.release
	return nil
}

func (s *blockedConnectTerminal) Disconnect() error {
	s.connected.Store(false)
	s.disconnectOnce.Do(func() { close(s.disconnected) })
	return nil
}

func TestWebConnectCloseAndRetryCannotResurrectLateSetup(t *testing.T) {
	a := webTestApp(t)
	s := &blockedConnectTerminal{controlledTerminal: &controlledTerminal{id: "blocked"}, entered: make(chan struct{}), release: make(chan struct{}), disconnected: make(chan struct{})}
	a.sessionManager.Add(s)
	if err := a.registerWebTerminal(s, session.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	var release sync.Once
	defer release.Do(func() { close(s.release) })
	if err := a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	<-s.entered
	if info := a.GetWebSessions()[0]; info.Status != session.StatusConnecting {
		t.Fatal("unfinished setup advertised connected")
	}
	if a.SessionWrite(s.id, "not-auth") == nil || a.WebTerminalInput(s.id, "browser", "not-auth", 80, 24, false) == nil {
		t.Fatal("connecting non-auth session accepted input")
	}
	if err := a.ResizeWebTerminal(s.id, "browser", 101, 31, true); err != nil {
		t.Fatal(err)
	}
	var retries sync.WaitGroup
	for i := 0; i < 20; i++ {
		retries.Add(1)
		go func() {
			defer retries.Done()
			if err := a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}); err != nil {
				t.Error(err)
			}
		}()
	}
	retries.Wait()
	if s.connects.Load() != 1 || len(s.writes) != 0 || s.cols != 0 || s.pendingCols != 101 {
		t.Fatal("pending connection duplicated setup, wrote input, or resized an uninitialized PTY")
	}
	closed := make(chan error, 1)
	go func() { closed <- a.CloseSession(s.id) }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("close waited for non-cancellable setup")
	}
	if len(a.ListSessions()) != 0 || len(a.GetWebSessions()) != 0 {
		t.Fatal("close did not immediately detach the in-flight session")
	}
	select {
	case <-s.disconnected:
		t.Fatal("Disconnect raced Connect's partially initialized handles")
	default:
	}
	release.Do(func() { close(s.release) })
	select {
	case <-s.disconnected:
	case <-time.After(time.Second):
		t.Fatal("late successful setup leaked its PTY")
	}
	if s.IsConnected() || a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}) == nil || a.WebTerminalInput(s.id, "browser", "late", 80, 24, false) == nil {
		t.Fatal("late success resurrected a detached session")
	}
	if len(a.GetWebSessions()) != 0 || a.webTransport.terminalSequence(s.id) != 0 {
		t.Fatal("late success resurrected terminal state")
	}
}

func TestWebConnectReadinessWaitsForSetupAndLatestGrid(t *testing.T) {
	a := webTestApp(t)
	s := &blockedConnectTerminal{controlledTerminal: &controlledTerminal{id: "early-ready"}, entered: make(chan struct{}), release: make(chan struct{}), disconnected: make(chan struct{})}
	s.onConnected = func() {
		a.emitSessionEvent(s, "session:status", map[string]any{"id": s.id, "status": session.StatusConnected})
	}
	a.sessionManager.Add(s)
	if err := a.registerWebTerminal(s, session.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 1)
	a.webTransport.native = func(e *application.CustomEvent) {
		if e.Name == "session:status" && e.Data.(map[string]any)["status"] == "connected" {
			ready <- struct{}{}
		}
	}
	var release sync.Once
	defer release.Do(func() { close(s.release) })
	if err := a.SessionStart(context.Background(), s.id, session.ConnectionConfig{}); err != nil {
		t.Fatal(err)
	}
	<-s.entered
	if err := a.ResizeWebTerminal(s.id, "browser", 109, 39, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
		t.Fatal("connected event preceded setup completion")
	default:
	}
	release.Do(func() { close(s.release) })
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("completed setup did not advertise readiness")
	}
	if err := a.WebTerminalInput(s.id, "browser", "first input", 109, 39, false); err != nil {
		t.Fatal(err)
	}
	if s.cols != 109 || s.rows != 39 || len(s.writes) != 1 {
		t.Fatal("readiness did not imply a writable PTY with the latest grid")
	}
}
