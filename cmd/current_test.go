package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/sessionindex"
)

// The current and rename suites run on the production backend, whose current
// session is ZMX_SESSION, so they set it rather than stage a fake.
const currentSessionName = "dotfiles-PpaTER"

func TestCurrentCommand(t *testing.T) {
	t.Run("it prints the session's name while it has no label", func(t *testing.T) {
		isolateStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)

		stdout, stderr, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		if stderr.Len() != 0 {
			t.Errorf("stderr = %q, want empty", stderr)
		}
	})

	t.Run("it prints the session's label from the index", func(t *testing.T) {
		dir := isolateStateDir(t)
		setLabel(t, dir, currentSessionName, "dotfiles · user search")
		setLabel(t, dir, "web-Qm3pLz", "web · checkout")
		t.Setenv("ZMX_SESSION", currentSessionName)

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), "dotfiles · user search\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it prints the session's name when the index cannot be read", func(t *testing.T) {
		blockStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it prints nothing and fails in one line outside a session", func(t *testing.T) {
		isolateStateDir(t)

		stdout, _, err := runRootCmd(t, "current")

		if !errors.Is(err, errNotInSession) || err.Error() != "not inside a Portal session" {
			t.Errorf("error = %v, want %q", err, "not inside a Portal session")
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want empty", stdout)
		}
	})

	t.Run("it takes no arguments", func(t *testing.T) {
		isolateStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)

		if stdout, _, err := runRootCmd(t, "current", "extra"); err == nil || stdout.Len() != 0 {
			t.Errorf("current extra = %q, %v; want a refusal and no output", stdout, err)
		}
	})

	t.Run("it stops on a version 1 index", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)
		t.Setenv("ZMX_SESSION", currentSessionName)

		stdout, _, err := runRootCmd(t, "current")

		if !errors.Is(err, sessionindex.ErrLegacyIndex) || stdout.Len() != 0 {
			t.Errorf("current = %q, %v; want the version 1 stop and no output", stdout, err)
		}
	})

	t.Run("it is exempt from the zmx binary lookup", func(t *testing.T) {
		if !bootstrapExempt["current"] {
			t.Error(`bootstrapExempt["current"] = false; want true`)
		}
	})
}
