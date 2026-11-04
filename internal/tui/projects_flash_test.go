package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
)

func newProjectsFlashModel(t *testing.T, w, h int, opts ...Option) Model {
	t.Helper()
	m := New(fakeLister{}, opts...)
	m.termWidth = w
	m.termHeight = h
	m.activePage = PageProjects
	projects := sampleProjects()
	m.setProjects(projects)
	m.projectList.SetItems(ProjectsToListItems(projects))
	m.applyProjectListSize(m.contentWidth(), m.contentHeight())
	return m
}

func newSessionsFlashPeer(t *testing.T, w, h int) Model {
	t.Helper()
	m := New(fakeLister{})
	m.termWidth = w
	m.termHeight = h
	m.applySessionListSize(m.contentWidth(), m.contentHeight())
	return m
}

func bandSlotRowCount(view string) int {
	n := 0
	for line := range strings.SplitSeq(ansi.Strip(view), "\n") {
		if strings.Contains(line, "Projects") {
			break
		}
		if strings.HasPrefix(line, noticeBarGlyph) {
			n++
		}
	}
	return n
}

func sectionHeaderRow(listView string) string {
	return strings.SplitN(listView, "\n", 2)[0]
}

func TestProjectsFlash_RendersInTheBandSlot(t *testing.T) {
	const flash = "__PROJECTS_FLASH__"
	m := newProjectsFlashModel(t, 90, 30)
	m.setFlash(flash)

	peer := newSessionsFlashPeer(t, 90, 30)
	peer.setFlash(flash)
	if got, want := m.renderActiveProjectNoticeBand(), peer.renderActiveNoticeBand(); got != want {
		t.Errorf("Projects flash band differs from the Sessions band for the same message/width:\n got %q\nwant %q", got, want)
	}

	lines := strings.Split(ansi.Strip(m.viewProjectList()), "\n")
	ruleIdx := -1
	for i, l := range lines {
		if strings.Contains(l, strings.Repeat(headerRuleGlyph, 4)) {
			ruleIdx = i
			break
		}
	}
	bandIdx := lineIndexContaining(lines, flash)
	sectionIdx := lineIndexContaining(lines, "Projects")
	if ruleIdx < 0 || bandIdx < 0 || sectionIdx < 0 {
		t.Fatalf("missing a landmark: rule=%d band=%d section=%d\n%s", ruleIdx, bandIdx, sectionIdx, strings.Join(lines, "\n"))
	}
	if bandIdx <= ruleIdx {
		t.Errorf("band index %d must be > separator-rule index %d (band under the separator)", bandIdx, ruleIdx)
	}
	if sectionIdx-bandIdx != 2 {
		t.Errorf("section header is %d rows below the band, want 2 (band → blank → section header)", sectionIdx-bandIdx)
	}
	if blank := lines[bandIdx+1]; strings.TrimSpace(blank) != "" {
		t.Errorf("row between the band and section header must be blank, got %q", blank)
	}
	if !strings.Contains(lines[bandIdx], flashWarningGlyph) {
		t.Errorf("Projects flash band missing the %q status glyph: %q", flashWarningGlyph, lines[bandIdx])
	}
}

func TestProjectsFlash_RecomputesListHeight(t *testing.T) {
	m := newProjectsFlashModel(t, 90, 30)
	base := m.projectList.Height()

	m.setFlash("__HEIGHT_FLASH__")
	slot := m.projectBandHeight()
	if slot < 2 {
		t.Fatalf("Projects flash slot height = %d, want >=2 (band + blank breathing row)", slot)
	}
	if got, want := m.projectList.Height(), base-slot; got != want {
		t.Errorf("project list height with the flash = %d, want %d (base %d − slot %d)", got, want, base, slot)
	}

	m.clearFlash()
	if got := m.projectBandHeight(); got != 0 {
		t.Errorf("projectBandHeight after clear = %d, want 0", got)
	}
	if got := m.projectList.Height(); got != base {
		t.Errorf("project list height after clear = %d, want the base %d (slot released)", got, base)
	}
}

func TestProjectsFlash_WinsTheSlotOverCommandPending(t *testing.T) {
	const flash = "__OUTRANKS__"
	m := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})

	role, _, ok := m.activeProjectNoticeBand()
	if !ok || role != bandCommand {
		t.Fatalf("baseline: activeProjectNoticeBand = (role %v, ok %v), want (bandCommand, true)", role, ok)
	}

	m.setFlash(flash)
	role, message, ok := m.activeProjectNoticeBand()
	if !ok {
		t.Fatalf("no band arbitrated while a flash is live")
	}
	if role != bandWarning || message != flash {
		t.Errorf("arbitrated band = (role %v, message %q), want (bandWarning, %q) — the flash outranks the banner", role, message, flash)
	}

	m.clearFlash()
	role, _, ok = m.activeProjectNoticeBand()
	if !ok || role != bandCommand {
		t.Errorf("after the flash clears: activeProjectNoticeBand = (role %v, ok %v), want the banner back (bandCommand, true)", role, ok)
	}
}

func TestProjectsFlash_SingleSlot(t *testing.T) {
	const flash = "__SINGLE_SLOT__"
	m := newCommandPendingTestModel(t, 90, 30, sampleProjects(), []string{"npm", "run", "dev"})
	m.setFlash(flash)

	view := m.viewProjectList()
	visible := ansi.Strip(view)
	if !strings.Contains(visible, flash) {
		t.Errorf("flash must render while it holds the slot:\n%s", visible)
	}
	for _, banned := range []string{commandBandText, commandBandCaret, "npm run dev"} {
		if strings.Contains(visible, banned) {
			t.Errorf("command-pending banner element %q must NOT co-render with the flash:\n%s", banned, visible)
		}
	}
	if got, want := bandSlotRowCount(view), lipgloss.Height(m.renderActiveProjectNoticeBand()); got != want {
		t.Errorf("notice slot carries %d `%s` band rows, want %d (exactly one arbitrated band)", got, noticeBarGlyph, want)
	}
	if got, want := lipgloss.Height(m.renderProjectBandSlot()), lipgloss.Height(m.renderActiveProjectNoticeBand())+1; got != want {
		t.Errorf("Projects band slot height = %d, want %d (one band + one blank row)", got, want)
	}
}

func TestProjectsFlash_ActionableKeyClearsAndFallsThrough(t *testing.T) {
	t.Run("x clears the flash and still switches to Sessions", func(t *testing.T) {
		m := projectsDispatchModel(t)
		m.setFlash("__CLEAR_ME__")

		m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'x', Text: "x"})
		if m.flashText != "" {
			t.Errorf("actionable key must clear the flash: flashText = %q, want empty", m.flashText)
		}
		if m.activePage != PageSessions {
			t.Errorf("x must still fall through to the page switch: active page = %d, want PageSessions", m.activePage)
		}
	})

	t.Run("e clears the flash and still opens the edit modal", func(t *testing.T) {
		m := projectsDispatchModel(t)
		m.setFlash("__CLEAR_ME__")

		m, _ = pressProject(t, m, tea.KeyPressMsg{Code: 'e', Text: "e"})
		if m.flashText != "" {
			t.Errorf("actionable key must clear the flash: flashText = %q, want empty", m.flashText)
		}
		if m.modal != modalEditProject {
			t.Errorf("e must still fall through to the edit modal: modal = %v, want modalEditProject", m.modal)
		}
	})
}

func TestProjectsFlash_SurvivesWindowSize(t *testing.T) {
	const flash = "__SURVIVES__"
	for _, tc := range []struct {
		name string
		msg  tea.Msg
	}{
		{"window size", tea.WindowSizeMsg{Width: 100, Height: 40}},
		{"focus", tea.FocusMsg{}},
		{"blur", tea.BlurMsg{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newProjectsFlashModel(t, 90, 30)
			m.setFlash(flash)

			updated, _ := m.Update(tc.msg)
			mm, ok := updated.(Model)
			if !ok {
				t.Fatalf("Update returned %T, want tui.Model", updated)
			}
			if mm.flashText != flash {
				t.Errorf("flashText after %s: got %q, want %q (non-key events never clear)", tc.name, mm.flashText, flash)
			}
		})
	}
}

func TestProjectsFlash_PageEnteredByMessageIsSized(t *testing.T) {
	m := newProjectsFlashModel(t, 90, 30)
	base := m.projectList.Height()

	m.activePage = PageSessions
	m.setFlash("__ENTERED_BY_MESSAGE__")
	m.activePage = PageProjects

	slot := m.projectBandHeight()
	if slot < 2 {
		t.Fatalf("Projects flash slot height = %d, want >=2 (band + blank breathing row)", slot)
	}
	if got, want := m.projectList.Height(), base-slot; got != want {
		t.Errorf("project list height after a flash raised off-page = %d, want %d (base %d − slot %d)", got, want, base, slot)
	}
}

func TestProjectsFlash_ColdBootWarningsLandOnASizedProjectsFrame(t *testing.T) {
	const warningLine = "the session saver is not running"

	var model tea.Model = New(postloadStubLister{},
		WithBootstrapping(true),
		WithProgressReceiver(func() tea.Msg { return nil }),
		WithProjectStore(stubProjectStore{}),
	)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(ProjectsLoadedMsg{Projects: oneProjectLoaded()})

	model, _ = model.Update(LoadingMinElapsedMsg{})
	model, _ = model.Update(BootstrapCompleteMsg{Warnings: []BootstrapWarning{{Lines: []string{warningLine}}}})
	interim := model.(Model)
	if interim.ActivePage() != PageSessions {
		t.Fatalf("route invariant: expected PageSessions after both loading gates, got %v", interim.ActivePage())
	}
	if interim.flashText == "" {
		t.Fatal("route invariant: expected the post-load warnings flash to be live on Sessions")
	}

	model, _ = interim.Update(SessionsMsg{})
	final := model.(Model)
	if final.ActivePage() != PageProjects {
		t.Fatalf("route invariant: a zero-session cold boot must land on PageProjects, got %v", final.ActivePage())
	}
	if final.flashText == "" {
		t.Fatal("route invariant: a SessionsMsg is not a keypress — the flash must still be live on the Projects landing")
	}

	if got, want := lipgloss.Height(final.viewProjectList()), final.contentHeight(); got > want {
		t.Errorf("composed Projects view is %d rows, want <= %d (the content region fillCanvas clamps to): the list was not sized for the flash band it renders\n%s",
			got, want, ansi.Strip(final.viewProjectList()))
	}
}

func TestProjectsFlash_TickClearsWithGenerationGuard(t *testing.T) {
	t.Run("a matching tick clears the flash", func(t *testing.T) {
		m := newProjectsFlashModel(t, 90, 30)
		m.setFlash("timeout-me")

		updated, _ := m.Update(flashTickMsg{Gen: m.flashGen})
		mm := updated.(Model)
		if mm.flashText != "" {
			t.Errorf("matching tick must clear the Projects flash: flashText = %q, want empty", mm.flashText)
		}
	})

	t.Run("a superseded tick does not clear a newer flash", func(t *testing.T) {
		m := newProjectsFlashModel(t, 90, 30)
		m.setFlash("first")
		m.setFlash("second")

		updated, _ := m.Update(flashTickMsg{Gen: 1})
		mm := updated.(Model)
		if mm.flashText != "second" {
			t.Errorf("superseded tick must not early-clear: flashText = %q, want %q", mm.flashText, "second")
		}
	})
}

func TestProjectsFlash_OnlyTheFlashContender(t *testing.T) {
	m := newProjectsFlashModel(t, 90, 30, WithInitialBurstOpening(1, 3))
	m.byTagSignpost = true
	m.multiSelectMode = true
	m.selectedSessions = map[string]struct{}{"alpha": {}}
	m.detectResolved = true
	m.detectResolution = spawn.ResolutionUnsupported
	m.detectIdentity = appleTerminalIdentity()

	if _, _, ok := m.activeProjectNoticeBand(); ok {
		t.Errorf("no Sessions-only contender may claim the Projects slot")
	}
	view := m.viewProjectList()
	if slot := m.renderProjectBandSlot(); slot != "" {
		t.Errorf("Projects band slot must be empty with no flash live, got %q", slot)
	}
	if got := bandSlotRowCount(view); got != 0 {
		t.Errorf("Projects notice slot carries %d `%s` band rows with no flash live, want 0", got, noticeBarGlyph)
	}
	for _, banned := range []string{byTagSignpostText, "selected", multiSelectCancelHint, unsupportedDocsHint, "Opening"} {
		if strings.Contains(ansi.Strip(view), banned) {
			t.Errorf("Sessions-only contender copy %q leaked onto the Projects page:\n%s", banned, ansi.Strip(view))
		}
	}
}

func TestProjectsFlash_Colourless(t *testing.T) {
	for _, tc := range []struct {
		name  string
		set   func(*Model, string)
		glyph string
	}{
		{"warning", (*Model).setFlash, flashWarningGlyph},
		{"success", (*Model).setSuccessFlash, flashSuccessGlyph},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newProjectsFlashModel(t, 90, 30, WithColourless(true))
			tc.set(&m, "__COLOURLESS__")

			band := m.renderActiveProjectNoticeBand()
			if band != ansi.Strip(band) {
				t.Errorf("NO_COLOR Projects band must carry no SGR colour sequences; got raw %q", band)
			}
			if !strings.HasPrefix(band, noticeBarGlyph) {
				t.Errorf("NO_COLOR Projects band must keep the %q left-bar: %q", noticeBarGlyph, band)
			}
			if !strings.Contains(band, tc.glyph) {
				t.Errorf("NO_COLOR Projects band must keep the %q status glyph: %q", tc.glyph, band)
			}
		})
	}
}

func TestProjectsFlash_SessionsBandUnchanged(t *testing.T) {
	const flash = "__SESSIONS_UNCHANGED__"

	t.Run("the multi-select co-render exception survives", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		m.multiSelectMode = true
		m.setFlash(flash)

		role, message, ok := m.activeNoticeBand()
		if !ok || role != bandWarning || message != flash {
			t.Errorf("Sessions arbiter = (role %v, message %q, ok %v), want the flash co-rendering with multi-select", role, message, ok)
		}
	})

	t.Run("the Sessions list still reserves the slot it always did", func(t *testing.T) {
		m := noticeBandModel("alpha-row")
		_, base := m.SessionListSize()

		m.setFlash(flash)
		slot := m.sessionBandHeight()
		if _, got := m.SessionListSize(); got != base-slot {
			t.Errorf("Sessions list height with the flash = %d, want %d (base %d − slot %d)", got, base-slot, base, slot)
		}

		m.clearFlash()
		if _, got := m.SessionListSize(); got != base {
			t.Errorf("Sessions list height after clear = %d, want the base %d", got, base)
		}
	})
}

func TestProjectsFlash_FilterHeaderPrecedenceUnchanged(t *testing.T) {
	const query = "portal"
	m := newProjectsFlashModel(t, 90, 30)
	m, _ = pressProject(t, m, tea.KeyPressMsg{Code: '/', Text: "/"})
	for _, r := range query {
		m, _ = pressProject(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m, _ = pressProject(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.projectList.FilterState() != list.FilterApplied {
		t.Fatalf("precondition: project filter state = %v, want FilterApplied", m.projectList.FilterState())
	}
	baseline := sectionHeaderRow(m.applyProjectsSectionHeader(m.projectList.View()))

	m.setFlash("__FILTER_PRECEDENCE__")
	if got := sectionHeaderRow(m.applyProjectsSectionHeader(m.projectList.View())); got != baseline {
		t.Errorf("a flash must not change the Projects filter header row (the filter-line flip is a separate concern):\n got %q\nwant %q", got, baseline)
	}
	if !strings.Contains(ansi.Strip(m.viewProjectList()), "__FILTER_PRECEDENCE__") {
		t.Errorf("the flash must still render in the band slot beneath an applied filter:\n%s", ansi.Strip(m.viewProjectList()))
	}
}
