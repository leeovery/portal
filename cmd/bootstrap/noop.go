package bootstrap

// Canonical no-op implementations of every Orchestrator step interface.
// These exist so tests across packages and production-side fallbacks
// share one source of truth — adding a new step interface to the
// orchestrator only requires extending this file rather than re-declaring
// a sibling private noop in each consumer package.
//
// Each type is a zero-value struct whose methods return the zero/no-error
// result for the interface they satisfy. They are useful in two places:
//
//   - Tests that exercise a subset of the bootstrap sequence and want to
//     stub out steps incidental to the scenario under test.
//   - Production fallbacks (e.g. cmd/bootstrap_production.go uses
//     NoOpStaleCleaner when hook-store path resolution fails) where the
//     spec mandates degrade-and-continue rather than aborting bootstrap.

// NoOpServer satisfies ServerBootstrapper. EnsureServer reports
// (false, nil) — i.e. server already running, no fatal error. Useful for
// tests / production fallbacks.
type NoOpServer struct{}

// EnsureServer always returns (false, nil).
func (NoOpServer) EnsureServer() (bool, error) { return false, nil }

// NoOpHooks satisfies HookRegistrar. RegisterPortalHooks always reports
// success. Useful for tests / production fallbacks.
type NoOpHooks struct{}

// RegisterPortalHooks always returns nil.
func (NoOpHooks) RegisterPortalHooks() error { return nil }

// NoOpRestoringMarker satisfies RestoringMarker. Both Set and Clear
// always report success. Useful for tests / production fallbacks.
type NoOpRestoringMarker struct{}

// Set always returns nil.
func (NoOpRestoringMarker) Set() error { return nil }

// Clear always returns nil.
func (NoOpRestoringMarker) Clear() error { return nil }

// NoOpSaver satisfies SaverBootstrapper. EnsureSaver always reports
// success. Useful for tests / production fallbacks.
type NoOpSaver struct{}

// EnsureSaver always returns nil.
func (NoOpSaver) EnsureSaver() error { return nil }

// NoOpRestorer satisfies Restorer. Restore always returns (false, nil) —
// the happy path under the (corrupt, err) Restorer contract. Useful for
// tests / production fallbacks.
type NoOpRestorer struct{}

// Restore always returns (false, nil).
func (NoOpRestorer) Restore() (bool, error) { return false, nil }

// NoOpFIFOSweeper satisfies FIFOSweeper. Sweep always reports success.
// FIFO sweeping is best-effort and degradable per spec, so a no-op is the
// natural fallback when the production wiring cannot resolve the state
// directory and matches the NoOp policy: "only steps that may degrade get
// a NoOp."
type NoOpFIFOSweeper struct{}

// Sweep always returns nil.
func (NoOpFIFOSweeper) Sweep() error { return nil }

// NoOpStaleCleaner satisfies StaleCleaner. CleanStale always reports
// success. Useful for tests / production fallbacks.
type NoOpStaleCleaner struct{}

// CleanStale always returns nil.
func (NoOpStaleCleaner) CleanStale() error { return nil }
