package portaltest

// portal_log.go — test-only diagnostic helper for reading the portal.log
// audit trail produced by `portal state daemon` (and other portal
// subcommands) under an isolated state directory.
//
// Used by integration tests to surface the daemon's log content in
// failure messages without forcing every call site to branch on the
// read error. Returns a placeholder string on failure so the caller
// can always embed the result in a `t.Errorf` / `t.Fatalf` format
// argument.
//
// Test-only. The *testing.T parameter is omitted because this helper
// performs a pure read-and-format with no test-state side effects;
// the package's structural protection (other helpers do take
// *testing.T, anchoring the testing import) still prevents production
// code from importing portaltest.
//
// Consolidates two formerly per-test-package duplicates:
//   - cmd/state_daemon_self_supervision_integration_test.go's
//     `readPortalLogSafe`
//   - cmd/bootstrap/composition_e2e_self_eject_integration_test.go's
//     `readPortalLogSafeBootstrap`

import (
	"fmt"
	"os"

	"github.com/leeovery/portal/internal/state"
)

// ReadPortalLogSafe reads portal.log under stateDir and returns its
// contents as a string, or a placeholder describing the read failure.
// The placeholder shape `(read portal.log failed: %v)` is stable so
// failure messages remain greppable across tests.
func ReadPortalLogSafe(stateDir string) string {
	data, err := os.ReadFile(state.PortalLog(stateDir))
	if err != nil {
		return fmt.Sprintf("(read portal.log failed: %v)", err)
	}
	return string(data)
}
