package tui_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// centralisedColourSites are the production files re-pointed onto the §2.9
// role-token layer in task 1-3. After re-pointing, none may construct a colour
// from a raw hex / ANSI-index literal — every colour flows from a theme token.
// This is the §2.8 "closed vocabulary, no literal hex at call sites" rule made
// executable, modelled on internal/log's single-owner discard guard.
//
// As later phases re-point more renderers onto the token layer, add their files
// here so the guard's coverage grows with the migration.
var centralisedColourSites = []string{
	"session_item.go",
	"project_item.go",
	"model.go",
}

// TestNoRawColourLiteralAtCentralisedSites parses each re-pointed file and fails
// if any lipgloss.Color(...) call is passed a raw string/int literal (a hex like
// "#777777" or an ANSI index like "212"/76). The only sanctioned home for raw
// colour values is internal/tui/theme — call sites must reference a token.
func TestNoRawColourLiteralAtCentralisedSites(t *testing.T) {
	for _, name := range centralisedColourSites {
		name := name
		t.Run(name, func(t *testing.T) {
			fset := token.NewFileSet()
			path := filepath.Join(".", name)
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", name, err)
			}

			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if !isLipglossColorCall(call) {
					return true
				}
				if len(call.Args) != 1 {
					return true
				}
				lit, ok := call.Args[0].(*ast.BasicLit)
				if !ok {
					// Non-literal argument (a token reference, a variable) is fine.
					return true
				}
				if lit.Kind == token.STRING || lit.Kind == token.INT {
					raw := lit.Value
					if lit.Kind == token.STRING {
						if unq, uerr := strconv.Unquote(lit.Value); uerr == nil {
							raw = unq
						}
					}
					pos := fset.Position(lit.Pos())
					t.Errorf("%s:%d constructs lipgloss.Color(%q) from a raw colour literal; reference an internal/tui/theme token instead", name, pos.Line, raw)
				}
				return true
			})
		})
	}
}

// isLipglossColorCall reports whether call is `lipgloss.Color(...)`.
func isLipglossColorCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	return pkg.Name == "lipgloss" && sel.Sel.Name == "Color" && !strings.Contains(pkg.Name, "_")
}
