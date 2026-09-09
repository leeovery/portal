package hooksweep

import (
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

func TestRunCycle(t *testing.T) {
	const entryDebugFmt = "stale-hook cleanup counts"
	const completionDebugFmt = "stale-hook cleanup removed"

	t.Run("hazard guard fires on empty live + non-empty persisted", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-a"},
  %q: {"on-resume": "cmd-b"}
}`, hookstest.ReapableSeedA, hookstest.ReapableSeedB)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})
		before := hookstest.HooksFileBytes(t, path)

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: tokenRows(), err: nil}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		hookstest.AssertHooksFileUnchanged(t, path, before, "modified by hazard-guard branch")

		if got := len(loggerSink.Records().Matching("hooks", entryDebugFmt).AtExactLevel(slog.LevelDebug)); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, loggerSink.Records())
		}

		assertStandDown(t, loggerSink, slog.LevelWarn, ReasonEmptyPaneRead)

		if got := len(loggerSink.Records().Matching("hooks", completionDebugFmt).AtExactLevel(slog.LevelDebug)); got != 0 {
			t.Errorf("completion Debug count = %d, want 0 (must NOT fire on hazard branch); entries=%+v", got, loggerSink.Records())
		}
	})

	t.Run("both-sides-empty no-op", func(t *testing.T) {
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: ""})
		before := hookstest.HooksFileBytes(t, path)

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: tokenRows(), err: nil}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		hookstest.AssertHooksFileUnchanged(t, path, before, "materialised under both-sides-empty path")

		if got := len(loggerSink.Records().Matching("hooks", entryDebugFmt).AtExactLevel(slog.LevelDebug)); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, loggerSink.Records())
		}

		for _, rec := range loggerSink.Records().AtExactLevel(slog.LevelWarn) {
			t.Errorf("unexpected Warn under both-sides-empty: %+v", rec)
		}

		if got := len(loggerSink.Records().Matching("hooks", completionDebugFmt).AtExactLevel(slog.LevelDebug)); got != 0 {
			t.Errorf("completion Debug count = %d, want 0; entries=%+v", got, loggerSink.Records())
		}
	})

	t.Run("ListAllPaneHookKeys error stands the cycle down and returns nil", func(t *testing.T) {
		seed := fmt.Sprintf(`{%q: {"on-resume": "cmd-a"}}`, hookstest.ReapableSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: nil, err: errors.New("tmux dead")}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run on ListAllPaneHookKeys error: want nil, got %v", err)
		}
		if outcome.DeclineReason != ReasonPaneReadFailed {
			t.Fatalf("DeclineReason = %q, want %q", outcome.DeclineReason, ReasonPaneReadFailed)
		}

		if got := len(loggerSink.Records().Matching("hooks", entryDebugFmt).AtExactLevel(slog.LevelDebug)); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 (must NOT fire on ListAllPaneHookKeys-error branch); entries=%+v", got, loggerSink.Records())
		}
	})

	t.Run("hookStore.Load error stands the cycle down and returns nil", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), err: nil}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run on a Load error: want nil, got %v", err)
		}
		if outcome.DeclineReason != ReasonStoreReadFailed {
			t.Fatalf("DeclineReason = %q, want %q", outcome.DeclineReason, ReasonStoreReadFailed)
		}

		if got := len(loggerSink.Records().Matching("hooks", entryDebugFmt).AtExactLevel(slog.LevelDebug)); got != 0 {
			t.Errorf("entry-point Debug count = %d, want 0 on Load-error branch; entries=%+v", got, loggerSink.Records())
		}
	})

	t.Run("the outcome names every removed entry", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-a"},
  %q: {"on-resume": "cmd-b"},
  %q: {"on-resume": "cmd-c"},
  %q: {"on-resume": "cmd-d"}
}`, hookstest.LiveSeedA, hookstest.ReapableSeedB, hookstest.ReapableSeedC, hookstest.ReapableSeedD)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), err: nil}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		removedSeen := outcome.Removed

		// CleanStale iterates a map, so removal order varies — assert set equality.
		want := map[string]struct{}{hookstest.ReapableSeedB: {}, hookstest.ReapableSeedC: {}, hookstest.ReapableSeedD: {}}
		if len(removedSeen) != len(want) {
			t.Errorf("Removed = %d (%v), want %d (%v)", len(removedSeen), removedSeen, len(want), want)
		}
		for _, k := range removedSeen {
			if _, ok := want[k]; !ok {
				t.Errorf("Removed carries unexpected key %q; want one of %v", k, want)
			}
		}
	})

	t.Run("happy-path normal removal emits entry + completion Debug", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-a"},
  %q: {"on-resume": "cmd-b"},
  %q: {"on-resume": "cmd-c"},
  %q: {"on-resume": "cmd-d"}
}`, hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC, hookstest.ReapableSeedD)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA, hookstest.LiveSeedB, hookstest.LiveSeedC), err: nil}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		wantKeys := map[string]struct{}{hookstest.LiveSeedA: {}, hookstest.LiveSeedB: {}, hookstest.LiveSeedC: {}}
		if len(postRun) != len(wantKeys) {
			t.Errorf("post-run hook count = %d (keys=%v), want %d", len(postRun), keysOf(postRun), len(wantKeys))
		}
		if _, ok := postRun[hookstest.ReapableSeedD]; ok {
			t.Errorf("post-run hooks still contains stale key %s; got %v", hookstest.ReapableSeedD, keysOf(postRun))
		}

		if got := len(loggerSink.Records().Matching("hooks", entryDebugFmt).AtExactLevel(slog.LevelDebug)); got != 1 {
			t.Errorf("entry-point Debug count = %d, want 1; entries=%+v", got, loggerSink.Records())
		}

		if got := len(loggerSink.Records().Matching("hooks", completionDebugFmt).AtExactLevel(slog.LevelDebug)); got != 1 {
			t.Errorf("completion Debug count = %d, want 1; entries=%+v", got, loggerSink.Records())
		}

		for _, rec := range loggerSink.Records().AtExactLevel(slog.LevelWarn) {
			t.Errorf("unexpected Warn on normal-removal path: %+v", rec)
		}
	})

	t.Run("it enumerates live keys via ListAllPaneHookKeys not ListAllPanesWithFormat", func(t *testing.T) {
		seed := fmt.Sprintf(`{%q: {"on-resume": "cmd-a"}}`, hookstest.LiveSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		rec := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}

		if err := runErr(rec, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if rec.calls != 1 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 1 (the enumeration must switch to the hook-key method)", rec.calls)
		}
	})

	t.Run("it preserves a hook whose token matches the live set", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-live"},
  %q: {"on-resume": "cmd-gone"}
}`, hookstest.LiveSeedA, hookstest.ReapableSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), err: nil}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.LiveSeedA]; !ok {
			t.Errorf("hook %s keyed on a live pane's token was removed; want preserved (present in live set); got %v", hookstest.LiveSeedA, keysOf(postRun))
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; ok {
			t.Errorf("truly-stale hook %s survived; want removed; got %v", hookstest.ReapableSeedA, keysOf(postRun))
		}
	})
}

// A key the reaper cannot parse as a token is not evidence of a dead pane, so
// no cycle may take it, however the sweep is reached.
func TestUnjudgeableHookKeyRetention(t *testing.T) {
	t.Run("it retains a non-token-shaped key across a sweep", func(t *testing.T) {
		retained := hookstest.UnjudgeableSeedA
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-live"},
  %q: {"on-resume": "cmd-old"}
}`, hookstest.LiveSeedA, retained)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})
		before := hookstest.HooksFileBytes(t, path)

		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}
		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[retained]; !ok {
			t.Errorf("non-token-shaped key was swept; got %v", keysOf(postRun))
		}

		hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten with nothing to remove")
	})

}

// A restore window is a hole in the reaper's judgement: every live pane carries
// no token between skeleton construction and the re-stamp, so a sweep landing
// there would reap every token-keyed entry on the machine.
func TestHookSweepStandsDownWhileRestoring(t *testing.T) {
	t.Run("it stands down before enumerating while the restore marker is set", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), restoring: true}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if lister.calls != 0 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 0 (the sweep must stand down before enumerating)", lister.calls)
		}
	})

	// A read that failed proves nothing about the marker, so the cycle still
	// stands down on it — the fail-safe posture is unchanged by the reason it
	// is reported under.
	t.Run("it stands down before enumerating when the marker read fails", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), restoringErr: errors.New("tmux dead")}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if outcome.DeclineReason != ReasonMarkerReadFailed {
			t.Fatalf("DeclineReason = %q, want %q", outcome.DeclineReason, ReasonMarkerReadFailed)
		}

		if lister.calls != 0 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 0 on a failed marker read", lister.calls)
		}
	})

	t.Run("it skips before loading the store", func(t *testing.T) {
		// The store fails loudly on any read, so a decline naming the restore
		// marker — rather than the store read — proves it was never loaded.
		store, _ := hookstest.StageStore(t, hookstest.Staging{Unreadable: true})

		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA), restoring: true}

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: want nil (store untouched), got %v", err)
		}
		if outcome.DeclineReason != ReasonRestoring {
			t.Errorf("DeclineReason = %q, want %q (a store read would have declined with %q)",
				outcome.DeclineReason, ReasonRestoring, ReasonStoreReadFailed)
		}
		if lister.calls != 0 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 0", lister.calls)
		}
		if len(outcome.Removed) != 0 {
			t.Errorf("Removed = %v, want none", outcome.Removed)
		}
	})

	t.Run("it sweeps normally when the marker is absent", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if lister.calls != 1 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 1", lister.calls)
		}
		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; ok {
			t.Errorf("stale key %s survived a marker-absent sweep; got %v", hookstest.ReapableSeedA, keysOf(postRun))
		}
		if _, ok := postRun[hookstest.LiveSeedA]; !ok {
			t.Errorf("live key was reaped; got %v", keysOf(postRun))
		}
	})

}

// A repair that silently did not run is indistinguishable from a repair that
// found nothing to do, so every stand-down reports itself to its caller.
func TestHookSweepReportsStandDown(t *testing.T) {
	t.Run("it reports nothing when the live set is empty and no entries are persisted", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: ""})
		lister := &stubReader{rows: tokenRows()}

		sink := logtest.Install(t)

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none with nothing to protect", outcome.DeclineReason)
		}
		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelWarn) {
			t.Errorf("unexpected Warn with nothing to protect: %+v", rec)
		}
	})

	t.Run("it reports the failed enumeration as a stand-down on the hooks component", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		lister := &stubReader{err: errors.New("tmux dead")}

		sink := logtest.Install(t)

		outcome, err := Run(lister, store)
		if err != nil {
			t.Fatalf("Run on an enumeration error: want nil, got %v", err)
		}
		if outcome.DeclineReason != ReasonPaneReadFailed {
			t.Fatalf("DeclineReason = %q, want %q", outcome.DeclineReason, ReasonPaneReadFailed)
		}

		assertStandDown(t, sink, slog.LevelWarn, ReasonPaneReadFailed)
	})
}

// Under lazy stamping a server with no stamped pane at all is the ordinary
// steady state, so the guard's question — did the tmux read succeed? — is
// answered by the pane rows, never by the tokens they carry.
func TestHookSweepGuardCountsPaneRowsNotTokens(t *testing.T) {
	t.Run("it does not fire the mass-deletion guard when no pane is stamped", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  %q: {"on-resume": "cmd-unjudgeable-a"},
  %q: {"on-resume": "cmd-unjudgeable-b"}
}`, hookstest.UnjudgeableSeedA, hookstest.UnjudgeableSeedB)
		store, path := hookstest.StageStore(t, hookstest.Staging{Seed: seed})
		before := hookstest.HooksFileBytes(t, path)

		sink := logtest.Install(t)

		outcome, err := Run(&stubReader{rows: unstampedRows(3)}, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none (rows present means the read succeeded)", outcome.DeclineReason)
		}
		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelWarn) {
			t.Errorf("unexpected Warn with rows present and no token: %+v", rec)
		}
		hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten with only unjudgeable entries present")
	})

	t.Run("it still deletes a token-shaped key absent from the live token set", func(t *testing.T) {
		seed := fmt.Sprintf(`{%q: {"on-resume": "cmd-gone"}}`, hookstest.ReapableSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		lister := &stubReader{rows: append(tokenRows(hookstest.ReapableSeedB), unstampedRows(1)...)}
		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; ok {
			t.Errorf("token-shaped key %s absent from the live token set survived; got %v", hookstest.ReapableSeedA, keysOf(postRun))
		}
	})

	t.Run("it takes the empty-pane-read branch only on zero rows", func(t *testing.T) {
		seed := fmt.Sprintf(`{%q: {"on-resume": "cmd-gone"}}`, hookstest.ReapableSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		outcome, err := Run(&stubReader{rows: nil}, store)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		if outcome.DeclineReason != ReasonEmptyPaneRead {
			t.Errorf("DeclineReason = %q, want %q", outcome.DeclineReason, ReasonEmptyPaneRead)
		}
		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; !ok {
			t.Errorf("a zero-row read reaped an entry; got %v", keysOf(postRun))
		}
	})

	t.Run("it counts the rows, not the tokens, on the counts line", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: fmt.Sprintf(`{%q: {"on-resume": "cmd-unjudgeable"}}`, hookstest.UnjudgeableSeedB)})

		loggerSink := logtest.Install(t)
		lister := &stubReader{rows: unstampedRows(4)}
		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		rec := loggerSink.Records().Matching("hooks", "stale-hook cleanup counts").AtExactLevel(slog.LevelDebug).Only(t, "DEBUG hooks stale-hook cleanup counts record")
		if got := rec.IntAttr(t, "panes"); got != 4 {
			t.Errorf("panes = %d, want 4 (the count is of live panes; none of these carries a token)", got)
		}
	})

	t.Run("it reaps an empty key when a pane carries no token", func(t *testing.T) {
		seed := fmt.Sprintf(`{
  "": {"on-resume": "cmd-empty"},
  %q: {"on-resume": "cmd-old"}
}`, hookstest.UnjudgeableSeedA)
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: seed})

		lister := &stubReader{rows: unstampedRows(2)}
		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		// An unstamped pane protects no key. Were its empty token to enter the
		// live set, this entry would be permanently unreachable by the reaper
		// and would fire its command on every unstamped restored pane.
		if _, ok := postRun[""]; ok {
			t.Errorf("empty-keyed entry survived alongside unstamped panes; got %v", keysOf(postRun))
		}
		if _, ok := postRun[hookstest.UnjudgeableSeedA]; !ok {
			t.Errorf("unjudgeable entry was swept; got %v", keysOf(postRun))
		}
	})
}

// With nothing persisted there is no key to judge and none to protect, so the
// cheapest and most decisive test is taken before the whole-server pane read a
// daemon would otherwise pay for on every cycle for the life of the process.
func TestHookSweepWithNothingPersisted(t *testing.T) {
	t.Run("it enumerates no panes when nothing is persisted", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		lister := &stubReader{rows: tokenRows(hookstest.LiveSeedA)}

		if err := runErr(lister, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		if lister.calls != 0 {
			t.Errorf("ListAllPaneHookKeys call count = %d, want 0 (an empty store has nothing to judge)", lister.calls)
		}
	})

	t.Run("it reports the entry count alone for an empty snapshot", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Seed: hooksBody()})
		sink := logtest.Install(t)

		if err := runErr(&stubReader{rows: tokenRows(hookstest.LiveSeedA)}, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		rec := sink.Records().Matching("hooks", "stale-hook cleanup counts").AtExactLevel(slog.LevelDebug).
			Only(t, "DEBUG hooks stale-hook cleanup counts record")
		if got := rec.IntAttr(t, "entries"); got != 0 {
			t.Errorf("entries = %d, want 0", got)
		}
		if rec.HasAttr("panes") {
			t.Errorf("counts line carries a pane count for a cycle that never enumerated: %+v", rec)
		}
	})

	t.Run("it answers the live token set for a non-empty snapshot", func(t *testing.T) {
		enumerate := liveTokenEnumeration(&stubReader{rows: tokenRows(hookstest.LiveSeedA)})

		if _, err := enumerate(hooks.Snapshot{}); !errors.Is(err, errNothingPersisted) {
			t.Errorf("empty snapshot err = %v, want %v", err, errNothingPersisted)
		}

		tokens, err := enumerate(hooks.Snapshot{hookstest.LiveSeedA: {"on-resume": "cmd-live"}})
		if err != nil {
			t.Fatalf("enumerate: %v", err)
		}
		if len(tokens) != 1 || tokens[0] != hookstest.LiveSeedA {
			t.Errorf("tokens = %v, want [%s]", tokens, hookstest.LiveSeedA)
		}
	})
}
