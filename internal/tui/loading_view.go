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
// bar)`). Top-to-bottom, all CENTRED as a column relative to one another
// (lipgloss.JoinVertical(Center) — never left-flushed to the widest element): the
// locked 5-row solid-block PORTAL wordmark + a flush 5-row violet caret bar, a
// 2-row gap, a thick violet progress bar on the bg.track track spanning the FULL
// rendered wordmark width, a 2-row gap, and a real 5-row tick-list whose rows stay
// left-aligned within the centred list block. On the §10.5 error frame the message
// and quit hint are appended as further centred elements.
//
// Every glyph carries the owned canvas background (leaf .Background(canvas), §1);
// the JoinVertical(Center) padding cells beside narrower elements are painted by
// the outer View()→fillCanvas backfill, so the centred block has no terminal-bg
// islands behind it; the outer fill in View() (model.fillCanvas) owns the
// surrounding canvas. Under the NO_COLOR
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
	// 5-row caret column from it and joins it ONCE (a loadingCaretGap-cell gap)
	// against the padded wordmark stack, so the caret is a single flush 5-row-tall
	// accent.violet bar beside the wordmark — not appended per-row (which jogged on
	// the ragged bottom row, breaking the caret into a detached comma).
	loadingCaretGlyph = "█"

	// loadingCaretGap is the blank-cell gap between the block wordmark's trailing L
	// and the violet caret bar. Deliberately wider than the 1-cell inter-letter gap
	// so the caret reads as a separate cursor, not a 7th letter — at block weight a
	// 1-cell gap rendered "PORTAL" + caret as a single word "PORTALI".
	loadingCaretGap = 3

	// loadingGlyphDone / loadingGlyphActive / loadingGlyphPending are the §10.3
	// tick glyphs. Each occupies a fixed-width slot so the labels align across
	// rows regardless of glyph cell width.
	loadingGlyphDone    = "✓"
	loadingGlyphActive  = "◐"
	loadingGlyphPending = "·"
	// loadingGlyphFailed is the §10.5 fatal-step marker — a state.red ✗ on the
	// step that aborted the boot. It is glyph-distinct from ✓/◐/· so the failure
	// reads under NO_COLOR without relying on the red hue (§2.2).
	loadingGlyphFailed = "✗"

	// loadingBarFilledGlyph / loadingBarTrackGlyph are the thick bar's cells — one
	// row of solid blocks. The filled run carries the accent.violet background, the
	// track the bg.track background, so the bar reads as one clean thick solid bar
	// (a full-block glyph over the matching background renders a flush solid cell).
	loadingBarFilledGlyph = "█"
	loadingBarTrackGlyph  = "█"
)

const (
	loadingTickGlyphSlot = 2 // fixed-width glyph column so labels align
	loadingTickGap       = "  "

	// loadingQuitHint is the §10.5 error-frame quit hint shown beneath the fatal
	// message. The spec mandates q/Esc quits; this tells the user how to exit.
	loadingQuitHint = "q quit · esc quit"
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
	// padded rows (max width) + the caret gap + the single-cell caret glyph.
	return blockBannerMaxRowWidth() + loadingCaretGap + lipgloss.Width(loadingCaretGlyph)
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

// loadingSectionGap is the blank-row count between the wordmark→bar and bar→list
// sections of the centred block (§10.3 — the design's ~34px gap reads as two
// terminal rows, vs the former single row). Dropped first under the height
// degrade so the bar(1) + list floor is never overflowed.
const loadingSectionGap = 2

// renderLoadingScreen composes the §10.3 honest loading screen centred in the
// inset content region (w × h), so View()→fillCanvas paints the owned canvas
// around it. It builds the centred column via composeLoadingBlock and places it
// dead-centre via lipgloss.Place.
func renderLoadingScreen(view LoadingProgressView, w, h int, mode theme.Mode, colourless bool) string {
	if w <= 0 {
		w = loadingFallbackWidth
	}
	if h <= 0 {
		h = loadingFallbackHeight
	}

	block := composeLoadingBlock(view, w, h, mode, colourless)

	// Centre the block in the inset content region. The whitespace lipgloss.Place
	// emits is canvas-backfilled by the outer View()→fillCanvas wrap (the single
	// owned-canvas fill, §1), so no per-cell whitespace background is set here —
	// matching every other page composer. Place never truncates, so a block taller
	// than h would overflow — the degrade in composeLoadingBlock keeps it within h.
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// composeLoadingBlock builds the §10.3 centred column: the hero wordmark, a
// 2-row gap, the thick bar (spanning the full wordmark width), a 2-row gap, the
// 5-row tick-list, and — on the §10.5 fatal frame — a 1-row spacer + the centred
// message line + the centred quit hint. Every element is joined via
// JoinVertical(lipgloss.Center) so the column centres relative to its WIDEST
// element regardless of which it is (the wide wordmark on the normal frame, the
// long message on the error frame) — the two frames then stop diverging.
//
// The bar width is DERIVED from the rendered wordmark width (not a fixed 30), so
// the bar always spans the full length of the logo — including on the
// width-degraded single-row / compact wordmark, where it matches that narrower
// form. The wordmark is therefore computed FIRST and measured.
//
// Degrade (§2.7), never overflow:
//   - Width: the wordmark steps block → single-row letter-spaced → compact; the
//     bar clamps to the content width w; tick rows truncate with `…`.
//   - Height: when the full block does not fit in h, the inter-section gaps are
//     dropped first (saves 2×loadingSectionGap rows), then the banner collapses
//     to a single row, then is dropped entirely — the bar(1) + list (+ budgeted
//     footer on the error frame) is the irreducible floor, so the step-list never
//     overflows.
func composeLoadingBlock(view LoadingProgressView, w, h int, mode theme.Mode, colourless bool) string {
	list := renderTickList(view.Labels, w, mode, colourless)
	listHeight := lipgloss.Height(list)

	// §10.5 error footer rows (message + hint, + a spacer) are SEPARATE centred
	// elements appended to the column — NOT folded into the left-joined list — so
	// each centres independently (the wide message becomes a centred caption while
	// the compact steps-block stays centred and no element sticks out
	// asymmetrically). The footer is height-BUDGETED to the rows beyond
	// bar(1)+list so it never pushes the floor past h; on a too-short terminal it
	// sheds the spacer, then the hint, then the message (the red ✗ on the failed
	// row still conveys the failure — §2.7 degrade-never-break).
	var footerParts []string
	footerHeight := 0
	if view.Message != "" {
		footerBudget := h - 1 - listHeight // 1 = the bar's single row
		footerParts = renderErrorFooter(view.Message, w, footerBudget, mode, colourless)
		for _, p := range footerParts {
			footerHeight += lipgloss.Height(p)
		}
	}

	// Compute the wordmark FIRST and measure it so the bar spans its full width.
	wordmark := renderLoadingWordmark(w, h, listHeight+footerHeight, mode, colourless)
	wordmarkWidth := lipgloss.Width(wordmark)
	bar := renderLoadingBar(view.BarFraction, w, wordmarkWidth, mode, colourless)

	floor := 1 + listHeight + footerHeight // bar(1) + list + footer

	// Height-driven layout. The floor is irreducible; spend the remaining rows
	// first on the wordmark (the taller block banner vs the single-row form), then
	// on the 2-row gaps above and below the bar. On a terminal too short for even
	// the single-row wordmark + floor, the wordmark is dropped entirely so the
	// step-list never overflows.
	parts := []string{bar, list}
	if singleRowWordmarkHeight+floor <= h {
		wordmarkHeight := lipgloss.Height(wordmark)
		// gaps fit only when there is height to spare beyond wordmark + gaps + floor.
		if wordmarkHeight+2*loadingSectionGap+floor <= h {
			gap := renderSectionGap(mode, colourless)
			parts = []string{wordmark, gap, bar, gap, list}
		} else {
			parts = []string{wordmark, bar, list}
		}
	}
	parts = append(parts, footerParts...)

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// renderSectionGap renders the 2-row inter-section blank gap, each row carrying
// the canvas background so the centred-padding cells beside narrower elements
// never leave a terminal-bg island (the canvas-backfill in fillCanvas paints the
// JoinVertical(Center) leading pad, and these styled blank rows keep the gap rows
// themselves on canvas). Under NO_COLOR the rows are bare.
func renderSectionGap(mode theme.Mode, colourless bool) string {
	row := loadingStyle(mode, colourless).Render("")
	rows := make([]string, loadingSectionGap)
	for i := range rows {
		rows[i] = row
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// loadingStyle is the leaf canvas-paint style for the loading screen — a
// Background(canvas) style for the mode, bare under NO_COLOR. It delegates to the
// shared header.go source (headerCanvasBg) rather than re-implementing the rule,
// so the leaf canvas-paint carve-out lives in exactly one place (mirroring how
// SessionDelegate.rowBg delegates to the shared rowBgStyle free function).
func loadingStyle(mode theme.Mode, colourless bool) lipgloss.Style {
	return headerCanvasBg(mode, colourless)
}

// loadingFg is the leaf token-foreground-over-canvas style for the loading screen
// (bare under NO_COLOR). It delegates to the shared header.go source (headerStyle)
// rather than re-implementing the rule (mirroring SessionDelegate.rowToken's
// delegation to the shared rowTokenStyle free function).
func loadingFg(fg theme.Token, mode theme.Mode, colourless bool) lipgloss.Style {
	return headerStyle(fg, mode, colourless)
}

// renderLoadingWordmark renders the hero wordmark for the laid-out width and the
// remaining height budget, degrading (§2.7) so it never overflows in either
// dimension: the 5-row block banner + flush violet caret bar when it fits BOTH the
// block width (≈37 cells) and the height budget (the bar + everything beneath it
// still fit), else the single-row letter-spaced wordmark + caret, else the compact
// wordmark + caret. belowHeight is the rendered height of everything below the bar
// (the tick-list plus, on the §10.5 error frame, the footer rows), used so the
// banner only claims its 5 rows when h leaves room for the bar (1) + that block.
func renderLoadingWordmark(w, h, belowHeight int, mode theme.Mode, colourless bool) string {
	blockFitsHeight := len(loadingWordmark)+1+belowHeight <= h
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
//     JoinHorizontal it ONCE — separated by a loadingCaretGap-cell gap column —
//     against the padded, JoinVertical'd wordmark stack. Appending the caret per-row
//     (the old
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
		gapRows = append(gapRows, pad.Render(strings.Repeat(" ", loadingCaretGap)))
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
// bar. The target width is barWidth — the rendered wordmark width, so the bar
// spans the full length of the logo (§10.3) — clamped to the content width w on a
// narrow terminal so it never overflows. Under NO_COLOR the bar drops both
// backgrounds and renders the filled run as solid glyphs over the track glyphs on
// the native bg (still a visible determinate bar via the block run, no colour
// required).
func renderLoadingBar(fraction float64, w, barWidth int, mode theme.Mode, colourless bool) string {
	barW := min(barWidth, w)
	if barW <= 0 {
		return loadingStyle(mode, colourless).Render("")
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := min(int(float64(barW)*fraction+0.5), barW)

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

// renderErrorFooter renders the §10.5 fatal error-frame footer as SEPARATE
// centred elements (a slice the caller appends to the JoinVertical(Center)
// column, so each element centres independently — the wide message becomes a
// centred caption, the hint a centred line beneath it). Within a row budget:
// ideally a blank spacer row, the one-line fatal message in state.red, and a quit
// hint in text.faint (all clamped to w, §2.7). The message reads as red/error
// (paired with the failed row's red ✗ above it, so red is never the only signal —
// §2.2); the hint tells the user q/Esc quits. When budget is too small to fit all
// three, rows are SHED in increasing priority — spacer first, then the hint, then
// the message — so the footer never overflows the height budget. A budget < 1
// returns nil (the red ✗ on the failed step row carries the failure on its own).
func renderErrorFooter(message string, w, budget int, mode theme.Mode, colourless bool) []string {
	if budget < 1 {
		return nil
	}
	messageRow := clampRow(loadingFg(theme.MV.StateRed, mode, colourless).Render(message), w)
	if budget == 1 {
		return []string{messageRow} // message wins the single available row
	}
	hintRow := clampRow(loadingFg(theme.MV.TextFaint, mode, colourless).Render(loadingQuitHint), w)
	if budget == 2 {
		return []string{messageRow, hintRow}
	}
	spacer := loadingStyle(mode, colourless).Render("")
	return []string{spacer, messageRow, hintRow}
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
// per the §10.3 table — the single mapping site so no row drifts. The §10.5
// LabelFailed row is the error-frame failed step: a state.red ✗ glyph with the
// label ALSO in state.red so the failure reads as red/error (and stays
// glyph-distinct under NO_COLOR via the ✗).
func tickRowTokens(state LabelState) (glyph string, glyphTok, labelTok theme.Token, bold bool) {
	switch state {
	case LabelDone:
		return loadingGlyphDone, theme.MV.StateGreen, theme.MV.TextMutedBright, false
	case LabelActive:
		return loadingGlyphActive, theme.MV.AccentCyan, theme.MV.TextPrimary, true
	case LabelFailed:
		return loadingGlyphFailed, theme.MV.StateRed, theme.MV.StateRed, true
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
