package log

import "path/filepath"

// Date-keyed log filename helpers.
//
// Every helper is keyed off the caller-supplied stateDir (a plain string joined
// here) so internal/log NEVER imports internal/state — that import would close
// a cycle (internal/state imports internal/log for its own logging). This is
// the same import-cycle guard documented on portalLogName in init.go.
//
// Only the helpers consumed by production code in this task (dayFile,
// symlinkPath) are declared here. The remaining name builders the rotation
// machinery will need — the size-cap `.N` segment names (Task 2-6), the
// `portal.log.swept.<date>` retention sentinel (Task 2-8), and the
// `portal.log.<pid>.symlink.tmp` atomic-swing temp (Task 2-3) — are deliberately
// NOT added here: each would be unused-by-production until its owning task wires
// it, and the `unused` linter (staticcheck U1000) flags unexported functions
// with no callers. They are introduced alongside their first call site.

// dayFile is the day's base log file: ${stateDir}/portal.log.<date>, where date
// is the time.Now().Format("2006-01-02") calendar key.
func dayFile(stateDir, date string) string {
	return filepath.Join(stateDir, portalLogName+"."+date)
}

// symlinkPath is the stable live-target indirection: ${stateDir}/portal.log. It
// is a symlink pointing at the current day's file so `tail -f portal.log`
// always follows today's file regardless of which process owns the swing.
func symlinkPath(stateDir string) string {
	return filepath.Join(stateDir, portalLogName)
}
