//go:build integration

package saverhost_test

import (
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"testing"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// realEnsurer brings up binary in dir's saver session at version, stopping
// through realStopper.
func realEnsurer(t *testing.T, dir portaltest.SaverDir, binary, version string) *saverhost.Ensurer {
	t.Helper()
	e, err := saverhost.NewEnsurer(saverhost.EnsurerConfig{
		Backend: dir.Backend,
		Stopper: realStopper(t, dir),
		Daemon:  saverhost.Daemon{Exe: binary, Version: version, StateDir: dir.StateDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// realStopper stops dir's saver, proving its daemon by the marks a zmx
// session's process carries. zmxtest proves each pid a process of the test's
// own zmx directory again as it signals it.
func realStopper(t *testing.T, dir portaltest.SaverDir) *saverhost.Stopper {
	t.Helper()
	marks, err := zmx.SessionMarks(saver.SessionName, dir.Backend.ReachEnv())
	if err != nil {
		t.Fatal(err)
	}
	s, err := saverhost.NewStopper(saverhost.Config{
		Backend: dir.Backend,
		Marks:   marks,
		Signal:  func(pid int, sig syscall.Signal) error { return zmxtest.Signal(dir.Backend, pid, sig) },
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// binaryVersion is the version binary records, as it reports it run in
// dir's isolation.
func binaryVersion(t *testing.T, dir portaltest.SaverDir, binary string) string {
	t.Helper()
	cmd := exec.Command(binary, "version")
	cmd.Env = dir.Env
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s version: %v", binary, err)
	}
	version, ok := strings.CutPrefix(strings.TrimSpace(string(out)), "portal version ")
	if !ok {
		t.Fatalf("%s version = %q, want portal version <version>", binary, out)
	}
	return version
}

// saverRunning is the saver session's process, holding the daemon's lock.
func saverRunning(t *testing.T, dir portaltest.SaverDir) int {
	t.Helper()
	s, err := dir.Backend.Session(t.Context(), saver.SessionName)
	if err != nil {
		t.Fatalf("read the saver session: %v", err)
	}
	if holder, err := state.ReadPIDFile(dir.StateDir); err != nil || holder != s.PID {
		t.Fatalf("daemon.pid = %d, %v; want the saver session's process %d holding the lock", holder, err, s.PID)
	}
	return s.PID
}

func ensure(t *testing.T, e *saverhost.Ensurer, dir portaltest.SaverDir) int {
	t.Helper()
	if err := e.EnsureSaver(t.Context()); err != nil {
		t.Fatalf("EnsureSaver: %v\n--- portal.log ---\n%s", err, portaltest.ReadPortalLogSafe(dir.StateDir))
	}
	return saverRunning(t, dir)
}

func TestEnsureSaverAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)

	t.Run("from cold it runs the daemon at the absolute exe, which the identity check, the sandboxed pgrep and the proof all find", func(t *testing.T) {
		dir := portaltest.NewSaverDir(t)
		version := binaryVersion(t, dir, binary)

		pid := ensure(t, realEnsurer(t, dir, binary, version), dir)

		if args, err := proctable.ReadArgs(pid); err != nil || !slices.Equal(args.Argv, []string{binary, "state", "daemon"}) {
			t.Errorf("the daemon's argv = %q, %v; want %s state daemon", args.Argv, err, binary)
		}
		if got, err := state.IdentifyDaemon(pid); err != nil || got != state.IdentifyIsPortalDaemon {
			t.Errorf("IdentifyDaemon(%d) = %v, %v; want the daemon identified", pid, got, err)
		}
		if pids, err := portaltest.PgrepPortalDaemons(); err != nil || !slices.Equal(pids, []int{pid}) {
			t.Errorf("PgrepPortalDaemons = %v, %v; want the daemon %d alone", pids, err, pid)
		}
		if err := realStopper(t, dir).Prove(pid); err != nil {
			t.Errorf("Prove(%d) = %v, want the saver's daemon proved", pid, err)
		}
		if got, err := state.ReadVersionFile(dir.StateDir); err != nil || got != version {
			t.Errorf("daemon.version = %q, %v at EnsureSaver's return; want %s", got, err, version)
		}
	})

	// The binary a test builds is a dev build, whose saver every EnsureSaver
	// replaces; a replacement for another version takes the same stop.
	t.Run("a dev build's saver is replaced, the old daemon stopped through its SIGTERM, its flush done before its session goes", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := portaltest.NewSaverDir(t)
		e := realEnsurer(t, dir, binary, binaryVersion(t, dir, binary))
		first := ensure(t, e, dir)

		second := ensure(t, e, dir)

		if second == first {
			t.Fatalf("the saver still runs %d, want it replaced", first)
		}
		recycled := sink.Records().Matching("saver", "recycle").Only(t, "the recycle")
		if reason := recycled.AttrString(t, "reason"); reason != "dev" {
			t.Errorf("recycle reason = %q, want dev", reason)
		}
		if err := zmxtest.Signal(dir.Backend, first, 0); err == nil {
			t.Errorf("the old daemon %d still runs in the test's zmx directory", first)
		}
		logged := portaltest.ReadPortalLogSafe(dir.StateDir)
		if !shutDown(logged, first, "daemon: shutdown reason=signal flush_completed=true") {
			t.Errorf("portal.log:\n%s\nwant daemon %d's shutdown through its SIGTERM with its flush completed", logged, first)
		}
	})

	t.Run("EnsureRunning keeps a live saver of any version, and brings one up where there is none", func(t *testing.T) {
		dir := portaltest.NewSaverDir(t)
		e := realEnsurer(t, dir, binary, "9.9.9")

		if err := e.EnsureRunning(t.Context()); err != nil {
			t.Fatalf("EnsureRunning from cold: %v\n--- portal.log ---\n%s", err, portaltest.ReadPortalLogSafe(dir.StateDir))
		}
		first := saverRunning(t, dir)
		if err := e.EnsureRunning(t.Context()); err != nil {
			t.Fatalf("EnsureRunning with a saver: %v", err)
		}

		if again := saverRunning(t, dir); again != first {
			t.Errorf("the saver runs %d, want %d kept", again, first)
		}
	})
}

// shutDown reports whether logged holds want as pid's line.
func shutDown(logged string, pid int, want string) bool {
	return slices.ContainsFunc(strings.Split(logged, "\n"), func(line string) bool {
		return strings.Contains(line, want) && strings.Contains(line, fmt.Sprintf(" pid=%d ", pid))
	})
}
