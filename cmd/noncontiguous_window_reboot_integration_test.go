//go:build integration

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/resumemode"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const (
	divergentSessionName = "divergentwin"

	// The window created between the two survivors and then killed: with
	// renumber-windows off the survivors keep their own indices rather than
	// closing the gap, which is what leaves the saved indices non-contiguous.
	divergentKilledWindow = 1

	divergentFirstMarker = "FIRST_WINDOW_HOOK_FIRED"
	divergentLastMarker  = "LAST_WINDOW_HOOK_FIRED"
)

// divergentPane names one pane of the fixture by role rather than by
// coordinates, which the whole test exists to distrust.
type divergentPane struct {
	role       string
	savedWin   int
	savedPane  int
	token      string
	markerFile string
	markerText string
}

type divergentRebootFixture struct {
	ts            *tmuxtest.Socket
	client        *tmux.Client
	stateDir      string
	binDir        string
	sideEffectDir string
	store         *hooks.Store

	stamped   []divergentPane
	unstamped divergentPane
	staleKey  string

	savedWindows    []int
	restoredWindows []int
	livePanes       []tmux.PaneCoord
	armedMarkers    map[string]struct{}
	armedFIFOKeys   []string
}

// divergentLivePane is one row of the single server-side pane enumeration every
// per-pane assertion reads from: the pane's durable token and the pane id its
// hook records as its own fire site.
type divergentLivePane struct {
	token  string
	paneID string
}

// divergentPaneRowFormat carries token, address and pane id in one row. The
// address half is written out literally rather than borrowing
// tmux.StructuralKeyFormat: here it is a lookup key for a live coordinate, not a
// structural key, and tying the two would couple this test to a key contract it
// does not participate in.
const divergentPaneRowFormat = tmux.HookKeyFormat +
	"|#{session_name}:#{window_index}.#{pane_index}|#{pane_id}"

// TestNonContiguousWindowReboot_KeepsTokenKeyedHooks drives the one moment no
// unit test reaches: a reboot whose restored window indices are not the ones
// that were saved. Every hook assertion here is only meaningful because the
// divergence guard below it runs first.
func TestNonContiguousWindowReboot_KeepsTokenKeyedHooks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test; -short")
	}
	tmuxtest.SkipIfNoTmux(t)

	fx := newDivergentRebootFixture(t)
	fx.reboot(t)

	t.Run("it restores a session whose saved window indices are non-contiguous", func(t *testing.T) {
		if _, err := fx.ts.TryRun("has-session", "-t", "="+divergentSessionName); err != nil {
			t.Fatalf("session %q not restored: %v", divergentSessionName, err)
		}
		if got, want := len(fx.livePanes), len(fx.stamped)+1; got != want {
			t.Fatalf("restored pane count = %d; want %d (panes=%v)", got, want, fx.livePanes)
		}
	})

	t.Run("it renumbers the restored windows away from the saved indices", func(t *testing.T) {
		if slices.Equal(fx.savedWindows, fx.restoredWindows) {
			t.Fatalf("restored window indices %v equal the saved ones %v; "+
				"a restore that did not renumber proves nothing about key durability",
				fx.restoredWindows, fx.savedWindows)
		}
	})

	// Past this point every assertion assumes the divergence, so a fixture that
	// failed to produce it must stop the run rather than report green subtests.
	if slices.Equal(fx.savedWindows, fx.restoredWindows) {
		t.Fatalf("aborting: saved and restored window indices both %v", fx.savedWindows)
	}

	fx.hydrate(t)

	// One enumeration serves both per-pane subtests below.
	liveRows := fx.livePaneRows(t)

	t.Run("it fires each pane's own hook after the reboot", func(t *testing.T) {
		// Two independent claims per pane, and both are needed. The marker
		// files alone only prove each command ran *somewhere*, so each hook
		// records the pane it actually ran in ($TMUX_PANE, which a
		// respawn-pane'd process inherits and which equals that pane's
		// #{pane_id}) — that pins the fire site directly rather than through
		// the stamp. The token check pins the pairing restore chose: restore
		// walks saved panes onto live ones by structural position, so the
		// trailing un-stamped sibling must come back carrying nothing.
		for i, live := range fx.livePanes {
			target := tmux.PaneTarget(divergentSessionName, live.Window, live.Pane)
			row, ok := liveRows[target]
			if !ok {
				t.Fatalf("live pane %s is absent from the live pane enumeration %v", target, liveRows)
			}

			wantToken := ""
			if i < len(fx.stamped) {
				wantToken = fx.stamped[i].token
			}
			if row.token != wantToken {
				t.Errorf("live pane %s (structural position %d) carries token %q; want %q — restore paired the wrong saved pane to it",
					target, i, row.token, wantToken)
			}
			if i < len(fx.stamped) {
				assertDivergentHookFiredInPane(t, fx.stamped[i], row.paneID)
			}
		}
	})

	t.Run("it fires no hook on the pane that carries no token", func(t *testing.T) {
		live := divergentLiveTokens(liveRows)
		for _, p := range fx.stamped {
			if !slices.Contains(live, p.token) {
				t.Errorf("stamped pane %s: token %q absent from the live enumeration %v", p.role, p.token, live)
			}
		}
		if got := len(live); got != len(fx.stamped) {
			t.Errorf("live token count = %d (%v); want %d — the un-stamped sibling must carry none",
				got, live, len(fx.stamped))
		}
		if got := divergentMarkerFileNames(t, fx.sideEffectDir); len(got) != len(fx.stamped) {
			t.Errorf("hook side-effect files = %v; want exactly one per stamped pane", got)
		}
	})

	t.Run("it pairs skeleton markers with the renumbered live coordinates", func(t *testing.T) {
		liveKeys := fx.livePaneKeys()
		if got := restoretest.SortedKeySet(fx.armedMarkers); !slices.Equal(got, liveKeys) {
			t.Errorf("skeleton markers = %v; want the live pane keys %v", got, liveKeys)
		}
		if !slices.Equal(fx.armedFIFOKeys, liveKeys) {
			t.Errorf("FIFOs present at arm time = %v; want one per live pane key %v", fx.armedFIFOKeys, liveKeys)
		}
		// The set equality above already says no marker names a saved
		// coordinate no live pane holds, and hydrate's
		// WaitForSkeletonMarkersCleared already fatals unless every marker
		// cleared, so neither needs restating here.
	})

	t.Run("it runs the sweep only after the restore marker clears", func(t *testing.T) {
		restoring, err := state.IsRestoringSet(fx.client)
		if err != nil {
			t.Fatalf("IsRestoringSet: %v", err)
		}
		if restoring {
			t.Fatalf("%s is still set; the sweep would stand down and the survival assertions would pass for the wrong reason",
				state.RestoringMarkerName)
		}
	})

	if err := sweepErr(fx.client, fx.store); err != nil {
		t.Fatalf("hooksweep.Run: %v", err)
	}
	swept, err := fx.store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("post-sweep store.Load: %v", err)
	}

	t.Run("it keeps both token-keyed entries across the post-restore sweep", func(t *testing.T) {
		for _, p := range fx.stamped {
			if _, ok := swept[p.token]; !ok {
				t.Errorf("hook for pane %s (token %q) was swept; want preserved — restore re-stamps the token, "+
					"so the key on disk still names a live pane whatever tmux did to the window indices. keys=%v",
					p.role, p.token, keysOf(swept))
			}
		}
	})

	t.Run("it still reaps a genuinely stale token-shaped key in the same sweep", func(t *testing.T) {
		if _, ok := swept[fx.staleKey]; ok {
			t.Errorf("stale key %q survived the sweep; survival must come from liveness, not from shape retention. keys=%v",
				fx.staleKey, keysOf(swept))
		}
	})
}

// newDivergentRebootFixture builds and saves the pre-reboot session: three
// windows, a split in the last, and the middle window killed under
// renumber-windows off so the saved indices are non-contiguous.
func newDivergentRebootFixture(t *testing.T) *divergentRebootFixture {
	t.Helper()

	binDir := restoretest.BuildPortalBinaryDir(t)

	_, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	hooksPath := filepath.Join(t.TempDir(), "hooks.json")
	t.Setenv("PORTAL_HOOKS_FILE", hooksPath)

	sideEffectDir := t.TempDir()
	fx := &divergentRebootFixture{
		stateDir:      stateDir,
		binDir:        binDir,
		sideEffectDir: sideEffectDir,
		store:         hooks.NewStore(hooksPath),
		staleKey:      hookstest.ReapableSeedA,
		stamped: []divergentPane{
			{
				role:       "first-window",
				savedWin:   0,
				savedPane:  0,
				token:      hookstest.LiveSeedA,
				markerFile: filepath.Join(sideEffectDir, "hook-first.txt"),
				markerText: divergentFirstMarker,
			},
			{
				role:       "last-window",
				savedWin:   divergentKilledWindow + 1,
				savedPane:  0,
				token:      hookstest.LiveSeedB,
				markerFile: filepath.Join(sideEffectDir, "hook-last.txt"),
				markerText: divergentLastMarker,
			},
		},
		unstamped: divergentPane{
			role:      "last-window-sibling",
			savedWin:  divergentKilledWindow + 1,
			savedPane: 1,
		},
	}

	fx.ts = tmuxtest.New(t, "ptl-3-5-diverge-")
	fx.client = fx.ts.Client()

	fx.buildNonContiguousSession(t)
	fx.stampTokens(t)
	fx.saveIndex(t)

	return fx
}

func (fx *divergentRebootFixture) buildNonContiguousSession(t *testing.T) {
	t.Helper()
	cwd := t.TempDir()
	target := divergentSessionName + ":"

	fx.ts.Run(t, "new-session", "-d", "-s", divergentSessionName, "-c", cwd, "sleep", "infinity")
	fx.ts.WaitForSession(t, divergentSessionName, 2*time.Second)
	fx.disableRenumberWindows(t)

	fx.ts.Run(t, "new-window", "-t", target, "-c", cwd, "sleep", "infinity")
	fx.ts.Run(t, "new-window", "-t", target, "-c", cwd, "sleep", "infinity")
	fx.ts.Run(t, "split-window", "-t", tmux.PaneTarget(divergentSessionName, divergentKilledWindow+1, 0),
		"-c", cwd, "sleep", "infinity")
	fx.ts.Run(t, "kill-window", "-t", fmt.Sprintf("%s:%d", divergentSessionName, divergentKilledWindow))
}

// disableRenumberWindows is applied to each server lifetime: a killed server
// loses the option, and the fixture's whole premise rests on it rather than on
// tmux's default.
func (fx *divergentRebootFixture) disableRenumberWindows(t *testing.T) {
	t.Helper()
	fx.ts.Run(t, "set-option", "-g", "renumber-windows", "off")
	if got := strings.TrimSpace(fx.ts.Run(t, "show-options", "-g", "-v", "renumber-windows")); got != "off" {
		t.Fatalf("renumber-windows = %q; want \"off\"", got)
	}
}

func (fx *divergentRebootFixture) stampTokens(t *testing.T) {
	t.Helper()
	for _, p := range fx.stamped {
		fx.ts.StampPaneToken(t, tmux.PaneTargetExact(divergentSessionName, p.savedWin, p.savedPane), p.token)
	}
}

// saveIndex captures the live topology, seeds one hook per stamped token plus a
// genuinely stale one, seeds each saved pane's scrollback, and persists the
// index the reboot will restore from.
func (fx *divergentRebootFixture) saveIndex(t *testing.T) {
	t.Helper()

	idx, _, err := state.CaptureStructure(fx.client, nil, nil, nil)
	if err != nil {
		t.Fatalf("CaptureStructure: %v", err)
	}
	sess := restoretest.FindCapturedSession(t, idx, divergentSessionName)

	for _, w := range sess.Windows {
		fx.savedWindows = append(fx.savedWindows, w.Index)
	}
	if !divergentIsNonContiguous(fx.savedWindows) {
		t.Fatalf("saved window indices %v are contiguous; the fixture must save a gap or the reboot proves nothing",
			fx.savedWindows)
	}

	for _, p := range fx.allPanes() {
		if got := divergentSavedToken(t, sess, p); got != p.token {
			t.Fatalf("saved token for pane %s (w%d.p%d) = %q; want %q",
				p.role, p.savedWin, p.savedPane, got, p.token)
		}
		restoretest.SeedScrollback(t, fx.stateDir, divergentSessionName, p.savedWin, p.savedPane,
			[]byte(fmt.Sprintf("before reboot: %s\n", p.role)))
	}

	for _, p := range fx.stamped {
		// $TMUX_PANE names the pane the hook actually ran in: the helper runs
		// the command under `sh -c`, and a respawn-pane'd process inherits it
		// from the pane's environment. Recording it is what makes a hook firing
		// in the wrong pane fail rather than pass.
		cmd := "echo " + p.markerText + " $TMUX_PANE >> " + p.markerFile
		if err := fx.store.Set(p.token, "on-resume", hooks.Registration{Command: cmd, Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
			t.Fatalf("hooks.Set %s: %v", p.role, err)
		}
	}
	if err := fx.store.Set(fx.staleKey, "on-resume", hooks.Registration{Command: "echo stale", Resume: resumemode.Eager}, hooks.ViaCLI); err != nil {
		t.Fatalf("hooks.Set stale: %v", err)
	}

	restoretest.WriteIndex(t, fx.stateDir, idx)
}

// reboot kills the server and restores from the saved index on a fresh one,
// recording the live topology and the skeleton markers restore armed.
func (fx *divergentRebootFixture) reboot(t *testing.T) {
	t.Helper()

	restoretest.RebootServer(t, fx.ts, fx.client)
	fx.disableRenumberWindows(t)

	if err := restoretest.RestoreFromState(t, fx.client, fx.stateDir, fx.binDir); err != nil {
		t.Fatalf("restore: %v", err)
	}

	livePanes, err := fx.client.ListPanesInSession(divergentSessionName)
	if err != nil {
		t.Fatalf("ListPanesInSession: %v", err)
	}
	fx.livePanes = livePanes
	for _, p := range livePanes {
		if !slices.Contains(fx.restoredWindows, p.Window) {
			fx.restoredWindows = append(fx.restoredWindows, p.Window)
		}
	}
	slices.Sort(fx.restoredWindows)

	markers, err := state.ListSkeletonMarkers(fx.client)
	if err != nil {
		t.Fatalf("ListSkeletonMarkers: %v", err)
	}
	fx.armedMarkers = markers

	// The FIFOs are read and unlinked by the hydrate helpers, so their presence
	// is recorded here, while the panes are still armed.
	for _, liveKey := range fx.livePaneKeys() {
		if _, err := os.Stat(state.FIFOPath(fx.stateDir, liveKey)); err == nil {
			fx.armedFIFOKeys = append(fx.armedFIFOKeys, liveKey)
		}
	}
	slices.Sort(fx.armedFIFOKeys)
}

func (fx *divergentRebootFixture) hydrate(t *testing.T) {
	t.Helper()
	restoretest.DriveSignalHydrate(t, fx.client, fx.stateDir, []string{divergentSessionName})
	restoretest.WaitForSkeletonMarkersCleared(t, fx.client, restoretest.HydrateBudget, restoretest.HydrateTick)
	// A count, not a presence: the helper unsets its skeleton marker before it
	// execs the hook, so the marker-clear above says nothing about how many
	// times the hook then ran.
	for _, p := range fx.stamped {
		restoretest.AssertMarkerCount(t, p.markerFile, p.markerText, 1)
	}
}

// livePaneRows takes the whole live-pane identity map from ONE server-side
// enumeration, keyed by pane address. Resolving no `-t` target is the point:
// `display-message -p -t <coordinate no pane answers to>` does not fail — tmux
// falls back to the session's *current* pane and exits 0 (verified on 3.7c), so
// a per-pane read would quietly answer with a neighbour's identity, which is
// precisely the confusion these assertions exist to detect. A missing address in
// the returned map is an honest "no pane is there".
func (fx *divergentRebootFixture) livePaneRows(t *testing.T) map[string]divergentLivePane {
	t.Helper()
	raw, err := fx.client.ListAllPanesWithFormat(divergentPaneRowFormat)
	if err != nil {
		t.Fatalf("ListAllPanesWithFormat: %v", err)
	}
	rows := make(map[string]divergentLivePane)
	for line := range strings.SplitSeq(raw, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "|")
		if len(fields) != 3 {
			t.Fatalf("unparseable live pane row %q; want token|address|pane_id", line)
		}
		rows[fields[1]] = divergentLivePane{token: fields[0], paneID: fields[2]}
	}
	return rows
}

// divergentLiveTokens is every non-empty token on the server, from the same
// single enumeration — an un-stamped pane contributes none.
func divergentLiveTokens(rows map[string]divergentLivePane) []string {
	var tokens []string
	for _, row := range rows {
		if row.token != "" {
			tokens = append(tokens, row.token)
		}
	}
	slices.Sort(tokens)
	return tokens
}

func (fx *divergentRebootFixture) livePaneKeys() []string {
	keys := make([]string, 0, len(fx.livePanes))
	for _, live := range fx.livePanes {
		keys = append(keys, state.SanitizePaneKey(divergentSessionName, live.Window, live.Pane))
	}
	slices.Sort(keys)
	return keys
}

// allPanes is the fixture's full saved topology: the stamped panes plus the
// un-stamped sibling.
func (fx *divergentRebootFixture) allPanes() []divergentPane {
	return append(slices.Clone(fx.stamped), fx.unstamped)
}

func divergentIsNonContiguous(indices []int) bool {
	for i := 1; i < len(indices); i++ {
		if indices[i] != indices[i-1]+1 {
			return true
		}
	}
	return false
}

func divergentSavedToken(t *testing.T, sess state.Session, p divergentPane) string {
	t.Helper()
	for _, w := range sess.Windows {
		if w.Index != p.savedWin {
			continue
		}
		for _, sp := range w.Panes {
			if sp.Index == p.savedPane {
				return sp.PortalPaneID
			}
		}
	}
	t.Fatalf("captured session has no pane w%d.p%d for %s", p.savedWin, p.savedPane, p.role)
	return ""
}

// assertDivergentHookFiredInPane pins both halves of "fires exactly once, in its
// own pane": the hook is a single `echo <marker> $TMUX_PANE >> <file>`, so the
// file's whole content is deterministic down to the id of the pane it ran in.
// Comparing it exactly against the pane expected to have run it catches a second
// firing, a hook that fired in a sibling pane, another marker leaking in, and any
// unexpected extra output alike.
func assertDivergentHookFiredInPane(t *testing.T, p divergentPane, wantPaneID string) {
	t.Helper()
	data, err := os.ReadFile(p.markerFile)
	if err != nil {
		t.Fatalf("read hook side-effect file for pane %s (a missed hook leaves it absent): %v", p.role, err)
	}
	if got, want := string(data), p.markerText+" "+wantPaneID+"\n"; got != want {
		t.Errorf("pane %s: hook side-effect file = %q; want exactly %q — the recorded pane id is the pane the hook really ran in",
			p.role, got, want)
	}
}

func divergentMarkerFileNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read side-effect dir %s: %v", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}
