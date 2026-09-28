package tui

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/spawn"
	"github.com/leeovery/portal/internal/spawntest"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func TestBurstPreflightAbort_AbortsAtomicallyNoAdapterNoSelfAttach(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 2},
		{Name: "agentic-workflows-codify", Windows: 1},
		{Name: "designlab-web-r8suyU", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	exists := func(name string) bool { return name != "fab-flowx-explore" }
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, exists, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = enterMultiSelectEmpty(t, m)
	m = markRow(t, m, 0)
	m = markRow(t, m, 1)
	m = markRow(t, m, 2)

	m, cmd := pressEnter(t, m)
	if !m.BurstPending() {
		t.Fatal("precondition: burst must be pending after dispatch")
	}

	mBefore, term := driveBurstToTerminal(t, m, cmd)
	abort, ok := term.(spawnAbortMsg)
	if !ok {
		t.Fatalf("terminal burst message = %T, want spawnAbortMsg", term)
	}
	if !slices.Equal(abort.Gone, []string{"fab-flowx-explore"}) {
		t.Fatalf("abort.Gone = %v, want [fab-flowx-explore]", abort.Gone)
	}
	if len(adapter.Calls) != 0 {
		t.Errorf("pre-flight abort must open NOTHING; adapter OpenWindow calls = %d, want 0", len(adapter.Calls))
	}

	updated, follow := mBefore.Update(abort)
	rm := updated.(Model)

	if rm.Selected() != "" {
		t.Errorf("pre-flight abort must not self-attach; Selected() = %q, want empty", rm.Selected())
	}
	if follow != nil {
		t.Error("pre-flight abort must not return a cmd (no tea.Quit — the picker stays open)")
	}
	if rm.BurstPending() {
		t.Error("pre-flight abort must clear burst-pending")
	}
	if !rm.MultiSelectActive() {
		t.Error("pre-flight abort must stay in multi-select mode")
	}
	if rm.flashText != "" {
		t.Errorf("zero windows opened → no leave-what-opened flash; flashText = %q, want empty", rm.flashText)
	}
	if rm.abortBannerText == "" {
		t.Error("pre-flight abort must set the abort banner text")
	}
	if _, ok := rm.goneFlagged["fab-flowx-explore"]; !ok {
		t.Errorf("pre-flight abort must flag the gone session; goneFlagged = %v", rm.goneFlagged)
	}
}

func TestBurstPreflightAbort_BannerNamesGoneSessionWithEscDismiss(t *testing.T) {
	m := newPendingBurstModel(t, []string{"fab-flowx-explore", "agentic-workflows-codify"})
	updated, _ := m.Update(spawnAbortMsg{Gone: []string{"fab-flowx-explore"}})
	rm := updated.(Model)

	if want := "'fab-flowx-explore' is gone — nothing opened"; rm.abortBannerText != want {
		t.Errorf("abortBannerText = %q, want %q (byte-match the design copy)", rm.abortBannerText, want)
	}

	first := ansi.Strip(bannerFirstLine(rm))
	wantLeft := flashWarningGlyph + " 'fab-flowx-explore' is gone — nothing opened"
	if !strings.Contains(first, wantLeft) {
		t.Errorf("section-header row must read %q:\n%s", wantLeft, first)
	}
	if !strings.Contains(first, "esc dismiss") {
		t.Errorf("section-header row must show the right-aligned %q hint:\n%s", "esc dismiss", first)
	}
	if strings.Contains(first, "selected") {
		t.Errorf("abort banner must own the row over the multi-select banner:\n%s", first)
	}
}

func TestPreflightAbortHeader_RedGlyphMessageDimHint(t *testing.T) {
	const msg = "'fab-flowx-explore' is gone — nothing opened"
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		header := renderPreflightAbortHeader(msg, sectionHeaderWidth, th, false)
		wantLeft := flashWarningGlyph + " " + msg

		if !strings.Contains(ansi.Strip(header), wantLeft) {
			t.Errorf("abort banner missing %q:\n%s", wantLeft, ansi.Strip(header))
		}
		redRun := headerStyle(th.StateDestructive, th, false).Render(wantLeft)
		if !strings.Contains(header, redRun) {
			t.Errorf("abort banner missing the state.destructive %q run:\n%s", wantLeft, header)
		}
		detailRun := headerStyle(th.TextMuted, th, false).Render("esc dismiss")
		if !strings.Contains(header, detailRun) {
			t.Errorf("abort banner missing the text.muted %q run:\n%s", "esc dismiss", header)
		}
	}
}

func TestPreflightAbortHeader_RightAlignedOneRow(t *testing.T) {
	header := renderPreflightAbortHeader("'x' is gone — nothing opened", sectionHeaderWidth, testDarkTheme(t), false)

	if got := lipgloss.Height(header); got != 1 {
		t.Errorf("abort banner height = %d, want exactly 1 row:\n%s", got, header)
	}
	if got := lipgloss.Width(header); got != sectionHeaderWidth {
		t.Errorf("abort banner width = %d, want exactly %d (flex spacer to content width)", got, sectionHeaderWidth)
	}
	stripped := ansi.Strip(header)
	msgIdx := strings.Index(stripped, "nothing opened")
	hintIdx := strings.LastIndex(stripped, "esc dismiss")
	if msgIdx < 0 || hintIdx < 0 {
		t.Fatalf("banner missing a cluster: msgIdx=%d hintIdx=%d\n%s", msgIdx, hintIdx, stripped)
	}
	if hintIdx < msgIdx {
		t.Errorf("hint (idx %d) appears before the message (idx %d); must be right-aligned", hintIdx, msgIdx)
	}
}

func TestPreflightAbortHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderPreflightAbortHeader("'x' is gone — nothing opened", sectionHeaderWidth, testDarkTheme(t), true)
	stripped := ansi.Strip(header)

	for _, sub := range []string{flashWarningGlyph, "nothing opened", "esc dismiss"} {
		if !strings.Contains(stripped, sub) {
			t.Errorf("colourless abort banner dropped %q:\n%s", sub, stripped)
		}
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(header, seq) {
		t.Errorf("colourless abort banner still paints the canvas background %q", seq)
	}
	for _, tok := range []theme.Token{testDarkTheme(t).StateDestructive, testDarkTheme(t).TextMuted} {
		if seq := tokenFgSeq(t, tok); strings.Contains(header, seq) {
			t.Errorf("colourless abort banner still emits a foreground role sequence %q", seq)
		}
	}
}

func TestSessionRow_GoneFlaggedShowsRedWarningAndBadge(t *testing.T) {
	d := SessionDelegate{
		Theme:       testDarkTheme(t),
		MultiSelect: true,
		Selected:    markedSet("agentic-workflows-codify", "designlab-web-r8suyU"),
		GoneFlagged: markedSet("fab-flowx-explore"),
	}
	items := flatItems(
		tmux.Session{Name: "agentic-workflows-codify", Windows: 1, Attached: true},
		tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false},
		tmux.Session{Name: "designlab-web-r8suyU", Windows: 3, Attached: true},
	)

	gone := renderRow(d, 80, items, 1, 1)
	strippedGone := ansi.Strip(gone)

	if col := visibleColOf(gone, flashWarningGlyph); col != 0 {
		t.Errorf("gone row ⚠ must sit at left-bar col 0, got col %d: %q", col, strippedGone)
	}
	if strings.Contains(strippedGone, multiSelectMarker) {
		t.Errorf("gone row must NOT render the ● marker (⚠ takes precedence): %q", strippedGone)
	}
	if !strings.Contains(strippedGone, goneBadge) {
		t.Errorf("gone row must render the red %q badge: %q", goneBadge, strippedGone)
	}
	if strings.Contains(strippedGone, rowIndicatorGlyph) {
		t.Errorf("gone row must NOT render the attached indicator: %q", strippedGone)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).StateDestructive); !strings.Contains(gone, seq) {
		t.Errorf("gone row missing the state.destructive role sequence %q: %q", seq, escSeq(gone))
	}

	survivor := renderRow(d, 80, items, 0, 1)
	if col := visibleColOf(survivor, multiSelectMarker); col != 0 {
		t.Errorf("survivor row must keep its ● at col 0, got col %d: %q", col, ansi.Strip(survivor))
	}
	if strings.Contains(ansi.Strip(survivor), flashWarningGlyph) {
		t.Errorf("survivor row must not render the ⚠: %q", ansi.Strip(survivor))
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); !strings.Contains(survivor, seq) {
		t.Errorf("survivor row missing the accent.primary ● role sequence %q: %q", seq, escSeq(survivor))
	}
}

func TestSessionRow_GoneFlaggedWidthByteUnchanged(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false},
		tmux.Session{Name: "designlab-web-r8suyU", Windows: 3, Attached: true},
	)
	gone := renderRow(SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, GoneFlagged: markedSet("fab-flowx-explore")}, w, items, 0, 1)
	normal := renderRow(SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true}, w, items, 0, 1)

	if gw, nw := lipgloss.Width(gone), lipgloss.Width(normal); gw != nw || gw != w {
		t.Errorf("gone row width changed by the flag: gone=%d normal=%d, want %d", gw, nw, w)
	}
	const name = "fab-flowx-explore"
	gc, nc := visibleColOf(gone, name), visibleColOf(normal, name)
	if gc < 0 || nc < 0 {
		t.Fatalf("name missing: gone=%q normal=%q", ansi.Strip(gone), ansi.Strip(normal))
	}
	if gc != nc {
		t.Errorf("name shifted by the gone flag: gone col %d, normal col %d", gc, nc)
	}
}

func TestSessionRow_GoneFlaggedColourlessSurvives(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), Colourless: true, MultiSelect: true, GoneFlagged: markedSet("fab-flowx-explore")}
	items := flatItems(tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false})
	out := renderRow(d, 80, items, 0, 0)
	stripped := ansi.Strip(out)

	if !strings.Contains(stripped, flashWarningGlyph) {
		t.Errorf("colourless gone row dropped the ⚠ glyph: %q", stripped)
	}
	if !strings.Contains(stripped, goneBadge) {
		t.Errorf("colourless gone row dropped the %q badge: %q", goneBadge, stripped)
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).StateDestructive); strings.Contains(out, seq) {
		t.Errorf("colourless gone row still emits the state.destructive fg %q", seq)
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(out, seq) {
		t.Errorf("colourless gone row still paints the canvas background %q", seq)
	}
}

func TestSessionRow_HeaderNeverGoneFlagged(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, GoneFlagged: markedSet("work")}
	items := []list.Item{HeaderItem{Heading: "work", Count: 3, Key: "work"}}
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(ansi.Strip(out), flashWarningGlyph) || strings.Contains(ansi.Strip(out), goneBadge) {
		t.Errorf("HeaderItem must never carry the gone flag: %q", ansi.Strip(out))
	}
}

func TestBurstPreflightAbort_PrunesGoneKeepsSurvivorsMarked(t *testing.T) {
	m := newPendingBurstModel(t, []string{"fab-flowx-explore", "agentic-workflows-codify", "designlab-web-r8suyU"})
	updated, _ := m.Update(spawnAbortMsg{Gone: []string{"fab-flowx-explore"}})
	rm := updated.(Model)

	if rm.IsSessionSelected("fab-flowx-explore") {
		t.Error("the gone session must be pruned from the selection")
	}
	for _, s := range []string{"agentic-workflows-codify", "designlab-web-r8suyU"} {
		if !rm.IsSessionSelected(s) {
			t.Errorf("the survivor %q must stay marked (a second Enter proceeds with survivors)", s)
		}
	}
	if rm.SelectedSessionCount() != 2 {
		t.Errorf("selection count = %d, want 2 (gone pruned, survivors kept)", rm.SelectedSessionCount())
	}
	if !rm.MultiSelectActive() {
		t.Error("prune must stay in multi-select mode")
	}
	view := ansi.Strip(rm.sessionList.View())
	if !strings.Contains(view, goneBadge) {
		t.Errorf("the rendered list must show the %q badge on the gone row:\n%s", goneBadge, view)
	}
}

func TestBurstPreflightAbort_MultipleGoneAllNamed(t *testing.T) {
	m := newPendingBurstModel(t, []string{"s1", "s2", "s3", "s4"})
	updated, _ := m.Update(spawnAbortMsg{Gone: []string{"s2", "s4"}})
	rm := updated.(Model)

	if want := "'s2', 's4' are gone — nothing opened"; rm.abortBannerText != want {
		t.Errorf("abortBannerText = %q, want %q (both named, plural verb)", rm.abortBannerText, want)
	}
	for _, s := range []string{"s2", "s4"} {
		if _, ok := rm.goneFlagged[s]; !ok {
			t.Errorf("both gone sessions must be flagged; %q missing from %v", s, rm.goneFlagged)
		}
		if rm.IsSessionSelected(s) {
			t.Errorf("both gone sessions must be pruned; %q still marked", s)
		}
	}
	for _, s := range []string{"s1", "s3"} {
		if !rm.IsSessionSelected(s) {
			t.Errorf("the survivor %q must stay marked", s)
		}
	}
}

func TestBurstPreflightAbort_EscDismissesWithoutExitingMode(t *testing.T) {
	m := newPendingBurstModel(t, []string{"fab-flowx-explore", "agentic-workflows-codify"})
	updated, _ := m.Update(spawnAbortMsg{Gone: []string{"fab-flowx-explore"}})
	rm := updated.(Model)
	if rm.abortBannerText == "" || len(rm.goneFlagged) == 0 {
		t.Fatal("precondition: the abort banner + gone flags must be set")
	}

	after, _ := rm.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	am := after.(Model)

	if am.abortBannerText != "" {
		t.Errorf("Esc must clear the abort banner text; got %q", am.abortBannerText)
	}
	if len(am.goneFlagged) != 0 {
		t.Errorf("Esc must clear the gone flags; got %v", am.goneFlagged)
	}
	if !am.MultiSelectActive() {
		t.Error("Esc dismissing the abort banner must STAY in multi-select mode")
	}
	if !am.IsSessionSelected("agentic-workflows-codify") {
		t.Error("the survivor must stay marked after dismissal")
	}
	am.termWidth = 120
	if footer := footerVisible(am.renderSessionsFooterForFilterState()); !strings.Contains(footer, "m toggle") {
		t.Errorf("after dismissal the multi-select footer must render (missing 'm toggle'):\n%s", footer)
	}

	after2, _ := am.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	am2 := after2.(Model)
	if am2.MultiSelectActive() {
		t.Error("a second Esc (no abort banner) must exit multi-select mode")
	}
}
