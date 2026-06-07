package tui

import (
	"strings"
	"testing"
)

// Tests for the sessions-page footer "s switch view" hint (spec § TUI
// Rendering & Toggle Behaviour → Mode indication, Toggle key). The hint
// must appear in the sessions-page manual keymap footer at all session
// counts (including zero), must NOT appear on the projects page (where s
// already means "go to sessions"), and the three-column footer layout
// must remain intact.

// sessionsFooterString renders the sessions-page manual keymap footer for m.
func sessionsFooterString(m Model) string {
	return renderKeymapFooter(&m.sessionList, sessionFooterBindings(&m.sessionList))
}

func TestSessionsFooter_ShowsSwitchViewHint(t *testing.T) {
	m := flashModelWithSessions("alpha-row", "beta-row")

	footer := sessionsFooterString(m)
	if !strings.Contains(footer, "switch view") {
		t.Errorf("sessions footer must contain %q hint, got:\n%s", "switch view", footer)
	}
	if !strings.Contains(footer, "s") {
		t.Errorf("sessions footer must advertise the %q key, got:\n%s", "s", footer)
	}

	// Also assert via the full rendered View() so the hint is wired through
	// the actual sessions page render path, not just the footer helper.
	view := m.View()
	if !strings.Contains(view, "switch view") {
		t.Errorf("rendered sessions View() must contain %q, got:\n%s", "switch view", view)
	}
}

func TestSessionsFooter_ShowsSwitchViewHintAtZeroSessions(t *testing.T) {
	// Footer is rendered independent of item count (viewSessionList composes
	// renderKeymapFooter regardless of session count), so the hint must be
	// present even with an empty list.
	m := NewModelWithSessions(nil)
	m.termWidth = 80
	m.termHeight = 24

	footer := sessionsFooterString(m)
	if !strings.Contains(footer, "switch view") {
		t.Errorf("sessions footer at zero sessions must contain %q, got:\n%s", "switch view", footer)
	}

	view := m.View()
	if !strings.Contains(view, "switch view") {
		t.Errorf("rendered sessions View() at zero sessions must contain %q, got:\n%s", "switch view", view)
	}
}

func TestProjectsFooter_UnchangedNoSwitchViewHint(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	m.activePage = PageProjects

	footer := renderKeymapFooter(&m.projectList, projectFooterBindings(&m.projectList, m.commandPending))
	if strings.Contains(footer, "switch view") {
		t.Errorf("projects footer must NOT contain %q, got:\n%s", "switch view", footer)
	}
	// Projects page still binds s/x → sessions.
	if !strings.Contains(footer, "s/x") {
		t.Errorf("projects footer must still show %q, got:\n%s", "s/x", footer)
	}
	if !strings.Contains(footer, "sessions") {
		t.Errorf("projects footer must still show %q, got:\n%s", "sessions", footer)
	}
}

func TestCommandPendingFooter_NoSwitchViewHint(t *testing.T) {
	// command-pending lands on the projects page; the toggle is a
	// sessions-page action and must not leak into command-pending mode.
	m := flashModelWithSessions("alpha-row")
	m.activePage = PageProjects
	m.commandPending = true

	footer := renderKeymapFooter(&m.projectList, projectFooterBindings(&m.projectList, true))
	if strings.Contains(footer, "switch view") {
		t.Errorf("command-pending footer must NOT contain %q, got:\n%s", "switch view", footer)
	}
}

func TestSessionsFooter_ThreeColumnLayoutIntact(t *testing.T) {
	// The new always-enabled binding must not break the three-column split.
	// chunkBindingsIntoThreeColumns filters disabled bindings then splits in
	// source order into columns of keymapFooterColumnSize; assert the new
	// binding lands within the three-column bound and the layout still
	// produces three columns of the fixed size with a (possibly short) tail.
	m := flashModelWithSessions("alpha-row", "beta-row")

	bindings := sessionFooterBindings(&m.sessionList)
	cols := chunkBindingsIntoThreeColumns(bindings)
	if len(cols) != 3 {
		t.Fatalf("expected exactly 3 columns, got %d", len(cols))
	}

	// No column may exceed the fixed per-column size.
	for i, c := range cols {
		if len(c) > keymapFooterColumnSize {
			t.Errorf("column %d has %d entries, exceeds keymapFooterColumnSize=%d", i, len(c), keymapFooterColumnSize)
		}
	}

	// The switch-view binding must be present among the enabled, chunked
	// bindings (i.e. it survived the disabled-filter and fits within the
	// three-column window).
	found := false
	for _, c := range cols {
		for _, b := range c {
			if b.Help().Desc == "switch view" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("switch view binding must appear within the three-column footer chunks")
	}
}
