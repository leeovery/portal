package tui_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/backend"
	"github.com/leeovery/portal/internal/tui"
)

// userMessage is an error that is its user-facing text, as cmd's fatals are.
type userMessage string

func (m userMessage) Error() string { return string(m) }

// errUnsupportedFatal stands in for the fatal cmd maps a refused create to.
var errUnsupportedFatal error = userMessage("Portal can't use this zmx: the sessions it makes speak a protocol Portal doesn't")

// unsupportedFatal is the CreateFatal seam as cmd wires it: a create the backend
// refused as unsupported is the fatal, and nothing else is.
func unsupportedFatal(err error) error {
	if errors.Is(err, backend.ErrUnsupportedProtocol) {
		return errUnsupportedFatal
	}
	return nil
}

var errRefusedCreate = fmt.Errorf("create session %q: %w", "alpha-x", backend.ErrUnsupportedProtocol)

// warmProjectsModel is a picker past its bootstrap on the Projects page, its
// mints failing with createErr.
func warmProjectsModel(t *testing.T, createErr error) tea.Model {
	t.Helper()
	deps := tui.Deps{
		Lister:      &mockSessionLister{sessions: []tui.Session{}},
		Creator:     &mockSessionCreator{err: createErr},
		CreateFatal: unsupportedFatal,
		CWD:         "/tmp/cwd",
	}
	var model tea.Model = tui.Build(deps)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
	model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
		t.Fatalf("ActivePage() = %d, want PageProjects", page)
	}
	return model
}

// mint presses Enter on the first project and delivers the mint's answer.
func mint(t *testing.T, model tea.Model) (tea.Model, tea.Cmd) {
	t.Helper()
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg := runCmd(cmd)
	if msg == nil {
		t.Fatal("Enter issued no mint")
	}
	return model.Update(msg)
}

func visible(model tea.Model) string { return ansi.Strip(model.(tui.Model).View().Content) }

// decision 75: a mint a zmx Portal can't talk to refused ends the run on the
// error frame, as a bootstrap's fatal does; any other failure is flashed.
func TestMintFailure(t *testing.T) {
	// No loading step ran in a warm picker's process, so none is shown.
	t.Run("a refused create in a warm picker shows the fatal alone, and ends the run with it", func(t *testing.T) {
		model, _ := mint(t, warmProjectsModel(t, errRefusedCreate))

		got := model.(tui.Model)
		if got.ActivePage() != tui.PageLoading || !errors.Is(got.FatalError(), errUnsupportedFatal) {
			t.Fatalf("page %d, FatalError %v; want the error frame carrying the fatal", got.ActivePage(), got.FatalError())
		}
		assertFatalAlone(t, model)
		if _, cmd := model.Update(tea.KeyPressMsg{Code: 'q', Text: "q"}); runCmd(cmd) == nil {
			t.Error("q on the error frame issued nothing, want the quit")
		}
		if _, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter}); runCmd(cmd) != nil {
			t.Error("Enter on the error frame minted again")
		}
	})

	t.Run("a refused create in a picker whose loading page ran keeps its steps", func(t *testing.T) {
		deps := tui.Deps{
			Lister:           &mockSessionLister{sessions: []tui.Session{}},
			Creator:          &mockSessionCreator{err: errRefusedCreate},
			CreateFatal:      unsupportedFatal,
			CWD:              "/tmp/cwd",
			ProgressReceiver: tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} }),
		}
		var model tea.Model = tui.Build(deps)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		for step := 1; step <= 6; step++ {
			model, _ = model.Update(tui.BootstrapProgressMsg{Index: step})
		}
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
			t.Fatalf("ActivePage() = %d, want PageProjects", page)
		}

		model, _ = mint(t, model)

		view := visible(model)
		for _, want := range []string{errUnsupportedFatal.Error(), "✓   " + tui.LabelCheckingZmx, "✓   " + tui.LabelStartingTheSaver, "✓   " + tui.LabelRestoringSessions} {
			if !strings.Contains(view, want) {
				t.Errorf("frame lacks %q:\n%s", want, view)
			}
		}
	})

	// A cold picker that waited out another Portal's bootstrap and owed none
	// of its own ran no step either.
	t.Run("a refused create in a picker whose loading page only waited shows the fatal alone", func(t *testing.T) {
		deps := tui.Deps{
			Lister:           &mockSessionLister{sessions: []tui.Session{}},
			Creator:          &mockSessionCreator{err: errRefusedCreate},
			CreateFatal:      unsupportedFatal,
			CWD:              "/tmp/cwd",
			ProgressReceiver: tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Waiting: true} }),
		}
		var model tea.Model = tui.Build(deps)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		model, _ = model.Update(tui.BootstrapProgressMsg{Waiting: true})
		model, _ = model.Update(tui.LoadingMinElapsedMsg{})
		model, _ = model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
		if page := model.(tui.Model).ActivePage(); page != tui.PageProjects {
			t.Fatalf("ActivePage() = %d, want PageProjects", page)
		}

		model, _ = mint(t, model)

		assertFatalAlone(t, model)
	})

	t.Run("any other failure flashes, and the picker stays open", func(t *testing.T) {
		model, cmd := mint(t, warmProjectsModel(t, errors.New("boom")))

		got := model.(tui.Model)
		if got.FatalError() != nil || got.ActivePage() != tui.PageProjects || cmd != nil {
			t.Fatalf("FatalError %v, page %d, cmd %v; want no fatal, the Projects page, nothing issued", got.FatalError(), got.ActivePage(), cmd)
		}
		if want := "Portal couldn't start that session: boom"; !strings.Contains(visible(model), want) {
			t.Errorf("view lacks the flash %q:\n%s", want, visible(model))
		}
	})

	t.Run("with no fatal mapped, a refused create flashes", func(t *testing.T) {
		deps := tui.Deps{Lister: &mockSessionLister{sessions: []tui.Session{}}, Creator: &mockSessionCreator{err: errRefusedCreate}}
		var model tea.Model = tui.Build(deps)
		model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
		model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
		model, _ = model.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})

		model, _ = mint(t, model)

		if got := model.(tui.Model).FatalError(); got != nil {
			t.Errorf("FatalError = %v, want none with no mapping", got)
		}
		if want := "Portal couldn't start that session: "; !strings.Contains(visible(model), want) {
			t.Errorf("view lacks the flash %q:\n%s", want, visible(model))
		}
	})

	t.Run("a staged mint refused ends the command-pending run with the fatal", func(t *testing.T) {
		creator := &mockSessionCreator{err: errRefusedCreate}
		model := stagedFatalModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		model, cmd := model.Update(tui.BootstrapCompleteMsg{})
		model, cmd = model.Update(runCmd(cmd))

		if got := model.(tui.Model).FatalError(); !errors.Is(got, errUnsupportedFatal) {
			t.Errorf("FatalError = %v, want the fatal", got)
		}
		if _, ok := runCmd(cmd).(tea.QuitMsg); !ok {
			t.Errorf("the staged mint's refusal issued %T, want the quit: a command-pending picker has no error frame", runCmd(cmd))
		}
	})

	t.Run("a staged mint failing otherwise flashes", func(t *testing.T) {
		creator := &mockSessionCreator{err: errors.New("boom")}
		model := stagedFatalModel(t, creator)
		model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

		model, cmd := model.Update(tui.BootstrapCompleteMsg{})
		model, _ = model.Update(runCmd(cmd))

		if got := model.(tui.Model).FatalError(); got != nil {
			t.Errorf("FatalError = %v, want none", got)
		}
		if want := "Portal couldn't start that session: boom"; !strings.Contains(visible(model), want) {
			t.Errorf("view lacks the flash %q:\n%s", want, visible(model))
		}
	})
}

// stagedFatalModel is the cold command-pending picker, its mints answered by
// creator and mapped by unsupportedFatal.
func stagedFatalModel(t *testing.T, creator *mockSessionCreator) tea.Model {
	t.Helper()
	deps := commandPendingDeps(tea.Cmd(func() tea.Msg { return tui.BootstrapProgressMsg{Index: 1} }))
	deps.Creator = creator
	deps.CreateFatal = unsupportedFatal
	deps.CWD = "/tmp/cwd"
	var model tea.Model = tui.Build(deps)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(tui.ProjectsLoadedMsg{Projects: stagedMintProjects})
	return model
}

// assertFatalAlone holds model's frame to the wordmark over the fatal and the
// quit hint: no progress bar and no step list.
func assertFatalAlone(t *testing.T, model tea.Model) {
	t.Helper()
	var rows []string
	for line := range strings.SplitSeq(visible(model), "\n") {
		if row := strings.TrimSpace(line); row != "" {
			rows = append(rows, row)
		}
	}
	if len(rows) < 3 {
		t.Fatalf("frame rows = %q, want the wordmark, the fatal and the quit hint", rows)
	}
	wordmark, footer := rows[:len(rows)-2], rows[len(rows)-2:]
	if footer[0] != errUnsupportedFatal.Error() || !strings.HasPrefix(footer[1], "q quit") {
		t.Errorf("frame ends %q, want the fatal, then the quit hint", footer)
	}
	if len(wordmark) != 5 {
		t.Errorf("frame draws %d rows above the fatal, want the wordmark's 5 alone:\n%s", len(wordmark), strings.Join(rows, "\n"))
	}
}
