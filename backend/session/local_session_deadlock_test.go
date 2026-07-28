package session

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestLocalSessionConnectDisconnectNoDeadlock exercises the full
// Connect → cmd.Wait goroutine → Disconnect cycle with a shell that
// exits quickly. The OLD buggy code had the cmd.Wait goroutine call
// s.Disconnect() after cmd.Wait returned; that Disconnect then
// deadlocked inside its own select-waitDone loop because waitDone
// is only closed by a defer that fires AFTER Disconnect returns.
//
// With the fix, the cmd.Wait goroutine does NOT call Disconnect, so
// Disconnect (called externally from this test) is free to wait on
// waitDone normally. The test fails if Disconnect blocks past the
// outer 3s timeout — which is what the bug did.
func TestLocalSessionConnectDisconnectNoDeadlock(t *testing.T) {
	s := NewLocalSession("regression-cycle")
	// `/usr/bin/true` exits with status 0 immediately, mimicking a
	// shell that has been told to exit. loginShellArgs on non-darwin
	// is nil so we don't need any login machinery. /usr/bin/true
	// exists on macOS and Linux (CI runners); /bin/true is a Linux
	// symlink that isn't always present on macOS.
	if err := s.Connect(ConnectionConfig{ShellPath: "/usr/bin/true"}); err != nil {
		t.Skipf("Connect: %v (likely /usr/bin/true missing on this platform)", err)
	}

	// External Disconnect — the way CloseAll / tab-close invokes it.
	// With the fix in place, this returns within ~500ms (it waits on
	// waitDone via the select-with-timeout path). With the buggy
	// Wait goroutine still calling Disconnect, the inner Disconnect
	// would have already taken waitDone and the outer one would
	// either deadlock or take the kill path before returning.
	discDone := make(chan struct{})
	go func() {
		defer close(discDone)
		_ = s.Disconnect()
	}()

	select {
	case <-discDone:
		// Good — Disconnect returned. Confirm the Wait goroutine
		// has also finished (it's just close(waitDone) now).
		select {
		case <-s.waitDone:
			// The Wait goroutine exited cleanly. The fix is in place.
		case <-time.After(2 * time.Second):
			t.Fatalf("cmd.Wait goroutine never exited after Disconnect returned")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Disconnect deadlocked — the cmd.Wait goroutine likely called Disconnect itself")
	}
}

// TestLocalSessionWaitGoroutineDoesNotCallDisconnect is a structural
// guard parsed from the source: it walks the Go AST of
// local_session_unix.go, locates the cmd.Wait goroutine launched in
// LocalSession.Connect (the one whose body contains a
// `defer close(s.waitDone)` followed by `cmd.Wait()`), and asserts
// that the same goroutine body does NOT also invoke `s.Disconnect`.
//
// The buggy pattern was:
//
//	go func() {
//	    defer close(s.waitDone)
//	    _ = s.cmd.Wait()
//	    s.Disconnect()  // <-- deadlock: Disconnect waits on waitDone
//	}
//
// Reintroducing that line trips this test immediately. We use the AST
// rather than string/regex matching so that comments containing
// "Disconnect" or "defer close(s.waitDone)" don't produce false
// positives — the AST only sees real code.
func TestLocalSessionWaitGoroutineDoesNotCallDisconnect(t *testing.T) {
	src := readLocalSource(t)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "local_session_unix.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	// Find every `go func() { ... }()` whose body contains a
	// `defer close(s.waitDone)` followed by `_ = s.cmd.Wait()`. That
	// uniquely identifies the Wait goroutine in Connect.
	waitGoroutines := findWaitGoroutines(fset, file)
	if len(waitGoroutines) == 0 {
		t.Fatalf("could not find the cmd.Wait goroutine in local_session_unix.go — has the file been refactored?")
	}
	if len(waitGoroutines) > 1 {
		t.Fatalf("found %d candidate cmd.Wait goroutines, expected exactly 1 — has the file been refactored?", len(waitGoroutines))
	}

	g := waitGoroutines[0]
	// Walk all statements (ExprStmt or AssignStmt) and flag any
	// Disconnect call. The bug typically manifested as a top-level
	// `s.Disconnect()` ExprStmt, but a future regression could
	// embed it in any statement form (if/return/AssignStmt) — we
	// cover them all by walking the whole subtree.
	var foundCall *ast.CallExpr
	ast.Inspect(g.Body, func(n ast.Node) bool {
		if foundCall != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isMethodCallOnRecv(call, "Disconnect") {
			foundCall = call
		}
		return true
	})
	if foundCall != nil {
		pos := fset.Position(foundCall.Pos())
		t.Fatalf("cmd.Wait goroutine at %s must NOT call Disconnect — that pattern deadlocks because Disconnect waits on waitDone, which only closes via the goroutine's defer that fires AFTER Disconnect returns.", pos)
	}
}

// findWaitGoroutines walks the AST and returns the func literals
// launched via `go func(){...}()` whose body contains the canonical
// `defer close(s.waitDone); _ = s.cmd.Wait()` pair.
func findWaitGoroutines(fset *token.FileSet, file *ast.File) []*ast.FuncLit {
	var out []*ast.FuncLit
	ast.Inspect(file, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		call, ok := goStmt.Call.Fun.(*ast.FuncLit)
		if !ok {
			return true
		}
		if hasDeferCloseWaitDone(call) {
			out = append(out, call)
		}
		return true
	})
	return out
}

// hasDeferCloseWaitDone reports whether fn's body contains a
// `defer close(<ident>)` AND a `cmd.Wait()` call. The latter can
// appear as either an ExprStmt (`cmd.Wait()`) or an AssignStmt
// (`_ = cmd.Wait()`); we look for the `cmd.Wait` selector chain
// directly so the check stays valid across both forms.
func hasDeferCloseWaitDone(fn *ast.FuncLit) bool {
	hasDeferClose := false
	hasCmdWait := false
	ast.Inspect(fn, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.DeferStmt:
			// close(ident)
			if call, ok := s.Call.Fun.(*ast.Ident); ok && call.Name == "close" {
				hasDeferClose = true
			}
		case *ast.CallExpr:
			// Either an ExprStmt `cmd.Wait()` or the RHS of an
			// AssignStmt `_ = cmd.Wait()`. Both reach us as a
			// *ast.CallExpr with fun = <recv>.cmd.Wait.
			if sel, ok := s.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Wait" {
				if recvSel, ok := sel.X.(*ast.SelectorExpr); ok && recvSel.Sel.Name == "cmd" {
					hasCmdWait = true
				}
			}
		}
		return true
	})
	return hasDeferClose && hasCmdWait
}

// isMethodCallOnRecv reports whether call is an expression of the
// form <receiver>.Disconnect() where the receiver is a selector chain
// ending in `Disconnect`.
func isMethodCallOnRecv(call *ast.CallExpr, name string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	return sel.Sel.Name == name
}

// readLocalSource reads local_session_unix.go from this test's package
// directory, without hard-coding an absolute path.
func readLocalSource(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := file[:strings.LastIndex(file, "/")]
	p := dir + "/local_session_unix.go"
	bs, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(bs)
}