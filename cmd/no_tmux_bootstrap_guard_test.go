package cmd

// No command runs the tmux bootstrap: its kill-barrier would take down the
// installed Portal's _portal-saver, its hook registration would rewrite that
// Portal's global hooks, and unregistering them would strip them, as the old
// uninstall did in tests. That rests on the binary holding no path to them,
// which this guard pins over the binary's production dependency closure
// rather than leaving to absence. The zmx bootstrap's orphan sweep is the
// binary's own, and kills only a daemon it proves of this backend.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

const portalModule = "github.com/leeovery/portal"

// tmuxBootstrapEntries names, by package, what no production source of the
// binary may reference. A package names its own declarations unqualified, so
// only another package's reference matches.
var tmuxBootstrapEntries = map[string][]string{
	portalModule + "/internal/tmux": {"EnsurePortalSaverVersion", "BootstrapPortalSaver", "RegisterPortalHooks", "UnregisterPortalHooks"},
}

func TestBinaryHoldsNoPathToTheTmuxBootstrap(t *testing.T) {
	root := sourceguardtest.ProjectRoot(t)
	deps := sourceguardtest.PackageDeps(t, portalModule, sourceguardtest.InDir(root))

	if !slices.Contains(deps, portalModule+"/cmd") {
		t.Fatalf("the binary's dependencies %v do not hold cmd; the reading is not of the binary", deps)
	}

	t.Run("it references no tmux bootstrap entry point outside its owner", func(t *testing.T) {
		var sources []sourceguardtest.ParsedSource
		for _, dep := range deps {
			if rel, ok := strings.CutPrefix(dep, portalModule); ok {
				sources = append(sources, sourceguardtest.ParsePackageSources(t, filepath.Join(root, rel), false)...)
			}
		}
		runTmuxBootstrapGuard(t, sources, func(path string) bool {
			_, err := sourceguardtest.PackageGoFiles(filepath.Join(root, strings.TrimPrefix(path, portalModule)), false)
			return err == nil
		})
	})
}

// runTmuxBootstrapGuard reports every reference sources make to a tmux
// bootstrap entry point. An entry's package no source imports is one the
// binary does not hold, so holds no path to. It fails when it scanned nothing,
// and when an entry's package is neither imported nor in the module, as
// inModule reports it: the vocabulary would have gone stale and the guard
// would pass by no longer looking.
func runTmuxBootstrapGuard(t harnesstest.TestingT, sources []sourceguardtest.ParsedSource, inModule func(path string) bool) {
	t.Helper()
	if len(sources) == 0 {
		t.Fatalf("the tmux bootstrap guard scanned no sources")
	}
	imported := map[string]bool{}
	for _, source := range sources {
		for path, local := range importNames(source.File) {
			names, banned := tmuxBootstrapEntries[path]
			if !banned {
				continue
			}
			imported[path] = true
			ast.Inspect(source.File, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == local && slices.Contains(names, sel.Sel.Name) {
					t.Errorf("%s references %s.%s, a path to the tmux bootstrap", source.Position(sel.Pos()), path, sel.Sel.Name)
				}
				return true
			})
		}
	}
	for path := range tmuxBootstrapEntries {
		if !imported[path] && !inModule(path) {
			t.Errorf("no scanned source imports %s and the module holds no such package; the guard's vocabulary has gone stale", path)
		}
	}
}

// importNames maps each import of file to the name it is referenced by.
func importNames(file *ast.File) map[string]string {
	names := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path[strings.LastIndex(path, "/")+1:]
		if spec.Name != nil {
			name = spec.Name.Name
		}
		names[path] = name
	}
	return names
}

func TestTmuxBootstrapGuard_FailurePaths(t *testing.T) {
	parse := func(t *testing.T, src string) []sourceguardtest.ParsedSource {
		t.Helper()
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "fixture.go", src, sourceguardtest.ParseMode)
		if err != nil {
			t.Fatalf("parse fixture: %v", err)
		}
		return []sourceguardtest.ParsedSource{{Path: "fixture.go", Fset: fset, File: file}}
	}
	const importsAll = `package fixture

import (
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/state"
	tm "github.com/leeovery/portal/internal/tmux"
)
`

	t.Run("it reports an aliased reference passed as a value", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		runTmuxBootstrapGuard(rec, parse(t, importsAll+`
var _ = bootstrap.Warning{}
var _ = state.Dir
var ensure = tm.EnsurePortalSaverVersion
`), inNoModule)
		if len(rec.Errors) != 1 || !strings.Contains(rec.Errors[0], "EnsurePortalSaverVersion") {
			t.Errorf("errors = %q, want one naming EnsurePortalSaverVersion", rec.Errors)
		}
	})

	t.Run("it reports each tmux entry point and not the zmx bootstrap's own", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		runTmuxBootstrapGuard(rec, parse(t, importsAll+`
func f() {
	_ = &bootstrap.OrphanSweepCore{Pgrep: state.PgrepPortalDaemons}
	_ = tm.RegisterPortalHooks(nil, nil)
	_ = tm.UnregisterPortalHooks(nil)
}
`), inNoModule)
		if len(rec.Errors) != 2 {
			t.Errorf("errors = %q, want two, the tmux entry points alone", rec.Errors)
		}
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runTmuxBootstrapGuard(rec, nil, inNoModule) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	t.Run("it fails when nothing imports an entry's package and the module holds none", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		runTmuxBootstrapGuard(rec, parse(t, "package fixture\n"), inNoModule)
		if len(rec.Errors) != len(tmuxBootstrapEntries) {
			t.Errorf("errors = %q, want one per entry package", rec.Errors)
		}
	})

	t.Run("it passes an entry's package the module holds and the binary does not", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		runTmuxBootstrapGuard(rec, parse(t, "package fixture\n"), func(string) bool { return true })
		if len(rec.Errors) != 0 {
			t.Errorf("errors = %q, want none", rec.Errors)
		}
	})
}

// inNoModule reports that the module holds no package at all, so the guard
// counts only what its sources import.
func inNoModule(string) bool { return false }
