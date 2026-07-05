package portaltest

// teardown_guard.go — bounded wait for a test state dir's writers to go
// quiet before t.TempDir's RemoveAll runs.
//
// The race it closes: tmuxtest.New's kill-server cleanup SIGHUPs the
// test's `_portal-saver` pane, whose daemon then performs its graceful
// shutdown flush (correct product behaviour) — and session-closed hooks
// can still be fork-execing `portal state commit-now` subprocesses.
// Either writer landing a file mid-RemoveAll fails the test with
// "TempDir RemoveAll cleanup: ... directory not empty" — a teardown
// flake, not a product defect. The composite bootstrap harness fixed the
// daemon half of this inline (setupCompositeHarness); this helper is the
// shared, hook-subprocess-aware version for the real-binary fixtures.

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// teardownGuardBudget bounds the total wait. Generous relative to the
// observed writer lifetimes (daemon flush < 1s, hook subprocesses
// < 500ms) — on a healthy teardown the guard exits in one or two polls.
const teardownGuardBudget = 3 * time.Second

// teardownGuardPollTick is the poll cadence for both the daemon-exit
// wait and the quiescence check.
const teardownGuardPollTick = 50 * time.Millisecond

// RegisterStateDirTeardownGuard registers a t.Cleanup that waits
// (bounded by teardownGuardBudget) for stateDir's writers to finish:
// first for the process recorded in <stateDir>/daemon.pid to exit, then
// for two consecutive identical directory snapshots (names+sizes+mtimes)
// so straggler hook subprocesses have landed their last write.
//
// CALL ORDER IS LOAD-BEARING: register AFTER the stateDir's TempDir
// exists (i.e. after IsolateStateForTest) and BEFORE tmuxtest.New, so
// LIFO cleanup ordering runs it after kill-server has SIGHUP'd the
// saver pane and before the TempDir RemoveAll it protects.
func RegisterStateDirTeardownGuard(t *testing.T, stateDir string) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(teardownGuardBudget)

		// Phase 1: the recorded saver daemon (if any) must exit — its
		// graceful SIGHUP shutdown flush is the biggest writer.
		if pid, err := state.ReadPIDFile(stateDir); err == nil && pid > 0 {
			for state.IsProcessAlive(pid) && time.Now().Before(deadline) {
				time.Sleep(teardownGuardPollTick)
			}
		}

		// Phase 2: quiescence — two consecutive identical shape
		// snapshots mean no writer landed anything for a full tick
		// (covers session-closed hook subprocesses kill-server may have
		// fork-exec'd). Best-effort: on budget exhaustion just return
		// and let RemoveAll take its chances (same as before the guard).
		prev := dirShapeSnapshot(stateDir)
		for time.Now().Before(deadline) {
			time.Sleep(teardownGuardPollTick)
			cur := dirShapeSnapshot(stateDir)
			if cur == prev {
				return
			}
			prev = cur
		}
	})
}

// dirShapeSnapshot renders a recursive name+size+mtime listing of dir as
// a single comparable string. Errors and missing entries render into the
// string (two identical error states also count as quiescent, which is
// the correct teardown semantic).
func dirShapeSnapshot(dir string) string {
	var b strings.Builder
	var walk func(string)
	walk = func(d string) {
		entries, err := os.ReadDir(d)
		if err != nil {
			fmt.Fprintf(&b, "%s!%v\n", d, err)
			return
		}
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				fmt.Fprintf(&b, "%s/%s!%v\n", d, e.Name(), err)
				continue
			}
			fmt.Fprintf(&b, "%s/%s|%d|%d\n", d, e.Name(), info.Size(), info.ModTime().UnixNano())
			if e.IsDir() {
				walk(d + "/" + e.Name())
			}
		}
	}
	walk(dir)
	return b.String()
}
