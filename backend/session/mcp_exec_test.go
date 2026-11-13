package session

import (
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/crypto/ssh"
)

// Each test uses a real SSH transport so channel opening, exec acknowledgement,
// exit status, output copying and cancellation follow the production path.
func newMCPTestSession(t *testing.T, serve func(ssh.NewChannel)) *SSHSession {
	t.Helper()
	signer, err := ssh.ParsePrivateKey([]byte(testHostKeyPEM))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		raw, err := ln.Accept()
		if err != nil {
			return
		}
		defer raw.Close()
		conn, channels, requests, err := ssh.NewServerConn(raw, cfg)
		if err != nil {
			return
		}
		defer conn.Close()
		go ssh.DiscardRequests(requests)
		for nc := range channels {
			go serve(nc)
		}
	}()
	client := dialTestSSHClient(t, ln.Addr().String())
	s := NewSSHSession("mcp-" + t.Name())
	s.clientRef = newSSHClientRef(client)
	s.setStatus(StatusConnected)
	t.Cleanup(func() {
		s.quitOnce.Do(func() { close(s.quit) })
		client.Close()
		waitMCP(t, func() bool { return s.clientRef.refs.Load() == 1 }, "command references released")
	})
	return s
}

func serveMCPTestCommand(nc ssh.NewChannel) {
	ch, requests, err := nc.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	for req := range requests {
		if req.Type != "exec" {
			_ = req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if ssh.Unmarshal(req.Payload, &payload) != nil {
			return
		}
		if payload.Command == "stall-start" {
			// Do not acknowledge exec. Channel close must unblock Start.
			_, _ = io.Copy(io.Discard, ch)
			return
		}
		_ = req.Reply(true, nil)
		_, _ = io.WriteString(ch, "stdout:"+payload.Command)
		_, _ = io.WriteString(ch.Stderr(), "stderr:"+payload.Command)
		if payload.Command == "long" {
			for signal := range requests {
				_ = signal.Reply(false, nil)
			}
			return
		}
		code := uint32(0)
		if payload.Command == "fail" {
			code = 7
		}
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Code uint32 }{code}))
		return
	}
}

func waitMCP(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out: " + message)
		}
		time.Sleep(time.Millisecond)
	}
}

func startMCPAsync(t *testing.T, s *SSHSession, command string, hardMs int) string {
	t.Helper()
	type result struct {
		id  string
		err error
	}
	returned := make(chan result, 1)
	go func() {
		_, _, _, id, err := s.MCPExec(command, 10, hardMs, 1024)
		returned <- result{id, err}
	}()
	select {
	case r := <-returned:
		if r.err != nil || r.id == "" {
			t.Fatalf("async exec: id=%q err=%v", r.id, r.err)
		}
		return r.id
	case <-time.After(time.Second):
		t.Fatal("sync wait budget did not return an async handle")
	}
	return ""
}

func awaitMCP(t *testing.T, id string) *mcpCommand {
	t.Helper()
	c, ok := mcpLookupCommand(id)
	if !ok {
		t.Fatal("command not registered: " + id)
	}
	select {
	case <-c.done:
	case <-time.After(3 * time.Second):
		t.Fatal("command did not finish: " + id)
	}
	return c
}

func TestMCPExecSynchronousResults(t *testing.T) {
	s := newMCPTestSession(t, serveMCPTestCommand)
	for _, command := range []string{"quick", "fail"} {
		out, errOut, code, id, err := s.MCPExec(command, 1000, 2000, 1024)
		wantCode := 0
		if command == "fail" {
			wantCode = 7
		}
		if string(out) != "stdout:"+command || string(errOut) != "stderr:"+command || code != wantCode || id != "" || (err != nil) != (wantCode != 0) {
			t.Fatalf("%s: out=%q stderr=%q code=%d id=%q err=%v", command, out, errOut, code, id, err)
		}
	}
}

func TestMCPExecAsyncPollingAndInterrupt(t *testing.T) {
	s := newMCPTestSession(t, serveMCPTestCommand)
	id := startMCPAsync(t, s, "long", 5000)
	waitMCP(t, func() bool {
		out, errOut, _, state, _ := s.MCPLatestOutput(id, 0, 1024)
		return string(out) == "stdout:long" && string(errOut) == "stderr:long" && state == "running"
	}, "pollable output")
	// Returned snapshots must not alias the live output buffer.
	out, _, _, _, _ := s.MCPLatestOutput(id, 0, 1024)
	out[0] = 'X'
	again, _, _, _, _ := s.MCPLatestOutput(id, 0, 1024)
	if string(again) != "stdout:long" {
		t.Fatal("poll mutated command output")
	}
	if s.clientRef.refs.Load() != 2 {
		t.Fatal("async command did not retain SSH client")
	}
	if err := s.MCPInterrupt(id, "SIGKILL"); err != nil {
		t.Fatal(err)
	}
	c := awaitMCP(t, id)
	_, _, code, state, err := c.snapshot()
	if state != "killed" || code != -1 || err == nil {
		t.Fatalf("state=%s code=%d err=%v", state, code, err)
	}
	if _, ok := mcpLookupCommand(id); !ok {
		t.Fatal("finished result removed before TTL")
	}
	// Cancelling one command must leave the shared transport usable.
	_, _, code, _, err = s.MCPExec("quick", 1000, 2000, 1024)
	if code != 0 || err != nil {
		t.Fatalf("shared transport: code=%d err=%v", code, err)
	}
}

func TestMCPExecConcurrencySlotsRecover(t *testing.T) {
	s := newMCPTestSession(t, serveMCPTestCommand)
	ids := make([]string, mcpMaxConcurrentCommands)
	for i := range ids {
		ids[i] = startMCPAsync(t, s, "long", 5000)
	}
	// A different session shares the same global budget.
	other := NewSSHSession("other")
	other.clientRef = s.clientRef
	other.setStatus(StatusConnected)
	_, _, _, _, err := other.MCPExec("quick", 10, 1000, 1024)
	if err == nil || !strings.Contains(err.Error(), "too many concurrent MCP commands") {
		t.Fatalf("limit: %v", err)
	}
	if err := s.MCPInterrupt(ids[0], "SIGKILL"); err != nil {
		t.Fatal(err)
	}
	awaitMCP(t, ids[0])
	_, _, code, _, err := other.MCPExec("quick", 1000, 2000, 1024)
	if code != 0 || err != nil {
		t.Fatalf("slot not recovered: code=%d err=%v", code, err)
	}
	for _, id := range ids[1:] {
		if err := s.MCPInterrupt(id, "SIGKILL"); err != nil {
			t.Fatal(err)
		}
		awaitMCP(t, id)
	}
}

func TestMCPExecHardTimeout(t *testing.T) {
	for _, command := range []string{"long", "stall-start"} {
		t.Run(command, func(t *testing.T) {
			s := newMCPTestSession(t, serveMCPTestCommand)
			id := startMCPAsync(t, s, command, 150)
			c := awaitMCP(t, id)
			_, _, _, state, err := c.snapshot()
			if state != "killed" || err == nil || !strings.Contains(err.Error(), "hard timeout") {
				t.Fatalf("state=%s err=%v", state, err)
			}
			_, _, code, _, err := s.MCPExec("quick", 1000, 2000, 1024)
			if code != 0 || err != nil {
				t.Fatalf("after timeout: code=%d err=%v", code, err)
			}
		})
	}
}

func TestMCPExecLateChannelClosed(t *testing.T) {
	gate := make(chan struct{})
	lateClosed := make(chan error, 1)
	s := newMCPTestSession(t, func(nc ssh.NewChannel) {
		<-gate
		ch, reqs, err := nc.Accept()
		if err != nil {
			lateClosed <- err
			return
		}
		defer ch.Close()
		go ssh.DiscardRequests(reqs)
		b := make([]byte, 1)
		_, err = ch.Read(b)
		lateClosed <- err
	})
	defer close(gate)
	id := startMCPAsync(t, s, "held-open", 150)
	c := awaitMCP(t, id)
	_, _, _, state, err := c.snapshot()
	if state != "killed" || err == nil {
		t.Fatalf("state=%s err=%v", state, err)
	}
	// Unblock NewSession only after the task timed out.
	gate <- struct{}{}
	select {
	case err := <-lateClosed:
		if err != io.EOF {
			t.Fatal(fmt.Errorf("late channel close: %w", err))
		}
	case <-time.After(time.Second):
		t.Fatal("late channel leaked")
	}
}

func TestMCPExecSessionClosed(t *testing.T) {
	s := newMCPTestSession(t, serveMCPTestCommand)
	id := startMCPAsync(t, s, "long", 5000)
	// Use quitOnce because cleanup also closes the lifecycle signal.
	s.quitOnce.Do(func() { close(s.quit) })
	c := awaitMCP(t, id)
	_, _, _, state, err := c.snapshot()
	if state != "session_closed" || err == nil {
		t.Fatalf("state=%s err=%v", state, err)
	}
}

func TestMCPCommandResultTTLStartsOnCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &mcpCommand{id: "ttl-test", state: "running"}
		mcpRegisterCommand(c)
		defer func() {
			mcpRegistry.mu.Lock()
			delete(mcpRegistry.commands, c.id)
			mcpRegistry.mu.Unlock()
		}()
		time.Sleep(2 * mcpCommandTTL)
		if _, ok := mcpLookupCommand(c.id); !ok {
			t.Fatal("running command expired")
		}
		c.setState("exited", 0, nil)
		mcpRetireCommand(c)
		time.Sleep(mcpCommandTTL - time.Nanosecond)
		if _, ok := mcpLookupCommand(c.id); !ok {
			t.Fatal("result expired before full retention period")
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if _, ok := mcpLookupCommand(c.id); ok {
			t.Fatal("completed result did not expire")
		}
	})
}

func TestMCPExecConcurrencySettingAppliesToNewCommands(t *testing.T) {
	SetMCPMaxConcurrentCommands(1)
	t.Cleanup(func() { SetMCPMaxConcurrentCommands(0) })
	s := newMCPTestSession(t, serveMCPTestCommand)
	ids := []string{startMCPAsync(t, s, "long", 5000)}
	rejected := func(limit int) {
		t.Helper()
		_, _, _, _, err := s.MCPExec("quick", 10, 1000, 1024)
		if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("max %d across", limit)) || !strings.Contains(err.Error(), "MCP settings") {
			t.Fatalf("expected configurable limit %d: %v", limit, err)
		}
	}
	rejected(1)
	SetMCPMaxConcurrentCommands(10)
	for len(ids) < 9 {
		ids = append(ids, startMCPAsync(t, s, "long", 5000))
	}
	SetMCPMaxConcurrentCommands(2)
	rejected(2)
	for _, id := range ids {
		_, _, _, state, _ := s.MCPLatestOutput(id, 0, 1024)
		if state != "running" {
			t.Fatalf("lowering limit interrupted command: %s", state)
		}
		if err := s.MCPInterrupt(id, "SIGKILL"); err != nil {
			t.Fatal(err)
		}
		awaitMCP(t, id)
	}
	_, _, code, _, err := s.MCPExec("quick", 1000, 2000, 1024)
	if err != nil || code != 0 {
		t.Fatalf("new command after releasing slots: %d, %v", code, err)
	}
}
