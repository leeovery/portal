package tui

// Task spectrum-tui-design-5-5 — VISUAL loading-screen render (§10.3).
//
// These tests pin the honest loading screen: the locked 5-row bold solid-block
// PORTAL wordmark + a flush 5-row violet caret bar, a thick violet progress bar
// on the bg.track track, and a real 5-row tick-list that ticks off
// done/active/pending from the live LoadingProgress accumulator (task 5-4) with
// the §2.9 token+weight mapping and the spaced `N / M` counter on the active
// "Restoring sessions" row. They also pin the narrow/short degrade, the NO_COLOR
// carve-out, the first-paint canvas gate, and the warm-path "no loading screen"
// gate.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// midRestoreProgress builds the reference mid-restore accumulator state: steps
// 1–2 done (Started tmux server, Registered hooks), "Restoring sessions" active
// with an 8/12 counter, the trailing labels pending. It folds the real
// BootstrapProgressMsg sequence through the accumulator so the test exercises the
// same path the live channel does.
func midRestoreProgress() LoadingProgress {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1, Name: "EnsureServer"})
	p = p.Apply(BootstrapProgressMsg{Index: 2, Name: "RegisterPortalHooks"})
	p = p.Apply(BootstrapProgressMsg{Index: 3, Name: "SetRestoring"})
	p = p.Apply(BootstrapProgressMsg{Index: 4, Name: "SweepOrphanDaemons"})
	p = p.Apply(BootstrapProgressMsg{Index: 5, Name: "EnsureSaver"})
	p = p.Apply(BootstrapProgressMsg{Index: restoreStep, Name: "Restore", RestoreN: 8, RestoreM: 12})
	return p
}

// TestLoadingScreen_RendersBlockBannerCaretBarAndList asserts the §10.3
// composition: the 5-row bold solid-block PORTAL banner, a flush 5-row violet
// caret bar to its right, a thick violet bar on the bg.track track, and the 5-row
// step-list.
func TestLoadingScreen_RendersBlockBannerCaretBarAndList(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, false)
	visible := ansi.Strip(out)

	// The five locked banner rows are present verbatim (the wordmark segments).
	// Rows 0–3 are right-padded at render time to the common max width, so assert
	// the trimmed segment is present (the trailing pad is whitespace).
	for i, row := range loadingWordmark {
		if !strings.Contains(visible, strings.TrimRight(row, " ")) {
			t.Errorf("loading screen missing block banner row %d %q:\n%s", i, row, visible)
		}
	}

	// All five friendly labels render as real rows.
	for _, label := range labelOrder {
		if !strings.Contains(visible, label) {
			t.Errorf("loading screen missing step-list label %q:\n%s", label, visible)
		}
	}

	// The three tick glyphs are present (done ✓ ×2, active ◐, pending · ×2).
	for _, glyph := range []string{loadingGlyphDone, loadingGlyphActive, loadingGlyphPending} {
		if !strings.Contains(visible, glyph) {
			t.Errorf("loading screen missing tick glyph %q:\n%s", glyph, visible)
		}
	}

	// The wordmark block letters carry text.primary; the caret bar carries
	// accent.violet; the filled bar carries the accent.violet background; the
	// track carries the bg.track background.
	if !strings.Contains(out, tokenFgSeq(t, theme.MV.TextPrimary, theme.Dark)) {
		t.Error("loading screen does not paint the wordmark in text.primary")
	}
	if !strings.Contains(out, tokenBgSeq(t, theme.MV.AccentViolet, theme.Dark)) {
		t.Error("loading screen does not paint the filled bar with the accent.violet background")
	}
	if !strings.Contains(out, tokenBgSeq(t, theme.MV.BgTrack, theme.Dark)) {
		t.Error("loading screen does not paint the bar track with the bg.track background")
	}
}

// TestLoadingScreen_CaretIsFlushAcrossBannerRows pins the review-fix invariant:
// the violet caret is a single FLUSH 5-row vertical bar, not a per-row appended
// glyph that jogs on the ragged-width bottom row (the broken-comma regression).
// The block banner is built by right-padding all rows to a common width and
// joining ONE 5-row caret column horizontally — so the caret glyph must land on
// the SAME column on every one of the five banner rows.
func TestLoadingScreen_CaretIsFlushAcrossBannerRows(t *testing.T) {
	block := ansi.Strip(renderBlockWordmark(theme.Dark, false))
	lines := strings.Split(block, "\n")
	if len(lines) != len(loadingWordmark) {
		t.Fatalf("block banner has %d rows, want %d", len(lines), len(loadingWordmark))
	}

	// The caret is the LAST solid block on each row (the wordmark letters precede
	// it, then a one-space gap, then the caret column). Find the caret's column as
	// the index of the final caret glyph on each row; it must be identical on all
	// five rows (a flush vertical bar).
	caretCol := -1
	for i, line := range lines {
		col := strings.LastIndex(line, loadingCaretGlyph)
		if col < 0 {
			t.Fatalf("banner row %d has no caret glyph %q: %q", i, loadingCaretGlyph, line)
		}
		// Convert the byte index to a rune column so multi-byte block glyphs count
		// as one cell each.
		runeCol := len([]rune(line[:col]))
		if caretCol == -1 {
			caretCol = runeCol
			continue
		}
		if runeCol != caretCol {
			t.Errorf("caret column drifts: row %d caret at col %d, want %d (caret not flush — ragged-row jog regression)", i, runeCol, caretCol)
		}
	}
}

// TestLoadingScreen_TickStatesUseSpecdTokens asserts each step row uses the
// §10.3 glyph + label token for its done/active/pending state, driven from live
// LoadingProgress.
func TestLoadingScreen_TickStatesUseSpecdTokens(t *testing.T) {
	view := midRestoreProgress().View()

	// Sanity: the accumulator produced the expected mid-restore states.
	wantStates := map[string]LabelState{
		LabelStartedTmuxServer:     LabelDone,
		LabelRegisteredHooks:       LabelDone,
		LabelRestoringSessions:     LabelActive,
		LabelReplayingScrollback:   LabelPending,
		LabelRunningResumeCommands: LabelPending,
	}
	for _, l := range view.Labels {
		if got := wantStates[l.Text]; got != l.State {
			t.Fatalf("label %q state = %v, want %v (fixture drift)", l.Text, l.State, got)
		}
	}

	// Each row is rendered with the matching glyph + label token.
	doneRow := renderTickRow(LoadingLabel{Text: LabelStartedTmuxServer, State: LabelDone}, theme.Dark, false)
	if !strings.Contains(ansi.Strip(doneRow), loadingGlyphDone) {
		t.Errorf("done row missing %q glyph: %q", loadingGlyphDone, ansi.Strip(doneRow))
	}
	if !strings.Contains(doneRow, tokenFgSeq(t, theme.MV.StateGreen, theme.Dark)) {
		t.Error("done glyph not painted state.green")
	}
	if !strings.Contains(doneRow, tokenFgSeq(t, theme.MV.TextMutedBright, theme.Dark)) {
		t.Error("done label not painted text.muted-bright")
	}

	activeRow := renderTickRow(LoadingLabel{Text: LabelRestoringSessions, State: LabelActive, Counter: "8/12"}, theme.Dark, false)
	if !strings.Contains(ansi.Strip(activeRow), loadingGlyphActive) {
		t.Errorf("active row missing %q glyph: %q", loadingGlyphActive, ansi.Strip(activeRow))
	}
	if !strings.Contains(activeRow, tokenFgSeq(t, theme.MV.AccentCyan, theme.Dark)) {
		t.Error("active glyph not painted accent.cyan")
	}
	if !strings.Contains(activeRow, tokenFgSeq(t, theme.MV.TextPrimary, theme.Dark)) {
		t.Error("active label not painted text.primary")
	}

	pendingRow := renderTickRow(LoadingLabel{Text: LabelReplayingScrollback, State: LabelPending}, theme.Dark, false)
	if !strings.Contains(ansi.Strip(pendingRow), loadingGlyphPending) {
		t.Errorf("pending row missing %q glyph: %q", loadingGlyphPending, ansi.Strip(pendingRow))
	}
	if !strings.Contains(pendingRow, tokenFgSeq(t, theme.MV.TextFaint, theme.Dark)) {
		t.Error("pending glyph not painted text.faint")
	}
	if !strings.Contains(pendingRow, tokenFgSeq(t, theme.MV.TextDim, theme.Dark)) {
		t.Error("pending label not painted text.dim")
	}
}

// TestLoadingScreen_CounterSpacedOnlyOnActiveRestore asserts the spaced `N / M`
// counter renders ONLY on the active "Restoring sessions" row, in text.detail,
// and never on any other label.
func TestLoadingScreen_CounterSpacedOnlyOnActiveRestore(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, false)
	visible := ansi.Strip(out)

	if !strings.Contains(visible, "8 / 12") {
		t.Errorf("active restore row missing spaced counter %q:\n%s", "8 / 12", visible)
	}
	// The un-spaced form from the accumulator must NOT leak through verbatim.
	if strings.Contains(visible, "8/12") {
		t.Errorf("loading screen rendered the un-spaced counter %q; want %q", "8/12", "8 / 12")
	}
	if !strings.Contains(out, tokenFgSeq(t, theme.MV.TextDetail, theme.Dark)) {
		t.Error("counter not painted text.detail")
	}
	// Exactly one counter on the whole screen (only the active restore row).
	if n := strings.Count(visible, "8 / 12"); n != 1 {
		t.Errorf("counter rendered %d times, want exactly 1 (active restore row only)", n)
	}
}

// TestLoadingScreen_SuppressesCounterWhenM0 asserts the M=0 empty-restore case
// renders no counter on the "Restoring sessions" row (task 5-4 suppresses it).
func TestLoadingScreen_SuppressesCounterWhenM0(t *testing.T) {
	var p LoadingProgress
	for i := 1; i <= totalBootstrapSteps; i++ {
		// Step 6 completes with RestoreM==0 (no per-session events) — empty restore.
		p = p.Apply(BootstrapProgressMsg{Index: i, Name: "step"})
	}
	out := renderLoadingScreen(p.View(), 80, 24, theme.Dark, false)
	visible := ansi.Strip(out)

	if strings.Contains(visible, "/") {
		t.Errorf("M=0 empty-restore screen rendered a counter slash; want none:\n%s", visible)
	}
}

// TestLoadingScreen_IsRealListNotInPlaceSwap asserts the tick-list is a real list
// of multiple distinct rows (every label on its own line), not a single in-place
// text swap.
func TestLoadingScreen_IsRealListNotInPlaceSwap(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, false)
	lines := strings.Split(ansi.Strip(out), "\n")

	seen := map[string]int{}
	for _, line := range lines {
		for _, label := range labelOrder {
			if strings.Contains(line, label) {
				seen[label]++
			}
		}
	}
	if len(seen) != len(labelOrder) {
		t.Errorf("tick-list shows %d distinct labels, want %d (a real list, not an in-place swap)", len(seen), len(labelOrder))
	}
	for label, count := range seen {
		if count != 1 {
			t.Errorf("label %q appears on %d lines, want exactly 1", label, count)
		}
	}
}

// TestViewLoading_PaintsCanvasFromFrameOneGated asserts the loading page paints
// the resolved (pinned/dark-fallback) canvas from frame one and is held behind
// the §2.6 detect-or-timeout first-paint gate — no paint-then-flip. Until the
// gate resolves the loading page shows the neutral blank frame; once resolved it
// paints the canvas-backed loading screen.
func TestViewLoading_PaintsCanvasFromFrameOneGated(t *testing.T) {
	m := New(fakeLister{}, WithServerStarted(true))
	// Arm the auto gate so the first-paint window is OPEN (no pin).
	m.armAppearanceDetection()
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Window open: the loading page must NOT paint the canvas yet (neutral blank).
	held := model.(Model)
	if held.modeResolved() {
		t.Fatal("gate resolved prematurely; expected the first-paint window to be open")
	}
	if strings.Contains(held.View().Content, tokenBgSeq(t, theme.MV.Canvas, theme.Dark)) {
		t.Error("loading page painted the canvas before the gate resolved (paint-then-flip risk)")
	}

	// Resolve via the timeout (dark fallback) — now the loading screen paints the
	// dark canvas from this, its first real frame.
	model, _ = model.Update(appearanceTimeoutMsg{})
	resolved := model.(Model)
	if !resolved.modeResolved() {
		t.Fatal("gate did not resolve on the timeout")
	}
	if !strings.Contains(resolved.View().Content, tokenBgSeq(t, theme.MV.Canvas, theme.Dark)) {
		t.Error("loading page did not paint the dark canvas after the gate resolved")
	}
}

// TestLoading_TransitionDualGated asserts the loading page stays until BOTH the
// terminal BootstrapCompleteMsg AND LoadingMinElapsedMsg arrive — neither alone
// dismisses it.
func TestLoading_TransitionDualGated(t *testing.T) {
	t.Run("complete-then-elapsed", func(t *testing.T) {
		m := New(fakeLister{}, WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(BootstrapCompleteMsg{})
		if model.(Model).ActivePage() != PageLoading {
			t.Fatal("dismissed on BootstrapCompleteMsg alone; want dual-gate")
		}
		model, _ = model.Update(LoadingMinElapsedMsg{})
		if model.(Model).ActivePage() == PageLoading {
			t.Error("did not dismiss after BOTH complete + elapsed")
		}
	})
	t.Run("elapsed-then-complete", func(t *testing.T) {
		m := New(fakeLister{}, WithServerStarted(true))
		var model tea.Model = m
		model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

		model, _ = model.Update(LoadingMinElapsedMsg{})
		if model.(Model).ActivePage() != PageLoading {
			t.Fatal("dismissed on LoadingMinElapsedMsg alone; want dual-gate")
		}
		model, _ = model.Update(BootstrapCompleteMsg{})
		if model.(Model).ActivePage() == PageLoading {
			t.Error("did not dismiss after BOTH elapsed + complete")
		}
	})
}

// TestLoadingScreen_DegradesNarrowWithoutOverflow asserts the loading screen
// degrades on a narrow terminal — 5-row block banner → single-row letter-spaced
// wordmark → compact wordmark — and never emits a row wider than the content
// width (no overflow). The block-banner threshold is now ~37 cells (the wider
// 5-row solid-block art + gap + caret), so a width below that degrades to the
// single-row form.
func TestLoadingScreen_DegradesNarrowWithoutOverflow(t *testing.T) {
	view := midRestoreProgress().View()
	for _, w := range []int{80, 37, 30, 18, 12, 8} {
		out := renderLoadingScreen(view, w, 24, theme.Dark, false)
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("width %d: line %d overflows (%d cells):\n%q", w, i, lw, ansi.Strip(line))
			}
		}
	}

	// The block banner's first row, trimmed of its render-time right-pad, is the
	// signature substring used to detect "the block banner is on screen".
	blockSignature := strings.TrimRight(loadingWordmark[0], " ")

	// At/above the ~37-cell block width the 5-row banner is used; below it the
	// single-row wordmark; narrower still the compact wordmark.
	wide := ansi.Strip(renderLoadingScreen(view, 80, 24, theme.Dark, false))
	if !strings.Contains(wide, blockSignature) {
		t.Error("wide terminal should render the 5-row block banner")
	}
	atThreshold := ansi.Strip(renderLoadingScreen(view, loadingBlockBannerWidth, 24, theme.Dark, false))
	if !strings.Contains(atThreshold, blockSignature) {
		t.Errorf("terminal at the block width (%d) should render the 5-row block banner", loadingBlockBannerWidth)
	}
	justBelow := ansi.Strip(renderLoadingScreen(view, loadingBlockBannerWidth-1, 24, theme.Dark, false))
	if strings.Contains(justBelow, blockSignature) {
		t.Errorf("terminal one cell below the block width (%d) should NOT render the block banner", loadingBlockBannerWidth-1)
	}
	mid := ansi.Strip(renderLoadingScreen(view, 18, 24, theme.Dark, false))
	if strings.Contains(mid, blockSignature) {
		t.Error("narrow terminal should NOT render the block banner (degrade to single-row)")
	}
	if !strings.Contains(mid, fullWordmark) {
		t.Errorf("narrow terminal should degrade to the single-row wordmark %q:\n%s", fullWordmark, mid)
	}
	compact := ansi.Strip(renderLoadingScreen(view, 8, 24, theme.Dark, false))
	if strings.Contains(compact, fullWordmark) {
		t.Error("very narrow terminal should NOT render the letter-spaced wordmark")
	}
	if !strings.Contains(compact, headerCompactWordmark) {
		t.Errorf("very narrow terminal should degrade to the compact wordmark %q:\n%s", headerCompactWordmark, compact)
	}
}

// TestLoadingScreen_ShortNoOverflow asserts a short terminal never overflows its
// height — the composed block fits within the content height. The bar (1 row) +
// the 5-row tick-list is the irreducible floor (6 rows); below that the terminal
// is below minimum support. The height degrade drops the inter-section gaps, then
// the (now taller, 5-row) block banner — collapsing it to the single-row form and
// finally dropping the wordmark entirely — so the step-list never overflows.
func TestLoadingScreen_ShortNoOverflow(t *testing.T) {
	view := midRestoreProgress().View()
	for _, h := range []int{24, 13, 12, 8, 6} {
		out := renderLoadingScreen(view, 80, h, theme.Dark, false)
		if got := lipgloss.Height(out); got > h {
			t.Errorf("height %d: loading screen is %d rows tall (overflow)", h, got)
		}
	}

	// At a short-but-fits height the wordmark is dropped (the bar + list floor),
	// but the step-list still renders all five rows.
	short := ansi.Strip(renderLoadingScreen(view, 80, 6, theme.Dark, false))
	for _, label := range labelOrder {
		if !strings.Contains(short, label) {
			t.Errorf("short terminal dropped step-list label %q (the list must never be cut):\n%s", label, short)
		}
	}
}

// TestLoadingScreen_ColourlessNoCanvasGlyphDistinct asserts the NO_COLOR
// carve-out: no canvas/hue is painted, but the state stays distinguishable by
// glyph (✓/◐/·) so the screen is usable colourless.
func TestLoadingScreen_ColourlessNoCanvasGlyphDistinct(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, theme.Dark, true)

	// No canvas background and no hue SGR survive the colourless path.
	if strings.Contains(out, tokenBgSeq(t, theme.MV.Canvas, theme.Dark)) {
		t.Error("colourless loading screen painted the canvas background")
	}
	if strings.Contains(out, tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark)) {
		t.Error("colourless loading screen painted an accent.violet hue")
	}
	if strings.Contains(out, tokenBgSeq(t, theme.MV.AccentViolet, theme.Dark)) {
		t.Error("colourless loading screen painted the violet bar fill")
	}

	// State stays glyph-distinct.
	visible := ansi.Strip(out)
	for _, glyph := range []string{loadingGlyphDone, loadingGlyphActive, loadingGlyphPending} {
		if !strings.Contains(visible, glyph) {
			t.Errorf("colourless loading screen missing distinguishing glyph %q:\n%s", glyph, visible)
		}
	}
}

// TestWarmPath_NoLoadingScreen asserts the warm path never lands on PageLoading
// (task 5-1 gates this on serverStarted), so viewLoading is never rendered.
func TestWarmPath_NoLoadingScreen(t *testing.T) {
	m := New(fakeLister{})
	if m.ActivePage() == PageLoading {
		t.Fatal("warm path landed on PageLoading; want straight to the picker")
	}
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: []tmux.Session{{Name: "a"}}})
	if model.(Model).ActivePage() == PageLoading {
		t.Error("warm path transitioned onto PageLoading; want never")
	}
}
