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

// TestLocalSessionConnectDisconnectNoDeadlock guards the cmd.Wait
// goroutine deadlock: if the goroutine calls Disconnect, every local
// teardown (tab close, app shutdown) hangs forever.
func TestLocalSessionConnectDisconnectNoDeadlock(t *testing.T) {
	s := NewLocalSession("regression-cycle")
	// /usr/bin/true exits immediately, mimicking a shell told to exit.
	// macOS + Linux both ship it; /bin/true is a Linux symlink.
	if err := s.Connect(ConnectionConfig{ShellPath: "/usr/bin/true"}); err != nil {
		t.Skipf("Connect: %v (likely /usr/bin/true missing on this platform)", err)
	}

	discDone := make(chan struct{})
	go func() {
		defer close(discDone)
		_ = s.Disconnect()
	}()

	select {
	case <-discDone:
		select {
		case <-s.waitDone:
		case <-time.After(2 * time.Second):
			t.Fatalf("cmd.Wait goroutine never exited after Disconnect returned")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("Disconnect deadlocked — the cmd.Wait goroutine likely called Disconnect itself")
	}
}

// TestLocalSessionWaitGoroutineDoesNotCallDisconnect is a structural
// guard: it parses local_session_unix.go as Go and asserts the cmd.Wait
// goroutine body never invokes s.Disconnect. Uses the AST so comments
// mentioning "Disconnect" / "defer close(s.waitDone)" don't trip it.
func TestLocalSessionWaitGoroutineDoesNotCallDisconnect(t *testing.T) {
	src := readLocalSource(t)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "local_session_unix.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	waitGoroutines := findWaitGoroutines(fset, file)
	if len(waitGoroutines) == 0 {
		t.Fatalf("could not find the cmd.Wait goroutine in local_session_unix.go — has the file been refactored?")
	}
	if len(waitGoroutines) > 1 {
		t.Fatalf("found %d candidate cmd.Wait goroutines, expected exactly 1 — has the file been refactored?", len(waitGoroutines))
	}

	// Walk the whole subtree (not just top-level stmts) so a future
	// regression embedding Disconnect inside an if/return still trips.
	var foundCall *ast.CallExpr
	ast.Inspect(waitGoroutines[0].Body, func(n ast.Node) bool {
		if foundCall != nil {
			return false
		}
		if call, ok := n.(*ast.CallExpr); ok && isMethodCallOnRecv(call, "Disconnect") {
			foundCall = call
		}
		return true
	})
	if foundCall != nil {
		pos := fset.Position(foundCall.Pos())
		t.Fatalf("cmd.Wait goroutine at %s must NOT call Disconnect — Disconnect waits on waitDone, which only closes via the goroutine's defer that fires AFTER Disconnect returns.", pos)
	}
}

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

// hasDeferCloseWaitDone identifies the cmd.Wait goroutine by its body:
// a `defer close(<ident>)` plus a `cmd.Wait()` call (which the AST
// represents the same way whether the source form is `cmd.Wait()` or
// `_ = cmd.Wait()`).
func hasDeferCloseWaitDone(fn *ast.FuncLit) bool {
	hasDeferClose := false
	hasCmdWait := false
	ast.Inspect(fn, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.DeferStmt:
			if call, ok := s.Call.Fun.(*ast.Ident); ok && call.Name == "close" {
				hasDeferClose = true
			}
		case *ast.CallExpr:
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