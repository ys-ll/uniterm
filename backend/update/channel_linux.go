//go:build linux

package update

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
)

// dpkgListPath is the dpkg file list for the uniterm package
// (the release workflow packages it with nfpm as "name: uniterm").
const dpkgListPath = "/var/lib/dpkg/info/uniterm.list"

// detectChannel reports ChannelDeb/ChannelRPM when the running executable was
// installed by the system package manager (/usr/bin/uniterm is root-owned, so
// replacing it in place fails), and ChannelPortable for the tar.gz binary.
func detectChannel() Channel {
	exe, err := os.Executable()
	if err != nil {
		return ChannelPortable
	}
	paths := []string{exe}
	if real, err := filepath.EvalSymlinks(exe); err == nil && real != exe {
		paths = append(paths, real)
	}
	if listContainsAny(dpkgListPath, paths) {
		return ChannelDeb
	}
	if rpmOwns(paths) {
		return ChannelRPM
	}
	return ChannelPortable
}

// listContainsAny reports whether the newline-separated file list at listPath
// contains one of paths. On merged-/usr systems the list may record either
// /usr/bin or /bin, so the usr-less spelling is checked too.
func listContainsAny(listPath string, paths []string) bool {
	f, err := os.Open(listPath)
	if err != nil {
		return false
	}
	defer f.Close()
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
		if rest, ok := cutPrefix(p, "/usr"); ok {
			want[rest] = true
		} else {
			want["/usr"+p] = true
		}
	}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if want[sc.Text()] {
			return true
		}
	}
	return false
}

func cutPrefix(s, prefix string) (string, bool) {
	if len(s) > len(prefix) && s[:len(prefix)] == prefix && s[len(prefix)] == '/' {
		return s[len(prefix):], true
	}
	return s, false
}

// rpmOwns reports whether rpm's database lists one of paths as belonging to
// an installed package.
func rpmOwns(paths []string) bool {
	rpm, err := exec.LookPath("rpm")
	if err != nil {
		return false
	}
	for _, p := range paths {
		if exec.Command(rpm, "-qf", p).Run() == nil {
			return true
		}
	}
	return false
}
