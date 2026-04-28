package cmd

import "github.com/leeovery/portal/internal/state"

// openNoRotateLogger opens portal.log for the non-daemon writer path. Per spec
// "Log Rotation → Concurrent-writer discipline", only the daemon rotates the
// log file; every other Portal writer (hydrate, signal-hydrate, bootstrap CLI
// surfaces) must append-only via O_APPEND so two processes never race a
// rename. This helper bundles the EnsureDir + OpenLogger(rotate=false)
// boilerplate so call sites do not have to repeat it (and accidentally pass
// rotate=true).
//
// On any error — directory creation failure, log file open failure — the
// helper returns nil. Callers either nil-check the result or rely on
// *Logger's nil-receiver no-op semantics. Logging failures must never fail
// the caller; degrading silently to "no logging" is preferable to aborting a
// resurrection-feature command for an opaque diagnostics-side failure.
func openNoRotateLogger() (*state.Logger, error) {
	dir, err := state.EnsureDir()
	if err != nil {
		return nil, err
	}
	logger, err := state.OpenLogger(state.PortalLog(dir), false)
	if err != nil {
		return nil, err
	}
	return logger, nil
}
