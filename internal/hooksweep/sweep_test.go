package hooksweep

import (
	"fmt"
	"log/slog"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// A key is stale when it names neither a session the backend lists, answering
// or not, nor an entry in sessions.json.
func TestSweepRule(t *testing.T) {
	survives := func(t *testing.T, w *world, key string) {
		t.Helper()
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(key, hookstest.ReapableSeedA)})

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !slices.Equal(outcome.Removed, []string{hookstest.ReapableSeedA}) {
			t.Errorf("Removed = %v, want only [%s]", outcome.Removed, hookstest.ReapableSeedA)
		}
		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if _, ok := h[key]; !ok {
			t.Errorf("%s was reaped; file holds %v", key, keysOf(h))
		}
	}

	t.Run("it keeps a key naming a listed session", func(t *testing.T) {
		survives(t, newWorld(t, hookstest.LiveSeedA), hookstest.LiveSeedA)
	})

	t.Run("it keeps a key naming a listed session that doesn't answer", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		w.backend.StopAnswering(hookstest.LiveSeedA)
		survives(t, w, hookstest.LiveSeedA)
	})

	t.Run("it keeps a key naming only an index entry", func(t *testing.T) {
		w := newWorld(t)
		w.index(t, hookstest.LiveSeedA)
		survives(t, w, hookstest.LiveSeedA)
	})

	t.Run("it reaps a key naming neither", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		w.index(t, hookstest.LiveSeedB)
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: hooksBody(hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.ReapableSeedB, hookstest.ReapableSeedC),
		})

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if want := []string{hookstest.ReapableSeedB, hookstest.ReapableSeedC}; !slices.Equal(outcome.Removed, want) {
			t.Errorf("Removed = %v, want %v", outcome.Removed, want)
		}
		h, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if got := slices.Sorted(slices.Values(keysOf(h))); !slices.Equal(got, []string{hookstest.LiveSeedA, hookstest.LiveSeedB}) {
			t.Errorf("file holds %v, want the listed and the indexed keys", got)
		}
	})
}

// A hook goes with its session's entry, so a sweep reaping three keys or more
// that are half of hooks.json or more is judging it against the wrong state:
// it holds them all, warning once.
func TestSweepHoldsAMassReap(t *testing.T) {
	reapable := []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedC}
	for name, listed := range map[string][]string{
		"three of four":                  {hookstest.LiveSeedA},
		"three of six, exactly the half": {hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC},
	} {
		t.Run("it holds "+name+", removing none", func(t *testing.T) {
			w := newWorld(t, listed...)
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(slices.Concat(listed, reapable)...)})
			before := hookstest.HooksFileBytes(t, path)
			persisted, err := store.Load(hooks.ViaInternal)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			sink := logtest.Install(t)

			outcome, err := Run(t.Context(), w.sources(), store)

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if outcome.DeclineReason != ReasonMassReap || len(outcome.Removed) != 0 {
				t.Errorf("Run = %+v, want nothing removed, declined as %q", outcome, ReasonMassReap)
			}
			hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten by a cycle that held a mass reap")
			rec := sink.Records().WithMessage(standDownMsg).AtExactLevel(slog.LevelWarn).Only(t, "the hold's warning")
			if got := rec.AttrString(t, "reason"); got != string(ReasonMassReap) {
				t.Errorf("reason = %q, want %q", got, ReasonMassReap)
			}
			if got := rec.IntAttr(t, "entries"); got != int64(len(reapable)) {
				t.Errorf("entries = %d, want the %d keys it held", got, len(reapable))
			}
			if stale, d := Stale(t.Context(), w.sources(), persisted); d.Reason() != ReasonMassReap || len(stale) != 0 {
				t.Errorf("Stale = (%v, %q), want none, declined as %q", stale, d.Reason(), ReasonMassReap)
			}
		})
	}

	for name, tc := range map[string]struct{ listed, keys []string }{
		"two keys, every one stale but fewer than three": {
			listed: []string{hookstest.LiveSeedA},
			keys:   []string{hookstest.ReapableSeedA, hookstest.ReapableSeedB},
		},
		"three stale keys of seven, fewer than half": {
			listed: []string{hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC, hookstest.SubjectSeedA},
			keys:   slices.Concat([]string{hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC, hookstest.SubjectSeedA}, reapable),
		},
	} {
		t.Run("it reaps "+name, func(t *testing.T) {
			w := newWorld(t, tc.listed...)
			store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(tc.keys...)})
			sink := logtest.Install(t)

			outcome, err := Run(t.Context(), w.sources(), store)

			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			want := slices.DeleteFunc(slices.Clone(tc.keys), func(key string) bool { return slices.Contains(tc.listed, key) })
			if outcome.DeclineReason != "" || !slices.Equal(outcome.Removed, want) {
				t.Errorf("Run = %+v, want %v removed", outcome, want)
			}
			if warnings := sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warnings) != 0 {
				t.Errorf("logged %v, want no warning", warnings)
			}
		})
	}
}

// standDownCase is a world in which the judgement may not run, and the reason
// it stands down under.
type standDownCase struct {
	name   string
	reason Reason
	// stage makes the world's one anomaly.
	stage func(t *testing.T, w *world)
	// listed is whether the cycle still takes its listing before it stands
	// down.
	listed bool
}

func standDownCases() []standDownCase {
	writeVersion := func(version int) func(*testing.T, *world) {
		return func(t *testing.T, w *world) {
			w.writeIndex(t, map[string]any{"version": version, "sessions": []any{}})
		}
	}
	return []standDownCase{
		{name: "a failed listing", reason: ReasonListFailed, listed: true, stage: func(t *testing.T, w *world) {
			w.backend.Fail(backendtest.OpList, errZmxDown)
		}},
		{name: "an unreadable index", reason: ReasonIndexUnusable, listed: true, stage: func(t *testing.T, w *world) {
			makeUnreadableAt(t, sessionindex.Path(w.dir))
		}},
		{name: "a version 1 index", reason: ReasonIndexUnusable, listed: true, stage: writeVersion(1)},
		{name: "an index from a newer Portal", reason: ReasonIndexUnusable, listed: true, stage: writeVersion(sessionindex.SchemaVersion + 1)},
		{name: "a corrupt index", reason: ReasonIndexUnusable, listed: true, stage: func(t *testing.T, w *world) {
			w.writeIndexBytes(t, []byte(`{"version": 2, "sessions": [`))
		}},
		{name: "the restoring flag raised", reason: ReasonRestoring, stage: func(t *testing.T, w *world) {
			w.raiseRestoring(t)
		}},
		{name: "the restoring flag unreadable", reason: ReasonRestoringReadFailed, stage: func(t *testing.T, w *world) {
			makeUnreadableAt(t, w.dir)
		}},
		{name: "no state directory", reason: ReasonStateDirUnresolved, stage: func(t *testing.T, w *world) {
			w.dir = ""
		}},
		{name: "nothing listed and no sessions.json", reason: ReasonNoSessions, listed: true, stage: func(*testing.T, *world) {}},
		{name: "nothing listed and an empty sessions.json", reason: ReasonNoSessions, listed: true, stage: func(t *testing.T, w *world) {
			w.index(t)
		}},
		// The daemon that sweeps runs in _portal-saver, so a listing always
		// holds it: a scratch state and zmx dir beside a real hooks.json must
		// still read as nothing listed.
		{name: "only _portal-saver listed beside an empty sessions.json", reason: ReasonNoSessions, listed: true, stage: func(t *testing.T, w *world) {
			w.backend.Add(backend.Session{Name: portalSaver})
			w.index(t)
		}},
	}
}

// Every stand-down removes nothing and says why, at DEBUG.
func TestSweepStandsDown(t *testing.T) {
	for _, tc := range standDownCases() {
		t.Run("it removes nothing on "+tc.name, func(t *testing.T) {
			w := newWorld(t)
			tc.stage(t, w)
			store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
			before := hookstest.HooksFileBytes(t, path)
			sink := logtest.Install(t)

			outcome, err := Run(t.Context(), w.sources(), store)
			if err != nil {
				t.Fatalf("Run: want a stand-down, got %v", err)
			}
			if outcome.DeclineReason != tc.reason {
				t.Errorf("DeclineReason = %q, want %q", outcome.DeclineReason, tc.reason)
			}
			if len(outcome.Removed) != 0 {
				t.Errorf("Removed = %v, want none", outcome.Removed)
			}
			hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten by a cycle that stood down")
			assertStandDown(t, sink, tc.reason)
			if listed := w.lists > 0; listed != tc.listed {
				t.Errorf("listed = %v, want %v", listed, tc.listed)
			}
		})
	}

	// The flag is read before the store, so a restore never waits on this
	// file's lock: a store that fails any read declines under the flag.
	t.Run("it stands down on the restoring flag before reading the store", func(t *testing.T) {
		w := newWorld(t)
		w.raiseRestoring(t)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != ReasonRestoring {
			t.Errorf("DeclineReason = %q, want %q (a store read would have failed the cycle)", outcome.DeclineReason, ReasonRestoring)
		}
	})

	t.Run("it sweeps once the restoring flag is lowered", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		w.raiseRestoring(t)
		if _, err := state.Restoring.Clear(w.dir); err != nil {
			t.Fatalf("lower the restoring flag: %v", err)
		}
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if !slices.Equal(outcome.Removed, []string{hookstest.ReapableSeedA}) {
			t.Errorf("Removed = %v, want [%s]", outcome.Removed, hookstest.ReapableSeedA)
		}
	})
}

// The diagnosis must answer the reaper's question with the reaper's own
// answer: a verdict the two disagree on is a user told a hook is lost by one
// command and reaped by the other.
func TestStaleAgreesWithTheSweep(t *testing.T) {
	for _, tc := range standDownCases() {
		t.Run("they stand down alike on "+tc.name, func(t *testing.T) {
			w := newWorld(t)
			tc.stage(t, w)
			persisted := hooks.Snapshot{hookstest.ReapableSeedA: {"on-resume": {Command: "gone"}}}
			store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.ReapableSeedA)})

			stale, d := Stale(t.Context(), w.sources(), persisted)
			outcome, err := Run(t.Context(), w.sources(), store)
			if err != nil {
				t.Fatalf("Run: %v", err)
			}
			if d.Reason() != tc.reason || outcome.DeclineReason != tc.reason {
				t.Errorf("Stale declined under %q and Run under %q, want both %q", d.Reason(), outcome.DeclineReason, tc.reason)
			}
			if len(stale) != 0 {
				t.Errorf("Stale = %v on a stand-down, want none", stale)
			}
		})
	}

	t.Run("they judge the same keys stale", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		w.index(t, hookstest.LiveSeedB)
		keys := []string{hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.ReapableSeedA, hookstest.ReapableSeedB}
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(keys...)})
		persisted, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("load: %v", err)
		}

		stale, d := Stale(t.Context(), w.sources(), persisted)
		if d.Declined() {
			t.Fatalf("Stale declined under %q", d.Reason())
		}
		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if len(stale) == 0 || !slices.Equal(stale, outcome.Removed) {
			t.Errorf("Stale = %v, Run removed %v; want the same, non-empty", stale, outcome.Removed)
		}
	})

	t.Run("they read nothing past the flag with nothing persisted", func(t *testing.T) {
		w := newWorld(t)
		w.backend.Fail(backendtest.OpList, errZmxDown)

		stale, d := Stale(t.Context(), w.sources(), hooks.Snapshot{})
		if d.Declined() || len(stale) != 0 {
			t.Errorf("Stale = (%v, %q), want nothing and no stand-down", stale, d.Reason())
		}
		if w.lists != 0 {
			t.Errorf("listings = %d, want 0", w.lists)
		}
	})
}

// One cycle is one INFO summary when it removed anything, with its counts and
// each key at DEBUG beside it.
func TestSweepLogging(t *testing.T) {
	t.Run("it counts the sessions and entries, and the keys it reaped", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA, hookstest.LiveSeedB)
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Seed: hooksBody(hookstest.LiveSeedA, hookstest.ReapableSeedA, hookstest.ReapableSeedB),
		})
		sink := logtest.Install(t)

		if err := runErr(t, w.sources(), store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		counts := sink.Records().Matching("hooks", countsMsg).AtExactLevel(slog.LevelDebug).Only(t, "the counts line")
		if got := counts.IntAttr(t, "sessions"); got != 2 {
			t.Errorf("sessions = %d, want 2", got)
		}
		if got := counts.IntAttr(t, "entries"); got != 3 {
			t.Errorf("entries = %d, want 3", got)
		}
		removed := sink.Records().Matching("hooks", removedMsg).AtExactLevel(slog.LevelDebug).Only(t, "the removed line")
		if got := removed.IntAttr(t, "reaped"); got != 2 {
			t.Errorf("reaped = %d, want 2", got)
		}
		summary := sink.Records().WithMessage("clean-stale").AtExactLevel(slog.LevelInfo)
		var summaries int
		for _, rec := range summary {
			if rec.HasAttr("entries") {
				summaries++
			}
		}
		if summaries != 1 {
			t.Errorf("clean-stale summaries = %d, want 1: %+v", summaries, summary)
		}
		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelWarn) {
			t.Errorf("unexpected WARN: %+v", rec)
		}
	})

	t.Run("it writes no summary when nothing is stale", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody(hookstest.LiveSeedA)})
		before := hookstest.HooksFileBytes(t, path)
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != "" || len(outcome.Removed) != 0 {
			t.Errorf("outcome = %+v, want a cycle that ran and removed nothing", outcome)
		}
		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelInfo) {
			t.Errorf("unexpected record at INFO or above: %+v", rec)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten with nothing to remove")
	})
}

// With nothing persisted there is no key to judge and none to protect, so the
// cheapest and most decisive test is taken before a listing that asks every
// session's daemon, which a daemon would otherwise pay on every cycle.
func TestSweepWithNothingPersisted(t *testing.T) {
	t.Run("it lists nothing and counts the entries alone", func(t *testing.T) {
		w := newWorld(t, hookstest.LiveSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), w.sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none with nothing persisted", outcome.DeclineReason)
		}
		if w.lists != 0 {
			t.Errorf("listings = %d, want 0 (an empty store has nothing to judge)", w.lists)
		}
		rec := sink.Records().Matching("hooks", countsMsg).AtExactLevel(slog.LevelDebug).Only(t, "the counts line")
		if got := rec.IntAttr(t, "entries"); got != 0 {
			t.Errorf("entries = %d, want 0", got)
		}
		if rec.HasAttr("sessions") {
			t.Errorf("counts line carries a session count for a cycle that never listed: %+v", rec)
		}
		if recs := sink.Records().WithMessage(standDownMsg); len(recs) != 0 {
			t.Errorf("stand-down records = %+v, want none", recs)
		}
	})

	t.Run("it reports nothing on an empty listing and an empty store", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: ""})
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), newWorld(t).sources(), store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none with nothing to protect", outcome.DeclineReason)
		}
		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelWarn) {
			t.Errorf("unexpected WARN with nothing to protect: %+v", rec)
		}
	})
}

// fmtSeed renders a one-entry hooks.json body under key.
func fmtSeed(key, command string) string {
	return fmt.Sprintf(`{%q: {"on-resume": %q}}`, key, command)
}
