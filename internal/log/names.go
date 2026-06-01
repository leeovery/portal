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
// alongside its call site in symlink.go (pidSymlinkTmp). The remaining name
// builder the rotation machinery will need — the `portal.log.swept.<date>`
// retention sentinel (Task 2-8) — is deliberately NOT added yet: it would be
// unused-by-production until its owning task wires it, and the `unused` linter
// (staticcheck U1000) flags unexported functions with no callers. It is
// introduced alongside its first call site.

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
