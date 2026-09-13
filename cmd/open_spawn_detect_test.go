package cmd

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui"
)

func TestBuildTUIModel_ThreadsDetectionSeams(t *testing.T) {
	cfg := defaultTestTUIConfig()
	cfg.detector = fakeTerminalDetector{id: spawn.NewIdentity("com.mitchellh.ghostty", "Ghostty")}
	cfg.resolve = spawn.NewResolver(spawn.TerminalsConfig{}).Resolve

	m := buildTUIModel(cfg, pickerLanding{}, nil)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(tui.SessionsMsg{Sessions: []tmux.Session{{Name: "alpha", Windows: 1}}})

	if !model.(tui.Model).DetectDispatched() {
		t.Error("buildTUIModel must thread cfg.detector/cfg.resolve so reaching PageSessions dispatches detection")
	}
}
