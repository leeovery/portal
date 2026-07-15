package tui

import (
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
)

// pressM is the lowercase `m` key press that drives the §5 multi-select
// enter/toggle dispatch through updateSessionList.
var pressM = tea.KeyPressMsg{Code: 'm', Text: "m"}

// sessionRowIndices returns the item indices of the SessionItem rows (the
// selectable rows), skipping the injected HeaderItem separators, so a By-Tag
// test can place the cursor on a specific underlying-session row.
func sessionRowIndices(items []list.Item) []int {
	var idxs []int
	for i, it := range items {
		if _, ok := it.(SessionItem); ok {
			idxs = append(idxs, i)
		}
	}
	return idxs
}

// TestMultiSelectEnterMode covers mark-on-entry: the first `m` from the normal list
// turns the mode on AND marks the currently-highlighted row (alpha, index 0) as the
// first selection.
func TestMultiSelectEnterMode(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	if m.MultiSelectActive() {
		t.Fatalf("precondition: a fresh model must not be in multi-select mode")
	}

	m = pressSession(t, m, pressM)

	if !m.MultiSelectActive() {
		t.Errorf("m from the normal list must enter multi-select mode")
	}
	if got := m.SelectedSessionCount(); got != 1 {
		t.Errorf("entering multi-select must mark the highlighted row; count = %d, want 1", got)
	}
	if !m.IsSessionSelected("alpha") {
		t.Errorf("entering multi-select must mark the highlighted session alpha")
	}
}

// TestMultiSelectToggleIdempotent covers the idempotent toggle pair: entering marks
// the highlighted alpha, a second `m` on that row unmarks it (zero selected via
// double-`m`), and a third re-marks it.
func TestMultiSelectToggleIdempotent(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter mode: mark-on-entry marks the highlighted row (alpha, index 0).
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("alpha") || m.SelectedSessionCount() != 1 {
		t.Errorf("entering must mark the highlighted alpha; count = %d, want 1", m.SelectedSessionCount())
	}

	// Toggle the same row OFF (double-`m` returns to zero selected).
	m = pressSession(t, m, pressM)
	if m.IsSessionSelected("alpha") {
		t.Errorf("second m must unmark alpha (double-m returns to zero selected)")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("count after unmarking = %d, want 0", got)
	}

	// Toggle the same row back ON.
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("alpha") {
		t.Errorf("third m must re-mark alpha (idempotent toggle pair)")
	}
	if got := m.SelectedSessionCount(); got != 1 {
		t.Errorf("count after re-marking = %d, want 1", got)
	}
}

// TestMultiSelectByTagIdentity covers the By-Tag multi-membership edge: a session
// that spans multiple rows (one per tag) is marked/unmarked on its single
// underlying Session.Name, so a single toggle changes the count by exactly 1
// regardless of how many rows the session occupies.
func TestMultiSelectByTagIdentity(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work", "infra"}}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	rows := sessionRowIndices(m.sessionList.Items())
	if len(rows) != 2 {
		t.Fatalf("precondition: multi-tag session must span 2 rows; got %d", len(rows))
	}

	// Enter mode with the cursor on the FIRST of the two rows: mark-on-entry marks
	// the underlying portal-abc via that row.
	m.sessionList.Select(rows[0])
	m = pressSession(t, m, pressM)

	if !m.IsSessionSelected("portal-abc") {
		t.Errorf("entering on one By-Tag row must mark the underlying session portal-abc")
	}
	if got := m.SelectedSessionCount(); got != 1 {
		t.Errorf("count after marking one row of a 2-row session = %d, want 1 (identity keyed on Session.Name)", got)
	}

	// Toggle via the SECOND row of the SAME session — unmarks the single name.
	m.sessionList.Select(rows[1])
	m = pressSession(t, m, pressM)

	if m.IsSessionSelected("portal-abc") {
		t.Errorf("toggling the other row of the same session must unmark portal-abc")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("count after unmarking via the other row = %d, want 0", got)
	}
}

// TestMultiSelectHeaderRowNoop covers the header edge: entering multi-select while
// the cursor is on a non-selectable HeaderItem opens the mode with zero marked
// (selectedSessionItem ok=false), and a further `m` on the header is still a no-op.
func TestMultiSelectHeaderRowNoop(t *testing.T) {
	dir := t.TempDir()
	projects := []project.Project{{Path: dir, Name: "Portal", Tags: []string{"work"}}}
	sessions := []tmux.Session{{Name: "portal-abc", Dir: dir}}

	m := newRebuildTestModel(prefs.ModeByTag, sessions, projects)
	m.rebuildSessionList()

	// Force the cursor onto the leading (non-selectable) header BEFORE entering, so
	// mark-on-entry hits the ok=false path and opens the mode with zero marked.
	m.sessionList.Select(0)
	if _, isHeader := m.sessionList.SelectedItem().(HeaderItem); !isHeader {
		t.Fatalf("precondition: index 0 must be a HeaderItem")
	}

	m = pressSession(t, m, pressM)

	if !m.MultiSelectActive() {
		t.Errorf("m on a HeaderItem must still enter multi-select mode")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("entering on a HeaderItem must mark nothing; count = %d, want 0", got)
	}

	// A further m while still on the header is a no-op that leaves the set empty.
	m = pressSession(t, m, pressM)

	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("m on a HeaderItem must be a no-op; count = %d, want 0", got)
	}
	if !m.MultiSelectActive() {
		t.Errorf("m on a HeaderItem must not leave multi-select mode")
	}
}

// TestMultiSelectEscExitsAndClears covers the exit edge: Esc (filter not focused)
// leaves the mode and clears the whole selection set.
func TestMultiSelectEscExitsAndClears(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1},
		{Name: "bravo", Windows: 2},
	})

	// Enter mode: mark-on-entry marks the highlighted row.
	m = pressSession(t, m, pressM)
	if m.SelectedSessionCount() != 1 {
		t.Fatalf("precondition: expected one marked session before Esc")
	}

	updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)

	if m.MultiSelectActive() {
		t.Errorf("Esc must exit multi-select mode")
	}
	if got := m.SelectedSessionCount(); got != 0 {
		t.Errorf("Esc must clear the whole selection set; count = %d, want 0", got)
	}
}

// TestMultiSelectUppercaseMNoop covers the retired uppercase binding: `M`
// (Text "M") neither enters the mode nor toggles a mark (isRuneKey matches "m"
// only).
func TestMultiSelectUppercaseMNoop(t *testing.T) {
	pressUpperM := tea.KeyPressMsg{Code: 'M', Text: "M"}

	t.Run("M does not enter the mode from the normal list", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m = pressSession(t, m, pressUpperM)
		if m.MultiSelectActive() {
			t.Errorf("uppercase M must NOT enter multi-select mode")
		}
	})

	t.Run("M does not toggle a mark while in the mode", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m = pressSession(t, m, pressM) // enter with lowercase m (marks the highlighted alpha)
		before := m.SelectedSessionCount()
		m = pressSession(t, m, pressUpperM)
		if got := m.SelectedSessionCount(); got != before {
			t.Errorf("uppercase M must NOT toggle a mark; count = %d, want %d (unchanged)", got, before)
		}
	})
}
