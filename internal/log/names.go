package log

import (
	"path/filepath"
	"strconv"
)

// Date-keyed log filename helpers.
//
// Every helper is keyed off the caller-supplied stateDir (a plain string joined
// here) so internal/log NEVER imports internal/state — that import would close
// a cycle (internal/state imports internal/log for its own logging). This is
// the same import-cycle guard documented on portalLogName in init.go.
//
// Only the helpers consumed by production code are declared here. The
// `portal.log.<pid>.symlink.tmp` atomic-swing temp builder (Task 2-3) lives
// alongside its call site in symlink.go (pidSymlinkTmp).

// sweptPrefix is the basename prefix of the retention single-winner sentinel:
// ${stateDir}/portal.log.swept.<date>. It deliberately shadows the date slot of
// a genuine log file with the literal "swept" segment, so pastDayLogDate's
// strict YYYY-MM-DD date-parse rejects it — the sentinel is never mistaken for
// a rotated log and never sealed (Task 2-5) nor deleted by the cutoff walk
// (Task 2-8); it is pruned only by the exact not-today rule in the retention
// sweep's step 3.
const sweptPrefix = portalLogName + ".swept."

// sweptSentinelFile is the retention single-winner sentinel path for a calendar
// date: ${stateDir}/portal.log.swept.<date>. The first process to create it for
// <today> (via O_CREAT|O_EXCL) owns that day's retention sweep; all others lose
// the gate and emit nothing.
func sweptSentinelFile(stateDir, date string) string {
	return filepath.Join(stateDir, sweptPrefix+date)
}

// dayFile is the day's base log file: ${stateDir}/portal.log.<date>, where date
// is the time.Now().Format("2006-01-02") calendar key.
func dayFile(stateDir, date string) string {
	return filepath.Join(stateDir, portalLogName+"."+date)
}

// daySegmentFile is a same-day size-cap overflow segment:
// ${stateDir}/portal.log.<date>.<n>. The base day file is portal.log.<date>;
// when it reaches the size cap, writes roll to portal.log.<date>.1, then .2, …
// (n monotonic, discovered via O_CREAT|O_EXCL retry against the highest existing
// .N — see rotateSameDay). n is always >= 1; .0 is never produced.
func daySegmentFile(stateDir, date string, n int) string {
	return filepath.Join(stateDir, portalLogName+"."+date+"."+strconv.Itoa(n))
}

// symlinkPath is the stable live-target indirection: ${stateDir}/portal.log. It
// is a symlink pointing at the current day's file so `tail -f portal.log`
// always follows today's file regardless of which process owns the swing.
func symlinkPath(stateDir string) string {
	return filepath.Join(stateDir, portalLogName)
}
