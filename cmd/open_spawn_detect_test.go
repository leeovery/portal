package cmd

// restore-host-terminal-windows-6-1 — buildTUIModel detection-seam wiring.
//
// Tests here mutate no package-level state but follow the cmd-package convention:
// no t.Parallel.

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

// TestBuildTUIModel_ThreadsDetectionSeams asserts buildTUIModel threads the
// cfg.detector + cfg.resolve seams field-for-field into the tui.Model: reaching
// PageSessions dispatches the async host-terminal detection, which is only
// possible if both seams were wired end to end (tuiConfig → tui.Deps → model).
func TestBuildTUIModel_ThreadsDetectionSeams(t *testing.T) {
	cfg := defaultTestTUIConfig()
	cfg.detector = fakeTerminalDetector{id: spawn.NewIdentity("com.mitchellh.ghostty", "Ghostty")}
	cfg.resolve = spawn.NewResolver(spawn.TerminalsConfig{}).Resolve

	m := buildTUIModel(cfg, "", nil)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{{Name: "alpha", Windows: 1}}})

	if !model.(tui.Model).DetectDispatched() {
		t.Error("buildTUIModel must thread cfg.detector/cfg.resolve so reaching PageSessions dispatches detection")
	}
}
