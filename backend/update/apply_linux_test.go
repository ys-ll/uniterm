//go:build linux

package update

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func fakeLookPath(available ...string) func(string) (string, error) {
	return func(name string) (string, error) {
		for _, a := range available {
			if a == name {
				return "/usr/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestPackageInstallCommand(t *testing.T) {
	pkg := "/tmp/uniterm-update-1/uniterm-linux-amd64-v1.9.3.deb"
	got, err := packageInstallCommand("deb", pkg, fakeLookPath("pkexec", "apt-get", "dpkg"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/usr/bin/pkexec", "/usr/bin/apt-get", "install", "-y", pkg}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("deb with apt: got %v want %v", got, want)
	}
	got, _ = packageInstallCommand("deb", pkg, fakeLookPath("pkexec", "dpkg"))
	if got[1] != "/usr/bin/dpkg" {
		t.Errorf("deb without apt should fall back to dpkg: %v", got)
	}
	got, _ = packageInstallCommand("rpm", "x.rpm", fakeLookPath("pkexec", "rpm"))
	if got[1] != "/usr/bin/rpm" {
		t.Errorf("rpm fallback: %v", got)
	}
	if _, err := packageInstallCommand("deb", pkg, fakeLookPath("apt-get")); err == nil {
		t.Error("missing pkexec must be an error")
	}
}

func TestListContainsAny(t *testing.T) {
	dir := t.TempDir()
	list := filepath.Join(dir, "uniterm.list")
	os.WriteFile(list, []byte("/.\n/usr\n/usr/bin\n/usr/bin/uniterm\n"), 0644)
	if !listContainsAny(list, []string{"/usr/bin/uniterm"}) {
		t.Error("exact path should match")
	}
	if !listContainsAny(list, []string{"/bin/uniterm"}) {
		t.Error("merged-usr spelling should match")
	}
	if listContainsAny(list, []string{"/home/u/bin/uniterm"}) {
		t.Error("portable path must not match")
	}
	if listContainsAny(filepath.Join(dir, "missing.list"), []string{"/usr/bin/uniterm"}) {
		t.Error("missing list must not match")
	}
}

func TestPackageStageDirReadable(t *testing.T) {
	stage, err := packageStageDir()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(stage)
	fi, err := os.Stat(stage)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0755 {
		t.Errorf("stage dir mode = %v, want 0755 so the package manager can read it", fi.Mode().Perm())
	}
}
