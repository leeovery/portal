package saver_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// kill kills the session as `portal kill` does: the session first, then its
// entry, forgetting what went with it under the same lock.
func (w *world) kill(name string) {
	w.t.Helper()
	if err := w.fake.Kill(w.t.Context(), name); err != nil {
		w.t.Fatalf("Kill(%s): %v", name, err)
	}
	if _, err := sessionindex.UpdateThen(w.dir, "cli", func(idx *sessionindex.Index) error {
		idx.Remove(name)
		return nil
	}, func(outcome sessionindex.Outcome) error {
		state.Forget(w.dir, nil, "cli", outcome.Removed...)
		return nil
	}); err != nil {
		w.t.Fatalf("remove %s's entry: %v", name, err)
	}
}

// mint creates a session and claims its entry as `portal open` does,
// returning the session.
func (w *world) mint(name string) backend.Session {
	w.t.Helper()
	s, err := w.fake.Create(w.t.Context(), name, "/proj/"+name, nil)
	if err != nil {
		w.t.Fatalf("Create(%s): %v", name, err)
	}
	w.claim(s, "/proj/"+name)
	return s
}

func TestPoll_KillRaces(t *testing.T) {
	t.Run("a kill between the listing and the commit doesn't bring the session back", func(t *testing.T) {
		w := newWorld(t)
		stageCaptured(w, "the screen")
		w.race.afterList = func() { w.kill("api") }

		w.after(time.Minute)

		if got := w.names(); len(got) != 0 {
			t.Errorf("entries = %v, want api's kill to stand", got)
		}
	})

	t.Run("a kill during the capture doesn't bring the session back, nor keep what was captured", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "the screen")
		w.fake.SetHistory("api", []byte("the last screen"))
		w.race.afterHistory = func(string) { w.kill("api") }

		w.after(time.Minute)

		if got := w.names(); len(got) != 0 {
			t.Errorf("entries = %v, want api's kill to stand", got)
		}
		if got, ok := w.read(scrollbackFile(t, api)); ok {
			t.Errorf("api's scrollback = %q, want what the capture wrote removed", got)
		}
	})
}

func TestPoll_RenameDuringCaptureKeepsItsLabel(t *testing.T) {
	w := newWorld(t)
	api := stageCaptured(w, "first")
	w.fake.SetHistory("api", []byte("second"))
	w.race.afterHistory = func(string) {
		w.update(func(idx *sessionindex.Index) {
			idx.Claim(api.Name, "", api.Created, api.PID, state.ClaimRule(w.dir))
			if err := idx.SetLabel("api", "Renamed"); err != nil {
				t.Fatal(err)
			}
		})
	}

	w.after(time.Minute)

	got, _ := w.entry("api")
	if got.Label != "Renamed" || got.ScrollbackFile != scrollbackFile(t, api) {
		t.Errorf("api = %+v, want the label set during the capture beside the scrollback it named", got)
	}
	if screen, _ := w.read(got.ScrollbackFile); screen != "second" {
		t.Errorf("scrollback = %q, want the second history", screen)
	}
}

func TestPoll_MintRaces(t *testing.T) {
	t.Run("a session minted between the listing and the commit is kept as minted, never dropped", func(t *testing.T) {
		w := newWorld(t)
		w.live("api", 1)
		w.poll()
		sink := logtest.Install(t)
		var minted []backend.Session
		w.race.afterList = func() {
			for _, name := range []string{"one", "two", "three"} {
				minted = append(minted, w.mint(name))
			}
		}

		w.after(5 * time.Second)
		for range 3 {
			w.after(5 * time.Second)
		}

		for _, s := range minted {
			want := entryOf(s)
			want.Dir, want.Cwd = "/proj/"+s.Name, s.Dir
			if got, ok := w.entry(s.Name); !ok || !sameEntry(got, want) {
				t.Errorf("%s = %+v, %v; want %+v", s.Name, got, ok, want)
			}
		}
		if held := sink.Records().Matching("sessions", "drops-held"); len(held) != 0 {
			t.Errorf("logged %d holds, want none: a listing older than a session can't show it missing", len(held))
		}
	})

	t.Run("a session made again within the second between the listing and the commit is claimed from the next listing", func(t *testing.T) {
		w := newWorld(t)
		listed := w.live("api", 1)
		again := listed
		again.PID = 4999
		w.race.afterList = func() {
			w.fake.Remove("api")
			w.fake.Add(again)
		}

		w.poll()
		if got := w.names(); len(got) != 0 {
			t.Fatalf("entries = %v, want no claim of the creation the listing showed", got)
		}

		w.after(5 * time.Second)
		want := entryOf(again)
		want.Cwd = again.Dir
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want the creation made again %+v", got, want)
		}
	})

	t.Run("a mint replacing a session the listing showed is never replaced by that listing", func(t *testing.T) {
		w := newWorld(t)
		w.live("api", 1)
		w.poll()
		var minted backend.Session
		w.race.afterList = func() {
			w.fake.Remove("api")
			minted = w.mint("api")
		}

		w.after(5 * time.Second)

		want := entryOf(minted)
		want.Dir = "/proj/api"
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want the minted creation's entry %+v", got, want)
		}
	})
}

func TestPoll_RestoreRaces(t *testing.T) {
	saved := func(w *world) sessionindex.Entry {
		e := labelled(w.beforeBoot(), 1717)
		w.seed(e)
		w.write(e.ScrollbackFile, "the saved transcript")
		return e
	}
	restore := func(w *world) backend.Session {
		w.raise(state.Hydrating, "api")
		s, err := w.fake.Create(w.t.Context(), "api", "/minted", nil)
		if err != nil {
			w.t.Fatalf("Create: %v", err)
		}
		return s
	}
	restored := func(w *world, s backend.Session) {
		w.update(func(idx *sessionindex.Index) { idx.Restored(s.Name, s.Created, s.PID) })
	}

	t.Run("a restore landing between the listing and the commit is honoured", func(t *testing.T) {
		w := newWorld(t)
		e := saved(w)
		var made backend.Session
		w.race.afterList = func() {
			made = restore(w)
			restored(w, made)
		}

		w.poll()
		for range 3 {
			w.after(5 * time.Second)
		}

		want := e
		want.CreatedAt, want.PID, want.Cwd = made.Created, made.PID, made.Dir
		if got, ok := w.entry("api"); !ok || !sameEntry(got, want) {
			t.Errorf("api = %+v, %v; want the restored creation %+v, keeping all it was saved with", got, ok, want)
		}
		if _, ok := w.read(e.ScrollbackFile); !ok {
			t.Error("the saved transcript went while the session hydrates")
		}
	})

	t.Run("a restore recorded between the listing and the commit, of a session the listing showed, is honoured", func(t *testing.T) {
		w := newWorld(t)
		e := saved(w)
		made := restore(w)
		w.race.afterList = func() { restored(w, made) }

		w.poll()

		want := e
		want.CreatedAt, want.PID, want.Cwd = made.Created, made.PID, made.Dir
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want the restored creation %+v, keeping all it was saved with", got, want)
		}
	})
}

func TestPoll_FlagRaisedDuringTheCaptureLeavesItUnnamed(t *testing.T) {
	w := newWorld(t)
	api := w.live("api", 1)
	w.fake.SetHistory("api", []byte("the screen"))
	w.race.afterHistory = func(string) { w.raise(state.ResumePending, "api") }

	w.poll()

	if got, _ := w.entry("api"); got.ScrollbackFile != "" {
		t.Errorf("ScrollbackFile = %q, want nothing named for a session flagged since its capture", got.ScrollbackFile)
	}
	if got, ok := w.read(scrollbackFile(t, api)); ok {
		t.Errorf("the capture = %q, want it removed, no entry naming it", got)
	}

	w.lower(state.ResumePending, "api")
	w.after(time.Minute)

	if got, _ := w.read(scrollbackFile(t, api)); got != "the screen" {
		t.Errorf("the capture once the flag went = %q, want the same history written again", got)
	}
	if got, _ := w.entry("api"); got.ScrollbackFile != scrollbackFile(t, api) {
		t.Errorf("ScrollbackFile once the flag went = %q, want the capture named", got.ScrollbackFile)
	}
}

func TestPoll_CreationChangedUnderTheLockLeavesTheCaptureUnnamed(t *testing.T) {
	w := newWorld(t)
	api := stageCaptured(w, "first")
	w.fake.SetHistory("api", []byte("the old creation's last screen"))
	var minted backend.Session
	w.race.afterHistory = func(string) {
		w.fake.Remove("api")
		minted = w.mint("api")
	}

	w.after(time.Minute)

	if got, _ := w.entry("api"); !got.CreatedAt.Equal(minted.Created) || got.ScrollbackFile != "" {
		t.Errorf("api = %+v, want the minted creation's entry, naming nothing of the old", got)
	}
	if got, ok := w.read(scrollbackFile(t, api)); ok {
		t.Errorf("the old creation's capture = %q, want it removed, no entry naming it", got)
	}
	if got := w.names(); !slices.Equal(got, []string{"api"}) {
		t.Errorf("entries = %v, want api alone", got)
	}
}
