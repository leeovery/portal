package tui_test

import (
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const (
	restoreFileName   = "restore.go"
	restoreHelperName = "RestoreTerminalBackground"
)

// Spelled in halves so this file does not itself contain the string it forbids
// tree-wide — do not join it.
var retiredCanvasHexHelper = "canvasHex" + "For"

var restoreComparisonReads = []string{
	"OriginalBackground",
	"colourless",
	"themeState.startupCanvasHex",
}

const restoreAnchorRead = "themeState.startupCanvasHex"

var restoreLaunchSites = []string{
	filepath.Join("cmd", "open.go"),
	filepath.Join("cmd", "capturetool", "main.go"),
}

func TestRestorePath_ReadsNoTheme(t *testing.T) {
	file := sourceguardtest.PackageSource(t, ".", restoreFileName).File

	t.Run("the exit path imports no theme package", func(t *testing.T) {
		for _, imp := range file.Imports {
			if strings.Contains(strings.Trim(imp.Path.Value, `"`), "internal/theme") {
				t.Errorf("%s imports %s; the exit-time restore compares against the RETAINED startup hex, so it needs no theme at all",
					restoreFileName, imp.Path.Value)
			}
		}
	})

	t.Run("the comparison reads only the retained startup hex", func(t *testing.T) {
		fn := restoreHelperDecl(t, file)
		model := modelParamName(t, fn)
		anchored := false

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			path, dotted := selectorPath(sel)
			if !dotted {
				return true
			}
			root, rest, _ := strings.Cut(path, ".")
			switch root {
			case "theme":
				t.Errorf("%s reads %s; the comparison must never be re-derived from a theme at exit",
					restoreHelperName, path)
			case model:
				if !slices.Contains(restoreComparisonReads, rest) {
					t.Errorf("%s reads %s; the only permitted reads are %s.{%s}",
						restoreHelperName, path, model, strings.Join(restoreComparisonReads, ", "))
				}
				if rest == restoreAnchorRead {
					anchored = true
				}
			}
			// The whole path has been judged; descending would re-judge a
			// permitted nested read as its bare prefix.
			return false
		})

		if !anchored {
			t.Errorf("%s never reads %s.%s; the guard above would pass vacuously for a helper that compares against nothing at all",
				restoreHelperName, model, restoreAnchorRead)
		}
	})

	t.Run("the retired canvas-hex helper is gone from the tree", func(t *testing.T) {
		root, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources)
		for _, source := range sources {
			body, err := os.ReadFile(filepath.Join(root, source.Path))
			if err != nil {
				t.Fatalf("read %s: %v", source.Path, err)
			}
			if strings.Contains(string(body), retiredCanvasHexHelper) {
				t.Errorf("%s still mentions %s; it is deleted outright so nothing can re-derive the comparison from a theme",
					source.Path, retiredCanvasHexHelper)
			}
		}
	})
}

func TestLaunchSites_RestoreIdentically(t *testing.T) {
	sites := restoreCallSites(t)

	for _, rel := range restoreLaunchSites {
		t.Run(rel, func(t *testing.T) {
			args, called := sites[rel]
			if !called {
				t.Fatalf("%s no longer calls %s after its program returns", rel, restoreHelperName)
			}
			if len(args) != 1 {
				t.Fatalf("%s reaches %s with %d writers (%s), want exactly 1 — one launch site, one writer, however many hops it takes to get there",
					rel, restoreHelperName, len(args), strings.Join(args, ", "))
			}
			if args[0] != "os.Stdout" {
				t.Errorf("%s calls %s(%s, ...); want os.Stdout — both sites must write to the program's output",
					rel, restoreHelperName, args[0])
			}
		})
	}

	t.Run("no third site", func(t *testing.T) {
		known := map[string]bool{}
		for _, rel := range restoreLaunchSites {
			known[rel] = true
		}
		for rel := range sites {
			if !known[rel] {
				t.Errorf("%s also calls %s; the restore is a two-site contract, and a third site is a place the behaviour can diverge",
					rel, restoreHelperName)
			}
		}
	})
}

func restoreHelperDecl(t *testing.T, file *ast.File) *ast.FuncDecl {
	t.Helper()
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Recv == nil && fn.Name.Name == restoreHelperName {
			return fn
		}
	}
	t.Fatalf("%s declares no %s", restoreFileName, restoreHelperName)
	return nil
}

func modelParamName(t *testing.T, fn *ast.FuncDecl) string {
	t.Helper()
	for _, param := range fn.Type.Params.List {
		ident, ok := param.Type.(*ast.Ident)
		if !ok || ident.Name != "Model" || len(param.Names) != 1 {
			continue
		}
		return param.Names[0].Name
	}
	t.Fatalf("%s takes no Model parameter", restoreHelperName)
	return ""
}

func restoreCallSites(t *testing.T) map[string][]string {
	t.Helper()
	sites := map[string][]string{}
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources)

	pkgFiles := map[string][]*ast.File{}
	for _, source := range sources {
		dir := filepath.Dir(source.Path)
		pkgFiles[dir] = append(pkgFiles[dir], source.File)
	}

	for _, source := range sources {
		rel := source.Path
		siblings := pkgFiles[filepath.Dir(rel)]
		// The whole file is walked, not its function bodies: a call hides just as
		// well in a package-level initializer, and cmd declares seams that way.
		ast.Inspect(source.File, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !callsRestoreHelper(call) || len(call.Args) == 0 {
				return true
			}
			sites[rel] = append(sites[rel], writerArgs(siblings, enclosingFunc(source.File, call), call.Args[0])...)
			return true
		})
	}
	return sites
}

// enclosingFunc is the declared function whose body the call sits in, or nil
// for one outside every function — a package-level initializer, say.
func enclosingFunc(file *ast.File, call *ast.CallExpr) *ast.FuncDecl {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok && fn.Pos() <= call.Pos() && call.End() <= fn.End() {
			return fn
		}
	}
	return nil
}

// writerArgs names the writer a restore call resolves to. A writer the enclosing
// function was handed is resolved one hop out, to whatever its own callers in
// the same package pass in that position — so routing the restore through a
// helper that takes the writer as a parameter keeps every launch site under the
// rule instead of hiding it behind the parameter's name, a second route in a
// sibling file included. The hop goes by name and refuses a parameter the
// function assigns to; a same-named binding introduced any other way — a nested
// var, a range clause, a closure parameter — resolves to the caller's argument
// all the same. An unresolvable hop yields the parameter name itself, which
// fails the assertion rather than vanishing.
func writerArgs(pkg []*ast.File, enclosing *ast.FuncDecl, writer ast.Expr) []string {
	ident, ok := writer.(*ast.Ident)
	if !ok || enclosing == nil {
		return []string{exprText(writer)}
	}
	index, isParam := paramIndex(enclosing, ident.Name)
	if !isParam {
		return []string{exprText(writer)}
	}
	if assignsTo(enclosing.Body, ident.Name) {
		return []string{ident.Name}
	}

	var passed []string
	for _, file := range pkg {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || sourceguardtest.CalleeName(call) != enclosing.Name.Name || len(call.Args) <= index {
				return true
			}
			passed = append(passed, exprText(call.Args[index]))
			return true
		})
	}
	if len(passed) == 0 {
		return []string{ident.Name}
	}
	return passed
}

// One scan covers `=` and `:=` alike.
func assignsTo(body *ast.BlockStmt, name string) bool {
	assigned := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name == name {
				assigned = true
			}
		}
		return true
	})
	return assigned
}

func paramIndex(fn *ast.FuncDecl, name string) (int, bool) {
	index := 0
	for _, param := range fn.Type.Params.List {
		for _, ident := range param.Names {
			if ident.Name == name {
				return index, true
			}
			index++
		}
	}
	return 0, false
}

func callsRestoreHelper(call *ast.CallExpr) bool {
	return sourceguardtest.CalleeName(call) == restoreHelperName
}

func selectorPath(sel *ast.SelectorExpr) (string, bool) {
	switch x := sel.X.(type) {
	case *ast.Ident:
		return x.Name + "." + sel.Sel.Name, true
	case *ast.SelectorExpr:
		prefix, ok := selectorPath(x)
		if !ok {
			return "", false
		}
		return prefix + "." + sel.Sel.Name, true
	default:
		return "", false
	}
}

func exprText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprText(e.X) + "." + e.Sel.Name
	default:
		return fmt.Sprintf("%T", expr)
	}
}
