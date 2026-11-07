package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
)

const (
	// Must stay the exact prefix of sessionListTitleForMode's output — the
	// suffix is the remainder after it.
	sectionLabel         = "Sessions"
	projectsSectionLabel = "Projects"
	// The `s switch view` hint is the footer's, deliberately not duplicated here.
	sectionFilterHint     = "/ to filter"
	multiSelectCancelHint = "esc cancel"

	unsupportedLabel    = "unsupported terminal"
	unsupportedDocsHint = "see docs"
	// OSC 8 link target — never printed as visible text.
	unsupportedDocsURL        = "https://github.com/leeovery/portal/blob/main/docs/custom-terminals.md"
	unsupportedIdentityDash   = " — "
	unsupportedIdentityMiddot = " · "

	// Esc clears the abort banner while staying in multi-select mode.
	preflightAbortDismissHint = "esc dismiss"
)

// The title's room is what the count and the hint leave, so a long current
// label is cut rather than pushing the hint off the row.
func renderSectionHeader(mode prefs.SessionListMode, currentSession string, count, width int, th theme.Theme, colourless bool) string {
	countText := strconv.Itoa(count)
	titleWidth := headerWidthOrFallback(width) - lipgloss.Width(sectionFilterHint) - 1 - len(" "+countText)
	left := sectionLeftCluster(sessionListTitleForMode(mode, currentSession, max(titleWidth, 1)), countText, th, colourless)
	return renderSectionHeaderRow(left, width, th, colourless)
}

func renderProjectsSectionHeader(count, width int, th theme.Theme, colourless bool) string {
	left := projectsLeftCluster(count, th, colourless)
	return renderSectionHeaderRow(left, width, th, colourless)
}

// A section-header variant, not a notice band — no `▌` left-bar.
func renderMultiSelectHeader(count, width int, th theme.Theme, colourless bool) string {
	left := headerStyle(th.AccentPrimary, th, colourless).Render(strconv.Itoa(count) + " selected")
	hint := headerStyle(th.TextMuted, th, colourless).Render(multiSelectCancelHint)
	return renderRightAnchoredSectionRow(left, hint, width, th, colourless)
}

// No right hint — the empty hint pads the right side with canvas.
func renderOpeningBand(done, total, width int, th theme.Theme, colourless bool) string {
	left := headerStyle(th.AccentPrimary, th, colourless).
		Render(fmt.Sprintf("Opening %d/%d…", done, total))
	return renderRightAnchoredSectionRow(left, "", width, th, colourless)
}

// Named-only: bundleID != "" always holds, because unsupportedBannerActive
// carries the !IsNull() discriminator.
func renderUnsupportedHeader(name, bundleID string, width int, th theme.Theme, colourless bool) string {
	left := unsupportedLeftCluster(name, bundleID, th, colourless)
	// OSC 8 is orthogonal to colour, so the link is emitted unconditionally and
	// degrades to the bare word where the terminal does not render it.
	hint := headerStyle(th.AccentKey, th, colourless).
		Hyperlink(unsupportedDocsURL).
		Render(unsupportedDocsHint)
	return renderRightAnchoredSectionRow(left, hint, width, th, colourless)
}

func renderPreflightAbortHeader(message string, width int, th theme.Theme, colourless bool) string {
	left := headerStyle(th.StateDestructive, th, colourless).
		Render(flashWarningGlyph + " " + message)
	hint := headerStyle(th.TextMuted, th, colourless).Render(preflightAbortDismissHint)
	return renderRightAnchoredSectionRow(left, hint, width, th, colourless)
}

func unsupportedLeftCluster(name, bundleID string, th theme.Theme, colourless bool) string {
	label := headerStyle(th.AccentAttention, th, colourless).
		Render(flashWarningGlyph + " " + unsupportedLabel)
	identity := headerStyle(th.TextMuted, th, colourless).
		Render(unsupportedIdentityDash + name + unsupportedIdentityMiddot + bundleID)
	return lipgloss.JoinHorizontal(lipgloss.Top, label, identity)
}

func renderSectionHeaderRow(left string, width int, th theme.Theme, colourless bool) string {
	hint := headerStyle(th.TextMuted, th, colourless).Render(sectionFilterHint)
	return renderRightAnchoredSectionRow(left, hint, width, th, colourless)
}

// Always exactly width cells, so section-header variants cannot drift in
// alignment or narrow degrade.
func renderRightAnchoredSectionRow(left, hint string, width int, th theme.Theme, colourless bool) string {
	w := headerWidthOrFallback(width)
	leftWidth := lipgloss.Width(left)
	hintWidth := lipgloss.Width(hint)

	if leftWidth >= w || leftWidth+1+hintWidth > w {
		return headerPadRight(left, leftWidth, w, th, colourless)
	}

	spacerWidth := w - leftWidth - hintWidth
	spacer := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, hint)
}

func projectsLeftCluster(count int, th theme.Theme, colourless bool) string {
	label := headerStyle(th.StatePositive, th, colourless).Render(projectsSectionLabel)
	gap := headerCanvasBg(th, colourless).Render(" ")
	countRun := headerStyle(th.TextMuted, th, colourless).Render(strconv.Itoa(count))
	return lipgloss.JoinHorizontal(lipgloss.Top, label, gap, countRun)
}

// Flush at the content's left edge — the same column as the header wordmark and
// the row cursor. Do not add a leading indent. The count sits between the
// title's first word and the rest of it.
func sectionLeftCluster(title, count string, th theme.Theme, colourless bool) string {
	label := headerStyle(th.AccentMode, th, colourless).Render(sectionLabel)

	gap := headerCanvasBg(th, colourless).Render(" ")
	countRun := headerStyle(th.StatePositive, th, colourless).Render(count)

	cluster := lipgloss.JoinHorizontal(lipgloss.Top, label, gap, countRun)

	if suffix := strings.TrimPrefix(title, sectionLabel); suffix != "" {
		suffixRun := headerStyle(th.TextMuted, th, colourless).Render(suffix)
		cluster = lipgloss.JoinHorizontal(lipgloss.Top, cluster, suffixRun)
	}
	return cluster
}

// Shared with the live filter input so the two modes read identically.
const filterPromptPrefix = "/ "

// The FilterApplied frame: no cursor, signalling the list rather than the input
// is focused. The input-active frame belongs to bubbles/list's own FilterInput.
func renderFilterQueryHeader(query string, width int, th theme.Theme, colourless bool) string {
	w := headerWidthOrFallback(width)
	run := headerStyle(th.AccentAttention, th, colourless).Render(filterPromptPrefix + query)
	runWidth := lipgloss.Width(run)
	return headerPadRight(run, runWidth, w, th, colourless)
}
