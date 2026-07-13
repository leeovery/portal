package tui

// restore-host-terminal-windows-6-5 — input-lock while a burst is pending + the
// `Opening n/N…` in-burst feedback band.
//
// These white-box (package tui) tests drive the §6-5 behaviour: while burstPending
// the picker is INERT to row actions (a second Enter, m, navigation, Space, /, s
// are swallowed) and only Ctrl-C / Esc stay live (routed to cancelBurst — §6-8);
// each streamed spawnProgressMsg advances the Opening counter while the denominator
// holds at N; and the `Opening n/N…` band owns the section-header row with
// precedence just below the live filter input (above the multi-select and
// unsupported banners), surviving NO_COLOR.
//
// No t.Parallel: consistent with the rest of the tui test surface.

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// burstPendingModel builds a resolved-supported multi-select model with every named
// session marked and burst-pending FORCED true (without a real async dispatch — so
// the input-lock is exercised in isolation, no goroutine, no pipe). The cursor is
// reset to the first row so a navigation key would move it if it were not swallowed.
// A subsequent Enter would dispatch a burst if it reached handleMultiSelectEnter, so
// a swallowed key proves the input-lock intercepts ahead of every handler.
func burstPendingModel(t *testing.T, names ...string) (Model, *spawntest.FakeAdapter) {
	t.Helper()
	sessions := make([]tmux.Session, len(names))
	for i, n := range names {
		sessions[i] = tmux.Session{Name: n, Windows: i + 1}
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	m.termWidth = 80
	m.termHeight = 24
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, allPresent, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = pressSession(t, m, pressM)
	for i := range names {
		m = markRow(t, m, i)
	}
	m.sessionList.Select(0)
	m.burstPending = true
	return m, adapter
}

// TestBurstInputLock_IgnoresSecondEnter covers the second-Enter guard: a second
// Enter while burst-pending is swallowed — it does not re-dispatch (no new pipe, no
// additional adapter call) and the burst stays pending.
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

// TestBurstInputLock_IgnoresRowActions covers the row-action lock: m, navigation,
// Space, /, and s are all no-ops while burst-pending (each would change state if it
// reached its handler — mark toggle, cursor move, preview page, filter focus,
// grouping cycle — so an unchanged model proves the swallow).
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

// TestBurstInputLock_CtrlCAndEscStayLive covers the cancellation carve-out: Ctrl-C
// and Esc reach cancelBurst while pending — Ctrl-C does NOT quit and Esc does NOT exit
// multi-select mode, so they are intercepted by the input-lock rather than falling
// through to the normal quit / exit handlers. cancelBurst KEEPS burstPending true
// (§6-8: it stays locked until the goroutine's terminal event lands); the full cancel
// lifecycle (ctx cancel + selection mutation) is covered in burst_cancel_test.go.
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

// TestBurstInputLock_AdvancesOpeningCounter covers the progress fold: each
// spawnProgressMsg advances BurstDone() and the rendered `Opening n/N…` band, so a
// 3-session batch reads `Opening 1/3…` then `Opening 2/3…`.
func TestBurstInputLock_AdvancesOpeningCounter(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
	m.termWidth = 80
	m.termHeight = 24
	m.burstPending = true
	m.burstTotal = 3
	m.burstDone = 0

	// The progress denominator (msg.Total) is the external count N-1 = 2; it must NOT
	// change the N=3 band denominator.
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

// TestBurstInputLock_HoldsDenominatorAtN covers the denominator invariant: the
// Opening band denominator holds at the dispatch-time N (marked-set size, incl. the
// trigger) across every progress message — msg.Total (the external count N-1) never
// overwrites BurstTotal, so the band never reads `2/2` and never reaches `3/3`.
func TestBurstInputLock_HoldsDenominatorAtN(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
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
	// The counter never reaches N (the trigger self-attaches silently — no N/N nag).
	if strings.Contains(ansi.Strip(bannerFirstLine(m)), "3/3") {
		t.Error("the Opening band must never reach 3/3 (the trigger self-attaches silently)")
	}
}

// TestBurstInputLock_OpeningBandPrecedence covers the section-header precedence: the
// Opening band owns the row just below the live filter input — it outranks the
// multi-select banner, the unsupported banner, and the standard header, and steps
// aside only for the live filter input.
func TestBurstInputLock_OpeningBandPrecedence(t *testing.T) {
	newOpeningModel := func() Model {
		m := NewModelWithSessions([]tmux.Session{
			{Name: "alpha", Windows: 1},
			{Name: "bravo", Windows: 2},
			{Name: "charlie", Windows: 3},
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

// TestOpeningBand_RendersVioletCounter pins the render contract: the band reads
// `Opening n/N…` (with the U+2026 ellipsis) in accent.violet, exactly one row, at
// the full content width (a canvas-painted flex spacer pads the right).
func TestOpeningBand_RendersVioletCounter(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		band := renderOpeningBand(1, 3, sectionHeaderWidth, mode, false)
		if !strings.Contains(ansi.Strip(band), "Opening 1/3…") {
			t.Errorf("band must read %q:\n%s", "Opening 1/3…", ansi.Strip(band))
		}
		violet := headerStyle(theme.MV.AccentViolet, mode, false).Render("Opening 1/3…")
		if !strings.Contains(band, violet) {
			t.Errorf("band missing the accent.violet %q run:\n%s", "Opening 1/3…", band)
		}
		if got := lipgloss.Height(band); got != 1 {
			t.Errorf("band height = %d, want exactly 1 row:\n%s", got, band)
		}
		if got := lipgloss.Width(band); got != sectionHeaderWidth {
			t.Errorf("band width = %d, want exactly %d (flex spacer to content width)", got, sectionHeaderWidth)
		}
		if seq := canvasSeq(t, mode); !strings.Contains(band, seq) {
			t.Errorf("band does not paint the canvas background sequence %q:\n%s", seq, band)
		}
	}
}

// TestOpeningBand_ColourlessDropsHueAndCanvas covers the NO_COLOR carve-out (§2.5):
// a colourless band carries no canvas background SGR and no foreground hue — the
// `Opening n/N…` text survives on the terminal's native fg/bg.
func TestOpeningBand_ColourlessDropsHueAndCanvas(t *testing.T) {
	band := renderOpeningBand(2, 3, sectionHeaderWidth, theme.Dark, true)

	if !strings.Contains(band, "Opening 2/3…") {
		t.Errorf("colourless band dropped the text:\n%s", band)
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(band, seq) {
		t.Errorf("colourless band still paints the canvas background sequence %q", seq)
	}
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); strings.Contains(band, seq) {
		t.Errorf("colourless band still emits the accent.violet foreground sequence %q", seq)
	}
}
