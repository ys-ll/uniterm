package packetcap

import (
	"strconv"
	"strings"
)

// Filter is a simple packet filter. The user may enter tokens (whitespace or
// comma separated); a packet is kept when every token matches, where each
// token is one of:
//
//	proto|tcp|udp        protocol  (case-insensitive)
//	port <n>             either source or destination port equals n
//	srcport|sport <n>    source port equals n
//	dstport|dport <n>    destination port equals n
//	host <ip>            either source or destination IP equals ip
//	src <ip>             source IP equals ip
//	dst <ip>             destination IP equals ip
//
// Naming matches tcpdump's compact form so users familiar with it feel at home.
// An empty filter keeps everything; a malformed token is ignored.
type Filter struct {
	protos   map[string]bool // lower-cased protocol names
	anyPort  int             // 0 = disabled
	srcPort  int
	dstPort  int
	anyHosts map[string]bool
	srcHosts map[string]bool
	dstHosts map[string]bool
}

// ParseFilter builds a Filter from a raw string. Never fails; bad tokens are
// dropped and the rest apply.
func ParseFilter(raw string) *Filter {
	f := &Filter{
		protos:   map[string]bool{},
		anyHosts: map[string]bool{},
		srcHosts: map[string]bool{},
		dstHosts: map[string]bool{},
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ' ' || r == ',' || r == ';'
	})
	for i := 0; i < len(fields); i++ {
		tok := strings.ToLower(fields[i])
		switch tok {
		case "tcp", "udp", "icmp", "arp", "ipv4", "ipv6":
			f.protos[tok] = true
		case "port":
			if n, ok := atoiSafe(next(fields, &i)); ok {
				f.anyPort = n
			}
		case "srcport", "sport":
			if n, ok := atoiSafe(next(fields, &i)); ok {
				f.srcPort = n
			}
		case "dstport", "dport":
			if n, ok := atoiSafe(next(fields, &i)); ok {
				f.dstPort = n
			}
		case "host":
			if v := next(fields, &i); v != "" {
				f.anyHosts[strings.ToLower(v)] = true
			}
		case "src":
			if v := next(fields, &i); v != "" {
				f.srcHosts[strings.ToLower(v)] = true
			}
		case "dst":
			if v := next(fields, &i); v != "" {
				f.dstHosts[strings.ToLower(v)] = true
			}
		default:
			// Bare value: treat as a host (most common shorthand) or protocol.
			if isHostLike(tok) {
				f.anyHosts[tok] = true
			}
		}
	}
	return f
}

func next(fields []string, i *int) string {
	if *i+1 < len(fields) {
		*i++
		return fields[*i]
	}
	return ""
}

func atoiSafe(s string) (int, bool) {
	n, err := strconv.Atoi(s)
	return n, err == nil
}

func isHostLike(s string) bool {
	// Accept dotted-quad IPs or hostname-ish tokens without spaces/colons.
	if strings.Contains(s, ".") {
		return true
	}
	return false
}

// Active reports whether a packet passes every active filter.
func (f *Filter) Active(p *Packet) bool {
	if p == nil {
		return false
	}
	if len(f.protos) > 0 {
		proto := strings.ToLower(p.Protocol)
		if proto == "p6" || proto == "p17" {
			proto = "" // unknown numeric protocol won't match named filters
		}
		// Match both the protocol name and the network layer.
		nw := strings.ToLower(p.Network)
		if !(f.protos[proto] || f.protos[nw]) {
			return false
		}
	}
	if f.anyPort > 0 && int(p.SourcePort) != f.anyPort && int(p.DestPort) != f.anyPort {
		return false
	}
	if f.srcPort > 0 && int(p.SourcePort) != f.srcPort {
		return false
	}
	if f.dstPort > 0 && int(p.DestPort) != f.dstPort {
		return false
	}
	if len(f.anyHosts) > 0 {
		src := strings.ToLower(p.SourceIP)
		dst := strings.ToLower(p.DestIP)
		if !(f.anyHosts[src] || f.anyHosts[dst]) {
			return false
		}
	}
	if len(f.srcHosts) > 0 && !f.srcHosts[strings.ToLower(p.SourceIP)] {
		return false
	}
	if len(f.dstHosts) > 0 && !f.dstHosts[strings.ToLower(p.DestIP)] {
		return false
	}
	return true
}
