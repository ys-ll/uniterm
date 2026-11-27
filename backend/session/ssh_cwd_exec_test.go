package session

import (
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCwdExecFraming(t *testing.T) {
	const marker = "__UNITERM_CWD_TEST__"
	payload := []byte("stat\x00/tmp/中文 space\n\n\x00stat")
	data := append([]byte("Login banner without a final newline"+marker+"\n"), payload...)
	data = append(data, []byte("\x00"+marker+"\x000\x00")...)
	got, err := decodeCwdExecOutput(data, marker)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("payload=%q err=%v", got, err)
	}
	for _, invalid := range [][]byte{nil, []byte(marker + "\ntruncated"), []byte(marker + "\nbody\x00" + marker + "\x001\x00")} {
		if _, err := decodeCwdExecOutput(invalid, marker); err == nil {
			t.Fatal("accepted incomplete/failed response")
		}
	}
	var output cwdExecOutput
	_, err = io.Copy(&output, io.LimitReader(bytes.NewReader(make([]byte, 300*1024)), 300*1024))
	if err == nil || output.buffer.Len() > 256*1024 {
		t.Fatal("exec output memory limit bypassed")
	}
}

func TestCwdExecRejectsPIDReuse(t *testing.T) {
	want, err := parseCwdProcess([]byte(procStat(101, 42, 101, 101, 1101)))
	if err != nil {
		t.Fatal(err)
	}
	fs := &cwdExecFS{context.Background(), func(context.Context, string) ([]byte, error) {
		return []byte(procStat(101, 42, 101, 101, 1101) + "\x00/tmp\n\x00" + procStat(101, 42, 101, 101, 9999)), nil
	}}
	if _, err := readTerminalCwd(fs, want); !errors.Is(err, errCwdProcessChanged) {
		t.Fatalf("PID reuse accepted: %v", err)
	}
}

// Emulate proc responses over a real SSH exec channel. Linux-specific tests
// below separately run the actual shell scripts against real procfs/PTYs.
func mockCwdExec(fs *testCwdFS, command string) ([]byte, error) {
	marker := regexp.MustCompile(`__UNITERM_CWD_[A-Za-z0-9]+__`).FindString(command)
	if marker == "" {
		return nil, errors.New("missing exec framing")
	}
	var payload []byte
	if strings.Contains(command, "/proc/[0-9]*/stat") {
		fs.mu.Lock()
		payload = append(payload, fs.files["/proc/self/stat"]...)
		payload = append(payload, 0)
		for _, pid := range fs.children {
			payload = append(payload, fs.files["/proc/"+strconv.Itoa(pid)+"/stat"]...)
			payload = append(payload, 0)
		}
		fs.mu.Unlock()
	} else {
		path := regexp.MustCompile(`/proc/[0-9]+/(?:stat|cwd|exe)`).FindString(command)
		if path == "" {
			return nil, errors.New("unexpected exec query")
		}
		base := path[:strings.LastIndexByte(path, '/')]
		if strings.Contains(command, base+"/cwd") {
			before, err := fs.ReadFile(base + "/stat")
			if err != nil {
				return nil, err
			}
			cwd, err := fs.ReadLink(base + "/cwd")
			if err != nil {
				return nil, err
			}
			after, err := fs.ReadFile(base + "/stat")
			if err != nil {
				return nil, err
			}
			payload = append(payload, before...)
			payload = append(payload, 0)
			payload = append(payload, cwd...)
			payload = append(payload, '\n', 0)
			payload = append(payload, after...)
		} else {
			link, err := fs.ReadLink(path)
			if err != nil {
				return nil, err
			}
			payload = []byte(link + "\n")
		}
	}
	framed := append([]byte("Remote login noise\n"+marker+"\n"), payload...)
	return append(framed, []byte("\x00"+marker+"\x000\x00")...), nil
}

func TestSSHCwdSCPExecDeniedPreservesTerminal(t *testing.T) {
	fs := newTestCwdFS()
	addr, counts := startCwdSSHServer(t, fs, "deny")
	config := probeTestConfig(addr)
	config.FileTransferProto = "scp"
	s := NewSSHSession("cwd-scp-denied")
	if err := s.Connect(config); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()
	if s.Status() != StatusConnected || counts.input.Load() != 0 || counts.subsystems.Load() != 0 {
		t.Fatal("denied exec affected terminal")
	}
	if err := s.Write([]byte("still usable\r")); err != nil {
		t.Fatal(err)
	}
}

func TestSCPListingPreservesDirectory(t *testing.T) {
	var paths []string
	var pathsMu sync.Mutex
	addr, _ := startCwdSSHServer(t, newTestCwdFS(), "deny", func(command string) ([]byte, error) {
		pathsMu.Lock()
		paths = append(paths, command)
		pathsMu.Unlock()
		return []byte("total 0\n"), nil
	})
	client := dialTestSSHClient(t, addr)
	defer client.Close()
	s := NewSCPSession("scp-list-cwd")
	s.sshClient = client
	s.cwd = "/home/u"
	if _, err := s.ListRemote("/etc"); err != nil {
		t.Fatal(err)
	}
	if result, err := s.ListRemote(""); err != nil || result.Dir != "/etc" {
		t.Fatalf("refresh=%+v err=%v", result, err)
	}
	pathsMu.Lock()
	defer pathsMu.Unlock()
	if len(paths) != 2 || paths[0] != "ls -la '/etc'" || paths[1] != paths[0] {
		t.Fatalf("listing commands=%v", paths)
	}
}

func TestCwdExecTimeoutPreservesTerminal(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	addr, _ := startCwdSSHServer(t, newTestCwdFS(), "deny", func(string) ([]byte, error) { <-release; return nil, nil })
	client := dialTestSSHClient(t, addr)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := runCwdExec(ctx, client, "true"); err == nil {
		t.Fatal("expected exec timeout")
	}
	if time.Since(started) > time.Second {
		t.Fatal("exec timeout was not bounded")
	}
	terminal, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err := terminal.Shell(); err != nil {
		t.Fatalf("exec timeout closed transport: %v", err)
	}
}
