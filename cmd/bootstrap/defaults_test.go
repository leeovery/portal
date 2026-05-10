package bootstrap_test

// Unit tests for the public NewWithDefaults helper (defaults.go). The helper
// promotes the previously test-package-only buildIntegrationOrchestrator
// pattern to a non-test location so both cmd/bootstrap_test and the cmd
// package can share one defaulting policy — eliminating the dual-builder tax
// that required every new step interface to be wired in two test files.
//
// Test scope:
//
//   - Defaulting policy: every degradable step seam (Hooks, Saver, Restore,
//     EagerSignaler, StaleMarkers, Sweeper, Clean) defaults to its NoOp form
//     when the corresponding With* option is not supplied.
//   - Option pass-through: each With* option sets the corresponding seam
//     verbatim.
//   - EagerSignaler conditional default (T4-2 contract): when WithRestore
//     is supplied with a non-nil Restorer, EagerSignaler defaults to a real
//     *EagerSignalCore wired with the helper's positional client / stateDir
//     / logger. Otherwise it defaults to NoOp. An explicit
//     WithEagerSignaler(NoOp...) is honoured even when WithRestore is real
//     (manual-harness opt-out).
//   - Mandatory positional seams (Server, Restoring) are wired through to
//     the orchestrator unchanged; Logger nil is tolerated and preserved.
//   - Smoke test: a minimal NewWithDefaults orchestrator is Run-callable
//     without immediate panic on the happy NoOp path.

import (
	"context"
	"testing"

	"github.com/leeovery/portal/cmd/bootstrap"
)

// stubServerSeam satisfies bootstrap.ServerSeam (ServerBootstrapper +
// state.ServerOptionLister) for tests that never invoke either method —
// they assert only on orchestrator shape post-construction. The smoke
// test that drives Run uses successServerSeam below.
type stubServerSeam struct{}

func (stubServerSeam) EnsureServer() (bool, error)           { return false, nil }
func (stubServerSeam) ShowAllServerOptions() (string, error) { return "", nil }

// stubRestoringMarker satisfies bootstrap.RestoringMarker. The smoke test
// drives Set / Clear via the orchestrator's Run method; shape-only tests
// never invoke either.
type stubRestoringMarker struct{}

func (stubRestoringMarker) Set() error   { return nil }
func (stubRestoringMarker) Clear() error { return nil }

// stubRestorer satisfies bootstrap.Restorer with the happy-path return.
// Used to exercise the WithRestore option's effect on the EagerSignaler
// default-selection branch.
type stubRestorer struct{}

func (stubRestorer) Restore() (bool, error) { return false, nil }

// TestNewWithDefaults_DefaultsAllDegradableStepsToNoOp asserts that
// supplying only the mandatory positional seams (server, stateDir,
// logger, restoring) produces an Orchestrator whose every degradable
// step is the canonical NoOp form. This is the baseline-defaulting
// contract: callers opt INTO real adapters via With* options.
func TestNewWithDefaults_DefaultsAllDegradableStepsToNoOp(t *testing.T) {
	o := bootstrap.NewWithDefaults(stubServerSeam{}, "", nil, stubRestoringMarker{})

	if _, ok := o.Hooks.(bootstrap.NoOpHooks); !ok {
		t.Errorf("Hooks type = %T; want bootstrap.NoOpHooks", o.Hooks)
	}
	if _, ok := o.Saver.(bootstrap.NoOpSaver); !ok {
		t.Errorf("Saver type = %T; want bootstrap.NoOpSaver", o.Saver)
	}
	if _, ok := o.Restore.(bootstrap.NoOpRestorer); !ok {
		t.Errorf("Restore type = %T; want bootstrap.NoOpRestorer", o.Restore)
	}
	if _, ok := o.EagerSignaler.(bootstrap.NoOpEagerHydrateSignaler); !ok {
		t.Errorf("EagerSignaler type = %T; want bootstrap.NoOpEagerHydrateSignaler (Restore is NoOp → eager step is vacuous)", o.EagerSignaler)
	}
	if _, ok := o.StaleMarkers.(bootstrap.NoOpMarkerCleaner); !ok {
		t.Errorf("StaleMarkers type = %T; want bootstrap.NoOpMarkerCleaner", o.StaleMarkers)
	}
	if _, ok := o.Sweeper.(bootstrap.NoOpFIFOSweeper); !ok {
		t.Errorf("Sweeper type = %T; want bootstrap.NoOpFIFOSweeper", o.Sweeper)
	}
	if _, ok := o.Clean.(bootstrap.NoOpStaleCleaner); !ok {
		t.Errorf("Clean type = %T; want bootstrap.NoOpStaleCleaner", o.Clean)
	}
}

// TestNewWithDefaults_WiresPositionalSeams asserts that the four
// mandatory positional inputs (server, stateDir, logger, restoring) are
// preserved verbatim onto the constructed Orchestrator. server fans out
// to Orchestrator.Server (step 1); restoring fans out to Orchestrator.
// Restoring (steps 3 and 7); logger is propagated as-is (nil tolerated).
func TestNewWithDefaults_WiresPositionalSeams(t *testing.T) {
	server := stubServerSeam{}
	restoring := stubRestoringMarker{}

	o := bootstrap.NewWithDefaults(server, "", nil, restoring)

	if o.Server != server {
		t.Errorf("Server = %v; want stubServerSeam{}", o.Server)
	}
	if o.Restoring != restoring {
		t.Errorf("Restoring = %v; want stubRestoringMarker{}", o.Restoring)
	}
	if o.Logger != nil {
		t.Errorf("Logger = %v; want nil (preserved verbatim)", o.Logger)
	}
}

// TestNewWithDefaults_HonorsAllWithOptions asserts that every With*
// option overrides its NoOp default with the supplied seam. Iterates
// every option constructor in one place so a future option that fails
// to wire its target field surfaces as a single test failure rather
// than slipping through with the existing default.
func TestNewWithDefaults_HonorsAllWithOptions(t *testing.T) {
	hooks := stubHooks{}
	saver := stubSaver{}
	restore := stubRestorer{}
	eager := stubEager{}
	staleMarkers := stubMarkerCleaner{}
	sweeper := stubSweeper{}
	clean := stubStaleCleaner{}

	o := bootstrap.NewWithDefaults(stubServerSeam{}, "", nil, stubRestoringMarker{},
		bootstrap.WithHooks(hooks),
		bootstrap.WithSaver(saver),
		bootstrap.WithRestore(restore),
		bootstrap.WithEagerSignaler(eager),
		bootstrap.WithStaleMarkers(staleMarkers),
		bootstrap.WithSweeper(sweeper),
		bootstrap.WithClean(clean),
	)

	if o.Hooks != hooks {
		t.Errorf("Hooks = %v; want stubHooks{}", o.Hooks)
	}
	if o.Saver != saver {
		t.Errorf("Saver = %v; want stubSaver{}", o.Saver)
	}
	if o.Restore != restore {
		t.Errorf("Restore = %v; want stubRestorer{}", o.Restore)
	}
	if o.EagerSignaler != eager {
		t.Errorf("EagerSignaler = %v; want stubEager{}", o.EagerSignaler)
	}
	if o.StaleMarkers != staleMarkers {
		t.Errorf("StaleMarkers = %v; want stubMarkerCleaner{}", o.StaleMarkers)
	}
	if o.Sweeper != sweeper {
		t.Errorf("Sweeper = %v; want stubSweeper{}", o.Sweeper)
	}
	if o.Clean != clean {
		t.Errorf("Clean = %v; want stubStaleCleaner{}", o.Clean)
	}
}

// TestNewWithDefaults_EagerSignalerDefaultsToRealWhenRestoreReal mirrors
// the buildIntegrationOrchestrator T4-2 contract test (orchestrator_
// builder_eager_default_test.go). Supplying WithRestore with a non-nil
// Restorer and leaving EagerSignaler unset must produce a real
// *bootstrap.EagerSignalCore — not the NoOp form. This is the rule the
// helper inherits from the legacy builder so production-fidelity is
// preserved across both consumers.
func TestNewWithDefaults_EagerSignalerDefaultsToRealWhenRestoreReal(t *testing.T) {
	o := bootstrap.NewWithDefaults(stubServerSeam{}, t.TempDir(), nil, stubRestoringMarker{},
		bootstrap.WithRestore(stubRestorer{}),
	)

	core, ok := o.EagerSignaler.(*bootstrap.EagerSignalCore)
	if !ok {
		t.Fatalf("EagerSignaler type = %T; want *bootstrap.EagerSignalCore (real adapter when WithRestore is supplied)", o.EagerSignaler)
	}
	// Spot-check the seam wiring so a future regression that ignores
	// the helper's positional inputs surfaces here rather than as a
	// runtime nil-deref deep inside EagerSignalHydrate.
	if core.Markers == nil {
		t.Errorf("EagerSignalCore.Markers is nil; want the helper's server seam threaded through")
	}
	if core.Signaler == nil {
		t.Errorf("EagerSignalCore.Signaler is nil; want state.DefaultFIFOSignaler{}")
	}
}

// TestNewWithDefaults_EagerSignalerDefaultsToNoOpWhenRestoreUnset asserts
// the unchanged branch: when WithRestore is not supplied (Restore
// defaults to NoOpRestorer → no skeleton markers will be set), the
// eager step would be vacuous and the helper retains the NoOp default.
// Mirrors the corresponding T4-2 unit test for buildIntegrationOrchestrator.
func TestNewWithDefaults_EagerSignalerDefaultsToNoOpWhenRestoreUnset(t *testing.T) {
	o := bootstrap.NewWithDefaults(stubServerSeam{}, "", nil, stubRestoringMarker{})

	if _, ok := o.EagerSignaler.(bootstrap.NoOpEagerHydrateSignaler); !ok {
		t.Errorf("EagerSignaler type = %T; want bootstrap.NoOpEagerHydrateSignaler (Restore is NoOp → eager step vacuous)", o.EagerSignaler)
	}
}

// TestNewWithDefaults_EagerSignalerExplicitOptOutHonored asserts the
// manual-harness opt-out path: tests that drive signal-hydrate
// themselves (e.g. reboot round-trip) pass an explicit
// bootstrap.NoOpEagerHydrateSignaler{} via WithEagerSignaler to suppress
// the conditional real default. The helper must honour an explicitly-
// provided EagerSignaler verbatim, even when WithRestore is also
// supplied with a real Restorer.
func TestNewWithDefaults_EagerSignalerExplicitOptOutHonored(t *testing.T) {
	o := bootstrap.NewWithDefaults(stubServerSeam{}, t.TempDir(), nil, stubRestoringMarker{},
		bootstrap.WithRestore(stubRestorer{}),
		bootstrap.WithEagerSignaler(bootstrap.NoOpEagerHydrateSignaler{}),
	)

	if _, ok := o.EagerSignaler.(bootstrap.NoOpEagerHydrateSignaler); !ok {
		t.Errorf("EagerSignaler type = %T; want bootstrap.NoOpEagerHydrateSignaler (explicit opt-out must win over WithRestore)", o.EagerSignaler)
	}
}

// TestNewWithDefaults_RunCallableSmokeTest is the spec's smoke test:
// "bootstrap.NewWithDefaults(server, restoring) returns an Orchestrator
// with non-nil seams whose Run() is callable without immediate panic."
// We supply only the mandatory positional inputs and assert Run completes
// without error or panic — proving every step seam was defaulted to a
// non-nil value (a nil seam would crash on method dispatch inside Run).
func TestNewWithDefaults_RunCallableSmokeTest(t *testing.T) {
	o := bootstrap.NewWithDefaults(stubServerSeam{}, "", nil, stubRestoringMarker{})

	_, _, err := o.Run(context.Background())
	if err != nil {
		t.Errorf("Run returned error: %v; want nil (every default NoOp step succeeds)", err)
	}
}

// stub seam types used by TestNewWithDefaults_HonorsAllWithOptions. Each
// satisfies the corresponding bootstrap.* interface with zero-effort
// happy-path returns; the assertion target is identity equality on the
// orchestrator's field, not behaviour.

type stubHooks struct{}

func (stubHooks) RegisterPortalHooks() error { return nil }

type stubSaver struct{}

func (stubSaver) EnsureSaver() error { return nil }

type stubEager struct{}

func (stubEager) EagerSignalHydrate() error { return nil }

type stubMarkerCleaner struct{}

func (stubMarkerCleaner) CleanStaleMarkers() error { return nil }

type stubSweeper struct{}

func (stubSweeper) Sweep() error { return nil }

type stubStaleCleaner struct{}

func (stubStaleCleaner) CleanStale() error { return nil }
