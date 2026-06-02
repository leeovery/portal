// Tests in this file mutate package-level state and MUST NOT use t.Parallel.
//
// Phase 5 Task 5-7: the four saver create/respawn/ready lifecycle INFO events
// emitted under component "saver" (via the package-level saverLogger) by
// BootstrapPortalSaver / waitForSaverDaemonReady:
//
//   - placeholder created     (create branch, after createPortalSaverWithRetry)
//   - destroy-unattached off  (both branches, after the set-option succeeds)
//   - respawn-daemon          (create branch, around respawn-pane -k)
//   - daemon ready            (readiness-barrier success only)
//
// These are emitted by bootstrap observing the saver from OUTSIDE — additive
// subsystem milestones, never a substitute for the saver process's own
// process: lines.
//
// Spec reference: § Saver and daemon lifecycle event taxonomy (saver event
// table); § Subsystem prefix taxonomy (lifecycle/contextual attrs
// tmux_pane / from_pid / to_pid / target_pid).

package tmux_test

import (
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// saverEventSink is a thin wrapper over the shared logtest.Sink that adds the
// saver-lifecycle event filtering (component=saver record selection by message
// and emission-order lookup). The lifecycle event tests assert on the
// structured record (component=saver, msg, attr values) via the sink's shared
// accessors.
type saverEventSink struct {
	*logtest.Sink
}

// saverEvents returns every record whose component=saver and msg matches the
// supplied message.
func (s *saverEventSink) saverEvents(msg string) []logtest.Record {
	var out []logtest.Record
	for _, r := range s.Records() {
		comp, ok := r.Attrs["component"]
		if !ok || comp.String() != "saver" || r.Msg != msg {
			continue
		}
		out = append(out, r)
	}
	return out
}

// onlySaverEvent asserts exactly one component=saver record with the supplied
// message was emitted and returns it.
func (s *saverEventSink) onlySaverEvent(t *testing.T, msg string) logtest.Record {
	t.Helper()
	evs := s.saverEvents(msg)
	if len(evs) != 1 {
		t.Fatalf("expected exactly 1 saver %q event, got %d: %+v", msg, len(evs), s.Records())
	}
	return evs[0]
}

// installSaverEventSink swaps the shared logtest.Sink into the process-wide log
// indirection for the duration of the test and returns the wrapper.
func installSaverEventSink(t *testing.T) *saverEventSink {
	t.Helper()
	sink := &saverEventSink{Sink: &logtest.Sink{}}
	log.SetTestHandler(t, sink.Sink)
	return sink
}

// ----------------------------------------------------------------------------
// SaverPaneID helper — #{pane_id} query.
// ----------------------------------------------------------------------------

func TestSaverPaneID_ReturnsTrimmedFirstLine(t *testing.T) {
	var observed []string
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			if args[0] == "list-panes" {
				observed = append([]string{}, args...)
				return "%42\n", nil
			}
			t.Fatalf("unexpected command: %v", args)
			return "", nil
		},
	}
	client := tmux.NewClient(mock)

	got, err := client.SaverPaneID("_portal-saver")
	if err != nil {
		t.Fatalf("SaverPaneID returned error: %v", err)
	}
	if got != "%42" {
		t.Errorf("SaverPaneID = %q, want %q", got, "%42")
	}

	want := "list-panes -t =_portal-saver -F #{pane_id}"
	if joined := strings.Join(observed, " "); joined != want {
		t.Errorf("list-panes argv = %q, want %q", joined, want)
	}
}

func TestSaverPaneID_PropagatesError(t *testing.T) {
	mock := &MockCommander{
		RunFunc: func(args ...string) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
	}
	client := tmux.NewClient(mock)

	_, err := client.SaverPaneID("_portal-saver")
	if err == nil {
		t.Fatal("expected error from SaverPaneID, got nil")
	}
}

// ----------------------------------------------------------------------------
// placeholder created — create branch only.
// ----------------------------------------------------------------------------

func TestBootstrapPortalSaver_EmitsPlaceholderCreatedWithTmuxPane(t *testing.T) {
	stubAliveCheck(t, false) // session absent → pure create branch
	shrinkRetryDelay(t)
	stubReadinessReady(t)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
		listPanes: func(format string, call int) (string, error) {
			if format == "#{pane_id}" {
				return "%7\n", nil
			}
			return "1234\n", nil // pane_pid for respawn-daemon
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "placeholder created")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.AttrString(t, "tmux_pane"); got != "%7" {
		t.Errorf("tmux_pane = %q, want %q", got, "%7")
	}
	// pid is the auto-injected baseline — the call site must NOT pass it.
}

// ----------------------------------------------------------------------------
// destroy-unattached off — both branches.
// ----------------------------------------------------------------------------

func TestBootstrapPortalSaver_EmitsDestroyUnattachedOffOnCreateBranch(t *testing.T) {
	stubAliveCheck(t, false) // create branch
	shrinkRetryDelay(t)
	stubReadinessReady(t)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
		listPanes: func(format string, call int) (string, error) {
			if format == "#{pane_id}" {
				return "%7\n", nil
			}
			return "1234\n", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "destroy-unattached off")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.AttrString(t, "tmux_pane"); got != "%7" {
		t.Errorf("tmux_pane = %q, want %q", got, "%7")
	}
}

func TestBootstrapPortalSaver_EmitsDestroyUnattachedOffOnAliveHappyPath_AndNotRespawnOrReady(t *testing.T) {
	stubAliveCheck(t, true) // session present AND daemon alive → no create, no respawn
	shrinkRetryDelay(t)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		hasSession: func(int) (string, error) { return "", nil }, // present
		setOption:  func(int) (string, error) { return "", nil },
		listPanes: func(format string, call int) (string, error) {
			if format == "#{pane_id}" {
				return "%9\n", nil
			}
			return "5678\n", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "destroy-unattached off")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.AttrString(t, "tmux_pane"); got != "%9" {
		t.Errorf("tmux_pane = %q, want %q", got, "%9")
	}

	// On the alive happy path, no respawn-daemon and no daemon ready events.
	if evs := sink.saverEvents("respawn-daemon"); len(evs) != 0 {
		t.Errorf("expected 0 respawn-daemon events on alive happy path, got %d: %+v", len(evs), evs)
	}
	if evs := sink.saverEvents("daemon ready"); len(evs) != 0 {
		t.Errorf("expected 0 daemon ready events on alive happy path, got %d: %+v", len(evs), evs)
	}
	// placeholder created is also create-branch-only.
	if evs := sink.saverEvents("placeholder created"); len(evs) != 0 {
		t.Errorf("expected 0 placeholder created events on alive happy path, got %d: %+v", len(evs), evs)
	}
}

// ----------------------------------------------------------------------------
// respawn-daemon — create branch only, from_pid pre / to_pid post.
// ----------------------------------------------------------------------------

func TestBootstrapPortalSaver_EmitsRespawnDaemonWithFromToPidAndTmuxPane(t *testing.T) {
	stubAliveCheck(t, false) // create branch
	shrinkRetryDelay(t)
	stubReadinessReady(t)
	sink := installSaverEventSink(t)

	// pane_pid reads: first (pre-respawn, from_pid) returns the placeholder pid,
	// second (post-respawn, to_pid) returns the daemon pid.
	panePIDCall := 0
	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
		listPanes: func(format string, call int) (string, error) {
			if format == "#{pane_id}" {
				return "%7\n", nil
			}
			// #{pane_pid}: from_pid then to_pid.
			panePIDCall++
			if panePIDCall == 1 {
				return "1111\n", nil
			}
			return "2222\n", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "respawn-daemon")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "from_pid"); got != 1111 {
		t.Errorf("from_pid = %d, want 1111", got)
	}
	if got := rec.IntAttr(t, "to_pid"); got != 2222 {
		t.Errorf("to_pid = %d, want 2222", got)
	}
	if got := rec.AttrString(t, "tmux_pane"); got != "%7" {
		t.Errorf("tmux_pane = %q, want %q", got, "%7")
	}
}

// TestBootstrapPortalSaver_StillEmitsRespawnDaemonBestEffortWhenPanePIDReadFails
// pins that a pane-pid read failure (the from_pid read here) does NOT abort the
// bootstrap; respawn-daemon still fires best-effort with the missing pid as 0,
// and the read failure is logged with the wrapped error attr.
func TestBootstrapPortalSaver_StillEmitsRespawnDaemonBestEffortWhenPanePIDReadFails(t *testing.T) {
	stubAliveCheck(t, false)
	shrinkRetryDelay(t)
	stubReadinessReady(t)
	sink := installSaverEventSink(t)

	panePIDCall := 0
	script := &portalSaverScript{
		hasSession: func(int) (string, error) {
			return "", errors.New("can't find session: _portal-saver")
		},
		newSession:  func(int) (string, error) { return "", nil },
		setOption:   func(int) (string, error) { return "", nil },
		respawnPane: func(int) (string, error) { return "", nil },
		listPanes: func(format string, call int) (string, error) {
			if format == "#{pane_id}" {
				return "%7\n", nil
			}
			// #{pane_pid}: first read (from_pid) fails; second (to_pid) succeeds.
			panePIDCall++
			if panePIDCall == 1 {
				return "", errors.New("transient list-panes failure")
			}
			return "2222\n", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.BootstrapPortalSaver(client, "/tmp/portal-state"); err != nil {
		t.Fatalf("BootstrapPortalSaver must not abort on a pid-read miss, got: %v", err)
	}

	rec := sink.onlySaverEvent(t, "respawn-daemon")
	if got := rec.IntAttr(t, "from_pid"); got != 0 {
		t.Errorf("from_pid = %d, want 0 (read failed)", got)
	}
	if got := rec.IntAttr(t, "to_pid"); got != 2222 {
		t.Errorf("to_pid = %d, want 2222", got)
	}

	// The read failure must be logged with the wrapped error attr, under saver.
	failures := sink.saverEvents("saver respawn: pane-pid read failed")
	if len(failures) == 0 {
		t.Fatalf("expected a pane-pid read-failure log line, got none: %+v", sink.Records())
	}
	if !failures[0].HasAttr("error") {
		t.Errorf("read-failure line missing error attr: %+v", failures[0].Attrs)
	}
}

// ----------------------------------------------------------------------------
// daemon ready — readiness-barrier success only.
// ----------------------------------------------------------------------------

func TestWaitForSaverDaemonReady_EmitsDaemonReadyWithTargetPidOnSuccess(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 500*time.Millisecond)
	sink := installSaverEventSink(t)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "daemon ready")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("target_pid = %d, want 4321", got)
	}
	// version is the auto-baseline — the call site must NOT pass it.
}

func TestWaitForSaverDaemonReady_EmitsNoDaemonReadyAndKeepsWarnOnTimeout(t *testing.T) {
	installReadinessPollInterval(t, 1*time.Millisecond)
	installReadinessTimeout(t, 20*time.Millisecond)
	sink := installSaverEventSink(t)

	installReadinessReadPID(t, func(string) (int, error) { return 4321, nil })
	installReadinessIdentify(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyDead, nil // never identifies → timeout
	})
	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)

	if err := tmux.WaitForSaverDaemonReady(t.TempDir()); err != nil {
		t.Fatalf("WaitForSaverDaemonReady returned error: %v", err)
	}

	if evs := sink.saverEvents("daemon ready"); len(evs) != 0 {
		t.Errorf("expected 0 daemon ready events on timeout, got %d: %+v", len(evs), evs)
	}
	if len(barrierLog.warns) != 1 {
		t.Errorf("expected exactly 1 WARN on timeout, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

// ----------------------------------------------------------------------------
// Task 5-8: kill-barrier lifecycle INFO events under component "saver" emitted
// by killSaverAndWaitForDaemon / escalateKillToSIGKILL.
//
//   - kill-barrier started   target_pid=X            (prior daemon alive, before kill-session)
//   - kill-barrier escalated target_pid=X reason=kill-session-timeout
//                                                     (IdentifyIsPortalDaemon escalation branch only)
//   - placeholder died       target_pid=X reason=signal
//                                                     (observed-exit poll branches: kill-session-exit
//                                                      and post-SIGKILL-exit)
//
// These are ADDITIVE INFO events: the at-most-one-WARN-per-invocation contract
// of the kill barrier is preserved unchanged.
//
// Spec reference: § Saver and daemon lifecycle event taxonomy (saver event
// table, closed reason value spaces); § Defensive invariants (Externally-
// killed-process footnote — the killer records the kill).
// ----------------------------------------------------------------------------

// firstSaverIndex returns the 0-based ordinal of the first component=saver
// record with the supplied message in emission order, or -1 if absent. Used to
// assert relative ordering of lifecycle events on the same captured stream
// (e.g. escalated INFO before the SIGKILL-escalation DEBUG breadcrumb).
func (s *saverEventSink) firstSaverIndex(msg string) int {
	for i, r := range s.Records() {
		comp, ok := r.Attrs["component"]
		if !ok || comp.String() != "saver" || r.Msg != msg {
			continue
		}
		return i
	}
	return -1
}

func TestKillSaverAndWaitForDaemon_EmitsKillBarrierStartedWhenPriorDaemonAlive(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	// Alive for the first two probes (initial check + first tick), then dead.
	calls := 0
	installBarrierIsAlive(t, func(int) bool {
		calls++
		return calls < 3
	})
	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	// killSession must run AFTER the started INFO has been recorded. Snapshot
	// the started-event count at kill-session time to pin the ordering.
	startedAtKillTime := -1
	script := &portalSaverScript{
		killSession: func(int) (string, error) {
			startedAtKillTime = len(sink.saverEvents("kill-barrier started"))
			return "", nil
		},
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "kill-barrier started")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("target_pid = %d, want 4321", got)
	}
	if startedAtKillTime != 1 {
		t.Errorf("kill-barrier started must be emitted BEFORE kill-session (count at kill time = %d, want 1)", startedAtKillTime)
	}
	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on clean exit, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

func TestKillSaverAndWaitForDaemon_NoKillBarrierStartedOnNoPriorPIDShortcut(t *testing.T) {
	installBarrierReadPID(t, func(string) (int, error) {
		return 0, errors.New("daemon.pid absent")
	})
	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if evs := sink.saverEvents("kill-barrier started"); len(evs) != 0 {
		t.Errorf("expected 0 kill-barrier started events on no-prior-PID shortcut, got %d: %+v", len(evs), evs)
	}
	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on tolerant-kill shortcut, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

func TestKillSaverAndWaitForDaemon_NoKillBarrierStartedWhenPriorDaemonAlreadyDead(t *testing.T) {
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
	installBarrierIsAlive(t, func(int) bool { return false }) // already dead
	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	if evs := sink.saverEvents("kill-barrier started"); len(evs) != 0 {
		t.Errorf("expected 0 kill-barrier started events when prior daemon already dead, got %d: %+v", len(evs), evs)
	}
	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on already-dead shortcut, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

func TestKillSaverAndWaitForDaemon_EmitsKillBarrierEscalatedAboveDebugBreadcrumbOnPortalDaemonBranch(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 5*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	// Snapshot the escalated-INFO count at SIGKILL time. Since both the escalated
	// INFO and the DEBUG breadcrumb precede SendSIGKILL, the escalated INFO must
	// already be recorded when SIGKILL fires.
	killCalls := 0
	escalatedAtKillTime := -1
	installBarrierSendSIGKILL(t, func(int) error {
		escalatedAtKillTime = len(sink.saverEvents("kill-barrier escalated"))
		killCalls++
		return nil
	})
	// Alive during session-kill poll; dead immediately after SIGKILL.
	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "kill-barrier escalated")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("target_pid = %d, want 4321", got)
	}
	if got := rec.AttrString(t, "reason"); got != "kill-session-timeout" {
		t.Errorf("reason = %q, want %q", got, "kill-session-timeout")
	}

	// escalated INFO must precede SIGKILL.
	if escalatedAtKillTime != 1 {
		t.Errorf("kill-barrier escalated must be emitted BEFORE SIGKILL (count at kill time = %d, want 1)", escalatedAtKillTime)
	}

	// The existing Phase-4 DEBUG breadcrumb must still be present exactly once,
	// and the escalated INFO must come BEFORE it.
	breadcrumbIdx := sink.firstSaverIndex("kill-barrier escalating to SIGKILL")
	if breadcrumbIdx < 0 {
		t.Fatalf("expected the existing DEBUG breadcrumb %q to still be present: %+v", "kill-barrier escalating to SIGKILL", sink.Records())
	}
	if breadcrumbs := sink.saverEvents("kill-barrier escalating to SIGKILL"); len(breadcrumbs) != 1 {
		t.Errorf("expected exactly 1 DEBUG breadcrumb, got %d: %+v", len(breadcrumbs), breadcrumbs)
	}
	escalatedIdx := sink.firstSaverIndex("kill-barrier escalated")
	if escalatedIdx < 0 || escalatedIdx >= breadcrumbIdx {
		t.Errorf("escalated INFO (idx %d) must precede the DEBUG breadcrumb (idx %d)", escalatedIdx, breadcrumbIdx)
	}

	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on clean escalation, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

func TestKillSaverAndWaitForDaemon_NoKillBarrierEscalatedAndKeepsSingleWarnOnIdentitySkip(t *testing.T) {
	cases := []struct {
		name   string
		result state.IdentifyResult
		idErr  error
	}{
		{"IdentifyDead", state.IdentifyDead, nil},
		{"IdentifyNotPortalDaemon", state.IdentifyNotPortalDaemon, nil},
		{"TransientError", 0, errors.New("ps exec failed: transient")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			installBarrierPollInterval(t, 1*time.Millisecond)
			installBarrierTimeout(t, 5*time.Millisecond)
			installBarrierEscalationTimeout(t, 5*time.Millisecond)
			installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
			installBarrierIsAlive(t, func(int) bool { return true })

			installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
				return tc.result, tc.idErr
			})

			killCalls := 0
			installBarrierSendSIGKILL(t, func(int) error {
				killCalls++
				return nil
			})

			barrierLog := &recordingBarrierLogger{}
			installBarrierLogger(t, barrierLog)
			sink := installSaverEventSink(t)

			script := &portalSaverScript{
				killSession: func(int) (string, error) { return "", nil },
			}
			mock := &MockCommander{RunFunc: script.run(t)}
			client := tmux.NewClient(mock)

			if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
				t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
			}

			if killCalls != 0 {
				t.Errorf("expected 0 SIGKILL seam calls on identity-skip branch, got %d", killCalls)
			}
			if evs := sink.saverEvents("kill-barrier escalated"); len(evs) != 0 {
				t.Errorf("expected 0 kill-barrier escalated events on identity-skip branch, got %d: %+v", len(evs), evs)
			}
			if len(barrierLog.warns) != 1 {
				t.Errorf("expected exactly 1 WARN on identity-skip branch, got %d: %v", len(barrierLog.warns), barrierLog.warns)
			}
		})
	}
}

func TestKillSaverAndWaitForDaemon_EmitsPlaceholderDiedReasonSignalOnKillSessionExit(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 500*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	// Alive for the initial check + first tick, then dead — observed exit after
	// kill-session, never reaching escalation.
	calls := 0
	installBarrierIsAlive(t, func(int) bool {
		calls++
		return calls < 3
	})
	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "placeholder died")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("target_pid = %d, want 4321", got)
	}
	if got := rec.AttrString(t, "reason"); got != "signal" {
		t.Errorf("reason = %q, want %q", got, "signal")
	}
	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on observed exit, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

func TestKillSaverAndWaitForDaemon_EmitsPlaceholderDiedReasonSignalOnPostSIGKILLExit(t *testing.T) {
	installBarrierPollInterval(t, 1*time.Millisecond)
	installBarrierTimeout(t, 5*time.Millisecond)
	installBarrierEscalationTimeout(t, 500*time.Millisecond)
	installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })

	installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
		return state.IdentifyIsPortalDaemon, nil
	})

	// Alive during the session-kill poll (forces escalation); dead immediately
	// after SIGKILL — observed exit on the post-SIGKILL poll branch.
	killCalls := 0
	installBarrierSendSIGKILL(t, func(int) error {
		killCalls++
		return nil
	})
	installBarrierIsAlive(t, func(int) bool { return killCalls == 0 })

	barrierLog := &recordingBarrierLogger{}
	installBarrierLogger(t, barrierLog)
	sink := installSaverEventSink(t)

	script := &portalSaverScript{
		killSession: func(int) (string, error) { return "", nil },
	}
	mock := &MockCommander{RunFunc: script.run(t)}
	client := tmux.NewClient(mock)

	if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
		t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
	}

	rec := sink.onlySaverEvent(t, "placeholder died")
	if rec.Level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "target_pid"); got != 4321 {
		t.Errorf("target_pid = %d, want 4321", got)
	}
	if got := rec.AttrString(t, "reason"); got != "signal" {
		t.Errorf("reason = %q, want %q", got, "signal")
	}
	if len(barrierLog.warns) != 0 {
		t.Errorf("expected 0 WARN lines on post-SIGKILL observed exit, got %d: %v", len(barrierLog.warns), barrierLog.warns)
	}
}

// TestKillSaverAndWaitForDaemon_PreservesAtMostOneWarnContractAcrossLifecycleEvents
// pins the at-most-one-WARN-per-invocation contract on the WARN-emitting paths
// after the additive INFO events. The escalation-survive path (SIGKILL sent but
// the process never exits) and the timeout-then-identity-skip path each emit
// exactly one WARN.
func TestKillSaverAndWaitForDaemon_PreservesAtMostOneWarnContractAcrossLifecycleEvents(t *testing.T) {
	t.Run("escalation survives SIGKILL", func(t *testing.T) {
		installBarrierPollInterval(t, 1*time.Millisecond)
		installBarrierTimeout(t, 5*time.Millisecond)
		installBarrierEscalationTimeout(t, 5*time.Millisecond)
		installBarrierReadPID(t, func(string) (int, error) { return 4321, nil })
		installBarrierIdentifyDaemon(t, func(int) (state.IdentifyResult, error) {
			return state.IdentifyIsPortalDaemon, nil
		})
		installBarrierSendSIGKILL(t, func(int) error { return nil })
		installBarrierIsAlive(t, func(int) bool { return true }) // never dies

		barrierLog := &recordingBarrierLogger{}
		installBarrierLogger(t, barrierLog)
		sink := installSaverEventSink(t)

		script := &portalSaverScript{
			killSession: func(int) (string, error) { return "", nil },
		}
		mock := &MockCommander{RunFunc: script.run(t)}
		client := tmux.NewClient(mock)

		if err := tmux.KillSaverAndWaitForDaemon(client, t.TempDir()); err != nil {
			t.Fatalf("KillSaverAndWaitForDaemon returned error: %v", err)
		}

		if len(barrierLog.warns) != 1 {
			t.Errorf("expected exactly 1 WARN on escalation-survive path, got %d: %v", len(barrierLog.warns), barrierLog.warns)
		}
		// started + escalated INFO emitted; placeholder died NOT (process never exited).
		if evs := sink.saverEvents("kill-barrier started"); len(evs) != 1 {
			t.Errorf("expected 1 kill-barrier started, got %d", len(evs))
		}
		if evs := sink.saverEvents("kill-barrier escalated"); len(evs) != 1 {
			t.Errorf("expected 1 kill-barrier escalated, got %d", len(evs))
		}
		if evs := sink.saverEvents("placeholder died"); len(evs) != 0 {
			t.Errorf("expected 0 placeholder died on never-exits path, got %d: %+v", len(evs), evs)
		}
	})
}
