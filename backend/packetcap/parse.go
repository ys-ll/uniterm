package packetcap

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"
)

// ParseFrame parses a single raw captured frame into a Packet. The frame may
// be an Ethernet frame (Linux/macOS) or a raw IP datagram (Windows SIO_RCVALL).
// Returns nil for frames we cannot classify.
func ParseFrame(frame []byte, t time.Time) *Packet {
	p := &Packet{Time: packetTime(t), Len: len(frame), Data: hexString(frame)}
	if len(frame) == 0 {
		return nil
	}

	// Try Ethernet first (Linux/macOS AF_PACKET / BPF hand the L2 frame over).
	// ethPayloadOffset 已负责剥离可选的 VLAN 标签，并给出正确的 L3 起始偏移。
	offset := 0
	var ver byte
	if etype, off, ok := ethPayloadOffset(frame); ok {
		switch etype {
		case 0x0800:
			offset = off
			ver = 4
		case 0x86dd:
			offset = off
			ver = 6
		case 0x0806: // ARP
			return arpPacket(frame[off:], off, t)
		default:
			// 其它 L2 协议（LLDP / STP 等）暂不处理。
			return nil
		}
	} else {
		// Raw IP (Windows raw socket gives us the IP header directly).
		if len(frame) == 0 {
			return nil
		}
		ver = frame[0] >> 4
	}

	if ver == 4 || ver == 6 {
		// Build the decoded protocol tree from the raw frame.
		p.decode(frame)
	}
	if ver == 4 {
		return parseIPv4(frame, offset, p)
	}
	if ver == 6 {
		return parseIPv6(frame, offset, p)
	}
	return nil
}

// ethPayloadOffset inspects a (possibly VLAN-tagged) Ethernet header and returns
// the inner EtherType, the byte offset where the L3 payload begins, and whether
// the frame is a recognised Ethernet frame. A single 802.1Q / 802.1ad tag
// (0x8100 / 0x88a8) is stripped so the payload offset moves 14 → 18 bytes.
//
// 这是 Linux「抓不到包 / 只能抓到少量包」的核心修复：运营商 IPTV 网络大量使用
// VLAN（尤其 Trunk 口），此前只认 0x0800/0x86dd/0x0806，遇到 0x8100/0x88a8 会把
// 帧误判为裸 IP，从而在 IP 头部偏移处读到错误字节导致丢弃。
func ethPayloadOffset(frame []byte) (etype uint16, offset int, ok bool) {
	if len(frame) < 14 {
		return 0, 0, false
	}
	etype = uint16(frame[12])<<8 | uint16(frame[13])
	offset = 14
	// 剥掉一层 VLAN 标签：内层 EtherType 位于 16-17。
	if (etype == 0x8100 || etype == 0x88a8) && len(frame) >= 18 {
		etype = uint16(frame[16])<<8 | uint16(frame[17])
		offset = 18
	}
	switch etype {
	case 0x0800, 0x86dd, 0x0806:
		return etype, offset, true
	}
	return 0, 0, false
}

func arpPacket(payload []byte, ethOffset int, t time.Time) *Packet {
	if len(payload) < 28 {
		return nil
	}
	// We only classify ARP to show it; src/dst IPs at offsets 14 and 24.
	src := net.IP(payload[14:18]).String()
	dst := net.IP(payload[24:28]).String()
	p := &Packet{
		Time:     packetTime(t),
		Network:  "ARP",
		Protocol: "ARP",
		SourceIP: src,
		DestIP:   dst,
		Len:      len(payload) + ethOffset,
		Data:     hexString(payload),
	}
	// ARP is always carried in an Ethernet frame here; decode the ARP body into
	// the field tree. Offset is reported relative to the ARP payload (the L2
	// header fields are not decoded for ARP).
	p.decodeARP(payload, 0)
	return p
}

func parseIPv4(frame []byte, base int, p *Packet) *Packet {
	if len(frame) < base+20 || frame[base]>>4 != 4 {
		return nil
	}
	ihl := int(frame[base]&0x0F) * 4
	if ihl < 20 || len(frame) < base+ihl {
		return nil
	}
	total := int(frame[base+2])<<8 | int(frame[base+3])
	if total > 0 && base+total > len(frame) {
		total = len(frame) - base
	}
	proto := frame[base+9]
	src := net.IP(frame[base+12 : base+16]).String()
	dst := net.IP(frame[base+16 : base+20]).String()
	p.Network = "IPv4"
	p.SourceIP = src
	p.DestIP = dst
	body := frame[base+ihl : base+total]

	switch proto {
	case 6:
		return tcpPacket(body, p, src, dst)
	case 17:
		return udpPacket(body, p, src, dst)
	case 1:
		p.Protocol = "ICMP"
	default:
		p.Protocol = fmt.Sprintf("p%d", proto)
	}
	return p
}

func parseIPv6(frame []byte, base int, p *Packet) *Packet {
	if len(frame) < base+40 || frame[base]>>4 != 6 {
		return nil
	}
	next := frame[base+6]
	src := net.IP(frame[base+8 : base+24]).String()
	dst := net.IP(frame[base+24 : base+40]).String()
	p.Network = "IPv6"
	p.SourceIP = src
	p.DestIP = dst
	body := frame[base+40:]

	switch next {
	case 6:
		return tcpPacket(body, p, src, dst)
	case 17:
		return udpPacket(body, p, src, dst)
	case 58:
		p.Protocol = "ICMPv6"
	default:
		p.Protocol = fmt.Sprintf("p%d", next)
	}
	return p
}

func tcpPacket(body []byte, p *Packet, src, dst string) *Packet {
	if len(body) < 20 {
		p.Protocol = "TCP"
		return p
	}
	sp := uint16(body[0])<<8 | uint16(body[1])
	dp := uint16(body[2])<<8 | uint16(body[3])
	p.Protocol = "TCP"
	p.SourcePort = sp
	p.DestPort = dp
	p.Service = serviceName(dp)
	p.Flags = tcpFlags(body[13])
	return p
}

func udpPacket(body []byte, p *Packet, src, dst string) *Packet {
	if len(body) < 8 {
		p.Protocol = "UDP"
		return p
	}
	sp := uint16(body[0])<<8 | uint16(body[1])
	dp := uint16(body[2])<<8 | uint16(body[3])
	p.Protocol = "UDP"
	p.SourcePort = sp
	p.DestPort = dp
	p.Service = serviceName(dp)
	return p
}

// tcpFlags renders the 8 TCP control bits into a compact string.
func tcpFlags(b byte) string {
	var f []string
	if b&0x08 != 0 {
		f = append(f, "PSH")
	}
	if b&0x10 != 0 {
		f = append(f, "ACK")
	}
	if b&0x02 != 0 {
		f = append(f, "SYN")
	}
	if b&0x01 != 0 {
		f = append(f, "FIN")
	}
	if b&0x04 != 0 {
		f = append(f, "RST")
	}
	if b&0x20 != 0 {
		f = append(f, "URG")
	}
	if len(f) == 0 {
		return ""
	}
	return strings.Join(f, "|")
}

// serviceName maps a well-known port to its common service name.
func serviceName(port uint16) string {
	names := map[uint16]string{
		20: "ftp-data", 21: "ftp", 22: "ssh", 23: "telnet", 25: "smtp",
		53: "dns", 67: "dhcp", 68: "dhcp", 69: "tftp", 80: "http",
		110: "pop3", 111: "rpc", 123: "ntp", 137: "netbios", 138: "netbios",
		139: "netbios", 143: "imap", 161: "snmp", 162: "snmptrap", 179: "bgp",
		389: "ldap", 443: "https", 445: "smb", 465: "smtps", 514: "syslog",
		587: "smtp", 636: "ldaps", 873: "rsync", 993: "imaps", 995: "pop3s",
		1080: "socks", 1433: "mssql", 1521: "oracle", 2049: "nfs",
		3306: "mysql", 3389: "rdp", 5432: "postgresql", 5900: "vnc",
		5901: "vnc-1", 5902: "vnc-2", 6379: "redis", 8080: "http-alt",
		8443: "https-alt", 8888: "http-alt", 9092: "kafka", 9200: "elastic",
		11211: "memcached", 27017: "mongodb", 5000: "upnp", 5222: "xmpp",
		6667: "irc", 8000: "http-alt", 9418: "git",
	}
	if n, ok := names[port]; ok {
		return n
	}
	return ""
}

func hexString(b []byte) string {
	return strings.ToUpper(hex.EncodeToString(b))
}
