package store

import "regexp"

// RiskLevel is the server-side override for the AI model's self-reported risk.
// "dangerous" is the strictest: even if the model claims "read", the command
// is treated as dangerous if a regex matches. Used to defend against prompt
// injection and compromised-model scenarios where the model downgrades risk
// to bypass user confirmation.
type RiskLevel string

const (
	RiskRead      RiskLevel = "read"
	RiskWrite     RiskLevel = "write"
	RiskDangerous RiskLevel = "dangerous"
)

// MaxRisk returns the stricter of two risk levels. dangerous > write > read.
func MaxRisk(a, b RiskLevel) RiskLevel {
	order := map[RiskLevel]int{RiskRead: 0, RiskWrite: 1, RiskDangerous: 2}
	if order[a] >= order[b] {
		return a
	}
	return b
}

// dangerousPatterns are command shapes that should always require user
// confirmation regardless of what the model claims. The list is intentionally
// narrow — false negatives are unacceptable, false positives (a "read"
// command misclassified as dangerous) just trigger an extra confirm, which is
// the safe direction.
//
// Each pattern is anchored to command-position (start of string or after a
// shell operator: `;`, `&&`, `||`, `|`, backtick, `$(`), with optional
// whitespace between the operator and the next command, so a quoted
// occurrence inside a `grep` argument does not trigger.
//
// Go's RE2 engine does not retry character-class matching from a later start
// position when the first match fails the trailing anchor. So when a pattern
// includes a fixed prefix (e.g. `chmod\s+`), the regex engine anchors at the
// position right after the prefix and cannot slide forward. This matters for
// the chmod pattern: "chmod 0777" must match as if it were "chmod 777". The
// fix is to allow an optional leading 0 in the octal-digit group.
var dangerousPatterns = []*regexp.Regexp{
	// rm with -r and -f in either order; covers -rf, -Rf, -fr, -frv.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*rm\s+-\S*([rR][fF]|[fF][rR])\S*\b`),
	// rm --force standalone.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*rm\s+--force\b`),
	// Redirection to absolute path.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(?:>|>>)\s*/`),
	// Disk / partition operations.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*dd\s+if=`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*mkfs(\.[a-z0-9]+)?\b`),
	// System shutdown / reboot.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(shutdown|reboot|halt|poweroff)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*init\s+[06]\b`),
	// chmod world-writable: 3 octal digits, last is 6 or 7. Leading 0 optional
	// so both "0777" and "777" match. "chmod 755" / "chmod 644" must NOT match.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*chmod\s+0?[0-7][0-7][67]\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*chown\s+-R\b`),
	// Remote download piped into a shell.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*curl\b[^` + "`" + `]*\|\s*(ba)?sh\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*wget\b[^` + "`" + `]*\|\s*(ba)?sh\b`),
	// Git force push.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*git\s+push\b[^|]*--force(-with-lease)?\b`),
	// Bulk destructive container / k8s operations.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*docker\s+system\s+prune\s+-a\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*kubectl\s+delete\b.*--all\b`),
	// crontab replacement / at scheduling.
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(crontab\s+-[ri]|at\s+now)\b`),
}

// writePatterns are commands that mutate but are not catastrophic. They force
// the risk to at least "write" so that "confirm_write" / "confirm_all" modes
// will trigger, even if the model claims "read".
//
// Same command-position anchoring as dangerousPatterns.
var writePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(cp|mv|touch|mkdir|rmdir|chmod)\s+`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(?:>|>>|tee)\s+`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*echo\s+[^|]*>`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*sed\s+-i\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*apt(-get)?\s+(install|remove|purge|upgrade)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(yum|dnf|pacman|brew)\s+(install|remove)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*npm\s+(install|i|add|rm|uninstall)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*pip\s+install\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*git\s+(commit|push|reset\s+--hard|checkout\s+--)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*(systemctl|service)\s+(start|stop|restart|enable|disable)\b`),
	regexp.MustCompile(`(?i)(?:^|[;&|` + "`" + `]|\$\()\s*chown\s+`),
}

// ClassifyCommandRisk inspects the command string and returns the minimum
// (strictest) risk level. Empty input returns "read" — there is nothing to
// classify. The function is pure: same input → same output, no I/O, no panic.
func ClassifyCommandRisk(command string) RiskLevel {
	if command == "" {
		return RiskRead
	}
	for _, re := range dangerousPatterns {
		if re.MatchString(command) {
			return RiskDangerous
		}
	}
	for _, re := range writePatterns {
		if re.MatchString(command) {
			return RiskWrite
		}
	}
	return RiskRead
}