//go:build integration

package cmd_test

import (
	"context"
	"testing"

	"github.com/leeovery/portal/cmd"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/bootstrapadapter"
	"github.com/leeovery/portal/internal/restoretest"
	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
)

// tmuxSaver ensures the fixture server's _portal-saver at the binary's own
// version, as the production bootstrap did.
type tmuxSaver struct {
	client   *tmux.Client
	stateDir string
}

func (s tmuxSaver) EnsureSaver() error {
	return tmux.EnsurePortalSaverVersion(s.client, s.stateDir, cmd.Version())
}

// runTmuxBootstrap runs the tmux version's full bootstrap — hooks, saver and
// restore — against the fixture's own server, wired as production wired it
// before the move to zmx took the bootstrap off every command. The tmux saver's
// suites still drive that saver, which no command reaches now. Every pane
// restore arms runs the staged portal binary, not this test binary.
func runTmuxBootstrap(t *testing.T, f symptomFixture) {
	t.Helper()
	client := f.sock.Client()
	logger := restoretest.OpenTestLogger(t, f.stateDir)
	orch := &bootstrap.Orchestrator{
		Server:        client,
		Hooks:         &bootstrapadapter.HookRegistrar{Client: client, Logger: logger, VersionLogger: logger},
		Restoring:     &bootstrapadapter.RestoringMarker{Client: client},
		OrphanSweeper: bootstrapadapter.NewOrphanSweeper(client, logger),
		Saver:         tmuxSaver{client: client, stateDir: f.stateDir},
		Restore:       restoretest.StagedRestoreAdapter(t, client, f.stateDir, logger, f.binDir),
		EagerSignaler: &bootstrap.EagerSignalCore{
			Markers:  client,
			StateDir: f.stateDir,
			Signaler: state.DefaultFIFOSignaler{},
			Logger:   logger,
		},
		StaleMarkers: &bootstrap.MarkerCleanupCore{
			Markers:  client,
			Panes:    client,
			Unsetter: client,
			Logger:   logger,
		},
		Sweeper: &bootstrapadapter.FIFOSweeper{Client: client, StateDir: f.stateDir, Logger: logger},
		Latch:   client,
		Version: cmd.Version(),
		Logger:  logger,
	}
	if _, _, err := orch.Run(context.Background()); err != nil {
		t.Fatalf("tmux bootstrap: %v\n%s", err, f.diagnostic())
	}
}
