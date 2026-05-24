//go:build integration

// Pre-fix dysfunction reproduction test for spec § "Composite End-to-End
// Verification" — task 6-2. Consumes setupCompositeHarness (defined in
// composition_e2e_harness_integration_test.go) and asserts the reporter's
// three-daemon scenario is observable BEFORE any fix runs:
//
//   - pgrep -fx '^portal state daemon( |$)' converges to 3 (the
//     three-daemon dysfunction reproduces in the harness).
//   - pgrep returns 3 (i.e. != 1), which would be the false-positive
//     converged-healthy state — guarding against a harness regression
//     where only one daemon survives.
//   - The legitimate stateDir's scrollback/ directory contains an
//     OSCILLATING .bin file count across 4 samples taken ~900 ms
//     apart. Three daemons racing on the same sessions.json + scrollback
//     subtree (each running its own commit cycle) produces visible churn:
//     files appear, get GC'd by a competing daemon, reappear, etc. The
//     variance check (≥2 distinct counts) is the load-bearing signal —
//     a single sample's value would be flaky on the 0↔1 boundary, but
//     observing two distinct counts across 4 ticks is a stable proof of
//     oscillation.
//
// No production-code coverage in this file; this is a harness-driven
// reproduction test that locks in the pre-fix dysfunction so the
// post-fix consumer tests in 6-3..6-6 can claim convergence from a
// known-broken baseline.
//
// No t.Parallel — cmd-package convention.

package bootstrap_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
)

// prefixOscillationSampleCount is the number of scrollback-dir samples
// taken to assert oscillation. Four samples gives three inter-sample
// gaps, enough to observe at least one full create-or-delete transition
// from each of the three racing daemons without inflating wall-time.
const prefixOscillationSampleCount = 4

// prefixOscillationSampleInterval is the wall-time gap between samples.
// 900 ms × 3 gaps = ~2.7 s of observation; with the read I/O itself the
// total elapsed is ~3.6 s. Spaced well above the daemon's per-tick
// capture cadence so successive samples are not always within the same
// tick window.
const prefixOscillationSampleInterval = 900 * time.Millisecond

// TestCompositeHarness_PreFixDysfunctionReproduces verifies that the
// composite harness reproduces the pre-fix dysfunction:
//
//   - Three live `portal state daemon` processes (reporter scenario).
//   - Scrollback directory oscillates as the three daemons race on the
//     same state subtree.
//
// This locks in the broken baseline that the fix is expected to converge.
func TestCompositeHarness_PreFixDysfunctionReproduces(t *testing.T) {
	h := setupCompositeHarness(t)

	// Pre-fix observation 1: three daemons live, simultaneously.
	// setupCompositeHarness's internal assertion already polled to N=3,
	// so a re-read here is a snapshot — no second poll needed. We use
	// portaltest.PgrepPortalDaemons (returns the PID set, not just a
	// count) so the failure diagnostic carries the PID set,
	// distinguishing "daemons exited between harness setup and this
	// assertion" from "harness never reached N=3".
	pids, err := portaltest.PgrepPortalDaemons()
	if err != nil {
		t.Fatalf("pgrep snapshot: %v", err)
	}
	if len(pids) != 3 {
		t.Fatalf("pgrep -fx returned %d daemons, want 3: %v\n"+
			"  h.LegitimateDaemonPID = %d (alive=%v)\n"+
			"  h.Orphan1PID = %d (alive=%v)\n"+
			"  h.Orphan2PID = %d (alive=%v)\n"+
			"  hint: a daemon exited between harness setup and the pre-fix observation",
			len(pids), pids,
			h.LegitimateDaemonPID, pidAlive(h.LegitimateDaemonPID),
			h.Orphan1PID, pidAlive(h.Orphan1PID),
			h.Orphan2PID, pidAlive(h.Orphan2PID))
	}

	// Pre-fix observation 2: explicitly assert pgrep != 1. Spelled out
	// distinctly from the == 3 assertion above so a future relaxation
	// of either form (e.g. accepting >=3) does not silently mask a
	// convergence false-positive where N collapsed to 1. The spec's
	// pre-fix end-state is explicitly "N=3, NOT 1".
	if len(pids) == 1 {
		t.Fatalf("pgrep -fx returned 1 daemon — pre-fix harness must observe 3, not the converged-healthy count\n"+
			"  PIDs: %v", pids)
	}

	// Pre-fix observation 3: scrollback directory oscillation.
	scrollbackDir := state.ScrollbackDir(h.StateDir)
	samples := sampleScrollbackBinCounts(t, scrollbackDir,
		prefixOscillationSampleCount, prefixOscillationSampleInterval)

	assertScrollbackOscillation(t, scrollbackDir, samples)
}

// sampleScrollbackBinCounts returns `count` samples of the number of
// `.bin` files directly under dir, with `interval` between samples.
// Layout is shallow (no recursion) — state.ScrollbackDir places .bin
// files directly under <stateDir>/scrollback/ keyed by paneKey.
//
// A non-existent directory yields 0 for that sample (the daemon may
// not have created scrollback/ yet on the very first tick). The
// function never fails the test; the caller owns oscillation-shape
// assertions and their diagnostics.
func sampleScrollbackBinCounts(t *testing.T, dir string, count int, interval time.Duration) []int {
	t.Helper()
	samples := make([]int, 0, count)
	for i := 0; i < count; i++ {
		if i > 0 {
			time.Sleep(interval)
		}
		samples = append(samples, countBinFiles(dir))
	}
	return samples
}

// countBinFiles returns the number of `.bin` files directly under dir.
// A missing dir returns 0 (treated as "no scrollback activity yet").
// Any other ReadDir error returns 0 — the oscillation assertion's
// "no activity at all" branch will surface the silence with a clear
// diagnostic; transient read errors during heavy daemon churn should
// not flake the test on a single sample.
func countBinFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".bin") {
			n++
		}
	}
	return n
}

// assertScrollbackOscillation fails the test unless the samples contain
// at least 2 distinct values (variance proves oscillation across ticks)
// AND at least one sample observed a non-zero count (otherwise the dir
// is silent — "no scrollback activity" is a distinct harness-broken
// failure mode from "no oscillation").
//
// The two-distinct-values check avoids the 0↔1 single-sample boundary
// flake: any pair of distinct counts across four samples is a stable
// signal that the three-daemon race is actively churning the dir.
func assertScrollbackOscillation(t *testing.T, dir string, samples []int) {
	t.Helper()

	// All-zero samples means no scrollback activity at all — distinct
	// from oscillation failure. Surface a clear diagnostic so the
	// failure-locus question is answerable from the test output alone.
	totalActivity := 0
	for _, s := range samples {
		totalActivity += s
	}
	if totalActivity == 0 {
		listing, _ := os.ReadDir(dir)
		t.Fatalf("no scrollback activity observed across %d samples in %s\n"+
			"  samples: %v\n"+
			"  dir listing: %v\n"+
			"  hint: the daemons may not be running, or capture loop never wrote to scrollback/",
			len(samples), dir, samples, dirNames(listing))
	}

	distinct := make(map[int]struct{}, len(samples))
	for _, s := range samples {
		distinct[s] = struct{}{}
	}
	if len(distinct) < 2 {
		t.Fatalf("scrollback dir did not oscillate across %d samples\n"+
			"  samples: %v\n"+
			"  distinct counts: %d (want >= 2)\n"+
			"  dir: %s\n"+
			"  hint: three racing daemons should produce visible .bin churn; "+
			"a stable count suggests only one daemon is writing",
			len(samples), samples, len(distinct), dir)
	}
}

// dirNames returns the leaf names of entries for diagnostic output.
func dirNames(entries []os.DirEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, filepath.Base(e.Name()))
	}
	return out
}
