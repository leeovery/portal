// Tests in this file mutate package-level state via seams and MUST NOT use
// t.Parallel. They cover the daemon lifecycle catalog events added in
// portal-observability-layer Phase 5: the "lock acquired" subsystem milestone
// and the normal-path "shutdown" event (reason + flush_completed).
package cmd

import (
	"context"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// countLines returns the number of captured lines whose rendered text contains
// every supplied substring.
func countLines(sink *logtest.Sink, substrs ...string) int {
	n := 0
	for _, line := range sink.Lines() {
		all := true
		for _, s := range substrs {
			if !strings.Contains(line, s) {
				all = false
				break
			}
		}
		if all {
			n++
		}
	}
	return n
}

func TestDefaultDaemonRun_EmitsLockAcquiredWithTmuxPane(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("TMUX_PANE", "%42")
	if _, err := state.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	withDaemonLockFileReset(t)

	// Short-circuit the tick loop so defaultDaemonRun returns immediately after
	// its startup write sequence (lock acquire + pidfile + versionfile).
	prevLoop := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error { return nil }
	t.Cleanup(func() { daemonTickLoopFunc = prevLoop })

	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.Logger = logger
	deps.Version = "test"

	if err := defaultDaemonRun(context.Background(), deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	if n := countLines(sink, "INFO", "lock acquired", "component=daemon", "tmux_pane=%42"); n != 1 {
		t.Errorf("expected exactly one 'lock acquired' INFO with tmux_pane=%%42; got %d in:\n%s", n, sink.Body())
	}
}

func TestDefaultDaemonRun_NoLockAcquiredAndKeepsWarnWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("TMUX_PANE", "%42")
	withDaemonLockFileReset(t)

	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, state.ErrDaemonLockHeld
	})

	prevLoop := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("tick loop must not be reached on the lock-held path")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prevLoop })

	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.Logger = logger

	if err := defaultDaemonRun(context.Background(), deps); err != nil {
		t.Fatalf("defaultDaemonRun on lock-held should return nil; got %v", err)
	}

	if n := countLines(sink, "lock acquired"); n != 0 {
		t.Errorf("expected no 'lock acquired' line on lock-held path; got %d in:\n%s", n, sink.Body())
	}
	if n := countLines(sink, "WARN", "another daemon holds the lock"); n != 1 {
		t.Errorf("expected exactly one contention WARN; got %d in:\n%s", n, sink.Body())
	}
}

func TestDefaultDaemonRun_NoLockAcquiredAndKeepsWarnOnLockError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	t.Setenv("TMUX_PANE", "%42")
	withDaemonLockFileReset(t)

	wantErr := errInjectedLockFailure{}
	withAcquireDaemonLockFake(t, func(_ string) (*os.File, error) {
		return nil, wantErr
	})

	prevLoop := daemonTickLoopFunc
	daemonTickLoopFunc = func(_ context.Context, _ *daemonDeps) error {
		t.Fatal("tick loop must not be reached on the lock-error path")
		return nil
	}
	t.Cleanup(func() { daemonTickLoopFunc = prevLoop })

	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.Logger = logger

	if err := defaultDaemonRun(context.Background(), deps); err == nil {
		t.Fatal("defaultDaemonRun on non-EWOULDBLOCK lock error should return a wrapped error; got nil")
	}

	if n := countLines(sink, "lock acquired"); n != 0 {
		t.Errorf("expected no 'lock acquired' line on lock-error path; got %d in:\n%s", n, sink.Body())
	}
	if n := countLines(sink, "WARN", "acquire daemon lock failed"); n != 1 {
		t.Errorf("expected exactly one lock-error WARN; got %d in:\n%s", n, sink.Body())
	}
}

// errInjectedLockFailure is a non-ErrDaemonLockHeld error used to drive the
// non-EWOULDBLOCK lock-error branch of defaultDaemonRun.
type errInjectedLockFailure struct{}

func (errInjectedLockFailure) Error() string { return "injected lock failure" }

func TestDefaultShutdownFlush_EmitsShutdownSighupFlushCompletedTrueOnCleanFlush(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGHUP)

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	// The flush ran (sessions.json committed).
	if _, err := os.Stat(state.SessionsJSON(dir)); err != nil {
		t.Errorf("clean flush did not write sessions.json: %v", err)
	}
	if n := countLines(sink, "INFO", "shutdown", "reason=sighup", "flush_completed=true"); n != 1 {
		t.Errorf("expected exactly one shutdown INFO reason=sighup flush_completed=true; got %d in:\n%s", n, sink.Body())
	}
}

func TestDefaultShutdownFlush_EmitsShutdownFlushCompletedFalseOnRestoringSkip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
		sessionsOut:  "work|1|0",
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGTERM)

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	// No flush ran when restoring.
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("flush ran list-sessions despite restoring marker: %v", got)
	}
	if n := countLines(sink, "INFO", "shutdown", "flush_completed=false"); n != 1 {
		t.Errorf("expected exactly one shutdown INFO flush_completed=false on restoring-skip; got %d in:\n%s", n, sink.Body())
	}
}

func TestDefaultShutdownFlush_EmitsShutdownFlushCompletedFalseWhenFinalCaptureErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGTERM)

	// Force captureAndCommit failure: make sessions.json a directory so the
	// commit's atomic rename cannot land.
	if err := os.MkdirAll(state.SessionsJSON(dir), 0o700); err != nil {
		t.Fatalf("create blocking dir: %v", err)
	}

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush should swallow the flush error and return nil; got %v", err)
	}

	if n := countLines(sink, "WARN", "final flush failed"); n != 1 {
		t.Errorf("expected the final-flush-failed WARN; got %d in:\n%s", n, sink.Body())
	}
	if n := countLines(sink, "INFO", "shutdown", "reason=signal", "flush_completed=false"); n != 1 {
		t.Errorf("expected exactly one shutdown INFO flush_completed=false when capture errors; got %d in:\n%s", n, sink.Body())
	}
}

func TestShutdownReason_MapsSignalsAndDefaultsToExit(t *testing.T) {
	tests := []struct {
		name   string
		record func(*daemonDeps)
		want   string
	}{
		{name: "sighup", record: func(d *daemonDeps) { d.recordShutdownSignal(syscall.SIGHUP) }, want: "sighup"},
		{name: "sigterm", record: func(d *daemonDeps) { d.recordShutdownSignal(syscall.SIGTERM) }, want: "signal"},
		{name: "no-signal", record: func(d *daemonDeps) {}, want: "exit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := &daemonDeps{}
			tc.record(deps)
			if got := deps.shutdownReason(); got != tc.want {
				t.Errorf("shutdownReason() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestShutdownReason_MapsSigtermToSignalEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGTERM)

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	if n := countLines(sink, "INFO", "shutdown", "reason=signal"); n != 1 {
		t.Errorf("expected reason=signal for SIGTERM; got %d in:\n%s", n, sink.Body())
	}
}

func TestShutdownReason_NoRecordedSignalRendersExitEndToEnd(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	// No recordShutdownSignal — models a non-signal ctx cancel.

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	if n := countLines(sink, "INFO", "shutdown", "reason=exit"); n != 1 {
		t.Errorf("expected reason=exit with no recorded signal; got %d in:\n%s", n, sink.Body())
	}
}

func TestDefaultShutdownFlush_EmitsExactlyOneShutdownLinePerInvocation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	sess, panes := oneSession()
	fc := &daemonFakeCommander{sessionsOut: sess, panesOut: panes}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGHUP)
	deps.TickerPeriod = time.Hour

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	if n := countLines(sink, "shutdown", "reason="); n != 1 {
		t.Errorf("expected exactly one shutdown line per invocation; got %d in:\n%s", n, sink.Body())
	}
}

// TestDefaultShutdownFlush_EmitsShutdownFlushCompletedFalseOnRestoringReadError
// covers the read-error branch: IsRestoringSet errors, the WARN is kept, and a
// shutdown INFO with flush_completed=false is emitted (conservatively skipped).
func TestDefaultShutdownFlush_EmitsShutdownFlushCompletedFalseOnRestoringReadError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	fc := &daemonFakeCommander{
		optionErr:   transportErrCommandError(),
		sessionsOut: "work|1|0",
	}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.recordShutdownSignal(syscall.SIGHUP)

	if err := defaultShutdownFlush(deps); err != nil {
		t.Fatalf("defaultShutdownFlush: %v", err)
	}

	// Conservative skip: no flush ran.
	if got := fc.callsContaining("list-sessions"); len(got) != 0 {
		t.Errorf("flush ran list-sessions despite restoring-read error: %v", got)
	}
	if n := countLines(sink, "WARN", "read @portal-restoring at shutdown failed"); n != 1 {
		t.Errorf("expected the restoring-read-error WARN; got %d in:\n%s", n, sink.Body())
	}
	if n := countLines(sink, "INFO", "shutdown", "flush_completed=false"); n != 1 {
		t.Errorf("expected exactly one shutdown INFO flush_completed=false on read-error; got %d in:\n%s", n, sink.Body())
	}
}
