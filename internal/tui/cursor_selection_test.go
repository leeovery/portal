package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// newCursorTestModel builds a Model with a real, production-sized session list
// loaded with the supplied grouped slice (HeaderItems interleaved with session
// rows), then nudges the selection off the leading header exactly as
// rebuildSessionList does in production.
func newCursorTestModel(t *testing.T, items []list.Item) Model {
	t.Helper()
	m := Model{
		sessionList: newSessionList(nil),
		activePage:  PageSessions,
	}
	m.applySessionListSize(80, 24)
	m.sessionList.SetItems(items)
	m.ensureSessionRowSelected()
	return m
}

// keyG is the bubbles/list GoToStart binding (g/home); keyShiftG is GoToEnd
// (G/end). keyUp/keyDown drive single-row navigation.
var (
	keyG      = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	keyShiftG = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
	keyUp     = tea.KeyMsg{Type: tea.KeyUp}
	keyDown   = tea.KeyMsg{Type: tea.KeyDown}
)

// selectedHeader reports whether the cursor currently rests on a HeaderItem.
func selectedHeader(m Model) bool {
	_, ok := m.sessionList.SelectedItem().(HeaderItem)
	return ok
}

// TestCursorLandsOnlyOnSessionInstances locks the post-overflow-fix
// selection/cursor contract: group headers are now REAL non-selectable list
// rows, and the cursor skips them so it only ever rests on a session instance —
// on initial load, on g/G, and on ordinary up/down navigation across group
// boundaries. selectedSessionItem resolves whichever instance the cursor sits
// on to its underlying tmux.Session.
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
		items := buildByProject(sessions, project.NewIndex(projects))

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
		// One session per project => slice is [H(Alpha), alpha-1, H(Bravo), bravo-1].
		sessions := []tmux.Session{
			{Name: "alpha-1", Dir: dirA},
			{Name: "bravo-1", Dir: dirB},
		}
		items := buildByProject(sessions, project.NewIndex(projects))
		m := newCursorTestModel(t, items)

		// From alpha-1, one Down would land on H(Bravo); the skip must carry it
		// through to bravo-1.
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
		items := buildByProject(sessions, project.NewIndex(projects))
		m := newCursorTestModel(t, items)

		// Move to the last row (bravo-1), then Up: it must skip H(Bravo) back to alpha-1.
		updated, _ := m.Update(keyShiftG)
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

	t.Run("it lands g/G on the first and last session row, never a header", func(t *testing.T) {
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
		items := buildByProject(sessions, project.NewIndex(projects))
		rows := sessionRows(items)

		m := newCursorTestModel(t, items)

		// G → GoToEnd: last list item is a session row.
		updated, _ := m.Update(keyShiftG)
		m = updated.(Model)
		if selectedHeader(m) {
			t.Fatalf("after G the cursor rests on a header")
		}
		last, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false after G")
		}
		if last.Session.Name != rows[len(rows)-1].Session.Name {
			t.Errorf("after G, selected = %q, want %q (last session row)", last.Session.Name, rows[len(rows)-1].Session.Name)
		}

		// g → GoToStart: lands on the leading header, which the skip carries to
		// the first session row.
		updated, _ = m.Update(keyG)
		m = updated.(Model)
		if selectedHeader(m) {
			t.Fatalf("after g the cursor rests on a header")
		}
		first, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false after g")
		}
		if first.Session.Name != rows[0].Session.Name {
			t.Errorf("after g, selected = %q, want %q (first session row)", first.Session.Name, rows[0].Session.Name)
		}
	})

	t.Run("it resolves two By-Tag instances of one session to the same underlying session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{
			{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}},
		}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		items := buildByTag(sessions, project.NewIndex(projects))
		rows := sessionRows(items)
		if len(rows) != 2 {
			t.Fatalf("expected 2 instances (one per tag), got %d", len(rows))
		}

		// The two instances must be distinct list views (different GroupKey,
		// i.e. distinct canonical tags) of the same underlying session.
		if rows[0].GroupKey == rows[1].GroupKey {
			t.Fatalf("expected distinct tags on the two instances, got %q and %q", rows[0].GroupKey, rows[1].GroupKey)
		}

		m := newCursorTestModel(t, items)

		// Select each session row directly (skip the header indices).
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
		items := buildByProject(sessions, project.NewIndex(projects))

		m := newCursorTestModel(t, items)
		m.sessionList.Select(0) // force onto the leading header

		if _, ok := m.selectedSessionItem(); ok {
			t.Errorf("selectedSessionItem ok=true on a header row, want false")
		}
	})
}
