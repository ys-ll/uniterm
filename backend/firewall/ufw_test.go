package firewall

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseUfwAddedLine guards the fix where `ufwListRules` parsed `ufw show
// added` as if it were the `ufw status` table. `ufw show added` actually emits
// command-form lines ("ufw allow 22/tcp"), which the old ALLOW/IN/Anywhere
// regex could never match → the saved rules never appeared in the list.
func TestParseUfwAddedLine(t *testing.T) {
	cases := []struct {
		line string
		want *PortRule
	}{
		{"ufw allow 22/tcp", &PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "22", Source: "", Persist: PersistPermanent}},
		{"ufw allow 22/tcp (v6)", &PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "22", Source: "", Persist: PersistPermanent}},
		{"ufw allow 8081", &PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "8081", Source: "", Persist: PersistPermanent}},
		{"ufw allow 49152:65535/tcp", &PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "49152:65535", Source: "", Persist: PersistPermanent}},
		{"ufw allow 53/udp", &PortRule{Direction: DirectionIn, Protocol: ProtocolUDP, Port: "53", Source: "", Persist: PersistPermanent}},
		{"ufw allow from 192.168.0.5 to any port 8080 proto tcp",
			&PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "8080", Source: "192.168.0.5", Persist: PersistPermanent}},
		{"ufw allow in on eth0 to 192.168.0.1 proto gre", nil}, // dest is not "any" → unmanageable
		{"ufw allow out on eth0 to any port 443 proto tcp", nil}, // outbound → skipped
		{"ufw allow OpenSSH", nil},                               // app profile, no numeric port
		{"ufw deny 21/tcp", nil},                                 // deny not surfaced
		{"ufw allow from any to any port 22/tcp", &PortRule{Direction: DirectionIn, Protocol: ProtocolTCP, Port: "22", Source: "", Persist: PersistPermanent}},
	}
	for _, c := range cases {
		got, ok := parseUfwAddedLine(c.line)
		if c.want == nil {
			if ok {
				t.Errorf("parseUfwAddedLine(%q) unexpectedly parsed: %+v", c.line, got)
			}
			continue
		}
		if !ok {
			t.Errorf("parseUfwAddedLine(%q) returned not-ok, want %+v", c.line, *c.want)
			continue
		}
		if !reflect.DeepEqual(got, *c.want) {
			t.Errorf("parseUfwAddedLine(%q) = %+v, want %+v", c.line, got, *c.want)
		}
	}
}

// TestUfwShowAddedAggregation mirrors the dedup behaviour of ufwListRules over
// a full `ufw show added` transcript: the v4+v6 twin collapses to one rule and
// the SSH self-lock check passes because 22/tcp is open from anywhere.
func TestUfwShowAddedAggregation(t *testing.T) {
	output := strings.Join([]string{
		"Added user rules (see 'ufw status' for running firewall):",
		"ufw allow 22/tcp",
		"ufw allow 22/tcp (v6)",
		"ufw allow 8081",
		"ufw allow from 192.168.0.5 to any port 8443 proto tcp",
		"ufw allow out on eth0 to any port 443 proto tcp",
		"ufw deny 21/tcp",
		"",
	}, "\n")

	var rules []PortRule
	seen := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		rule, ok := parseUfwAddedLine(line)
		if !ok {
			continue
		}
		key := ruleKey(rule)
		if seen[key] {
			continue
		}
		seen[key] = true
		rules = append(rules, rule)
	}

	// 22/tcp (deduped v4+v6), 8081 (unrestricted), 192.168.0.5→8443 (restricted).
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d: %+v", len(rules), rules)
	}
	if !sshPortAllowed(rules, 22) {
		t.Errorf("port 22 should allow SSH (unrestricted source)")
	}
	if sshPortAllowed(rules, 8443) {
		t.Errorf("port 8443 from 192.168.0.5 should NOT satisfy the source-unrestricted SSH check")
	}
}

func TestPortRangeContains(t *testing.T) {
	cases := []struct {
		value  string
		port   int
		expect bool
	}{
		{"22", 22, true}, {"22", 23, false}, {"8000-8010", 8005, true},
		{"8000-8010", 8011, false}, {"abc", 80, false}, {"0", 0, false},
	}
	for _, c := range cases {
		if got := portRangeContains(c.value, c.port); got != c.expect {
			t.Errorf("portRangeContains(%q, %d) = %v, want %v", c.value, c.port, got, c.expect)
		}
	}
}
