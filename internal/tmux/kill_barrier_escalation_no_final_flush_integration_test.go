//go:build integration

package tmux_test

// Real-tmux integration test for spec § Component A acceptance bullet 4:
// the "no final-flush GC cycle on escalation-killed orphans" invariant.
//
// Component A escalates the kill-barrier to SIGKILL on a recorded prior
// PID that survived the session-kill poll, deliberately bypassing the
// daemon's signal handler (`defaultShutdownFlush`) so the orphan does
// NOT execute one more captureAndCommit / gcOrphanScrollback cycle on
// its way out. The spec's verifiable shape: snapshot the scrollback
// directory immediately before SIGKILL and again 200 ms after the
// orphan exits — the two snapshots must be byte-equal across all five
// fingerprint fields (existence, size, mtime ns, ctime ns, SHA-256 for
// files ≤ 1 MiB; lstat semantics for symlinks).
//
// Test choreography (mirrors the "divergent-view orphan" scenario the
// spec calls out — orphan is a live `portal state daemon` writing to
// the test's state directory but is NOT the saver pane process):
//
//  1. tmuxtest.SkipIfNoTmux + isolated state dir via
//     portaltest.IsolateStateForTest (which folds in the host-noise
//     scrub — HOME=<tempdir>, XDG_CONFIG_HOME="" — before its
//     pre-snapshot). PORTAL_STATE_DIR is pushed to the test process
//     env so any subprocess we spawn inherits it.
//  2. Stand up an isolated tmux server via tmuxtest.New. Create a
//     regular `work` session so the daemon's captureAndCommit has at
//     least one pane to enumerate (otherwise the daemon ticks but
//     writes no scrollback and the precondition for the equality
//     assertion fails as "snapshot never taken").
//  3. Spawn the orphan: `portal state daemon` with PORTAL_STATE_DIR =
//     stateDir. Because no `_portal-saver` session exists on this
//     server, the orphan's per-tick saver-membership probe
//     (defaultSaverMembershipProbe → c.HasSession) returns false on
//     every tick — the orphan is structurally a divergent-view daemon.
//     It will self-eject after selfSupervisionHysteresisTicks (3) ticks
//     ≈ 3 s; the test must complete its snapshot + SIGKILL within that
//     window.
//  4. Touch save.requested under stateDir so the orphan's tick captures
//     immediately (instead of waiting up to maxGap = 30 s).
//  5. Poll until ≥1 .bin appears in <stateDir>/scrollback/, bounded
//     3 s. On timeout the test fails loudly ("snapshot never taken")
//     so it can never silently pass against an empty pre-snapshot.
//  6. Snapshot the scrollback directory via portaltest.SnapshotStateDir.
//  7. syscall.Kill(orphan.PID, syscall.SIGKILL).
//  8. Poll syscall.Kill(orphan.PID, 0) until ESRCH (orphan reaped),
//     bounded 3 s.
//  9. time.Sleep(200 * time.Millisecond) — the no-final-flush window
//     the spec demands the post-exit snapshot is taken in.
// 10. Snapshot the scrollback directory again.
// 11. Assert byte-equal across the two maps: same keys, same field-
//     by-field Fingerprint per key. On any delta, print a diagnostic
//     citing the key and the field(s) that differ, both values.
//
// Why no _portal-saver session is created: the orphan's view is
// already divergent by construction (no saver to be bound to). The
// "divergent view" branch in the spec is exactly what this test
// mirrors — a `portal state daemon` writing to the state dir while
// NOT being the saver pane process. Creating a sibling _portal-saver
// session with a placeholder would not change the orphan's
// pid-mismatch outcome (probe still returns false) but would
// introduce noise into the equality assertion (the placeholder's
// daemon, were one to start, would also write — but no daemon is
// the placeholder, it's `tail -f /dev/null`, so this is academic).
// The minimal divergent-view shape is the cleanest fixture.
//
// Race window: from orphan-Start to self-eject is ≈ 3 s. The test
// budget — snapshot + SIGKILL + ESRCH poll — must fit comfortably
// inside that window. Empirical timing (darwin/arm64, tmux 3.6b):
// first .bin appears ~1.0–1.2 s post-Start; SnapshotStateDir of a
// scrollback dir with one .bin takes <5 ms; SIGKILL → ESRCH takes
// <50 ms; 200 ms sleep is the dominant cost. Comfortable margin.
//
// If the orphan self-ejects before our SIGKILL syscall lands (e.g.
// the host is under load and the 3-tick hysteresis fires before
// step 7), the SIGKILL returns ESRCH and the post-exit snapshot is
// still valid — the spec's invariant is "no final-flush after
// escalation kill"; a self-eject is also a no-final-flush path
// (cmd/state_daemon.go's osExit(0) call bypasses
// daemonShutdownFunc just as SIGKILL bypasses the signal handler).
// The equality assertion holds either way; the test diagnostic
// includes the SIGKILL errno so a self-eject is observable in the
// run log without changing the pass/fail outcome.
//
// No t.Parallel: the cmd-package convention applies here too —
// mock-injection via package-level mutable state cleaned up by
// t.Cleanup.

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

// scrollbackEmergenceTimeout bounds the poll window for the orphan
// to write at least one .bin file under <stateDir>/scrollback/.
// Sized to cover daemon cold-start + ticker.NewTicker first-fire
// (1 s) + captureAndCommit wall time with margin on slow CI.
const scrollbackEmergenceTimeout = 3 * time.Second

// scrollbackEmergencePollTick is the cadence at which the .bin-
// emergence poll re-scans the scrollback dir. 50 ms matches the
// existing readiness-barrier cadence so the test composes cleanly
// with the rest of the integration-test suite.
const scrollbackEmergencePollTick = 50 * time.Millisecond

// orphanExitTimeout bounds the wait for the SIGKILLed orphan to
// reach an ESRCH-yielding state via syscall.Kill(pid, 0). The
// process is unreaped (we never call Wait until cleanup, see below),
// but kill(pid, 0) on a defunct unreaped child still succeeds on
// most platforms until the child is waited; therefore the
// "process gone" semantics we rely on is the kernel having actually
// torn the process down. Concretely we wait until syscall.Kill
// returns syscall.ESRCH.
const orphanExitTimeout = 3 * time.Second

// orphanExitPollTick mirrors scrollbackEmergencePollTick for the
// same reasons — short enough to observe sub-second SIGKILL-to-exit
// latency without busy-spinning.
const orphanExitPollTick = 20 * time.Millisecond

// postExitSettleWindow is the spec-mandated 200 ms gap between
// "process death observed" and the post-exit snapshot. Any final
// flush the orphan attempted to do would have completed inside
// this window — taking the snapshot here is the load-bearing
// evidence that no such flush occurred.
const postExitSettleWindow = 200 * time.Millisecond

// TestKillBarrierEscalation_NoScrollbackDeltaIn200msPostExit pins
// the Component A acceptance criterion that an escalation-killed
// orphan does not mutate the scrollback directory on the way out.
// See the file-header comment for the full choreography rationale.
func TestKillBarrierEscalation_NoScrollbackDeltaIn200msPostExit(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	sock := tmuxtest.New(t, "ptl-killesc-")
	client := sock.Client()

	// Give the daemon something to capture. Without at least one
	// non-internal session, CaptureStructure enumerates zero panes
	// and the orphan ticks indefinitely without ever writing a .bin
	// — the precondition for the snapshot-equality assertion would
	// then fail as "snapshot never taken" (which is the correct
	// failure shape per spec, but masks the actual invariant under
	// test).
	if err := client.NewDetachedSessionNoCwd(
		"work", "sh -c 'exec tail -f /dev/null'",
	); err != nil {
		t.Fatalf("create work session: %v", err)
	}

	// Spawn the orphan: a `portal state daemon` subprocess whose
	// PORTAL_STATE_DIR points at the isolated state dir. With no
	// `_portal-saver` session on this tmux server, the daemon's
	// saver-membership probe returns false on every tick — the
	// orphan is structurally divergent-view from the moment it
	// starts. It will self-eject after 3 ticks (≈ 3 s).
	//
	// LOAD-FLAKE GUARD: the .bin-emergence window below races that
	// self-eject — under host load the orphan's cold start + first
	// capture tick can lose to the 3-tick hysteresis, in which case
	// the orphan exits before writing anything and the poll times
	// out. That is a scheduling symptom, not the invariant under
	// test, so a self-ejected-before-capture orphan is respawned
	// (bounded attempts). An orphan that is still ALIVE after the
	// window without writing a .bin is a genuine capture failure and
	// fails immediately.
	//
	// Daemon spawn shares the isolated stateDir with the test rather
	// than getting its own (the assertion below reads scrollback
	// under that stateDir), so we cannot use SpawnIsolatedDaemon
	// (which forces a per-call tempdir). The spawn shape mirrors
	// SpawnIsolatedDaemon's body verbatim; reap+SIGKILL cleanup is
	// delegated to RegisterSubprocessCleanup so the load-bearing
	// rationale lives in one place (see portaltest/spawn_daemon.go).
	orphanEnv := append([]string{}, envSlice...)
	orphanEnv = append(orphanEnv, "PORTAL_STATE_DIR="+stateDir)
	// Pin the orphan to the TEST server (overrides the poison TMUX in
	// envSlice; last-wins). Without this the orphan inherited the ambient
	// TMUX and captured the developer's REAL sessions — the root cause of
	// both this test's load-flakiness (a first tick over the real
	// 30-session server blew the 3s emergence window) and a real-system
	// read breach (real scrollback bytes landing in the test state dir).
	orphanEnv = append(orphanEnv, "TMUX="+sock.SocketPath()+",0,0")
	scrollbackDir := state.ScrollbackDir(stateDir)

	const maxOrphanAttempts = 3
	var orphanPID int
	var reaped <-chan struct{}
	for attempt := 1; ; attempt++ {
		orphan := exec.Command("portal", "state", "daemon")
		orphan.Env = orphanEnv
		if err := orphan.Start(); err != nil {
			t.Fatalf("start orphan daemon: %v", err)
		}
		orphanPID = orphan.Process.Pid
		reaped = portaltest.RegisterSubprocessCleanup(t, orphan)

		// Force a tick to capture immediately rather than waiting up
		// to maxGap = 30 s. The daemon's tick loop reads
		// save.requested as a dirty flag and triggers a save on the
		// very next ticker fire when it is present. Re-touched per
		// attempt: the previous orphan may have consumed the flag on
		// a tick that self-ejected before capturing.
		if err := state.TouchSaveRequested(stateDir); err != nil {
			t.Fatalf("touch save.requested: %v", err)
		}

		// Wait for the orphan to write ≥1 .bin under scrollback/.
		// This is the precondition for the equality assertion —
		// without a non-empty pre-snapshot the test would silently
		// green-pass on a no-op orphan, which is the exact failure
		// mode the spec names.
		if tmuxtest.PollUntil(t, scrollbackEmergenceTimeout, scrollbackEmergencePollTick, func() bool {
			return countBinFiles(scrollbackDir) >= 1
		}) {
			break
		}

		select {
		case <-reaped:
			// Orphan exited without capturing — the self-eject won the
			// race (host load). Respawn unless attempts are exhausted.
			if attempt < maxOrphanAttempts {
				t.Logf("attempt %d/%d: orphan %d self-ejected before first capture (host load); respawning",
					attempt, maxOrphanAttempts, orphanPID)
				continue
			}
			t.Fatalf("snapshot never taken — orphan self-ejected before first capture on all %d attempts "+
				"(host under sustained load?)\n  scrollback dir: %s\n  contents: %v",
				maxOrphanAttempts, scrollbackDir, listDirSafe(scrollbackDir))
		default:
			t.Fatalf("snapshot never taken — orphan %d is ALIVE but wrote no scrollback within %s "+
				"(genuine capture failure, not load)\n  scrollback dir: %s\n  contents: %v",
				orphanPID, scrollbackEmergenceTimeout, scrollbackDir, listDirSafe(scrollbackDir))
		}
	}

	// LOAD-BEARING STEP 1: pre-SIGKILL snapshot. Captured at the
	// caller-chosen point in time (immediately before SIGKILL); a
	// t.Cleanup-driven snapshot cannot achieve this granularity.
	pre, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("pre-SIGKILL snapshot: %v", err)
	}
	if !hasAnyBin(pre) {
		t.Fatalf("snapshot never taken — pre-SIGKILL snapshot contained no .bin entries\n"+
			"  scrollback dir: %s\n"+
			"  pre keys: %v",
			scrollbackDir, slices.Sorted(maps.Keys(pre)))
	}

	// LOAD-BEARING STEP 2: SIGKILL the orphan. Direct kill(2) syscall
	// — no SIGTERM first, no waiting on signal handlers. This is the
	// production escalation primitive (cmd/bootstrap → Component A's
	// escalateKillToSIGKILL → killBarrierSendSIGKILL → syscall.Kill).
	// SIGKILL is delivered atomically by the kernel; the orphan's
	// signal handler (defaultShutdownFlush) is bypassed by the
	// kernel itself, which is the structural guarantee under test.
	killErr := syscall.Kill(orphanPID, syscall.SIGKILL)
	// We tolerate ESRCH here: if the orphan self-ejected between
	// the .bin poll and this syscall the equality invariant still
	// holds (osExit(0) also bypasses defaultShutdownFlush). Other
	// errors are diagnostic noise only — observability, not gating.
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		t.Logf("SIGKILL syscall returned %v (proceeding; orphan may have already exited)", killErr)
	}

	// Wait for the reaper goroutine to observe the process exit AND
	// poll syscall.Kill(pid, 0) until it returns ESRCH — the kernel's
	// canonical "process does not exist" signal. Reaping happens
	// first so kill(pid, 0) is not racing against an unreaped zombie
	// (which would return 0, not ESRCH, indefinitely).
	select {
	case <-reaped:
	case <-time.After(orphanExitTimeout):
		t.Fatalf("orphan PID %d was not reaped within %s; "+
			"the no-final-flush window cannot be timed from process death",
			orphanPID, orphanExitTimeout)
	}
	exited := tmuxtest.PollUntil(t, orphanExitTimeout, orphanExitPollTick, func() bool {
		err := syscall.Kill(orphanPID, 0)
		return errors.Is(err, syscall.ESRCH)
	})
	if !exited {
		t.Fatalf("orphan PID %d did not reach ESRCH within %s after reap; "+
			"the no-final-flush window cannot be timed from process death",
			orphanPID, orphanExitTimeout)
	}

	// LOAD-BEARING STEP 3: spec-mandated 200 ms settle window. Any
	// final flush the orphan attempted would have completed inside
	// this window — taking the snapshot here is the load-bearing
	// evidence that no such flush occurred.
	time.Sleep(postExitSettleWindow)

	// LOAD-BEARING STEP 4: post-exit snapshot.
	post, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("post-exit snapshot: %v", err)
	}

	// EQUALITY ASSERTION via portaltest.DiffFingerprints: every key
	// in pre must be present in post with byte-equal Fingerprint.
	if deltas := portaltest.DiffFingerprints(pre, post); len(deltas) > 0 {
		lines := make([]string, len(deltas))
		for i, d := range deltas {
			lines[i] = "  " + portaltest.FormatDelta(d)
		}
		t.Fatalf("scrollback dir mutated between pre-SIGKILL snapshot and "+
			"200 ms post-exit snapshot (spec § Component A: no final-flush "+
			"GC cycle on escalation-killed orphans)\n"+
			"  scrollback dir: %s\n"+
			"  pre keys (%d): %v\n"+
			"  post keys (%d): %v\n"+
			"  delta(s):\n%s",
			scrollbackDir, len(pre), slices.Sorted(maps.Keys(pre)), len(post), slices.Sorted(maps.Keys(post)),
			strings.Join(lines, "\n"))
	}
}

// countBinFiles returns the number of regular .bin files directly
// under dir. Directories and non-.bin entries are ignored. A
// missing dir yields 0 — the orphan may not have created
// scrollback/ yet on its first tick.
func countBinFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.Type().IsRegular() && filepath.Ext(e.Name()) == ".bin" {
			n++
		}
	}
	return n
}

// listDirSafe returns directory entries for failure diagnostics.
// Missing dirs yield a stub message rather than an error so the
// caller's t.Fatalf format string stays readable.
func listDirSafe(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []string{fmt.Sprintf("(ReadDir failed: %v)", err)}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// hasAnyBin reports whether snap contains at least one .bin entry.
// Used to guard the pre-snapshot precondition: an empty snapshot
// would silently green-pass the equality assertion.
func hasAnyBin(snap map[string]portaltest.Fingerprint) bool {
	for k := range snap {
		if filepath.Ext(k) == ".bin" {
			return true
		}
	}
	return false
}
