package sourceguardtest_test

import (
	"go/ast"
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

func TestTypeChecker_ResolvesSelectionsByType(t *testing.T) {
	pkg := sourceguardtest.NewTypeChecker(t, ".").Sources(t, map[string]string{"fixture.go": `package fixture

import (
	"go/token"
	s "strings"
)

func f(fs *token.FileSet, file token.File, b *s.Builder) {
	fs.Base()
	file.Base()
	b.Len()
	_ = s.ToUpper
}
`})
	selections := map[string]*ast.SelectorExpr{}
	for _, source := range pkg.Sources {
		ast.Inspect(source.File, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					selections[x.Name+"."+sel.Sel.Name] = sel
				}
			}
			return true
		})
	}

	for _, tc := range []struct {
		sel                     string
		pkgPath, typeName, name string
		want                    bool
	}{
		{"fs.Base", "go/token", "FileSet", "Base", true},
		{"file.Base", "go/token", "File", "Base", true},
		{"file.Base", "go/token", "FileSet", "Base", false},
		{"b.Len", "go/token", "FileSet", "Len", false},
	} {
		if got := pkg.IsMethodOf(selections[tc.sel], tc.pkgPath, tc.typeName, tc.name); got != tc.want {
			t.Errorf("IsMethodOf(%s, %s.%s.%s) = %v, want %v", tc.sel, tc.pkgPath, tc.typeName, tc.name, got, tc.want)
		}
	}
	if !pkg.IsPackageFunc(selections["s.ToUpper"].Sel, "strings", "ToUpper") {
		t.Error("IsPackageFunc missed strings.ToUpper imported as s")
	}
	if pkg.IsPackageFunc(selections["fs.Base"].Sel, "go/token", "Base") {
		t.Error("IsPackageFunc took a method for a package function")
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
