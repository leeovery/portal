package session_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
)

// fakePaneReader is a single-method stand-in for the active-pane reader seam.
// It exposes only ActivePaneCurrentPath, so a resolver that tried to enumerate
// all panes would have no method to call — the "active pane only" contract is
// structurally enforced. It records calls so tests can assert the resolver
// reads exactly once.
type fakePaneReader struct {
	path     string
	err      error
	sessions []string
}

func (f *fakePaneReader) ActivePaneCurrentPath(session string) (string, error) {
	f.sessions = append(f.sessions, session)
	return f.path, f.err
}

// fakeRunner is a resolver.CommandRunner stand-in. It records the argv it was
// invoked with and returns a canned git-root for rev-parse --show-toplevel.
type fakeRunner struct {
	gitRoot string
	err     error
	called  bool
	lastCmd string
	lastArg []string
}

func (r *fakeRunner) Run(name string, args ...string) (string, error) {
	r.called = true
	r.lastCmd = name
	r.lastArg = args
	if r.err != nil {
		return "", r.err
	}
	return r.gitRoot, nil
}

// Compile-time proof that the production *tmux.Client satisfies the narrow
// PaneCurrentPathReader seam, so production wiring is the trivial pass of a
// *tmux.Client plus a &resolver.RealCommandRunner{}.
var _ session.PaneCurrentPathReader = (*tmux.Client)(nil)

func TestResolveSessionDir(t *testing.T) {
	t.Run("resolves the active pane current_path to a canonical git root", func(t *testing.T) {
		// A real temp dir so ResolveGitRoot's os.Stat succeeds and
		// CanonicalDirKey can EvalSymlinks the derived root.
		gitRoot := t.TempDir()
		paneCwd := filepath.Join(gitRoot, "sub", "dir")
		if err := os.MkdirAll(paneCwd, 0o755); err != nil {
			t.Fatalf("mkdir pane cwd: %v", err)
		}

		reader := &fakePaneReader{path: paneCwd}
		runner := &fakeRunner{gitRoot: gitRoot}

		dir, ok, err := session.ResolveSessionDir("my-session", reader, runner)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected ok=true for a resolvable session")
		}
		want := project.CanonicalDirKey(gitRoot)
		if dir != want {
			t.Errorf("dir = %q, want canonical %q", dir, want)
		}
	})

	t.Run("reads only the active pane exactly once", func(t *testing.T) {
		gitRoot := t.TempDir()
		reader := &fakePaneReader{path: gitRoot}
		runner := &fakeRunner{gitRoot: gitRoot}

		_, _, err := session.ResolveSessionDir("my-session", reader, runner)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(reader.sessions) != 1 {
			t.Fatalf("expected exactly 1 active-pane read, got %d: %v", len(reader.sessions), reader.sessions)
		}
		if reader.sessions[0] != "my-session" {
			t.Errorf("read session %q, want %q", reader.sessions[0], "my-session")
		}
	})

	t.Run("returns unresolvable when the session was killed mid-resolve", func(t *testing.T) {
		killed := errors.New("display-message failed: " + tmux.ErrNoSuchSession.Error())
		wrapped := errors.Join(tmux.ErrNoSuchSession, killed)
		reader := &fakePaneReader{err: wrapped}
		runner := &fakeRunner{gitRoot: "/should/not/be/used"}

		dir, ok, err := session.ResolveSessionDir("gone", reader, runner)

		if err != nil {
			t.Fatalf("a killed session must not abort the render, got error: %v", err)
		}
		if ok {
			t.Error("expected ok=false for a killed session")
		}
		if dir != "" {
			t.Errorf("expected empty dir, got %q", dir)
		}
		if runner.called {
			t.Error("git-root resolution must not run when the pane read failed")
		}
	})

	t.Run("returns no-directory when the active pane has no readable current_path", func(t *testing.T) {
		// Trimmed-empty path with nil error: a dead pane / blank value. Must
		// NOT be passed to ResolveGitRoot (which would os.Stat("") and error).
		reader := &fakePaneReader{path: ""}
		runner := &fakeRunner{gitRoot: "/should/not/be/used"}

		dir, ok, err := session.ResolveSessionDir("blank", reader, runner)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected ok=false for an empty current_path")
		}
		if dir != "" {
			t.Errorf("expected empty dir, got %q", dir)
		}
		if runner.called {
			t.Error("git-root resolution must not run for an empty current_path")
		}
	})

	t.Run("canonicalises the derived directory to match stored Project.Path keying", func(t *testing.T) {
		gitRoot := t.TempDir()
		// A pane sitting deep inside the repo; git rev-parse reports the root.
		paneCwd := filepath.Join(gitRoot, "internal", "pkg")
		if err := os.MkdirAll(paneCwd, 0o755); err != nil {
			t.Fatalf("mkdir pane cwd: %v", err)
		}
		reader := &fakePaneReader{path: paneCwd}
		runner := &fakeRunner{gitRoot: gitRoot}

		dir, ok, err := session.ResolveSessionDir("my-session", reader, runner)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected ok=true")
		}

		// The resolver's output must equal the same canonical key the project
		// store would derive for the stored Project.Path, so a By-Project
		// lookup matches. Simulate a stored project rooted at gitRoot.
		stored := []project.Project{{Path: gitRoot, Name: "proj"}}
		_, matched := project.MatchProjectByDir(stored, dir)
		if !matched {
			t.Errorf("derived dir %q did not match stored Project.Path %q via canonical key", dir, gitRoot)
		}
	})

	t.Run("a non-repo pane still yields its real cwd directory", func(t *testing.T) {
		// ResolveGitRoot returns the input dir unchanged when not a repo; the
		// adopted contract: a real current_path always yields ok=true.
		cwd := t.TempDir()
		reader := &fakePaneReader{path: cwd}
		runner := &fakeRunner{err: errors.New("not a git repository")}

		dir, ok, err := session.ResolveSessionDir("my-session", reader, runner)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("a real cwd must resolve to ok=true even outside a git repo")
		}
		if dir != project.CanonicalDirKey(cwd) {
			t.Errorf("dir = %q, want canonical cwd %q", dir, project.CanonicalDirKey(cwd))
		}
	})
}
