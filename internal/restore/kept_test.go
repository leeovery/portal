package restore_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// keptNames is the names of the entries Kept finds in the world's index, judged
// by booted, as its restorer judges them, with a saver running.
func (w *world) keptNames(t *testing.T) []string {
	t.Helper()
	return w.keptJudging(t, false)
}

// keptJudging is the names of the entries Kept finds in the world's index,
// judged by booted, with no saver running when noSaver is set.
func (w *world) keptJudging(t *testing.T, noSaver bool) []string {
	t.Helper()
	idx, err := sessionindex.Read(w.dir)
	if err != nil {
		t.Fatalf("read the index: %v", err)
	}
	live, err := w.fake.List(t.Context())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var names []string
	keeping := restore.Keeping{Rule: sessionindex.NewRule(booted, nil), Names: w.witness, NoSaver: noSaver}
	for _, e := range restore.Kept(t.Context(), idx.Entries(), live, keeping) {
		names = append(names, e.Name)
	}
	return names
}

// decision 78: a kept entry is exactly what the next restore makes again, so
// x <name> and doctor see what a bootstrap would bring back.
func TestKept_IsWhatRestoreMakes(t *testing.T) {
	w := newWorld(t, backend.Session{Name: "live"}, backend.Session{Name: "silent"}, backend.Session{Name: "Spelt"})
	w.fake.StopAnswering("silent")
	unrecorded := entry(t, "unrecorded")
	unrecorded.CreatedAt, unrecorded.PID = time.Time{}, 0
	w.save(t,
		entry(t, "live"),
		entry(t, "silent"),
		entry(t, "spelt"),
		entry(t, "_portal-saver"),
		thisBoot(t, "ended"),
		owing(thisBoot(t, "owed")),
		entry(t, "saved"),
		unrecorded,
		entry(t, "logs"),
	)

	kept := w.keptNames(t)
	w.r.Restore(t.Context())

	want := []string{"owed", "saved", "unrecorded"}
	if !slices.Equal(kept, want) {
		t.Errorf("Kept = %q, want %q: owed or from before the boot, not live under its name or another spelling, not Portal's own, under a name Create takes", kept, want)
	}
	if made := w.created(); !slices.Equal(kept, made) {
		t.Errorf("Kept = %q, but Restore made %q; want the same sessions", kept, made)
	}
}

func TestKept_AnotherSpelling(t *testing.T) {
	t.Run("a listed session holding the name under another spelling keeps it from being kept", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "WORK"})
		w.save(t, entry(t, "work"))

		if kept := w.keptNames(t); len(kept) != 0 {
			t.Errorf("Kept = %q, want nothing", kept)
		}
	})

	for name, answer := range otherSpellingAnswers {
		t.Run("a lookup that "+name+" keeps the entry kept", func(t *testing.T) {
			w := newWorld(t, backend.Session{Name: "WORK"})
			w.save(t, entry(t, "work"))
			w.answerSession("work", answer)

			if kept := w.keptNames(t); !slices.Equal(kept, []string{"work"}) {
				t.Errorf("Kept = %q, want work", kept)
			}
		})
	}

	t.Run("an entry no listed session spells another way costs no lookup", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "api"})
		w.save(t, entry(t, "work"))

		if kept := w.keptNames(t); !slices.Equal(kept, []string{"work"}) {
			t.Errorf("Kept = %q, want work", kept)
		}
		if asks := w.fake.CallsTo(backendtest.OpSession); len(asks) != 0 {
			t.Errorf("Session calls = %+v, want none", asks)
		}
	})
}

// decision 58: with no saver running, the next bootstrap marks owed every
// saved session no session holds, so all of them are kept.
func TestKept_WithNoSaverIsWhatTheBootstrapMakes(t *testing.T) {
	w := newWorld(t, backend.Session{Name: "live"}, backend.Session{Name: "Spelt"})
	w.save(t,
		thisBoot(t, "live"),
		entry(t, "spelt"),
		thisBoot(t, "_portal-saver"),
		thisBoot(t, "ended"),
		entry(t, "saved"),
		thisBoot(t, "logs"),
	)

	running, gone := w.keptNames(t), w.keptJudging(t, true)
	if err := w.r.MarkOwed(t.Context()); err != nil {
		t.Fatalf("MarkOwed: %v", err)
	}
	w.r.Restore(t.Context())

	if want := []string{"saved"}; !slices.Equal(running, want) {
		t.Errorf("Kept with a saver running = %q, want %q: the ended entry is the saver's to drop", running, want)
	}
	if want := []string{"ended", "saved"}; !slices.Equal(gone, want) {
		t.Errorf("Kept with no saver = %q, want %q: every entry not live under any spelling, not Portal's own, under a name Create takes", gone, want)
	}
	if made := w.created(); !slices.Equal(gone, made) {
		t.Errorf("Kept with no saver = %q, but the bootstrap's MarkOwed and Restore made %q; want the same sessions", gone, made)
	}
}

func TestBringBack(t *testing.T) {
	t.Run("it makes a kept session again as Restore does, owed until its helper finishes", func(t *testing.T) {
		w := newWorld(t)
		saved, transcript := w.saveTranscript(t, entry(t, "work"))
		w.save(t, saved)

		brought, err := w.r.BringBack(t.Context(), "work")

		if brought != restore.BroughtBack || err != nil {
			t.Fatalf("BringBack = %v, %v; want it brought back", brought, err)
		}
		call := w.onlyCreate(t)
		if want := []string{exe, "state", "hydrate", "--session", "work", "--file", transcript}; !slices.Equal(call.Command, want) || call.Dir != saved.Dir {
			t.Errorf("Create(%s, %q), want the helper %q in %s", call.Dir, call.Command, want, saved.Dir)
		}
		if !w.witness.raised["work"] || !w.hydrating(t, "work") {
			t.Errorf("hydrating flag raised before Create = %v, up after = %v; want both", w.witness.raised["work"], w.hydrating(t, "work"))
		}
		w.assertRestoredWithoutWarning(t, saved)
		w.assertSummary(t, "work", 1, 0)
	})

	t.Run("it makes an owed entry of this boot again", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, owing(thisBoot(t, "work")))

		if brought, err := w.r.BringBack(t.Context(), "work"); brought != restore.BroughtBack || err != nil {
			t.Errorf("BringBack = %v, %v; want it brought back", brought, err)
		}
	})

	t.Run("a session live by now is attached as it is, with no flag raised and nothing made", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.save(t, entry(t, "work"))

		brought, err := w.r.BringBack(t.Context(), "work")

		if brought != restore.AlreadyLive || err != nil {
			t.Errorf("BringBack = %v, %v; want it live already", brought, err)
		}
		w.assertSkipped(t, restore.Result{}, "work", "it is live")
		w.assertSummary(t, "work", 0, 0)
	})

	for _, tc := range []struct {
		name, reason string
		save         func(t *testing.T, w *world)
	}{
		{"an entry gone", "its entry is gone", func(t *testing.T, w *world) { w.save(t, entry(t, "notes")) }},
		{"an entry of this boot owed nothing", "it ended this boot", func(t *testing.T, w *world) { w.save(t, thisBoot(t, "work")) }},
		{"an entry under a name Create refuses", "its name is one the backend can never create", func(t *testing.T, w *world) {
			w.fake.Validate = func(string) error { return backend.ErrInvalidName }
			w.save(t, entry(t, "work"))
		}},
		{"an entry another spelling of whose name is live", "another spelling of its name is live", func(t *testing.T, w *world) {
			w.fake.Add(backend.Session{Name: "WORK"})
			w.save(t, entry(t, "work"))
		}},
	} {
		t.Run(tc.name+" is kept no longer", func(t *testing.T) {
			w := newWorld(t)
			tc.save(t, w)

			brought, err := w.r.BringBack(t.Context(), "work")

			if brought != restore.NoLongerKept || err != nil {
				t.Errorf("BringBack = %v, %v; want it kept no longer", brought, err)
			}
			w.assertSkipped(t, restore.Result{}, "work", tc.reason)
		})
	}

	for name, answer := range otherSpellingAnswers {
		t.Run("a lookup that "+name+" leaves it kept, and tried", func(t *testing.T) {
			w := newWorld(t, backend.Session{Name: "WORK"})
			w.save(t, entry(t, "work"))
			w.answerSession("work", answer)

			if brought, _ := w.r.BringBack(t.Context(), "work"); brought == restore.NoLongerKept {
				t.Error("BringBack = kept no longer, want it tried")
			}
			w.assertTried(t, "work")
		})
	}

	t.Run("one of Portal's own is kept no longer", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "_portal-saver"))

		if brought, err := w.r.BringBack(t.Context(), "_portal-saver"); brought != restore.NoLongerKept || err != nil {
			t.Errorf("BringBack = %v, %v; want it kept no longer", brought, err)
		}
		w.assertSkipped(t, restore.Result{}, "_portal-saver", "it is one of Portal's own")
	})

	t.Run("a failed try is one try, its entry left kept", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.fake.Fail(backendtest.OpCreate, errRefused)

		brought, err := w.r.BringBack(t.Context(), "work")

		if !errors.Is(err, errRefused) || brought != 0 {
			t.Fatalf("BringBack = %v, %v; want the refusal", brought, err)
		}
		if tries := len(w.created()); tries != 1 {
			t.Errorf("Create was tried %d times, want once: no retry within the bring-back", tries)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag it raised is still up, with no session to lower it")
		}
		w.assertFailedKeepingEntry(t, saved)
		if kept := w.keptNames(t); !slices.Equal(kept, []string{"work"}) {
			t.Errorf("Kept after the failure = %q, want work still kept", kept)
		}
		w.assertSummary(t, "work", 0, 1)
	})

	t.Run("a Create that reads no creation asks again, failing once the session is gone", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
			return backend.Session{Name: name}, nil
		})

		brought, err := w.r.BringBack(t.Context(), "work")

		if err == nil || brought != 0 {
			t.Fatalf("BringBack = %v, %v; want the session that never answered failed", brought, err)
		}
		if asks := len(w.fake.CallsTo(backendtest.OpSession)); asks != 1 || len(w.slept) != 1 {
			t.Errorf("asked again %d times after %v; want once, a probe's ceiling on", asks, w.slept)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag it raised is still up, with no session to lower it")
		}
		w.assertFailedKeepingEntry(t, saved)
	})

	t.Run("a refusal of a session of another protocol is returned as that refusal", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.fake.Fail(backendtest.OpCreate, fmt.Errorf("create session %q: %w", "work", backend.ErrUnsupportedProtocol))

		_, err := w.r.BringBack(t.Context(), "work")

		if !errors.Is(err, backend.ErrUnsupportedProtocol) {
			t.Fatalf("BringBack = %v, want the refusal", err)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag it raised is still up, with no session to lower it")
		}
		w.assertFailedKeepingEntry(t, saved)
	})

	t.Run("a hydrating flag up before it is left to whoever raised it", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		if err := state.Hydrating.Set(w.dir, "work"); err != nil {
			t.Fatal(err)
		}
		w.fake.Fail(backendtest.OpCreate, errRefused)

		if _, err := w.r.BringBack(t.Context(), "work"); err == nil {
			t.Fatal("BringBack = nil, want the refusal")
		}
		if !w.hydrating(t, "work") {
			t.Error("the hydrating flag raised before the bring-back came down")
		}
	})

	t.Run("an index it can't read is a failure, nothing asked of the backend", func(t *testing.T) {
		w := newWorld(t)
		w.writeIndex(t, "{")

		if _, err := w.r.BringBack(t.Context(), "work"); err == nil {
			t.Fatal("BringBack = nil, want the unreadable index")
		}
		if calls := w.fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
		w.sink.Records().WithMessage("sessions.json unreadable").AtExactLevel(slog.LevelWarn).Only(t, "the unreadable index")
	})

	t.Run("sessions it can't list are a failure, nothing made", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.fake.Fail(backendtest.OpList, errRefused)

		if _, err := w.r.BringBack(t.Context(), "work"); !errors.Is(err, errRefused) {
			t.Fatalf("BringBack = %v, want the listing's failure", err)
		}
		if made := w.created(); len(made) != 0 || w.hydrating(t, "work") {
			t.Errorf("created %q, flag up = %v; want nothing made and no flag raised", made, w.hydrating(t, "work"))
		}
		w.sink.Records().WithMessage("list sessions failed").AtExactLevel(slog.LevelWarn).Only(t, "the failed listing")
	})
}

// assertSummary checks the bring-back's one summary names the session and
// counts what it did.
func (w *world) assertSummary(t *testing.T, name string, restored, failed int64) {
	t.Helper()
	summary := w.sink.Records().WithMessage("restore complete").AtExactLevel(slog.LevelInfo).Only(t, "bring-back summary")
	if got := summary.AttrString(t, "session"); got != name {
		t.Errorf("summary names %q, want %q", got, name)
	}
	if summary.IntAttr(t, "restored") != restored || summary.IntAttr(t, "failed") != failed {
		t.Errorf("summary = %v, want %d restored and %d failed", summary.Attrs, restored, failed)
	}
	summary.RequireDuration(t, "took")
}
