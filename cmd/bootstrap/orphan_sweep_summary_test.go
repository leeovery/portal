package bootstrap

import (
	"errors"
	"log/slog"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
)

func TestSweepOrphanDaemons_EmitsCleanSummaryCountingSuccessfulKills(t *testing.T) {
	sink := logtest.Install(t)
	f := &sweepFakes{}

	run(t, f.sweep([]int{2001, 2002}, f.saver(0, false, nil), log.For("bootstrap")))

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if rec.Level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "killed"); got != 2 {
		t.Errorf("killed = %d, want 2", got)
	}
	rec.RequireDuration(t, "took")
}

func TestSweepOrphanDaemons_LogsEachKillAtDebugUnderClean(t *testing.T) {
	sink := logtest.Install(t)
	f := &sweepFakes{}

	run(t, f.sweep([]int{2001}, f.saver(0, false, nil), log.For("bootstrap")))

	if infos := sink.Records().WithMessage("orphan killed").AtExactLevel(slog.LevelInfo); infos != nil {
		t.Errorf("per-kill lines at INFO: %+v", infos)
	}
	dbg := sink.Records().Matching("clean", "orphan killed").AtExactLevel(slog.LevelDebug).Only(t, "DEBUG 'orphan killed' under clean")
	if pid := dbg.IntAttr(t, "target_pid"); pid != 2001 {
		t.Errorf("DEBUG 'orphan killed' target_pid = %d, want 2001", pid)
	}
}

func TestSweepOrphanDaemons_ExcludesSkippedAndFailedFromKilled(t *testing.T) {
	sink := logtest.Install(t)
	f := &sweepFakes{
		failed:     map[int]error{3001: errNotTheSaversDaemon},
		signalErrs: map[int]error{3002: syscall.ESRCH},
		exitErrs:   map[int]error{3004: errors.New("pid 3004 still hadn't finished exiting 2s later")},
	}

	run(t, f.sweep([]int{3001, 3002, 3003, 3004}, f.saver(0, false, nil), log.For("bootstrap")))

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if got := rec.IntAttr(t, "killed"); got != 1 {
		t.Errorf("killed = %d, want 1 (excludes the skip, the failed kill and the orphan still exiting)", got)
	}
}

func TestSweepOrphanDaemons_CountsZeroWhenPgrepFindsNothing(t *testing.T) {
	sink := logtest.Install(t)
	f := &sweepFakes{}

	run(t, f.sweep(nil, f.saver(0, false, nil), log.For("bootstrap")))

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if got := rec.IntAttr(t, "killed"); got != 0 {
		t.Errorf("killed = %d, want 0", got)
	}
}

func TestSweepOrphanDaemons_NoSummaryWhenItCannotJudge(t *testing.T) {
	for name, c := range map[string]func(f *sweepFakes) *OrphanSweepCore{
		"pgrep failing": func(f *sweepFakes) *OrphanSweepCore {
			c := f.sweep(nil, f.saver(0, false, nil), log.For("bootstrap"))
			c.Pgrep = func() ([]int, error) { return nil, errors.New("pgrep boom") }
			return c
		},
		"the saver unreadable": func(f *sweepFakes) *OrphanSweepCore {
			return f.sweep([]int{4001}, f.saver(0, false, errors.New("no answer")), log.For("bootstrap"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			sink := logtest.Install(t)

			run(t, c(&sweepFakes{}))

			if got := sink.Records().Matching("clean", "orphan-daemon sweep complete"); len(got) != 0 {
				t.Errorf("summaries = %+v, want none: the sweep judged nothing", got)
			}
		})
	}
}
