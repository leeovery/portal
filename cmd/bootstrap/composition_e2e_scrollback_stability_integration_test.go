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
//   - Empty baseline is NOT a valid pass — the harness seeds two
//     sessions running `while sleep 0.1; do echo "hello $RANDOM"; done`
//     before bootstrap, so by the time the post-bootstrap buffer tick
//     elapses the surviving daemon's capture loop MUST have produced at
//     least one `.bin` file. An empty baseline therefore signals a
//     broken capture pipeline (Component E regression) and the test
//     fails loudly rather than admitting "stable at ∅" as a pass.
//   - Missing scrollback dir at baseline is similarly a regression
//     signal — the daemon should have created it during its first tick
//     — and is distinguished from "dir exists but empty" by
//     snapshotScrollbackPaths' second return value.
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
	"maps"
	"os"
	"path/filepath"
	"slices"
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
	// window starts AFTER the first post-bootstrap tick. The harness
	// seeded two sessions running `while sleep 0.1; do echo "hello
	// $RANDOM"; done` before bootstrap (see compositeUserSessionSeedScript)
	// so by the time this buffer elapses the surviving daemon's capture
	// loop MUST have produced at least one `.bin` file under
	// scrollback/ — an empty baseline IS a regression signal (broken
	// capture pipeline) and is asserted as such below.
	time.Sleep(stabilityPostBootstrapBufferTick)

	scrollbackDir := state.ScrollbackDir(h.StateDir)

	// Baseline: snapshot the scrollback path-set.
	baseline, dirExists := snapshotScrollbackPaths(t, scrollbackDir)
	if !dirExists {
		t.Fatalf("scrollback dir does not exist: %s", scrollbackDir)
	}
	if len(baseline) == 0 {
		t.Fatalf("scrollback baseline empty after first post-bootstrap " +
			"tick — capture pipeline may be broken or seed activity " +
			"insufficient")
	}

	// Observation loop: sample 10× at 1 s cadence. Compare each
	// observation's path-set to the baseline. Any add / remove fails.
	for i := 1; i <= stabilityObservationCount; i++ {
		time.Sleep(stabilityObservationInterval)
		observation, _ := snapshotScrollbackPaths(t, scrollbackDir)
		assertPathSetEqual(t, baseline, observation, i, scrollbackDir)
	}
}

// snapshotScrollbackPaths walks dir via filepath.WalkDir and returns
// the set of relative paths (relative to dir) for regular files only,
// along with a bool indicating whether the root dir exists.
//
// Return contract:
//   - (paths, true)  → dir exists; paths is the (possibly empty) set of
//     regular files under it.
//   - (nil, false)   → dir does not exist (ENOENT at root). The caller
//     is expected to treat this as a regression signal and fail with a
//     `scrollback dir does not exist` diagnostic — the post-bootstrap
//     buffer tick is sized so the surviving daemon's capture loop
//     should have created the dir by baseline time. Empty-set with
//     dir-present is structurally distinct and is caught by the
//     caller's separate len(baseline) > 0 assertion.
//
// Symlinks and subdirectories are excluded — the daemon's capture loop
// writes only regular `.bin` files keyed by paneKey directly under
// scrollback/.
//
// Uses lstat semantics (the file mode reported by fs.DirEntry.Type() is
// the lstat mode, not the stat mode) so a symlink pointing at a regular
// file is correctly excluded — symlinks under scrollback/ would be a
// production regression we want to surface separately, not silently
// admit into the path-set.
func snapshotScrollbackPaths(t *testing.T, dir string) (map[string]struct{}, bool) {
	t.Helper()
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil, false
		}
		t.Fatalf("snapshotScrollbackPaths stat(%q): %v", dir, err)
	}
	paths := make(map[string]struct{})
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
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
	if err != nil {
		t.Fatalf("snapshotScrollbackPaths(%q): %v", dir, err)
	}
	return paths, true
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
		slices.Sorted(maps.Keys(baseline)), slices.Sorted(maps.Keys(observation)),
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
