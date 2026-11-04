package cmd

import (
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/state"
)

// hookSweepDaemon is a daemon whose hook store holds StaleHookSeed, its
// backend listing LiveSeedA beside the saver session, its last sweep at epoch.
func hookSweepDaemon(t *testing.T) (*daemonDeps, string) {
	t.Helper()
	deps, fake := newTestDaemon(t, &fakeSaver{})
	fake.Add(backend.Session{Name: hookstest.LiveSeedA})
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
	deps.Hooks, deps.lastHookSweep = store, epoch
	return deps, path
}

func TestSweepStaleHooks(t *testing.T) {
	t.Run("it sweeps an interval after the last sweep, and not before", func(t *testing.T) {
		deps, _ := hookSweepDaemon(t)
		logtest.Install(t)

		deps.sweepStaleHooks(t.Context(), epoch.Add(hookSweepInterval-time.Second))
		if !slices.Contains(hookKeys(t, deps.Hooks), hookstest.ReapableSeedA) {
			t.Error("the stale hook went before the interval passed")
		}
		if !deps.lastHookSweep.Equal(epoch) {
			t.Errorf("last sweep = %v before the interval, want it left at %v", deps.lastHookSweep, epoch)
		}

		at := epoch.Add(hookSweepInterval)
		deps.sweepStaleHooks(t.Context(), at)
		if keys := hookKeys(t, deps.Hooks); !slices.Equal(keys, []string{hookstest.LiveSeedA}) {
			t.Errorf("hooks.json holds %v once the interval passed, want only the listed key", keys)
		}
		if !deps.lastHookSweep.Equal(at) {
			t.Errorf("last sweep = %v, want %v", deps.lastHookSweep, at)
		}
	})

	t.Run("it sweeps on an idle tick, never on one that polls", func(t *testing.T) {
		deps, _ := hookSweepDaemon(t)
		logtest.Install(t)

		deps.tick(t.Context(), epoch.Add(hookSweepInterval))
		if !slices.Contains(hookKeys(t, deps.Hooks), hookstest.ReapableSeedA) {
			t.Error("a polling tick swept the hooks; the sweep is left to an idle one")
		}

		deps.tick(t.Context(), epoch.Add(hookSweepInterval+time.Second))
		if slices.Contains(hookKeys(t, deps.Hooks), hookstest.ReapableSeedA) {
			t.Error("an idle tick an interval on left the stale hook")
		}
	})

	// The sweep logs its own failure, so the daemon adds none: one WARN, and
	// the next sweep still an interval away.
	t.Run("a sweep that fails is one WARN, and waits an interval like any other", func(t *testing.T) {
		deps, _ := newTestDaemon(t, &fakeSaver{})
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Dir:           filepath.Join(t.TempDir(), "lock-open-denied"),
			Seed:          hookstest.StaleHookSeed,
			SidecarAbsent: true,
			WritesDenied:  true,
		})
		deps.Hooks, deps.lastHookSweep = store, epoch
		sink := logtest.Install(t)

		at := epoch.Add(hookSweepInterval)
		deps.sweepStaleHooks(t.Context(), at)

		rec := sink.Records().AtOrAboveLevel(slog.LevelWarn).Only(t, "the failed sweep's WARN")
		if got := rec.AttrOrEmpty("component"); got != "hooks" || rec.Msg != "stale-hook cleanup failed" {
			t.Errorf("WARN = %s %q, want the sweep's own failure on the hooks component", got, rec.Msg)
		}
		if !deps.lastHookSweep.Equal(at) {
			t.Errorf("last sweep = %v, want %v", deps.lastHookSweep, at)
		}
	})

	t.Run("a sweep that stands down logs at DEBUG, and waits an interval like any other", func(t *testing.T) {
		deps, path := hookSweepDaemon(t)
		deps.Backend.(*backendtest.Fake).Fail(backendtest.OpList, errZmxDirUnreadable)
		before := readFileBytes(t, path)
		sink := logtest.Install(t)

		at := epoch.Add(hookSweepInterval)
		deps.sweepStaleHooks(t.Context(), at)

		for _, rec := range sink.Records().AtOrAboveLevel(slog.LevelInfo) {
			t.Errorf("a stand-down logged above DEBUG: %+v", rec)
		}
		sink.Records().Matching("hooks", "clean-stale-skipped").AtExactLevel(slog.LevelDebug).Only(t, "the stand-down")
		assertHooksFileUnchanged(t, path, before, "touched by a sweep that stood down")
		if !deps.lastHookSweep.Equal(at) {
			t.Errorf("last sweep = %v, want %v", deps.lastHookSweep, at)
		}
	})

	t.Run("it judges against the daemon's own state dir", func(t *testing.T) {
		deps, path := hookSweepDaemon(t)
		if err := state.Restoring.Set(deps.Dir); err != nil {
			t.Fatalf("raise the restoring flag: %v", err)
		}
		before := readFileBytes(t, path)
		logtest.Install(t)

		deps.sweepStaleHooks(t.Context(), epoch.Add(hookSweepInterval))

		assertHooksFileUnchanged(t, path, before, "swept while the daemon's state dir is restoring")
	})

	t.Run("it sweeps nothing with no hook store", func(t *testing.T) {
		deps, fake := newTestDaemon(t, &fakeSaver{})
		deps.lastHookSweep = epoch

		deps.sweepStaleHooks(t.Context(), epoch.Add(hookSweepInterval))

		if calls := fake.CallsTo(backendtest.OpList); len(calls) != 0 {
			t.Errorf("listed %d times with no hook store; want none", len(calls))
		}
	})

	t.Run("the daemon sweeps the configured hook store, first an interval after it starts", func(t *testing.T) {
		_, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
		t.Setenv("PORTAL_HOOKS_FILE", path)
		logtest.Install(t)
		started := time.Now()

		deps := newDaemonDeps(t.TempDir(), backendtest.New(), nil, nil)

		if deps.Hooks == nil {
			t.Fatal("the daemon holds no hook store to sweep")
		}
		if got := hookKeys(t, deps.Hooks); !slices.Contains(got, hookstest.ReapableSeedA) {
			t.Errorf("the daemon's hook store holds %v, want the configured hooks.json's keys", got)
		}
		if deps.lastHookSweep.Before(started) {
			t.Errorf("last sweep = %v, want it anchored to the start, no earlier than %v", deps.lastHookSweep, started)
		}
	})
}
