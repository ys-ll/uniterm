package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// --- LIST names must not carry the server's column-padding spaces ---

// Legacy/embedded devices pad every column of their `ls`-style LIST output
// with several spaces. The LIST parser consumes a single delimiter after the
// last field, so the surplus spaces land inside the entry name (" led_data").
// The file panel hides them (HTML collapses whitespace), but the first
// navigation into the entry fails: the client sends CWD "/ led_data" and the
// server — which only knows "led_data" — rejects it. Every name coming out
// of ftpList must therefore be trimmed at both ends; interior spaces stay put.
func TestFTPListTrimsPaddedNames(t *testing.T) {
	srv := newFakeFTPServer(t)
	s := NewFTPSession("ftp-test")
	cfg := ConnectionConfig{
		Host:          srv.host,
		Port:          srv.port,
		User:          "tester",
		Password:      "secret",
		FtpEncryption: "none",
	}
	if err := s.Connect(cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := s.ListRemote("")
	if err != nil {
		t.Fatalf("root listing: %v", err)
	}

	want := []struct {
		name  string
		isDir bool
	}{
		{"etc", true},
		{"usr", true},
		{"file1.txt", false},
		{"my file2.txt", false},
	}
	if len(res.Files) != len(want) {
		t.Fatalf("root listing: got %d entries, want %d", len(res.Files), len(want))
	}
	for i, w := range want {
		got := res.Files[i]
		if got.Name != w.name {
			t.Errorf("entry %d name = %q, want %q", i, got.Name, w.name)
		}
		if got.IsDir != w.isDir {
			t.Errorf("entry %d (%s) isDir = %v, want %v", i, got.Name, got.IsDir, w.isDir)
		}
	}

	// Navigate with the names exactly as the file panel received them. The
	// fake server only accepts CWD for the clean paths, so a padded name
	// would fail exactly like the real device does.
	if _, err := s.ChangeRemoteDir("etc"); err != nil {
		t.Errorf("ChangeRemoteDir(\"etc\"): %v", err)
	}
	if _, err := s.ListRemote("/usr"); err != nil {
		t.Errorf("ListRemote(\"/usr\"): %v", err)
	}
}

// setFTPCodec configures s with the encoding a Connect(config) would pick.
func setFTPCodec(s *FTPSession, name string) {
	if name == "" || name == "utf-8" {
		name = ""
	}
	s.encName = name
	s.enc = encodingByName(name)
}

// --- Name decoding (garbled Chinese directory names) ---

func TestFTPDecodeNameUTF8Fallback(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	// Server sends GBK bytes for "中文" — invalid UTF-8, must be decoded.
	if got := s.ftpDecodeName("\xd6\xd0\xce\xc4"); got != "中文" {
		t.Fatalf("GBK fallback decode: got %q, want 中文", got)
	}
	// Valid UTF-8 names must pass through untouched.
	if got := s.ftpDecodeName("日本語.txt"); got != "日本語.txt" {
		t.Fatalf("valid UTF-8 passthrough: got %q", got)
	}
}

func TestFTPDecodeNameExplicitGBK(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "gbk")

	if got := s.ftpDecodeName("\xd6\xd0\xce\xc4"); got != "中文" {
		t.Fatalf("GBK decode: got %q, want 中文", got)
	}
}

func TestFTPDecodeNameShiftJIS(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "shift-jis")

	if got := s.ftpDecodeName("\x93\xfa\x96{"); got != "日本" {
		t.Fatalf("Shift-JIS decode: got %q, want 日本", got)
	}
}

func TestFTPDecodeNameLatin1(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "latin-1")

	if got := s.ftpDecodeName("\xe9"); got != "é" {
		t.Fatalf("latin-1 decode: got %q, want é", got)
	}
}

// --- Path encoding (outgoing Retr/Stor/etc. must speak the server charset) ---

func TestFTPEncodePathGBK(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "gbk")

	want := "/a/\xd6\xd0\xce\xc4/x.txt"
	if got := s.ftpEncodePath("/a/中文/x.txt"); got != want {
		t.Fatalf("GBK encode: got %q, want %q", got, want)
	}
}

func TestFTPEncodePathPassthroughUTF8(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	in := "/a/中文/x.txt"
	if got := s.ftpEncodePath(in); got != in {
		t.Fatalf("UTF-8 passthrough: got %q, want %q", got, in)
	}
}

// A name recovered via the GBK fallback must round-trip: operations on it
// go out with the exact raw bytes the server originally sent.
func TestFTPEncodePathRestoresRawBytes(t *testing.T) {
	s := NewFTPSession("ftp-test")
	setFTPCodec(s, "utf-8")

	raw := "\xd6\xd0\xce\xc4"
	dec := s.ftpDecodeName(raw)
	if dec != "中文" {
		t.Fatalf("decode prerequisite failed: got %q", dec)
	}
	want := "/d/" + raw
	if got := s.ftpEncodePath("/d/" + dec); got != want {
		t.Fatalf("raw-byte restore: got %q, want %q", got, want)
	}
}

// --- Transfer cancellation (X button has no effect) ---

type blockingReader struct{ ch chan struct{} }

func (b *blockingReader) Read([]byte) (int, error) {
	<-b.ch
	return 0, errors.New("unreachable")
}

func TestFTPProgressReaderCancel(t *testing.T) {
	s := NewFTPSession("ftp-test")
	task := &TransferTask{ID: "t1"}
	task.start()
	defer task.done()
	task.cancel() // cancel before the first read

	pr := &progressReader{r: &blockingReader{ch: make(chan struct{})}, task: task, s: s}
	errCh := make(chan error, 1)
	go func() {
		_, err := pr.Read(make([]byte, 128))
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Read after cancel: got %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read blocked after cancellation")
	}
}

// The idle deadline must be re-armed before every read so a silent server
// cannot park the data-socket read forever.
type deadlineRecorder struct {
	calls []time.Time
}

func (d *deadlineRecorder) SetDeadline(t time.Time) error {
	d.calls = append(d.calls, t)
	return nil
}

func TestFTPProgressReaderIdleDeadline(t *testing.T) {
	s := NewFTPSession("ftp-test")
	task := &TransferTask{ID: "t2"}
	task.start()
	defer task.done()

	rec := &deadlineRecorder{}
	r := &sliceReader{data: []byte("ab"), errs: []error{nil, io.EOF}}
	pr := &progressReader{r: r, task: task, s: s, deadline: rec}

	buf := make([]byte, 8)
	for {
		if _, err := pr.Read(buf); err != nil {
			break
		}
	}
	if len(rec.calls) != 2 {
		t.Fatalf("SetDeadline calls: got %d, want 2", len(rec.calls))
	}
	for _, call := range rec.calls {
		delay := time.Until(call)
		if delay < ftpDataIdleTimeout-time.Second || delay > ftpDataIdleTimeout+time.Second {
			t.Fatalf("deadline delay %v, want ~%v", delay, ftpDataIdleTimeout)
		}
	}
}

type sliceReader struct {
	data []byte
	errs []error
	i    int
}

func (r *sliceReader) Read(p []byte) (int, error) {
	if r.i >= len(r.errs) {
		return 0, io.EOF
	}
	e := r.errs[r.i]
	r.i++
	if e != nil {
		return 0, e
	}
	n := copy(p, r.data)
	return n, nil
}

// --- Shut-timeout deadline must not poison the control connection ---

// After every data operation (List/Retr/Stor) the library bounds its wait for
// the "226 closing data connection" reply with an absolute deadline of
// now+ftpShutTimeout on the control connection. If that deadline is not
// cleared after the reply is read, every command sent once it has expired
// fails instantly with i/o timeout: the session dies after ~30s of idle
// browsing, directory entry reports "no such directory" while the path bar
// still updates, and nothing recovers until reconnect.
func TestFTPControlConnSurvivesShutDeadline(t *testing.T) {
	old := ftpShutTimeout
	ftpShutTimeout = 1200 * time.Millisecond
	defer func() { ftpShutTimeout = old }()

	srv := newFakeFTPServer(t)
	s := NewFTPSession("ftp-test")
	cfg := ConnectionConfig{
		Host:          srv.host,
		Port:          srv.port,
		User:          "tester",
		Password:      "secret",
		FtpEncryption: "none",
	}
	if err := s.Connect(cfg); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if _, err := s.ListRemote(""); err != nil {
		t.Fatalf("first listing: %v", err)
	}

	// Simulate the user studying the root listing before double-clicking a
	// directory: wait until the stale shut deadline is long past.
	time.Sleep(ftpShutTimeout + 1500*time.Millisecond)

	if _, err := s.ListRemote(""); err != nil {
		t.Fatalf("listing after shut deadline expired (control connection poisoned): %v", err)
	}
	if _, err := s.ChangeRemoteDir("/etc"); err != nil {
		t.Fatalf("subdir listing after shut deadline expired: %v", err)
	}
}

// fakeFTPServer speaks just enough of the RFC 959 command set to exercise the
// session's List/ChangeRemoteDir paths, mirroring a minimal legacy server
// (AIX ftpd): FEAT and EPSV are unknown commands, so the client must fall
// back to PASV and plain LIST, and directories must be entered with CWD
// before the bare LIST.
type fakeFTPServer struct {
	ln   net.Listener
	host string
	port int
	cwd  string
}

func newFakeFTPServer(t *testing.T) *fakeFTPServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("fake ftp listen: %v", err)
	}
	s := &fakeFTPServer{
		ln:   ln,
		host: "127.0.0.1",
		port: ln.Addr().(*net.TCPAddr).Port,
	}
	go s.serve()
	t.Cleanup(func() {
		ln.Close()
	})
	return s
}

func (s *fakeFTPServer) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeFTPServer) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	w := bufio.NewWriter(conn)
	reply := func(format string, args ...any) {
		fmt.Fprintf(w, format+"\r\n", args...)
		_ = w.Flush()
	}

	reply("220 fake FTP server ready")

	var dataLn net.Listener
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		fields := strings.Fields(strings.TrimSpace(scanner.Text()))
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "USER":
			reply("331 Password required")
		case "PASS":
			reply("230 User logged in")
		case "FEAT":
			reply("500 'FEAT': command not understood.")
		case "EPSV":
			reply("500 'EPSV': command not understood.")
		case "PASV":
			ln, err := net.Listen("tcp", s.host+":0")
			if err != nil {
				reply("425 cannot open data connection")
				continue
			}
			dataLn = ln
			port := ln.Addr().(*net.TCPAddr).Port
			reply("227 Entering Passive Mode (%s,%d,%d)",
				strings.ReplaceAll(s.host, ".", ","), port/256, port%256)
		case "CWD":
			dir := strings.Join(fields[1:], " ")
			if dir == "/" || dir == "/etc" || dir == "/usr" {
				s.cwd = dir
				reply("250 directory changed to %s", dir)
			} else {
				reply("550 %s: no such directory", dir)
			}
		case "LIST":
			if dataLn == nil {
				reply("425 use PASV first")
				continue
			}
			if tl, ok := dataLn.(*net.TCPListener); ok {
				_ = tl.SetDeadline(time.Now().Add(5 * time.Second))
			}
			dataConn, err := dataLn.Accept()
			if err != nil {
				reply("425 data connection failed")
				continue
			}
			reply("150 Opening data connection for %s", s.cwd)

			// Unix ls -l lines; parseListLine needs the 10-char mode field.
			// The columns are padded device-style with several spaces, so the
			// surplus delimiters land inside the parsed names ("  etc") —
			// exactly what embedded servers emit and the session must trim.
			var listing string
			if s.cwd == "/etc" {
				listing = strings.Join([]string{
					"total 2",
					"-rw-r--r--    1 root     system          5 Nov 15 08:00   cfg1.conf",
					"-rw-r--r--    1 root     system          5 Nov 15 08:00   cfg2.conf",
				}, "\r\n") + "\r\n"
			} else {
				listing = strings.Join([]string{
					"total 4",
					"drwxr-xr-x    2 root     system       4096 Nov 15 08:00   etc",
					"drwxr-xr-x    4 root     system       4096 Nov 15 08:00   usr",
					"-rw-r--r--    1 root     system         14 Nov 15 08:00   file1.txt",
					"-rw-r--r--    1 root     system         14 Nov 15 08:00   my file2.txt",
				}, "\r\n") + "\r\n"
			}
			_, _ = dataConn.Write([]byte(listing))
			_ = dataConn.Close()
			dataLn.Close()
			dataLn = nil

			reply("226 Transfer complete")
		case "QUIT":
			reply("221 Goodbye")
			return
		default:
			// TYPE and friends succeed silently.
			reply("200 command OK")
		}
	}
}
