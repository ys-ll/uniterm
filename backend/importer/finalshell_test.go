package importer

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFinalShellDir builds a conn directory from raw JSON files and returns
// its path.
func writeFinalShellDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func finalShellConn(name, host, parent, typ string) string {
	return `{"name":"` + name + `","host":"` + host + `","port":22,"user_name":"deploy","parent_id":"` +
		parent + `","conection_type":` + typ + `,"description":"desc","delete_time":0}`
}

func TestFinalShellImportsRootNestedAndGroups(t *testing.T) {
	dir := writeFinalShellDir(t, map[string]string{
		"folder.json":                            `{"id":"f1","name":"Production","parent_id":"root","delete_time":0}`,
		"sub/folder.json":                        `{"id":"f2","name":"East","parent_id":"f1","delete_time":0}`,
		"aa11_connect_config.json":               finalShellConn("Root Host", "10.0.0.1", "root", "100"),
		"sub/bb22_connect_config.json":           finalShellConn("Nested Host", "10.0.0.2", "f1", "100"),
		"sub/cc33_connect_config.json":           finalShellConn("Deep Host", "10.0.0.3", "f2", "100"),
		"sub/deleted_connect_config.json":        `{"name":"Deleted","host":"10.9.9.9","parent_id":"f1","conection_type":100,"delete_time":1795688478380}`,
		"sub/legacy_connect_config.json":         finalShellConn("Legacy SSH", "10.0.0.4", "root", "1"),
		"sub/rdp_connect_config.json":            `{"name":"Windows Box","host":"10.0.0.5","port":3389,"user_name":"admin","parent_id":"root","conection_type":101,"custom_size":true,"width":1280,"height":1024,"delete_time":0}`,
		"sub/unknown_connect_config.json":        finalShellConn("Weird", "10.0.0.6", "root", "7"),
		"sub/nohost_connect_config.json":         finalShellConn("No Host", "", "root", "100"),
		"sub/unrelated_connect_config.json.json": `{"keep":"not a connection"}`,
	})

	res, err := parseFinalShell(dir)
	if err != nil {
		t.Fatal(err)
	}

	if got := len(res.Connections); got != 5 {
		t.Fatalf("expected 5 connections, got %d: %+v", got, res.Connections)
	}
	groupsByName := map[string]string{}
	for _, g := range res.Groups {
		groupsByName[g.Name] = g.ID
	}
	byName := map[string]int{}
	for i, c := range res.Connections {
		byName[c.Name] = i
	}

	root := res.Connections[byName["Root Host"]]
	if root.Type != "ssh" || root.Host != "10.0.0.1" || root.Port != 22 || root.User != "deploy" ||
		root.AuthType != "password" || root.Remark != "desc" || root.GroupId != nil {
		t.Errorf("unexpected root connection: %+v", root)
	}

	nested := res.Connections[byName["Nested Host"]]
	if nested.GroupId == nil || *nested.GroupId != groupsByName["Production"] {
		t.Errorf("nested host should map to the Production group: %+v", nested)
	}

	deep := res.Connections[byName["Deep Host"]]
	if deep.GroupId == nil || *deep.GroupId != groupsByName["East"] {
		t.Errorf("deep host should map to the East group: %+v", deep)
	}
	if east := res.Groups; len(east) != 2 {
		t.Errorf("expected 2 groups, got %d", len(east))
	}

	legacy := res.Connections[byName["Legacy SSH"]]
	if legacy.Type != "ssh" || legacy.User != "deploy" {
		t.Errorf("legacy conection_type 1 should import as ssh: %+v", legacy)
	}

	rdp := res.Connections[byName["Windows Box"]]
	if rdp.Type != "rdp" || rdp.Port != 3389 || rdp.RdpFixedWidth != 1280 || rdp.RdpFixedHeight != 1024 {
		t.Errorf("rdp connection should keep port and custom size: %+v", rdp)
	}
	if rdp.AuthType != "" {
		t.Errorf("rdp connection should not get the ssh password auth type: %+v", rdp)
	}

	if len(res.Warnings) != 2 {
		t.Fatalf("expected 2 warnings (unknown type, empty host), got %v", res.Warnings)
	}
}

func TestFinalShellDefaults(t *testing.T) {
	dir := writeFinalShellDir(t, map[string]string{
		"a_connect_config.json": `{"name":"","host":"10.1.1.1","port":0,"user_name":"","parent_id":"","conection_type":100,"delete_time":0}`,
		"b_connect_config.json": `{"name":"RDP Defaults","host":"10.1.1.2","port":0,"parent_id":"","conection_type":101,"fullscreen":true,"delete_time":0}`,
	})

	res, err := parseFinalShell(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(res.Connections))
	}

	sshConn := res.Connections[0]
	if sshConn.Name != "10.1.1.1" || sshConn.Port != 22 || sshConn.User != "root" {
		t.Errorf("ssh defaults (name=host, port 22, user root) not applied: %+v", sshConn)
	}
	rdpConn := res.Connections[1]
	if rdpConn.Port != 3389 || rdpConn.RdpFixedWidth != -1 || rdpConn.RdpFixedHeight != -1 {
		t.Errorf("rdp defaults (port 3389, fullscreen sentinel) not applied: %+v", rdpConn)
	}
}

func TestFinalShellSourceMustBeDirectory(t *testing.T) {
	if _, err := parseFinalShell(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected an error for a missing path")
	}
	plain := filepath.Join(t.TempDir(), "not-a-dir.txt")
	if err := os.WriteFile(plain, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := parseFinalShell(plain); err == nil {
		t.Fatal("expected an error for a non-directory source")
	}
}
