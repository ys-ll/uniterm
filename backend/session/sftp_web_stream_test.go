package session

import "testing"

func TestArchiveRelPath(t *testing.T) {
	cases := []struct {
		root, p, want string
	}{
		{"/root/app", "/root/app", "app"},
		{"/root/app", "/root/app/bin/run", "app/bin/run"},
		{"/root/app/", "/root/app/bin/run", "app/bin/run"},
		{"/", "/", "archive"},
		{"/", "/etc/hosts", "archive/etc/hosts"},
	}
	for _, c := range cases {
		if got := archiveRelPath(c.root, c.p); got != c.want {
			t.Errorf("archiveRelPath(%q, %q) = %q, want %q", c.root, c.p, got, c.want)
		}
	}
}

func TestSFTPSessionResolveRemotePath(t *testing.T) {
	s := &SFTPSession{cwd: "/home/user"}
	if got := s.resolveRemotePath("file.txt"); got != "/home/user/file.txt" {
		t.Errorf("relative path = %q", got)
	}
	if got := s.resolveRemotePath("/etc/hosts"); got != "/etc/hosts" {
		t.Errorf("absolute path = %q", got)
	}

	// Windows-style remote paths: drive-letter paths are absolute (path.IsAbs
	// would wrongly join them onto the cwd) and keep backslash separators.
	ws := &SFTPSession{cwd: `C:\Users\admin`}
	if got := ws.resolveRemotePath(`C:\Users\admin\file.txt`); got != `C:\Users\admin\file.txt` {
		t.Errorf("windows absolute path = %q", got)
	}
	if got := ws.resolveRemotePath(`file.txt`); got != `C:\Users\admin\file.txt` {
		t.Errorf("windows relative path = %q", got)
	}
}

func TestSFTPRemotePathHelpers(t *testing.T) {
	cases := []struct {
		name string
		p    string
		abs  bool
		win  bool
	}{
		{"posix absolute", "/etc/hosts", true, false},
		{"posix relative", "etc/hosts", false, false},
		{"drive letter", `C:\Users\admin`, true, true},
		{"drive letter forward slash", "C:/Users/admin", true, true},
		{"unc", `\\host\share\dir`, true, true},
		{"backslash rooted", `\Users`, true, false},
		{"single letter", "x", false, false},
		{"colon mid path", "etc:hosts", false, false},
	}
	for _, tc := range cases {
		if got := remoteAbs(tc.p); got != tc.abs {
			t.Errorf("%s: remoteAbs(%q) = %v, want %v", tc.name, tc.p, got, tc.abs)
		}
		if got := isWindowsStylePath(tc.p); got != tc.win {
			t.Errorf("%s: isWindowsStylePath(%q) = %v, want %v", tc.name, tc.p, got, tc.win)
		}
	}

	joinCases := []struct{ base, name, want string }{
		{"/home/user", "file.txt", "/home/user/file.txt"},
		{`C:\Users\admin`, "file.txt", `C:\Users\admin\file.txt`},
		{"C:/Users/admin", "file.txt", `C:/Users/admin\file.txt`},
	}
	for _, tc := range joinCases {
		if got := joinRemotePath(tc.base, tc.name); got != tc.want {
			t.Errorf("joinRemotePath(%q, %q) = %q, want %q", tc.base, tc.name, got, tc.want)
		}
	}

	dirCases := []struct{ p, want string }{
		{"/home/user/file.txt", "/home/user"},
		{`C:\Users\admin\file.txt`, `C:\Users\admin`},
		{`C:\file.txt`, `C:\`},
	}
	for _, tc := range dirCases {
		if got := dirRemotePath(tc.p); got != tc.want {
			t.Errorf("dirRemotePath(%q) = %q, want %q", tc.p, got, tc.want)
		}
	}
}
