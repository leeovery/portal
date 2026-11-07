package portaltest

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/state"
)

// Generous: a healthy teardown exits after one or two polls.
const teardownGuardBudget = 3 * time.Second

const teardownGuardPollTick = 50 * time.Millisecond

// SaverPIDSource yields the pid of the process the teardown wait must outlive,
// or ok=false when there is none to wait on.
type SaverPIDSource func() (pid int, ok bool)

// cleanupT narrows *testing.T to the one method the guard registers through, so
// the wait itself can be driven without ending a test.
type cleanupT interface {
	Cleanup(fn func())
}

// RegisterStateDirTeardownGuard registers a bounded cleanup wait for stateDir's
// writers to finish, so a daemon's shutdown flush or a straggler hook subprocess
// cannot land a file mid-RemoveAll and fail the test with "directory not empty".
// The daemon it waits on is the one named by <stateDir>/daemon.pid.
// Call order is load-bearing: register after the state dir exists and before
// zmxtest.New (or, in migrate's tests, tmuxtest.New), so LIFO cleanup runs this
// after the sessions are killed and before the RemoveAll it protects.
func RegisterStateDirTeardownGuard(t *testing.T, stateDir string) {
	t.Helper()
	registerTeardownGuard(t, stateDir, func() (int, bool) {
		pid, err := state.ReadPIDFile(stateDir)
		return pid, err == nil && pid > 0
	})
}

// RegisterStateDirTeardownGuardWithPIDSource is RegisterStateDirTeardownGuard
// for a fixture whose <stateDir>/daemon.pid does not name the daemon the wait
// must outlive — one that overwrites the file, or hosts a saver that never
// wrote it — taking that pid from saverPID instead. The same call-order rule
// applies, and saverPID is read during cleanup, after the sessions are killed.
func RegisterStateDirTeardownGuardWithPIDSource(t *testing.T, stateDir string, saverPID SaverPIDSource) {
	t.Helper()
	registerTeardownGuard(t, stateDir, saverPID)
}

func registerTeardownGuard(t cleanupT, stateDir string, saverPID SaverPIDSource) {
	t.Cleanup(func() {
		deadline := time.Now().Add(teardownGuardBudget)

		// The saver daemon's shutdown flush is the biggest writer.
		if pid, ok := saverPID(); ok {
			for state.IsProcessAlive(pid) && time.Now().Before(deadline) {
				time.Sleep(teardownGuardPollTick)
			}
		}

		awaitDirQuiescent(stateDir, deadline)
	})
}

// A var so a test can observe the directory IsolateStateForTest registers over.
var registerHomeQuiescenceGuard = registerDirQuiescenceGuard

// registerDirQuiescenceGuard registers a bounded cleanup wait for dir's writers
// to finish. A test's isolated HOME is one: shells outliving their killed
// sessions flush per-session files into it, and one landing mid-RemoveAll fails
// the test with "directory not empty" after its assertions passed.
func registerDirQuiescenceGuard(t cleanupT, dir string) {
	t.Cleanup(func() {
		awaitDirQuiescent(dir, time.Now().Add(teardownGuardBudget))
	})
}

// Two identical snapshots mean nothing landed for a full tick. On deadline
// exhaustion just return and let RemoveAll take its chances.
func awaitDirQuiescent(dir string, deadline time.Time) {
	prev := dirShapeSnapshot(dir)
	for time.Now().Before(deadline) {
		time.Sleep(teardownGuardPollTick)
		cur := dirShapeSnapshot(dir)
		if cur == prev {
			return
		}
		prev = cur
	}
}

// Errors are folded into the string, so two identical error states also count as
// quiescent.
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
