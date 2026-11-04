package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// resumeStateDir points PORTAL_STATE_DIR at a state directory of the test's
// own and returns it.
func resumeStateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PORTAL_STATE_DIR", dir)
	return dir
}

func raiseFlag(t *testing.T, dir, session string) string {
	t.Helper()
	if err := state.ResumePending.Set(dir, session); err != nil {
		t.Fatalf("raise %s's flag: %v", session, err)
	}
	path, err := state.ResumePending.Path(dir, session)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func flagRaised(t *testing.T, dir, session string) bool {
	t.Helper()
	raised, err := state.ResumePending.Exists(dir, session)
	if err != nil {
		t.Fatalf("look for %s's flag: %v", session, err)
	}
	return raised
}

// unsearchable takes the search bit off dir until the test ends, so nothing
// in it can be looked up. The bit comes back first at cleanup, so the temp
// dir's removal can reach what is inside.
func unsearchable(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

func TestResumePendingFlag(t *testing.T) {
	const session = "proj-a1b2"

	t.Run("it raises the session's flag in the state directory and returns its path", func(t *testing.T) {
		dir := resumeStateDir(t)

		path, err := raiseResumePending(session)
		if err != nil {
			t.Fatalf("raiseResumePending: %v", err)
		}

		want, _ := state.ResumePending.Path(dir, session)
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		if !flagRaised(t, dir, session) {
			t.Error("the flag is not raised")
		}
	})

	t.Run("it refuses a name no file can carry, raising nothing", func(t *testing.T) {
		dir := resumeStateDir(t)

		if _, err := raiseResumePending("a/b"); !errors.Is(err, state.ErrUnsafeName) {
			t.Errorf("raiseResumePending(a/b) = %v, want ErrUnsafeName", err)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, string(state.ResumePending))); len(entries) != 0 {
			t.Errorf("flags = %v, want none", entries)
		}
	})

	t.Run("it lowers the flag, and lowering it again succeeds", func(t *testing.T) {
		dir := resumeStateDir(t)
		raiseFlag(t, dir, session)

		for range 2 {
			if err := clearResumePending(session); err != nil {
				t.Fatalf("clearResumePending: %v", err)
			}
		}
		if flagRaised(t, dir, session) {
			t.Error("the flag is still raised")
		}
	})

	t.Run("it reads the flag as it stands", func(t *testing.T) {
		dir := resumeStateDir(t)

		if raised, err := resumePendingRaised(session); err != nil || raised {
			t.Errorf("before raising = %v, %v; want lowered", raised, err)
		}
		raiseFlag(t, dir, session)
		if raised, err := resumePendingRaised(session); err != nil || !raised {
			t.Errorf("after raising = %v, %v; want raised", raised, err)
		}
	})

	t.Run("a flag it can't look for is an error, never a lowered flag", func(t *testing.T) {
		dir := resumeStateDir(t)
		raiseFlag(t, dir, session)
		unsearchable(t, filepath.Join(dir, string(state.ResumePending)))

		if _, err := resumePendingRaised(session); err == nil {
			t.Error("resumePendingRaised over an unsearchable directory = nil error, want one")
		}
		if err := clearResumePending(session); err == nil {
			t.Error("clearResumePending over an unsearchable directory = nil error, want one")
		}
	})
}
