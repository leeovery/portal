package log

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestSweepLogs_DeletesEveryPriorDayKeepsTodayWithCutoffToday pins the
// `portal clean --logs` contract: retentionDays==0 => cutoff == today, so every
// rotated file with a date strictly before today is deleted while today's base
// file and its .N segments survive (cutoff is computed from the injected clock,
// so the test must pin today via fixedClock).
func TestSweepLogs_DeletesEveryPriorDayKeepsTodayWithCutoffToday(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	priorDay := touchFile(t, dir, "portal.log.2026-05-29")   // < today: delete
	priorSeg := touchFile(t, dir, "portal.log.2026-05-29.1") // segment < today: delete
	older := touchFile(t, dir, "portal.log.2026-01-01")      // far < today: delete
	todayBase := touchFile(t, dir, "portal.log.2026-05-30")  // == today: keep
	todaySeg := touchFile(t, dir, "portal.log.2026-05-30.1") // today's segment: keep

	if err := SweepLogs(dir, 0, false); err != nil {
		t.Fatalf("SweepLogs returned error: %v", err)
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

// TestSweepLogs_BypassesGateWhenSentinelPresent pins the gate-bypass contract:
// SweepLogs is invoked with gated==false, so an existing portal.log.swept.<today>
// sentinel must NOT short-circuit the sweep — it runs anyway.
func TestSweepLogs_BypassesGateWhenSentinelPresent(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	// Pre-seed today's sentinel: a gated sweep would no-op here.
	touchFile(t, dir, sweptSentinelName("2026-05-30"))
	old := touchFile(t, dir, "portal.log.2026-01-15")

	if err := SweepLogs(dir, 0, false); err != nil {
		t.Fatalf("SweepLogs returned error: %v", err)
	}

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; --logs sweep must bypass the swept.<today> gate", filepath.Base(old))
	}
}

// TestSweepLogs_RemovesAllSweptSentinelsIncludingToday pins the --logs-only
// prune behaviour: the gated==false path removes EVERY portal.log.swept.*
// sentinel, today's included (an explicit user clean wants a clean slate; the
// next per-process startup re-claims its own gate).
func TestSweepLogs_RemovesAllSweptSentinelsIncludingToday(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))

	stale1 := touchFile(t, dir, sweptSentinelName("2026-05-28"))
	stale2 := touchFile(t, dir, sweptSentinelName("2026-05-29"))
	today := touchFile(t, dir, sweptSentinelName("2026-05-30"))

	if err := SweepLogs(dir, 0, false); err != nil {
		t.Fatalf("SweepLogs returned error: %v", err)
	}

	for _, p := range []string{stale1, stale2, today} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still present; --logs must remove ALL swept.* sentinels (today included)", filepath.Base(p))
		}
	}
}

// TestSweepLogs_GatedPathKeepsTodaySentinel pins that the gated==true variant
// (the per-process startup path) still KEEPS today's sentinel — the all-sentinel
// prune is tied to gated==false and must not leak into the gated path.
func TestSweepLogs_GatedPathKeepsTodaySentinel(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	stale := touchFile(t, dir, sweptSentinelName("2026-05-28"))
	// today's sentinel is NOT pre-seeded — the gated winner creates it via the
	// O_EXCL claim; pre-seeding would lose the gate and skip the sweep entirely.
	todaySentinel := sweptSentinelFile(dir, "2026-05-30")

	// retentionDays<0 sentinel value: gated path resolves from env, so pass any
	// value (the gated path ignores the explicit days and reads the env).
	if err := SweepLogs(dir, -1, true); err != nil {
		t.Fatalf("SweepLogs returned error: %v", err)
	}

	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale sentinel %s still present; gated prune must remove date != today", filepath.Base(stale))
	}
	if _, err := os.Stat(todaySentinel); err != nil {
		t.Errorf("today's sentinel %s pruned; gated path must KEEP today's live claim", filepath.Base(todaySentinel))
	}
}

// TestSweepLogs_ForcesCutoffTodayRegardlessOfEnv pins that the --logs path's
// retentionDays==0 forces cutoff==today even when the env requests a wider
// retention window — the explicit days override the env value.
func TestSweepLogs_ForcesCutoffTodayRegardlessOfEnv(t *testing.T) {
	dir := t.TempDir()
	fixedClock(t, mustDate(2026, 5, 30))
	// Env asks for 365-day retention; --logs (retentionDays=0) must still delete
	// a file only a day old because it forces cutoff=today.
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "365")

	recent := touchFile(t, dir, "portal.log.2026-05-29")

	if err := SweepLogs(dir, 0, false); err != nil {
		t.Fatalf("SweepLogs returned error: %v", err)
	}

	if _, err := os.Stat(recent); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; --logs forces cutoff=today regardless of PORTAL_LOG_RETENTION_DAYS", filepath.Base(recent))
	}
}
