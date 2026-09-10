package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/hooksweep"
)

// The diagnostic must answer the reaper's question with the reaper's own
// answer: a verdict the two disagree on is a user told a hook is lost by one
// command and reaped by the other.
func TestStaleHookVerdictParity(t *testing.T) {
	cases := []struct {
		name      string
		lister    *stubStaleSweepReader
		persisted []string
		judgeable bool
	}{
		{"empty rows with entries present", &stubStaleSweepReader{rows: tokenRows()}, []string{hookstest.ReapableSeedA}, false},
		{"enumeration error", &stubStaleSweepReader{err: errors.New("tmux transient")}, []string{hookstest.ReapableSeedA}, false},
		{"restore marker set", restoringHookLister(), []string{hookstest.ReapableSeedA}, false},
		{"live rows with unstamped panes", &stubStaleSweepReader{rows: unstampedRows(2)}, []string{hookstest.UnjudgeableSeedA}, true},
		{"nothing persisted with the enumeration failing", &stubStaleSweepReader{err: errors.New("tmux transient")}, nil, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(tc.persisted...)})
			sweepStore, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(tc.persisted...)})

			got := checkStaleHooks(tc.lister, checkStore)
			checkJudgeable := got.status != checkNotEvaluable

			outcome, _ := hooksweep.Run(tc.lister, sweepStore)
			sweepJudgeable := outcome.DeclineReason == ""

			if checkJudgeable != tc.judgeable {
				t.Errorf("checkStaleHooks judgeable = %v (status %v, detail %q), want %v", checkJudgeable, got.status, got.detail, tc.judgeable)
			}
			if sweepJudgeable != tc.judgeable {
				t.Errorf("sweep judgeable = %v (reason %q), want %v", sweepJudgeable, outcome.DeclineReason, tc.judgeable)
			}
			if checkJudgeable != sweepJudgeable {
				t.Errorf("the diagnostic and the sweep disagree: check judgeable = %v, sweep judgeable = %v", checkJudgeable, sweepJudgeable)
			}
		})
	}
}
