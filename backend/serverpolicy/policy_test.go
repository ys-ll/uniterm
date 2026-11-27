package serverpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mapEnv(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestDefaultsAllowOnlySafeGroups(t *testing.T) {
	policy := Load(mapEnv(nil))
	if err := policy.LoadError(); err != nil {
		t.Fatalf("default policy load error: %v", err)
	}
	if policy.Source() != "defaults" {
		t.Fatalf("source = %q, want defaults", policy.Source())
	}

	allowed := []string{
		"CreateSession",      // remote sessions (local/local-file stay blocked inside)
		"TestConnection",     // remote probe; container local transport guarded inside
		"K8sConnect",         // K8s
		"K8sRequest",         // K8s
		"ChatCompletion",     // AI
		"SyncNow",            // sync
		"LoadSettings",       // settings store
		"SaveSettings",       // settings store
		"StartIPScan",        // netscan
		"StartPortScan",      // netscan
		"ContainerConnect",   // filter must allow it so the transport guard can run
		"ContainerList",      // container over an established (SSH) connection
		"ContainerLoadImage", // guarded local path
		"SftpGetContent",     // remote SFTP
		"SftpGet",            // local path guarded
		"ReadFileBase64",     // tenant path confined
		"RecorderWriteData",  // records dir confined
		"ParseImportFile",    // staging path confined
		"SetBackgroundImage", // staging path confined
		"ExportConnections",  // tenant path confined
		"GetSaveTempPath",    // staging path only
		"ListMCPClients",     // MCP client config
		"SaveMCPClient",      // MCP client config (persist only; dial is gated)
		"UpdateMCPClient",    // MCP client config
		"RemoveMCPClient",    // MCP client config
		"MCPClientStatuses",  // MCP client config
		"ListMCPTools",       // MCP client config
		"ServiceStartup",     // internal hook, never bound
	}
	for _, method := range allowed {
		if !policy.Allowed(method) {
			t.Errorf("default policy denied %s, want allowed", method)
		}
	}

	denied := []string{
		"StartTelnetServer",       // localServices
		"StartHTTPServer",         // localServices
		"OpenExternalEditorLocal", // externalEditor
		"OpenPathInExplorer",      // externalEditor
		"GetDesktopPath",          // localFilesystem
		"GetDefaultSSHKeyPaths",   // localFilesystem
		"OpenFileDialog",          // nativeDialogs
		"SaveFileDialog",          // nativeDialogs
		"StartMCP",                // mcpServer
		"ListMCPTokens",           // mcpServer
		"StartMCPClient",          // mcpClientProcess
		"TestMCPClient",           // mcpClientProcess
		"CallMCPTool",             // mcpClientProcess
		"CheckForUpdate",          // update
		"DownloadUpdate",          // update
		"StartCapture",            // packetCapture
		"ListCaptureInterfaces",   // packetCapture
		"FirewallProbe",           // firewall
		"X11DesktopConnect",       // x11Desktop
		"SetDataDir",              // admin
		"GetDataDirInfo",          // admin
		"ResetCredentials",        // admin
		"QuitApp",                 // admin
		"RelaunchApp",             // admin
	}
	for _, method := range denied {
		if policy.Allowed(method) {
			t.Errorf("default policy allowed %s, want denied", method)
		}
	}

	if policy.Allowed("ThisMethodDoesNotExist") {
		t.Fatal("unknown method must be denied: no implicit remote default")
	}
	if policy.WorkerDocker() {
		t.Fatal("workerDocker must be denied by default")
	}
}

func TestOptionalGroupRequiresRiskAcceptance(t *testing.T) {
	withoutRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"localServices":true}`,
	}))
	if err := withoutRisk.LoadError(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	if withoutRisk.Allowed("StartTelnetServer") {
		t.Fatal("localServices must stay denied when iAcceptRisk is absent")
	}
	if withoutRisk.Allowed("StartMCP") {
		t.Fatal("other optional groups must stay denied")
	}

	withRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"localServices":true,"iAcceptRisk":true}`,
	}))
	if !withRisk.Allowed("StartTelnetServer") {
		t.Fatal("localServices must be allowed with iAcceptRisk")
	}
	if withRisk.Allowed("StartMCP") {
		t.Fatal("enabling one group must not enable another")
	}
	if withRisk.Allowed("SetDataDir") {
		t.Fatal("admin methods must stay denied even with iAcceptRisk")
	}

	falseRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"localServices":true,"iAcceptRisk":false}`,
	}))
	if falseRisk.Allowed("StartTelnetServer") {
		t.Fatal("iAcceptRisk:false must keep localServices denied")
	}
}

func TestAdminCannotBeEnabled(t *testing.T) {
	policy := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"admin":true,"iAcceptRisk":true}`,
	}))
	if err := policy.LoadError(); err == nil {
		t.Fatal("admin must be rejected as non-configurable, not silently accepted")
	}
	if policy.Allowed("SetDataDir") || policy.Allowed("ResetCredentials") {
		t.Fatal("admin methods must stay denied")
	}
}

func TestMCPClientProcessRequiresRiskAcceptance(t *testing.T) {
	defaults := Load(mapEnv(nil))
	for _, method := range []string{"StartMCPClient", "TestMCPClient", "CallMCPTool"} {
		if defaults.Allowed(method) {
			t.Fatalf("default policy allowed %s", method)
		}
	}
	if !defaults.Allowed("SaveMCPClient") {
		t.Fatal("MCP client config must stay allowed while the process side is denied")
	}

	enabled := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"mcpClientProcess":true,"iAcceptRisk":true}`,
	}))
	for _, method := range []string{"StartMCPClient", "TestMCPClient", "CallMCPTool"} {
		if !enabled.Allowed(method) {
			t.Fatalf("operator-enabled policy denied %s", method)
		}
	}

	noRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"mcpClientProcess":true}`,
	}))
	if noRisk.Allowed("CallMCPTool") {
		t.Fatal("mcpClientProcess must require iAcceptRisk")
	}
}

func TestWorkerDockerCapability(t *testing.T) {
	defaults := Load(mapEnv(nil))
	if defaults.WorkerDocker() {
		t.Fatal("workerDocker must default to false")
	}
	enabled := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"workerDocker":true,"iAcceptRisk":true}`,
	}))
	if !enabled.WorkerDocker() {
		t.Fatal("workerDocker must be enabled with iAcceptRisk")
	}
	noRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"workerDocker":true}`,
	}))
	if noRisk.WorkerDocker() {
		t.Fatal("workerDocker must require iAcceptRisk")
	}
}

func TestMalformedPolicyFailsClosed(t *testing.T) {
	cases := map[string]string{
		"empty object is fine but not this": `{`,
		"null":                              `null`,
		"array":                             `[]`,
		"string":                            `"localServices"`,
		"non-boolean group":                 `{"localServices":"yes"}`,
		"non-boolean risk":                  `{"iAcceptRisk":"yes"}`,
		"unknown key":                       `{"localService":true}`,
		"trailing data":                     `{"localServices":true} {}`,
		"admin key":                         `{"admin":true,"iAcceptRisk":true}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			policy := Load(mapEnv(map[string]string{"UNITERM_SERVER_POLICY": raw}))
			if policy.LoadError() == nil {
				t.Fatalf("expected fail-closed error for %q", raw)
			}
			if policy.Allowed("StartTelnetServer") || policy.Allowed("StartMCP") || policy.Allowed("SetDataDir") {
				t.Fatal("malformed policy must deny every optional group")
			}
			if !policy.Allowed("CreateSession") || !policy.Allowed("K8sConnect") {
				t.Fatal("malformed policy must still allow the safe tenant groups")
			}
		})
	}
}

func TestPolicyFileWinsOverEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "policy.json")
	if err := os.WriteFile(file, []byte(`{"localServices":true,"iAcceptRisk":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	policy := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY_FILE": file,
		"UNITERM_SERVER_POLICY":      `{"localServices":false,"iAcceptRisk":true}`,
	}))
	if err := policy.LoadError(); err != nil {
		t.Fatalf("load error: %v", err)
	}
	if !policy.Allowed("StartTelnetServer") {
		t.Fatal("the file must win over the inline env JSON")
	}
	if !strings.HasPrefix(policy.Source(), "file:") {
		t.Fatalf("source = %q, want file:<path>", policy.Source())
	}
}

func TestMissingPolicyFileFailsClosed(t *testing.T) {
	policy := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY_FILE": filepath.Join(t.TempDir(), "missing.json"),
		"UNITERM_SERVER_POLICY":      `{"localServices":true,"iAcceptRisk":true}`,
	}))
	if policy.LoadError() == nil {
		t.Fatal("missing policy file must fail closed")
	}
	if policy.Allowed("StartTelnetServer") {
		t.Fatal("missing file must ignore the env JSON and deny localServices")
	}
}

func TestMalformedPolicyFileFailsClosed(t *testing.T) {
	file := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(file, []byte(`{"localServices":`), 0o600); err != nil {
		t.Fatal(err)
	}
	policy := Load(mapEnv(map[string]string{"UNITERM_SERVER_POLICY_FILE": file}))
	if policy.LoadError() == nil {
		t.Fatal("malformed policy file must fail closed")
	}
	if policy.Allowed("StartTelnetServer") {
		t.Fatal("malformed file must deny localServices")
	}
}

func TestEmptyEnvironmentUsesDefaults(t *testing.T) {
	policy := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY":      "   ",
		"UNITERM_SERVER_POLICY_FILE": "  ",
	}))
	if err := policy.LoadError(); err != nil {
		t.Fatalf("blank environment must be treated as unset: %v", err)
	}
	if policy.Allowed("StartTelnetServer") {
		t.Fatal("blank environment must use secure defaults")
	}
}

func TestNilGetterFailsClosed(t *testing.T) {
	policy := Load(nil)
	if policy.LoadError() == nil {
		t.Fatal("nil getter must fail closed")
	}
	if policy.Allowed("StartTelnetServer") {
		t.Fatal("nil getter must deny optional groups")
	}
}
