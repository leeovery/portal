// Tests in this file mutate package-level state via the saverMembershipProbe
// and osExit seams and MUST NOT use t.Parallel.
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

// withSaverMembershipProbeFake swaps the package-level saverMembershipProbe
// seam for the duration of the test and restores it via t.Cleanup.
func withSaverMembershipProbeFake(t *testing.T, fake func(*tmux.Client, int) bool) {
	t.Helper()
	prev := saverMembershipProbe
	saverMembershipProbe = fake
	t.Cleanup(func() { saverMembershipProbe = prev })
}

// withOsExitFake swaps the package-level osExit seam for the duration of the
// test. The supplied function is invoked in place of os.Exit; tests typically
// record the call and then panic with the supplied sentinel to abort the
// ticker for-loop (since osExit returning would let the loop continue, which
// is not the production behaviour we're modelling).
func withOsExitFake(t *testing.T, fake func(int)) {
	t.Helper()
	prev := osExit
	osExit = fake
	t.Cleanup(func() { osExit = prev })
}

// withDaemonShutdownFuncFake swaps daemonShutdownFunc for the duration of the
// test and restores via t.Cleanup. Tests use this to record whether the
// shutdown path ran during a self-eject (it must not).
func withDaemonShutdownFuncFake(t *testing.T, fake func(*daemonDeps) error) {
	t.Helper()
	prev := daemonShutdownFunc
	daemonShutdownFunc = fake
	t.Cleanup(func() { daemonShutdownFunc = prev })
}

// runDaemonLoopUntilEject runs defaultDaemonRun in a goroutine and returns a
// channel that closes when the daemon returns (which happens either via the
// supplied osExit fake panicking to unwind the loop, or via ctx-cancel).
//
// The deps.TickerPeriod should be sub-millisecond so the ticker fires fast
// enough to keep the test wall time bounded.
//
// The osExit fake is expected to panic after recording so the loop unwinds
// even though the real os.Exit would have terminated the process.
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

// TestDaemonLoop_SelfCheckBypassesShutdownOnEject asserts the load-bearing
// invariant of the eject: osExit fires AND daemonShutdownFunc does NOT run.
// This proves the eject bypasses the deferred final-flush path — the spec is
// explicit that a divergent-view daemon must not execute one more
// captureAndCommit cycle on its way out.
func TestDaemonLoop_SelfCheckBypassesShutdownOnEject(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Probe always returns false → counter climbs every tick.
	var probeCalls int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		atomic.AddInt32(&probeCalls, 1)
		return false
	})

	var exitCalls int32
	var exitCode int32 = -1
	withOsExitFake(t, func(code int) {
		atomic.StoreInt32(&exitCode, int32(code))
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked — abort loop")
	})

	// daemonShutdownFunc must not run on the eject path; record if it does.
	var shutdownCalls int32
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error {
		atomic.AddInt32(&shutdownCalls, 1)
		return nil
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now() // gap=false so tick body is a no-op fast path

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
	if got := atomic.LoadInt32(&shutdownCalls); got != 0 {
		t.Errorf("daemonShutdownFunc invoked %d times on eject path; want 0", got)
	}
	if probe := atomic.LoadInt32(&probeCalls); probe < int32(selfSupervisionHysteresisTicks) {
		t.Errorf("probe invoked %d times; want at least %d before eject", probe, selfSupervisionHysteresisTicks)
	}
}

// TestDaemonLoop_SelfCheckSkipsCaptureOnEjectTick asserts that on the eject
// tick itself (the N-th consecutive probe-false), captureAndCommit is NOT
// invoked. Below-threshold ticks DO run captureAndCommit (the self-check is
// non-disruptive until divergence is confirmed). The proof: with the dirty
// flag set ONCE, only ONE tick reaches list-sessions (the first), but the
// counter still climbs to N over subsequent no-op-fast-path ticks and ejects;
// captureAndCommit is never invoked on the eject tick because the eject
// short-circuits before tick().
//
// Concretely: tick 1 runs (probe-false → counter=1; tick body fires; flag
// cleared); ticks 2 and 3 run with no dirty flag → tick body is fast-path
// no-op but still counter increments. On tick 3 (=N), eject fires BEFORE
// tick(). Net: exactly one list-sessions call total, regardless of N.
func TestDaemonLoop_SelfCheckSkipsCaptureOnEjectTick(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })
	withOsExitFake(t, func(_ int) { panic("osExit invoked") })

	fc := &daemonFakeCommander{
		sessionsOut: "work|1|0",
		panesOut:    "work|||0|||main|||layout|||0|||1|||0|||/tmp|||1|||zsh",
	}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now() // gap=false; only dirty flag drives tick body
	touchSaveRequested(t, dir)   // arm exactly once

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(cancel)

	done := runDaemonLoopUntilEject(t, deps, ctx)
	<-done

	// At most one list-sessions call — the first dirty tick. The eject tick
	// (and any no-op ticks in between) must NOT add a second call. If the
	// eject were placed AFTER tick (incorrect ordering), a re-arming dirty
	// flag scenario would produce > 1 call. The single-arm scenario here
	// pins the simpler invariant: the eject tick does not run captureAndCommit.
	gotList := len(fc.callsContaining("list-sessions"))
	if gotList > 1 {
		t.Errorf("list-sessions invoked %d times; want ≤ 1 (eject tick must not run captureAndCommit)", gotList)
	}
}

// TestDaemonLoop_SelfCheckRunsBeforeIsRestoringSet asserts that the self-check
// fires even when @portal-restoring is set. If the self-check were placed
// inside tick (after IsRestoringSet), the restoring early-return would mask
// the divergence; the spec explicitly forbids this ordering.
func TestDaemonLoop_SelfCheckRunsBeforeIsRestoringSet(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool { return false })

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked")
	})

	// @portal-restoring is set — if the self-check were inside tick after the
	// restoring early-return, the eject would never fire.
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

// TestDaemonLoop_SelfCheckDoesNotDeleteDaemonPID asserts that the eject path
// leaves daemon.pid on disk — Component C's pre-check on the next acquire
// handles cleanup. Deleting here would be racy and would invert the
// layered-enforcement contract.
//
// Post-Component-C-step-4 refactor, defaultDaemonRun writes daemon.pid at
// the head of the function (as the statement immediately following the
// acquireDaemonLock guard). The test therefore observes the CURRENT
// process's pid in daemon.pid after the eject — not the pre-seeded sentinel.
// The invariant under test is "daemon.pid exists after eject" (not deleted);
// the value is the live daemon's pid by construction.
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

	// daemon.pid must still exist post-eject — the eject path must not
	// delete it. The value is the live daemon's pid because defaultDaemonRun
	// writes it at startup (immediately after the acquireDaemonLock guard).
	got, err := state.ReadPIDFile(dir)
	if err != nil {
		t.Fatalf("daemon.pid missing after eject; ReadPIDFile: %v", err)
	}
	if got != os.Getpid() {
		t.Errorf("daemon.pid = %d; want %d (live daemon pid written at startup)", got, os.Getpid())
	}
}

// TestDaemonLoop_SelfCheckResetsCounterOnProbeTrue asserts the canonical
// hysteresis: two consecutive false returns followed by a true reset the
// counter, so no eject happens even after many subsequent ticks (assuming
// they stay true).
func TestDaemonLoop_SelfCheckResetsCounterOnProbeTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Sequence: false, false, true, true, true, ... — must NOT eject.
	var tickIdx int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		idx := atomic.AddInt32(&tickIdx, 1)
		return idx >= 3 // first two false, then true forever
	})

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
		panic("osExit invoked unexpectedly")
	})

	fc := &daemonFakeCommander{}
	deps := makeDeps(t, dir, fc)
	deps.TickerPeriod = 1 * time.Millisecond
	deps.LastSaveAt = time.Now() // gap=false → no tick body work either way

	// Bound the loop so we let many ticks fire and then cancel.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	t.Cleanup(cancel)

	// Replace shutdown with a no-op so ctx-cancel exits cleanly.
	withDaemonShutdownFuncFake(t, func(_ *daemonDeps) error { return nil })

	if err := defaultDaemonRun(ctx, deps); err != nil {
		t.Fatalf("defaultDaemonRun returned: %v", err)
	}

	if exitCalls != 0 {
		t.Errorf("osExit invoked %d times despite reset; want 0", exitCalls)
	}
	// Sanity: probe ran enough times to have triggered an eject if the
	// counter hadn't reset.
	if got := atomic.LoadInt32(&tickIdx); got < int32(selfSupervisionHysteresisTicks)+2 {
		t.Errorf("probe invoked %d times; want at least %d to make the test meaningful",
			got, selfSupervisionHysteresisTicks+2)
	}
}

// TestDaemonLoop_SelfCheckEjectsExactlyOnNthFalse asserts that exactly N
// consecutive false probes trigger eject. The probe records each call and
// returns false N times; the eject must fire on the N-th probe, not earlier
// and not later.
func TestDaemonLoop_SelfCheckEjectsExactlyOnNthFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	var probeCalls int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		atomic.AddInt32(&probeCalls, 1)
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
	if got := atomic.LoadInt32(&probeCalls); got != int32(selfSupervisionHysteresisTicks) {
		t.Errorf("probe invoked %d times before eject; want exactly %d",
			got, selfSupervisionHysteresisTicks)
	}
}

// TestDaemonLoop_SelfCheckResetOnEachTrue asserts the spec's reset semantics:
// the counter resets to 0 (not decrement) on every probe-true. Sequence:
// false × (N-1), true, false × (N-1), true, false × N → eject only on the
// final N-th consecutive false.
func TestDaemonLoop_SelfCheckResetOnEachTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	// Pre-compute the script.
	N := selfSupervisionHysteresisTicks
	// pattern: (false × N-1, true) × 2, then false × N → eject on final false
	script := make([]bool, 0, 3*N)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for i := 0; i < N-1; i++ {
		script = append(script, false)
	}
	script = append(script, true)
	for i := 0; i < N; i++ {
		script = append(script, false)
	}

	var probeCalls int32
	withSaverMembershipProbeFake(t, func(_ *tmux.Client, _ int) bool {
		idx := atomic.AddInt32(&probeCalls, 1)
		i := int(idx) - 1
		if i >= len(script) {
			// After the script, return true so we don't accidentally eject
			// past the planned event.
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
	// Eject should occur on the final consecutive false at script index
	// 2*N − 1 (1-based: 2*N), so probeCalls should equal len(script) when the
	// eject fires (modulo the final extra-true tail we never reach).
	gotProbes := atomic.LoadInt32(&probeCalls)
	wantProbes := int32(len(script))
	if gotProbes != wantProbes {
		t.Errorf("probe invoked %d times; want %d (reset must happen on each true)",
			gotProbes, wantProbes)
	}
}

// TestDaemonLoop_SelfCheckLogsInfoOnEject asserts that the cataloged INFO log
// entry is emitted under ComponentDaemon at the hysteresis trip: the
// "self-eject" event with ticks (consecutive-absence count) and threshold
// (the configured ejection threshold) attrs. (Task 5-10 replaced the ad-hoc
// "self-supervision: saver-membership lost, exiting" line with this cataloged
// event.)
func TestDaemonLoop_SelfCheckLogsInfoOnEject(t *testing.T) {
	// INFO is below the default WARN threshold; bump explicitly.
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
	// The legacy ad-hoc line must be gone.
	if strings.Contains(got, "self-supervision: saver-membership lost") {
		t.Errorf("legacy self-supervision INFO line still present; got:\n%s", got)
	}
	// The consecutive-tick count rides the ticks attr; the configured threshold
	// rides the threshold attr.
	want := fmt.Sprintf("ticks=%d", selfSupervisionHysteresisTicks)
	if !strings.Contains(got, want) {
		t.Errorf("expected consecutive-count %q in log; got:\n%s", want, got)
	}
	wantThreshold := fmt.Sprintf("threshold=%d", selfSupervisionHysteresisTicks)
	if !strings.Contains(got, wantThreshold) {
		t.Errorf("expected threshold %q in log; got:\n%s", wantThreshold, got)
	}
}

// scriptedProbe returns a saverMembershipProbe stub backed by the given bool
// sequence. The first call returns script[0], the second script[1], and so on.
// After the script is exhausted the stub returns true (steady-state legitimate
// daemon) — this is deliberate, NOT silent underrun: ctx-cancel rather than
// script exhaustion is the loop-termination mechanism for the reset-invariant
// tests, so the daemon must keep ticking without ejecting after the planned
// pattern completes.
//
// Tests assert a minimum probe-call count to guarantee the planned pattern was
// fully exercised; the post-pattern steady-state tail is unbounded.
func scriptedProbe(script []bool) (probe func(*tmux.Client, int) bool, calls func() int32) {
	var n int32
	probe = func(_ *tmux.Client, _ int) bool {
		idx := atomic.AddInt32(&n, 1)
		i := int(idx) - 1
		if i >= len(script) {
			return true
		}
		return script[i]
	}
	calls = func() int32 { return atomic.LoadInt32(&n) }
	return probe, calls
}

// runDaemonUntilCancel runs defaultDaemonRun synchronously after installing a
// no-op shutdown fake and a panicking osExit fake. Returns the number of
// osExit invocations recorded (must be 0 for the reset-invariant tests).
//
// The osExit fake panics so any eject immediately unwinds the loop and the
// surrounding recover() converts the panic into a test-visible exitCalls > 0.
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

// TestSelfSupervisionCounter_ResetsFullyOnFirstProbeTrue pins the reset (not
// decrement) invariant: after k = N-1 consecutive probe-false ticks, a single
// probe-true must fully reset the counter to 0 — so the next N-1 probe-false
// ticks also do NOT eject. A buggy `counter--` implementation would pass the
// canonical eject tests but fail this one (after k=N-1 + 1 true, counter would
// be N-2 instead of 0, and a further N-1 falses would push it to 2N-3 ≥ N → eject).
func TestSelfSupervisionCounter_ResetsFullyOnFirstProbeTrue(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// Script: (false × N-1, true, false × N-1). After the script is
	// exhausted the stub returns true (steady-state) so the daemon keeps
	// ticking until ctx-cancel. Under a buggy decrement impl, after the
	// final false segment the counter would be (N-1)-1 + (N-1) = 2N-3 ≥ N
	// (for N ≥ 3) → eject. Under correct reset semantics, counter resets
	// to 0 on the true, then climbs only to N-1 → no eject.
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
	deps.LastSaveAt = time.Now() // gap=false; tick body is fast-path no-op

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	exitCalls := runDaemonUntilCancel(t, deps, ctx)

	if exitCalls != 0 {
		t.Fatalf("osExit invoked %d times; counter-reset invariant violated", exitCalls)
	}
	// Ensure the load-bearing segment ran: probe must have fired at least
	// 2N-1 times (the full script). Without this guard a too-short ctx
	// could let the test pass without exercising the reset.
	if got := probeCalls(); got < int32(2*N-1) {
		t.Fatalf("probe invoked %d times; want ≥ %d to exercise full reset script",
			got, 2*N-1)
	}
}

// TestSelfSupervisionCounter_BoundaryKEqualsNMinus1 exercises the exact
// boundary case: k = N-1 absent ticks then 1 present, repeated for many
// cycles. The daemon must never exit. Pins the spec's "no false-positive
// exit on legitimate transient" invariant at the worst-case threshold.
func TestSelfSupervisionCounter_BoundaryKEqualsNMinus1(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks
	const cycles = 5

	// Build (false × N-1, true) × cycles. Post-script the stub returns
	// true (steady-state) until ctx-cancel.
	script := make([]bool, 0, cycles*N)
	for c := 0; c < cycles; c++ {
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

// TestSelfSupervisionCounter_ManyAbsentPresentCycles compounds the boundary
// test: many short absent-present cycles with varying absent-streak lengths,
// none reaching N. Counter must reset on every present, daemon must never exit.
func TestSelfSupervisionCounter_ManyAbsentPresentCycles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// Vary the absent-streak length per cycle: 1, 2, ..., N-1, repeated
	// `rounds` times. Post-script the stub returns true until ctx-cancel.
	const rounds = 5
	script := make([]bool, 0, 64)
	for r := 0; r < rounds; r++ {
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
	// At least one full round must have completed for the test to be meaningful.
	minCallsForOneRound := 0
	for k := 1; k <= N-1; k++ {
		minCallsForOneRound += k + 1
	}
	if got := probeCalls(); got < int32(minCallsForOneRound) {
		t.Fatalf("probe invoked %d times; want ≥ %d to cover one full round",
			got, minCallsForOneRound)
	}
}

// TestSelfSupervisionCounter_IncrementsUniformlyOnProbeFalse pins the
// uniform-increment invariant: the counter increments on every probe-false
// regardless of cause (the seam returns a single bool — the daemon cannot
// and must not discriminate between absence sub-types). Verified indirectly
// by asserting that N consecutive falses always eject after exactly N probe
// calls into the final segment, independent of which false-positions came
// before. A buggy "only count contiguous-from-zero" or "reset on first false"
// impl would produce a different total probe count.
func TestSelfSupervisionCounter_IncrementsUniformlyOnProbeFalse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)

	N := selfSupervisionHysteresisTicks

	// Prelude pattern (false, true) × P guarantees the counter is exactly 0
	// entering the final segment (every false bumps to 1, every true resets
	// to 0). Then N consecutive falses must push the counter to N and
	// trigger eject on probe call number 2*P + N.
	//
	// A buggy "only count contiguous-from-zero" or any non-uniform increment
	// would yield a different total probe count.
	const preludePairs = 3
	script := make([]bool, 0, 2*preludePairs+N)
	for p := 0; p < preludePairs; p++ {
		script = append(script, false, true)
	}
	for i := 0; i < N; i++ {
		script = append(script, false)
	}

	probe, probeCalls := scriptedProbe(script)
	withSaverMembershipProbeFake(t, probe)

	var exitCalls int32
	withOsExitFake(t, func(_ int) {
		atomic.AddInt32(&exitCalls, 1)
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

	if got := atomic.LoadInt32(&exitCalls); got != 1 {
		t.Fatalf("osExit invoked %d times; want exactly 1", got)
	}
	wantCalls := int32(2*preludePairs + N)
	if got := probeCalls(); got != wantCalls {
		t.Errorf("probe invoked %d times before eject; want %d (uniform increment + reset-on-true)",
			got, wantCalls)
	}
}

// TestSelfSupervisionHysteresisTicks_LowerBound is the spec-mandated
// deliberately-weak guard against accidental zeroing of the hysteresis
// constant (spec § Component D: "A unit test asserts
// selfSupervisionHysteresisTicks >= 1 to prevent accidental zeroing").
// The stronger clamp envelope (3 ≤ N ≤ 9) is asserted separately by
// TestSelfSupervisionHysteresisTicks_ClampInvariant; this test exists
// as a distinct cheap floor so future tuning decisions can relax the
// clamp without removing the load-bearing >= 1 invariant.
func TestSelfSupervisionHysteresisTicks_LowerBound(t *testing.T) {
	if selfSupervisionHysteresisTicks < 1 {
		t.Fatalf("selfSupervisionHysteresisTicks must be >= 1, got %d", selfSupervisionHysteresisTicks)
	}
}
