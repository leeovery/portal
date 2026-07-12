package log

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// rotateLogger is the component-bound logger for rotation and retention events.
// internal/log can bind its own component via For with no import cycle — it IS
// the log package. The "log-rotate" component owns rotation/retention events per
// the subsystem taxonomy.
var rotateLogger = For("log-rotate")

// sealedMode is the immutable permission applied to rotated (past-day) log files:
// owner read-only. Invariant 1 (rotated-file immutability) narrows the
// destruction surface to today's file only — a buggy library cannot overwrite a
// 0400 past-day file.
const sealedMode os.FileMode = 0o400

// chmodFunc is the test-only seam over os.Chmod so the WARN-and-continue path can
// be exercised deterministically. Production always uses os.Chmod; tests swap it
// and restore via t.Cleanup.
var chmodFunc = os.Chmod

// sealPastDayFiles implements step 2d of the rotation rule (Invariant 1:
// rotated-file immutability). It is invoked from the sink's day-roll seam ONLY
// when the calendar date advanced — never on a same-day inode-mismatch reopen —
// and OUTSIDE the sink mutex, so the chmod-failure WARN below can safely
// re-enter the sink's Write (see fireDayRoll).
//
// It lists ${stateDir}/portal.log.* and chmod 0400s every GENUINE past-day log
// file: a sibling matching the strict portal.log.<YYYY-MM-DD>[.<N>] shape whose
// date is not today and whose mode is not already 0400. Three families are
// skipped by the strict date-parse:
//   - portal.log.<pid>.symlink.tmp — the atomic-swing temp. LOAD-BEARING: leaving
//     it writable keeps its best-effort reclamation from being bricked by a 0400.
//   - portal.log.swept.<date> — the retention single-winner sentinel.
//   - any future non-log sibling.
//
// Today's file and today's same-day .N segments are NOT sealed: a peer process
// may hold an open O_APPEND fd on a same-day segment (chmod does not evict an
// already-open writer on Unix), so same-day files stay part of today's active
// write surface. The date != today filter excludes them; the next day's sweep
// seals all of yesterday's segments at once.
//
// A multi-day downtime catches up in one pass: every candidate with date != today
// is sealed, so no per-missed-day catchup logic is needed.
//
// The sweep is best-effort: a chmod failure emits ONE WARN under log-rotate and
// the sweep CONTINUES to the remaining files — it never aborts.
func sealPastDayFiles(stateDir, today string) {
	matches, err := filepath.Glob(filepath.Join(stateDir, portalLogName+".*"))
	if err != nil {
		// Glob only errors on a malformed pattern, which portalLogName+".*" never
		// is; treat any error as "nothing to seal" and return.
		return
	}

	for _, path := range matches {
		date, ok := pastDayLogDate(filepath.Base(path))
		if !ok || date == today {
			continue // Not a strict past-day log file (skip), or today's file.
		}

		info, err := os.Stat(path)
		if err != nil {
			continue // Vanished or unstatable between glob and stat — skip.
		}
		if info.Mode().Perm() == sealedMode {
			continue // Already sealed: no redundant chmod.
		}

		if err := chmodFunc(path, sealedMode); err != nil {
			rotateLogger.Warn("chmod failed", "error", err, "path", path)
		}
	}
}

// pastDayLogDate strict-parses the date portion of a portal.log sibling basename,
// reporting the date string and whether the name matches the
// portal.log.<YYYY-MM-DD>[.<N>] shape. Anything else — the symlink temp, the
// swept sentinel, any non-log sibling — returns ok=false so the caller skips it.
//
// The date portion is the segment immediately after the "portal.log." prefix; an
// optional single .<N> segment of digits may follow (the size-cap overflow
// segment). The date segment must parse via time.Parse(dateLayout, ...).
func pastDayLogDate(base string) (date string, ok bool) {
	const prefix = portalLogName + "."
	rest, found := strings.CutPrefix(base, prefix)
	if !found || rest == "" {
		return "", false
	}

	segments := strings.SplitN(rest, ".", 2)
	dateSeg := segments[0]
	if _, err := time.Parse(dateLayout, dateSeg); err != nil {
		return "", false // Not a YYYY-MM-DD date: skip (symlink temp, sentinel, etc.).
	}

	// An optional trailing segment must be a non-empty all-digit .<N>; anything
	// else (e.g. portal.log.<date>.symlink.tmp, were it ever to exist) is not a
	// genuine log segment and is skipped.
	if len(segments) == 2 && !isAllDigits(segments[1]) {
		return "", false
	}

	return dateSeg, true
}

// isAllDigits reports whether s is non-empty and consists solely of ASCII digits.
func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
