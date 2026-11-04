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
	"github.com/leeovery/portal/internal/state"
)

const (
	forgotten = "api-x7Kd9a"
	// neighbour shares forgotten's name as a prefix, so a removal matching by
	// prefix would take it too.
	neighbour = "api-x7Kd9a-2"
)

// stageForgettable stages a state directory holding everything Portal keeps
// for forgotten and neighbour, and the state files no session owns, beside a
// hooks.json registering a hook for each.
func stageForgettable(t *testing.T) (dir string, store *hooks.Store, hooksPath string) {
	t.Helper()
	dir = t.TempDir()
	for _, name := range []string{forgotten, neighbour} {
		for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
			if err := flag.Set(dir, name); err != nil {
				t.Fatal(err)
			}
		}
		path, err := state.SessionScrollbackPath(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, path, "transcript of "+name)
	}
	writeFile(t, filepath.Join(dir, "scrollback", forgotten+".bin.old"), "not the session's file")
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
	return dir, store, hooksPath
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

func TestForget(t *testing.T) {
	t.Run("it removes the session's two flags, scrollback and hook, and nothing else", func(t *testing.T) {
		dir, store, _ := stageForgettable(t)
		before := tree(t, dir)

		state.Forget(dir, store, forgotten, "cli")

		want := maps.Clone(before)
		for _, gone := range []string{"hydrating/" + forgotten, "resume-pending/" + forgotten, "scrollback/" + forgotten + ".bin"} {
			if _, ok := want[gone]; !ok {
				t.Fatalf("the fixture staged no %s", gone)
			}
			delete(want, gone)
		}
		if got := tree(t, dir); !maps.Equal(got, want) {
			t.Errorf("state dir = %v\nwant %v", slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want)))
		}
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want %s's alone", got, neighbour)
		}
	})

	t.Run("it logs one INFO naming what went, with the removed hook's command", func(t *testing.T) {
		dir, store, _ := stageForgettable(t)
		sink := logtest.Install(t)

		state.Forget(dir, store, forgotten, "tui")

		rec := sink.Records().Matching("sessions", "forget").Only(t, "forget record")
		logtest.AssertRecord(t, rec, logtest.RecordWant{Level: slog.LevelInfo, Msg: "forget", Component: "sessions", Op: "forget", Via: "tui"})
		if got := rec.AttrString(t, "session"); got != forgotten {
			t.Errorf("session = %q, want %q", got, forgotten)
		}
		if got, want := rec.AttrString(t, "detail"), "removed its hydrating flag, resume-pending flag, scrollback, resume hook"; got != want {
			t.Errorf("detail = %q, want %q", got, want)
		}
		if got := rec.AttrString(t, "value"); got != "claude --resume" {
			t.Errorf("value = %q, want the hook's command, to copy back out of the log", got)
		}
		sink.Records().Matching("hooks", "rm").Only(t, "the hooks store's own removal record")
	})

	t.Run("a session with nothing kept logs nothing and takes no hooks lock", func(t *testing.T) {
		dir := t.TempDir()
		store, hooksPath := hookstest.StageStore(t, hookstest.Staging{SidecarAbsent: true, Entries: map[string]string{neighbour: "vim"}})
		before := tree(t, dir)
		sink := logtest.Install(t)

		state.Forget(dir, store, forgotten, "cli")

		if n := len(sink.Records().AtOrAboveLevel(slog.LevelInfo)); n != 0 {
			t.Errorf("records at INFO or above = %d, want none", n)
		}
		if got := tree(t, dir); !maps.Equal(got, before) {
			t.Errorf("state dir = %v, want it untouched", got)
		}
		assertAbsent(t, hookstest.SidecarPath(hooksPath))
	})

	t.Run("a nil hook store keeps the hook and still removes the rest", func(t *testing.T) {
		dir, store, _ := stageForgettable(t)

		state.Forget(dir, nil, forgotten, "cli")

		if got := hookKeys(t, store); !slices.Equal(got, []string{forgotten, neighbour}) {
			t.Errorf("hooks = %v, want both kept", got)
		}
		assertAbsent(t, filepath.Join(dir, "hydrating", forgotten))
	})

	t.Run("a removal that fails is logged and the rest still go", func(t *testing.T) {
		dir, store, _ := stageForgettable(t)
		scrollback := filepath.Join(dir, "scrollback")
		if err := os.Chmod(scrollback, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(scrollback, 0o700) })
		sink := logtest.Install(t)

		state.Forget(dir, store, forgotten, "cli")

		rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the failed removal's warning")
		if got := rec.AttrString(t, "detail"); got != "kept its scrollback" {
			t.Errorf("detail = %q, want it to name the scrollback", got)
		}
		if got, want := rec.AttrString(t, "path"), filepath.Join(scrollback, forgotten+".bin"); got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if rec.ErrorAttr(t, "error") == nil {
			t.Error("the warning carries no error")
		}
		assertAbsent(t, filepath.Join(dir, "hydrating", forgotten))
		assertAbsent(t, filepath.Join(dir, "resume-pending", forgotten))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want the hook removed all the same", got)
		}
	})

	t.Run("a hooks lock held past its bound keeps the hook, warns, and the files still go", func(t *testing.T) {
		dir, store, hooksPath := stageForgettable(t)
		hookstest.HoldHooksSidecar(t, hooksPath)
		hooks.SetLockTimeoutForTest(t, 30*time.Millisecond)
		sink := logtest.Install(t)

		state.Forget(dir, store, forgotten, "cli")

		rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the kept hook's warning")
		if got := rec.AttrString(t, "detail"); got != "kept its resume hook" {
			t.Errorf("detail = %q, want it to name the hook", got)
		}
		if got := hookKeys(t, store); !slices.Equal(got, []string{forgotten, neighbour}) {
			t.Errorf("hooks = %v, want both kept", got)
		}
		assertAbsent(t, filepath.Join(dir, "scrollback", forgotten+".bin"))
	})

	// Forget may run where commit.lock is held: it must never take that lock
	// itself, or the order commit.lock, then the hooks lock could invert.
	t.Run("it takes no commit lock, finishing while another holder has it", func(t *testing.T) {
		dir, store, _ := stageForgettable(t)
		commitlock.HoldForTest(t, dir)
		commitlock.SetTimeoutForTest(t, 30*time.Millisecond)

		state.Forget(dir, store, forgotten, "cli")

		assertAbsent(t, filepath.Join(dir, "hydrating", forgotten))
		assertAbsent(t, filepath.Join(dir, "scrollback", forgotten+".bin"))
		if got := hookKeys(t, store); !slices.Equal(got, []string{neighbour}) {
			t.Errorf("hooks = %v, want the hook removed", got)
		}
	})

	for _, name := range unsafeNames {
		t.Run("the name "+strconv.Quote(name)+" removes no file, and warns", func(t *testing.T) {
			// Empty flag directories, and files an unsafe name would compose
			// as a scrollback path, which a removal by that name would take.
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

			state.Forget(dir, store, name, "cli")

			if got := tree(t, dir); !maps.Equal(got, before) {
				t.Errorf("state dir = %v, want it untouched", got)
			}
			rec := sink.Records().Matching("sessions", "forget").AtExactLevel(slog.LevelWarn).Only(t, "the refusal's warning")
			if got := rec.AttrString(t, "detail"); !strings.Contains(got, "files") {
				t.Errorf("detail = %q, want it to say the files were kept", got)
			}
		})
	}
}
