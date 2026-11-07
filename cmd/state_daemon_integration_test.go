//go:build integration

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/portal/internal/harnesstest"
	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/sessionindex"
	"github.com/leeovery/portal/internal/state"
)

// daemonSettle bounds the waits on a daemon's polls, a poll every pollEvery,
// and on its exit.
const daemonSettle = 20 * time.Second

func TestDaemonClaimsSessionsAgainstZmx(t *testing.T) {
	d := portaltest.StartSaverDaemon(t, portalbintest.Build(t))

	made, err := d.Backend.Create(t.Context(), "work", t.TempDir(), []string{"/bin/sh", "-c", "exec sleep 600"})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}

	entry, ok := awaitEntry(t, d.StateDir, "work", func(e sessionindex.Entry) bool {
		return e.SameCreation(made.Created, made.PID)
	})
	if !ok {
		t.Fatalf("the daemon never claimed work (%+v) into %s, holding %+v\n--- portal.log ---\n%s",
			made, sessionindex.Path(d.StateDir), entry, portaltest.ReadPortalLogSafe(d.StateDir))
	}
	if entry.PID != made.PID || entry.PID <= 0 {
		t.Errorf("entry = %+v, want the session's pid %d", entry, made.PID)
	}
}

func TestDaemonSelfEjectsOutsideTheSaverAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)
	inside := portaltest.StartSaverDaemon(t, binary)
	// A session beside the saver, which the daemon outside it claims only if
	// it reads the saver's own zmx directory.
	made, err := inside.Backend.Create(t.Context(), "work", t.TempDir(), []string{"/bin/sh", "-c", "exec sleep 600"})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}

	stateDir := t.TempDir()
	portaltest.OwnStateDir(t, stateDir)
	outside := exec.Command(binary, "state", "daemon")
	outside.Env = append(slices.Clone(inside.Env), "PORTAL_STATE_DIR="+stateDir)
	outside.Env = append(outside.Env, inside.Backend.ReachEnv()...)
	started := time.Now()
	if err := outside.Start(); err != nil {
		t.Fatalf("start a daemon outside %s: %v", saver.SessionName, err)
	}
	reaped := portaltest.RegisterSubprocessCleanup(t, outside)

	select {
	case <-reaped:
	case <-time.After(daemonSettle):
		t.Fatalf("the daemon outside the saver still ran %s on\n--- portal.log ---\n%s",
			daemonSettle, portaltest.ReadPortalLogSafe(stateDir))
	}
	took := time.Since(started)
	logged := portaltest.ReadPortalLogSafe(stateDir)

	t.Run("it reads the saver's zmx directory, claiming its sessions", func(t *testing.T) {
		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			t.Fatalf("read index: %v", err)
		}
		if e, ok := idx.Lookup("work"); !ok || !e.SameCreation(made.Created, made.PID) {
			t.Errorf("work's entry = %+v, %v; want work (%+v) claimed", e, ok, made)
		}
	})

	t.Run("it ejects once the saver session has read as another process's for the hysteresis", func(t *testing.T) {
		least := (selfSupervisionHysteresisTicks - 1) * daemonTick
		most := (selfSupervisionHysteresisTicks + 3) * daemonTick
		if took < least || took > most {
			t.Errorf("the daemon ejected %s after its start, want within [%s, %s]", took, least, most)
		}
		want := fmt.Sprintf("daemon: self-eject ticks=%[1]d threshold=%[1]d", selfSupervisionHysteresisTicks)
		if !strings.Contains(logged, want) {
			t.Errorf("portal.log:\n%s\nwant %q", logged, want)
		}
	})

	t.Run("it exits 0 without its shutdown flush, leaving its pid file", func(t *testing.T) {
		if !strings.Contains(logged, "process: exit code=0") {
			t.Errorf("portal.log:\n%s\nwant the process's exit at code 0", logged)
		}
		if strings.Contains(logged, "daemon: shutdown") {
			t.Errorf("portal.log:\n%s\nwant no shutdown: an ejecting daemon flushes nothing", logged)
		}
		if pid, err := state.ReadPIDFile(stateDir); err != nil || pid != outside.Process.Pid {
			t.Errorf("daemon.pid = %d, %v; want the ejected daemon's %d left stale", pid, err, outside.Process.Pid)
		}
	})

	t.Run("it leaves the daemon in the saver running", func(t *testing.T) {
		if err := inside.Signal(0); err != nil {
			t.Errorf("the saver's daemon %d: %v", inside.PID, err)
		}
	})
}

func TestDaemonSIGTERMFlushesScrollbackAgainstZmx(t *testing.T) {
	d := portaltest.StartSaverDaemon(t, portalbintest.Build(t))

	// The session prints its second screen once the test makes go.
	goAhead := filepath.Join(t.TempDir(), "go")
	script := `echo first-screen; while [ ! -e "$1" ]; do sleep 0.1; done; echo second-screen; exec sleep 600`
	made, err := d.Backend.Create(t.Context(), "work", t.TempDir(), []string{"/bin/sh", "-c", script, "sh", goAhead})
	if err != nil {
		t.Fatalf("create work: %v", err)
	}

	// Whatever screen the first poll caught, the shell's echo or none yet.
	entry, ok := awaitEntry(t, d.StateDir, "work", func(e sessionindex.Entry) bool {
		return e.SameCreation(made.Created, made.PID) && e.ScrollbackFile != ""
	})
	if !ok {
		t.Fatalf("the daemon never claimed work and named its scrollback, holding %+v\n--- portal.log ---\n%s",
			entry, portaltest.ReadPortalLogSafe(d.StateDir))
	}

	if err := os.WriteFile(goAhead, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if !harnesstest.PollUntil(t, daemonSettle, 50*time.Millisecond, func() bool {
		vt, err := d.Backend.History(t.Context(), "work", 0)
		return err == nil && strings.Contains(string(vt), "second-screen")
	}) {
		t.Fatal("work never printed its second screen")
	}
	// The saver asks a creation for its history once a minute at most, so no
	// poll has saved the second screen: the flush alone can.
	if got := scrollbackOf(d.StateDir, entry); strings.Contains(got, "second-screen") {
		t.Fatalf("the saved scrollback already holds the second screen:\n%q", got)
	}

	if err := d.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM the daemon %d: %v", d.PID, err)
	}
	if !d.Exited(t, daemonSettle) {
		t.Fatalf("the daemon %d still ran %s after its SIGTERM", d.PID, daemonSettle)
	}

	idx, err := sessionindex.Read(d.StateDir)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	flushed, ok := idx.Lookup("work")
	if !ok || flushed.ScrollbackFile != entry.ScrollbackFile {
		t.Fatalf("work's entry = %+v, %v; want it still naming %s", flushed, ok, entry.ScrollbackFile)
	}
	if got := scrollbackOf(d.StateDir, flushed); !strings.Contains(got, "second-screen") {
		t.Errorf("the scrollback the index names at exit:\n%q\nwant the second screen, flushed", got)
	}
	logged := portaltest.ReadPortalLogSafe(d.StateDir)
	for _, want := range []string{"capture: flush complete captured=1", "daemon: shutdown reason=signal flush_completed=true"} {
		if !strings.Contains(logged, want) {
			t.Errorf("portal.log:\n%s\nwant %q", logged, want)
		}
	}
}

// awaitEntry waits for the index in stateDir to hold an entry for name that
// done accepts, returning the last entry it read.
func awaitEntry(t *testing.T, stateDir, name string, done func(sessionindex.Entry) bool) (sessionindex.Entry, bool) {
	t.Helper()
	var entry sessionindex.Entry
	ok := harnesstest.PollUntil(t, daemonSettle, 100*time.Millisecond, func() bool {
		idx, err := sessionindex.Read(stateDir)
		if err != nil {
			return false
		}
		var found bool
		entry, found = idx.Lookup(name)
		return found && done(entry)
	})
	return entry, ok
}

// scrollbackOf is the scrollback file e names, empty for none.
func scrollbackOf(stateDir string, e sessionindex.Entry) string {
	path, err := state.ScrollbackPath(stateDir, e)
	if err != nil {
		return ""
	}
	data, _ := os.ReadFile(path)
	return string(data)
}
