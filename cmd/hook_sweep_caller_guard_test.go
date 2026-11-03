package cmd

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// hookSweepReferences reports each "<file>: <reference>" a cmd source outside
// the tmux state commands makes to the hook sweep or its staleness rule: the
// sweep's package, hooks.StaleKeys, or a hooks store's CleanStale, which runs
// it. StaleKeys reaps a token-shaped key absent from tmux's live pane tokens,
// and a six-character alphanumeric zmx name such as notes1 is token-shaped, so
// every hook keyed by one would be deleted.
func hookSweepReferences(pkg sourceguardtest.TypedPackage) []string {
	var found []string
	for _, source := range pkg.Sources {
		name := filepath.Base(source.Path)
		if strings.HasPrefix(name, "state_") {
			continue
		}
		if _, ok := importNames(source.File)[hooksweepPkgPath]; ok {
			found = append(found, name+": "+hooksweepPkgPath)
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			switch {
			case !ok:
			case pkg.IsPackageFunc(sel.Sel, hooksPkgPath, "StaleKeys"):
				found = append(found, name+": hooks.StaleKeys")
			case pkg.IsMethodOf(sel, hooksPkgPath, "Store", "CleanStale"):
				found = append(found, name+": (*hooks.Store).CleanStale")
			}
			return true
		})
	}
	return found
}

// runHookSweepGuard fails on each reference hookSweepReferences finds, and
// when it scanned nothing.
func runHookSweepGuard(t harnesstest.TestingT, pkg sourceguardtest.TypedPackage) {
	t.Helper()
	if len(pkg.Sources) == 0 {
		t.Fatalf("the hook sweep guard scanned no sources")
	}
	for _, found := range hookSweepReferences(pkg) {
		t.Errorf("%s; only the tmux state commands may judge a hook key stale", found)
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

	const reachesTheSweep = `package cmd

import (
	_ "github.com/leeovery/portal/internal/hooksweep"
	h "github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/project"
)

var stale = h.StaleKeys

func prune(hooks *h.Store, projects *project.Store) {
	_, _ = hooks.CleanStale(nil)
	_, _ = projects.CleanStale()
}
`

	t.Run("it reports the sweep, StaleKeys and a hooks store's CleanStale, under any alias", func(t *testing.T) {
		got := hookSweepReferences(checker.Sources(t, map[string]string{"hooks.go": reachesTheSweep}))
		want := []string{"hooks.go: " + hooksweepPkgPath, "hooks.go: hooks.StaleKeys", "hooks.go: (*hooks.Store).CleanStale"}
		if !slices.Equal(got, want) {
			t.Errorf("hookSweepReferences = %v, want %v: a project store's CleanStale passes", got, want)
		}
	})

	t.Run("it lets a state command reach them", func(t *testing.T) {
		got := hookSweepReferences(checker.Sources(t, map[string]string{"state_daemon.go": reachesTheSweep}))
		if len(got) != 0 {
			t.Errorf("hookSweepReferences = %v, want none for a state command", got)
		}
	})
}
