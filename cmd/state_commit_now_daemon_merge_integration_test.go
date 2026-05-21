package cmd_test

// Real-tmux integration regression test pinning the
// killed-session-resurrects-within-tick-window spec's "Daemon Merge
// Interaction (Verified Safe)" invariant under live conditions.
//
// Spec references:
//   - § Daemon Merge Interaction — "mergeSkippedPanes filters by live
//     structure, not by PrevIndex — it only retains prev panes that are
//     also present in the fresh capture. The killed session won't be in
//     fresh, so it won't be re-introduced regardless of PrevIndex
//     staleness."
//   - § Acceptance Criteria item 11 — "daemon-merge-reintroduces-dead-
//     sessions invariant preserved. The daemon's next tick (and every
//     subsequent tick) after a commit-now invocation must produce a
//     sessions.json whose set of session names does NOT contain the
//     killed session. Byte-equivalence between commit-now's output and
//     the daemon's next-tick output is NOT required — the daemon
//     legitimately enriches the schema with fields that commit-now
//     preserves from prev (scrollback hashes, per-pane content
//     references). The invariant is semantic: dead sessions stay out,
//     live sessions stay in."
//   - § Testing Requirements → Regression Tests → "Daemon merge
//     stability after commit-now".
//
// Why the gate exists despite by-inspection safety:
// The spec marks the merge interaction as verified safe by inspection
// of mergeSkippedPanes (internal/state/capture.go). This regression
// test pins that verdict against a future refactor that might
// over-generalise PrevIndex behaviour (e.g. restoring a session from
// PrevIndex when its name is missing from fresh structure). A
// regression would manifest as the killed session B reappearing in the
// daemon's post-commit-now sessions.json.
//
// Flow:
//
//  1. Stand up an isolated tmux server with sessions A and B via the
//     shared symptomFixture scaffold (which also bootstraps
//     _portal-saver, the daemon host, via the real bootstrap path).
//  2. Kill session B externally. The session-closed hook fires
//     commit-now synchronously; poll sessions.json until B is observed
//     absent.
//  3. Force the daemon's next tick by touching save.requested. The
//     daemon's 1-second ticker fires within ~1s; on the next tick the
//     daemon will read its in-memory (potentially stale) PrevIndex,
//     re-query live tmux via CaptureStructure (which omits the just-
//     killed B), and atomically rewrite sessions.json. A successful
//     tick removes save.requested as its post-commit step.
//  4. Wait for the daemon's tick to settle — observable as
//     save.requested being deleted by the daemon after captureAndCommit
//     succeeds.
//  5. Re-read sessions.json and assert on the SET of session names —
//     B absent, A present. Byte-equivalence is NOT asserted (the daemon
//     legitimately enriches schema fields that commit-now carries over
//     verbatim from prev).
//
// Skip behaviour mirrors the sibling real-tmux integration tests:
//   - tmuxtest.SkipIfNoTmux skips when tmux is not on PATH.
//   - portalbintest.StagePortalBinary skips cleanly if `go build`
//     fails — a dev machine without `go` or a broken build is not a
//     merge-stability failure.
//
// No t.Parallel: cmd-package convention forbids it (CLAUDE.md).
//
// Default-lane integration test (no //go:build integration tag),
// matching cmd/state_commit_now_symptom_integration_test.go and
// cmd/state_commit_now_reentrancy_integration_test.go.

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// daemonTickBudget bounds the wait for the daemon to consume a
// freshly-touched save.requested marker and produce a follow-up
// sessions.json write. The production daemon ticker period is 1s and a
// healthy tick completes well under that; 4s covers worst-case CI
// scheduling jitter, cold-cache `go build` amortisation, and the
// ~50-300ms aggregate per-pane wall time on the symptom fixture's two
// trivial user sessions.
const daemonTickBudget = 4 * time.Second

// daemonTickPollInterval is the cadence at which the test re-checks
// save.requested for the daemon's post-tick deletion. 50ms is short
// enough that the load-bearing 1s ticker fire is observed within a
// handful of polls without busy-spinning.
const daemonTickPollInterval = 50 * time.Millisecond

// TestCommitNowDaemonMergeStability is the regression gate for the
// daemon-merge-reintroduces-dead-sessions invariant (spec § Acceptance
// Criteria item 11). After a commit-now-driven sessions.json write
// that omits a killed session, the daemon's next tick must NOT
// re-introduce that session, regardless of how stale the daemon's
// in-memory PrevIndex is relative to the just-written file.
//
// The test asserts on the SET of session names in the post-daemon-tick
// sessions.json, not on byte-equivalence — the daemon legitimately
// repopulates per-pane scrollback hashes and content references that
// commit-now carries over verbatim from prev.
func TestCommitNowDaemonMergeStability(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// Build the portal binary once and PATH-prepend so every subprocess
	// (portal list, portal state commit-now via hook, portal state
	// daemon via _portal-saver) resolves to the freshly built binary.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	fixture := newSymptomFixture(t, binary, binDir, "ptl-merge-stable-")

	// Step 1: kill B externally. The session-closed hook fires
	// commit-now synchronously; the resulting sessions.json on disk
	// omits B before the hook subprocess returns. This is the
	// pre-condition for the merge-stability check: commit-now has
	// already produced a sessions.json that omits B, and we are
	// asking "does the daemon's NEXT tick respect that?".
	fixture.sock.Run(t, "kill-session", "-t", "B")

	ctx, cancel := context.WithTimeout(context.Background(), symptomKillBudget)
	defer cancel()
	if perr := pollSessionsJSON(ctx, fixture.stateDir, []string{"A"}, []string{"B"}); perr != nil {
		t.Fatalf(
			"commit-now did not remove B from sessions.json within %s "+
				"(pre-condition for merge-stability assertion): %v\n%s",
			symptomKillBudget, perr, fixture.diagnostic(),
		)
	}

	// Step 2: force the daemon's next tick by touching save.requested.
	// The daemon's 1s ticker observes the dirty flag and runs
	// captureAndCommit, which writes sessions.json and then removes
	// save.requested. A regression that re-introduces B via PrevIndex
	// staleness would still go through this same code path — the
	// failure mode would manifest as a sessions.json that contains B
	// even after the daemon successfully ticks. Polling on
	// save.requested deletion is the load-bearing readiness signal
	// that the daemon's commit cycle has completed.
	if err := state.TouchSaveRequested(fixture.stateDir); err != nil {
		t.Fatalf("touch save.requested to force daemon tick: %v\n%s", err, fixture.diagnostic())
	}

	// Step 3: wait for the daemon's tick to consume save.requested.
	// A successful captureAndCommit cycle removes the dirty flag as
	// its post-commit step; observing deletion proves the tick
	// completed without short-circuiting (the @portal-restoring
	// marker is clear in this fixture, so the only branch that could
	// skip removal is captureAndCommit returning an error, which
	// would also leave sessions.json unchanged and surface any merge
	// regression on the next forced tick).
	tickCtx, tickCancel := context.WithTimeout(context.Background(), daemonTickBudget)
	defer tickCancel()
	if err := waitForSaveRequestedConsumed(tickCtx, fixture.stateDir); err != nil {
		t.Fatalf(
			"daemon did not consume save.requested within %s "+
				"(daemon likely not running or wedged): %v\n%s",
			daemonTickBudget, err, fixture.diagnostic(),
		)
	}

	// Step 4: re-read sessions.json after the daemon's tick. The
	// semantic invariant (spec § Acceptance Criteria item 11) is on
	// the SET of session names: B absent, A present. Byte-equivalence
	// between commit-now's output and the daemon's post-tick output
	// is NOT required.
	idx, skip, err := state.ReadIndex(fixture.stateDir)
	if err != nil || skip {
		t.Fatalf(
			"post-daemon-tick ReadIndex: skip=%v err=%v\n%s",
			skip, err, fixture.diagnostic(),
		)
	}
	present := sessionNames(idx)

	t.Run("daemon's next tick after commit-now does not re-introduce the killed session by name", func(t *testing.T) {
		if _, reintroduced := present["B"]; reintroduced {
			t.Fatalf(
				"daemon-merge regression: killed session B re-introduced into sessions.json "+
					"after daemon's post-commit-now tick; "+
					"present session names = %v\n%s",
				keysOf(present), fixture.diagnostic(),
			)
		}
	})

	t.Run("daemon's next tick after commit-now retains all live sessions by name", func(t *testing.T) {
		if _, ok := present["A"]; !ok {
			t.Fatalf(
				"daemon-merge regression: live session A dropped from sessions.json "+
					"after daemon's post-commit-now tick; "+
					"present session names = %v\n%s",
				keysOf(present), fixture.diagnostic(),
			)
		}
	})
}

// waitForSaveRequestedConsumed polls until save.requested at the given
// state dir is observed absent (ENOENT) or until ctx is cancelled. A
// successful daemon captureAndCommit cycle removes save.requested as
// its post-commit step (see cmd/state_daemon.go tick()); observing the
// removal is the load-bearing readiness signal that the daemon's next
// tick after the test's touch has completed.
//
// Returns nil on observed deletion, ctx.Err on timeout, and a wrapped
// error for unexpected non-ENOENT stat failures (e.g. permission
// denied, which would indicate a fixture issue rather than a
// regression).
func waitForSaveRequestedConsumed(ctx context.Context, stateDir string) error {
	ticker := time.NewTicker(daemonTickPollInterval)
	defer ticker.Stop()
	path := state.SaveRequested(stateDir)
	for {
		_, err := os.Stat(path)
		switch {
		case err == nil:
			// File still present — daemon hasn't ticked yet.
		case errors.Is(err, fs.ErrNotExist):
			// File deleted — the daemon's post-commit step ran.
			return nil
		default:
			return fmt.Errorf("stat save.requested during poll: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
