//go:build integration

package cmd_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const daemonTickBudget = 4 * time.Second

const daemonTickPollInterval = 50 * time.Millisecond

// The SET of session names rather than byte-equivalence: the daemon legitimately
// repopulates per-pane scrollback hashes and content references that commit-now
// carries over verbatim from prev.
func TestCommitNowDaemonMergeStability(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)

	// PATH-prepend so every portal subprocess resolves to the freshly built binary.
	binDir := portalbintest.StagePortalBinary(t)
	binary, err := exec.LookPath("portal")
	if err != nil {
		t.Skipf("portal not on PATH after build+prepend; skipping: %v", err)
	}

	fixture := newSymptomFixture(t, binary, binDir, "ptl-merge-stable-")

	// Pre-condition: commit-now must have already produced a sessions.json that
	// omits B, so the question below is whether the daemon's next tick respects it.
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

	// Forces the daemon's next tick.
	if err := state.TouchSaveRequested(fixture.stateDir); err != nil {
		t.Fatalf("touch save.requested to force daemon tick: %v\n%s", err, fixture.diagnostic())
	}

	tickCtx, tickCancel := context.WithTimeout(context.Background(), daemonTickBudget)
	defer tickCancel()
	if err := waitForForcedTickSettled(tickCtx, fixture.stateDir); err != nil {
		t.Fatalf(
			"daemon's forced tick did not settle within %s "+
				"(daemon likely not running or wedged): %v\n%s",
			daemonTickBudget, err, fixture.diagnostic(),
		)
	}
	if strings.Contains(portaltest.ReadPortalLogSafe(fixture.stateDir), "daemon: tick failed") {
		t.Fatalf(
			"daemon's forced tick failed rather than committed; the reads below "+
				"would see commit-now's sessions.json, not the daemon's\n%s\n--- portal.log ---\n%s",
			fixture.diagnostic(), portaltest.ReadPortalLogSafe(fixture.stateDir),
		)
	}

	t.Run("daemon's next tick after commit-now does not re-introduce the killed session by name", func(t *testing.T) {
		idx, skip, err := state.ReadIndex(fixture.stateDir)
		if err != nil || skip {
			t.Fatalf(
				"post-daemon-tick ReadIndex: skip=%v err=%v\n%s",
				skip, err, fixture.diagnostic(),
			)
		}
		present := sessionNames(idx)
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
		idx, skip, err := state.ReadIndex(fixture.stateDir)
		if err != nil || skip {
			t.Fatalf(
				"post-daemon-tick ReadIndex: skip=%v err=%v\n%s",
				skip, err, fixture.diagnostic(),
			)
		}
		present := sessionNames(idx)
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

// waitForForcedTickSettled waits until the tick forced by a touch of
// save.requested has returned. The tick consumes the flag before its cycle runs,
// so its disappearance alone says only that the cycle has started. Ticks run
// one after another, so a second touch consumed means a later tick has started
// — and therefore that the forced one has returned.
func waitForForcedTickSettled(ctx context.Context, stateDir string) error {
	if err := waitForSaveRequestedConsumed(ctx, stateDir); err != nil {
		return fmt.Errorf("forced tick did not start: %w", err)
	}
	if err := state.TouchSaveRequested(stateDir); err != nil {
		return fmt.Errorf("touch save.requested to order a later tick: %w", err)
	}
	if err := waitForSaveRequestedConsumed(ctx, stateDir); err != nil {
		return fmt.Errorf("no tick started after the forced one: %w", err)
	}
	return nil
}

// waitForSaveRequestedConsumed waits for save.requested to disappear, which a
// tick does as it starts its cycle.
func waitForSaveRequestedConsumed(ctx context.Context, stateDir string) error {
	ticker := time.NewTicker(daemonTickPollInterval)
	defer ticker.Stop()
	path := state.SaveRequested(stateDir)
	for {
		_, err := os.Stat(path)
		switch {
		case err == nil:
		case errors.Is(err, fs.ErrNotExist):
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
