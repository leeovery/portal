package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// newCursorTestModel builds a Model with a real, production-sized session list
// loaded with the supplied grouped slice. It drives the cursor/selection
// contract (task 2-6) against the genuine bubbles/list model and the extended
// 2-5 delegate — no toggle is wired, the grouped slice is loaded directly.
func newCursorTestModel(t *testing.T, items []list.Item) Model {
	t.Helper()
	m := Model{
		sessionList: newSessionList(nil),
		activePage:  PageSessions,
	}
	m.applySessionListSize(80, 24)
	m.sessionList.SetItems(items)
	return m
}

// keyG is the bubbles/list GoToStart binding (g/home); keyShiftG is GoToEnd
// (G/end). They drive cursor navigation over list items — all of which are
// SessionItems — so the cursor lands on a session instance, never a header.
var (
	keyG      = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}}
	keyShiftG = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}
)

// TestCursorLandsOnlyOnSessionInstances locks the selection/cursor contract:
// because group headings are render-layer separators and never list items,
// the initial cursor, g/G, and ordinary navigation all operate on the
// all-SessionItem slice; and selectedSessionItem resolves whichever instance
// the cursor sits on to its underlying tmux.Session. See spec § TUI Rendering
// & Toggle Behaviour → Group headers and § Item model → Selection/cursor
// contract.
func TestCursorLandsOnlyOnSessionInstances(t *testing.T) {
	t.Run("it places the initial cursor on the first session instance", func(t *testing.T) {
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

		if got := m.sessionList.Index(); got != 0 {
			t.Fatalf("initial Index() = %d, want 0", got)
		}
		si, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem returned ok=false on a populated list")
		}
		wantFirst := asSessionItem(t, items[0]).Session.Name
		if si.Session.Name != wantFirst {
			t.Errorf("initial selected session = %q, want %q (first instance)", si.Session.Name, wantFirst)
		}
	})

	t.Run("it lands g/G on the first and last session instance, never a header", func(t *testing.T) {
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
		if len(items) < 2 {
			t.Fatalf("need at least 2 items to exercise g/G, got %d", len(items))
		}

		m := newCursorTestModel(t, items)

		// Move the cursor off index 0 so G demonstrably travels.
		m.sessionList.Select(1)

		// G → GoToEnd: last list item, which is a session instance.
		updated, _ := m.Update(keyShiftG)
		m = updated.(Model)
		if got := m.sessionList.Index(); got != len(items)-1 {
			t.Fatalf("after G, Index() = %d, want %d (last item)", got, len(items)-1)
		}
		last, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false after G")
		}
		wantLast := asSessionItem(t, items[len(items)-1]).Session.Name
		if last.Session.Name != wantLast {
			t.Errorf("after G, selected = %q, want %q (last session instance)", last.Session.Name, wantLast)
		}

		// g → GoToStart: first list item, which is a session instance.
		updated, _ = m.Update(keyG)
		m = updated.(Model)
		if got := m.sessionList.Index(); got != 0 {
			t.Fatalf("after g, Index() = %d, want 0 (first item)", got)
		}
		first, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false after g")
		}
		wantFirst := asSessionItem(t, items[0]).Session.Name
		if first.Session.Name != wantFirst {
			t.Errorf("after g, selected = %q, want %q (first session instance)", first.Session.Name, wantFirst)
		}
	})

	t.Run("it resolves two By-Tag instances of one session to the same underlying session", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{
			{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}},
		}
		sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}
		items := buildByTag(sessions, project.NewIndex(projects))
		if len(items) != 2 {
			t.Fatalf("expected 2 instances (one per tag), got %d", len(items))
		}

		// The two instances must be distinct list views (different Tag/GroupKey)
		// of the same underlying session — proving multi-instance reachability.
		i0 := asSessionItem(t, items[0])
		i1 := asSessionItem(t, items[1])
		if i0.Tag == i1.Tag {
			t.Fatalf("expected distinct tags on the two instances, got %q and %q", i0.Tag, i1.Tag)
		}

		m := newCursorTestModel(t, items)

		m.sessionList.Select(0)
		first, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false at instance 0")
		}

		m.sessionList.Select(1)
		second, ok := m.selectedSessionItem()
		if !ok {
			t.Fatalf("selectedSessionItem ok=false at instance 1")
		}

		if first.Session.Name != second.Session.Name {
			t.Errorf("two By-Tag instances resolve to different sessions: %q vs %q",
				first.Session.Name, second.Session.Name)
		}
		if first.Session.Name != "portal-abc" {
			t.Errorf("resolved session = %q, want portal-abc", first.Session.Name)
		}
	})

	t.Run("it returns the underlying session from selectedSessionItem for the highlighted instance", func(t *testing.T) {
		dir := t.TempDir()
		projects := []project.Project{
			{Path: dir, Name: "Portal", Tags: []string{"alpha", "beta", "gamma"}},
		}
		sessions := []tmux.Session{{Name: "portal-xyz", Dir: dir}}
		items := buildByTag(sessions, project.NewIndex(projects))
		if len(items) != 3 {
			t.Fatalf("expected 3 instances (one per tag), got %d", len(items))
		}

		m := newCursorTestModel(t, items)

		// Every cursor position must resolve to the same underlying session,
		// carrying the exact tmux.Session of the highlighted instance.
		for idx := range items {
			m.sessionList.Select(idx)
			si, ok := m.selectedSessionItem()
			if !ok {
				t.Fatalf("selectedSessionItem ok=false at index %d", idx)
			}
			want := asSessionItem(t, items[idx])
			if si != want {
				t.Errorf("index %d: selectedSessionItem = %+v, want %+v", idx, si, want)
			}
			if si.Session != sessions[0] {
				t.Errorf("index %d: underlying Session = %+v, want %+v", idx, si.Session, sessions[0])
			}
		}
	})
}
