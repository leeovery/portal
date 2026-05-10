package bootstrap

// NewWithDefaults composes a *Orchestrator with NoOp defaults for every
// degradable step seam. It promotes the previously test-package-only
// builder pattern (cmd/bootstrap/orchestrator_builder_test.go and
// cmd/reattach_integration_test.go) to the bootstrap package itself so
// every consumer — production wiring helpers, cmd/bootstrap_test, and the
// cmd-package integration tests — can share one defaulting policy.
//
// Eliminates the dual-builder tax: prior to this helper, adding a new
// step interface required editing two test files (the cmd/bootstrap
// helper and the cmd-package sibling) because Go test-file symbols are
// not visible across packages. With NewWithDefaults living in a non-test
// file, both consumers import it and a future eleventh step requires
// only a new Option constructor here plus one default wire-up at the
// helper site.
//
// Production keeps direct construction (cmd/bootstrap_production.go)
// because it does not use NoOps — every step seam is a real adapter.
// The helper's defaulting policy is for tests / fallback paths only.
//
// Defaulting policy (mirrors the legacy buildIntegrationOrchestrator
// contract):
//
//   - Hooks, Saver, Restore, StaleMarkers, Sweeper, Clean: default to
//     their canonical NoOp form when the corresponding With* option is
//     not supplied.
//   - EagerSignaler: defaults to a real *EagerSignalCore wired with the
//     helper's positional server / stateDir / logger ONLY when the
//     caller supplied WithRestore with a non-nil Restorer (the T4-2
//     contract — production fidelity for tests that wire real Restore).
//     Otherwise defaults to NoOpEagerHydrateSignaler{}. An explicit
//     WithEagerSignaler(NoOp...) always wins (manual-harness opt-out).
//   - Server, Restoring: positional; mandatory; have no NoOp form because
//     they back fatal-on-failure steps (1, 3, 7) of the bootstrap
//     sequence and silently degrading would violate the contract.
//   - Logger: positional; nil tolerated (Run substitutes its internal
//     noopLogger at entry).

import (
	"github.com/leeovery/portal/internal/state"
)

// ServerSeam is the union interface the helper requires for the
// orchestrator's positional server seam: ServerBootstrapper backs step 1
// (EnsureServer), and state.ServerOptionLister backs the conditional real
// EagerSignalCore default's Markers field. *tmux.Client satisfies both
// directly. Defining the interface in this package avoids importing
// internal/tmux from cmd/bootstrap — tests can construct lightweight
// stubs that satisfy it without pulling tmux into the test binary.
type ServerSeam interface {
	ServerBootstrapper
	state.ServerOptionLister
}

// Option mutates the in-progress orchestrator wiring captured in
// defaultsConfig. Callers compose a NewWithDefaults call from any
// number of With* options; each one toggles a single field.
type Option func(*defaultsConfig)

// defaultsConfig is the internal carrier the helper threads through
// every Option before materialising the *Orchestrator. Pulled into a
// private struct so the public Option type stays a single-argument
// closure and a future field addition does not change Option's
// signature.
type defaultsConfig struct {
	hooks         HookRegistrar
	saver         SaverBootstrapper
	restore       Restorer
	eagerSignaler EagerHydrateSignaler
	staleMarkers  MarkerCleaner
	sweeper       FIFOSweeper
	clean         StaleCleaner

	// restoreSet latches whether the caller invoked WithRestore at all
	// (vs. left it unset). The EagerSignaler default-selection branch
	// keys off the original caller intent, not the post-defaulting
	// field value — without this latch a NoOp Restore (from defaulting)
	// would be indistinguishable from a real Restorer and the eager
	// step would always default to real, losing the "vacuous when no
	// skeletons are armed" guard.
	restoreSet bool

	// eagerSignalerSet latches whether the caller invoked
	// WithEagerSignaler at all (vs. left it unset). An explicit
	// WithEagerSignaler(NoOp...) must win over the conditional real
	// default; tracking the call separately from the field value lets
	// the helper distinguish "caller chose NoOp" from "caller defaulted".
	eagerSignalerSet bool
}

// WithHooks supplies a real HookRegistrar for step 2.
func WithHooks(h HookRegistrar) Option {
	return func(c *defaultsConfig) { c.hooks = h }
}

// WithSaver supplies a real SaverBootstrapper for step 4.
func WithSaver(s SaverBootstrapper) Option {
	return func(c *defaultsConfig) { c.saver = s }
}

// WithRestore supplies a real Restorer for step 5. Setting this also
// flips the EagerSignaler default to a real *EagerSignalCore (T4-2).
func WithRestore(r Restorer) Option {
	return func(c *defaultsConfig) {
		c.restore = r
		c.restoreSet = true
	}
}

// WithEagerSignaler supplies a real EagerHydrateSignaler for step 6.
// Passing bootstrap.NoOpEagerHydrateSignaler{} is the manual-harness
// opt-out: it suppresses the conditional real default that WithRestore
// would otherwise trigger.
func WithEagerSignaler(s EagerHydrateSignaler) Option {
	return func(c *defaultsConfig) {
		c.eagerSignaler = s
		c.eagerSignalerSet = true
	}
}

// WithStaleMarkers supplies a real MarkerCleaner for step 8.
func WithStaleMarkers(m MarkerCleaner) Option {
	return func(c *defaultsConfig) { c.staleMarkers = m }
}

// WithSweeper supplies a real FIFOSweeper for step 9.
func WithSweeper(s FIFOSweeper) Option {
	return func(c *defaultsConfig) { c.sweeper = s }
}

// WithClean supplies a real StaleCleaner for step 10.
func WithClean(s StaleCleaner) Option {
	return func(c *defaultsConfig) { c.clean = s }
}

// NewWithDefaults constructs an *Orchestrator with NoOp defaults for
// every degradable step seam. Mandatory positional inputs:
//
//   - server: backs step 1 (EnsureServer) and the conditional real
//     EagerSignalCore default's Markers field.
//   - stateDir: the resolved Portal state directory used to derive each
//     pane's FIFO path inside the conditional real EagerSignalCore.
//     Tests that do not exercise real eager signaling pass "" — the
//     value is ignored when EagerSignaler defaults to NoOp.
//   - logger: optional; nil tolerated. Propagated verbatim to
//     Orchestrator.Logger and (when applicable) the conditional real
//     EagerSignalCore default's Logger field.
//   - restoring: backs steps 3 (Set) and 7 (Clear) of the bootstrap
//     sequence. Mandatory because both steps are fatal-on-failure and
//     no NoOp form exists.
//
// Variadic With* options override individual seams; see WithHooks,
// WithSaver, WithRestore, WithEagerSignaler, WithStaleMarkers,
// WithSweeper, WithClean.
//
// The returned *Orchestrator never has a nil step seam — Run is callable
// without immediate panic on the happy NoOp path. Step ordering lives in
// Run; this helper only constructs the field set.
func NewWithDefaults(
	server ServerSeam,
	stateDir string,
	logger Logger,
	restoring RestoringMarker,
	opts ...Option,
) *Orchestrator {
	cfg := defaultsConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.hooks == nil {
		cfg.hooks = NoOpHooks{}
	}
	if cfg.saver == nil {
		cfg.saver = NoOpSaver{}
	}
	if cfg.restore == nil {
		cfg.restore = NoOpRestorer{}
	}
	if !cfg.eagerSignalerSet {
		// EagerSignaler default-selection: real EagerSignalCore when
		// WithRestore was supplied (skeleton markers will be set →
		// eager step has work to do), NoOp otherwise (no markers →
		// vacuous).
		if cfg.restoreSet {
			cfg.eagerSignaler = &EagerSignalCore{
				Markers:  server,
				StateDir: stateDir,
				Signaler: state.DefaultFIFOSignaler{},
				Logger:   logger,
			}
		} else {
			cfg.eagerSignaler = NoOpEagerHydrateSignaler{}
		}
	}
	if cfg.staleMarkers == nil {
		cfg.staleMarkers = NoOpMarkerCleaner{}
	}
	if cfg.sweeper == nil {
		cfg.sweeper = NoOpFIFOSweeper{}
	}
	if cfg.clean == nil {
		cfg.clean = NoOpStaleCleaner{}
	}

	return &Orchestrator{
		Server:        server,
		Hooks:         cfg.hooks,
		Restoring:     restoring,
		Saver:         cfg.saver,
		Restore:       cfg.restore,
		EagerSignaler: cfg.eagerSignaler,
		StaleMarkers:  cfg.staleMarkers,
		Sweeper:       cfg.sweeper,
		Clean:         cfg.clean,
		Logger:        logger,
	}
}
