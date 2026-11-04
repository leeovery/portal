//go:build integration

package portaltest_test

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
	"github.com/leeovery/portal/internal/proctable"
	"github.com/leeovery/portal/internal/state"
)

func TestStartSaverDaemonAgainstZmx(t *testing.T) {
	binary := portalbintest.Build(t)

	var d *portaltest.SaverDaemon
	var shutdown string
	t.Run("it runs the daemon at an absolute path, which the identity check and the sandboxed pgrep both find", func(t *testing.T) {
		// A cleanup registered after the test's first TempDir, so it runs once
		// the daemon is stopped and before its state directory goes.
		_ = t.TempDir()
		t.Cleanup(func() {
			if d != nil {
				shutdown = portaltest.ReadPortalLogSafe(d.StateDir)
			}
		})
		// Exports of the developer's, which the daemon must never see.
		developersHooks := filepath.Join(t.TempDir(), "hooks.json")
		t.Setenv("PORTAL_HOOKS_FILE", developersHooks)
		t.Setenv("PORTAL_TEST_DEVELOPERS_OWN", "leaked")
		t.Setenv("SHELL", "/bin/zsh")
		// The test's own logging, which the daemon must see.
		t.Setenv("PORTAL_LOG_LEVEL", "debug")

		d = portaltest.StartSaverDaemon(t, binary)
		portaltest.AssertLogLevelResolved(t, state.PortalLog(d.StateDir), d.PID, "debug")

		if got, err := state.IdentifyDaemon(d.PID); err != nil || got != state.IdentifyIsPortalDaemon {
			t.Errorf("IdentifyDaemon(%d) = %v, %v; want the daemon at %s identified", d.PID, got, err, binary)
		}
		if pids, err := portaltest.PgrepPortalDaemons(); err != nil || !slices.Equal(pids, []int{d.PID}) {
			t.Errorf("PgrepPortalDaemons = %v, %v; want the test's daemon alone", pids, err)
		}
		if lock, err := state.AcquireDaemonLock(d.StateDir); !errors.Is(err, state.ErrDaemonLockHeld) {
			if lock != nil {
				_ = lock.Close()
			}
			t.Errorf("AcquireDaemonLock = %v, want the daemon holding it", err)
		}

		args, err := proctable.ReadArgs(d.PID)
		if err != nil {
			t.Fatalf("read the daemon's environment: %v", err)
		}
		env := map[string]string{}
		for _, entry := range args.Env {
			key, value, _ := strings.Cut(entry, "=")
			env[key] = value
		}
		if got := env["PORTAL_HOOKS_FILE"]; got == developersHooks || !strings.HasPrefix(got, env["XDG_CONFIG_HOME"]+"/") {
			t.Errorf("the daemon's PORTAL_HOOKS_FILE = %q, want one in its scratch config %q, never the export %q",
				got, env["XDG_CONFIG_HOME"], developersHooks)
		}
		if got, ok := env["PORTAL_TEST_DEVELOPERS_OWN"]; ok {
			t.Errorf("the daemon's environment holds PORTAL_TEST_DEVELOPERS_OWN=%q, want none of this process's beyond the isolation", got)
		}
		if got := env["SHELL"]; got != "/bin/sh" {
			t.Errorf("the daemon's SHELL = %q, want zmxtest's /bin/sh", got)
		}
	})

	if d == nil {
		t.Fatal("the daemon never started")
	}
	// zmx ends a session's process with SIGHUP.
	if want := "daemon: shutdown reason=sighup flush_completed=true"; !strings.Contains(shutdown, want) {
		t.Errorf("portal.log at cleanup:\n%s\nwant %q: the session's kill ending the daemon through its final flush", shutdown, want)
	}
	if state.IsProcessAlive(d.PID) {
		t.Errorf("the daemon %d survived its cleanup", d.PID)
	}
}
