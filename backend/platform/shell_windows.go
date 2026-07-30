//go:build windows

package platform

import (
	"os"
	"path/filepath"
)

// defaultShellOS picks PowerShell 7, then Windows PowerShell, then Git Bash,
// then bash.exe on PATH, finally falling back to cmd.exe. Git Bash is
// preferred over WSL bash because it doesn't require the WSL subsystem or
// introduce a virtualised relay hop.
func defaultShellOS() string {
	if _, err := hasExecutable("pwsh.exe"); err == nil {
		return "pwsh.exe"
	}
	if _, err := hasExecutable("powershell.exe"); err == nil {
		return "powershell.exe"
	}
	gitBashPaths := []string{
		`C:\Program Files\Git\bin\bash.exe`,
		`C:\Program Files (x86)\Git\bin\bash.exe`,
		filepath.Join(os.Getenv("ProgramFiles"), "Git", "bin", "bash.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Git", "bin", "bash.exe"),
	}
	for _, p := range gitBashPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if _, err := hasExecutable("bash.exe"); err == nil {
		return "bash.exe"
	}
	return "cmd.exe"
}
