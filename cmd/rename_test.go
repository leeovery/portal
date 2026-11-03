package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/sessionindex"
)

// assertLabel holds name's index entry to want, "" for no entry.
func assertLabel(t *testing.T, dir, name, want string) {
	t.Helper()
	idx := readIndex(t, dir)
	e, _ := idx.Lookup(name)
	if e.Label != want {
		t.Errorf("label of %s = %q, want %q: %+v", name, e.Label, want, idx.Sessions)
	}
}

func TestRenameCommand(t *testing.T) {
	t.Run("it sets the current session's label from its arguments joined with spaces", func(t *testing.T) {
		dir := isolateStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)

		if _, _, err := runRootCmd(t, "rename", "dotfiles", "·", "user", "search"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "dotfiles · user search")
	})

	t.Run("it takes one quoted argument as the whole label", func(t *testing.T) {
		dir := isolateStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)

		if _, _, err := runRootCmd(t, "rename", "api  work"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, currentSessionName, "api  work")
	})

	t.Run("it leaves other sessions' labels alone", func(t *testing.T) {
		dir := isolateStateDir(t)
		setLabel(t, dir, "web-Qm3pLz", "web · checkout")
		t.Setenv("ZMX_SESSION", currentSessionName)

		if _, _, err := runRootCmd(t, "rename", "dotfiles"); err != nil {
			t.Fatalf("rename: %v", err)
		}

		assertLabel(t, dir, "web-Qm3pLz", "web · checkout")
		assertLabel(t, dir, currentSessionName, "dotfiles")
	})

	t.Run("it resets the label to the name given no text", func(t *testing.T) {
		for _, args := range [][]string{{"rename"}, {"rename", ""}, {"rename", "  "}, {"rename", currentSessionName}} {
			dir := isolateStateDir(t)
			setLabel(t, dir, currentSessionName, "dotfiles · user search")
			t.Setenv("ZMX_SESSION", currentSessionName)

			if _, _, err := runRootCmd(t, args...); err != nil {
				t.Fatalf("%q: %v", args, err)
			}

			assertLabel(t, dir, currentSessionName, "")
			stdout, _, err := runRootCmd(t, "current")
			if err != nil || stdout.String() != currentSessionName+"\n" {
				t.Errorf("after %q, current = %q, %v; want the name", args, stdout, err)
			}
		}
	})

	t.Run("it refuses an unprintable label and changes nothing", func(t *testing.T) {
		dir := isolateStateDir(t)
		setLabel(t, dir, currentSessionName, "dotfiles")
		t.Setenv("ZMX_SESSION", currentSessionName)

		_, _, err := runRootCmd(t, "rename", "api\nwork")

		want := "label refused: it holds a control or formatting character, such as a newline, tab or escape"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertLabel(t, dir, currentSessionName, "dotfiles")
	})

	t.Run("it fails in one line outside a session and writes nothing", func(t *testing.T) {
		dir := isolateStateDir(t)

		_, _, err := runRootCmd(t, "rename", "api work")

		if !errors.Is(err, errNotInSession) {
			t.Errorf("error = %v, want %v", err, errNotInSession)
		}
		if entries := dirListing(t, dir); len(entries) != 0 {
			t.Errorf("state directory = %v, want nothing written", entries)
		}
	})

	t.Run("it stops on a version 1 index and leaves it as it was", func(t *testing.T) {
		dir := seedSessionsJSON(t, legacyIndexBody)
		t.Setenv("ZMX_SESSION", currentSessionName)

		_, _, err := runRootCmd(t, "rename", "api work")

		if !errors.Is(err, sessionindex.ErrLegacyIndex) {
			t.Errorf("error = %v, want the version 1 stop", err)
		}
		if got := string(readFileBytes(t, sessionindex.Path(dir))); got != legacyIndexBody {
			t.Errorf("sessions.json = %q, want it untouched", got)
		}
	})

	t.Run("it is exempt from the zmx binary lookup", func(t *testing.T) {
		if !bootstrapExempt["rename"] {
			t.Error(`bootstrapExempt["rename"] = false; want true`)
		}
	})
}
