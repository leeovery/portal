package cmd

import (
	"errors"
	"fmt"
	"syscall"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/cmd/bootstrap"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tui"
)

// The zmx a command binds ends a session an older zmx made through
// signalProcess, the one seam cmd's tests fake, should that zmx's kill fail;
// doctor's binds nothing that signals.
func TestZmxConfigSignalsThroughSignalProcess(t *testing.T) {
	var sent []int
	withFuncSeam(t, &signalProcess, func(pid int, sig syscall.Signal) error {
		if sig == syscall.SIGTERM {
			sent = append(sent, pid)
		}
		return nil
	})

	cfg := zmxConfig(zmxProgram, "/srv/zmx")
	if cfg.Signal == nil {
		t.Fatal("zmxConfig binds no Signal: a refused session whose kill fails would be left holding its name")
	}
	if err := cfg.Signal(4242, syscall.SIGTERM); err != nil || len(sent) != 1 || sent[0] != 4242 {
		t.Errorf("Signal(4242) = %v, signalProcess saw %v; want it to reach signalProcess", err, sent)
	}
	if _, err := bindZmxAt(zmxProgram, "/srv/zmx"); err != nil {
		t.Errorf("bindZmxAt = %v", err)
	}
}

// failingCreator fails every mint with err.
type failingCreator struct{ err error }

func (c failingCreator) CreateFromDir(string, []string) (string, error) { return "", c.err }

// The picker cmd builds maps a mint's refusal through createFatal, so the
// refusal is the older-zmx fatal, never a flash and a zero exit.
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
	if !ok || fatal.UserMessage != bootstrap.OlderZmxMessage {
		t.Errorf("FatalError = %v, want the older-zmx fatal", model.(tui.Model).FatalError())
	}
}
