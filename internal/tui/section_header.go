package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §3.2 / §4.2 Sessions section header: directly under the §3.1 separator
// rule, a left cluster — `Sessions` in accent.cyan, the live count in state.green
// (the SAME font size as the label, distinguished only by its dim colour, NOT a
// smaller / superscript glyph — §13.6, so it shares the baseline / cap-height),
// and the mode suffix (`— by project` / `— by tag`) plus the inside-tmux
// `(current: %s)` decoration in text.detail — with a right-aligned `/ to filter`
// hint (text.detail) on the SAME row, the gap between filled with a canvas-painted
// flex spacer to the content width.
//
// Parity is load-bearing: the suffix text and the inside-tmux decoration are
// SOURCED from sessionListTitleForMode (the single title producer) so the
// rendered strings stay byte-identical to the pre-reskin title — only the colour
// split (label vs count vs suffix) and the added right hint differ. The plain
// title field (m.sessionList.Title) is left untouched for the same reason; this
// render REPLACES the rendered title line in viewSessionList, it does not rewrite
// the title value.
//
// Every cell carries the owned canvas background (leaf .Background(canvas), §1),
// mirroring header.go, so the right-aligned spacer gap is canvas-painted with no
// terminal-bg island. Under the NO_COLOR carve-out (§2.5) every hue and the
// canvas drop; the structure (label / count / suffix / hint) renders intact on
// the terminal's native fg/bg.

const (
	// sectionLabel is the Sessions page label (accent.cyan, §3.2). It is the
	// fixed prefix of every sessionListTitleForMode output — the suffix is the
	// remainder after this prefix, so the two split cleanly with no duplicated
	// wording.
	sectionLabel = "Sessions"
	// sectionFilterHint is the persistent right-aligned hint shown on every
	// filterable session view (§3.2 / §4.2). The `s switch view` hint is the
	// footer's responsibility and is deliberately NOT duplicated here.
	sectionFilterHint = "/ to filter"
)

// renderSectionHeader renders the §3.2 / §4.2 Sessions section header for the
// active grouping mode, inside-tmux state, live session count, and resolved
// canvas mode (and the NO_COLOR carve-out). The single rendered row is always
// exactly width cells wide: the left cluster and the right hint are separated by a
// canvas-painted flex spacer. Below the width at which the left cluster + a spacer
// + the hint no longer fit, the right hint drops rather than overflow (§2.7).
func renderSectionHeader(mode prefs.SessionListMode, insideTmux bool, currentSession string, count, width int, canvasMode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	left := sectionLeftCluster(mode, insideTmux, currentSession, count, canvasMode, colourless)
	leftWidth := lipgloss.Width(left)

	hint := headerStyle(theme.MV.TextDetail, canvasMode, colourless).Render(sectionFilterHint)
	hintWidth := lipgloss.Width(hint)

	// Narrow degrade (§2.7): the left cluster already meets/exceeds the row, OR the
	// hint no longer fits beside it leaving at least one spacer cell — drop the
	// hint and pad the left cluster to width with canvas spaces.
	if leftWidth >= w || leftWidth+1+hintWidth > w {
		return headerPadRight(left, leftWidth, w, canvasMode, colourless)
	}

	spacerWidth := w - leftWidth - hintWidth
	spacer := headerCanvasBg(canvasMode, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, hint)
}

// sectionLeftCluster renders the left cluster flush at the content's left edge
// (col 0 of the inset region — the SAME column as the header.go PORTAL wordmark and
// the row cursor/selector): `Sessions` in accent.cyan, the count in state.green,
// then the mode suffix + inside-tmux decoration in text.detail. NO leading indent —
// header.go's headerBand renders the wordmark flush at col 0 with no indent, so the
// section header must too (the old col-2 indent was a legacy artifact of the
// bubbles/list TitleBar PaddingLeft=2 that this header REPLACES). The group-header
// indent (groupHeaderIndent) is a separate concern — it nests the `client ··· N`
// group rows, not this section header. The suffix is the remainder of
// sessionListTitleForMode after the fixed "Sessions" prefix, so the wording stays
// byte-parity-identical.
func sectionLeftCluster(mode prefs.SessionListMode, insideTmux bool, currentSession string, count int, canvasMode theme.Mode, colourless bool) string {
	label := headerStyle(theme.MV.AccentCyan, canvasMode, colourless).Render(sectionLabel)

	// The count sits one space right of the label, at the SAME font size (a plain
	// run — no smaller / superscript glyph), distinguished only by state.green
	// (§13.6 — shares the baseline / cap-height with the label).
	gap := headerCanvasBg(canvasMode, colourless).Render(" ")
	countRun := headerStyle(theme.MV.StateGreen, canvasMode, colourless).Render(strconv.Itoa(count))

	cluster := lipgloss.JoinHorizontal(lipgloss.Top, label, gap, countRun)

	// The mode suffix (and the inside-tmux decoration) are everything
	// sessionListTitleForMode emits after the fixed "Sessions" prefix — sourced
	// from the single title producer so the wording stays byte-identical.
	if suffix := sectionModeSuffix(mode, insideTmux, currentSession); suffix != "" {
		suffixRun := headerStyle(theme.MV.TextDetail, canvasMode, colourless).Render(suffix)
		cluster = lipgloss.JoinHorizontal(lipgloss.Top, cluster, suffixRun)
	}
	return cluster
}

// sectionModeSuffix returns the section header's suffix text — the mode suffix
// (`— by project` / `— by tag`) and the inside-tmux `(current: %s)` decoration —
// derived from sessionListTitleForMode by stripping the fixed "Sessions" prefix.
// Sourcing it from the title producer (rather than re-deriving) keeps the wording
// byte-identical to the pre-reskin title (parity). It carries a single leading
// space so it renders as ` — by tag` / ` (current: foo)` after the count.
func sectionModeSuffix(mode prefs.SessionListMode, insideTmux bool, currentSession string) string {
	title := sessionListTitleForMode(mode, insideTmux, currentSession)
	return strings.TrimPrefix(title, sectionLabel)
}
