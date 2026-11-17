package importer

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/ys-ll/uniterm/backend/session"
)

// FormatRDM is Devolutions Remote Desktop Manager's .rdm export
// (File > Export > Export entries as .rdm).
const FormatRDM = "rdm"

// rdmInheritedCredentialID is the well-known CredentialConnectionID that
// selects "Inherited" credentials: the entry uses its parent folder's.
const rdmInheritedCredentialID = "1310cf82-6fab-4b7a-9eea-3e2e451ca2cf"

type rdmDocument struct {
	XMLName     xml.Name   `xml:"RDMExport"`
	Connections []rdmEntry `xml:"Connections>Connection"`
}

// rdmCreds is the credential block shape shared by GroupDetails, Credentials,
// RDP and Terminal (which spells it Username instead of UserName).
type rdmCreds struct {
	UserName     string `xml:"UserName"`
	Username     string `xml:"Username"`
	Domain       string `xml:"Domain"`
	Password     string `xml:"Password"`
	SafePassword string `xml:"SafePassword"`
}

func (c *rdmCreds) user() string {
	if c == nil {
		return ""
	}
	if u := strings.TrimSpace(c.UserName); u != "" {
		return u
	}
	return strings.TrimSpace(c.Username)
}

type rdmEntry struct {
	ConnectionType         string `xml:"ConnectionType"`
	Group                  string `xml:"Group"`
	ID                     string `xml:"ID"`
	Name                   string `xml:"Name"`
	Description            string `xml:"Description"`
	URL                    string `xml:"Url"`
	CredentialConnectionID string `xml:"CredentialConnectionID"`

	GroupDetails *struct {
		rdmCreds
		GroupType string `xml:"GroupType"`
	} `xml:"GroupDetails"`
	Credentials *struct {
		rdmCreds
		CredentialType     string `xml:"CredentialType"`
		PrivateKeyType     string `xml:"PrivateKeyType"`
		PrivateKeyData     string `xml:"PrivateKeyData"`
		SafePrivateKeyData string `xml:"SafePrivateKeyData"`
		OverrideUsername   string `xml:"PrivateKeyOverrideUsername"`
	} `xml:"Credentials"`
	RDP *struct {
		rdmCreds
		Port string `xml:"Port"`
	} `xml:"RDP"`
	Terminal *struct {
		rdmCreds
		Host             string `xml:"Host"`
		HostPort         string `xml:"HostPort"`
		UseSSHGateway    string `xml:"UseSSHGateway"`
		LinkedSSHGateway string `xml:"LinkedSSHGateway"`
		ProxyType        string `xml:"ProxyType"`
		PortForwards     *struct {
			Items []struct{} `xml:"PortForward"`
		} `xml:"PortForwards"`
	} `xml:"Terminal"`
	VPN *struct {
		Application string `xml:"Application"`
		Enabled     string `xml:"Enabled"`
		Gateways    []struct {
			Host     string `xml:"Host"`
			Port     string `xml:"Port"`
			Username string `xml:"Username"`
		} `xml:"SSHGateways>SSHGateway"`
	} `xml:"VPN"`
}

// rdmCred is a credential set resolved for one connection.
type rdmCred struct {
	user, domain, password, key string
}

// parseRDM maps an RDM export onto uniterm groups and connections.
//
// Folders come from each entry's backslash-separated Group path. Credentials
// are copied inline: an entry's own fields, a linked Credential entry, or for
// "Inherited" the nearest ancestor folders that define them. RDM
// exports protect secrets as SafePassword / SafePrivateKeyData with an
// undocumented scheme, so only plaintext secrets are imported and the rest
// are reported in a warning.
func parseRDM(data []byte) (*ImportResult, error) {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	var doc rdmDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		// XML errors can quote input. Do not include possible credential material.
		return nil, errors.New("invalid Remote Desktop Manager export (.rdm)")
	}

	res := &ImportResult{}
	pathMap := map[string]string{}
	byID := map[string]*rdmEntry{}
	folders := map[string]*rdmEntry{}
	for i := range doc.Connections {
		e := &doc.Connections[i]
		byID[strings.ToLower(strings.TrimSpace(e.ID))] = e
		if strings.EqualFold(e.ConnectionType, "Group") {
			folders[rdmPathKey(rdmSplitPath(e.Group))] = e
		}
	}

	// encryptedHit records that resolving the current connection met an
	// encrypted secret; encrypted counts connections left without one.
	encrypted, encryptedHit := 0, false
	secret := func(plain, safe string) string {
		if plain != "" {
			return plain
		}
		if strings.TrimSpace(safe) != "" {
			encryptedHit = true
		}
		return ""
	}
	skipped := map[string]int{}
	var notes []string
	notesSeen := map[string]bool{}
	note := func(s string) {
		if !notesSeen[s] {
			notesSeen[s] = true
			notes = append(notes, s)
		}
	}

	// linked resolves a Credential entry referenced by CredentialConnectionID.
	linked := func(id string) (rdmCred, bool) {
		e := byID[strings.ToLower(strings.TrimSpace(id))]
		if e == nil || !strings.EqualFold(e.ConnectionType, "Credential") || e.Credentials == nil {
			return rdmCred{}, false
		}
		c := e.Credentials
		out := rdmCred{user: c.user(), domain: strings.TrimSpace(c.Domain)}
		if strings.EqualFold(c.CredentialType, "PrivateKey") {
			if out.user == "" {
				out.user = strings.TrimSpace(c.OverrideUsername)
			}
			if strings.EqualFold(c.PrivateKeyType, "Data") || c.PrivateKeyType == "" {
				out.key = secret(c.PrivateKeyData, c.SafePrivateKeyData)
			} else {
				note(fmt.Sprintf("RDM private key %q is not embedded in the export and was not imported", e.Name))
			}
		}
		out.password = secret(c.Password, c.SafePassword)
		return out, true
	}

	// inherited walks folders from the entry's own folder towards the root:
	// the account (username and secret) comes from the nearest folder that
	// names a user, the domain from the nearest folder that sets one.
	inherited := func(path []string) rdmCred {
		var out rdmCred
		for i := len(path); i > 0; i-- {
			f := folders[rdmPathKey(path[:i])]
			if f == nil {
				continue
			}
			var c rdmCred
			cid := strings.ToLower(strings.TrimSpace(f.CredentialConnectionID))
			if cid != "" && cid != rdmInheritedCredentialID {
				c, _ = linked(cid)
			} else if f.GroupDetails != nil {
				g := f.GroupDetails
				c = rdmCred{user: g.user(), domain: strings.TrimSpace(g.Domain), password: secret(g.Password, g.SafePassword)}
			}
			// The secret belongs to the account, so it comes from the same
			// folder as the username; a folder may override only the domain.
			if out.user == "" && c.user != "" {
				out.user, out.password, out.key = c.user, c.password, c.key
			}
			if out.domain == "" {
				out.domain = c.domain
			}
			if out.user != "" && out.domain != "" {
				break
			}
		}
		return out
	}

	// resolve picks the credential source selected by an entry.
	resolve := func(e *rdmEntry, path []string, own *rdmCreds) rdmCred {
		cid := strings.ToLower(strings.TrimSpace(e.CredentialConnectionID))
		switch {
		case cid == rdmInheritedCredentialID:
			return inherited(path)
		case cid != "":
			if c, ok := linked(cid); ok {
				return c
			}
			note(fmt.Sprintf("RDM entry %q uses a credential source that is not in the export; its credentials were not imported", e.Name))
			return rdmCred{}
		case own != nil:
			return rdmCred{user: own.user(), domain: strings.TrimSpace(own.Domain), password: secret(own.Password, own.SafePassword)}
		}
		return rdmCred{}
	}

	// Folders: every Group entry (except credential-only folders) plus any
	// path a connection lives in. Sorted so parents precede children.
	var folderPaths [][]string
	for _, e := range folders {
		if e.GroupDetails != nil && strings.EqualFold(e.GroupDetails.GroupType, "Credentials") {
			continue
		}
		folderPaths = append(folderPaths, rdmSplitPath(e.Group))
	}
	sort.Slice(folderPaths, func(i, j int) bool { return rdmPathKey(folderPaths[i]) < rdmPathKey(folderPaths[j]) })
	for _, p := range folderPaths {
		ensureGroupPath(p, pathMap, &res.Groups, newGroupID)
	}

	// SSH gateways are created first so linked connections can point at them.
	gatewayConn := map[string]string{}
	var conns []session.ConnectionConfig
	for i := range doc.Connections {
		e := &doc.Connections[i]
		if !strings.EqualFold(e.ConnectionType, "VPN") || e.VPN == nil || !strings.EqualFold(e.VPN.Application, "SSHGateway") || len(e.VPN.Gateways) == 0 {
			continue
		}
		gw := e.VPN.Gateways[0]
		if len(e.VPN.Gateways) > 1 {
			note(fmt.Sprintf("RDM gateway %q has several hops; only the first was imported", e.Name))
		}
		path := rdmSplitPath(e.Group)
		c := session.ConnectionConfig{
			ID: newConnectionID(), Name: e.Name, Remark: e.Description, Type: "ssh",
			Host: strings.TrimSpace(gw.Host), Port: rdmPort(gw.Port, "ssh"),
			User: strings.TrimSpace(gw.Username), AuthType: "password",
			GroupId: ensureGroupPath(path, pathMap, &res.Groups, newGroupID),
		}
		if c.User == "" {
			c.User = "root"
		}
		gatewayConn[strings.ToLower(strings.TrimSpace(e.ID))] = c.ID
		conns = append(conns, c)
	}

	for i := range doc.Connections {
		e := &doc.Connections[i]
		path := rdmSplitPath(e.Group)
		encryptedHit = false
		kind := strings.ToLower(strings.TrimSpace(e.ConnectionType))
		switch kind {
		case "group", "credential":
			continue
		case "vpn":
			if _, ok := gatewayConn[strings.ToLower(strings.TrimSpace(e.ID))]; ok {
				continue
			}
		}

		var c session.ConnectionConfig
		switch kind {
		case "sshshell", "sshtunnel", "telnet":
			t := e.Terminal
			if t == nil {
				skipped[e.ConnectionType]++
				continue
			}
			typ := "ssh"
			if kind == "telnet" {
				typ = "telnet"
			}
			cred := resolve(e, path, &t.rdmCreds)
			c = session.ConnectionConfig{
				Type: typ, Host: strings.TrimSpace(t.Host), Port: rdmPort(t.HostPort, typ),
				User: cred.user, Password: cred.password,
			}
			if typ == "ssh" {
				c.AuthType = "password"
				if cred.key != "" {
					c.AuthType, c.KeyContent, c.Password = "keyText", cred.key, ""
				}
				if c.User == "" {
					c.User = "root"
				}
				if rdmTrue(t.UseSSHGateway) {
					if gw, ok := gatewayConn[strings.ToLower(strings.TrimSpace(t.LinkedSSHGateway))]; ok {
						c.TunnelSSHConnID = gw
					} else {
						note(fmt.Sprintf("RDM entry %q uses an SSH gateway that is not in the export; it was imported without a jump host", e.Name))
					}
				}
			}
			if t.ProxyType != "" && !strings.EqualFold(t.ProxyType, "None") {
				note("RDM proxy settings were not imported; assign a proxy to the affected connections in uniterm")
			}
			if kind == "sshtunnel" && t.PortForwards != nil && len(t.PortForwards.Items) > 0 {
				note(fmt.Sprintf("RDM SSH tunnel %q was imported as an SSH connection; its port forwards were not imported", e.Name))
			}
		case "rdpconfigured":
			var own *rdmCreds
			port := ""
			if e.RDP != nil {
				own, port = &e.RDP.rdmCreds, e.RDP.Port
			}
			host, urlPort := rdmSplitHostPort(e.URL)
			if urlPort != "" {
				port = urlPort
			}
			cred := resolve(e, path, own)
			c = session.ConnectionConfig{
				Type: "rdp", Host: host, Port: rdmPort(port, "rdp"),
				User: cred.user, Password: cred.password, RdpDomain: cred.domain,
			}
			if cred.key != "" {
				note(fmt.Sprintf("RDM entry %q links a private key, which RDP cannot use", e.Name))
			}
		default:
			skipped[e.ConnectionType]++
			continue
		}
		if c.Host == "" {
			note(fmt.Sprintf("RDM entry %q has no host and was skipped", e.Name))
			continue
		}
		if e.VPN != nil && rdmTrue(e.VPN.Enabled) {
			note(fmt.Sprintf("RDM entry %q opens a VPN or SSH session first; that dependency was not imported", e.Name))
		}
		c.ID = newConnectionID()
		c.Name = e.Name
		c.Remark = e.Description
		c.GroupId = ensureGroupPath(path, pathMap, &res.Groups, newGroupID)
		conns = append(conns, c)
		if encryptedHit && c.Password == "" && c.KeyContent == "" {
			encrypted++
		}
	}
	res.Connections = conns

	if encrypted > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf(
			"%d RDM connection(s) have passwords or private keys stored encrypted (SafePassword) that were not imported; enter them in uniterm after import", encrypted))
	}
	if len(skipped) > 0 {
		kinds := make([]string, 0, len(skipped))
		for k := range skipped {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		parts := make([]string, len(kinds))
		for i, k := range kinds {
			parts[i] = fmt.Sprintf("%s (%d)", k, skipped[k])
		}
		res.Warnings = append(res.Warnings, "skipped unsupported RDM entry types: "+strings.Join(parts, ", "))
	}
	res.Warnings = append(res.Warnings, notes...)
	return res, nil
}

// rdmSplitPath splits RDM's backslash-separated folder path.
func rdmSplitPath(group string) []string {
	var out []string
	for _, seg := range strings.Split(group, `\`) {
		if seg = strings.TrimSpace(seg); seg != "" {
			out = append(out, seg)
		}
	}
	return out
}

// rdmPathKey matches ensureGroupPath's key format.
func rdmPathKey(path []string) string { return strings.Join(path, "/") }

func rdmTrue(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "true") }

func rdmPort(v, typ string) int {
	port, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || port < 1 || port > 65535 {
		return defaultPort(typ)
	}
	return port
}

// rdmSplitHostPort splits an RDP Url of the form host, host:port or
// [v6]:port. A bare IPv6 address is returned as the host.
func rdmSplitHostPort(u string) (string, string) {
	u = strings.TrimSpace(u)
	if h, p, err := net.SplitHostPort(u); err == nil {
		return h, p
	}
	return strings.Trim(u, "[]"), ""
}
