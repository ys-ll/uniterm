package session

import (
	"bufio"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// --- Shut-timeout deadline must not poison the control connection (#1489) ---

// After every data operation (List/Retr/Stor) the library bounds its wait for
// the "226 closing data connection" reply with an absolute deadline of
// now+ftpShutTimeout on the control connection. If that deadline is not
// cleared after the reply is read, every command sent once it has expired
// fails instantly with i/o timeout: the session dies after ~30s of idle
// browsing, directory entry reports "no such directory" while the path bar
// still updates, and nothing recovers until reconnect — exactly issue #1489.
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
// back to PASV and plain LIST.
type fakeFTPServer struct {
	ln   net.Listener
	host string
	port int
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
			path := strings.Join(fields[1:], " ")
			reply("150 Opening data connection for %s", path)

			// Unix ls -l lines; parseListLine needs the 10-char mode field.
			var listing string
			if strings.HasSuffix(path, "/etc") {
				listing = strings.Join([]string{
					"total 2",
					"-rw-r--r-- 1 root system 5 Nov 15 08:00 cfg1.conf",
					"-rw-r--r-- 1 root system 5 Nov 15 08:00 cfg2.conf",
				}, "\r\n") + "\r\n"
			} else {
				listing = strings.Join([]string{
					"total 4",
					"drwxr-xr-x 2 root system 4096 Nov 15 08:00 etc",
					"drwxr-xr-x 4 root system 4096 Nov 15 08:00 usr",
					"-rw-r--r-- 1 root system 14 Nov 15 08:00 file1.txt",
					"-rw-r--r-- 1 root system 14 Nov 15 08:00 file2.txt",
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
