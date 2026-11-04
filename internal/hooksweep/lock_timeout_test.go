package hooksweep

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// lockedSweepFixture stages the stale seed under a sidecar held exclusively
// from an independent open file description, so the sweep's mutation contends
// with a writer it cannot see and times out at the lowered bound. The returned
// release frees the lock for a retry case.
func lockedSweepFixture(t *testing.T, bound time.Duration) (*hooks.Store, string, func()) {
	t.Helper()
	hooks.SetLockTimeoutForTest(t, bound)
	store, path := hookstest.StageStore(t, hookstest.Staging{Seed: hookstest.StaleHookSeed})
	return store, path, hookstest.HoldHooksSidecar(t, path)
}

func TestHookSweepStandsDownOnLockTimeout(t *testing.T) {
	src := newWorld(t, hookstest.LiveSeedA).sources()

	t.Run("it deletes nothing when the sweep cannot take the lock", func(t *testing.T) {
		store, path, _ := lockedSweepFixture(t, lockBound)
		before := hookstest.HooksFileBytes(t, path)

		outcome, err := Run(t.Context(), src, store)
		if err != nil {
			t.Fatalf("Run: want nil on a lock timeout, got %v", err)
		}
		if outcome.DeclineReason != ReasonLockTimeout || len(outcome.Removed) != 0 {
			t.Errorf("outcome = %+v, want a lock-timeout stand-down removing nothing", outcome)
		}

		hookstest.AssertHooksFileUnchanged(t, path, before, "rewritten under a held lock")
	})

	t.Run("it reports one stand-down per stood-down cycle", func(t *testing.T) {
		store, _, _ := lockedSweepFixture(t, lockBound)
		sink := logtest.Install(t)

		if err := runErr(t, src, store); err != nil {
			t.Fatalf("Run: %v", err)
		}

		rec := assertStandDown(t, sink, ReasonLockTimeout)
		if !errors.Is(rec.ErrorAttr(t, "error"), hooks.ErrLockHeld) {
			t.Errorf("stand-down error attr = %v, want the lock timeout", rec.ErrorAttr(t, "error"))
		}
	})

	// The sweep takes the sidecar twice, and only the mutation waits at the full
	// bound: the pre-read's near-zero snapshot bound is what keeps a contended
	// cycle from parking the daemon's 1s tick for two of them.
	t.Run("it costs one bound for a fully contended cycle", func(t *testing.T) {
		const bound = 300 * time.Millisecond
		store, _, _ := lockedSweepFixture(t, bound)

		start := time.Now()
		if err := runErr(t, src, store); err != nil {
			t.Fatalf("Run: %v", err)
		}
		elapsed := time.Since(start)

		if elapsed < bound {
			t.Errorf("cycle took %v, want at least one full bound of %v (the mutation must wait)", elapsed, bound)
		}
		if elapsed > bound*3/2 {
			t.Errorf("cycle took %v, want well under two bounds of %v (the pre-read must not wait at the full bound)", elapsed, 2*bound)
		}
	})

	t.Run("it retries on the next cadence", func(t *testing.T) {
		store, _, release := lockedSweepFixture(t, lockBound)

		if err := runErr(t, src, store); err != nil {
			t.Fatalf("Run under the lock: %v", err)
		}
		release()

		outcome, err := Run(t.Context(), src, store)
		if err != nil {
			t.Fatalf("Run after release: %v", err)
		}

		if len(outcome.Removed) != 1 || outcome.Removed[0] != hookstest.ReapableSeedA {
			t.Errorf("Removed = %v, want [%s]", outcome.Removed, hookstest.ReapableSeedA)
		}
		postRun, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatalf("store.Load post-run: %v", err)
		}
		if _, ok := postRun[hookstest.ReapableSeedA]; ok {
			t.Errorf("stale key %s survived the retry; got %v", hookstest.ReapableSeedA, keysOf(postRun))
		}
		if _, ok := postRun[hookstest.LiveSeedA]; !ok {
			t.Errorf("live key was reaped by the retry; got %v", keysOf(postRun))
		}
	})
}

func TestHookSweepDiscriminatesLockTimeoutFromFailure(t *testing.T) {
	src := newWorld(t, hookstest.LiveSeedA).sources()

	t.Run("it still returns an error for a save failure", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Dir:          filepath.Join(t.TempDir(), "write-denied"),
			Seed:         hookstest.StaleHookSeed,
			WritesDenied: true,
		})
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), src, store)
		if err == nil {
			t.Fatal("Run: want an error on a save failure, got nil")
		}
		if errors.Is(err, hooks.ErrLockHeld) {
			t.Errorf("save failure %v reported as a lock timeout", err)
		}
		// The failure the fixture exists to produce: the mutation took its lock
		// and read cleanly, then failed at the temp create. A staging change
		// that failed at the lock open instead would carry a different class.
		if !errors.Is(err, fileutil.ErrWriteTempCreate) {
			t.Errorf("err = %v, want a write-phase failure carrying %v", err, fileutil.ErrWriteTempCreate)
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none on a save failure", outcome.DeclineReason)
		}
		for _, rec := range sink.Records() {
			if rec.Msg == standDownMsg {
				t.Errorf("save failure emitted a stand-down record: %+v", rec)
			}
		}
	})

	// The directory name puts the sentinel's own words into the save failure's
	// message, so a substring check would mistake it for a lock timeout.
	t.Run("it never matches on error text", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{
			Dir:          filepath.Join(t.TempDir(), hooks.ErrLockHeld.Error()),
			Seed:         hookstest.StaleHookSeed,
			WritesDenied: true,
		})
		sink := logtest.Install(t)

		outcome, err := Run(t.Context(), src, store)
		if err == nil {
			t.Fatal("Run: want an error, got nil")
		}
		if !strings.Contains(err.Error(), hooks.ErrLockHeld.Error()) {
			t.Fatalf("fixture error %q does not carry the sentinel's text; the case measures nothing", err)
		}
		if outcome.DeclineReason != "" {
			t.Errorf("DeclineReason = %q, want none for a non-sentinel error", outcome.DeclineReason)
		}
		for _, rec := range sink.Records() {
			if rec.Msg == standDownMsg {
				t.Errorf("a non-sentinel error whose text carries the sentinel was treated as a stand-down: %+v", rec)
			}
		}
	})
}
