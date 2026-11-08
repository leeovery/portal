package tui

import (
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const (
	helpTitleGlyph = "?"
	helpTitle      = "Keybindings"
	helpColumnGap  = "   "
	// Sized for the widest glyph ("Home/End") so labels share a left edge.
	helpKeyColumnWidth = 10
)

const (
	attachedLegendLabel = "Session attached"
	pendingLegendLabel  = "Resume pending"
)

// Deliberately not a keymapEntry: an indicator binds no key.
type helpLegendEntry struct {
	indicator string
	label     string
}

// Rendered through the session row's own indicator renderer, in the row's
// packing order, so the legend moves with any change to a glyph, token or the
// NO_COLOR letter.
func sessionsIndicatorLegend(th theme.Theme, colourless bool) []helpLegendEntry {
	d := SessionDelegate{Theme: th, Colourless: colourless}
	return []helpLegendEntry{
		{indicator: d.indicatorCluster(rowIndicators{attached: true}, false), label: attachedLegendLabel},
		{indicator: d.indicatorCluster(rowIndicators{pending: true}, false), label: pendingLegendLabel},
	}
}

func renderHelpModalContent(entries []keymapEntry, th theme.Theme, colourless bool, legend []helpLegendEntry) string {
	return renderJoinedPanel(helpModalLayout(entries, legend, false, th, colourless).compartments, th.Border, th, colourless)
}

// helpModalLayout is the help panel: its title holds still, and the keys and
// the legend under them scroll. scrolls has the title name the scroll keys.
func helpModalLayout(entries []keymapEntry, legend []helpLegendEntry, scrolls bool, th theme.Theme, colourless bool) panelLayout {
	bodyRows := helpModalBodyRows(entries, th, colourless)
	legendRows := helpModalLegendRows(legend, th, colourless)

	// The title must be laid out here rather than inside renderJoinedPanel: its
	// width depends on contentWidth, since `esc close` right-aligns to it.
	contentWidth := lipgloss.Width(helpModalHeader(0, scrolls, th, colourless))
	for _, r := range slices.Concat(bodyRows, legendRows) {
		contentWidth = max(contentWidth, lipgloss.Width(r))
	}
	title := helpModalHeader(contentWidth, scrolls, th, colourless)

	compartments := [][]string{{title}, bodyRows}
	if len(legendRows) > 0 {
		compartments = append(compartments, legendRows)
	}
	return panelLayout{compartments: compartments, head: 1}
}

// helpModalHint is what the title says of the help modal's own keys, each as
// its descriptor names it; the scroll keys only where the body scrolls.
func helpModalHint(scrolls bool) string {
	var hints []string
	for _, e := range helpModalKeymap() {
		if e.Key == helpScrollKey && !scrolls {
			continue
		}
		hints = append(hints, e.Key+" "+e.Action)
	}
	return strings.Join(hints, footerEntrySeparator)
}

// The indicator arrives already rendered, so the glyph column takes it
// unstyled rather than re-colouring it.
func helpModalLegendRows(legend []helpLegendEntry, th theme.Theme, colourless bool) []string {
	rows := make([]string, 0, len(legend))
	for _, e := range legend {
		rows = append(rows, keyColumnRow(
			e.indicator, e.label,
			lipgloss.NewStyle(),
			headerStyle(th.TextSecondary, th, colourless),
			helpKeyColumnWidth, helpColumnGap, th, colourless,
		))
	}
	return rows
}

// At or below the natural width (including the width-0 probe) it renders at
// natural width rather than dropping the hint or wrapping.
func helpModalHeader(width int, scrolls bool, th theme.Theme, colourless bool) string {
	left := modalTitle(helpTitleGlyph, helpTitle, th, colourless)
	leftWidth := lipgloss.Width(left)

	dismiss := headerStyle(th.TextMuted, th, colourless).Render(helpModalHint(scrolls))
	dismissWidth := lipgloss.Width(dismiss)

	naturalWidth := leftWidth + 1 + dismissWidth
	spacerWidth := 1
	if width > naturalWidth {
		spacerWidth = width - leftWidth - dismissWidth
	}
	spacer := headerCanvasBg(th, colourless).Render(strings.Repeat(" ", spacerWidth))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, spacer, dismiss)
}

// modalTitle is a modal's title as the help modal draws its own: glyph in the
// accent, then title in bold primary text. Under NO_COLOR the glyph and the
// bold carry it.
func modalTitle(glyph, title string, th theme.Theme, colourless bool) string {
	glyphSeg := headerStyle(th.AccentPrimary, th, colourless).Bold(true).Render(glyph)
	gap := headerCanvasBg(th, colourless).Render(" ")
	titleSeg := headerStyle(th.TextPrimary, th, colourless).Bold(true).Render(title)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyphSeg, gap, titleSeg)
}

func helpModalBody(entries []keymapEntry, th theme.Theme, colourless bool) string {
	return lipgloss.JoinVertical(lipgloss.Left, helpModalBodyRows(entries, th, colourless)...)
}

func helpModalBodyRows(entries []keymapEntry, th theme.Theme, colourless bool) []string {
	rows := make([]string, 0, len(entries))
	for _, e := range entries {
		// Skip the `?` self-entry — a help modal does not list its own open key.
		if e.RightAligned {
			continue
		}
		rows = append(rows, helpModalRow(e, th, colourless))
	}
	return rows
}

func helpModalRow(e keymapEntry, th theme.Theme, colourless bool) string {
	keyTok := th.AccentKey
	if isDestructiveHelpKey(e) {
		keyTok = th.StateDestructive
	}
	return keyColumnRow(
		helpKeyGlyph(e), helpActionLabel(e),
		headerStyle(keyTok, th, colourless).Bold(true),
		headerStyle(th.TextSecondary, th, colourless),
		helpKeyColumnWidth, helpColumnGap, th, colourless,
	)
}

func helpActionLabel(e keymapEntry) string {
	if e.HelpAction != "" {
		return e.HelpAction
	}
	return e.Action
}

func helpKeyGlyph(e keymapEntry) string {
	if e.HelpKey != "" {
		return e.HelpKey
	}
	return e.Key
}

// Reads the structural flag rather than matching key glyphs, so a future
// non-destructive `d`/`k` binding cannot render red.
func isDestructiveHelpKey(e keymapEntry) bool {
	return e.Destructive
}
