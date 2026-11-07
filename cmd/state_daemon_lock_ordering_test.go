package cmd

import (
	"go/ast"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// A daemon that took the lock and then did anything before writing its pid
// would leave daemon.pid naming a dead predecessor while it holds the lock,
// which the lock's pre-check then reads as no daemon.
func TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "state_daemon.go")
	body := findFuncDeclInFile(t, source.File, "defaultDaemonRun").Body.List

	acquire := -1
	for i, stmt := range body {
		if assign, ok := stmt.(*ast.AssignStmt); ok && len(assign.Rhs) == 1 && holdsCallTo(assign.Rhs[0], "acquireDaemonLock") {
			acquire = i
			break
		}
	}
	if acquire < 0 || acquire+2 >= len(body) {
		t.Fatalf("defaultDaemonRun has no acquireDaemonLock assignment followed by two statements")
	}
	if guard, ok := body[acquire+1].(*ast.IfStmt); !ok || !isErrGuard(guard) {
		t.Errorf("line %d: the statement after acquireDaemonLock is not its `if err != nil { return }` guard",
			source.Position(body[acquire+1].Pos()).Line)
	}
	if write, ok := body[acquire+2].(*ast.IfStmt); !ok || !holdsCallTo(write.Init, "WritePIDFile") {
		t.Errorf("line %d: the statement after the lock's guard does not write daemon.pid",
			source.Position(body[acquire+2].Pos()).Line)
	}
}

func TestAcquireDaemonLock_SingleProductionCallSite(t *testing.T) {
	var sites []string
	for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
		ast.Inspect(source.File, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				if name := sourceguardtest.CalleeName(call); name == "acquireDaemonLock" || name == "AcquireDaemonLock" {
					sites = append(sites, source.Position(call.Pos()).String())
				}
			}
			return true
		})
	}
	if len(sites) != 1 {
		t.Errorf("acquireDaemonLock is called at %v, want once, in defaultDaemonRun: a second caller would escape the acquire-then-write-pid order", sites)
	}
}

// holdsCallTo reports whether node holds a call to a function or method named name.
func holdsCallTo(node ast.Node, name string) bool {
	if node == nil {
		return false
	}
	found := false
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && sourceguardtest.CalleeName(call) == name {
			found = true
		}
		return !found
	})
	return found
}

// isErrGuard matches `if err != nil { … return … }`.
func isErrGuard(stmt *ast.IfStmt) bool {
	cond, ok := stmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	if x, ok := cond.X.(*ast.Ident); !ok || x.Name != "err" {
		return false
	}
	if y, ok := cond.Y.(*ast.Ident); !ok || y.Name != "nil" {
		return false
	}
	returns := false
	ast.Inspect(stmt.Body, func(n ast.Node) bool {
		_, isReturn := n.(*ast.ReturnStmt)
		returns = returns || isReturn
		return !returns
	})
	return returns
}
