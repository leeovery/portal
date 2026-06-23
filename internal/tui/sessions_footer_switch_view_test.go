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
	// The STANDARD condensed footer renderer always carries the hint (it is item-count
	// independent), so renderSessionsFooter must contain it at any count.
	m := NewModelWithSessions(nil)
	m.termWidth = 120
	m.termHeight = 24

	footer := sessionsFooterString(m)
	if !strings.Contains(footer, "switch view") {
		t.Errorf("standard sessions footer must contain %q, got:\n%s", "switch view", footer)
	}

	// But §11.1: with ZERO sessions the rendered View() FULLY REPLACES the footer with
	// the empty-sessions footer (`n new in cwd · x projects · / filter · ? help`), so
	// `switch view` is intentionally ABSENT from the rendered empty-state View — the
	// empty-state footer is its own surface, not the standard footer with items hidden.
	view := m.View().Content
	if strings.Contains(view, "switch view") {
		t.Errorf("rendered empty-sessions View() must REPLACE the footer (no %q), got:\n%s", "switch view", view)
	}
	if !strings.Contains(view, "new in cwd") {
		t.Errorf("rendered empty-sessions View() must show the replaced empty-state footer, got:\n%s", view)
	}
}

// TestProjectsFooter_NoSwitchViewHint asserts the §6.3 condensed Projects footer
// (driven by the projectsKeymap descriptor, the single source of truth) shows the
// `x sessions` page toggle and never the Sessions-only `switch view` hint, and —
// post §12.2 — never the dropped `s/x` alias copy (x is the sole both-directions
// toggle). Asserted against the production footer renderer, not the retired
// three-column path.
func TestProjectsFooter_NoSwitchViewHint(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	m.activePage = PageProjects
	m.termWidth = 120

	footer := footerVisible(renderProjectsFooter(m.contentWidth(), m.canvasMode, m.colourless))
	if strings.Contains(footer, "switch view") {
		t.Errorf("projects footer must NOT contain %q, got:\n%s", "switch view", footer)
	}
	// §12.2: the Projects-side s alias is dropped — x is the sole page toggle, so
	// the legacy `s/x` copy must be gone.
	if strings.Contains(footer, "s/x") {
		t.Errorf("projects footer must NOT show the dropped %q alias copy, got:\n%s", "s/x", footer)
	}
	// x is the sole both-directions page toggle.
	if !strings.Contains(footer, "x sessions") {
		t.Errorf("projects footer must show %q, got:\n%s", "x sessions", footer)
	}
}

func TestCommandPendingFooter_NoSwitchViewHint(t *testing.T) {
	// command-pending lands on the projects page; the toggle is a
	// sessions-page action and must not leak into the §11.4 command-pending footer.
	m := flashModelWithSessions("alpha-row")
	m.activePage = PageProjects
	m.commandPending = true

	footer := footerVisible(renderCommandPendingFooter(m.contentWidth(), m.canvasMode, m.colourless))
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
