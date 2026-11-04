package saver_test

import (
	"fmt"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// stageGone seeds an index of two live sessions and gone, whose session of
// this boot has ended, with the flags and scrollback Portal keeps beside it.
func stageGone(w *world) sessionindex.Entry {
	w.t.Helper()
	gone := backend.Session{Name: "gone", Created: w.thisBoot(0), PID: 4000}
	entry := entryOf(gone)
	entry.Label, entry.ScrollbackFile = "Gone", scrollbackFile(w.t, gone)
	w.seed(entryOf(w.live("api", 1)), entryOf(w.live("web", 2)), entry)
	w.write(entry.ScrollbackFile, "gone's transcript")
	w.raise(state.ResumePending, "gone")
	return entry
}

func TestPoll_DropsAnEntryMissingFromTwoPollsTenSecondsApart(t *testing.T) {
	t.Run("missing 9 s it is kept, missing 10 s it is dropped, logged, and what it kept forgotten", func(t *testing.T) {
		w := newWorld(t)
		gone := stageGone(w)
		w.poll()
		w.after(9 * time.Second)
		if _, ok := w.entry("gone"); !ok {
			t.Fatal("gone was dropped missing 9 s, want it kept")
		}
		sink := logtest.Install(t)

		w.after(time.Second)

		if got := w.names(); !slices.Equal(got, []string{"api", "web"}) {
			t.Errorf("entries = %v, want gone dropped", got)
		}
		rec := sink.Records().Matching("sessions", "session dropped").Only(t, "gone's drop")
		if rec.Level != slog.LevelInfo || rec.AttrString(t, "session") != "gone" || rec.AttrString(t, "via") != "internal" {
			t.Errorf("drop record = %+v, want INFO naming gone, via internal", rec)
		}
		if _, ok := w.read(gone.ScrollbackFile); ok {
			t.Error("gone's scrollback is kept, want it forgotten")
		}
		if up, _ := state.ResumePending.Exists(w.dir, "gone"); up {
			t.Error("gone's resume-pending flag is raised still, want it forgotten")
		}
	})

	t.Run("a listing that shows it again starts its wait over", func(t *testing.T) {
		w := newWorld(t)
		stageGone(w)
		w.poll()

		w.fake.Add(backend.Session{Name: "gone", Created: w.thisBoot(0), PID: 4000})
		w.after(5 * time.Second)
		w.fake.Remove("gone")
		w.after(4 * time.Second)
		w.after(6 * time.Second)
		if _, ok := w.entry("gone"); !ok {
			t.Fatal("gone was dropped 15 s after it first went missing, 6 s after it went again, want it kept")
		}

		w.after(4 * time.Second)

		if _, ok := w.entry("gone"); ok {
			t.Error("gone is kept missing 10 s since it was last listed, want it dropped")
		}
	})

	t.Run("a listing that shows it again starts its wait over though the poll's commit fails", func(t *testing.T) {
		w := newWorld(t)
		stageGone(w)
		w.poll()
		index, err := os.ReadFile(sessionindex.Path(w.dir))
		if err != nil {
			t.Fatal(err)
		}

		w.fake.Add(backend.Session{Name: "gone", Created: w.thisBoot(0), PID: 4000})
		w.write("sessions.json", "not an index")
		w.clock.at = w.clock.at.Add(5 * time.Second)
		if err := w.saver.Poll(t.Context()); err == nil {
			t.Fatal("Poll = nil, want the unreadable index's failure")
		}
		w.write("sessions.json", string(index))
		w.fake.Remove("gone")
		w.after(5 * time.Second)

		if _, ok := w.entry("gone"); !ok {
			t.Error("gone was dropped 5 s after a listing showed it, want its wait started over")
		}
	})

	t.Run("a failed listing between neither starts its wait over nor counts", func(t *testing.T) {
		w := newWorld(t)
		stageGone(w)
		w.poll()

		w.fake.Fail(backendtest.OpList, fmt.Errorf("zmx dir unreadable"))
		w.clock.at = w.clock.at.Add(5 * time.Second)
		if err := w.saver.Poll(t.Context()); err == nil {
			t.Fatal("Poll = nil, want the listing's failure")
		}
		w.fake.Fail(backendtest.OpList, nil)
		w.after(5 * time.Second)

		if _, ok := w.entry("gone"); ok {
			t.Error("gone is kept missing from listings 10 s apart, want it dropped")
		}
	})
}

func TestPoll_NeverDrops(t *testing.T) {
	for _, tc := range []struct {
		name    string
		created func(w *world) time.Time
	}{
		{"an entry created before this boot, for the next bootstrap to restore", func(w *world) time.Time { return w.beforeBoot() }},
		{"an entry recording no creation", func(*world) time.Time { return time.Time{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			kept := sessionindex.Entry{Name: "kept", Label: "Kept", CreatedAt: tc.created(w)}
			w.seed(entryOf(w.live("api", 1)), kept)

			w.poll()
			for range 6 {
				w.after(10 * time.Second)
			}

			if got, ok := w.entry("kept"); !ok || !sameEntry(got, kept) {
				t.Errorf("kept = %+v, %v; want %+v untouched", got, ok, kept)
			}
		})
	}
}

func TestPoll_HoldsAMassVanish(t *testing.T) {
	for _, tc := range []struct {
		name          string
		vanish, total int
		held          bool
	}{
		{"exactly three, half of those it may drop, are held", 3, 6, true},
		{"exactly half of those it may drop, more than three, are held", 4, 8, true},
		{"three, under half of those it may drop, are dropped", 3, 7, false},
		{"two, half of those it may drop, are dropped", 2, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorld(t)
			names := stageLive(w, tc.total)
			w.poll()
			sink := logtest.Install(t)

			for _, name := range names[:tc.vanish] {
				w.fake.Remove(name)
			}
			w.after(5 * time.Second)
			w.after(10 * time.Second)
			w.after(time.Minute)

			want := names
			if !tc.held {
				want = names[tc.vanish:]
			}
			if got := w.names(); !slices.Equal(got, want) {
				t.Errorf("entries = %v, want %v", got, want)
			}
			warned := sink.Records().Matching("sessions", "drops-held")
			if !tc.held {
				if len(warned) != 0 {
					t.Errorf("logged %d holds, want none", len(warned))
				}
				return
			}
			rec := warned.AtExactLevel(slog.LevelWarn).Only(t, "the one hold")
			if got := rec.IntAttr(t, "sessions"); got != int64(tc.vanish) {
				t.Errorf("the hold counts %d sessions, want %d", got, tc.vanish)
			}
		})
	}

	t.Run("a held entry listed again goes back to being dropped when it goes", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		w.poll()
		for _, name := range names[:3] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)

		w.live(names[0], 0)
		w.after(5 * time.Second)
		w.fake.Remove(names[0])
		w.after(5 * time.Second)
		w.after(10 * time.Second)

		if got, want := w.names(), names[1:]; !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: %s dropped, the others still held", got, want, names[0])
		}
	})

	t.Run("a vanish split across polls is held whole, those still waiting among it", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 10)
		w.poll()
		sink := logtest.Install(t)

		for _, name := range names[:2] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		for _, name := range names[2:6] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		w.after(10 * time.Second)
		w.after(time.Minute)

		if got := w.names(); !slices.Equal(got, names) {
			t.Errorf("entries = %v, want every one held", got)
		}
		rec := sink.Records().Matching("sessions", "drops-held").Only(t, "the one hold")
		if got := rec.IntAttr(t, "sessions"); got != 6 {
			t.Errorf("the hold counts %d sessions, want all 6 missing", got)
		}
	})

	t.Run("a session ending on its own after a held vanish is dropped, those held not counted", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		w.poll()
		for _, name := range names[:3] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)

		w.fake.Remove(names[3])
		w.after(5 * time.Second)
		w.after(10 * time.Second)

		if got, want := w.names(), slices.Concat(names[:3], names[4:]); !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: %s dropped, the three held kept", got, want, names[3])
		}
	})

	t.Run("a vanish among the entries not held is held, those held not counted among those it may drop", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 8)
		w.poll()
		for _, name := range names[:4] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		sink := logtest.Install(t)

		for _, name := range names[4:7] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		w.after(10 * time.Second)
		w.after(time.Minute)

		if got := w.names(); !slices.Equal(got, names) {
			t.Errorf("entries = %v, want every one held: three of the four not held missing is a mass vanish", got)
		}
		rec := sink.Records().Matching("sessions", "drops-held").Only(t, "the second hold")
		if got := rec.IntAttr(t, "sessions"); got != 3 {
			t.Errorf("the hold counts %d sessions, want the 3 missing since", got)
		}
	})

	t.Run("a held name whose entry now records another pid, created the same second, is judged afresh", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		w.poll()
		for _, name := range names[:3] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)

		w.claim(backend.Session{Name: names[0], Created: w.thisBoot(0), PID: 5000}, "/proj/"+names[0])
		w.after(5 * time.Second)
		w.after(10 * time.Second)

		if got, want := w.names(), names[1:]; !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: %s's later creation, never listed, dropped", got, want, names[0])
		}
	})

	t.Run("a held name whose entry now records another creation is judged afresh, its wait begun anew", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		w.poll()
		w.fake.Remove(names[0])
		w.after(5 * time.Second)
		for _, name := range names[1:3] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)

		w.claim(backend.Session{Name: names[0], Created: w.thisBoot(30), PID: 5000}, "/proj/"+names[0])
		w.after(5 * time.Second)
		if _, ok := w.entry(names[0]); !ok {
			t.Fatalf("%s's later creation was dropped on its first poll missing, want its wait begun anew", names[0])
		}
		w.after(10 * time.Second)

		if got, want := w.names(), names[1:]; !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: %s's later creation, never listed, dropped", got, want, names[0])
		}
	})

	t.Run("a fresh saver counts what is missing on its first poll", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 4)
		for _, name := range names[:3] {
			w.fake.Remove(name)
		}

		w.poll()
		w.after(10 * time.Second)
		w.after(time.Minute)

		if got := w.names(); !slices.Equal(got, names) {
			t.Errorf("entries = %v, want every one held", got)
		}
	})

	t.Run("only names not starting with _ count among those it may drop", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		var own []sessionindex.Entry
		for i, name := range []string{"_one", "_two", "_three"} {
			own = append(own, entryOf(w.live(name, 10+i)))
		}
		w.seed(append(entriesOf(w, names), own...)...)
		w.poll()

		for _, name := range names[:3] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		w.after(10 * time.Second)

		if got, want := w.names(), append([]string{"_one", "_three", "_two"}, names...); !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: three of six missing is a mass vanish", got, want)
		}
	})

	t.Run("only names not starting with _ count missing", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 6)
		own := []sessionindex.Entry{
			{Name: "_one", CreatedAt: w.thisBoot(0), PID: 1},
			{Name: "_two", CreatedAt: w.thisBoot(0), PID: 2},
			{Name: "_three", CreatedAt: w.thisBoot(0), PID: 3},
		}
		w.seed(append(entriesOf(w, names), own...)...)
		w.poll()

		for _, name := range names[:2] {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		w.after(10 * time.Second)

		if got, want := w.names(), append([]string{"_one", "_three", "_two"}, names[2:]...); !slices.Equal(got, want) {
			t.Errorf("entries = %v, want %v: two of six missing is no mass vanish", got, want)
		}
	})

	t.Run("entries that may predate this boot count in neither the missing nor those it may drop", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 4)
		var failed []sessionindex.Entry
		for i := range 6 {
			failed = append(failed, sessionindex.Entry{Name: fmt.Sprintf("restore-%d", i), CreatedAt: w.beforeBoot(), PID: 100 + i})
		}
		w.seed(append(entriesOf(w, names), failed...)...)
		w.poll()

		for _, name := range names {
			w.fake.Remove(name)
		}
		w.after(5 * time.Second)
		w.after(10 * time.Second)
		w.after(time.Minute)

		if got := w.names(); len(got) != 10 {
			t.Errorf("entries = %v, want all ten kept: every session of this boot gone is a mass vanish beside six failed restores", got)
		}
	})

	t.Run("an entry that may predate this boot is not counted missing", func(t *testing.T) {
		w := newWorld(t)
		names := stageLive(w, 3)
		saved := []sessionindex.Entry{
			{Name: "saved-1", CreatedAt: w.beforeBoot()},
			{Name: "saved-2", CreatedAt: w.beforeBoot()},
			{Name: "saved-3"},
		}
		w.seed(append(entriesOf(w, names), saved...)...)
		w.fake.Remove(names[0])

		w.poll()
		w.after(10 * time.Second)

		if _, ok := w.entry(names[0]); ok {
			t.Errorf("%s is kept, want it dropped: one entry of this boot missing is no mass vanish", names[0])
		}
	})
}

func TestPoll_ReplaceIsNoDrop(t *testing.T) {
	w := newWorld(t)
	w.live("api", 1)
	w.seed(labelled(w.thisBoot(0), 1717))
	sink := logtest.Install(t)

	w.poll()

	if dropped := sink.Records().Matching("sessions", "session dropped"); len(dropped) != 0 {
		t.Errorf("a replace logged %d drops, want none", len(dropped))
	}
	sink.Records().Matching("sessions", "replace").Only(t, "the replace")
}

// stageLive holds n live sessions of this boot, each with its entry, and
// returns their names in order.
func stageLive(w *world, n int) []string {
	w.t.Helper()
	var names []string
	for i := range n {
		names = append(names, fmt.Sprintf("s%d", i))
		w.live(names[i], i)
	}
	w.seed(entriesOf(w, names)...)
	return names
}

// entriesOf is the entry of each held session named.
func entriesOf(w *world, names []string) []sessionindex.Entry {
	var entries []sessionindex.Entry
	for i, name := range names {
		entries = append(entries, sessionindex.Entry{Name: name, CreatedAt: w.thisBoot(i), PID: 4000 + i})
	}
	return entries
}
