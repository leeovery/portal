package restore_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

var (
	errRefused  = errors.New("zmx attach: refused")
	errTimedOut = fmt.Errorf("create session: %w", backend.ErrTimeout)
)

func TestNew(t *testing.T) {
	for name, change := range map[string]func(*restore.Config){
		"it refuses no backend":                       func(c *restore.Config) { c.Backend = nil },
		"it refuses no state directory":               func(c *restore.Config) { c.StateDir = "" },
		"it refuses an empty exe, having no default":  func(c *restore.Config) { c.Exe = "" },
		"it refuses a relative exe":                   func(c *restore.Config) { c.Exe = "portal" },
		"it refuses a test binary":                    func(c *restore.Config) { c.Exe = "/var/folders/x/go-build1/b001/restore.test" },
		"it refuses a debugger's build":               func(c *restore.Config) { c.Exe = "/work/portal/__debug_bin3721882" },
		"it refuses a test binary built under a name": func(c *restore.Config) { c.Exe = "/tmp/build/x" },
		"it refuses a name that only starts portal":   func(c *restore.Config) { c.Exe = "/opt/portal/bin/portal-dev" },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := restore.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Exe: exe}
			change(&cfg)

			if r, err := restore.New(cfg); err == nil || r != nil {
				t.Errorf("New(%+v) = %v, %v; want a refusal", cfg, r, err)
			}
		})
	}

	t.Run("it takes an absolute exe named portal, wherever it lives", func(t *testing.T) {
		cfg := restore.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Exe: "/opt/tools.test/bin/portal"}

		if r, err := restore.New(cfg); err != nil || r == nil {
			t.Errorf("New(%+v) = %v, %v; want a Restorer", cfg, r, err)
		}
	})
}

// The one test that hands New the running binary, which the exe guard keeps
// from every other: New must refuse it on whatever route it arrives.
func TestNew_RefusesTheRunningTestBinary(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for name, exe := range map[string]string{"os.Args[0]": os.Args[0], "os.Executable": self} {
		t.Run("it refuses "+name, func(t *testing.T) {
			if !filepath.IsAbs(exe) {
				t.Skipf("%s = %q is relative, so its refusal would prove nothing of the name rule", name, exe)
			}

			if r, err := restore.New(restore.Config{Backend: backendtest.New(), StateDir: t.TempDir(), Exe: exe}); err == nil || r != nil {
				t.Errorf("New(Exe: %q) = %v, %v; want a refusal", exe, r, err)
			}
		})
	}
}

func TestRestore_RestoresEachSavedSession(t *testing.T) {
	w := newWorld(t)
	work, notes := entry(t, "work"), entry(t, "notes")
	notes.CreatedAt, notes.PID = time.Time{}, 0
	w.save(t, work, notes)

	res := w.r.Restore(t.Context())

	if want := (restore.Result{Restored: 2}); res != want {
		t.Errorf("Restore = %+v, want %+v", res, want)
	}
	if got, want := w.created(), []string{"work", "notes"}; !slices.Equal(got, want) {
		t.Errorf("created %q, want %q, in index order, an entry recording no creation among them", got, want)
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
	if summary.IntAttr(t, "restored") != 2 || summary.IntAttr(t, "failed") != 0 || summary.IntAttr(t, "anomalous") != 0 {
		t.Errorf("summary = %v, want 2 sessions restored, none failed and nothing anomalous", summary.Attrs)
	}
	summary.RequireDuration(t, "took")
}

func TestRestore_Skips(t *testing.T) {
	t.Run("a session the listing holds, answering", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.save(t, entry(t, "work"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "work", "it is live")
	})

	t.Run("a session the listing holds, not answering", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.fake.StopAnswering("work")
		w.save(t, entry(t, "work"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "work", "it is live")
	})

	t.Run("one of Portal's own", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "_portal-saver"))

		w.assertSkipped(t, w.r.Restore(t.Context()), "_portal-saver", "it is one of Portal's own")
	})

	// decision 55: a session of this boot that is gone ended this boot while
	// a saver watched, by the user or otherwise. A bootstrap again within the
	// boot, as a version change runs, must not bring it back.
	t.Run("an entry created this boot", func(t *testing.T) {
		w := newWorld(t)
		e := thisBoot(t, "work")
		w.save(t, e)

		w.assertSkipped(t, w.r.Restore(t.Context()), "work", "it ended this boot")
		if got, ok := w.lookup(t, "work"); !ok || !sameEntry(got, e) {
			t.Errorf("entry = %+v, %v; want it left for the saver to drop, %+v", got, ok, e)
		}
	})
}

// thisBoot is a saved session's entry created after booted.
func thisBoot(t *testing.T, name string) sessionindex.Entry {
	t.Helper()
	e := entry(t, name)
	e.CreatedAt = booted.Add(time.Hour)
	return e
}

// owing is e marked owed a restore.
func owing(e sessionindex.Entry) sessionindex.Entry {
	e.Owed = true
	return e
}

// decisions 58 and 62: with no saver running, none saw the sessions end, so
// the bootstrap marks this boot's owed, and they come back with the rest.
func TestMarkOwed(t *testing.T) {
	t.Run("it marks owed every entry not live but Portal's own, under the commit lock", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "live"})
		w.save(t, thisBoot(t, "since"), entry(t, "before"), thisBoot(t, "live"), thisBoot(t, "_portal-saver"))

		if err := w.r.MarkOwed(t.Context()); err != nil {
			t.Fatalf("MarkOwed: %v", err)
		}

		for name, want := range map[string]bool{"since": true, "before": true, "live": false, "_portal-saver": false} {
			if got, _ := w.lookup(t, name); got.Owed != want {
				t.Errorf("%s owed = %v, want %v", name, got.Owed, want)
			}
		}
	})

	t.Run("it marks nothing, and fails, when the sessions can't be listed", func(t *testing.T) {
		w := newWorld(t)
		listing := errors.New("read zmx dir: permission denied")
		w.fake.Fail(backendtest.OpList, listing)
		w.save(t, thisBoot(t, "since"))

		if err := w.r.MarkOwed(t.Context()); !errors.Is(err, listing) {
			t.Errorf("MarkOwed = %v, want the listing's failure", err)
		}
		if got, _ := w.lookup(t, "since"); got.Owed {
			t.Errorf("since = %+v, want it unmarked", got)
		}
	})

	t.Run("it fails when the commit lock can't be had", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, thisBoot(t, "since"))
		commitlock.SetTimeoutForTest(t, 50*time.Millisecond)
		commitlock.HoldForTest(t, w.dir)

		if err := w.r.MarkOwed(t.Context()); !errors.Is(err, commitlock.ErrHeld) {
			t.Errorf("MarkOwed = %v, want ErrHeld", err)
		}
	})

	t.Run("it marks nothing in an index it can't read, and leaves that for Restore to report", func(t *testing.T) {
		w := newWorld(t)
		if err := os.WriteFile(sessionindex.Path(w.dir), []byte("{"), 0o600); err != nil {
			t.Fatal(err)
		}

		if err := w.r.MarkOwed(t.Context()); err != nil {
			t.Errorf("MarkOwed = %v, want nil", err)
		}
		if calls := w.fake.CallsTo(backendtest.OpList); len(calls) != 0 {
			t.Errorf("listed %d times, want none for an index with nothing to mark", len(calls))
		}
	})
}

func TestRestore_OwedEntries(t *testing.T) {
	// decision 72: a restore completes at its helper's claim, so until then
	// the mark keeps the entry from a drop and brings it back should the
	// helper die. An entry from before the boot records a creation of this
	// boot once restore has made it, so it is marked too.
	t.Run("it restores an owed entry of this boot with the rest, each owed until its helper finishes", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, owing(thisBoot(t, "since")), entry(t, "before"))

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 2}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"since", "before"}) {
			t.Errorf("created %q, want both", got)
		}
		for _, name := range []string{"since", "before"} {
			if got, _ := w.lookup(t, name); !got.Owed {
				t.Errorf("%s = %+v, want it owed until its helper's claim", name, got)
			}
		}
	})

	t.Run("a session whose helper never finished is made again by the next restore once it has ended", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.r.Restore(t.Context())
		w.fake.Remove("work")
		if _, err := state.Hydrating.Clear(w.dir, "work"); err != nil {
			t.Fatal(err)
		}

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"work", "work"}) {
			t.Errorf("created %q, want work made by each restore", got)
		}
		made, _ := w.fake.Held("work")
		if got, _ := w.lookup(t, "work"); !got.Owed || got.PID != made.PID || !got.CreatedAt.Equal(made.Created) {
			t.Errorf("work = %+v, want the second creation, %+v, recorded and owed until its helper's claim", got, made)
		}
	})

	t.Run("an owed entry whose Create fails stays owed for the next restore", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Fail(backendtest.OpCreate, errRefused)
		w.save(t, owing(thisBoot(t, "since")))

		if res := w.r.Restore(t.Context()); res.Failed != 1 {
			t.Errorf("Restore = %+v, want one failed", res)
		}
		if got, _ := w.lookup(t, "since"); !got.Owed {
			t.Errorf("since = %+v, want it owed still", got)
		}
	})

	t.Run("it still skips an owed entry live, and Portal's own", func(t *testing.T) {
		w := newWorld(t, backend.Session{Name: "work"})
		w.save(t, owing(thisBoot(t, "work")), owing(thisBoot(t, "_portal-saver")))

		if res := w.r.Restore(t.Context()); res != (restore.Result{}) {
			t.Errorf("Restore = %+v, want nothing", res)
		}
		if created := w.created(); len(created) > 0 {
			t.Errorf("created %q, want nothing", created)
		}
	})
}

// assertSkipped checks the restore made nothing and raised no flag for name,
// saying why at DEBUG.
func (w *world) assertSkipped(t *testing.T, res restore.Result, name, reason string) {
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
	skip := w.sink.Records().WithMessage("session skipped").AtExactLevel(slog.LevelDebug).Only(t, "skip")
	if got := skip.AttrString(t, "session"); got != name {
		t.Errorf("skip names %q, want %q", got, name)
	}
	if got := skip.AttrString(t, "reason"); got != reason {
		t.Errorf("skip reason = %q, want %q", got, reason)
	}
}

// decision 56: Create's name rules are zmx's, and the saver saves no session
// under a name they refuse, but an index from before that rule may hold one.
func TestRestore_NameCreateRefuses(t *testing.T) {
	w := newWorld(t)
	w.save(t, entry(t, "logs"), entry(t, "-x"), entry(t, "foo*"), entry(t, "work"))

	res := w.r.Restore(t.Context())

	if want := (restore.Result{Restored: 1, Failed: 3}); res != want {
		t.Errorf("Restore = %+v, want %+v", res, want)
	}
	if got := w.created(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("created %q, want work alone: a name Create refuses is never tried", got)
	}
	failures := w.sink.Records().WithMessage("restore session failed").AtExactLevel(slog.LevelWarn)
	var named []string
	for _, rec := range failures {
		named = append(named, rec.AttrString(t, "session"))
		if !errors.Is(rec.ErrorAttr(t, "error"), backend.ErrInvalidName) {
			t.Errorf("%s's failure = %v, want it to say the name can never be created", named[len(named)-1], rec.Attrs)
		}
	}
	if !slices.Equal(named, []string{"logs", "-x", "foo*"}) {
		t.Errorf("failures name %q, want each refused name", named)
	}
	for _, name := range []string{"logs", "foo*"} {
		if w.hydrating(t, name) {
			t.Errorf("%s's hydrating flag was raised for a name Create refuses", name)
		}
	}
	summary := w.sink.Records().WithMessage("restore complete").Only(t, "restore summary")
	if summary.IntAttr(t, "restored") != 1 || summary.IntAttr(t, "failed") != 3 || summary.IntAttr(t, "anomalous") != 3 {
		t.Errorf("summary = %v, want 1 restored, 3 failed and 3 anomalous", summary.Attrs)
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

// A Create that failed may have made the session all the same, so restore
// looks the name up before it settles the session's flag and its count.
func TestRestore_FailedCreate(t *testing.T) {
	type outcome struct {
		res restore.Result
		// keepsOwn and keepsEarlier say whether the hydrating flag is still up
		// after, raised by this run or before it.
		keepsOwn, keepsEarlier bool
	}
	var (
		restored = outcome{res: restore.Result{Restored: 1}, keepsOwn: true, keepsEarlier: true}
		other    = outcome{res: restore.Result{Failed: 1}}
		none     = outcome{res: restore.Result{Failed: 1}, keepsEarlier: true}
		unknown  = outcome{res: restore.Result{Failed: 1}, keepsOwn: true, keepsEarlier: true}
	)
	makes := func(w *world, then func(name string) error) {
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				return backend.Session{}, err
			}
			return backend.Session{}, then(name)
		})
	}
	first := func(w *world, made func(ctx context.Context, name, dir string, command []string)) {
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			made(ctx, name, dir, command)
			return w.fake.Create(ctx, name, dir, command)
		})
	}

	for _, tc := range []struct {
		name  string
		stage func(w *world)
		want  outcome
	}{
		{
			name: "a session restore's helper runs, though Create timed out, is restored",
			stage: func(w *world) {
				makes(w, func(string) error { return errTimedOut })
			},
			want: restored,
		},
		{
			name: "a session a concurrent restore made first, running restore's helper, is restored",
			stage: func(w *world) {
				first(w, func(ctx context.Context, name, dir string, command []string) {
					_, _ = w.fake.Create(ctx, name, dir, command)
				})
			},
			want: restored,
		},
		{
			name: "a session made since the listing, running anything else, fails",
			stage: func(w *world) {
				first(w, func(_ context.Context, name, _ string, _ []string) {
					w.fake.Add(backend.Session{Name: name, Command: "vim notes.md"})
				})
			},
			want: other,
		},
		{
			name: "a session made since the listing, running the user's shell, fails",
			stage: func(w *world) {
				first(w, func(_ context.Context, name, _ string, _ []string) { w.fake.Add(backend.Session{Name: name}) })
			},
			want: other,
		},
		// zmx finds a session by statting its socket, so on a filesystem that
		// folds case one held as WORK answers for work.
		{
			name:  "a session held under another spelling fails",
			stage: func(w *world) { w.fake.Add(backend.Session{Name: "WORK"}) },
			want:  other,
		},
		{
			name:  "a refusal leaving no session fails",
			stage: func(w *world) { w.fake.Fail(backendtest.OpCreate, errRefused) },
			want:  none,
		},
		{
			name: "a refusal as held that names no holder, and holds nothing, fails",
			stage: func(w *world) {
				w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
					return backend.Session{}, &backend.SessionExistsError{Name: name}
				})
			},
			want: none,
		},
		{
			name: "a session that can't be looked up fails, as one that may be the helper's",
			stage: func(w *world) {
				makes(w, func(name string) error {
					w.fake.StopAnswering(name)
					return errTimedOut
				})
			},
			want: unknown,
		},
	} {
		for _, earlier := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s (flag raised before: %v)", tc.name, earlier), func(t *testing.T) {
				w := newWorld(t)
				saved := entry(t, "work")
				w.save(t, saved)
				if earlier {
					if err := state.Hydrating.Set(w.dir, "work"); err != nil {
						t.Fatal(err)
					}
				}
				tc.stage(w)

				res := w.r.Restore(t.Context())

				if res != tc.want.res {
					t.Errorf("Restore = %+v, want %+v", res, tc.want.res)
				}
				if up, want := w.hydrating(t, "work"), tc.want.keepsOwn && !earlier || tc.want.keepsEarlier && earlier; up != want {
					t.Errorf("hydrating flag up = %v, want %v", up, want)
				}
				if tc.want.res.Restored == 0 {
					w.assertFailedKeepingEntry(t, saved)
					return
				}
				w.assertRestoredWithoutWarning(t, saved)
			})
		}
	}
}

func (w *world) assertFailedKeepingEntry(t *testing.T, saved sessionindex.Entry) {
	t.Helper()
	w.failureLogged(t, saved.Name)
	if e, ok := w.lookup(t, saved.Name); !ok || !sameEntry(e, saved) {
		t.Errorf("entry = %+v, %v; want it kept as saved for the next bootstrap, %+v", e, ok, saved)
	}
}

func (w *world) assertRestoredWithoutWarning(t *testing.T, saved sessionindex.Entry) {
	t.Helper()
	live, ok := w.fake.Held(saved.Name)
	if !ok {
		t.Fatalf("%s is not held", saved.Name)
	}
	if got, _ := w.lookup(t, saved.Name); !sameEntry(got, restoredAs(saved, live)) {
		t.Errorf("entry = %+v, want the helper's creation recorded, owed until it finishes, %+v", got, restoredAs(saved, live))
	}
	if warnings := w.warnings(); len(warnings) > 0 {
		t.Errorf("a session restore made logged %v, want no warning", warnings)
	}
}

// restoredAs is saved as restore records it once it has made live: live's
// creation, owed a restore until live's helper finishes it.
func restoredAs(saved sessionindex.Entry, live backend.Session) sessionindex.Entry {
	saved.CreatedAt, saved.PID, saved.Owed = live.Created, live.PID, true
	return saved
}

// failureLogged checks one WARN names the session that failed and the summary
// counts it, and returns that WARN.
func (w *world) failureLogged(t *testing.T, name string) logtest.Record {
	t.Helper()
	rec := w.sink.Records().WithMessage("restore session failed").AtExactLevel(slog.LevelWarn).Only(t, "failure")
	if got := rec.AttrString(t, "session"); got != name {
		t.Errorf("failure names session %q, want %q", got, name)
	}
	if got := w.anomalous(t); got != 1 {
		t.Errorf("anomalous = %d, want the one failure", got)
	}
	return rec
}

// A Create cut short once zmx has forked the session's daemon, before its
// socket listens, leaves a session no one can reach yet: the first ask finds
// none, though the helper is about to start.
func TestRestore_AsksAgainBeforeJudgingNoSession(t *testing.T) {
	t.Run("a session that answers on the second ask, running restore's helper, is restored", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			w.asleep = func() {
				if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
					t.Fatal(err)
				}
			}
			return backend.Session{}, errTimedOut
		})

		if res, want := w.r.Restore(t.Context()), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if !w.hydrating(t, "work") {
			t.Error("the hydrating flag came down under a helper about to start")
		}
		w.assertRestoredWithoutWarning(t, saved)
		if !slices.Equal(w.slept, []time.Duration{time.Second}) {
			t.Errorf("waited %v, want one probe ceiling, a second", w.slept)
		}
	})

	t.Run("a session no ask finds fails once both have, on each try", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.fake.Fail(backendtest.OpCreate, errRefused)

		w.r.Restore(t.Context())

		var asks []string
		for _, call := range w.fake.Calls() {
			if call.Op == backendtest.OpCreate || call.Op == backendtest.OpSession {
				asks = append(asks, string(call.Op))
			}
		}
		create, ask := string(backendtest.OpCreate), string(backendtest.OpSession)
		tries := len(w.created())
		want := []string{create, ask, ask}
		for range tries - 1 {
			want = append(want, ask, create, ask, ask)
		}
		if !slices.Equal(asks, want) {
			t.Errorf("calls = %q, want each of the %d tries' Create followed by two asks, each retry asking first", asks, tries)
		}
		if len(w.slept) == 0 || w.slept[0] != time.Second {
			t.Errorf("waited %v, want the first try's asks a second apart", w.slept)
		}
		if w.hydrating(t, "work") {
			t.Error("the hydrating flag this run raised is left up")
		}
	})

	// The second ask's wait outlives a cancelled bootstrap, which leaves
	// nothing to try again.
	t.Run("New's restorer waits out the second", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		ctx, cancel := context.WithCancelCause(t.Context())
		w.createWith(func(context.Context, string, string, []string) (backend.Session, error) {
			cancel(errors.New("bootstrap abandoned"))
			return backend.Session{}, errRefused
		})
		r, err := restore.New(restore.Config{Backend: w.witness, StateDir: w.dir, Exe: exe})
		if err != nil {
			t.Fatal(err)
		}
		restore.JudgeByBoot(r, booted)
		start := time.Now()

		r.Restore(ctx)

		if took := time.Since(start); took < time.Second {
			t.Errorf("Restore took %v, want at least the second between the asks", took)
		}
	})

	t.Run("a holder found on the first ask is not asked for again", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		w.fake.Add(backend.Session{Name: "WORK"})

		w.r.Restore(t.Context())

		if asks := len(w.fake.CallsTo(backendtest.OpSession)); asks != 1 || len(w.slept) > 0 {
			t.Errorf("asked %d times, waiting %v; want once, with no wait", asks, w.slept)
		}
	})
}

func TestRestore_RefusedCreateNamesItsCause(t *testing.T) {
	w := newWorld(t)
	w.save(t, entry(t, "work"))
	w.fake.Fail(backendtest.OpCreate, errRefused)

	w.r.Restore(t.Context())

	if cause := w.failureLogged(t, "work").ErrorAttr(t, "error"); !errors.Is(cause, errRefused) {
		t.Errorf("logged cause %v, want Create's refusal", cause)
	}
}

func TestRestore_CountsEveryWarningAsAnomalous(t *testing.T) {
	t.Run("a creation it can't record", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		commitlock.SetTimeoutForTest(t, 50*time.Millisecond)
		commitlock.HoldForTest(t, w.dir)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v: the session was made", res, want)
		}
		w.warnedOnce(t, "record session creation failed")
	})

	t.Run("a flag it can't lower", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "work"))
		flags := filepath.Join(w.dir, string(state.Hydrating))
		w.createWith(func(_ context.Context, name, _ string, _ []string) (backend.Session, error) {
			if err := os.Chmod(flags, 0o500); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(flags, 0o700) })
			w.fake.Add(backend.Session{Name: name, Command: "vim notes.md"})
			return backend.Session{}, errRefused
		})

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Failed: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		w.sink.Records().WithMessage("clear hydrating flag failed").AtExactLevel(slog.LevelWarn).Only(t, "clear failure")
		if got := w.anomalous(t); got != 2 {
			t.Errorf("anomalous = %d, want the failure and the flag left up", got)
		}
	})

	t.Run("a transcript file it refuses", func(t *testing.T) {
		w := newWorld(t)
		e, _ := w.saveTranscript(t, entry(t, "notes"))
		e.Name = "work"
		w.save(t, e)

		res := w.r.Restore(t.Context())

		if want := (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		w.warnedOnce(t, "scrollback file refused")
	})
}

// warnedOnce checks the run's one warning is msg, naming work, and the summary
// counts it.
func (w *world) warnedOnce(t *testing.T, msg string) {
	t.Helper()
	rec := w.warnings().Only(t, "warning")
	if rec.Msg != msg || rec.AttrString(t, "session") != "work" {
		t.Errorf("warning = %q %v, want %q naming work", rec.Msg, rec.Attrs, msg)
	}
	if got := w.anomalous(t); got != 1 {
		t.Errorf("anomalous = %d, want 1", got)
	}
}

func TestRestore_StopsOnceItsContextEnds(t *testing.T) {
	cause := errors.New("bootstrap abandoned")

	t.Run("it makes no session after the one it was making", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "a"), entry(t, "b"), entry(t, "c"))
		ctx, cancel := context.WithCancelCause(t.Context())
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			s, err := w.fake.Create(ctx, name, dir, command)
			cancel(cause)
			return s, err
		})

		res := w.r.Restore(ctx)

		if want := (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v: the rest neither made nor failed", res, want)
		}
		if got := w.created(); !slices.Equal(got, []string{"a"}) {
			t.Errorf("created %q, want a alone", got)
		}
		w.assertCancelled(t, cause)
		for _, name := range []string{"b", "c"} {
			if w.hydrating(t, name) {
				t.Errorf("%s's hydrating flag was raised after the context ended", name)
			}
		}
	})

	t.Run("it still looks up a session the Create it cut short made, and restores it", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "a")
		w.save(t, saved, entry(t, "b"))
		ctx, cancel := context.WithCancelCause(t.Context())
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			if _, err := w.fake.Create(ctx, name, dir, command); err != nil {
				return backend.Session{}, err
			}
			cancel(cause)
			return backend.Session{}, cause
		})

		if res, want := w.r.Restore(ctx), (restore.Result{Restored: 1}); res != want {
			t.Errorf("Restore = %+v, want %+v", res, want)
		}
		if !w.hydrating(t, "a") {
			t.Error("a's hydrating flag is down: its helper runs, and lowers it")
		}
		live, _ := w.fake.Held("a")
		if got, _ := w.lookup(t, "a"); !sameEntry(got, restoredAs(saved, live)) {
			t.Errorf("entry = %+v, want the helper's creation recorded, %+v", got, restoredAs(saved, live))
		}
		w.assertCancelled(t, cause)
	})

	t.Run("it makes none once it has ended before the run", func(t *testing.T) {
		w := newWorld(t)
		w.save(t, entry(t, "a"))
		ctx, cancel := context.WithCancelCause(t.Context())
		cancel(cause)

		if res := w.r.Restore(ctx); res != (restore.Result{}) {
			t.Errorf("Restore = %+v, want nothing", res)
		}
		if created := w.created(); len(created) > 0 {
			t.Errorf("created %q, want nothing", created)
		}
		w.assertCancelled(t, cause)
	})
}

func (w *world) assertCancelled(t *testing.T, cause error) {
	t.Helper()
	rec := w.warnings().Only(t, "warning")
	if rec.Msg != "restore cancelled" || !errors.Is(rec.ErrorAttr(t, "error"), cause) {
		t.Errorf("warning = %q %v, want the cancellation, once, with its cause", rec.Msg, rec.Attrs)
	}
}

func TestRestore_StartDir(t *testing.T) {
	gone := filepath.Join(t.TempDir(), "gone")
	// An executable file passes the search check a directory must, so only
	// being no directory refuses it.
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	sealed := t.TempDir()
	if err := os.Chmod(sealed, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sealed, 0o700) })
	cwd, dir, home := t.TempDir(), t.TempDir(), t.TempDir()

	for _, tc := range []struct {
		name, cwd, dir, home, want string
	}{
		{name: "its last working directory", cwd: cwd, dir: dir, want: cwd},
		{name: "the directory it was made in, its last one gone", cwd: gone, dir: dir, want: dir},
		{name: "the directory it was made in, its last one a file", cwd: file, dir: dir, want: dir},
		{name: "the directory it was made in, its last one not searchable", cwd: sealed, dir: dir, want: dir},
		{name: "the directory it was made in, its last one relative", cwd: ".", dir: dir, want: dir},
		{name: "the directory it was made in, recording no last one", dir: dir, want: dir},
		{name: "HOME, both gone", cwd: gone, dir: gone, want: home},
		{name: "HOME, both relative", cwd: ".", dir: ".", want: home},
		{name: "the root, HOME gone as well", cwd: gone, dir: gone, home: gone, want: "/"},
		{name: "the root, HOME not searchable", cwd: gone, dir: gone, home: sealed, want: "/"},
		{name: "the root, HOME relative", cwd: gone, dir: gone, home: ".", want: "/"},
	} {
		t.Run("it starts in "+tc.name, func(t *testing.T) {
			t.Setenv("HOME", cmp.Or(tc.home, home))
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
		if warnings := w.warnings(); len(warnings) > 0 {
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

func TestRunsHelper(t *testing.T) {
	t.Run("it counts each session restore made, with a transcript or without", func(t *testing.T) {
		w := newWorld(t)
		withFile, _ := w.saveTranscript(t, entry(t, "work"))
		w.save(t, withFile, entry(t, "notes"))

		w.r.Restore(t.Context())

		for _, name := range []string{"work", "notes"} {
			s, ok := w.fake.Held(name)
			if !ok {
				t.Fatalf("%s was not restored", name)
			}
			if !w.r.RunsHelper(s) {
				t.Errorf("RunsHelper(%+v) = false, want restore's own session counted", s)
			}
		}
	})

	for _, tc := range []struct {
		name    string
		command []string
	}{
		{"the helper for another name", []string{exe, "state", "hydrate", "--session", "work-2"}},
		{"another portal's helper", []string{"/usr/local/bin/portal", "state", "hydrate", "--session", "work"}},
		{"another of portal's commands", []string{exe, "state", "resume-wait", "--session", "work"}},
		{"another program", []string{"vim", "notes.md"}},
		{"the user's shell", nil},
	} {
		t.Run("it counts no session running "+tc.name, func(t *testing.T) {
			w := newWorld(t)
			s, err := w.fake.Create(t.Context(), "work", t.TempDir(), tc.command)
			if err != nil {
				t.Fatal(err)
			}

			if w.r.RunsHelper(s) {
				t.Errorf("RunsHelper(%+v) = true, want false", s)
			}
		})
	}

	t.Run("it counts no session that didn't answer, known by its name alone", func(t *testing.T) {
		if newWorld(t).r.RunsHelper(backend.Session{Name: "work"}) {
			t.Error("RunsHelper = true for a session that told nothing of its command")
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
		if got, _ := w.lookup(t, "work"); !sameEntry(got, restoredAs(e, live)) {
			t.Errorf("entry = %+v, want %+v", got, restoredAs(e, live))
		}
	})

	// The second ask of an unconfirmed Create reads no creation either, as a
	// session the backend never answers for: the helper's claim records it.
	t.Run("a creation no ask reads changes nothing", func(t *testing.T) {
		w := newWorld(t)
		w.fake.Now = func() time.Time { return time.Time{} }
		saved := entry(t, "work")
		w.save(t, saved)

		if res := w.r.Restore(t.Context()); res.Restored != 1 {
			t.Fatalf("Restore = %+v, want work restored", res)
		}

		if got, _ := w.lookup(t, "work"); !sameEntry(got, saved) {
			t.Errorf("entry = %+v, want it as saved, %+v", got, saved)
		}
	})

	// A raw recreation of the name, claimed while restore waited for the lock,
	// is the live session now: recording restore's creation over it would have
	// the next claim replace the entry and lose what it was saved with.
	t.Run("an entry a claim took over since restore read it is left to the claim", func(t *testing.T) {
		w := newWorld(t)
		saved := entry(t, "work")
		w.save(t, saved)
		claimed := saved
		claimed.CreatedAt, claimed.PID = time.Now().Truncate(time.Second), 9999
		w.createWith(func(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
			w.save(t, claimed)
			return w.fake.Create(ctx, name, dir, command)
		})

		if res := w.r.Restore(t.Context()); res.Restored != 1 {
			t.Fatalf("Restore = %+v, want work restored", res)
		}

		if got, _ := w.lookup(t, "work"); !sameEntry(got, claimed) {
			t.Errorf("entry = %+v, want the claim's, %+v", got, claimed)
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

// decision 75: a Create the backend refuses because the session it made speaks
// a protocol Portal doesn't support stops the restore there, since every
// Create after it would make another; the entries it didn't reach stay.
func TestRestore_StopsOnASessionOfAnUnsupportedProtocol(t *testing.T) {
	w := newWorld(t)
	w.fake.Fail(backendtest.OpCreate, fmt.Errorf("create session %q: %w", "work", backend.ErrUnsupportedProtocol))
	w.save(t, entry(t, "work"), entry(t, "notes"))

	res := w.r.Restore(t.Context())

	if !errors.Is(res.Unsupported, backend.ErrUnsupportedProtocol) || res.Failed != 1 || res.Restored != 0 {
		t.Errorf("Restore = %+v, want one failed, carrying the refusal", res)
	}
	if got := w.created(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("created %q, want work alone: nothing is made after the refusal", got)
	}
	for _, name := range []string{"work", "notes"} {
		if w.hydrating(t, name) {
			t.Errorf("%s's hydrating flag is up, want it down: no session runs the helper", name)
		}
		if _, ok := w.lookup(t, name); !ok {
			t.Errorf("%s's entry is gone, want it kept for the next restore", name)
		}
	}
}
