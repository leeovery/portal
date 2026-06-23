package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// Task spectrum-tui-design-5-5 — the honest loading screen render (§10.3 /
// §10.4). This is the VISUAL layer over task 5-4's pure LoadingProgress
// accumulator: it consumes a LoadingProgressView (bar fraction + the ordered five
// friendly labels with their done/active/pending states + counter) and composes
// the centred block the user watches during a cold boot.
//
// Reference: testdata/vhs/reference/loading-mv.png (`Loading 6 — Combined (thick
// bar)`). Top-to-bottom: the locked 5-row solid-block PORTAL wordmark + a flush
// 5-row violet caret bar, a thick violet progress bar on the bg.track track, and a
// real 5-row tick-list.
//
// Every glyph carries the owned canvas background (leaf .Background(canvas), §1)
// so the centred block has no terminal-bg islands behind it; the outer fill in
// View() (model.fillCanvas) owns the surrounding canvas. Under the NO_COLOR
// carve-out (§2.5) every hue and the canvas background drop, leaving the
// structure (block banner / bar / glyph+label list) intact on the terminal's
// native fg/bg — state stays distinguishable by glyph (✓/◐/·) + bold/dim (§2.2).

// loadingWordmark is THE LOCKED hero wordmark (user-approved): a bold 5-row
// SOLID-block banner spelling PORTAL. A terminal has no font size, so "large
// PORTAL" becomes a multi-row block banner; the earlier compact 3-row half-block
// form rendered ambiguously in the real terminal font ("A" read as "U"), so the
// art is now a bold 5-row solid `█` letterform that reads unambiguously. These
// five rows are the SINGLE SOURCE OF TRUTH for the banner — do not re-derive or
// substitute. Each letter is a 5-cell solid-block glyph with single-space gaps;
// the trailing `L` is `█` on rows 1–4 and `█████` on row 5, so the rows are
// NATURALLY RAGGED (rows 0–3 are 31 cells, row 4 is 35). renderBlockWordmark
// right-pads every row to the common max width at render time — this both renders
// the L's flush-left stem correctly AND keeps the caret a flush vertical bar
// (NOTE: do NOT add trailing spaces to these constant rows — gofmt/editors strip
// them, so the padding is applied at runtime).
var loadingWordmark = [5]string{
	"█████ █████ █████ █████ █████ █",
	"█   █ █   █ █   █   █   █   █ █",
	"█████ █   █ █████   █   █████ █",
	"█     █   █ █  █    █   █   █ █",
	"█     █████ █   █   █   █   █ █████",
}

const (
	// loadingCaretGlyph is the violet caret block. renderBlockWordmark builds a
	// 5-row caret column from it and joins it ONCE (one space gap) against the
	// padded wordmark stack, so the caret is a single flush 5-row-tall accent.violet
	// bar beside the wordmark — not appended per-row (which jogged on the ragged
	// bottom row, breaking the caret into a detached comma).
	loadingCaretGlyph = "█"

	// loadingGlyphDone / loadingGlyphActive / loadingGlyphPending are the §10.3
	// tick glyphs. Each occupies a fixed-width slot so the labels align across
	// rows regardless of glyph cell width.
	loadingGlyphDone    = "✓"
	loadingGlyphActive  = "◐"
	loadingGlyphPending = "·"

	// loadingBarFilledGlyph / loadingBarTrackGlyph are the thick bar's cells — one
	// row of solid blocks. The filled run carries the accent.violet background, the
	// track the bg.track background, so the bar reads as one clean thick solid bar
	// (a full-block glyph over the matching background renders a flush solid cell).
	loadingBarFilledGlyph = "█"
	loadingBarTrackGlyph  = "█"
)

const (
	// loadingBlockWordmarkWidth is the rendered width of the widest banner row plus
	// the caret (one space + the caret glyph). Below this (with margins) the screen
	// degrades to the single-row letter-spaced wordmark, then the compact form
	// (§2.7). Computed once in init from loadingWordmark so it never drifts.
	loadingTickGlyphSlot = 2 // fixed-width glyph column so labels align
	loadingTickGap       = "  "

	// loadingBarWidth is the target thick-bar width in cells (clamped to the
	// content width on a narrow terminal). It matches the reference proportions —
	// roughly the width of the block wordmark.
	loadingBarWidth = 30
)

// loadingBlockBannerWidth is the rendered cell width of the 5-row block banner
// INCLUDING the violet caret (the common max row width + one space gap + the caret
// glyph). Derived from loadingWordmark so the degrade threshold tracks the banner,
// never a magic number (≈37 cells for the current art). Computed once at package
// init.
var loadingBlockBannerWidth = computeBlockBannerWidth()

// blockBannerMaxRowWidth is the widest of the (naturally ragged) banner rows — the
// common width every row is right-padded to so the rows are uniform and the caret
// stays flush. Single source for both the render padding and the degrade-threshold
// width.
func blockBannerMaxRowWidth() int {
	widest := 0
	for _, row := range loadingWordmark {
		if w := lipgloss.Width(row); w > widest {
			widest = w
		}
	}
	return widest
}

func computeBlockBannerWidth() int {
	// padded rows (max width) + one space gap + the single-cell caret glyph.
	return blockBannerMaxRowWidth() + 1 + lipgloss.Width(loadingCaretGlyph)
}

// loadingFallbackWidth/Height mirror the header/viewLoading zero-size fallback so
// the screen still composes before the first WindowSizeMsg.
const (
	loadingFallbackWidth  = 80
	loadingFallbackHeight = 24
)

// singleRowWordmarkHeight is the row count of the degraded single-row wordmark —
// always one row. Named so the height-degrade arithmetic reads intent.
const singleRowWordmarkHeight = 1

// renderLoadingScreen composes the §10.3 honest loading screen centred in the
// inset content region (w × h), so View()→fillCanvas paints the owned canvas
// around it. The composition is top-to-bottom: the hero wordmark, a 1-row gap,
// the thick bar, a 1-row gap, the 5-row tick-list. The whole block is centred via
// lipgloss.Place.
//
// Degrade (§2.7), never overflow:
//   - Width: the wordmark steps block → single-row letter-spaced → compact; the
//     bar clamps to w; tick rows truncate with `…` so no row ever exceeds w.
//   - Height: when the full block does not fit in h, the inter-section blank gaps
//     are dropped first (saves 2 rows), then the banner collapses to a single row
//     (the 5-row list + 1 bar is the irreducible floor), so the step-list never
//     overflows.
func renderLoadingScreen(view LoadingProgressView, w, h int, mode theme.Mode, colourless bool) string {
	if w <= 0 {
		w = loadingFallbackWidth
	}
	if h <= 0 {
		h = loadingFallbackHeight
	}

	bar := renderLoadingBar(view.BarFraction, w, mode, colourless)
	list := renderTickList(view.Labels, w, mode, colourless)
	listHeight := lipgloss.Height(list)

	// Height-driven layout. The bar (1) + list is the irreducible floor; spend the
	// remaining rows first on the wordmark (the taller block banner vs the
	// single-row form), then on the one-row gaps above and below the bar. On a
	// terminal too short for even the single-row wordmark + bar + list, the
	// wordmark is dropped entirely so the step-list never overflows.
	parts := []string{bar, list}
	if singleRowWordmarkHeight+1+listHeight <= h {
		wordmark := renderLoadingWordmark(w, h, listHeight, mode, colourless)
		wordmarkHeight := lipgloss.Height(wordmark)
		// gaps fit only when there is height to spare beyond wordmark + bar + list.
		if wordmarkHeight+1+1+1+1+listHeight <= h {
			gap := loadingStyle(mode, colourless).Render("")
			parts = []string{wordmark, gap, bar, gap, list}
		} else {
			parts = []string{wordmark, bar, list}
		}
	}

	block := lipgloss.JoinVertical(lipgloss.Left, parts...)

	// Centre the block in the inset content region. The whitespace lipgloss.Place
	// emits is canvas-backfilled by the outer View()→fillCanvas wrap (the single
	// owned-canvas fill, §1), so no per-cell whitespace background is set here —
	// matching every other page composer. Place never truncates, so a block taller
	// than h would overflow — the degrade above keeps it within h.
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// loadingStyle is the leaf style for the loading screen: a role-token FOREGROUND
// over the owned canvas Background for the mode. Under NO_COLOR it returns a bare
// style (no hue, no canvas) so the run renders on the terminal's native fg/bg.
func loadingStyle(mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Background(theme.MV.Canvas.ColorFor(mode))
}

// loadingFg returns the leaf style carrying the token foreground over the canvas
// (bare under NO_COLOR).
func loadingFg(fg theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	if colourless {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().
		Foreground(fg.ColorFor(mode)).
		Background(theme.MV.Canvas.ColorFor(mode))
}

// renderLoadingWordmark renders the hero wordmark for the laid-out width and the
// remaining height budget, degrading (§2.7) so it never overflows in either
// dimension: the 5-row block banner + flush violet caret bar when it fits BOTH the
// block width (≈37 cells) and the height budget (the bar + list still fit beneath
// it), else the single-row letter-spaced wordmark + caret, else the compact
// wordmark + caret. listHeight is the rendered height of the tick-list, used so
// the banner only claims its 5 rows when h leaves room for the bar (1) + the list.
func renderLoadingWordmark(w, h, listHeight int, mode theme.Mode, colourless bool) string {
	blockFitsHeight := len(loadingWordmark)+1+listHeight <= h
	if w >= loadingBlockBannerWidth && blockFitsHeight {
		return renderBlockWordmark(mode, colourless)
	}
	// Single-row letter-spaced wordmark + caret (header.go's fullWordmark), if it
	// fits with the caret (one space + one cell).
	full := lipgloss.Width(fullWordmark) + 1 + lipgloss.Width(headerCaret)
	if w >= full {
		return renderSingleRowWordmark(fullWordmark, mode, colourless)
	}
	return renderSingleRowWordmark(headerCompactWordmark, mode, colourless)
}

// renderBlockWordmark renders the locked 5-row solid-block banner so the caret is
// a single FLUSH 5-row violet bar regardless of the art's ragged row widths:
//
//  1. Compute the common max row width across the five banner rows.
//  2. RIGHT-PAD every row to that width (so all rows are uniform — this fixes the
//     ragged-row caret jog AND renders the trailing `L` correctly, since padding
//     rows 0–3 leaves the L's stem flush-left with trailing space, matching row
//     4's `█████` foot).
//  3. Build the caret as its OWN 5-row block (one caret glyph per row) and
//     JoinHorizontal it ONCE — separated by a one-space gap column — against the
//     padded, JoinVertical'd wordmark stack. Appending the caret per-row (the old
//     path) jogged it on the wider bottom row, breaking it into a detached comma.
//
// The letters carry text.primary (bold); the caret carries accent.violet.
func renderBlockWordmark(mode theme.Mode, colourless bool) string {
	lettersStyle := loadingFg(theme.MV.TextPrimary, mode, colourless).Bold(true)
	caretStyle := loadingFg(theme.MV.AccentViolet, mode, colourless)
	pad := loadingStyle(mode, colourless)

	maxWidth := blockBannerMaxRowWidth()

	letterRows := make([]string, 0, len(loadingWordmark))
	caretRows := make([]string, 0, len(loadingWordmark))
	gapRows := make([]string, 0, len(loadingWordmark))
	for _, seg := range loadingWordmark {
		padded := seg
		if missing := maxWidth - lipgloss.Width(seg); missing > 0 {
			padded += strings.Repeat(" ", missing)
		}
		letterRows = append(letterRows, lettersStyle.Render(padded))
		caretRows = append(caretRows, caretStyle.Render(loadingCaretGlyph))
		gapRows = append(gapRows, pad.Render(" "))
	}

	wordmark := lipgloss.JoinVertical(lipgloss.Left, letterRows...)
	gapColumn := lipgloss.JoinVertical(lipgloss.Left, gapRows...)
	caretBar := lipgloss.JoinVertical(lipgloss.Left, caretRows...)
	return lipgloss.JoinHorizontal(lipgloss.Top, wordmark, gapColumn, caretBar)
}

// renderSingleRowWordmark renders the narrow-degrade single-row wordmark: the
// text.primary (bold) wordmark + the violet header caret (§3.1 form), reusing
// header.go's glyphs so the degrade matches the shared chrome.
func renderSingleRowWordmark(wordmark string, mode theme.Mode, colourless bool) string {
	letters := loadingFg(theme.MV.TextPrimary, mode, colourless).Bold(true).Render(wordmark)
	gap := loadingStyle(mode, colourless).Render(" ")
	caret := loadingFg(theme.MV.AccentViolet, mode, colourless).Render(headerCaret)
	return lipgloss.JoinHorizontal(lipgloss.Top, letters, gap, caret)
}

// renderLoadingBar renders the thick block progress bar: one row of solid cells,
// the filled prefix (driven by fraction) carrying the accent.violet background
// and the track the bg.track background, so it reads as one clean thick solid
// bar. The bar width is loadingBarWidth, clamped to the content width on a narrow
// terminal so it never overflows. Under NO_COLOR the bar drops both backgrounds
// and renders the filled run as solid glyphs over the track glyphs on the native
// bg (still a visible determinate bar via the block run, no colour required).
func renderLoadingBar(fraction float64, w int, mode theme.Mode, colourless bool) string {
	barW := loadingBarWidth
	if barW > w {
		barW = w
	}
	if barW <= 0 {
		return loadingStyle(mode, colourless).Render("")
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := int(float64(barW)*fraction + 0.5)
	if filled > barW {
		filled = barW
	}

	if colourless {
		// No hue/canvas: a solid run for the filled prefix, the track glyph for the
		// remainder, on the terminal's native bg. Still determinate.
		return strings.Repeat(loadingBarFilledGlyph, filled) +
			strings.Repeat(loadingBarTrackGlyph, barW-filled)
	}

	filledStyle := lipgloss.NewStyle().
		Foreground(theme.MV.AccentViolet.ColorFor(mode)).
		Background(theme.MV.AccentViolet.ColorFor(mode))
	trackStyle := lipgloss.NewStyle().
		Foreground(theme.MV.BgTrack.ColorFor(mode)).
		Background(theme.MV.BgTrack.ColorFor(mode))

	filledRun := filledStyle.Render(strings.Repeat(loadingBarFilledGlyph, filled))
	trackRun := trackStyle.Render(strings.Repeat(loadingBarTrackGlyph, barW-filled))
	return lipgloss.JoinHorizontal(lipgloss.Top, filledRun, trackRun)
}

// renderTickList renders the §10.3 5-row tick-list — a real list, one row per
// friendly label — clamped to w so no row overflows (§2.7).
func renderTickList(labels []LoadingLabel, w int, mode theme.Mode, colourless bool) string {
	rows := make([]string, 0, len(labels))
	for _, l := range labels {
		rows = append(rows, clampRow(renderTickRow(l, mode, colourless), w))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// clampRow truncates a rendered row to w cells with an ellipsis (§2.7 — names /
// rows truncate with `…` rather than overflow), preserving the row's SGR runs. A
// row already within w is returned unchanged.
func clampRow(row string, w int) string {
	if w <= 0 || lipgloss.Width(row) <= w {
		return row
	}
	return ansi.Truncate(row, w, "…")
}

// renderTickRow renders one tick-list row: the state glyph in a fixed-width slot,
// a two-space gap, the friendly label, and (active "Restoring sessions" only) the
// spaced `N / M` counter. Token + weight per the §10.3 mapping:
//
//	✓ done    → glyph state.green,  label text.muted-bright (regular)
//	◐ active  → glyph accent.cyan,  label text.primary (BOLD)
//	· pending → glyph text.faint,   label text.dim (regular)
//
// The active row's counter is the task-5-4 "N/M" reformatted to the frame's
// spaced "N / M" and painted text.detail.
func renderTickRow(l LoadingLabel, mode theme.Mode, colourless bool) string {
	glyph, glyphTok, labelTok, bold := tickRowTokens(l.State)

	glyphCell := padGlyphSlot(glyph)
	glyphRun := loadingFg(glyphTok, mode, colourless).Render(glyphCell)
	gap := loadingStyle(mode, colourless).Render(loadingTickGap)
	labelRun := loadingFg(labelTok, mode, colourless).Bold(bold).Render(l.Text)

	row := lipgloss.JoinHorizontal(lipgloss.Top, glyphRun, gap, labelRun)

	if counter := spacedCounter(l); counter != "" {
		counterGap := loadingStyle(mode, colourless).Render("  ")
		counterRun := loadingFg(theme.MV.TextDetail, mode, colourless).Render(counter)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, counterGap, counterRun)
	}
	return row
}

// tickRowTokens maps a label state to its (glyph, glyph token, label token, bold)
// per the §10.3 table — the single mapping site so no row drifts.
func tickRowTokens(state LabelState) (glyph string, glyphTok, labelTok theme.Token, bold bool) {
	switch state {
	case LabelDone:
		return loadingGlyphDone, theme.MV.StateGreen, theme.MV.TextMutedBright, false
	case LabelActive:
		return loadingGlyphActive, theme.MV.AccentCyan, theme.MV.TextPrimary, true
	default:
		return loadingGlyphPending, theme.MV.TextFaint, theme.MV.TextDim, false
	}
}

// padGlyphSlot right-pads a tick glyph to the fixed-width slot so labels align
// across rows regardless of the glyph's cell width.
func padGlyphSlot(glyph string) string {
	w := lipgloss.Width(glyph)
	if w >= loadingTickGlyphSlot {
		return glyph
	}
	return glyph + strings.Repeat(" ", loadingTickGlyphSlot-w)
}

// spacedCounter reformats the task-5-4 "N/M" counter to the frame's spaced
// "N / M" form for display (§10.3 — the frame shows `8 / 12`). It returns "" for
// every non-counter label and the M=0 suppressed case (task 5-4 already emits ""
// there), so the counter shows ONLY on the active "Restoring sessions" row.
func spacedCounter(l LoadingLabel) string {
	if l.Counter == "" {
		return ""
	}
	n, m, ok := strings.Cut(l.Counter, "/")
	if !ok {
		return l.Counter
	}
	return n + " / " + m
}
