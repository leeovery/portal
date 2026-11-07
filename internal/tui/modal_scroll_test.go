package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/project"
)

// The owner's short terminal, where the help and edit modals outgrow the
// room, and a tall one, where every modal fits.
const (
	shortModalWidth, shortModalHeight = 138, 15
	tallModalWidth, tallModalHeight   = 120, 40
)

func pressed(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, k := range keys {
		updated, _ := m.Update(k)
		m = updated.(Model)
	}
	return m
}

func drawn(m Model) string {
	return ansi.Strip(m.View().Content)
}

// helpOpenAt is the Sessions page at w×h with its help modal open.
func helpOpenAt(t *testing.T, w, h int) Model {
	t.Helper()
	m := resizedTo(t, New(fakeLister{}), w, h)
	m.activePage = PageSessions
	return pressed(t, m, tea.KeyPressMsg{Code: '?', Text: "?"})
}

// projectsAt is the Projects page at w×h, listing a project with tags.
func projectsAt(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(fakeLister{})
	m.projectEditor, m.aliasEditor = stubTagsProjectEditor{}, stubTagsAliasEditor{}
	updated, _ := m.Update(ProjectsLoadedMsg{Projects: []project.Project{{Path: "/p/one", Name: "one", Tags: []string{"web", "api"}}}})
	m = resizedTo(t, updated.(Model), w, h)
	m.activePage = PageProjects
	return m
}

// editOpenAt is the Projects page at w×h with the edit modal open.
func editOpenAt(t *testing.T, w, h int) Model {
	t.Helper()
	return pressed(t, projectsAt(t, w, h), tea.KeyPressMsg{Code: 'e', Text: "e"})
}

func down() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyDown} }
func up() tea.KeyPressMsg   { return tea.KeyPressMsg{Code: tea.KeyUp} }
func tab() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyTab} }

// panelBottom is the drawn modal's bottom border row, "" where none is drawn.
func panelBottom(view string) string {
	for row := range strings.SplitSeq(view, "\n") {
		if strings.HasPrefix(strings.TrimSpace(row), panelFrameBottomLeft) {
			return row
		}
	}
	return ""
}

// The help modal's ↑/↓ scroll its keys a row at a time, from the top to
// where its last row shows, and no further either way.
func TestTheHelpModalsArrowsScrollItsKeys(t *testing.T) {
	top := helpOpenAt(t, shortModalWidth, shortModalHeight)
	if !strings.Contains(drawn(top), "Move selection") || strings.Contains(drawn(top), "Rename session") {
		t.Fatalf("the help modal opens other than at the top of its keys:\n%s", drawn(top))
	}

	once := pressed(t, top, down())
	if strings.Contains(drawn(once), "Move selection") || !strings.Contains(drawn(once), "Next / prev page") {
		t.Errorf("↓ doesn't scroll the keys a row:\n%s", drawn(once))
	}

	bottom := pressed(t, top, slices.Repeat([]tea.KeyPressMsg{down()}, 30)...)
	if !strings.Contains(drawn(bottom), pendingLegendLabel) {
		t.Errorf("↓ held doesn't reach the last row:\n%s", drawn(bottom))
	}
	if past := pressed(t, bottom, down()); drawn(past) != drawn(bottom) {
		t.Errorf("↓ scrolls past the last row:\n%s", drawn(past))
	}
	if back := pressed(t, bottom, up()); drawn(back) == drawn(bottom) {
		t.Errorf("↑ from the last row doesn't scroll back:\n%s", drawn(back))
	}
	if home := pressed(t, bottom, slices.Repeat([]tea.KeyPressMsg{up()}, 30)...); drawn(home) != drawn(top) {
		t.Errorf("↑ held doesn't return to the top:\n%s", drawn(home))
	}
	if above := pressed(t, top, up()); drawn(above) != drawn(top) {
		t.Errorf("↑ at the top moves the keys:\n%s", drawn(above))
	}
}

// Tab carries the edit modal's view to each field it focuses, every row of
// the field shown, round to the first again.
func TestTabKeepsTheEditModalsFocusedFieldInView(t *testing.T) {
	m := editOpenAt(t, shortModalWidth, shortModalHeight)
	for i := range 4 {
		view := drawn(m)
		for _, row := range m.editModalFields(m.themeState.active, m.colourless)[m.editFocus] {
			if shown := strings.TrimSpace(ansi.Strip(row)); !strings.Contains(view, shown) {
				t.Errorf("after %d tabs, focused on field %d, its row %q isn't shown:\n%s", i, m.editFocus, shown, view)
			}
		}
		m = pressed(t, m, tab())
	}
}

// The bottom border signs which way rows are hidden while some are, and is
// plain while none is; the title names the scroll keys only while the keys
// scroll.
func TestAModalSignsHiddenRowsOnlyWhileItHidesThem(t *testing.T) {
	top := helpOpenAt(t, shortModalWidth, shortModalHeight)
	mid := pressed(t, top, down())
	end := pressed(t, top, slices.Repeat([]tea.KeyPressMsg{down()}, 30)...)
	editTop := editOpenAt(t, shortModalWidth, shortModalHeight)
	editEnd := pressed(t, editTop, tab(), tab())
	for _, tt := range []struct {
		name string
		m    Model
		sign string
	}{
		{name: "help at the top", m: top, sign: panelMoreBelow},
		{name: "help scrolled", m: mid, sign: panelMoreBoth},
		{name: "help at the foot", m: end, sign: panelMoreAbove},
		{name: "edit on its first field", m: editTop, sign: panelMoreBelow},
		{name: "edit on its last field", m: editEnd, sign: panelMoreAbove},
		{name: "help with room", m: helpOpenAt(t, tallModalWidth, tallModalHeight)},
		{name: "edit with room", m: editOpenAt(t, tallModalWidth, tallModalHeight)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bottom := panelBottom(drawn(tt.m))
			signed := slices.DeleteFunc([]string{panelMoreAbove, panelMoreBelow, panelMoreBoth}, func(s string) bool {
				return !strings.Contains(bottom, " "+s+" ")
			})
			if want := slices.DeleteFunc([]string{tt.sign}, func(s string) bool { return s == "" }); !slices.Equal(signed, want) {
				t.Errorf("the bottom border signs %q, want %q: %q", signed, want, bottom)
			}
		})
	}
	if !strings.Contains(drawn(top), helpModalHint(true)) {
		t.Errorf("scrolling, the help title doesn't name the scroll keys:\n%s", drawn(top))
	}
	if tall := drawn(helpOpenAt(t, tallModalWidth, tallModalHeight)); strings.Contains(tall, helpScrollKey+" scroll") {
		t.Errorf("with room, the help title names the scroll keys:\n%s", tall)
	}
}

// Every modal, at every height from the chrome's floor up, draws its whole
// border inside the page, clear of the header's and footer's rows.
func TestEveryModalDrawsItsBorderWithinThePage(t *testing.T) {
	modals := map[string]func(w, h int) Model{
		"help on Sessions": func(w, h int) Model { return helpOpenAt(t, w, h) },
		"help on Projects": func(w, h int) Model {
			return pressed(t, projectsAt(t, w, h), tea.KeyPressMsg{Code: '?', Text: "?"})
		},
		"edit": func(w, h int) Model { return pressed(t, editOpenAt(t, w, h), tab()) },
		"kill": func(w, h int) Model {
			m := resizedTo(t, New(fakeLister{}), w, h)
			m.activePage, m.modal, m.pendingKill = PageSessions, modalKillConfirm, Session{Name: "kiln-relay"}
			return m
		},
		"rename": func(w, h int) Model {
			m := resizedTo(t, New(fakeLister{}), w, h)
			m.activePage, m.modal, m.renameInput, m.renameTarget = PageSessions, modalRename, textinput.New(), Session{Name: "kiln-relay"}
			return m
		},
		"delete": func(w, h int) Model {
			m := projectsAt(t, w, h)
			m.modal, m.pendingDeleteName, m.pendingDeletePath = modalDeleteProject, "one", "/p/one"
			return m
		},
	}
	for name, open := range modals {
		t.Run(name, func(t *testing.T) {
			for h := 12; h <= tallModalHeight; h++ {
				if problem := borderProblem(drawn(open(shortModalWidth, h)), h); problem != "" {
					t.Errorf("at %d rows %s:\n%s", h, problem, drawn(open(shortModalWidth, h)))
				}
			}
		})
	}
}

// borderProblem is how the drawn modal fails to draw its border whole inside
// a page h rows tall, "" where it doesn't.
func borderProblem(view string, h int) string {
	rows := strings.Split(view, "\n")
	if len(rows) != h {
		return fmt.Sprintf("it draws %d rows", len(rows))
	}
	var tops, bottoms []int
	for i, row := range rows {
		switch trimmed := strings.TrimSpace(row); {
		case strings.HasPrefix(trimmed, panelFrameTopLeft):
			tops = append(tops, i)
		case strings.HasPrefix(trimmed, panelFrameBottomLeft):
			bottoms = append(bottoms, i)
		}
	}
	switch {
	case len(tops) != 1 || len(bottoms) != 1:
		return fmt.Sprintf("it draws top borders on rows %v and bottom borders on rows %v", tops, bottoms)
	case tops[0] <= Vinset:
		return "its top border reaches the header's row"
	case bottoms[0] >= h-1-Vinset:
		return "its bottom border reaches the footer's row"
	}
	return ""
}

// Resized taller with the help scrolled further than the new room lets it,
// the help draws at the furthest it now scrolls, and the first ↑ moves it
// from there.
func TestTheHelpScrollsUpAtOnceAfterAResizeShortensItsScroll(t *testing.T) {
	bottom := pressed(t, helpOpenAt(t, shortModalWidth, shortModalHeight), slices.Repeat([]tea.KeyPressMsg{down()}, 30)...)
	taller := resizedTo(t, bottom, shortModalWidth, shortModalHeight+4)
	if !strings.Contains(panelBottom(drawn(taller)), panelMoreAbove) {
		t.Fatalf("resized taller, the help no longer scrolls: nothing to hold\n%s", drawn(taller))
	}
	if scrolled := pressed(t, taller, up()); drawn(scrolled) == drawn(taller) {
		t.Errorf("the first ↑ after the resize doesn't move the help:\n%s", drawn(scrolled))
	}
}
