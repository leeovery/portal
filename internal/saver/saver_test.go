package saver_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// stageWork leaves a poll something of every kind to do, a minute on from a
// first poll: an entry whose session ended 61 s ago to drop, a session the
// index lacks to claim, a changed history to capture, and a file no entry
// names to remove. It returns that file.
func stageWork(w *world) string {
	w.t.Helper()
	kept := w.live("kept", 1)
	w.fake.SetHistory("kept", []byte("first screen"))
	w.seed(entryOf(kept), sessionindex.Entry{Name: "gone", CreatedAt: w.thisBoot(2), PID: 4002})
	w.poll()

	w.fake.SetHistory("kept", []byte("second screen"))
	w.live("newcomer", 3)
	stray := scrollbackFile(w.t, backend.Session{Name: "stray", Created: w.thisBoot(4), PID: 4004})
	w.write(stray, "nobody's")
	w.clock.at = w.clock.at.Add(time.Minute + time.Second)
	return stray
}

func TestPoll_StandsDown(t *testing.T) {
	boom := errors.New("zmx dir unreadable")

	for _, tc := range []struct {
		name string
		// stand makes the poll stand down, returning what lifts it.
		stand   func(w *world) (lift func())
		wantErr func(err error) bool
	}{
		{
			name: "while the restoring flag is raised",
			stand: func(w *world) func() {
				if err := state.Restoring.Set(w.dir); err != nil {
					w.t.Fatal(err)
				}
				return func() { _, _ = state.Restoring.Clear(w.dir) }
			},
			wantErr: func(err error) bool { return errors.Is(err, saver.ErrRestoring) },
		},
		{
			name: "while the restoring flag can't be read",
			stand: func(w *world) func() {
				if err := os.Chmod(w.dir, 0o600); err != nil {
					w.t.Fatal(err)
				}
				lift := func() { _ = os.Chmod(w.dir, 0o700) }
				w.t.Cleanup(lift)
				return lift
			},
			wantErr: func(err error) bool { return err != nil && !errors.Is(err, saver.ErrRestoring) },
		},
		{
			name: "when the listing fails",
			stand: func(w *world) func() {
				w.fake.Fail(backendtest.OpList, boom)
				return func() { w.fake.Fail(backendtest.OpList, nil) }
			},
			wantErr: func(err error) bool { return errors.Is(err, boom) },
		},
	} {
		t.Run(tc.name+", it claims, drops, captures and removes nothing", func(t *testing.T) {
			w := newWorld(t)
			stray := stageWork(w)
			index, err := os.ReadFile(sessionindex.Path(w.dir))
			if err != nil {
				t.Fatal(err)
			}
			mark := w.mark()

			lift := tc.stand(w)
			result, err := w.saver.Poll(t.Context())
			lift()

			if !tc.wantErr(err) || !result.Quiet() {
				t.Errorf("Poll = %+v, %v, want it to stand down, reporting nothing done", result, err)
			}
			if after, _ := os.ReadFile(sessionindex.Path(w.dir)); !bytes.Equal(after, index) {
				t.Errorf("sessions.json became %s, want it untouched", after)
			}
			if calls := slices.DeleteFunc(w.fake.Calls()[mark:], func(c backendtest.Call) bool { return c.Op == backendtest.OpList }); len(calls) != 0 {
				t.Errorf("the poll asked the backend %+v, want nothing past a listing", calls)
			}
			if got, _ := w.read(scrollbackFile(t, w.held("kept"))); got != "first screen" {
				t.Errorf("kept's scrollback = %q, want the first screen still", got)
			}
			if _, ok := w.read(stray); !ok {
				t.Error("the file no entry names was removed")
			}

			w.poll()

			if got := w.names(); !slices.Equal(got, []string{"kept", "newcomer"}) {
				t.Errorf("entries once the poll runs = %v, want gone dropped and newcomer claimed", got)
			}
			if got, _ := w.read(scrollbackFile(t, w.held("kept"))); got != "second screen" {
				t.Errorf("kept's scrollback once the poll runs = %q, want the second screen", got)
			}
			if _, ok := w.read(stray); ok {
				t.Error("the file no entry names is kept once the poll runs, want it removed")
			}
		})
	}
}

func TestPoll_ReportsWhatItDid(t *testing.T) {
	t.Run("it counts the entries it claimed and dropped, the sessions it captured and the files it removed", func(t *testing.T) {
		w := newWorld(t)
		stageWork(w)

		got := w.poll()

		if want := (saver.Result{Claimed: 1, Dropped: 1, Captured: 1, Removed: 1}); got != want {
			t.Errorf("Poll = %+v, want newcomer claimed, gone dropped, kept captured and the stray removed: %+v", got, want)
		}
	})

	t.Run("an adopt, a replace and a filled-in pid each count as a claim", func(t *testing.T) {
		w := newWorld(t)
		adopted, replaced, filled := w.live("adopted", 1), w.live("replaced", 2), w.live("filled", 3)
		w.seed(
			sessionindex.Entry{Name: "adopted", CreatedAt: w.beforeBoot()},
			sessionindex.Entry{Name: "replaced", CreatedAt: w.thisBoot(0), PID: 999},
			sessionindex.Entry{Name: "filled", CreatedAt: filled.Created},
		)

		if got := w.poll(); got.Claimed != 3 {
			t.Errorf("Claimed = %d, want 3: %s adopted, %s replaced, %s's pid filled in", got.Claimed, adopted.Name, replaced.Name, filled.Name)
		}
	})

	t.Run("a poll that changes nothing but where a session works reports nothing", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		w.fake.SetHistory("api", []byte("the screen"))
		w.poll()

		api.Dir = "/srv/api/sub"
		w.fake.Add(api)
		got := w.after(5 * time.Second)

		if !got.Quiet() {
			t.Errorf("Poll = %+v, want nothing reported", got)
		}
		if e, _ := w.entry("api"); e.Cwd != "/srv/api/sub" {
			t.Errorf("Cwd = %q, want the move recorded all the same", e.Cwd)
		}
	})

	t.Run("a poll whose write fails reports nothing", func(t *testing.T) {
		w := newWorld(t)
		w.live("api", 1)
		w.write("sessions.json", "not an index")

		got, err := w.saver.Poll(t.Context())

		if err == nil || !got.Quiet() {
			t.Errorf("Poll = %+v, %v, want the unreadable index's failure, reporting nothing", got, err)
		}
	})
}

func TestPoll_ClaimsAnsweringSessions(t *testing.T) {
	t.Run("a session the index lacks gets an entry of its creation, with no dir, recording where it works", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)

		w.poll()

		want := entryOf(api)
		want.Cwd = "/srv/api"
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want %+v", got, want)
		}
	})

	t.Run("an entry of the session's creation keeps everything, taking the pid it lacks", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		held := sessionindex.Entry{Name: "api", Label: "API", Dir: "/minted", CreatedAt: api.Created}
		w.seed(held)

		w.poll()

		want := held
		want.PID, want.Cwd = api.PID, "/srv/api"
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want %+v", got, want)
		}
	})

	for _, tc := range []struct {
		name  string
		held  func(w *world) sessionindex.Entry
		stage func(w *world)
		owed  bool
	}{
		{
			name: "an entry saved before this boot",
			held: func(w *world) sessionindex.Entry { return labelled(w.beforeBoot(), 1717) },
		},
		{
			name: "an entry recording no creation",
			held: func(*world) sessionindex.Entry { return labelled(time.Time{}, 0) },
		},
		{
			name:  "an entry of an earlier creation this boot, its session hydrating",
			held:  func(w *world) sessionindex.Entry { return labelled(w.thisBoot(0), 1717) },
			stage: func(w *world) { w.raise(state.Hydrating, "api") },
			owed:  true,
		},
		{
			name:  "an entry of an earlier creation this boot, its session waiting to resume",
			held:  func(w *world) sessionindex.Entry { return labelled(w.thisBoot(0), 1717) },
			stage: func(w *world) { w.raise(state.ResumePending, "api") },
		},
	} {
		t.Run(tc.name+" is adopted, keeping its label and dirs", func(t *testing.T) {
			w := newWorld(t)
			api := w.live("api", 1)
			held := tc.held(w)
			w.seed(held)
			if tc.stage != nil {
				tc.stage(w)
			}

			w.poll()

			want := held
			want.CreatedAt, want.PID, want.Cwd = api.Created, api.PID, "/srv/api"
			want.Owed = tc.owed
			if got, _ := w.entry("api"); !sameEntry(got, want) {
				t.Errorf("api = %+v, want %+v", got, want)
			}
		})
	}

	t.Run("an entry of an earlier creation this boot is replaced, and what it kept is forgotten", func(t *testing.T) {
		store, _ := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{"api": "claude --resume"}})
		w := newWorld(t).withHooks(store)
		api := w.live("api", 1)
		earlier := labelled(w.thisBoot(0), 1717)
		w.seed(earlier)
		w.write(earlier.ScrollbackFile, "the earlier creation's")

		w.poll()

		want := entryOf(api)
		want.Cwd = "/srv/api"
		if got, _ := w.entry("api"); !sameEntry(got, want) {
			t.Errorf("api = %+v, want the live creation's fresh entry %+v", got, want)
		}
		if _, ok := w.read(earlier.ScrollbackFile); ok {
			t.Error("the earlier creation's scrollback is kept, want it forgotten")
		}
		if keys := hookKeys(t, store); len(keys) != 0 {
			t.Errorf("hooks = %v, want the earlier creation's forgotten", keys)
		}
	})
}

func TestPoll_LeavesSessionsThatDontAnswerAndPortalsOwnAlone(t *testing.T) {
	t.Run("a session listed but not answering gets no entry", func(t *testing.T) {
		w := newWorld(t)
		w.live("wedged", 1)
		w.fake.StopAnswering("wedged")

		w.poll()

		if got := w.names(); len(got) != 0 {
			t.Errorf("entries = %v, want none", got)
		}
	})

	t.Run("an entry whose session is listed but not answering is never claimed or dropped", func(t *testing.T) {
		w := newWorld(t)
		wedged := w.live("wedged", 1)
		w.fake.StopAnswering("wedged")
		held := labelled(w.thisBoot(0), 1717)
		held.Name = "wedged"
		w.seed(held)

		for range 4 {
			w.after(10 * time.Second)
		}

		if got, ok := w.entry("wedged"); !ok || !sameEntry(got, held) {
			t.Errorf("wedged = %+v, %v; want %+v untouched, not claimed for %+v", got, ok, held, wedged)
		}
		if calls := w.fake.CallsTo(backendtest.OpHistory); len(calls) != 0 {
			t.Errorf("History asked %+v, want nothing of a session that doesn't answer", calls)
		}
	})

	t.Run("Portal's own sessions get no entry and no capture, and their entries are never dropped", func(t *testing.T) {
		w := newWorld(t)
		w.live("_portal-saver", 1)
		w.fake.SetHistory("_portal-saver", []byte("the daemon's screen"))
		own := sessionindex.Entry{Name: "_old", CreatedAt: w.thisBoot(0), PID: 1717}
		w.seed(own)

		for range 4 {
			w.after(10 * time.Second)
		}

		if got := w.names(); !slices.Equal(got, []string{"_old"}) {
			t.Errorf("entries = %v, want _old alone, untouched", got)
		}
		if calls := w.fake.CallsTo(backendtest.OpHistory); len(calls) != 0 {
			t.Errorf("History asked %+v, want nothing of Portal's own", calls)
		}
	})
}

// A session made outside Portal under a name Create refuses could never be
// restored, so saving it would only leave an entry restore must fail.
func TestPoll_SavesNoSessionUnderANameCreateRefuses(t *testing.T) {
	w := newWorld(t)
	for i, name := range []string{"foo*", "-x", "logs", "api"} {
		w.live(name, i)
		w.fake.SetHistory(name, []byte("the screen of "+name))
	}

	w.poll()
	w.after(5 * time.Second)

	if got := w.names(); !slices.Equal(got, []string{"api"}) {
		t.Errorf("entries = %v, want api alone", got)
	}
	for _, call := range w.fake.CallsTo(backendtest.OpHistory) {
		if call.Name != "api" {
			t.Errorf("History asked of %q, want nothing of a session never saved", call.Name)
		}
	}
}

func TestPoll_RecordsWhereEachSessionWorks(t *testing.T) {
	t.Run("a session that moves has its new directory recorded", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		w.poll()

		api.Dir = "/srv/api/sub"
		w.fake.Add(api)
		w.after(5 * time.Second)

		if got, _ := w.entry("api"); got.Cwd != "/srv/api/sub" {
			t.Errorf("Cwd = %q, want the new directory", got.Cwd)
		}
	})

	t.Run("a session reporting no directory keeps the one recorded", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		w.poll()

		api.Dir = ""
		w.fake.Add(api)
		w.after(5 * time.Second)

		if got, _ := w.entry("api"); got.Cwd != "/srv/api" {
			t.Errorf("Cwd = %q, want the one recorded kept", got.Cwd)
		}
	})
}

func TestPoll_ConfirmsUnderTheLockOnlyWhatTheListingMayHaveWrong(t *testing.T) {
	w := newWorld(t)
	w.live("api", 1)
	w.live("web", 2)

	mark := w.mark()
	w.poll()
	if got := w.calls(backendtest.OpSession, mark); len(got) != 2 {
		t.Errorf("a poll making two entries confirmed %+v, want both sessions confirmed", got)
	}

	mark = w.mark()
	w.after(5 * time.Second)
	if got := w.calls(backendtest.OpSession, mark); len(got) != 0 {
		t.Errorf("a quiet poll confirmed %+v, want no backend call under the lock", got)
	}
}

func TestPoll_BoundsItsConfirmationsTogetherInsideTheCommitLocksBound(t *testing.T) {
	w := newWorld(t)
	w.live("api", 1)
	w.live("web", 2)
	var deadlines []time.Time
	w.race.onSession = func(ctx context.Context) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Error("a confirmation ran with no deadline")
		}
		if left := time.Until(deadline); left > 500*time.Millisecond {
			t.Errorf("a confirmation had %s left, want 500 ms at most", left)
		}
		deadlines = append(deadlines, deadline)
	}

	w.poll()

	if len(deadlines) != 2 || !deadlines[0].Equal(deadlines[1]) {
		t.Errorf("confirmations ran to deadlines %v, want both to one", deadlines)
	}
}

func TestPoll_LeavesAClaimItsConfirmationsRanOutOfTimeForToTheNextPoll(t *testing.T) {
	saver.SetConfirmBudget(t, 20*time.Millisecond)
	w := newWorld(t)
	w.live("api", 1)
	w.live("web", 2)
	w.race.onSession = func(ctx context.Context) { <-ctx.Done() }
	sink := logtest.Install(t)

	w.poll()

	if got := w.names(); len(got) != 0 {
		t.Errorf("entries = %v, want no claim made past the budget", got)
	}
	skips := sink.Records().Matching("sessions", "claim-skipped").AtExactLevel(slog.LevelDebug)
	if len(skips) != 2 {
		t.Fatalf("logged %d skipped claims, want api's and web's: %+v", len(skips), skips)
	}
	for i, name := range []string{"api", "web"} {
		if got := skips[i].AttrString(t, "session"); got != name {
			t.Errorf("skip %d names %s, want %s", i, got, name)
		}
		if got := skips[i].AttrString(t, "detail"); got != "it was not confirmed in time" {
			t.Errorf("%s's skip says %q, want it not confirmed in time", name, got)
		}
		if err := skips[i].ErrorAttr(t, "error"); !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("%s's skip carries %v, want the budget's ErrTimeout", name, err)
		}
	}

	w.race.onSession = nil
	w.after(5 * time.Second)

	if got := w.names(); !slices.Equal(got, []string{"api", "web"}) {
		t.Errorf("entries at the next poll = %v, want both claimed", got)
	}
}

func TestPoll_NeverClaimsForASessionReportingNoCreation(t *testing.T) {
	t.Run("its entry is left as it is", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Add(backend.Session{Name: "api", Dir: "/srv/api", PID: 4001})
		held := labelled(w.thisBoot(0), 1717)
		w.seed(held)

		mark := w.mark()
		w.poll()

		if got, _ := w.entry("api"); !sameEntry(got, held) {
			t.Errorf("api = %+v, want %+v untouched", got, held)
		}
		if got := w.calls(backendtest.OpSession, mark); len(got) != 0 {
			t.Errorf("the poll confirmed %+v, want no backend call for a claim that can change nothing", got)
		}
	})

	t.Run("it gets no entry, which no drop could remove", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Add(backend.Session{Name: "api", Dir: "/srv/api", PID: 4001})
		sink := logtest.Install(t)

		mark := w.mark()
		w.poll()

		if got := w.names(); len(got) != 0 {
			t.Errorf("entries = %v, want none", got)
		}
		if got := w.calls(backendtest.OpSession, mark); len(got) != 0 {
			t.Errorf("the poll confirmed %+v, want no backend call for a claim it won't make", got)
		}
		rec := sink.Records().Matching("sessions", "claim-skipped").AtExactLevel(slog.LevelDebug).Only(t, "the skipped claim")
		if rec.AttrString(t, "session") != "api" || rec.AttrString(t, "detail") != "it reports no creation" {
			t.Errorf("record = %+v, want api skipped as reporting no creation", rec)
		}
	})
}

// labelled is api's entry holding all a claim must not lose, created at at,
// running pid.
func labelled(at time.Time, pid int) sessionindex.Entry {
	e := sessionindex.Entry{Name: "api", Label: "API work", Dir: "/minted", Cwd: "/was/here", CreatedAt: at, PID: pid}
	if !at.IsZero() {
		e.ScrollbackFile, _ = state.SessionScrollbackFile("api", at, pid)
	}
	return e
}

// sameEntry compares entries field by field, creation times as instants.
func sameEntry(got, want sessionindex.Entry) bool {
	if !got.CreatedAt.Equal(want.CreatedAt) {
		return false
	}
	got.CreatedAt, want.CreatedAt = time.Time{}, time.Time{}
	return got == want
}

func hookKeys(t *testing.T, store *hooks.Store) []string {
	t.Helper()
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	var keys []string
	for key := range held {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
