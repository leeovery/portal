package restoretest

import (
	"log/slog"
	"testing"

	"github.com/leeovery/portal/internal/restore"
	"github.com/leeovery/portal/internal/tmux"
)

// FakeHydrateExePath is what an orchestrator built by NewFakeExeOrchestrator
// arms its panes with. It is absolute and names nothing on any machine on
// purpose: a pane armed with it dies visibly, rather than taking the
// os.Executable() fallback StagedHydrateExe documents.
const FakeHydrateExePath = "/portal-test-no-such-binary/portal"

// NewFakeExeOrchestrator builds a restore.Orchestrator for a test that never
// hands a restored pane to a real binary: one driven through a mock commander,
// or one whose restore fails before it arms anything. Everything else about it
// is the caller's — fields the caller does care about (Progress) are assigned
// after construction.
//
// It exists so that no test composes the struct itself. Exe is opt-in on the
// struct and a forgotten one is silent, so the only way to keep it pinned
// everywhere is to leave no bare literal anywhere; a source guard over
// *_test.go enforces that, and this is the route for the cases with no staged
// binary to point at. A case that has one takes NewRestoreOrchestrator, or
// StagedRestoreAdapter where the restore is reached through a bootstrap.
func NewFakeExeOrchestrator(t *testing.T, client *tmux.Client, stateDir string, logger *slog.Logger) *restore.Orchestrator {
	t.Helper()
	return &restore.Orchestrator{
		Client:   client,
		StateDir: stateDir,
		Logger:   logger,
		Exe:      func() (string, error) { return FakeHydrateExePath, nil },
	}
}
