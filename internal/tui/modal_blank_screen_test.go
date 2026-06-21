package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// openKillModal puts the model into the kill-confirm modal for the named session
// so the blank-screen render path is exercised without driving the key handler
// (handler parity is covered by the dispatch/parity tests; these tests target the
// render shell only).
func openKillModal(m *Model, name string) {
	m.modal = modalKillConfirm
	m.pendingKillName = name
	m.pendingKillWindows = 1
}

// TestModalBlankScreen_ClearsListRowsBehindModal asserts the §8.1/13.5 blank-
// screen modal layer: when a modal is open the composed Sessions view no longer
// renders the live list behind the panel — the session row names and the section
// header are gone, replaced by the cleared owned canvas with the centred panel on
// it. This is the visible effect of the plumbing task.
func TestModalBlankScreen_ClearsListRowsBehindModal(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)

	// Sanity: the live (no-modal) view DOES show the list rows and header, so the
	// assertions below prove the modal cleared them (not that they were never
	// there).
	live := m.viewSessionList()
	for _, row := range []string{"alpha", "bravo", "charlie", "Sessions"} {
		if !strings.Contains(live, row) {
			t.Fatalf("pre-modal sanity: live view should contain %q, got:\n%s", row, live)
		}
	}

	openKillModal(&m, "alpha")
	view := m.viewSessionList()

	// The panel copy is present: the §8.3 header and the kill target name (alpha
	// legitimately appears in the panel body)...
	if !strings.Contains(view, "Kill session?") {
		t.Errorf("modal view should contain the kill panel header, got:\n%s", view)
	}
	if !strings.Contains(view, "alpha") {
		t.Errorf("modal view should contain the kill target name 'alpha', got:\n%s", view)
	}
	// ...but the OTHER session rows and the section header behind it are gone.
	for _, gone := range []string{"bravo", "charlie", "Sessions"} {
		if strings.Contains(view, gone) {
			t.Errorf("modal view should NOT contain list/header text %q (page must be cleared), got:\n%s", gone, view)
		}
	}
}

// TestModalBlankScreen_CentresPanelUsingTerminalDims asserts the panel is centred
// horizontally and vertically on the content region derived from the terminal
// dimensions (reusing the existing centre maths sized to contentW/contentH). The
// fully-composed frame (View → fillCanvas) is exactly termW × termH and the panel
// border sits roughly centred, not flush at the top-left.
func TestModalBlankScreen_CentresPanelUsingTerminalDims(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)
	openKillModal(&m, "alpha")

	frame := m.View().Content

	if got := lipgloss.Height(frame); got != h {
		t.Errorf("composed modal frame height = %d, want exactly %d", got, h)
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("composed modal frame line %d width = %d, want exactly %d", i, lw, w)
		}
	}

	// The panel's top border must NOT be on the first row and must NOT be on the
	// last row — it is centred vertically with canvas above and below.
	topBorderRow := -1
	for i, line := range lines {
		if strings.ContainsAny(line, "╭┌") {
			topBorderRow = i
			break
		}
	}
	if topBorderRow <= 0 {
		t.Fatalf("modal top border should be centred (not on the first row), found at row %d:\n%s", topBorderRow, frame)
	}
	if topBorderRow >= h-1 {
		t.Errorf("modal top border at row %d is not vertically centred (h=%d)", topBorderRow, h)
	}
}

// TestModalBlankScreen_PaintsOwnedCanvasBackdrop asserts the cleared backdrop is
// the owned mode-matched canvas (§1) — the canvas background SGR is present in the
// composed frame — and that it is inherited from the Phase 1 fillCanvas primitive
// (no list rows). Verified for both dark and light canvases.
func TestModalBlankScreen_PaintsOwnedCanvasBackdrop(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const w, h = 90, 24
			m := newCanvasTestModel(t, w, h, tc.mode)
			openKillModal(&m, "alpha")

			frame := m.View().Content
			if seq := canvasSeq(t, tc.mode); !strings.Contains(frame, seq) {
				t.Errorf("modal frame does not contain the canvas background sequence %q (backdrop must be the owned canvas)", seq)
			}
		})
	}
}

// TestModalBlankScreen_ColourlessClearsToNativeBg asserts the §2.5 NO_COLOR
// carve-out is INHERITED: under colourless the cleared backdrop is the terminal
// native bg with no painted canvas SGR, via the same fillCanvas → fillColourless
// path that suppresses the canvas elsewhere (no second NO_COLOR branch in the
// modal shell). The panel is still centred and the frame is full-size.
func TestModalBlankScreen_ColourlessClearsToNativeBg(t *testing.T) {
	const w, h = 90, 24
	m := New(fakeLister{}, WithColourless(true))
	m.termWidth = w
	m.termHeight = h
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
	})
	openKillModal(&m, "alpha")

	frame := m.View().Content

	if got := lipgloss.Height(frame); got != h {
		t.Errorf("colourless modal frame height = %d, want exactly %d", got, h)
	}
	// No painted canvas: neither the dark nor light canvas SGR may appear.
	if seq := canvasSeq(t, theme.Dark); strings.Contains(frame, seq) {
		t.Errorf("colourless modal frame must not contain the dark canvas SGR %q (native bg only)", seq)
	}
	if seq := canvasSeq(t, theme.Light); strings.Contains(frame, seq) {
		t.Errorf("colourless modal frame must not contain the light canvas SGR %q (native bg only)", seq)
	}
	// The panel copy is still present and the list rows are still cleared.
	if !strings.Contains(frame, "Kill session?") {
		t.Errorf("colourless modal frame should contain the kill panel header, got:\n%s", frame)
	}
	if !strings.Contains(frame, "alpha") {
		t.Errorf("colourless modal frame should contain the kill target name 'alpha', got:\n%s", frame)
	}
	if strings.Contains(frame, "bravo") {
		t.Errorf("colourless modal frame should NOT contain list row 'bravo' (page cleared), got:\n%s", frame)
	}
}

// TestModalBlankScreen_ZeroDimsFallback asserts the edge case: pre-first-
// WindowSizeMsg the cached terminal dimensions are 0, and the cleared canvas must
// fall back to 80×24 (never zero-sized) just like every other owned-canvas page.
func TestModalBlankScreen_ZeroDimsFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.Dark)
	openKillModal(&m, "alpha")

	frame := m.View().Content
	if got := lipgloss.Height(frame); got != 24 {
		t.Errorf("zero-size modal frame height = %d, want 24 fallback", got)
	}
	lines := strings.Split(frame, "\n")
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != 80 {
			t.Errorf("zero-size modal frame line %d width = %d, want 80 fallback", i, lw)
		}
	}
}

// TestModalBlankScreen_NoFlashBandLeaksIntoClearedView asserts the edge case: a
// flash band present before the modal opened must NOT leak into the cleared modal
// view (the page behind the modal is gone, band and all).
func TestModalBlankScreen_NoFlashBandLeaksIntoClearedView(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.Dark)
	const flash = "session \"x\" no longer exists"
	m.setFlash(flash)
	if !strings.Contains(m.viewSessionList(), flash) {
		t.Fatalf("pre-modal sanity: flash band %q should be visible before the modal opens", flash)
	}

	openKillModal(&m, "alpha")
	view := m.viewSessionList()
	if strings.Contains(view, flash) {
		t.Errorf("flash band %q leaked into the cleared modal view:\n%s", flash, view)
	}
}

// TestModalBlankScreen_ProjectsDeleteClearsList asserts the shared backdrop is
// inherited by the projects-page delete modal too (a single shared change every
// modal inherits): the project list rows behind it are cleared.
func TestModalBlankScreen_ProjectsDeleteClearsList(t *testing.T) {
	const w, h = 90, 24
	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	m.termWidth = w
	m.termHeight = h
	m.activePage = PageProjects
	projects := []project.Project{
		{Path: "/home/user/code/keep", Name: "proj-keep"},
		{Path: "/home/user/code/other", Name: "proj-other"},
	}
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())

	live := m.viewProjectList()
	if !strings.Contains(live, "proj-keep") {
		t.Fatalf("pre-modal sanity: projects view should contain a project row, got:\n%s", live)
	}

	m.modal = modalDeleteProject
	m.pendingDeleteName = "proj-keep"
	view := m.viewProjectList()

	if !strings.Contains(view, "Delete proj-keep?") {
		t.Errorf("projects delete modal should contain the delete panel copy, got:\n%s", view)
	}
	if strings.Contains(view, "proj-other") {
		t.Errorf("projects delete modal should NOT contain other project rows (page cleared), got:\n%s", view)
	}
}
