//go:build integration

package bootstrap_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

type roundTripCfg struct {
	saveBase, savePaneBase       int
	restoreBase, restorePaneBase int
	useBinary                    bool
}

func TestPhase5RebootRoundTripEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 0, restorePaneBase: 0,
		useBinary: true,
	})
}

func TestPhase5RebootRoundTripBaseIndexDrift(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	runRebootRoundTrip(t, roundTripCfg{
		saveBase: 0, savePaneBase: 0,
		restoreBase: 1, restorePaneBase: 1,
		useBinary: false,
	})
}

func runRebootRoundTrip(t *testing.T, cfg roundTripCfg) {
	t.Helper()

	// Restored panes respawn into this binary's `state hydrate`, pinned through
	// restoretest.StagedRestoreAdapter, so the pane survives to the assertions on
	// the build under test rather than on whatever release is installed.
	binDir := restoretest.BuildPortalBinaryDir(t)

	env, stateDir := newIntegrationStateDir(t)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	hookFireFile := filepath.Join(t.TempDir(), "hook-fired.txt")

	cwdAlphaW0 := t.TempDir()
	cwdAlphaW1 := t.TempDir()
	cwdBeta := t.TempDir()

	envValue := "round-trip-test-value"

	const savedHookKey = "alphaPaneToken"

	hookCmd := fmt.Sprintf("echo HOOK_FIRED >> %s", hookFireFile)
	store := hooks.NewStore(hooksPath)
	if err := store.Set(savedHookKey, "on-resume", hookCmd, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set: %v", err)
	}

	ts := tmuxtest.New(t, "ptl-rt-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)

	tmuxtest.ApplyBaseIndices(t, ts, cfg.saveBase, cfg.savePaneBase)

	createSavedTopology(t, ts, savedTopologyArgs{
		envValue:   envValue,
		cwdAlphaW0: cwdAlphaW0,
		cwdAlphaW1: cwdAlphaW1,
		cwdBeta:    cwdBeta,
		base:       cfg.saveBase,
		paneBase:   cfg.savePaneBase,
	})

	hookPaneTarget := tmux.PaneTargetExact("alpha", cfg.saveBase+0, cfg.savePaneBase+0)
	ts.StampPaneToken(t, hookPaneTarget, savedHookKey)

	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())

	restoretest.SeedScrollback(t, stateDir, "alpha", cfg.saveBase+0, cfg.savePaneBase+0,
		[]byte(restoretest.ANSIScrollback))

	verifyCapturedIndex(t, idx, cfg)

	restoretest.RebootServer(t, ts, client)
	// Base indices are server-lifetime options, so the drift under test is
	// re-applied onto the fresh server.
	tmuxtest.ApplyBaseIndices(t, ts, cfg.restoreBase, cfg.restorePaneBase)

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})

	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	verifyPostBootstrapSessionSet(t, ts, client,
		[]string{tmux.PortalBootstrapName, tmux.PortalSaverName},
		[]string{"alpha", "beta"})

	verifyLiveStructure(t, ts, cfg)
	verifyLayoutAndZoom(t, ts, cfg)
	verifyCWDs(t, ts, cfg, cwdAlphaW0, cwdAlphaW1, cwdBeta)
	verifyEnvironment(t, client, "alpha",
		"PORTAL_TEST_ENV", envValue)

	if cfg.useBinary {
		restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
			stateDir, hooksPath, []string{"alpha", "beta"}, env)
	} else {
		restoretest.DriveSignalHydrate(t, client, stateDir,
			[]string{"alpha", "beta"})
	}

	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.HydrateBudget, restoretest.HydrateTick)

	verifyANSIScrollback(t, ts, "alpha",
		cfg.restoreBase+0, cfg.restorePaneBase+0)

	restoretest.AssertMarkerCount(t, hookFireFile, "HOOK_FIRED", 1)

	verifyNoPredictedVsLiveWarns(t, filepath.Join(stateDir, "portal.log"))
}

func assertNoLogLineMatches(t *testing.T, logPath string, pred func(string) bool, failFmt string, args ...any) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("read portal.log %s: %v", logPath, err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if pred(line) {
			t.Fatalf(failFmt, append(args, line)...)
		}
	}
}

func verifyNoPredictedVsLiveWarns(t *testing.T, logPath string) {
	t.Helper()
	assertNoLogLineMatches(t, logPath, predictedVsLiveWarnRegex.MatchString,
		"portal.log contains predicted-vs-live WARN line "+
			"(spec AC #4 — diagnostic must be gone, not silenced): %s")
}

type savedTopologyArgs struct {
	envValue   string
	cwdAlphaW0 string
	cwdAlphaW1 string
	cwdBeta    string
	base       int
	paneBase   int
}

func createSavedTopology(t *testing.T, ts *tmuxtest.Socket, args savedTopologyArgs) {
	t.Helper()
	ts.Run(t, "new-session", "-d", "-s", "alpha", "-c", args.cwdAlphaW0, "sleep", "infinity")
	ts.WaitForSession(t, "alpha", 2*time.Second)

	ts.Run(t, "set-environment", "-t", "alpha", "PORTAL_TEST_ENV", args.envValue)

	ts.Run(t, "split-window", "-t", "alpha", "-c", args.cwdAlphaW0, "sleep", "infinity")

	ts.Run(t, "new-window", "-t", "alpha", "-c", args.cwdAlphaW1, "sleep", "infinity")

	zoomTarget := tmux.PaneTarget("alpha", args.base+0, args.paneBase+1)
	ts.Run(t, "resize-pane", "-t", zoomTarget, "-Z")

	ts.Run(t, "new-session", "-d", "-s", "beta", "-c", args.cwdBeta, "sleep", "infinity")
	ts.WaitForSession(t, "beta", 2*time.Second)
}

func verifyCapturedIndex(t *testing.T, idx state.Index, cfg roundTripCfg) {
	t.Helper()
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2", got)
	}
	if idx.Sessions[0].Name != "alpha" || idx.Sessions[1].Name != "beta" {
		t.Fatalf("session names = [%s, %s]; want [alpha, beta]",
			idx.Sessions[0].Name, idx.Sessions[1].Name)
	}
	alpha := idx.Sessions[0]
	if got := len(alpha.Windows); got != 2 {
		t.Fatalf("alpha windows = %d; want 2", got)
	}
	if got := len(alpha.Windows[0].Panes); got != 2 {
		t.Fatalf("alpha w0 panes = %d; want 2", got)
	}
	if got := len(alpha.Windows[1].Panes); got != 1 {
		t.Fatalf("alpha w1 panes = %d; want 1", got)
	}
	if !alpha.Windows[0].Zoomed {
		t.Fatalf("alpha w0 not zoomed in capture; want zoomed=true")
	}
	if alpha.Windows[0].Index != cfg.saveBase {
		t.Errorf("alpha w0.Index = %d; want %d", alpha.Windows[0].Index, cfg.saveBase)
	}
	if alpha.Windows[0].Panes[0].Index != cfg.savePaneBase {
		t.Errorf("alpha w0p0.Index = %d; want %d", alpha.Windows[0].Panes[0].Index, cfg.savePaneBase)
	}
	if got := alpha.Environment["PORTAL_TEST_ENV"]; got == "" {
		t.Errorf("alpha env PORTAL_TEST_ENV missing in capture; got %v", alpha.Environment)
	}
}

func verifyLiveStructure(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("session %q missing post-restore; got %q", want, out)
		}
	}
	alphaPanes := restoretest.LivePaneCoords(t, ts, "alpha")
	wantAlphaPanes := []string{
		fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+0),
		fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+1),
		fmt.Sprintf("%d:%d", cfg.restoreBase+1, cfg.restorePaneBase+0),
	}
	for _, want := range wantAlphaPanes {
		if !slices.Contains(alphaPanes, want) {
			t.Errorf("alpha live pane %q missing; got %q", want, alphaPanes)
		}
	}
	betaPanes := restoretest.LivePaneCoords(t, ts, "beta")
	wantBeta := fmt.Sprintf("%d:%d", cfg.restoreBase+0, cfg.restorePaneBase+0)
	if !slices.Contains(betaPanes, wantBeta) {
		t.Errorf("beta live pane %q missing; got %q", wantBeta, betaPanes)
	}
}

func verifyPostBootstrapSessionSet(t *testing.T, ts *tmuxtest.Socket, client *tmux.Client, allowedReserved []string, expectedRestored []string) {
	t.Helper()

	rawOut := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	rawSet := map[string]struct{}{}
	for line := range strings.SplitSeq(rawOut, "\n") {
		name := strings.TrimSpace(line)
		if name == "" {
			continue
		}
		rawSet[name] = struct{}{}
	}

	allowed := map[string]struct{}{}
	for _, n := range allowedReserved {
		allowed[n] = struct{}{}
	}
	for _, n := range expectedRestored {
		allowed[n] = struct{}{}
	}

	for name := range rawSet {
		if _, ok := allowed[name]; !ok {
			t.Errorf("raw tmux session list contains unexpected name %q; "+
				"allowed reserved=%v, expected restored=%v, raw=%v",
				name, allowedReserved, expectedRestored,
				sortedStringSet(rawSet))
		}
	}

	sessions, err := client.ListSessions()
	if err != nil {
		t.Fatalf("Client.ListSessions: %v", err)
	}
	gotSet := map[string]struct{}{}
	for _, s := range sessions {
		gotSet[s.Name] = struct{}{}
	}

	for _, reserved := range []string{tmux.PortalBootstrapName, tmux.PortalSaverName} {
		if _, leaked := gotSet[reserved]; leaked {
			t.Errorf("Client.ListSessions leaked reserved name %q; "+
				"underscore-prefix filter regression. got=%v",
				reserved, sortedStringSet(gotSet))
		}
	}

	expectedSet := map[string]struct{}{}
	for _, n := range expectedRestored {
		expectedSet[n] = struct{}{}
	}
	for name := range gotSet {
		if _, ok := expectedSet[name]; !ok {
			t.Errorf("Client.ListSessions returned unexpected name %q; "+
				"expected restored=%v, got=%v",
				name, expectedRestored, sortedStringSet(gotSet))
		}
	}
	for _, want := range expectedRestored {
		if _, ok := gotSet[want]; !ok {
			t.Errorf("Client.ListSessions missing expected restored name %q; got=%v",
				want, sortedStringSet(gotSet))
		}
	}
}

func sortedStringSet(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func verifyLayoutAndZoom(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg) {
	t.Helper()
	w0 := ts.Run(t, "display-message", "-p",
		"-t", fmt.Sprintf("alpha:%d", cfg.restoreBase+0),
		"#{window_zoomed_flag}")
	if strings.TrimSpace(w0) != "1" {
		t.Errorf("alpha:%d zoom flag = %q; want 1", cfg.restoreBase+0, w0)
	}
	w1 := ts.Run(t, "display-message", "-p",
		"-t", fmt.Sprintf("alpha:%d", cfg.restoreBase+1),
		"#{window_zoomed_flag}")
	if strings.TrimSpace(w1) != "0" {
		t.Errorf("alpha:%d zoom flag = %q; want 0", cfg.restoreBase+1, w1)
	}
}

func verifyCWDs(t *testing.T, ts *tmuxtest.Socket, cfg roundTripCfg, cwdAlphaW0, cwdAlphaW1, cwdBeta string) {
	t.Helper()
	cases := []struct {
		target string
		want   string
	}{
		{tmux.PaneTarget("alpha", cfg.restoreBase+0, cfg.restorePaneBase+0), cwdAlphaW0},
		{tmux.PaneTarget("alpha", cfg.restoreBase+0, cfg.restorePaneBase+1), cwdAlphaW0},
		{tmux.PaneTarget("alpha", cfg.restoreBase+1, cfg.restorePaneBase+0), cwdAlphaW1},
		{tmux.PaneTarget("beta", cfg.restoreBase+0, cfg.restorePaneBase+0), cwdBeta},
	}
	for _, c := range cases {
		got := strings.TrimSpace(ts.Run(t, "display-message", "-p",
			"-t", c.target, "#{pane_current_path}"))
		gotResolved := resolveSymlinks(got)
		wantResolved := resolveSymlinks(c.want)
		if gotResolved != wantResolved {
			t.Errorf("cwd %s = %q (resolved %q); want %q (resolved %q)",
				c.target, got, gotResolved, c.want, wantResolved)
		}
	}
}

func resolveSymlinks(p string) string {
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		return p
	}
	return r
}

func verifyEnvironment(t *testing.T, client *tmux.Client, session, key, want string) {
	t.Helper()
	out, err := client.ShowEnvironment(session)
	if err != nil {
		t.Fatalf("ShowEnvironment %q: %v", session, err)
	}
	wantLine := key + "=" + want
	if !strings.Contains(out, wantLine) {
		t.Errorf("session %q env missing %q; got:\n%s", session, wantLine, out)
	}
}

func verifyANSIScrollback(t *testing.T, ts *tmuxtest.Socket, session string, win, pane int) {
	t.Helper()
	target := tmux.PaneTarget(session, win, pane)
	out := ts.Run(t, "capture-pane", "-e", "-p", "-S", "-", "-t", target)
	checks := []struct {
		needle string
		desc   string
	}{
		{"\x1b[31m", "red SGR escape"},
		{"red", "red literal"},
		{"before reboot", "fixture text"},
	}
	for _, c := range checks {
		if !strings.Contains(out, c.needle) {
			t.Errorf("scrollback for %s missing %s (%q); got:\n%q",
				target, c.desc, c.needle, out)
		}
	}
}

func TestPhase5RebootRoundTripBothSessionsHydrateViaSignalHydrateBinary(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	binDir := restoretest.BuildPortalBinaryDir(t)

	env, stateDir := newIntegrationStateDir(t)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	cwdAlpha := t.TempDir()
	cwdBeta := t.TempDir()

	ts := tmuxtest.New(t, "ptl-rt-switch-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)

	ts.Run(t, "new-session", "-d", "-s", "alpha", "-c", cwdAlpha, "sleep", "infinity")
	ts.WaitForSession(t, "alpha", 2*time.Second)
	ts.Run(t, "new-session", "-d", "-s", "beta", "-c", cwdBeta, "sleep", "infinity")
	ts.WaitForSession(t, "beta", 2*time.Second)

	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())
	if got := len(idx.Sessions); got != 2 {
		t.Fatalf("captured %d sessions; want 2", got)
	}

	// The gap only: this fixture's bootstrap starts the server itself at step 1,
	// and finding one already up would answer the question it is here to ask.
	restoretest.OpenRebootGap(t, ts)

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	verifyPostBootstrapSessionSet(t, ts, client,
		[]string{tmux.PortalBootstrapName, tmux.PortalSaverName},
		[]string{"alpha", "beta"})

	markersBefore, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers (pre-drive): %v", err)
	}
	if len(markersBefore) != 2 {
		t.Fatalf("expected 2 skeleton markers before drive; got %d (%v)",
			len(markersBefore), restoretest.SortedKeySet(markersBefore))
	}

	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{"alpha"}, env)

	waitForSessionMarkerCleared(t, client, "alpha", restoretest.HydrateBudget)

	markersMid, err := state.ListSkeletonMarkers(client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers (mid-drive): %v", err)
	}
	if len(markersMid) != 1 {
		t.Errorf("after alpha-only signal, expected 1 marker still set (beta); got %d (%v)",
			len(markersMid), restoretest.SortedKeySet(markersMid))
	}

	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{"beta"}, env)

	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.HydrateBudget, restoretest.HydrateTick)

	verifySwitchClientLiveStructure(t, ts)
}

func waitForSessionMarkerCleared(t *testing.T, client *tmux.Client, session string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	prefix := session + "__"
	for time.Now().Before(deadline) {
		markers, err := state.ListSkeletonMarkers(client)
		if err != nil {
			t.Fatalf("ListSkeletonMarkers: %v", err)
		}
		stillSet := false
		for k := range markers {
			if strings.HasPrefix(k, prefix) {
				stillSet = true
				break
			}
		}
		if !stillSet {
			return
		}
		time.Sleep(restoretest.HydrateTick)
	}
	markers, _ := state.ListSkeletonMarkers(client)
	t.Fatalf("session %q skeleton markers still set after %s; markers=%v",
		session, timeout, restoretest.SortedKeySet(markers))
}

func verifySwitchClientLiveStructure(t *testing.T, ts *tmuxtest.Socket) {
	t.Helper()
	out := ts.Run(t, "list-sessions", "-F", "#{session_name}")
	for _, want := range []string{"alpha", "beta"} {
		if !strings.Contains(out, want) {
			t.Errorf("session %q missing post-hydrate; got %q", want, out)
		}
	}
	for _, sess := range []string{"alpha", "beta"} {
		panes := restoretest.LivePaneCoords(t, ts, sess)
		if !slices.Contains(panes, "0:0") {
			t.Errorf("%s live pane 0:0 missing; got %q", sess, panes)
		}
	}
}

func TestRebootRoundTrip_LeadingDashSessionName(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	const sessionName = "-dotfiles-test"
	const saveBase, savePaneBase = 1, 1
	const restoreBase, restorePaneBase = 1, 1

	binDir := restoretest.BuildPortalBinaryDir(t)
	// This fixture registers the real global hooks, whose bodies invoke `portal`
	// by name through run-shell — so unlike its siblings it needs the staged
	// binary on PATH as well as pinned into the restore's arming.
	restoretest.PrependPATH(t, binDir)

	env, stateDir := newIntegrationStateDir(t)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	cwd := t.TempDir()

	ts := tmuxtest.New(t, "ptl-rt-leadingdash-")
	client := ts.Client()

	ts.Run(t, "new-session", "-d", "-s", "_seed")
	ts.WaitForSession(t, "_seed", 2*time.Second)
	tmuxtest.ApplyBaseIndices(t, ts, saveBase, savePaneBase)

	createLeadingDashSession(t, ts, sessionName, cwd)
	ts.WaitForSession(t, sessionName, 2*time.Second)

	idx := runDaemonTick(t, client, stateDir, withoutSkipGuard(), withEmptyScrollback())
	if got := len(idx.Sessions); got != 1 {
		t.Fatalf("captured %d sessions; want 1", got)
	}
	if idx.Sessions[0].Name != sessionName {
		t.Fatalf("captured session name = %q; want %q", idx.Sessions[0].Name, sessionName)
	}

	restoretest.SeedScrollback(t, stateDir, sessionName, saveBase+0, savePaneBase+0,
		[]byte(restoretest.ANSIScrollback))

	restoretest.RebootServer(t, ts, client)
	// Base indices are server-lifetime options, so they are re-applied onto the
	// fresh server.
	tmuxtest.ApplyBaseIndices(t, ts, restoreBase, restorePaneBase)

	logger := restoretest.OpenTestLogger(t, stateDir)

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Hooks:   &bootstrapadapter.HookRegistrar{Client: client, Logger: logger},
		Restore: restoretest.StagedRestoreAdapter(t, client, stateDir, logger, binDir),
		Sweeper: &bootstrapadapter.FIFOSweeper{
			Client:   client,
			StateDir: stateDir,
			Logger:   logger,
		},
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})
	if _, _, err := o.Run(context.Background()); err != nil {
		t.Fatalf("Orchestrator.Run: %v", err)
	}

	verifyHydrationHookEntries(t, client)

	restoretest.DriveSignalHydrateBinary(t, binDir, ts.SocketPath(),
		stateDir, hooksPath, []string{sessionName}, env)

	restoretest.WaitForSkeletonMarkersCleared(t, client, restoretest.HydrateBudget, restoretest.HydrateTick)

	verifyNoHydrateTimeoutWarns(t, filepath.Join(stateDir, "portal.log"), sessionName)

	verifyANSIScrollback(t, ts, sessionName, restoreBase+0, restorePaneBase+0)
}

func createLeadingDashSession(t *testing.T, ts *tmuxtest.Socket, name, cwd string) {
	t.Helper()
	if _, err := ts.TryRun("new-session", "-d", "-s", name, "-c", cwd, "sleep", "infinity"); err == nil {
		return
	}
	if _, err := ts.TryRun("new-session", "-d", "-s", "--", name, "-c", cwd, "sleep", "infinity"); err == nil {
		return
	}
	t.Fatalf("could not create leading-dash session %q via positional or `--` form; tmux CLI rejected both shapes", name)
}

func verifyHydrationHookEntries(t *testing.T, client *tmux.Client) {
	t.Helper()
	for _, event := range tmux.HydrationTriggerEvents {
		raw, err := client.ShowGlobalHooksForEvent(event)
		if err != nil {
			t.Fatalf("ShowGlobalHooksForEvent(%s): %v", event, err)
		}
		entries := tmux.ParseShowHooks(raw)[event]
		var matching []string
		for _, e := range entries {
			if strings.Contains(e.Command, "portal state signal-hydrate") {
				matching = append(matching, e.Command)
			}
		}
		if len(matching) != 1 {
			t.Errorf("event %q: %d entries contain `portal state signal-hydrate`; want 1\nentries:\n%s",
				event, len(matching), strings.Join(matching, "\n"))
			continue
		}
		if !strings.Contains(matching[0], "portal state signal-hydrate -- ") {
			t.Errorf("event %q: entry missing `-- ` separator before #{session_name}; got %q",
				event, matching[0])
		}
	}
}

func verifyNoHydrateTimeoutWarns(t *testing.T, logPath, session string) {
	t.Helper()
	pred := func(line string) bool {
		return strings.Contains(line, "hydrate timeout") && strings.Contains(line, session)
	}
	assertNoLogLineMatches(t, logPath, pred,
		"portal.log contains `hydrate timeout` WARN for session %q: %s", session)
}
