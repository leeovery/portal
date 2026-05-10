//go:build integration

package cmd

// Unit tests for the EagerSignaler default-selection logic in
// buildReattachOrchestrator (reattach_integration_test.go). Mirrors
// orchestrator_builder_eager_default_test.go in cmd/bootstrap/ — the
// flip introduced by task 4-2 must apply at both helpers (the cmd
// package's reattach builder cannot delegate to the cmd/bootstrap
// helper because Go test-file symbols are not visible across packages).
//
// Build-tagged with //go:build integration because the symbol under
// test (buildReattachOrchestrator) lives in an integration-tagged
// file. Tests here construct dummy clients only to satisfy the
// builder signature; tmux is never invoked.

import (
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/tmux"
)

// TestBuildReattachOrchestrator_EagerSignalerDefaultsToReal asserts the
// new default: buildReattachOrchestrator wires a real RestoreAdapter
// internally, so its EagerSignaler must default to a real
// *bootstrap.EagerSignalCore (mirroring buildProductionOrchestrator) —
// not bootstrap.NoOpEagerHydrateSignaler{} as it did pre-task-4-2.
func TestBuildReattachOrchestrator_EagerSignalerDefaultsToReal(t *testing.T) {
	// PORTAL_STATE_DIR is consulted by state.Dir() inside the builder
	// when resolving stateDir for the real EagerSignalCore. A t.TempDir
	// is sufficient — the test asserts only on orchestrator shape and
	// never invokes Run.
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())

	stateDir := t.TempDir()
	client := &tmux.Client{}

	o := buildReattachOrchestrator(t, client, stateDir)

	if _, ok := o.EagerSignaler.(*bootstrap.EagerSignalCore); !ok {
		t.Errorf("EagerSignaler type = %T; want *bootstrap.EagerSignalCore (real adapter — buildReattachOrchestrator always wires a real RestoreAdapter)", o.EagerSignaler)
	}
}
