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
// calendar date (dateChanged==true), AFTER today's file is opened, so every
// breadcrumb (and the invalid-env WARN) lands in today's already-open file —
// never in the file being aged out.
//
// gated selects the single-winner discipline:
//   - gated==true (the per-process-startup path): step 0 claims the day via an
//     O_CREAT|O_EXCL portal.log.swept.<today> sentinel. On EEXIST another process
//     already owns today's sweep — return immediately, run nothing, emit nothing.
//     This dedupes the reboot-storm (~32 processes each emitting process: start as
//     their first log call) to a single-sourced deletion audit.
//   - gated==false (the `portal clean --logs` path, Task 2-9): step 0 is SKIPPED
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
	if gated && !claimSweepGate(stateDir, today) {
		return // Lost the single-winner gate: another process owns today's sweep.
	}

	retentionDays, source, raw := resolveRetentionDays(os.Getenv("PORTAL_LOG_RETENTION_DAYS"))
	if source == sourceFallback {
		rotateLogger.Warn("invalid PORTAL_LOG_RETENTION_DAYS", "raw", raw, "retention", retentionDays)
	}

	cutoff, ok := retentionCutoff(today, retentionDays)
	if !ok {
		return // today did not parse — nothing to base a cutoff on; skip the walk.
	}

	deletePastCutoff(stateDir, cutoff, retentionDays)
	pruneStaleSentinels(stateDir, today)
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

// pruneStaleSentinels unlinks every portal.log.swept.<date> sentinel whose date
// is not today (step 3). The date-cutoff walk (deletePastCutoff) excludes the
// swept.* family by construction (its date slot is the literal "swept", which
// pastDayLogDate rejects), so sentinels are reclaimed here by an exact not-today
// rule rather than by the retention cutoff. An unlink failure emits ONE WARN and
// the prune CONTINUES.
func pruneStaleSentinels(stateDir, today string) {
	matches, err := filepath.Glob(filepath.Join(stateDir, sweptPrefix+"*"))
	if err != nil {
		return
	}

	for _, path := range matches {
		date, ok := sweptSentinelDate(filepath.Base(path))
		if !ok || date == today {
			continue // Not a swept.<date> sentinel, or today's (the live claim).
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
