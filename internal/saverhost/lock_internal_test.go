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
	up := CreateCeiling + RunGrace + JudgeRun + ReadyWait + ReadCeiling
	const margin = 15 * time.Second

	if run := settle + stop + up; LockWait < run+margin {
		t.Errorf("LockWait = %s, want at least a holder's longest run, %s, with %s over", LockWait, run, margin)
	}
}

// The bootstrap judges the saver it makes within ensure's hold of the lock,
// and ends one Portal can't use with zmx kill and then, when that fails, the
// proved signal.
func TestJudgeRunCoversTheEndOfASaverMade(t *testing.T) {
	if judge := 2*ReadCeiling + 2*KillCeiling + RunGrace; JudgeRun < judge {
		t.Errorf("JudgeRun = %s, want at least two reads, a zmx kill with its run's grace and a signalled end's wait, %s", JudgeRun, judge)
	}
}
