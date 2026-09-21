package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

// The two forms a pane screen takes. The card is measured rather than described,
// so its width and height are stated only where it is built.
type paneScreenParts struct {
	card  func() string
	stack func(width int) []string
}

// Renders parts into a string of exactly w by h cells. The fallback to a plain
// stack exists because a clipped frame would hide the key hints that say which
// keys act.
func renderPaneScreen(parts paneScreenParts, w, h int, th theme.Theme, colourless bool) string {
	w, h = dimsOrFallback(w, h)
	card := parts.card()
	if lipgloss.Width(card) <= w && lipgloss.Height(card) <= h {
		return fillPaneCanvas(placeModalOnClearedCanvas(card, w, h), w, h, th, colourless)
	}
	stack := strings.Join(clampStackToPane(parts.stack(w), w, h), "\n")
	return fillPaneCanvas(stack, w, h, th, colourless)
}

// Drops from the middle: both screens stack their key hints last, and a pane
// that swallows every key it is not answered with must not hide which keys
// answer it. Overflowing instead would scroll the transcript underneath.
func clampStackToPane(rows []string, w, h int) []string {
	clamped := make([]string, 0, len(rows))
	for _, row := range rows {
		clamped = append(clamped, clampToWidth(row, w))
	}
	if len(clamped) <= h {
		return clamped
	}
	if h == 1 {
		return clamped[:1]
	}
	kept := clamped[: h-1 : h-1]
	return append(kept, clamped[len(clamped)-1])
}

// The pane panel's fill: edge to edge, with none of the picker's gutter inset
// and no theme-panel composite.
func fillPaneCanvas(view string, w, h int, th theme.Theme, colourless bool) string {
	if colourless {
		return fillColourless(view, w, h)
	}
	canvas := lipgloss.NewStyle().Background(th.Canvas.Color())
	canvasBg := canvasBgParams(th.Canvas.Color())
	parser := ansi.NewParser()

	out := make([]string, 0, h)
	for line := range strings.SplitSeq(view, "\n") {
		if len(out) == h {
			break
		}
		out = append(out, padLineToCanvasWidth(backfillCanvasBackground(line, canvasBg, parser), w, canvas))
	}
	blank := canvas.Render(strings.Repeat(" ", w))
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}
