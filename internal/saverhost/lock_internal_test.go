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

// The saver's create reads a session that gave no answer within its
// read-back once more, past its ceiling; then the bootstrap judges the saver
// it made within ensure's hold of the lock, and ends one Portal can't use
// with zmx kill and then, when that fails, the proved signal.
func TestJudgeRunCoversTheEndOfASaverMade(t *testing.T) {
	if judge := 2*ReadCeiling + 2*KillCeiling + RunGrace; JudgeRun < judge {
		t.Errorf("JudgeRun = %s, want at least the create's read, the judgement's read, a zmx kill with its run's grace and a signalled end's wait, %s", JudgeRun, judge)
	}
}

// The saver's create, refusing a session of another protocol, reads its
// protocol and ends it with zmx kill in the judge's place.
func TestJudgeRunCoversTheRefusalOfASaverCreated(t *testing.T) {
	if refusal := ReadCeiling + KillCeiling + RunGrace; JudgeRun < refusal {
		t.Errorf("JudgeRun = %s, want at least a read and a zmx kill with its run's grace, %s", JudgeRun, refusal)
	}
}
