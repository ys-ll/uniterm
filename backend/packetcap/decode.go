package packetcap

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// decode builds the Wireshark-style decoded protocol tree (Packet.Fields) for an
// already-parsed frame. It re-walks the raw bytes using the same offsets the
// strict parsers (parse.go) already validated, so it never panics on a short
// frame: every field helper bounds-checks before reading.
//
// The tree mirrors what ParseFrame recognised: it assumes Ethernet when
// ethPayloadOffset recognises a (possibly VLAN-tagged) MAC+EtherType header,
// otherwise a raw IP datagram.
func (p *Packet) decode(frame []byte) {
	// The frontend only needs the tree for protocols it can also display in the
	// table; skip building anything for unknown payloads to keep it cheap.
	if p == nil || frame == nil {
		return
	}

	// Frame root（对标 Wireshark 的 "Frame N" 根节点）：承载整帧长度等信息。
	p.Fields = []FieldNode{
		fn("Frame (帧)", fmt.Sprintf("%d bytes on wire", len(frame)), 0, len(frame)),
	}

	if etype, off, ok := ethPayloadOffset(frame); ok {
		// innerTypeOff 是内层 EtherType 在帧中的字节偏移（无 VLAN=12，有 VLAN=16）。
		p.Fields = append(p.Fields, ethernetNode(frame, etype, off-2))
		// 若剥掉了一层 VLAN，额外附加一个 802.1Q 节点，展示标签的优先级/VID。
		if off > 14 {
			p.Fields = append(p.Fields, vlanNode(frame[12:off], 12))
		}
		payload := frame[off:]
		switch etype {
		case 0x0800:
			p.decodeIPV4(payload, off)
		case 0x86dd:
			p.decodeIPV6(payload, off)
		case 0x0806:
			p.decodeARP(payload, off)
		}
		return
	}

	// Raw IP datagram (Windows SIO_RCVALL).
	switch frame[0] >> 4 {
	case 4:
		p.decodeIPV4(frame, 0)
	case 6:
		p.decodeIPV6(frame, 0)
	}
}

// fn is a small helper to append a leaf FieldNode.
func fn(name, value string, offset, length int) FieldNode {
	return FieldNode{Name: name, Value: value, Offset: offset, Len: length}
}

func ethernetNode(frame []byte, etype uint16, innerTypeOff int) FieldNode {
	// EtherType 名称映射（对标 Wireshark 的 "Type: IPv4 (0x0800)"）。
	etypeName := map[uint16]string{0x0800: "IPv4", 0x0806: "ARP", 0x86dd: "IPv6", 0x8100: "VLAN", 0x88cc: "LLDP"}[etype]
	// Len = 14（无 VLAN）或 18（含 802.1Q 标签，内层 EtherType 位于 16-17）。
	node := FieldNode{
		Name:   "Ethernet II",
		Offset: 0,
		Len:    innerTypeOff + 2,
		Children: []FieldNode{
			fn("目的地址 (Destination)", ouiLabel(frame[0:6]), 0, 6),
			fn("源地址 (Source)", ouiLabel(frame[6:12]), 6, 6),
			fn("类型 (Type)", fmt.Sprintf("%s (0x%04x)", etypeName, etype), innerTypeOff, 2),
		},
	}
	return node
}

// vlanNode renders a 4-byte 802.1Q tag (TPID + TCI) as its own tree node. base is
// the tag's absolute offset in the frame (12 for a single tag). 让用户能直接看到
// 运营商 IPTV 网络常用的 VLAN ID。
func vlanNode(tag []byte, base int) FieldNode {
	tci := uint16(tag[2])<<8 | uint16(tag[3])
	prio := tci >> 13
	dei := (tci >> 12) & 1
	vid := tci & 0x0FFF
	return FieldNode{
		Name:   "802.1Q Virtual LAN (VLAN)",
		Offset: base,
		Len:    4,
		Children: []FieldNode{
			fn("优先度 (Priority)", strconv.Itoa(int(prio)), base, 2),
			fn("DEI", strconv.Itoa(int(dei)), base, 2),
			fn("VLAN ID (VID)", strconv.Itoa(int(vid)), base+2, 2),
		},
	}
}

// decode() 将 L3 payload 从以太网偏移处切片后传入，因此这里 frame 是相对偏移
// 0 起的 IP 报文字节（base 仅用于详情面板的 Offset 展示）。字节访问一律用相对
// 索引；若再用绝对 base 访问会错位并可能导致越界 panic（Linux 抓包被杀）。
func (p *Packet) decodeIPV4(frame []byte, base int) {
	if len(frame) < 20 {
		return
	}
	ihl := int(frame[0]&0x0F) * 4
	if ihl < 20 || ihl > len(frame) {
		return
	}
	ttl := frame[8]
	proto := frame[9]
	src := net.IP(frame[12:16]).String()
	dst := net.IP(frame[16:20]).String()
	total := int(frame[2])<<8 | int(frame[3])
	// 钳制 IP 层总长度：分片 / 截断帧的总长字段可能小于 ihl 或大于实际长度，
	// 原样切片会触发越界 panic（这是 Linux「抓包中断 / 抓不到包」的关键回归点）。
	if total < ihl {
		total = ihl
	}
	if total > len(frame) {
		total = len(frame)
	}

	children := []FieldNode{
		fn("版本 (Version)", "4", base, 1),
		fn("报头长度 (Header Length)", fmt.Sprintf("%d bytes (%d x 4)", ihl, ihl/4), base, 1),
		fn("服务类型 (TOS/DSCP)", fmt.Sprintf("0x%02x", frame[1]), base+1, 1),
		fn("总长度 (Total Length)", strconv.Itoa(total), base+2, 2),
		fn("标识 (Identification)", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(frame[4:6])), base+4, 2),
	}

	// 标志位展开 + 分片偏移（对标 Wireshark：单独列出 DF/MF 位与 Fragment Offset）。
	flagsRaw := binary.BigEndian.Uint16(frame[6:8])
	df := "0"
	if flagsRaw&0x4000 != 0 {
		df = "1 (不分片/Don't fragment)"
	}
	mf := "0"
	if flagsRaw&0x2000 != 0 {
		mf = "1 (更多分片/More fragments)"
	}
	offset := flagsRaw & 0x1FFF
	children = append(children,
		fn("标志位 (Flags)", fmt.Sprintf("0x%03x", flagsRaw>>13), base+6, 2),
		fn("  └ 保留位 (Reserved)", fmt.Sprintf("%d", (flagsRaw>>15)&1), base+6, 2),
		fn("  └ 不分片 (Don't fragment)", df, base+6, 2),
		fn("  └ 更多分片 (More fragments)", mf, base+6, 2),
		fn("分片偏移 (Fragment offset)", strconv.Itoa(int(offset)), base+6, 2),
	)
	children = append(children,
		fn("生存时间 (TTL)", strconv.Itoa(int(ttl)), base+8, 1),
		fn("下一层协议 (Protocol)", fmt.Sprintf("%d (%s)", proto, ipProtoName(proto)), base+9, 1),
		fn("校验和 (Checksum)", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(frame[10:12])), base+10, 2),
		fn("源地址 (Source)", src, base+12, 4),
		fn("目的地址 (Destination)", dst, base+16, 4),
	)

	bodyStart := ihl
	body := frame[bodyStart:total]
	node := FieldNode{Name: "IPv4", Offset: base, Len: ihl, Children: children}
	p.Fields = append(p.Fields, node)

	switch proto {
	case 6:
		p.decodeTCP(body, base+bodyStart)
	case 17:
		p.decodeUDP(body, base+bodyStart)
	case 1:
		p.decodeICMP(body, base+bodyStart)
	default:
		p.Fields = append(p.Fields, FieldNode{
			Name:   fmt.Sprintf("未知上层协议 (proto %d)", proto),
			Offset: base + bodyStart,
			Len:    len(body),
		})
	}
}

func (p *Packet) decodeIPV6(frame []byte, base int) {
	if len(frame) < 40 {
		return
	}
	next := frame[6]
	hop := frame[7]
	src := net.IP(frame[8:24]).String()
	dst := net.IP(frame[24:40]).String()
	payloadLen := int(binary.BigEndian.Uint16(frame[4:6]))

	children := []FieldNode{
		fn("版本 (Version)", "6", base, 1),
		fn("流量类别 (Traffic Class)", fmt.Sprintf("0x%02x", frame[1]), base+1, 1),
		fn("流标签 (Flow Label)", fmt.Sprintf("0x%05x", uint32(frame[1]&0x0F)<<16|uint32(frame[2])<<8|uint32(frame[3])), base+1, 3),
		fn("载荷长度 (Payload Length)", strconv.Itoa(int(payloadLen)), base+4, 2),
		fn("下一层头部 (Next Header)", fmt.Sprintf("%d (%s)", next, ipProtoName(next)), base+6, 1),
		fn("跳数限制 (Hop Limit)", strconv.Itoa(int(hop)), base+7, 1),
		fn("源地址 (Source)", src, base+8, 16),
		fn("目的地址 (Destination)", dst, base+24, 16),
	}
	p.Fields = append(p.Fields, FieldNode{Name: "IPv6", Offset: base, Len: 40, Children: children})

	bodyStart := 40
	body := frame[bodyStart:]
	// 钳制 payload：iHL（next header 链）总长不能超出实际捕获长度。
	if payloadLen >= 0 && bodyStart+payloadLen < len(body) {
		body = body[:payloadLen]
	}
	switch next {
	case 6:
		p.decodeTCP(body, base+bodyStart)
	case 17:
		p.decodeUDP(body, base+bodyStart)
	case 58:
		p.decodeICMP(body, base+bodyStart)
	default:
		p.Fields = append(p.Fields, FieldNode{
			Name:   fmt.Sprintf("未知上层协议 (next %d)", next),
			Offset: base + bodyStart,
			Len:    len(body),
		})
	}
}

func (p *Packet) decodeARP(payload []byte, base int) {
	if len(payload) < 28 {
		return
	}
	// ARP payload starts at base; hardware/protocol types are the first 8 bytes,
	// op at 6-7, sender IP at 14-17, target IP at 24-27.
	hwType := binary.BigEndian.Uint16(payload[0:2])
	protoType := binary.BigEndian.Uint16(payload[2:4])
	hwSize := payload[4]
	protoSize := payload[5]
	op := binary.BigEndian.Uint16(payload[6:8])
	srcIP := ""
	if protoSize == 4 {
		srcIP = net.IP(payload[14:18]).String()
	}
	tgtIP := ""
	if protoSize == 4 {
		tgtIP = net.IP(payload[24:28]).String()
	}

	opName := strconv.Itoa(int(op))
	switch op {
	case 1:
		opName = "1 (请求/request)"
	case 2:
		opName = "2 (应答/reply)"
	}
	p.Fields = append(p.Fields, FieldNode{
		Name:  "ARP",
		Offset: base,
		Len:   28,
		Children: []FieldNode{
			fn("硬件类型 (Hardware type)", fmt.Sprintf("0x%04x", hwType), base, 2),
			fn("协议类型 (Protocol type)", fmt.Sprintf("0x%04x", protoType), base+2, 2),
			fn("硬件地址长度 (HW size)", strconv.Itoa(int(hwSize)), base+4, 1),
			fn("协议地址长度 (Proto size)", strconv.Itoa(int(protoSize)), base+5, 1),
			fn("操作码 (Opcode)", fmt.Sprintf("%d (%s)", op, opName), base+6, 2),
			fn("发送方 MAC (Sender MAC)", ouiLabel(payload[8:14]), base+8, 6),
			fn("发送方 IP (Sender IP)", srcIP, base+14, int(protoSize)),
			fn("目标 MAC (Target MAC)", ouiLabel(payload[18:24]), base+18, 6),
			fn("目标 IP (Target IP)", tgtIP, base+24, int(protoSize)),
		},
	})
}

func (p *Packet) decodeTCP(body []byte, base int) {
	if len(body) < 20 {
		p.Fields = append(p.Fields, FieldNode{Name: "TCP", Offset: base, Len: len(body)})
		return
	}
	sport := binary.BigEndian.Uint16(body[0:2])
	dport := binary.BigEndian.Uint16(body[2:4])
	seq := binary.BigEndian.Uint32(body[4:8])
	ack := binary.BigEndian.Uint32(body[8:12])
	dataOffset := int(body[12]>>4) * 4
	// 钳制数据偏移：畸形 / 截断报文 dataOffset 可能大于实际长度，原样用
	// body[20:dataOffset] 切片会越界 panic（同 IP 总长问题，一并修复）。
	if dataOffset < 20 || dataOffset > len(body) {
		dataOffset = len(body)
	}
	flags := tcpFlags(body[13])
	flagName := flags
	switch uint16(body[13]) & 0x1F {
	case 0x02:
		flagName = "SYN"
	case 0x10:
		flagName = "ACK"
	case 0x18:
		flagName = "PSH, ACK"
	case 0x12:
		flagName = "SYN, ACK"
	case 0x11:
		flagName = "FIN, ACK"
	case 0x01:
		flagName = "FIN"
	}
	window := binary.BigEndian.Uint16(body[14:16])
	checksum := binary.BigEndian.Uint16(body[16:18])
	urgent := binary.BigEndian.Uint16(body[18:20])

	children := []FieldNode{
		fn("源端口 (Source Port)", packPort(sport), base, 2),
		fn("目的端口 (Destination Port)", packPort(dport), base+2, 2),
		fn("序号 (Sequence)", "0x"+fmt.Sprintf("%08x", seq), base+4, 4),
		fn("确认号 (Acknowledgment)", "0x"+fmt.Sprintf("%08x", ack), base+8, 4),
		fn("数据偏移 (Data Offset)", fmt.Sprintf("%d bytes", dataOffset), base+12, 1),
		fn("标志 (Flags)", flagName, base+13, 1),
	}
	// 标志位逐位展开（对标 Wireshark：NS/CWR/ECE/URG/ACK/PSH/RST/SYN/FIN）。
	b13 := body[13]
	children = append(children,
		fn("  └ NS", fmt.Sprintf("%d", b13>>7&1), base+13, 1),
		fn("  └ CWR", fmt.Sprintf("%d", b13>>6&1), base+13, 1),
		fn("  └ ECE", fmt.Sprintf("%d", b13>>5&1), base+13, 1),
		fn("  └ URG", fmt.Sprintf("%d", b13>>4&1), base+13, 1),
		fn("  └ ACK", fmt.Sprintf("%d", b13>>3&1), base+13, 1),
		fn("  └ PSH", fmt.Sprintf("%d", b13>>2&1), base+13, 1),
		fn("  └ RST", fmt.Sprintf("%d", b13>>1&1), base+13, 1),
		fn("  └ SYN", fmt.Sprintf("%d", b13>>0&1), base+13, 1),
		fn("  └ FIN", fmt.Sprintf("%d", b13&1), base+13, 1),
	)
	children = append(children,
		fn("窗口大小 (Window)", fmt.Sprintf("%d", window), base+14, 2),
		fn("校验和 (Checksum)", fmt.Sprintf("0x%04x", checksum), base+16, 2),
		fn("紧急指针 (Urgent Pointer)", strconv.Itoa(int(urgent)), base+18, 2),
	)
	// TCP 选项（对标 Wireshark 的 Options 子树，常见 MSS/窗口缩放/SACK/时间戳）。
	if opts := decodeTCPOptions(body[20:dataOffset], base+20); len(opts) > 0 {
		children = append(children, FieldNode{
			Name:     fmt.Sprintf("选项 (Options, %d 个)", len(opts)),
			Offset:   base + 20,
			Len:      dataOffset - 20,
			Children: opts,
		})
	}
	p.Fields = append(p.Fields, FieldNode{Name: "TCP", Offset: base, Len: dataOffset, Children: children})

	// Lightweight L7 hint for common ports.
	if hint := appLayer(body[dataOffset:], dport); hint.Name != "" {
		p.Fields = append(p.Fields, hint)
	}
}

func (p *Packet) decodeUDP(body []byte, base int) {
	if len(body) < 8 {
		p.Fields = append(p.Fields, FieldNode{Name: "UDP", Offset: base, Len: len(body)})
		return
	}
	sport := binary.BigEndian.Uint16(body[0:2])
	dport := binary.BigEndian.Uint16(body[2:4])
	length := binary.BigEndian.Uint16(body[4:6])
	children := []FieldNode{
		fn("源端口 (Source Port)", packPort(sport), base, 2),
		fn("目的端口 (Destination Port)", packPort(dport), base+2, 2),
		fn("长度 (Length)", strconv.Itoa(int(length)), base+4, 2),
		fn("校验和 (Checksum)", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(body[6:8])), base+6, 2),
	}
	p.Fields = append(p.Fields, FieldNode{Name: "UDP", Offset: base, Len: int(length), Children: children})

	if hint := appLayer(body[8:], dport); hint.Name != "" {
		p.Fields = append(p.Fields, hint)
	}
}

func (p *Packet) decodeICMP(body []byte, base int) {
	if len(body) < 8 {
		p.Fields = append(p.Fields, FieldNode{Name: "ICMP", Offset: base, Len: len(body)})
		return
	}
	typ := body[0]
	code := body[1]
	typeName := strconv.Itoa(int(typ))
	switch typ {
	case 0:
		typeName = "0 (回显应答/Echo reply)"
	case 3:
		typeName = "3 (目标不可达/Destination unreachable)"
	case 5:
		typeName = "5 (重定向/Redirect)"
	case 8:
		typeName = "8 (回显请求/Echo request)"
	case 11:
		typeName = "11 (超时/Time exceeded)"
	}
	p.Fields = append(p.Fields, FieldNode{
		Name:  "ICMP",
		Offset: base,
		Len:   8,
		Children: []FieldNode{
			fn("类型 (Type)", fmt.Sprintf("%d (%s)", typ, typeName), base, 1),
			fn("代码 (Code)", strconv.Itoa(int(code)), base+1, 1),
			fn("校验和 (Checksum)", fmt.Sprintf("0x%04x", binary.BigEndian.Uint16(body[2:4])), base+2, 2),
		},
	})
}

// appLayer returns a light L7 decode hint (DNS/HTTP/TLS) for common ports, or an
// empty node when nothing decodes. payload starts at the TCP/UDP payload start.
func appLayer(payload []byte, dport uint16) FieldNode {
	if len(payload) == 0 {
		return FieldNode{}
	}
	switch dport {
	case 53:
		return decodeDNS(payload, 0)
	case 67, 68: // DHCP 服务器 / 客户端
		return decodeDHCP(payload, 0)
	case 80, 8080, 8000, 8888:
		return decodeHTTP(payload, 0)
	}
	return FieldNode{}
}

// dhcpMsgTypeNames maps DHCP message-type option code to its friendly name.
var dhcpMsgTypeNames = map[byte]string{
	1: "DHCPDISCOVER",
	2: "DHCPOFFER",
	3: "DHCPREQUEST",
	4: "DHCPDECLINE",
	5: "DHCPACK",
	6: "DHCPNAK",
	7: "DHCPRELEASE",
	8: "DHCPINFORM",
}

// decodeDHCP renders a dynamic host configuration (DHCP) message as a readable
// tree. IPTV 机顶盒开机正是通过 DHCP 报文（尤其 Option 125 厂商自定义选项）
// 从运营商网络下发 IPTV 业务配置，因此这里把报头、magic cookie 以及每个
// option（含 125 企业子选项）逐项解码展示。
func decodeDHCP(payload []byte, base int) FieldNode {
	if len(payload) < 24 {
		return FieldNode{}
	}

	children := []FieldNode{}
	// 固定报头（BOOTP 部分）
	op := payload[0]
	opName := strconv.Itoa(int(op))
	switch op {
	case 1:
		opName = "1 (BOOTREQUEST, 客户端->服务器)"
	case 2:
		opName = "2 (BOOTREPLY, 服务器->客户端)"
	}
	children = append(children, fn("消息类型 (Message type)", opName, base, 1))

	htype := payload[1]
	hlen := payload[2]
	hops := payload[3]
	children = append(children,
		fn("硬件类型 (Hardware type)", strconv.Itoa(int(htype)), base+1, 1),
		fn("硬件地址长度 (Hardware len)", strconv.Itoa(int(hlen)), base+2, 1),
		fn("跳数 (Hops)", strconv.Itoa(int(hops)), base+3, 1),
	)

	if len(payload) >= 8 {
		children = append(children, fn("事务 ID (Transaction ID)", fmt.Sprintf("0x%08x", binary.BigEndian.Uint32(payload[4:8])), base+4, 4))
	}
	if len(payload) >= 10 {
		children = append(children, fn("经过秒数 (Seconds elapsed)", strconv.Itoa(int(binary.BigEndian.Uint16(payload[8:10]))), base+8, 2))
	}
	if len(payload) >= 12 {
		flags := binary.BigEndian.Uint16(payload[10:12])
		broadcast := "0"
		if flags&0x8000 != 0 {
			broadcast = "1 (广播)"
		}
		children = append(children, fn("标志 (Flags)", fmt.Sprintf("0x%04x (广播位=%s)", flags, broadcast), base+10, 2))
	}
	if len(payload) >= 16 {
		children = append(children, fn("客户端地址 (Client IP)", net.IP(payload[12:16]).String(), base+12, 4))
	}
	if len(payload) >= 20 {
		children = append(children, fn("你的地址 (Your IP)", net.IP(payload[16:20]).String(), base+16, 4))
	}
	if len(payload) >= 24 {
		children = append(children, fn("服务器地址 (Server IP)", net.IP(payload[20:24]).String(), base+20, 4))
	}
	if len(payload) >= 28 {
		children = append(children, fn("网关地址 (Gateway IP)", net.IP(payload[24:28]).String(), base+24, 4))
	}
	// chaddr: 前 hlen 字节为客户端 MAC（通常 16 字节字段但只用 6）
	if len(payload) >= 44 {
		chaddr := payload[28:44]
		mac := ""
		if hlen > 0 && hlen <= 16 {
			mac = net.HardwareAddr(chaddr[:hlen]).String()
		}
		children = append(children, fn("客户端硬件地址 (Client MAC)", mac, base+28, int(hlen)))
	}
	if len(payload) >= 172 {
		children = append(children, fn("服务器主机名 (Server host name)", dhcpStr(payload[44:108]), base+44, 64))
		children = append(children, fn("启动文件名 (Boot file name)", dhcpStr(payload[108:236]), base+108, 128))
	}
	// Magic cookie 应为 0x63825363
	if len(payload) >= 240 {
		cookie := binary.BigEndian.Uint32(payload[236:240])
		cookieName := fmt.Sprintf("0x%08x", cookie)
		if cookie == 0x63825363 {
			cookieName += " (DHCP)"
		}
		children = append(children, fn("魔法数 (Magic cookie)", cookieName, base+236, 4))
	}

	// Options（TLV），从 240 起
	if len(payload) > 240 {
		opts := decodeDHCPOptions(payload[240:], base+240)
		if len(opts) > 0 {
			children = append(children, FieldNode{
				Name:  fmt.Sprintf("选项 (Options, %d 个)", len(opts)),
				Offset: base + 240,
				Len:   len(payload) - 240,
				Children: opts,
			})
		}
	}

	return FieldNode{
		Name:     "DHCP (动态主机配置)",
		Offset:   base,
		Len:      len(payload),
		Children: children,
	}
}

// decodeDHCPOptions walks the DHCP option TLV area and returns one FieldNode per
// option. data starts at the first option byte (after the 4-byte magic cookie).
func decodeDHCPOptions(data []byte, base int) []FieldNode {
	var out []FieldNode
	i := 0
	count := 0
	// 保护：最多解析 200 个 option，防止畸形报文死循环/越界
	for i < len(data) && count < 200 {
		code := data[i]
		switch code {
		case 0: // Pad
			i++
			continue
		case 255: // End
			out = append(out, fn("End (结束)", "0xFF", base+i, 1))
			return out
		}
		if i+1 >= len(data) {
			break
		}
		length := int(data[i+1])
		if i+2+length > len(data) {
			break
		}
		val := data[i+2 : i+2+length]
		bound := FieldNode{
			Name:   decodeDHCPOptionName(code),
			Value:  decodeDHCPOptionValue(code, val),
			Offset: base + i,
			Len:    2 + length,
		}
		// Option 125：附上原始十六进制 + 可打印 ASCII 供用户核对
		if code == 125 {
			bound.Children = append(bound.Children,
				fn("原始数据 (Raw, hex)", dhcpHex(val), base+i+2, len(val)),
				fn("可打印内容 (ASCII)", dhcpASCII(val), base+i+2, len(val)),
			)
		}
		out = append(out, bound)
		count++
		i += 2 + length
	}
	return out
}

// decodeDHCPOptionName maps an option code to a human-friendly label.
func decodeDHCPOptionName(code byte) string {
	switch code {
	case 1:
		return "子网掩码 (Subnet Mask, option 1)"
	case 3:
		return "路由器 (Router, option 3)"
	case 6:
		return "域名服务器 (Domain Name Server, option 6)"
	case 12:
		return "主机名 (Host Name, option 12)"
	case 15:
		return "域名 (Domain Name, option 15)"
	case 51:
		return "租约时间 (Lease Time, option 51)"
	case 53:
		return "消息类型 (Message Type, option 53)"
	case 54:
		return "服务器标识 (Server Identifier, option 54)"
	case 55:
		return "参数请求列表 (Parameter Request List, option 55)"
	case 58:
		return "续租时间 (Renewal Time, option 58)"
	case 59:
		return "重绑时间 (Rebinding Time, option 59)"
	case 60:
		return "厂商类别 (Vendor Class, option 60)"
	case 61:
		return "客户端标识 (Client Identifier, option 61)"
	case 125:
		return "厂商自定义选项 (Vendor-Identifying, option 125)"
	default:
		return fmt.Sprintf("选项 (Option %d)", code)
	}
}

// decodeDHCPOptionValue renders an option's bytes into a readable string.
func decodeDHCPOptionValue(code byte, val []byte) string {
	if len(val) == 0 {
		return "(空)"
	}
	switch code {
	case 1, 3, 6, 54: // IP 地址列表
		ips := make([]string, 0, len(val)/4)
		for i := 0; i+4 <= len(val); i += 4 {
			ips = append(ips, net.IP(val[i:i+4]).String())
		}
		if len(ips) > 0 {
			return strings.Join(ips, ", ")
		}
	case 12, 15, 60: // ASCII 字符串
		return dhcpStr(val)
	case 51, 58, 59: // 秒数
		if len(val) == 4 {
			return strconv.Itoa(int(binary.BigEndian.Uint32(val))) + " 秒"
		}
	case 53: // 消息类型
		if len(val) >= 1 {
			if name, ok := dhcpMsgTypeNames[val[0]]; ok {
				return fmt.Sprintf("%d (%s)", val[0], name)
			}
			return strconv.Itoa(int(val[0]))
		}
	case 61: // 客户端标识：首字节类型 + 硬件地址
		if len(val) >= 2 {
			return fmt.Sprintf("类型 %d, 硬件 %s", val[0], net.HardwareAddr(val[1:]).String())
		}
	case 55: // 参数请求列表：列出每个请求的 option 码
		codes := make([]string, 0, len(val))
		for _, c := range val {
			codes = append(codes, strconv.Itoa(int(c)))
		}
		return strings.Join(codes, ", ")
	case 125: // 厂商自定义：企业 ID + 子选项
		return dhcpVendor125(val)
	}
	return dhcpHex(val)
}

// dhcpVendor125 renders RFC 3925 vendor-identifying option data: a 4-byte
// enterprise number followed by sub-options (option 125 是 IPTV 配置的关键承载).
func dhcpVendor125(val []byte) string {
	if len(val) < 4 {
		return dhcpHex(val)
	}
	ent := binary.BigEndian.Uint32(val[0:4])
	sub := val[4:]
	subDesc := ""
	if len(sub) > 0 {
		subDesc = "; 子选项 " + dhcpSubOptionsSummary(sub)
	}
	return fmt.Sprintf("企业 ID (Enterprise) 0x%08x (%d)%s", ent, ent, subDesc)
}

// dhcpSubOptionsSummary returns a compact comma-joined summary of the sub-option
// codes inside a vendor option, so users can spot IPTV 业务配置项.
func dhcpSubOptionsSummary(sub []byte) string {
	var parts []string
	i := 0
	for i+1 < len(sub) {
		code := sub[i]
		if code == 0 {
			i++
			continue
		}
		if code == 255 {
			break
		}
		length := int(sub[i+1])
		if i+2+length > len(sub) {
			break
		}
		parts = append(parts, fmt.Sprintf("%d(%s)", code, dhcpSubValueHint(code, sub[i+2:i+2+length])))
		i += 2 + length
	}
	if len(parts) == 0 {
		return dhcpHex(sub)
	}
	return strings.Join(parts, ", ")
}

// dhcpSubValueHint gives a short readable hint for a vendor sub-option's bytes.
func dhcpSubValueHint(code byte, val []byte) string {
	if len(val) == 0 {
		return ""
	}
	// 常见厂商子选项：若整体/开头是可见文本则直接显示，否则显示十六进制。
	if s := printableAll(val); s != "" {
		return s
	}
	return dhcpHex(val)
}

// printableAll returns the trimmed printable-ASCII rendering of b, or "" if none.
func printableAll(b []byte) string {
	s := dhcpASCII(b)
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return strings.TrimSpace(s)
}

// dhcpHex renders bytes as uppercase space-separated hex ("AA BB ..").
func dhcpHex(b []byte) string {
	if len(b) == 0 {
		return "(空)"
	}
	var sb strings.Builder
	for i, v := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02X", v)
	}
	return sb.String()
}

// dhcpASCII renders bytes as printable ASCII, '.' for non-printable.
func dhcpASCII(b []byte) string {
	var sb strings.Builder
	for _, v := range b {
		if v >= 32 && v <= 126 {
			sb.WriteByte(v)
		} else {
			sb.WriteByte('.')
		}
	}
	return sb.String()
}

// dhcpStr renders a fixed-size field's bytes as a trimmed ASCII string.
func dhcpStr(b []byte) string {
	return strings.TrimSpace(string(b))
}

// ipProtoName maps an IP protocol number to its IANA name (对标 Wireshark).
func ipProtoName(p byte) string {
	switch p {
	case 1:
		return "ICMP"
	case 2:
		return "IGMP"
	case 4:
		return "IPIP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	case 41:
		return "IPv6"
	case 47:
		return "GRE"
	case 50:
		return "ESP"
	case 51:
		return "AH"
	case 58:
		return "ICMPv6"
	case 89:
		return "OSPF"
	case 103:
		return "PIM"
	default:
		return "未知 (unknown)"
	}
}

// decodeTCPOptions walks the TCP option area (kind/len TLVs) and returns one
// FieldNode per recognisable option; unknown kinds are shown as raw hex.
func decodeTCPOptions(buf []byte, base int) []FieldNode {
	var out []FieldNode
	i := 0
	for i < len(buf) {
		kind := buf[i]
		switch kind {
		case 0: // End of Option List
			out = append(out, fn("End of Option List (选项表结束)", "0x00", base+i, 1))
			return out
		case 1: // No-Operation
			out = append(out, fn("[NOP] No-Operation (填充无操作)", "1 byte", base+i, 1))
			i++
			continue
		}
		if i+1 >= len(buf) {
			break
		}
		length := int(buf[i+1])
		if length < 2 || i+length > len(buf) {
			break
		}
		val := buf[i+2 : i+length]
		node := FieldNode{Offset: base + i, Len: length}
		switch kind {
		case 2: // MSS
			if len(val) == 2 {
				node.Name = "MSS (最大分段大小, option 2)"
				node.Value = strconv.Itoa(int(binary.BigEndian.Uint16(val))) + " bytes"
			}
		case 3: // Window Scale
			if len(val) == 1 {
				node.Name = "窗口缩放 (Window Scale, option 3)"
				node.Value = fmt.Sprintf("%d (乘数 %d)", val[0], 1<<val[0])
			}
		case 4: // SACK Permitted
			node.Name = "SACK 允许 (SACK Permitted, option 4)"
			node.Value = "true"
		case 5: // SACK
			if len(val) >= 8 && len(val)%8 == 0 {
				node.Name = "SACK (选择确认, option 5)"
				var edges []string
				for j := 0; j+8 <= len(val); j += 8 {
					edges = append(edges, fmt.Sprintf("[%d→%d]", binary.BigEndian.Uint32(val[j:j+4]), binary.BigEndian.Uint32(val[j+4:j+8])))
				}
				node.Value = strings.Join(edges, " ")
			}
		case 8: // Timestamps
			if len(val) == 8 {
				node.Name = "时间戳 (Timestamps, option 8)"
				node.Value = fmt.Sprintf("TSval %d, TSecr %d", binary.BigEndian.Uint32(val[0:4]), binary.BigEndian.Uint32(val[4:8]))
			}
		case 28: // UTO / User Timeout
			node.Name = fmt.Sprintf("用户超时 (User Timeout, option %d)", kind)
		}
		if node.Name == "" {
			node.Name = fmt.Sprintf("TCP 选项 (Option %d)", kind)
			node.Value = dhcpHex(val)
		}
		out = append(out, node)
		i += length
	}
	return out
}

// decodeDNS renders a full DNS message (对标 Wireshark)：报头 flags 逐位展开，
// 并对 Queries / Answers / Authorities / Additions 逐条解码，包含名称压缩解引用、
// 类型/类别可读名、TTL，以及按类型解析的 rdata（IPv4/IPv6/CNAME/NS/SOA/TXT…）。
func decodeDNS(payload []byte, base int) FieldNode {
	if len(payload) < 12 {
		return FieldNode{}
	}
	// Header: ID(2) flags(2) QDCOUNT(2) ANCOUNT(2) NSCOUNT(2) ARCOUNT(2)
	id := binary.BigEndian.Uint16(payload[0:2])
	flags := binary.BigEndian.Uint16(payload[2:4])
	qd := binary.BigEndian.Uint16(payload[4:6])
	an := binary.BigEndian.Uint16(payload[6:8])
	ns := binary.BigEndian.Uint16(payload[8:10])
	ar := binary.BigEndian.Uint16(payload[10:12])

	// 响应码 / 操作码可读名
	rcodeName := map[byte]string{
		0: "No error", 1: "Format error", 2: "Server failure",
		3: "Name error", 4: "Not implemented", 5: "Refused",
	}[byte(flags&0x000F)]
	opcode := (flags >> 11) & 0x000F
	opcodeName := map[uint16]string{0: "QUERY", 1: "IQUERY", 2: "STATUS"}[opcode]

	children := []FieldNode{
		fn("事务 ID (Transaction ID)", fmt.Sprintf("0x%04x", id), base, 2),
		fn("标志 (Flags)", fmt.Sprintf("0x%04x", flags), base+2, 2),
		fn("  └ 响应 (Response)", fmt.Sprintf("%d (%s)", flags>>15&1, map[uint16]string{0: "查询/query", 1: "应答/response"}[flags>>15&1]), base+2, 2),
		fn("  └ 操作码 (Opcode)", fmt.Sprintf("%d (%s)", opcode, opcodeName), base+2, 2),
		fn("  └ 权威应答 (Authoritative)", fmt.Sprintf("%d", flags>>10&1), base+2, 1),
		fn("  └ 截断 (Truncated)", fmt.Sprintf("%d", flags>>9&1), base+2, 1),
		fn("  └ 希望递归 (Recursion desired)", fmt.Sprintf("%d", flags>>8&1), base+2, 1),
		fn("  └ 可用递归 (Recursion available)", fmt.Sprintf("%d", flags>>7&1), base+2, 1),
		fn("  └ 保留 (Reserved)", fmt.Sprintf("%d", flags>>6&1), base+2, 1),
		fn("  └ 认证数据 (Authentic data)", fmt.Sprintf("%d", flags>>5&1), base+2, 1),
		fn("  └ 检查禁用 (Checking disabled)", fmt.Sprintf("%d", flags>>4&1), base+2, 1),
		fn("  └ 响应码 (Response code)", fmt.Sprintf("%d (%s)", flags&0x000F, rcodeName), base+3, 1),
		fn("问题数 (Questions)", strconv.Itoa(int(qd)), base+4, 2),
		fn("回答数 (Answer RRs)", strconv.Itoa(int(an)), base+6, 2),
		fn("权威数 (Authority RRs)", strconv.Itoa(int(ns)), base+8, 2),
		fn("附加数 (Additional RRs)", strconv.Itoa(int(ar)), base+10, 2),
	}

	// 问题区之后是回答/权威/附加，顺序解析并解引用压缩名称。
	off := 12
	cursor := &off
	if qd > 0 {
		qs, next := decodeDNSSection(payload, cursor, int(qd), "问题 (Queries)", true, base)
		children = append(children, qs)
		*cursor = next
	}
	if an > 0 {
		rs, next := decodeDNSSection(payload, cursor, int(an), "回答 (Answers)", false, base)
		children = append(children, rs)
		*cursor = next
	}
	if ns > 0 {
		rs, next := decodeDNSSection(payload, cursor, int(ns), "权威 (Authoritative)", false, base)
		children = append(children, rs)
		*cursor = next
	}
	if ar > 0 {
		rs, _ := decodeDNSSection(payload, cursor, int(ar), "附加 (Additional)", false, base)
		children = append(children, rs)
	}

	return FieldNode{
		Name:     "DNS",
		Offset:   base,
		Len:      len(payload),
		Children: children,
	}
}

// decodeDNSSection parses `count` resource records (queries or RRs) starting at
// `*off`. Returns a label FieldNode for the group plus the next byte offset.
func decodeDNSSection(payload []byte, off *int, count int, label string, isQuery bool, base int) (FieldNode, int) {
	var items []FieldNode
	start := *off
	i := *off
	for n := 0; n < count && i < len(payload); n++ {
		name, _, rawNext, ok := dnsName(payload, i)
		if !ok {
			break
		}
		// rawNext 是 name 在原始字节流中的结束位置（压缩指针时只算指针本身）。
		// type/QCLASS（query）或 type/class/TTL/rdlen/rd（RR）
		if rawNext+10 > len(payload) {
			break
		}
		typ := binary.BigEndian.Uint16(payload[rawNext : rawNext+2])
		klass := binary.BigEndian.Uint16(payload[rawNext+2 : rawNext+4])
		relBase := base + i
		res := FieldNode{
			Name:   fmt.Sprintf("%d. %s %s", n+1, dnsTypeName(typ), name),
			Offset: relBase,
		}
		res.Children = append(res.Children,
			fn("名称 (Name)", name, relBase, rawNext-i),
			fn("类型 (Type)", fmt.Sprintf("%d (%s)", typ, dnsTypeName(typ)), base+rawNext, 2),
			fn("类别 (Class)", dnsClass(klass), base+rawNext+2, 2),
		)
		if isQuery {
			i = rawNext + 4
			items = append(items, res)
			continue
		}
		if rawNext+12 > len(payload) {
			break
		}
		ttl := binary.BigEndian.Uint32(payload[rawNext+4 : rawNext+8])
		rdlen := int(binary.BigEndian.Uint16(payload[rawNext+8 : rawNext+10]))
		rdStart := rawNext + 10
		if rdStart+rdlen > len(payload) {
			break
		}
		rdata := payload[rdStart : rdStart+rdlen]
		res.Children = append(res.Children,
			fn("生存时间 (Time to live)", fmt.Sprintf("%d", ttl), base+rawNext+4, 4),
			fn("数据长度 (Data length)", strconv.Itoa(rdlen), base+rawNext+8, 2),
		)
		if rd := dnsRData(payload, typ, rdata, rdStart, base); rd != "" {
			res.Children = append(res.Children, fn("数据 (Data)", rd, base+rdStart, rdlen))
		} else {
			res.Children = append(res.Children, fn("数据 (Data)", dnsHexSpace(rdata), base+rdStart, rdlen))
		}
		items = append(items, res)
		i = rdStart + rdlen
	}
	return FieldNode{Name: label, Offset: base + start, Len: i - start, Children: items}, i
}

// dnsName reads a (possibly compressed) DNS name at i in payload, returning the
// readable dotted name Amphorism, the next byte offset after the name in the raw
// stream, and ok. Compression pointers (0xC0 0x0X) jump elsewhere in the message.
func dnsName(payload []byte, i int) (name string, consumed int, next int, ok bool) {
	var labels []string
	pos := i
	jumps := 0
	next = i
	for {
		if pos >= len(payload) {
			return "", 0, 0, false
		}
		b := payload[pos]
		if b == 0 { // end of name
			if consumed == 0 {
				consumed = 1
			}
			if next == i {
				next = pos + 1
			}
			consumed += 1
			return joinLabels(labels), consumed, next, true
		}
		if b&0xC0 == 0xC0 { // pointer
			if pos+1 >= len(payload) {
				return "", 0, 0, false
			}
			p := int(binary.BigEndian.Uint16(payload[pos:pos+2]) & 0x3FFF)
			if next == i {
				next = pos + 2
				consumed = pos - i + 2
			}
			jumps++
			if jumps > 128 || p >= len(payload) {
				return "", 0, 0, false
			}
			pos = p
			continue
		}
		// label
		l := int(b)
		if pos+1+l > len(payload) {
			return "", 0, 0, false
		}
		labels = append(labels, string(payload[pos+1:pos+1+l]))
		pos += 1 + l
		if next == i {
			consumed = pos - i
		}
	}
}

func joinLabels(labels []string) string {
	if len(labels) == 0 {
		return "(空/root)"
	}
	return strings.Join(labels, ".")
}

// dnsTypeName maps a DNS RR type code to its mnemonic name.
func dnsTypeName(t uint16) string {
	switch t {
	case 1:
		return "A"
	case 2:
		return "NS"
	case 5:
		return "CNAME"
	case 6:
		return "SOA"
	case 12:
		return "PTR"
	case 15:
		return "MX"
	case 16:
		return "TXT"
	case 28:
		return "AAAA"
	case 33:
		return "SRV"
	case 35:
		return "NAPTR"
	case 41:
		return "OPT"
	case 43:
		return "DS"
	case 46:
		return "RRSIG"
	case 47:
		return "NSEC"
	case 48:
		return "DNSKEY"
	case 255:
		return "ANY"
	default:
		return strconv.Itoa(int(t))
	}
}

// dnsClass maps a DNS CLASS code to a readable name.
func dnsClass(c uint16) string {
	switch c {
	case 1:
		return "IN (互联网)"
	case 3:
		return "CH (混沌)"
	case 4:
		return "HS (Hesiod)"
	default:
		return fmt.Sprintf("%d", c)
	}
}

// dnsRData renders a resource record's data field according to its type. For
// names (NS/CNAME/PTR/MX/SOA) the target may itself be compressed, so it is
// resolved through the original payload.
func dnsRData(payload []byte, typ uint16, rd []byte, rdStart int, base int) string {
	switch typ {
	case 1: // A
		if len(rd) >= 4 {
			return net.IP(rd[0:4]).String()
		}
	case 28: // AAAA
		if len(rd) >= 16 {
			return net.IP(rd[0:16]).String()
		}
	case 2, 5, 12: // NS / CNAME / PTR
		if n, _, _, ok := dnsName(payload, rdStart); ok {
			return n
		}
	case 6: // SOA — MNAME RNAME SERIAL REFRESH RETRY EXPIRE MINIMUM
		if m, _, mnext, ok := dnsName(payload, rdStart); ok {
			if r, _, rnext, ok2 := dnsName(payload, mnext); ok2 && rnext+20 <= len(payload) {
				serial := binary.BigEndian.Uint32(payload[rnext : rnext+4])
				refresh := binary.BigEndian.Uint32(payload[rnext+4 : rnext+8])
				retry := binary.BigEndian.Uint32(payload[rnext+8 : rnext+12])
				expire := binary.BigEndian.Uint32(payload[rnext+12 : rnext+16])
				minimum := binary.BigEndian.Uint32(payload[rnext+16 : rnext+20])
				return fmt.Sprintf("MNAME %s, RNAME %s, serial %d, refresh %d, retry %d, expire %d, min %d",
					m, r, serial, refresh, retry, expire, minimum)
			}
			return m
		}
	case 15: // MX — preference + exchange name
		if len(rd) >= 3 {
			pref := binary.BigEndian.Uint16(rd[0:2])
			if n, _, _, ok := dnsName(payload, rdStart+2); ok {
				return fmt.Sprintf("%d %s", pref, n)
			}
		}
	case 16: // TXT — 一段或多段长度前缀字符串
		var parts []string
		p := 0
		for p < len(rd) {
			l := int(rd[p])
			if p+1+l > len(rd) {
				break
			}
			parts = append(parts, string(rd[p+1:p+1+l]))
			p += 1 + l
		}
		return strings.Join(parts, "")
	case 33: // SRV — priority weight port target
		if len(rd) >= 7 {
			prio := binary.BigEndian.Uint16(rd[0:2])
			weight := binary.BigEndian.Uint16(rd[2:4])
			port := binary.BigEndian.Uint16(rd[4:6])
			if n, _, _, ok := dnsName(payload, rdStart+6); ok {
				return fmt.Sprintf("prio %d, weight %d, port %d, target %s", prio, weight, port, n)
			}
		}
	}
	return ""
}

// dnsHexSpace renders bytes as uppercase hex, space-separated.
func dnsHexSpace(b []byte) string {
	if len(b) == 0 {
		return "(空)"
	}
	var sb strings.Builder
	for i, v := range b {
		if i > 0 {
			sb.WriteByte(' ')
		}
		fmt.Fprintf(&sb, "%02X", v)
	}
	return sb.String()
}

// decodeHTTP renders the first request/response line (method + URI, or status
// line) so users see the app-layer intent without full dissection.
func decodeHTTP(payload []byte, base int) FieldNode {
	s := printableUntil(payload)
	if s == "" {
		return FieldNode{}
	}
	return FieldNode{
		Name:  "HTTP",
		Offset: base,
		Len:   len(payload),
		Children: []FieldNode{
			fn("请求/响应 (Request/Response)", s, base, len(s)),
		},
	}
}

// printableUntil returns the printable ASCII prefix of b up to the first CR/LF.
func printableUntil(b []byte) string {
	for i := 0; i < len(b); i++ {
		if b[i] == '\r' || b[i] == '\n' || b[i] < 32 {
			return strings.TrimSpace(string(b[:i]))
		}
	}
	return strings.TrimSpace(string(b))
}

// packPort renders a port with its service name when known.
func packPort(port uint16) string {
	if s := serviceName(port); s != "" {
		return fmt.Sprintf("%d (%s)", port, s)
	}
	return strconv.Itoa(int(port))
}
