package cmd

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

// failingCreator fails every mint with err.
type failingCreator struct{ err error }

func (c failingCreator) CreateFromDir(string, []string) (string, error) { return "", c.err }

// The picker cmd builds maps a mint's refusal through createFatal, so the
// refusal is the unsupported-create fatal, never a flash and a zero exit.
func TestBuildTUIModelMapsAMintRefusalToTheFatal(t *testing.T) {
	cfg := defaultTestTUIConfig()
	cfg.sessionCreator = failingCreator{err: fmt.Errorf("create session %q: %w", "alpha-x", backend.ErrUnsupportedProtocol)}
	var model tea.Model = buildTUIModel(cfg, pickerLanding{}, nil)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: []project.Project{{Name: "alpha", Path: "/srv/alpha"}}})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
		t.Fatalf("ActivePage() = %d, want PageProjects", page)
	}

	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msgs := drainCmd(t, cmd)
	if len(msgs) == 0 {
		t.Fatal("Enter issued no mint")
	}
	for _, msg := range msgs {
		model, _ = model.Update(msg)
	}

	fatal, ok := errors.AsType[*bootstrap.FatalError](model.(tui.Model).FatalError())
	if !ok || fatal.UserMessage != bootstrap.UnsupportedCreateMessage {
		t.Errorf("FatalError = %v, want the unsupported-create fatal", model.(tui.Model).FatalError())
	}
}
