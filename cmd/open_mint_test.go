package cmd

import (
	"go/ast"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

// A session name's suffix is the general-purpose nanoid. The pane-token
// generator is hooks.json's key shape, free to change width on its own.
func TestMintDrawsSuffixesFromTheGeneralPurposeGenerator(t *testing.T) {
	source := sourceguardtest.PackageSource(t, ".", "open_mint.go")
	fn := findFuncDeclInFile(t, source.File, "newIndexingCreator")

	var generators []string
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "nanoid" {
					generators = append(generators, sel.Sel.Name)
				}
			}
		}
		return true
	})
	if len(generators) != 1 || generators[0] != "NewGenerator" {
		t.Errorf("newIndexingCreator draws on nanoid.%v, want NewGenerator alone", generators)
	}
}
