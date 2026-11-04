package saver_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// stageCaptured holds api answering with screen as its history, and polls
// once, so its scrollback is written and named.
func stageCaptured(w *world, screen string) backend.Session {
	w.t.Helper()
	api := w.live("api", 1)
	w.fake.SetHistory("api", []byte(screen))
	w.poll()
	return api
}

func TestPoll_CapturesScrollback(t *testing.T) {
	t.Run("a session's history is written to its creation's file, which its entry then names", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "the screen")

		file := scrollbackFile(t, api)
		if got, ok := w.read(file); !ok || got != "the screen" {
			t.Errorf("scrollback = %q, %v; want the history", got, ok)
		}
		if got, _ := w.entry("api"); got.ScrollbackFile != file {
			t.Errorf("ScrollbackFile = %q, want %q", got.ScrollbackFile, file)
		}
		info, err := os.Stat(state.ScrollbackDir(w.dir))
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("scrollback dir = %v, %v; want it made 0700", info, err)
		}
		if calls := w.fake.CallsTo(backendtest.OpHistory); len(calls) != 1 || calls[0].Rows != 0 {
			t.Errorf("History asked %+v, want it asked once for all the scrollback", calls)
		}
	})

	t.Run("a directory sync failing once the file landed is a warning, the file named all the same", func(t *testing.T) {
		w := newWorld(t)
		fileutil.FailDirSyncForTest(t, syscall.EIO)
		sink := logtest.Install(t)

		api := stageCaptured(w, "the screen")

		rec := sink.Records().Matching("capture", "unsynced").AtExactLevel(slog.LevelWarn).Only(t, "the scrollback's unsynced write")
		logtest.AssertWriteFailure(t, rec, "write-failed-fsync", fileutil.ErrWriteFsync)
		if got, _ := w.entry("api"); got.ScrollbackFile != scrollbackFile(t, api) {
			t.Errorf("ScrollbackFile = %q, want the file that landed named", got.ScrollbackFile)
		}
	})

	t.Run("a creation's history is asked for at most once a minute", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "first")
		w.fake.SetHistory("api", []byte("second"))

		mark := w.mark()
		w.after(59 * time.Second)
		if calls := w.calls(backendtest.OpHistory, mark); len(calls) != 0 {
			t.Errorf("History asked %+v within a minute, want nothing", calls)
		}

		w.after(time.Second)
		if got, _ := w.read(scrollbackFile(t, api)); got != "second" {
			t.Errorf("scrollback a minute on = %q, want the second history", got)
		}
	})

	t.Run("a history unchanged since the index named it is not written again", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "same")
		file := scrollbackFile(t, api)
		w.write(file, "marker")

		mark := w.mark()
		w.after(time.Minute)

		if calls := w.calls(backendtest.OpHistory, mark); len(calls) != 1 {
			t.Errorf("History asked %d times, want once", len(calls))
		}
		if got, _ := w.read(file); got != "marker" {
			t.Errorf("scrollback = %q, want it not rewritten", got)
		}
	})

	t.Run("a fresh saver writes each session's history once", func(t *testing.T) {
		w := newWorld(t)
		api := stageCaptured(w, "same")
		file := scrollbackFile(t, api)
		w.write(file, "marker")

		w.restart()
		w.after(time.Second)

		if got, _ := w.read(file); got != "same" {
			t.Errorf("scrollback = %q, want the history written again", got)
		}
	})

	t.Run("a new creation of the name is asked for at once", func(t *testing.T) {
		w := newWorld(t)
		stageCaptured(w, "first creation")
		later := w.live("api", 2)
		w.fake.SetHistory("api", []byte("later creation"))

		w.after(5 * time.Second)

		if got, _ := w.read(scrollbackFile(t, later)); got != "later creation" {
			t.Errorf("the later creation's scrollback = %q, want its history", got)
		}
	})

	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		t.Run("a session flagged "+string(flag)+" is not captured until the flag goes", func(t *testing.T) {
			w := newWorld(t)
			api := w.live("api", 1)
			w.fake.SetHistory("api", []byte("the panel"))
			w.raise(flag, "api")

			w.poll()
			if calls := w.fake.CallsTo(backendtest.OpHistory); len(calls) != 0 {
				t.Errorf("History asked %+v of a flagged session, want nothing", calls)
			}
			if got, _ := w.entry("api"); got.ScrollbackFile != "" {
				t.Errorf("ScrollbackFile = %q, want none", got.ScrollbackFile)
			}

			w.lower(flag, "api")
			w.after(5 * time.Second)
			if _, ok := w.read(scrollbackFile(t, api)); !ok {
				t.Error("no scrollback once the flag went, want it captured")
			}
		})
	}

	for _, tc := range []struct {
		name  string
		fail  func(w *world)
		level slog.Level
		msg   string
	}{
		{"a session with no screen to give", func(*world) {}, slog.LevelDebug, "capture skipped"},
		{"a session not answering in time", failHistory(fmt.Errorf("history: %w", backend.ErrTimeout)), slog.LevelDebug, "capture skipped"},
		{"a session gone since the listing", failHistory(fmt.Errorf("history: %w", backend.ErrNoSuchSession)), slog.LevelDebug, "capture skipped"},
		{"any other failure", failHistory(errors.New("socket closed")), slog.LevelWarn, "capture failed"},
	} {
		t.Run(tc.name+" is skipped this round, its entry naming nothing", func(t *testing.T) {
			w := newWorld(t)
			w.live("api", 1)
			tc.fail(w)
			sink := logtest.Install(t)

			w.poll()

			if got, ok := w.entry("api"); !ok || got.ScrollbackFile != "" {
				t.Errorf("api = %+v, %v; want it claimed, naming no scrollback", got, ok)
			}
			rec := sink.Records().Matching("capture", tc.msg).Only(t, "the skip")
			if rec.Level != tc.level || rec.AttrString(t, "session") != "api" {
				t.Errorf("record = %+v, want %v naming api", rec, tc.level)
			}
		})
	}

	t.Run("a history the session didn't give is asked for again a minute on, not at the next poll", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		w.poll()
		w.fake.SetHistory("api", []byte("the screen"))

		w.after(59 * time.Second)
		if _, ok := w.read(scrollbackFile(t, api)); ok {
			t.Fatal("the history was asked for again within a minute")
		}

		w.after(time.Second)
		if got, _ := w.read(scrollbackFile(t, api)); got != "the screen" {
			t.Errorf("scrollback a minute on = %q, want the history", got)
		}
	})

	t.Run("a file that can't be written is never named; the index names only a file that has landed", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		w.fake.SetHistory("api", []byte("the screen"))
		w.write("scrollback", "a file where the directory goes")
		sink := logtest.Install(t)

		w.poll()

		if got, _ := w.entry("api"); got.ScrollbackFile != "" {
			t.Errorf("ScrollbackFile = %q, want none named for a file that never landed", got.ScrollbackFile)
		}
		rec := sink.Records().Matching("capture", "write failed").AtExactLevel(slog.LevelWarn).Only(t, "the write's failure")
		logtest.AssertWriteFailure(t, rec, "write-failed-temp-create", fileutil.ErrWriteTempCreate)

		if err := os.Remove(filepath.Join(w.dir, "scrollback")); err != nil {
			t.Fatal(err)
		}
		w.after(time.Minute)
		if got, _ := w.read(scrollbackFile(t, api)); got != "the screen" {
			t.Errorf("scrollback once it can be written = %q, want the history", got)
		}
	})
}

func TestPoll_BoundsItsCapturesTogether(t *testing.T) {
	t.Run("a poll's History calls share one deadline, 3 s at most", func(t *testing.T) {
		w := newWorld(t)
		for i, name := range []string{"api", "web"} {
			w.live(name, i)
			w.fake.SetHistory(name, []byte(name+"'s screen"))
		}
		var deadlines []time.Time
		w.race.onHistory = func(ctx context.Context, _ string) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Error("a capture ran with no deadline")
			}
			if left := time.Until(deadline); left > 3*time.Second {
				t.Errorf("a capture had %s left, want 3 s at most", left)
			}
			deadlines = append(deadlines, deadline)
		}

		w.poll()

		if len(deadlines) != 2 || !deadlines[0].Equal(deadlines[1]) {
			t.Errorf("captures ran to deadlines %v, want both to one", deadlines)
		}
	})

	t.Run("a session the budget doesn't reach is asked at the next poll, and one it ran out on a minute on", func(t *testing.T) {
		saver.SetCaptureBudget(t, 20*time.Millisecond)
		w := newWorld(t)
		stalled, reached := w.live("api", 1), w.live("web", 2)
		w.fake.SetHistory("api", []byte("api's screen"))
		w.fake.SetHistory("web", []byte("web's screen"))
		w.race.onHistory = func(ctx context.Context, name string) {
			if name != "api" {
				return
			}
			select {
			case <-ctx.Done():
			case <-time.After(5 * time.Second):
				t.Error("api's History ran 5 s, want it cut short by the poll's budget")
			}
		}
		sink := logtest.Install(t)

		mark := w.mark()
		w.poll()
		if calls := w.calls(backendtest.OpHistory, mark); len(calls) != 1 || calls[0].Name != "api" {
			t.Errorf("History asked %+v, want api alone, the budget spent on it", calls)
		}
		rec := sink.Records().Matching("capture", "capture skipped").AtExactLevel(slog.LevelDebug).Only(t, "api's skipped capture")
		if err := rec.ErrorAttr(t, "error"); rec.AttrString(t, "session") != "api" || !errors.Is(err, backend.ErrTimeout) {
			t.Errorf("record = %+v, want api's capture skipped on the budget's ErrTimeout", rec)
		}

		w.race.onHistory = nil
		mark = w.mark()
		w.after(time.Second)
		if calls := w.calls(backendtest.OpHistory, mark); len(calls) != 1 || calls[0].Name != "web" {
			t.Errorf("History asked %+v at the next poll, want web alone", calls)
		}
		if got, _ := w.read(scrollbackFile(t, reached)); got != "web's screen" {
			t.Errorf("web's scrollback = %q, want its history", got)
		}

		mark = w.mark()
		w.after(59 * time.Second)
		if calls := w.calls(backendtest.OpHistory, mark); len(calls) != 1 || calls[0].Name != "api" {
			t.Errorf("History asked %+v a minute on, want api alone", calls)
		}
		if got, _ := w.read(scrollbackFile(t, stalled)); got != "api's screen" {
			t.Errorf("api's scrollback = %q, want its history", got)
		}
	})
}

func failHistory(err error) func(w *world) {
	return func(w *world) {
		w.fake.SetHistory("api", []byte("the screen"))
		w.fake.Fail(backendtest.OpHistory, err)
	}
}

func TestPoll_RemovesScrollbackNoEntryNames(t *testing.T) {
	t.Run("it removes only Portal's per-creation files no entry names", func(t *testing.T) {
		w := newWorld(t)
		api := w.live("api", 1)
		named := scrollbackFile(t, api)
		w.seed(sessionindex.Entry{Name: "api", CreatedAt: api.Created, PID: api.PID, ScrollbackFile: named})
		w.write(named, "api's")
		stray := scrollbackFile(t, backend.Session{Name: "stray", Created: w.thisBoot(5), PID: 4005})
		w.write(stray, "nobody's")
		left := []string{
			"scrollback/api.bin",
			"scrollback/notes.txt",
			scrollbackFile(t, backend.Session{Name: "linked", Created: w.thisBoot(6), PID: 4006}),
		}
		for _, rel := range left[:2] {
			w.write(rel, "not the saver's")
		}
		if err := os.Symlink(filepath.Join(w.dir, named), filepath.Join(w.dir, filepath.FromSlash(left[2]))); err != nil {
			t.Fatal(err)
		}
		folder := scrollbackFile(t, backend.Session{Name: "folder", Created: w.thisBoot(7), PID: 4007})
		if err := os.Mkdir(filepath.Join(w.dir, filepath.FromSlash(folder)), 0o700); err != nil {
			t.Fatal(err)
		}

		w.poll()

		if _, ok := w.read(stray); ok {
			t.Error("the file no entry names is kept, want it removed")
		}
		for _, rel := range append([]string{named}, left...) {
			if _, err := os.Lstat(filepath.Join(w.dir, filepath.FromSlash(rel))); err != nil {
				t.Errorf("%s: %v, want it left", rel, err)
			}
		}
		if _, err := os.Stat(filepath.Join(w.dir, filepath.FromSlash(folder))); err != nil {
			t.Errorf("the directory named as a creation's file: %v, want it left", err)
		}
	})

	t.Run("it removes the temp files a write left there an hour ago or more, and no other", func(t *testing.T) {
		w := newWorld(t)
		w.live("api", 1)
		aged := func(rel string, age time.Duration) string {
			target := filepath.Join(w.dir, filepath.FromSlash(rel))
			if err := os.Chtimes(target, w.clock.at.Add(-age), w.clock.at.Add(-age)); err != nil {
				t.Fatal(err)
			}
			return target
		}
		var abandoned, kept []string
		for rel, age := range map[string]time.Duration{"scrollback/.atomic-123.tmp": time.Hour, "scrollback/.atomic-9.tmp": 25 * time.Hour} {
			w.write(rel, "half a transcript")
			abandoned = append(abandoned, aged(rel, age))
		}
		for rel, age := range map[string]time.Duration{"scrollback/.atomic-456.tmp": 59 * time.Minute, "scrollback/.atomic-x.tmp": 2 * time.Hour, ".atomic-789.tmp": 2 * time.Hour} {
			w.write(rel, "not the saver's")
			kept = append(kept, aged(rel, age))
		}
		folder := "scrollback/.atomic-5.tmp"
		if err := os.Mkdir(filepath.Join(w.dir, filepath.FromSlash(folder)), 0o700); err != nil {
			t.Fatal(err)
		}
		kept = append(kept, aged(folder, 2*time.Hour))
		sink := logtest.Install(t)

		w.poll()

		for _, target := range abandoned {
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Errorf("%s: %v, want it removed", target, err)
			}
		}
		for _, target := range kept {
			if _, err := os.Lstat(target); err != nil {
				t.Errorf("%s: %v, want it left", target, err)
			}
		}
		rec := sink.Records().Matching("capture", "cleaned").Only(t, "the clean")
		if got := rec.IntAttr(t, "reaped"); got != 2 {
			t.Errorf("the clean reaped %d, want the two abandoned temp files", got)
		}
	})

	t.Run("it removes at most once a minute", func(t *testing.T) {
		w := newWorld(t)
		w.live("api", 1)
		w.poll()
		stray := scrollbackFile(t, backend.Session{Name: "stray", Created: w.thisBoot(5), PID: 4005})
		w.write(stray, "nobody's")

		w.after(59 * time.Second)
		if _, ok := w.read(stray); !ok {
			t.Fatal("the file no entry names was removed within a minute of the last removal")
		}

		w.after(time.Second)
		if _, ok := w.read(stray); ok {
			t.Error("the file no entry names is kept a minute on, want it removed")
		}
	})

	t.Run("an adopted entry's earlier file, once the entry names the later creation's, goes at the next removal due", func(t *testing.T) {
		w := newWorld(t)
		saved := labelled(w.beforeBoot(), 1717)
		w.seed(saved)
		w.write(saved.ScrollbackFile, "the transcript restore replays")
		api := w.live("api", 1)
		w.fake.SetHistory("api", []byte("the replayed screen"))
		w.raise(state.Hydrating, "api")

		w.poll()
		if got, _ := w.entry("api"); !got.CreatedAt.Equal(api.Created) || got.ScrollbackFile != saved.ScrollbackFile {
			t.Fatalf("api = %+v, want the live creation adopted, naming the saved transcript", got)
		}

		w.lower(state.Hydrating, "api")
		w.after(5 * time.Second)
		if got, _ := w.entry("api"); got.ScrollbackFile != scrollbackFile(t, api) {
			t.Errorf("ScrollbackFile = %q, want the live creation's own", got.ScrollbackFile)
		}
		if _, ok := w.read(saved.ScrollbackFile); !ok {
			t.Error("the saved transcript went before the next removal was due")
		}

		w.after(time.Minute)
		if _, ok := w.read(saved.ScrollbackFile); ok {
			t.Error("the saved transcript no entry names is kept, want it removed")
		}
		if got := w.names(); !slices.Equal(got, []string{"api"}) {
			t.Errorf("entries = %v, want api alone", got)
		}
	})
}
