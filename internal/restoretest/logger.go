package restoretest

import (
	"path/filepath"
	"testing"

	"github.com/leeovery/portal/internal/state"
)

// OpenTestLogger opens a *state.Logger writing to <stateDir>/portal.log
// and registers a t.Cleanup that closes it on test completion. This
// consolidates the OpenLogger + Cleanup preamble that was previously
// duplicated across 12 integration-test sites in cmd/bootstrap (via the
// package-private openTestLogger helper) and one sibling site in cmd/
// (cmd/reattach_integration_test.go's buildReattachOrchestrator).
//
// Promotion to internal/restoretest mirrors the precedent set by
// SeedSessionsJSON / SeedSessionsJSONWithSavedAt and WaitForFileExists:
// untagged file, exported helper, single source of truth for both
// default-tagged and integration-tagged callers. The helper itself
// depends only on stdlib + testing + internal/state, so it is safe to
// build under default `go test ./...`; the integration-tagged consumer
// files import it through the same package path.
//
// On OpenLogger failure the helper calls t.Fatalf — production tests
// cannot proceed without a logger when a logger is what the asserted
// adapter (FIFOSweeper, HookRegistrar, RestoreAdapter) requires.
func OpenTestLogger(t *testing.T, stateDir string) *state.Logger {
	t.Helper()
	logger, err := state.OpenLogger(filepath.Join(stateDir, "portal.log"), false)
	if err != nil {
		t.Fatalf("OpenLogger: %v", err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	return logger
}
