package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// tokenFgSeq returns the bare `38;2;r;g;b` foreground SGR parameter substring for
// a role token in the given mode. The header composes the foreground with a
// background (and sometimes bold) into ONE SGR sequence, so the standalone
// `\x1b[...m` wrapper never appears verbatim — the colour role is asserted by the
// `38;2;...` core, which is present regardless of the other params merged in.
func tokenFgSeq(t *testing.T, tok theme.Token, m theme.Mode) string {
	t.Helper()
	probe := lipgloss.NewStyle().Foreground(tok.ColorFor(m)).Render("x")
	// probe is like "\x1b[38;2;192;202;245mx\x1b[m"; slice out the params between
	// the CSI "[" and the final "m".
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		t.Fatalf("could not derive foreground SGR core from %q", probe)
	}
	return probe[start+1 : end]
}

// tokenBgSeq returns the bare `48;2;r;g;b` background SGR parameter substring for
// a role token in the given mode — the background analogue of tokenFgSeq, used to
// assert a tint IS or is NOT painted (e.g. the §11.3 info band must NOT carry the
// bg.warning flash tint).
func tokenBgSeq(t *testing.T, tok theme.Token, m theme.Mode) string {
	t.Helper()
	probe := lipgloss.NewStyle().Background(tok.ColorFor(m)).Render("x")
	start := strings.IndexByte(probe, '[')
	end := strings.IndexByte(probe, 'm')
	if start < 0 || end <= start {
		t.Fatalf("could not derive background SGR core from %q", probe)
	}
	return probe[start+1 : end]
}

// TestHeaderBlock_RendersWordmarkCaretSubtitleRule asserts the §3.1 header block
// renders the PORTAL wordmark (letter-spaced text.primary), an immediately-right
// violet block caret, a right-aligned "session manager" subtitle (text.detail),
// over a full-width 2px (two-row, heavy) border.separator rule — all via tokens.
func TestHeaderBlock_RendersWordmarkCaretSubtitleRule(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const w = 80
			header := renderHeaderBlock(w, tc.mode, false)

			// Wordmark glyphs are present, letter-spaced (a space between each glyph).
			if !strings.Contains(header, "P O R T A L") {
				t.Errorf("header does not contain the letter-spaced wordmark %q:\n%s", "P O R T A L", header)
			}
			// Caret glyph present.
			if !strings.Contains(header, "▌") {
				t.Errorf("header does not contain the block caret %q:\n%s", "▌", header)
			}
			// Subtitle present.
			if !strings.Contains(header, "session manager") {
				t.Errorf("header does not contain the subtitle %q:\n%s", "session manager", header)
			}

			// Colour roles: wordmark text.primary, caret accent.violet, subtitle
			// text.detail, the rule border.separator — each via its token.
			for _, want := range []struct {
				role string
				tok  theme.Token
			}{
				{"text.primary wordmark", theme.MV.TextPrimary},
				{"accent.violet caret", theme.MV.AccentViolet},
				{"text.detail subtitle", theme.MV.TextDetail},
			} {
				if seq := tokenFgSeq(t, want.tok, tc.mode); !strings.Contains(header, seq) {
					t.Errorf("header missing the %s foreground role sequence %q", want.role, seq)
				}
			}
			// The 2px rule is drawn with the border.separator colour (used as a
			// foreground for the heavy rule glyphs).
			if seq := tokenFgSeq(t, theme.MV.BorderSeparator, tc.mode); !strings.Contains(header, seq) {
				t.Errorf("header missing the border.separator rule role sequence %q", seq)
			}

			// Every line is exactly the requested width (no overflow). The header is
			// the wordmark band + the separator rule beneath.
			lines := strings.Split(header, "\n")
			if got := lipgloss.Height(header); got < 2 {
				t.Errorf("header height = %d, want >= 2 (band + rule)", got)
			}
			for i, line := range lines {
				if lw := lipgloss.Width(line); lw != w {
					t.Errorf("header line %d width = %d, want exactly %d (full-width, no overflow)", i, lw, w)
				}
			}
		})
	}
}

// visibleContent strips ANSI/SGR sequences and returns what would actually print
// on the terminal — used to assert a row is BLANK (no non-space printable glyph)
// regardless of any canvas-background SGR painted across its cells.
func visibleContent(line string) string {
	return ansi.Strip(line)
}

// TestHeaderBlock_VerticalRhythm pins the §3.1 header-block row structure: the
// PORTAL band, the separator rule FLUSH beneath it (the rule is the wordmark's
// underline — no blank between them), then ONE blank row (rule → "Sessions"
// section-header gap). Three rows total. The wordmark→rule gap is flush because a
// blank row there renders visibly TALLER than the Vinset=1 gutter above the band
// (glyph-to-glyph vs edge-to-glyph), which read as an imbalance. The blank row
// carries NO visible glyph (canvas-painted spaces only). This guard exists because
// a spacing miss slipped through the element-scoped checks; it locks the structure
// so it cannot silently regress.
func TestHeaderBlock_VerticalRhythm(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, theme.Dark, false)
	lines := strings.Split(header, "\n")

	if len(lines) != 3 {
		t.Fatalf("header block has %d lines, want exactly 3 (band, rule, 1 blank):\n%s", len(lines), header)
	}

	// Line 0: the wordmark band.
	if !strings.Contains(visibleContent(lines[0]), "P O R T A L") {
		t.Errorf("line 0 should be the PORTAL band, got %q", visibleContent(lines[0]))
	}
	// Line 1: the separator rule, FLUSH beneath the band (no blank between).
	if !strings.Contains(visibleContent(lines[1]), headerRuleGlyph) {
		t.Errorf("line 1 should be the separator rule flush under the band, got %q", visibleContent(lines[1]))
	}
	// Line 2: blank (rule → "Sessions" section-header gap).
	if got := strings.TrimSpace(visibleContent(lines[2])); got != "" {
		t.Errorf("line 2 should be blank (rule → section-header gap), got visible %q", got)
	}

	// Every line is exactly the requested width (the blank rows are full-width
	// canvas-painted, not collapsed).
	for i, line := range lines {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("header block line %d width = %d, want exactly %d", i, lw, w)
		}
	}
}

// TestHeaderBlock_BlankRowsPaintCanvas asserts the band→rule and rule→section
// blank rows carry the owned canvas background (canvas-painted spaces) so there is
// no terminal-bg island between the chrome rows. Under the NO_COLOR carve-out the
// same rows carry no canvas SGR (native bg).
func TestHeaderBlock_BlankRowsPaintCanvas(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, theme.Dark, false)
	lines := strings.Split(header, "\n")
	seq := canvasSeq(t, theme.Dark)
	for _, idx := range []int{2} {
		if !strings.Contains(lines[idx], seq) {
			t.Errorf("blank row %d does not paint the canvas background sequence %q: %q", idx, seq, lines[idx])
		}
	}

	colourless := renderHeaderBlock(w, theme.Dark, true)
	clLines := strings.Split(colourless, "\n")
	if len(clLines) != 3 {
		t.Fatalf("colourless header block has %d lines, want 3", len(clLines))
	}
	for _, idx := range []int{2} {
		if strings.Contains(clLines[idx], seq) {
			t.Errorf("colourless blank row %d still paints the canvas background sequence %q", idx, seq)
		}
	}
}

// TestHeaderBlock_SeparatorRule asserts the separator rule is a full-width
// single-row heavy rule (terminal 2px ≈ a heavy/thick horizontal rule, matching
// the Paper frame weight — the reference shows one thin full-width line).
func TestHeaderBlock_SeparatorRule(t *testing.T) {
	const w = 80
	header := renderHeaderBlock(w, theme.Dark, false)
	rule := headerSeparatorRule(w, theme.Dark, false)
	if got := lipgloss.Height(rule); got != 1 {
		t.Errorf("separator rule height = %d, want 1 (single full-width heavy rule matching the frame)", got)
	}
	if lw := lipgloss.Width(rule); lw != w {
		t.Errorf("rule width = %d, want %d (full-width)", lw, w)
	}
	// The rule row appears verbatim as one of the block's lines (its exact
	// position — band, 1 blank, rule, 2 blank — is pinned by
	// TestHeaderBlock_VerticalRhythm; the block no longer ENDS with the rule now
	// that the trailing rule → section-header gap blanks follow it).
	if !strings.Contains(header, rule) {
		t.Errorf("header block does not contain the separator rule row")
	}
}

// TestHeaderBlock_NarrowDegradeProgressive asserts the per-dimension narrow
// degrade: at full width both wordmark and subtitle render; below the subtitle
// threshold the subtitle drops but the full wordmark stays; below the wordmark
// threshold the wordmark collapses to its compact form. Short-but-wide keeps the
// full wordmark (width is the only driver here).
func TestHeaderBlock_NarrowDegradeProgressive(t *testing.T) {
	full := renderHeaderBlock(120, theme.Dark, false)
	if !strings.Contains(full, "P O R T A L") {
		t.Errorf("wide header missing full wordmark:\n%s", full)
	}
	if !strings.Contains(full, "session manager") {
		t.Errorf("wide header missing subtitle:\n%s", full)
	}

	// Step 1: just below the subtitle threshold — subtitle drops, wordmark stays.
	noSub := renderHeaderBlock(headerSubtitleMinWidth-1, theme.Dark, false)
	if strings.Contains(noSub, "session manager") {
		t.Errorf("header at width %d still shows the subtitle (step-1 drop failed):\n%s", headerSubtitleMinWidth-1, noSub)
	}
	if !strings.Contains(noSub, "P O R T A L") {
		t.Errorf("header at width %d dropped the full wordmark too early (step-2 before step-1):\n%s", headerSubtitleMinWidth-1, noSub)
	}

	// Step 2: below the wordmark threshold — wordmark collapses to compact.
	compact := renderHeaderBlock(headerWordmarkMinWidth-1, theme.Dark, false)
	if strings.Contains(compact, "P O R T A L") {
		t.Errorf("header at width %d still shows the full letter-spaced wordmark (compact collapse failed):\n%s", headerWordmarkMinWidth-1, compact)
	}
	if !strings.Contains(compact, headerCompactWordmark) {
		t.Errorf("header at width %d missing the compact wordmark %q:\n%s", headerWordmarkMinWidth-1, headerCompactWordmark, compact)
	}
}

// TestHeaderBlock_NeverOverflowsAtMinWidth asserts the header never overflows the
// viewport at the minimum supported terminal width: every line is exactly the
// requested width (no line wider than the terminal).
func TestHeaderBlock_NeverOverflowsAtMinWidth(t *testing.T) {
	for _, w := range []int{minTerminalWidth, headerWordmarkMinWidth - 1, headerSubtitleMinWidth - 1, 20, 8} {
		header := renderHeaderBlock(w, theme.Dark, false)
		for i, line := range strings.Split(header, "\n") {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("at width %d, header line %d width = %d (overflow)", w, i, lw)
			}
		}
	}
}

// TestHeaderBlock_PaintsOnCanvasNoEdgeBleed asserts the header cells carry the
// owned canvas background (leaf .Background(canvas)) so there is no terminal-bg
// island behind the band or in the right-aligned spacer gap.
func TestHeaderBlock_PaintsOnCanvasNoEdgeBleed(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := renderHeaderBlock(80, tc.mode, false)
			seq := canvasSeq(t, tc.mode)
			if !strings.Contains(header, seq) {
				t.Errorf("header does not paint the canvas background sequence %q:\n%s", seq, header)
			}
		})
	}
}

// TestHeaderBlock_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless header carries no canvas background SGR and no foreground
// hue — it renders on the terminal's native fg/bg, structure intact.
func TestHeaderBlock_ColourlessDropsHueAndCanvas(t *testing.T) {
	header := renderHeaderBlock(80, theme.Dark, true)

	// Structure preserved: wordmark + caret + subtitle + rule all present.
	if !strings.Contains(header, "P O R T A L") {
		t.Errorf("colourless header missing wordmark:\n%s", header)
	}
	if !strings.Contains(header, "session manager") {
		t.Errorf("colourless header missing subtitle:\n%s", header)
	}
	// No canvas background painted.
	if seq := canvasSeq(t, theme.Dark); strings.Contains(header, seq) {
		t.Errorf("colourless header still paints the canvas background sequence %q", seq)
	}
	// No foreground hue from any header role.
	for _, tok := range []theme.Token{theme.MV.TextPrimary, theme.MV.AccentViolet, theme.MV.TextDetail, theme.MV.BorderSeparator} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(header, seq) {
			t.Errorf("colourless header still emits a foreground role sequence %q", seq)
		}
	}
}

// TestHeaderBlock_ZeroWidthFallsBackTo80 asserts a zero/unset width falls back to
// 80 so the header still composes (mirroring fillCanvas / viewLoading).
func TestHeaderBlock_ZeroWidthFallsBackTo80(t *testing.T) {
	header := renderHeaderBlock(0, theme.Dark, false)
	for i, line := range strings.Split(header, "\n") {
		if lw := lipgloss.Width(line); lw != 80 {
			t.Errorf("zero-width header line %d width = %d, want 80 fallback", i, lw)
		}
	}
}

// TestHeaderHeight_EqualsThreeRows asserts the §3.1 header-block height the list
// budget reserves (m.headerHeight) is exactly 3 at a normal width — band + rule
// (flush) + 1 blank. The budget auto-reserves whatever this measures, so the value
// is the contract the §3.5 pagination invariant depends on. It stays 3 under the
// NO_COLOR carve-out (the blank row exists either way; only the SGR differs).
func TestHeaderHeight_EqualsThreeRows(t *testing.T) {
	const w = 80
	for _, tc := range []struct {
		name       string
		colourless bool
	}{
		{"coloured", false},
		{"colourless", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := New(fakeLister{}, WithCanvasMode(theme.Dark))
			m.colourless = tc.colourless
			if got := m.headerHeight(w); got != 3 {
				t.Errorf("headerHeight(%d) = %d, want 3 (band, rule, 1 blank)", w, got)
			}
		})
	}
}

// TestViewSessionList_ComposesHeaderFirst asserts the Sessions view composes the
// header block ABOVE the bubbles/list title — the header is the first visible
// chrome, the list title sits below it.
func TestViewSessionList_ComposesHeaderFirst(t *testing.T) {
	m := newCanvasTestModel(t, 90, 24, theme.Dark)
	view := m.viewSessionList()

	portalIdx := strings.Index(view, "P O R T A L")
	if portalIdx < 0 {
		t.Fatalf("Sessions view does not contain the header wordmark:\n%s", view)
	}
	titleIdx := strings.Index(view, "Sessions")
	if titleIdx < 0 {
		t.Fatalf("Sessions view does not contain the list title")
	}
	if portalIdx > titleIdx {
		t.Errorf("header wordmark (idx %d) appears after the list title (idx %d); header must be first", portalIdx, titleIdx)
	}
}

// TestHeaderHeight_SubtractedFromListBudget asserts the header height is folded
// into the list height budget at every size-apply site so pagination stays
// exact: the list's per-page item count drops by exactly the header height when
// the header is introduced (compared to the same model sized without the header).
func TestHeaderHeight_SubtractedFromListBudget(t *testing.T) {
	const w, h = 90, 24
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)

	headerH := lipgloss.Height(renderHeaderBlock(w, theme.Dark, false))
	if headerH <= 0 {
		t.Fatalf("header height = %d, want > 0", headerH)
	}

	// The composed Sessions view (header + list + footer) must never exceed termH.
	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("composed Sessions view height = %d, want <= %d (header folded into budget)", got, h)
	}
	// And the full filled frame is exactly termH (no overflow class regression).
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("filled frame height = %d, want exactly %d", got, h)
	}
}

// TestHeaderHeight_CountedAtEverySizeApplySite asserts the construction seed, a
// window-resize, and a rebuild all reserve the header height — the composed view
// stays within termH after each path.
func TestHeaderHeight_CountedAtEverySizeApplySite(t *testing.T) {
	const w, h = 90, 20
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	// Construction seed (New → applySessionListSize(80,24)) then a resize.
	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = updated.(Model)
	m.applySessions(sessions) // rebuild path

	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("after resize+rebuild composed view height = %d, want <= %d", got, h)
	}
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("after resize+rebuild filled frame height = %d, want exactly %d", got, h)
	}
}
