package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// sweptSentinelMode is the permission applied to the retention single-winner
// sentinel (${stateDir}/portal.log.swept.<today>): owner read/write. It is a
// zero-content marker file whose existence — not contents — claims the day's
// sweep, so the mode only governs who may re-create it after a prune.
const sweptSentinelMode os.FileMode = 0o600

// removeFunc is the test-only seam over os.Remove so the WARN-and-continue path
// of the retention deletion loop can be exercised deterministically (forcing a
// genuine unlink failure on a chosen path is otherwise non-portable). Production
// always uses os.Remove; tests swap it and restore via t.Cleanup. It mirrors the
// chmodFunc / openSegmentFunc seam convention.
var removeFunc = os.Remove

// runRetentionSweep bounds rotated history to PORTAL_LOG_RETENTION_DAYS (default
// 30) and emits one auditable INFO breadcrumb per deletion. It implements the
// retention rule (spec § Retention policy and audit, Mechanical rule steps 0-3).
//
// It is invoked from the sink's day-roll seam on the first Handle of each
// calendar date (dateChanged==true), AFTER today's file is opened and OUTSIDE
// the sink mutex (its breadcrumbs re-enter the sink's Write — see
// fireDayRoll), so every breadcrumb (and the invalid-env WARN) lands in
// today's already-open file — never in the file being aged out.
//
// gated selects the single-winner discipline:
//   - gated==true (the per-process-startup path): step 0 claims the day via an
//     O_CREAT|O_EXCL portal.log.swept.<today> sentinel. On EEXIST another process
//     already owns today's sweep — return immediately, run nothing, emit nothing.
//     This dedupes the reboot-storm (~32 processes each emitting process: start as
//     their first log call) to a single-sourced deletion audit.
//   - gated==false (the `portal doctor --fix` path, Task 2-9): step 0 is SKIPPED
//     entirely — an explicit user invocation always runs regardless of any
//     existing sentinel.
//
// Best-effort and synchronous inside the winner's first-of-day Handle. ACCEPTED
// PARTIAL-SWEEP RISK: a winner SIGKILL'd or crashing mid-deletion-loop leaves the
// sentinel created with deletions partial, so a few extra rotated files persist
// until the next day's fresh winner sweeps. Retention is a disk-space bound, not
// a correctness boundary — the slip self-heals at ~tens-of-MB cost, so there is
// no resumable sentinel (rejected as disproportionate complexity).
func runRetentionSweep(stateDir, today string, gated bool) {
	// nil forcedDays => resolve the retention window from the environment (the
	// per-process-startup path). The `portal doctor --fix` path threads an
	// explicit value via runRetentionSweepWithDays instead.
	runRetentionSweepWithDays(stateDir, today, gated, nil)
}

// runRetentionSweepWithDays is the shared sweep implementing the retention rule
// (spec § Retention policy and audit, Mechanical rule steps 0-3). It is the
// single source of the walk/delete/prune logic — both the per-process-startup
// path (runRetentionSweep, forcedDays==nil) and the `portal doctor --fix` path
// (SweepLogsForClean, forcedDays!=nil) delegate here so the algorithm is never
// duplicated.
//
// forcedDays selects the retention window:
//   - forcedDays==nil: resolve PORTAL_LOG_RETENTION_DAYS from the environment
//     (default 30); an invalid value emits the canonical fallback WARN.
//   - forcedDays!=nil: use *forcedDays verbatim, bypassing env resolution. The
//     `doctor --fix` path passes 0 (cutoff == today: delete every prior-day rotated
//     file, leaving only today's). No invalid-env WARN is emitted on this path —
//     the value is explicit, not resolved.
//
// gated selects the single-winner discipline (step 0) AND the sentinel-prune
// breadth (step 3):
//   - gated==true (per-process startup): step 0 claims the day via an
//     O_CREAT|O_EXCL portal.log.swept.<today> sentinel; EEXIST means another
//     process owns today's sweep — return immediately, run nothing, emit
//     nothing. Step 3 prunes only swept.<date> sentinels where date != today
//     (KEEPS today's live claim).
//   - gated==false (`portal doctor --fix`): step 0 is SKIPPED — an explicit user
//     invocation always runs regardless of any existing sentinel. Step 3 removes
//     EVERY swept.* sentinel, today's included (an explicit user clean wants a
//     clean slate; the next per-process startup re-claims its own gate).
func runRetentionSweepWithDays(stateDir, today string, gated bool, forcedDays *int) {
	if gated && !claimSweepGate(stateDir, today) {
		return // Lost the single-winner gate: another process owns today's sweep.
	}

	retentionDays := resolveSweepRetentionDays(forcedDays)

	cutoff, ok := retentionCutoff(today, retentionDays)
	if !ok {
		return // today did not parse — nothing to base a cutoff on; skip the walk.
	}

	deletePastCutoff(stateDir, cutoff, retentionDays)
	pruneStaleSentinels(stateDir, today, gated)
}

// resolveSweepRetentionDays returns the retention-window day count for the
// sweep. With an explicit forcedDays (the `doctor --fix` path) it is used verbatim and
// no env resolution or invalid-value WARN occurs. With forcedDays==nil (the
// per-process path) it resolves PORTAL_LOG_RETENTION_DAYS from the environment,
// emitting the canonical fallback WARN on an invalid value.
func resolveSweepRetentionDays(forcedDays *int) int {
	if forcedDays != nil {
		return *forcedDays
	}

	retentionDays, source, raw := resolveRetentionDays(os.Getenv("PORTAL_LOG_RETENTION_DAYS"))
	if source == sourceFallback {
		rotateLogger.Warn("invalid PORTAL_LOG_RETENTION_DAYS", "raw", raw, "retention", retentionDays)
	}
	return retentionDays
}

// SweepLogsForClean is the exported entry point for the explicit user-invoked
// `portal doctor --fix` retention sweep: ungated, cutoff == today (delete every
// prior-day rotated file, leaving only today's), and removing every
// portal.log.swept.* sentinel. It computes today's calendar-day key from the
// injectable clock (nowFunc) and delegates to the shared runRetentionSweepWithDays
// implementation with gated=false and an explicit retentionDays of 0 (cutoff ==
// today) — so the walk/delete/prune algorithm is never duplicated.
//
// The gated per-process startup sweep is NOT reachable from here: it lives behind
// the unexported runRetentionSweep, wired into the sink's dayRoll seam, where it
// resolves its window from PORTAL_LOG_RETENTION_DAYS and claims the single-winner
// portal.log.swept.<today> gate.
//
// The error return is reserved for future failure surfacing; the sweep is
// best-effort (per-file failures WARN and continue) so SweepLogsForClean
// currently always returns nil.
func SweepLogsForClean(stateDir string) error {
	today := nowFunc().Format(dateLayout)
	cleanCutoffDays := 0
	runRetentionSweepWithDays(stateDir, today, false, &cleanCutoffDays)
	return nil
}

// claimSweepGate creates ${stateDir}/portal.log.swept.<today> via O_CREAT|O_EXCL
// and reports whether THIS process won the day's sweep. EEXIST (another process
// already created it) returns false; any other open error also returns false
// (best-effort: a sweep we cannot gate is skipped rather than run un-deduped). On
// success the sentinel fd is closed immediately — its existence, not an open
// handle, holds the claim.
func claimSweepGate(stateDir, today string) bool {
	f, err := os.OpenFile(sweptSentinelFile(stateDir, today), os.O_CREATE|os.O_EXCL|os.O_WRONLY, sweptSentinelMode)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// retentionCutoff computes the exclusive deletion boundary: a rotated file whose
// date is strictly before cutoff is deleted. cutoff = today - retentionDays. It
// returns ok=false when today does not parse as a date (the caller then skips the
// walk).
func retentionCutoff(today string, retentionDays int) (cutoff time.Time, ok bool) {
	todayTime, err := time.Parse(dateLayout, today)
	if err != nil {
		return time.Time{}, false
	}
	return todayTime.AddDate(0, 0, -retentionDays), true
}

// deletePastCutoff lists ${stateDir}/portal.log.* and deletes every genuine
// rotated log file whose strict-parsed date is before cutoff, emitting one INFO
// breadcrumb BEFORE each os.Remove (step 2). The strict date-parse (pastDayLogDate,
// REUSED) skips the symlink temp, the swept sentinel, and any non-log sibling —
// they are NEVER deletion candidates. A remove failure emits ONE WARN with the
// error attr and the sweep CONTINUES to the remaining files (best-effort).
func deletePastCutoff(stateDir string, cutoff time.Time, retentionDays int) {
	matches, err := filepath.Glob(filepath.Join(stateDir, portalLogName+".*"))
	if err != nil {
		// Glob only errors on a malformed pattern, which portalLogName+".*" never
		// is; treat any error as "nothing to delete" and return.
		return
	}

	for _, path := range matches {
		date, ok := pastDayLogDate(filepath.Base(path))
		if !ok {
			continue // Not a strict rotated-log name (symlink temp, sentinel, sibling).
		}
		fileDate, err := time.Parse(dateLayout, date)
		if err != nil || !fileDate.Before(cutoff) {
			continue // Within the retention window (date >= cutoff): keep.
		}

		// INFO breadcrumb BEFORE the unlink so it lands in today's open file even
		// if the process is killed between the Info and the Remove.
		rotateLogger.Info("deleted", "path", path, "retention", retentionDays)
		if err := removeFunc(path); err != nil {
			rotateLogger.Warn("delete failed", "error", err, "path", path)
		}
	}
}

// pruneStaleSentinels unlinks portal.log.swept.<date> sentinels (step 3). The
// date-cutoff walk (deletePastCutoff) excludes the swept.* family by
// construction (its date slot is the literal "swept", which pastDayLogDate
// rejects), so sentinels are reclaimed here rather than by the retention cutoff.
// An unlink failure emits ONE WARN and the prune CONTINUES.
//
// gated selects the prune breadth:
//   - gated==true (per-process startup): keep today's sentinel (the live
//     single-winner claim) and unlink only date != today.
//   - gated==false (`portal doctor --fix`): remove EVERY swept.* sentinel,
//     today's included. This is doctor --fix-only behaviour — an explicit user
//     repair wants a clean slate, and the next per-process startup re-claims its
//     own gate.
func pruneStaleSentinels(stateDir, today string, gated bool) {
	matches, err := filepath.Glob(filepath.Join(stateDir, sweptPrefix+"*"))
	if err != nil {
		return
	}

	for _, path := range matches {
		date, ok := sweptSentinelDate(filepath.Base(path))
		if !ok {
			continue // Not a swept.<date> sentinel.
		}
		if gated && date == today {
			continue // Per-process path keeps today's live claim; doctor --fix removes it.
		}
		if err := removeFunc(path); err != nil {
			rotateLogger.Warn("sentinel prune failed", "error", err, "path", path)
		}
	}
}

// sweptSentinelDate extracts the <date> portion of a portal.log.swept.<date>
// sentinel basename, reporting ok=false for anything that is not a swept
// sentinel. The date is returned verbatim (no strict YYYY-MM-DD parse) so a
// malformed or legacy sentinel whose date != today is still pruned.
func sweptSentinelDate(base string) (date string, ok bool) {
	rest, found := strings.CutPrefix(base, sweptPrefix)
	if !found || rest == "" {
		return "", false
	}
	return rest, true
}
