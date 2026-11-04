//go:build integration

package saverhost_test

import (
	"fmt"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/backend/zmx"
	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/logtest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/saverhost"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/zmxtest"
)

// otherVersion is a release no daemon a test builds records: each records
// its binary's own dev version.
const otherVersion = "9.9.9"

// realEnsurer brings up binary in dir's saver session, proving its daemon by
// the marks a zmx session's process carries. zmxtest proves each pid a
// process of the test's own zmx directory again as it signals it.
func realEnsurer(t *testing.T, dir portaltest.SaverDir, binary string) (*saverhost.Ensurer, *saverhost.Stopper) {
	t.Helper()
	stopper, err := saverhost.NewStopper(saverhost.Config{
		Backend: dir.Backend,
		Marks:   append([]string{zmx.SessionEntry(saver.SessionName)}, dir.Backend.ReachEnv()...),
		Signal:  func(pid int, sig syscall.Signal) error { return zmxtest.Signal(dir.Backend, pid, sig) },
	})
	if err != nil {
		t.Fatal(err)
	}
	e, err := saverhost.NewEnsurer(stopper, saverhost.Daemon{Exe: binary, Version: otherVersion, StateDir: dir.StateDir})
	if err != nil {
		t.Fatal(err)
	}
	return e, stopper
}

func ensure(t *testing.T, e *saverhost.Ensurer, dir portaltest.SaverDir) int {
	t.Helper()
	if err := e.EnsureSaver(t.Context()); err != nil {
		t.Fatalf("EnsureSaver: %v\n--- portal.log ---\n%s", err, portaltest.ReadPortalLogSafe(dir.StateDir))
	}
	s, err := dir.Backend.Session(t.Context(), saver.SessionName)
	if err != nil {
		t.Fatalf("read the saver session: %v", err)
	}
	if holder, err := state.ReadPIDFile(dir.StateDir); err != nil || holder != s.PID {
		t.Fatalf("daemon.pid = %d, %v; want the saver session's process %d holding the lock", holder, err, s.PID)
	}
	return s.PID
}

func TestEnsureSaverAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)

	t.Run("from cold it runs the daemon at the absolute exe, which the identity check, the sandboxed pgrep and the proof all find", func(t *testing.T) {
		dir := portaltest.NewSaverDir(t)
		e, stopper := realEnsurer(t, dir, binary)

		pid := ensure(t, e, dir)

		if args, err := proctable.ReadArgs(pid); err != nil || !slices.Equal(args.Argv, []string{binary, "state", "daemon"}) {
			t.Errorf("the daemon's argv = %q, %v; want %s state daemon", args.Argv, err, binary)
		}
		if got, err := state.IdentifyDaemon(pid); err != nil || got != state.IdentifyIsPortalDaemon {
			t.Errorf("IdentifyDaemon(%d) = %v, %v; want the daemon identified", pid, got, err)
		}
		if pids, err := portaltest.PgrepPortalDaemons(); err != nil || !slices.Equal(pids, []int{pid}) {
			t.Errorf("PgrepPortalDaemons = %v, %v; want the daemon %d alone", pids, err, pid)
		}
		if err := stopper.Prove(pid); err != nil {
			t.Errorf("Prove(%d) = %v, want the saver's daemon proved", pid, err)
		}
	})

	t.Run("a change of version stops the daemon through its SIGTERM, its flush done before its session goes, and starts another", func(t *testing.T) {
		sink := logtest.Install(t)
		dir := portaltest.NewSaverDir(t)
		e, _ := realEnsurer(t, dir, binary)
		first := ensure(t, e, dir)
		// The daemon records its version just after its pid.
		if !harnesstest.PollUntil(t, 5*time.Second, 10*time.Millisecond, func() bool {
			v, err := state.ReadVersionFile(dir.StateDir)
			return err == nil && v != ""
		}) {
			t.Fatalf("the daemon %d never recorded its version", first)
		}

		second := ensure(t, e, dir)

		if second == first {
			t.Fatalf("the saver still runs %d, want it replaced", first)
		}
		recycled := sink.Records().Matching("saver", "recycle").Only(t, "the recycle")
		if reason := recycled.AttrString(t, "reason"); reason != "version" {
			t.Errorf("recycle reason = %q, want version", reason)
		}
		if err := zmxtest.Signal(dir.Backend, first, 0); err == nil {
			t.Errorf("the old daemon %d still runs in the test's zmx directory", first)
		}
		logged := portaltest.ReadPortalLogSafe(dir.StateDir)
		if !shutDown(logged, first, "daemon: shutdown reason=signal flush_completed=true") {
			t.Errorf("portal.log:\n%s\nwant daemon %d's shutdown through its SIGTERM with its flush completed", logged, first)
		}
	})
}

// shutDown reports whether logged holds want as pid's line.
func shutDown(logged string, pid int, want string) bool {
	return slices.ContainsFunc(strings.Split(logged, "\n"), func(line string) bool {
		return strings.Contains(line, want) && strings.Contains(line, fmt.Sprintf(" pid=%d ", pid))
	})
}
