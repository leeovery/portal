// Tests in this file mutate no package-level state and operate solely on the
// source file's AST. They are unit-level guards for the spec § Component C
// step 4 acquire+WritePIDFile adjacency invariant — they would NOT detect a
// runtime regression (the runtime contract is covered by the lock-acquire
// integration tests in state_daemon_run_test.go).
package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// daemonRunFuncName is the production function whose body must contain the
// acquire-lock + WritePIDFile adjacency. Centralized so a future rename
// produces a single fail-fast diagnostic instead of multiple drift-prone
// matchers across this file.
const daemonRunFuncName = "defaultDaemonRun"

// acquireDaemonLockIdent is the name of the package-level seam that wraps
// state.AcquireDaemonLock. Test 1 finds the call site via name match;
// Test 2 counts production call sites via name match (tolerating both the
// seam call and any future bare state.AcquireDaemonLock invocation).
const acquireDaemonLockIdent = "acquireDaemonLock"

// writePIDFileIdent is the name of the WritePIDFile helper. We match by
// name only (not by full selector) so the test tolerates either a bare
// WritePIDFile call (if the cmd package ever exposes a local wrapper) or
// the canonical state.WritePIDFile call shape.
const writePIDFileIdent = "WritePIDFile"

// stateDaemonSourcePath is the source file the AST tests parse. Hard-coded
// because the test is intentionally tied to this file — a refactor that
// moves defaultDaemonRun to a different file should produce a fail-fast
// diagnostic so reviewers see the move and update the path explicitly.
var stateDaemonSourcePath = filepath.Join("state_daemon.go")

// TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire asserts the spec
// § Component C step 4 adjacency invariant on the production source file's
// AST: inside defaultDaemonRun, the WritePIDFile call must be the next
// statement after the acquireDaemonLock call's err-guard. Any statement
// inserted between them — log line, metric tick, intermediate work — fails
// this test with a diagnostic naming the intruding statement's AST type and
// line number.
//
// Spec contract (specification.md § Component C step 4):
//
//	"The daemon must write daemon.pid as the next statement after the
//	 successful acquireDaemonLock return in cmd/state_daemon.go's
//	 defaultDaemonRun. ... The window between acquire and pid-write must
//	 remain bounded by a single state.WritePIDFile call — implementers
//	 MUST NOT insert other work between them."
//
// Body-list shape this test pins:
//
//	Body.List[i]   = *ast.AssignStmt   — lockFile, err := acquireDaemonLock(deps.Dir)
//	Body.List[i+1] = *ast.IfStmt       — if err != nil { ... return ... }
//	Body.List[i+2] = *ast.IfStmt       — if err := WritePIDFile(...); err != nil { ... }
//
// A regression that inserts ANY statement between i+1 and i+2 fails here
// with a diagnostic naming the intruding node's type and source line.
func TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire(t *testing.T) {
	src, err := os.ReadFile(stateDaemonSourcePath)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, stateDaemonSourcePath, src, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	fn := findFuncDecl(file, daemonRunFuncName)
	if fn == nil {
		t.Fatalf("function %q not found in %s — has it been renamed or moved?",
			daemonRunFuncName, stateDaemonSourcePath)
	}
	if fn.Body == nil {
		t.Fatalf("function %q has nil Body — cannot inspect statements", daemonRunFuncName)
	}

	acquireIdx := -1
	for i, stmt := range fn.Body.List {
		if isAssignCallTo(stmt, acquireDaemonLockIdent) {
			acquireIdx = i
			break
		}
	}
	if acquireIdx < 0 {
		t.Fatalf("no AssignStmt calling %q found in %s body",
			acquireDaemonLockIdent, daemonRunFuncName)
	}

	if acquireIdx+2 >= len(fn.Body.List) {
		t.Fatalf("body has insufficient statements after %q (idx=%d, len=%d) — "+
			"expected err-guard at i+1 and WritePIDFile if-stmt at i+2",
			acquireDaemonLockIdent, acquireIdx, len(fn.Body.List))
	}

	// i+1: err-guard if-stmt wrapping the acquireDaemonLock return.
	errGuard, ok := fn.Body.List[acquireIdx+1].(*ast.IfStmt)
	if !ok {
		got := fn.Body.List[acquireIdx+1]
		t.Fatalf("statement at index %d after %q is not an *ast.IfStmt; "+
			"got %T at line %d — the err-guard for the acquire call must be the "+
			"immediately-following statement",
			acquireIdx+1, acquireDaemonLockIdent, got, fset.Position(got.Pos()).Line)
	}
	if !ifStmtIsErrGuard(errGuard) {
		t.Fatalf("statement at index %d (line %d) is an *ast.IfStmt but does not "+
			"match the err-guard shape (`if err != nil { ... return ... }`)",
			acquireIdx+1, fset.Position(errGuard.Pos()).Line)
	}

	// i+2: WritePIDFile if-stmt (the `if err := WritePIDFile(...); err != nil { ... }`
	// shape OR a bare WritePIDFile call wrapped in any if-stmt that references it).
	writePIDStmt := fn.Body.List[acquireIdx+2]
	writePIDIfStmt, ok := writePIDStmt.(*ast.IfStmt)
	if !ok {
		t.Fatalf("statement at index %d after err-guard is not an *ast.IfStmt; "+
			"got %T at line %d — the spec mandates WritePIDFile is the next "+
			"statement after the acquire err-guard. Did a refactor insert "+
			"intermediate work between acquireDaemonLock's err-guard and WritePIDFile?",
			acquireIdx+2, writePIDStmt, fset.Position(writePIDStmt.Pos()).Line)
	}
	if !ifStmtContainsCallTo(writePIDIfStmt, writePIDFileIdent) {
		t.Fatalf("statement at index %d (line %d) is an *ast.IfStmt but does not "+
			"contain a call to %q. The spec mandates the acquire err-guard be "+
			"immediately followed by WritePIDFile — see specification.md § "+
			"Component C step 4.",
			acquireIdx+2, fset.Position(writePIDIfStmt.Pos()).Line, writePIDFileIdent)
	}
}

// TestAcquireDaemonLock_SingleProductionCallSite asserts that the production
// (non-test) source under cmd/ contains exactly ONE call site invoking
// either the package-level seam `acquireDaemonLock` OR the underlying
// `state.AcquireDaemonLock` directly. The single allowed call site is the
// one inside defaultDaemonRun pinned by Test 1.
//
// Spec contract (specification.md § Component C step 4):
//
//	"No other production call site of AcquireDaemonLock exists; the spec
//	 contract is 'production daemon's defaultDaemonRun only'."
//
// This test is the structural guard against a future refactor that adds a
// second caller. A second caller would bypass the ordering invariant
// pinned by Test 1 (which targets defaultDaemonRun specifically) and
// re-introduce the very race Component C exists to close.
func TestAcquireDaemonLock_SingleProductionCallSite(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read cmd dir: %v", err)
	}

	fset := token.NewFileSet()
	count := 0
	var locations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		file, err := parser.ParseFile(fset, name, src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callTargetMatches(call, acquireDaemonLockIdent) ||
				callSelectorMatches(call, "state", "AcquireDaemonLock") {
				count++
				locations = append(locations,
					name+":"+positionString(fset, call.Pos()))
			}
			return true
		})
	}

	if count != 1 {
		t.Errorf("expected exactly 1 production call site to acquireDaemonLock / "+
			"state.AcquireDaemonLock in cmd/; got %d at: %v\n\n"+
			"Spec § Component C step 4 mandates a single production call site "+
			"inside defaultDaemonRun. A second caller would bypass the "+
			"acquire+WritePIDFile adjacency check enforced by "+
			"TestDaemonAcquireLockOrdering_WritePIDFollowsAcquire and "+
			"re-introduce the race Component C closes.",
			count, locations)
	}
}

// findFuncDecl returns the top-level *ast.FuncDecl with the given name, or
// nil if not found. Only top-level FuncDecls are inspected — closures and
// nested funcs cannot host the daemon-startup ceremony.
func findFuncDecl(file *ast.File, name string) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if fn.Name != nil && fn.Name.Name == name {
			return fn
		}
	}
	return nil
}

// isAssignCallTo reports whether stmt is `<lhs...> := <ident>(<args...>)`
// where <ident> matches the supplied name. Used to find the acquire call's
// AssignStmt anchor for the adjacency check.
func isAssignCallTo(stmt ast.Stmt, name string) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok {
		return false
	}
	if len(assign.Rhs) != 1 {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	return callTargetMatches(call, name)
}

// callTargetMatches reports whether call's Fun is a bare *ast.Ident whose
// name equals the supplied name. Selector expressions (e.g. state.Foo) are
// matched by callSelectorMatches instead.
func callTargetMatches(call *ast.CallExpr, name string) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	return ident.Name == name
}

// callSelectorMatches reports whether call's Fun is a selector matching
// <pkg>.<sel>. Used to detect state.AcquireDaemonLock alongside the bare
// acquireDaemonLock seam in the call-site count.
func callSelectorMatches(call *ast.CallExpr, pkg, sel string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return x.Name == pkg && selector.Sel != nil && selector.Sel.Name == sel
}

// ifStmtIsErrGuard reports whether ifStmt is shaped like the canonical
// `if err != nil { ... return ... }` err-guard. The check is structural
// (Cond is a BinaryExpr comparing an identifier to nil) plus a return
// inside Body — tolerant of any wrapping content (log lines, error
// constructions) between the open brace and the return.
func ifStmtIsErrGuard(ifStmt *ast.IfStmt) bool {
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok {
		return false
	}
	// Left side is an identifier (typically "err"); right side is the nil ident.
	leftIdent, leftOk := bin.X.(*ast.Ident)
	rightIdent, rightOk := bin.Y.(*ast.Ident)
	if !leftOk || !rightOk {
		return false
	}
	if leftIdent.Name == "" || rightIdent.Name != "nil" {
		return false
	}
	// Body must contain a return somewhere.
	if ifStmt.Body == nil {
		return false
	}
	hasReturn := false
	ast.Inspect(ifStmt.Body, func(n ast.Node) bool {
		if _, ok := n.(*ast.ReturnStmt); ok {
			hasReturn = true
			return false
		}
		return true
	})
	return hasReturn
}

// ifStmtContainsCallTo reports whether the if-stmt (init, cond, or body)
// contains anywhere a *ast.CallExpr whose target is a bare ident with the
// given name OR a selector ending in that name. Tolerates both bare
// `WritePIDFile(...)` and `state.WritePIDFile(...)` shapes.
func ifStmtContainsCallTo(ifStmt *ast.IfStmt, name string) bool {
	found := false
	visit := func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		switch fun := call.Fun.(type) {
		case *ast.Ident:
			if fun.Name == name {
				found = true
				return false
			}
		case *ast.SelectorExpr:
			if fun.Sel != nil && fun.Sel.Name == name {
				found = true
				return false
			}
		}
		return true
	}
	if ifStmt.Init != nil {
		ast.Inspect(ifStmt.Init, visit)
	}
	if !found && ifStmt.Cond != nil {
		ast.Inspect(ifStmt.Cond, visit)
	}
	if !found && ifStmt.Body != nil {
		ast.Inspect(ifStmt.Body, visit)
	}
	return found
}

// positionString returns a human-readable "line:col" for diagnostics.
func positionString(fset *token.FileSet, pos token.Pos) string {
	p := fset.Position(pos)
	return strings.TrimPrefix(p.String(), p.Filename+":")
}
