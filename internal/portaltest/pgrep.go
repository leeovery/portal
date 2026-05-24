package portaltest

// pgrep.go — test-only enumeration of live `portal state daemon` processes.
// Thin forwarder over state.PgrepPortalDaemons, the canonical primitive
// shared with the production bootstrap-step-4 orphan-sweep adapter
// (internal/bootstrapadapter/orphan_sweep.go). The forwarder exists so
// integration tests can observe the same candidate set the production
// sweep observes (mod inherent race windows) without portaltest importing
// internal/bootstrapadapter.
//
// Test-only. The package-level *testing.T enforcement is structural: this
// file ships no testing dependency itself, but the package's other helpers
// do, so importing portaltest from production code would fail to build.

import (
	"github.com/leeovery/portal/internal/state"
)

// PgrepPortalDaemons enumerates live `portal state daemon` PIDs. Forwards
// to state.PgrepPortalDaemons — see that function's doc comment for the
// canonical three-shape return contract.
func PgrepPortalDaemons() ([]int, error) {
	return state.PgrepPortalDaemons()
}
