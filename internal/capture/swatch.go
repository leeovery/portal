package capture

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

const ContrastValidationFixture = "contrast-validation"

// NewContrastValidationModel renders one palette's tints against that
// palette's own canvas.
func NewContrastValidationModel(th theme.Theme) tea.Model {
	return newSwatchModel(th)
}

// Deliberately not routed through tui.Build — a standalone validation surface.
type swatchModel struct {
	th theme.Theme

	width  int
	height int

	// originalBg is what the terminal looked like before the swatch painted its
	// canvas, so the caller can set it back on exit.
	originalBg string
}

func newSwatchModel(th theme.Theme) swatchModel {
	return swatchModel{th: th}
}

func (s swatchModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

// OriginalBackground is empty until the terminal answers, and empty forever on
// one that never does.
func (s swatchModel) OriginalBackground() string { return s.originalBg }

// PaintedCanvasHex is what View sets the terminal background to, so a caller can
// tell a genuine original from the terminal echoing our own canvas back.
func (s swatchModel) PaintedCanvasHex() string { return s.th.Canvas.Value }

func (s swatchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		// The nil guard is required: String() panics on a nil Color.
		if msg.Color != nil {
			s.originalBg = msg.String()
		}
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return s, tea.Quit
		}
	}
	return s, nil
}

// The terminal background is set (OSC 11) to the theme's canvas so tints are
// judged against the surface they will render on, not an arbitrary bg.
func (s swatchModel) View() tea.View {
	v := tea.NewView(s.fillCanvas(renderSwatch(s.th)))
	v.AltScreen = true
	v.BackgroundColor = s.th.Canvas.Color()
	return v
}

// A fixed 80x24 fallback keeps a sizeless render bounded.
func (s swatchModel) fillCanvas(view string) string {
	w, h := s.width, s.height
	if w == 0 {
		w = 80
	}
	if h == 0 {
		h = 24
	}
	canvas := s.th.Canvas.Color()
	blank := fill(canvas, w)
	lines := strings.Split(view, "\n")
	out := make([]string, 0, h)
	for _, line := range lines {
		if len(out) == h {
			break
		}
		line += fill(canvas, w-lipgloss.Width(line))
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, blank)
	}
	return strings.Join(out, "\n")
}

// Narrow enough to leave the canvas visible on either side, so the tint reads
// as a band rather than edge-to-edge fill.
const bandWidth = 56

const (
	swatchSessionName  = "portal-9fk2"
	swatchSessionPath  = "~/code/portal"
	swatchWindowCount  = "3 windows"
	swatchIndicatorDot = "●"
	swatchAttentionMsg = "⚠ state saver is not running — restore may be incomplete"
)

// Token names are literals — a hand-built Theme carries values without names,
// and the label is exactly what a user types into a theme file.
func renderSwatch(th theme.Theme) string {
	var b strings.Builder

	title := lipgloss.NewStyle().
		Foreground(th.TextPrimary.Color()).
		Background(th.Canvas.Color()).
		Bold(true).
		Render(fmt.Sprintf("CONTRAST VALIDATION — canvas %s", th.Canvas.Value))
	b.WriteString(title)
	b.WriteString("\n\n")

	b.WriteString(tintLabel(th, "bg.selection", th.BgSelection))
	b.WriteString("\n")
	b.WriteString(selectionBand(th))
	b.WriteString("\n")
	// Caption order matches the band's left-to-right order — the order is the mapping.
	b.WriteString(caption(th, "fg-on-tint: text.on-selection · text.tertiary · text.secondary · state.positive · accent.attention"))
	b.WriteString("\n\n")

	b.WriteString(tintLabel(th, "bg.attention", th.BgAttention))
	b.WriteString("\n")
	b.WriteString(attentionBand(th))
	b.WriteString("\n")
	b.WriteString(caption(th, "fg-on-tint: text.on-attention (⚠ message)"))
	b.WriteString("\n\n")

	b.WriteString(tintLabel(th, "bg.subtle", th.BgSubtle))
	b.WriteString("\n")
	b.WriteString(subtleBand(th))
	b.WriteString("\n")
	b.WriteString(caption(th, "bar: accent.primary over the track"))
	b.WriteString("\n\n")

	b.WriteString(tintLabel(th, "border", th.Border))
	b.WriteString("\n")
	b.WriteString(borderRule(th))

	return b.String()
}

func tintLabel(th theme.Theme, name string, tok theme.Token) string {
	return caption(th, fmt.Sprintf("%s  %s", name, tok.Value))
}

func caption(th theme.Theme, text string) string {
	return onCanvas(th, th.TextMuted).Render(text)
}

func onCanvas(th theme.Theme, tok theme.Token) lipgloss.Style {
	return onTint(tok, th.Canvas.Color())
}

func onTint(tok theme.Token, tint color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(tok.Color()).Background(tint)
}

// A non-positive width renders nothing at all — not an empty styled span — so
// callers can hand over an unguarded width difference without adding bytes.
func fill(tint color.Color, n int) string {
	if n <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Background(tint).Render(strings.Repeat(" ", n))
}

func selectionBand(th theme.Theme) string {
	tint := th.BgSelection.Color()
	gap := onTint(th.TextOnSelection, tint).Render("  ")
	content := strings.Join([]string{
		onTint(th.TextOnSelection, tint).Bold(true).Render(swatchSessionName),
		onTint(th.TextTertiary, tint).Render(swatchSessionPath),
		onTint(th.TextSecondary, tint).Render(swatchWindowCount),
		// Packed as the picker's indicator cluster packs them: attached, then
		// pending, adjacent.
		onTint(th.StatePositive, tint).Render(swatchIndicatorDot) +
			onTint(th.AccentAttention, tint).Render(swatchIndicatorDot),
	}, gap)
	return padBand(content, tint)
}

func attentionBand(th theme.Theme) string {
	tint := th.BgAttention.Color()
	return padBand(onTint(th.TextOnAttention, tint).Render(swatchAttentionMsg), tint)
}

// Leaves enough filled bar and empty track for both to be judged.
const subtleBarWidth = 18

func subtleBand(th theme.Theme) string {
	bar := fill(th.AccentPrimary.Color(), subtleBarWidth)
	track := fill(th.BgSubtle.Color(), bandWidth-subtleBarWidth)
	return bar + track
}

func borderRule(th theme.Theme) string {
	return onCanvas(th, th.Border).Render(strings.Repeat("─", bandWidth))
}

func padBand(content string, tint color.Color) string {
	return content + fill(tint, bandWidth-lipgloss.Width(content))
}
