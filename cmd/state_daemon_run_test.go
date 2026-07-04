// Tests in this file mutate package-level state via Cobra and MUST NOT use t.Parallel.
package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// daemonFakeCommander is a tmux.Commander that dispatches per-command and
// records every invocation. Tests configure the per-command outputs they care
// about via the dispatch maps; unset commands return ("", nil) so unrelated
// tmux calls do not fail the test.
//
// The mutex is necessary because RunRaw is invoked from CaptureAndHashPane in
// captureAndCommit, while Run is invoked from various other tmux helpers — the
// daemon itself is single-threaded but the in-test goroutines (defaultDaemonRun
// path) may interleave.
type daemonFakeCommander struct {
	mu sync.Mutex

	// markersOut is the stdout for "show-options -sv".
	markersOut string
	markersErr error

	// optionByName maps option name → value for "show-option -sv <name>".
	// optionErr seeds an error returned for any show-option call.
	optionByName map[string]string
	optionErr    error

	// sessionsOut is the stdout for "list-sessions ...".
	sessionsOut string
	sessionsErr error

	// panesOut is the stdout for "list-panes -a -F ...".
	panesOut string
	panesErr error

	// envBySession maps session name → "show-environment -t <name>" output.
	envBySession map[string]string

	// captureByTarget maps capture target → "capture-pane ..." output.
	// captureErrByTarget seeds errors per target.
	captureByTarget    map[string]string
	captureErrByTarget map[string]error

	// dispatchHook, if non-nil, is invoked after every dispatch resolution
	// with the args of the just-handled call. Used by ctx-cancellation tests
	// to fire cancel() while a tmux subcall is "in flight" inside
	// CaptureStructure (e.g. on show-environment) so the surrounding code
	// observes the cancellation at the next ctx.Done() check.
	dispatchHook func(args []string)

	// commitErr, if non-nil, is returned for the "set-option -s" / similar
	// writes — currently unused since Commit writes via os.WriteFile, not tmux.

	// Recorded calls.
	calls    [][]string
	rawCalls [][]string
}

func (c *daemonFakeCommander) Run(args ...string) (string, error) {
	c.mu.Lock()
	c.calls = append(c.calls, append([]string(nil), args...))
	hook := c.dispatchHook
	c.mu.Unlock()
	out, err := c.dispatch(args)
	if hook != nil {
		hook(args)
	}
	return out, err
}

func (c *daemonFakeCommander) RunRaw(args ...string) (string, error) {
	c.mu.Lock()
	c.rawCalls = append(c.rawCalls, append([]string(nil), args...))
	hook := c.dispatchHook
	c.mu.Unlock()
	out, err := c.dispatch(args)
	if hook != nil {
		hook(args)
	}
	return out, err
}

func (c *daemonFakeCommander) dispatch(args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}
	switch args[0] {
	case "show-options":
		return c.markersOut, c.markersErr
	case "show-option":
		if c.optionErr != nil {
			return "", c.optionErr
		}
		// args == [show-option, -sv, <name>]
		if len(args) >= 3 {
			if v, ok := c.optionByName[args[2]]; ok {
				return v, nil
			}
		}
		// Mirror production: RealCommander never surfaces a bare
		// ErrOptionNotFound through the Commander layer. tmux exits non-zero
		// with an absence-pattern stderr and runCommand wraps that into a
		// *CommandError; the discriminator inside GetServerOption is what
		// maps it to ErrOptionNotFound. Returning the bare sentinel here
		// would bypass the discriminator and let the fake diverge from
		// production for any future test that asserts on the underlying
		// *CommandError shape.
		name := ""
		if len(args) >= 3 {
			name = args[2]
		}
		return "", &tmux.CommandError{
			Stderr: "unknown option: " + name,
			Err:    errors.New("exit status 1"),
		}
	case "list-sessions":
		return c.sessionsOut, c.sessionsErr
	case "list-panes":
		return c.panesOut, c.panesErr
	case "show-environment":
		// args == [show-environment, -t, <session>]
		if len(args) >= 3 {
			if v, ok := c.envBySession[args[2]]; ok {
				return v, nil
			}
		}
		return "", nil
	case "capture-pane":
		// args == [capture-pane, -e, -p, -S, -, -t, <target>]
		var target string
		if len(args) >= 7 {
			target = args[6]
		}
		if err, ok := c.captureErrByTarget[target]; ok {
			return "", err
		}
		if v, ok := c.captureByTarget[target]; ok {
			return v, nil
		}
		return "", nil
	}
	return "", nil
}

// callsContaining returns recorded calls whose first argument equals cmd.
func (c *daemonFakeCommander) callsContaining(cmd string) [][]string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := [][]string{}
	for _, call := range c.calls {
		if len(call) > 0 && call[0] == cmd {
			out = append(out, call)
		}
	}
	for _, call := range c.rawCalls {
		if len(call) > 0 && call[0] == cmd {
			out = append(out, call)
		}
	}
	return out
}

// makeDeps assembles a daemonDeps for tick-level tests. The supplied dir is
// expected to be a t.TempDir already EnsureDir-prepared by the caller (so the
// scrollback subdirectory exists). PrevIndex defaults to nil; HashMap to a
// fresh empty map; LastSaveAt to zero (which makes gap=true on the first tick
// — most tests override it).
//
// HookStore + lastCleanup default to production-shaped values so the tick's
// idle branch (which now runs the throttled hooks-cleanup gate) is safe for
// every tick-level test: HookStore is a fresh empty store (production always
// builds one via loadHookStore) and lastCleanup is anchored to "now" (mirroring
// the daemon-start anchor), so the ~10s throttle is NOT elapsed by default and
// the gate no-ops unless a test explicitly rewinds deps.lastCleanup and seeds
// deps.HookStore.
func makeDeps(t *testing.T, dir string, fc *daemonFakeCommander) *daemonDeps {
	t.Helper()
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	store, _ := newTempHooksStore(t, "")
	return &daemonDeps{
		Dir:          dir,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Client:       tmux.NewClient(fc),
		HookStore:    store,
		lastCleanup:  time.Now(),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
	}
}

// sentinelIndex returns a fixed-shape *state.Index distinguishable by name.
// Used as a seed for deps.PrevIndex so post-call assertions can verify the
// pointer was (or was not) replaced by captureAndCommit. The exact shape is
// not load-bearing — it just needs to be obviously distinct from any fixture-
// driven capture so a stale reference is easy to spot.
func sentinelIndex(name string) *state.Index {
	return &state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{{
			Name:    name,
			Windows: []state.Window{{Index: 9, Name: "old", Panes: []state.Pane{{Index: 9, CWD: "/old"}}}},
		}},
	}
}

// assertNoCommit asserts the two invariants of the "captureAndCommit returned
// without committing" outcome:
//
//  1. deps.PrevIndex pointer is unchanged (still references sentinel).
//  2. sessions.json does not exist in stateDir.
//
// Used by the cancellation-path tests where captureAndCommit must short-circuit
// without mutating PrevIndex or writing the commit artifact.
func assertNoCommit(t *testing.T, deps *daemonDeps, sentinel *state.Index, stateDir string) {
	t.Helper()
	if deps.PrevIndex != sentinel {
		t.Errorf("PrevIndex pointer replaced; want sentinel preserved")
	}
	if _, err := os.Stat(state.SessionsJSON(stateDir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json written when no commit expected; stat err = %v", err)
	}
}

// assertCommitReplacedPrev is the peer of assertNoCommit for the happy-path
// regression guard: captureAndCommit must replace deps.PrevIndex with a fresh
// non-nil pointer (not merely mutate the sentinel's contents).
func assertCommitReplacedPrev(t *testing.T, deps *daemonDeps, sentinel *state.Index, stateDir string) {
	t.Helper()
	if deps.PrevIndex == sentinel {
		t.Errorf("PrevIndex pointer not replaced; still references sentinel")
	}
	if deps.PrevIndex == nil {
		t.Fatal("PrevIndex is nil after successful capture; expected new &idx")
	}
	_ = stateDir // reserved for future on-disk assertions; kept for signature parity with assertNoCommit
}

// touchSaveRequested creates an empty save.requested in dir.
func touchSaveRequested(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.SaveRequested(dir), nil, 0o600); err != nil {
		t.Fatalf("touch save.requested: %v", err)
	}
}

// oneSession returns canned commander outputs for a single session "work" with
// one window and one pane. Useful as a fixture for the captureAndCommit happy
// path.
func oneSession() (sessionsOut, panesOut string) {
	sessionsOut = "work|1|0|"
	// Format matches captureFormat in internal/state/capture.go. The trailing
	// empty |||-separated field is the un-stamped @portal-id column (11th
	// field) added by the fixed-arity bump; a legacy session resolves it to "".
	panesOut = "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||"
	return
}

// transportErrCommandError returns the canonical *tmux.CommandError used by
// transport-error fault-injection tests. The Stderr does NOT match the
// option-absent pattern family, so GetServerOption propagates it as a
// non-ErrOptionNotFound error through TryGetServerOption to IsRestoringSet.
func transportErrCommandError() *tmux.CommandError {
	return &tmux.CommandError{
		Stderr: "lost server",
		Err:    errors.New("exit status 1"),
	}
}

func TestDaemonTick_NoOpWhenNeitherDirtyNorGap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	deps.MaxGap = 30 * time.Second

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked when not dirty and not gap: %v", got)
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json should not be written; stat err=%v", err)
	}
}

func TestDaemonTick_FiresWhenDirty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now() // gap=false
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked when dirty")
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("sessions.json not written when dirty: %v", err)
	}
}

func TestDaemonTick_FiresAfterMaxGap(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.MaxGap = 10 * time.Millisecond
	deps.LastSaveAt = time.Now().Add(-1 * time.Hour) // very old

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked after max-gap")
	}
}

func TestDaemonTick_FiresOnFirstTickWhenLastSaveAtZero(t *testing.T) {
	// Initial LastSaveAt is the zero value; gap should be true so the first
	// eligible tick fires even without an explicit save.requested.
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	// Don't set LastSaveAt — leave it as zero.

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("first tick should fire even without dirty flag (LastSaveAt zero)")
	}
}

func TestDaemonTick_SkipsEntireTickWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
		// Seed sessions output so any leak through would trip the check.
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked during restore: %v", got)
	}
}

func TestDaemonTick_PreservesSaveRequestedWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
	}
	deps := makeDeps(t, dir, fc)
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested should survive a restore-suppressed tick; stat=%v", err)
	}
}

func TestDaemonTick_RemovesSaveRequestedAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Errorf("save.requested should be removed after successful capture; stat=%v", err)
	}
}

func TestDaemonTick_PreservesSaveRequestedOnError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	// list-sessions fails so CaptureStructure errors and tick logs+returns
	// without removing the dirty flag.
	fc := &daemonFakeCommander{sessionsErr: errors.New("tmux down")}
	// list-sessions is wrapped — ListSessions returns ([], nil) on err. To
	// force the failure path through CaptureStructure we instead force
	// list-panes to fail; ListSessionNames swallows list-sessions errors.
	fc.sessionsErr = nil
	fc.sessionsOut = "work|1|0|"
	fc.panesErr = errors.New("list-panes failed")

	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested should survive a failed cycle; stat=%v", err)
	}
}

func TestDaemonTick_PicksUpNotifyArrivingBetweenTicks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now() // gap=false; only dirty drives subsequent tick

	// Tick 1: dirty flag set, fires capture, clears flag.
	touchSaveRequested(t, dir)
	tick(t.Context(), deps)
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Fatalf("save.requested should be cleared after first tick; stat=%v", err)
	}
	firstCalls := len(fc.callsContaining("list-sessions"))

	// Notify arrives between ticks.
	touchSaveRequested(t, dir)

	// Tick 2: dirty flag set again, fires another capture.
	tick(t.Context(), deps)

	secondCalls := len(fc.callsContaining("list-sessions"))
	if secondCalls <= firstCalls {
		t.Errorf("second tick did not fire after re-touched save.requested: first=%d second=%d", firstCalls, secondCalls)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Errorf("save.requested should be cleared after second tick; stat=%v", err)
	}
}

func TestDaemonTick_SkipsSkeletonMarkedPanesInScrollback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Session "work" with two panes 0 and 1; mark pane 1 as skeleton.
	skipKey := state.SanitizePaneKey("work", 0, 1)
	markersOut := fmt.Sprintf(`%s%s "1"`, state.SkeletonMarkerPrefix, skipKey)

	fc := &daemonFakeCommander{
		markersOut:  markersOut,
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||",
		captureByTarget: map[string]string{
			"work:0.0": "captured-pane-0",
			"work:0.1": "should-not-be-captured",
		},
	}
	// Seed PrevIndex with the skeleton-marked pane so the merge keeps it
	// in the index.
	prevPane := state.Pane{Index: 1, CWD: "/prev", ScrollbackFile: "scrollback/" + skipKey + ".bin"}
	prev := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{{
		Name:    "work",
		Windows: []state.Window{{Index: 0, Name: "main", Layout: "layout", Panes: []state.Pane{prevPane}}},
	}}}
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = &prev
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	for _, call := range fc.callsContaining("capture-pane") {
		// args[6] is the -t target.
		if len(call) >= 7 && call[6] == "work:0.1" {
			t.Errorf("capture-pane invoked for skeleton-marked target work:0.1: %v", call)
		}
	}
}

func TestDaemonTick_ContinuesOnPerPaneCaptureError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||",
		captureErrByTarget: map[string]error{
			"work:0.0": errors.New("flaky pane"),
		},
		captureByTarget: map[string]string{
			"work:0.1": "ok-bytes",
		},
	}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	// commit must still happen.
	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("sessions.json must commit despite per-pane error: %v", err)
	}
	// scrollback for the surviving pane was written.
	survivingKey := state.SanitizePaneKey("work", 0, 1)
	if _, err := os.Stat(state.ScrollbackFile(dir, survivingKey)); err != nil {
		t.Errorf("surviving pane scrollback not written: %v", err)
	}
}

func TestDaemonTick_LogsAndSkipsOnShowOptionsError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{markersErr: errors.New("show-options blew up")}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	got := sink.Body()
	if !strings.Contains(got, "tick failed") {
		t.Errorf("expected tick failure log entry; got:\n%s", got)
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json should not be written on list-markers error; stat=%v", err)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested should survive list-markers failure: %v", err)
	}
}

func TestDaemonTick_LogsAndSkipsOnCaptureStructureError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesErr:    errors.New("list-panes blew up"),
	}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json should not be written on capture-structure error; stat=%v", err)
	}
}

func TestDaemonTick_LogsAndSkipsOnCommitErrorWithoutAdvancingLastSaveAt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	originalLastSave := time.Now().Add(-1 * time.Hour)
	deps.LastSaveAt = originalLastSave

	// Force commit failure: make sessions.json a directory so AtomicWrite
	// cannot rename onto it.
	if err := os.MkdirAll(state.SessionsJSON(dir), 0o700); err != nil {
		t.Fatalf("create blocking dir: %v", err)
	}
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if !deps.LastSaveAt.Equal(originalLastSave) {
		t.Errorf("LastSaveAt advanced despite commit failure: %v != %v", deps.LastSaveAt, originalLastSave)
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Errorf("save.requested should survive commit failure: %v", err)
	}
}

// TestDaemonTick_RunsHookCleanupOnIdleTick pins the load-bearing placement of
// the daemon-owned hooks stale-cleanup gate (spec § Daemon-Owned Hooks Cleanup
// → Placement in the tick): on an idle tick (!dirty && !gap, @portal-restoring
// unset) with the throttle elapsed, maybeRunHookCleanup runs and reaps the stale
// entry — and it does so on the idle fast path, so NO capture cycle (list-sessions)
// runs that tick.
func TestDaemonTick_RunsHookCleanupOnIdleTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"},
  "live:0.0": {"on-resume": "cmd-live"}
}`
	store, _ := newTempHooksStore(t, seed)

	// panesOut carries the single live pane key; the stale key is absent so the
	// cleanup reaps it. No sessionsOut — capture must not run on an idle tick.
	fc := &daemonFakeCommander{panesOut: "live:0.0"}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()                                          // gap=false
	deps.MaxGap = 30 * time.Second                                        // !gap
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // throttle elapsed
	// No save.requested → !dirty. @portal-restoring unset (default).

	tick(t.Context(), deps)

	// Cleanup ran on the idle branch: the stale entry is reaped, live survives.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; ok {
		t.Errorf("stale hook entry not reaped on idle tick; hooks=%v", keysOf(postRun))
	}
	if _, ok := postRun["live:0.0"]; !ok {
		t.Errorf("live hook entry wrongly reaped on idle tick; hooks=%v", keysOf(postRun))
	}

	// Still the idle fast path — no capture cycle ran.
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked on an idle tick (capture must not run): %v", got)
	}
}

// TestDaemonTick_SkipsHookCleanupWhenRestoring pins that a @portal-restoring-set
// tick short-circuits the whole tick BEFORE the idle branch, so cleanup never
// runs during a restore window even with the throttle elapsed (spec § Placement
// in the tick — skipped entirely while @portal-restoring is set).
func TestDaemonTick_SkipsHookCleanupWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)

	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
		panesOut:     "live:0.0",
	}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // throttle elapsed

	tick(t.Context(), deps)

	// @portal-restoring returns before the idle branch — the stale entry survives.
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; !ok {
		t.Errorf("stale hook entry reaped during restore window; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}

	// The cleanup's ListAllPanes (list-panes) must never have been invoked.
	if got := fc.callsContaining("list-panes"); len(got) != 0 {
		t.Errorf("list-panes (cleanup) invoked during restore window: %v", got)
	}
}

// TestDaemonTick_SkipsHookCleanupOnDirtyCaptureTick pins that a capture-pending
// (dirty) tick takes the captureAndCommit branch and never reaches the idle
// branch, so cleanup is skipped that tick — scrollback always wins (spec §
// Placement in the tick — dirty||gap → capture runs, cleanup skipped).
func TestDaemonTick_SkipsHookCleanupOnDirtyCaptureTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()                                          // gap=false
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // throttle elapsed
	touchSaveRequested(t, dir)                                            // dirty=true

	tick(t.Context(), deps)

	// dirty → capture branch taken; the idle branch (cleanup) is never reached.
	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; capture must run on a dirty tick")
	}
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; !ok {
		t.Errorf("stale hook entry reaped on a capture-pending tick; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}
}

// TestDaemonTick_SkipsHookCleanupOnMaxGapCaptureTick is the gap-driven peer of
// the dirty-tick guard: a max-gap capture tick (!dirty but gap=true) runs
// captureAndCommit and never reaches the idle branch, so cleanup is skipped that
// tick (spec § Placement in the tick — dirty||gap → capture runs).
func TestDaemonTick_SkipsHookCleanupOnMaxGapCaptureTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := `{
  "stale:0.0": {"on-resume": "cmd-stale"}
}`
	store, _ := newTempHooksStore(t, seed)

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.MaxGap = 10 * time.Millisecond
	deps.LastSaveAt = time.Now().Add(-1 * time.Hour)                      // gap=true
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second) // throttle elapsed
	// No save.requested → !dirty; gap=true drives the capture branch.

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; capture must run on a max-gap tick")
	}
	postRun, err := store.Load()
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun["stale:0.0"]; !ok {
		t.Errorf("stale hook entry reaped on a max-gap capture tick; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}
}

func TestDaemonShutdownFlush_FlushesOnContextCancelWhenNotRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = time.Hour // ensure no ticker firing during test

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	// Final flush should have committed sessions.json and called list-sessions.
	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("final flush did not invoke list-sessions")
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("final flush did not write sessions.json: %v", err)
	}
	if got := sink.Body(); !strings.Contains(got, "shutdown") || !strings.Contains(got, "flush_completed=true") {
		t.Errorf("expected a 'shutdown' INFO with flush_completed=true; got:\n%s", got)
	}
}

func TestDaemonShutdownFlush_SkipsWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
		sessionsOut:  "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("final flush ran list-sessions despite restoring marker: %v", got)
	}
	if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json should not be written when restoring; stat=%v", err)
	}
}

// TestDefaultShutdownFlush_SkipsOnTransportError covers the conservative-on-
// error branch in defaultShutdownFlush: when IsRestoringSet errors (e.g.,
// transport failure during the restoration window), the final flush is skipped
// and nil is returned. The fault-injection shape is a *tmux.CommandError whose
// Stderr does NOT match the option-absent pattern family, so GetServerOption
// propagates it as a non-ErrOptionNotFound error through TryGetServerOption to
// IsRestoringSet.
func TestDefaultShutdownFlush_SkipsOnTransportError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionErr: transportErrCommandError(),
		// Seed sessions output so any leak through to captureAndCommit would
		// surface as a list-sessions call we can assert against.
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)

	t.Run("returns_nil", func(t *testing.T) {
		if err := defaultShutdownFlush(deps); err != nil {
			t.Errorf("defaultShutdownFlush() = %v, want nil", err)
		}
	})

	t.Run("zero_commits", func(t *testing.T) {
		// captureAndCommit's first tmux call is list-sessions (via
		// CaptureStructure). Zero list-sessions invocations is the structural
		// proof that no commit cycle ran.
		if got := fc.callsContaining("list-sessions"); len(got) != 0 {
			t.Errorf("list-sessions invoked despite transport error on @portal-restoring read: %v", got)
		}
		if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json should not be written on transport error; stat=%v", err)
		}
	})
}

// TestTick_SkipsOnTransportError covers the conservative-on-error branch in
// tick() (cmd/state_daemon.go:95-99). Same fault-injection shape as the
// shutdown-flush test: when IsRestoringSet errors via a non-absent
// *tmux.CommandError, tick logs at WARN and returns without performing capture
// or commit.
func TestTick_SkipsOnTransportError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionErr:   transportErrCommandError(),
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	t.Run("no_capture", func(t *testing.T) {
		// capture-pane is invoked inside captureAndCommit, only reached if the
		// restoring-marker check did not abort the tick.
		if got := fc.callsContaining("capture-pane"); len(got) != 0 {
			t.Errorf("capture-pane invoked despite transport error on @portal-restoring read: %v", got)
		}
		// list-sessions is the first tmux call in captureAndCommit; zero
		// invocations is independent structural proof.
		if got := fc.callsContaining("list-sessions"); len(got) != 0 {
			t.Errorf("list-sessions invoked despite transport error on @portal-restoring read: %v", got)
		}
	})

	t.Run("no_commit", func(t *testing.T) {
		if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json should not be written on transport error; stat=%v", err)
		}
		// save.requested must survive a skipped tick — the dirty flag is only
		// cleared after a successful capture-and-commit.
		if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
			t.Errorf("save.requested should survive transport-error skip: %v", err)
		}
	})
}

func TestDaemonStartup_SeedsHashMapFromDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Pre-seed scrollback files before invoking the daemon RunE.
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	preseed := map[string][]byte{
		"work__0.0":  []byte("alpha"),
		"side__1.2":  []byte("beta"),
		"third__0.0": []byte("gamma"),
	}
	for k, v := range preseed {
		if err := os.WriteFile(state.ScrollbackFile(dir, k), v, 0o600); err != nil {
			t.Fatalf("seed scrollback %s: %v", k, err)
		}
	}

	// Capture deps via the run-func seam so we can inspect HashMap.
	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("runStateDaemon: %v", err)
	}

	if *holder == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	hm := (*holder).HashMap
	for k := range preseed {
		if _, ok := hm[k]; !ok {
			t.Errorf("HashMap missing pre-seeded entry for %q", k)
		}
	}
}

func TestDaemonStartup_LoadsPrevIndexFromSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Pre-seed sessions.json with a known structure.
	want := state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{{
			Name:        "work",
			Environment: map[string]string{"FOO": "bar"},
			Windows:     []state.Window{{Index: 0, Name: "main", Panes: []state.Pane{{Index: 0, CWD: "/tmp"}}}},
		}},
	}
	data, err := state.EncodeIndex(want)
	if err != nil {
		t.Fatalf("EncodeIndex: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), data, 0o600); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("runStateDaemon: %v", err)
	}
	if *holder == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	pi := (*holder).PrevIndex
	if pi == nil {
		t.Fatal("PrevIndex is nil; expected loaded index")
	}
	if len(pi.Sessions) != 1 || pi.Sessions[0].Name != "work" {
		t.Errorf("PrevIndex sessions = %+v; want one session named 'work'", pi.Sessions)
	}
}

func TestDaemonStartup_HandlesMissingSessionsJSONAsNilPrev(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// The daemon RunE logs via daemonLogger (log.For("daemon")); capture those
	// records in-process so we can assert no ReadIndex warning was emitted.
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("runStateDaemon: %v", err)
	}
	if *holder == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	if (*holder).PrevIndex != nil {
		t.Errorf("PrevIndex = %+v; want nil for missing sessions.json", (*holder).PrevIndex)
	}
	// Missing-file is a clean skip: no warning about reading sessions.json
	// should be logged. This is the corrupt-vs-missing classification we
	// inherit from state.ReadIndex.
	if data := sink.Body(); strings.Contains(data, "ReadIndex") || strings.Contains(data, "sessions.json") {
		t.Errorf("missing sessions.json should not produce a ReadIndex warning; got:\n%s", data)
	}
}

func TestDaemonStartup_LogsWarningOnUndecodableSessionsJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	if err := os.WriteFile(state.SessionsJSON(dir), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed bad sessions.json: %v", err)
	}

	// The daemon RunE logs via daemonLogger (log.For("daemon")); capture those
	// records in-process.
	sink := &logtest.Sink{}
	log.SetTestHandler(t, sink)

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runStateDaemon(t); err != nil {
		t.Fatalf("runStateDaemon: %v", err)
	}
	if (*holder).PrevIndex != nil {
		t.Errorf("PrevIndex should be nil on decode error; got %+v", *(*holder).PrevIndex)
	}
	// state.ReadIndex wraps decode failures with ErrCorruptIndex, whose
	// message is "sessions.json corrupt" — it rides the error attr so the
	// daemon distinguishes corrupt-content from a missing file or other
	// read errors.
	if logged := sink.Body(); !strings.Contains(logged, "sessions.json corrupt") {
		t.Errorf("expected corrupt-index warning in log; got:\n%s", logged)
	}
}

// TestCaptureAndCommit_UncancelledCtxMatchesPreThreadingBehaviour is the
// happy-path regression guard for the ctx-threading plumbing step (spec
// § Change 2 / Phase 2 Task 2-1). It drives a multi-pane fixture through
// captureAndCommit with a never-cancelled context.Background() and asserts the
// pre-threading semantics still hold:
//
//  1. PrevIndex is replaced with the freshly captured index (the input
//     pointer is overwritten with the post-capture &idx).
//  2. state.Commit is invoked exactly once — observable as a single
//     sessions.json file on disk containing the captured sessions.
//  3. All panes are processed — every (sess, win, pane) tuple in the
//     fixture surfaces a scrollback file and a capture-pane call.
//
// Subsequent tasks (2-2/2-3/2-4) will introduce mid-iteration ctx.Done()
// observation points; this test pins the uncancelled-ctx behaviour so those
// future changes cannot silently regress the happy path.
func TestCaptureAndCommit_UncancelledCtxMatchesPreThreadingBehaviour(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Two sessions, two windows total, three panes — exercises the inner
	// loop's pane-iteration across the (sess, win, pane) nesting.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|\nside|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||\n" +
			"side|||0|||main|||layout|||0|||1|||0|||/var|||1|||zsh|||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"side:0.0": "side-pane-0-bytes",
		},
	}

	// Seed PrevIndex with a sentinel value distinct from anything the fixture
	// will produce, so we can prove the pointer was replaced (not merely
	// mutated to equivalent contents).
	sentinelPrev := sentinelIndex("sentinel-must-be-replaced")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit returned error on happy path: %v", err)
	}

	// (1) PrevIndex pointer replacement.
	assertCommitReplacedPrev(t, deps, sentinelPrev, dir)
	if len(deps.PrevIndex.Sessions) != 2 {
		t.Errorf("PrevIndex.Sessions length = %d; want 2", len(deps.PrevIndex.Sessions))
	}
	for _, sess := range deps.PrevIndex.Sessions {
		if sess.Name == "sentinel-must-be-replaced" {
			t.Errorf("PrevIndex still contains sentinel session; want fresh capture only")
		}
	}

	// (2) state.Commit invoked exactly once — sessions.json exists with the
	// captured sessions decoded back.
	sessionsJSONPath := state.SessionsJSON(dir)
	data, err := os.ReadFile(sessionsJSONPath)
	if err != nil {
		t.Fatalf("sessions.json not written by commit: %v", err)
	}
	committed, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("decode committed sessions.json: %v", err)
	}
	if len(committed.Sessions) != 2 {
		t.Errorf("committed sessions length = %d; want 2", len(committed.Sessions))
	}

	// (3) All panes processed — three capture-pane calls (one per pane) and
	// three scrollback files on disk.
	captureCalls := fc.callsContaining("capture-pane")
	if len(captureCalls) != 3 {
		t.Errorf("capture-pane call count = %d; want 3 (one per pane): %v", len(captureCalls), captureCalls)
	}
	for _, key := range []string{
		state.SanitizePaneKey("work", 0, 0),
		state.SanitizePaneKey("work", 0, 1),
		state.SanitizePaneKey("side", 0, 0),
	} {
		if _, err := os.Stat(state.ScrollbackFile(dir, key)); err != nil {
			t.Errorf("scrollback file missing for pane %q: %v", key, err)
		}
	}
}

// TestCaptureAndCommit_PreCancelledCtxReturnsImmediately covers observation
// point 1 of 3 (spec § Change 2 / Phase 2 Task 2-2): the ctx.Done() check at
// captureAndCommit's entry, before any tmux enumeration work. A context that
// is already cancelled when captureAndCommit is invoked must:
//
//  1. Return nil (not an error — tick logs WARN on non-nil; cancellation must
//     not log).
//  2. Not invoke ListSkeletonMarkers (show-options), CaptureStructure
//     (list-sessions / list-panes), or any capture-pane call.
//  3. Leave deps.PrevIndex unchanged (no pointer replacement).
//  4. Produce no commit — sessions.json must not be written.
func TestCaptureAndCommit_PreCancelledCtxReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Seed a multi-pane fixture so that, if the cancellation check were
	// missing, the function would do observable work (enumerate sessions,
	// capture panes, commit) — the assertions below would catch the leak.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut:    "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
		},
	}

	// Seed PrevIndex with a sentinel — assertion target for "unchanged".
	sentinelPrev := sentinelIndex("sentinel-must-be-preserved")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on pre-cancelled ctx: %v; want nil", err)
	}

	// No tmux work: ListSkeletonMarkers (show-options), CaptureStructure
	// (list-sessions / list-panes), capture-pane.
	if got := fc.callsContaining("show-options"); len(got) != 0 {
		t.Errorf("show-options invoked on pre-cancelled ctx (ListSkeletonMarkers leaked): %v", got)
	}
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked on pre-cancelled ctx (CaptureStructure leaked): %v", got)
	}
	if got := fc.callsContaining("list-panes"); len(got) != 0 {
		t.Errorf("list-panes invoked on pre-cancelled ctx (CaptureStructure leaked): %v", got)
	}
	if got := fc.callsContaining("capture-pane"); len(got) != 0 {
		t.Errorf("capture-pane invoked on pre-cancelled ctx: %v", got)
	}

	// PrevIndex pointer unchanged + no commit on disk.
	assertNoCommit(t, deps, sentinelPrev, dir)
}

// TestCaptureAndCommit_CancelDuringCaptureStructureReturnsBeforePerPaneWork
// covers observation point 2 of 3 (spec § Change 2 / Phase 2 Task 2-3): the
// ctx.Done() check immediately after CaptureStructure returns and before the
// per-pane loop begins. The test triggers cancel() from inside the commander's
// dispatch hook while CaptureStructure is still running (specifically on its
// final tmux subcall, show-environment) — so CaptureStructure completes
// successfully with a populated multi-pane index, but the ctx is observed
// cancelled at the post-enumeration check before any per-pane work runs.
//
// Pins the timing-specific behaviour: cancellation observed AFTER
// CaptureStructure, not at entry. Distinguishes observation point 2 from
// observation point 1 — point 1 would catch a pre-call cancel; point 2 is
// load-bearing for cancellations that arrive during CaptureStructure's
// subprocess calls.
//
// Asserts:
//  1. Returns nil (not an error — tick logs WARN on non-nil).
//  2. CaptureStructure ran to completion (list-sessions / list-panes /
//     show-environment all observed).
//  3. No per-pane work began: zero capture-pane calls.
//  4. No commit landed on disk: sessions.json absent.
//  5. deps.PrevIndex pointer unchanged from its pre-call value (no
//     post-loop replacement).
func TestCaptureAndCommit_CancelDuringCaptureStructureReturnsBeforePerPaneWork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Multi-pane fixture — if the post-enumeration check were missing, the
	// per-pane loop would iterate three panes and the assertions below would
	// catch the leak (capture-pane calls / scrollback files / committed
	// sessions.json).
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|\nside|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||\n" +
			"side|||0|||main|||layout|||0|||1|||0|||/var|||1|||zsh|||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"side:0.0": "side-pane-0-bytes",
		},
	}

	// Seed PrevIndex with a sentinel — assertion target for "unchanged".
	sentinelPrev := sentinelIndex("sentinel-must-be-preserved")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Wire the dispatch hook: cancel() fires when CaptureStructure makes its
	// final subcall (show-environment), which runs once per kept session
	// after list-sessions / list-panes. By the time CaptureStructure
	// returns, ctx is cancelled — but the function itself completes
	// successfully with the freshly populated multi-pane index.
	//
	// The ctx.Done() check at observation point 2 then fires before the
	// per-pane loop begins.
	fc.dispatchHook = func(args []string) {
		if len(args) > 0 && args[0] == "show-environment" {
			cancel()
		}
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on mid-CaptureStructure cancel: %v; want nil", err)
	}

	// CaptureStructure ran to completion — all three of its tmux subcalls
	// landed before observation point 2 fired.
	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; CaptureStructure did not start")
	}
	if got := fc.callsContaining("list-panes"); len(got) == 0 {
		t.Errorf("list-panes not invoked; CaptureStructure did not enumerate panes")
	}
	if got := fc.callsContaining("show-environment"); len(got) == 0 {
		t.Errorf("show-environment not invoked; CaptureStructure did not reach its env phase")
	}

	// No per-pane work began.
	if got := fc.callsContaining("capture-pane"); len(got) != 0 {
		t.Errorf("capture-pane invoked after post-enumeration cancel: %v", got)
	}
	// No scrollback files written.
	for _, key := range []string{
		state.SanitizePaneKey("work", 0, 0),
		state.SanitizePaneKey("work", 0, 1),
		state.SanitizePaneKey("side", 0, 0),
	} {
		if _, err := os.Stat(state.ScrollbackFile(dir, key)); !os.IsNotExist(err) {
			t.Errorf("scrollback file unexpectedly written for %q on cancel: stat err = %v", key, err)
		}
	}

	// PrevIndex pointer unchanged + no commit on disk.
	assertNoCommit(t, deps, sentinelPrev, dir)
}

// TestCaptureAndCommit_CancelMidLoopAfterKofNPanesProcessed covers observation
// point 3 of 3 (spec § Change 2 / Phase 2 Task 2-4): the ctx.Done() check at
// the top of the innermost per-pane loop body. The test drives a multi-pane
// fixture (1 session × 1 window × 3 panes) and cancels after the first pane's
// capture-pane subcall fires — so observation point 3 fires before iteration
// k+1 begins.
//
// Per-pane scrollback files from completed iterations may remain on disk
// (atomic writes — no rollback). The spec's no-partial-commit invariant is
// about sessions.json, not per-pane files (see spec § Change 2 Cancellation
// semantics).
//
// Asserts:
//  1. Returns nil (not an error — tick logs WARN on non-nil).
//  2. capture-pane invoked at least once (the iteration that triggered the
//     cancel) but fewer than 3 times (subsequent iterations short-circuit).
//  3. sessions.json absent — Commit was not invoked.
//  4. deps.PrevIndex pointer unchanged from its pre-call value (no
//     post-loop replacement).
func TestCaptureAndCommit_CancelMidLoopAfterKofNPanesProcessed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Single session, single window, three panes — exercises the innermost
	// pane-iteration loop.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||\n" +
			"work|||0|||main|||layout|||0|||1|||2|||/tmp|||0|||fish|||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"work:0.2": "work-pane-2-bytes",
		},
	}

	sentinelPrev := sentinelIndex("sentinel-must-be-preserved")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Wire the dispatch hook: cancel() fires after the first pane's
	// capture-pane subcall. The current iteration completes its write; the
	// next iteration's observation-point-3 check observes ctx.Done() and
	// returns nil before invoking CaptureAndHashPane on pane 2.
	fc.dispatchHook = func(args []string) {
		if len(args) > 0 && args[0] == "capture-pane" {
			cancel()
		}
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on mid-loop cancel: %v; want nil", err)
	}

	// capture-pane invoked at least once but fewer than 3 times.
	captureCalls := fc.callsContaining("capture-pane")
	if len(captureCalls) < 1 {
		t.Errorf("capture-pane invoked %d times; want at least 1", len(captureCalls))
	}
	if len(captureCalls) >= 3 {
		t.Errorf("capture-pane invoked %d times; want fewer than 3 (mid-loop cancel should short-circuit): %v", len(captureCalls), captureCalls)
	}

	// PrevIndex pointer unchanged + no commit on disk. Per-pane scrollback
	// files from completed iterations MAY remain on disk; that is intentional
	// (atomic writes, no rollback), and the spec's no-partial-commit
	// invariant is about sessions.json, not per-pane files.
	assertNoCommit(t, deps, sentinelPrev, dir)
}

// TestCaptureAndCommit_UncancelledMultiPaneFixtureProcessesAllPanesAndCommits
// is the regression guard for observation point 3 (spec § Change 2 / Phase 2
// Task 2-4): the same multi-pane fixture as the mid-loop-cancel test, run
// without any cancellation, must process every pane and commit fully. Pins
// that observation point 3's `default:` arm does not silently short-circuit
// the happy path.
//
// Asserts:
//  1. Returns nil.
//  2. capture-pane invoked exactly 3 times (one per pane).
//  3. sessions.json exists and decodes to the captured index.
//  4. deps.PrevIndex pointer replaced (no longer the sentinel).
func TestCaptureAndCommit_UncancelledMultiPaneFixtureProcessesAllPanesAndCommits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh|||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash|||\n" +
			"work|||0|||main|||layout|||0|||1|||2|||/tmp|||0|||fish|||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"work:0.2": "work-pane-2-bytes",
		},
	}

	sentinelPrev := sentinelIndex("sentinel-must-be-replaced")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit returned error on uncancelled multi-pane fixture: %v", err)
	}

	captureCalls := fc.callsContaining("capture-pane")
	if len(captureCalls) != 3 {
		t.Errorf("capture-pane call count = %d; want 3 (one per pane): %v", len(captureCalls), captureCalls)
	}

	// sessions.json exists and decodes back to the captured index.
	data, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("sessions.json not written by commit: %v", err)
	}
	committed, err := state.DecodeIndex(data)
	if err != nil {
		t.Fatalf("decode committed sessions.json: %v", err)
	}
	if len(committed.Sessions) != 1 {
		t.Errorf("committed sessions length = %d; want 1", len(committed.Sessions))
	}

	// PrevIndex pointer replaced.
	assertCommitReplacedPrev(t, deps, sentinelPrev, dir)
}

// TestDefaultDaemonRun_WritesVersionFileFromDepsVersion is the regression guard
// for the WriteVersionFile move (RunE → defaultDaemonRun). It invokes
// defaultDaemonRun directly (bypassing RunE entirely) with a daemonDeps whose
// Version field is set to a sentinel value, and asserts that daemon.version
// lands on disk with the sentinel content.
//
// Pins the contract: defaultDaemonRun is the sole production write site for
// daemon.version, sourcing the value from deps.Version. A regression that
// moves WriteVersionFile back into RunE would fail this test because RunE is
// not on the call path here.
func TestDefaultDaemonRun_WritesVersionFileFromDepsVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	// Short-circuit the tick loop so defaultDaemonRun returns immediately
	// after its startup write sequence; pre-cancel the ctx as a belt-and-
	// braces in case the seam swap is bypassed.
	prevLoop := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error { return nil }
	t.Cleanup(func() { daemonTickLoopFunc = prevLoop })

	prevLock := daemonLockFile
	daemonLockFile = nil
	t.Cleanup(func() { daemonLockFile = prevLock })

	const want = "regression-sentinel-1.2.3"
	deps := &daemonDeps{
		Dir:          dir,
		Version:      want,
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		TickerPeriod: time.Hour,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	got, err := state.ReadVersionFile(dir)
	if err != nil {
		t.Fatalf("ReadVersionFile after defaultDaemonRun: %v", err)
	}
	if got != want {
		t.Errorf("daemon.version = %q; want %q (WriteVersionFile must run from defaultDaemonRun using deps.Version)", got, want)
	}
}
