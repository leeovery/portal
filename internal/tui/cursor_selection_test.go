package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

func newCursorTestModel(t *testing.T, items []list.Item) Model {
	t.Helper()
	m := Model{
		sessionList: newSessionList(nil),
		activePage:  PageSessions,
		themeState:  themeState{active: testDarkTheme(t)},
	}
	m.applySessionListSize(80, 24)
	m.sessionList.SetItems(items)
	m.ensureSessionRowSelected()
	return m
}

var (
	keyUp   = tea.KeyPressMsg{Code: tea.KeyUp}
	keyDown = tea.KeyPressMsg{Code: tea.KeyDown}
)

func selectedHeader(m Model) bool {
	_, ok := m.sessionList.SelectedItem().(HeaderItem)
	return ok
}

func TestCursorLandsOnlyOnSessionInstances(t *testing.T) {
	t.Run("it places the initial cursor on the first session row, skipping the leading header", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
		}
		items := buildByProject(sessions, project.NewIndex(projects), nil)

		m := newCursorTestModel(t, items)

		if selectedHeader(m) {
			t.Fatalf("initial selection rests on a header, want a session row")
		}
		si, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem returned ok=false on a populated list")
		}
		wantFirst := sessionRows(items)[0].Session.Name
		if si.Session.Name != wantFirst {
			t.Errorf("initial selected session = %q, want %q (first session row)", si.Session.Name, wantFirst)
		}
	})

	t.Run("down navigation skips the header between two groups", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
		}
		items := buildByProject(sessions, project.NewIndex(projects), nil)
		m := newCursorTestModel(t, items)

		updated, _ := m.Update(keyDown)
		m = updated.(Model)

		if selectedHeader(m) {
			t.Fatalf("Down landed on a header, want it skipped to the next session")
		}
		si, _ := m.selectedSessionItem()
		if si.Session.Name != "bravo-1" {
			t.Errorf("after Down, selected = %q, want bravo-1 (header skipped)", si.Session.Name)
		}
	})

	t.Run("up navigation skips the header between two groups", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
		}
		items := buildByProject(sessions, project.NewIndex(projects), nil)
		m := newCursorTestModel(t, items)

		updated, _ := m.Update(keyDown)
		m = updated.(Model)
		updated, _ = m.Update(keyUp)
		m = updated.(Model)

		if selectedHeader(m) {
			t.Fatalf("Up landed on a header, want it skipped to the previous session")
		}
		si, _ := m.selectedSessionItem()
		if si.Session.Name != "alpha-1" {
			t.Errorf("after Up, selected = %q, want alpha-1 (header skipped)", si.Session.Name)
		}
	})

	t.Run("it lands on the first and last session row via arrow nav, never a header", func(t *testing.T) {
		dirA := t.TempDir()
		dirB := t.TempDir()
		projects := []project.Project{
			{Path: dirA, Name: "Alpha"},
			{Path: dirB, Name: "Bravo"},
		}
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "alpha-2", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
		}
		items := buildByProject(sessions, project.NewIndex(projects), nil)
		rows := sessionRows(items)

		m := newCursorTestModel(t, items)

		for range len(items) {
			updated, _ := m.Update(keyDown)
			m = updated.(Model)
		}
		if selectedHeader(m) {
			t.Fatalf("after Down-to-end the cursor rests on a header")
		}
		last, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false at the end")
		}
		if last.Session.Name != rows[len(rows)-1].Session.Name {
			t.Errorf("at the end, selected = %q, want %q (last session row)", last.Session.Name, rows[len(rows)-1].Session.Name)
		}

		for range len(items) {
			updated, _ := m.Update(keyUp)
			m = updated.(Model)
		}
		if selectedHeader(m) {
			t.Fatalf("after Up-to-start the cursor rests on a header")
		}
		first, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false at the start")
		}
		if first.Session.Name != rows[0].Session.Name {
			t.Errorf("at the start, selected = %q, want %q (first session row)", first.Session.Name, rows[0].Session.Name)
		}
	})

	t.Run("it resolves two By-Tag instances of one session to the same underlying session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{
			{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}},
		}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		items := buildByTag(sessions, project.NewIndex(projects), nil)
		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("expected 2 instances (one per tag), got %d", len(rows))
		}

		if rows[0].GroupKey == rows[1].GroupKey {
			t.Fatalf("expected distinct tags on the two instances, got %q and %q", rows[0].GroupKey, rows[1].GroupKey)
		}

		m := newCursorTestModel(t, items)

		var resolved []string
		for idx, it := range items {
			if _, ok := it.(SessionItem); !ok {
				continue
			}
			m.sessionList.Select(idx)
			si, ok := m.selectedSessionItem()
			if !ok {
				t.Fatalf("selectedSessionItem ok=false at index %d", idx)
			}
			resolved = append(resolved, si.Session.Name)
		}

		if len(resolved) != 2 || resolved[0] != "portal-abc" || resolved[1] != "portal-abc" {
			t.Errorf("By-Tag instances resolved to %v, want both portal-abc", resolved)
		}
	})

	t.Run("selectedSessionItem returns false when the cursor is parked on a header", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{{Path: dir, Name: "Portal"}}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		items := buildByProject(sessions, project.NewIndex(projects), nil)

		m := newCursorTestModel(t, items)
		m.sessionList.Select(0)

		if _, ok := m.selectedSessionItem(); ok {
			t.Errorf("selectedSessionItem ok=true on a header row, want false")
		}
	})
}
