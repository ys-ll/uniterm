package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMRemoteNGProtocolsAndPorts(t *testing.T) {
	for _, tt := range []struct {
		protocol, port, typ string
		wantPort            int
	}{
		{"SSH2", "22", "ssh", 22}, {"ssh1", "", "ssh", 22},
		{"RDP", "invalid", "rdp", 3389}, {"vNc", "0", "vnc", 5900},
		{"Telnet", "-1", "telnet", 23}, {"SSH2", "2222", "ssh", 2222},
		{"VNC", "65536", "vnc", 5900}, {"SSH2", "65535", "ssh", 65535},
	} {
		t.Run(tt.protocol+"/"+tt.port, func(t *testing.T) {
			data := fmt.Sprintf(`<Connections><Node Name="server01" Type="Connection" Hostname="192.0.2.10" Protocol="%s" Port="%s" Username="operator" Descr="Example server" Domain="EXAMPLE" ConnectToConsole="True" UseCredSsp="True" /></Connections>`, tt.protocol, tt.port)
			res := mustParseMRemoteNG(t, data, ParseOptions{})
			if len(res.Connections) != 1 || len(res.Warnings) != 0 || len(res.Groups) != 0 {
				t.Fatal("expected one connection without groups or warnings")
			}
			c := res.Connections[0]
			if c.Name != "server01" || c.Host != "192.0.2.10" || c.Type != tt.typ || c.Port != tt.wantPort || c.User != "operator" || c.Remark != "Example server" || c.GroupId != nil || c.ID == "" {
				t.Fatal("incorrect connection field mapping")
			}
			if tt.typ == "ssh" && c.AuthType != "password" {
				t.Fatal("SSH must use password authentication")
			}
			if tt.typ == "rdp" {
				if c.RdpDomain != "EXAMPLE" || !c.RdpAdminSession || !c.RdpEnableNLA {
					t.Fatal("incorrect RDP field mapping")
				}
			} else if c.RdpDomain != "" || c.RdpAdminSession || c.RdpEnableNLA {
				t.Fatal("RDP settings must only apply to RDP connections")
			}
		})
	}
}

func TestMRemoteNGFolders(t *testing.T) {
	res := mustParseMRemoteNG(t, `<Connections Name="Connections">
	<Node Name="Production" Type="Container">
	  <Node Name="Web" Type="Container"><Node Name="Region" Type="Container">
	    <Node Name="one" Type="Connection" Hostname="192.0.2.1" Protocol="SSH2" />
	    <Node Name="two" Type="Connection" Hostname="192.0.2.2" Protocol="SSH2" />
	  </Node></Node>
	  <Node Name="Empty" Type="Container" />
	</Node>
	<Node Name="Other" Type="Container"><Node Name="Web" Type="Container" /></Node>
	<Node Name="root" Type="Connection" Hostname="192.0.2.3" Protocol="Telnet" />
	</Connections>`, ParseOptions{})
	if len(res.Groups) != 6 || len(res.Connections) != 3 {
		t.Fatalf("got %d groups and %d connections", len(res.Groups), len(res.Connections))
	}
	wantPaths := []string{"Production", "Production/Web", "Production/Web/Region", "Production/Empty", "Other", "Other/Web"}
	ids := map[string]bool{}
	for i, g := range res.Groups {
		if g.ID == "" || ids[g.ID] || groupPathFor(res.Groups, g.ID) != wantPaths[i] {
			t.Fatalf("incorrect or duplicate group at index %d", i)
		}
		ids[g.ID] = true
	}
	for _, c := range res.Connections[:2] {
		if c.GroupId == nil || groupPathFor(res.Groups, *c.GroupId) != "Production/Web/Region" || c.User != "root" {
			t.Fatal("nested connection lost its folder or default SSH user")
		}
		if ids[c.ID] || c.ID == "" {
			t.Fatal("duplicate or empty connection ID")
		}
		ids[c.ID] = true
	}
	if res.Connections[2].GroupId != nil || res.Connections[2].User != "" {
		t.Fatal("root Telnet connection should have no group or default user")
	}
}

func TestMRemoteNGInheritance(t *testing.T) {
	res := mustParseMRemoteNG(t, `<Connections><Node Name="Parent" Type="Container" Hostname="must-not-inherit.example" Username="admin" Protocol="RDP" Port="3390" Domain="EXAMPLE" Descr="Inherited description" ConnectToConsole="True" UseCredSsp="True">
	<Node Name="Middle" Type="Container" Username="ignored" Protocol="SSH2" Port="22" Domain="ignored" InheritUsername="True" InheritProtocol="True" InheritPort="True" InheritDomain="True" InheritDescription="True" InheritUseConsoleSession="True" InheritUseCredSsp="True">
	<Node Name="inherited" Type="Connection" Hostname="192.0.2.20" Username="ignored" Protocol="VNC" Port="5901" Domain="ignored" InheritUsername="true" InheritProtocol="TRUE" InheritPort="True" InheritDomain="True" InheritDescription="True" InheritUseConsoleSession="True" InheritUseCredSsp="True" />
	<Node Name="explicit" Type="Connection" Hostname="192.0.2.21" Username="local" Protocol="SSH1" Port="2223" Domain="LOCAL" InheritUsername="False" InheritProtocol="False" InheritPort="False" />
	<Node Name="empty" Type="Connection" Protocol="Telnet" Username="" />
	</Node></Node></Connections>`, ParseOptions{})
	if len(res.Connections) != 3 {
		t.Fatal("expected three connections")
	}
	c := res.Connections[0]
	if c.Name != "inherited" || c.Host != "192.0.2.20" || c.User != "admin" || c.Type != "rdp" || c.Port != 3390 || c.RdpDomain != "EXAMPLE" || c.Remark != "Inherited description" || !c.RdpEnableNLA || !c.RdpAdminSession {
		t.Fatal("effective values were not inherited through multiple containers")
	}
	c = res.Connections[1]
	if c.User != "local" || c.Type != "ssh" || c.Port != 2223 || c.Remark != "" || c.RdpEnableNLA || c.RdpAdminSession {
		t.Fatal("explicit child fields must win when inheritance is disabled")
	}
	c = res.Connections[2]
	if c.User != "" || c.Host != "" || c.Port != 23 {
		t.Fatal("missing fields must not implicitly inherit")
	}
}

func TestMRemoteNGUnsupportedProtocol(t *testing.T) {
	res := mustParseMRemoteNG(t, `<Connections><Node Name="Empty" Type="Container"><Node Name="Citrix01" Type="Connection" Protocol="ICA" /></Node><Node Name="ok" Type="Connection" Protocol="SSH2" Hostname="192.0.2.1" /></Connections>`, ParseOptions{})
	if len(res.Groups) != 1 || len(res.Connections) != 1 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "ICA") || !strings.Contains(res.Warnings[0], "Citrix01") {
		t.Fatal("unsupported connection should warn and leave other nodes intact")
	}
}

func TestMRemoteNGInvalidXML(t *testing.T) {
	for _, data := range []string{"", `<Connections><Node></Connections>`, `<WrongRoot />`, `<Connections /><Connections />`, `<Connections />trailing`} {
		if _, err := parseMRemoteNG([]byte(data), ParseOptions{}); err == nil {
			t.Fatal("expected invalid XML to be rejected")
		}
	}
}

func TestMRemoteNGFullFileEncryption(t *testing.T) {
	for _, data := range []string{`<Connections FullFileEncryption="True">ZW5jcnlwdGVk</Connections>`, "ZW5jcnlwdGVk"} {
		if _, err := parseMRemoteNG([]byte(data), ParseOptions{}); err == nil || !strings.Contains(err.Error(), "fully encrypted mRemoteNG") {
			t.Fatal("expected a clear full-file encryption error")
		}
	}
}

func TestMRemoteNGPasswordFailures(t *testing.T) {
	for _, attrs := range []string{
		`EncryptionEngine="Twofish" BlockCipherMode="GCM" KdfIterations="1000"`,
		`EncryptionEngine="AES" BlockCipherMode="CBC" KdfIterations="1000"`,
		`EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="0"`,
		`EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="invalid"`,
		`EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="2147483647"`,
		`EncryptionEngine="AES" BlockCipherMode="GCM" KdfIterations="1000"`,
		"",
	} {
		res := mustParseMRemoteNG(t, `<Connections `+attrs+`><Node Name="a" Type="Connection" Hostname="192.0.2.1" Protocol="SSH2" Password="not-a-valid-ciphertext" /><Node Name="b" Type="Connection" Protocol="RDP" Password="not-a-valid-ciphertext" /></Connections>`, ParseOptions{Password: "synthetic-master"})
		if len(res.Connections) != 2 || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "password") {
			t.Fatal("failed passwords should produce one useful warning and preserve connections")
		}
		for _, c := range res.Connections {
			if c.Password != "" {
				t.Fatal("failed decryption imported a password")
			}
		}
		if strings.Contains(res.Warnings[0], "not-a-valid-ciphertext") || strings.Contains(res.Warnings[0], "synthetic-master") {
			t.Fatal("warning exposed credential material")
		}
	}
}

func TestMRemoteNGDispatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "confCons.xml")
	if err := os.WriteFile(path, []byte(`<Connections><Node Name="ssh" Type="Connection" Protocol="SSH2" Hostname="192.0.2.1" /></Connections>`), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := Parse("mremoteng", path, ParseOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Connections) != 1 || res.Connections[0].Type != "ssh" {
		t.Fatal("mremoteng was not dispatched correctly")
	}
}

func mustParseMRemoteNG(t *testing.T, data string, opts ParseOptions) *ImportResult {
	t.Helper()
	res, err := parseMRemoteNG([]byte(data), opts)
	if err != nil {
		t.Fatal(err)
	}
	return res
}
