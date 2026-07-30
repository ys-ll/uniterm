package platform

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultShell returns the user's preferred interactive shell for this OS.
// Resolution order:
//   - macOS / Linux: $SHELL → LookPath("bash") → "sh"
//   - Windows:       PowerShell 7 → Windows PowerShell → Git Bash → cmd.exe
//
// Callers can override by passing a non-empty path explicitly; this helper is
// only used when the caller has no configured preference.
func DefaultShell() string {
	return defaultShellOS()
}

// ShellName returns the basename of the shell executable with the .exe suffix
// stripped on Windows. Used to dispatch per-shell login-arg behaviour without
// hard-coding paths to /bin/bash etc.
func ShellName(path string) string {
	base := filepath.Base(path)
	if runtime.GOOS == "windows" {
		base = strings.TrimSuffix(base, ".exe")
	}
	return base
}

// LoginShellArgs returns the arguments needed to start the shell as a login
// shell on macOS.
//
// On macOS, GUI-launched apps inherit a minimal PATH; a plain interactive
// shell won't restore /usr/local/bin / /opt/homebrew/bin from /etc/paths.d.
// Terminal.app and iTerm work around this by launching login shells. Linux
// terminal emulators conventionally launch non-login interactive shells
// and the GUI session already exports a full PATH, so we leave that path
// alone there.
//
// bash/zsh/sh/dash/ksh/csh/tcsh accept -l; fish uses --login. Unknown shells
// get no argument to avoid passing a flag they might reject.
func LoginShellArgs(shellPath string) []string {
	if runtime.GOOS != "darwin" {
		return nil
	}
	switch ShellName(shellPath) {
	case "bash", "zsh", "sh", "dash", "ksh", "mksh", "csh", "tcsh":
		return []string{"-l"}
	case "fish":
		return []string{"--login"}
	default:
		return nil
	}
}

// hasExecutable is a thin wrapper over exec.LookPath so tests can stub it
// without pulling in a dependency-injection framework.
var hasExecutable = exec.LookPath
