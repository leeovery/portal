package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/session"
)

type fakeRunner struct {
	gitRoot string
	err     error
	called  bool
}

func (r *fakeRunner) Run(name string, args ...string) (string, error) {
	r.called = true
	if r.err != nil {
		return "", r.err
	}
	return r.gitRoot, nil
}

func TestGroupingDir(t *testing.T) {
	t.Run("it resolves the cwd to its canonical git root", func(t *testing.T) {
		gitRoot := t.TempDir()
		cwd := filepath.Join(gitRoot, "sub", "dir")
		if err := os.MkdirAll(cwd, 0o755); err != nil {
			t.Fatalf("mkdir cwd: %v", err)
		}

		dir, ok := session.GroupingDir(cwd, &fakeRunner{gitRoot: gitRoot})

		if !ok {
			t.Fatal("ok = false for a resolvable cwd")
		}
		if want := project.CanonicalDirKey(gitRoot); dir != want {
			t.Errorf("dir = %q, want canonical %q", dir, want)
		}
	})

	t.Run("it derives nothing from an empty cwd, and never asks git", func(t *testing.T) {
		runner := &fakeRunner{gitRoot: "/should/not/be/used"}

		dir, ok := session.GroupingDir(" ", runner)

		if ok || dir != "" {
			t.Errorf("GroupingDir = %q, %v; want nothing", dir, ok)
		}
		if runner.called {
			t.Error("git-root resolution ran for an empty cwd")
		}
	})

	t.Run("it derives nothing from a cwd gone from disk", func(t *testing.T) {
		gone := filepath.Join(t.TempDir(), "gone")

		dir, ok := session.GroupingDir(gone, &fakeRunner{gitRoot: gone})

		if ok || dir != "" {
			t.Errorf("GroupingDir = %q, %v; want nothing", dir, ok)
		}
	})

	t.Run("a cwd outside a repository yields the cwd itself", func(t *testing.T) {
		cwd := t.TempDir()

		dir, ok := session.GroupingDir(cwd, &fakeRunner{err: errors.New("not a git repository")})

		if !ok || dir != project.CanonicalDirKey(cwd) {
			t.Errorf("GroupingDir = %q, %v; want the canonical cwd %q", dir, ok, project.CanonicalDirKey(cwd))
		}
	})
}
