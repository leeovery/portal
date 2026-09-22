package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// Both screens' cards are built on this one width, so content cannot move
// their geometry.
const resumeCardContentWidth = destructiveBodyWidth

const resumeCommandMaxLines = 3

const resumeEllipsis = "…"

// A registered command is arbitrary user-authored text: an ESC left in it would
// repaint the card around it, and a newline or tab would move its geometry.
func sanitiseCommandText(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

func resumeCommandLines(command string, width int) []string {
	return wrapCapped(sanitiseCommandText(command), width, resumeCommandMaxLines, resumeEllipsis)
}

func resumeCommandRows(command string, width int, tok theme.Token, bold bool, th theme.Theme, colourless bool) []string {
	style := headerStyle(tok, th, colourless)
	if bold {
		style = style.Bold(true)
	}
	lines := resumeCommandLines(command, width)
	rows := make([]string, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, resumePaddedRow(style.Render(line), width, th, colourless))
	}
	return rows
}

// Truncated rather than wrapped: a report that wrapped would change the card's
// height between two screens that are otherwise identical.
func resumeReportRow(report string, width int, th theme.Theme, colourless bool) (string, bool) {
	if report == "" {
		return "", false
	}
	text := ansi.Truncate(sanitiseCommandText(report), width, resumeEllipsis)
	seg := headerStyle(th.AccentAttention, th, colourless).Render(text)
	return resumePaddedRow(seg, width, th, colourless), true
}

func resumePaddedRow(seg string, width int, th theme.Theme, colourless bool) string {
	return headerPadRight(seg, lipgloss.Width(seg), width, th, colourless)
}
