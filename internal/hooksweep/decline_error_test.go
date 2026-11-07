package hooksweep

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// The reason a cycle declines has to travel with the error that declines it: a
// reason carried beside the error is a second channel to forget, and a decline
// reported with no reason reads as a cycle that ran and found nothing.
func TestHookSweepDeclineReasonTravelsWithTheError(t *testing.T) {
	assertDeclined := func(t *testing.T, names []string, err error, want Reason) {
		t.Helper()
		if names != nil {
			t.Errorf("names = %v, want none on a decline", names)
		}
		var declined declinedError
		if !errors.As(err, &declined) {
			t.Fatalf("errors.As(%v, *declinedError) = false; want the reason to ride the error", err)
		}
		if declined.reason != want {
			t.Errorf("reason = %q, want %q", declined.reason, want)
		}
		if wantMsg := "hook staleness cycle declined: " + string(want); err.Error() != wantMsg {
			t.Errorf("Error() = %q, want %q", err.Error(), wantMsg)
		}
	}

	t.Run("it carries a failed listing's reason inside the error the listing returns", func(t *testing.T) {
		w := newWorld(t)
		w.backend.Fail(backendtest.OpList, errZmxDown)

		names, err := (&clean{ctx: t.Context(), src: w.sources()}).listing(hooks.Snapshot{hookstest.ReapableSeedA: {"on-resume": {Command: "cmd-gone"}}})
		assertDeclined(t, names, err, ReasonListFailed)
	})

	t.Run("it carries an unusable index's reason inside the error the index read returns", func(t *testing.T) {
		w := newWorld(t)
		w.writeIndex(t, map[string]any{"version": 1, "sessions": []any{}})
		if _, err := sessionindex.Read(w.dir); !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Fatalf("fixture: Read = %v, want a version 1 index", err)
		}

		names, err := (&clean{src: w.sources(), listed: []string{hookstest.LiveSeedA}}).indexing()
		assertDeclined(t, names, err, ReasonIndexUnusable)
	})

	t.Run("it carries no-sessions inside the error the index read returns, with nothing listed", func(t *testing.T) {
		w := newWorld(t)
		w.index(t)

		names, err := (&clean{src: w.sources()}).indexing()
		assertDeclined(t, names, err, ReasonNoSessions)
	})

	t.Run("it leaves DeclineReason empty for a cycle that ran and removed nothing", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB)})
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), newWorld(t, hookstest.LiveSeedA, hookstest.LiveSeedB).sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none for a cycle that ran", outcome.DeclineReason)
		}
		if len(outcome.Removed) != 0 {
			t.Errorf("Removed = %v, want none (every key is listed)", outcome.Removed)
		}
		if recs := sink.Records().WithMessage(standDownMsg); len(recs) != 0 {
			t.Errorf("stand-down records = %+v, want none", recs)
		}
	})
}
