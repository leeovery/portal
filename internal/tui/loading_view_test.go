package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func midRestoreProgress() LoadingProgress {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1})
	p = p.Apply(BootstrapProgressMsg{Index: 2})
	p = p.Apply(BootstrapProgressMsg{Index: 3})
	p = p.Apply(BootstrapProgressMsg{Index: 4})
	p = p.Apply(BootstrapProgressMsg{Index: restoreStep, RestoreN: 8, RestoreM: 12})
	return p
}

// The one frame carrying all three glyphs: Restoring sessions, the last label,
// is still pending only while an earlier label is active.
func startingSaverProgress() LoadingProgress {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1})
	p = p.Apply(BootstrapProgressMsg{Index: 2})
	return p
}

func TestLoadingScreen_RendersBlockBannerCaretBarAndList(t *testing.T) {
	view := startingSaverProgress().View()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), false)
	visible := ansi.Strip(out)

	for i, row := range loadingWordmark {
		if !strings.Contains(visible, strings.TrimRight(row, " ")) {
			t.Errorf("loading screen missing block banner row %d %q:\n%s", i, row, visible)
		}
	}

	for _, label := range labelOrder {
		if !strings.Contains(visible, label) {
			t.Errorf("loading screen missing step-list label %q:\n%s", label, visible)
		}
	}

	for _, glyph := range []string{loadingGlyphDone, loadingGlyphActive, loadingGlyphPending} {
		if !strings.Contains(visible, glyph) {
			t.Errorf("loading screen missing tick glyph %q:\n%s", glyph, visible)
		}
	}

	if !strings.Contains(out, tokenFgSeq(t, testDarkTheme(t).TextPrimary)) {
		t.Error("loading screen does not paint the wordmark in text.primary")
	}
	if !strings.Contains(out, tokenBgSeq(t, testDarkTheme(t).AccentPrimary)) {
		t.Error("loading screen does not paint the filled bar with the accent.primary background")
	}
	if !strings.Contains(out, tokenBgSeq(t, testDarkTheme(t).BgSubtle)) {
		t.Error("loading screen does not paint the bar track with the bg.subtle background")
	}
}

func blockLeadingPad(line string) int {
	stripped := ansi.Strip(line)
	return len(stripped) - len(strings.TrimLeft(stripped, " "))
}

func firstLineContaining(lines []string, sub string) string {
	for _, line := range lines {
		if strings.Contains(ansi.Strip(line), sub) {
			return line
		}
	}
	return ""
}

func firstBarLine(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(ansi.Strip(line))
		if trimmed == "" {
			continue
		}
		allBlock := true
		for _, r := range trimmed {
			if string(r) != loadingBarFilledGlyph {
				allBlock = false
				break
			}
		}
		if allBlock {
			return line
		}
	}
	return ""
}

func TestLoadingScreen_BarWidthEqualsWordmarkWidth(t *testing.T) {
	wordmark := renderBlockWordmark(testDarkTheme(t), false)
	wordmarkW := lipgloss.Width(wordmark)

	bar := renderLoadingBar(midRestoreProgress().View().BarFraction, 80, wordmarkW, testDarkTheme(t), false)
	barW := lipgloss.Width(bar)

	if barW != wordmarkW {
		t.Errorf("bar width = %d, want %d (the bar must span the full block-wordmark width)", barW, wordmarkW)
	}
}

func TestLoadingScreen_BlockColumnIsCentered(t *testing.T) {
	block := composeLoadingBlock(midRestoreProgress().View(), 80, 24, testDarkTheme(t), false)
	lines := strings.Split(block, "\n")
	blockWidth := lipgloss.Width(block)

	wordmarkLine := firstLineContaining(lines, strings.TrimRight(loadingWordmark[0], " "))
	barLine := firstBarLine(lines)
	listLine := firstLineContaining(lines, LabelRestoringSessions)
	if wordmarkLine == "" || barLine == "" || listLine == "" {
		t.Fatalf("could not locate all three elements in the block:\n%s", ansi.Strip(block))
	}

	if pad := blockLeadingPad(wordmarkLine); pad != 0 {
		t.Errorf("wordmark line is not flush at the block left edge: leading pad = %d", pad)
	}
	if pad := blockLeadingPad(barLine); pad != 0 {
		t.Errorf("bar line is not flush at the block left edge: leading pad = %d", pad)
	}

	listWidth := lipgloss.Width(strings.TrimSpace(ansi.Strip(listLine)))
	wantPad := (blockWidth - listWidth) / 2
	gotPad := blockLeadingPad(listLine)
	if gotPad == 0 {
		t.Errorf("list row is left-flush (pad 0) — the block is left-aligned, not centred (regression)")
	}
	if gotPad < wantPad-1 || gotPad > wantPad+1 {
		t.Errorf("list row not centred: leading pad = %d, want ≈ %d (block width %d, list width %d)", gotPad, wantPad, blockWidth, listWidth)
	}
}

func TestLoadingScreen_SectionGapsAreTwoRows(t *testing.T) {
	block := composeLoadingBlock(midRestoreProgress().View(), 80, 24, testDarkTheme(t), false)
	lines := strings.Split(block, "\n")

	isBarRow := func(line string) bool {
		trimmed := strings.TrimSpace(ansi.Strip(line))
		if trimmed == "" {
			return false
		}
		for _, r := range trimmed {
			if string(r) != loadingBarFilledGlyph {
				return false
			}
		}
		return true
	}

	barIdx, listIdx := -1, -1
	lastWordmarkIdx := -1
	for i, line := range lines {
		stripped := ansi.Strip(line)
		if strings.Contains(stripped, strings.TrimRight(loadingWordmark[len(loadingWordmark)-1], " ")) {
			lastWordmarkIdx = i
		}
		if barIdx == -1 && isBarRow(line) {
			barIdx = i
		}
		if listIdx == -1 && strings.Contains(stripped, LabelCheckingZmx) {
			listIdx = i
		}
	}
	if lastWordmarkIdx == -1 || barIdx == -1 || listIdx == -1 {
		t.Fatalf("could not locate wordmark/bar/list rows in block:\n%s", ansi.Strip(block))
	}

	if gap := barIdx - lastWordmarkIdx - 1; gap != 2 {
		t.Errorf("wordmark→bar gap = %d rows, want 2", gap)
	}
	if gap := listIdx - barIdx - 1; gap != 2 {
		t.Errorf("bar→list gap = %d rows, want 2", gap)
	}
}

func TestLoadingScreen_TickRowsLeftAlignedWithinList(t *testing.T) {
	view := midRestoreProgress().View()
	list := renderTickList(view.Labels, 80, testDarkTheme(t), false)
	for i, line := range strings.Split(list, "\n") {
		if pad := blockLeadingPad(line); pad != 0 {
			t.Errorf("tick row %d is not left-flush within the list block: leading pad = %d (icons must share a column)", i, pad)
		}
	}
}

func errorFrameView() LoadingProgressView {
	var p LoadingProgress
	p = p.Apply(BootstrapProgressMsg{Index: 1})
	return p.FailedView(2, "Portal failed to raise the restoring flag: permission denied")
}

func TestLoadingScreen_ErrorFrameCentredComposition(t *testing.T) {
	view := errorFrameView()
	block := composeLoadingBlock(view, 80, 24, testDarkTheme(t), false)
	lines := strings.Split(block, "\n")
	blockWidth := lipgloss.Width(block)

	msgLine := firstLineContaining(lines, "Portal failed to raise")
	if msgLine == "" {
		t.Fatalf("error block missing the fatal message:\n%s", ansi.Strip(block))
	}
	if pad := blockLeadingPad(msgLine); pad != 0 {
		t.Errorf("message caption is not flush at the block left edge: leading pad = %d", pad)
	}

	listLine := firstLineContaining(lines, LabelCheckingZmx)
	if listLine == "" {
		t.Fatalf("error block missing the step-list:\n%s", ansi.Strip(block))
	}
	if pad := blockLeadingPad(listLine); pad == 0 {
		t.Error("step-list row is left-flush in the error frame — the long message yanked it left (regression)")
	}

	hintLine := firstLineContaining(lines, "q quit")
	if hintLine == "" {
		t.Fatalf("error block missing the quit hint:\n%s", ansi.Strip(block))
	}
	hintStripped := ansi.Strip(hintLine)
	leadPad := lipgloss.Width(hintStripped) - lipgloss.Width(strings.TrimLeft(hintStripped, " "))
	trailPad := blockWidth - lipgloss.Width(strings.TrimRight(hintStripped, " "))
	if leadPad == 0 {
		t.Error("quit hint is left-flush (pad 0) — not centred in the error frame")
	}
	if diff := leadPad - trailPad; diff < -1 || diff > 1 {
		t.Errorf("quit hint not centred: leading pad %d vs trailing pad %d (block width %d)", leadPad, trailPad, blockWidth)
	}
}

func TestLoadingScreen_ErrorFrameNeverOverflowsHeight(t *testing.T) {
	view := errorFrameView()
	for _, h := range []int{24, 14, 12, 9, 7, 6} {
		out := renderLoadingScreen(view, 80, h, testDarkTheme(t), false)
		if got := lipgloss.Height(out); got > h {
			t.Errorf("height %d: error frame is %d rows tall (overflow)", h, got)
		}
	}
}

func TestLoadingScreen_CentredPaddingCarriesCanvasNoIslands(t *testing.T) {
	for _, frame := range []struct {
		name string
		view LoadingProgressView
	}{
		{"normal", midRestoreProgress().View()},
		{"error", errorFrameView()},
	} {
		t.Run(frame.name, func(t *testing.T) {
			th := testLightTheme(t)
			placed := lipgloss.Place(80, 24, lipgloss.Center, lipgloss.Center,
				composeLoadingBlock(frame.view, 80, 24, th, false))

			canvasBg := canvasBgParams(th.Canvas.Color())
			parser := ansi.NewParser()
			canvas := lipgloss.NewStyle().Background(th.Canvas.Color())

			for i, raw := range strings.Split(placed, "\n") {
				bf := backfillCanvasBackground(raw, canvasBg, parser)
				padded := padLineToCanvasWidth(bf, 80, canvas)
				if island := bareCanvasRun(padded); island != "" {
					t.Errorf("row %d has a canvas island (unpainted cell %q) — centring padding leaked the terminal bg", i, island)
				}
			}
		})
	}
}

func bareCanvasRun(line string) string {
	parser := ansi.NewParser()
	src := []byte(line)
	state := byte(0)
	bgActive := false
	var run []byte
	for len(src) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(src, state, parser)
		if n == 0 {
			break
		}
		if ansi.HasCsiPrefix(seq) && seq[len(seq)-1] == 'm' {
			bgActive = sgrBackgroundActive(bgActive, sgrParamsList(string(seq)))
		} else if width > 0 && !bgActive {
			run = append(run, seq...)
		}
		src = src[n:]
		state = newState
	}
	return string(run)
}

func TestLoadingScreen_CaretIsFlushAcrossBannerRows(t *testing.T) {
	block := ansi.Strip(renderBlockWordmark(testDarkTheme(t), false))
	lines := strings.Split(block, "\n")
	if len(lines) != len(loadingWordmark) {
		t.Fatalf("block banner has %d rows, want %d", len(lines), len(loadingWordmark))
	}

	caretCol := -1
	for i, line := range lines {
		col := strings.LastIndex(line, loadingCaretGlyph)
		if col < 0 {
			t.Fatalf("banner row %d has no caret glyph %q: %q", i, loadingCaretGlyph, line)
		}
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

func TestLoadingScreen_TickStatesUseSpecdTokens(t *testing.T) {
	view := midRestoreProgress().View()

	wantStates := map[string]LabelState{
		LabelCheckingZmx:       LabelDone,
		LabelStartingTheSaver:  LabelDone,
		LabelRestoringSessions: LabelActive,
	}
	for _, l := range view.Labels {
		if got := wantStates[l.Text]; got != l.State {
			t.Fatalf("label %q state = %v, want %v (fixture drift)", l.Text, l.State, got)
		}
	}

	doneRow := renderTickRow(LoadingLabel{Text: LabelCheckingZmx, State: LabelDone}, testDarkTheme(t), false)
	if !strings.Contains(ansi.Strip(doneRow), loadingGlyphDone) {
		t.Errorf("done row missing %q glyph: %q", loadingGlyphDone, ansi.Strip(doneRow))
	}
	if !strings.Contains(doneRow, tokenFgSeq(t, testDarkTheme(t).StatePositive)) {
		t.Error("done glyph not painted state.positive")
	}
	if !strings.Contains(doneRow, tokenFgSeq(t, testDarkTheme(t).TextTertiary)) {
		t.Error("done label not painted text.tertiary")
	}

	activeRow := renderTickRow(LoadingLabel{Text: LabelRestoringSessions, State: LabelActive, Counter: "8/12"}, testDarkTheme(t), false)
	if !strings.Contains(ansi.Strip(activeRow), loadingGlyphActive) {
		t.Errorf("active row missing %q glyph: %q", loadingGlyphActive, ansi.Strip(activeRow))
	}
	if !strings.Contains(activeRow, tokenFgSeq(t, testDarkTheme(t).AccentMode)) {
		t.Error("active glyph not painted accent.mode")
	}
	if !strings.Contains(activeRow, tokenFgSeq(t, testDarkTheme(t).TextPrimary)) {
		t.Error("active label not painted text.primary")
	}

	pendingRow := renderTickRow(LoadingLabel{Text: LabelStartingTheSaver, State: LabelPending}, testDarkTheme(t), false)
	if !strings.Contains(ansi.Strip(pendingRow), loadingGlyphPending) {
		t.Errorf("pending row missing %q glyph: %q", loadingGlyphPending, ansi.Strip(pendingRow))
	}
	if !strings.Contains(pendingRow, tokenFgSeq(t, testDarkTheme(t).TextFaint)) {
		t.Error("pending glyph not painted text.faint")
	}
	if !strings.Contains(pendingRow, tokenFgSeq(t, testDarkTheme(t).TextSubtle)) {
		t.Error("pending label not painted text.subtle")
	}
}

func TestLoadingScreen_CounterSpacedOnlyOnActiveRestore(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), false)
	visible := ansi.Strip(out)

	if !strings.Contains(visible, "8 / 12") {
		t.Errorf("active restore row missing spaced counter %q:\n%s", "8 / 12", visible)
	}
	if strings.Contains(visible, "8/12") {
		t.Errorf("loading screen rendered the un-spaced counter %q; want %q", "8/12", "8 / 12")
	}
	if !strings.Contains(out, tokenFgSeq(t, testDarkTheme(t).TextMuted)) {
		t.Error("counter not painted text.muted")
	}
	if n := strings.Count(visible, "8 / 12"); n != 1 {
		t.Errorf("counter rendered %d times, want exactly 1 (active restore row only)", n)
	}
}

func TestLoadingScreen_SuppressesCounterWhenM0(t *testing.T) {
	var p LoadingProgress
	for i := 1; i <= totalBootstrapSteps; i++ {
		p = p.Apply(BootstrapProgressMsg{Index: i})
	}
	out := renderLoadingScreen(p.View(), 80, 24, testDarkTheme(t), false)
	visible := ansi.Strip(out)

	if strings.Contains(visible, "/") {
		t.Errorf("M=0 empty-restore screen rendered a counter slash; want none:\n%s", visible)
	}
}

func TestLoadingScreen_IsRealListNotInPlaceSwap(t *testing.T) {
	view := midRestoreProgress().View()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), false)
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

func TestViewLoading_PaintsCanvasFromFrameOneGated(t *testing.T) {
	m := New(fakeLister{}, WithBootstrapping(true), WithThemeNomination(testBuiltinPair(t)))
	m.armAppearanceDetection()
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	held := model.(Model)
	if held.modeResolved() {
		t.Fatal("gate resolved prematurely; expected the first-paint window to be open")
	}
	if strings.Contains(held.View().Content, tokenBgSeq(t, testDarkTheme(t).Canvas)) {
		t.Error("loading page painted the canvas before the gate resolved (paint-then-flip risk)")
	}

	model, _ = model.Update(appearanceTimeoutMsg{})
	resolved := model.(Model)
	if !resolved.modeResolved() {
		t.Fatal("gate did not resolve on the timeout")
	}
	if !strings.Contains(resolved.View().Content, tokenBgSeq(t, testDarkTheme(t).Canvas)) {
		t.Error("loading page did not paint the dark canvas after the gate resolved")
	}
}

func TestLoading_TransitionDualGated(t *testing.T) {
	t.Run("complete-then-elapsed", func(t *testing.T) {
		m := New(fakeLister{}, WithBootstrapping(true))
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
		m := New(fakeLister{}, WithBootstrapping(true))
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

func TestLoadingScreen_DegradesNarrowWithoutOverflow(t *testing.T) {
	view := midRestoreProgress().View()
	for _, w := range []int{80, 37, 30, 18, 12, 8} {
		out := renderLoadingScreen(view, w, 24, testDarkTheme(t), false)
		for i, line := range strings.Split(out, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("width %d: line %d overflows (%d cells):\n%q", w, i, lw, ansi.Strip(line))
			}
		}
	}

	blockSignature := strings.TrimRight(loadingWordmark[0], " ")

	wide := ansi.Strip(renderLoadingScreen(view, 80, 24, testDarkTheme(t), false))
	if !strings.Contains(wide, blockSignature) {
		t.Error("wide terminal should render the 5-row block banner")
	}
	atThreshold := ansi.Strip(renderLoadingScreen(view, loadingBlockBannerWidth, 24, testDarkTheme(t), false))
	if !strings.Contains(atThreshold, blockSignature) {
		t.Errorf("terminal at the block width (%d) should render the 5-row block banner", loadingBlockBannerWidth)
	}
	justBelow := ansi.Strip(renderLoadingScreen(view, loadingBlockBannerWidth-1, 24, testDarkTheme(t), false))
	if strings.Contains(justBelow, blockSignature) {
		t.Errorf("terminal one cell below the block width (%d) should NOT render the block banner", loadingBlockBannerWidth-1)
	}
	mid := ansi.Strip(renderLoadingScreen(view, 18, 24, testDarkTheme(t), false))
	if strings.Contains(mid, blockSignature) {
		t.Error("narrow terminal should NOT render the block banner (degrade to single-row)")
	}
	if !strings.Contains(mid, fullWordmark) {
		t.Errorf("narrow terminal should degrade to the single-row wordmark %q:\n%s", fullWordmark, mid)
	}
	compact := ansi.Strip(renderLoadingScreen(view, 8, 24, testDarkTheme(t), false))
	if strings.Contains(compact, fullWordmark) {
		t.Error("very narrow terminal should NOT render the letter-spaced wordmark")
	}
	if !strings.Contains(compact, headerCompactWordmark) {
		t.Errorf("very narrow terminal should degrade to the compact wordmark %q:\n%s", headerCompactWordmark, compact)
	}
}

func TestLoadingScreen_ShortNoOverflow(t *testing.T) {
	view := midRestoreProgress().View()
	for _, h := range []int{24, 13, 12, 8, 6} {
		out := renderLoadingScreen(view, 80, h, testDarkTheme(t), false)
		if got := lipgloss.Height(out); got > h {
			t.Errorf("height %d: loading screen is %d rows tall (overflow)", h, got)
		}
	}

	short := ansi.Strip(renderLoadingScreen(view, 80, 6, testDarkTheme(t), false))
	for _, label := range labelOrder {
		if !strings.Contains(short, label) {
			t.Errorf("short terminal dropped step-list label %q (the list must never be cut):\n%s", label, short)
		}
	}
}

func TestLoadingScreen_ColourlessNoCanvasGlyphDistinct(t *testing.T) {
	view := startingSaverProgress().View()
	out := renderLoadingScreen(view, 80, 24, testDarkTheme(t), true)

	if strings.Contains(out, tokenBgSeq(t, testDarkTheme(t).Canvas)) {
		t.Error("colourless loading screen painted the canvas background")
	}
	if strings.Contains(out, tokenFgSeq(t, testDarkTheme(t).AccentPrimary)) {
		t.Error("colourless loading screen painted an accent.primary hue")
	}
	if strings.Contains(out, tokenBgSeq(t, testDarkTheme(t).AccentPrimary)) {
		t.Error("colourless loading screen painted the violet bar fill")
	}

	visible := ansi.Strip(out)
	for _, glyph := range []string{loadingGlyphDone, loadingGlyphActive, loadingGlyphPending} {
		if !strings.Contains(visible, glyph) {
			t.Errorf("colourless loading screen missing distinguishing glyph %q:\n%s", glyph, visible)
		}
	}
}

func TestWarmPath_NoLoadingScreen(t *testing.T) {
	m := New(fakeLister{})
	if m.ActivePage() == PageLoading {
		t.Fatal("warm path landed on PageLoading; want straight to the picker")
	}
	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	model, _ = model.Update(SessionsMsg{Sessions: []Session{{Name: "a"}}})
	if model.(Model).ActivePage() == PageLoading {
		t.Error("warm path transitioned onto PageLoading; want never")
	}
}
