package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/theme"
)

func burstPendingModel(t *testing.T, names ...string) (Model, *spawntest.FakeAdapter) {
	t.Helper()
	m, adapter, _ := markedSupportedBurstModel(t, names)
	m.termWidth = 80
	m.termHeight = 24
	m.sessionList.Select(0)
	m.burstPending = true
	return m, adapter
}

func TestBurstInputLock_IgnoresSecondEnter(t *testing.T) {
	m, adapter := burstPendingModel(t, "alpha", "bravo", "charlie")
	pipeBefore := m.burstPipe

	updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = updated.(Model)

	if cmd != nil {
		t.Error("a second Enter while burst-pending must be swallowed (nil cmd, no re-dispatch)")
	}
	if len(adapter.Calls) != 0 {
		t.Errorf("a second Enter must open no window; adapter.Calls = %d, want 0", len(adapter.Calls))
	}
	if m.burstPipe != pipeBefore {
		t.Error("a second Enter must not create a new burst pipe")
	}
	if !m.BurstPending() {
		t.Error("burst must stay pending after the swallowed Enter")
	}
}

func TestBurstInputLock_IgnoresRowActions(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  tea.KeyPressMsg
	}{
		{"m (mark)", pressM},
		{"down (nav)", tea.KeyPressMsg{Code: tea.KeyDown}},
		{"up (nav)", tea.KeyPressMsg{Code: tea.KeyUp}},
		{"space (preview)", tea.KeyPressMsg{Code: tea.KeySpace}},
		{"slash (filter)", tea.KeyPressMsg{Code: '/', Text: "/"}},
		{"s (grouping)", tea.KeyPressMsg{Code: 's', Text: "s"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := burstPendingModel(t, "alpha", "bravo", "charlie")
			wantPage := m.activePage
			wantMode := m.sessionListMode
			wantCount := m.SelectedSessionCount()
			wantIndex := m.sessionList.Index()

			updated, cmd := m.updateSessionList(tc.key)
			m = updated.(Model)

			if cmd != nil {
				t.Errorf("%s while burst-pending must be swallowed (nil cmd)", tc.name)
			}
			if m.activePage != wantPage {
				t.Errorf("%s changed the active page %d → %d (must be inert)", tc.name, wantPage, m.activePage)
			}
			if m.sessionListMode != wantMode {
				t.Errorf("%s changed the grouping mode (must be inert)", tc.name)
			}
			if got := m.SelectedSessionCount(); got != wantCount {
				t.Errorf("%s changed the marked count %d → %d (must be inert)", tc.name, wantCount, got)
			}
			if got := m.sessionList.Index(); got != wantIndex {
				t.Errorf("%s moved the cursor %d → %d (must be inert)", tc.name, wantIndex, got)
			}
			if m.sessionList.FilterState() != list.Unfiltered {
				t.Errorf("%s focused the filter input (must be inert)", tc.name)
			}
			if !m.BurstPending() {
				t.Errorf("%s must leave the burst pending", tc.name)
			}
		})
	}
}

func TestBurstInputLock_CtrlCAndEscStayLive(t *testing.T) {
	t.Run("Ctrl-C routes to cancelBurst (does not quit)", func(t *testing.T) {
		m, _ := burstPendingModel(t, "alpha", "bravo")
		updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
		m = updated.(Model)
		if isQuitCmd(cmd) {
			t.Error("Ctrl-C while burst-pending must route to cancelBurst, NOT tea.Quit")
		}
		if !m.BurstPending() {
			t.Error("cancelBurst must keep the burst pending until the terminal event lands")
		}
	})

	t.Run("Esc routes to cancelBurst (does not exit multi-select)", func(t *testing.T) {
		m, _ := burstPendingModel(t, "alpha", "bravo")
		if !m.multiSelectMode {
			t.Fatal("precondition: model must be in multi-select mode")
		}
		updated, cmd := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
		m = updated.(Model)
		if isQuitCmd(cmd) {
			t.Error("Esc while burst-pending must not quit")
		}
		if !m.multiSelectMode {
			t.Error("Esc while burst-pending must route to cancelBurst, NOT exitMultiSelect")
		}
		if !m.BurstPending() {
			t.Error("cancelBurst must keep the burst pending until the terminal event lands")
		}
	})
}

func TestBurstInputLock_AdvancesOpeningCounter(t *testing.T) {
	m := NewModelWithSessions([]Session{{Name: "alpha", Answered: true}})
	m.termWidth = 80
	m.termHeight = 24
	m.burstPending = true
	m.burstTotal = 3
	m.burstDone = 0

	updated, _ := m.Update(spawnProgressMsg{Done: 1, Total: 2})
	m = updated.(Model)
	if got := m.BurstDone(); got != 1 {
		t.Errorf("BurstDone() = %d after the first progress msg, want 1", got)
	}
	if first := ansi.Strip(bannerFirstLine(m)); !strings.Contains(first, "Opening 1/3…") {
		t.Errorf("section-header row = %q, want it to contain %q", first, "Opening 1/3…")
	}

	updated, _ = m.Update(spawnProgressMsg{Done: 2, Total: 2})
	m = updated.(Model)
	if got := m.BurstDone(); got != 2 {
		t.Errorf("BurstDone() = %d after the second progress msg, want 2", got)
	}
	if first := ansi.Strip(bannerFirstLine(m)); !strings.Contains(first, "Opening 2/3…") {
		t.Errorf("section-header row = %q, want it to contain %q", first, "Opening 2/3…")
	}
}

func TestBurstInputLock_HoldsDenominatorAtN(t *testing.T) {
	m := NewModelWithSessions([]Session{{Name: "alpha", Answered: true}})
	m.termWidth = 80
	m.termHeight = 24
	m.burstPending = true
	m.burstTotal = 3
	m.burstDone = 0

	for done := 1; done <= 2; done++ {
		updated, _ := m.Update(spawnProgressMsg{Done: done, Total: 2})
		m = updated.(Model)
		if got := m.BurstTotal(); got != 3 {
			t.Errorf("BurstTotal() = %d after progress Done=%d Total=2, want it held at 3", got, done)
		}
		first := ansi.Strip(bannerFirstLine(m))
		if strings.Contains(first, "/2") {
			t.Errorf("section-header row = %q must not use the external denominator (/2)", first)
		}
	}
	if strings.Contains(ansi.Strip(bannerFirstLine(m)), "3/3") {
		t.Error("the Opening band must never reach 3/3 (the trigger self-attaches silently)")
	}
}

func TestBurstInputLock_OpeningBandPrecedence(t *testing.T) {
	newOpeningModel := func() Model {
		m := NewModelWithSessions([]Session{
			{Name: "alpha", Answered: true},
			{Name: "bravo", Answered: true},
			{Name: "charlie", Answered: true},
		})
		m.termWidth = 80
		m.termHeight = 24
		m.burstPending = true
		m.burstTotal = 3
		m.burstDone = 1
		return m
	}

	t.Run("outranks the multi-select banner", func(t *testing.T) {
		m := newOpeningModel()
		m.multiSelectMode = true
		m.selectedSessions = markedSet("alpha", "bravo", "charlie")

		first := ansi.Strip(bannerFirstLine(m))
		if !strings.Contains(first, "Opening 1/3…") {
			t.Errorf("burst-pending row = %q, want the Opening band", first)
		}
		if strings.Contains(first, "selected") {
			t.Errorf("burst-pending row = %q must NOT show the multi-select banner", first)
		}
	})

	t.Run("outranks the unsupported banner and the standard header", func(t *testing.T) {
		m := newOpeningModel()
		m.detectResolved = true
		m.detectResolution = spawn.ResolutionUnsupported
		m.detectIdentity = ghosttyIdentity()

		first := ansi.Strip(bannerFirstLine(m))
		if !strings.Contains(first, "Opening 1/3…") {
			t.Errorf("burst-pending row = %q, want the Opening band", first)
		}
		if strings.Contains(first, unsupportedLabel) {
			t.Errorf("burst-pending row = %q must NOT show the unsupported banner", first)
		}
		if strings.Contains(first, sectionLabel) {
			t.Errorf("burst-pending row = %q must NOT show the standard %q header", first, sectionLabel)
		}
	})

	t.Run("steps aside for the live filter input", func(t *testing.T) {
		m := newOpeningModel()
		m.sessionList.SetFilterState(list.Filtering)

		listView := m.sessionList.View()
		got := m.applySectionHeader(listView)
		if got != listView {
			t.Errorf("the live filter input must own the row (Opening band steps aside); got:\n%s", got)
		}
		if strings.Contains(ansi.Strip(bannerFirstLine(m)), "Opening") {
			t.Error("the Opening band must not render while the filter input is focused")
		}
	})
}

func TestOpeningBand_RendersVioletCounter(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		band := renderOpeningBand(1, 3, sectionHeaderWidth, th, false)
		if !strings.Contains(ansi.Strip(band), "Opening 1/3…") {
			t.Errorf("band must read %q:\n%s", "Opening 1/3…", ansi.Strip(band))
		}
		violet := headerStyle(th.AccentPrimary, th, false).Render("Opening 1/3…")
		if !strings.Contains(band, violet) {
			t.Errorf("band missing the accent.primary %q run:\n%s", "Opening 1/3…", band)
		}
		if got := lipgloss.Height(band); got != 1 {
			t.Errorf("band height = %d, want exactly 1 row:\n%s", got, band)
		}
		if got := lipgloss.Width(band); got != sectionHeaderWidth {
			t.Errorf("band width = %d, want exactly %d (flex spacer to content width)", got, sectionHeaderWidth)
		}
		if seq := canvasSeq(t, th); !strings.Contains(band, seq) {
			t.Errorf("band does not paint the canvas background sequence %q:\n%s", seq, band)
		}
	}
}

func TestOpeningBand_ColourlessDropsHueAndCanvas(t *testing.T) {
	band := renderOpeningBand(2, 3, sectionHeaderWidth, testDarkTheme(t), true)

	if !strings.Contains(band, "Opening 2/3…") {
		t.Errorf("colourless band dropped the text:\n%s", band)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(band, seq) {
		t.Errorf("colourless band still paints the canvas background sequence %q", seq)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); strings.Contains(band, seq) {
		t.Errorf("colourless band still emits the accent.primary foreground sequence %q", seq)
	}
}
