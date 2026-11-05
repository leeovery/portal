package cmd

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/fileutil"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// keptCreated is the creation whose scrollback stageKept writes.
var keptCreated = time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)

// keptScrollback is the scrollback file stageKept writes for name, as an
// index entry records it.
func keptScrollback(t *testing.T, name string) string {
	t.Helper()
	file, err := state.SessionScrollbackFile(name, keptCreated, 1717)
	if err != nil {
		t.Fatal(err)
	}
	return file
}

// stageKept gives each named session everything Portal keeps beside its index
// entry in the state directory dir: both flags, and a scrollback file, which
// the session's entry, where it has one, records.
func stageKept(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
			if err := flag.Set(dir, name); err != nil {
				t.Fatal(err)
			}
		}
		file := keptScrollback(t, name)
		path := filepath.Join(dir, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("transcript"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
			idx.SetScrollback(name, file)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

// stageHooks stages a hooks.json registering a hook for each name, where the
// commands under test resolve it, and returns a store over it.
func stageHooks(t *testing.T, names ...string) (*hooks.Store, string) {
	t.Helper()
	entries := map[string]string{}
	for _, name := range names {
		entries[name] = "resume " + name
	}
	store, path := hookstest.StageStore(t, hookstest.Staging{Entries: entries})
	t.Setenv("PORTAL_HOOKS_FILE", path)
	return store, path
}

// keptFor lists what the state directory and hooks.json still keep for name.
func keptFor(t *testing.T, dir string, store *hooks.Store, name string) []string {
	t.Helper()
	var kept []string
	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		if raised, err := flag.Exists(dir, name); err != nil || raised {
			kept = append(kept, string(flag))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(keptScrollback(t, name)))); !errors.Is(err, os.ErrNotExist) {
		kept = append(kept, "scrollback")
	}
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := held[name]; ok {
		kept = append(kept, "hook")
	}
	return kept
}

var everythingKept = []string{"hydrating", "resume-pending", "scrollback", "hook"}

func assertKept(t *testing.T, dir string, store *hooks.Store, name string, want []string) {
	t.Helper()
	if got := keptFor(t, dir, store, name); !slices.Equal(got, want) {
		t.Errorf("kept for %s = %v, want %v", name, got, want)
	}
}

func TestKill_Forgets(t *testing.T) {
	t.Run("it forgets the killed session: its flags, scrollback and hook go, a neighbour's stay", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"}, backend.Session{Name: "api-x7Kd9a-2"})
		dir := stageKill(t, fake, "api-x7Kd9a", "api-x7Kd9a-2")
		stageKept(t, dir, "api-x7Kd9a", "api-x7Kd9a-2")
		store, _ := stageHooks(t, "api-x7Kd9a", "api-x7Kd9a-2")

		if _, stderr, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill: %v\nstderr: %s", err, stderr)
		}

		assertKept(t, dir, store, "api-x7Kd9a", nil)
		assertKept(t, dir, store, "api-x7Kd9a-2", everythingKept)
	})

	t.Run("it forgets a session that ended on its own as it drops its entry", func(t *testing.T) {
		dir := stageKill(t, backendtest.New(), "api-x7Kd9a")
		stageKept(t, dir, "api-x7Kd9a")
		store, _ := stageHooks(t, "api-x7Kd9a")

		_, _, _ = runRootCmd(t, "kill", "api-x7Kd9a")

		assertIndexed(t, dir, "api-x7Kd9a", false)
		assertKept(t, dir, store, "api-x7Kd9a", nil)
	})

	t.Run("it forgets the flags and hook of a session the index holds no entry for, keeping a scrollback no entry names", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		dir := stageKill(t, fake)
		stageKept(t, dir, "api-x7Kd9a")
		store, _ := stageHooks(t, "api-x7Kd9a")
		sink := logtest.Install(t)

		if _, stderr, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill: %v\nstderr: %s", err, stderr)
		}

		assertKept(t, dir, store, "api-x7Kd9a", []string{"scrollback"})
		r := sink.Records().Matching("sessions", "forget").Only(t, "forget record")
		logtest.AssertRecord(t, r, logtest.RecordWant{Level: slog.LevelInfo, Msg: "forget", Component: "sessions", Op: "forget", Via: "cli"})
	})

	t.Run("a kill the backend refuses forgets nothing", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "wedged-Lp4s"})
		fake.StopAnswering("wedged-Lp4s")
		dir := stageKill(t, fake, "wedged-Lp4s")
		stageKept(t, dir, "wedged-Lp4s")
		store, _ := stageHooks(t, "wedged-Lp4s")

		if _, _, err := runRootCmd(t, "kill", "wedged-Lp4s"); err == nil {
			t.Fatal("kill = nil, want the refusal")
		}

		assertKept(t, dir, store, "wedged-Lp4s", everythingKept)
	})

	for name, indexed := range map[string]bool{"an entry": true, "no entry": false} {
		t.Run("a kill whose index write fails, of a session with "+name+", forgets nothing", func(t *testing.T) {
			fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
			dir := stageKill(t, fake)
			if indexed {
				storedLabel(t, dir, "api-x7Kd9a", "api work")
			}
			stageKept(t, dir, "api-x7Kd9a")
			store, _ := stageHooks(t, "api-x7Kd9a")
			commitlock.HoldForTest(t, dir)
			commitlock.SetTimeoutForTest(t, 30*time.Millisecond)

			_, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

			if err == nil || !strings.HasPrefix(err.Error(), "killed api-x7Kd9a, but could not remove it from the session index: ") {
				t.Fatalf("kill = %v, want the index failure reported", err)
			}
			assertKept(t, dir, store, "api-x7Kd9a", everythingKept)
		})
	}

	// The order is commit.lock, then the hooks lock: the index write has landed
	// before the hook's removal waits on the hooks lock, so a wait past its
	// bound costs only the hook.
	t.Run("a hooks lock held past its bound costs the hook, never the kill or its index write", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		dir := stageKill(t, fake, "api-x7Kd9a")
		stageKept(t, dir, "api-x7Kd9a")
		store, hooksPath := stageHooks(t, "api-x7Kd9a")
		release := hookstest.HoldHooksSidecar(t, hooksPath)
		hooks.SetLockTimeoutForTest(t, 30*time.Millisecond)
		sink := logtest.Install(t)

		if _, _, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill = %v, want it to succeed", err)
		}

		assertIndexed(t, dir, "api-x7Kd9a", false)
		release()
		assertKept(t, dir, store, "api-x7Kd9a", []string{"hook"})
		r := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the kept hook's warning")
		if got := r.AttrString(t, "detail"); got != "kept its resume hook" {
			t.Errorf("detail = %q, want the hook named", got)
		}
	})
}

func TestRename_ForgetsAnEarlierCreation(t *testing.T) {
	t.Run("a label claimed over an earlier creation's entry forgets what that one kept", func(t *testing.T) {
		fake, _ := stageRename(t)
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
			`{"name": "`+currentSessionName+`", "label": "old", "created_at": "2026-10-28T09:00:00Z"}]}`)
		stageKept(t, dir, currentSessionName)
		store, _ := stageHooks(t, currentSessionName)

		if err := (pickerLabeller{ctx: context.Background(), b: fake}).SetLabel(currentSessionName, "api work"); err != nil {
			t.Fatalf("SetLabel: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "api work")
		assertKept(t, dir, store, currentSessionName, nil)
	})

	t.Run("a label on the entry's own creation forgets nothing", func(t *testing.T) {
		_, dir := stageRename(t)
		labelLive(t, dir, "old")
		stageKept(t, dir, currentSessionName)
		store, _ := stageHooks(t, currentSessionName)

		if _, _, err := runRootCmd(t, "rename", "api work"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "api work")
		assertKept(t, dir, store, currentSessionName, everythingKept)
	})
}

// A session saved before the boot, then brought back by a restore that read
// back no creation for it, keeps what it was saved with: the next claim of it
// takes the entry over rather than replacing it.
func TestClaim_KeepsAnEntryRestoreRecordedNoCreationFor(t *testing.T) {
	saved := sessionindex.Entry{
		Name: currentSessionName, Label: "api work", Dir: "/work/api", Cwd: "/work/api/src",
		CreatedAt: testBoot.AddDate(0, 0, -3), PID: 1717,
	}
	stage := func(t *testing.T) (*backendtest.Fake, string, *hooks.Store) {
		t.Helper()
		fake, _ := stageRename(t)
		dir := os.Getenv("PORTAL_STATE_DIR")
		if _, err := sessionindex.Update(dir, "internal", func(idx *sessionindex.Index) error {
			if _, err := idx.Claim(saved.Name, saved.Dir, saved.CreatedAt, saved.PID, claimRule(dir, acceptName)); err != nil {
				return err
			}
			if err := idx.SetLabel(saved.Name, saved.Label); err != nil {
				return err
			}
			idx.RecordCwd(saved.Name, saved.Cwd)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		stageKept(t, dir, currentSessionName)
		store, _ := stageHooks(t, currentSessionName)
		if _, err := sessionindex.Update(dir, "internal", func(idx *sessionindex.Index) error {
			if idx.Restored(saved, time.Time{}, 0) {
				t.Error("Restored of no creation = true, want it to record nothing")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return fake, dir, store
	}
	assertAdopted := func(t *testing.T, dir, label string) {
		t.Helper()
		want := saved
		want.Label, want.CreatedAt, want.PID, want.ScrollbackFile = label, renameCreated, renamePID, keptScrollback(t, currentSessionName)
		want.Owed = true // its hydrating flag is up: its restore is unfinished until its helper's claim
		if got := entryOf(t, dir, currentSessionName); !got.CreatedAt.Equal(want.CreatedAt) || !sameButCreation(got, want) {
			t.Errorf("entry = %+v\nwant %+v", got, want)
		}
	}

	t.Run("the live session shows the saved label before any claim", func(t *testing.T) {
		fake, _, _ := stage(t)
		live, err := fake.Session(context.Background(), currentSessionName)
		if err != nil {
			t.Fatal(err)
		}

		shown, err := readShownIndex()
		if err != nil {
			t.Fatal(err)
		}
		if got := shown.show(live); got.Label != "api work" || got.Dir != "/work/api" {
			t.Errorf("shown = %+v, want the saved label and dir", got)
		}
	})

	t.Run("a rename takes the entry over, keeping its dir, cwd, scrollback, flags and hook", func(t *testing.T) {
		_, dir, store := stage(t)

		if _, _, err := runRootCmd(t, "rename", "api work 2"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertAdopted(t, dir, "api work 2")
		assertKept(t, dir, store, currentSessionName, everythingKept)
	})

	t.Run("a hook set takes the entry over, keeping its label, dir, cwd, scrollback and flags", func(t *testing.T) {
		_, dir, store := stage(t)

		if _, err := runHookSet(t, "claude --resume"); err != nil {
			t.Fatalf("hook set: %v", err)
		}

		assertAdopted(t, dir, "api work")
		assertKept(t, dir, store, currentSessionName, everythingKept)
	})
}

func sameButCreation(a, b sessionindex.Entry) bool {
	a.CreatedAt, b.CreatedAt = time.Time{}, time.Time{}
	return a == b
}

// bareReadback is a backend whose Create reads back nothing of the session it
// made but its name, as one whose readback failed does.
type bareReadback struct{ *backendtest.Fake }

func (b bareReadback) Create(ctx context.Context, name, dir string, command []string) (backend.Session, error) {
	if _, err := b.Fake.Create(ctx, name, dir, command); err != nil {
		return backend.Session{}, err
	}
	return backend.Session{Name: name}, nil
}

// bareMinter is a creator over b that asks again with no wait, recording each
// wait it would have made.
func bareMinter(b backend.Backend, waited *[]time.Duration) indexingCreator {
	c := newIndexingCreator(context.Background(), &recordingProjectStore{}, b, "cli")
	c.wait = func(_ context.Context, d time.Duration) error {
		*waited = append(*waited, d)
		return nil
	}
	return c
}

// A Create whose read of the session it made outlasts its ceiling under load
// reads back no creation; asked again, the session reports one. The session
// exists whatever the mint's context does, so that ask outlasts it.
func TestMint_AsksAgainForACreationItsCreateReadBackNone(t *testing.T) {
	for name, endsFirst := range map[string]bool{
		"its answer is recorded":                                   false,
		"a context ending before it still gets it, and records it": true,
	} {
		t.Run(name, func(t *testing.T) {
			fake := backendtest.New()
			stateDir := stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
			dir := filepath.Join(t.TempDir(), "myproj")
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			var waited []time.Duration
			minter := bareMinter(bareReadback{fake}, &waited)
			if endsFirst {
				ctx, cancel := context.WithCancel(t.Context())
				minter.ctx, minter.wait = ctx, endingFirst(cancel, minter.wait)
			}

			minted, err := minter.CreateFromDir(dir, nil)
			if err != nil {
				t.Fatalf("CreateFromDir: %v", err)
			}

			held, _ := fake.Held(minted)
			if e := entryOf(t, stateDir, minted); !e.CreatedAt.Equal(held.Created) || e.PID != held.PID || e.Dir != dir {
				t.Errorf("entry = %+v, want the creation the second ask read, %v pid %d, and the minted dir %s", e, held.Created, held.PID, dir)
			}
			if !slices.Equal(waited, []time.Duration{backend.RecheckAfter}) {
				t.Errorf("waited %v, want one probe ceiling before the second ask", waited)
			}
		})
	}
}

// endingFirst is wait with cancel called as it starts.
func endingFirst(cancel context.CancelFunc, wait func(context.Context, time.Duration) error) func(context.Context, time.Duration) error {
	return func(ctx context.Context, d time.Duration) error {
		cancel()
		return wait(ctx, d)
	}
}

// decision 74: a mint never records an entry without a creation. A session
// minted with none read back, twice, is warned of and left unrecorded, for a
// claim to record once it reports one: a hook set on it survives a rename.
func TestMint_RecordsNoEntryForASessionReportingNoCreation(t *testing.T) {
	sink := logtest.Install(t)
	fake := backendtest.New()
	stateDir := stageMint(t, fake, time.Date(2026, 11, 3, 9, 30, 0, 0, time.UTC))
	withBackendDeps(t, BackendDeps{Backend: fake})
	fake.Fail(backendtest.OpSession, backend.ErrTimeout)
	var waited []time.Duration
	minted, err := bareMinter(bareReadback{fake}, &waited).CreateFromDir(t.TempDir(), nil)
	if err != nil {
		t.Fatalf("CreateFromDir: %v", err)
	}
	fake.Fail(backendtest.OpSession, nil)
	if len(fake.CallsTo(backendtest.OpSession)) != 1 {
		t.Errorf("Session calls = %d, want the one second ask", len(fake.CallsTo(backendtest.OpSession)))
	}
	if idx := readIndex(t, stateDir); idx.Len() != 0 {
		e, _ := idx.Lookup(minted)
		t.Fatalf("minted entry = %+v, want none recorded without a creation", e)
	}
	r := sink.Records().Matching("sessions", "unrecorded").AtExactLevel(slog.LevelWarn).Only(t, "unrecorded warning")
	if err := r.ErrorAttr(t, "error"); !errors.Is(err, sessionindex.ErrNoCreation) {
		t.Errorf("unrecorded error = %v, want it to name the missing creation", err)
	}
	fake.SetCurrent(minted)
	_, hooksFile := hooksFileInTempDir(t, nil)

	if _, err := runHookSet(t, "claude --resume"); err != nil {
		t.Fatalf("hook set: %v", err)
	}
	if _, _, err := runRootCmd(t, "rename", "api work"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	if got := readHooksJSON(t, hooksFile)[minted]["on-resume"].Command; got != "claude --resume" {
		t.Errorf("hook = %q, want it kept through the rename", got)
	}
	held, _ := fake.Held(minted)
	if e := entryOf(t, stateDir, minted); e.Label != "api work" || !e.CreatedAt.Equal(held.Created) || e.PID != held.PID {
		t.Errorf("entry = %+v, want the label on the live creation", e)
	}
}

func TestUpdateSessionIndex_Forgets(t *testing.T) {
	t.Run("a claim replacing an earlier creation's entry, as a mint's does, forgets it", func(t *testing.T) {
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
			`{"name": "api-x7Kd9a", "label": "old", "created_at": "2026-10-28T09:00:00Z"},`+
			`{"name": "web-Qm3pLz", "label": "web", "created_at": "2026-10-28T09:00:00Z"}]}`)
		stageKept(t, dir, "api-x7Kd9a", "web-Qm3pLz")
		store, _ := stageHooks(t, "api-x7Kd9a", "web-Qm3pLz")

		err := claimSession("tui", acceptName, claimed{Name: "api-x7Kd9a", Dir: "/work/api", Created: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), PID: 4242}, nil, nil)
		if err != nil {
			t.Fatalf("claimSession: %v", err)
		}

		assertKept(t, dir, store, "api-x7Kd9a", nil)
		assertKept(t, dir, store, "web-Qm3pLz", everythingKept)
	})

	t.Run("a replaced entry's own scrollback goes, and the scrollback a later write names stays", func(t *testing.T) {
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [{"name": "api-x7Kd9a", "created_at": "2026-10-28T09:00:00Z"}]}`)
		stageKept(t, dir, "api-x7Kd9a")
		stageHooks(t)
		created := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
		later, err := state.SessionScrollbackFile("api-x7Kd9a", created, 4242)
		if err != nil {
			t.Fatal(err)
		}
		laterPath := filepath.Join(dir, filepath.FromSlash(later))
		if err := os.WriteFile(laterPath, []byte("the new creation's transcript"), 0o600); err != nil {
			t.Fatal(err)
		}

		err = claimSession("cli", acceptName, claimed{Name: "api-x7Kd9a", Created: created, PID: 4242}, func(idx *sessionindex.Index) error {
			idx.SetScrollback("api-x7Kd9a", later)
			return nil
		}, nil)
		if err != nil {
			t.Fatalf("claimSession: %v", err)
		}

		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(keptScrollback(t, "api-x7Kd9a")))); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stat the replaced entry's scrollback = %v, want it removed", err)
		}
		if _, err := os.Stat(laterPath); err != nil {
			t.Errorf("stat the new entry's scrollback = %v, want it kept", err)
		}
	})

	t.Run("an update that drops nothing reads no hooks.json", func(t *testing.T) {
		isolateStateDir(t)
		hooksPath := filepath.Join(t.TempDir(), "hooks.json")
		if err := os.Mkdir(hooksPath, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
		sink := logtest.Install(t)

		live := claimed{Name: "a", Created: time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), PID: 4242}
		if err := claimSession("cli", acceptName, live, func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") }, nil); err != nil {
			t.Fatalf("claimSession: %v", err)
		}

		if recs := sink.Records().Matching("sessions", "forget"); len(recs) != 0 {
			t.Errorf("forget records = %d, want none", len(recs))
		}
		if recs := sink.Records().WithMessage("load-unlocked"); len(recs) != 0 {
			t.Errorf("hooks reads = %d, want none", len(recs))
		}
	})

	t.Run("hooks.json that can't be found keeps every hook, warns, and the rest still go", func(t *testing.T) {
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [{"name": "api-x7Kd9a"}]}`)
		stageKept(t, dir, "api-x7Kd9a")
		t.Setenv("PORTAL_HOOKS_FILE", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")
		sink := logtest.Install(t)

		if err := forgetSession("api-x7Kd9a", "cli"); err != nil {
			t.Fatalf("forgetSession: %v", err)
		}

		r := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the kept hook's warning")
		if got := r.AttrString(t, "session"); got != "api-x7Kd9a" {
			t.Errorf("session = %q, want api-x7Kd9a", got)
		}
		if raised, err := state.Hydrating.Exists(dir, "api-x7Kd9a"); err != nil || raised {
			t.Errorf("hydrating flag = %v, %v; want it removed", raised, err)
		}
	})
}

// A write whose rename lands has landed, whatever the directory sync after it
// answers: what went with a dropped entry goes too.
func TestIndexWrite_LandsThoughTheDirectorySyncFails(t *testing.T) {
	t.Run("a kill succeeds, its entry goes, and everything kept beside it is forgotten", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		dir := stageKill(t, fake, "api-x7Kd9a")
		stageKept(t, dir, "api-x7Kd9a")
		store, _ := stageHooks(t, "api-x7Kd9a")
		fileutil.FailDirSyncForTest(t, syscall.EIO)

		if _, stderr, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill = %v, want it to succeed\nstderr: %s", err, stderr)
		}

		if idx := readIndex(t, dir); idx.Len() != 0 {
			t.Errorf("index = %+v, want it empty", idx.Entries())
		}
		assertKept(t, dir, store, "api-x7Kd9a", nil)
	})

	t.Run("a rename replacing an earlier creation's entry forgets that one's hook", func(t *testing.T) {
		fake, _ := stageRename(t)
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
			`{"name": "`+currentSessionName+`", "label": "old", "created_at": "2026-10-28T09:00:00Z"}]}`)
		stageKept(t, dir, currentSessionName)
		store, _ := stageHooks(t, currentSessionName)
		fileutil.FailDirSyncForTest(t, syscall.EIO)

		if err := setSessionLabel(context.Background(), fake, currentSessionName, "api work", "cli"); err != nil {
			t.Fatalf("rename = %v, want it to succeed", err)
		}

		assertLabel(t, dir, currentSessionName, "api work")
		assertKept(t, dir, store, currentSessionName, nil)
	})
}

// The reviewers' interleaving: a rename replaces the entry an earlier creation
// left, and while it forgets that creation's hook, a hook set for the live
// creation claims, finding its own entry, and writes its hook. Were the
// forgetting run after the rename's unlock, it could read that hook and
// discard it.
func TestForget_RunsUnderTheCommitLock(t *testing.T) {
	fake, _ := stageRename(t)
	dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
		`{"name": "`+currentSessionName+`", "label": "old", "created_at": "2026-10-28T09:00:00Z"}]}`)
	store, _ := stageHooks(t, currentSessionName)
	paused, resume := make(chan struct{}), make(chan struct{})
	var pausing atomic.Bool
	pausing.Store(true)
	withFuncSeam(t, &forget, func(dir string, store *hooks.Store, via string, entries ...sessionindex.Entry) {
		if pausing.Swap(false) {
			close(paused)
			<-resume
		}
		state.Forget(dir, store, via, entries...)
	})

	renamed := make(chan error, 1)
	go func() { renamed <- setSessionLabel(context.Background(), fake, currentSessionName, "api work", "cli") }()
	<-paused
	set := make(chan error, 1)
	go func() {
		set <- setResumeHook(context.Background(), fake, currentSessionName, hooks.Registration{Command: "make serve"})
	}()
	select {
	case err := <-set:
		t.Errorf("hook set returned %v while the rename was forgetting, want it held at the commit lock", err)
		set <- err
	case <-time.After(200 * time.Millisecond):
	}
	close(resume)

	if err := <-renamed; err != nil {
		t.Fatalf("rename: %v", err)
	}
	if err := <-set; err != nil {
		t.Fatalf("hook set: %v", err)
	}
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatal(err)
	}
	if got := held[currentSessionName][hooks.EventOnResume.String()].Command; got != "make serve" {
		t.Errorf("hook = %q, want the live creation's, set after the forgetting, kept", got)
	}
	assertLabel(t, dir, currentSessionName, "api work")
}

// The kill in hook set's gap: a hook set claims the live session's entry, and
// before its hook lands a kill removes the entry and forgets what it keeps.
// Were the hook written after the claim's unlock, the kill would find no hook
// to forget, and the hook would then land on a dead name.
func TestHookSet_WritesItsHookUnderTheClaimsLock(t *testing.T) {
	fake, dir := stageRename(t)
	store, _ := stageHooks(t)
	paused, resume := make(chan struct{}), make(chan struct{})
	var pausing atomic.Bool
	pausing.Store(true)
	withFuncSeam(t, &registerHook, func(store *hooks.Store, key string, event hooks.Event, registration hooks.Registration, via hooks.Via) error {
		if pausing.Swap(false) {
			close(paused)
			<-resume
		}
		return store.Set(key, event, registration, via)
	})

	set := make(chan error, 1)
	go func() {
		set <- setResumeHook(context.Background(), fake, currentSessionName, hooks.Registration{Command: "make serve"})
	}()
	<-paused
	killed := make(chan error, 1)
	go func() { killed <- killSession(context.Background(), fake, currentSessionName, "cli") }()
	select {
	case err := <-killed:
		t.Errorf("kill returned %v while hook set was writing its hook, want it held at the commit lock", err)
		killed <- err
	case <-time.After(200 * time.Millisecond):
	}
	close(resume)

	if err := <-set; err != nil {
		t.Fatalf("hook set: %v", err)
	}
	if err := <-killed; err != nil {
		t.Fatalf("kill: %v", err)
	}
	if got := hookKeys(t, store); len(got) != 0 {
		t.Errorf("hooks = %v, want the killed session's forgotten", got)
	}
	assertIndexed(t, dir, currentSessionName, false)
}

// A hook set over an earlier creation's entry replaces it, and the forgetting
// of that entry, keyed by the same name, runs before the new hook is written.
func TestHookSet_KeepsItsHookOverAnEarlierCreationsEntry(t *testing.T) {
	fake, _ := stageRename(t)
	dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
		`{"name": "`+currentSessionName+`", "label": "old", "created_at": "2026-10-28T09:00:00Z"}]}`)
	stageKept(t, dir, currentSessionName)
	store, _ := stageHooks(t, currentSessionName)

	if err := setResumeHook(context.Background(), fake, currentSessionName, hooks.Registration{Command: "make serve"}); err != nil {
		t.Fatalf("hook set: %v", err)
	}

	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatal(err)
	}
	if got := held[currentSessionName][hooks.EventOnResume.String()].Command; got != "make serve" {
		t.Errorf("hook = %q, want the one set, not the earlier creation's or none", got)
	}
	if e := entryOf(t, dir, currentSessionName); !e.CreatedAt.Equal(renameCreated) || e.Label != "" {
		t.Errorf("entry = %+v, want the live creation's, made afresh", e)
	}
}

func hookKeys(t *testing.T, store *hooks.Store) []string {
	t.Helper()
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(held))
}
