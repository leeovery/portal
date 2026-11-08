package log_test

import (
	"go/ast"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

func TestNoDiscardLoggerConstruction(t *testing.T) {
	scan := scanForDiscardConstruction(t)

	for _, finding := range scan.findings {
		t.Errorf("%s constructs a discard-backed *slog.Logger; route through log.OrDiscard / log.Discard instead", finding)
	}
}

func TestDiscardGuard_Rule(t *testing.T) {
	const openCoded = `package fixture

import (
	"io"
	"log/slog"
)

var logger = slog.New(slog.NewTextHandler(io.Discard, nil))
`
	const clean = "package fixture\n"
	paint := filepath.Join("internal", "tui", "paint_test.go")

	for _, tc := range []struct {
		name         string
		files        map[string]string
		wantScanned  int
		wantFindings []string
	}{
		{
			name:         "it fails for an open-coded discard handler in a test file",
			files:        map[string]string{paint: openCoded},
			wantScanned:  1,
			wantFindings: []string{paint + ":8"},
		},
		{
			name: "it fails for slog's own discard handler",
			files: map[string]string{paint: `package fixture

import "log/slog"

var logger = slog.New(slog.DiscardHandler)
`},
			wantScanned:  1,
			wantFindings: []string{paint + ":5"},
		},
		{
			name: "it fails for a JSON handler on io.Discard",
			files: map[string]string{paint: `package fixture

import (
	"io"
	"log/slog"
)

var logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
`},
			wantScanned:  1,
			wantFindings: []string{paint + ":8"},
		},
		{
			name: "it fails for a handler on io/ioutil's Discard, under either name",
			files: map[string]string{paint: `package fixture

import (
	"io/ioutil"
	"log/slog"
	old "io/ioutil"
)

var (
	text = slog.New(slog.NewTextHandler(ioutil.Discard, nil))
	json = slog.New(slog.NewJSONHandler(old.Discard, nil))
)
`},
			wantScanned:  1,
			wantFindings: []string{paint + ":10", paint + ":11"},
		},
		{
			name: "it reads the packages under the names a file imports them by",
			files: map[string]string{paint: `package fixture

import (
	stdio "io"
	sl "log/slog"
)

var (
	text    = sl.New(sl.NewTextHandler(stdio.Discard, nil))
	discard = sl.DiscardHandler
)
`},
			wantScanned:  1,
			wantFindings: []string{paint + ":9", paint + ":10"},
		},
		{
			name: "it passes a handler on a writer that discards nothing, and names it shares with no import",
			files: map[string]string{paint: `package fixture

import (
	"io"
	"log/slog"
	"os"
)

type sink struct{ Discard io.Writer }

var (
	stderr = slog.New(slog.NewTextHandler(os.Stderr, nil))
	held   = sink{Discard: io.Discard}
	field  = slog.NewTextHandler(held.Discard, nil)
)
`},
			wantScanned: 1,
		},
		{
			name: "it exempts internal/log's own in-package tests",
			files: map[string]string{
				filepath.Join("internal", "log", "rotate_test.go"): openCoded,
				paint: clean,
			},
			wantScanned: 1,
		},
		{
			name: "it exempts internal/prefs for its stated reason",
			files: map[string]string{
				filepath.Join("internal", "prefs", "store.go"): openCoded,
				paint: clean,
			},
			wantScanned: 1,
		},
		{
			// prefs' leaf rule binds its production imports alone, so its test
			// files keep the route and stay under the rule.
			name:         "it covers internal/prefs test files, which may reach internal/log",
			files:        map[string]string{filepath.Join("internal", "prefs", "store_test.go"): openCoded},
			wantScanned:  1,
			wantFindings: []string{filepath.Join("internal", "prefs", "store_test.go") + ":8"},
		},
		{
			name: "it enumerates through the shared repo walk",
			files: map[string]string{
				filepath.Join("vendor", "dep", "vendored.go"):      openCoded,
				filepath.Join(".worktrees", "old", "stale.go"):     openCoded,
				filepath.Join("node_modules", "pkg", "bundle.go"):  openCoded,
				filepath.Join("internal", "tui", "nested_test.go"): clean,
			},
			wantScanned: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := stageDiscardGuardTree(t, tc.files)

			scan := scanForDiscardConstruction(t, sourceguardtest.Rooted(root))

			if scan.scanned != tc.wantScanned {
				t.Fatalf("scanned %d files, want %d", scan.scanned, tc.wantScanned)
			}
			if !slices.Equal(scan.findings, tc.wantFindings) {
				t.Errorf("findings = %v, want %v", scan.findings, tc.wantFindings)
			}
		})
	}
}

// A guard whose scan came back empty would report a clean tree forever.
func TestDiscardGuard_FatalsWhenItScansNothing(t *testing.T) {
	recorder := &harnesstest.Recorder{}
	recorder.Run(func() {
		scanForDiscardConstruction(recorder, sourceguardtest.Rooted(t.TempDir()))
	})

	if len(recorder.Fatals) != 1 {
		t.Fatalf("scan of a tree holding no sources reported %d fatals, want 1: %v", len(recorder.Fatals), recorder.Fatals)
	}
	if !strings.Contains(recorder.Fatals[0], "stopped looking") {
		t.Errorf("fatal message %q does not say the guard would pass having stopped looking", recorder.Fatals[0])
	}
}

type discardScan struct {
	scanned  int
	findings []string
}

// scanForDiscardConstruction reads every .go file the shared repo walk offers,
// production and test alike, and reports each site in the unexempted ones
// that builds a discard-backed handler by hand.
func scanForDiscardConstruction(t harnesstest.TestingT, opts ...sourceguardtest.ScanOption) discardScan {
	t.Helper()

	_, sources := sourceguardtest.RepoSources(t, sourceguardtest.AllSources, opts...)

	var scan discardScan
	for _, source := range sources {
		if discardConstructionExempt(source.Path) {
			continue
		}
		scan.scanned++
		scan.findings = append(scan.findings, discardSites(source)...)
	}
	return scan
}

// discardPackages are the packages that export Discard, the writer that
// throws away what it is given: io, and io/ioutil, which still re-exports it.
var discardPackages = []string{"io", "io/ioutil"}

// discardSites are the sites in source that build a discard-backed handler:
// slog.DiscardHandler, or slog.NewTextHandler or slog.NewJSONHandler handed
// io.Discard or ioutil.Discard, read under the names the file imports those
// packages by. A dot import names no package, and none is in the tree.
func discardSites(source sourceguardtest.ParsedSource) []string {
	slogNames := importNames(source.File, "log/slog")
	if len(slogNames) == 0 {
		return nil
	}
	discardNames := importNames(source.File, discardPackages...)
	var sites []string
	ast.Inspect(source.File, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if refersTo(n, slogNames, "DiscardHandler") {
				sites = append(sites, siteOf(source, n))
			}
		case *ast.CallExpr:
			if handsDiscardToHandler(n, slogNames, discardNames) {
				sites = append(sites, siteOf(source, n))
			}
		}
		return true
	})
	return sites
}

// handsDiscardToHandler reports whether call builds a text or JSON handler
// writing to Discard, from a package imported under one of discardNames.
func handsDiscardToHandler(call *ast.CallExpr, slogNames, discardNames []string) bool {
	if len(call.Args) == 0 || !refersTo(call.Args[0], discardNames, "Discard") {
		return false
	}
	return refersTo(call.Fun, slogNames, "NewTextHandler") || refersTo(call.Fun, slogNames, "NewJSONHandler")
}

// refersTo reports whether expr is pkg.name, pkg being one of the names a
// file imports a package by.
func refersTo(expr ast.Expr, pkgs []string, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && slices.Contains(pkgs, x.Name)
}

// importNames are the names file imports any of paths by, leaving out a
// blank or dot import, which names none.
func importNames(file *ast.File, paths ...string) []string {
	var names []string
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil || !slices.Contains(paths, imported) {
			continue
		}
		switch {
		case spec.Name == nil:
			names = append(names, filepath.Base(imported))
		case spec.Name.Name != "_" && spec.Name.Name != ".":
			names = append(names, spec.Name.Name)
		}
	}
	return names
}

func siteOf(source sourceguardtest.ParsedSource, n ast.Node) string {
	position := source.Position(n.Pos())
	return position.Filename + ":" + strconv.Itoa(position.Line)
}

// discardConstructionExempt reports the files that may hand-roll the
// construction. discard.go is that route's own home, and internal/log's tests
// are the route's own, so they are exempted as one directory-wide rule.
// internal/prefs has no route at all: it is a leaf whose production code must
// not import internal/log — a constraint that binds its imports alone, so its
// test files keep the route and stay under the rule.
func discardConstructionExempt(path string) bool {
	dir, file := filepath.Split(path)
	dir = filepath.Clean(dir)
	isTest := strings.HasSuffix(file, "_test.go")

	switch dir {
	case filepath.Join("internal", "log"):
		return isTest || file == "discard.go"
	case filepath.Join("internal", "prefs"):
		return !isTest
	}
	return false
}

func stageDiscardGuardTree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}
