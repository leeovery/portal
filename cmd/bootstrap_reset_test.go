package cmd

// Tests driving PersistentPreRunE must call this first: a gate carried over
// from a prior test short-circuits the orchestrator call and surfaces as a
// flaky "Run was not called" assertion.

import (
	"sync"
	"testing"
)

func resetBootstrapOnce(t *testing.T) {
	t.Helper()
	bootstrapOnce = sync.Once{}
	bootstrapStarted = false
	bootstrapWarningsSlice = nil
	bootstrapErr = nil
	bootstrapWarnings = &BootstrapWarningsSink{}
	t.Cleanup(func() {
		bootstrapOnce = sync.Once{}
		bootstrapStarted = false
		bootstrapWarningsSlice = nil
		bootstrapErr = nil
		bootstrapWarnings = &BootstrapWarningsSink{}
	})
}

// The sink is package-global and other cmd tests write to it, so clear it on
// both sides of the body.
func resetBootstrapWarnings(t *testing.T) {
	t.Helper()
	bootstrapWarnings.Drain()
	t.Cleanup(func() { bootstrapWarnings.Drain() })
}
