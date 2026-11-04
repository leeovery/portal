package restore_test

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var errRefused = errors.New("zmx attach: refused")

func TestNew(t *testing.T) {
	for name, exe := range map[string]string{
		"it refuses an empty exe, having no default": "",
		"it refuses a relative exe":                  "portal",
	} {
		t.Run(name, func(t *testing.T) {
			r, err := restore.New(restore.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Exe: exe})
			if err == nil || r != nil {
				t.Errorf("New(Exe: %q) = %v, %v; want a refusal", exe, r, err)
			}
		})
	}
}

func TestRestore_RestoresEachSavedSession(t *testing.T) {
	w := newWorld(t)
	work, notes := entry(t, "work"), entry(t, "notes")
	w.save(t, work, notes)

	res := w.r.Restore(t.Context())

	if want := (restore.Result{Restored: 2}); res != want {
		t.Errorf("Restore = %+v, want %+v", res, want)
	}
	if got, want := w.created(), []string{"work", "notes"}; !slices.Equal(got, want) {
		t.Errorf("created %q, want %q, in index order", got, want)
	}
	for _, name := range []string{"work", "notes"} {
		if !w.witness.raised[name] {
			t.Errorf("%s was created before its hydrating flag was raised", name)
		}
		if !w.hydrating(t, name) {
			t.Errorf("%s's hydrating flag is down: the helper lowers it once its replay is done, never restore", name)
		}
	}
	summary := w.sink.Records().WithMessage("restore complete").AtExactLevel(slog.LevelInfo).Only(t, "restore summary")
	if summary.IntAttr(t, "sessions") != 2 || summary.IntAttr(t, "anomalous") != 0 {
		t.Errorf("summary = %v, want 2 sessions restored and none failed", summary.Attrs)
	}
	summary.RequireDuration(t, "took")
}

func TestRestore_Skips(t *testing.T) {
	t.Run("a session the listing holds, answering", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.save(t, entry(t, "work"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "work")
	})

	t.Run("a session the listing holds, not answering", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.fake.StopAnswering("work")
		w.save(t, entry(t, "work"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "work")
	})

	t.Run("one of Portal's own", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "_portal-saver"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "_portal-saver")
	})

	// zmx finds a session by statting its socket, so on a filesystem that
	// folds case one held as work answers for Work. Only the backend can say
	// so, and Create does, refusing the name.
	t.Run("a session held under another spelling", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		saved := entry(t, "Work")
		w.save(t, saved)

		res := w.r.Restore(t.Context())

		if res != (restore.Result{}) {
			t.Errorf("Restore = %+v, want nothing restored and nothing failed", res)
		}
		if _, ok := w.fake.Held("Work"); ok {
			t.Error("Work was made beside work")
		}
		if w.hydrating(t, "Work") {
			t.Error("Work's hydrating flag, raised by this run, is left up")
		}
		if e, ok := w.lookup(t, "Work"); !ok || e != saved {
			t.Errorf("Work's entry = %+v, %v; want it kept as saved, %+v", e, ok, saved)
		}
		if failures := w.sink.Records().AtOrAboveLevel(slog.LevelWarn); len(failures) > 0 {
			t.Errorf("a held name logged %v, want no warning", failures)
		}
	})
}

// assertSkipped checks the restore made nothing and raised no flag for name.
func (w *world) assertSkipped(t *testing.T, res restore.Result, name string) {
	t.Helper()
	if res != (restore.Result{}) {
		t.Errorf("Restore = %+v, want nothing restored and nothing failed", res)
	}
	if created := w.created(); len(created) > 0 {
		t.Errorf("created %q, want nothing", created)
	}
	if w.hydrating(t, name) {
		t.Errorf("%s's hydrating flag was raised for a session restore skips", name)
	}
}

func TestRestore_ListingFails(t *testing.T) {
	w := newWorld(t)
	w.save(t, entry(t, "work"))
	failure := errors.New("read zmx dir: permission denied")
	w.fake.Fail(backendtest.OpList, failure)

	res := w.r.Restore(t.Context())

	if !errors.Is(res.ListErr, failure) || res.Restored != 0 || res.Failed != 0 {
		t.Errorf("Restore = %+v, want the listing's failure and nothing else", res)
	}
	if created := w.created(); len(created) > 0 {
		t.Errorf("created %q, want nothing", created)
	}
	if _, err := os.Stat(filepath.Join(w.dir, string(state.Hydrating))); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("hydrating flags were written for a listing that failed: %v", err)
	}
}

func TestRestore_IndexUnusable(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		is         error
	}{
		{name: "corrupt", body: `{"version": 2, "sessions": [`},
		{name: "version 1", body: `{"version": 1, "sessions": []}`, is: sessionindex.ErrLegacyIndex},
		{name: "newer", body: `{"version": 3, "sessions": [{"name": "work"}]}`},
	} {
		t.Run("a "+tc.name+" index restores nothing", func(t *testing.T) {
			w := newWorld(t)
			w.writeIndex(t, tc.body)

			res := w.r.Restore(t.Context())

			if res.IndexErr == nil || res.Restored != 0 || res.Failed != 0 {
				t.Errorf("Restore = %+v, want the index's error and nothing else", res)
			}
			if tc.is != nil && !errors.Is(res.IndexErr, tc.is) {
				t.Errorf("IndexErr = %v, want %v", res.IndexErr, tc.is)
			}
			if calls := w.fake.Calls(); len(calls) > 0 {
				t.Errorf("backend calls %+v, want none", calls)
			}
		})
	}
}

func TestRestore_NoIndexAsksNothing(t *testing.T) {
	w := newWorld(t)

	if res := w.r.Restore(t.Context()); res != (restore.Result{}) {
		t.Errorf("Restore = %+v, want nothing", res)
	}
	if calls := w.fake.Calls(); len(calls) > 0 {
		t.Errorf("backend calls %+v, want none: nothing is saved, so nothing is listed", calls)
	}
}

func TestRestore_FlagCantBeRaised(t *testing.T) {
	for name, block := range map[string]func(t *testing.T, flags string){
		"a file where the flags' directory goes": func(t *testing.T, flags string) {
			if err := os.WriteFile(flags, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"a flags directory it can search but not write to": func(t *testing.T, flags string) {
			if err := os.Mkdir(flags, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })
		},
	} {
		t.Run(name, func(t *testing.T) {
			w := newWorld(t)
			w.save(t, entry(t, "work"))
			block(t, filepath.Join(w.dir, string(state.Hydrating)))

			res := w.r.Restore(t.Context())

			if want := (restore.Result{Failed: 1}); res != want {
				t.Errorf("Restore = %+v, want %+v", res, want)
			}
			if created := w.created(); len(created) > 0 {
				t.Errorf("created %q without its hydrating flag raised", created)
			}
			w.failureLogged(t, "work")
		})
	}
}

func TestRestore_RefusedCreate(t *testing.T) {
	t.Run("it lowers the flag it raised and keeps the entry", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.fake.Fail(backendtest.OpCreate, errRefused)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag this run raised is left up")
		}
		if e, ok := w.lookup(t, "work"); !ok || e != saved {
			t.Errorf("entry = %+v, %v; want it kept as saved for the next bootstrap, %+v", e, ok, saved)
		}
		if cause := w.failureLogged(t, "work").ErrorAttr(t, "error"); !errors.Is(cause, errRefused) {
			t.Errorf("logged cause %v, want Create's refusal", cause)
		}
	})

	t.Run("it keeps a flag raised before it", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		if err := state.Hydrating.Set(w.dir, "work"); err != nil {
			t.Fatal(err)
		}
		w.fake.Fail(backendtest.OpCreate, errRefused)

		w.r.Restore(t.Context())

		if !w.hydrating(t, "work") {
			t.Error("a hydrating flag an earlier restore raised was lowered")
		}
	})
}

// failureLogged checks one WARN names the session that failed and the summary
// counts it, and returns that WARN.
func (w *world) failureLogged(t *testing.T, name string) logtest.Record {
	t.Helper()
	rec := w.sink.Records().WithMessage("restore session failed").AtExactLevel(slog.LevelWarn).Only(t, "failure")
	if got := rec.AttrString(t, "session"); got != name {
		t.Errorf("failure names session %q, want %q", got, name)
	}
	summary := w.sink.Records().WithMessage("restore complete").Only(t, "restore summary")
	if summary.IntAttr(t, "anomalous") != 1 {
		t.Errorf("summary = %v, want one session failed", summary.Attrs)
	}
	return rec
}

func TestRestore_StartDir(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cwd, dir, home := t.TempDir(), t.TempDir(), t.TempDir()

	for _, tc := range []struct {
		name, cwd, dir, want string
	}{
		{name: "its last working directory", cwd: cwd, dir: dir, want: cwd},
		{name: "the directory it was made in, its last one gone", cwd: gone, dir: dir, want: dir},
		{name: "HOME, both gone", cwd: gone, dir: gone, want: home},
		{name: "the directory it was made in, its last one a file", cwd: file, dir: dir, want: dir},
		{name: "the directory it was made in, recording no last one", dir: dir, want: dir},
	} {
		t.Run("it starts in "+tc.name, func(t *testing.T) {
			t.Setenv("HOME", home)
			w := newWorld(t)
			e := entry(t, "work")
			e.Cwd, e.Dir = tc.cwd, tc.dir
			w.save(t, e)

			w.r.Restore(t.Context())

			if got := w.onlyCreate(t).Dir; got != tc.want {
				t.Errorf("started in %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRestore_HelperArgv(t *testing.T) {
	bare := []string{exe, "state", "hydrate", "--session", "work"}

	t.Run("it names the transcript saved", func(t *testing.T) {
		w := newWorld(t)
		e, path := w.saveTranscript(t, entry(t, "work"))
		w.save(t, e)

		w.r.Restore(t.Context())

		if got, want := w.onlyCreate(t).Command, append(slices.Clone(bare), "--file", path); !slices.Equal(got, want) {
			t.Errorf("command = %q, want %q", got, want)
		}
	})

	t.Run("it names none for a session that saved none", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))

		w.r.Restore(t.Context())

		if got := w.onlyCreate(t).Command; !slices.Equal(got, bare) {
			t.Errorf("command = %q, want %q", got, bare)
		}
		if warnings := w.sink.Records().AtOrAboveLevel(slog.LevelWarn); len(warnings) > 0 {
			t.Errorf("a session that saved no transcript logged %v, want nothing amiss", warnings)
		}
	})

	t.Run("it names none whose file is gone", func(t *testing.T) {
		w := newWorld(t)
		e, path := w.saveTranscript(t, entry(t, "work"))
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		w.save(t, e)

		w.r.Restore(t.Context())

		if got := w.onlyCreate(t).Command; !slices.Equal(got, bare) {
			t.Errorf("command = %q, want %q", got, bare)
		}
	})

	t.Run("it names one it can't look for, for the helper to say why", func(t *testing.T) {
		w := newWorld(t)
		e, path := w.saveTranscript(t, entry(t, "work"))
		w.save(t, e)
		sealed := filepath.Dir(path)
		if err := os.Chmod(sealed, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(sealed, 0o700) })

		w.r.Restore(t.Context())

		if got, want := w.onlyCreate(t).Command, append(slices.Clone(bare), "--file", path); !slices.Equal(got, want) {
			t.Errorf("command = %q, want %q", got, want)
		}
	})

	t.Run("it names none that is no transcript of the session", func(t *testing.T) {
		w := newWorld(t)
		e, _ := w.saveTranscript(t, entry(t, "notes"))
		e.Name = "work"
		w.save(t, e)

		w.r.Restore(t.Context())

		if got := w.onlyCreate(t).Command; !slices.Equal(got, bare) {
			t.Errorf("command = %q, want %q", got, bare)
		}
		rec := w.sink.Records().WithMessage("scrollback file refused").AtExactLevel(slog.LevelWarn).Only(t, "refusal")
		if !errors.Is(rec.ErrorAttr(t, "error"), state.ErrForeignScrollback) {
			t.Errorf("refusal = %v, want the foreign file named", rec.Attrs)
		}
	})
}

func TestRestore_RecordsTheCreation(t *testing.T) {
	t.Run("its entry takes the creation Create returned, keeping the rest", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Now = func() time.Time { return time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC) }
		e, _ := w.saveTranscript(t, entry(t, "work"))
		e.Cwd = e.Dir
		w.save(t, e)

		w.r.Restore(t.Context())

		live, ok := w.fake.Held("work")
		if !ok {
			t.Fatal("work was not made")
		}
		want := e
		want.CreatedAt, want.PID = live.Created, live.PID
		if got, _ := w.lookup(t, "work"); got != want {
			t.Errorf("entry = %+v, want %+v", got, want)
		}
	})

	t.Run("a creation Create read none of changes nothing", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Now = func() time.Time { return time.Time{} }
		saved := entry(t, "work")
		w.save(t, saved)

		if res := w.r.Restore(t.Context()); res.Restored != 1 {
			t.Fatalf("Restore = %+v, want work restored", res)
		}

		if got, _ := w.lookup(t, "work"); got != saved {
			t.Errorf("entry = %+v, want it as saved, %+v", got, saved)
		}
	})
}

func TestRestore_Progress(t *testing.T) {
	w := newWorld(t, backend.Session{Name: "live"})
	w.save(t, entry(t, "a"), entry(t, "live"), entry(t, "b"), entry(t, "c"))
	w.fake.Fail(backendtest.OpCreate, errRefused)
	var got [][2]int
	w.r.SetProgress(func(n, m int) { got = append(got, [2]int{n, m}) })

	w.r.Restore(t.Context())

	if want := [][2]int{{1, 3}, {2, 3}, {3, 3}}; !slices.Equal(got, want) {
		t.Errorf("progress = %v, want %v: each session restore takes up, failed or not, of those it restores", got, want)
	}
}
