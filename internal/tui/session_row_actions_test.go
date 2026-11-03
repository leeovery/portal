package tui

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// rowActionLister answers each listing with sessions, or with err once set.
type rowActionLister struct {
	sessions []Session
	err      error
}

func (l *rowActionLister) ListSessions() ([]Session, error) { return l.sessions, l.err }

// rowActionModel is a picker showing sessions, with killer and labeller wired.
func rowActionModel(t *testing.T, lister *rowActionLister, killer SessionKiller, labeller SessionLabeller) Model {
	t.Helper()
	m := New(lister, WithKiller(killer), WithLabeller(labeller))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	updated, _ = updated.Update(SessionsMsg{Sessions: lister.sessions})
	return updated.(Model)
}

// pressKeys sends each key in turn, returning the model and the last key's command.
func pressKeys(m Model, keys ...tea.KeyPressMsg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	for _, key := range keys {
		var updated tea.Model
		updated, cmd = m.Update(key)
		m = updated.(Model)
	}
	return m, cmd
}

// land runs cmd, as the program would, and delivers what it answers.
func land(t *testing.T, m Model, cmd tea.Cmd) (Model, tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatal("want a command to land, got none")
	}
	updated, next := m.Update(cmd())
	return updated.(Model), next
}

// quits reports whether cmd is tea.Quit. A flash's tick blocks for its whole
// duration when run, so this reads the command rather than running it.
func quits(cmd tea.Cmd) bool {
	return cmd != nil && reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Quit).Pointer()
}

// assertFlashedAndOpen holds m to showing want in its flash, still open on the
// sessions page with rows.
func assertFlashedAndOpen(t *testing.T, m Model, next tea.Cmd, want string, rows []string) {
	t.Helper()
	if quits(next) {
		t.Error("the picker quit")
	}
	if m.modal != modalNone {
		t.Errorf("modal = %v, want none", m.modal)
	}
	if m.flashText != want {
		t.Errorf("flash = %q, want %q", m.flashText, want)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, want) {
		t.Errorf("the frame does not show %q:\n%s", want, view)
	}
	if got := visibleSessionNames(m); !slices.Equal(got, rows) {
		t.Errorf("rows = %v, want %v", got, rows)
	}
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want nothing to attach", m.Selected())
	}
}

func TestRowActions_FailuresFlashAndKeepThePickerOpen(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}

	t.Run("a failed kill flashes its words and still refreshes the rows", func(t *testing.T) {
		lister := &rowActionLister{sessions: sessions}
		killer := &killerStub{err: errors.New("No session found: alpha")}
		m := rowActionModel(t, lister, killer, &recordingLabeller{})

		m, cmd := pressKeys(m, press('k'), press('y'))
		lister.sessions = []Session{{Name: "bravo"}}
		m, next := land(t, m, cmd)

		assertFlashedAndOpen(t, m, next, "No session found: alpha", []string{"bravo"})
	})

	t.Run("a failed kill whose refresh fails too keeps its own flash and the last rows", func(t *testing.T) {
		lister := &rowActionLister{sessions: sessions}
		killer := &killerStub{err: errors.New("session alpha is not answering; the kill may still land")}
		m := rowActionModel(t, lister, killer, &recordingLabeller{})

		m, cmd := pressKeys(m, press('k'), press('y'))
		lister.err = errors.New("zmx: listing timed out")
		m, next := land(t, m, cmd)

		assertFlashedAndOpen(t, m, next, "session alpha is not answering; the kill may still land", []string{"alpha", "bravo"})
	})

	t.Run("a refused label flashes its words over the refreshed rows", func(t *testing.T) {
		lister := &rowActionLister{sessions: sessions}
		refusal := errors.New("label refused: it holds a control or formatting character, such as a newline, tab or escape")
		m := rowActionModel(t, lister, &killerStub{}, &recordingLabeller{err: refusal})

		m, cmd := pressKeys(m, press('r'), press('\u200b'), keyEnter)
		m, next := land(t, m, cmd)

		assertFlashedAndOpen(t, m, next, refusal.Error(), []string{"alpha", "bravo"})
	})

	t.Run("a refresh that fails after a kill flashes and keeps the last rows", func(t *testing.T) {
		lister := &rowActionLister{sessions: sessions}
		killer := &killerStub{}
		m := rowActionModel(t, lister, killer, &recordingLabeller{})

		m, cmd := pressKeys(m, press('k'), press('y'))
		lister.err = errors.New("zmx: listing timed out")
		m, next := land(t, m, cmd)

		if killer.killedName != "alpha" {
			t.Errorf("killed %q, want alpha", killer.killedName)
		}
		assertFlashedAndOpen(t, m, next, "zmx: listing timed out", []string{"alpha", "bravo"})
	})

	t.Run("any later listing that fails flashes and keeps the last rows", func(t *testing.T) {
		lister := &rowActionLister{sessions: sessions}
		m := rowActionModel(t, lister, &killerStub{}, &recordingLabeller{})

		lister.err = errors.New("zmx: listing timed out")
		m, next := land(t, m, m.fetchSessionsCmd())

		assertFlashedAndOpen(t, m, next, "zmx: listing timed out", []string{"alpha", "bravo"})
		if m.ListError() != nil {
			t.Errorf("ListError() = %v, want nil once rows were shown", m.ListError())
		}
	})
}

func TestRowActions_AFirstListingThatFailsEndsThePicker(t *testing.T) {
	listErr := errors.New("zmx: listing timed out")
	m := New(&rowActionLister{err: listErr})

	m, next := land(t, m, m.fetchSessionsCmd())

	if !quits(next) {
		t.Error("a first listing that failed left the picker running")
	}
	if !errors.Is(m.ListError(), listErr) {
		t.Errorf("ListError() = %v, want %v", m.ListError(), listErr)
	}
	if m.Selected() != "" {
		t.Errorf("Selected() = %q, want nothing to attach", m.Selected())
	}
}

func TestRowActions_SucceedingActionsRefreshTheRows(t *testing.T) {
	t.Run("a kill", func(t *testing.T) {
		lister := &rowActionLister{sessions: []Session{{Name: "alpha"}, {Name: "bravo"}}}
		m := rowActionModel(t, lister, &killerStub{}, &recordingLabeller{})

		m, cmd := pressKeys(m, press('k'), press('y'))
		lister.sessions = []Session{{Name: "bravo"}}
		m, _ = land(t, m, cmd)

		if got := visibleSessionNames(m); !slices.Equal(got, []string{"bravo"}) {
			t.Errorf("rows = %v, want [bravo]", got)
		}
	})

	t.Run("a label", func(t *testing.T) {
		lister := &rowActionLister{sessions: []Session{{Name: "alpha"}}}
		m := rowActionModel(t, lister, &killerStub{}, &recordingLabeller{})

		m, cmd := pressKeys(m, press('r'), press('!'), keyEnter)
		lister.sessions = []Session{{Name: "alpha", Label: "alpha!"}}
		m, _ = land(t, m, cmd)

		if len(m.sessions) != 1 || m.sessions[0].Label != "alpha!" {
			t.Errorf("sessions = %+v, want alpha labelled alpha!", m.sessions)
		}
	})
}

func TestRowActions_ModalsShowTheLabel(t *testing.T) {
	sessions := []Session{{Name: "alpha-Hq7wNe", Label: "api work"}}

	t.Run("the rename modal is pre-filled with it and names its target by it", func(t *testing.T) {
		labeller := &recordingLabeller{}
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, labeller)

		m, _ = pressKeys(m, press('r'))

		if got := m.renameInput.Value(); got != "api work" {
			t.Errorf("input = %q, want the label", got)
		}
		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "was: api work") || strings.Contains(view, "alpha-Hq7wNe") {
			t.Errorf("want the modal naming its target by the label alone:\n%s", view)
		}

		_, cmd := pressKeys(m, keyEnter)
		cmd()
		if labeller.name != "alpha-Hq7wNe" {
			t.Errorf("labelled %q, want the session's name", labeller.name)
		}
	})

	t.Run("the rename modal of an unlabelled session is pre-filled with its name", func(t *testing.T) {
		m := rowActionModel(t, &rowActionLister{sessions: []Session{{Name: "alpha"}}}, &killerStub{}, &recordingLabeller{})

		m, _ = pressKeys(m, press('r'))

		if got := m.renameInput.Value(); got != "alpha" {
			t.Errorf("input = %q, want the name", got)
		}
	})

	t.Run("the kill modal names its target by it", func(t *testing.T) {
		killer := &killerStub{}
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, killer, &recordingLabeller{})

		m, _ = pressKeys(m, press('k'))

		view := ansi.Strip(m.View().Content)
		if !strings.Contains(view, "api work") || strings.Contains(view, "alpha-Hq7wNe") {
			t.Errorf("want the kill modal naming its target by the label alone:\n%s", view)
		}

		_, cmd := pressKeys(m, press('y'))
		cmd()
		if killer.killedName != "alpha-Hq7wNe" {
			t.Errorf("killed %q, want the session's name", killer.killedName)
		}
	})
}

func TestRowActions_ASessionNotAnswering(t *testing.T) {
	sessions := []Session{{Name: "wedged-Lp4s", NotAnswering: true}, {Name: "zulu"}}

	t.Run("Enter flashes why and attaches nothing", func(t *testing.T) {
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, &recordingLabeller{})

		m, next := pressKeys(m, keyEnter)

		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; nothing was opened", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("k confirms, tries the kill and flashes the killer's words", func(t *testing.T) {
		killer := &killerStub{err: errors.New("session wedged-Lp4s is not answering; the kill may still land")}
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, killer, &recordingLabeller{})

		m, _ = pressKeys(m, press('k'))
		if m.modal != modalKillConfirm {
			t.Fatalf("modal = %v, want the kill confirmation", m.modal)
		}
		m, cmd := pressKeys(m, press('y'))
		m, next := land(t, m, cmd)

		if killer.killedName != "wedged-Lp4s" {
			t.Errorf("killed %q, want wedged-Lp4s tried", killer.killedName)
		}
		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; the kill may still land", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("r flashes the labeller's refusal", func(t *testing.T) {
		labeller := &recordingLabeller{err: errors.New("session wedged-Lp4s is not answering; its label is unchanged")}
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, labeller)

		m, cmd := pressKeys(m, press('r'), press('x'), keyEnter)
		m, next := land(t, m, cmd)

		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; its label is unchanged", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("Enter from its preview flashes on the list and attaches nothing", func(t *testing.T) {
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, &recordingLabeller{})
		m.activePage = pagePreview
		m.preview = NewPreviewModel(sessions[0], previewNotAnswering, 120, 40)

		updated, next := m.Update(previewOpenMsg{Session: "wedged-Lp4s"})
		m = updated.(Model)

		if m.ActivePage() != PageSessions {
			t.Errorf("page = %v, want the list the refusal is read on", m.ActivePage())
		}
		assertFlashedAndOpen(t, m, next, "session wedged-Lp4s is not answering; nothing was opened", []string{"wedged-Lp4s", "zulu"})
	})

	t.Run("the one marked session of a multi-select flashes and attaches nothing", func(t *testing.T) {
		m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, &recordingLabeller{})
		m.multiSelectMode = true
		m.selectedSessions = map[string]struct{}{"wedged-Lp4s": {}}

		updated, next := m.handleMultiSelectEnter()

		assertFlashedAndOpen(t, updated.(Model), next, "session wedged-Lp4s is not answering; nothing was opened", []string{"wedged-Lp4s", "zulu"})
	})
}

func TestRowActions_EveryOpenOfAnAnsweringSessionAttaches(t *testing.T) {
	sessions := []Session{{Name: "alpha"}, {Name: "bravo"}}
	opens := map[string]func(Model) (tea.Model, tea.Cmd){
		"Enter": func(m Model) (tea.Model, tea.Cmd) { return m.Update(keyEnter) },
		"preview Enter": func(m Model) (tea.Model, tea.Cmd) {
			m.activePage = pagePreview
			m.preview = NewPreviewModel(sessions[0], "", 120, 40)
			return m.Update(previewOpenMsg{Session: "alpha"})
		},
		"multi-select Enter": func(m Model) (tea.Model, tea.Cmd) {
			m.multiSelectMode = true
			m.selectedSessions = map[string]struct{}{"alpha": {}}
			return m.handleMultiSelectEnter()
		},
	}
	for route, open := range opens {
		t.Run(route, func(t *testing.T) {
			m := rowActionModel(t, &rowActionLister{sessions: sessions}, &killerStub{}, &recordingLabeller{})

			updated, cmd := open(m)

			if !quits(cmd) || updated.(Model).Selected() != "alpha" {
				t.Errorf("Selected() = %q, quits = %v; want alpha picked and the picker ended", updated.(Model).Selected(), quits(cmd))
			}
		})
	}
}
