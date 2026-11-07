//go:build integration

package restore_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
)

// decisions 55 and 58, through state's boot-time seam, which an integration
// build reads from BootTimeEnv.
func TestRestore_JudgesEachEntryByTheBoot(t *testing.T) {
	before, since := entry(t, "before"), entry(t, "since")
	before.CreatedAt, since.CreatedAt = booted.Add(-time.Hour), booted.Add(time.Hour)

	t.Run("it restores an entry from before the boot and skips one from since", func(t *testing.T) {
		t.Setenv(state.BootTimeEnv, booted.Format(time.RFC3339Nano))
		w := newWorldJudgingByState(t)
		w.save(t, before, since)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"before"}) {
			t.Errorf("created %q, want before alone", got)
		}
		skip := w.sink.Records().WithMessage("session skipped").Only(t, "skip")
		if skip.AttrString(t, "session") != "since" || skip.AttrString(t, "reason") != "it ended this boot" {
			t.Errorf("skip = %v, want since, as ended this boot", skip.Attrs)
		}
	})

	t.Run("it restores the entry from since the boot as well once it is owed", func(t *testing.T) {
		t.Setenv(state.BootTimeEnv, booted.Format(time.RFC3339Nano))
		w := newWorldJudgingByState(t)
		w.save(t, before, since)
		if err := w.r.MarkOwed(t.Context()); err != nil {
			t.Fatalf("MarkOwed: %v", err)
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 2}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
	})

	t.Run("with no boot time to judge by, it restores every entry not live", func(t *testing.T) {
		t.Setenv(state.BootTimeEnv, "not a time")
		w := newWorldJudgingByState(t)
		w.save(t, before, since)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 2}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"before", "since"}) {
			t.Errorf("created %q, want both", got)
		}
	})
}
