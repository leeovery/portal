package portaltest_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The calls and the env var the rule is expressed in.
const (
	isolateCall  = "portaltest.IsolateStateForTest"
	serverCall   = "tmuxtest.New"
	guardCall    = "portaltest.RegisterStateDirTeardownGuard"
	stateDirEnv  = "PORTAL_STATE_DIR"
	guardPIDCall = "portaltest.RegisterStateDirTeardownGuardWithPIDSource"
)

// fixtureCalls records where one function makes the calls the rule is expressed
// in. A line of zero means the scope does not make that call at all; a recorded
// line is the first such call, and lines are only comparable against each other
// within the one scope.
type fixtureCalls struct {
	NamesStateDir bool
	ServerLine    int
	IsolateLine   int
	GuardLine     int
	LocalCalls    []localCall
}

// localCall is one unqualified call to a function of the fixture's own package
// — the single hop the rule follows to find an arrange that starts the server,
// isolates, or registers the guard on the caller's behalf.
type localCall struct {
	Name string
	Line int
}

// scannedFunc is one function of one file, carrying the package it resolves its
// local calls within. Its calls are recorded one record per lexical scope — the
// declaration's own body and each function literal within it — because line
// order only says what ran first inside a single scope: sibling closures run in
// whatever order their caller invokes them.
type scannedFunc struct {
	Pkg    string
	File   string
	Name   string
	Scopes []fixtureCalls
}

// aggregate flattens the function's scopes into what it does anywhere, which is
// how presence — as against order — is judged.
func (f scannedFunc) aggregate() fixtureCalls {
	var all fixtureCalls
	earliest := func(slot *int, line int) {
		if line != 0 && (*slot == 0 || line < *slot) {
			*slot = line
		}
	}
	for _, scope := range f.Scopes {
		all.NamesStateDir = all.NamesStateDir || scope.NamesStateDir
		earliest(&all.ServerLine, scope.ServerLine)
		earliest(&all.IsolateLine, scope.IsolateLine)
		earliest(&all.GuardLine, scope.GuardLine)
		all.LocalCalls = append(all.LocalCalls, scope.LocalCalls...)
	}
	return all
}

// qualifies reports whether the function is subject to the rule: it drives a
// live tmux server against a state directory, whether it named that directory
// itself or took one from the isolation helper.
func (c fixtureCalls) qualifies() bool {
	return c.ServerLine != 0 && (c.NamesStateDir || c.IsolateLine != 0)
}

// presenceDefect renders the calls the function never makes, or "" when it makes
// them all.
func (c fixtureCalls) presenceDefect() string {
	switch {
	case c.IsolateLine == 0 && c.GuardLine == 0:
		return fmt.Sprintf("starts a tmux server at line %d, calling neither %s nor %s", c.ServerLine, isolateCall, guardCall)
	case c.IsolateLine == 0:
		return fmt.Sprintf("starts a tmux server at line %d without calling %s", c.ServerLine, isolateCall)
	case c.GuardLine == 0:
		return fmt.Sprintf("starts a tmux server at line %d without calling %s", c.ServerLine, guardCall)
	}
	return ""
}

// orderDefect renders an inverted order within one scope, or "" when the order
// holds or the scope makes too few of the calls to judge. Order is part of the
// rule, not a nicety: cleanups run LIFO, so a guard registered after the server
// runs its quiescence wait before kill-server rather than between kill-server
// and the TempDir RemoveAll it protects.
func (c fixtureCalls) orderDefect() string {
	if c.ServerLine == 0 {
		return ""
	}
	switch {
	case c.IsolateLine != 0 && c.ServerLine < c.IsolateLine:
		return fmt.Sprintf("starts a tmux server at line %d before isolating at line %d", c.ServerLine, c.IsolateLine)
	case c.GuardLine != 0 && c.ServerLine < c.GuardLine:
		return fmt.Sprintf("starts a tmux server at line %d before registering the teardown guard at line %d", c.ServerLine, c.GuardLine)
	}
	return ""
}

// throughLocalArrange folds in the server start, the isolation and the guard
// registration a same-package arrange makes on the caller's behalf, each
// attributed to the line the caller calls that arrange on. All three fold, so
// neither half of the pairing can leave the rule's reach by moving into a
// helper. Exactly one hop is followed: an arrange that delegates further is not
// chased.
func (c fixtureCalls) throughLocalArrange(pkg map[string]fixtureCalls) fixtureCalls {
	for _, call := range c.LocalCalls {
		arrange, known := pkg[call.Name]
		if !known {
			continue
		}
		if c.ServerLine == 0 && arrange.ServerLine != 0 {
			c.ServerLine = call.Line
		}
		if c.IsolateLine == 0 && arrange.IsolateLine != 0 {
			c.IsolateLine = call.Line
		}
		if c.GuardLine == 0 && arrange.GuardLine != 0 {
			c.GuardLine = call.Line
		}
	}
	return c
}

// TestTeardownGuardCoversEveryServerHostingFixture polices the rule that a test
// function pairing a state directory with a live tmux server must isolate that
// directory and register the teardown guard over it, both before it starts the
// server. Such a server can host writers into the state dir — a saver daemon's
// SIGHUP flush, a session-closed hook subprocess, a hydrate helper — and those
// outlive kill-server, so without the guard the TempDir RemoveAll races them and
// fails the test with "directory not empty" after its assertions have already
// passed. Without the isolation the fixture is writing somewhere it does not own
// at all, and with the calls made in the wrong order the guard waits at the
// wrong moment, which is the same as not having one.
//
// The state directory is a trigger in its own right, alongside the isolation
// call, because a trigger keyed only on the helper can fire on files that
// already opted in — and skipping that call is exactly the defect worth
// catching.
//
// The pairing is judged per function, with one hop into a same-package arrange,
// so routing a suite through a shared setup is judged rather than skipped. What
// stays out of reach is an arrange in another package; an inversion split
// across two scopes (order is compared within a single scope, because sibling
// closures run in whatever order their caller invokes them); and an inversion
// sealed inside one same-package arrange that neither isolates nor names the
// state directory — such an arrange does not qualify on its own, and the fold
// attributes both its server start and its guard registration to the single
// line the caller reaches it on, so the two compare equal and the order goes
// unjudged while both calls still read as present.
func TestTeardownGuardCoversEveryServerHostingFixture(t *testing.T) {
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.TestSources)

	var funcs []scannedFunc
	for _, source := range sources {
		rel := source.Path
		pkg, calls := fixtureCallsIn(source.Fset, source.File)
		for name, scopes := range calls {
			funcs = append(funcs, scannedFunc{
				Pkg:    filepath.Dir(rel) + ":" + pkg,
				File:   rel,
				Name:   name,
				Scopes: scopes,
			})
		}
	}

	if msg := coverageFailure(auditFixtureCoverage(funcs)); msg != "" {
		t.Fatal(msg)
	}
}

// fixtureCallsIn scans one parsed file, returning its package name and one
// record per lexical scope of each function declaration it holds. Build tags are
// ignored on purpose: the integration-tagged files are the whole subject, and
// the unit lane must still police them.
func fixtureCallsIn(fset *token.FileSet, file *ast.File) (pkg string, funcs map[string][]fixtureCalls) {
	funcs = make(map[string][]fixtureCalls)
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		funcs[declKey(fn)] = scopesIn(fset, fn.Body)
	}
	return file.Name.Name, funcs
}

// declKey names a declaration within its file's record set. A method is keyed
// under its receiver type as well as its own name, so it neither collides with
// a plain function of the same name nor answers a local call, which names one.
func declKey(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

// receiverName renders a receiver type as its bare name, seeing through the
// pointer and type-parameter wrappers a declaration may spell it with.
func receiverName(expr ast.Expr) string {
	switch typ := expr.(type) {
	case *ast.StarExpr:
		return receiverName(typ.X)
	case *ast.IndexExpr:
		return receiverName(typ.X)
	case *ast.IndexListExpr:
		return receiverName(typ.X)
	case *ast.Ident:
		return typ.Name
	}
	return "?"
}

// scopesIn records body's own calls, then those of each function literal within
// it, one record per scope and each in source order.
func scopesIn(fset *token.FileSet, body *ast.BlockStmt) []fixtureCalls {
	var calls fixtureCalls
	var nested []*ast.FuncLit
	record := func(slot *int, pos token.Pos) {
		if *slot == 0 {
			*slot = fset.Position(pos).Line
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		if lit, isLiteral := n.(*ast.FuncLit); isLiteral {
			nested = append(nested, lit)
			return false
		}
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING && strings.Contains(node.Value, stateDirEnv) {
				calls.NamesStateDir = true
			}
		case *ast.CallExpr:
			switch selectorName(node) {
			case serverCall:
				record(&calls.ServerLine, node.Pos())
			case isolateCall:
				record(&calls.IsolateLine, node.Pos())
			case guardCall, guardPIDCall:
				record(&calls.GuardLine, node.Pos())
			case "":
				if name := localCallName(node); name != "" {
					calls.LocalCalls = append(calls.LocalCalls, localCall{Name: name, Line: fset.Position(node.Pos()).Line})
				}
			}
		}
		return true
	})

	scopes := []fixtureCalls{calls}
	for _, lit := range nested {
		scopes = append(scopes, scopesIn(fset, lit.Body)...)
	}
	return scopes
}

// selectorName renders a call as <package>.<func>, or "" when it is not one.
func selectorName(call *ast.CallExpr) string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return pkg.Name + "." + sel.Sel.Name
}

// localCallName renders an unqualified call as its own name, or "" for any
// other callee shape.
func localCallName(call *ast.CallExpr) string {
	ident, isIdent := call.Fun.(*ast.Ident)
	if !isIdent {
		return ""
	}
	return ident.Name
}

// auditFixtureCoverage returns how many functions the rule applied to and the
// sorted descriptions of those failing it.
func auditFixtureCoverage(funcs []scannedFunc) (scanned int, defects []string) {
	arranges := arrangesByPackage(funcs)
	for _, fn := range funcs {
		calls := fn.aggregate().throughLocalArrange(arranges[fn.Pkg])
		if !calls.qualifies() {
			continue
		}
		scanned++
		if reason := firstDefect(fn, calls, arranges[fn.Pkg]); reason != "" {
			defects = append(defects, fmt.Sprintf("%s %s: %s", fn.File, fn.Name, reason))
		}
	}
	sort.Strings(defects)
	return scanned, defects
}

// firstDefect reports what one qualifying function gets wrong, given the
// aggregate its caller already resolved: a call it never makes anywhere, else
// the first scope making them in the wrong order.
func firstDefect(fn scannedFunc, aggregate fixtureCalls, arranges map[string]fixtureCalls) string {
	if reason := aggregate.presenceDefect(); reason != "" {
		return reason
	}
	for _, scope := range fn.Scopes {
		if reason := scope.throughLocalArrange(arranges).orderDefect(); reason != "" {
			return reason
		}
	}
	return ""
}

// arrangesByPackage indexes every scanned function by package and name, so a
// caller's unqualified call can be resolved to what that callee does.
func arrangesByPackage(funcs []scannedFunc) map[string]map[string]fixtureCalls {
	byPkg := make(map[string]map[string]fixtureCalls)
	for _, fn := range funcs {
		if byPkg[fn.Pkg] == nil {
			byPkg[fn.Pkg] = make(map[string]fixtureCalls)
		}
		byPkg[fn.Pkg][fn.Name] = fn.aggregate()
	}
	return byPkg
}

// coverageFailure renders the guard's failure, or "" when the tree passes.
func coverageFailure(scanned int, defects []string) string {
	if scanned == 0 {
		return fmt.Sprintf("no function pairs %s with either %s or %s; the guard has stopped looking rather than passed",
			serverCall, stateDirEnv, isolateCall)
	}
	if len(defects) == 0 {
		return ""
	}
	return fmt.Sprintf("%d of %d state-dir fixtures leave their state dir unguarded:\n  %s\n"+
		"  a server can host writers into that state dir past kill-server, so the TempDir\n"+
		"  RemoveAll races them; isolate the dir and register the guard, both before %s",
		len(defects), scanned, strings.Join(defects, "\n  "), serverCall)
}
