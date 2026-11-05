package restore_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
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
		if !strings.Contains(rec.AttrString(t, "error"), "never answered") || rec.AttrString(t, "detail") != "may be tried again within this bootstrap" {
			t.Errorf("failure = %v, want the unconfirmed session named, and that it may be tried again", rec.Attrs)
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

	// Create may already have made the session, so what settles it outlasts
	// the bootstrap's context.
	t.Run("a context ending before the second ask still gets that ask's answer", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		ctx, cancel := context.WithCancelCause(t.Context())
		unconfirmed(w, func(ctx context.Context, name, dir string, command []string) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				t.Fatal(err)
			}
			cancel(errors.New("bootstrap abandoned"))
		})

		if res, want := w.r.Restore(ctx), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		w.assertRestoredWithoutWarning(t, saved)
	})

	t.Run("a session that can't be looked up on the second ask fails, its flag kept, asked for again but never made again", func(t *testing.T) {
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
		if tries, asks := len(w.created()), len(w.fake.CallsTo(backendtest.OpSession)); tries != 1 || asks < 2 {
			t.Errorf("Create was tried %d times, asking %d times; want one try, its retries asking without making", tries, asks)
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
		if rec.AttrString(t, "detail") != "may be tried again within this bootstrap" {
			t.Errorf("failure = %v, want it to say it may be tried again", rec.Attrs)
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

	// decision 72, amended: restore never raises a flag on a name unless a
	// lookup has just found no session there.
	t.Run("a retry whose lookup can't tell raises nothing, so a flag the helper lowered stays down", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if len(w.created()) > 1 {
				return w.fake.Create(ctx, name, dir, command)
			}
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				t.Fatal(err)
			}
			w.fake.StopAnswering(name)
			return backend.Session{}, errTimedOut
		})
		w.asleep = func() {
			switch len(w.slept) {
			case 1: // its helper replayed, claimed and lowered the flag; the session is slow to answer still
				if _, err := state.Hydrating.Clear(w.dir, "work"); err != nil {
					t.Fatal(err)
				}
			case 2:
				live, _ := w.fake.Held("work")
				w.fake.Add(live)
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if w.hydrating(t, "work") {
			t.Error("a retry raised again the flag the session's helper had lowered")
		}
		if tries := len(w.created()); tries != 1 {
			t.Errorf("Create was tried %d times, want once: no retry made what it couldn't look up", tries)
		}
	})

	for name, holder := range map[string]backend.Session{
		"another program holds":                        {Name: "work", Command: "vim notes.md"},
		"another program holds under another spelling": {Name: "WORK", Command: "vim notes.md"},
	} {
		t.Run("a name "+name+" by the retry fails for good, nothing raised and nothing made", func(t *testing.T) {
			w := newWorld(t)
			w.save(t, entry(t, "work"))
			w.fake.Fail(backendtest.OpCreate, errRefused)
			w.asleep = func() {
				if len(w.slept) == 2 { // the first round's wait, past the first try's second ask
					w.fake.Add(holder)
				}
			}

			if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
				t.Errorf("Restore = %+v, want %+v", res, want)
			}
			if tries := len(w.created()); tries != 1 {
				t.Errorf("Create was tried %d times, want once", tries)
			}
			if w.hydrating(t, "work") {
				t.Error("the hydrating flag is up on another program's session")
			}
		})
	}

	for name, tc := range map[string]struct {
		since func(t *testing.T, w *world, saved sessionindex.Entry)
		why   string
	}{
		// A helper whose replay failed claims the entry and leaves it owed,
		// so only the creation it records tells it from the entry restore read.
		"claimed by its helper, its replay failed": {func(t *testing.T, w *world, saved sessionindex.Entry) {
			saved.CreatedAt, saved.PID, saved.Owed = booted.Add(2*time.Hour), 4242, true
			w.save(t, saved)
		}, "its entry records another creation"},
		"claimed by its helper, its replay done": {func(t *testing.T, w *world, saved sessionindex.Entry) {
			saved.CreatedAt, saved.PID = booted.Add(2*time.Hour), 4242
			w.save(t, saved)
		}, "its entry records another creation"},
		"dropped by the user's end": {func(t *testing.T, w *world, _ sessionindex.Entry) { w.save(t) }, "its entry is gone"},
	} {
		t.Run("an entry "+name+" between rounds is restore's no more, never made again", func(t *testing.T) {
			w := newWorld(t)
			saved := entry(t, "work")
			w.save(t, saved)
			w.fake.Fail(backendtest.OpCreate, errRefused)
			w.asleep = func() {
				if len(w.slept) == 2 {
					tc.since(t, w, saved)
				}
			}
			before := w.indexFile(t)

			if res := w.r.Restore(t.Context()); res != (restore.Result{}) {
				t.Errorf("Restore = %+v, want nothing restored and nothing failed", res)
			}
			if tries := len(w.created()); tries != 1 {
				t.Errorf("Create was tried %d times, want once", tries)
			}
			if w.indexFile(t) == before {
				t.Fatal("the entry never changed under the retry; the test is vacuous")
			}
			skip := w.sink.Records().WithMessage("session skipped").AtExactLevel(slog.LevelDebug).Only(t, "skip")
			if skip.AttrString(t, "session") != "work" || skip.AttrString(t, "reason") != tc.why {
				t.Errorf("skip = %v, want work skipped as %q", skip.Attrs, tc.why)
			}
		})
	}

	t.Run("a context ending during the last round's tries is warned of, the sessions left failed", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		ctx, cancel := context.WithCancelCause(t.Context())
		cause := errors.New("bootstrap abandoned")
		w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
			if len(w.created()) == 6 { // at 13 s, the last round no wait can follow
				cancel(cause)
			}
			w.fake.Add(backend.Session{Name: name})
			w.fake.StopAnswering(name)
			return backend.Session{}, errTimedOut
		})
		w.asleep = func() { w.fake.Remove("work") }

		if res, want := w.r.Restore(ctx), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if tries := len(w.created()); tries != 6 {
			t.Fatalf("Create was tried %d times, want 6: the cancel never reached the last round", tries)
		}
		rec := w.sink.Records().WithMessage("restore cancelled").AtExactLevel(slog.LevelWarn).Only(t, "cancellation")
		if !errors.Is(rec.ErrorAttr(t, "error"), cause) {
			t.Errorf("cancellation = %v, want its cause", rec.Attrs)
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
		w.stickCreates(&tried, 0)

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
		w.stickCreates(&tried, 5*time.Second)

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

// zmx finds a session by statting its socket, so on a filesystem that folds
// case the socket a session WORK left answers a lookup of work as held under
// WORK. Create replaces such a socket, so it holds nothing, and a Create that
// failed beside it may pass.
func TestRestore_StaleSocketUnderAnotherSpelling(t *testing.T) {
	for name, firstTry := range map[string]func(name string) (backend.Session, error){
		"that failed": func(string) (backend.Session, error) { return backend.Session{}, errTimedOut },
		"reporting a session made that never answered": func(name string) (backend.Session, error) {
			return backend.Session{Name: name}, nil
		},
	} {
		t.Run("a Create "+name+" is tried again, not failed for good", func(t *testing.T) {
			w := newWorld(t)
			w.save(t, entry(t, "work"))
			w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
				if len(w.created()) == 1 {
					return firstTry(name)
				}
				return w.fake.Create(ctx, name, dir, command)
			})
			w.answerSession("work", errHeldUnderWORK)

			if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
				t.Errorf("Restore = %+v, want %+v", res, want)
			}
			if tries := len(w.created()); tries != 2 {
				t.Errorf("Create was tried %d times, want twice", tries)
			}
			rec := w.failureLogged(t, "work")
			if detail := rec.AttrString(t, "detail"); detail != "may be tried again within this bootstrap" || strings.Contains(rec.AttrString(t, "error"), "held as") {
				t.Errorf("failure = %v, want one that may be tried again, naming no holder", rec.Attrs)
			}
		})
	}
}

// Restore's count is checked against a listing after its first pass and
// another once its retries end, so it says neither more nor less than it made.
func TestRestore_CountsWhatTheListingsShow(t *testing.T) {
	// A helper that dies before its claim, on a panic or an exec that fails,
	// ends its session at once, its entry still owed.
	t.Run("a session whose helper died before its claim is warned of, and made again", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.witness.listing = func(n int) {
			if n == 2 { // the listing after the first pass
				w.fake.Remove("work")
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"work", "work"}) {
			t.Errorf("created %q, want work made again", got)
		}
		rec := w.failureLogged(t, "work")
		if !strings.Contains(rec.AttrString(t, "error"), "ended before its helper finished") || rec.AttrString(t, "detail") != "may be tried again within this bootstrap" {
			t.Errorf("failure = %v, want the helper's death named, and that it may be tried again", rec.Attrs)
		}
		live, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(saved, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(saved, live))
		}
	})

	t.Run("a session whose helper died, and no retry could make again, counts failed, its entry owed", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.witness.listing = func(n int) {
			if n == 2 {
				w.fake.Remove("work")
				w.fake.Fail(backendtest.OpCreate, errRefused)
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got, ok := w.lookup(t, "work"); !ok || !got.Owed {
			t.Errorf("entry = %+v, %v; want it kept, owed", got, ok)
		}
	})

	// The user can close a session the moment its helper has claimed it.
	t.Run("a session that ended once its helper claimed it counts restored, never made again", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.witness.listing = func(n int) {
			if n == 2 {
				live, _ := w.fake.Held("work")
				claimed := restoredAs(saved, live)
				claimed.Owed = false
				w.save(t, claimed)
				w.fake.Remove("work")
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"work"}) {
			t.Errorf("created %q, want work made once", got)
		}
		if warnings := w.warnings(); len(warnings) > 0 {
			t.Errorf("logged %v, want no warning", warnings)
		}
	})

	// Under load a Create can time out after zmx made the session, and every
	// lookup after it time out too, while the session runs its helper.
	t.Run("a session no lookup could settle that the last listing finds running restore's helper counts restored", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				t.Fatal(err)
			}
			w.fake.StopAnswering(name)
			return backend.Session{}, errTimedOut
		})
		w.witness.listing = func(n int) {
			if live, ok := w.fake.Held("work"); n > 1 && ok { // the listing once the retries end
				w.fake.Add(live)
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"work"}) {
			t.Errorf("created %q, want work made once", got)
		}
		live, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(saved, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(saved, live))
		}
		if !w.hydrating(t, "work") {
			t.Error("the hydrating flag came down under the helper, which lowers it")
		}
	})

	t.Run("a listing after the first pass that fails is warned of, leaving the count as it stands", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.witness.listing = func(n int) {
			if n == 2 {
				w.fake.Fail(backendtest.OpList, errRefused)
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		rec := w.sink.Records().WithMessage("list sessions failed").AtExactLevel(slog.LevelWarn).Only(t, "the listing's failure")
		if !errors.Is(rec.ErrorAttr(t, "error"), errRefused) || !strings.Contains(rec.AttrString(t, "detail"), "unchecked") {
			t.Errorf("failure = %v, want the listing's error, saying what it leaves unchecked", rec.Attrs)
		}
	})

	t.Run("a session the last listing finds running anything else stays failed", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.fake.Fail(backendtest.OpCreate, errRefused)
		w.witness.listing = func(n int) {
			if n > 1 {
				w.fake.Add(backend.Session{Name: "work", Answered: true, Command: "vim notes.md"})
			}
		}

		if res, want := w.r.Restore(t.Context()), (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
	})
}

// decision 75: a Create the backend refuses as a session of a protocol Portal
// doesn't support stops the retries as it stops the first pass, since every
// Create after it would make another, and the refused name is never looked up
// again.
func TestRestore_RetriesStopOnASessionOfAnUnsupportedProtocol(t *testing.T) {
	second := time.Second
	for name, tc := range map[string]struct {
		// refused is the Create, counted from 1, the backend refuses; every
		// other fails for a reason a retry may clear.
		refused int
		created []string
		slept   []time.Duration
	}{
		"refused on the first pass, nothing is tried again": {
			refused: 2, created: []string{"a", "b"}, slept: []time.Duration{second},
		},
		"refused in a retry, nothing more is tried": {
			refused: 3, created: []string{"a", "b", "a"}, slept: []time.Duration{second, second, second},
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.save(t, entry(t, "a"), entry(t, "b"))
			lookups := -1
			w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
				if len(w.created()) == tc.refused {
					lookups = len(w.fake.CallsTo(backendtest.OpSession))
					return backend.Session{}, fmt.Errorf("create session %q: %w", name, backend.ErrUnsupportedProtocol)
				}
				return backend.Session{}, errRefused
			})

			res := w.r.Restore(t.Context())

			if !errors.Is(res.Unsupported, backend.ErrUnsupportedProtocol) || res.Failed != 2 || res.Restored != 0 {
				t.Errorf("Restore = %+v, want both failed, carrying the refusal", res)
			}
			if lookups < 0 {
				t.Fatal("no Create was refused")
			}
			if after := len(w.fake.CallsTo(backendtest.OpSession)) - lookups; after != 0 {
				t.Errorf("looked sessions up %d times after the refusal, want none", after)
			}
			if got := w.created(); !slices.Equal(got, tc.created) {
				t.Errorf("created %q, want %q: nothing made after the refusal", got, tc.created)
			}
			if !slices.Equal(w.slept, tc.slept) {
				t.Errorf("waited %v, want %v: no round after the refusal", w.slept, tc.slept)
			}
			for _, name := range []string{"a", "b"} {
				if w.hydrating(t, name) {
					t.Errorf("%s's hydrating flag is up, want it down: no session runs the helper", name)
				}
				if _, ok := w.lookup(t, name); !ok {
					t.Errorf("%s's entry is gone, want it kept for the next restore", name)
				}
			}
			for _, rec := range w.sink.Records().WithMessage("restore session failed") {
				if errors.Is(rec.ErrorAttr(t, "error"), backend.ErrUnsupportedProtocol) && rec.HasAttr("detail") {
					t.Errorf("the refusal's failure = %v, want nothing said of trying it again", rec.Attrs)
				}
			}
		})
	}
}

// stickCreates makes each Create take took on the world's clock, recording
// the time it started in tried, and fail as a session that can't be looked up
// does: a failure a retry may clear, judged with no wait. The wedged session
// it leaves is gone by the next wait, so each round finds the name free.
func (w *world) stickCreates(tried *[]time.Time, took time.Duration) {
	wedged := map[string]bool{}
	w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
		*tried = append(*tried, w.clock)
		w.clock = w.clock.Add(took)
		w.fake.Add(backend.Session{Name: name})
		w.fake.StopAnswering(name)
		wedged[name] = true
		return backend.Session{}, errTimedOut
	})
	w.asleep = func() {
		for name := range wedged {
			w.fake.Remove(name)
		}
		clear(wedged)
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
