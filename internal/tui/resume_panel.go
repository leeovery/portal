package tui

import (
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/resumekeys"
	"github.com/leeovery/portal/internal/theme"
)

const (
	resumePanelTitle   = "Resume session"
	resumePausedBadge  = "● PAUSED"
	resumeCommandLabel = "ON RESUME"

	resumeKeyResume    = "⏎"
	resumeLabelResume  = "resume"
	resumeLabelDiscard = "discard"
	resumeKeyBack      = "esc"
	resumeLabelBack    = "back"
)

// The whole input to the waiting panel. Report is the row the card carries only
// when there is something to say.
type ResumeScreen struct {
	Command    string
	Report     string
	Width      int
	Height     int
	Theme      theme.Theme
	Colourless bool
}

// RenderResumePanel draws the waiting panel as a string of exactly Width by
// Height cells, or a fallback size where either is not positive, as the card
// when the pane holds it and as the plain stack below that size.
func RenderResumePanel(s ResumeScreen) string {
	parts := paneScreenParts{
		card:  func() string { return resumeCard(s) },
		stack: func(width int) []string { return resumeStack(s, width) },
	}
	return renderPaneScreen(parts, s.Width, s.Height, s.Theme, s.Colourless)
}

func resumeCard(s ResumeScreen) string {
	header := []string{resumeCardHeaderRow(s.Theme, s.Colourless)}
	footer := []string{resumeKeyHintRow(s.Theme, s.Colourless)}
	compartments := [][]string{header, resumeCardBodyRows(s), footer}
	return renderJoinedPanel(compartments, s.Theme.Border, s.Theme, s.Colourless)
}

func resumeCardHeaderRow(th theme.Theme, colourless bool) string {
	title := headerStyle(th.TextPrimary, th, colourless).Bold(true).Render(resumePanelTitle)
	return renderHeaderWithBadge(title, resumeCardContentWidth, true, resumePausedBadge, th, colourless)
}

func resumeCardBodyRows(s ResumeScreen) []string {
	label := headerStyle(s.Theme.AccentPrimary, s.Theme, s.Colourless).Render(resumeCommandLabel)
	rows := []string{resumePaddedRow(label, resumeCardContentWidth, s.Theme, s.Colourless)}
	rows = append(rows, resumeCommandRows(s.Command, resumeCardContentWidth, s.Theme.TextPrimary, false, s.Theme, s.Colourless)...)
	return appendResumeReportRow(rows, s, resumeCardContentWidth)
}

// The title alone opens it and the key hints close it: the pane's clamp drops
// from the middle, so what the smallest pane keeps is what it is holding and
// which keys answer it.
func resumeStack(s ResumeScreen, width int) []string {
	title := clampToWidth(resumePanelTitle, width)
	rows := []string{headerStyle(s.Theme.TextPrimary, s.Theme, s.Colourless).Bold(true).Render(title)}
	rows = append(rows, resumeCommandRows(s.Command, width, s.Theme.TextPrimary, false, s.Theme, s.Colourless)...)
	rows = appendResumeReportRow(rows, s, width)
	return append(rows, resumeStackKeyHintRow(width, s.Theme, s.Colourless))
}

func appendResumeReportRow(rows []string, s ResumeScreen, width int) []string {
	if row, ok := resumeReportRow(s.Report, width, s.Theme, s.Colourless); ok {
		return append(rows, row)
	}
	return rows
}

// The two keys that answer the panel lead, and Escape, which backs out of the
// pane instead, comes last.
func resumeKeyHints() []footerHintGroup {
	return []footerHintGroup{
		{resumeKeyResume, resumeLabelResume},
		{string(resumekeys.Discard), resumeLabelDiscard},
		{resumeKeyBack, resumeLabelBack},
	}
}

func resumeKeyHintRow(th theme.Theme, colourless bool) string {
	return renderModalFooter(resumeKeyHints(), th, colourless)
}

// A stack too narrow for every hint gives up Escape's whole rather than showing
// a clipped word of it. The two answering hints are never dropped: below their
// width the pane's clamp clips them as it always has.
func resumeStackKeyHintRow(width int, th theme.Theme, colourless bool) string {
	hints := resumeKeyHints()
	if row := renderModalFooter(hints, th, colourless); lipgloss.Width(row) <= width {
		return row
	}
	return renderModalFooter(hints[:len(hints)-1], th, colourless)
}
