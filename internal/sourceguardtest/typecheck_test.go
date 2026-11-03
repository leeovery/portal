package sourceguardtest_test

import (
	"go/ast"
	"go/types"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestTypeChecker_ChecksThePackagesOwnSources(t *testing.T) {
	pkg := sourceguardtest.NewTypeChecker(t, ".").Package(t)

	if len(pkg.Sources) == 0 {
		t.Fatal("the package type-checked with no sources")
	}
	for _, source := range pkg.Sources {
		if source.File.Name.Name != "sourceguardtest" {
			t.Errorf("%s is package %s, want only the package's own non-test sources", source.Path, source.File.Name.Name)
		}
	}
}

func TestTypeChecker_ResolvesIdentifiersToTheirObjects(t *testing.T) {
	pkg := sourceguardtest.NewTypeChecker(t, ".").Sources(t, map[string]string{"fixture.go": `package fixture

import (
	"go/token"
	s "strings"
)

type wrapped struct{ *token.FileSet }

func f(w wrapped, file token.File) {
	w.Base()
	file.Base()
	_ = (*token.FileSet).Base
	_ = s.ToUpper
}
`})
	objects := map[string][]string{}
	for _, source := range pkg.Sources {
		ast.Inspect(source.File, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				if fn, ok := pkg.Object(ident).(*types.Func); ok {
					objects[ident.Name] = append(objects[ident.Name], fn.FullName())
				}
			}
			return true
		})
	}

	want := map[string][]string{
		"f":       {"fixture.f"},
		"Base":    {"(*go/token.FileSet).Base", "(*go/token.File).Base", "(*go/token.FileSet).Base"},
		"ToUpper": {"strings.ToUpper"},
	}
	for name, fullNames := range want {
		if got := objects[name]; !slices.Equal(got, fullNames) {
			t.Errorf("objects named %s = %v, want %v: a promoted method and a method expression resolve to the method itself", name, got, fullNames)
		}
	}
	for _, source := range pkg.Sources {
		ast.Inspect(source.File, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "ToUpper" && !pkg.IsPackageFunc(sel.Sel, "strings", "ToUpper") {
				t.Error("IsPackageFunc missed strings.ToUpper imported as s")
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Base" && pkg.IsPackageFunc(sel.Sel, "go/token", "Base") {
				t.Error("IsPackageFunc took a method for a package function")
			}
			return true
		})
	}
}

func TestTypeChecker_FatalsOnADirectoryHoldingNoPackage(t *testing.T) {
	stub := &recordingT{}

	if tc := sourceguardtest.NewTypeChecker(stub, t.TempDir()); tc != nil || !stub.fataled {
		t.Errorf("NewTypeChecker = %v, fatal = %v; want a fatal and no checker", tc, stub.fataled)
	}
}

func TestTypeChecker_FatalsOnASourceThatDoesNotTypeCheck(t *testing.T) {
	tc := sourceguardtest.NewTypeChecker(t, ".")
	stub := &recordingT{}

	tc.Sources(stub, map[string]string{"fixture.go": "package fixture\n\nvar _ = undefined\n"})

	if !stub.fataled {
		t.Error("a source that does not type-check passed")
	}
}
