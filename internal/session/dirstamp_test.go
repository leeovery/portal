package session_test

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/session"
	"github.com/leeovery/portal/internal/tmux"
)

// fakeStampClient is a stand-in for the combined seam the lazy stamp helper
// needs: it reads the active pane's current_path AND stamps a session option.
// It records each call so tests can assert the derive-use-then-stamp ordering
// and the best-effort write semantics.
type fakeStampClient struct {
	path string
	err  error

	setErr error

	readSessions []string
	setCalls     []setSessionOptionCall
}

type setSessionOptionCall struct {
	session string
	name    string
	value   string
}

func (f *fakeStampClient) ActivePaneCurrentPath(session string) (string, error) {
	f.readSessions = append(f.readSessions, session)
	return f.path, f.err
}

func (f *fakeStampClient) SetSessionOption(session, name, value string) error {
	f.setCalls = append(f.setCalls, setSessionOptionCall{session: session, name: name, value: value})
	return f.setErr
}

// Compile-time proof that the production *tmux.Client satisfies the combined
// reader+stamper seam the lazy stamp helper consumes.
var _ session.PaneStamper = (*tmux.Client)(nil)

func TestResolveAndStampDir(t *testing.T) {
	t.Run("fast path: a present stamp is returned without reading panes or stamping", func(t *testing.T) {
		client := &fakeStampClient{path: "/should/not/be/read"}
		runner := &fakeRunner{gitRoot: "/should/not/be/used"}

		dir, ok := session.ResolveAndStampDir("sess", "/already/stamped", client, runner)

		if !ok {
			t.Fatal("expected ok=true on the fast path")
		}
		if dir != "/already/stamped" {
			t.Errorf("dir = %q, want %q", dir, "/already/stamped")
		}
		if len(client.readSessions) != 0 {
			t.Errorf("fast path must not read panes, got reads: %v", client.readSessions)
		}
		if runner.called {
			t.Error("fast path must not derive a git root")
		}
		if len(client.setCalls) != 0 {
			t.Errorf("fast path must not re-stamp, got: %v", client.setCalls)
		}
	})

	t.Run("it stamps the derived directory after a successful lazy resolution", func(t *testing.T) {
		gitRoot := t.TempDir()
		client := &fakeStampClient{path: gitRoot}
		runner := &fakeRunner{gitRoot: gitRoot}

		dir, ok := session.ResolveAndStampDir("sess", "", client, runner)

		if !ok {
			t.Fatal("expected ok=true for a resolvable un-stamped session")
		}
		want := project.CanonicalDirKey(gitRoot)
		if dir != want {
			t.Errorf("dir = %q, want canonical %q", dir, want)
		}
		if len(client.setCalls) != 1 {
			t.Fatalf("expected exactly 1 stamp write, got %d: %v", len(client.setCalls), client.setCalls)
		}
		got := client.setCalls[0]
		if got.session != "sess" || got.name != session.PortalDirOption || got.value != want {
			t.Errorf("stamp call = %+v, want session=sess name=%s value=%s", got, session.PortalDirOption, want)
		}
	})

	t.Run("it uses the derived value for the current render even when the stamp write fails", func(t *testing.T) {
		gitRoot := t.TempDir()
		client := &fakeStampClient{path: gitRoot, setErr: errors.New("set-session-option failed")}
		runner := &fakeRunner{gitRoot: gitRoot}

		dir, ok := session.ResolveAndStampDir("sess", "", client, runner)

		if !ok {
			t.Fatal("a stamp-write failure must not drop the session: expected ok=true")
		}
		want := project.CanonicalDirKey(gitRoot)
		if dir != want {
			t.Errorf("dir = %q, want canonical %q (the derived value, used irrespective of write outcome)", dir, want)
		}
		if len(client.setCalls) != 1 {
			t.Errorf("expected exactly 1 stamp attempt, got %d: %v", len(client.setCalls), client.setCalls)
		}
	})

	t.Run("it swallows a SetSessionOption error without dropping the session", func(t *testing.T) {
		gitRoot := t.TempDir()
		client := &fakeStampClient{path: gitRoot, setErr: errors.New("tmux: no such session")}
		runner := &fakeRunner{gitRoot: gitRoot}

		// The helper returns (dir, ok) only — a propagated error would have to
		// surface as ok=false or a panic. Assert it neither panics nor drops.
		dir, ok := session.ResolveAndStampDir("sess", "", client, runner)

		if !ok || dir == "" {
			t.Fatalf("a swallowed write error must still yield the derived dir: got dir=%q ok=%v", dir, ok)
		}
	})

	t.Run("it re-attempts the stamp on the next render after a write failure", func(t *testing.T) {
		gitRoot := t.TempDir()
		// First render: stamp write fails, so Dir stays empty next render.
		client := &fakeStampClient{path: gitRoot, setErr: errors.New("first write fails")}
		runner := &fakeRunner{gitRoot: gitRoot}

		dir1, ok1 := session.ResolveAndStampDir("sess", "", client, runner)
		if !ok1 {
			t.Fatalf("first render expected ok=true, got dir=%q", dir1)
		}

		// Second render: stamp still absent (currentDir==""), write now succeeds.
		client.setErr = nil
		dir2, ok2 := session.ResolveAndStampDir("sess", "", client, runner)
		if !ok2 {
			t.Fatal("second render expected ok=true")
		}

		want := project.CanonicalDirKey(gitRoot)
		if dir1 != want || dir2 != want {
			t.Errorf("both renders must derive the canonical dir: dir1=%q dir2=%q want=%q", dir1, dir2, want)
		}
		// Both renders re-entered the lazy path: two reads, two stamp attempts.
		if len(client.readSessions) != 2 {
			t.Errorf("expected 2 active-pane reads (re-derive each render), got %d: %v", len(client.readSessions), client.readSessions)
		}
		if len(client.setCalls) != 2 {
			t.Errorf("expected 2 stamp attempts (re-stamp each render), got %d: %v", len(client.setCalls), client.setCalls)
		}
	})

	t.Run("it does not stamp when no git-root is derivable", func(t *testing.T) {
		// Empty current_path => ResolveSessionDir returns ok=false: no directory,
		// nothing to stamp, routes to Unknown/Untagged (Phase 2).
		client := &fakeStampClient{path: ""}
		runner := &fakeRunner{gitRoot: "/should/not/be/used"}

		dir, ok := session.ResolveAndStampDir("sess", "", client, runner)

		if ok {
			t.Error("expected ok=false when no git-root is derivable")
		}
		if dir != "" {
			t.Errorf("expected empty dir, got %q", dir)
		}
		if len(client.setCalls) != 0 {
			t.Errorf("must not stamp when nothing was derived, got: %v", client.setCalls)
		}
	})
}
