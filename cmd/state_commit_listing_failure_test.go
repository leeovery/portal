package cmd

import (
	"bytes"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// savedStateFixture is a committed sessions.json naming two sessions, each
// with its transcript on disk.
type savedStateFixture struct {
	dir        string
	index      state.Index
	sessions   []byte
	scrollback map[string]string
}

func seedSavedState(t *testing.T) savedStateFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	if err := os.MkdirAll(state.ScrollbackDir(dir), 0o700); err != nil {
		t.Fatalf("create scrollback dir: %v", err)
	}
	idx := state.Index{Version: state.SchemaVersion, Sessions: []state.Session{}}
	for _, name := range []string{"work", "notes"} {
		file := state.SanitizePaneKey(name, 0, 0) + ".bin"
		if err := os.WriteFile(filepath.Join(state.ScrollbackDir(dir), file), []byte(name+"-transcript"), 0o600); err != nil {
			t.Fatalf("seed scrollback %s: %v", file, err)
		}
		idx.Sessions = append(idx.Sessions, state.Session{
			Name:        name,
			Environment: map[string]string{},
			Windows: []state.Window{{
				Index: 0, Name: "main", Layout: "tiled", Active: true,
				Panes: []state.Pane{{Index: 0, CWD: "/tmp", Active: true, CurrentCommand: "zsh", ScrollbackFile: "scrollback/" + file}},
			}},
		})
	}
	if err := state.Commit(dir, idx, false, nil); err != nil {
		t.Fatalf("seed sessions.json: %v", err)
	}
	sessions, err := os.ReadFile(state.SessionsJSON(dir))
	if err != nil {
		t.Fatalf("read seeded sessions.json: %v", err)
	}
	return savedStateFixture{dir: dir, index: idx, sessions: sessions, scrollback: readScrollbackDir(t, dir)}
}

func readScrollbackDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(state.ScrollbackDir(dir))
	if err != nil {
		t.Fatalf("read scrollback dir: %v", err)
	}
	contents := make(map[string]string, len(entries))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(state.ScrollbackDir(dir), e.Name()))
		if err != nil {
			t.Fatalf("read scrollback %s: %v", e.Name(), err)
		}
		contents[e.Name()] = string(data)
	}
	return contents
}

func (f savedStateFixture) assertUnchanged(t *testing.T) {
	t.Helper()
	sessions, err := os.ReadFile(state.SessionsJSON(f.dir))
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	if !bytes.Equal(sessions, f.sessions) {
		t.Errorf("sessions.json rewritten:\nbefore %s\nafter  %s", f.sessions, sessions)
	}
	if got := readScrollbackDir(t, f.dir); !maps.Equal(got, f.scrollback) {
		t.Errorf("scrollback = %v, want unchanged %v", got, f.scrollback)
	}
}

// failingListSessionsCommander answers @portal-restoring as unset and fails
// list-sessions the way the production commander reports a tmux failure.
func failingListSessionsCommander() *daemonFakeCommander {
	return &daemonFakeCommander{
		sessionsErr: &tmux.CommandError{
			Args:   []string{"list-sessions"},
			Stderr: "server exited unexpectedly",
			Err:    errors.New("exit status 1"),
		},
	}
}

func TestCommittersStandDownOnAFailedSessionListing(t *testing.T) {
	t.Run("the daemon's tick", func(t *testing.T) {
		saved := seedSavedState(t)
		deps := makeDeps(t, saved.dir, failingListSessionsCommander())
		deps.PrevIndex = &saved.index

		tick(t.Context(), deps)

		saved.assertUnchanged(t)
		if deps.PrevIndex != &saved.index {
			t.Error("PrevIndex replaced by a cycle whose session listing failed")
		}
	})

	t.Run("the daemon's shutdown flush", func(t *testing.T) {
		saved := seedSavedState(t)
		deps := makeDeps(t, saved.dir, failingListSessionsCommander())
		deps.PrevIndex = &saved.index

		if err := defaultShutdownFlush(deps); err != nil {
			t.Fatalf("defaultShutdownFlush: %v", err)
		}

		saved.assertUnchanged(t)
	})

	t.Run("commit-now", func(t *testing.T) {
		saved := seedSavedState(t)
		client := tmux.NewClient(failingListSessionsCommander())
		withCommitNowDeps(t, CommitNowDeps{
			NewClient:   func() state.CaptureCycleClient { return client },
			IsRestoring: func() (bool, error) { return state.IsRestoringSet(client) },
		})

		_, _, err := runRootCmd(t, "state", "commit-now")

		if !errors.Is(err, errCommitNowFailed) {
			t.Errorf("commit-now error = %v, want one wrapping errCommitNowFailed", err)
		}
		saved.assertUnchanged(t)
	})
}
