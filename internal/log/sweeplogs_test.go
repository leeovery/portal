package log

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSweepLogsForClean_DeletesEveryPriorDayKeepsTodayWithCutoffToday pins the
// `portal doctor --fix` log-sweep contract: the ungated cutoff==today sweep deletes every
// rotated file with a date strictly before today while today's base file and its
// .N segments survive (cutoff is computed from the injected clock, so the test
// must pin today via fixedClock).
func TestSweepLogsForClean_DeletesEveryPriorDayKeepsTodayWithCutoffToday(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	priorDay := touchFile(t, dir, "portal.log.2026-05-29")   // < today: delete
	priorSeg := touchFile(t, dir, "portal.log.2026-05-29.1") // segment < today: delete
	older := touchFile(t, dir, "portal.log.2026-01-01")      // far < today: delete
	todayBase := touchFile(t, dir, "portal.log.2026-05-30")  // == today: keep
	todaySeg := touchFile(t, dir, "portal.log.2026-05-30.1") // today's segment: keep

	if err := SweepLogsForClean(dir); err != nil {
		t.Fatalf("SweepLogsForClean returned error: %v", err)
	}

	for _, p := range []string{priorDay, priorSeg, older} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still present; cutoff=today must delete every prior-day file", filepath.Base(p))
		}
	}
	for _, p := range []string{todayBase, todaySeg} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing; today's file (date == cutoff, strict <) must survive", filepath.Base(p))
		}
	}
}

// TestSweepLogsForClean_BypassesGateWhenSentinelPresent pins the gate-bypass
// contract: the ungated `--logs` sweep runs even when an existing
// portal.log.swept.<today> sentinel is present — it must NOT short-circuit.
func TestSweepLogsForClean_BypassesGateWhenSentinelPresent(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	// Pre-seed today's sentinel: a gated sweep would no-op here.
	touchFile(t, dir, sweptSentinelName("2026-05-30"))
	old := touchFile(t, dir, "portal.log.2026-01-15")

	if err := SweepLogsForClean(dir); err != nil {
		t.Fatalf("SweepLogsForClean returned error: %v", err)
	}

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; --logs sweep must bypass the swept.<today> gate", filepath.Base(old))
	}
}

// TestSweepLogsForClean_RemovesAllSweptSentinelsIncludingToday pins the
// --logs-only prune behaviour: the ungated path removes EVERY
// portal.log.swept.* sentinel, today's included (an explicit user clean wants a
// clean slate; the next per-process startup re-claims its own gate).
func TestSweepLogsForClean_RemovesAllSweptSentinelsIncludingToday(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	stale1 := touchFile(t, dir, sweptSentinelName("2026-05-28"))
	stale2 := touchFile(t, dir, sweptSentinelName("2026-05-29"))
	today := touchFile(t, dir, sweptSentinelName("2026-05-30"))

	if err := SweepLogsForClean(dir); err != nil {
		t.Fatalf("SweepLogsForClean returned error: %v", err)
	}

	for _, p := range []string{stale1, stale2, today} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still present; --logs must remove ALL swept.* sentinels (today included)", filepath.Base(p))
		}
	}
}

// TestSweepLogsForClean_ForcesCutoffTodayRegardlessOfEnv pins that the --logs
// path forces cutoff==today even when the env requests a wider retention window —
// the explicit clean cutoff overrides any PORTAL_LOG_RETENTION_DAYS value.
func TestSweepLogsForClean_ForcesCutoffTodayRegardlessOfEnv(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))
	// Env asks for 365-day retention; --logs must still delete a file only a day
	// old because it forces cutoff=today.
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "365")

	recent := touchFile(t, dir, "portal.log.2026-05-29")

	if err := SweepLogsForClean(dir); err != nil {
		t.Fatalf("SweepLogsForClean returned error: %v", err)
	}

	if _, err := os.Stat(recent); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; --logs forces cutoff=today regardless of PORTAL_LOG_RETENTION_DAYS", filepath.Base(recent))
	}
}
