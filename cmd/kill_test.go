package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/backendtest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/sessionindex"
)

// stageKill stages fake as the backend in a state directory of the test's own
// whose index labels every session fake holds, and returns the directory.
func stageKill(t *testing.T, fake *backendtest.Fake, names ...string) string {
	t.Helper()
	withBackendDeps(t, BackendDeps{Backend: fake})
	dir := isolateStateDir(t)
	for _, name := range names {
		storedLabel(t, dir, name, name+" · work")
	}
	return dir
}

func readIndex(t *testing.T, dir string) sessionindex.Index {
	t.Helper()
	idx, err := sessionindex.Read(dir)
	if err != nil {
		t.Fatalf("read sessions.json: %v", err)
	}
	return idx
}

func assertIndexed(t *testing.T, dir, name string, want bool) {
	t.Helper()
	idx := readIndex(t, dir)
	if _, got := idx.Lookup(name); got != want {
		t.Errorf("index entry for %s present = %v, want %v: %+v", name, got, want, idx.Entries())
	}
}

func TestKillCommand(t *testing.T) {
	t.Run("it kills the session and removes its index entry", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"}, backend.Session{Name: "web-Qm3pLz"})
		dir := stageKill(t, fake, "api-x7Kd9a", "web-Qm3pLz")

		if _, stderr, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill: %v\nstderr: %s", err, stderr)
		}

		if _, held := fake.Held("api-x7Kd9a"); held {
			t.Error("the backend still holds the killed session")
		}
		if calls := fake.CallsTo(backendtest.OpKill); len(calls) != 1 || calls[0].Name != "api-x7Kd9a" {
			t.Errorf("Kill calls = %+v, want one for api-x7Kd9a", calls)
		}
		assertIndexed(t, dir, "api-x7Kd9a", false)
		assertIndexed(t, dir, "web-Qm3pLz", true)
	})

	t.Run("it kills a session with no index entry", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		stageKill(t, fake)

		if _, _, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil {
			t.Fatalf("kill: %v", err)
		}
		if _, held := fake.Held("api-x7Kd9a"); held {
			t.Error("the backend still holds the killed session")
		}
	})

	t.Run("it says a missing session is missing, matching names exactly", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "my-session-abc123"})
		dir := stageKill(t, fake, "my-session-abc123")

		_, _, err := runRootCmd(t, "kill", "my-session")

		if err == nil || err.Error() != "No session found: my-session" {
			t.Errorf("error = %v, want %q", err, "No session found: my-session")
		}
		if _, held := fake.Held("my-session-abc123"); !held {
			t.Error("a kill of a prefix of its name ended my-session-abc123")
		}
		assertIndexed(t, dir, "my-session-abc123", true)
	})

	t.Run("it removes the entry of a session that ended on its own", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "web-Qm3pLz"})
		dir := stageKill(t, fake, "api-x7Kd9a", "web-Qm3pLz")

		_, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

		if err == nil || err.Error() != "No session found: api-x7Kd9a" {
			t.Errorf("error = %v, want %q", err, "No session found: api-x7Kd9a")
		}
		assertIndexed(t, dir, "api-x7Kd9a", false)
		assertIndexed(t, dir, "web-Qm3pLz", true)
	})

	t.Run("it names the spelling a session is held under", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "dotfiles-PpaTER"})
		dir := stageKill(t, fake, "dotfiles-PpaTER")

		_, _, err := runRootCmd(t, "kill", "DOTFILES-ppater")

		want := "No session found: DOTFILES-ppater; did you mean dotfiles-PpaTER?"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if _, held := fake.Held("dotfiles-PpaTER"); !held {
			t.Error("a kill under another spelling ended dotfiles-PpaTER")
		}
		assertIndexed(t, dir, "dotfiles-PpaTER", true)
	})

	t.Run("it reports a missing session whose index entry could not be removed", func(t *testing.T) {
		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
		blockStateDir(t)

		_, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

		if err == nil || !strings.HasPrefix(err.Error(), "No session found: api-x7Kd9a, and its session index entry could not be removed: ") {
			t.Errorf("error = %v, want the missing session reported alongside the index failure", err)
		}
	})

	t.Run("it says a session that does not answer may still be killed, keeping its entry", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "wedged-Lp4s"})
		fake.StopAnswering("wedged-Lp4s")
		dir := stageKill(t, fake, "wedged-Lp4s")

		_, _, err := runRootCmd(t, "kill", "wedged-Lp4s")

		want := "session wedged-Lp4s is not answering; the kill may still land"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertIndexed(t, dir, "wedged-Lp4s", true)
	})

	t.Run("it says a session whose daemon has crashed will not be killed, and how to clear it", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "crashed-Lp4s"})
		fake.Fail(backendtest.OpKill, fmt.Errorf("kill crashed-Lp4s: %w", zmx.ErrRefusing))
		dir := stageKill(t, fake, "crashed-Lp4s")

		_, _, err := runRootCmd(t, "kill", "crashed-Lp4s")

		want := "session crashed-Lp4s refuses connections: " + crashedDaemon
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertIndexed(t, dir, "crashed-Lp4s", true)
	})

	t.Run("it returns any other backend failure as given, keeping the entry", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		injected := errors.New("kill api-x7Kd9a: socket write failed")
		fake.Fail(backendtest.OpKill, injected)
		dir := stageKill(t, fake, "api-x7Kd9a")

		if _, _, err := runRootCmd(t, "kill", "api-x7Kd9a"); !errors.Is(err, injected) || err.Error() != injected.Error() {
			t.Errorf("error = %v, want %v unwrapped", err, injected)
		}
		assertIndexed(t, dir, "api-x7Kd9a", true)
	})

	t.Run("it reports a killed session whose index entry could not be removed", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		withBackendDeps(t, BackendDeps{Backend: fake})
		blockStateDir(t)

		_, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

		if err == nil || !strings.HasPrefix(err.Error(), "killed api-x7Kd9a, but could not remove it from the session index: ") {
			t.Errorf("error = %v, want the kill reported alongside the index failure", err)
		}
		if _, held := fake.Held("api-x7Kd9a"); held {
			t.Error("the backend still holds the killed session")
		}
	})

	t.Run("it asks for exactly one name", func(t *testing.T) {
		fake := backendtest.New()
		stageKill(t, fake)

		for _, args := range [][]string{{"kill"}, {"kill", "a", "b"}} {
			if _, _, err := runRootCmd(t, args...); err == nil {
				t.Errorf("%v ran", args)
			}
		}
		if calls := fake.Calls(); len(calls) != 0 {
			t.Errorf("backend calls = %+v, want none", calls)
		}
	})
}

func TestPickerKiller(t *testing.T) {
	t.Run("it kills as portal kill does, removing the index entry via the tui", func(t *testing.T) {
		sink := logtest.Install(t)
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"}, backend.Session{Name: "web-Qm3pLz"})
		dir := stageKill(t, fake, "api-x7Kd9a", "web-Qm3pLz")

		if err := (pickerKiller{ctx: context.Background(), b: fake}).KillSession("api-x7Kd9a"); err != nil {
			t.Fatalf("KillSession: %v", err)
		}

		if _, held := fake.Held("api-x7Kd9a"); held {
			t.Error("the backend still holds the killed session")
		}
		assertIndexed(t, dir, "api-x7Kd9a", false)
		assertIndexed(t, dir, "web-Qm3pLz", true)
		r := sink.Records().Matching("sessions", "rm").Only(t, "index removal record")
		if got := r.AttrString(t, "via"); got != "tui" {
			t.Errorf("via = %q, want tui", got)
		}
	})

	t.Run("it words a refusal as portal kill does, keeping the entry", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "wedged-Lp4s"})
		fake.StopAnswering("wedged-Lp4s")
		dir := stageKill(t, fake, "wedged-Lp4s")

		err := (pickerKiller{ctx: context.Background(), b: fake}).KillSession("wedged-Lp4s")

		want := "session wedged-Lp4s is not answering; the kill may still land"
		if err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		assertIndexed(t, dir, "wedged-Lp4s", true)
	})
}
