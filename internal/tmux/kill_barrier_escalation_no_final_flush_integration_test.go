//go:build integration

package tmux_test

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

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmuxtest"
)

const scrollbackEmergencePollTick = 50 * time.Millisecond

// The scrollback dir fills through observable steps (the daemon's temp files,
// then the renamed .bin), so Stall bounds how long the directory may sit
// unchanged rather than how long the first capture takes. Stall stays short on
// purpose: the orphan self-ejects after ~3 divergent-view ticks, and the caller
// respawns it, so a wait that lingers only delays that retry.
var scrollbackEmergenceWait = harnesstest.ProgressWait{
	Stall:   4 * time.Second,
	Ceiling: 30 * time.Second,
	Tick:    scrollbackEmergencePollTick,
}

const orphanExitTimeout = 3 * time.Second

const orphanExitPollTick = 20 * time.Millisecond

// The orphan is already reaped by the time this runs, so its exit is observable
// within a poll or two; Stall only covers a descheduled probe.
var orphanExitWait = harnesstest.ProgressWait{
	Stall:   orphanExitTimeout,
	Ceiling: 15 * time.Second,
	Tick:    orphanExitPollTick,
}

const postExitSettleWindow = 200 * time.Millisecond

func TestKillBarrierEscalation_NoScrollbackDeltaIn200msPostExit(t *testing.T) {
	tmuxtest.SkipIfNoTmux(t)
	_ = portalbintest.StagePortalBinary(t)

	envSlice, stateDir := portaltest.IsolateStateForTest(t)
	t.Setenv("PORTAL_STATE_DIR", stateDir)

	// LIFO runs this wait between kill-server and the TempDir RemoveAll.
	portaltest.RegisterStateDirTeardownGuard(t, stateDir)

	sock := tmuxtest.New(t, "ptl-killesc-")
	client := sock.Client()

	// The daemon needs one non-internal session to capture; with none it ticks
	// forever without ever writing a .bin, and the snapshot is never taken.
	if err := client.NewDetachedSessionNoCwd(
		"work", "sh -c 'exec tail -f /dev/null'",
	); err != nil {
		t.Fatalf("create work session: %v", err)
	}

	// Spawned by hand rather than through portaltest.SpawnIsolatedDaemon: the
	// orphan must share the test's stateDir, which that helper forces to a
	// per-call tempdir.
	orphanEnv := append([]string{}, envSlice...)
	orphanEnv = append(orphanEnv, "PORTAL_STATE_DIR="+stateDir)
	// Pin the orphan to the TEST server — last-wins over the poisoned TMUX in
	// envSlice. Without it the orphan attaches to the developer's real server
	// and captures their live sessions. The pid names that server as the
	// daemon's own, without which it commits nothing.
	serverPID := strings.TrimSpace(sock.Run(t, "display-message", "-p", "#{pid}"))
	orphanEnv = append(orphanEnv, fmt.Sprintf("TMUX=%s,%s,0", sock.SocketPath(), serverPID))
	scrollbackDir := state.ScrollbackDir(stateDir)

	// The orphan self-ejects after 3 divergent-view ticks (~3s), which under
	// host load can beat its first capture; that case is respawned, while an
	// orphan still alive with no .bin is a genuine capture failure.
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

		// Force a capture on the next tick instead of waiting out maxGap (30s).
		if err := state.TouchSaveRequested(stateDir); err != nil {
			t.Fatalf("touch save.requested: %v", err)
		}

		emergence := harnesstest.AwaitProgress(t, scrollbackEmergenceWait,
			func() scrollbackObservation { return observeScrollback(scrollbackDir) },
			func(o scrollbackObservation) bool { return o.Bins >= 1 })
		if emergence.Reached {
			break
		}

		select {
		case <-reaped:
			if attempt < maxOrphanAttempts {
				t.Logf("attempt %d/%d: orphan %d self-ejected before first capture (host load); respawning",
					attempt, maxOrphanAttempts, orphanPID)
				continue
			}
			t.Fatalf("snapshot never taken — orphan self-ejected before first capture on all %d attempts "+
				"(host under sustained load?)\n  scrollback dir: %s\n  contents: %v",
				maxOrphanAttempts, scrollbackDir, listDirSafe(scrollbackDir))
		default:
			t.Fatalf("snapshot never taken — orphan %d is ALIVE but wrote no scrollback (%s) "+
				"(genuine capture failure, not load)\n  scrollback dir: %s\n  contents: %v",
				orphanPID, emergence, scrollbackDir, listDirSafe(scrollbackDir))
		}
	}

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

	killErr := syscall.Kill(orphanPID, syscall.SIGKILL)
	// ESRCH is tolerated: a self-eject bypasses the shutdown flush just as
	// SIGKILL does, so the invariant still holds.
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		t.Logf("SIGKILL syscall returned %v (proceeding; orphan may have already exited)", killErr)
	}

	// Reap before polling kill(pid, 0): an unreaped zombie answers 0 forever
	// and never ESRCH, so process death could not be timed.
	select {
	case <-reaped:
	case <-time.After(orphanExitTimeout):
		t.Fatalf("orphan PID %d was not reaped within %s; "+
			"the no-final-flush window cannot be timed from process death",
			orphanPID, orphanExitTimeout)
	}
	exited := harnesstest.AwaitProgress(t, orphanExitWait,
		func() bool { return errors.Is(syscall.Kill(orphanPID, 0), syscall.ESRCH) },
		func(esrch bool) bool { return esrch })
	if !exited.Reached {
		t.Fatalf("orphan PID %d did not reach ESRCH after reap (%s); "+
			"the no-final-flush window cannot be timed from process death",
			orphanPID, exited)
	}

	time.Sleep(postExitSettleWindow)

	post, err := portaltest.SnapshotStateDir(scrollbackDir)
	if err != nil {
		t.Fatalf("post-exit snapshot: %v", err)
	}

	if deltas := portaltest.DiffFingerprints(pre, post); len(deltas) > 0 {
		lines := make([]string, len(deltas))
		for i, d := range deltas {
			lines[i] = "  " + portaltest.FormatDelta(d)
		}
		t.Fatalf("scrollback dir mutated between pre-SIGKILL snapshot and "+
			"200 ms post-exit snapshot; an escalation-killed orphan must run "+
			"no final-flush GC cycle\n"+
			"  scrollback dir: %s\n"+
			"  pre keys (%d): %v\n"+
			"  post keys (%d): %v\n"+
			"  delta(s):\n%s",
			scrollbackDir, len(pre), slices.Sorted(maps.Keys(pre)), len(post), slices.Sorted(maps.Keys(post)),
			strings.Join(lines, "\n"))
	}
}

// scrollbackObservation is comparable so the wait can tell a scrollback dir the
// daemon is still filling from one it has stopped touching: Entries moves while
// temp files come and go, Bins is the reading the target is stated against.
type scrollbackObservation struct {
	Bins    int
	Entries int
}

func (o scrollbackObservation) String() string {
	return fmt.Sprintf("bins=%d entries=%d", o.Bins, o.Entries)
}

func observeScrollback(dir string) scrollbackObservation {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return scrollbackObservation{}
	}
	obs := scrollbackObservation{Entries: len(entries)}
	for _, e := range entries {
		if e.Type().IsRegular() && filepath.Ext(e.Name()) == ".bin" {
			obs.Bins++
		}
	}
	return obs
}

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

func hasAnyBin(snap map[string]portaltest.Fingerprint) bool {
	for k := range snap {
		if filepath.Ext(k) == ".bin" {
			return true
		}
	}
	return false
}
