package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// Rows are naturally ragged; renderBlockWordmark right-pads at render time. Do
// not pad here — gofmt and editors strip trailing spaces.
var loadingWordmark = [5]string{
	"█████ █████ █████ █████ █████ █",
	"█   █ █   █ █   █   █   █   █ █",
	"█████ █   █ █████   █   █████ █",
	"█     █   █ █  █    █   █   █ █",
	"█     █████ █   █   █   █   █ █████",
}

const (
	loadingCaretGlyph = "█"

	// Deliberately wider than the 1-cell inter-letter gap: at block weight a
	// 1-cell gap read the caret as a seventh letter.
	loadingCaretGap = 3

	loadingGlyphDone    = "✓"
	loadingGlyphActive  = "◐"
	loadingGlyphPending = "·"
	// Glyph-distinct from ✓/◐/· so the failure reads under NO_COLOR.
	loadingGlyphFailed = "✗"

	// Full blocks over matching backgrounds render as flush solid cells.
	loadingBarFilledGlyph = "█"
	loadingBarTrackGlyph  = "█"
	// Under NO_COLOR no background tells the track from the fill, so the
	// track takes a glyph of its own.
	loadingBarColourlessTrackGlyph = "░"
)

const (
	loadingTickGlyphSlot = 2
	loadingTickGap       = "  "

	loadingQuitHint = "q quit · esc quit"
)

var loadingBlockBannerWidth = computeBlockBannerWidth()

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
	return blockBannerMaxRowWidth() + loadingCaretGap + lipgloss.Width(loadingCaretGlyph)
}

// Mirrors the header's zero-size fallback so the screen composes before the
// first WindowSizeMsg.
const (
	loadingFallbackWidth  = 80
	loadingFallbackHeight = 24
)

const singleRowWordmarkHeight = 1

const loadingSectionGap = 2

func renderLoadingScreen(view LoadingProgressView, w, h int, th theme.Theme, colourless bool) string {
	if w <= 0 {
		w = loadingFallbackWidth
	}
	if h <= 0 {
		h = loadingFallbackHeight
	}

	block := composeLoadingBlock(view, w, h, th, colourless)

	// Place never truncates, so composeLoadingBlock's degrade is what keeps the
	// block within h.
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, block)
}

// Degrade, never overflow: bar + list (+ error footer) is the irreducible floor.
// The bar width derives from the rendered wordmark, so it spans the logo in
// every degraded form.
func composeLoadingBlock(view LoadingProgressView, w, h int, th theme.Theme, colourless bool) string {
	if len(view.Labels) == 0 {
		return composeFatalAlone(view.Message, w, h, th, colourless)
	}
	list := renderTickList(view.Labels, w, th, colourless)
	listHeight := lipgloss.Height(list)

	// The error footer rows stay separate centred elements rather than folding
	// into the left-joined list, so each centres independently.
	var footerParts []string
	footerHeight := 0
	if view.Message != "" {
		footerBudget := h - 1 - listHeight
		footerParts = renderErrorFooter(view.Message, w, footerBudget, th, colourless)
		for _, p := range footerParts {
			footerHeight += lipgloss.Height(p)
		}
	}

	wordmark := renderLoadingWordmark(w, h, listHeight+footerHeight, th, colourless)
	wordmarkWidth := lipgloss.Width(wordmark)
	bar := renderLoadingBar(view.BarFraction, w, wordmarkWidth, th, colourless)

	floor := 1 + listHeight + footerHeight

	parts := []string{bar, list}
	if singleRowWordmarkHeight+floor <= h {
		wordmarkHeight := lipgloss.Height(wordmark)
		if wordmarkHeight+2*loadingSectionGap+floor <= h {
			gap := renderSectionGap(th, colourless)
			parts = []string{wordmark, gap, bar, gap, list}
		} else {
			parts = []string{wordmark, bar, list}
		}
	}
	parts = append(parts, footerParts...)

	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

// composeFatalAlone is a fatal no loading step came before, as a warm
// picker's mint meets: the wordmark, room allowing, over the error footer,
// with no bar and no step list.
func composeFatalAlone(message string, w, h int, th theme.Theme, colourless bool) string {
	parts := renderErrorFooter(message, w, h, th, colourless)
	footerHeight := 0
	for _, p := range parts {
		footerHeight += lipgloss.Height(p)
	}
	if singleRowWordmarkHeight+footerHeight <= h {
		parts = append([]string{renderLoadingWordmark(w, h, footerHeight, th, colourless)}, parts...)
	}
	return lipgloss.JoinVertical(lipgloss.Center, parts...)
}

func renderSectionGap(th theme.Theme, colourless bool) string {
	row := loadingStyle(th, colourless).Render("")
	rows := make([]string, loadingSectionGap)
	for i := range rows {
		rows[i] = row
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

func loadingStyle(th theme.Theme, colourless bool) lipgloss.Style {
	return headerCanvasBg(th, colourless)
}

func loadingFg(fg theme.Token, th theme.Theme, colourless bool) lipgloss.Style {
	return headerStyle(fg, th, colourless)
}

// belowHeight is the rendered height of everything below the bar, so the block
// banner claims its rows only when the rest still fits.
func renderLoadingWordmark(w, h, belowHeight int, th theme.Theme, colourless bool) string {
	blockFitsHeight := len(loadingWordmark)+1+belowHeight <= h
	if w >= loadingBlockBannerWidth && blockFitsHeight {
		return renderBlockWordmark(th, colourless)
	}
	full := lipgloss.Width(fullWordmark) + 1 + lipgloss.Width(headerCaret)
	if w >= full {
		return renderSingleRowWordmark(fullWordmark, th, colourless)
	}
	return renderSingleRowWordmark(headerCompactWordmark, th, colourless)
}

// The caret is its own column joined once against the padded stack: appending it
// per-row jogs on the wider bottom row and breaks into a detached comma.
func renderBlockWordmark(th theme.Theme, colourless bool) string {
	lettersStyle := loadingFg(th.TextPrimary, th, colourless).Bold(true)
	caretStyle := loadingFg(th.AccentPrimary, th, colourless)
	pad := loadingStyle(th, colourless)

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

func renderSingleRowWordmark(wordmark string, th theme.Theme, colourless bool) string {
	letters := loadingFg(th.TextPrimary, th, colourless).Bold(true).Render(wordmark)
	gap := loadingStyle(th, colourless).Render(" ")
	caret := loadingFg(th.AccentPrimary, th, colourless).Render(headerCaret)
	return lipgloss.JoinHorizontal(lipgloss.Top, letters, gap, caret)
}

func renderLoadingBar(fraction float64, w, barWidth int, th theme.Theme, colourless bool) string {
	barW := min(barWidth, w)
	if barW <= 0 {
		return loadingStyle(th, colourless).Render("")
	}
	if fraction < 0 {
		fraction = 0
	}
	if fraction > 1 {
		fraction = 1
	}
	filled := min(int(float64(barW)*fraction+0.5), barW)

	if colourless {
		return strings.Repeat(loadingBarFilledGlyph, filled) +
			strings.Repeat(loadingBarColourlessTrackGlyph, barW-filled)
	}

	filledStyle := lipgloss.NewStyle().
		Foreground(th.AccentPrimary.Color()).
		Background(th.AccentPrimary.Color())
	trackStyle := lipgloss.NewStyle().
		Foreground(th.BgSubtle.Color()).
		Background(th.BgSubtle.Color())

	filledRun := filledStyle.Render(strings.Repeat(loadingBarFilledGlyph, filled))
	trackRun := trackStyle.Render(strings.Repeat(loadingBarTrackGlyph, barW-filled))
	return lipgloss.JoinHorizontal(lipgloss.Top, filledRun, trackRun)
}

func renderTickList(labels []LoadingLabel, w int, th theme.Theme, colourless bool) string {
	rows := make([]string, 0, len(labels))
	for _, l := range labels {
		rows = append(rows, clampRow(renderTickRow(l, th, colourless), w))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...)
}

// Below one row the footer vanishes and the failed step's ✗ carries it alone.
func renderErrorFooter(message string, w, budget int, th theme.Theme, colourless bool) []string {
	if budget < 1 {
		return nil
	}
	messageRow := clampRow(loadingFg(th.StateDestructive, th, colourless).Render(message), w)
	if budget == 1 {
		return []string{messageRow}
	}
	hintRow := clampRow(loadingFg(th.TextFaint, th, colourless).Render(loadingQuitHint), w)
	if budget == 2 {
		return []string{messageRow, hintRow}
	}
	spacer := loadingStyle(th, colourless).Render("")
	return []string{spacer, messageRow, hintRow}
}

// Truncates to w cells preserving the row's SGR runs.
func clampRow(row string, w int) string {
	if w <= 0 || lipgloss.Width(row) <= w {
		return row
	}
	return ansi.Truncate(row, w, "…")
}

func renderTickRow(l LoadingLabel, th theme.Theme, colourless bool) string {
	glyph, glyphTok, labelTok, bold := tickRowTokens(l.State, th)

	glyphCell := padGlyphSlot(glyph)
	glyphRun := loadingFg(glyphTok, th, colourless).Render(glyphCell)
	gap := loadingStyle(th, colourless).Render(loadingTickGap)
	labelRun := loadingFg(labelTok, th, colourless).Bold(bold).Render(l.Text)

	row := lipgloss.JoinHorizontal(lipgloss.Top, glyphRun, gap, labelRun)

	if counter := spacedCounter(l); counter != "" {
		counterGap := loadingStyle(th, colourless).Render("  ")
		counterRun := loadingFg(th.TextMuted, th, colourless).Render(counter)
		row = lipgloss.JoinHorizontal(lipgloss.Top, row, counterGap, counterRun)
	}
	return row
}

func tickRowTokens(state LabelState, th theme.Theme) (glyph string, glyphTok, labelTok theme.Token, bold bool) {
	switch state {
	case LabelDone:
		return loadingGlyphDone, th.StatePositive, th.TextTertiary, false
	case LabelActive:
		return loadingGlyphActive, th.AccentMode, th.TextPrimary, true
	case LabelFailed:
		return loadingGlyphFailed, th.StateDestructive, th.StateDestructive, true
	default:
		return loadingGlyphPending, th.TextFaint, th.TextSubtle, false
	}
}

// Fixed-width slot so labels align across rows regardless of glyph cell width.
func padGlyphSlot(glyph string) string {
	w := lipgloss.Width(glyph)
	if w >= loadingTickGlyphSlot {
		return glyph
	}
	return glyph + strings.Repeat(" ", loadingTickGlyphSlot-w)
}

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
