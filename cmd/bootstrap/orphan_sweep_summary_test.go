package bootstrap

import (
	"errors"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/log"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

func TestSweepOrphanDaemons_EmitsCleanSummaryCountingSuccessfulKills(t *testing.T) {
	sink := logtest.Install(t)
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{2001, 2002}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if rec.Level != slog.LevelInfo {
		t.Errorf("summary level = %v, want INFO", rec.Level)
	}
	if got := rec.IntAttr(t, "killed"); got != 2 {
		t.Errorf("killed = %d, want 2", got)
	}
	rec.RequireDuration(t, "took")
}

func TestSweepOrphanDaemons_DemotesPerKillInfoToDebug(t *testing.T) {
	sink := logtest.Install(t)
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyIsPortalDaemon}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{2001}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	for _, r := range sink.Records() {
		if r.Level == slog.LevelInfo && r.Msg == "orphan killed" {
			t.Errorf("per-kill line must not be INFO: %+v", r)
		}
		if r.Level == slog.LevelInfo && r.Msg == "sweep: killed orphan daemon" {
			t.Errorf("old per-kill INFO message must be gone: %+v", r)
		}
	}
	dbg := sink.Records().Matching("clean", "orphan killed").AtExactLevel(slog.LevelDebug).Only(t, "DEBUG 'orphan killed' under clean")
	if pid := dbg.IntAttr(t, "target_pid"); pid != 2001 {
		t.Errorf("DEBUG 'orphan killed' target_pid = %d, want 2001", pid)
	}
}

func TestSweepOrphanDaemons_ExcludesSkippedAndFailedFromKilled(t *testing.T) {
	sink := logtest.Install(t)
	identify := &recordingIdentify{
		results: map[int]identifyOutcome{
			3001: {res: state.IdentifyNotPortalDaemon},
			3002: {res: state.IdentifyIsPortalDaemon},
			3003: {res: state.IdentifyIsPortalDaemon},
		},
	}
	kill := &recordingKill{errs: map[int]error{3002: errors.New("kill: no such process")}}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{3001, 3002, 3003}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if got := rec.IntAttr(t, "killed"); got != 1 {
		t.Errorf("killed = %d, want 1 (excludes skip + failed kill)", got)
	}

	_ = sink.Records().Matching("bootstrap", "sweep: pid not identity-checked as portal daemon, skipping").AtExactLevel(slog.LevelDebug).
		Only(t, "identity-skip DEBUG")
	_ = sink.Records().Matching("bootstrap", "sweep: kill failed").AtExactLevel(slog.LevelWarn).Only(t, "kill-failure WARN")
}

func TestSweepOrphanDaemons_NoSummaryWhenPgrepFails(t *testing.T) {
	sink := logtest.Install(t)
	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return nil, errors.New("pgrep boom") },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, nil },
		Identify:     func(pid int) (state.IdentifyResult, error) { return state.IdentifyIsPortalDaemon, nil },
		Kill:         func(pid int) error { return nil },
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	if got := sink.Records().Matching("clean", "orphan-daemon sweep complete"); len(got) != 0 {
		t.Errorf("expected no summary on pgrep failure (returns before loop), got %d: %+v", len(got), got)
	}
}

func TestSweepOrphanDaemons_SummaryWithZeroKilledWhenSaverPanePIDErrors(t *testing.T) {
	sink := logtest.Install(t)
	identify := &recordingIdentify{def: identifyOutcome{res: state.IdentifyDead}}
	kill := &recordingKill{}

	c := &OrphanSweepCore{
		Pgrep:        func() ([]int, error) { return []int{4001, 4002}, nil },
		SaverPanePID: func() (pid int, present bool, err error) { return 0, false, errors.New("list-panes boom") },
		Identify:     identify.fn,
		Kill:         kill.fn,
		Logger:       log.For("bootstrap"),
	}
	if err := c.SweepOrphanDaemons(); err != nil {
		t.Fatalf("SweepOrphanDaemons returned error: %v", err)
	}

	rec := sink.Records().Matching("clean", "orphan-daemon sweep complete").Only(t, "clean orphan-daemon sweep complete record")
	if got := rec.IntAttr(t, "killed"); got != 0 {
		t.Errorf("killed = %d, want 0", got)
	}
	rec.RequireDuration(t, "took")
}
