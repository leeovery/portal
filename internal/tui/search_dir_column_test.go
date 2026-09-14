package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// Wide enough that fitSessionDir renders a temp-dir path whole, so an assertion
// on the raw directory is an assertion on the column rather than on the fit.
const dirColumnTestWidth = 220

func sizeForDirColumn(t *testing.T, m Model) Model {
	t.Helper()
	return pressSettled(t, m, tea.WindowSizeMsg{Width: dirColumnTestWidth, Height: 30})
}

func dirColumnModel(t *testing.T, deps Deps, sessions []tmux.Session, projects []project.Project) Model {
	t.Helper()
	// The preview dismissal re-lists, so the lister must answer with the same
	// fixture rather than emptying the picker mid-test.
	deps.Lister = &stepListerStub{steps: [][]tmux.Session{sessions}}
	deps.ProjectStore = stubProjectStore{}
	m := ingestLanding(t, Build(deps), sessions, projects)
	return sizeForDirColumn(t, m)
}

func searchColumnModel(t *testing.T, mode prefs.SessionListMode, term string, sessions []tmux.Session, projects []project.Project) Model {
	t.Helper()
	return dirColumnModel(t, Deps{InitialMode: mode, Search: &SearchForm{Term: term}}, sessions, projects)
}

// runCmd drives a command and whatever it returns back through Update, so a
// rebuild whose filter pass is an async command is settled before the rows are
// read. The list re-filters that way on any SetItems while a filter is applied.
func runCmd(t *testing.T, m Model, cmd tea.Cmd, depth int) Model {
	t.Helper()
	if cmd == nil || depth >= 8 {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			m = runCmd(t, m, c, depth+1)
		}
		return m
	}
	updated, next := m.Update(msg)
	return runCmd(t, updated.(Model), next, depth+1)
}

func pressSettled(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, cmd := m.Update(msg)
	return runCmd(t, updated.(Model), cmd, 0)
}

func renderedRows(m Model) string {
	return ansi.Strip(m.sessionList.View())
}

func assertDirRendered(t *testing.T, m Model, dir string) {
	t.Helper()
	if rows := renderedRows(m); !strings.Contains(rows, dir) {
		t.Errorf("session rows must carry the recorded directory %q:\n%s", dir, rows)
	}
}

func assertDirAbsent(t *testing.T, m Model, dir string) {
	t.Helper()
	if rows := renderedRows(m); strings.Contains(rows, dir) {
		t.Errorf("session rows must not carry the directory %q:\n%s", dir, rows)
	}
}

func dirColumnFixture(t *testing.T) (dir string, sessions []tmux.Session, projects []project.Project) {
	t.Helper()
	dir = t.TempDir()
	return dir,
		[]tmux.Session{{Name: "portal-abc", Windows: 2, Dir: dir}},
		[]project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
}

func TestSearchDirColumn(t *testing.T) {
	t.Run("it renders the directory column in a search-opened picker", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		assertDirRendered(t, m, dir)
	})

	t.Run("it renders the column for the term-less search form", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "", sessions, projects)

		assertDirRendered(t, m, dir)
	})

	t.Run("it renders the column in By Project and By Tag", func(t *testing.T) {
		for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
			dir, sessions, projects := dirColumnFixture(t)
			m := searchColumnModel(t, mode, "portal", sessions, projects)

			assertDirRendered(t, m, dir)
		}
	})

	t.Run("it leaves a -f picker's rows unchanged", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := dirColumnModel(t, Deps{InitialMode: prefs.ModeFlat, InitialFilter: "portal"}, sessions, projects)

		assertDirAbsent(t, m, dir)
	})

	t.Run("it leaves a no-filter picker's rows unchanged", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := dirColumnModel(t, Deps{InitialMode: prefs.ModeFlat}, sessions, projects)

		assertDirAbsent(t, m, dir)
	})

	t.Run("it keeps the column after an s regroup", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		for i, want := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag, prefs.ModeFlat} {
			m = pressSettled(t, m, tea.KeyPressMsg{Code: 's', Text: "s"})
			if m.sessionListMode != want {
				t.Fatalf("press %d left the list in mode %v, want %v — s no longer regroups from a committed filter", i+1, m.sessionListMode, want)
			}
			assertDirRendered(t, m, dir)
		}
	})

	t.Run("it keeps the column after a preview and back", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)
		m.enumerator = keymapParityEnumerator{}
		m.reader = keymapParityReader{}

		m = pressSettled(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
		if m.activePage != pagePreview {
			t.Fatalf("precondition: Space must open the preview, active page = %d", m.activePage)
		}
		m = pressSettled(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
		if m.activePage != PageSessions {
			t.Fatalf("precondition: Esc must dismiss the preview, active page = %d", m.activePage)
		}

		assertDirRendered(t, m, dir)
	})

	t.Run("it keeps the column after a sessions refresh", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		m = pressSettled(t, m, SessionsMsg{Sessions: sessions})

		assertDirRendered(t, m, dir)
	})

	t.Run("it keeps the column after a marked-set mutation", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		m = pressSettled(t, m, pressM)
		if !m.MultiSelectActive() {
			t.Fatalf("precondition: m must enter multi-select")
		}

		assertDirRendered(t, m, dir)
	})

	t.Run("it keeps the column after a live theme swap", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		m.ApplyTheme(testLightTheme(t))

		assertDirRendered(t, m, dir)
	})

	t.Run("it keeps the column after the filter text is edited", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		// Reopening the filter input keeps the committed text, so this appends.
		m = pressSettled(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		for _, r := range "-a" {
			m = pressSettled(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		m = pressSettled(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})

		if got := m.sessionList.FilterValue(); got != "portal-a" {
			t.Fatalf("precondition: filter value = %q, want %q", got, "portal-a")
		}
		assertDirRendered(t, m, dir)
	})

	t.Run("it keeps the column after the filter is cleared", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		m := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		m = pressSettled(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
		m = pressSettled(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})

		if got := m.sessionList.FilterValue(); got != "" {
			t.Fatalf("precondition: filter value = %q, want it cleared", got)
		}
		assertDirRendered(t, m, dir)
	})

	t.Run("it renders no directory on a group header row", func(t *testing.T) {
		for _, mode := range []prefs.SessionListMode{prefs.ModeByProject, prefs.ModeByTag} {
			dir, sessions, projects := dirColumnFixture(t)
			m := searchColumnModel(t, mode, "", sessions, projects)

			heading := "Portal"
			if mode == prefs.ModeByTag {
				heading = "work"
			}
			var found bool
			for line := range strings.SplitSeq(renderedRows(m), "\n") {
				if !strings.Contains(line, heading) || strings.Contains(line, "portal-abc") {
					continue
				}
				found = true
				if strings.Contains(line, dir) {
					t.Errorf("group header row must render no directory: %q", line)
				}
			}
			if !found {
				t.Fatalf("precondition: no %q header row rendered:\n%s", heading, renderedRows(m))
			}
		}
	})

	t.Run("it shows no directory for a session whose directory is known only to grouping", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{{Name: "portal-abc", Windows: 1, Dir: ""}}

		m := dirColumnModel(t, Deps{
			InitialMode: prefs.ModeByProject,
			Search:      &SearchForm{Term: "portal"},
			DirReader:   &fakeStamper{path: dir},
			DirRunner:   &fakeDirRunner{gitRoot: dir},
		}, sessions, projects)

		if m.derivedDirs[sessions[0].Name] == "" {
			t.Fatalf("precondition: the grouped rebuild must derive a directory, got %v", m.derivedDirs)
		}
		assertDirAbsent(t, m, dir)
	})

	t.Run("it issues no pane read for the column", func(t *testing.T) {
		dir, sessions, projects := dirColumnFixture(t)
		reader := &fakeStamper{path: dir}
		m := dirColumnModel(t, Deps{
			InitialMode: prefs.ModeFlat,
			Search:      &SearchForm{Term: "portal"},
			DirReader:   reader,
			DirRunner:   &fakeDirRunner{gitRoot: dir},
		}, sessions, projects)

		assertDirRendered(t, m, dir)

		if len(reader.reads) != 0 {
			t.Errorf("the column must issue no pane read, got reads for %v", reader.reads)
		}
	})

	t.Run("it keeps every matching row in the list at a width that drops the directory", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
		sessions := []tmux.Session{
			{Name: "portal-abc", Windows: 1, Dir: dir},
			{Name: "portal-xyz", Windows: 2, Dir: dir},
		}
		wide := searchColumnModel(t, prefs.ModeFlat, "portal", sessions, projects)

		narrow := pressSettled(t, wide, tea.WindowSizeMsg{Width: 34, Height: 30})

		assertDirAbsent(t, narrow, dir)
		wideNames := sessionNames(wide.sessionList.VisibleItems())
		narrowNames := sessionNames(narrow.sessionList.VisibleItems())
		if strings.Join(wideNames, ",") != strings.Join(narrowNames, ",") {
			t.Errorf("visible rows = %v at a narrow width, want %v (the wide set)", narrowNames, wideNames)
		}
	})
}
