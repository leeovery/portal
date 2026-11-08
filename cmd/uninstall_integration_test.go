//go:build integration

package cmd_test

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/hookstest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/saver"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/xdg"
	"github.com/leeovery/portal/internal/zmxtest"
)

func TestUninstallAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	d := portaltest.StartSaverDaemon(t, binary)
	latch := state.Latch{BootID: "4DAA6ADD-0334-49EC-9FC6-8323D691249E", Version: "0.13.0"}
	if err := state.WriteLatch(d.StateDir, latch); err != nil {
		t.Fatal(err)
	}
	// A pre-token key, which no sweep reaps.
	hookstest.SeedHooksJSON(t, d.Env, map[string]string{"work": "make dev"})
	configDir, err := xdg.PortalDir(xdg.EnvSlice(d.Env))
	if err != nil {
		t.Fatal(err)
	}
	before := configFiles(t, configDir, d.StateDir)

	out, err := runUninstall(binary, d, os.Getenv("PATH"))
	if err != nil {
		t.Fatalf("portal uninstall: %v\n%s\n--- portal.log ---\n%s", err, out, portaltest.ReadPortalLogSafe(d.StateDir))
	}
	logged := portaltest.ReadPortalLogSafe(d.StateDir)

	t.Run("it says the daemon stopped, and where the saved sessions and config are", func(t *testing.T) {
		want := "Portal's save daemon stopped. Saved sessions and config are untouched at " + configDir + "/.\n" +
			"To remove Portal completely, uninstall the binary and delete that directory.\n"
		if string(out) != want {
			t.Errorf("output = %q, want %q", out, want)
		}
	})

	t.Run("the daemon exited through its SIGTERM, its flush done, and its session is gone", func(t *testing.T) {
		if d.Signal(0) == nil {
			t.Errorf("the daemon %d still runs", d.PID)
		}
		if !shutDownThroughSignal(logged, d.PID) {
			t.Errorf("portal.log:\n%s\nwant daemon %d's shutdown through its SIGTERM with its flush completed", logged, d.PID)
		}
		if _, err := d.Backend.Session(t.Context(), saver.SessionName); !errors.Is(err, backend.ErrNoSuchSession) {
			t.Errorf("Session(%s) = %v, want it gone", saver.SessionName, err)
		}
	})

	t.Run("it keeps the latch, and touches no config file", func(t *testing.T) {
		if got, err := state.ReadLatch(d.StateDir); err != nil || got != latch {
			t.Errorf("latch = %+v, %v; want %+v kept", got, err, latch)
		}
		if after := configFiles(t, configDir, d.StateDir); !maps.Equal(before, after) {
			t.Errorf("config files = %v, want them as they were, %v", after, before)
		}
	})

	t.Run("it starts nothing", func(t *testing.T) {
		if pids, err := portaltest.PgrepPortalDaemons(); err != nil || len(pids) != 0 {
			t.Errorf("PgrepPortalDaemons = %v, %v; want no daemon", pids, err)
		}
		if held, err := d.Backend.Exists(t.Context(), saver.SessionName); err != nil || held {
			t.Errorf("Exists(%s) = %v, %v; want no saver", saver.SessionName, held, err)
		}
	})
}

func TestUninstallRefusesAnUnprovedSaverAgainstZmx(t *testing.T) {
	binary := buildPortalBinary(t)
	d := portaltest.StartSaverDaemon(t, binary)

	// With no ps to run, nothing can identify the saver session's process.
	out, err := runUninstall(binary, d, "")

	if exitErr, ok := errors.AsType[*exec.ExitError](err); !ok || exitErr.ExitCode() != 1 {
		t.Fatalf("portal uninstall = %v, want exit 1\n%s", err, out)
	}
	if want := fmt.Sprintf("couldn't stop Portal's save daemon: pid %d isn't confirmed as the save daemon, so nothing was sent to it: ", d.PID); !strings.HasPrefix(string(out), want) {
		t.Errorf("output = %q, want it to begin %q", out, want)
	}
	if err := d.Signal(0); err != nil {
		t.Errorf("the daemon %d: %v, want it left running", d.PID, err)
	}
	if held, err := d.Backend.Exists(t.Context(), saver.SessionName); err != nil || !held {
		t.Errorf("Exists(%s) = %v, %v; want the saver left", saver.SessionName, held, err)
	}
}

// Decision 68: uninstall ends a saver whose daemon speaks zmx 0.8.1, which it
// can't read to stop, as the bootstrap ends one.
func TestUninstallEndsASaverOnOlderZmxAgainstZmx(t *testing.T) {
	older := zmxtest.OlderBinary(t)
	w := newBootWorld(t)
	w.olderSaver(t, older)
	configDir, err := xdg.PortalDir(xdg.EnvSlice(w.env))
	if err != nil {
		t.Fatal(err)
	}

	out, err := w.command("uninstall").CombinedOutput()

	if err != nil {
		t.Fatalf("portal uninstall: %v\n%s\n--- portal.log ---\n%s", err, out, w.log())
	}
	want := "Portal's save daemon stopped. Saved sessions and config are untouched at " + configDir + "/.\n" +
		"To remove Portal completely, uninstall the binary and delete that directory.\n"
	if string(out) != want {
		t.Errorf("output = %q, want %q", out, want)
	}
	w.awaitNoSessions(t)
	if pids, err := portaltest.PgrepPortalDaemons(); err != nil || len(pids) != 0 {
		t.Errorf("PgrepPortalDaemons = %v, %v; want the saver's daemon gone with it", pids, err)
	}
}

// runUninstall runs binary's uninstall over d's zmx directory and state, with
// path as its PATH.
func runUninstall(binary string, d *portaltest.SaverDaemon, path string) ([]byte, error) {
	uninstall := exec.Command(binary, "uninstall")
	uninstall.Env = append(slices.Clone(d.Env), d.Backend.ReachEnv()...)
	uninstall.Env = append(uninstall.Env, "PATH="+path)
	return uninstall.CombinedOutput()
}

// shutDownThroughSignal reports whether logged holds pid's shutdown through a
// signal with its flush completed.
func shutDownThroughSignal(logged string, pid int) bool {
	return slices.ContainsFunc(strings.Split(logged, "\n"), func(line string) bool {
		return strings.Contains(line, "daemon: shutdown reason=signal flush_completed=true") &&
			strings.Contains(line, fmt.Sprintf(" pid=%d ", pid))
	})
}

// configFiles is every file under configDir but those in stateDir, which the
// daemon writes to as it ends, with their bytes.
func configFiles(t *testing.T, configDir, stateDir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(configDir, func(path string, e fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case path == stateDir:
			return fs.SkipDir
		case e.IsDir():
			return nil
		}
		data, err := os.ReadFile(path)
		files[path] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
