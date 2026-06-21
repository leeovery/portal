package capture

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tui/theme"
)

// TestSwatchBandsCoverEveryLightTint asserts the contrast-validation swatch
// renders a labelled band for each of the four 1-9-pinned light surface tints,
// every band carrying its token name + hex so the human can read what they are
// eyeballing against #e1e2e7. This is the §4.1 / §15.6 validation surface — the
// swatch the human captures and eyeballs, NOT the real Sessions surfaces (those
// land in later phases).
func TestSwatchBandsCoverEveryLightTint(t *testing.T) {
	out := renderSwatch(theme.Light)

	mustContain := []string{
		// token names
		"bg.selection", "bg.warning", "bg.track", "border.separator",
		// pinned light hexes
		theme.MV.BgSelection.Light, // #D0C6F0
		theme.MV.BgWarning.Light,   // #E8D6A8
		theme.MV.BgTrack.Light,     // #D2D4DE
		theme.MV.BorderSeparator.Light,
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("swatch (light) missing %q\n--- swatch ---\n%s", want, out)
		}
	}
}

// TestSwatchCoversForegroundOnTintPairings asserts the swatch renders every §4.1
// foreground-on-tint pairing's label so the human can eyeball each on its tint:
// the name (text.on-selection), the count (text.strong) and the attached marker
// (state.green — the single token, darkened light so it clears on bg.selection too)
// ON bg.selection; the warning message (text.on-warning) ON bg.warning.
func TestSwatchCoversForegroundOnTintPairings(t *testing.T) {
	out := renderSwatch(theme.Light)

	mustContain := []string{
		"text.on-selection",
		"text.strong",
		"state.green", // the single green carries the §4.1 `● attached` marker on the tint
		"● attached",  // the green-on-bg.selection glyph+label marker (§4.1)
		"text.on-warning",
		"⚠", // the warning glyph on bg.warning
	}
	for _, want := range mustContain {
		if !strings.Contains(out, want) {
			t.Errorf("swatch (light) missing pairing label %q\n--- swatch ---\n%s", want, out)
		}
	}
}

// TestSwatchAttachedMarkerUsesStateGreen pins that the `● attached` marker on the
// bg.selection band renders in the SINGLE state.green token — whose light value was
// darkened to #3B5E18 so it clears the floor on the tint (the former per-context
// on-selection override was folded into the global token). The swatch surfaces the
// marker so the human re-eyeballs the fix.
func TestSwatchAttachedMarkerUsesStateGreen(t *testing.T) {
	out := selectionBand(theme.Light)
	// The marker renders in the light state.green value (#3B5E18 = rgb 59,94,24).
	if theme.MV.StateGreen.Light != "#3B5E18" {
		t.Fatalf("state.green light = %q, want #3B5E18 (darkened so the single token clears bg.selection)", theme.MV.StateGreen.Light)
	}
	if !strings.Contains(out, "59;94;24") { // #3B5E18 = rgb(59,94,24)
		t.Errorf("selection band does not render the attached marker in state.green #3B5E18 (rgb 59,94,24)\n--- band ---\n%q", out)
	}
}

// TestSwatchRendersBothModes asserts the swatch renders in dark and light — the
// owned canvas is selected by the mode, so the same band set renders against the
// dark canvas (#0b0c14) and the light canvas (#e1e2e7). The dark render must
// carry the dark tint hexes (its own canvas pass), proving the mode actually
// drives the resolved variant.
func TestSwatchRendersBothModes(t *testing.T) {
	dark := renderSwatch(theme.Dark)
	light := renderSwatch(theme.Light)

	if dark == light {
		t.Fatal("dark and light swatches are byte-identical — the mode is not driving the resolved tints")
	}
	if !strings.Contains(dark, theme.MV.BgSelection.Dark) {
		t.Errorf("dark swatch missing dark bg.selection hex %q", theme.MV.BgSelection.Dark)
	}
	if !strings.Contains(light, theme.MV.BgSelection.Light) {
		t.Errorf("light swatch missing light bg.selection hex %q", theme.MV.BgSelection.Light)
	}
}

// TestSwatchModeFromAppearance maps the prefs.Appearance pin the capture harness
// drives to the theme.Mode the swatch resolves its owned canvas from, mirroring
// the production WithCanvasMode pin path (a light/dark pin paints from frame one;
// auto falls back to the dark canvas).
func TestSwatchModeFromAppearance(t *testing.T) {
	cases := []struct {
		name string
		mode theme.Mode
		want string // the canvas hex that mode paints
	}{
		{"dark", theme.Dark, theme.MV.Canvas.Dark},
		{"light", theme.Light, theme.MV.Canvas.Light},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := theme.MV.Canvas.ColorFor(c.mode); got == nil {
				t.Fatal("canvas resolved to nil")
			}
			// The swatch's View must paint that canvas — assert the resolved hex is
			// present in the swatch's reported background.
			s := newSwatchModel(c.mode)
			if s.canvasHex() != c.want {
				t.Errorf("canvasHex() = %q, want %q", s.canvasHex(), c.want)
			}
		})
	}
}
