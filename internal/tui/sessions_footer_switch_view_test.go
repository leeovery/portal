package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/prefs"
)

// Tests for the §3.4 condensed sessions footer "s switch view" / "x projects"
// hints. Both must appear on the sessions footer at all session counts
// (including zero) and on every session view, and must NOT appear on the
// projects page (where s/x already mean "go to sessions").

// sessionsFooterString renders the §3.4 condensed sessions footer for m.
func sessionsFooterString(m Model) string {
	return renderSessionsFooter(m.contentWidth(), m.canvasMode, m.colourless)
}

func TestSessionsFooter_ShowsSwitchViewHint(t *testing.T) {
	m := flashModelWithSessions("alpha-row", "beta-row")
	// The condensed footer shows all six Core keys at the reference terminal width
	// (the vhs capture is 1280px ≈ wide). At a narrow 80-col terminal the §2.7
	// truncation legitimately drops the lower-priority entries, so size the model
	// to the reference width for the full-content assertions.
	m.termWidth = 120

	footer := sessionsFooterString(m)
	if !strings.Contains(footer, "switch view") {
		t.Errorf("sessions footer must contain %q hint, got:\n%s", "switch view", footer)
	}
	if !strings.Contains(footer, "projects") {
		t.Errorf("sessions footer must contain %q hint, got:\n%s", "projects", footer)
	}

	// Also assert via the full rendered View() so the hint is wired through
	// the actual sessions page render path, not just the footer helper.
	view := m.View().Content
	if !strings.Contains(view, "switch view") {
		t.Errorf("rendered sessions View() must contain %q, got:\n%s", "switch view", view)
	}
}

func TestSessionsFooter_ShowsSwitchViewHintAtZeroSessions(t *testing.T) {
	// Footer is rendered independent of item count (viewSessionList composes
	// renderSessionsFooter regardless of session count), so the hint must be
	// present even with an empty list.
	m := NewModelWithSessions(nil)
	m.termWidth = 120
	m.termHeight = 24

	footer := sessionsFooterString(m)
	if !strings.Contains(footer, "switch view") {
		t.Errorf("sessions footer at zero sessions must contain %q, got:\n%s", "switch view", footer)
	}

	view := m.View().Content
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

// TestSessionsFooter_ShowsSwitchAndProjectsOnFlatView asserts s switch view and
// x projects render on the Flat view (they are NOT grouping-conditional — they
// appear on ALL session views, §3.4 / edge cases).
func TestSessionsFooter_ShowsSwitchAndProjectsOnFlatView(t *testing.T) {
	m := flashModelWithSessions("alpha-row", "beta-row")
	m.termWidth = 120
	m.sessionListMode = prefs.ModeFlat
	if m.sessionListMode != prefs.ModeFlat {
		t.Fatalf("precondition: want Flat view, got %v", m.sessionListMode)
	}

	footer := sessionsFooterString(m)
	for _, want := range []string{"switch view", "projects"} {
		if !strings.Contains(footer, want) {
			t.Errorf("Flat-view footer must contain %q, got:\n%s", want, footer)
		}
	}
}
