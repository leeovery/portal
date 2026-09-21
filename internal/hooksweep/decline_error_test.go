package hooksweep

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// The reason a cycle declines has to travel with the error that declines it: a
// reason carried beside the error is a second channel to forget, and a decline
// reported with no reason reads as a cycle that ran and found nothing.
func TestHookSweepDeclineReasonTravelsWithTheError(t *testing.T) {
	t.Run("it carries the decline reason inside the error the closure returns", func(t *testing.T) {
		enumerate := liveTokenEnumeration(&stubReader{rows: nil})

		tokens, err := enumerate(hooks.Snapshot{hookstest.ReapableSeedA: {"on-resume": {Command: "cmd-gone"}}})

		if tokens != nil {
			t.Errorf("tokens = %v, want none on a decline", tokens)
		}
		var declined declinedError
		if !errors.As(err, &declined) {
			t.Fatalf("errors.As(%v, *declinedError) = false; want the reason to ride the error", err)
		}
		if declined.reason != ReasonEmptyPaneRead {
			t.Errorf("reason = %q, want %q", declined.reason, ReasonEmptyPaneRead)
		}
		if want := "hook staleness cycle declined: " + string(ReasonEmptyPaneRead); err.Error() != want {
			t.Errorf("Error() = %q, want %q", err.Error(), want)
		}
	})

	t.Run("it leaves DeclineReason empty for a cycle that ran and removed nothing", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB)})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA, hookstest.LiveSeedB)}

		sink := logtest.Install(t)

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none for a cycle that ran", outcome.DeclineReason)
		}
		if len(outcome.Removed) != 0 {
			t.Errorf("Removed = %v, want none (every key is live)", outcome.Removed)
		}
		if recs := sink.Records().WithMessage(standDownMsg); len(recs) != 0 {
			t.Errorf("stand-down records = %+v, want none", recs)
		}
	})

	t.Run("it still returns nothing-persisted without a stand-down line", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}

		sink := logtest.Install(t)

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none with nothing persisted", outcome.DeclineReason)
		}
		if recs := sink.Records().WithMessage(standDownMsg); len(recs) != 0 {
			t.Errorf("stand-down records = %+v, want none", recs)
		}
	})
}
