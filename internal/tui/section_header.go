package tui

import (
	"fmt"
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
	// projectsSectionLabel is the Projects page label (state.green, §6 / §3.2).
	// The Projects header carries no mode suffix — it is the label + a text.detail
	// count + the shared `/ to filter` hint.
	projectsSectionLabel = "Projects"
	// sectionFilterHint is the persistent right-aligned hint shown on every
	// filterable session/project view (§3.2 / §4.2 / §6). The `s switch view` hint
	// is the footer's responsibility and is deliberately NOT duplicated here.
	sectionFilterHint = "/ to filter"
	// multiSelectCancelHint is the §5 multi-select banner's right-aligned hint —
	// `esc cancel` in text.detail (the SAME dim chrome token the standard
	// `/ to filter` hint uses), signalling Esc leaves the mode. It is the single
	// source of the wording.
	multiSelectCancelHint = "esc cancel"

	// unsupportedLabel is the §6.2 named-identity warning label — `unsupported
	// terminal` in accent.orange (amber) — shown when detection resolves a non-NULL
	// but undriven host terminal (e.g. com.apple.Terminal). It reads before the dim
	// identity string.
	unsupportedLabel = "unsupported terminal"
	// unsupportedNullLabel is the §6.2 NULL-identity honest label — `no host-local
	// terminal` in accent.orange — shown when detection resolves the NULL identity
	// (remote/mosh, bundleID == ""). Matches CLI task 2-7's IsNull copy branch; it
	// carries NO identity string and NO `see docs` hint.
	unsupportedNullLabel = "no host-local terminal"
	// unsupportedDocsHint is the §6.2 right-anchored blue link hint — `see docs` in
	// accent.blue — shown only on the named (non-NULL) banner. It is the single
	// source of the wording.
	unsupportedDocsHint = "see docs"
	// unsupportedIdentityDash / unsupportedIdentityMiddot are the §6.2 identity
	// separators, matching the delivered frame EXACTLY: a spaced em-dash (U+2014)
	// before the friendly name and a spaced middot (U+00B7) before the bundle id, so
	// the dim identity reads ` — Apple Terminal · com.apple.Terminal`.
	unsupportedIdentityDash   = " — "
	unsupportedIdentityMiddot = " · "
)

// renderSectionHeader renders the §3.2 / §4.2 Sessions section header for the
// active grouping mode, inside-tmux state, live session count, and resolved
// canvas mode (and the NO_COLOR carve-out). The single rendered row is always
// exactly width cells wide: the left cluster and the right hint are separated by a
// canvas-painted flex spacer. Below the width at which the left cluster + a spacer
// + the hint no longer fit, the right hint drops rather than overflow (§2.7).
func renderSectionHeader(mode prefs.SessionListMode, insideTmux bool, currentSession string, count, width int, canvasMode theme.Mode, colourless bool) string {
	left := sectionLeftCluster(mode, insideTmux, currentSession, count, canvasMode, colourless)
	return renderSectionHeaderRow(left, width, canvasMode, colourless)
}

// renderProjectsSectionHeader renders the §6 / §3.2 Projects section header: a
// left cluster — `Projects` in state.green plus the live count in text.detail (at
// the SAME cap-height as the label, distinguished only by its dim colour, NOT a
// smaller / superscript glyph — §13.6) — with a right-aligned `/ to filter` hint
// (text.detail) on the same row, the gap filled with a canvas-painted flex spacer
// to the content width. Unlike the Sessions header it carries no mode suffix.
//
// It shares the layout core (renderSectionHeaderRow) with the Sessions header so
// the flex-spacer right-alignment, the §2.7 narrow degrade, and the canvas paint
// stay identical; only the left cluster (label text + label colour + count colour)
// differs. Under the NO_COLOR carve-out (§2.5) every hue and the canvas drop; the
// structure (label / count / hint) renders intact on the terminal's native fg/bg.
func renderProjectsSectionHeader(count, width int, canvasMode theme.Mode, colourless bool) string {
	left := projectsLeftCluster(count, canvasMode, colourless)
	return renderSectionHeaderRow(left, width, canvasMode, colourless)
}

// renderMultiSelectHeader renders the §5 multi-select banner in the section-header
// row position: a left cluster — `N selected` in accent.violet — with a
// right-aligned `esc cancel` hint (text.detail) on the same row, the gap filled
// with a canvas-painted flex spacer to the content width. It REPLACES the standard
// `Sessions ··· N` section header while multi-select mode is active — a filter-line
// analogue carrying NO `▌` left-bar (it is a section-header variant, not a §11
// notice band).
//
// It routes through the SAME right-anchor core (renderRightAnchoredSectionRow) the
// standard Sessions/Projects section headers use, so its right-alignment, the
// canvas-painted flex spacer, and the §2.7 narrow degrade match those headers
// EXACTLY — only the left cluster (violet `N selected`) and the right hint (`esc
// cancel`) differ. Under the NO_COLOR carve-out (§2.5) every hue and the canvas
// drop; the `N selected` / `esc cancel` text renders intact on the terminal's
// native fg/bg. The single rendered row is exactly one line.
func renderMultiSelectHeader(count, width int, mode theme.Mode, colourless bool) string {
	left := headerStyle(theme.MV.AccentViolet, mode, colourless).Render(strconv.Itoa(count) + " selected")
	hint := headerStyle(theme.MV.TextDetail, mode, colourless).Render(multiSelectCancelHint)
	return renderRightAnchoredSectionRow(left, hint, width, mode, colourless)
}

// renderOpeningBand renders the §6.5 in-burst `Opening n/N…` pending affordance in
// the section-header row position — the HIGHEST section-header claimant (just below
// the live filter input) while an N≥2 spawn burst is in flight. It is a left cluster
// `Opening <done>/<total>…` in accent.violet (the mode accent — no new token; the
// U+2026 horizontal ellipsis signals the burst is still awaiting per-window token
// acks), composed through the SAME right-anchor core the standard section headers and
// the §5 multi-select banner use — with NO right hint, so the empty hint pads the
// whole right side with the canvas. Its right-alignment, the canvas-painted flex
// spacer, and the §2.7 narrow degrade therefore match those headers EXACTLY. NO `▌`
// left-bar (it is a section-header variant, not a §11 notice band). The single
// rendered row is exactly one line.
//
// Under the NO_COLOR carve-out (§2.5) every hue and the canvas drop; the `Opening
// n/N…` text survives on the terminal's native fg/bg.
func renderOpeningBand(done, total, width int, mode theme.Mode, colourless bool) string {
	left := headerStyle(theme.MV.AccentViolet, mode, colourless).
		Render(fmt.Sprintf("Opening %d/%d…", done, total))
	return renderRightAnchoredSectionRow(left, "", width, mode, colourless)
}

// renderUnsupportedHeader renders the §6.2 proactive unsupported/NULL terminal
// banner in the section-header row position — a filter-line analogue that REPLACES
// the standard `Sessions ··· N` section header when detection has resolved the host
// terminal to an unsupported resolution. It branches on the identity shape (NULL
// via bundleID == ""):
//
//   - NAMED (non-NULL, bundleID != ""): a left cluster of the `⚠` glyph +
//     `unsupported terminal` in accent.orange (amber — the existing warning accent,
//     no new token), then ` — <name> · <bundleID>` in text.detail (the dim identity
//     string, the copy-paste key), with a right-anchored `see docs` hint in
//     accent.blue.
//   - NULL (bundleID == "", remote/mosh): the honest `⚠ no host-local terminal`
//     line in accent.orange — NO identity, NO `see docs` hint (matching CLI task
//     2-7's IsNull copy branch). The empty hint routes through the same assembler,
//     padding the whole right side with the canvas.
//
// It routes through the SAME right-anchor core (renderRightAnchoredSectionRow) the
// standard Sessions/Projects section headers and the §5 multi-select banner use, so
// its right-alignment, the canvas-painted flex spacer, and the §2.7 narrow degrade
// match those headers EXACTLY. NO `▌` left-bar (it is a section-header variant, not
// a §11 notice band). The single rendered row is exactly one line.
//
// Under the NO_COLOR carve-out (§2.5) every hue and the canvas drop; the `⚠`, the
// label, the identity string, and `see docs` survive on the terminal's native fg/bg
// (glyph-backed, never colour-only).
func renderUnsupportedHeader(name, bundleID string, width int, mode theme.Mode, colourless bool) string {
	left := unsupportedLeftCluster(name, bundleID, mode, colourless)
	// The `see docs` hint is shown only for a named (non-NULL) identity; the NULL
	// branch carries no hint (its empty right pads to width through the assembler).
	var hint string
	if bundleID != "" {
		hint = headerStyle(theme.MV.AccentBlue, mode, colourless).Render(unsupportedDocsHint)
	}
	return renderRightAnchoredSectionRow(left, hint, width, mode, colourless)
}

// unsupportedLeftCluster renders the §6.2 banner's left cluster flush at the
// content's left edge (mirroring sectionLeftCluster — no leading indent). For a
// NULL identity (bundleID == "") it is the honest `⚠ no host-local terminal` label
// in accent.orange with no identity string. For a named identity it is the amber
// `⚠ unsupported terminal` label followed by the dim ` — <name> · <bundleID>`
// identity in text.detail. The `⚠` glyph is shared with the §11.2 warning flash
// (flashWarningGlyph) so the two warning surfaces stay glyph-consistent.
func unsupportedLeftCluster(name, bundleID string, mode theme.Mode, colourless bool) string {
	amber := headerStyle(theme.MV.AccentOrange, mode, colourless)
	if bundleID == "" {
		return amber.Render(flashWarningGlyph + " " + unsupportedNullLabel)
	}
	label := amber.Render(flashWarningGlyph + " " + unsupportedLabel)
	identity := headerStyle(theme.MV.TextDetail, mode, colourless).
		Render(unsupportedIdentityDash + name + unsupportedIdentityMiddot + bundleID)
	return lipgloss.JoinHorizontal(lipgloss.Top, label, identity)
}

// renderSectionHeaderRow lays out a pre-rendered left cluster and the standard
// right-aligned `/ to filter` hint into the single section-header row via the
// shared right-anchor core (renderRightAnchoredSectionRow). It is the entry point
// both the Sessions and Projects section headers route through, so their
// right-alignment and §2.7 narrow degrade can never drift from each other or from
// the §5 multi-select banner (which shares the same core).
func renderSectionHeaderRow(left string, width int, canvasMode theme.Mode, colourless bool) string {
	// The standard section header's fixed `/ to filter` right hint; the shared core
	// (renderRightAnchoredSectionRow) owns the flex-spacer geometry and the §2.7
	// degrade, so it is passed the rendered hint.
	hint := headerStyle(theme.MV.TextDetail, canvasMode, colourless).Render(sectionFilterHint)
	return renderRightAnchoredSectionRow(left, hint, width, canvasMode, colourless)
}

// renderRightAnchoredSectionRow lays out a pre-rendered left cluster and a
// pre-rendered right hint into the single section-header row, always exactly width
// cells wide: the cluster and the hint are separated by a canvas-painted flex
// spacer. Below the width at which the cluster + a spacer + the hint no longer fit,
// the right hint drops rather than overflow (§2.7). It is the shared right-anchor
// core BOTH the standard Sessions/Projects section headers (`/ to filter` hint, via
// renderSectionHeaderRow) and the §5 multi-select banner (`esc cancel` hint, via
// renderMultiSelectHeader) route through, so their right-alignment, flex spacer, and
// narrow degrade can never drift — only the left cluster and the hint text differ.
func renderRightAnchoredSectionRow(left, hint string, width int, canvasMode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	leftWidth := lipgloss.Width(left)
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

// projectsLeftCluster renders the Projects header's left cluster flush at the
// content's left edge (col 0 of the inset region — the SAME column as the PORTAL
// wordmark and the row selector): `Projects` in state.green, then the count one
// space right at the SAME cap-height (a plain run — no smaller / superscript
// glyph), distinguished only by text.detail (§13.6). NO leading indent (mirrors
// sectionLeftCluster). No mode suffix — the Projects list has a single view.
func projectsLeftCluster(count int, canvasMode theme.Mode, colourless bool) string {
	label := headerStyle(theme.MV.StateGreen, canvasMode, colourless).Render(projectsSectionLabel)
	gap := headerCanvasBg(canvasMode, colourless).Render(" ")
	countRun := headerStyle(theme.MV.TextDetail, canvasMode, colourless).Render(strconv.Itoa(count))
	return lipgloss.JoinHorizontal(lipgloss.Top, label, gap, countRun)
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

// filterPromptPrefix is the §7 accent.orange filter prompt — the `/` glyph plus
// one trailing space — rendered both in the live input (input-active, via the
// bubbles/list FilterInput prompt) and in the locked-query header (list-active,
// via renderFilterQueryHeader). It is the single source of the prompt's wording so
// the two modes read identically aside from the cursor.
const filterPromptPrefix = "/ "

// renderFilterQueryHeader renders the §7.1 list-active LOCKED filter query in the
// section-header row position: the accent.orange `/ ` prompt followed by the
// committed query, also in accent.orange, with NO cursor (the cursor-less locked
// query signals the list is filtered) and NO background tint (the filter input
// carries no bg.selection band, §7.1). The row is padded to the content width with
// canvas spaces so it occupies the full row like the standard section header. It
// REPLACES the section header (and the bubbles/list title) for the FilterApplied
// frame — the input-active frame is owned by the live bubbles/list FilterInput,
// not this function.
//
// Under the NO_COLOR carve-out the hues and the canvas drop; the `/ query` text
// renders on the terminal's native fg/bg (still structurally distinct).
func renderFilterQueryHeader(query string, width int, mode theme.Mode, colourless bool) string {
	w := headerWidthOrFallback(width)
	run := headerStyle(theme.MV.AccentOrange, mode, colourless).Render(filterPromptPrefix + query)
	runWidth := lipgloss.Width(run)
	return headerPadRight(run, runWidth, w, mode, colourless)
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
