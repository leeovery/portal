package cmd

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// A save request landing while a tick's cycle runs — a commit-now that timed
// out on the tick's lock, a hook set, a notify — asks for a capture taken after
// it, which the running cycle cannot provide. It must survive that tick.
func TestDaemonTick_KeepsASaveRequestThatLandsDuringTheCycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	// Stands in for a commit-now that timed out mid-dump: the user killed the
	// session after the tick's structure read, and the retry request lands
	// while the tick is still dumping.
	fc.dispatchHook = func(args []string) {
		if len(args) == 0 || args[0] != "capture-pane" {
			return
		}
		fc.sessionsOut, fc.panesOut = "", ""
		if err := state.TouchSaveRequested(dir); err != nil {
			t.Errorf("touch save.requested from capture-pane: %v", err)
		}
	}

	tick(t.Context(), deps)
	fc.dispatchHook = nil

	first, _, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("first tick did not commit: %v", err)
	}
	if len(first.Sessions) != 1 {
		t.Fatalf("first tick committed %d sessions, want the 1 captured before the kill", len(first.Sessions))
	}
	if _, err := os.Stat(state.SaveRequested(dir)); err != nil {
		t.Fatalf("save.requested touched during the cycle must survive the tick; stat err = %v", err)
	}

	listsBefore := len(fc.callsContaining("list-sessions"))
	tick(t.Context(), deps)

	if got := len(fc.callsContaining("list-sessions")); got <= listsBefore {
		t.Errorf("next tick ran no fresh capture: list-sessions calls %d -> %d", listsBefore, got)
	}
	second, _, err := state.ReadIndex(dir)
	if err != nil {
		t.Fatalf("next tick did not commit: %v", err)
	}
	if len(second.Sessions) != 0 {
		t.Errorf("next tick committed %d sessions, want the killed session gone", len(second.Sessions))
	}
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Errorf("save.requested stat err = %v, want it consumed by the committing tick", err)
	}
}

func TestDaemonTick_WarnsWhenReRaisingSaveRequestedAfterAFailedCycleFails(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesErr:    errors.New("list-panes failed"),
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	originalLastSave := time.Now()
	deps.LastSaveAt = originalLastSave
	touchSaveRequested(t, dir)

	// A directory where save.requested belongs makes the re-raise's open fail.
	fc.dispatchHook = func(args []string) {
		if len(args) == 0 || args[0] != "list-panes" {
			return
		}
		if err := os.Mkdir(state.SaveRequested(dir), 0o700); err != nil && !os.IsExist(err) {
			t.Errorf("block save.requested: %v", err)
		}
	}

	tick(t.Context(), deps)

	sink.Records().Matching("daemon", tickBackedOff).AtExactLevel(slog.LevelWarn).Only(t, "tick back-off WARN")
	sink.Records().Matching("daemon", "touch save.requested failed").AtExactLevel(slog.LevelWarn).Only(t, "failed re-raise WARN")
	sink.Records().Matching("daemon", absentIndexWarn).AtExactLevel(slog.LevelWarn).Only(t, "absent sessions.json WARN")
	if n := len(sink.Records().AtOrAboveLevel(slog.LevelWarn)); n != 3 {
		t.Errorf("WARN records = %d, want 3 in:\n%s", n, sink.Body())
	}
	if !deps.LastSaveAt.Equal(originalLastSave) {
		t.Errorf("LastSaveAt advanced despite a failed cycle: %v != %v", deps.LastSaveAt, originalLastSave)
	}
}

func TestDaemonTick_ConsumesSaveRequestedWhenTheCycleIsCancelled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	// Two panes, so the cancel fired from the first pane's capture is observed
	// at the second pane's check; a single pane would finish and commit.
	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut: "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||\n" +
			"work|||0|||main|||layout|||0|||1|||1|||/tmp|||0|||bash||||||",
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	fc.dispatchHook = func(args []string) {
		if len(args) > 0 && args[0] == "capture-pane" {
			cancel()
		}
	}

	tick(ctx, deps)

	if _, err := os.Stat(state.SessionsJSON(dir)); !os.IsNotExist(err) {
		t.Errorf("sessions.json stat err = %v, want no commit from a cancelled cycle", err)
	}
	if n := len(fc.callsContaining("capture-pane")); n >= 2 {
		t.Errorf("capture-pane invoked %d times, want the cancel to stop the dump before the second pane", n)
	}
	warns := sink.Records().AtOrAboveLevel(slog.LevelWarn)
	if len(warns) != 1 || len(warns.WithMessage(absentIndexWarn)) != 1 {
		t.Errorf("cancelled cycle logged a WARN beyond the absent sessions.json one; body:\n%s", sink.Body())
	}
	if _, err := os.Stat(state.SaveRequested(dir)); !os.IsNotExist(err) {
		t.Errorf("save.requested stat err = %v, want it consumed rather than re-raised", err)
	}
}
