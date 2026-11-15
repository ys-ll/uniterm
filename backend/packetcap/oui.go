package packetcap

import "net"

// ouiVendor returns the vendor name for a MAC address's first 3 octets (the OUI),
// or "" when unknown. 对标 Wireshark 在 Ethernet/ARP 源目的地址后显示的厂商名。
//
// 这里内置一份常见网卡/设备厂商的 OUI → 厂商名表（精选常用项，未覆盖到全部
// IEEE OUI）。命中项返回如 "Intel"、"TP-Link"，未命中返回空串。
func ouiVendor(mac []byte) string {
	if len(mac) < 3 {
		return ""
	}
	key := uint32(mac[0])<<16 | uint32(mac[1])<<8 | uint32(mac[2])
	if v, ok := ouiVendors[key]; ok && v != "" {
		return v
	}
	return ""
}

// ouiLabel returns the vendor name appended label for the MAC, e.g. `7c:50:79:1a:81:9b (Intel)`.
// Returns the bare MAC when vendor is unknown (对标 Wireshark 未知厂商的显示方式)。
func ouiLabel(mac []byte) string {
	hw := net.HardwareAddr(mac).String()
	if v := ouiVendor(mac); v != "" {
		return hw + " (" + v + ")"
	}
	return hw
}

// ouiVendors maps the 24-bit OUI (from the first 3 MAC octets) to a short vendor
// name. 覆盖常见家用路由器/网卡/运营商终端品牌，便于 IPTV 抓包时识别设备厂商。
var ouiVendors = map[uint32]string{
	// 华为 / 荣耀
	0x4C1FCC: "Huawei", 0x0C1D48: "Huawei", 0x282CE8: "Huawei", 0x485B39: "Huawei",
	0x4C0F6E: "Huawei", 0x7054D2: "Huawei", 0x90CDB6: "Huawei", 0x98E7F5: "Huawei",
	0x38FB14: "Huawei", 0x24695A: "Huawei", 0x70F1A1: "Huawei", 0x98F0AB: "Huawei",
	0xD8C7C8: "Huawei", 0xE02ADD: "Huawei", 0x54263B: "Huawei", 0x1C7B21: "Huawei",
	0x2C1B30: "Huawei", 0x001F6C: "Huawei", 0x10F00C: "Huawei",
	// 小米 / Redmi
	0x78C1C7: "Xiaomi", 0x640980: "Xiaomi", 0x0CE7CE: "Xiaomi", 0x50A62B: "Xiaomi",
	0x8C0CA2: "Xiaomi", 0x342A0E: "Xiaomi", 0xF05B77: "Xiaomi", 0x487B6B: "Xiaomi",
	0x48030B: "Xiaomi", 0x100ACA: "Xiaomi", 0xF8DC7A: "Xiaomi", 0xE4E002: "Xiaomi",
	// TP-Link
	0x147DDA: "TP-Link", 0x1880F5: "TP-Link", 0x1CFA68: "TP-Link", 0x2CAE2B: "TP-Link",
	0x3C1039: "TP-Link", 0x3CE5A6: "TP-Link", 0x4C614B: "TP-Link", 0x50C7BF: "TP-Link",
	0x5C2E59: "TP-Link", 0x647E45: "TP-Link", 0x6C1252: "TP-Link", 0x7CB01F: "TP-Link",
	0x8032E6: "TP-Link", 0x846EB0: "TP-Link", 0x8C210A: "TP-Link", 0xA8A407: "TP-Link",
	0xB0523C: "TP-Link", 0xBC5416: "TP-Link", 0xC05B5C: "TP-Link", 0xD09F1B: "TP-Link",
	0xE0B335: "TP-Link", 0xF0C59C: "TP-Link", 0x100D2F: "TP-Link", 0x54A245: "TP-Link",
	// 中兴 ZTE
	0x6CBE90: "ZTE", 0x8499B1: "ZTE", 0xD0E465: "ZTE", 0x00E0C6: "ZTE",
	0x8825A9: "ZTE", 0x3C5D0E: "ZTE", 0x1CBB10: "ZTE", 0x101163: "ZTE",
	// 爱快 iKuai
	0x089B4B: "iKuaiNetwork", 0x28A08E: "iKuaiNetwork",
	// Intel
	0x7C5079: "Intel", 0x001590: "Intel", 0x001B21: "Intel", 0x00241B: "Intel",
	0x002541: "Intel", 0x006096: "Intel", 0x3C970E: "Intel", 0x4C024B: "Intel",
	0x54EE75: "Intel", 0x88B111: "Intel", 0x8C1645: "Intel", 0xA08869: "Intel",
	0xBC5FF4: "Intel", 0xFC4DD4: "Intel", 0xD83B14: "Intel", 0x2C9972: "Intel",
	0x0013E8: "Intel", 0x002288: "Intel", 0x0021D8: "Intel",
	// VMware / 虚拟化
	0x005056: "VMware",
	// Realtek
	0x0090CC: "Realtek", 0x00E04C: "Realtek", 0x001322: "Realtek", 0x7845C4: "Realtek",
	0xAC9E17: "Realtek", 0xB4A9FC: "Realtek", 0xE45F01: "Realtek", 0xC83A35: "Realtek",
	// Apple
	0x001612: "Apple", 0x00061B: "Apple", 0x00A040: "Apple", 0x00A089: "Apple",
	0x000A95: "Apple", 0x001266: "Apple", 0x001686: "Apple", 0x20C9D0: "Apple",
	0x28CFDA: "Apple", 0x3CD9CE: "Apple", 0x44D884: "Apple", 0x5CF938: "Apple",
	0x68A0F9: "Apple", 0x8C85A0: "Apple", 0xACBC32: "Apple", 0xC0D92F: "Apple",
	0xD4F584: "Apple", 0xF0D1A9: "Apple", 0x049144: "Apple", 0xF4A52D: "Apple",
	// Cisco
	0x00000C: "Cisco", 0x0001C9: "Cisco", 0x00235E: "Cisco", 0x00308E: "Cisco",
	0x00605C: "Cisco", 0x00D0BC: "Cisco", 0x00500B: "Cisco", 0x7C0E7A: "Cisco",
	// Broadcom
	0x0002B3: "Broadcom", 0x0003E9: "Broadcom", 0x001BE5: "Broadcom",
	// Qualcomm/Atheros
	0x00031C: "Atheros", 0x0012F0: "Atheros", 0x001AA4: "Atheros", 0x44DDAC: "Qualcomm",
	0x906FA2: "Qualcomm", 0x9C98D8: "Qualcomm",
	// MediaTek
	0x00117C: "MediaTek", 0x00337B: "MediaTek", 0x7CC8D0: "MediaTek", 0x94D76C: "MediaTek",
	0xF0AED6: "MediaTek", 0x9C3DCF: "MediaTek", 0xE0E31D: "MediaTek",
	// 联想 Lenovo / 惠普 HP / 戴尔 Dell
	0x30227A: "Lenovo", 0x6CD068: "Lenovo", 0x901515: "Lenovo", 0x4025C2: "Lenovo",
	0x3C122E: "HP", 0x90785C: "HP", 0x9C7F0D: "HP", 0x44A6E5: "HP",
	0xF8A5C5: "HP", 0x0056B3: "Dell", 0x00B0D0: "Dell", 0x10B7F6: "Dell",
	0x14BB6E: "Dell", 0x3464A9: "Dell", 0x9C2E78: "Dell",
	// 三星 Samsung
	0x7C1ABF: "Samsung", 0x0CB5D0: "Samsung", 0x8C8D38: "Samsung", 0x483D42: "Samsung",
	0x08125A: "Samsung", 0x0C8905: "Samsung", 0x28C65E: "Samsung", 0x90B3A4: "Samsung",
	// OPPO / vivo / 一加
	0x3CDB02: "OPPO", 0x58B9FA: "OPPO", 0xC4A81D: "OPPO", 0x00B15A: "vivo",
	0xD282BB: "vivo", 0xF05C19: "vivo", 0x14A940: "OnePlus", 0xA01D48: "OnePlus",
	// 其它常见
	0x0016E3: "Quanta", 0x001A8F: "OnLive", 0x0026C7: "Tenda",
}
