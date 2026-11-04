package cmd

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/sourceguardtest"
)

// The \bCleanStale\b word boundary is deliberate: the marker-sweep method
// CleanStaleMarkers carries a trailing word char, so only the bare hooks
// method matches. The cycle itself now lives in internal/hooksweep, so the
// package name is what names it here: a bootstrap step has no business reaching
// that package at all, whichever of its entry points it reached for. Only
// production files are scanned — test files legitimately mention the name in
// prose.
func TestHooksCleanStale_NoBootstrapStepIsAnAutomaticCaller(t *testing.T) {
	forbidden := regexp.MustCompile(`\bCleanStale\b|\bhooksweep\b`)

	const bootstrapDir = "bootstrap"
	entries, err := os.ReadDir(bootstrapDir)
	if err != nil {
		t.Fatalf("read %s: %v", bootstrapDir, err)
	}

	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(bootstrapDir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		scanned++
		if loc := forbidden.FindIndex(src); loc != nil {
			line := 1 + strings.Count(string(src[:loc[0]]), "\n")
			t.Errorf("%s:%d references the hooks CleanStale path; the daemon (maybeRunHookCleanup) "+
				"is the only permitted caller — a bootstrap step must not clean hooks",
				path, line)
		}
	}

	// The directory must actually contain production source, else a layout
	// change would leave this guard scanning nothing.
	if scanned == 0 {
		t.Fatalf("no production .go files scanned under %q; guard is vacuous", bootstrapDir)
	}
}

const (
	hooksPkgPath     = "github.com/leeovery/portal/internal/hooks"
	hooksweepPkgPath = "github.com/leeovery/portal/internal/hooksweep"
)

// hookSweepReferences reports each "<file>:<line>: <reach>" a cmd source
// outside the tmux state commands makes for the hook sweep: an import of its
// package, hooks.StaleKeys, the (*hooks.Store).CleanStale method in any form
// (a call, a method value or expression, or promoted through an embedded
// store), or an interface's CleanStale taking a func(hooks.Snapshot), the
// shape of a seam over it. Each is judged by the object an identifier names,
// not by how the source spelled it. StaleKeys reaps a token-shaped key absent
// from tmux's live pane tokens, and a six-character alphanumeric zmx name such
// as notes1 is token-shaped, so every hook keyed by one would be deleted.
// Reflection, and a state_*.go helper called from another cmd file, are
// outside its bound, accepted.
func hookSweepReferences(pkg sourceguardtest.TypedPackage) []string {
	var found []string
	for _, source := range pkg.Sources {
		name := filepath.Base(source.Path)
		if strings.HasPrefix(name, "state_") {
			continue
		}
		at := func(n ast.Node, reach string) {
			found = append(found, fmt.Sprintf("%s:%d: %s", name, source.Position(n.Pos()).Line, reach))
		}
		for _, spec := range source.File.Imports {
			if path, err := strconv.Unquote(spec.Path.Value); err == nil && path == hooksweepPkgPath {
				at(spec, hooksweepPkgPath)
			}
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok {
				if reach := hookSweepReach(pkg.Object(ident)); reach != "" {
					at(ident, reach)
				}
			}
			return true
		})
	}
	return found
}

// hookSweepReach names what obj reaches of the hook sweep, empty for nothing.
func hookSweepReach(obj types.Object) string {
	fn, ok := obj.(*types.Func)
	switch {
	case !ok:
		return ""
	case fn.FullName() == hooksPkgPath+".StaleKeys":
		return "hooks.StaleKeys"
	case fn.FullName() == "(*"+hooksPkgPath+".Store).CleanStale":
		return "(*hooks.Store).CleanStale"
	case fn.Name() == "CleanStale" && isInterfaceMethod(fn) && takesSnapshotFunc(fn.Signature()):
		return "a CleanStale seam taking func(hooks.Snapshot)"
	}
	return ""
}

func isInterfaceMethod(fn *types.Func) bool {
	recv := fn.Signature().Recv()
	return recv != nil && types.IsInterface(recv.Type())
}

// takesSnapshotFunc reports whether sig has a parameter that is a func taking
// a hooks.Snapshot first, as (*hooks.Store).CleanStale's enumerator is.
func takesSnapshotFunc(sig *types.Signature) bool {
	for param := range sig.Params().Variables() {
		fn, ok := param.Type().Underlying().(*types.Signature)
		if !ok || fn.Params().Len() == 0 {
			continue
		}
		if named, ok := fn.Params().At(0).Type().(*types.Named); ok && named.Obj().Pkg() != nil &&
			named.Obj().Pkg().Path() == hooksPkgPath && named.Obj().Name() == "Snapshot" {
			return true
		}
	}
	return false
}

// runHookSweepGuard fails on each reference hookSweepReferences finds, and
// when it scanned nothing.
func runHookSweepGuard(t harnesstest.TestingT, pkg sourceguardtest.TypedPackage) {
	t.Helper()
	if len(pkg.Sources) == 0 {
		t.Fatalf("the hook sweep guard scanned no sources")
	}
	for _, found := range hookSweepReferences(pkg) {
		t.Errorf("%s; in cmd, only the tmux state commands may judge a hook key stale", found)
	}
}

func TestOnlyTheTmuxStateCommandsReachTheHookSweep(t *testing.T) {
	checker := sourceguardtest.NewTypeChecker(t, ".")

	t.Run("it finds no reference outside the state commands", func(t *testing.T) {
		runHookSweepGuard(t, checker.Package(t))
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runHookSweepGuard(rec, sourceguardtest.TypedPackage{}) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	// cmd no longer imports the sweep, so its export data is outside the type
	// checker's reach; the import rule reads syntax alone and needs none.
	t.Run("it judges an import of the sweep", func(t *testing.T) {
		const src = "package cmd\n\nimport _ \"" + hooksweepPkgPath + "\"\n"
		for name, want := range map[string][]string{
			"hooks.go":        {"hooks.go:3: " + hooksweepPkgPath},
			"state_daemon.go": nil,
		} {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, name, src, sourceguardtest.ParseMode)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			pkg := sourceguardtest.TypedPackage{
				Sources: []sourceguardtest.ParsedSource{{Path: name, Fset: fset, File: file}},
				Info:    &types.Info{},
			}
			if got := hookSweepReferences(pkg); !slices.Equal(got, want) {
				t.Errorf("hookSweepReferences(%s) = %q, want %q", name, got, want)
			}
		}
	})

	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"StaleKeys under an alias", `import h "github.com/leeovery/portal/internal/hooks"

var stale = h.StaleKeys`,
			[]string{"hooks.go:5: hooks.StaleKeys"}},
		{"a call", `import "github.com/leeovery/portal/internal/hooks"

func prune(s *hooks.Store) { _, _ = s.CleanStale(nil) }`,
			[]string{"hooks.go:5: (*hooks.Store).CleanStale"}},
		{"a method value", `import "github.com/leeovery/portal/internal/hooks"

func prune(s *hooks.Store) { clean := s.CleanStale; _ = clean }`,
			[]string{"hooks.go:5: (*hooks.Store).CleanStale"}},
		{"a method expression", `import "github.com/leeovery/portal/internal/hooks"

var clean = (*hooks.Store).CleanStale`,
			[]string{"hooks.go:5: (*hooks.Store).CleanStale"}},
		{"a method promoted through an embedded store", `import "github.com/leeovery/portal/internal/hooks"

type store struct{ *hooks.Store }

func prune(s store) { _, _ = s.CleanStale(nil) }`,
			[]string{"hooks.go:7: (*hooks.Store).CleanStale"}},
		{"a seam typed as an interface", `import "github.com/leeovery/portal/internal/hooks"

type cleaner interface {
	CleanStale(func(hooks.Snapshot) ([]string, error)) ([]string, error)
}

func prune(c cleaner) { _, _ = c.CleanStale(nil) }`,
			[]string{"hooks.go:6: a CleanStale seam taking func(hooks.Snapshot)", "hooks.go:9: a CleanStale seam taking func(hooks.Snapshot)"}},
		{"a project store's CleanStale", `import "github.com/leeovery/portal/internal/project"

type cleaner interface{ CleanStale() ([]project.Project, error) }

func prune(s *project.Store, c cleaner) { _, _ = s.CleanStale(); _, _ = c.CleanStale() }`,
			nil},
	} {
		src := "package cmd\n\n" + tc.body + "\n"
		t.Run("it judges "+tc.name, func(t *testing.T) {
			if got := hookSweepReferences(checker.Sources(t, map[string]string{"hooks.go": src})); !slices.Equal(got, tc.want) {
				t.Errorf("hookSweepReferences = %q, want %q", got, tc.want)
			}
		})
		t.Run("it lets a state command reach "+tc.name, func(t *testing.T) {
			if got := hookSweepReferences(checker.Sources(t, map[string]string{"state_daemon.go": src})); len(got) != 0 {
				t.Errorf("hookSweepReferences = %q, want none for a state command", got)
			}
		})
	}
}
