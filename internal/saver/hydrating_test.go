package saver_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// decision 72: a restore completes at its helper's claim. Until then the
// session's hydrating flag is up and its entry owed: the saver doesn't repay
// it, and being owed keeps it from every drop.

// restored is api as restore leaves it: saved before this boot, made again by
// Create and recorded as that creation, owed, its hydrating flag raised, with
// the transcript the helper replays and a hook registered for it. web is live
// beside it, so api alone going is no mass vanish.
type restored struct {
	w     *world
	store *hooks.Store
	entry sessionindex.Entry
	made  backend.Session
}

func stageRestored(t *testing.T) *restored {
	t.Helper()
	store, _ := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{"api": "claude --resume"}})
	w := newWorld(t).withHooks(store)
	saved := labelled(w.beforeBoot(), 1717)
	w.seed(saved, entryOf(w.live("web", 2)))
	w.write(saved.ScrollbackFile, "the saved transcript")
	w.raise(state.Hydrating, "api")
	made, err := w.fake.Create(t.Context(), "api", "/minted", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	w.update(func(idx *sessionindex.Index) { idx.Restored(saved, made.Created, made.PID) })
	entry, _ := w.entry("api")
	if !entry.Owed {
		t.Fatalf("api = %+v, want restore to leave it owed", entry)
	}
	return &restored{w: w, store: store, entry: entry, made: made}
}

// pollFor polls every 10 s for a minute: long past the wait a drop takes.
func (r *restored) pollFor() {
	r.w.poll()
	for range 6 {
		r.w.after(10 * time.Second)
	}
}

// assertKept fails unless api's entry, transcript and hook are as restore left
// them.
func (r *restored) assertKept(t *testing.T) {
	t.Helper()
	if got, ok := r.w.entry("api"); !ok || !got.Owed || got.ScrollbackFile != r.entry.ScrollbackFile {
		t.Errorf("api = %+v, %v; want it kept, owed, naming %s", got, ok, r.entry.ScrollbackFile)
	}
	if got, ok := r.w.read(r.entry.ScrollbackFile); !ok || got != "the saved transcript" {
		t.Errorf("the saved transcript = %q, %v; want it kept", got, ok)
	}
	if keys := hookKeys(t, r.store); !slices.Equal(keys, []string{"api"}) {
		t.Errorf("hooks = %v, want api's kept", keys)
	}
}

func TestPoll_RepaysNoEntryOfASessionHydrating(t *testing.T) {
	r := stageRestored(t)

	r.pollFor()

	if got, _ := r.w.entry("api"); !got.Owed {
		t.Fatalf("api = %+v, want it owed while its session hydrates", got)
	}
	r.w.lower(state.Hydrating, "api")
	r.w.after(5 * time.Second)
	if got, _ := r.w.entry("api"); got.Owed {
		t.Errorf("api = %+v, want it repaid once its session is live and hydrates no more", got)
	}
}

// decision 72, amended: owed keeps an unfinished restore from a drop, and the
// flag adds nothing, so one stuck up never turns the user's close into a
// resurrection.
func TestPoll_JudgesAFlaggedSessionForADropByItsEntryAlone(t *testing.T) {
	for _, tc := range []struct {
		name    string
		owed    bool
		dropped bool
	}{
		{"a flagged session whose entry is owed is kept once gone", true, false},
		{"a flagged session whose entry is repaid is dropped once gone, its flag stuck up", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			gone := entryOf(backend.Session{Name: "gone", Created: w.thisBoot(0), PID: 4000})
			gone.Owed = tc.owed
			w.seed(entryOf(w.live("api", 1)), entryOf(w.live("web", 2)), gone)
			w.raise(state.Hydrating, "gone")

			w.poll()
			for range 6 {
				w.after(10 * time.Second)
			}

			if _, held := w.entry("gone"); held == tc.dropped {
				t.Errorf("gone held = %v, want dropped %v", held, tc.dropped)
			}
		})
	}
}

func TestPoll_KeepsASessionWhoseHelperNeverFinished(t *testing.T) {
	for _, tc := range []struct {
		name     string
		seenLive bool
	}{
		{"its replay failed, and the user closed it", true},
		{"its helper died once Create had made it", false},
	} {
		t.Run(tc.name+": its entry, transcript and hook are kept, past the flag's lowering", func(t *testing.T) {
			r := stageRestored(t)
			if tc.seenLive {
				r.w.fake.SetHistory("api", []byte("the screen since"))
				mark := r.w.mark()
				r.w.poll()
				if asked := historyAsked(r.w, mark); slices.Contains(asked, "api") {
					t.Errorf("History asked %v, want nothing captured of a session hydrating", asked)
				}
			}
			r.w.fake.Remove("api")

			r.pollFor()
			r.assertKept(t)

			r.w.lower(state.Hydrating, "api")
			r.pollFor()
			r.assertKept(t)
		})
	}
}

func TestPoll_DropsASessionClosedOnceItsRestoreFinished(t *testing.T) {
	r := stageRestored(t)
	r.w.poll()

	r.w.update(func(idx *sessionindex.Index) { idx.Hydrated("api", r.made.Created, r.made.PID) })
	r.w.lower(state.Hydrating, "api")
	r.w.fake.Remove("api")
	r.w.after(5 * time.Second)
	r.w.after(10 * time.Second)

	if _, ok := r.w.entry("api"); ok {
		t.Error("api is kept, want it dropped as any session of this boot that ends under a saver")
	}
	if keys := hookKeys(t, r.store); len(keys) != 0 {
		t.Errorf("hooks = %v, want api's forgotten with it", keys)
	}
	if _, ok := r.w.read(r.entry.ScrollbackFile); ok {
		t.Error("the saved transcript is kept, want it forgotten with api")
	}
}
