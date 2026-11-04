//go:build integration

package portaltest_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/portalbintest"
	"github.com/leeovery/portal/internal/portaltest"
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
		d = portaltest.StartSaverDaemon(t, binary)

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
	})

	if d == nil {
		t.Fatal("the daemon never started")
	}
	if !strings.Contains(shutdown, "daemon: shutdown") || !strings.Contains(shutdown, "flush_completed=true") {
		t.Errorf("portal.log at cleanup:\n%s\nwant the daemon ended through its shutdown, its final flush completed", shutdown)
	}
	if state.IsProcessAlive(d.PID) {
		t.Errorf("the daemon %d survived its cleanup", d.PID)
	}
}
