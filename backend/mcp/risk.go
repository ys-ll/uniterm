package mcp

import (
	"strings"
)

// classifyCommand grades a shell command into one of three risk classes used
// by the approval policy. It is a heuristic, not a sandbox: the policy dialog
// is the real boundary; classification only decides when to bother the user.
//
// Grade rules (case-insensitive, token-based on the first word of each
// ;-/&&/||/|/newline segment, after stripping env assignments and sudo/ssh
// wrappers):
//   - dangerous: rm -rf, mkfs*, dd of=, fork bombs, shutdown/reboot/init/halt/
//     poweroff, writes into /etc /boot /sys /dev /proc/sys
//   - write: mv, rm, cp -f, truncate, chmod, chown, kill/pkill/killall, tee,
//     userdel/usermod/passwd, systemctl/service state changes, package
//     install/remove, git push/reset/clean/checkout/rebase/rm, docker/kubectl
//     mutations, file redirection (except >/dev/null), curl/wget
//   - read: everything else
func classifyCommand(cmd string) RiskClass {
	low := strings.ToLower(cmd)
	// Fork bombs are line-level patterns (":(){ :|:& };:"): pipe-splitting
	// shreds them, so check before segmentation.
	if strings.Contains(low, ":|:") || strings.Contains(low, "|:&") {
		return RiskDangerous
	}
	overall := RiskRead
	for _, seg := range splitSegments(cmd) {
		switch classifySegment(seg) {
		case RiskDangerous:
			return RiskDangerous
		case RiskWrite:
			overall = RiskWrite
		}
	}
	return overall
}

// splitSegments cuts a command line on ; && || | and newlines.
func splitSegments(cmd string) []string {
	return strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ';' || r == '\n' || r == '|'
	})
}

// classifySegment grades one pipeline segment.
func classifySegment(seg string) RiskClass {
	seg = strings.TrimSpace(seg)
	if seg == "" {
		return RiskRead
	}
	words := strings.Fields(seg)
	head := strings.ToLower(words[0])
	// Strip env assignments and sudo/nohup wrappers: "sudo rm -rf" is judged
	// as "rm -rf"; "FOO=1 rm" likewise.
	for isWrapper(head) || (strings.Contains(head, "=") && len(words) > 1) {
		words = words[1:]
		head = strings.ToLower(words[0])
	}
	joined := strings.ToLower(seg)

	// Fork bombs: ":(){ :|:& };:" and variants — a segment that pipes a
	// self-invocation into the background.
	if strings.Contains(joined, ":|:") || strings.Contains(joined, "|:&") {
		return RiskDangerous
	}

	if matchesDangerous(head, words, joined) {
		return RiskDangerous
	}

	risk := gradeWrite(head, words, joined)

	// Writes into dangerous paths escalate to RiskDangerous regardless of the
	// command form — including plain "echo … > /etc/…" whose echo alone would
	// grade read. Match both ">path" and "> path" forms. /dev/null is the
	// universal silencer and stays exempt.
	for _, p := range dangerousPaths {
		if strings.Contains(joined, ">"+p) || strings.Contains(joined, ">>"+p) ||
			strings.Contains(joined, "> "+p) || strings.Contains(joined, ">> "+p) {
			if p == "/dev/" && (strings.Contains(joined, "/dev/null")) {
				continue
			}
			return RiskDangerous
		}
		if isWriteCommand(head) && strings.Contains(joined, " "+p) {
			return RiskDangerous
		}
	}
	return risk
}

// isWriteCommand reports whether head is a command whose target argument is
// a file being written.
func isWriteCommand(head string) bool {
	switch head {
	case "cp", "mv", "tee", "install", "rsync", "dd", "truncate":
		return true
	}
	return false
}

var dangerHeads = []string{
	"mkfs", "shutdown", "reboot", "halt", "poweroff", "init", "fdisk",
	"parted", "wipefs", "shred",
}

var dangerousPaths = []string{"/etc/", "/boot/", "/sys/", "/dev/", "/proc/sys/"}

// matchesDangerous checks unconditional danger patterns.
func matchesDangerous(head string, words []string, joined string) bool {
	if head == "rm" && (containsAnyFlag(words, "-rf", "-fr") || strings.Contains(joined, "rm -rf")) {
		return true
	}
	if strings.HasPrefix(head, "mkfs") {
		return true
	}
	if head == "dd" && strings.Contains(joined, "of=/dev/") {
		return true
	}
	for _, h := range dangerHeads {
		if head == h {
			return true
		}
	}
	return false
}

// gradeWrite grades write-level patterns.
func gradeWrite(head string, words []string, joined string) RiskClass {
	switch head {
	case "mv", "truncate", "chmod", "chown", "kill", "pkill", "killall",
		"tee", "userdel", "usermod", "passwd", "visudo", "rm":
		return RiskWrite
	case "cp":
		if containsAnyFlag(words, "-f", "--force") {
			return RiskWrite
		}
	}
	if head == "systemctl" || head == "service" {
		for _, sub := range []string{"restart", "stop", "disable", "mask", "kill", "start", "enable"} {
			if containsWord(words, sub) {
				return RiskWrite
			}
		}
	}
	if inList(packageManagers, head) {
		for _, sub := range []string{"install", "remove", "uninstall", "purge", "upgrade", "add"} {
			if containsWord(words, sub) {
				return RiskWrite
			}
		}
	}
	if head == "git" {
		for _, sub := range []string{"push", "reset", "clean", "checkout", "rebase", "rm"} {
			if containsWord(words, sub) {
				return RiskWrite
			}
		}
	}
	if head == "docker" || head == "podman" || head == "kubectl" {
		for _, sub := range []string{"rm", "rmi", "prune", "delete", "stop", "kill"} {
			if containsWord(words, sub) {
				return RiskWrite
			}
		}
	}
	if isDownloader(head) {
		return RiskWrite
	}
	// Redirections write to a file — except the universal /dev/null silencer
	// (with or without spaces around >).
	if strings.Contains(joined, ">") {
		redir := joined
		redir = strings.ReplaceAll(redir, " ", "")
		if !strings.Contains(redir, ">/dev/null") && !strings.Contains(redir, "2>/dev/null") && !strings.Contains(redir, "&>/dev/null") {
			return RiskWrite
		}
	}
	return RiskRead
}

// classifyDownloadPipe detects "curl/wget … | sh" across the full command
// line and upgrades the whole line to dangerous.
func classifyDownloadPipe(cmd string) bool {
	low := strings.ToLower(cmd)
	if !strings.Contains(low, "|") {
		return false
	}
	for _, left := range []string{"curl", "wget", "fetch"} {
		if !strings.Contains(low, left) {
			continue
		}
		for _, right := range []string{"| sh", "|sh", "| bash", "|bash", "| zsh", "|zsh", "| sudo sh", "| sudo bash"} {
			if strings.Contains(low, right) {
				return true
			}
		}
	}
	return false
}

var packageManagers = []string{
	"apt", "apt-get", "yum", "dnf", "pacman", "zypper", "brew", "pip",
	"pip3", "npm", "yarn", "pnpm", "gem", "cargo",
}

func isWrapper(head string) bool {
	switch head {
	case "sudo", "doas", "nohup", "time", "nice", "env":
		return true
	}
	return false
}

func isDownloader(head string) bool {
	return head == "curl" || head == "wget" || head == "fetch"
}

// containsAnyFlag reports whether any word equals one of the flags (exact
// token match, so "-rf" matches but "-r -f" does not — the latter is also
// dangerous but rare; substring matching below catches combined flags).
func containsAnyFlag(words []string, flags ...string) bool {
	for _, w := range words {
		lw := strings.ToLower(w)
		for _, f := range flags {
			if lw == f || strings.Contains(lw, f) {
				return true
			}
		}
	}
	return false
}

func containsWord(words []string, w string) bool {
	for _, word := range words {
		if strings.ToLower(word) == w {
			return true
		}
	}
	return false
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
