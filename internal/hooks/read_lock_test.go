package hooks_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
)

// numberedEntries names n hooks, so a read fixture that only cares how many
// entries come back need not author them.
func numberedEntries(n int) map[string]string {
	entries := make(map[string]string, n)
	for i := range n {
		entries[fmt.Sprintf("k%02d", i)] = fmt.Sprintf("cmd%02d", i)
	}
	return entries
}

func TestReadSharedLock(t *testing.T) {
	t.Run("it takes a shared lock for a read", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 2*time.Second)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(2)})
		hookstest.HoldHooksSidecarShared(t, path)

		sink := logtest.Install(t)
		start := time.Now()
		h, err := store.Load(hooks.ViaCLI)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(h) != 2 {
			t.Fatalf("got %d entries, want 2", len(h))
		}
		if elapsed >= 500*time.Millisecond {
			t.Fatalf("Load took %v against another shared holder — it acquired exclusively", elapsed)
		}
		if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
			t.Fatalf("read degraded despite a grantable shared lock: %+v", got)
		}
	})

	t.Run("it releases the shared lock before it returns", func(t *testing.T) {
		// Repeated reads would each leak a held fd, so an exclusive acquire
		// afterwards is granted only if every one of them released.
		hooks.SetLockTimeoutForTest(t, 2*time.Second)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})

		for range 3 {
			if _, err := store.Load(hooks.ViaCLI); err != nil {
				t.Fatalf("Load: %v", err)
			}
		}
		if _, err := store.List(hooks.ViaCLI); err != nil {
			t.Fatalf("List: %v", err)
		}
		if _, err := store.CleanStale(abortingEnumeration(nil)); !errors.Is(err, errAbortEnumeration) {
			t.Fatalf("CleanStale snapshot read: %v", err)
		}
		if _, _, err := store.LookupOnResume("k00", hooks.ViaHydrate); err != nil {
			t.Fatalf("LookupOnResume: %v", err)
		}

		hookstest.AssertSidecarFree(t, path)

		// A mutation follows without contending with a read that already
		// returned — the sweep's pre-read must not make its own CleanStale wait.
		if err := store.Set("k99", "on-resume", "cmd99", hooks.ViaCLI); err != nil {
			t.Fatalf("mutation after reads: %v", err)
		}
	})

	t.Run("it lets two reads proceed concurrently", func(t *testing.T) {
		// A shared hold from a third fd is the discriminator: both reads are
		// granted alongside it, so neither can reach the (low) bound. An
		// exclusive read would block out against the hold and degrade.
		hooks.SetLockTimeoutForTest(t, 300*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(4)})
		hookstest.HoldHooksSidecarShared(t, path)

		sink := logtest.Install(t)
		var wg sync.WaitGroup
		elapsed := make([]time.Duration, 2)
		errs := make([]error, 2)
		counts := make([]int, 2)
		start := time.Now()
		for i := range 2 {
			wg.Go(func() {
				began := time.Now()
				h, err := store.Load(hooks.ViaCLI)
				elapsed[i] = time.Since(began)
				errs[i] = err
				counts[i] = len(h)
			})
		}
		wg.Wait()
		total := time.Since(start)

		for i := range 2 {
			if errs[i] != nil {
				t.Fatalf("read %d: %v", i, errs[i])
			}
			if counts[i] != 4 {
				t.Errorf("read %d saw %d entries, want 4", i, counts[i])
			}
			if elapsed[i] >= 300*time.Millisecond {
				t.Errorf("read %d took %v — it blocked to the bound", i, elapsed[i])
			}
		}
		if total >= 300*time.Millisecond {
			t.Errorf("two overlapping reads took %v — they serialised", total)
		}
		if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
			t.Fatalf("a concurrent read degraded: %+v", got)
		}
	})

	t.Run("it reads anyway when the lock cannot be taken", func(t *testing.T) {
		bound := 60 * time.Millisecond
		hooks.SetLockTimeoutForTest(t, bound)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(3)})
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		start := time.Now()
		h, err := store.Load(hooks.ViaCLI)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Load returned an error under a held exclusive lock: %v", err)
		}
		if len(h) != 3 {
			t.Fatalf("got %d entries, want 3 — the data must come back regardless", len(h))
		}
		if elapsed < bound {
			t.Errorf("Load returned after %v — it did not wait out the %v bound", elapsed, bound)
		}
		hookstest.AssertDegradedRead(t, sink, "cli")
	})

	t.Run("it logs one DEBUG per degraded read", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(42)})
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		h, err := store.Load(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(h) != 42 {
			t.Fatalf("got %d entries, want 42", len(h))
		}
		hookstest.AssertDegradedRead(t, sink, "cli")
	})

	t.Run("it degrades when the sidecar is absent", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{
			Entries:       map[string]string{"k0": "cmd0"},
			SidecarAbsent: true,
		})

		sink := logtest.Install(t)
		h, err := store.Load(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if h["k0"]["on-resume"] != "cmd0" {
			t.Fatalf("entries did not come back: %v", h)
		}
		if _, statErr := os.Stat(path + ".lock"); !os.IsNotExist(statErr) {
			t.Fatalf("the degraded read created the sidecar: %v", statErr)
		}
		hookstest.AssertDegradedRead(t, sink, "cli")
	})

	t.Run("it creates nothing when it reads", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
		dir := filepath.Join(t.TempDir(), "absent")
		path := filepath.Join(dir, "hooks.json")

		h, err := hooks.NewStore(path).Load(hooks.ViaCLI)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if len(h) != 0 {
			t.Fatalf("got %d entries, want an empty map", len(h))
		}

		for _, p := range []string{dir, path, path + ".lock"} {
			if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
				t.Errorf("a read created %s: %v", p, statErr)
			}
		}
	})

	t.Run("it emits no load-unlocked record from inside a mutation", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "hooks.json")
		store := hooks.NewStore(path)

		sink := logtest.Install(t)
		if err := store.Set("k0", "on-resume", "cmd0", hooks.ViaCLI); err != nil {
			t.Fatalf("Set: %v", err)
		}
		if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
			t.Fatalf("a mutation's non-locking load emitted %+v — it is exclusive, not degraded", got)
		}
	})
}

func TestReadSharedLockBoundSelection(t *testing.T) {
	t.Run("it takes the bound from the parameter, not from via", func(t *testing.T) {
		// via=internal identifies the sweep's pre-read uniquely today, so a
		// via-driven branch would pass every other case here. An ordinary read
		// carrying that same value must still wait at lockTimeout.
		bound := 120 * time.Millisecond
		hooks.SetLockTimeoutForTest(t, bound)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})
		hookstest.HoldHooksSidecar(t, path)

		start := time.Now()
		if _, err := store.Load(hooks.ViaInternal); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if elapsed := time.Since(start); elapsed < bound {
			t.Errorf("Load(\"internal\") returned after %v — the bound was chosen from via, not from the parameter", elapsed)
		}
	})

	t.Run("it waits at the ordinary bound for every other read", func(t *testing.T) {
		bound := 120 * time.Millisecond
		hooks.SetLockTimeoutForTest(t, bound)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})
		hookstest.HoldHooksSidecar(t, path)

		reads := map[string]func() error{
			"Load": func() error { _, err := store.Load(hooks.ViaCLI); return err },
			"List": func() error { _, err := store.List(hooks.ViaCLI); return err },
			"LookupOnResume": func() error {
				_, _, err := store.LookupOnResume("k00", hooks.ViaHydrate)
				return err
			},
		}
		for name, read := range reads {
			t.Run(name, func(t *testing.T) {
				start := time.Now()
				if err := read(); err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if elapsed := time.Since(start); elapsed < bound {
					t.Errorf("%s returned after %v — it did not wait at lockTimeout (%v)", name, elapsed, bound)
				}
			})
		}
	})
}

func TestReadSharedLockVia(t *testing.T) {
	// Every read names its caller in the degradation breadcrumb, from the
	// caller's own Via — including the hydrate lookup.
	cases := []struct {
		name string
		via  hooks.Via
		read func(*hooks.Store) error
	}{
		{"Load", hooks.ViaCLI, func(s *hooks.Store) error { _, err := s.Load(hooks.ViaCLI); return err }},
		{"List", hooks.ViaDoctor, func(s *hooks.Store) error { _, err := s.List(hooks.ViaDoctor); return err }},
		{"CleanStale snapshot", hooks.ViaInternal, func(s *hooks.Store) error {
			if _, err := s.CleanStale(abortingEnumeration(nil)); !errors.Is(err, errAbortEnumeration) {
				return err
			}
			return nil
		}},
		{"LookupOnResume", hooks.ViaHydrate, func(s *hooks.Store) error {
			_, _, err := s.LookupOnResume("k00", hooks.ViaHydrate)
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hooks.SetLockTimeoutForTest(t, 20*time.Millisecond)
			store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})
			hookstest.HoldHooksSidecar(t, path)

			sink := logtest.Install(t)
			if err := tc.read(store); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			hookstest.AssertDegradedRead(t, sink, tc.via.String())
		})
	}
}

func TestLookupOnResumeUnderHeldLock(t *testing.T) {
	t.Run("it returns the hook for a hydrating pane while a mutation holds the lock", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 40*time.Millisecond)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{
			"tok01": "claude --resume abc",
		}})
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		cmd, ok, err := store.LookupOnResume("tok01", hooks.ViaHydrate)
		if err != nil {
			t.Fatalf("LookupOnResume returned an error under a held lock: %v", err)
		}
		if !ok || cmd != "claude --resume abc" {
			t.Fatalf("got (%q, %v), want the registered command — a busy lock must not drop a pane to a bare shell", cmd, ok)
		}
		hookstest.AssertDegradedRead(t, sink, "hydrate")
	})

	t.Run("an empty key takes no lock and logs nothing", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, time.Second)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		start := time.Now()
		cmd, ok, err := store.LookupOnResume("", hooks.ViaHydrate)
		elapsed := time.Since(start)

		if err != nil || ok || cmd != "" {
			t.Fatalf("got (%q, %v, %v), want the empty-key early return", cmd, ok, err)
		}
		if elapsed >= 100*time.Millisecond {
			t.Errorf("empty-key lookup took %v — it reached the acquire", elapsed)
		}
		if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
			t.Fatalf("empty-key lookup emitted %+v", got)
		}
	})
}

// errAbortEnumeration ends a clean at its enumeration, so a fixture measures
// the snapshot read that precedes it and nothing after.
var errAbortEnumeration = errors.New("abort the enumeration")

// abortingEnumeration records the snapshot's size, when entries is non-nil, and
// aborts.
func abortingEnumeration(entries *int) func(hooks.Snapshot) ([]string, error) {
	return func(snapshot hooks.Snapshot) ([]string, error) {
		if entries != nil {
			*entries = len(snapshot)
		}
		return nil, errAbortEnumeration
	}
}

// halfRelationCrossoverBound is the mutation bound the half-relation is tightest
// at: the fraction is already exhausted there, so the pre-read stands at exactly
// half the mutation bound and a comparison against a mutation bound halved in
// integer arithmetic reads the two as equal.
const halfRelationCrossoverBound = 10 * time.Millisecond

// sampledMutationBounds are the production mutation bound and the lowered
// figures a test drives it at.
func sampledMutationBounds() []time.Duration {
	return []time.Duration{2 * time.Second, 300 * time.Millisecond, 60 * time.Millisecond}
}

// pollIntervalFloor reports the floor the derived pre-read bound rests on. A
// mutation bound of a single nanosecond exhausts the fraction — it divides away
// to nothing — so what comes back is the floor and nothing else.
func pollIntervalFloor(t *testing.T) time.Duration {
	t.Helper()
	hooks.SetLockTimeoutForTest(t, time.Nanosecond)
	return hooks.SnapshotLockBoundForTest(t)
}

// assertHalfRelation pins the pre-read bound at no more than half the mutation
// bound, so a contended clean costs the daemon's tick one bound rather than two.
// Multiplying the pre-read rather than halving the mutation bound keeps the
// comparison off Go's integer division, which discards the remainder of an odd
// bound and would read a pre-read below half as equal to it.
func assertHalfRelation(t *testing.T, mutation time.Duration) {
	t.Helper()
	hooks.SetLockTimeoutForTest(t, mutation)
	preRead := hooks.SnapshotLockBoundForTest(t)
	if 2*preRead > mutation {
		t.Errorf("pre-read bound = %v at a %v mutation bound — a contended clean must cost one bound, not two", preRead, mutation)
	}
}

// The clean's advisory pre-read is bounded as a fraction of the mutation bound
// rather than at a figure of its own, so the relationship between the two is
// visible at the declaration and a test that lowers one lowers both.
func TestSnapshotLockBoundDerivation(t *testing.T) {
	t.Run("it pins the pre-read bound at the poll-interval floor", func(t *testing.T) {
		floor := pollIntervalFloor(t)
		if floor <= 0 {
			t.Fatalf("the floor is %v — a bound at or below zero is waited out before it is ever tested", floor)
		}
		for _, mutation := range append(sampledMutationBounds(), halfRelationCrossoverBound) {
			hooks.SetLockTimeoutForTest(t, mutation)
			if preRead := hooks.SnapshotLockBoundForTest(t); preRead < floor {
				t.Errorf("pre-read bound = %v at a %v mutation bound — it must not fall below the %v floor, under which the deadline is re-tested only after a poll sleep and the figure named stops being the figure waited", preRead, mutation, floor)
			}
		}
	})

	t.Run("it grants an uncontended acquire whatever the bound", func(t *testing.T) {
		for _, bound := range []time.Duration{2 * time.Second, 0, -time.Second} {
			hooks.SetLockTimeoutForTest(t, bound)
			store, _ := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(1)})

			sink := logtest.Install(t)
			h, err := store.Load(hooks.ViaCLI)
			if err != nil {
				t.Fatalf("Load at a %v bound: %v", bound, err)
			}
			if len(h) != 1 {
				t.Fatalf("got %d entries at a %v bound, want 1", len(h), bound)
			}
			if got := hookstest.UnlockedRecords(t, sink); len(got) != 0 {
				t.Errorf("a %v bound degraded an uncontended read to an unlocked one: %+v — the lock is granted before any deadline is tested, so no figure of the bound is what grants it", bound, got)
			}
		}
	})

	t.Run("it holds the half-relation at the production bound and the lowered test bounds", func(t *testing.T) {
		for _, mutation := range sampledMutationBounds() {
			assertHalfRelation(t, mutation)
		}
	})

	t.Run("it holds the half-relation at the integer-division crossover bound", func(t *testing.T) {
		assertHalfRelation(t, halfRelationCrossoverBound)
	})

	t.Run("it lowers the pre-read bound with the mutation bound under test", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, 2*time.Second)
		production := hooks.SnapshotLockBoundForTest(t)

		hooks.SetLockTimeoutForTest(t, 200*time.Millisecond)
		lowered := hooks.SnapshotLockBoundForTest(t)

		if lowered >= production {
			t.Errorf("pre-read bound stayed at %v when the mutation bound fell to 200ms (was %v at 2s)", lowered, production)
		}
	})

	t.Run("it degrades the pre-read to an unlocked read when the sidecar is held", func(t *testing.T) {
		hooks.SetLockTimeoutForTest(t, time.Second)
		preRead := hooks.SnapshotLockBoundForTest(t)
		store, path := hookstest.StageStore(t, hookstest.Staging{Entries: numberedEntries(2)})
		hookstest.HoldHooksSidecar(t, path)

		sink := logtest.Install(t)
		var entries int
		start := time.Now()
		// Aborted at the enumeration, so the elapsed time measures the pre-read
		// alone rather than the exclusive acquire that would follow it.
		_, err := store.CleanStale(abortingEnumeration(&entries))
		elapsed := time.Since(start)

		if !errors.Is(err, errAbortEnumeration) {
			t.Fatalf("CleanStale: %v", err)
		}
		if entries != 2 {
			t.Errorf("the snapshot held %d entries, want 2 — the degraded read still read the file", entries)
		}
		if elapsed < preRead {
			t.Errorf("the pre-read returned after %v — it did not wait out its %v bound", elapsed, preRead)
		}
		if elapsed >= 500*time.Millisecond {
			t.Errorf("the pre-read took %v — it waited at the mutation bound, not the derived one", elapsed)
		}
		hookstest.AssertDegradedRead(t, sink, "internal")
	})
}
