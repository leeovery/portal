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
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// saverEventSink is a slog.Handler that records every emitted record with its
// level, message, and attrs (including those bound via WithAttrs — notably the
// component attr log.For binds at the logger). The saver-lifecycle event tests
// assert on the structured record (component=saver, msg, attr values), so a
// substring sink would be too lossy.
type saverEventSink struct {
	mu      sync.Mutex
	records []saverEventRecord
	shared  *saverEventSink
	bound   []slog.Attr
}

type saverEventRecord struct {
	level slog.Level
	msg   string
	attrs map[string]slog.Value
}

func (s *saverEventSink) owner() *saverEventSink {
	if s.shared != nil {
		return s.shared
	}
	return s
}

func (s *saverEventSink) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (s *saverEventSink) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make([]slog.Attr, 0, len(s.bound)+len(attrs))
	next = append(next, s.bound...)
	next = append(next, attrs...)
	return &saverEventSink{shared: s.owner(), bound: next}
}

func (s *saverEventSink) WithGroup(_ string) slog.Handler {
	return &saverEventSink{shared: s.owner(), bound: s.bound}
}

func (s *saverEventSink) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]slog.Value, len(s.bound)+r.NumAttrs())
	for _, a := range s.bound {
		attrs[a.Key] = a.Value
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	rec := saverEventRecord{level: r.Level, msg: r.Message, attrs: attrs}
	owner := s.owner()
	owner.mu.Lock()
	owner.records = append(owner.records, rec)
	owner.mu.Unlock()
	return nil
}

func (s *saverEventSink) all() []saverEventRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]saverEventRecord, len(s.records))
	copy(out, s.records)
	return out
}

// saverEvents returns every record whose component=saver and msg matches the
// supplied message.
func (s *saverEventSink) saverEvents(msg string) []saverEventRecord {
	var out []saverEventRecord
	for _, r := range s.all() {
		comp, ok := r.attrs["component"]
		if !ok || comp.String() != "saver" || r.msg != msg {
			continue
		}
		out = append(out, r)
	}
	return out
}

// onlySaverEvent asserts exactly one component=saver record with the supplied
// message was emitted and returns it.
func (s *saverEventSink) onlySaverEvent(t *testing.T, msg string) saverEventRecord {
	t.Helper()
	evs := s.saverEvents(msg)
	if len(evs) != 1 {
		t.Fatalf("expected exactly 1 saver %q event, got %d: %+v", msg, len(evs), s.all())
	}
	return evs[0]
}

func (r saverEventRecord) attrString(t *testing.T, key string) string {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.attrs)
	}
	return v.String()
}

func (r saverEventRecord) intAttr(t *testing.T, key string) int64 {
	t.Helper()
	v, ok := r.attrs[key]
	if !ok {
		t.Fatalf("record missing attr %q: %+v", key, r.attrs)
	}
	if v.Kind() != slog.KindInt64 {
		t.Fatalf("attr %q kind = %v, want Int64: %+v", key, v.Kind(), v)
	}
	return v.Int64()
}

func (r saverEventRecord) hasAttr(key string) bool {
	_, ok := r.attrs[key]
	return ok
}

// installSaverEventSink swaps a capturing handler into the process-wide log
// indirection for the duration of the test and returns the sink.
func installSaverEventSink(t *testing.T) *saverEventSink {
	t.Helper()
	sink := &saverEventSink{}
	log.SetTestHandler(t, sink)
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
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if got := rec.attrString(t, "tmux_pane"); got != "%7" {
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
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if got := rec.attrString(t, "tmux_pane"); got != "%7" {
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
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if got := rec.attrString(t, "tmux_pane"); got != "%9" {
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
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if got := rec.intAttr(t, "from_pid"); got != 1111 {
		t.Errorf("from_pid = %d, want 1111", got)
	}
	if got := rec.intAttr(t, "to_pid"); got != 2222 {
		t.Errorf("to_pid = %d, want 2222", got)
	}
	if got := rec.attrString(t, "tmux_pane"); got != "%7" {
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
	if got := rec.intAttr(t, "from_pid"); got != 0 {
		t.Errorf("from_pid = %d, want 0 (read failed)", got)
	}
	if got := rec.intAttr(t, "to_pid"); got != 2222 {
		t.Errorf("to_pid = %d, want 2222", got)
	}

	// The read failure must be logged with the wrapped error attr, under saver.
	failures := sink.saverEvents("saver respawn: pane-pid read failed")
	if len(failures) == 0 {
		t.Fatalf("expected a pane-pid read-failure log line, got none: %+v", sink.all())
	}
	if !failures[0].hasAttr("error") {
		t.Errorf("read-failure line missing error attr: %+v", failures[0].attrs)
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
	if rec.level != slog.LevelInfo {
		t.Errorf("level = %v, want INFO", rec.level)
	}
	if got := rec.intAttr(t, "target_pid"); got != 4321 {
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
