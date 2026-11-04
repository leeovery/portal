package sourceguardtest_test

import (
	"go/ast"
	"go/types"
	"path/filepath"
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

func TestTypeChecker_ChecksTheBuildItIsGiven(t *testing.T) {
	root := sourceguardtest.ProjectRoot(t)
	sourceNames := func(pkg sourceguardtest.TypedPackage) []string {
		var names []string
		for _, source := range pkg.Sources {
			names = append(names, filepath.Base(source.Path))
		}
		return names
	}

	t.Run("ForGOOS checks that platform's files", func(t *testing.T) {
		names := sourceNames(sourceguardtest.NewTypeChecker(t, filepath.Join(root, "internal", "bootid"), sourceguardtest.ForGOOS("linux")).Package(t))

		if !slices.Contains(names, "bootid_linux.go") || slices.Contains(names, "bootid_darwin.go") {
			t.Errorf("sources = %v, want linux's file and not darwin's", names)
		}
	})

	t.Run("WithBuildTags checks the files behind the tag", func(t *testing.T) {
		dir := filepath.Join(root, "internal", "state")
		tagged := sourceNames(sourceguardtest.NewTypeChecker(t, dir, sourceguardtest.WithBuildTags(sourceguardtest.IntegrationTag)).Package(t))
		untagged := sourceNames(sourceguardtest.NewTypeChecker(t, dir).Package(t))

		if !slices.Contains(tagged, "pgrep_sandbox.go") || slices.Contains(untagged, "pgrep_sandbox.go") {
			t.Errorf("pgrep_sandbox.go checked with the tag = %v, without = %v; want only with it",
				slices.Contains(tagged, "pgrep_sandbox.go"), slices.Contains(untagged, "pgrep_sandbox.go"))
		}
	})
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
