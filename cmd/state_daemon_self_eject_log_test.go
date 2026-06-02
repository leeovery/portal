// Tests in this file mutate package-level state via the saverMembershipProbe,
// osExit, and daemonShutdownFunc seams (plus the process-wide log handler via
// log.SetTestHandler) and MUST NOT use t.Parallel. They cover the daemon
// self-eject lifecycle event added in portal-observability-layer Task 5-10:
// the cataloged "self-eject" INFO, the load-bearing
// self-eject -> log.Close(0) -> osExit(0) ordering, the below-threshold DEBUG
// breadcrumb, and the absence of a "shutdown" line on the eject path.
package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/tmux"
)

// newSharedSelfEjectCapture installs a single capture sink as the process-wide
// log handler (via log.SetTestHandler) and returns both the sink and a
// daemon-component logger bound over that same sink. Records emitted via the
// returned logger (self-eject INFO) and via log.Close(0) -> log.For("process")
// (process: exit) interleave in the one sink.lines buffer in emission order,
// which lets a test assert their relative order.
func newSharedSelfEjectCapture(t *testing.T) (*cmdCaptureSink, *daemonDeps) {
	t.Helper()
	sink := &cmdCaptureSink{}
	// Route log.For(...) (process: exit via log.Close) through the sink.
	log.SetTestHandler(t, sink)
	// deps.Logger is the same sink, pre-bound to component=daemon.
	depsLogger := slog.New(sink).With("component", "daemon")
	return sink, &daemonDeps{Logger: depsLogger}
}

// TestDaemonSelfEject_EmitsCatalogedEventAtTrip asserts the cataloged
// "self-eject" INFO fires at the hysteresis trip with ticks=N threshold=3.
func TestDaemonSelfEject_EmitsCatalogedEventAtTrip(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	want := fmt.Sprintf("ticks=%d", selfSupervisionHysteresisTicks)
	wantThreshold := fmt.Sprintf("threshold=%d", selfSupervisionHysteresisTicks)
	if n := countLines(sink, "INFO", "self-eject", "component=daemon", want, wantThreshold); n != 1 {
		t.Errorf("expected exactly one cataloged self-eject INFO with %s %s; got %d in:\n%s",
			want, wantThreshold, n, sink.body())
	}
}

// TestDaemonSelfEject_RemovesLegacyInfoLine asserts the legacy
// "self-supervision: saver-membership lost" INFO line is gone — replaced by the
// cataloged "self-eject" event.
func TestDaemonSelfEject_RemovesLegacyInfoLine(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if strings.Contains(sink.body(), "self-supervision: saver-membership lost") {
		t.Errorf("legacy 'self-supervision: saver-membership lost' INFO line still present in:\n%s", sink.body())
	}
}

// TestDaemonSelfEject_OrderSelfEjectThenProcessExitThenOsExit asserts the
// load-bearing ordering: the self-eject INFO is emitted, THEN log.Close(0)
// emits the process: exit line, THEN osExit(0) is called. The osExit stub
// snapshots the captured-record count at the moment it fires — both the
// self-eject and the process: exit lines must already be recorded.
func TestDaemonSelfEject_OrderSelfEjectThenProcessExitThenOsExit(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })

	sink, depsBase := newSharedSelfEjectCapture(t)

	// At the instant osExit fires, snapshot the sink so we can assert both
	// lifecycle lines were already emitted (in order) before the exit call.
	var linesAtExit []string
	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		sink.mu.Lock()
		linesAtExit = append([]string(nil), sink.lines...)
		sink.mu.Unlock()
		panic("osExit invoked")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.Logger = depsBase.Logger
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if atomic.LoadInt32(&exitCalls) != 1 {
		t.Fatalf("osExit invoked %d times; want exactly 1", exitCalls)
	}

	selfEjectIdx := indexOfLineContaining(linesAtExit, "self-eject", "component=daemon")
	processExitIdx := indexOfLineContaining(linesAtExit, "exit", "component=process", "code=0")
	if selfEjectIdx < 0 {
		t.Fatalf("self-eject INFO not recorded before osExit; lines at exit:\n%s", strings.Join(linesAtExit, "\n"))
	}
	if processExitIdx < 0 {
		t.Fatalf("process: exit (via log.Close) not recorded before osExit; lines at exit:\n%s", strings.Join(linesAtExit, "\n"))
	}
	if selfEjectIdx >= processExitIdx {
		t.Errorf("ordering violated: self-eject at index %d, process: exit at index %d; want self-eject FIRST then process: exit\nlines at exit:\n%s",
			selfEjectIdx, processExitIdx, strings.Join(linesAtExit, "\n"))
	}
}

// TestDaemonSelfEject_ProcessExitCarriesCodeZero asserts log.Close(0) emits a
// process: exit line with code=0 (not itself an os.Exit — the line must be
// recorded before the osExit stub fires).
func TestDaemonSelfEject_ProcessExitCarriesCodeZero(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	sink, depsBase := newSharedSelfEjectCapture(t)

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.Logger = depsBase.Logger
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if n := countLines(sink, "INFO", "exit", "component=process", "code=0"); n != 1 {
		t.Errorf("expected exactly one process: exit INFO with code=0 (via log.Close(0)); got %d in:\n%s", n, sink.body())
	}
}

// TestDaemonSelfEject_DoesNotEmitShutdownLine asserts no "daemon: shutdown"
// line fires on the self-eject path (daemonShutdownFunc is NOT called).
func TestDaemonSelfEject_DoesNotEmitShutdownLine(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	// Record whether daemonShutdownFunc runs — it must not on the eject path.
	var shutdownCalls int32
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error {
		atomic.AddInt32(&shutdownCalls, 1)
		return nil
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now() // gap=false so tick body is a no-op fast path

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if got := atomic.LoadInt32(&shutdownCalls); got != 0 {
		t.Errorf("daemonShutdownFunc invoked %d times on eject path; want 0", got)
	}
	if n := countLines(sink, "shutdown"); n != 0 {
		t.Errorf("expected no 'shutdown' line on self-eject path; got %d in:\n%s", n, sink.body())
	}
}

// TestDaemonSelfEject_BelowThresholdEmitsDebugOnly asserts that per-tick probe
// failures BELOW the threshold emit a DEBUG breadcrumb (with ticks + threshold
// attrs) and NO INFO until the trip. The probe returns false for the first N-1
// ticks then true forever, so the counter never reaches N and the daemon never
// ejects — only DEBUG breadcrumbs fire.
func TestDaemonSelfEject_BelowThresholdEmitsDebugOnly(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks
	// false × (N-1) then true forever — counter climbs to N-1, never trips.
	var tickIdx int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		idx := atomic.AddInt32(&tickIdx, 1)
		return int(idx) >= N // first N-1 false, then true forever
	})

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked unexpectedly below threshold")
	})
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error { return nil })

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now() // gap=false → tick body is a no-op fast path

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)

	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	if got := atomic.LoadInt32(&exitCalls); got != 0 {
		t.Fatalf("osExit invoked %d times below threshold; want 0", got)
	}
	// At least N-1 DEBUG breadcrumbs must have fired (one per failing tick).
	wantThreshold := fmt.Sprintf("threshold=%d", N)
	if n := countLines(sink, "DEBUG", "saver-membership probe failed", wantThreshold); n < N-1 {
		t.Errorf("expected at least %d DEBUG probe-failure breadcrumbs; got %d in:\n%s", N-1, n, sink.body())
	}
	// No INFO self-eject line — the trip never happened.
	if n := countLines(sink, "INFO", "self-eject"); n != 0 {
		t.Errorf("expected no self-eject INFO below threshold; got %d in:\n%s", n, sink.body())
	}
}

// TestDaemonSelfEject_PassingProbeResetsCounter asserts a failing-then-passing
// sequence does not trip: false × (N-1), true, false × (N-1), true forever.
// The passing probe resets the counter to 0, so the daemon never ejects and no
// self-eject INFO is emitted.
func TestDaemonSelfEject_PassingProbeResetsCounter(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "debug")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks
	// (false × N-1, true) × 2, then true forever.
	script := make([]bool, 0, 2*N)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked despite reset")
	})
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error { return nil })

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)

	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun: %v", err)
	}

	if got := atomic.LoadInt32(&exitCalls); got != 0 {
		t.Fatalf("osExit invoked %d times despite counter reset; want 0", got)
	}
	if n := countLines(sink, "INFO", "self-eject"); n != 0 {
		t.Errorf("expected no self-eject INFO when counter resets; got %d in:\n%s", n, sink.body())
	}
	// Sanity: the full reset script ran.
	if got := probeCalls(); got < int32(2*N-1) {
		t.Fatalf("probe invoked %d times; want >= %d to exercise the full reset script", got, 2*N-1)
	}
}

// TestDaemonSelfEject_UsesOsExitSeam asserts the eject routes through the osExit
// seam (recorded) with code 0, never bare os.Exit.
func TestDaemonSelfEject_UsesOsExitSeam(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })

	var exitCalls int32
	var exitCode int32 = -1
	withOsExitFake(t, func(code int) {
		atomic.StoreInt32(&exitCode, int32(code))
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if got := atomic.LoadInt32(&exitCalls); got != 1 {
		t.Fatalf("osExit seam invoked %d times; want exactly 1", got)
	}
	if got := atomic.LoadInt32(&exitCode); got != 0 {
		t.Errorf("osExit code = %d; want 0", got)
	}
}

// indexOfLineContaining returns the index of the first line in lines that
// contains every supplied substring, or -1 if none match.
func indexOfLineContaining(lines []string, substrs ...string) int {
	for i, line := range lines {
		all := true
		for _, s := range substrs {
			if !strings.Contains(line, s) {
				all = false
				break
			}
		}
		if all {
			return i
		}
	}
	return -1
}
