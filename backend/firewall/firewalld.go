package firewall

import (
	"fmt"
	"strings"
)

// --- firewalld adapter -------------------------------------------------------

// firewalldProbe checks whether firewall-cmd is installed and whether the
// firewalld service is currently active.
func firewalldProbe(r *ResolvedSSH) (installed, active bool, err error) {
	if _, err := runCommand(r, "command -v firewall-cmd"); err != nil {
		return false, false, nil // not installed
	}
	out, err := runCommand(r, "systemctl is-active firewalld")
	if err != nil {
		// service present but not running → installed, inactive
		return true, false, nil
	}
	return true, strings.TrimSpace(out) == "active", nil
}

func firewalldSetEnabled(r *ResolvedSSH, enable bool) error {
	cmd := "systemctl start firewalld"
	if !enable {
		cmd = "systemctl stop firewalld"
	}
	if _, err := runCommand(r, cmd); err != nil {
		return err
	}
	if enable {
		// firewalld only manages rules while the service is running.
		runCommand(r, "systemctl enable firewalld")
	} else {
		runCommand(r, "systemctl disable firewalld")
	}
	return nil
}

func firewalldDefaultPolicy(r *ResolvedSSH) string {
	if _, err := runCommand(r, "firewall-cmd --get-default-zone"); err != nil {
		return ""
	}
	return "default-zone"
}

// firewalldAutoStart reports whether the firewalld service is enabled to start
// on boot (systemctl is-enabled), independent of whether it is running now.
func firewalldAutoStart(r *ResolvedSSH) bool {
	out, err := runCommand(r, "systemctl is-enabled firewalld")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "enabled"
}

// firewalldSetAutoStart only toggles the boot-time enablement via systemctl
// enable/disable. It never starts/stops the daemon, so the currently active
// rules are left untouched.
func firewalldSetAutoStart(r *ResolvedSSH, enable bool) error {
	cmd := "systemctl disable firewalld"
	if enable {
		cmd = "systemctl enable firewalld"
	}
	_, err := runCommand(r, cmd)
	return err
}

// firewalldSaveRule adds/removes a permanent port rule. Plain --add-port/--remove-port
// covers the common inbound-any-source case; source-specific inbound and all
// outbound rules use rich rules (which firewalld follows for both directions
// via the "out"/"top" keywords in newer versions).
func firewalldSaveRule(r *ResolvedSSH, rule PortRule, action Action) error {
	// Validate direction: firewalld plain --add-port is inbound-only. Outbound
	// uses rich rules. All writes here are permanent-only; the runtime config is
	// not changed until firewalldReload is explicitly requested.
	useRich := rule.Direction == DirectionOut || rule.Source != ""

	var setup string
	if useRich {
		setup = firewalldRichRule(rule, action)
	} else {
		flag := "--add-port"
		if action == ActionDeny {
			flag = "--remove-port"
		}
		setup = fmt.Sprintf("firewall-cmd --permanent %s %s/%s", flag,
			shellQuote(rule.Port), string(rule.Protocol))
	}
	_, err := runCommand(r, setup)
	return err
}

func firewalldReload(r *ResolvedSSH) error {
	_, err := runCommand(r, "firewall-cmd --reload")
	return err
}

// firewalldRichRule builds a rich rule for source-specific inbound or for
// outbound allows (firewalld >= 0.9 rich rules support direction="out").
func firewalldRichRule(rule PortRule, action Action) string {
	dir := "in"
	if rule.Direction == DirectionOut {
		dir = "out"
	}
	var b strings.Builder
	b.WriteString("firewall-cmd --permanent ")
	if action == ActionDeny {
		b.WriteString("--remove-rich-rule=")
	} else {
		b.WriteString("--add-rich-rule=")
	}
	// --remove-rich-rule must exactly match the original accept rule. Deny is
	// the UI's removal action, not a request to create a reject rule.
	ruleAction := action
	if action == ActionDeny {
		ruleAction = ActionAllow
	}
	b.WriteString(shellQuote(buildRichRuleBody(rule, dir, ruleAction)))
	return b.String()
}

func buildRichRuleBody(rule PortRule, dir string, action Action) string {
	var b strings.Builder
	fmt.Fprintf(&b, `rule family="ipv4" direction="%s"`, dir)
	if rule.Source != "" {
		// For inbound the address is a source; for outbound it is a destination.
		role := "source"
		if dir == "out" {
			role = "destination"
		}
		fmt.Fprintf(&b, ` %s address="%s"`, role, rule.Source)
	}
	fmt.Fprintf(&b, ` port port="%s" protocol="%s"`, rule.Port, string(rule.Protocol))
	if action == ActionDeny {
		b.WriteString(` reject`)
	} else {
		b.WriteString(` accept`)
	}
	return b.String()
}

// firewalldListRules lists the permanent configuration, not the runtime rules.
// This is the configuration edited by SaveConfig and later activated by reload.
func firewalldListRules(r *ResolvedSSH) ([]PortRule, error) {
	var rules []PortRule

	out, err := runCommand(r, "firewall-cmd --permanent --list-ports")
	if err != nil {
		return nil, err
	}
	if out != "" {
		for _, tok := range strings.Fields(out) {
			// tok like "8080/tcp" possibly "8000-8100/udp"
			proto := ProtocolTCP
			port := tok
			if i := strings.LastIndex(tok, "/"); i >= 0 {
				port, proto = tok[:i], Protocol(tok[i+1:])
			}
			rules = append(rules, PortRule{
				Direction: DirectionIn,
				Protocol:  proto,
				Port:      port,
				Persist:   PersistPermanent,
			})
		}
	}

	rich, err := runCommand(r, "firewall-cmd --permanent --list-rich-rules")
	if err != nil {
		return nil, err
	}
	if rich != "" {
		for _, line := range strings.Split(rich, "\n") {
			if pr, ok := parseFirewalldRich(line); ok && pr.Direction == DirectionIn {
				rules = append(rules, pr)
			}
		}
	}
	return rules, nil
}

// parseFirewalldRich extracts a PortRule from a rich-rule line. Best-effort;
// unrecognised rules are skipped so we never mangle user-defined rules.
func parseFirewalldRich(line string) (PortRule, bool) {
	l := strings.ToLower(line)
	if !strings.Contains(l, "accept") || strings.Contains(l, `family="ipv6"`) {
		return PortRule{}, false
	}
	var pr PortRule
	pr.Direction = DirectionIn
	if strings.Contains(l, `direction="out"`) {
		pr.Direction = DirectionOut
	}
	// port port="8080" protocol="tcp"
	if i := strings.Index(l, `port="`); i >= 0 {
		rest := l[i+len(`port="`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			pr.Port = rest[:j]
		}
	}
	if i := strings.Index(l, `protocol="`); i >= 0 {
		rest := l[i+len(`protocol="`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			pr.Protocol = Protocol(rest[:j])
		}
	}
	// Inbound uses source address, outbound uses destination address; both map
	// to the same PortRule.Source field.
	if i := strings.Index(l, `source address="`); i >= 0 {
		rest := l[i+len(`source address="`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			pr.Source = rest[:j]
		}
	} else if i := strings.Index(l, `destination address="`); i >= 0 {
		rest := l[i+len(`destination address="`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			pr.Source = rest[:j]
		}
	}
	if pr.Port == "" {
		return PortRule{}, false
	}
	pr.Persist = PersistPermanent
	return pr, true
}
