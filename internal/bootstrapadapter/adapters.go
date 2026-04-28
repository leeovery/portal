// Package bootstrapadapter holds the production-shape adapters that wire
// tmux-client-level primitives to the bootstrap.Orchestrator step
// interfaces. Each adapter is a thin wrapper:
//
//   - Method-name shaping: bootstrap.RestoringMarker requires Set/Clear;
//     *tmux.Client exposes Set/UnsetServerOption with a name argument.
//   - Argument capture: the long-lived *tmux.Client is held on the adapter
//     so the orchestrator's step interfaces stay argument-free.
//
// The adapters live in their own package (rather than under cmd/) so test
// suites that need production-equivalent wiring can import them without
// pulling in the rest of cmd/. Production-only adapters that carry richer
// context (state dir, hooks store, restore orchestrator, logger) stay in
// cmd/bootstrap_production.go — they are not reusable from tests in their
// current shape.
package bootstrapadapter

import (
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// RestoringMarker manages the @portal-restoring server-option lifecycle
// that suppresses the save daemon during skeleton restore. Implements
// bootstrap.RestoringMarker — steps 3 (Set) and 6 (Clear) of the
// bootstrap sequence.
//
// The wrapped Client must be non-nil; behaviour with a nil client is
// undefined and will panic at the first method call (matching tmux.Client
// semantics elsewhere in the codebase).
type RestoringMarker struct {
	Client *tmux.Client
}

// Set writes @portal-restoring="1" at server scope. The option name comes
// from state.RestoringMarkerName so this adapter cannot drift from the
// canonical constant.
func (m *RestoringMarker) Set() error {
	return m.Client.SetServerOption(state.RestoringMarkerName, "1")
}

// Clear removes @portal-restoring at server scope. Idempotent under tmux
// — unsetting an already-absent option is a no-op and does not error.
func (m *RestoringMarker) Clear() error {
	return m.Client.UnsetServerOption(state.RestoringMarkerName)
}

// HookRegistrar wraps tmux.RegisterPortalHooks to satisfy
// bootstrap.HookRegistrar. Step 2 of the bootstrap sequence; idempotent
// — safe to invoke on every bootstrap.
type HookRegistrar struct {
	Client *tmux.Client
}

// RegisterPortalHooks delegates to the package-level helper on the
// wrapped client.
func (r *HookRegistrar) RegisterPortalHooks() error {
	return tmux.RegisterPortalHooks(r.Client)
}
