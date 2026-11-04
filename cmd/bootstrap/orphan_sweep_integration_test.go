//go:build integration

package bootstrap_test

import (
	"errors"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/portaltest"
)

const recycledPIDSettleWindow = 200 * time.Millisecond

// A pgrep hit can name a pid recycled to an unrelated process by the time the
// sweep reaches it. The production identify must refuse it, so the production
// kill never fires. The candidate is the test's own child, so a failure kills
// nothing the test doesn't own.
func TestSweepOrphanDaemons_Integration_RecycledPIDRefusal(t *testing.T) {
	portaltest.IsolateStateForTest(t)

	sleeper := exec.Command("sh", "-c", "exec sleep 30")
	if err := sleeper.Start(); err != nil {
		t.Fatalf("start sleep process: %v", err)
	}
	sleeperPID := sleeper.Process.Pid
	portaltest.RegisterSubprocessCleanup(t, sleeper)

	core := &bootstrap.OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{sleeperPID}, nil },
		SaverPanePID: func() (int, bool, error) { return 0, false, nil },
	}
	if err := core.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned non-nil error: %v", err)
	}

	time.Sleep(recycledPIDSettleWindow)

	if err := syscall.Kill(sleeperPID, 0); errors.Is(err, syscall.ESRCH) {
		t.Fatalf("sleeper PID %d was killed by SweepOrphanDaemons; the identity check failed to refuse a non-daemon PID", sleeperPID)
	}
}
