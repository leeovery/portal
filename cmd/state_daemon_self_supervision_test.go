package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

func withSaverMembershipProbeFake(t *testing.T, fake func(*tmux.Client, int) bool) {
	t.Helper()
	withFuncSeam(t, &saverMembershipProbe, fake)
}

// withOsExitFake swaps the osExit seam. The replacement is expected to panic
// after recording: were it to return, the ticker loop would keep running, which
// the real os.Exit would never allow.
func withOsExitFake(t *testing.T, fake func(int)) {
	t.Helper()
	withFuncSeam(t, &osExit, fake)
}

func withDaemonShutdownFuncFake(t *testing.T, fake func(*daemonDeps) error) {
	t.Helper()
	withFuncSeam(t, &daemonShutdownFunc, fake)
}

// runDaemonLoopUntilEject runs defaultDaemonRun in a goroutine, returning a
// channel closed when it returns. Keep deps.TickerPeriod sub-millisecond so the
// wall time stays bounded.
func runDaemonLoopUntilEject(t *testing.T, deps *daemonDeps, ctx context.Context) <-chan struct{} {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = recover() }()
		_ = defaultDaemonRun(ctx, deps)
	}()
	return done
}

func TestDaemonLoop_SelfCheckBypassesShutdownOnEject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	var probeCalls atomic.Int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		probeCalls.Add(1)
		return false
	})

	var exitCalls int32
	var exitCode int32 = -1
	withOsExitFake(t, func(code int) {
		atomic.StoreInt32(&exitCode, int32(code))
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked — abort loop")
	})

	var shutdownCalls atomic.Int32
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error {
		shutdownCalls.Add(1)
		return nil
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if atomic.LoadInt32(&exitCalls) != 1 {
		t.Fatalf("osExit invoked %d times; want 1", exitCalls)
	}
	if got := atomic.LoadInt32(&exitCode); got != 0 {
		t.Errorf("osExit code = %d; want 0", got)
	}
	if got := shutdownCalls.Load(); got != 0 {
		t.Errorf("daemonShutdownFunc invoked %d times on eject path; want 0", got)
	}
	if probe := probeCalls.Load(); probe < int32(selfSupervisionHysteresisTicks) {
		t.Errorf("probe invoked %d times; want at least %d before eject", probe, selfSupervisionHysteresisTicks)
	}
}

func TestDaemonLoop_SelfCheckSkipsCaptureOnEjectTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0|",
		panesOut:    "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh||||||",
	}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()
	touchSaveRequested(t, dir)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	// The eject tick, and any no-op ticks before it, must add no further call.
	gotList := len(fc.callsContaining("list-sessions"))
	if gotList > 1 {
		t.Errorf("list-sessions invoked %d times; want ≤ 1 (eject tick must not run captureAndCommit)", gotList)
	}
}

func TestDaemonLoop_SelfCheckRunsBeforeIsRestoringSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked")
	})

	// With @portal-restoring set, a self-check inside tick would sit behind the
	// restoring early-return and never eject.
	fc := &daemonFakeCommander{
		optionByName: map[string]string{state.RestoringMarkerName: "1"},
	}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if atomic.LoadInt32(&exitCalls) != 1 {
		t.Errorf("osExit invoked %d times despite @portal-restoring; want 1", exitCalls)
	}
}

func TestDaemonLoop_SelfCheckDoesNotDeleteDaemonPID(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	withDaemonLockFileReset(t)

	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("seed dir: %v", err)
	}

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) {
		panic("osExit invoked")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	// The value is the live daemon's pid: defaultDaemonRun writes it at startup.
	got, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("daemon.pid missing after eject; ReadPIDFile: %v", err)
	}
	if got != os.Getpid() {
		t.Errorf("daemon.pid = %d; want %d (live daemon pid written at startup)", got, os.Getpid())
	}
}

func TestDaemonLoop_SelfCheckResetsCounterOnProbeTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	var tickIdx atomic.Int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		idx := tickIdx.Add(1)
		return idx >= 3
	})

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked unexpectedly")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)

	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error { return nil })

	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun returned: %v", err)
	}

	if exitCalls != 0 {
		t.Errorf("osExit invoked %d times despite reset; want 0", exitCalls)
	}
	if got := tickIdx.Load(); got < int32(selfSupervisionHysteresisTicks)+2 {
		t.Errorf("probe invoked %d times; want at least %d to make the test meaningful",
			got, selfSupervisionHysteresisTicks+2)
	}
}

func TestDaemonLoop_SelfCheckEjectsExactlyOnNthFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	var probeCalls atomic.Int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		probeCalls.Add(1)
		return false
	})

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
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

	if atomic.LoadInt32(&exitCalls) != 1 {
		t.Fatalf("osExit invoked %d times; want exactly 1", exitCalls)
	}
	if got := probeCalls.Load(); got != int32(selfSupervisionHysteresisTicks) {
		t.Errorf("probe invoked %d times before eject; want exactly %d",
			got, selfSupervisionHysteresisTicks)
	}
}

func TestDaemonLoop_SelfCheckResetOnEachTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks
	// (false x N-1, true) x 2, then false x N: eject on the final false.
	script := make([]bool, 0, 3*N)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for range N {
		script = append(script, false)
	}

	var probeCalls atomic.Int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		idx := probeCalls.Add(1)
		i := int(idx) - 1
		if i >= len(script) {
			return true
		}
		return script[i]
	})

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
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

	if atomic.LoadInt32(&exitCalls) != 1 {
		t.Errorf("osExit invoked %d times; want 1", exitCalls)
	}
	gotProbes := probeCalls.Load()
	wantProbes := int32(len(script))
	if gotProbes != wantProbes {
		t.Errorf("probe invoked %d times; want %d (reset must happen on each true)",
			gotProbes, wantProbes)
	}
}

func TestDaemonLoop_SelfCheckLogsInfoOnEject(t *testing.T) {
	t.Setenv("PORTAL_LOG_LEVEL", "info")
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) {
		panic("osExit invoked")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	logger, sink := newCaptureLoggerForComponent(t, "daemon")
	deps.Logger = logger
	deps.TickerPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	got := sink.Body()
	if !strings.Contains(got, "INFO") {
		t.Errorf("expected INFO log line; got:\n%s", got)
	}
	if !strings.Contains(got, "daemon") {
		t.Errorf("expected ComponentDaemon = %q in log; got:\n%s", "daemon", got)
	}
	if !strings.Contains(got, "self-eject") {
		t.Errorf("expected cataloged self-eject event; got:\n%s", got)
	}
	if strings.Contains(got, "self-supervision: saver-membership lost") {
		t.Errorf("legacy self-supervision INFO line still present; got:\n%s", got)
	}
	want := fmt.Sprintf("ticks=%d", selfSupervisionHysteresisTicks)
	if !strings.Contains(got, want) {
		t.Errorf("expected consecutive-count %q in log; got:\n%s", want, got)
	}
	wantThreshold := fmt.Sprintf("threshold=%d", selfSupervisionHysteresisTicks)
	if !strings.Contains(got, wantThreshold) {
		t.Errorf("expected threshold %q in log; got:\n%s", wantThreshold, got)
	}
}

// scriptedProbe returns a saverMembershipProbe stub backed by a bool sequence.
// Once the script is exhausted it returns true forever: ctx-cancel, not script
// exhaustion, is what terminates these loops.
func scriptedProbe(script []bool) (probe func(*tmux.Client, int) bool, calls func() int32) {
	var n atomic.Int32
	probe = func(_ *tmux.Client, _ int) bool {
		idx := n.Add(1)
		i := int(idx) - 1
		if i >= len(script) {
			return true
		}
		return script[i]
	}
	calls = func() int32 { return n.Load() }
	return probe, calls
}

// runDaemonUntilCancel runs defaultDaemonRun synchronously and returns the
// osExit call count. The osExit fake panics so an eject unwinds the loop and the
// surrounding recover turns it into a visible non-zero count.
func runDaemonUntilCancel(t *testing.T, deps *daemonDeps, ctx context.Context) (exitCalls int32) {
	t.Helper()
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error { return nil })
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked — counter-reset invariant violated")
	})
	func() {
		defer func() { _ = recover() }()
		_ = defaultDaemonRun(ctx, deps)
	}()
	return atomic.LoadInt32(&exitCalls)
}

func TestSelfSupervisionCounter_ResetsFullyOnFirstProbeTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// (false x N-1, true, false x N-1). A decrementing counter would reach
	// 2N-3 >= N by the end of the final segment and eject; a resetting one
	// climbs only to N-1.
	script := make([]bool, 0, 2*N)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	exitCalls := runDaemonUntilCancel(t, deps, ctx)

	if exitCalls != 0 {
		t.Fatalf("osExit invoked %d times; counter-reset invariant violated", exitCalls)
	}
	if got := probeCalls(); got < int32(2*N-1) {
		t.Fatalf("probe invoked %d times; want ≥ %d to exercise full reset script",
			got, 2*N-1)
	}
}

func TestSelfSupervisionCounter_BoundaryKEqualsNMinus1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks
	const cycles = 5

	// (false x N-1, true) x cycles.
	script := make([]bool, 0, cycles*N)
	for range cycles {
		for i := 0; i < N-1; i++ {
			script = append(script, false)
		}
		script = append(script, true)
	}

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	exitCalls := runDaemonUntilCancel(t, deps, ctx)

	if exitCalls != 0 {
		t.Fatalf("osExit invoked %d times across %d boundary cycles; want 0", exitCalls, cycles)
	}
	if got := probeCalls(); got < int32(cycles*N) {
		t.Fatalf("probe invoked %d times; want ≥ %d to cover all %d cycles",
			got, cycles*N, cycles)
	}
}

func TestSelfSupervisionCounter_ManyAbsentPresentCycles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// Absent-streak length varies per cycle: 1, 2, ..., N-1.
	const rounds = 5
	script := make([]bool, 0, 64)
	for range rounds {
		for k := 1; k <= N-1; k++ {
			for i := 0; i < k; i++ {
				script = append(script, false)
			}
			script = append(script, true)
		}
	}

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	exitCalls := runDaemonUntilCancel(t, deps, ctx)

	if exitCalls != 0 {
		t.Fatalf("osExit invoked %d times across mixed absent-present cycles; want 0", exitCalls)
	}
	minCallsForOneRound := 0
	for k := 1; k <= N-1; k++ {
		minCallsForOneRound += k + 1
	}
	if got := probeCalls(); got < int32(minCallsForOneRound) {
		t.Fatalf("probe invoked %d times; want ≥ %d to cover one full round",
			got, minCallsForOneRound)
	}
}

func TestSelfSupervisionCounter_IncrementsUniformlyOnProbeFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// The (false, true) prelude leaves the counter at exactly 0 entering the
	// final segment, so the eject must land on probe call 2*P + N.
	const preludePairs = 3
	script := make([]bool, 0, 2*preludePairs+N)
	for range preludePairs {
		script = append(script, false, true)
	}
	for range N {
		script = append(script, false)
	}

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	var exitCalls atomic.Int32
	withOsExitFake(t, func(_ int) {
		exitCalls.Add(1)
		panic("osExit invoked")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	if got := exitCalls.Load(); got != 1 {
		t.Fatalf("osExit invoked %d times; want exactly 1", got)
	}
	wantCalls := int32(2*preludePairs + N)
	if got := probeCalls(); got != wantCalls {
		t.Errorf("probe invoked %d times before eject; want %d (uniform increment + reset-on-true)",
			got, wantCalls)
	}
}

func TestSelfSupervisionHysteresisTicks_LowerBound(t *testing.T) {
	if selfSupervisionHysteresisTicks < 1 {
		t.Fatalf("selfSupervisionHysteresisTicks must be >= 1, got %d", selfSupervisionHysteresisTicks)
	}
}
