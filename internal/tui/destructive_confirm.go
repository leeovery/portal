package tui

import (
	"cmp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

const (
	destructiveTitleGlyph = "▲"
	// Wrap target and minimum content width, so the panel size is stable
	// regardless of target-name/path length.
	destructiveBodyWidth   = 52
	destructiveKeyCancel   = "esc"
	destructiveLabelCancel = "cancel"
)

type destructiveConfirmSpec struct {
	// title heads the confirmation in its own compartment; empty, it has none.
	title      string
	targetName string
	// Pre-rendered rows standing in for the single name row, for a target that
	// does not fit one. Empty leaves the name row in place.
	targetRows    []string
	extraBodyRows []string
	// consequence follows the rows above after a blank one; empty, neither is
	// drawn.
	consequence string
	// The row a confirmation carries only when its own act could not be carried
	// out; it closes the body, immediately above the key hints.
	reportRows   []string
	confirmKey   string
	confirmLabel string
	// keep is a choice beside the cancel that also keeps the target, its hint
	// between the confirmation's and the cancel's. Its zero value offers none.
	keep footerHintGroup
	// cancelLabel is what Esc's hint says it does, cancel when empty.
	cancelLabel string
}

func renderDestructiveConfirm(spec destructiveConfirmSpec, th theme.Theme, colourless bool) string {
	return renderJoinedPanel(destructiveConfirmLayout(spec, th, colourless).compartments, th.Border, th, colourless)
}

// destructiveConfirmLayout is the framed confirmation as a modal fits it: its
// title and key hints hold still, and its body scrolls.
func destructiveConfirmLayout(spec destructiveConfirmSpec, th theme.Theme, colourless bool) panelLayout {
	compartments := destructiveConfirmCompartments(spec, destructiveBodyWidth, th, colourless)
	return panelLayout{compartments: compartments, head: len(compartments) - 2, foot: 1}
}

// The confirmation's parts, so a caller that cannot afford the frame stacks the
// same rows the frame would have held rather than assembling its own lookalike.
// wrapWidth is the width the consequence is wrapped at: the framed path passes
// the panel's own, a pane passes its own.
func destructiveConfirmCompartments(spec destructiveConfirmSpec, wrapWidth int, th theme.Theme, colourless bool) [][]string {
	body := destructiveBodyRows(spec, wrapWidth, th, colourless)
	footer := []string{destructiveFooterRow(spec, th, colourless)}
	if spec.title == "" {
		return [][]string{body, footer}
	}
	header := []string{destructiveHeaderRow(spec.title, th, colourless)}
	return [][]string{header, body, footer}
}

// Glyph + bold carry the destructive signal under NO_COLOR, where the hue drops.
func destructiveHeaderRow(title string, th theme.Theme, colourless bool) string {
	style := headerStyle(th.StateDestructive, th, colourless).Bold(true)
	glyph := style.Render(destructiveTitleGlyph)
	gap := headerCanvasBg(th, colourless).Render(" ")
	titleSeg := style.Render(title)
	return lipgloss.JoinHorizontal(lipgloss.Top, glyph, gap, titleSeg)
}

func destructiveBodyRows(spec destructiveConfirmSpec, wrapWidth int, th theme.Theme, colourless bool) []string {
	rows := make([]string, 0, len(spec.targetRows)+len(spec.extraBodyRows)+len(spec.reportRows)+3)
	if len(spec.targetRows) > 0 {
		rows = append(rows, spec.targetRows...)
	} else {
		rows = append(rows, destructiveNameRow(spec.targetName, th, colourless))
	}
	rows = append(rows, spec.extraBodyRows...)
	if spec.consequence != "" {
		rows = append(rows, headerCanvasBg(th, colourless).Render(""))
		rows = append(rows, destructiveConsequenceRows(spec.consequence, wrapWidth, th, colourless)...)
	}
	return append(rows, spec.reportRows...)
}

func destructiveNameRow(name string, th theme.Theme, colourless bool) string {
	return headerStyle(th.StateDestructive, th, colourless).Bold(true).Render(name)
}

func destructiveConsequenceRows(text string, wrapWidth int, th theme.Theme, colourless bool) []string {
	wrapped := ansi.Wordwrap(text, wrapWidth, "")
	style := headerStyle(th.TextMuted, th, colourless)
	lines := strings.Split(wrapped, "\n")
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, style.Render(line))
	}
	return rows
}

func destructiveFooterRow(spec destructiveConfirmSpec, th theme.Theme, colourless bool) string {
	groups := []footerHintGroup{{spec.confirmKey, spec.confirmLabel}}
	if spec.keep.key != "" {
		groups = append(groups, spec.keep)
	}
	groups = append(groups, footerHintGroup{destructiveKeyCancel, cmp.Or(spec.cancelLabel, destructiveLabelCancel)})
	return renderModalFooter(groups, th, colourless)
}
