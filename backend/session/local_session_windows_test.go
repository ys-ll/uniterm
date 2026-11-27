package session

import (
"reflect"
	"strconv"
	"testing"
)

// ExecFilterEncoding is empty for every non-MSYS shell and reports the system
// OEM code page for MSYS bash (the shell-exec wrapper's filter input).
func TestExecFilterEncoding(t *testing.T) {
	s := NewLocalSession("t")
	if got := s.ExecFilterEncoding(); got != "" {
		t.Fatalf("non-MSYS session reported %q", got)
	}
	s.mu.Lock()
	s.msysShell = true
	s.mu.Unlock()
	enc := s.ExecFilterEncoding()
	if len(enc) < 3 || enc[:2] != "cp" {
		t.Fatalf("MSYS session reported %q, want cp<oemcp>", enc)
	}
	if _, err := strconv.Atoi(enc[2:]); err != nil {
		t.Fatalf("MSYS session reported %q, want numeric code page", enc)
	}
}


// shellName must disambiguate the three common Windows bash.exe flavors by
// install path: Git for Windows, Cygwin and MSYS2 all ship a binary with the
// same basename, and a bare "bash" tab title tells the user nothing.
func TestShellNameDisambiguatesBashFlavors(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`C:\Program Files\Git\bin\bash.exe`, "Git Bash"},
		{`C:\Users\me\scoop\apps\git\current\bin\bash.exe`, "Git Bash (Scoop)"},
		{`C:\ProgramData\chocolatey\bin\bash.exe`, "Git Bash"},
		{`C:\cygwin64\bin\bash.exe`, "Cygwin bash"},
		{`D:\tools\cygwin\bin\bash.exe`, "Cygwin bash"},
		{`C:\msys64\usr\bin\bash.exe`, "MSYS2 bash"},
		{`C:\tools\msys32\usr\bin\bash.exe`, "MSYS2 bash"},
		// Unknown bash installs keep the historical basename label.
		{`E:\custom\bash.exe`, "bash"},
		// Non-bash shells keep the basename label.
		{`C:\Windows\System32\cmd.exe`, "cmd"},
		{`C:\Program Files\PowerShell\7\pwsh.exe`, "pwsh"},
		{`C:\Windows\System32\bash.exe`, "bash"},
	}
	for _, tc := range cases {
		if got := shellName(tc.path); got != tc.want {
			t.Errorf("shellName(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}


func TestBuildWSLStartArgs(t *testing.T) {
	for _, tc := range []struct {
		name      string
		distro    string
		cwd       string
		startArgs []string
		want      []string
	}{
		{
			name:      "no cwd starts in linux home",
			distro:    "Ubuntu-24.04",
			cwd:       "",
			startArgs: nil,
			want:      []string{"-d", "Ubuntu-24.04", "--cd", "~"},
		},
		{
			name:      "explicit cwd skips home",
			distro:    "Ubuntu-24.04",
			cwd:       `C:\Users\me\project`,
			startArgs: nil,
			want:      []string{"-d", "Ubuntu-24.04"},
		},
		{
			name:      "shell integration args appended after cd",
			distro:    "Ubuntu-24.04",
			cwd:       "",
			startArgs: []string{"-e", "bash", "--rcfile", "/tmp/uniterm-abc123"},
			want:      []string{"-d", "Ubuntu-24.04", "--cd", "~", "-e", "bash", "--rcfile", "/tmp/uniterm-abc123"},
		},
		{
			name:      "explicit cwd with integration",
			distro:    "Debian",
			cwd:       `D:\work`,
			startArgs: []string{"-e", "env", "ZDOTDIR=/tmp/uniterm-xyz", "zsh"},
			want:      []string{"-d", "Debian", "-e", "env", "ZDOTDIR=/tmp/uniterm-xyz", "zsh"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := buildWSLStartArgs(tc.distro, tc.cwd, tc.startArgs)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildWSLStartArgs(%q, %q, %v) = %v, want %v", tc.distro, tc.cwd, tc.startArgs, got, tc.want)
			}
		})
	}
}
