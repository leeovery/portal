package state_test

import (
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/commitlock"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

const (
	forgotten = "api-x7Kd9a"
	// neighbour shares forgotten's name as a prefix, so a removal matching by
	// prefix would take it too.
	neighbour = "api-x7Kd9a-2"
)

// stageForgettable stages a state directory holding everything Portal keeps
// for forgotten and neighbour, a later creation's scrollback under
// forgotten's name, and the state files no session owns, beside a hooks.json
// registering a hook for each. It returns forgotten's entry as the index held
// it.
func stageForgettable(t *testing.T) (dir string, entry sessionindex.Entry, store *hooks.Store, hooksPath string) {
	t.Helper()
	dir = t.TempDir()
	for _, name := range []string{forgotten, neighbour} {
		stageKept(t, dir, name)
	}
	later := scrollbackFile(t, forgotten, apiCreated.Add(time.Hour), 5151)
	writeFile(t, filepath.Join(dir, filepath.FromSlash(later)), "a later creation's transcript")
	writeFile(t, filepath.Join(dir, "scrollback", forgotten+".bin"), "named by the name alone")
	writeFile(t, filepath.Join(dir, "scrollback", apiStem+".bin.old"), "not the session's file")
	writeFile(t, filepath.Join(dir, "sessions.json"), `{"version":2,"sessions":[]}`)
	if err := state.Restoring.Set(dir); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteLatch(dir, running); err != nil {
		t.Fatal(err)
	}
	store, hooksPath = hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{
		forgotten: "claude --resume",
		neighbour: "vim",
	}})
	entry = sessionindex.Entry{
		Name: forgotten, Label: "api work", CreatedAt: apiCreated, PID: 4242,
		ScrollbackFile: scrollbackFile(t, forgotten, apiCreated, 4242),
	}
	return dir, entry, store, hooksPath
}

// stageKept raises both of name's flags and writes the scrollback of its
// creation at apiCreated.
func stageKept(t *testing.T, dir, name string) {
	t.Helper()
	for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
		if err := flag.Set(dir, name); err != nil {
			t.Fatal(err)
		}
	}
	file := scrollbackFile(t, name, apiCreated, 4242)
	writeFile(t, filepath.Join(dir, filepath.FromSlash(file)), "transcript of "+name)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// tree is every file and directory under dir, by relative path, with each
// file's content.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			files[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(path)
		files[rel] = string(data)
		return err
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return files
}

func hookKeys(t *testing.T, store *hooks.Store) []string {
	t.Helper()
	held, err := store.Load(hooks.ViaInternal)
	if err != nil {
		t.Fatalf("load hooks: %v", err)
	}
	return slices.Sorted(maps.Keys(held))
}

// without is before less each of gone, every one of which the fixture staged.
func without(t *testing.T, before map[string]string, gone ...string) map[string]string {
	t.Helper()
	want := maps.Clone(before)
	for _, path := range gone {
		if _, ok := want[path]; !ok {
			t.Fatalf("the fixture staged no %s", path)
		}
		delete(want, path)
	}
	return want
}

func assertTree(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	if got := tree(t, dir); !maps.Equal(got, want) {
		t.Errorf("state dir = %v\nwant %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
	}
}

func TestForget(t *testing.T) {
	t.Run("it removes the session's two flags, the scrollback its entry names, and its hook, and nothing else", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		before := tree(t, dir)

		state.Forget(dir, store, entry, "cli")

		assertTree(t, dir, without(t, before, "hydrating/"+apiStem, "resume-pending/"+apiStem, filepath.FromSlash(entry.ScrollbackFile)))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want %s's alone", got, neighbour)
		}
	})

	t.Run("it logs one INFO naming what went, and the hooks store's discard carries the command and the caller's via", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		sink := logtest.Install(t)

		state.Forget(dir, store, entry, "tui")

		rec := sink.Records().Matching("sessions", "forget").Only(t, "forget record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "forget", Component: "sessions", Op: "forget", Via: "tui"})
		if got := rec.AttrString(t, "session"); got != forgotten {
			t.Errorf("session = %q, want %q", got, forgotten)
		}
		if got, want := rec.AttrString(t, "detail"), "removed its hydrating flag, resume-pending flag, scrollback, resume hook"; got != want {
			t.Errorf("detail = %q, want %q", got, want)
		}
		discard := sink.Records().Matching("hooks", "discard").Only(t, "the hooks store's own removal record")
		logtest.AssertRecord(t, discard, logtest.RecordWant{Level: slog.LevelInfo, Msg: "discard", Component: "hooks", Op: "discard", Via: "tui"})
		if got := discard.AttrString(t, "value"); got != "claude --resume" {
			t.Errorf("value = %q, want the hook's command, to copy back out of the log", got)
		}
	})

	for _, via := range []string{"cli", "tui", "internal"} {
		t.Run("the hooks store's records carry the caller's via, "+via, func(t *testing.T) {
			dir, entry, store, _ := stageForgettable(t)
			sink := logtest.Install(t)

			state.Forget(dir, store, entry, via)

			rec := sink.Records().Matching("hooks", "discard").Only(t, "discard record")
			if got := rec.AttrOrEmpty("via"); got != via {
				t.Errorf("discard via = %q, want %q", got, via)
			}
		})
	}

	t.Run("a hook set again after Forget read it is kept", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		state.AfterForgetReadsHooksForTest(t, func() {
			if err := store.Set(forgotten, hooks.EventOnResume, hooks.Registration{Command: "make serve"}, hooks.ViaCLI); err != nil {
				t.Fatalf("set the hook again: %v", err)
			}
		})
		sink := logtest.Install(t)

		state.Forget(dir, store, entry, "cli")

		held, err := store.Load(hooks.ViaInternal)
		if err != nil {
			t.Fatal(err)
		}
		if got := held[forgotten][hooks.EventOnResume.String()].Command; got != "make serve" {
			t.Errorf("hook = %q, want the one set again kept", got)
		}
		rec := sink.Records().Matching("sessions", "forget").Only(t, "forget record")
		if got := rec.AttrString(t, "detail"); strings.Contains(got, "resume hook") {
			t.Errorf("detail = %q, want no hook named among what went", got)
		}
	})

	t.Run("an entry naming no scrollback removes no scrollback, the rest still going", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		entry.ScrollbackFile = ""
		before := tree(t, dir)

		state.Forget(dir, store, entry, "cli")

		assertTree(t, dir, without(t, before, "hydrating/"+apiStem, "resume-pending/"+apiStem))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want %s's alone", got, neighbour)
		}
	})

	t.Run("an entry naming another session's scrollback keeps it, and warns", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		entry.ScrollbackFile = scrollbackFile(t, neighbour, apiCreated, 4242)
		before := tree(t, dir)
		sink := logtest.Install(t)

		state.Forget(dir, store, entry, "cli")

		assertTree(t, dir, without(t, before, "hydrating/"+apiStem, "resume-pending/"+apiStem))
		rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the kept scrollback's warning")
		if got := rec.AttrString(t, "detail"); got != "kept its scrollback" {
			t.Errorf("detail = %q, want it to name the scrollback", got)
		}
		if got := rec.AttrString(t, "path"); got != entry.ScrollbackFile {
			t.Errorf("path = %q, want the file the entry named, %q", got, entry.ScrollbackFile)
		}
	})

	t.Run("forgetting a name keeps everything of a name differing only in case", func(t *testing.T) {
		dir := t.TempDir()
		stageKept(t, dir, "Foo")
		store, _ := hookstest.StageStore(t, hookstest.Staging{Entries: map[string]string{"Foo": "vim"}})
		before := tree(t, dir)

		state.Forget(dir, store, sessionindex.Entry{Name: "foo", ScrollbackFile: scrollbackFile(t, "foo", apiCreated, 4242)}, "cli")

		assertTree(t, dir, before)
		if got := hookKeys(t, store); !slices.Equal(got, []string{"Foo"}) {
			t.Errorf("hooks = %v, want Foo's kept", got)
		}
		if !foldsCase(t, dir) {
			t.Log("this filesystem keeps case, so it could not have folded the names together")
		}
	})

	t.Run("a session with nothing kept logs nothing and takes no hooks lock", func(t *testing.T) {
		dir := t.TempDir()
		store, hooksPath := hookstest.StageStore(t, hookstest.Staging{SidecarAbsent: true, Entries: map[string]string{neighbour: "vim"}})
		before := tree(t, dir)
		sink := logtest.Install(t)

		state.Forget(dir, store, sessionindex.Entry{Name: forgotten}, "cli")

		if n := len(sink.Records().AtOrAboveLevel(slog.LevelInfo)); n != 0 {
			t.Errorf("records at INFO or above = %d, want none", n)
		}
		assertTree(t, dir, before)
		assertAbsent(t, hookstest.SidecarPath(hooksPath))
	})

	t.Run("a nil hook store keeps the hook and still removes the rest", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)

		state.Forget(dir, nil, entry, "cli")

		if got := hookKeys(t, store); !slices.Equal(got, []string{forgotten, neighbour}) {
			t.Errorf("hooks = %v, want both kept", got)
		}
		assertAbsent(t, filepath.Join(dir, "hydrating", apiStem))
	})

	t.Run("a removal that fails is logged and the rest still go", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		scrollback := filepath.Join(dir, "scrollback")
		if err := os.Chmod(scrollback, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(scrollback, 0o700) })
		sink := logtest.Install(t)

		state.Forget(dir, store, entry, "cli")

		rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the failed removal's warning")
		if got := rec.AttrString(t, "detail"); got != "kept its scrollback" {
			t.Errorf("detail = %q, want it to name the scrollback", got)
		}
		if got, want := rec.AttrString(t, "path"), filepath.Join(dir, filepath.FromSlash(entry.ScrollbackFile)); got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if rec.ErrorAttr(t, "error") == nil {
			t.Error("the warning carries no error")
		}
		assertAbsent(t, filepath.Join(dir, "hydrating", apiStem))
		assertAbsent(t, filepath.Join(dir, "resume-pending", apiStem))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want the hook removed all the same", got)
		}
	})

	t.Run("a hooks lock held past its bound keeps the hook, warns, and the files still go", func(t *testing.T) {
		dir, entry, store, hooksPath := stageForgettable(t)
		hookstest.HoldHooksSidecar(t, hooksPath)
		hooks.SetLockTimeoutForTest(t, 30*time.Millisecond)
		sink := logtest.Install(t)

		state.Forget(dir, store, entry, "cli")

		rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the kept hook's warning")
		if got := rec.AttrString(t, "detail"); got != "kept its resume hook" {
			t.Errorf("detail = %q, want it to name the hook", got)
		}
		if got := hookKeys(t, store); !slices.Equal(got, []string{forgotten, neighbour}) {
			t.Errorf("hooks = %v, want both kept", got)
		}
		assertAbsent(t, filepath.Join(dir, filepath.FromSlash(entry.ScrollbackFile)))
	})

	// Forget may run where commit.lock is held: it must never take that lock
	// itself, or the order commit.lock, then the hooks lock could invert.
	t.Run("it takes no commit lock, finishing while another holder has it", func(t *testing.T) {
		dir, entry, store, _ := stageForgettable(t)
		commitlock.HoldForTest(t, dir)
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)

		state.Forget(dir, store, entry, "cli")

		assertAbsent(t, filepath.Join(dir, "hydrating", apiStem))
		assertAbsent(t, filepath.Join(dir, filepath.FromSlash(entry.ScrollbackFile)))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want the hook removed", got)
		}
	})

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" removes no file, and warns", func(t *testing.T) {
			// Empty flag directories, and files an unsafe name might compose as
			// a path, which a removal by that name would take.
			dir := t.TempDir()
			for _, sub := range []string{"hydrating", "resume-pending"} {
				if err := os.Mkdir(filepath.Join(dir, sub), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			for _, decoy := range []string{".bin", "..bin", "...bin"} {
				writeFile(t, filepath.Join(dir, "scrollback", decoy), "decoy")
			}
			store, _ := hookstest.StageStore(t, hookstest.Staging{})
			before := tree(t, dir)
			sink := logtest.Install(t)

			state.Forget(dir, store, sessionindex.Entry{Name: name, ScrollbackFile: "scrollback/.bin"}, "cli")

			assertTree(t, dir, before)
			rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the refusal's warning")
			if got := rec.AttrString(t, "detail"); !strings.Contains(got, "files") {
				t.Errorf("detail = %q, want it to say the files were kept", got)
			}
		})
	}
}
