package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
)

func canvasStyle(th theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Background(th.Canvas.Color())
}

// Renders view as w by h painted cells: content taller than h is clamped, a
// shorter view padded with blank canvas rows. Each line is
// padded individually rather than via lipgloss.Place, which pads only beyond the
// single widest line. Under NO_COLOR the layout holds and every padding cell is a
// plain space with no background SGR.
func fillPaneCanvas(view string, w, h int, th theme.Theme, colourless bool) string {
	if colourless {
		return fillColourless(view, w, h)
	}
	canvas := canvasStyle(th)
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
