package cmd

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

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/hooks"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// stageKept gives each named session everything Portal keeps beside its index
// entry in the state directory dir: both flags and a scrollback file.
func stageKept(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		for _, flag := range []state.SessionFlag{state.Hydrating, state.ResumePending} {
			if err := flag.Set(dir, name); err != nil {
				t.Fatal(err)
			}
		}
		path, err := state.SessionScrollbackPath(dir, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("transcript"), 0o600); err != nil {
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
	path, err := state.SessionScrollbackPath(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
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

	t.Run("a kill whose index write fails forgets nothing", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		dir := stageKill(t, fake, "api-x7Kd9a")
		stageKept(t, dir, "api-x7Kd9a")
		store, _ := stageHooks(t, "api-x7Kd9a")
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		_, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

		if err == nil || !strings.HasPrefix(err.Error(), "killed api-x7Kd9a, but could not remove it from the session index: ") {
			t.Fatalf("kill = %v, want the index failure reported", err)
		}
		assertKept(t, dir, store, "api-x7Kd9a", everythingKept)
	})

	// The order is commit.lock, then the hooks lock: the index write has landed,
	// and its lock is released, before the hook's removal waits on its own.
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
			`{"name": "`+currentSessionName+`", "label": "old", "created_at": "2026-10-28T09:00:00Z", "scrollback_file": "scrollback/`+currentSessionName+`.bin"}]}`)
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

func TestUpdateSessionIndex_Forgets(t *testing.T) {
	t.Run("a claim replacing an earlier creation's entry, as a mint's does, forgets it", func(t *testing.T) {
		dir := seedSessionsJSON(t, `{"version": 2, "sessions": [`+
			`{"name": "api-x7Kd9a", "label": "old", "created_at": "2026-10-28T09:00:00Z"},`+
			`{"name": "web-Qm3pLz", "label": "web", "created_at": "2026-10-28T09:00:00Z"}]}`)
		stageKept(t, dir, "api-x7Kd9a", "web-Qm3pLz")
		store, _ := stageHooks(t, "api-x7Kd9a", "web-Qm3pLz")

		err := updateSessionIndex("tui", func(idx *sessionindex.Index) error {
			idx.Claim("api-x7Kd9a", "/work/api", time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC), 4242)
			return nil
		})
		if err != nil {
			t.Fatalf("updateSessionIndex: %v", err)
		}

		assertKept(t, dir, store, "api-x7Kd9a", nil)
		assertKept(t, dir, store, "web-Qm3pLz", everythingKept)
	})

	t.Run("an update that drops nothing reads no hooks.json", func(t *testing.T) {
		isolateStateDir(t)
		hooksPath := filepath.Join(t.TempDir(), "hooks.json")
		if err := os.Mkdir(hooksPath, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PORTAL_HOOKS_FILE", hooksPath)
		sink := logtest.Install(t)

		if err := updateSessionIndex("cli", func(idx *sessionindex.Index) error { return idx.SetLabel("a", "Alpha") }); err != nil {
			t.Fatalf("updateSessionIndex: %v", err)
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

		if err := updateSessionIndex("cli", func(idx *sessionindex.Index) error { idx.Remove("api-x7Kd9a"); return nil }); err != nil {
			t.Fatalf("updateSessionIndex: %v", err)
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
