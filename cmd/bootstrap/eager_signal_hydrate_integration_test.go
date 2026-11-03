//go:build integration

package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

func TestPhase1Integration_EagerSignalHydrate_MultiSessionMarkersClearedWithin2s(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	// Restored panes respawn into this binary's `state hydrate`, pinned through
	// restoretest.StagedRestoreAdapter, so the helper that unsets the markers
	// polled for below is the build under test rather than whatever release is
	// installed.
	binDir := restoretest.BuildPortalBinaryDir(t)

	cases := []struct {
		name     string
		sessions []string
	}{
		{"N=2_DefaultIndices", []string{"alpha", "beta"}},
		{"N=3_LargerSet", []string{"alpha", "beta", "gamma"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runEagerSignalMultiSessionAC1(t, binDir, tc.sessions)
		})
	}
}

func runEagerSignalMultiSessionAC1(t *testing.T, binDir string, sessions []string) {
	t.Helper()

	_, stateDir := newIntegrationStateDir(t)

	restoretest.SeedSessionsJSON(t, stateDir, sessions...)

	ts := tmuxtest.New(t, "ptl-eager-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	for _, name := range sessions {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("session %q unexpectedly live before Run", name)
		}
	}

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		Logger:  logger,
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q "+
				"(non-vacuity guard cannot be evaluated — Restore did "+
				"not skeleton-create)", name, liveOut)
		}
	}

	defer dumpPortalLogOnFailure(t, stateDir)
	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.PaneReactionBudget, restoretest.PaneReactionTick)
}

func dumpPortalLogOnFailure(t *testing.T, stateDir string) {
	t.Helper()
	if !t.Failed() {
		return
	}
	data, err := os.ReadFile(filepath.Join(stateDir, "portal.log"))
	if err != nil {
		return
	}
	t.Logf("portal.log contents on failure:\n%s", data)
}

func TestPhase1Integration_DaemonResumesCaptureAfterEagerSignal_AC4(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := newIntegrationStateDir(t)

	sessions := []string{"alpha", "beta"}
	restoretest.SeedSessionsJSON(t, stateDir, sessions...)

	ts := tmuxtest.New(t, "ptl-eager-ac4-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	for _, name := range sessions {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("session %q unexpectedly live before Run", name)
		}
	}

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		Logger:  logger,
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q",
				name, liveOut)
		}
	}

	defer dumpPortalLogOnFailure(t, stateDir)
	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.PaneReactionBudget, restoretest.PaneReactionTick)

	// Force a newline-terminated record into the pane: a freshly-exec'd shell
	// can leave only a partial prompt line, which is no record.
	betaPaneKey := state.SanitizePaneKey("beta", 0, 0)
	betaTarget := tmux.PaneTargetExact("beta", 0, 0)
	ts.SendKeys(t, betaTarget, "echo ac4-marker")
	waitForPaneText(t, client, betaTarget, "ac4-marker", restoretest.PaneReactionBudget, restoretest.PaneReactionTick)

	runDaemonTick(t, client, stateDir)

	scrollbackPath := state.ScrollbackFile(stateDir, betaPaneKey)
	scrollback, err := os.ReadFile(scrollbackPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read %s: %v", scrollbackPath, err)
	}
	if !bytes.Contains(scrollback, []byte("\n")) {
		t.Fatalf("AC4 violation: scrollback for non-attached pane %s "+
			"holds no terminated records after daemon tick; want "+
			"non-empty (Fix 1: eager-signal unset beta's marker → "+
			"daemon resumed capturing beta). scrollback path=%s",
			betaPaneKey, scrollbackPath)
	}
}

func TestPhase1Integration_DaemonSkipsCaptureWithoutEagerSignal_AC4NegativeControl(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := newIntegrationStateDir(t)

	sessions := []string{"alpha", "beta"}
	restoretest.SeedSessionsJSON(t, stateDir, sessions...)

	ts := tmuxtest.New(t, "ptl-eager-ac4-neg-")
	client := ts.Client()
	if _, err := client.EnsureServer(); err != nil {
		t.Fatalf("EnsureServer: %v", err)
	}

	for _, name := range sessions {
		if _, err := ts.TryRun("has-session", "-t", name); err == nil {
			t.Fatalf("session %q unexpectedly live before Run", name)
		}
	}

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore:       restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
		Logger:        logger,
	})

	defer dumpPortalLogOnFailure(t, stateDir)

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	liveOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, name := range sessions {
		if !strings.Contains(liveOut, name) {
			t.Fatalf("session %q not live after Run; list-sessions=%q",
				name, liveOut)
		}
	}

	betaPaneKey := state.SanitizePaneKey("beta", 0, 0)
	betaMarker := state.SkeletonMarkerPrefix + betaPaneKey
	_, found, err := client.TryGetServerOption(betaMarker)
	if err != nil {
		t.Fatalf("TryGetServerOption %s: %v", betaMarker, err)
	}
	if !found {
		t.Fatalf("marker %s absent after no-op eager-signal bootstrap; "+
			"want present (negative-control contract requires beta's "+
			"marker to survive when step 7 is the no-op signaler — "+
			"something else is clearing the marker and the assertion "+
			"below would be vacuously satisfied)", betaMarker)
	}

	betaTarget := tmux.PaneTargetExact("beta", 0, 0)
	ts.SendKeys(t, betaTarget, "echo ac4-negctrl")
	waitForPaneText(t, client, betaTarget, "ac4-negctrl", restoretest.PaneReactionBudget, restoretest.PaneReactionTick)

	runDaemonTick(t, client, stateDir)

	scrollbackPath := state.ScrollbackFile(stateDir, betaPaneKey)
	if _, err := os.Stat(scrollbackPath); !os.IsNotExist(err) {
		t.Fatalf("scrollback file %s exists after daemon tick under "+
			"NoOpEagerHydrateSignaler; want absent (skip-save guard "+
			"should suppress writes while beta's marker is still set — "+
			"a write here means the eager-signal seam fired despite "+
			"the no-op signaler, or the daemon's marker-aware "+
			"skip-save guard regressed). stat err=%v",
			scrollbackPath, err)
	}
}

func waitForPaneText(t *testing.T, client *tmux.Client, target tmux.Target, needle string, budget, tick time.Duration) {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		out, err := client.CapturePane(target)
		if err == nil && strings.Contains(out, needle) {
			return
		}
		time.Sleep(tick)
	}
	t.Fatalf("pane %s did not echo %q within %s", target, needle, budget)
}
