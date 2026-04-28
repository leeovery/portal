package cmd

// resetBootstrapOnce zeroes the package-level memoisation gate used by
// PersistentPreRunE. Tests that drive PersistentPreRunE MUST call this at
// the top of their bodies; otherwise the gate carried over from a prior
// test will short-circuit the orchestrator call and surface as a flaky
// "Run was not called / wrong return value" assertion.
//
// Mirrors the pattern in cmd/version_guard.go (resetVersionCheckForTest):
// reset the gate up-front so the test sees a fresh state, AND register a
// cleanup that resets it again so the next test starts clean. The cmd
// package forbids t.Parallel() per CLAUDE.md precisely because of these
// shared mutable seams.

import (
	"sync"
	"testing"
)

func resetBootstrapOnce(t *testing.T) {
	t.Helper()
	bootstrapOnce = sync.Once{}
	bootstrapStarted = false
	bootstrapErr = nil
	t.Cleanup(func() {
		bootstrapOnce = sync.Once{}
		bootstrapStarted = false
		bootstrapErr = nil
	})
}
