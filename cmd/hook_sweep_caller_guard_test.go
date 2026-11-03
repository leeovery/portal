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

// The hook commands key by session name, and the staleness rule judges a key
// against live pane tokens: run from them, it would read every key the tmux
// version wrote as stale and reap it. Until the zmx saver keys the sweep by
// session name, none of them may reach it.
func TestHookCommandsNeverJudgeStaleness(t *testing.T) {
	forbidden := regexp.MustCompile(`\bStaleKeys\b|\bCleanStale\b|\bhooksweep\b`)

	const source = "hooks.go"
	src, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}
	if !strings.Contains(string(src), "hooksSetCmd") {
		t.Fatalf("%s no longer declares the hook commands; guard is vacuous", source)
	}
	if loc := forbidden.FindIndex(src); loc != nil {
		line := 1 + strings.Count(string(src[:loc[0]]), "\n")
		t.Errorf("%s:%d reaches the hook staleness sweep; the hook commands must never judge a key stale", source, line)
	}
}

const (
	hooksPkgPath     = "github.com/leeovery/portal/internal/hooks"
	hooksweepPkgPath = "github.com/leeovery/portal/internal/hooksweep"
)

// hookSweepReferences reports each "<file>: <reference>" a cmd source outside
// the tmux state commands makes to the hook sweep or its staleness rule.
// StaleKeys reaps a token-shaped key absent from tmux's live pane tokens, and a
// zmx session name such as notes1 is token-shaped, so judged by it every
// name-keyed hook would be deleted.
func hookSweepReferences(sources []sourceguardtest.ParsedSource) []string {
	var found []string
	for _, source := range sources {
		name := filepath.Base(source.Path)
		if strings.HasPrefix(name, "state_") {
			continue
		}
		imports := importNames(source.File)
		if _, ok := imports[hooksweepPkgPath]; ok {
			found = append(found, name+": "+hooksweepPkgPath)
		}
		hooks, ok := imports[hooksPkgPath]
		if !ok {
			continue
		}
		ast.Inspect(source.File, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == "StaleKeys" {
				if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == hooks {
					found = append(found, name+": hooks.StaleKeys")
				}
			}
			return true
		})
	}
	return found
}

// runHookSweepGuard fails on each reference hookSweepReferences finds, and
// when it scanned nothing.
func runHookSweepGuard(t harnesstest.TestingT, sources []sourceguardtest.ParsedSource) {
	t.Helper()
	if len(sources) == 0 {
		t.Fatalf("the hook sweep guard scanned no sources")
	}
	for _, found := range hookSweepReferences(sources) {
		t.Errorf("%s; only the tmux state commands may judge a hook key stale", found)
	}
}

func TestOnlyTheTmuxStateCommandsReachTheHookSweep(t *testing.T) {
	t.Run("it finds no reference outside the state commands", func(t *testing.T) {
		runHookSweepGuard(t, sourceguardtest.ParsePackageSources(t, ".", false))
	})

	t.Run("it fails when it scans nothing", func(t *testing.T) {
		rec := &harnesstest.Recorder{}
		rec.Run(func() { runHookSweepGuard(rec, nil) })
		if !rec.Failed() {
			t.Error("the guard passed over no sources")
		}
	})

	t.Run("it reports an import of the sweep and a StaleKeys reference under any alias", func(t *testing.T) {
		synthetic := parseSyntheticSource(t, `package cmd

import (
	_ "github.com/leeovery/portal/internal/hooksweep"
	h "github.com/leeovery/portal/internal/hooks"
)

var stale = h.StaleKeys
`)
		want := []string{"synthetic.go: " + hooksweepPkgPath, "synthetic.go: hooks.StaleKeys"}
		if got := hookSweepReferences(synthetic); !slices.Equal(got, want) {
			t.Errorf("hookSweepReferences = %v, want %v", got, want)
		}
	})

	t.Run("it lets a state command reach them", func(t *testing.T) {
		synthetic := parseSyntheticSource(t, `package cmd

import "github.com/leeovery/portal/internal/hooksweep"

var run = hooksweep.Run
`)
		synthetic[0].Path = filepath.Join(filepath.Dir(synthetic[0].Path), "state_daemon.go")
		if got := hookSweepReferences(synthetic); len(got) != 0 {
			t.Errorf("hookSweepReferences = %v, want none for a state command", got)
		}
	})
}
