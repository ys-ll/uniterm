package platform

import (
	"runtime"
	"testing"
)

func TestShellNameStripsExeOnWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		if got := ShellName(`C:\Program Files\Git\bin\bash.exe`); got != "bash" {
			t.Errorf("ShellName(bash.exe) = %q, want %q", got, "bash")
		}
	}
	if got := ShellName("/usr/bin/bash"); got != "bash" {
		t.Errorf("ShellName(/usr/bin/bash) = %q, want %q", got, "bash")
	}
	if got := ShellName("zsh"); got != "zsh" {
		t.Errorf("ShellName(zsh) = %q, want %q", got, "zsh")
	}
}

func TestLoginShellArgsMacOSOnly(t *testing.T) {
	if runtime.GOOS == "darwin" {
		cases := []struct {
			shell string
			want  []string
		}{
			{"bash", []string{"-l"}},
			{"zsh", []string{"-l"}},
			{"sh", []string{"-l"}},
			{"dash", []string{"-l"}},
			{"fish", []string{"--login"}},
			{"unknown-shell", nil},
		}
		for _, tc := range cases {
			got := LoginShellArgs(tc.shell)
			if !equalSlices(got, tc.want) {
				t.Errorf("LoginShellArgs(%q) = %v, want %v", tc.shell, got, tc.want)
			}
		}
	} else {
		// Non-darwin must always return nil regardless of the shell.
		for _, shell := range []string{"bash", "zsh", "fish"} {
			if got := LoginShellArgs(shell); got != nil {
				t.Errorf("LoginShellArgs(%q) on %s = %v, want nil", shell, runtime.GOOS, got)
			}
		}
	}
}

func TestDefaultShellIsNonEmpty(t *testing.T) {
	got := DefaultShell()
	if got == "" {
		t.Fatalf("DefaultShell returned empty string")
	}
	// On Windows the resolution chain has a guaranteed cmd.exe fallback; on
	// unix SHELL is usually set in CI so we just require something concrete.
	if runtime.GOOS == "windows" {
		for _, want := range []string{"pwsh.exe", "powershell.exe", "bash.exe", "cmd.exe"} {
			if got == want {
				return
			}
		}
		t.Errorf("DefaultShell() = %q, expected one of pwsh.exe / powershell.exe / bash.exe / cmd.exe", got)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
