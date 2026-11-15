//go:build linux

package update

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ys-ll/uniterm/backend/log"
)

// applyPlatform installs the staged update. Portable (tar.gz) installs replace
// the binary in place. deb/rpm installs live in root-owned /usr/bin, so the
// new package is installed through the system package manager under pkexec.
func (m *Manager) applyPlatform(pend *PendingUpdate, onProgress func(Progress)) error {
	if isSystemPackage(pend.Kind) {
		return installSystemPackage(pend, onProgress)
	}
	return applyBinary(pend.NewBinary, onProgress)
}

// packageInstallCommand returns the privileged command line that installs pkg
// for the given kind, preferring the distro's dependency-resolving front end.
// lookPath is exec.LookPath outside tests.
func packageInstallCommand(kind, pkg string, lookPath func(string) (string, error)) ([]string, error) {
	pkexec, err := lookPath("pkexec")
	if err != nil {
		return nil, errors.New("pkexec not found")
	}
	type candidate struct {
		tool string
		args []string
	}
	var cands []candidate
	switch kind {
	case "deb":
		cands = []candidate{
			{"apt-get", []string{"install", "-y", pkg}},
			{"dpkg", []string{"-i", pkg}},
		}
	case "rpm":
		cands = []candidate{
			{"dnf", []string{"install", "-y", pkg}},
			{"zypper", []string{"--non-interactive", "install", "--allow-unsigned-rpm", pkg}},
			{"rpm", []string{"-U", "--replacepkgs", pkg}},
		}
	default:
		return nil, fmt.Errorf("unsupported package kind %q", kind)
	}
	for _, c := range cands {
		if p, err := lookPath(c.tool); err == nil {
			return append([]string{pkexec, p}, c.args...), nil
		}
	}
	return nil, fmt.Errorf("no package manager found for .%s", kind)
}

// manualInstallHint is the command shown when the privileged install cannot
// run, so the user can finish the update from a terminal.
func manualInstallHint(kind, pkg string) string {
	if kind == "rpm" {
		return "sudo rpm -U " + pkg
	}
	return "sudo apt install " + pkg
}

func installSystemPackage(pend *PendingUpdate, onProgress func(Progress)) error {
	if onProgress != nil {
		onProgress(Progress{Phase: "applying", Total: -1})
	}
	argv, err := packageInstallCommand(pend.Kind, pend.FilePath, exec.LookPath)
	if err != nil {
		return fmt.Errorf("%v; install the update manually with: %s", err, manualInstallHint(pend.Kind, pend.FilePath))
	}
	log.Writef("[update] installing package: %s", strings.Join(argv, " "))
	out, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
	if err != nil {
		log.Writef("[update] package install failed: %v\n%s", err, out)
		var exitErr *exec.ExitError
		// pkexec exits 126 when the authentication dialog is dismissed or
		// authorization is denied, 127 when no polkit agent is available.
		if errors.As(err, &exitErr) && (exitErr.ExitCode() == 126 || exitErr.ExitCode() == 127) {
			return fmt.Errorf("administrator authorization was not granted; install the update manually with: %s", manualInstallHint(pend.Kind, pend.FilePath))
		}
		return fmt.Errorf("package install failed: %v: %s", err, lastLines(string(out), 5))
	}
	log.Writef("[update] package installed: %s", pend.FilePath)
	_ = os.RemoveAll(pend.StageDir)
	return nil
}

// lastLines keeps the tail of command output for an error message.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
