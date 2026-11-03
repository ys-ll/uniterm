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
}
