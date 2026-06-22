package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tmux"
)

// Tests for the Sessions-page inline-flash render contract (spec §
// Inline flash — feature-local infrastructure > Render). The flash row
// is conditionally inserted between the filter input (title row) and
// the Sessions list. When flashText is empty no row is reserved; when
// non-empty exactly one styled row carrying the verbatim text appears.

// lineIndexContaining returns the first index in lines that contains
// the given substring, or -1.
func lineIndexContaining(lines []string, substr string) int {
	for i, l := range lines {
		if strings.Contains(l, substr) {
			return i
		}
	}
	return -1
}

// renderedSessionLines returns the View output of m split on newlines.
func renderedSessionLines(t *testing.T, m Model) []string {
	t.Helper()
	return strings.Split(m.View().Content, "\n")
}

// flashModelWithSessions builds a Model on the Sessions page seeded with
// the given session names so the rendered list contains predictable
// substrings the tests can locate.
func flashModelWithSessions(names ...string) Model {
	sessions := make([]tmux.Session, 0, len(names))
	for _, n := range names {
		sessions = append(sessions, tmux.Session{Name: n, Windows: 1, Attached: false})
	}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	return m
}

func TestSessionsView_NoFlashRow_WhenFlashTextEmpty(t *testing.T) {
	// Baseline contract: with flashText empty, the Sessions page renders
	// the bubbles/list.View() output as the list section, with the §3.4
	// condensed keymap footer (see renderSessionsFooter) composed below
	// it via lipgloss.JoinVertical. No flash row is inserted and no
	// existing list chrome is replaced; only the condensed footer is added.
	//
	// The composed view is then wrapped by the single outer canvas fill (§1) as
	// the LAST layer in View(); the assertion compares against the same
	// fillCanvas wrap so it pins "no flash row, footer composed below" without
	// re-asserting the fill (covered by canvas_paint_test.go). The §3.1 header
	// block is composed FIRST (above the list), so it is part of the expected
	// composition.
	m := flashModelWithSessions("alpha-row")
	if m.flashText != "" {
		t.Fatalf("setup invariant: want empty flashText, got %q", m.flashText)
	}

	got := m.View().Content
	header := m.renderHeader()
	// The §3.2 / §4.2 section header replaces the plain bubbles/list title line in
	// the composed view (applySectionHeader), so the expected list section is the
	// list view with that same in-place title swap applied — no flash row, footer
	// composed below.
	listView := m.applySectionHeader(m.sessionList.View())
	footer := renderSessionsFooter(m.contentWidth(), m.canvasMode, m.colourless)
	want := m.fillCanvas(lipgloss.JoinVertical(lipgloss.Left, header, listView, footer))
	if got != want {
		t.Errorf("View() with empty flashText must equal fillCanvas(header + section-headed list.View() + manual footer)\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

// TestSessionsView_FlashRow_AppearsAboveSectionHeader asserts the §11 placement
// convention: the transient flash band sits directly under the title separator,
// ABOVE the section header (`Sessions`) — the section header + list shift down.
// (Pre-§11 the flash inserted BELOW the title row; §11 moved it above the section
// header as the shared notice-slot convention.)
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
	// §11: band ABOVE the section header, which is above the list rows.
	if flashIdx >= titleIdx {
		t.Errorf("flash index %d must be < section-header index %d (band above the section header)", flashIdx, titleIdx)
	}
	if rowIdx <= titleIdx {
		t.Errorf("session row index %d should be > section-header index %d", rowIdx, titleIdx)
	}
}

// TestSessionsView_FlashActivation_ShiftsListDownByTwo asserts the band slot
// consumes TWO rows on activation — the band PLUS its blank breathing row beneath
// it — so the list shifts down by two.
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

// TestSessionsView_FlashDeactivation_ShiftsListUpByTwo asserts the band slot
// releases both rows (band + blank) on clear, so the list shifts back up by two.
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

	// The flash band is inserted as a single row whose height is absorbed by the
	// list shrinking one row underneath the outer canvas fill (§1, the "list
	// height recompute underneath the fill") — so the rendered frame height does
	// NOT grow with the band; it re-pads to exactly termH. (Pre-canvas the band
	// was additive and overflowed termH by one; that overflow is the bug class
	// §3.5 / §4.1 forbids, so the frame height must stay constant now.)
	if len(flashedLines) != len(baselineLines) {
		t.Errorf("flash insertion must not change the frame height (band absorbed under the fill): baseline=%d flashed=%d",
			len(baselineLines), len(flashedLines))
	}

	// The flash text itself appears on exactly one line — exactly one band row.
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

func TestProjectsPage_FlashTextNotRendered(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "__SHOULD_NOT_APPEAR_ON_PROJECTS__"
	m.setFlash(flash)

	// Switch to Projects page; the flash row must not appear there.
	m.activePage = PageProjects
	out := m.View().Content
	if strings.Contains(out, flash) {
		t.Errorf("flash text leaked onto Projects page render:\n%s", out)
	}
}

func TestLoadingPage_FlashTextNotRendered(t *testing.T) {
	m := flashModelWithSessions("alpha-row")
	const flash = "__SHOULD_NOT_APPEAR_ON_LOADING__"
	m.setFlash(flash)

	// Switch to loading page; flash text must not appear there.
	m.activePage = PageLoading
	out := m.View().Content
	if strings.Contains(out, flash) {
		t.Errorf("flash text leaked onto Loading page render:\n%s", out)
	}
}
