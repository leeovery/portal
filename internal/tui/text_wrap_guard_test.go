package tui_test

import (
	"fmt"
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

const wrapHelperFile = "text_wrap.go"

func TestWrapsThroughOneImplementation(t *testing.T) {
	t.Run("it wraps through one implementation", func(t *testing.T) {
		var sites []string
		for _, source := range sourceguardtest.ParsePackageSources(t, ".", false) {
			ast.Inspect(source.File, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isAnsiWrapCall(call) {
					return true
				}
				sites = append(sites, fmt.Sprintf("%s:%d", filepath.Base(source.Path), source.Position(call.Pos()).Line))
				return true
			})
		}
		if len(sites) != 1 {
			t.Fatalf("ansi.Wrap is called at %d sites (%q); every wrapping screen reaches it through the one in %s", len(sites), sites, wrapHelperFile)
		}
		if !strings.HasPrefix(sites[0], wrapHelperFile+":") {
			t.Errorf("the single ansi.Wrap call is at %s, want it in %s", sites[0], wrapHelperFile)
		}
	})
}

func isAnsiWrapCall(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Wrap" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "ansi"
}
