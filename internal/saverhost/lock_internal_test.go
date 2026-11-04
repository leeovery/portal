package saverhost

import (
	"testing"
	"time"
)

// A waiter that gave up while another holder was still within its bounds
// would fail a bootstrap or uninstall that only had to wait.
func TestLockWaitOutlastsAHoldersRun(t *testing.T) {
	settle := ReadyWait + ReadCeiling
	stop := ReadCeiling + TermWait + killWait + ReadCeiling + KillCeiling + ReadCeiling
	up := CreateCeiling + ReadyWait + ReadCeiling
	const margin = 15 * time.Second

	if run := settle + stop + up; LockWait < run+margin {
		t.Errorf("LockWait = %s, want at least a holder's longest run, %s, with %s over", LockWait, run, margin)
	}
}
