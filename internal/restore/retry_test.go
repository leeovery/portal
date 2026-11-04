package restore_test

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/state"
)

// decision 72: zmx reports a session made before its daemon has the session's
// pty, so a Create that returns no creation may have made nothing. Restore
// asks again a probe's ceiling on before it counts the session.
func TestRestore_UnconfirmedCreate(t *testing.T) {
	// unconfirmed is a Create that reports the session made, by its name
	// alone, having done make, if given, to the backend.
	unconfirmed := func(w *world, make func(ctx context.Context, name, dir string, command []string)) {
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if make != nil {
				make(ctx, name, dir, command)
			}
			return backend.Session{Name: name}, nil
		})
	}

	t.Run("a session gone by the second ask fails, its flag lowered, and is tried again", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		unconfirmed(w, nil)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if asks, tries := len(w.fake.CallsTo(backendtest.OpSession)), len(w.created()); asks != 2*tries-1 || len(w.slept) == 0 || w.slept[0] != time.Second {
			t.Errorf("asked %d times in %d tries, waiting %v; want each try to ask once a probe's ceiling on, each retry asking first besides", asks, tries, w.slept)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag of a session never made is left up")
		}
		if tries := len(w.created()); tries < 2 {
			t.Errorf("Create was tried %d times, want it tried again", tries)
		}
		rec := w.failureLogged(t, "work")
		if !strings.Contains(rec.AttrString(t, "error"), "never answered") || rec.AttrString(t, "detail") != "tried again within this bootstrap" {
			t.Errorf("failure = %v, want the unconfirmed session named, and tried again", rec.Attrs)
		}
	})

	t.Run("a session gone by the second ask is restored by a retry that makes it", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if len(w.created()) == 1 {
				return backend.Session{Name: name}, nil
			}
			return w.fake.Create(ctx, name, dir, command)
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		live, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(saved, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(saved, live))
		}
	})

	t.Run("a session there on the second ask, running restore's helper, is restored, its creation that ask's", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		unconfirmed(w, func(ctx context.Context, name, dir string, command []string) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				t.Fatal(err)
			}
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if !w.hydrating(t, "work") {
			t.Error("the hydrating flag came down under the helper, which lowers it")
		}
		if !slices.Equal(w.slept, []time.Duration{time.Second}) {
			t.Errorf("waited %v, want one probe ceiling", w.slept)
		}
		w.assertRestoredWithoutWarning(t, saved)
	})

	t.Run("a session there on the second ask, running anything else, fails for good, its flag lowered", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		unconfirmed(w, func(_ context.Context, name, _ string, _ []string) {
			w.fake.Add(backend.Session{Name: name, Command: "vim notes.md"})
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag on another's session is left up")
		}
		if tries := len(w.created()); tries != 1 {
			t.Errorf("Create was tried %d times, want once: another session holds the name", tries)
		}
	})

	t.Run("a session that can't be looked up on the second ask fails, its flag kept, and is tried again", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		unconfirmed(w, func(ctx context.Context, name, dir string, command []string) {
			if _, err := w.fake.Create(ctx, name, dir, command); err == nil {
				w.fake.StopAnswering(name)
			}
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if !w.hydrating(t, "work") {
			t.Error("the hydrating flag of a session that may be the helper's came down")
		}
		if tries := len(w.created()); tries < 2 {
			t.Errorf("Create was tried %d times, want it tried again", tries)
		}
	})
}

// decision 73: restore tries again, within one fixed budget, each session its
// first pass failed for a reason that may pass.
func TestRestore_Retries(t *testing.T) {
	t.Run("a failure that clears is restored on a retry, after every session's first try, warned of once", func(t *testing.T) {
		w := newWorld(t)
		work, notes := entry(t, "work"), entry(t, "notes")
		w.save(t, work, notes)
		failed := false
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if name == "work" && !failed {
				failed = true
				return backend.Session{}, errRefused
			}
			return w.fake.Create(ctx, name, dir, command)
		})

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 2}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"work", "notes", "work"}) {
			t.Errorf("created %q, want work tried again once notes had its first try", got)
		}
		rec := w.failureLogged(t, "work")
		if rec.AttrString(t, "detail") != "tried again within this bootstrap" {
			t.Errorf("failure = %v, want it to say it is tried again", rec.Attrs)
		}
		if n := len(w.sink.Records().WithMessage("restore retry failed")); n != 0 {
			t.Errorf("logged %d retries failing, want none", n)
		}
		live, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(work, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(work, live))
		}
	})

	t.Run("a session an earlier try made after all is restored by the retry as it stands, its flag never raised again", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				t.Fatal(err)
			}
			w.fake.StopAnswering(name)
			return backend.Session{}, errors.New("zmx: timed out waiting for the session")
		})
		// The session answers once its helper has run, which lowered its flag.
		w.asleep = func() {
			if live, ok := w.fake.Held("work"); ok && !live.Answered {
				w.fake.Add(live)
				if _, err := state.Hydrating.Clear(w.dir, "work"); err != nil {
					t.Fatal(err)
				}
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if tries := len(w.created()); tries != 1 {
			t.Errorf("Create was tried %d times, want once: the retry found the session made", tries)
		}
		if w.hydrating(t, "work") {
			t.Error("the retry raised again the flag the session's helper had lowered")
		}
		live, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(saved, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(saved, live))
		}
	})

	t.Run("a name Create refuses is never tried again, and waits for nothing", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "foo*"))

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if len(w.slept) > 0 {
			t.Errorf("waited %v, want no retry", w.slept)
		}
	})

	t.Run("a name another session holds is never tried again", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
			w.fake.Add(backend.Session{Name: name, Command: "vim notes.md"})
			return backend.Session{}, &backend.SessionExistsError{Name: name, Held: name}
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if tries := len(w.created()); tries != 1 || len(w.slept) > 0 {
			t.Errorf("tried %d times, waiting %v; want one try and no wait", tries, w.slept)
		}
	})

	t.Run("its rounds wait 1 s, then 3 s, each started only while the budget has room for its wait", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		start := w.clock
		var tried []time.Time
		w.createWith(stuckCreate(w, &tried, 0))

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		second := time.Second
		if want := []time.Duration{0, second, 4 * second, 7 * second, 10 * second, 13 * second}; !slices.Equal(offsets(tried, start), want) {
			t.Errorf("tries at %v, want %v: none once a wait would pass %s", offsets(tried, start), want, restore.RetryBudget)
		}
		if want := []time.Duration{second, 3 * second, 3 * second, 3 * second, 3 * second}; !slices.Equal(w.slept, want) {
			t.Errorf("waited %v, want %v", w.slept, want)
		}
		if n := len(w.sink.Records().WithMessage("restore retry failed").AtExactLevel(slog.LevelDebug)); n != 5 {
			t.Errorf("logged %d retries failing, want each of the 5 at DEBUG", n)
		}
		if got := w.anomalous(t); got != 1 {
			t.Errorf("anomalous = %d, want the first failure's WARN alone", got)
		}
	})

	// A try under way is never cut short, so the budget runs from the first
	// pass's end and bounds when a try may start, never when it ends.
	t.Run("the budget runs from the first pass's end, and starts no try once spent", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "a"), entry(t, "b"))
		start := w.clock
		var tried []time.Time
		w.createWith(stuckCreate(w, &tried, 5*time.Second))

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 2}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		second := time.Second
		if want := []time.Duration{0, 5 * second, 11 * second, 16 * second, 24 * second}; !slices.Equal(offsets(tried, start), want) {
			t.Errorf("tries at %v, want %v: b's last try would start past the budget", offsets(tried, start), want)
		}
	})

	t.Run("a context ending during a wait stops the retries, the sessions left failed", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.fake.Fail(backendtest.OpCreate, errRefused)
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("bootstrap abandoned")
		w.asleep = func() {
			if len(w.slept) == 2 {
				cancel(cause)
			}
		}

		if res, want := w.r.Restore(ctx), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if tries := len(w.created()); tries != 1 {
			t.Errorf("Create was tried %d times, want none after the context ended", tries)
		}
		rec := w.sink.Records().WithMessage("restore cancelled").AtExactLevel(slog.LevelWarn).Only(t, "cancellation")
		if !errors.Is(rec.ErrorAttr(t, "error"), cause) {
			t.Errorf("cancellation = %v, want its cause", rec.Attrs)
		}
	})
}

// stuckCreate is a Create each of whose calls takes took on the world's
// clock, recording the time it started in tried, and fails as a session that
// can't be looked up does: a failure a retry may clear, judged with no wait.
func stuckCreate(w *world, tried *[]time.Time, took time.Duration) func(context.Context, string, string, []string) (backend.Session, error) {
	return func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
		*tried = append(*tried, w.clock)
		w.clock = w.clock.Add(took)
		if _, held := w.fake.Held(name); !held {
			w.fake.Add(backend.Session{Name: name})
			w.fake.StopAnswering(name)
		}
		return backend.Session{}, errTimedOut
	}
}

// offsets is each of tried, a clock reading, as an offset from start.
func offsets(tried []time.Time, start time.Time) []time.Duration {
	var out []time.Duration
	for _, at := range tried {
		out = append(out, at.Sub(start))
	}
	return out
}
