package bootstrap_test

// Unit tests for the EagerSignaler default-selection logic in
// buildIntegrationOrchestrator (orchestrator_builder_test.go).
//
// Task 4-2 flips the default: when the caller has provided a real
// RestoreAdapter, leaving EagerSignaler unset must produce a real
// *bootstrap.EagerSignalCore (not bootstrap.NoOpEagerHydrateSignaler{}).
// The rationale is fidelity — most reboot/reattach integration tests want
// the eager step to actually fire so a regression in the eager pipeline
// surfaces here, rather than the previous shape where every site silently
// degraded to NoOp unless it remembered to opt in.
//
// When the caller did NOT provide a RestoreAdapter (Restore is nil and
// the builder defaults it to bootstrap.NoOpRestorer{}), no skeleton
// markers will be set so the eager step would be vacuous — the builder
// retains the NoOp default for that branch.
//
// These tests do not require tmux; they construct dummy clients only to
// satisfy the builder signature and never invoke them.

import (
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/tmux"
)

// TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToRealWhenRestoreReal
// asserts the new default: providing a real RestoreAdapter and leaving
// EagerSignaler nil yields a real *bootstrap.EagerSignalCore. This is the
// core flip introduced by task 4-2.
func TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToRealWhenRestoreReal(t *testing.T) {
	// PORTAL_STATE_DIR is consulted by state.Dir() when the builder
	// resolves stateDir for the real EagerSignalCore. A t.TempDir is
	// sufficient — we never read or write inside it; we only need the
	// path to be non-empty so the EagerSignalCore field gets a valid
	// value. t.Setenv is auto-restored on test exit.
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())

	// Dummy *tmux.Client — never invoked. The builder requires the type
	// to satisfy bootstrap.ServerBootstrapper / state.ServerOptionLister
	// at the type level; this test asserts only on the orchestrator
	// shape, never on Run behaviour.
	client := &tmux.Client{}

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore: &bootstrapadapter.RestoreAdapter{Inner: &restore.Orchestrator{}},
	})

	// Type assertion: the EagerSignaler must be the real production-shape
	// adapter, not the NoOp.
	if _, ok := o.EagerSignaler.(*bootstrap.EagerSignalCore); !ok {
		t.Errorf("EagerSignaler type = %T; want *bootstrap.EagerSignalCore (real adapter when RestoreAdapter is real)", o.EagerSignaler)
	}
}

// TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToNoOpWhenRestoreNil
// asserts the unchanged branch: when no RestoreAdapter is provided
// (default-NoOp restore implies no skeleton markers will be set), the
// eager step would be vacuous and the builder retains the NoOp default.
func TestBuildIntegrationOrchestrator_EagerSignalerDefaultsToNoOpWhenRestoreNil(t *testing.T) {
	client := &tmux.Client{}

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		// Restore intentionally unset — exercises the "no skeleton
		// markers, eager step vacuous" branch.
	})

	if _, ok := o.EagerSignaler.(bootstrap.NoOpEagerHydrateSignaler); !ok {
		t.Errorf("EagerSignaler type = %T; want bootstrap.NoOpEagerHydrateSignaler (NoOp when RestoreAdapter is nil)", o.EagerSignaler)
	}
}

// TestBuildIntegrationOrchestrator_EagerSignalerExplicitOptOutHonoured
// asserts the manual-harness opt-out path: tests that drive
// signal-hydrate themselves (e.g. reboot round-trip) pass an explicit
// bootstrap.NoOpEagerHydrateSignaler{} to suppress the new default. The
// builder must honour an explicitly-set EagerSignaler verbatim, even
// when a real RestoreAdapter is also wired.
func TestBuildIntegrationOrchestrator_EagerSignalerExplicitOptOutHonoured(t *testing.T) {
	t.Setenv("PORTAL_STATE_DIR", t.TempDir())
	client := &tmux.Client{}

	o := buildIntegrationOrchestrator(t, client, orchestratorOpts{
		Restore:       &bootstrapadapter.RestoreAdapter{Inner: &restore.Orchestrator{}},
		EagerSignaler: bootstrap.NoOpEagerHydrateSignaler{},
	})

	if _, ok := o.EagerSignaler.(bootstrap.NoOpEagerHydrateSignaler); !ok {
		t.Errorf("EagerSignaler type = %T; want explicit bootstrap.NoOpEagerHydrateSignaler{} (manual-harness opt-out must be honoured)", o.EagerSignaler)
	}
}
