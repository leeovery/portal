package hooks_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The staleness rule has one implementation under one exported name, and every
// reader of staleness reaches it rather than restating it. An unexported twin
// beside it would be a second name for the same rule, and a caller reaching the
// twin would be a reader the exported name's callers cannot account for.
func TestStalenessRuleHasOneExportedFunction(t *testing.T) {
	t.Run("it applies the staleness rule through the single exported function from both callers", func(t *testing.T) {
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			for _, decl := range source.File.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Name.Name == "staleKeys" {
					t.Errorf("%s: an unexported staleKeys survives beside the exported StaleKeys", source.Fset.Position(fn.Pos()))
				}
			}
		}

		assertCallsStaleKeys(t, ".", "deleteStale")
		assertCallsStaleKeys(t, "../../cmd", "checkStaleHooks")
	})
}

// assertCallsStaleKeys fails unless the named function in the package at dir
// reaches the staleness rule by calling StaleKeys.
func assertCallsStaleKeys(t *testing.T, dir, funcName string) {
	t.Helper()

	var called bool
	for _, source := range sourceguardtest.ParsePackageSources(t, dir, false) {
		sourceguardtest.ForEachFuncCall(source.File, func(enclosing string, call *ast.CallExpr) bool {
			if enclosing == funcName && sourceguardtest.CalleeName(call) == "StaleKeys" {
				called = true
			}
			return true
		})
	}
	if !called {
		t.Errorf("%s does not call StaleKeys — every reader of staleness must reach the one exported function", funcName)
	}
}

// Every mutation takes the file under one exclusive hold, so each must reach
// the file through the unexported load/save — which do no locking of their own.
// Routing through a locking front door instead nests a second acquisition
// inside the hold the mutation already owns: a deadlock-shaped regression that
// degrades into a silent multi-second stall rather than failing a test. Neither
// side is named here — a mutation is whatever Store method takes the mutation
// lock, and a front door is whatever Store method reaches acquireLock — so a
// rename or a new method on either side is judged rather than slipped past.
func TestMutationsDoNotReenterALockingFrontDoor(t *testing.T) {
	verdict := judgeLockReentrancy(sourceguardtest.ParsePackageSources(t, ".", false))
	if verdict.mutations == 0 {
		t.Fatalf("no Store method calls %s — the guard is judging nothing", mutationAcquire)
	}
	if verdict.frontDoors == 0 {
		t.Fatalf("no Store method reaches %s — the guard has nothing to forbid", lockRoot)
	}
	for _, violation := range verdict.violations {
		t.Error(violation)
	}
}

const (
	// lockRoot is the one function every acquisition passes through; a method
	// reaching it, directly or through another method, is a locking front door.
	lockRoot = "acquireLock"
	// mutationAcquire is how a mutation takes its exclusive hold; the method
	// calling it is a mutation, and that call is the one front door it may use.
	mutationAcquire = "acquireMutationLock"
	// storeType is the type whose methods are judged.
	storeType = "Store"
)

type lockReentrancyVerdict struct {
	violations []string
	mutations  int
	frontDoors int
}

type storeMethod struct {
	source      sourceguardtest.ParsedSource
	receiver    string
	calls       []receiverCall
	reachesRoot bool
}

type receiverCall struct {
	name string
	pos  token.Pos
}

// judgeLockReentrancy reads every Store method off the sources, derives the
// front doors as the methods reaching lockRoot through any chain of receiver
// calls, and reports each mutation that calls one other than its own acquire.
func judgeLockReentrancy(sources []sourceguardtest.ParsedSource) lockReentrancyVerdict {
	methods := map[string]*storeMethod{}
	for _, source := range sources {
		for _, decl := range source.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || receiverTypeName(fn) != storeType {
				continue
			}
			method := &storeMethod{source: source, receiver: receiverName(fn)}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if sourceguardtest.CalleeName(call) == lockRoot {
					method.reachesRoot = true
				}
				if name, ok := receiverCallName(call, method.receiver); ok {
					method.calls = append(method.calls, receiverCall{name: name, pos: call.Pos()})
				}
				return true
			})
			methods[fn.Name.Name] = method
		}
	}

	locking := map[string]bool{}
	for name, method := range methods {
		locking[name] = method.reachesRoot
	}
	for changed := true; changed; {
		changed = false
		for name, method := range methods {
			if locking[name] {
				continue
			}
			for _, call := range method.calls {
				if locking[call.name] {
					locking[name] = true
					changed = true
					break
				}
			}
		}
	}

	var verdict lockReentrancyVerdict
	for _, isLocking := range locking {
		if isLocking {
			verdict.frontDoors++
		}
	}
	for _, name := range slices.Sorted(maps.Keys(methods)) {
		method := methods[name]
		if !slices.ContainsFunc(method.calls, func(c receiverCall) bool { return c.name == mutationAcquire }) {
			continue
		}
		verdict.mutations++
		for _, call := range method.calls {
			if call.name == mutationAcquire || !locking[call.name] {
				continue
			}
			verdict.violations = append(verdict.violations, fmt.Sprintf(
				"%s: %s calls %s.%s — a mutation must reach the file through the unexported load/save, never re-enter a locking front door",
				method.source.Position(call.pos), name, method.receiver, call.name))
		}
	}
	return verdict
}

// receiverTypeName reports the type a method is declared on, with a pointer
// receiver unwrapped, or "" for a plain function.
func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	ident, ok := expr.(*ast.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// receiverName reports the identifier a method binds its receiver to, or ""
// when the receiver is unnamed.
func receiverName(fn *ast.FuncDecl) string {
	if len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
}

// receiverCallName reports the method a call names on receiver ("Load" for
// s.Load()), and false for any call not made on that identifier.
func receiverCallName(call *ast.CallExpr, receiver string) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok || receiver == "" || ident.Name != receiver {
		return "", false
	}
	return sel.Sel.Name, true
}
