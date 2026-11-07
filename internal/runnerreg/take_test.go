package runnerreg_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/runnerreg"
	"github.com/leeovery/portal/internal/runnertoken"
)

// asAnotherProcess has a take judge every runner this test registers
// another process's, as each would be in its own Portal.
func asAnotherProcess(t *testing.T) {
	runnerreg.SetSelf(t, 0)
}

func TestProtocol_Take(t *testing.T) {
	t.Run("take hands the handler the session named and where it went", func(t *testing.T) {
		h := &handler{}
		g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)

		r := call(t, g.Socket(), `{"version":1,"request":"take","session":"api-x7Kd9a","by":"SSH from 192.0.2.20",`+due()+`}`+"\n")

		if want := []takeCall{{"api-x7Kd9a", "SSH from 192.0.2.20"}}; !r.OK || r.PID != os.Getpid() || !slices.Equal(h.askedTakes(), want) {
			t.Errorf("answer = %+v, takes %+v; want %+v accepted by pid %d", r, h.askedTakes(), want, os.Getpid())
		}
	})

	d := due()
	for _, tc := range []struct {
		name, raw, says string
	}{
		{"a take naming no session", `{"version":1,"request":"take","by":"Ghostty on this Mac",` + d + "}\n", "take names no session"},
		{"a take saying not where the session went", `{"version":1,"request":"take","session":"api-x7Kd9a",` + d + "}\n", "take names no by"},
		{"a take given an empty by", `{"version":1,"request":"take","session":"api-x7Kd9a","by":"",` + d + "}\n", "take names no by"},
		{"a switch given a by", `{"version":1,"request":"switch","session":"blog-c3d4","by":"Ghostty on this Mac",` + d + "}\n", "switch takes no by"},
		{"a detach-to-picker given a null by", `{"version":1,"request":"detach-to-picker","by":null,` + d + "}\n", "detach-to-picker takes no by"},
	} {
		t.Run("it refuses "+tc.name, func(t *testing.T) {
			h := &handler{}
			g := register(t, runnerreg.New(stateDir(t)), "tok123", "api-x7Kd9a", h)

			r := call(t, g.Socket(), tc.raw)

			if r.OK || !strings.Contains(r.Refusal, tc.says) {
				t.Errorf("answer = %+v; want a refusal saying %q", r, tc.says)
			}
			if switches, detaches := h.asked(); len(switches) != 0 || detaches != 0 || len(h.askedTakes()) != 0 {
				t.Errorf("the handler was asked to switch %q, detach %d times, take %+v; want nothing", switches, detaches, h.askedTakes())
			}
		})
	}
}

func TestTake(t *testing.T) {
	t.Run("it asks every runner on the session, and no other, to let it go, saying where it went", func(t *testing.T) {
		asAnotherProcess(t)
		r := runnerreg.New(stateDir(t))
		first, second, elsewhere := &handler{}, &handler{}, &handler{}
		register(t, r, "first1", "api-x7Kd9a", first)
		register(t, r, "second", "api-x7Kd9a", second)
		register(t, r, "elsewh", "blog-c3d4", elsewhere)

		accepted, err := r.Take(t.Context(), "api-x7Kd9a", "SSH from 192.0.2.20")

		if err != nil || accepted != 2 {
			t.Errorf("Take = %d, %v; want both runners on the session", accepted, err)
		}
		want := []takeCall{{"api-x7Kd9a", "SSH from 192.0.2.20"}}
		for name, h := range map[string]*handler{"first": first, "second": second} {
			if got := h.askedTakes(); !slices.Equal(got, want) {
				t.Errorf("the %s runner was asked %+v, want %+v", name, got, want)
			}
		}
		if got := elsewhere.askedTakes(); len(got) != 0 {
			t.Errorf("a runner on another session was asked %+v", got)
		}
	})

	t.Run("it never asks this process's own runner", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		own := &handler{}
		register(t, r, "own123", "api-x7Kd9a", own)

		accepted, err := r.Take(t.Context(), "api-x7Kd9a", "Ghostty on this Mac")

		if accepted != 0 || !errors.Is(err, runnertoken.ErrNoRunner) {
			t.Errorf("Take = %d, %v; want no runner to ask", accepted, err)
		}
		if got := own.askedTakes(); len(got) != 0 {
			t.Errorf("this process's own runner was asked %+v", got)
		}
	})

	t.Run("a runner that refuses or doesn't understand is the reason none let it go", func(t *testing.T) {
		asAnotherProcess(t)
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		register(t, r, "first1", "api-x7Kd9a", &handler{refuse: errors.New("the runner is not attached")})
		writeEntry(t, r, "older1", entryFor(os.Getpid(), "api-x7Kd9a"))
		answeringSocket(t, r.SocketPath("older1"), ourAnswer(`"ok":false,"refusal":"unknown request \"take\""`), nil)

		accepted, err := r.Take(t.Context(), "api-x7Kd9a", "Ghostty on this Mac")

		if accepted != 0 || !errors.Is(err, runnertoken.ErrRefused) || errors.Is(err, runnertoken.ErrNoRunner) {
			t.Fatalf("Take = %d, %v; want none accepted, refused", accepted, err)
		}
		for _, why := range []string{"let it go", "the runner is not attached", `unknown request "take"`} {
			if !strings.Contains(err.Error(), why) {
				t.Errorf("Take = %q, want it to say %q", err, why)
			}
		}
		recs := sink.Records().Matching("attach", "take").AtExactLevel(slog.LevelInfo)
		if len(recs) != 2 {
			t.Errorf("take records = %d, want one for each runner asked", len(recs))
		}
	})

	t.Run("no runner on the session is ErrNoRunner, and says so", func(t *testing.T) {
		sink := logtest.Install(t)

		if accepted, err := runnerreg.New(stateDir(t)).Take(t.Context(), "api-x7Kd9a", "Ghostty on this Mac"); !errors.Is(err, runnertoken.ErrNoRunner) || accepted != 0 {
			t.Errorf("Take = %d, %v; want none, ErrNoRunner", accepted, err)
		}
		rec := sink.Records().Matching("attach", "take").Only(t, "take record")
		if got := rec.AttrString(t, "result"); got != "no runner" {
			t.Errorf("result = %q, want no runner", got)
		}
	})

	t.Run("a read-only registry asks no runner", func(t *testing.T) {
		asAnotherProcess(t)
		r := runnerreg.New(stateDir(t))
		h := &handler{}
		register(t, r, "live01", "api-x7Kd9a", h)

		if accepted, err := r.ReadOnly().Take(t.Context(), "api-x7Kd9a", "Ghostty on this Mac"); err == nil || errors.Is(err, runnertoken.ErrNoRunner) || accepted != 0 {
			t.Errorf("Take = %d, %v; want the read-only registry's refusal", accepted, err)
		}
		if got := h.askedTakes(); len(got) != 0 {
			t.Errorf("the runner was asked %+v", got)
		}
	})
}

func TestTakeRecord(t *testing.T) {
	t.Run("a take recorded for a session says where it went to a runner that attached before it", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		attached := time.Now()

		if err := r.RecordTake("api-x7Kd9a", "SSH from 192.0.2.20"); err != nil {
			t.Fatalf("RecordTake: %v", err)
		}

		if got := r.TakenBy("api-x7Kd9a", attached); got != "SSH from 192.0.2.20" {
			t.Errorf("TakenBy = %q, want where the take went", got)
		}
	})

	t.Run("the record is only this user's, and no runner's entry", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := r.RecordTake("api-x7Kd9a", "Ghostty on this Mac"); err != nil {
			t.Fatalf("RecordTake: %v", err)
		}

		record := r.TakePath("api-x7Kd9a")
		for path, want := range map[string]os.FileMode{r.Dir(): 0o700, filepath.Dir(record): 0o700, record: 0o600} {
			if info, err := os.Stat(path); err != nil || info.Mode().Perm() != want {
				t.Errorf("%s: mode %v, %v; want %v", path, info.Mode().Perm(), err, want)
			}
		}
		if accepted, err := r.DetachToPicker(t.Context(), "api-x7Kd9a"); !errors.Is(err, runnertoken.ErrNoRunner) || accepted != 0 {
			t.Errorf("DetachToPicker = %d, %v; want the record read as no runner", accepted, err)
		}
		if !exists(t, record) {
			t.Error("reading the registry removed the record")
		}
	})

	for _, tc := range []struct {
		name    string
		session string
		since   func(recorded time.Time) time.Time
	}{
		{"of another session says nothing", "blog-c3d4", func(recorded time.Time) time.Time { return recorded.Add(-time.Second) }},
		{"of a name the filesystem folds together with the session's says nothing", "API-X7KD9A", func(recorded time.Time) time.Time { return recorded.Add(-time.Second) }},
		{"recorded before the attach began says nothing", "api-x7Kd9a", func(recorded time.Time) time.Time { return recorded.Add(time.Second) }},
	} {
		t.Run("a take "+tc.name, func(t *testing.T) {
			r := runnerreg.New(stateDir(t))
			recorded := time.Now()
			if err := r.RecordTake("api-x7Kd9a", "Ghostty on this Mac"); err != nil {
				t.Fatalf("RecordTake: %v", err)
			}

			if got := r.TakenBy(tc.session, tc.since(recorded)); got != "" {
				t.Errorf("TakenBy = %q, want nothing", got)
			}
		})
	}

	t.Run("each take of a session replaces its last one's record", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		attached := time.Now()
		for _, by := range []string{"Ghostty on this Mac", "SSH from 192.0.2.20"} {
			if err := r.RecordTake("api-x7Kd9a", by); err != nil {
				t.Fatalf("RecordTake: %v", err)
			}
		}

		if got := r.TakenBy("api-x7Kd9a", attached); got != "SSH from 192.0.2.20" {
			t.Errorf("TakenBy = %q, want the last take's", got)
		}
	})

	t.Run("takes of two sessions at once each keep their own record", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		attached := time.Now()
		takes := map[string]string{"api-x7Kd9a": "SSH from 192.0.2.20", "blog-c3d4": "Ghostty on this Mac"}
		for session, by := range takes {
			if err := r.RecordTake(session, by); err != nil {
				t.Fatalf("RecordTake %s: %v", session, err)
			}
		}

		for session, by := range takes {
			if got := r.TakenBy(session, attached); got != by {
				t.Errorf("TakenBy(%s) = %q, want its own take's %q", session, got, by)
			}
		}
	})

	t.Run("no record, or one that can't be read, says nothing", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if got := r.TakenBy("api-x7Kd9a", time.Time{}); got != "" {
			t.Errorf("TakenBy = %q with no record, want nothing", got)
		}
		record := r.TakePath("api-x7Kd9a")
		if err := os.MkdirAll(filepath.Dir(record), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(record, []byte(`{"by":"Ghostty on this Mac","at":"2026-11-07T10:00:00Z","window":1}`), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := r.TakenBy("api-x7Kd9a", time.Time{}); got != "" {
			t.Errorf("TakenBy = %q from a record with a field it doesn't know, want nothing", got)
		}
	})

	t.Run("a read-only registry records nothing", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))

		if err := r.ReadOnly().RecordTake("api-x7Kd9a", "Ghostty on this Mac"); err == nil {
			t.Error("RecordTake = nil, want the read-only registry's refusal")
		}
		if exists(t, r.TakePath("api-x7Kd9a")) {
			t.Error("a read-only registry wrote the record")
		}
	})
}

func TestLockTake(t *testing.T) {
	t.Run("a take holding a session's lock holds off the next of that session until it releases", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		release, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}

		if _, err := r.LockTake(t.Context(), "api-x7Kd9a", 30*time.Millisecond); !errors.Is(err, fileutil.ErrLockTimedOut) {
			t.Errorf("LockTake while held = %v, want it timed out", err)
		}
		release()
		release()
		next, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake once released: %v", err)
		}
		next()
	})

	t.Run("a take of another session holds off nothing", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		release, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}
		defer release()

		for _, other := range []string{"blog-c3d4", "API-X7KD9A"} {
			next, err := r.LockTake(t.Context(), other, 30*time.Millisecond)
			if err != nil {
				t.Errorf("LockTake(%s) while api-x7Kd9a's is held = %v, want its own", other, err)
				continue
			}
			next()
		}
	})

	t.Run("its release leaves no file behind", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		release, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}
		if !exists(t, r.TakeLockPath("api-x7Kd9a")) {
			t.Fatal("the held lock has no file")
		}

		release()

		if exists(t, r.TakeLockPath("api-x7Kd9a")) {
			t.Error("the released lock's file is left")
		}
	})

	// The release removes the lock's file, so a take waiting on it locks a
	// file no longer there, which locks nothing: it must lock the one there
	// now, or the next take would hold the lock beside it.
	t.Run("a take that waited on a file its holder removed holds the lock there now", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		first, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}
		waited := make(chan func(), 1)
		go func() {
			release, err := r.LockTake(context.Background(), "api-x7Kd9a", 5*time.Second)
			if err != nil {
				release = nil
			}
			waited <- release
		}()
		// Long enough for the waiter to open the file first holds, whose
		// release then removes it.
		time.Sleep(100 * time.Millisecond)

		first()
		second := <-waited
		if second == nil {
			t.Fatal("the waiting take got no lock")
		}
		defer second()

		if _, err := r.LockTake(t.Context(), "api-x7Kd9a", 30*time.Millisecond); !errors.Is(err, fileutil.ErrLockTimedOut) {
			t.Errorf("LockTake beside the waiter's = %v, want it held off", err)
		}
	})

	t.Run("the lock is no runner's, and a runner registering leaves it", func(t *testing.T) {
		asAnotherProcess(t)
		r := runnerreg.New(stateDir(t))
		release, err := r.LockTake(t.Context(), "api-x7Kd9a", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}
		defer release()

		register(t, r, "tok123", "blog-c3d4", &handler{})

		if _, err := r.Take(t.Context(), "api-x7Kd9a", "Ghostty on this Mac"); !errors.Is(err, runnertoken.ErrNoRunner) {
			t.Errorf("Take = %v, want no runner on the session", err)
		}
		if !exists(t, r.TakeLockPath("api-x7Kd9a")) {
			t.Error("the lock is gone")
		}
	})

	t.Run("a read-only registry takes nothing", func(t *testing.T) {
		if _, err := runnerreg.New(stateDir(t)).ReadOnly().LockTake(t.Context(), "api-x7Kd9a", time.Second); err == nil {
			t.Error("LockTake = nil, want the read-only registry's refusal")
		}
	})
}

// aged makes path's file look written hours ago.
func aged(t *testing.T, path string) {
	t.Helper()
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
}

func TestTakeSweep(t *testing.T) {
	t.Run("a runner registering removes the take files an hour old, and logs each", func(t *testing.T) {
		sink := logtest.Install(t)
		r := runnerreg.New(stateDir(t))
		if err := r.RecordTake("api-x7Kd9a", "Ghostty on this Mac"); err != nil {
			t.Fatalf("RecordTake: %v", err)
		}
		left := r.TakeLockPath("api-x7Kd9a")
		if err := os.WriteFile(left, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{r.TakePath("api-x7Kd9a"), left} {
			aged(t, path)
		}

		register(t, r, "tok123", "blog-c3d4", &handler{})

		for _, path := range []string{r.TakePath("api-x7Kd9a"), left} {
			if exists(t, path) {
				t.Errorf("%s is left", path)
			}
		}
		if got := sink.Records().Matching("attach", "stale take removed").AtExactLevel(slog.LevelDebug); len(got) != 2 {
			t.Errorf("removals logged = %d, want one for each file", len(got))
		}
	})

	t.Run("it leaves a newer record, and an old lock a take holds", func(t *testing.T) {
		r := runnerreg.New(stateDir(t))
		if err := r.RecordTake("api-x7Kd9a", "Ghostty on this Mac"); err != nil {
			t.Fatalf("RecordTake: %v", err)
		}
		release, err := r.LockTake(t.Context(), "blog-c3d4", time.Second)
		if err != nil {
			t.Fatalf("LockTake: %v", err)
		}
		defer release()
		aged(t, r.TakeLockPath("blog-c3d4"))

		register(t, r, "tok123", "blog-c3d4", &handler{})

		for _, path := range []string{r.TakePath("api-x7Kd9a"), r.TakeLockPath("blog-c3d4")} {
			if !exists(t, path) {
				t.Errorf("%s is gone", path)
			}
		}
	})
}
