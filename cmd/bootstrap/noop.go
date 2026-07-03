package bootstrap

// Canonical no-op implementations exist only for the Orchestrator steps the
// spec permits to degrade-and-continue: Hooks, OrphanSweeper, Saver, Restore,
// EagerHydrateSignaler, MarkerCleaner, and FIFOSweeper. Server and
// RestoringMarker are fatal-on-failure (steps 1, 3, and 8); they
// intentionally have no NoOp because reaching for a "default" would silently
// violate the bootstrap contract.
//
// These exist so tests across packages and production-side fallbacks
// share one source of truth — adding a new degradable step interface to
// the orchestrator only requires extending this file rather than
// re-declaring a sibling private noop in each consumer package.
//
// Each type is a zero-value struct whose methods return the zero/no-error
// result for the interface they satisfy. They are useful in two places:
//
//   - Tests that exercise a subset of the bootstrap sequence and want to
//     stub out steps incidental to the scenario under test.
//   - Production fallbacks (e.g. cmd/bootstrap_production.go uses a NoOp
//     step seam when its dependencies cannot be resolved) where the spec
//     mandates degrade-and-continue rather than aborting bootstrap.

// NoOpHooks satisfies HookRegistrar. RegisterPortalHooks always reports
// success. Useful for tests / production fallbacks.
type NoOpHooks struct{}

// RegisterPortalHooks always returns nil.
func (NoOpHooks) RegisterPortalHooks() error { return nil }

// NoOpOrphanSweeper satisfies OrphanSweeper. SweepOrphanDaemons always
// reports success. Useful for tests / production fallbacks where the
// orphan-sweep step is irrelevant to the scenario under test.
type NoOpOrphanSweeper struct{}

// SweepOrphanDaemons always returns nil.
func (NoOpOrphanSweeper) SweepOrphanDaemons() error { return nil }

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

// NoOpEagerHydrateSignaler satisfies EagerHydrateSignaler. EagerSignalHydrate
// always reports success. The eager-signal step is best-effort and degradable
// per spec, so a no-op is the natural fallback when the production wiring
// cannot resolve dependencies (e.g. the state directory) — matching the NoOp
// policy: "only steps that may degrade get a NoOp."
type NoOpEagerHydrateSignaler struct{}

// EagerSignalHydrate always returns nil.
func (NoOpEagerHydrateSignaler) EagerSignalHydrate() error { return nil }

// NoOpMarkerCleaner satisfies MarkerCleaner. CleanStaleMarkers always
// reports success. Stale-marker cleanup is best-effort and degradable per
// spec, so a no-op is the natural fallback when the production wiring
// cannot resolve dependencies — matching the NoOp policy: "only steps
// that may degrade get a NoOp."
type NoOpMarkerCleaner struct{}

// CleanStaleMarkers always returns nil.
func (NoOpMarkerCleaner) CleanStaleMarkers() error { return nil }

// NoOpFIFOSweeper satisfies FIFOSweeper. Sweep always reports success.
// FIFO sweeping is best-effort and degradable per spec, so a no-op is the
// natural fallback when the production wiring cannot resolve the state
// directory and matches the NoOp policy: "only steps that may degrade get
// a NoOp."
type NoOpFIFOSweeper struct{}

// Sweep always returns nil.
func (NoOpFIFOSweeper) Sweep() error { return nil }
