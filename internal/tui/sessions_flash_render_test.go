package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func lineIndexContaining(lines []string, substr string) int {
	for i, l := range lines {
		if strings.Contains(l, substr) {
			return i
		}
	}
	return -1
}

func renderedSessionLines(t *testing.T, m Model) []string {
	t.Helper()
	return strings.Split(m.View().Content, "\n")
}

func flashModelWithSessions(names ...string) Model {
	sessions := make([]Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, Session{Name: n, Answered: true, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	return m
}

func TestSessionsView_NoFlashRow_WhenFlashTextEmpty(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	if m.flashText != "" {
		t.Fatalf("setup invariant: want empty flashText, got %q", m.flashText)
	}

	got := m.View().Content
	header := m.renderHeader()
	listView := m.applySectionHeader(m.sessionList.View())
	footer := renderSessionsFooter(m.sessionsHelpKeymap(), m.contentWidth(), m.themeState.active, m.colourless)
	want := m.fillCanvas(lipgloss.JoinVertical(lipgloss.Left, header, listView, footer))
	if got != want {
		t.Errorf("View() with empty flashText must equal fillCanvas(header + section-headed list.View() + manual footer)\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestSessionsView_FlashRow_AppearsAboveSectionHeader(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "session \"alpha\" no longer exists"
	m.setFlash(flash)

	lines := renderedSessionLines(t, m)
	flashIdx := lineIndexContaining(lines, flash)
	if flashIdx < 0 {
		t.Fatalf("flash text %q not found in render:\n%s", flash, strings.Join(lines, "\n"))
	}
	titleIdx := lineIndexContaining(lines, "Sessions")
	if titleIdx < 0 {
		t.Fatalf("title %q not found in render:\n%s", "Sessions", strings.Join(lines, "\n"))
	}
	rowIdx := lineIndexContaining(lines, "alpha-row")
	if rowIdx < 0 {
		t.Fatalf("session row not found in render:\n%s", strings.Join(lines, "\n"))
	}
	if flashIdx >= titleIdx {
		t.Errorf("flash index %d must be < section-header index %d (band above the section header)", flashIdx, titleIdx)
	}
	if rowIdx <= titleIdx {
		t.Errorf("session row index %d should be > section-header index %d", rowIdx, titleIdx)
	}
}

func TestSessionsView_FlashActivation_ShiftsListDownByTwo(t *testing.T) {
	m := flashModelWithSessions("alpha-row")

	beforeLines := renderedSessionLines(t, m)
	beforeIdx := lineIndexContaining(beforeLines, "alpha-row")
	if beforeIdx < 0 {
		t.Fatalf("session row missing in baseline render")
	}

	m.setFlash("transient")
	afterLines := renderedSessionLines(t, m)
	afterIdx := lineIndexContaining(afterLines, "alpha-row")
	if afterIdx < 0 {
		t.Fatalf("session row missing in flash render")
	}

	if afterIdx-beforeIdx != 2 {
		t.Errorf("activation row shift: want +2 (band + blank), got %d (before=%d after=%d)",
			afterIdx-beforeIdx, beforeIdx, afterIdx)
	}
}

func TestSessionsView_FlashDeactivation_ShiftsListUpByTwo(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	m.setFlash("transient")

	withFlashLines := renderedSessionLines(t, m)
	withFlashIdx := lineIndexContaining(withFlashLines, "alpha-row")
	if withFlashIdx < 0 {
		t.Fatalf("session row missing in flash render")
	}

	m.clearFlash()
	clearedLines := renderedSessionLines(t, m)
	clearedIdx := lineIndexContaining(clearedLines, "alpha-row")
	if clearedIdx < 0 {
		t.Fatalf("session row missing in cleared render")
	}

	if withFlashIdx-clearedIdx != 2 {
		t.Errorf("deactivation row shift: want -2 (i.e. cleared idx + 2 == flash idx), got delta %d (flash=%d cleared=%d)",
			withFlashIdx-clearedIdx, withFlashIdx, clearedIdx)
	}
}

func TestSessionsView_FlashText_AppearsVerbatim(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = `session "weird-name with spaces" no longer exists`
	m.setFlash(flash)

	rendered := m.View().Content
	if !strings.Contains(rendered, flash) {
		t.Errorf("expected verbatim flash text %q in rendered output, got:\n%s", flash, rendered)
	}
}

func TestSessionsView_OnlyOneFlashRowAdded(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "__FLASH_MARKER_42__"

	baselineLines := renderedSessionLines(t, m)
	m.setFlash(flash)
	flashedLines := renderedSessionLines(t, m)

	if len(flashedLines) != len(baselineLines) {
		t.Errorf("flash insertion must not change the frame height (band absorbed under the fill): baseline=%d flashed=%d",
			len(baselineLines), len(flashedLines))
	}

	count := 0
	for _, l := range flashedLines {
		if strings.Contains(l, flash) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("flash text occurrences: want 1, got %d", count)
	}
}

func TestProjectsPage_FlashRendered(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "__PROJECTS_PAGE_FLASH__"
	m.setFlash(flash)
	m.activePage = PageProjects

	out := m.View().Content
	if !strings.Contains(out, flash) {
		t.Errorf("a flash raised on the Projects page must render there:\n%s", out)
	}
}

func TestLoadingPage_FlashTextNotRendered(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "__SHOULD_NOT_APPEAR_ON_LOADING__"
	m.setFlash(flash)

	m.activePage = PageLoading
	out := m.View().Content
	if strings.Contains(out, flash) {
		t.Errorf("flash text leaked onto Loading page render:\n%s", out)
	}
}
