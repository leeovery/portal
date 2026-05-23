//go:build integration

// Composite end-to-end scrollback-stability test for spec § "Composite
// End-to-End Verification" bullet 6 — task 6-4.
//
// Consumes the shared compositeHarness (3-daemon pre-state: legitimate
// saver-pane daemon + 2 orphans; legitimate stateDir's daemon.pid
// references orphan1). Invokes the same production bootstrap slice as
// 6-3 (`SweepOrphanDaemons` + `BootstrapPortalSaver`) and then asserts
// the scrollback directory's path-set is invariant across 10 consecutive
// 1 s observations — no `.bin` file additions or removals across the
// observation window (Components A+B+E composition end-state).
//
// Path-set comparison only: mtime, size, and file contents are
// EXPLICITLY allowed to change between samples (the surviving daemon's
// per-tick capture loop will legitimately overwrite the same `.bin`
// files each tick). Only the set of paths under <stateDir>/scrollback/
// must remain stable — additions are an unexplained-write regression,
// removals are a GC-race regression. The spec calls this out at
// § "End-State Verification": "Scrollback directory is stable across
// 10 consecutive 1 s observations — no `.bin` file deletions or
// unexpected new files".
//
// Edge cases handled:
//   - Legitimate per-tick `.bin` writes update mtime/size but do not
//     change path-set membership — passes.
//   - Failure diagnostic lists which path(s) appeared / disappeared at
//     which observation index, so a regression points the engineer at
//     the offending writer.
//   - If baseline is empty AND observations stay empty → valid pass
//     (no daemon writes during the window, but no regression either —
//     the path-set is stable at ∅).
//
// Observation window starts AFTER the first post-bootstrap tick (1 s
// buffer) so the baseline reflects steady-state scrollback contents,
// not a mid-bootstrap snapshot where the surviving daemon's first tick
// may still be writing.
//
// Polled os.ReadDir / filepath.WalkDir; no fsnotify (per acceptance
// criteria).
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// stabilityPostBootstrapBufferTick is the wait between the bootstrap
// slice returning and the first (baseline) scrollback observation. Sized
// to one TickerPeriod (1 s) so the observation window starts AFTER the
// first post-bootstrap tick of the surviving daemon's capture loop —
// guarantees the baseline reflects steady-state contents rather than a
// mid-initialization snapshot.
const stabilityPostBootstrapBufferTick = 1 * time.Second

// stabilityObservationCount is the number of post-baseline samples taken
// to assert path-set invariance. Spec § "Composite End-to-End
// Verification" bullet 6 calls out "10 consecutive 1 s observations" —
// sized verbatim to the spec.
const stabilityObservationCount = 10

// stabilityObservationInterval is the gap between successive samples.
// 1 s matches the daemon's TickerPeriod so each sample falls inside a
// distinct tick window (catches per-tick mutations the path-set would
// otherwise telescope away).
const stabilityObservationInterval = 1 * time.Second

// TestCompositeBootstrap_ScrollbackDirPathSetStableAcross10Observations
// exercises the composite end-to-end scrollback-stability assertion
// against the 3-daemon harness pre-state. See the file-header comment
// for the assertion shape and rationale.
func TestCompositeBootstrap_ScrollbackDirPathSetStableAcross10Observations(t *testing.T) {
	h := setupCompositeHarness(t)

	// Bootstrap slice: same direct-adapter order as the orchestrator
	// runs at steps 4–5 (and as 6-3 invokes). The orphan sweeper is
	// constructed with a nil Logger here — we are NOT asserting on
	// log contents in this test (6-3 covers that). All we need is the
	// production-correct convergence-to-1 end-state so the surviving
	// daemon's capture loop is the only writer to scrollback/.
	sweeper := bootstrapadapter.NewOrphanSweeper(h.Client, nil)
	if err := sweeper.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error "+
			"(best-effort step must return nil): %v", err)
	}
	if err := tmux.BootstrapPortalSaver(h.Client, h.StateDir); err != nil {
		t.Fatalf("BootstrapPortalSaver (post-sweep idempotent re-run): %v", err)
	}

	// Post-bootstrap buffer: wait one TickerPeriod so the observation
	// window starts AFTER the first post-bootstrap tick. The surviving
	// daemon may still be initializing its capture loop at the instant
	// bootstrap returns; the buffer lets the dust settle.
	time.Sleep(stabilityPostBootstrapBufferTick)

	scrollbackDir := state.ScrollbackDir(h.StateDir)

	// Baseline: snapshot the scrollback path-set. Empty-baseline is a
	// valid starting point — if the surviving daemon has not yet
	// written anything, the invariant becomes "stays empty".
	baseline := snapshotScrollbackPaths(t, scrollbackDir)

	// Observation loop: sample 10× at 1 s cadence. Compare each
	// observation's path-set to the baseline. Any add / remove fails.
	for i := 1; i <= stabilityObservationCount; i++ {
		time.Sleep(stabilityObservationInterval)
		observation := snapshotScrollbackPaths(t, scrollbackDir)
		assertPathSetEqual(t, baseline, observation, i, scrollbackDir)
	}
}

// snapshotScrollbackPaths walks dir via filepath.WalkDir and returns
// the set of relative paths (relative to dir) for regular files only.
// Symlinks and subdirectories are excluded — the daemon's capture loop
// writes only regular `.bin` files keyed by paneKey directly under
// scrollback/. A non-existent dir yields an empty set (the surviving
// daemon may not have written anything yet — valid baseline shape per
// the spec's "baseline empty AND observations empty → valid pass" edge
// case).
//
// Uses lstat semantics (the file mode reported by fs.DirEntry.Type() is
// the lstat mode, not the stat mode) so a symlink pointing at a regular
// file is correctly excluded — symlinks under scrollback/ would be a
// production regression we want to surface separately, not silently
// admit into the path-set.
func snapshotScrollbackPaths(t *testing.T, dir string) map[string]struct{} {
	t.Helper()
	paths := make(map[string]struct{})
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// ENOENT at the root means the dir does not exist yet —
			// return an empty set. Any other error at the root or
			// during traversal is unexpected and surfaces below.
			if os.IsNotExist(walkErr) && path == dir {
				return filepath.SkipDir
			}
			return walkErr
		}
		if path == dir {
			return nil
		}
		// Type() is the lstat mode (per filepath.WalkDir contract).
		// Exclude directories and non-regular files (symlinks, sockets,
		// FIFOs — none of which the daemon should be writing under
		// scrollback/).
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("snapshotScrollbackPaths(%q): %v", dir, err)
	}
	return paths
}

// assertPathSetEqual fails the test with a rich diagnostic if the
// observed path-set differs from the baseline. The diagnostic lists
// added paths (appeared in this observation but absent from baseline)
// and removed paths (present in baseline but absent from this
// observation) separately, so the failure points at the offending
// writer (or GC race) directly.
//
// The observationIndex argument identifies which of the 10 samples
// surfaced the regression — useful for distinguishing "first
// observation diverged" (likely a baseline-timing issue) from "10th
// observation diverged" (a slow-developing race).
func assertPathSetEqual(t *testing.T, baseline, observation map[string]struct{},
	observationIndex int, scrollbackDir string,
) {
	t.Helper()
	added := setDifference(observation, baseline)
	removed := setDifference(baseline, observation)
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	t.Fatalf("scrollback path-set diverged at observation %d/%d "+
		"(baseline = %d paths, observation = %d paths)\n"+
		"  added paths (appeared in observation, absent from baseline): %v\n"+
		"  removed paths (absent from observation, present in baseline): %v\n"+
		"  baseline: %v\n"+
		"  observation: %v\n"+
		"  scrollback dir: %s\n"+
		"  hint: additions point at an unexpected writer; removals point at "+
		"a GC race (Components A+B+E composition regression)",
		observationIndex, stabilityObservationCount,
		len(baseline), len(observation),
		added, removed,
		sortedKeys(baseline), sortedKeys(observation),
		scrollbackDir)
}

// setDifference returns the sorted slice of keys present in a but not
// in b. Sorted for deterministic diagnostic output across runs.
func setDifference(a, b map[string]struct{}) []string {
	out := make([]string, 0)
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// sortedKeys returns the sorted keys of m. Sorted for deterministic
// diagnostic output.
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
