package hooksweep

import (
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// A hooks.json the cycle can't read, or that doesn't parse, is a defect rather
// than a condition to wait out: no cadence repairs it, so it is reported as a
// failure, at WARN, at whichever of the cycle's two reads meets it.
func TestHookSweepReportsAStoreItCannotReadAsADefect(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stage func(t *testing.T) (*world, *hooks.Store, string)
		// malformed is whether the failure is a file that doesn't parse.
		malformed bool
	}{
		{name: "a hooks.json unreadable from the outset", stage: func(t *testing.T) (*world, *hooks.Store, string) {
			store, path := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})
			return newWorld(t, hookstest.LiveSeedA), store, path
		}},
		{name: "a hooks.json that goes unreadable while the listing is out", stage: func(t *testing.T) (*world, *hooks.Store, string) {
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
			w := newWorld(t, hookstest.LiveSeedA)
			w.during = func() {
				if err := os.Remove(path); err != nil {
					t.Errorf("remove the staged hooks.json: %v", err)
					return
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Errorf("stage a directory at the hooks.json path: %v", err)
				}
			}
			return w, store, path
		}},
		{name: "a hooks.json that doesn't parse", malformed: true, stage: func(t *testing.T) (*world, *hooks.Store, string) {
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: `{"` + hookstest.ReapableSeedA + `": {`})
			return newWorld(t, hookstest.LiveSeedA), store, path
		}},
	} {
		t.Run("it fails, at WARN, on "+tc.name, func(t *testing.T) {
			w, store, path := tc.stage(t)
			var before []byte
			if tc.malformed {
				before = hookstest.HooksFileBytes(t, path)
			}
			sink := logtest.Install(t)

			outcome, err := Run(t.Context(), w.sources(), store)

			if !errors.Is(err, hooks.ErrStoreRead) {
				t.Fatalf("Run error = %v, want errors.Is ErrStoreRead", err)
			}
			if tc.malformed && !errors.Is(err, hooks.ErrMalformed) {
				t.Errorf("Run error = %v, want errors.Is ErrMalformed", err)
			}
			if outcome.DeclineReason != "" || len(outcome.Removed) != 0 {
				t.Errorf("outcome = %+v, want neither a stand-down nor a removal", outcome)
			}
			rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the failure's WARN")
			if rec.Msg != sweepFailedMsg || rec.AttrOrEmpty("component") != "hooks" {
				t.Errorf("WARN = %s %q, want %q on hooks", rec.AttrOrEmpty("component"), rec.Msg, sweepFailedMsg)
			}
			if recs := sink.Records().WithMessage(standDownMsg); len(recs) != 0 {
				t.Errorf("stand-down records = %+v, want none for a defect", recs)
			}
			if tc.malformed {
				if w.lists != 0 {
					t.Errorf("listings = %d, want 0: a file that doesn't parse is never judged", w.lists)
				}
				hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten while malformed")
			}
		})
	}

	t.Run("it leaves a failed save on the same path", func(t *testing.T) {
		logtest.Install(t)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed, WritesDenied: true})

		outcome, err := Run(t.Context(), newWorld(t, hookstest.LiveSeedA).sources(), store)
		if err == nil {
			t.Fatal("Run returned no error for a save it could not make")
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none — a failed write is no stand-down", outcome.DeclineReason)
		}
	})
}
