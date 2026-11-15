package importer

import (
	"strings"
	"testing"

	"github.com/ys-ll/uniterm/backend/session"
)

const rdmSample = `<?xml version="1.0" encoding="utf-8"?>
<RDMExport>
  <Connections>
    <Connection>
      <ConnectionType>Group</ConnectionType>
      <Group>Windows</Group>
      <ID>a0000000-0000-0000-0000-000000000001</ID>
      <Name>Windows</Name>
      <GroupDetails><Domain>EXAMPLE.LOCAL</Domain><UserName>operator</UserName><Password>folder-secret</Password></GroupDetails>
    </Connection>
    <Connection>
      <ConnectionType>Group</ConnectionType>
      <Group>Windows\Branch</Group>
      <ID>a0000000-0000-0000-0000-000000000002</ID>
      <Name>Branch</Name>
      <GroupDetails><Domain>BRANCH</Domain><GroupType>Domain</GroupType></GroupDetails>
    </Connection>
    <Connection>
      <ConnectionType>Group</ConnectionType>
      <Group>Windows\Linked</Group>
      <ID>a0000000-0000-0000-0000-000000000003</ID>
      <Name>Linked</Name>
      <CredentialConnectionID>c0000000-0000-0000-0000-000000000001</CredentialConnectionID>
      <GroupDetails><GroupType>Domain</GroupType></GroupDetails>
    </Connection>
    <Connection>
      <ConnectionType>Credential</ConnectionType>
      <Group>Windows\Linked</Group>
      <ID>c0000000-0000-0000-0000-000000000001</ID>
      <Name>Domain account</Name>
      <Credentials><Domain>CORP</Domain><UserName>admin</UserName><SafePassword>ENCRYPTED</SafePassword></Credentials>
    </Connection>
    <Connection>
      <ConnectionType>Group</ConnectionType>
      <Group>Keys</Group>
      <ID>a0000000-0000-0000-0000-000000000004</ID>
      <Name>Keys</Name>
      <GroupDetails><GroupType>Credentials</GroupType></GroupDetails>
    </Connection>
    <Connection>
      <ConnectionType>Credential</ConnectionType>
      <Group>Keys</Group>
      <ID>c0000000-0000-0000-0000-000000000002</ID>
      <Name>Deploy key</Name>
      <Credentials><CredentialType>PrivateKey</CredentialType><PrivateKeyType>Data</PrivateKeyType><PrivateKeyOverrideUsername>deploy</PrivateKeyOverrideUsername><PrivateKeyData>-----BEGIN OPENSSH PRIVATE KEY-----</PrivateKeyData></Credentials>
    </Connection>
    <Connection>
      <ConnectionType>RDPConfigured</ConnectionType>
      <Group>Windows\Branch</Group>
      <ID>b0000000-0000-0000-0000-000000000001</ID>
      <Name>inherited</Name>
      <CredentialConnectionID>1310CF82-6FAB-4B7A-9EEA-3E2E451CA2CF</CredentialConnectionID>
      <Url>192.0.2.10:3390</Url>
      <RDP />
    </Connection>
    <Connection>
      <ConnectionType>RDPConfigured</ConnectionType>
      <Group>Windows\Branch</Group>
      <ID>b0000000-0000-0000-0000-000000000002</ID>
      <Name>own</Name>
      <Description>Example server</Description>
      <Url>192.0.2.11</Url>
      <RDP><Domain>.</Domain><UserName>Administrator</UserName><SafePassword>ENCRYPTED</SafePassword></RDP>
    </Connection>
    <Connection>
      <ConnectionType>RDPConfigured</ConnectionType>
      <Group>Windows\Linked</Group>
      <ID>b0000000-0000-0000-0000-000000000003</ID>
      <Name>via folder link</Name>
      <CredentialConnectionID>1310CF82-6FAB-4B7A-9EEA-3E2E451CA2CF</CredentialConnectionID>
      <Url>192.0.2.12</Url>
    </Connection>
    <Connection>
      <ConnectionType>VPN</ConnectionType>
      <Group>Linux</Group>
      <ID>d0000000-0000-0000-0000-000000000001</ID>
      <Name>jump</Name>
      <VPN><Application>SSHGateway</Application><SSHGateways><SSHGateway><Host>gw.example.com</Host><Port>1022</Port><Username>jumper</Username></SSHGateway></SSHGateways></VPN>
    </Connection>
    <Connection>
      <ConnectionType>SSHShell</ConnectionType>
      <Group>Linux\Web</Group>
      <ID>b0000000-0000-0000-0000-000000000004</ID>
      <Name>web01</Name>
      <Terminal><Host>web01.example.com</Host><HostPort>2222</HostPort><Username>ops</Username><Password>plain-secret</Password><UseSSHGateway>true</UseSSHGateway><LinkedSSHGateway>D0000000-0000-0000-0000-000000000001</LinkedSSHGateway></Terminal>
    </Connection>
    <Connection>
      <ConnectionType>SSHShell</ConnectionType>
      <Group>Linux</Group>
      <ID>b0000000-0000-0000-0000-000000000005</ID>
      <Name>keyed</Name>
      <CredentialConnectionID>c0000000-0000-0000-0000-000000000002</CredentialConnectionID>
      <Terminal><Host>192.0.2.20</Host><HostPort>22</HostPort></Terminal>
    </Connection>
    <Connection>
      <ConnectionType>WebBrowser</ConnectionType>
      <Group>Linux</Group>
      <ID>b0000000-0000-0000-0000-000000000006</ID>
      <Name>portal</Name>
      <WebBrowserUrl>https://192.0.2.30</WebBrowserUrl>
    </Connection>
    <Connection>
      <ConnectionType>SSHShell</ConnectionType>
      <ID>b0000000-0000-0000-0000-000000000007</ID>
      <Name>root level</Name>
      <Terminal><Host>192.0.2.40</Host></Terminal>
    </Connection>
  </Connections>
  <DatabaseID>00000000-0000-0000-0000-000000000000</DatabaseID>
  <Version>2</Version>
</RDMExport>`

func TestRDMImport(t *testing.T) {
	res, err := parseRDM([]byte(rdmSample))
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, g := range res.Groups {
		paths = append(paths, groupPathFor(res.Groups, g.ID))
	}
	if got := strings.Join(paths, ","); got != "Windows,Windows/Branch,Windows/Linked,Linux,Linux/Web" {
		t.Fatalf("unexpected folders %q (credential-only folders must be skipped)", got)
	}

	conns := map[string]session.ConnectionConfig{}
	for _, c := range res.Connections {
		if c.ID == "" {
			t.Fatalf("connection %q has no id", c.Name)
		}
		conns[c.Name] = c
	}
	if len(conns) != 7 {
		t.Fatalf("got %d connections", len(conns))
	}
	folder := func(c session.ConnectionConfig) string {
		if c.GroupId == nil {
			return ""
		}
		return groupPathFor(res.Groups, *c.GroupId)
	}

	// Inherited: user and password from Windows, nearer domain from Branch.
	c := conns["inherited"]
	if c.Type != "rdp" || c.Host != "192.0.2.10" || c.Port != 3390 || c.User != "operator" ||
		c.RdpDomain != "BRANCH" || c.Password != "folder-secret" || folder(c) != "Windows/Branch" {
		t.Fatalf("inherited RDP mapped incorrectly: %+v", c)
	}
	c = conns["own"]
	if c.User != "Administrator" || c.RdpDomain != "." || c.Password != "" || c.Port != 3389 || c.Remark != "Example server" {
		t.Fatalf("own-credential RDP mapped incorrectly: %+v", c)
	}
	// Inherited through a folder that links a Credential entry.
	c = conns["via folder link"]
	if c.User != "admin" || c.RdpDomain != "CORP" || c.Password != "" {
		t.Fatalf("folder credential link mapped incorrectly: %+v", c)
	}

	gw := conns["jump"]
	if gw.Type != "ssh" || gw.Host != "gw.example.com" || gw.Port != 1022 || gw.User != "jumper" || folder(gw) != "Linux" {
		t.Fatalf("SSH gateway mapped incorrectly: %+v", gw)
	}
	c = conns["web01"]
	if c.Type != "ssh" || c.Port != 2222 || c.User != "ops" || c.Password != "plain-secret" ||
		c.AuthType != "password" || c.TunnelSSHConnID != gw.ID || folder(c) != "Linux/Web" {
		t.Fatalf("SSH via gateway mapped incorrectly: %+v", c)
	}
	c = conns["keyed"]
	if c.AuthType != "keyText" || c.KeyContent == "" || c.User != "deploy" || c.Password != "" {
		t.Fatalf("linked private key mapped incorrectly: %+v", c)
	}
	c = conns["root level"]
	if c.GroupId != nil || c.Port != 22 || c.User != "root" {
		t.Fatalf("root-level SSH mapped incorrectly: %+v", c)
	}

	warnings := strings.Join(res.Warnings, "\n")
	for _, want := range []string{"2 RDM connection(s) have passwords", "WebBrowser (1)"} {
		if !strings.Contains(warnings, want) {
			t.Fatalf("missing warning %q in %q", want, warnings)
		}
	}
	if strings.Contains(warnings, "ENCRYPTED") {
		t.Fatal("warnings must not echo secret material")
	}
}

func TestRDMMissingCredentialAndInvalidXML(t *testing.T) {
	res, err := parseRDM([]byte(`<RDMExport><Connections><Connection>
	<ConnectionType>RDPConfigured</ConnectionType><Name>srv</Name>
	<CredentialConnectionID>e0000000-0000-0000-0000-000000000001</CredentialConnectionID>
	<Url>[2001:db8::1]:3391</Url></Connection></Connections></RDMExport>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Connections) != 1 || res.Connections[0].Host != "2001:db8::1" || res.Connections[0].Port != 3391 {
		t.Fatalf("unexpected result %+v", res.Connections)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "not in the export") {
		t.Fatalf("expected missing-credential warning, got %q", res.Warnings)
	}

	if _, err := parseRDM([]byte(`<RDMExport><Connections><Connection><Name>x</Name>`)); err == nil {
		t.Fatal("truncated XML must fail")
	}
	if _, err := parseRDM([]byte(`<Connections />`)); err == nil {
		t.Fatal("non-RDM XML must fail")
	}
}
