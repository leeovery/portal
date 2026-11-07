package saver_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

func TestFlush(t *testing.T) {
	t.Run("it writes and names the sessions whose history changed since the poll that saved it, and only those", func(t *testing.T) {
		w := newWorld(t)
		api, web := w.live("api", 1), w.live("web", 2)
		w.fake.SetHistory("api", []byte("api's first screen"))
		w.fake.SetHistory("web", []byte("web's screen"))
		w.poll()
		webInfo, err := os.Stat(w.path(scrollbackFile(t, web)))
		if err != nil {
			t.Fatal(err)
		}
		w.fake.SetHistory("api", []byte("api's last screen"))

		mark := w.mark()
		got := w.flush(t.Context())

		if got != (saver.Result{Captured: 1}) {
			t.Errorf("Flush = %+v, want api alone captured", got)
		}
		if asked := historyAsked(w, mark); !slices.Equal(asked, []string{"api", "web"}) {
			t.Errorf("History asked %v, want every session asked the instant after the poll, a minute's spacing or not", asked)
		}
		if content, _ := w.read(scrollbackFile(t, api)); content != "api's last screen" {
			t.Errorf("api's scrollback = %q, want its last screen", content)
		}
		if e, _ := w.entry("api"); e.ScrollbackFile != scrollbackFile(t, api) {
			t.Errorf("api's ScrollbackFile = %q, want its capture named", e.ScrollbackFile)
		}
		if info, err := os.Stat(w.path(scrollbackFile(t, web))); err != nil || !info.ModTime().Equal(webInfo.ModTime()) {
			t.Errorf("web's scrollback was written again (%v), want an unchanged history left as it was", err)
		}
	})

	t.Run("it claims, drops and removes nothing", func(t *testing.T) {
		w := newWorld(t)
		stray := stageWork(w)
		index, err := os.ReadFile(sessionindex.Path(w.dir))
		if err != nil {
			t.Fatal(err)
		}

		mark := w.mark()
		got := w.flush(t.Context())

		if got != (saver.Result{Captured: 1}) {
			t.Errorf("Flush = %+v, want kept's changed history alone captured", got)
		}
		if after, _ := os.ReadFile(sessionindex.Path(w.dir)); !bytes.Equal(after, index) {
			t.Errorf("sessions.json became %s, want gone kept, newcomer unclaimed and kept's file named as it was", after)
		}
		if calls := w.calls(backendtest.OpSession, mark); len(calls) != 0 {
			t.Errorf("the flush confirmed %+v, want no claim to confirm", calls)
		}
		if _, ok := w.read(stray); !ok {
			t.Error("the file no entry names was removed")
		}
		if content, _ := w.read(scrollbackFile(t, w.held("kept"))); content != "second screen" {
			t.Errorf("kept's scrollback = %q, want its second screen", content)
		}
	})

	t.Run("it skips a session Portal is restoring", func(t *testing.T) {
		for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
			t.Run(string(flag), func(t *testing.T) {
				w := newWorld(t)
				api := stageCaptured(w, "the saved screen")
				w.fake.SetHistory("api", []byte("the panel"))
				w.raise(flag, "api")

				mark := w.mark()
				got := w.flush(t.Context())

				if asked := historyAsked(w, mark); len(asked) != 0 || !got.Quiet() {
					t.Errorf("Flush = %+v asking %v, want nothing asked of a flagged session", got, asked)
				}
				if content, _ := w.read(scrollbackFile(t, api)); content != "the saved screen" {
					t.Errorf("api's scrollback = %q, want the saved screen kept", content)
				}
			})
		}
	})

	t.Run("it names nothing for a session flagged since its capture, so the next writes the same history again", func(t *testing.T) {
		w := newWorld(t)
		stageCaptured(w, "first")
		w.fake.SetHistory("api", []byte("second"))
		w.race.afterHistory = func(string) { w.raise(state.ResumePending, "api") }

		if got := w.flush(t.Context()); !got.Quiet() {
			t.Errorf("Flush = %+v, want nothing named for a session flagged since its capture", got)
		}

		w.lower(state.ResumePending, "api")
		if got := w.flush(t.Context()); got != (saver.Result{Captured: 1}) {
			t.Errorf("Flush once the flag went = %+v, want the history never named written and named", got)
		}
	})

	t.Run("it names nothing for a creation the index no longer records", func(t *testing.T) {
		w := newWorld(t)
		stageCaptured(w, "first")
		w.fake.SetHistory("api", []byte("the old creation's last screen"))
		var minted backend.Session
		w.race.afterHistory = func(string) {
			w.fake.Remove("api")
			minted = w.mint("api")
		}

		if got := w.flush(t.Context()); !got.Quiet() {
			t.Errorf("Flush = %+v, want nothing named", got)
		}
		if e, _ := w.entry("api"); !e.CreatedAt.Equal(minted.Created) || e.ScrollbackFile != "" {
			t.Errorf("api = %+v, want the minted creation's entry, naming nothing of the old", e)
		}
	})

	t.Run("a capture its write to the index didn't name is written again by the next", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "first")
		index, err := os.ReadFile(sessionindex.Path(w.dir))
		if err != nil {
			t.Fatal(err)
		}
		w.fake.SetHistory("api", []byte("second"))
		w.write("sessions.json", "not an index")

		if got, err := w.saver.Flush(t.Context()); err == nil || !got.Quiet() {
			t.Fatalf("Flush = %+v, %v, want the unreadable index's failure, reporting nothing", got, err)
		}
		w.write("sessions.json", string(index))

		if got := w.flush(t.Context()); got != (saver.Result{Captured: 1}) {
			t.Errorf("Flush = %+v, want the same history written and named again", got)
		}
		if e, _ := w.entry("api"); e.ScrollbackFile != scrollbackFile(t, api) {
			t.Errorf("api's ScrollbackFile = %q, want the capture named", e.ScrollbackFile)
		}
	})

	t.Run("it asks every session, however long their histories take on the saver's clock", func(t *testing.T) {
		w := newWorld(t)
		var entries []sessionindex.Entry
		for i, name := range []string{"s0", "s1", "s2", "s3"} {
			entries = append(entries, entryOf(w.live(name, i)))
			w.fake.SetHistory(name, []byte(name+"'s screen"))
		}
		w.seed(entries...)
		w.race.onHistory = func(context.Context, string) { w.clock.at = w.clock.at.Add(2 * time.Second) }

		mark := w.mark()
		got := w.flush(t.Context())

		if asked := historyAsked(w, mark); len(asked) != 4 || got.Captured != 4 {
			t.Errorf("Flush = %+v asking %v, want all four asked and saved, past a poll's 3 s budget", got, asked)
		}
	})

	t.Run("it stops asking once its context ends, and names what it saved before", func(t *testing.T) {
		w := newWorld(t)
		api, web, zed := w.live("api", 1), w.live("web", 2), w.live("zed", 3)
		w.seed(entryOf(api), entryOf(web), entryOf(zed))
		for _, name := range []string{"api", "web", "zed"} {
			w.fake.SetHistory(name, []byte(name+"'s screen"))
		}
		// The context ends as web's History is asked, never on a clock, so
		// api's History and its write are done first however loaded the
		// machine is.
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		var cancelled time.Time
		w.race.onHistory = func(ctx context.Context, name string) {
			if name == "web" {
				cancelled = time.Now()
				cancel()
				<-ctx.Done()
			}
		}

		mark := w.mark()
		got := w.flush(ctx)

		if took := time.Since(cancelled); took > 2*time.Second {
			t.Errorf("Flush took %s past its context's end, want it back at once", took)
		}
		if asked := historyAsked(w, mark); !slices.Equal(asked, []string{"api", "web"}) {
			t.Errorf("History asked %v, want zed, after the context ended, not asked", asked)
		}
		if got != (saver.Result{Captured: 1}) {
			t.Errorf("Flush = %+v, want api's capture named", got)
		}
		if e, _ := w.entry("api"); e.ScrollbackFile != scrollbackFile(t, api) {
			t.Errorf("api's ScrollbackFile = %q, want it named after the context ended", e.ScrollbackFile)
		}
		if _, ok := w.read(scrollbackFile(t, web)); ok {
			t.Error("web's scrollback was written, want nothing from a History the context cut")
		}
	})

	t.Run("it does nothing while the restoring flag is raised", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "first")
		w.fake.SetHistory("api", []byte("second"))
		if err := state.Restoring.Set(w.dir); err != nil {
			t.Fatal(err)
		}

		mark := w.mark()
		got, err := w.saver.Flush(t.Context())

		if !errors.Is(err, saver.ErrRestoring) || !got.Quiet() {
			t.Errorf("Flush = %+v, %v, want it to stand down", got, err)
		}
		if calls := w.fake.Calls()[mark:]; len(calls) != 0 {
			t.Errorf("the flush asked the backend %+v, want nothing", calls)
		}
		if content, _ := w.read(scrollbackFile(t, api)); content != "first" {
			t.Errorf("api's scrollback = %q, want the first screen kept", content)
		}
	})
}
