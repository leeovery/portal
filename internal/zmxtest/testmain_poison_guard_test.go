package zmxtest_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
	"github.com/leeovery/portal/internal/zmxtest"
)

// Every TestMain poisons TMUX_TMPDIR, through os.Setenv with
// zmxtest.PoisonedTmpdir or its value written out: a test session has no
// TMUX, and its login shell finds the installed tmux and Portal first on
// PATH, so a plain tmux there, or any a test runs, would otherwise reach the
// developer's default server.
func TestEveryTestMainPoisonsTmpdir(t *testing.T) {
	t.Run("it finds every TestMain in the repository poisoning TMUX_TMPDIR", func(t *testing.T) {
		for _, missing := range scanTestMainsUnpoisoned(t) {
			t.Errorf("%s: TestMain doesn't set TMUX_TMPDIR to zmxtest.PoisonedTmpdir (%q)", missing, zmxtest.PoisonedTmpdir)
		}
	})

	t.Run("it reports each TestMain that sets no poison or another value", func(t *testing.T) {
		root := stageTestMains(t, map[string]string{
			"a/main_test.go":     `os.Setenv("TMUX_TMPDIR", zmxtest.PoisonedTmpdir)`,
			"b/main_test.go":     fmt.Sprintf(`errs := []error{os.Setenv("TMUX_TMPDIR", %q)}; _ = errs`, zmxtest.PoisonedTmpdir),
			"c/main_test.go":     `os.Setenv("ZMX_DIR", zmxtest.PoisonedDir)`,
			"d/main_test.go":     `os.Setenv("TMUX_TMPDIR", "/tmp")`,
			"e/main_test.go":     `os.Setenv("TMUX_TMPDIR", other.PoisonedTmpdir)`,
			"f/helper_test.go":   "",
			"g/not_a_test.go":    "",
			"h/testmain_test.go": `os.Unsetenv("TMUX_TMPDIR")`,
		})
		var rec harnesstest.Recorder
		var missing []string
		rec.Run(func() { missing = scanTestMainsUnpoisoned(&rec, sourceguardtest.Rooted(root)) })

		want := []string{"c/main_test.go:3", "d/main_test.go:3", "e/main_test.go:3", "h/testmain_test.go:3"}
		if rec.Failed() || !slices.Equal(missing, want) {
			t.Errorf("reported %q and %s, want %q", missing, rec.Report(), want)
		}
	})

	t.Run("it fails when it finds no TestMain", func(t *testing.T) {
		root := stageTestMains(t, map[string]string{"f/helper_test.go": ""})
		var rec harnesstest.Recorder
		rec.Run(func() { scanTestMainsUnpoisoned(&rec, sourceguardtest.Rooted(root)) })
		if len(rec.Fatals) != 1 {
			t.Errorf("a tree with no TestMain reported %s, want one fatal", rec.Report())
		}
	})
}

// scanTestMainsUnpoisoned returns, sorted, the position of each TestMain in
// the tree the options name that doesn't poison TMUX_TMPDIR. Finding no
// TestMain is fatal: the scan has stopped seeing what it guards.
func scanTestMainsUnpoisoned(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) []string {
	t.Helper()
	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.TestSources, opts...)
	var found int
	var missing []string
	for _, source := range sources {
		for _, decl := range source.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" || fn.Body == nil {
				continue
			}
			found++
			if !poisonsTmpdir(fn.Body) {
				pos := source.Position(fn.Pos())
				missing = append(missing, fmt.Sprintf("%s:%d", filepath.ToSlash(pos.Filename), pos.Line))
			}
		}
	}
	if found == 0 {
		t.Fatalf("no TestMain found; the scan no longer sees what it guards")
		return nil
	}
	slices.Sort(missing)
	return missing
}

// poisonsTmpdir reports whether body calls os.Setenv("TMUX_TMPDIR", v), v
// zmxtest.PoisonedTmpdir or a literal of its value.
func poisonsTmpdir(body *ast.BlockStmt) bool {
	var poisons bool
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 2 || !isSelector(call.Fun, "os", "Setenv") || stringLit(call.Args[0]) != "TMUX_TMPDIR" {
			return true
		}
		if stringLit(call.Args[1]) == zmxtest.PoisonedTmpdir || isSelector(call.Args[1], "zmxtest", "PoisonedTmpdir") {
			poisons = true
		}
		return true
	})
	return poisons
}

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg
}

func stringLit(expr ast.Expr) string {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return s
}

// stageTestMains writes a Go source at each path of files under a temp root:
// a TestMain whose body is the value, or, for an empty value, a plain test
// helper. A path not ending _test.go is no test source.
func stageTestMains(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for file, body := range files {
		src := "package p\n\n"
		if body == "" {
			src += "func helper() {}\n"
		} else {
			src += "func TestMain(m *testing.M) {\n\t" + body + "\n}\n"
		}
		at := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(at), 0o755); err != nil {
			t.Fatalf("stage %s: %v", file, err)
		}
		if err := os.WriteFile(at, []byte(src), 0o644); err != nil {
			t.Fatalf("stage %s: %v", file, err)
		}
	}
	return root
}
