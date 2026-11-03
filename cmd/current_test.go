package cmd

import (
	"errors"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backendtest"
)

// Where the current suite runs on the production backend, whose current session
// is ZMX_SESSION, it sets that rather than stage a fake; ZMX_DIR is poisoned
// there, so that backend reads no session.
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
		current := backend.Session{Name: currentSessionName, Created: joinCreated}
		web := backend.Session{Name: "web-Qm3pLz", Created: joinCreated}
		fake := backendtest.New(current, web)
		fake.SetCurrent(currentSessionName)
		withBackendDeps(t, BackendDeps{Backend: fake})
		labelSession(t, dir, current, "dotfiles · user search")
		labelSession(t, dir, web, "web · checkout")

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
		fake := backendtest.New(backend.Session{Name: currentSessionName, Created: joinCreated})
		fake.SetCurrent(currentSessionName)
		withBackendDeps(t, BackendDeps{Backend: fake})

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it prints the session's name when the session cannot be read", func(t *testing.T) {
		dir := isolateStateDir(t)
		t.Setenv("ZMX_SESSION", currentSessionName)
		labelSession(t, dir, backend.Session{Name: currentSessionName, Created: joinCreated}, "dotfiles · user search")

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it prints the name of a session that is not answering", func(t *testing.T) {
		dir := isolateStateDir(t)
		current := backend.Session{Name: currentSessionName, Created: joinCreated}
		fake := backendtest.New(current)
		fake.StopAnswering(currentSessionName)
		fake.SetCurrent(currentSessionName)
		withBackendDeps(t, BackendDeps{Backend: fake})
		labelSession(t, dir, current, "dotfiles · user search")

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it prints nothing and fails silently outside a session", func(t *testing.T) {
		isolateStateDir(t)

		assertSilentOutside(t)
	})

	t.Run("it stays silent outside a session on a version 1 sessions.json", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)

		assertSilentOutside(t)
	})

	t.Run("it prints the name inside a session on a version 1 sessions.json", func(t *testing.T) {
		seedSessionsJSON(t, legacyIndexBody)
		fake := backendtest.New(backend.Session{Name: currentSessionName, Created: joinCreated})
		fake.SetCurrent(currentSessionName)
		withBackendDeps(t, BackendDeps{Backend: fake})

		stdout, _, err := runRootCmd(t, "current")

		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if got, want := stdout.String(), currentSessionName+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("it stays silent outside a session with a zmx dir it cannot bind", func(t *testing.T) {
		isolateStateDir(t)
		t.Setenv("ZMX_DIR", "rel/zmx")

		assertSilentOutside(t)
	})

	t.Run("it reports a zmx dir it cannot bind inside a session", func(t *testing.T) {
		isolateStateDir(t)
		t.Setenv("ZMX_DIR", "rel/zmx")
		t.Setenv("ZMX_SESSION", currentSessionName)

		stdout, _, err := runRootCmd(t, "current")

		if err == nil || IsSilentExitError(err) {
			t.Errorf("error = %v, want the bind's failure, printed", err)
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

	t.Run("it is exempt from the zmx binary lookup", func(t *testing.T) {
		if !bootstrapExempt["current"] {
			t.Error(`bootstrapExempt["current"] = false; want true`)
		}
	})
}

// assertSilentOutside runs current outside a session and holds it to its
// answer there: nothing printed, and an error main exits 1 on without a word.
func assertSilentOutside(t *testing.T) {
	t.Helper()
	stdout, stderr, err := runRootCmd(t, "current")
	if !errors.Is(err, errNoCurrentSession) || !IsSilentExitError(err) {
		t.Errorf("error = %v, want the silent %v", err, errNoCurrentSession)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("stdout = %q, stderr = %q, want both empty", stdout, stderr)
	}
}
