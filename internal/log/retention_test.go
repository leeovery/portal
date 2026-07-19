package log

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// recordsByMessage returns every captured record whose message equals msg.
func recordsByMessage(captured *[]capturedRecord, msg string) []capturedRecord {
	var out []capturedRecord
	for _, r := range *captured {
		if r.message == msg {
			out = append(out, r)
		}
	}
	return out
}

// sweptSentinelName builds the retention single-winner sentinel basename for a
// given date — used by tests to pre-seed the gate or stale sentinels.
func sweptSentinelName(date string) string {
	return portalLogName + ".swept." + date
}

func TestRunRetentionSweep_ReturnsImmediatelyWhenGateLost(t *testing.T) {
	dir := t.TempDir()

	// Pre-seed today's sentinel so the O_EXCL gate is lost.
	touchFile(t, dir, sweptSentinelName("2026-05-30"))

	// Seed a clearly-deletable past-day file: if the sweep ran it would be removed.
	old := touchFile(t, dir, "portal.log.2026-01-01")

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	runRetentionSweep(dir, "2026-05-30", true)

	if len(*captured) != 0 {
		t.Errorf("gate-lost sweep emitted %d records; want 0 (run nothing, emit nothing)", len(*captured))
	}
	if _, err := os.Stat(old); err != nil {
		t.Errorf("gate-lost sweep deleted %s; want untouched (sweep must not run)", filepath.Base(old))
	}
}

func TestRunRetentionSweep_EmitsInfoBeforeEachRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	// 30-day window from 2026-05-30 → cutoff 2026-04-30. This file predates it.
	old := touchFile(t, dir, "portal.log.2026-01-15")

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	runRetentionSweep(dir, "2026-05-30", true)

	infos := recordsByMessage(captured, "deleted")
	if len(infos) != 1 {
		t.Fatalf("got %d 'deleted' INFO records, want 1", len(infos))
	}
	info := infos[0]
	if got := info.attrs["component"]; got != "log-rotate" {
		t.Errorf("INFO component = %q, want log-rotate", got)
	}
	if got := info.attrs["path"]; got != old {
		t.Errorf("INFO path = %q, want %q", got, old)
	}
	if got := info.attrs["retention"]; got != "30" {
		t.Errorf("INFO retention = %q, want 30", got)
	}
	// The INFO record exists AND the file is gone after — the INFO precedes the
	// os.Remove (it landed in today's already-open file before the unlink).
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file %s still present after sweep; want deleted", filepath.Base(old))
	}
}

func TestRunRetentionSweep_DeletesOlderKeepsWithinWindow(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	// today 2026-05-30, 30-day window → cutoff 2026-04-30. Strictly older < cutoff
	// is deleted; on-or-after cutoff is kept.
	deleted1 := touchFile(t, dir, "portal.log.2026-04-29")   // < cutoff: delete
	deleted2 := touchFile(t, dir, "portal.log.2026-04-29.1") // segment < cutoff: delete
	keptCutoff := touchFile(t, dir, "portal.log.2026-04-30") // == cutoff: keep
	keptRecent := touchFile(t, dir, "portal.log.2026-05-29") // within window: keep

	runRetentionSweep(dir, "2026-05-30", true)

	for _, p := range []string{deleted1, deleted2} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s still present; want deleted (date < cutoff)", filepath.Base(p))
		}
	}
	for _, p := range []string{keptCutoff, keptRecent} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s missing; want kept (date >= cutoff)", filepath.Base(p))
		}
	}
}

func TestRunRetentionSweep_FallsBackTo30WithWarnOnInvalidEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "banana")

	// With fallback 30 days from 2026-05-30 the cutoff is 2026-04-30; this file
	// predates it and must still be deleted (fallback retention is applied).
	old := touchFile(t, dir, "portal.log.2026-01-01")

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	runRetentionSweep(dir, "2026-05-30", true)

	warns := recordsByMessage(captured, "invalid PORTAL_LOG_RETENTION_DAYS")
	if len(warns) != 1 {
		t.Fatalf("got %d invalid-env WARN records, want 1", len(warns))
	}
	w := warns[0]
	if got := w.attrs["component"]; got != "log-rotate" {
		t.Errorf("WARN component = %q, want log-rotate", got)
	}
	if got := w.attrs["raw"]; got != "banana" {
		t.Errorf("WARN raw = %q, want banana (verbatim)", got)
	}
	if got := w.attrs["retention"]; got != "30" {
		t.Errorf("WARN retention = %q, want 30", got)
	}
	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; fallback retention=30 should delete it", filepath.Base(old))
	}
}

func TestRunRetentionSweep_NeverDeletesSymlinkTmpOrSweptSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "0")

	// retention=0 → cutoff = today, so EVERYTHING with a strict past date is
	// deleted. The non-log siblings must survive the cutoff walk regardless. The
	// winner creates today's sentinel itself via claimSweepGate — we assert that
	// live claim survives the walk too, so today's sentinel is NOT pre-seeded
	// (pre-seeding would lose the gate and the walk would never run).
	tmp := touchFile(t, dir, "portal.log."+strconv.Itoa(os.Getpid())+".symlink.tmp")
	other := touchFile(t, dir, "portal.log.notes")
	sentinel := sweptSentinelFile(dir, "2026-05-30")

	runRetentionSweep(dir, "2026-05-30", true)

	for _, p := range []string{tmp, sentinel, other} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was deleted by the cutoff walk; strict date-parse must skip non-log siblings", filepath.Base(p))
		}
	}
}

func TestRunRetentionSweep_PrunesStaleSweptSentinelsKeepsToday(t *testing.T) {
	dir := t.TempDir()

	// Today's sentinel is NOT pre-seeded — claimSweepGate creates it as the winner.
	// Pre-seeding it would lose the gate and abort before the prune ever runs.
	stale1 := touchFile(t, dir, sweptSentinelName("2026-05-28"))
	stale2 := touchFile(t, dir, sweptSentinelName("2026-05-29"))
	todaySentinel := sweptSentinelFile(dir, "2026-05-30")

	runRetentionSweep(dir, "2026-05-30", true)

	for _, p := range []string{stale1, stale2} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("stale sentinel %s still present; want pruned (date != today)", filepath.Base(p))
		}
	}
	if _, err := os.Stat(todaySentinel); err != nil {
		t.Errorf("today's sentinel %s pruned; want kept (live claim)", filepath.Base(todaySentinel))
	}
}

func TestRunRetentionSweep_WarnsAndContinuesOnRemoveFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "0")

	// retention=0 → cutoff = today, so both past-day files are deletion candidates.
	failPath := filepath.Join(dir, "portal.log.2026-04-01")
	okPath := touchFile(t, dir, "portal.log.2026-04-02")
	touchFile(t, dir, "portal.log.2026-04-01")

	prev := removeFunc
	removeFunc = func(path string) error {
		if path == failPath {
			return errors.New("synthetic remove failure")
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { removeFunc = prev })

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	runRetentionSweep(dir, "2026-05-30", true)

	warns := recordsByMessage(captured, "delete failed")
	if len(warns) != 1 {
		t.Fatalf("got %d 'delete failed' WARN records, want 1", len(warns))
	}
	w := warns[0]
	if got := w.attrs["component"]; got != "log-rotate" {
		t.Errorf("WARN component = %q, want log-rotate", got)
	}
	if got := w.attrs["path"]; got != failPath {
		t.Errorf("WARN path = %q, want %q", got, failPath)
	}
	if _, ok := w.attrs["error"]; !ok {
		t.Errorf("WARN missing error attr")
	}

	// The sweep continued past the failure: the other candidate was deleted.
	if _, err := os.Stat(okPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; sweep must continue past a remove failure", filepath.Base(okPath))
	}
}

func TestRunRetentionSweep_SingleSourcesBreadcrumbsAcrossConcurrentStartups(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	touchFile(t, dir, "portal.log.2026-01-15") // deletion candidate

	rec, captured := newComponentCapture()
	SetTestHandler(t, rec)

	// First winner sweeps (creates today's sentinel via O_EXCL, deletes, emits).
	runRetentionSweep(dir, "2026-05-30", true)
	// Second process the same day loses the gate: must add NO further breadcrumbs.
	runRetentionSweep(dir, "2026-05-30", true)

	infos := recordsByMessage(captured, "deleted")
	if len(infos) != 1 {
		t.Errorf("got %d 'deleted' breadcrumbs across two startups, want 1 (single-sourced)", len(infos))
	}
	warns := recordsByMessage(captured, "delete failed")
	if len(warns) != 0 {
		t.Errorf("got %d 'delete failed' WARNs, want 0 (second process must not re-attempt deletions)", len(warns))
	}
}

func TestRunRetentionSweep_UngatedAlwaysRunsRegardlessOfSentinel(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	// today's sentinel is present — a gated sweep would no-op. The ungated path
	// (portal doctor --fix, Task 2-9) must run anyway.
	touchFile(t, dir, sweptSentinelName("2026-05-30"))
	old := touchFile(t, dir, "portal.log.2026-01-15")

	runRetentionSweep(dir, "2026-05-30", false)

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("%s still present; ungated sweep must run regardless of the sentinel", filepath.Base(old))
	}
}

func TestRotatingSink_RunsRetentionSweepOnRealDayRoll(t *testing.T) {
	day1 := mustDate(2026, 5, 29)
	set := fixedClock(t, day1)
	t.Setenv("PORTAL_LOG_RETENTION_DAYS", "30")

	dir := t.TempDir()

	s := newRotatingSink(dir, defaultRotateSize)
	t.Cleanup(func() { _ = s.close() })

	if _, err := s.Write([]byte("day-one\n")); err != nil {
		t.Fatalf("day-one Write: %v", err)
	}

	// Seed an aged-out file AFTER the first-of-day write (which now fires its own
	// gated sweep — PART 1). It must survive until the next day's roll deletes it.
	// cutoff on 2026-05-30 with 30-day retention is 2026-04-30; this predates it.
	old := touchFile(t, dir, "portal.log.2026-01-01")
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("aged file removed before the roll; want untouched until the day roll")
	}

	// Roll past midnight — the dayRoll seam must now run retention alongside seal.
	set(mustDate(2026, 5, 30))
	if _, err := s.Write([]byte("day-two\n")); err != nil {
		t.Fatalf("day-two Write: %v", err)
	}

	if _, err := os.Stat(old); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("aged file still present after day roll; sink must wire runRetentionSweep into dayRoll")
	}
}
