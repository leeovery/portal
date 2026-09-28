package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/project"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func openKillModal(m *Model, name string) {
	m.modal = modalKillConfirm
	m.pendingKillName = name
	m.pendingKillWindows = 1
}

func TestModalBlankScreen_ClearsListRowsBehindModal(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)

	live := m.viewSessionList()
	for _, row := range []string{"alpha", "bravo", "charlie", "Sessions"} {
		if !strings.Contains(live, row) {
			t.Fatalf("pre-modal sanity: live view should contain %q, got:\n%s", row, live)
		}
	}

	openKillModal(&m, "alpha")
	view := m.viewSessionList()

	if !strings.Contains(view, "Kill session?") {
		t.Errorf("modal view should contain the kill panel header, got:\n%s", view)
	}
	if !strings.Contains(view, "alpha") {
		t.Errorf("modal view should contain the kill target name 'alpha', got:\n%s", view)
	}
	for _, gone := range []string{"bravo", "charlie", "Sessions"} {
		if strings.Contains(view, gone) {
			t.Errorf("modal view should NOT contain list/header text %q (page must be cleared), got:\n%s", gone, view)
		}
	}
}

func TestModalBlankScreen_CentresPanelUsingTerminalDims(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)
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

func TestModalBlankScreen_PaintsOwnedCanvasBackdrop(t *testing.T) {
	forEachCanvasMode(t, func(t *testing.T, mode theme.Member) {
		const w, h = 90, 24
		m := newCanvasTestModel(t, w, h, mode)
		openKillModal(&m, "alpha")

		frame := m.View().Content
		if seq := canvasSeq(t, themeForAppearance(t, mode)); !strings.Contains(frame, seq) {
			t.Errorf("modal frame does not contain the canvas background sequence %q (backdrop must be the owned canvas)", seq)
		}
	})
}

func TestModalBlankScreen_ColourlessClearsToNativeBg(t *testing.T) {
	const w, h = 90, 24
	m := New(fakeLister{}, WithColourless(true))
	m.termWidth = w
	m.termHeight = h
	m.applySessions([]tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
	}, nil)
	openKillModal(&m, "alpha")

	frame := m.View().Content

	if got := lipgloss.Height(frame); got != h {
		t.Errorf("colourless modal frame height = %d, want exactly %d", got, h)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(frame, seq) {
		t.Errorf("colourless modal frame must not contain the dark canvas SGR %q (native bg only)", seq)
	}
	if seq := canvasSeq(t, testLightTheme(t)); strings.Contains(frame, seq) {
		t.Errorf("colourless modal frame must not contain the light canvas SGR %q (native bg only)", seq)
	}
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

func TestModalBlankScreen_ZeroDimsFallback(t *testing.T) {
	m := newCanvasTestModel(t, 0, 0, theme.MemberDark)
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

func TestModalBlankScreen_NoFlashBandLeaksIntoClearedView(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)
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

func TestModalBlankScreen_ProjectsDeleteClearsList(t *testing.T) {
	const w, h = 90, 24
	m := New(fakeLister{}, WithCanvasMode(theme.MemberDark))
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
	m.pendingDeletePath = "/home/user/code/keep"
	view := m.viewProjectList()

	if !strings.Contains(view, "Delete project?") {
		t.Errorf("projects delete modal should contain the '▲ Delete project?' header, got:\n%s", view)
	}
	if !strings.Contains(view, "proj-keep") {
		t.Errorf("projects delete modal should contain the target name 'proj-keep', got:\n%s", view)
	}
	if strings.Contains(view, "proj-other") {
		t.Errorf("projects delete modal should NOT contain other project rows (page cleared), got:\n%s", view)
	}
}
