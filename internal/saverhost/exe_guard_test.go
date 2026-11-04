package saverhost_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The saver's executable comes from the Ensurer's caller alone. Under go test
// os.Executable is the test binary, which run as `state daemon` in the saver
// session would run the whole suite there, outside the test's isolation.
func TestSaverhostNeverFindsAnExecutableOfItsOwn(t *testing.T) {
	runExecutableGuard(t, sourceguardtest.ParsePackageSources(t, ".", false))
}

func runExecutableGuard(t harnesstest.TestingT, sources []sourceguardtest.ParsedSource) {
	t.Helper()
	if len(sources) == 0 {
		t.Fatalf("the executable guard scanned no sources")
	}
	for _, source := range sources {
		local := osImportName(source.File)
		if local == "" {
			continue
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "Executable" {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == local {
				t.Errorf("%s references os.Executable: the daemon's executable is its caller's to give", source.Position(sel.Pos()))
			}
			return true
		})
	}
}

// osImportName is the name file refers to package os by, empty when it
// doesn't import it.
func osImportName(file *ast.File) string {
	for _, spec := range file.Imports {
		if path, err := strconv.Unquote(spec.Path.Value); err != nil || path != "os" {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		return "os"
	}
	return ""
}

func TestExecutableGuard_FailurePaths(t *testing.T) {
	parse := func(t *testing.T, src string) []sourceguardtest.ParsedSource {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", src, sourceguardtest.ParseMode)
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}
		return []sourceguardtest.ParsedSource{{Path: "fixture.go", Fset: fset, File: file}}
	}

	t.Run("it reports os.Executable, under any name os is imported by", func(t *testing.T) {
		for _, src := range []string{
			"package fixture\n\nimport \"os\"\n\nfunc f() { _, _ = os.Executable() }\n",
			"package fixture\n\nimport sys \"os\"\n\nvar find = sys.Executable\n",
		} {
			rec := &harnesstest.Recorder{}
			runExecutableGuard(rec, parse(t, src))
			if len(rec.Errors) != 1 {
				t.Errorf("errors = %q for\n%s\nwant one", rec.Errors, src)
			}
		}
	})

	t.Run("it passes another package's Executable", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		runExecutableGuard(rec, parse(t, "package fixture\n\nimport \"example.com/os\"\n\nvar find = os.Executable\n"))
		if len(rec.Errors) != 0 {
			t.Errorf("errors = %q, want none", rec.Errors)
		}
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runExecutableGuard(rec, nil) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})
}
