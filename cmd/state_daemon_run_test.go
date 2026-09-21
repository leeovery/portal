package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commandertest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// daemonFakeCommander is kept bespoke rather than retired onto
// commandertest.Scripted: it is not an argv-pattern script but a model of the
// daemon's whole tmux surface, and it carries one behaviour a static script
// cannot express — dispatchHook, a callback that fires after every dispatch,
// matched or not, so a caller can cancel the context while a tmux subcall is
// in flight. The mutex covers the concurrent drive from the goroutines
// running defaultDaemonRun. Its Run/RunRaw still route through
// commandertest.Trim/Verbatim, so the trim-versus-verbatim contract keeps its
// single implementation.
//
// Unset commands return ("", nil), so unrelated tmux calls do not fail a test.
type daemonFakeCommander struct {
	mu sync.Mutex

	markersOut string
	markersErr error

	optionByName map[string]string
	optionErr    error

	sessionsOut string
	sessionsErr error

	panesOut string
	panesErr error

	envBySession map[string]string

	captureByTarget    map[string]string
	captureErrByTarget map[string]error

	// Invoked after every dispatch resolution, so a cancellation test can fire
	// cancel() while a tmux subcall is in flight.
	dispatchHook func(args []string)

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
	return commandertest.Trim(out, err)
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
	return commandertest.Verbatim(out, err)
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
		if len(args) >= 3 {
			if v, ok := c.optionByName[args[2]]; ok {
				return v, nil
			}
		}
		// An absence must surface as a *CommandError, never a bare
		// ErrOptionNotFound: production reaches that sentinel only through
		// GetServerOption's stderr discriminator, which a bare return bypasses.
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
		if len(args) >= 3 {
			if v, ok := c.envBySession[sessionFromExactTarget(args[2])]; ok {
				return v, nil
			}
		}
		return "", nil
	case "capture-pane":
		var target string
		if len(args) >= 7 {
			// Keyed on the plain coordinate, the way real tmux resolves the
			// client's exact-match target.
			target = sessionFromExactTarget(args[6])
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

// LastSaveAt is left zero, which makes gap=true on the first tick. lastCleanup
// is anchored to now so the idle branch's throttled cleanup gate no-ops unless a
// test rewinds it explicitly.
func makeDeps(t *testing.T, dir string, fc *daemonFakeCommander) *daemonDeps {
	t.Helper()
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: ""})
	return &daemonDeps{
		Dir:          dir,
		Logger:       log.Discard(),
		Client:       tmux.NewClient(fc),
		HookStore:    store,
		lastCleanup:  time.Now(),
		HashMap:      state.HashMap{},
		TickerPeriod: 1 * time.Millisecond,
		MaxGap:       30 * time.Second,
	}
}

func sentinelIndex(name string) *state.Index {
	return &state.Index{
		Version: state.SchemaVersion,
		Sessions: []state.Session{{
			Name:    name,
			Windows: []state.Window{{Index: 9, Name: "old", Panes: []state.Pane{{Index: 9, CWD: "/old"}}}},
		}},
	}
}

func assertNoCommit(t *testing.T, deps *daemonDeps, sentinel *state.Index, stateDir string) {
	t.Helper()
	if deps.PrevIndex != sentinel {
		t.Errorf("PrevIndex pointer replaced; want sentinel preserved")
	}
	if _, err := os.Stat(state.SessionsJSON(stateDir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json written when no commit expected; stat err = %v", err)
	}
}

func assertCommitReplacedPrev(t *testing.T, deps *daemonDeps, sentinel *state.Index, stateDir string) {
	t.Helper()
	if deps.PrevIndex == sentinel {
		t.Errorf("PrevIndex pointer not replaced; still references sentinel")
	}
	if deps.PrevIndex == nil {
		t.Fatal("PrevIndex is nil after successful capture; expected new &idx")
	}
	_ = stateDir
}

func touchSaveRequested(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(state.SaveRequested(dir), nil, 0o600); err != nil {
		t.Fatalf("touch save.requested: %v", err)
	}
}

func oneSession() (sessionsOut, panesOut string) {
	sessionsOut = "work|1|0|"
	// Fields match captureFormat; the two trailing empty ones are the
	// pane-token and resume-pending columns, which an un-stamped, unmarked
	// pane reads as "".
	panesOut = "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||"
	return
}

// The Stderr deliberately does not match the option-absent pattern family, so
// GetServerOption propagates it rather than mapping it to ErrOptionNotFound.
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
	deps.LastSaveAt = time.Now()
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
	deps.LastSaveAt = time.Now().Add(-1 * time.Hour)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked after max-gap")
	}
}

func TestDaemonTick_FiresOnFirstTickWhenLastSaveAtZero(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)

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
		// Seeded so a leak through the restore guard trips the assertion.
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked during restore: %v", got)
	}
}

func TestDaemonTick_SkipsEntireTickAndWarnsOnRestoringReadError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionErr: transportErrCommandError(),
		// Seeded so a leak through the restore guard trips the assertion.
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked despite a failed restoring read: %v", got)
	}
	if n := countLines(sink, "WARN", "read @portal-restoring failed"); n != 1 {
		t.Errorf("expected the restoring-read-error WARN; got %d in:\n%s", n, sink.Body())
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
	// The seeded list-sessions error is deliberately cleared: ListSessionNames
	// swallows it, so list-panes is the only way to fail CaptureStructure.
	fc := &daemonFakeCommander{sessionsErr: errors.New("tmux down")}
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
	deps.LastSaveAt = time.Now()

	touchSaveRequested(t, dir)
	tick(t.Context(), deps)
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Fatalf("save.requested should be cleared after first tick; stat=%v", err)
	}
	firstCalls := len(fc.callsContaining("list-sessions"))

	touchSaveRequested(t, dir)

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

	skipKey := state.SanitizePaneKey("work", 0, 1)
	markersOut := fmt.Sprintf(`%s%s "1"`, state.SkeletonMarkerPrefix, skipKey)

	fc := &daemonFakeCommander{
		markersOut:  markersOut,
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||",
		captureByTarget: map[string]string{
			"work:0.0": "captured-pane-0",
			"work:0.1": "should-not-be-captured",
		},
	}
	// The merge only keeps the skeleton-marked pane if PrevIndex carries it.
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
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||",
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

	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("sessions.json must commit despite per-pane error: %v", err)
	}
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

	// A directory at sessions.json forces commit failure: AtomicWrite cannot
	// rename onto it.
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

func TestDaemonTick_RunsHookCleanupOnIdleTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

	// The stale key is absent from panesOut so the cleanup reaps it; no
	// sessionsOut, because capture must not run on an idle tick.
	fc := &daemonFakeCommander{panesOut: livePaneRowOut}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()
	deps.MaxGap = 30 * time.Second
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second)

	assertHookKeysStaged(t, store, hookstest.ReapableSeedA, hookstest.LiveSeedA)

	tick(t.Context(), deps)

	postRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun[hookstest.ReapableSeedA]; ok {
		t.Errorf("stale hook entry not reaped on idle tick; hooks=%v", keysOf(postRun))
	}
	if _, ok := postRun[hookstest.LiveSeedA]; !ok {
		t.Errorf("live hook entry wrongly reaped on idle tick; hooks=%v", keysOf(postRun))
	}

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked on an idle tick (capture must not run): %v", got)
	}
}

func TestDaemonTick_SkipsHookCleanupWhenRestoring(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-gone"}
}`, hookstest.ReapableSeedA)
	store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
		panesOut:     livePaneRowOut,
	}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second)

	tick(t.Context(), deps)

	postRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun[hookstest.ReapableSeedA]; !ok {
		t.Errorf("stale hook entry reaped during restore window; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}

	if got := fc.callsContaining("list-panes"); len(got) != 0 {
		t.Errorf("list-panes (cleanup) invoked during restore window: %v", got)
	}
}

func TestDaemonTick_SkipsHookCleanupOnDirtyCaptureTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-gone"}
}`, hookstest.ReapableSeedA)
	store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.LastSaveAt = time.Now()
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second)
	touchSaveRequested(t, dir)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; capture must run on a dirty tick")
	}
	postRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun[hookstest.ReapableSeedA]; !ok {
		t.Errorf("stale hook entry reaped on a capture-pending tick; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}
}

func TestDaemonTick_SkipsHookCleanupOnMaxGapCaptureTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-gone"}
}`, hookstest.ReapableSeedA)
	store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.HookStore = store
	deps.MaxGap = 10 * time.Millisecond
	deps.LastSaveAt = time.Now().Add(-1 * time.Hour)
	deps.lastCleanup = time.Now().Add(-hookCleanupInterval - time.Second)

	tick(t.Context(), deps)

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; capture must run on a max-gap tick")
	}
	postRun, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if _, ok := postRun[hookstest.ReapableSeedA]; !ok {
		t.Errorf("stale hook entry reaped on a max-gap capture tick; cleanup must be skipped; hooks=%v", keysOf(postRun))
	}
}

func TestDaemonTick_RunsProjectCleanupOnIdleTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	live := t.TempDir()
	gone := filepath.Join(t.TempDir(), "gone-dir-does-not-exist")
	store, _ := seedProjectsJSON(t, live, gone)

	// No sessionsOut: capture must not run on an idle tick. makeDeps anchors
	// lastCleanup to now, leaving the hooks gate throttled so this test isolates
	// the project prune.
	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.ProjectStore = store
	deps.LastSaveAt = time.Now()
	deps.MaxGap = 30 * time.Second
	deps.lastProjectCleanup = time.Now().Add(-projectCleanupInterval - time.Second)

	tick(t.Context(), deps)

	if paths := projectPaths(t, store); len(paths) != 1 || paths[0] != live {
		t.Errorf("project prune did not run on the idle branch; paths=%v, want [%s]", paths, live)
	}

	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("list-sessions invoked on an idle tick (capture must not run): %v", got)
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
	deps.TickerPeriod = time.Hour

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

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

func TestDefaultShutdownFlush_SkipsOnTransportError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionErr: transportErrCommandError(),
		// Seeded so a leak through to captureAndCommit surfaces as a
		// list-sessions call the assertions below can catch.
		sessionsOut: "work|1|0|",
	}
	deps := makeDeps(t, dir, fc)

	t.Run("returns_nil", func(t *testing.T) {
		if err := defaultShutdownFlush(deps); err != nil {
			t.Errorf("defaultShutdownFlush() = %v, want nil", err)
		}
	})

	t.Run("zero_commits", func(t *testing.T) {
		if got := fc.callsContaining("list-sessions"); len(got) != 0 {
			t.Errorf("list-sessions invoked despite transport error on @portal-restoring read: %v", got)
		}
		if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json should not be written on transport error; stat=%v", err)
		}
	})
}

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
		if got := fc.callsContaining("capture-pane"); len(got) != 0 {
			t.Errorf("capture-pane invoked despite transport error on @portal-restoring read: %v", got)
		}
		if got := fc.callsContaining("list-sessions"); len(got) != 0 {
			t.Errorf("list-sessions invoked despite transport error on @portal-restoring read: %v", got)
		}
	})

	t.Run("no_commit", func(t *testing.T) {
		if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
			t.Errorf("sessions.json should not be written on transport error; stat=%v", err)
		}
		if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
			t.Errorf("save.requested should survive transport-error skip: %v", err)
		}
	})
}

func TestDaemonStartup_SeedsHashMapFromDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

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

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("runRootCmd(state daemon): %v", err)
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

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("runRootCmd(state daemon): %v", err)
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

	sink := logtest.Install(t)

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("runRootCmd(state daemon): %v", err)
	}
	if *holder == nil {
		t.Fatal("daemonRunFunc not invoked")
	}
	if (*holder).PrevIndex != nil {
		t.Errorf("PrevIndex = %+v; want nil for missing sessions.json", (*holder).PrevIndex)
	}
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

	sink := logtest.Install(t)

	holder := withImmediateRun(t)
	withDaemonLockFileReset(t)

	if _, _, err := runRootCmd(t, "state", "daemon"); err != nil {
		t.Fatalf("runRootCmd(state daemon): %v", err)
	}
	if (*holder).PrevIndex != nil {
		t.Errorf("PrevIndex should be nil on decode error; got %+v", *(*holder).PrevIndex)
	}
	if logged := sink.Body(); !strings.Contains(logged, "sessions.json corrupt") {
		t.Errorf("expected corrupt-index warning in log; got:\n%s", logged)
	}
}

func TestCaptureAndCommit_UncancelledCtxMatchesPreThreadingBehaviour(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|\nside|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||\n" +
			"side|||0|||main|||layout|||0|||1|||0|||/var|||1|||zsh||||||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"side:0.0": "side-pane-0-bytes",
		},
	}

	sentinelPrev := sentinelIndex("sentinel-must-be-replaced")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	if err := captureAndCommit(context.Background(), deps); err != nil {
		t.Fatalf("captureAndCommit returned error on happy path: %v", err)
	}

	assertCommitReplacedPrev(t, deps, sentinelPrev, dir)
	if len(deps.PrevIndex.Sessions) != 2 {
		t.Errorf("PrevIndex.Sessions length = %d; want 2", len(deps.PrevIndex.Sessions))
	}
	for _, sess := range deps.PrevIndex.Sessions {
		if sess.Name == "sentinel-must-be-replaced" {
			t.Errorf("PrevIndex still contains sentinel session; want fresh capture only")
		}
	}

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

func TestCaptureAndCommit_PreCancelledCtxReturnsImmediately(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// A fixture with real work in it, so a missing cancellation check leaks
	// observable tmux calls rather than passing silently.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut:    "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
		},
	}

	sentinelPrev := sentinelIndex("sentinel-must-be-preserved")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on pre-cancelled ctx: %v; want nil", err)
	}

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

	assertNoCommit(t, deps, sentinelPrev, dir)
}

func TestCaptureAndCommit_CancelDuringCaptureStructureReturnsBeforePerPaneWork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|\nside|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||\n" +
			"side|||0|||main|||layout|||0|||1|||0|||/var|||1|||zsh||||||",
		captureByTarget: map[string]string{
			"work:0.0": "work-pane-0-bytes",
			"work:0.1": "work-pane-1-bytes",
			"side:0.0": "side-pane-0-bytes",
		},
	}

	sentinelPrev := sentinelIndex("sentinel-must-be-preserved")
	deps := makeDeps(t, dir, fc)
	deps.PrevIndex = sentinelPrev

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// show-environment is CaptureStructure's final subcall, so cancelling there
	// leaves it completing successfully with a populated index while the ctx is
	// already done — the post-enumeration check, not the entry check.
	fc.dispatchHook = func(args []string) {
		if len(args) > 0 && args[0] == "show-environment" {
			cancel()
		}
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on mid-CaptureStructure cancel: %v; want nil", err)
	}

	if got := fc.callsContaining("list-sessions"); len(got) == 0 {
		t.Errorf("list-sessions not invoked; CaptureStructure did not start")
	}
	if got := fc.callsContaining("list-panes"); len(got) == 0 {
		t.Errorf("list-panes not invoked; CaptureStructure did not enumerate panes")
	}
	if got := fc.callsContaining("show-environment"); len(got) == 0 {
		t.Errorf("show-environment not invoked; CaptureStructure did not reach its env phase")
	}

	if got := fc.callsContaining("capture-pane"); len(got) != 0 {
		t.Errorf("capture-pane invoked after post-enumeration cancel: %v", got)
	}
	for _, key := range []string{
		state.SanitizePaneKey("work", 0, 0),
		state.SanitizePaneKey("work", 0, 1),
		state.SanitizePaneKey("side", 0, 0),
	} {
		if _, err := os.Stat(state.ScrollbackFile(dir, key)); !os.IsNotExist(err) {
			t.Errorf("scrollback file unexpectedly written for %q on cancel: stat err = %v", key, err)
		}
	}

	assertNoCommit(t, deps, sentinelPrev, dir)
}

func TestCaptureAndCommit_CancelMidLoopAfterKofNPanesProcessed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||\n" +
			"work|||0|||main|||layout|||0|||1|||2|||/tmp|||0|||fish||||||",
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

	// The first pane's iteration completes its write; the next iteration's
	// check observes ctx.Done() before capturing pane 2.
	fc.dispatchHook = func(args []string) {
		if len(args) > 0 && args[0] == "capture-pane" {
			cancel()
		}
	}

	if err := captureAndCommit(ctx, deps); err != nil {
		t.Errorf("captureAndCommit returned error on mid-loop cancel: %v; want nil", err)
	}

	captureCalls := fc.callsContaining("capture-pane")
	if len(captureCalls) < 1 {
		t.Errorf("capture-pane invoked %d times; want at least 1", len(captureCalls))
	}
	if len(captureCalls) >= 3 {
		t.Errorf("capture-pane invoked %d times; want fewer than 3 (mid-loop cancel should short-circuit): %v", len(captureCalls), captureCalls)
	}

	// Scrollback files from completed iterations may remain on disk: writes are
	// atomic with no rollback, and the no-partial-commit invariant covers
	// sessions.json only.
	assertNoCommit(t, deps, sentinelPrev, dir)
}

func TestCaptureAndCommit_UncancelledMultiPaneFixtureProcessesAllPanesAndCommits(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||\n" +
			"work|||0|||main|||layout|||0|||1|||2|||/tmp|||0|||fish||||||",
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

	assertCommitReplacedPrev(t, deps, sentinelPrev, dir)
}

func TestDefaultDaemonRun_WritesVersionFileFromDepsVersion(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}

	withFuncSeam(t, &daemonTickLoopFunc, func(_ context.Context, _ *daemonDeps) error { return nil })

	prevLock := daemonLockFile
	daemonLockFile = nil
	t.Cleanup(func() { daemonLockFile = prevLock })

	const want = "regression-sentinel-1.2.3"
	deps := &daemonDeps{
		Dir:          dir,
		Version:      want,
		Logger:       log.Discard(),
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
