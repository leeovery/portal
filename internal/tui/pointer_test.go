package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
)

const (
	pointerTermW = 80
	pointerTermH = 24
	// Any column: a tap anywhere along a row lands on it.
	pointerX = 10
)

// The wheel reports content scrolling, so a finger drawn down the screen
// arrives as a step up.
var (
	fingerDown = tea.MouseWheelMsg{X: pointerX, Y: 5, Button: tea.MouseWheelUp}
	fingerUp   = tea.MouseWheelMsg{X: pointerX, Y: 5, Button: tea.MouseWheelDown}
)

func pointerSessions(names ...string) []Session {
	sessions := make([]Session, 0, len(names))
	for _, name := range names {
		sessions = append(sessions, Session{Name: name})
	}
	return sessions
}

func pointerSessionsModel(t *testing.T, termH int, sessions []Session) Model {
	t.Helper()
	m := NewModelWithSessions(sessions)
	m.termWidth = pointerTermW
	m.termHeight = termH
	m.resyncPageLayouts()
	openedASettleAgo(&m)
	return m
}

// openedASettleAgo stages a picker that has been open, the wheel quiet, for
// wheelSettle, so its first wheel step begins a gesture.
func openedASettleAgo(m *Model) {
	m.wheel.last = m.wheel.clock().Add(-wheelSettle)
}

func pointerProjectsModel(t *testing.T, projects []project.Project, creator SessionCreator) Model {
	t.Helper()
	m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha"))
	m.sessionCreator = creator
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.projectList.Select(0)
	m.activePage = PageProjects
	m.resyncPageLayouts()
	return m
}

func pointerUpdate(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	next, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", updated)
	}
	return next, cmd
}

// Returns the release's command, which carries what the tap opened.
func tapAt(t *testing.T, m Model, y int) (Model, tea.Cmd) {
	t.Helper()
	m, _ = pointerUpdate(t, m, tea.MouseClickMsg{X: pointerX, Y: y, Button: tea.MouseLeft})
	return pointerUpdate(t, m, tea.MouseReleaseMsg{X: pointerX, Y: y, Button: tea.MouseLeft})
}

// The terminal row the composed frame draws text on, read off the real render so
// the test never restates the layout it is checking.
func screenRowOf(t *testing.T, m Model, text string) int {
	t.Helper()
	found := -1
	for y, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if !strings.Contains(line, text) {
			continue
		}
		if found >= 0 {
			t.Fatalf("fixture: %q is drawn on rows %d and %d, want exactly one", text, found, y)
		}
		found = y
	}
	if found < 0 {
		t.Fatalf("fixture: %q is not drawn anywhere in the frame:\n%s", text, ansi.Strip(m.View().Content))
	}
	return found
}

func requireOpened(t *testing.T, m Model, cmd tea.Cmd, want string) {
	t.Helper()
	if got := m.Selected(); got != want {
		t.Fatalf("tap opened %q, want %q", got, want)
	}
	if !isQuitCmd(cmd) {
		t.Fatalf("tap on %q did not quit the picker to connect", want)
	}
}

func requireNothingOpened(t *testing.T, m Model, cmd tea.Cmd) {
	t.Helper()
	if got := m.Selected(); got != "" {
		t.Fatalf("opened %q, want nothing", got)
	}
	if cmd != nil {
		t.Fatalf("returned a command (%T), want none", cmd())
	}
}

func TestMouseReportingIsOnForThePicker(t *testing.T) {
	m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha"))
	if got := m.View().MouseMode; got != tea.MouseModeCellMotion {
		t.Fatalf("View().MouseMode = %v, want MouseModeCellMotion so wheel and tap events reach the picker", got)
	}
}

func TestTapOpensTheSessionUnderIt(t *testing.T) {
	names := []string{"alpha", "bravo", "charlie", "delta"}

	t.Run("each row of a flat list opens its own session", func(t *testing.T) {
		for _, name := range names {
			m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
			m, cmd := tapAt(t, m, screenRowOf(t, m, name))
			requireOpened(t, m, cmd, name)
		}
	})

	t.Run("a notice band above the list moves the rows and the mapping together", func(t *testing.T) {
		for _, name := range names {
			m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
			m.setFlash("something to say")
			if (&m).sessionBandHeight() == 0 {
				t.Fatal("fixture: the flash raised no band")
			}
			m, cmd := tapAt(t, m, screenRowOf(t, m, name))
			requireOpened(t, m, cmd, name)
		}
	})

	t.Run("a row on a later page opens the session drawn there", func(t *testing.T) {
		many := make([]string, 0, 40)
		for i := range 40 {
			many = append(many, fmt.Sprintf("sess-%02d", i))
		}
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(many...))
		m.sessionList.Select(len(many) - 1)
		if m.sessionList.Paginator.Page == 0 {
			t.Fatal("fixture: the list fits one page, so there is no later page to tap")
		}
		target := many[m.sessionList.Paginator.Page*m.sessionList.Paginator.PerPage]
		m, cmd := tapAt(t, m, screenRowOf(t, m, target))
		requireOpened(t, m, cmd, target)
	})

	t.Run("under an applied filter the tap opens the match drawn on that row", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m.SetSessionListFilter("char")
		m.resyncPageLayouts()
		m, cmd := tapAt(t, m, screenRowOf(t, m, "charlie"))
		requireOpened(t, m, cmd, "charlie")
	})

	t.Run("while the filter input holds the keyboard the tap still opens its row", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m, _ = pointerUpdate(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		if !m.sessionList.SettingFilter() {
			t.Fatal("fixture: / did not focus the filter input")
		}
		m, cmd := tapAt(t, m, screenRowOf(t, m, "bravo"))
		requireOpened(t, m, cmd, "bravo")
	})

	t.Run("in a grouped list a heading opens nothing and a session row opens", func(t *testing.T) {
		dirA, dirB := t.TempDir(), t.TempDir()
		projects := []project.Project{{Path: dirA, Name: "Orion"}, {Path: dirB, Name: "Vega"}}
		sessions := []Session{{Name: "orion-1", Dir: dirA}, {Name: "vega-1", Dir: dirB}}
		m := pointerSessionsModel(t, pointerTermH, sessions)
		m.sessionListMode = prefs.ModeByProject
		m.sessionList.SetItems(buildByProject(sessions, project.NewIndex(projects), nil))
		m.ensureSessionRowSelected()

		headed, cmd := tapAt(t, m, screenRowOf(t, m, "Vega"))
		requireNothingOpened(t, headed, cmd)

		m, cmd = tapAt(t, m, screenRowOf(t, m, "vega-1"))
		requireOpened(t, m, cmd, "vega-1")
	})
}

func TestTapOpensNothingOffARow(t *testing.T) {
	names := []string{"alpha", "bravo", "charlie"}

	t.Run("a press on one row released on another is a drag, not a tap", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m, _ = pointerUpdate(t, m, tea.MouseClickMsg{X: pointerX, Y: screenRowOf(t, m, "alpha"), Button: tea.MouseLeft})
		m, cmd := pointerUpdate(t, m, tea.MouseReleaseMsg{X: pointerX, Y: screenRowOf(t, m, "charlie"), Button: tea.MouseLeft})
		requireNothingOpened(t, m, cmd)
	})

	t.Run("a release with no press before it", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m, cmd := pointerUpdate(t, m, tea.MouseReleaseMsg{X: pointerX, Y: screenRowOf(t, m, "bravo"), Button: tea.MouseLeft})
		requireNothingOpened(t, m, cmd)
	})

	t.Run("a right-button press", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		y := screenRowOf(t, m, "bravo")
		m, _ = pointerUpdate(t, m, tea.MouseClickMsg{X: pointerX, Y: y, Button: tea.MouseRight})
		m, cmd := pointerUpdate(t, m, tea.MouseReleaseMsg{X: pointerX, Y: y, Button: tea.MouseRight})
		requireNothingOpened(t, m, cmd)
	})

	t.Run("every row that draws no session", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		rows := map[int]bool{}
		for _, name := range names {
			rows[screenRowOf(t, m, name)] = true
		}
		for y := range pointerTermH {
			if rows[y] {
				continue
			}
			got, cmd := tapAt(t, m, y)
			if got.Selected() != "" || cmd != nil {
				t.Errorf("a tap on row %d, which draws no session, opened %q", y, got.Selected())
			}
		}
	})

	t.Run("a modal over the list", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		y := screenRowOf(t, m, "bravo")
		m.modal = modalHelp
		m, cmd := tapAt(t, m, y)
		requireNothingOpened(t, m, cmd)
		if m.modal != modalHelp {
			t.Fatal("the tap dismissed the modal; a modal owns the input")
		}
	})
}

func TestTapInMultiSelectMarksTheRow(t *testing.T) {
	m := pointerSessionsModel(t, pointerTermH, pointerSessions("alpha", "bravo", "charlie"))
	m.multiSelectMode = true
	m.selectedSessions = map[string]struct{}{}

	y := screenRowOf(t, m, "charlie")
	m, cmd := tapAt(t, m, y)
	if cmd != nil || m.Selected() != "" {
		t.Fatalf("a tap in multi-select opened %q; it must mark the row instead", m.Selected())
	}
	if !m.IsSessionSelected("charlie") || m.SelectedSessionCount() != 1 {
		t.Fatalf("a tap on charlie marked %d session(s), want charlie alone", m.SelectedSessionCount())
	}

	m, _ = tapAt(t, m, y)
	if m.SelectedSessionCount() != 0 {
		t.Fatal("a second tap on a marked row must unmark it")
	}
}

func TestTapOpensTheProjectUnderIt(t *testing.T) {
	projects := []project.Project{
		{Path: "/work/orion", Name: "Orion"},
		{Path: "/work/vega", Name: "Vega"},
	}
	for _, p := range projects {
		// A project row is two lines tall: its name over its path. Both are the row.
		for _, line := range []string{p.Name, p.Path} {
			t.Run(fmt.Sprintf("the %q line", line), func(t *testing.T) {
				creator := &recordingCreator{}
				m := pointerProjectsModel(t, projects, creator)
				_, cmd := tapAt(t, m, screenRowOf(t, m, line))
				if cmd == nil {
					t.Fatalf("a tap on %q started no session", line)
				}
				cmd()
				if creator.dir != p.Path {
					t.Fatalf("a tap on %q created a session in %q, want %q", line, creator.dir, p.Path)
				}
			})
		}
	}
}

func TestSwipeMovesTheHighlightWithTheFinger(t *testing.T) {
	names := []string{"alpha", "bravo", "charlie"}

	t.Run("down then up", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := m.sessionList.Index(); got != 1 {
			t.Fatalf("after one swipe step down the selection is at %d, want 1", got)
		}
		m, _ = pointerUpdate(t, m, fingerUp)
		if got := m.sessionList.Index(); got != 0 {
			t.Fatalf("after a swipe step back up the selection is at %d, want 0", got)
		}
	})

	t.Run("a step past the bottom wraps to the top", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m.sessionList.Select(len(names) - 1)
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := m.sessionList.Index(); got != 0 {
			t.Fatalf("a swipe step down at the bottom moved the selection to %d, want it to wrap to 0", got)
		}
	})

	t.Run("a step past the top wraps to the bottom", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m, _ = pointerUpdate(t, m, fingerUp)
		if got := m.sessionList.Index(); got != len(names)-1 {
			t.Fatalf("a swipe step up at the top moved the selection to %d, want it to wrap to %d", got, len(names)-1)
		}
	})

	t.Run("in a grouped list a step down skips the heading, and off the bottom wraps past the first", func(t *testing.T) {
		dirA, dirB := t.TempDir(), t.TempDir()
		projects := []project.Project{{Path: dirA, Name: "Orion"}, {Path: dirB, Name: "Vega"}}
		sessions := []Session{{Name: "orion-1", Dir: dirA}, {Name: "vega-1", Dir: dirB}}
		m := pointerSessionsModel(t, pointerTermH, sessions)
		m.sessionList.SetItems(buildByProject(sessions, project.NewIndex(projects), nil))
		m.ensureSessionRowSelected()

		m, _ = pointerUpdate(t, m, fingerDown)
		if si, ok := m.selectedSessionItem(); !ok || si.Session.Name != "vega-1" {
			t.Fatalf("a swipe step down did not skip the heading onto vega-1 (selected %+v)", m.sessionList.SelectedItem())
		}
		m, _ = pointerUpdate(t, m, fingerDown)
		if si, ok := m.selectedSessionItem(); !ok || si.Session.Name != "orion-1" {
			t.Fatalf("a swipe step down off the bottom did not wrap to orion-1 (selected %+v)", m.sessionList.SelectedItem())
		}
	})

	t.Run("a modal holds the list still", func(t *testing.T) {
		m := pointerSessionsModel(t, pointerTermH, pointerSessions(names...))
		m.modal = modalHelp
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := m.sessionList.Index(); got != 0 {
			t.Fatalf("a swipe step under a modal moved the selection to %d", got)
		}
		if m.modal != modalHelp {
			t.Fatal("a swipe step dismissed the modal")
		}
	})

	t.Run("the projects list steps too", func(t *testing.T) {
		projects := []project.Project{{Path: "/work/orion", Name: "Orion"}, {Path: "/work/vega", Name: "Vega"}}
		m := pointerProjectsModel(t, projects, nil)
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := m.projectList.Index(); got != 1 {
			t.Fatalf("a swipe step down on Projects left the selection at %d, want 1", got)
		}
	})
}

func TestSwipeMovesTheThemePanelCursorWithTheFinger(t *testing.T) {
	rows := arrowValidRows(t, 4)

	t.Run("down moves the cursor as the arrow does", func(t *testing.T) {
		m := newArrowPanelModel(t, rows, arrowSlug(1))
		openedASettleAgo(&m)
		want := arrowPanelIndex(dispatchPanelArrow(t, m, arrowDown))
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := arrowPanelIndex(m); got != want {
			t.Fatalf("a swipe step down left the panel cursor at %d, want %d (where Down lands)", got, want)
		}
	})

	t.Run("on the last row it lands where Down does", func(t *testing.T) {
		m := newArrowPanelModel(t, rows, arrowSlug(len(rows)-1))
		openedASettleAgo(&m)
		want := arrowPanelIndex(dispatchPanelArrow(t, m, arrowDown))
		m, _ = pointerUpdate(t, m, fingerDown)
		if got := arrowPanelIndex(m); got != want {
			t.Fatalf("a swipe step down on the last row left the panel cursor at %d, want %d (where Down lands)", got, want)
		}
	})
}

func dispatchPanelArrow(t *testing.T, m Model, press tea.KeyPressMsg) Model {
	t.Helper()
	next, _ := pointerUpdate(t, m, press)
	return next
}

// The scrollback moves with the finger, so the preview takes the wheel unflipped:
// a finger drawn down pulls older lines into view.
func TestSwipeScrollsThePreviewWithTheFinger(t *testing.T) {
	sessions := pointerSessions("alpha")
	lines := make([]string, 0, 200)
	for i := range 200 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	m := modelWithSeams(t, sessions, historyFor(t, strings.Join(lines, "\n"), sessions))
	m = openPreviewOn(t, m)
	if m.activePage != pagePreview {
		t.Fatal("fixture: Space did not open the preview")
	}
	before := m.preview.viewport.YOffset()

	m, _ = pointerUpdate(t, m, fingerDown)
	if got := m.preview.viewport.YOffset(); got >= before {
		t.Fatalf("a swipe step down left the preview at offset %d, want older lines above %d", got, before)
	}
}
