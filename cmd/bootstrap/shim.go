package bootstrap

import "context"

// Runner is the abstraction cmd/root.go depends on so PersistentPreRunE
// does not import the concrete *Orchestrator type. Orchestrator implicitly
// satisfies Runner; tests inject lightweight fakes (e.g. NewShim) without
// pulling in the full step interfaces.
type Runner interface {
	Run(ctx context.Context) (bool, error)
}

// shimRunner is a Runner that only performs step 1 (EnsureServer) of the
// canonical eight-step bootstrap sequence. It exists as a transitional
// adapter so cmd-package tests written against the legacy
// BootstrapDeps.Bootstrapper seam continue to compile and pass through the
// Phase 5 cutover.
//
// TODO(phase-6): delete shimRunner and NewShim once every cmd-package test
// migrates to the full Orchestrator seam (BootstrapDeps.Orchestrator).
type shimRunner struct {
	server ServerBootstrapper
}

// Run delegates to ServerBootstrapper.EnsureServer. The returned values are
// passed through verbatim — no wrapping, no additional steps. A nil
// ServerBootstrapper yields a no-op Run that returns (false, nil).
func (s *shimRunner) Run(_ context.Context) (bool, error) {
	if s.server == nil {
		return false, nil
	}
	return s.server.EnsureServer()
}

// NewShim returns a Runner that wraps the given ServerBootstrapper. The
// Runner's Run only invokes EnsureServer — it does NOT register hooks, set
// the @portal-restoring marker, ensure the saver, restore sessions, or run
// CleanStale. It is a legacy compatibility seam for tests that pre-date
// the full Orchestrator wiring.
//
// Deprecated: scheduled for removal in Phase 6 once every cmd-package test
// migrates to the full Orchestrator seam.
func NewShim(b ServerBootstrapper) Runner {
	return &shimRunner{server: b}
}
