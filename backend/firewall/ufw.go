package firewall

import (
	"strings"
)

// --- ufw adapter -------------------------------------------------------------

// ufwProbe checks whether ufw is installed and active ("Status: active").
func ufwProbe(r *ResolvedSSH) (installed, active bool, err error) {
	out, err := runCommand(r, "command -v ufw && ufw status")
	if err != nil {
		// 检查命令是否真的装着（ufw 可能存在但切换失败导致 stderr）。
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "No such file") {
			return false, false, nil
		}
		// ufw status 需要 root；装了但没权限也视为 installed（active 未知）。
		out2, e2 := runCommand(r, "command -v ufw")
		if e2 != nil {
			return false, false, nil
		}
		_ = out2
		return true, false, nil
	}
	return true, strings.Contains(strings.ToLower(out), "status: active"), nil
}

func ufwSetEnabled(r *ResolvedSSH, enable bool) error {
	// ufw >=0.36 prompts "Command may disrupt existing ssh connections. Proceed
	// with operation (y|n)?" when enabling with an active SSH session. runCommand
	// opens the SSH session with no TTY/stdin, so that prompt would hang/abort
	// and leave the firewall off even though the call "succeeds". Piping "y"
	// into it (via the shell) answers the prompt on every version: old ufw has
	// no prompt and ignores the input, new ufw reads it and confirms. --force is
	// ufw>=0.36-only and would error on older releases, so the pipe is the
	// portable choice.
	cmd := "echo y | ufw enable"
	if !enable {
		cmd = "ufw disable"
	}
	_, err := runCommand(r, cmd)
	return err
}

func ufwDefaultPolicy(r *ResolvedSSH) string {
	if _, err := runCommand(r, "ufw status"); err != nil {
		return ""
	}
	return "default"
}

// ufwAutoStart reports whether the ufw service is enabled to start on boot.
// systemctl is-enabled prints "enabled"/"disabled" and exits non-zero for the
// disabled/unknown cases, so the boolean comes from parsing the output rather
// than the exit code. A firewall that is running right now can still be
// disabled at boot (systemctl disable only affects the next reboot).
func ufwAutoStart(r *ResolvedSSH) bool {
	out, err := runCommand(r, "systemctl is-enabled ufw")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "enabled"
}

// ufwSetAutoStart only toggles whether ufw starts at boot via systemctl
// enable/disable. It deliberately does NOT touch the currently active rules or
// ufw's own "enable/disable" flag, which would also alter the runtime kernel
// rules. systemctl enable/disable is idempotent and safe to call repeatedly.
func ufwSetAutoStart(r *ResolvedSSH, enable bool) error {
	cmd := "systemctl disable ufw"
	if enable {
		cmd = "systemctl enable ufw"
	}
	_, err := runCommand(r, cmd)
	return err
}

// ufwSaveRule adds/removes a port rule.
//   - ActionAllow (开放): runs `ufw allow ...` to permit the traffic.
//   - ActionDeny (关闭): runs `ufw delete allow ...` to remove the existing
//     allow rule, returning the port to the default (blocked) policy.
//
// ufw rules persist by default, so there is no separate "permanent" flag.
// Only inbound rules are managed (outbound was removed), so we use the plain
// `ufw allow port/proto` form which defaults to IN.
func ufwSaveRule(r *ResolvedSSH, rule PortRule, action Action) error {
	var cmd string
	if action == ActionDeny {
		cmd = "ufw delete allow"
	} else {
		cmd = "ufw allow"
	}

	if rule.Source != "" {
		cmd += " from " + shellQuote(rule.Source)
		cmd += " to any port " + shellQuote(rule.Port)
		cmd += " proto " + string(rule.Protocol)
	} else {
		cmd += " " + shellQuote(rule.Port) + "/" + string(rule.Protocol)
	}
	_, err := runCommand(r, cmd)
	return err
}

// ufwListRules parses the persisted rules from `ufw show added` and works
// whether or not ufw is currently active, mirroring firewalld's `--permanent`
// reading. The `show added` output is in command form — one line per rule, e.g.:
//
//	ufw allow 22/tcp
//	ufw allow 22/tcp (v6)
//	ufw allow 8081
//	ufw allow 49152:65535/tcp
//	ufw allow from 10.0.0.0/8 to any port 8080 proto tcp
//	ufw allow in on eth0 to 192.168.0.1 proto gre
//	ufw allow out on eth0 to any port 443 proto tcp
//
// (NOT the `ALLOW IN Anywhere` table of `ufw status`.) Only inbound allow
// rules that carry a port are returned; outbound and port-less rules are
// skipped because this tool only manages inbound allow ports.
func ufwListRules(r *ResolvedSSH) ([]PortRule, error) {
	out, err := runCommand(r, "ufw show added")
	if err != nil {
		return nil, err
	}
	var rules []PortRule
	seen := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		rule, ok := parseUfwAddedLine(line)
		if !ok {
			continue
		}
		// "ufw allow 22/tcp" and "ufw allow 22/tcp (v6)" both describe the same
		// logical rule; keep only the first so the list stays free of dups.
		key := ruleKey(rule)
		if seen[key] {
			continue
		}
		seen[key] = true
		rules = append(rules, rule)
	}
	return rules, nil
}

// parseUfwAddedLine parses a single `ufw show added` command-form line into a
// PortRule. It returns ok=false for lines that are not inbound allow-port
// rules (header, empty, deny, outbound, or rules without a port).
func parseUfwAddedLine(line string) (PortRule, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return PortRule{}, false
	}
	// First token must be "ufw"; then an action (allow/deny/reject/limit).
	if !strings.EqualFold(fields[0], "ufw") || len(fields) < 2 {
		return PortRule{}, false
	}
	action := strings.ToLower(fields[1])
	switch action {
	case "allow", "deny", "reject", "limit":
	default:
		return PortRule{}, false
	}
	// Only the "allow" rules are surfaced: deny/reject represent blocked ports
	// which the allow-list model restores by deleting the allow rule.
	if action != "allow" {
		return PortRule{}, false
	}

	dir := DirectionIn
	proto := ""
	source := ""
	port := ""
	onIface := false

	i := 2
	for ; i < len(fields); i++ {
		tok := strings.ToLower(fields[i])
		switch tok {
		case "in":
			dir = DirectionIn
		case "out":
			dir = DirectionOut
		case "on":
			onIface = true // consumes the next token (the interface)
			i++
		case "proto":
			if i+1 < len(fields) {
				proto = strings.ToLower(fields[i+1])
				i++
			}
		case "from":
			if i+1 < len(fields) {
				source = fields[i+1]
				i++
			}
		case "to":
			if i+1 < len(fields) && !strings.EqualFold(fields[i+1], "any") {
				// "to any" means all destinations; a literal dest IP is not
				// modelled, so treat the rule as unmanageable.
				return PortRule{}, false
			}
			i++
		case "port":
			if i+1 < len(fields) {
				port = fields[i+1]
				i++
			}
		default:
			// The simple form: `ufw allow 22/tcp` (and optionally `(v6)`).
			if port == "" && looksLikePortOrApp(fields[i]) && !onIface {
				port = fields[i]
			}
		}
	}

	// Outbound egress rules are not managed by this tool.
	if dir == DirectionOut {
		return PortRule{}, false
	}
	// We only model rules that open a specific port/range.
	if port == "" {
		return PortRule{}, false
	}

	// A bare port token (no '/proto') and a `ufw allow <port> (v6)` line carry
	// no explicit protocol → ufw defaults to tcp.
	if proto == "" {
		proto = "tcp"
	}
	// Split "22/tcp" style into port + protocol.
	if j := strings.LastIndex(port, "/"); j >= 0 {
		proto = port[j+1:]
		port = port[:j]
	}

	// "from any / from anywhere" means all sources → empty (unrestricted).
	src := strings.TrimSpace(source)
	switch strings.ToLower(src) {
	case "", "any", "anywhere":
		src = ""
	}

	return PortRule{
		Direction: dir,
		Protocol:  Protocol(strings.ToLower(proto)),
		Port:      strings.TrimSpace(port),
		Source:    src,
		Persist:   PersistPermanent, // ufw rules persist by default
	}, true
}

// looksLikePortOrApp reports whether a bare token in the simple rule form is a
// port number, a "start:end" range, or a service name. Port tokens may carry a
// "/protocol" suffix and "(v6)" qualifier is dropped before this is reached.
func looksLikePortOrApp(tok string) bool {
	base := tok
	if j := strings.IndexByte(base, '/'); j >= 0 {
		base = base[:j]
	}
	// Range "1000:2000" or comma list "80,443" or plain number.
	for _, part := range strings.Split(base, ",") {
		if strings.Contains(part, ":") {
			segs := strings.SplitN(part, ":", 2)
			for _, s := range segs {
				if !isAllDigits(s) {
					return false
				}
			}
		} else if !isAllDigits(part) {
			return false
		}
	}
	return true
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
