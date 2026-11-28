package session

// Remote-desktop connection probes for the "Test connection" button. Each
// probe goes as far up the protocol stack as the backend can go WITHOUT the
// frontend WASM engines:
//
//   - rdp:  real X.224 negotiation + server TLS leg via the same code path
//     (and TLS strategy cache) the session proxy uses — establishUpstream.
//     Credentials can NOT be verified here: CredSSP (NLA) runs inside the
//     WASM client. The probe reports the security protocol the server
//     selected instead.
//   - vnc:  full RFB handshake (3.3 / 3.7 / 3.8) including VNC password
//     authentication (DES challenge-response, stdlib only). Servers that
//     only offer unsupported security types (VeNCrypt, Tight, ...) degrade
//     to a banner check.
//   - spice: SpiceLinkMess/Reply link handshake on the plaintext port plus
//     the SPICE ticket (password) authentication round-trip. SASL-only
//     servers degrade to the link handshake result.
//   - x11-desktop: full SSH probe (same dial as a terminal session) plus a
//     remote `command -v` check for the desktop command, plus the local X
//     server check (non-Windows — Windows spawns its own VcXsrv).

import (
	"context"
	"crypto/des"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// probeTimeout bounds one protocol probe. Long enough for a slow proxy chain,
// short enough that a failed test never feels stuck.
const probeTimeout = 10 * time.Second

// ── SSH dial shared by the ssh-family probes ────────────────────────────────

// dialSSHProbe connects and authenticates exactly like the terminal session's
// first hop (proxy honored, algorithm sets resolved, auth retry). Non-
// interactive: keyboard-interactive challenges are rejected so a test can
// never block waiting for typed input.
func dialSSHProbe(config ConnectionConfig) (*ssh.Client, error) {
	addr := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	kb := func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		return nil, fmt.Errorf("interactive auth not allowed during connection test")
	}
	newConfig := func(challenge ssh.KeyboardInteractiveChallenge) sshClientConfigFactory {
		return func() (*ssh.ClientConfig, func(), error) {
			authMethods, cleanup, err := makeSSHAuthMethodsForAttempt(config, challenge)
			if err != nil {
				return nil, nil, err
			}
			return &ssh.ClientConfig{User: config.User, Auth: authMethods, Timeout: 15 * time.Second, HostKeyCallback: ssh.InsecureIgnoreHostKey()}, cleanup, nil
		}
	}
	var keyboardConfig sshClientConfigFactory
	if config.AuthType != "kerberos" {
		keyboardConfig = newConfig(kb)
	}
	sets, err := resolveSSHDialAlgorithms(config.SSHAlgorithms)
	if err != nil {
		return nil, err
	}
	return dialSSHWithAuthRetry(addr, sets, newConfig(nil), keyboardConfig, func() (net.Conn, error) {
		return dialFirstHop(addr, config.Proxy)
	})
}

// ── RDP ─────────────────────────────────────────────────────────────────────

// rdpNegProtocols builds the requestedProtocols bitmask of the X.224
// negotiation request, mirroring what the real session offers: TLS and
// standard RDP security always, CredSSP when the security mode offers it.
// "unencrypted" requests standard RDP security alone (the session disables
// TLS outright in that mode); "tls" never offers CredSSP.
func rdpNegProtocols(config ConnectionConfig) uint32 {
	switch config.RdpSecurityMode {
	case "unencrypted":
		return x224ProtoRDP
	case "tls":
		return x224ProtoSSL
	}
	p := x224ProtoRDP | x224ProtoSSL
	if config.SecurityOffersCredSSP() {
		p |= x224ProtoHybrid | x224ProtoHybridEx
	}
	return p
}

// buildX224ConnectionRequest encodes a TPKT-framed X.224 Connection Request
// carrying an RDP NEG_REQ requesting protocols. Layout matches the frames the
// proxy's rewriteX224SslOnly / parseX224SelectedProtocol helpers expect:
// [TPKT 4][LI 14][CR 0xE0][dst 2][src 2][class 0][type 0x01][flags 0]
// [len 8][protocols u32 LE] = 19 bytes.
func buildX224ConnectionRequest(protocols uint32) []byte {
	pdu := make([]byte, 19)
	pdu[0] = 3     // TPKT version
	binary.BigEndian.PutUint16(pdu[2:], 19) // TPKT length: the whole frame
	pdu[4] = 14    // X.224 length indicator
	pdu[5] = 0xE0  // Connection Request
	pdu[11] = 0x01 // NEG_REQ
	pdu[13] = 8    // negative length
	binary.LittleEndian.PutUint32(pdu[15:], protocols)
	return pdu
}

func rdpNegSecurityName(p uint32) string {
	switch p {
	case x224ProtoRDP:
		return "standard RDP security"
	case x224ProtoSSL:
		return "TLS"
	case x224ProtoHybrid, x224ProtoHybridEx:
		return "NLA (CredSSP)"
	}
	return fmt.Sprintf("protocol 0x%x", p)
}

// probeRDP runs the real X.224 negotiation against the RDP server and, when
// TLS was selected, completes the server TLS leg with the same mstsc-shaped
// strategies (and strategy cache) the session proxy uses — so a passing test
// means the exact connect path works up to the credential step. Credentials
// themselves are verified only by the WASM engine: NLA is reported as
// "selected", not validated.
func probeRDP(config ConnectionConfig) (string, error) {
	port := config.Port
	if port <= 0 {
		port = DefaultRDPPort
	}
	target := net.JoinHostPort(config.Host, strconv.Itoa(port))
	x224Req := buildX224ConnectionRequest(rdpNegProtocols(config))

	ctx, cancel := context.WithTimeout(context.Background(), probeTimeout)
	defer cancel()
	// establishUpstream dials (honoring the proxy chain), replays the request,
	// reads the Connection Confirm and terminates the TLS leg. cacheKey is the
	// plain target, like the proxy — a successful test warms the strategy
	// cache the session will reuse.
	conn, selected, _, resp, err := establishUpstream(ctx, target, target, config.Proxy, x224Req)
	if err != nil {
		return "", fmt.Errorf("rdp: %w", err)
	}
	conn.Close()
	if selected == x224ProtoFailure {
		code, _ := parseX224FailureCode(resp)
		return "", fmt.Errorf("rdp: %s rejected the negotiation (failure code %d)", target, code)
	}
	security := rdpNegSecurityName(selected)
	if selected == x224ProtoHybrid || selected == x224ProtoHybridEx {
		// NLA credentials are checked by the client engine, not here.
		return fmt.Sprintf("rdp: handshake OK with %s (%s; login not verified)", target, security), nil
	}
	return fmt.Sprintf("rdp: handshake OK with %s (%s)", target, security), nil
}

// ── VNC ─────────────────────────────────────────────────────────────────────

// vncAuthResponse computes the standard VNC challenge-response: the password
// (max 8 chars, zero padded) with each key byte's bits reversed is the DES
// key, and the challenge is encrypted in ECB mode.
func vncAuthResponse(password string, challenge []byte) []byte {
	key := make([]byte, 8)
	copy(key, password)
	for i := range key {
		b := key[i]
		b = (b&0xF0)>>4 | (b&0x0F)<<4
		b = (b&0xCC)>>2 | (b&0x33)<<2
		b = (b&0xAA)>>1 | (b&0x55)<<1
		key[i] = b
	}
	cipher, err := des.NewCipher(key)
	if err != nil {
		return nil // unreachable: the key is always 8 bytes
	}
	out := make([]byte, 16)
	cipher.Encrypt(out[0:8], challenge[0:8])
	cipher.Encrypt(out[8:16], challenge[8:16])
	return out
}

// probeVNC performs the full RFB handshake. With a stored password and a
// server offering standard VNC authentication the credentials are verified
// for real; without one the probe reports what the server demands.
func probeVNC(config ConnectionConfig) (string, error) {
	port := config.Port
	if port <= 0 {
		port = 5900
	} else if port < 100 {
		// libvirt display port format: :1 -> 5901, :23 -> 5923
		port += 5900
	}
	addr := net.JoinHostPort(config.Host, strconv.Itoa(port))
	conn, err := dialFirstHop(addr, config.Proxy)
	if err != nil {
		return "", fmt.Errorf("vnc: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(probeTimeout))

	// Server greets first: "RFB mmm.nnn\n".
	banner := make([]byte, 12)
	if _, err := readFull(conn, banner); err != nil {
		return "", fmt.Errorf("vnc: reading banner from %s: %w", addr, err)
	}
	if !strings.HasPrefix(string(banner), "RFB ") {
		return "", fmt.Errorf("vnc: %s is not a VNC server (bad banner %q)", addr, strings.TrimRight(string(banner), "\n"))
	}
	major, _ := strconv.Atoi(string(banner[4:7]))
	minor, _ := strconv.Atoi(string(banner[8:11]))

	// Reply with the highest version we share. Versions above 3.8 (e.g.
	// RealVNC's 003.889) speak 3.8 semantics; reply 003.008 to them.
	ourMinor := minor
	switch {
	case major > 3 || (major == 3 && minor > 8) || minor >= 100:
		ourMinor = 8
	case minor < 3:
		ourMinor = 3
	}
	reply := fmt.Sprintf("RFB %03d.%03d\n", 3, ourMinor)
	if _, err := conn.Write([]byte(reply)); err != nil {
		return "", fmt.Errorf("vnc: %w", err)
	}

	authenticated := false
	needAuth := func(secType byte) (bool, error) {
		switch secType {
		case 1: // None
			return false, nil
		case 2: // VNC password authentication
			if config.Password == "" {
				return false, fmt.Errorf("vnc: %s requires VNC password authentication (set a password to verify it)", addr)
			}
			challenge := make([]byte, 16)
			if _, err := readFull(conn, challenge); err != nil {
				return false, fmt.Errorf("vnc: reading challenge: %w", err)
			}
			if _, err := conn.Write(vncAuthResponse(config.Password, challenge)); err != nil {
				return false, fmt.Errorf("vnc: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("vnc: %s offers no supported security type (needs VeNCrypt/Tight or similar)", addr)
	}

	switch {
	case major > 3 || ourMinor == 8:
		// 3.8: server lists security types, client picks, server answers
		// with a u32 SecurityResult for every type.
		n, err := readU8(conn)
		if err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		types := make([]byte, n)
		if _, err := readFull(conn, types); err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		secType := pickVNCSecurityType(types, config.Password)
		if secType == 0 {
			return "", fmt.Errorf("vnc: %s offers no supported security type (%v)", addr, types)
		}
		if _, err := conn.Write([]byte{secType}); err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		authenticated, err = needAuth(secType)
		if err != nil {
			return "", err
		}
		result, err := readU32(conn)
		if err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		if result != 0 {
			return "", fmt.Errorf("vnc: %s rejected the security handshake (result %d%s)", addr, result, vncResultHint(result))
		}
	case ourMinor == 7:
		// 3.7: same as 3.8 but no SecurityResult for None; the result only
		// follows the VNC challenge-response.
		n, err := readU8(conn)
		if err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		types := make([]byte, n)
		if _, err := readFull(conn, types); err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		secType := pickVNCSecurityType(types, config.Password)
		if secType == 0 {
			return "", fmt.Errorf("vnc: %s offers no supported security type (%v)", addr, types)
		}
		if _, err := conn.Write([]byte{secType}); err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		if secType == 2 {
			authenticated, err = needAuth(2)
			if err != nil {
				return "", err
			}
			result, err := readU32(conn)
			if err != nil {
				return "", fmt.Errorf("vnc: %w", err)
			}
			if result != 0 {
				return "", fmt.Errorf("vnc: authentication failed on %s (wrong password?)", addr)
			}
		}
	default:
		// 3.3: the server picks the security type itself (u32).
		sec, err := readU32(conn)
		if err != nil {
			return "", fmt.Errorf("vnc: %w", err)
		}
		if sec > 2 {
			return "", fmt.Errorf("vnc: %s selected unsupported security type %d", addr, sec)
		}
		authenticated, err = needAuth(byte(sec))
		if err != nil {
			return "", err
		}
		if sec == 2 {
			result, err := readU32(conn)
			if err != nil {
				return "", fmt.Errorf("vnc: %w", err)
			}
			if result != 0 {
				return "", fmt.Errorf("vnc: authentication failed on %s (wrong password?)", addr)
			}
		}
	}

	if authenticated {
		return fmt.Sprintf("vnc: RFB %d.%d handshake OK with %s, password accepted", major, minor, addr), nil
	}
	return fmt.Sprintf("vnc: RFB %d.%d handshake OK with %s (no authentication required)", major, minor, addr), nil
}

// pickVNCSecurityType chooses the security type the probe can handle: None
// when offered, otherwise VNC authentication when a password is available.
func pickVNCSecurityType(types []byte, password string) byte {
	has := func(t byte) bool {
		for _, v := range types {
			if v == t {
				return true
			}
		}
		return false
	}
	if has(1) {
		return 1
	}
	if has(2) && password != "" {
		return 2
	}
	return 0
}

// vncResultHint annotates a 3.8 SecurityResult with the special reasons.
func vncResultHint(result uint32) string {
	switch result {
	case 1:
		return ": authentication failed"
	case 2:
		return ": too many attempts"
	case 3:
		return ": server is busy"
	}
	return ""
}

// ── SPICE ───────────────────────────────────────────────────────────────────

// Spice link protocol constants (spice_protocol.h / spice_enums.h).
const (
	spiceLinkMagic             = "REDQ"
	spiceMajorVersion          = 2
	spiceMinorVersion          = 2
	spiceChannelMain           = 1
	spiceAuthMechSpice         = 1      // SPICE_LINK_AUTH_MECHANISM_SPICE
	spiceCommonCapSpice        = 1 << 0 // SPICE_COMMON_CAP_AUTH_SPICE
	spiceCommonCapSASL         = 1 << 1 // SPICE_COMMON_CAP_AUTH_SASL
	spiceLinkErrOK             = 0
	spiceLinkErrNeedSecure     = 9
	spiceLinkErrNeedUnsecure   = 8
	spiceLinkErrBadPassword    = 10
	spiceLinkErrPermissionDeny = 11
)

// spiceProbeLinkMess encodes the SpiceLinkMess the probe sends: main channel,
// one common-cap word advertising AUTH_SPICE.
func spiceProbeLinkMess() []byte {
	// Packed wire layout: [connection_id 4][channel_type 1][channel_id 1]
	// [num_common_caps 4][num_channel_caps 4][caps 4] = 18 bytes; the size
	// field covers everything after the fixed header, caps included.
	body := make([]byte, 18)
	body[4] = spiceChannelMain
	binary.LittleEndian.PutUint32(body[6:], 1)  // one common cap word
	binary.LittleEndian.PutUint32(body[10:], 0) // no channel caps
	binary.LittleEndian.PutUint32(body[14:], spiceCommonCapSpice)
	msg := make([]byte, 0, 16+len(body))
	msg = append(msg, spiceLinkMagic...)
	msg = binary.LittleEndian.AppendUint32(msg, spiceMajorVersion)
	msg = binary.LittleEndian.AppendUint32(msg, spiceMinorVersion)
	msg = binary.LittleEndian.AppendUint32(msg, uint32(len(body)))
	return append(msg, body...)
}

// probeSPICE performs the plaintext-port link handshake and, when the server
// supports SPICE ticket auth and a password is stored, verifies the
// credentials. SASL-only or TLS-only servers degrade with an explicit reason.
func probeSPICE(config ConnectionConfig) (string, error) {
	port := config.Port
	if port <= 0 {
		port = 5900
	} else if port < 100 {
		// libvirt display port format, same rule as the SPICE session.
		port += 5900
	}
	addr := net.JoinHostPort(config.Host, strconv.Itoa(port))
	conn, err := dialFirstHop(addr, config.Proxy)
	if err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(probeTimeout))

	// SpiceLinkMess (encoded by spiceProbeLinkMess, tested byte-exact).
	if _, err := conn.Write(spiceProbeLinkMess()); err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}

	// SpiceLinkReply: [magic 4][major 4][minor 4][size 4][error 4]
	// [num_common_caps 4][num_channel_caps 4][caps...].
	head := make([]byte, 28)
	if _, err := readFull(conn, head); err != nil {
		return "", fmt.Errorf("spice: reading link reply from %s: %w", addr, err)
	}
	if string(head[0:4]) != spiceLinkMagic {
		return "", fmt.Errorf("spice: %s is not a SPICE server (bad reply magic %q)", addr, string(head[0:4]))
	}
	linkErr := binary.LittleEndian.Uint32(head[16:20])
	switch linkErr {
	case spiceLinkErrOK:
	case spiceLinkErrNeedSecure:
		return "", fmt.Errorf("spice: %s requires a TLS connection (secure port)", addr)
	case spiceLinkErrNeedUnsecure:
		return "", fmt.Errorf("spice: %s requires the plaintext (insecure) port", addr)
	default:
		return "", fmt.Errorf("spice: %s rejected the link handshake (error %d)", addr, linkErr)
	}
	major := binary.LittleEndian.Uint32(head[4:8])
	minor := binary.LittleEndian.Uint32(head[8:12])

	// Auth: only the SPICE ticket mechanism is supported here; SASL-only
	// servers degrade to a validated link handshake.
	commonCapWords := int(binary.LittleEndian.Uint32(head[20:24]))
	channelCapWords := int(binary.LittleEndian.Uint32(head[24:28]))
	capsBytes := make([]byte, (commonCapWords+channelCapWords)*4)
	if _, err := readFull(conn, capsBytes); err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}
	var commonCaps uint32
	if commonCapWords > 0 {
		commonCaps = binary.LittleEndian.Uint32(capsBytes)
	}
	if commonCaps&spiceCommonCapSpice == 0 {
		if commonCaps&spiceCommonCapSASL != 0 {
			return fmt.Sprintf("spice: link handshake OK with %s (protocol %d.%d, SASL auth not verified)", addr, major, minor), nil
		}
		return fmt.Sprintf("spice: link handshake OK with %s (protocol %d.%d, authentication not verified)", addr, major, minor), nil
	}
	if config.Password == "" {
		return fmt.Sprintf("spice: link handshake OK with %s (protocol %d.%d; set a password to verify the ticket)", addr, major, minor), nil
	}

	auth := make([]byte, 4)
	binary.LittleEndian.PutUint32(auth, spiceAuthMechSpice)
	if _, err := conn.Write(auth); err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}
	// Ticket: password bytes + NUL terminator.
	if _, err := conn.Write(append([]byte(config.Password), 0)); err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}
	result, err := readU32(conn)
	if err != nil {
		return "", fmt.Errorf("spice: %w", err)
	}
	switch result {
	case spiceLinkErrOK:
		return fmt.Sprintf("spice: link handshake OK with %s (protocol %d.%d), password accepted", addr, major, minor), nil
	case spiceLinkErrBadPassword:
		return "", fmt.Errorf("spice: authentication failed on %s (wrong password)", addr)
	case spiceLinkErrPermissionDeny:
		return "", fmt.Errorf("spice: permission denied on %s", addr)
	}
	return "", fmt.Errorf("spice: %s rejected the ticket (error %d)", addr, result)
}

// ── X11 desktop ─────────────────────────────────────────────────────────────

// x11DesktopProbeBins maps the preset desktop types to the desktop binary the
// session wrapper execs remotely. dbus-run-session (the wrapper) is checked
// alongside: a missing wrapper fails the session exactly like a missing DE.
var x11DesktopProbeBins = map[string]string{
	"gnome":    "gnome-session",
	"kde":      "startkde",
	"xfce":     "startxfce4",
	"mate":     "mate-session",
	"cinnamon": "cinnamon-session",
	"openbox":  "openbox-session",
}

// shQuote wraps s in single quotes for a POSIX shell command line.
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// readU8 / readU32 read fixed-width little-endian fields used by the RFB and
// Spice link protocols.
func readU8(conn net.Conn) (byte, error) {
	var b [1]byte
	if _, err := readFull(conn, b[:]); err != nil {
		return 0, err
	}
	return b[0], nil
}

func readU32(conn net.Conn) (uint32, error) {
	var b [4]byte
	if _, err := readFull(conn, b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

// x11DesktopCommandBins lists the remote binaries the desktop session will
// need: the dbus wrapper plus the DE for presets, or the custom command's
// first word (skipping leading VAR=value environment assignments).
func x11DesktopCommandBins(config ConnectionConfig) ([]string, error) {
	if config.X11DesktopDesktopType == "custom" {
		for _, f := range strings.Fields(config.X11DesktopCustomCmd) {
			if !strings.Contains(f, "=") {
				return []string{f}, nil
			}
		}
		return nil, fmt.Errorf("x11-desktop: custom command has no executable")
	}
	bin, ok := x11DesktopProbeBins[config.X11DesktopDesktopType]
	if !ok {
		return nil, fmt.Errorf("x11-desktop: unknown desktop type %q", config.X11DesktopDesktopType)
	}
	return []string{"dbus-run-session", bin}, nil
}

// probeX11Desktop validates the full path an X11 desktop session takes:
// SSH authentication (the session's dial), the remote desktop binaries, and
// — except on Windows, which spawns a dedicated VcXsrv per session — the
// local X server the remote clients will render on.
func probeX11Desktop(config ConnectionConfig) (string, error) {
	client, err := dialSSHProbe(config)
	if err != nil {
		return "", fmt.Errorf("x11-desktop: %w", err)
	}
	defer client.Close()

	if _, err := resolveDesktopCommand(config); err != nil {
		return "", err
	}
	bins, err := x11DesktopCommandBins(config)
	if err != nil {
		return "", err
	}
	sess, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("x11-desktop: new session: %w", err)
	}
	for _, bin := range bins {
		if _, err := sess.Output("command -v " + shQuote(bin) + " >/dev/null 2>&1"); err != nil {
			sess.Close()
			return "", fmt.Errorf("x11-desktop: %q not found on the remote host", bin)
		}
	}
	sess.Close()

	// Local render target. Windows is exempt: the session spawns (and tears
	// down) its own VcXsrv per desktop.
	if runtime.GOOS != "windows" {
		display := os.Getenv("DISPLAY")
		if display == "" {
			return "", fmt.Errorf("x11-desktop: local $DISPLAY is not set (start the local X server first)")
		}
		if _, derr := DialLocalX(display); derr != nil {
			return "", fmt.Errorf("x11-desktop: local X server unreachable: %w", derr)
		}
	}

	return fmt.Sprintf("x11-desktop: ssh %s@%s OK, %s available on the remote host", config.User, config.Host, strings.Join(bins, ", ")), nil
}
