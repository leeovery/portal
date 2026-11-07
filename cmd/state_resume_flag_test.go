package cmd

import (
	"errors"
	"os"
	"os/exec"
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

// flagWorld is one way a session's flag can stand. stage builds it under a
// state directory of the test's own, which PORTAL_STATE_DIR names, and
// returns the flag's path.
type flagWorld struct {
	name   string
	raised bool
	stage  func(t *testing.T, session string) string
}

func flagWorlds() []flagWorld {
	return []flagWorld{
		{name: "raised", raised: true, stage: func(t *testing.T, session string) string {
			return raiseFlag(t, resumeStateDir(t), session)
		}},
		{name: "lowered", raised: false, stage: func(t *testing.T, session string) string {
			dir := resumeStateDir(t)
			path := raiseFlag(t, dir, session)
			if _, err := state.ResumePending.Clear(dir, session); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{name: "never raised, its directory never made", raised: false, stage: func(t *testing.T, session string) string {
			path, _ := state.ResumePending.Path(resumeStateDir(t), session)
			return path
		}},
		{name: "its directory unsearchable", raised: true, stage: func(t *testing.T, session string) string {
			path := raiseFlag(t, resumeStateDir(t), session)
			unsearchable(t, filepath.Dir(path))
			return path
		}},
		{name: "its directory unsearchable, the flag lowered", raised: true, stage: func(t *testing.T, session string) string {
			dir := resumeStateDir(t)
			path := raiseFlag(t, dir, session)
			if _, err := state.ResumePending.Clear(dir, session); err != nil {
				t.Fatal(err)
			}
			unsearchable(t, filepath.Dir(path))
			return path
		}},
		{name: "the state directory gone", raised: true, stage: func(t *testing.T, session string) string {
			dir := resumeStateDir(t)
			path, _ := state.ResumePending.Path(dir, session)
			if err := os.Remove(dir); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{name: "the state directory unsearchable, the flag's directory never made", raised: true, stage: func(t *testing.T, session string) string {
			dir := resumeStateDir(t)
			path, _ := state.ResumePending.Path(dir, session)
			unsearchable(t, dir)
			return path
		}},
		{name: "a directory standing at the flag's path", raised: true, stage: func(t *testing.T, session string) string {
			path, _ := state.ResumePending.Path(resumeStateDir(t), session)
			if err := os.MkdirAll(path, 0o700); err != nil {
				t.Fatal(err)
			}
			return path
		}},
	}
}

func TestResumePendingFlag(t *testing.T) {
	const session = "proj-a1b2"

	t.Run("it raises the session's flag in the state directory and returns its path", func(t *testing.T) {
		dir := resumeStateDir(t)

		path, lowerErr, err := raiseResumePending(session, state.ResumePending.Set)
		if err != nil || lowerErr != nil {
			t.Fatalf("raiseResumePending = %v, %v", lowerErr, err)
		}

		want, _ := state.ResumePending.Path(dir, session)
		if path != want {
			t.Errorf("path = %q, want %q", path, want)
		}
		if !flagRaised(t, dir, session) {
			t.Error("the flag is not raised")
		}
	})

	t.Run("it refuses a name no file can carry, raising nothing and owing no lowering", func(t *testing.T) {
		dir := resumeStateDir(t)
		set := func(string, string) error { t.Error("set reached for a name no file can carry"); return nil }

		_, lowerErr, err := raiseResumePending("a/b", set)

		if !errors.Is(err, state.ErrUnsafeName) || lowerErr != nil {
			t.Errorf("raiseResumePending(a/b) = %v, %v; want ErrUnsafeName and no lowering", lowerErr, err)
		}
		if entries, _ := os.ReadDir(filepath.Join(dir, string(state.ResumePending))); len(entries) != 0 {
			t.Errorf("flags = %v, want none", entries)
		}
	})

	t.Run("a set that made the file and then failed is lowered again", func(t *testing.T) {
		dir := resumeStateDir(t)
		setErr := errors.New("close: input/output error")
		set := func(dir, name string) error {
			if err := state.ResumePending.Set(dir, name); err != nil {
				t.Fatal(err)
			}
			return setErr
		}

		_, lowerErr, err := raiseResumePending(session, set)

		if !errors.Is(err, setErr) || lowerErr != nil {
			t.Errorf("raiseResumePending = %v, %v; want the set's error and a lowering that landed", lowerErr, err)
		}
		if flagRaised(t, dir, session) {
			t.Error("the flag a failed set made is still raised")
		}
	})

	t.Run("a lowering after a failed set that fails too is reported", func(t *testing.T) {
		resumeStateDir(t)
		set := func(dir, name string) error {
			if err := state.ResumePending.Set(dir, name); err != nil {
				t.Fatal(err)
			}
			unsearchable(t, filepath.Join(dir, string(state.ResumePending)))
			return errors.New("close: input/output error")
		}

		_, lowerErr, err := raiseResumePending(session, set)

		if err == nil || lowerErr == nil {
			t.Errorf("raiseResumePending = %v, %v; want the set's error and the lowering's", lowerErr, err)
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

	for _, w := range flagWorlds() {
		t.Run("it reads a flag "+w.name+" as the shell's gate does", func(t *testing.T) {
			w.stage(t, session)

			raised, err := resumePendingRaised(session)

			if err != nil || raised != w.raised {
				t.Errorf("resumePendingRaised = %v, %v; want %v", raised, err, w.raised)
			}
		})
	}

	t.Run("a state directory that can't be resolved is an error", func(t *testing.T) {
		t.Setenv("PORTAL_STATE_DIR", "")
		t.Setenv("XDG_CONFIG_HOME", "")
		t.Setenv("HOME", "")

		if _, err := resumePendingRaised(session); err == nil {
			t.Error("resumePendingRaised = nil error, want one: the chain counts it as raised")
		}
	})
}

// TestResumeFlagGates_Agree runs the parked chain's shell gates beside the Go
// read the chain's tail takes, over every world, so the two never disagree on
// whether a session still waits.
func TestResumeFlagGates_Agree(t *testing.T) {
	const session = "proj-a1b2"
	for _, w := range flagWorlds() {
		t.Run("a flag "+w.name, func(t *testing.T) {
			path := w.stage(t, session)

			goRaised, err := resumePendingRaised(session)
			if err != nil {
				t.Fatal(err)
			}
			if goRaised != w.raised {
				t.Errorf("the Go read says raised=%v, want %v", goRaised, w.raised)
			}
			if got := shellSucceeds(t, flagStillRaised(path)); got != w.raised {
				t.Errorf("the draw loop's gate says raised=%v, want %v", got, w.raised)
			}
			if got := shellSucceeds(t, flagProvedLowered(path)); got != !w.raised {
				t.Errorf("the backstop's gate says lowered=%v, want %v", got, !w.raised)
			}
		})
	}
}

// shellSucceeds runs test under /bin/sh and reports whether it exits 0. Any
// status but 0 and a test's 1, a syntax error's 2 among them, fails t.
func shellSucceeds(t *testing.T, test string) bool {
	t.Helper()
	err := exec.Command("/bin/sh", "-c", test).Run()
	if exit, failed := errors.AsType[*exec.ExitError](err); failed && exit.ExitCode() == 1 {
		return false
	}
	if err != nil {
		t.Fatalf("run %q: %v", test, err)
	}
	return true
}
