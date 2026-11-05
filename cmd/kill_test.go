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
	"github.com/leeovery/portal/internal/state"
)

// stageKill stages fake as the backend in a state directory of the test's own
// whose index labels every session fake holds, owing no bootstrap, and returns
// the directory.
func stageKill(t *testing.T, fake *backendtest.Fake, names ...string) string {
	t.Helper()
	withBootstrapDeps(t, BootstrapDeps{})
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

	// A saved session that isn't running, kept for the next restore, is one
	// the user can see and bring back; killing it discards it for good.
	t.Run("it discards a saved session that isn't running, saying so, and succeeds", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "web-Qm3pLz"})
		dir := stageKill(t, fake, "api-x7Kd9a", "web-Qm3pLz")

		stdout, _, err := runRootCmd(t, "kill", "api-x7Kd9a")

		if err != nil {
			t.Fatalf("kill: %v", err)
		}
		if want := fmt.Sprintf(discardedCopy, "api-x7Kd9a") + "\n"; stdout.String() != want {
			t.Errorf("stdout = %q, want %q", stdout, want)
		}
		assertIndexed(t, dir, "api-x7Kd9a", false)
		assertIndexed(t, dir, "web-Qm3pLz", true)
	})

	t.Run("it says nothing more of a live session it kills", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "api-x7Kd9a"})
		stageKill(t, fake, "api-x7Kd9a")

		if stdout, _, err := runRootCmd(t, "kill", "api-x7Kd9a"); err != nil || stdout.Len() != 0 {
			t.Errorf("kill = %q, %v; want nothing printed and no error", stdout, err)
		}
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

	// The saved foo is kept while Foo lives, a folding filesystem holding
	// foo's socket as Foo's.
	t.Run("it removes nothing under a name held under another spelling, its own entry kept", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: "Foo"})
		dir := stageKill(t, fake, "foo", "Foo")
		stageKept(t, dir, "foo")

		stdout, _, err := runRootCmd(t, "kill", "foo")

		if want := "No session found: foo; did you mean Foo?"; err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing discarded", stdout)
		}
		assertIndexed(t, dir, "foo", true)
		assertIndexed(t, dir, "Foo", true)
		if raised, err := state.Hydrating.Exists(dir, "foo"); err != nil || !raised {
			t.Errorf("foo's hydrating flag = %v, %v; want it kept", raised, err)
		}
	})

	t.Run("it reports a missing session whose index entry could not be removed", func(t *testing.T) {
		withBootstrapDeps(t, BootstrapDeps{})
		withBackendDeps(t, BackendDeps{Backend: backendtest.New()})
		blockStateDir(t)
		withFuncSeam(t, &holdBootstrapLock, func(context.Context, string) (func(), error) { return func() {}, nil })

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

		want := "session crashed-Lp4s refuses connections: " + stillHeld
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
		withBootstrapDeps(t, BootstrapDeps{})
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

// bootstrapHold is bootstrap.lock's hold, staged: under runs once it is
// taken, standing in for what x <name> did while the kill waited, and
// keptAtRelease is whether the index held the session's entry as it was let
// go.
type bootstrapHold struct {
	taken         int
	dir           string
	keptAtRelease bool
}

func stageBootstrapHold(t *testing.T, name string, under func()) *bootstrapHold {
	t.Helper()
	h := &bootstrapHold{}
	withFuncSeam(t, &holdBootstrapLock, func(_ context.Context, dir string) (func(), error) {
		h.taken++
		h.dir = dir
		if under != nil {
			under()
		}
		return func() {
			idx := readIndex(t, dir)
			_, h.keptAtRelease = idx.Lookup(name)
		}, nil
	})
	return h
}

// x <name> brings a kept session back under bootstrap.lock, so a kill that
// finds the session not running judges it again under that lock, held through
// the discard.
func TestKillCommand_JudgesASessionNotRunningAgainUnderBootstrapLock(t *testing.T) {
	const name = "api-x7Kd9a"

	t.Run("a session brought back while it waited is killed as a live one", func(t *testing.T) {
		fake := backendtest.New()
		dir := stageKill(t, fake, name)
		hold := stageBootstrapHold(t, name, func() { fake.Add(backend.Session{Name: name}) })

		stdout, _, err := runRootCmd(t, "kill", name)

		if err != nil || stdout.Len() != 0 {
			t.Errorf("kill = %q, %v; want nothing printed and no error", stdout, err)
		}
		if _, held := fake.Held(name); held {
			t.Error("the backend still holds the session brought back")
		}
		if hold.taken != 1 || hold.dir != dir || hold.keptAtRelease {
			t.Errorf("hold = %+v, want the state dir's lock taken once and its entry forgotten under it", hold)
		}
		assertIndexed(t, dir, name, false)
	})

	t.Run("an entry gone while it waited is no session found", func(t *testing.T) {
		fake := backendtest.New()
		dir := stageKill(t, fake, name)
		hold := stageBootstrapHold(t, name, func() {
			if _, err := sessionindex.Update(dir, "cli", func(idx *sessionindex.Index) error {
				idx.Remove(name)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})

		stdout, _, err := runRootCmd(t, "kill", name)

		if want := "No session found: " + name; err == nil || err.Error() != want {
			t.Errorf("error = %v, want %q", err, want)
		}
		if stdout.Len() != 0 || hold.taken != 1 {
			t.Errorf("stdout = %q, hold = %+v; want nothing discarded, judged under the lock", stdout, hold)
		}
	})

	t.Run("a session still kept is discarded under it", func(t *testing.T) {
		fake := backendtest.New()
		dir := stageKill(t, fake, name)
		hold := stageBootstrapHold(t, name, nil)

		stdout, _, err := runRootCmd(t, "kill", name)

		if want := fmt.Sprintf(discardedCopy, name) + "\n"; err != nil || stdout.String() != want {
			t.Errorf("kill = %q, %v; want %q", stdout, err, want)
		}
		if hold.taken != 1 || hold.keptAtRelease {
			t.Errorf("hold = %+v, want its entry discarded under the lock", hold)
		}
		assertIndexed(t, dir, name, false)
	})

	t.Run("a lock it can't take is the failure, and nothing is discarded", func(t *testing.T) {
		fake := backendtest.New()
		dir := stageKill(t, fake, name)
		held := errors.New("bootstrap.lock is held by another process")
		withFuncSeam(t, &holdBootstrapLock, func(context.Context, string) (func(), error) { return nil, held })

		stdout, _, err := runRootCmd(t, "kill", name)

		if !errors.Is(err, held) || stdout.Len() != 0 {
			t.Errorf("kill = %q, %v; want the lock's failure and nothing discarded", stdout, err)
		}
		if calls := fake.CallsTo(backendtest.OpKill); len(calls) != 1 {
			t.Errorf("Kill calls = %+v, want none after the lock failed", calls)
		}
		assertIndexed(t, dir, name, true)
	})

	t.Run("a live session is killed without it", func(t *testing.T) {
		fake := backendtest.New(backend.Session{Name: name})
		stageKill(t, fake, name)
		hold := stageBootstrapHold(t, name, nil)

		if _, _, err := runRootCmd(t, "kill", name); err != nil {
			t.Fatalf("kill: %v", err)
		}
		if hold.taken != 0 {
			t.Errorf("bootstrap.lock taken %d times, want none for a live session", hold.taken)
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
