package importer

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/ys-ll/uniterm/backend/session"
	"golang.org/x/crypto/pbkdf2"
)

type mRemoteNGDocument struct {
	XMLName            xml.Name        `xml:"Connections"`
	EncryptionEngine   string          `xml:"EncryptionEngine,attr"`
	BlockCipherMode    string          `xml:"BlockCipherMode,attr"`
	KdfIterations      string          `xml:"KdfIterations,attr"`
	FullFileEncryption string          `xml:"FullFileEncryption,attr"`
	Nodes              []mRemoteNGNode `xml:"Node"`
}

type mRemoteNGValues struct {
	Username         string `xml:"Username,attr"`
	Domain           string `xml:"Domain,attr"`
	Password         string `xml:"Password,attr"`
	Port             string `xml:"Port,attr"`
	Protocol         string `xml:"Protocol,attr"`
	Description      string `xml:"Descr,attr"`
	ConnectToConsole string `xml:"ConnectToConsole,attr"`
	UseCredSsp       string `xml:"UseCredSsp,attr"`
}

type mRemoteNGNode struct {
	mRemoteNGValues
	Name                     string          `xml:"Name,attr"`
	Type                     string          `xml:"Type,attr"`
	Hostname                 string          `xml:"Hostname,attr"`
	InheritUsername          string          `xml:"InheritUsername,attr"`
	InheritDomain            string          `xml:"InheritDomain,attr"`
	InheritPassword          string          `xml:"InheritPassword,attr"`
	InheritPort              string          `xml:"InheritPort,attr"`
	InheritProtocol          string          `xml:"InheritProtocol,attr"`
	InheritDescription       string          `xml:"InheritDescription,attr"`
	InheritUseConsoleSession string          `xml:"InheritUseConsoleSession,attr"`
	InheritUseCredSsp        string          `xml:"InheritUseCredSsp,attr"`
	Nodes                    []mRemoteNGNode `xml:"Node"`
}

func parseMRemoteNG(data []byte, opts ParseOptions) (*ImportResult, error) {
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	// Before format 2.6, full-file encryption produced bare base64, without XML.
	if len(data) > 0 && data[0] != '<' {
		if _, err := base64.StdEncoding.DecodeString(string(data)); err == nil {
			return nil, errors.New("fully encrypted mRemoteNG connection files are not supported yet")
		}
	}
	var doc mRemoteNGDocument
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&doc); err != nil {
		// XML errors can quote input. Do not include possible credential material.
		return nil, errors.New("invalid mRemoteNG connection XML")
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, errors.New("invalid mRemoteNG connection XML")
		}
		switch t := token.(type) {
		case xml.Comment, xml.ProcInst:
		case xml.CharData:
			if strings.TrimSpace(string(t)) != "" {
				return nil, errors.New("invalid mRemoteNG connection XML")
			}
		default:
			return nil, errors.New("invalid mRemoteNG connection XML")
		}
	}
	if mRemoteNGTrue(doc.FullFileEncryption) {
		return nil, errors.New("fully encrypted mRemoteNG connection files are not supported yet")
	}

	res := &ImportResult{}
	pathMap := map[string]string{}
	password := opts.Password
	if password == "" {
		// RootNodeInfo.DefaultPassword in the official mRemoteNG implementation.
		password = "mR3m"
	}
	iterations, _ := strconv.Atoi(strings.TrimSpace(doc.KdfIterations))
	passwordWarning := ""
	passwords := map[string]string{}
	importPassword := func(encoded string) string {
		if strings.TrimSpace(encoded) == "" {
			return ""
		}
		if decoded, ok := passwords[encoded]; ok {
			return decoded
		}
		var decoded string
		switch {
		case !strings.EqualFold(doc.EncryptionEngine, "AES") || !strings.EqualFold(doc.BlockCipherMode, "GCM"):
			passwordWarning = "mRemoteNG passwords were not imported: only AES/GCM encryption is supported"
		case iterations < 1000 || iterations > 1_000_000:
			// Bound work from file-supplied metadata; mRemoteNG requires at least 1000.
			passwordWarning = "mRemoteNG passwords were not imported: KdfIterations must be between 1000 and 1000000"
		default:
			var err error
			decoded, err = decryptMRemoteNGPassword(encoded, password, iterations)
			if err != nil {
				passwordWarning = "some mRemoteNG passwords could not be decrypted; check the configuration password and file integrity"
			}
		}
		passwords[encoded] = decoded
		return decoded
	}

	var walk func(mRemoteNGNode, []string, mRemoteNGValues)
	walk = func(n mRemoteNGNode, ancestors []string, parent mRemoteNGValues) {
		values := n.effectiveValues(parent)
		if strings.EqualFold(n.Type, "Container") {
			path := append(ancestors, n.Name)
			ensureGroupPath(path, pathMap, &res.Groups, newGroupID)
			for _, child := range n.Nodes {
				walk(child, path, values)
			}
			return
		}
		if n.Type != "" && !strings.EqualFold(n.Type, "Connection") {
			res.Warnings = append(res.Warnings, fmt.Sprintf("skipped mRemoteNG node type %q for %q", n.Type, n.Name))
			return
		}
		var typ string
		switch strings.ToUpper(strings.TrimSpace(values.Protocol)) {
		case "SSH1", "SSH2":
			typ = "ssh"
		case "TELNET":
			typ = "telnet"
		case "RDP":
			typ = "rdp"
		case "VNC":
			typ = "vnc"
		default:
			res.Warnings = append(res.Warnings, fmt.Sprintf("skipped mRemoteNG protocol %q for connection %q", values.Protocol, n.Name))
			return
		}
		port, _ := strconv.Atoi(strings.TrimSpace(values.Port))
		if port < 1 || port > 65535 {
			port = defaultPort(typ)
		}
		conn := session.ConnectionConfig{
			ID: newConnectionID(), Name: n.Name, Host: n.Hostname, Type: typ,
			Port: port, User: values.Username, Remark: values.Description,
			Password: importPassword(values.Password),
			GroupId:  ensureGroupPath(ancestors, pathMap, &res.Groups, newGroupID),
		}
		if typ == "ssh" {
			conn.AuthType = "password"
			if conn.User == "" {
				conn.User = "root"
			}
		}
		if typ == "rdp" {
			conn.RdpDomain = values.Domain
			// RdpProtocol maps these to ConnectToAdministerServer and EnableCredSspSupport,
			// the same ActiveX properties used by uniTerm's RDP implementation.
			conn.RdpAdminSession = mRemoteNGTrue(values.ConnectToConsole)
			conn.RdpEnableNLA = mRemoteNGTrue(values.UseCredSsp)
		}
		res.Connections = append(res.Connections, conn)
	}
	for _, node := range doc.Nodes {
		walk(node, nil, mRemoteNGValues{})
	}
	if passwordWarning != "" {
		res.Warnings = append(res.Warnings, passwordWarning)
	}
	return res, nil
}

func mRemoteNGTrue(value string) bool {
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

func (n mRemoteNGNode) effectiveValues(parent mRemoteNGValues) mRemoteNGValues {
	inherit := func(flag, own, inherited string) string {
		if mRemoteNGTrue(flag) {
			return inherited
		}
		return own
	}
	return mRemoteNGValues{
		Username:         inherit(n.InheritUsername, n.Username, parent.Username),
		Domain:           inherit(n.InheritDomain, n.Domain, parent.Domain),
		Password:         inherit(n.InheritPassword, n.Password, parent.Password),
		Port:             inherit(n.InheritPort, n.Port, parent.Port),
		Protocol:         inherit(n.InheritProtocol, n.Protocol, parent.Protocol),
		Description:      inherit(n.InheritDescription, n.Description, parent.Description),
		ConnectToConsole: inherit(n.InheritUseConsoleSession, n.ConnectToConsole, parent.ConnectToConsole),
		UseCredSsp:       inherit(n.InheritUseCredSsp, n.UseCredSsp, parent.UseCredSsp),
	}
}

// decryptMRemoteNGPassword follows AeadCryptographyProvider and Pkcs5S2KeyGenerator:
// https://github.com/mRemoteNG/mRemoteNG/tree/06ac7036f01172490a96bc5871a3ad7e6380c311/mRemoteNG/Security
// The base64 payload is salt(16) || nonce(16) || ciphertext || tag(16).
// PBKDF2-HMAC-SHA1 derives an AES-256 key; the salt is authenticated as AAD.
func decryptMRemoteNGPassword(encoded, password string, iterations int) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil || len(payload) < 48 {
		return "", errors.New("invalid mRemoteNG password payload")
	}
	// BouncyCastle's Pkcs5PasswordToBytes truncates each .NET UTF-16 char to a byte.
	chars := utf16.Encode([]rune(password))
	passwordBytes := make([]byte, len(chars))
	for i, char := range chars {
		passwordBytes[i] = byte(char)
	}
	key := pbkdf2.Key(passwordBytes, payload[:16], iterations, 32, sha1.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("invalid mRemoteNG encryption key")
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, 16)
	if err != nil {
		return "", errors.New("invalid mRemoteNG encryption parameters")
	}
	plain, err := gcm.Open(nil, payload[16:32], payload[32:], payload[:16])
	if err != nil {
		return "", errors.New("mRemoteNG password authentication failed")
	}
	return string(plain), nil
}
