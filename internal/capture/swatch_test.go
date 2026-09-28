package capture

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

func swatchTestPalette(canvas, hi string) theme.Theme {
	value := func(n int) theme.Token {
		return theme.Token{Value: fmt.Sprintf("#%s%04d", hi, n)}
	}
	return theme.Theme{
		TextPrimary:      value(1),
		TextSecondary:    value(2),
		TextTertiary:     value(3),
		TextMuted:        value(4),
		TextSubtle:       value(5),
		TextFaint:        value(6),
		TextOnSelection:  value(7),
		AccentPrimary:    value(8),
		AccentKey:        value(9),
		AccentMode:       value(10),
		AccentAttention:  value(11),
		StatePositive:    value(12),
		StateDestructive: value(13),
		Canvas:           theme.Token{Value: canvas},
		BgSelection:      value(15),
		BgAttention:      value(16),
		BgSubtle:         value(17),
		Border:           value(18),
		TextOnAttention:  value(19),
	}
}

func wantStyle(fg, bg theme.Token) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg.Color()).Background(bg.Color())
}

func TestSwatch_RendersFromInjectedTheme(t *testing.T) {
	injected := swatchTestPalette("#0B0C14", "AA")
	other := swatchTestPalette("#E1E2E7", "BB")

	out := renderSwatch(injected)

	for _, tok := range []theme.Token{injected.Canvas, injected.BgSelection, injected.BgAttention, injected.BgSubtle, injected.Border} {
		if !strings.Contains(out, tok.Value) {
			t.Errorf("swatch does not carry the injected theme's %s\n--- swatch ---\n%s", tok.Value, out)
		}
	}

	for _, tok := range other.All() {
		if strings.Contains(out, tok.Value) {
			t.Errorf("swatch carries %s, which belongs to a theme it was never handed", tok.Value)
		}
	}

	if renderSwatch(other) == out {
		t.Error("two disjoint palettes render byte-identical swatches — the theme is not driving the render")
	}
}

func TestSwatch_UsesThemeOwnCanvas(t *testing.T) {
	cases := []struct {
		name  string
		theme theme.Theme
	}{
		{"dark theme", swatchTestPalette("#0B0C14", "AA")},
		{"light theme", swatchTestPalette("#E1E2E7", "BB")},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSwatchModel(c.theme)

			if got := s.View().BackgroundColor; got != c.theme.Canvas.Color() {
				t.Errorf("View().BackgroundColor = %v, want the theme's canvas %v", got, c.theme.Canvas.Color())
			}

			out := renderSwatch(c.theme)
			if title := fmt.Sprintf("CONTRAST VALIDATION — canvas %s", c.theme.Canvas.Value); !strings.Contains(out, title) {
				t.Errorf("swatch title does not read %q\n--- swatch ---\n%s", title, out)
			}
			wantLabel := wantStyle(c.theme.TextMuted, c.theme.Canvas).
				Render(fmt.Sprintf("border  %s", c.theme.Border.Value))
			if !strings.Contains(out, wantLabel) {
				t.Errorf("the border label is not painted on the theme's canvas\nwant %q\n--- swatch ---\n%s", wantLabel, out)
			}
			wantRule := wantStyle(c.theme.Border, c.theme.Canvas).Render(strings.Repeat("─", bandWidth))
			if got := borderRule(c.theme); got != wantRule {
				t.Errorf("the border rule is not painted on the theme's canvas\n got %q\nwant %q", got, wantRule)
			}
		})
	}
}

func TestSwatchBandsCoverEveryPinnedTint(t *testing.T) {
	th := swatchTestPalette("#E1E2E7", "CC")
	out := renderSwatch(th)

	pinned := []struct {
		label string
		token theme.Token
	}{
		{"bg.selection", th.BgSelection},
		{"bg.attention", th.BgAttention},
		{"bg.subtle", th.BgSubtle},
		{"border", th.Border},
	}

	for _, tint := range pinned {
		want := fmt.Sprintf("%s  %s", tint.label, tint.token.Value)
		if !strings.Contains(out, want) {
			t.Errorf("swatch is missing the labelled band %q\n--- swatch ---\n%s", want, out)
		}
	}

	surfaces := map[string]string{
		"bg.selection": selectionBand(th),
		"bg.attention": attentionBand(th),
		"bg.subtle":    subtleBand(th),
		"border":       borderRule(th),
	}
	for label, surface := range surfaces {
		if got := lipgloss.Width(surface); got != bandWidth {
			t.Errorf("the %s band is %d cells wide, want %d", label, got, bandWidth)
		}
		if !strings.Contains(out, surface) {
			t.Errorf("the %s band is not in the composed swatch\n--- swatch ---\n%s", label, out)
		}
	}

	bar := lipgloss.NewStyle().Background(th.AccentPrimary.Color()).Render(strings.Repeat(" ", subtleBarWidth))
	track := lipgloss.NewStyle().Background(th.BgSubtle.Color()).Render(strings.Repeat(" ", bandWidth-subtleBarWidth))
	if got := surfaces["bg.subtle"]; got != bar+track {
		t.Errorf("the bg.subtle band is not the accent.primary bar over the bg.subtle track\n got %q\nwant %q", got, bar+track)
	}
}

// Independently expressed styled-blank padding — the reference the swatch's own
// rendering is held to, byte for byte.
func wantFilledCanvas(th theme.Theme, view string, w, h int) string {
	style := lipgloss.NewStyle().Background(th.Canvas.Color())
	lines := strings.Split(view, "\n")
	out := make([]string, 0, h)
	for _, line := range lines {
		if len(out) == h {
			break
		}
		if gap := w - lipgloss.Width(line); gap > 0 {
			line += style.Render(strings.Repeat(" ", gap))
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, style.Render(strings.Repeat(" ", w)))
	}
	return strings.Join(out, "\n")
}

func TestSwatchViewFillsTheCanvasVerbatim(t *testing.T) {
	th := swatchTestPalette("#0B0C14", "EE")

	cases := []struct {
		name  string
		set   bool
		w, h  int
		wantW int
		wantH int
	}{
		{name: "explicit size", set: true, w: 100, h: 30, wantW: 100, wantH: 30},
		{name: "truncating size", set: true, w: 40, h: 5, wantW: 40, wantH: 5},
		{name: "sizeless fallback", wantW: 80, wantH: 24},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newSwatchModel(th)
			if c.set {
				updated, _ := s.Update(tea.WindowSizeMsg{Width: c.w, Height: c.h})
				s = updated.(swatchModel)
			}

			got := s.View().Content
			want := wantFilledCanvas(th, renderSwatch(th), c.wantW, c.wantH)
			if got != want {
				t.Errorf("filled canvas differs from the styled-blank reference\n got %q\nwant %q", got, want)
			}
			if lines := strings.Split(got, "\n"); len(lines) != c.wantH {
				t.Errorf("filled canvas is %d lines tall, want %d", len(lines), c.wantH)
			}
			// Padding widens short lines; an over-wide content line is left as it is.
			for i, line := range strings.Split(got, "\n") {
				if w := lipgloss.Width(line); w < c.wantW {
					t.Errorf("line %d is %d cells wide, want at least %d", i, w, c.wantW)
				}
			}
		})
	}
}

func TestFillIsEmptyForNonPositiveWidth(t *testing.T) {
	tint := swatchTestPalette("#0B0C14", "FF").BgSubtle.Color()

	for _, n := range []int{0, -1, -12} {
		if got := fill(tint, n); got != "" {
			t.Errorf("fill(tint, %d) = %q, want an empty string", n, got)
		}
	}
}

func TestSwatchCoversForegroundOnTintPairings(t *testing.T) {
	th := swatchTestPalette("#0B0C14", "DD")
	out := renderSwatch(th)

	pairings := []struct {
		label string
		band  string
		want  string
	}{
		{"text.on-selection", selectionBand(th),
			wantStyle(th.TextOnSelection, th.BgSelection).Bold(true).Render(swatchSessionName)},
		{"text.tertiary", selectionBand(th),
			wantStyle(th.TextTertiary, th.BgSelection).Render(swatchSessionPath)},
		{"text.secondary", selectionBand(th),
			wantStyle(th.TextSecondary, th.BgSelection).Render(swatchWindowCount)},
		{"state.positive", selectionBand(th),
			wantStyle(th.StatePositive, th.BgSelection).Render(swatchIndicatorDot) +
				wantStyle(th.AccentAttention, th.BgSelection).Render(swatchIndicatorDot)},
		{"accent.attention", selectionBand(th),
			wantStyle(th.StatePositive, th.BgSelection).Render(swatchIndicatorDot) +
				wantStyle(th.AccentAttention, th.BgSelection).Render(swatchIndicatorDot)},
		{"text.on-attention", attentionBand(th),
			wantStyle(th.TextOnAttention, th.BgAttention).Render(swatchAttentionMsg)},
	}

	for _, pairing := range pairings {
		if !strings.Contains(pairing.band, pairing.want) {
			t.Errorf("%s is not rendered on its tint\n--- band ---\n%q", pairing.label, pairing.band)
		}
		if !strings.Contains(out, pairing.label) {
			t.Errorf("no caption names the %s pairing\n--- swatch ---\n%s", pairing.label, out)
		}
	}

	if strings.Contains(out, "attached") {
		t.Errorf("swatch still carries the retired worded attached marker\n--- swatch ---\n%s", out)
	}

	selectionCaption := "state.positive · accent.attention"
	if !strings.Contains(out, selectionCaption) {
		t.Errorf("the selection-band caption does not name accent.attention after state.positive\n--- swatch ---\n%s", out)
	}

	for _, glyph := range []string{"●", "⚠"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("swatch is missing the %q glyph\n--- swatch ---\n%s", glyph, out)
		}
	}
}
