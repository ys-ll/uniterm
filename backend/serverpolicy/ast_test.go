package serverpolicy

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

type appMethodDecl struct {
	files []string
	decls []*ast.FuncDecl
	fset  *token.FileSet
}

func parseAppMethods(t *testing.T, root string) map[string]*appMethodDecl {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read repo root: %v", err)
	}
	fset := token.NewFileSet()
	methods := make(map[string]*appMethodDecl)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(root, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			if !receiverIsApp(fn.Recv.List[0].Type) {
				continue
			}
			entry := methods[fn.Name.Name]
			if entry == nil {
				entry = &appMethodDecl{fset: fset}
				methods[fn.Name.Name] = entry
			}
			entry.files = append(entry.files, name)
			entry.decls = append(entry.decls, fn)
		}
	}
	return methods
}

func receiverIsApp(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	ident, ok := star.X.(*ast.Ident)
	return ok && ident.Name == "App"
}

func renderDecl(entry *appMethodDecl, decl *ast.FuncDecl) string {
	var buf bytes.Buffer
	_ = printer.Fprint(&buf, entry.fset, decl)
	return buf.String()
}

func callsIdentifier(entry *appMethodDecl, decl *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(decl, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			if fun.Sel.Name == name {
				found = true
				return false
			}
		case *ast.Ident:
			if fun.Name == name {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// TestMethodsCoverEveryAppMethod is the completeness gate: every exported
// *App method must be classified, and every policy key must name a real
// exported method. Unexported *App methods are collected too (the transport
// guard test inspects one), but Wails never binds them, so they are not policy
// entries.
func TestMethodsCoverEveryAppMethod(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)

	var unclassified []string
	exportedCount := 0
	for name := range discovered {
		if !token.IsExported(name) {
			continue
		}
		exportedCount++
		if _, ok := Methods[name]; !ok {
			unclassified = append(unclassified, name)
		}
	}
	sort.Strings(unclassified)
	if len(unclassified) > 0 {
		t.Fatalf("unclassified App methods (add each to backend/serverpolicy/methods.go): %s", strings.Join(unclassified, ", "))
	}

	var ghost []string
	for name := range Methods {
		entry, ok := discovered[name]
		if !ok || !token.IsExported(name) || entry == nil {
			ghost = append(ghost, name)
		}
	}
	sort.Strings(ghost)
	if len(ghost) > 0 {
		t.Fatalf("policy map keys that are not real *App methods: %s", strings.Join(ghost, ", "))
	}

	if len(Methods) != exportedCount {
		t.Fatalf("policy map has %d entries, AST has %d exported App methods", len(Methods), exportedCount)
	}
}

func TestGroupTableIsConsistent(t *testing.T) {
	known := make(map[Group]bool, len(Groups))
	for _, group := range Groups {
		if known[group] {
			t.Fatalf("group %q listed twice", group)
		}
		known[group] = true
	}
	for name, group := range Methods {
		if !known[group] {
			t.Fatalf("method %s uses undeclared group %q", name, group)
		}
		if group == GroupWorkerDocker {
			t.Fatalf("method %s must not be bound to workerDocker directly; use the transport guard", name)
		}
	}
	for _, group := range OptionalGroups {
		if AlwaysAllowedGroups[group] {
			t.Fatalf("group %q is both optional and always allowed", group)
		}
		if PermanentDeniedGroups[group] {
			t.Fatalf("group %q is both optional and permanently denied", group)
		}
	}
}

func TestTenantFilesMethodsCallLocalPathGuard(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)

	// SftpListLocalDrives has no path argument; GetSaveTempPath constructs the
	// staging path itself and is checked separately.
	noPathArgument := map[string]bool{
		"SftpListLocalDrives": true,
		"GetSaveTempPath":     true,
	}

	var unguarded []string
	for name, group := range Methods {
		if group != GroupTenantFiles || noPathArgument[name] {
			continue
		}
		entry := discovered[name]
		if entry == nil || len(entry.decls) == 0 {
			unguarded = append(unguarded, name+" (no declaration)")
			continue
		}
		for _, decl := range entry.decls {
			if !callsIdentifier(entry, decl, "confineLocalPath") {
				unguarded = append(unguarded, name)
			}
		}
	}
	sort.Strings(unguarded)
	if len(unguarded) > 0 {
		t.Fatalf("tenantFiles methods missing a.confineLocalPath guard: %s", strings.Join(unguarded, ", "))
	}
}

func TestGetSaveTempPathUsesStagingRoot(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)
	entry := discovered["GetSaveTempPath"]
	if entry == nil {
		t.Fatal("GetSaveTempPath not found")
	}
	found := false
	for _, decl := range entry.decls {
		if callsIdentifier(entry, decl, "serverStagingDir") {
			found = true
		}
	}
	if !found {
		t.Fatal("GetSaveTempPath must build its path through serverStagingDir in server mode")
	}
}

func TestContainerTransportGuardsAreWired(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)
	for _, name := range []string{"ContainerConnect", "createContainerFileSession", "testContainerConnection"} {
		entry := discovered[name]
		if entry == nil {
			t.Fatalf("%s not found", name)
		}
		found := false
		for _, decl := range entry.decls {
			if callsIdentifier(entry, decl, "checkContainerTransport") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must call a.checkContainerTransport", name)
		}
	}
}

func TestRemoteLocalPathMethodsCallGuard(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)

	for _, name := range []string{"DBPreviewCSV", "DBImportCSV", "readKubeconfigSource"} {
		entry := discovered[name]
		if entry == nil {
			t.Fatalf("%s not found", name)
		}
		found := false
		for _, decl := range entry.decls {
			if callsIdentifier(entry, decl, "confineLocalPath") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must call a.confineLocalPath", name)
		}
	}

	for _, name := range []string{"CreateSession", "TestConnection"} {
		entry := discovered[name]
		if entry == nil {
			t.Fatalf("%s not found", name)
		}
		found := false
		for _, decl := range entry.decls {
			if callsIdentifier(entry, decl, "confineSQLiteHost") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must call a.confineSQLiteHost", name)
		}
	}
}

func TestMCPClientProcessGuardsAreWired(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)

	// Every path that can dial/spawn a persisted external MCP server must
	// consult the policy: the bound methods are filtered out, but startup,
	// save, remove and sync-pull reloads call the manager directly.
	for _, name := range []string{
		"StartMCPClient",
		"saveMCPClient",
		"reloadMCPClientsAfterSync",
		"RemoveMCPClient",
		"TestMCPClient",
		"CallMCPTool",
	} {
		entry := discovered[name]
		if entry == nil {
			t.Fatalf("%s not found", name)
		}
		found := false
		for _, decl := range entry.decls {
			if callsIdentifier(entry, decl, "serverMCPClientProcessEnabled") {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must consult serverMCPClientProcessEnabled", name)
		}
	}
}

func TestLocalTerminalDenialRemains(t *testing.T) {
	root := repoRoot(t)
	discovered := parseAppMethods(t, root)
	entry := discovered["CreateSession"]
	if entry == nil || len(entry.decls) == 0 {
		t.Fatal("CreateSession not found")
	}
	source := renderDecl(entry, entry.decls[0])
	for _, want := range []string{
		`sessionType == "local"`,
		`sessionType == "local-file"`,
		"local terminals are disabled in server mode",
		"serverMode",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("CreateSession lost the permanent local terminal denial (%q missing)", want)
		}
	}
}

func TestEveryOptionalGroupHasAMemberOrIsADocumentedCapability(t *testing.T) {
	counts := make(map[Group]int)
	for _, group := range Methods {
		counts[group]++
	}
	for _, group := range OptionalGroups {
		if group == GroupWorkerDocker {
			if counts[group] != 0 {
				t.Fatalf("workerDocker is a capability, not a method group")
			}
			continue
		}
		if counts[group] == 0 {
			t.Fatalf("optional group %q has no methods", group)
		}
	}
}
