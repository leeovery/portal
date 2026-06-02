package restoretest

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// portalLogName is the portal.log basename joined onto the caller-supplied
// stateDir. Mirrors internal/log's own constant — restoretest must not import
// internal/log just to read it, and keeping the literal local avoids any
// import-cycle risk.
const portalLogName = "portal.log"

// portalLogDateLayout is the calendar-day key format used in the dated day-file
// basename (portal.log.<date>). It mirrors internal/log's dateLayout constant —
// kept local for the same no-import-cycle reason as portalLogName. The two
// MUST stay in lockstep: the production sink's reopen swings the portal.log
// symlink at portal.log.<today> using this exact format, so OpenTestLogger must
// produce the identical dated basename for a co-resident real binary to append
// to the same file rather than open a divergent one.
const portalLogDateLayout = "2006-01-02"

// OpenTestLogger returns a *slog.Logger that writes a text-format audit trail
// to the production rotating sink's on-disk shape under stateDir: a dated day
// file <stateDir>/portal.log.<date> plus a <stateDir>/portal.log SYMLINK
// pointing at it (the bare relative dated basename, exactly as the production
// rotatingSink.reopen swings it).
//
// Honoring the symlink shape — rather than the previous bare regular-file
// portal.log — is load-bearing whenever a test opens this logger against the
// same stateDir it also spawns the real portal binary into. Production's
// rotatingSink owns portal.log as a symlink and its reopen runs a migration
// guard that os.Remove()s any *regular-file* portal.log on first write. A bare
// regular file here would be deleted out from under the test's writer by that
// guard; a symlink is left untouched (the guard no-ops on a symlink), so both
// writers append to the same dated day file via O_APPEND and never contend
// destructively. Reading <stateDir>/portal.log follows the symlink to the day
// file, so portaltest.ReadPortalLogSafe and friends observe real on-disk
// records from both writers.
//
// The opened day-file fd is closed via t.Cleanup. The signature keeps the
// *testing.T-first shape and *slog.Logger return type so the promoted call
// sites continue to compile unchanged after the observability migration retyped
// every logging seam to *slog.Logger. Tests that want to capture log output
// in-process (without touching disk) use log.SetTestHandler instead.
func OpenTestLogger(t *testing.T, stateDir string) *slog.Logger {
	t.Helper()

	dayName := portalLogName + "." + time.Now().Format(portalLogDateLayout)
	dayPath := filepath.Join(stateDir, dayName)

	f, err := os.OpenFile(dayPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenTestLogger: open %s: %v", dayPath, err)
	}
	t.Cleanup(func() { _ = f.Close() })

	if err := swingPortalLogSymlink(stateDir, dayName); err != nil {
		t.Fatalf("OpenTestLogger: %v", err)
	}

	return slog.New(slog.NewTextHandler(f, nil))
}

// swingPortalLogSymlink atomically points <stateDir>/portal.log at target (the
// bare relative dated basename, e.g. portal.log.<date>), mirroring the
// production sink's swingSymlink: os.Symlink to a temp then os.Rename over the
// link. Rename is atomic on Unix and last-writer-wins, so refreshing an
// existing portal.log (symlink or regular file) is safe and leaves it as a
// symlink — which is exactly what the production migration guard expects.
func swingPortalLogSymlink(stateDir, target string) error {
	link := filepath.Join(stateDir, portalLogName)
	tmp := link + ".restoretest.symlink.tmp"

	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale symlink temp %s: %w", tmp, err)
	}
	if err := os.Symlink(target, tmp); err != nil {
		return fmt.Errorf("create symlink temp %s -> %s: %w", tmp, target, err)
	}
	if err := os.Rename(tmp, link); err != nil {
		return fmt.Errorf("rename symlink temp %s -> %s: %w", tmp, link, err)
	}
	return nil
}
