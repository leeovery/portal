package migrate_test

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// Only portal migrate knows tmux: its own client runs the binary, and
// tmuxtest runs it for migrate's tests. Nothing else may, so a tmux call
// creeping back in fails here rather than reaching the developer's server.
const (
	tmuxProgram = "tmux"
	execPkgPath = "os/exec"
	// migrateTmuxFile is the one production source that runs tmux.
	migrateTmuxFile = "internal/migrate/tmux.go"
	// tmuxtestDir is the harness that runs tmux for migrate's tests.
	tmuxtestDir = "internal/tmuxtest"
)

// execRunners are os/exec's functions that name a program to run or find.
var execRunners = []string{"Command", "CommandContext", "LookPath"}

// tmuxRun is a call that names the tmux program to os/exec.
type tmuxRun struct {
	file string
	line int
}

func (r tmuxRun) String() string { return fmt.Sprintf("%s:%d", r.file, r.line) }

func TestOnlyMigrateRunsTmux(t *testing.T) {
	t.Run("it finds tmux run only by migrate's client and tmuxtest", func(t *testing.T) {
		root, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources)
		runs, unresolved := tmuxByLiteral(sources)
		for _, dir := range unresolved {
			for _, lane := range sourceguardtest.Lanes() {
				runs = append(runs, tmuxByConstant(typedPackage(t, root, dir, lane))...)
			}
		}
		for _, run := range judgeTmuxRuns(t, runs) {
			t.Errorf("%s runs tmux; only portal migrate's client (%s) and %s may", run, migrateTmuxFile, tmuxtestDir)
		}
	})

	t.Run("it finds tmux named by a literal to each runner, under any import name", func(t *testing.T) {
		const src = `package x

import (
	"context"
	"os/exec"
	run "os/exec"
)

func a() { _ = exec.Command("tmux", "ls") }
func b(ctx context.Context) { _ = exec.CommandContext(ctx, "tmux") }
func c() { _, _ = exec.LookPath("tmux") }
func d() { _ = run.Command("/opt/homebrew/bin/tmux") }
func e() { _ = exec.Command("zmx", "tmux") }
func f(ctx context.Context) { _ = exec.CommandContext(ctx, "sh", "-c", "tmux") }
func g(p string) { _ = exec.Command(p) }
`
		runs, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x.go", src)})

		want := []string{"internal/x/x.go:9", "internal/x/x.go:10", "internal/x/x.go:11", "internal/x/x.go:12"}
		if got := runStrings(runs); !slices.Equal(got, want) {
			t.Errorf("tmuxByLiteral found %q, want %q", got, want)
		}
		if !slices.Equal(unresolved, []string{"internal/x"}) {
			t.Errorf("tmuxByLiteral left %q to the type checker, want the directory of the call naming a variable", unresolved)
		}
	})

	t.Run("it finds no run in a call that is not os/exec's", func(t *testing.T) {
		const src = `package x

import exec "example.com/notexec"

func a() { _ = exec.Command("tmux") }
`
		if runs, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x.go", src)}); len(runs) != 0 || len(unresolved) != 0 {
			t.Errorf("tmuxByLiteral found %v and left %q, want nothing: the package is not os/exec", runs, unresolved)
		}
	})

	t.Run("it leaves a test's variable to no one, the type checker reading production alone", func(t *testing.T) {
		const src = `package x

import "os/exec"

func a(p string) { _ = exec.Command(p) }
`
		if _, unresolved := tmuxByLiteral([]sourceguardtest.ParsedSource{parseFixture(t, "internal/x/x_test.go", src)}); len(unresolved) != 0 {
			t.Errorf("tmuxByLiteral left %q to the type checker, want nothing for a test source", unresolved)
		}
	})

	checker := sourceguardtest.NewTypeChecker(t, ".")
	t.Run("it finds tmux named by a constant", func(t *testing.T) {
		const src = `package migrate

import (
	"context"
	"os/exec"
)

const program = "tmux"

func a() { _ = exec.Command(program) }
func b(ctx context.Context) { _ = exec.CommandContext(ctx, program) }
func c(p string) { _ = exec.Command(p) }
func d() { _ = exec.Command("tmux") }
`
		const file = "internal/x/x.go"
		got := runStrings(tmuxByConstant(checker.Sources(t, map[string]string{file: src})))
		if want := []string{file + ":10", file + ":11"}; !slices.Equal(got, want) {
			t.Errorf("tmuxByConstant found %q, want %q: the constants alone, a literal being tmuxByLiteral's", got, want)
		}
	})

	t.Run("it passes runs in migrate's client and tmuxtest", func(t *testing.T) {
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = judgeTmuxRuns(&rec, allowedRuns()) })
		if rec.Failed() || len(found) != 0 {
			t.Errorf("judgeTmuxRuns reported %v and %s, want nothing", found, rec.Report())
		}
	})

	t.Run("it reports a run anywhere else, once", func(t *testing.T) {
		stray := tmuxRun{file: "cmd/open.go", line: 7}
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() { found = judgeTmuxRuns(&rec, append(allowedRuns(), stray, stray)) })
		if rec.Failed() || !slices.Equal(found, []string{stray.String()}) {
			t.Errorf("judgeTmuxRuns reported %v and %s, want %s alone", found, rec.Report(), stray)
		}
	})

	for _, owner := range []string{migrateTmuxFile, tmuxtestDir} {
		t.Run("it fails when it finds no run in "+owner, func(t *testing.T) {
			runs := slices.DeleteFunc(allowedRuns(), func(r tmuxRun) bool { return strings.HasPrefix(r.file, owner) })
			var rec harnesstest.Recorder
			rec.Run(func() { judgeTmuxRuns(&rec, runs) })
			if len(rec.Fatals) != 1 {
				t.Errorf("judgeTmuxRuns without a run in %s reported %s, want one fatal", owner, rec.Report())
			}
		})
	}
}

func TestOnlyCmdMigrateBuildsTheTmuxClient(t *testing.T) {
	t.Run("it finds migrate.Tmux named outside migrate only by cmd/migrate.go", func(t *testing.T) {
		_, sources := sourceguardtest.RepoSources(t, sourceguardtest.NonTestSources)
		for _, found := range judgeTmuxClientBuilders(t, sources) {
			t.Errorf("%s names migrate.Tmux; only cmd/migrate.go builds portal migrate's tmux client", found)
		}
	})

	const names = `package x

import m "github.com/leeovery/portal/internal/migrate"

var _ m.TmuxServer = m.Tmux{}
`
	t.Run("it reports migrate.Tmux under any import name outside migrate and cmd/migrate.go", func(t *testing.T) {
		var rec harnesstest.Recorder
		var found []string
		rec.Run(func() {
			found = judgeTmuxClientBuilders(&rec, []sourceguardtest.ParsedSource{
				parseFixture(t, tmuxClientBuilder, names),
				parseFixture(t, "cmd/open.go", names),
				parseFixture(t, "internal/migrate/run.go", names),
			})
		})
		if rec.Failed() || !slices.Equal(found, []string{"cmd/open.go:5"}) {
			t.Errorf("judgeTmuxClientBuilders reported %v and %s, want cmd/open.go:5 alone", found, rec.Report())
		}
	})

	t.Run("it fails when cmd/migrate.go no longer names migrate.Tmux", func(t *testing.T) {
		var rec harnesstest.Recorder
		rec.Run(func() {
			judgeTmuxClientBuilders(&rec, []sourceguardtest.ParsedSource{parseFixture(t, "cmd/open.go", names)})
		})
		if len(rec.Fatals) != 1 {
			t.Errorf("judgeTmuxClientBuilders without cmd/migrate.go reported %s, want one fatal", rec.Report())
		}
	})
}

// tmuxByLiteral returns each call in sources that names the tmux program to
// os/exec as a string literal, by path or bare name, and the directory of
// each production source with a call naming its program any other way, which
// only the type checker can resolve. It reads every source, whatever its
// build tags or platform. A test source's program is judged as written: the
// type checker reads production sources alone.
func tmuxByLiteral(sources []sourceguardtest.ParsedSource) (runs []tmuxRun, unresolved []string) {
	for _, source := range sources {
		names := importNames(source.File, execPkgPath)
		if len(names) == 0 {
			continue
		}
		file := filepath.ToSlash(source.Path)
		ast.Inspect(source.File, func(n ast.Node) bool {
			program, ok := execProgram(n, func(sel *ast.SelectorExpr) bool {
				pkg, ok := sel.X.(*ast.Ident)
				return ok && slices.Contains(names, pkg.Name)
			})
			if !ok {
				return true
			}
			lit, isLit := program.(*ast.BasicLit)
			switch {
			case isLit && namesTmux(lit):
				runs = append(runs, tmuxRun{file: file, line: source.Position(program.Pos()).Line})
			case !isLit && !strings.HasSuffix(file, "_test.go"):
				unresolved = append(unresolved, path.Dir(file))
			}
			return true
		})
	}
	slices.Sort(unresolved)
	return runs, slices.Compact(unresolved)
}

// tmuxByConstant returns each call in pkg that names the tmux program to
// os/exec through a constant. A literal is tmuxByLiteral's to find, and a
// variable's value is outside what a source can be read for, accepted.
func tmuxByConstant(pkg sourceguardtest.TypedPackage) []tmuxRun {
	var runs []tmuxRun
	for _, source := range pkg.Sources {
		file := filepath.ToSlash(source.Path)
		ast.Inspect(source.File, func(n ast.Node) bool {
			program, ok := execProgram(n, func(sel *ast.SelectorExpr) bool {
				fn, ok := pkg.Object(sel.Sel).(*types.Func)
				return ok && fn.Pkg() != nil && fn.Pkg().Path() == execPkgPath
			})
			if ok && constantNamesTmux(pkg, program) {
				runs = append(runs, tmuxRun{file: file, line: source.Position(program.Pos()).Line})
			}
			return true
		})
	}
	return runs
}

// execProgram returns the program n names, when n is a call of one of
// execRunners selected as isExec judges.
func execProgram(n ast.Node, isExec func(*ast.SelectorExpr) bool) (ast.Expr, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return nil, false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !slices.Contains(execRunners, sel.Sel.Name) || !isExec(sel) {
		return nil, false
	}
	at := 0
	if sel.Sel.Name == "CommandContext" {
		at = 1
	}
	if len(call.Args) <= at {
		return nil, false
	}
	return call.Args[at], true
}

// importNames are the names file imports the package at pkgPath under.
func importNames(file *ast.File, pkgPath string) []string {
	var names []string
	for _, spec := range file.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != pkgPath {
			continue
		}
		if spec.Name != nil {
			names = append(names, spec.Name.Name)
			continue
		}
		names = append(names, path.Base(pkgPath))
	}
	return names
}

func namesTmux(lit *ast.BasicLit) bool {
	value, err := strconv.Unquote(lit.Value)
	return lit.Kind == token.STRING && err == nil && path.Base(value) == tmuxProgram
}

func constantNamesTmux(pkg sourceguardtest.TypedPackage, program ast.Expr) bool {
	var ident *ast.Ident
	switch p := program.(type) {
	case *ast.Ident:
		ident = p
	case *ast.SelectorExpr:
		ident = p.Sel
	default:
		return false
	}
	c, ok := pkg.Object(ident).(*types.Const)
	return ok && c.Val().Kind() == constant.String && path.Base(constant.StringVal(c.Val())) == tmuxProgram
}

// judgeTmuxRuns returns each run outside migrate's client and tmuxtest, once.
// Finding no run in either is fatal: the scan has stopped seeing the calls
// the rule allows, so it would pass over the ones it forbids too.
func judgeTmuxRuns(t harnesstest.TestingT, runs []tmuxRun) []string {
	t.Helper()
	var inClient, inHarness bool
	var found []string
	for _, run := range runs {
		switch {
		case run.file == migrateTmuxFile:
			inClient = true
		case path.Dir(run.file) == tmuxtestDir:
			inHarness = true
		default:
			found = append(found, run.String())
		}
	}
	if !inClient || !inHarness {
		t.Fatalf("found tmux run in %s: %v, in %s: %v; the scan no longer sees the runs it allows", migrateTmuxFile, inClient, tmuxtestDir, inHarness)
		return nil
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// typedPackage type-checks the production sources of the package at the
// module path dir under lane, naming them by module path.
func typedPackage(t *testing.T, root, dir string, lane sourceguardtest.DepsOption) sourceguardtest.TypedPackage {
	t.Helper()
	pkg := sourceguardtest.NewTypeChecker(t, filepath.Join(root, filepath.FromSlash(dir)), lane).Package(t)
	for i, source := range pkg.Sources {
		rel, err := filepath.Rel(root, source.Path)
		if err != nil {
			t.Fatalf("relativise %s: %v", source.Path, err)
		}
		pkg.Sources[i].Path = rel
	}
	return pkg
}

// tmuxClientBuilder is the one production source outside migrate that may
// name migrate.Tmux, portal migrate's tmux client.
const tmuxClientBuilder = "cmd/migrate.go"

const migratePkgPath = "github.com/leeovery/portal/internal/migrate"

// judgeTmuxClientBuilders returns each place a production source outside
// migrate and cmd/migrate.go names migrate.Tmux. cmd/migrate.go naming it
// nowhere is fatal: the scan has stopped seeing the one builder it allows.
func judgeTmuxClientBuilders(t harnesstest.TestingT, sources []sourceguardtest.ParsedSource) []string {
	t.Helper()
	var builderSeen bool
	var found []string
	for _, source := range sources {
		file := filepath.ToSlash(source.Path)
		if strings.HasPrefix(file, "internal/migrate/") {
			continue
		}
		for _, pos := range tmuxClientNames(source.File) {
			if file == tmuxClientBuilder {
				builderSeen = true
				continue
			}
			found = append(found, fmt.Sprintf("%s:%d", file, source.Position(pos).Line))
		}
	}
	if !builderSeen {
		t.Fatalf("%s names migrate.Tmux nowhere; the scan no longer sees the builder it allows", tmuxClientBuilder)
		return nil
	}
	return found
}

// tmuxClientNames returns each place file names migrate.Tmux, under whatever
// name it imports migrate.
func tmuxClientNames(file *ast.File) []token.Pos {
	names := importNames(file, migratePkgPath)
	if len(names) == 0 {
		return nil
	}
	var found []token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "Tmux" {
			if pkg, ok := sel.X.(*ast.Ident); ok && slices.Contains(names, pkg.Name) {
				found = append(found, sel.Pos())
			}
		}
		return true
	})
	return found
}

// allowedRuns is a run in each place the rule allows one.
func allowedRuns() []tmuxRun {
	return []tmuxRun{{file: migrateTmuxFile, line: 143}, {file: tmuxtestDir + "/socket.go", line: 52}}
}

func parseFixture(t *testing.T, file, src string) sourceguardtest.ParsedSource {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, src, sourceguardtest.ParseMode)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", file, err)
	}
	return sourceguardtest.ParsedSource{Path: file, Fset: fset, File: parsed}
}

func runStrings(runs []tmuxRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.String()
	}
	return out
}
