package tui

// restore-host-terminal-windows-6-7 — pre-flight abort UI: gone flash + prune
// keeping survivors.
//
// White-box (package tui) tests of the spawnAbortMsg handler and its render
// surfaces. If pre-flight (has-session over every marked session on Enter) finds a
// marked session gone, the burst aborts atomically — nothing spawns, no window
// opens, no self-attach. These tests assert Portal:
//   - aborts with ZERO adapter calls, no self-attach (Selected()=="" / no tea.Quit),
//     stays in multi-select mode, and sets no leave-what-opened flash,
//   - renders the red `⚠ '<session>' is gone — nothing opened` banner at the
//     section-header row with a right-aligned dim `esc dismiss`,
//   - flags the gone row with a red ⚠ marker + red `session gone` badge while every
//     surviving marked row keeps its violet ●,
//   - prunes the gone session(s) from the selection keeping survivors marked (a
//     second Enter proceeds with the survivors, not a re-abort),
//   - names every gone session in the one-line message,
//   - dismisses the banner + gone flags on Esc WITHOUT exiting multi-select mode.
//
// Shared seam helpers (wireBurstSeams, allPresent, resolveDetection, markRow,
// driveBurstToTerminal, newPendingBurstModel, markedSet, renderRow, visibleColOf,
// bannerFirstLine) live in the sibling burst / row / banner test files. No
// t.Parallel: consistent with the rest of the tui test surface.

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
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// TestBurstPreflightAbort_AbortsAtomicallyNoAdapterNoSelfAttach drives an
// end-to-end N≥2 Enter where one marked session vanished between marking and Enter
// (the sessionExists probe returns false for it). The goroutine pre-flights, finds
// the gone session, and emits a terminal spawnAbortMsg BEFORE calling the burster —
// so nothing spawns. The handler self-attaches nothing, quits nothing, stays in
// multi-select mode, and sets no leave-what-opened flash.
func TestBurstPreflightAbort_AbortsAtomicallyNoAdapterNoSelfAttach(t *testing.T) {
	sessions := []tmux.Session{
		{Name: "fab-flowx-explore", Windows: 2},
		{Name: "agentic-workflows-codify", Windows: 1},
		{Name: "designlab-web-r8suyU", Windows: 3},
	}
	ack := &spawntest.FakeAckChannel{}
	adapter := &spawntest.FakeAdapter{Ack: ack}
	m := NewModelWithSessions(sessions)
	// fab-flowx-explore vanished between marking and Enter.
	exists := func(name string) bool { return name != "fab-flowx-explore" }
	wireBurstSeams(&m, adapter, spawn.ResolutionNative, exists, ack)
	m = resolveDetection(t, m, ghosttyIdentity())

	m = pressSession(t, m, pressM)
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
	// Zero adapter calls — the goroutine aborted before Burster.Run, so nothing spawned.
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

// TestBurstPreflightAbort_BannerNamesGoneSessionWithEscDismiss asserts the exact
// abort banner copy: `'<session>' is gone — nothing opened` (byte-matching the
// delivered design frame), with the ⚠ glyph added by renderPreflightAbortHeader.
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
	// The abort banner OUTRANKS the multi-select `N selected` banner (per the frame).
	if strings.Contains(first, "selected") {
		t.Errorf("abort banner must own the row over the multi-select banner:\n%s", first)
	}
}

// TestPreflightAbortHeader_RedGlyphMessageDimHint pins the colour roles of the
// standalone renderer: the ⚠ + message in state.red, the `esc dismiss` hint in
// text.detail, on both the dark and light canvas.
func TestPreflightAbortHeader_RedGlyphMessageDimHint(t *testing.T) {
	const msg = "'fab-flowx-explore' is gone — nothing opened"
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		header := renderPreflightAbortHeader(msg, sectionHeaderWidth, mode, false)
		wantLeft := flashWarningGlyph + " " + msg

		if !strings.Contains(ansi.Strip(header), wantLeft) {
			t.Errorf("abort banner missing %q:\n%s", wantLeft, ansi.Strip(header))
		}
		redRun := headerStyle(theme.MV.StateRed, mode, false).Render(wantLeft)
		if !strings.Contains(header, redRun) {
			t.Errorf("abort banner missing the state.red %q run:\n%s", wantLeft, header)
		}
		detailRun := headerStyle(theme.MV.TextDetail, mode, false).Render("esc dismiss")
		if !strings.Contains(header, detailRun) {
			t.Errorf("abort banner missing the text.detail %q run:\n%s", "esc dismiss", header)
		}
	}
}

// TestPreflightAbortHeader_RightAlignedOneRow asserts the hint is right-anchored and
// the single rendered row is exactly the content width (§3.5 pagination budget).
func TestPreflightAbortHeader_RightAlignedOneRow(t *testing.T) {
	header := renderPreflightAbortHeader("'x' is gone — nothing opened", sectionHeaderWidth, theme.Dark, false)

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

// TestPreflightAbortHeader_ColourlessDropsHueAndCanvas asserts the NO_COLOR
// carve-out (§2.5): a colourless banner carries no canvas background SGR and no
// foreground hue — the ⚠, the message, and `esc dismiss` survive on the native fg/bg.
func TestPreflightAbortHeader_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderPreflightAbortHeader("'x' is gone — nothing opened", sectionHeaderWidth, theme.Dark, true)
	stripped := ansi.Strip(header)

	for _, sub := range []string{flashWarningGlyph, "nothing opened", "esc dismiss"} {
		if !strings.Contains(stripped, sub) {
			t.Errorf("colourless abort banner dropped %q:\n%s", sub, stripped)
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless abort banner still paints the canvas background %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.StateRed, theme.MV.TextDetail} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless abort banner still emits a foreground role sequence %q", seq)
		}
	}
}

// TestSessionRow_GoneFlaggedShowsRedWarningAndBadge asserts the delegate render: a
// GoneFlagged row draws a red ⚠ in the left-bar column (in place of the ●/▌) and a
// red `session gone` badge in place of the attached badge, while a surviving marked
// row keeps its violet ●.
func TestSessionRow_GoneFlaggedShowsRedWarningAndBadge(t *testing.T) {
	d := SessionDelegate{
		Mode:        theme.Dark,
		MultiSelect: true,
		Selected:    markedSet("agentic-workflows-codify", "designlab-web-r8suyU"),
		GoneFlagged: markedSet("fab-flowx-explore"),
	}
	items := flatItems(
		tmux.Session{Name: "agentic-workflows-codify", Windows: 1, Attached: true},
		tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false},
		tmux.Session{Name: "designlab-web-r8suyU", Windows: 3, Attached: true},
	)

	// The gone row is the cursor/banded row in the frame (index 1, selected).
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
	if strings.Contains(strippedGone, attachedMarker) {
		t.Errorf("gone row must NOT render the attached badge: %q", strippedGone)
	}
	if seq := tokenFgSeq(t, theme.MV.StateRed, theme.Dark); !strings.Contains(gone, seq) {
		t.Errorf("gone row missing the state.red role sequence %q: %q", seq, escSeq(gone))
	}

	// A surviving marked row keeps its violet ● and never renders the ⚠.
	survivor := renderRow(d, 80, items, 0, 1)
	if col := visibleColOf(survivor, multiSelectMarker); col != 0 {
		t.Errorf("survivor row must keep its ● at col 0, got col %d: %q", col, ansi.Strip(survivor))
	}
	if strings.Contains(ansi.Strip(survivor), flashWarningGlyph) {
		t.Errorf("survivor row must not render the ⚠: %q", ansi.Strip(survivor))
	}
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); !strings.Contains(survivor, seq) {
		t.Errorf("survivor row missing the accent.violet ● role sequence %q: %q", seq, escSeq(survivor))
	}
}

// TestSessionRow_GoneFlaggedWidthByteUnchanged is the one-delegate-line invariant
// (§3.5 / §4.1): the gone flag must NOT change the row width or shift the name/count
// columns — the ⚠ occupies the same 2-cell left-bar column, and the `session gone`
// badge occupies the same fixed (attached slot + right margin) trailing region.
func TestSessionRow_GoneFlaggedWidthByteUnchanged(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false},
		tmux.Session{Name: "designlab-web-r8suyU", Windows: 3, Attached: true},
	)
	gone := renderRow(SessionDelegate{Mode: theme.Dark, MultiSelect: true, GoneFlagged: markedSet("fab-flowx-explore")}, w, items, 0, 1)
	normal := renderRow(SessionDelegate{Mode: theme.Dark, MultiSelect: true}, w, items, 0, 1)

	if gw, nw := lipgloss.Width(gone), lipgloss.Width(normal); gw != nw || gw != w {
		t.Errorf("gone row width changed by the flag: gone=%d normal=%d, want %d", gw, nw, w)
	}
	for _, sub := range []string{"fab-flowx-explore", "window"} {
		gc, nc := visibleColOf(gone, sub), visibleColOf(normal, sub)
		if gc < 0 || nc < 0 {
			t.Fatalf("column %q missing: gone=%q normal=%q", sub, ansi.Strip(gone), ansi.Strip(normal))
		}
		if gc != nc {
			t.Errorf("column %q shifted by the gone flag: gone col %d, normal col %d", sub, gc, nc)
		}
	}
}

// TestSessionRow_GoneFlaggedColourlessSurvives asserts the NO_COLOR carve-out for
// the row: the ⚠ glyph and the `session gone` badge text survive with no state.red
// hue and no canvas background.
func TestSessionRow_GoneFlaggedColourlessSurvives(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, Colourless: true, MultiSelect: true, GoneFlagged: markedSet("fab-flowx-explore")}
	items := flatItems(tmux.Session{Name: "fab-flowx-explore", Windows: 2, Attached: false})
	out := renderRow(d, 80, items, 0, 0)
	stripped := ansi.Strip(out)

	if !strings.Contains(stripped, flashWarningGlyph) {
		t.Errorf("colourless gone row dropped the ⚠ glyph: %q", stripped)
	}
	if !strings.Contains(stripped, goneBadge) {
		t.Errorf("colourless gone row dropped the %q badge: %q", goneBadge, stripped)
	}
	if seq := tokenFgSeq(t, theme.MV.StateRed, theme.Dark); strings.Contains(out, seq) {
		t.Errorf("colourless gone row still emits the state.red fg %q", seq)
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(out, seq) {
		t.Errorf("colourless gone row still paints the canvas background %q", seq)
	}
}

// TestSessionRow_HeaderNeverGoneFlagged asserts a HeaderItem never carries the gone
// flag (the header render arm is untouched), mirroring the ● marker guard.
func TestSessionRow_HeaderNeverGoneFlagged(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: true, GoneFlagged: markedSet("work")}
	items := []list.Item{HeaderItem{Heading: "work", Count: 3, Key: "work"}}
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(ansi.Strip(out), flashWarningGlyph) || strings.Contains(ansi.Strip(out), goneBadge) {
		t.Errorf("HeaderItem must never carry the gone flag: %q", ansi.Strip(out))
	}
}

// TestBurstPreflightAbort_PrunesGoneKeepsSurvivorsMarked asserts the prune rule: the
// gone session leaves the selection while every survivor stays marked, so a second
// Enter proceeds with the survivors (not a re-abort loop). The delegate is refreshed
// so the survivors keep their ● and the gone row shows the red flag.
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
	// The delegate reflects the pruned set + gone flag: the survivors keep their ●,
	// the gone row shows the red flag (not a ●).
	view := ansi.Strip(rm.sessionList.View())
	if !strings.Contains(view, goneBadge) {
		t.Errorf("the rendered list must show the %q badge on the gone row:\n%s", goneBadge, view)
	}
}

// TestBurstPreflightAbort_MultipleGoneAllNamed asserts the plural-safe message: two
// gone sessions are BOTH named, with the plural verb `are`.
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

// TestBurstPreflightAbort_EscDismissesWithoutExitingMode asserts the dismissal
// precedence: a first Esc while the abort banner shows clears the banner + the gone
// flags and STAYS in multi-select mode; a second Esc (no banner) exits the mode as
// normal.
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
	// AC6: the multi-select footer is unchanged after dismissal (still in mode).
	am.termWidth = 120
	if footer := footerVisible(am.renderSessionsFooterForFilterState()); !strings.Contains(footer, "m toggle") {
		t.Errorf("after dismissal the multi-select footer must render (missing 'm toggle'):\n%s", footer)
	}

	// A second Esc (no abort banner) exits the mode as normal (Task 5.1).
	after2, _ := am.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	am2 := after2.(Model)
	if am2.MultiSelectActive() {
		t.Error("a second Esc (no abort banner) must exit multi-select mode")
	}
}
