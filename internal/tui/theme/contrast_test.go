package theme_test

import (
	"math"
	"testing"

	"github.com/leeovery/portal/internal/tui/theme"
	"github.com/lucasb-eyer/go-colorful"
)

// This file is the §2.3 / §2.9 contrast-floor numeric gate. It measures every
// foreground token against ITS OWN mode-canvas (dark vs #0b0c14, light vs
// #e1e2e7) — the two variants resolve INDEPENDENTLY (§2.9: each value need only
// clear against its own canvas). The floor is a hard gate checked numerically
// before any taste judgement; the in-terminal eyeball (1-9) is the visual lock
// on top of this, not a replacement for it.
//
// The math is WCAG 2.x relative luminance + contrast ratio. go-colorful's
// LinearRgb() applies the exact sRGB linearization WCAG specifies (v/12.92 below
// 0.04045, ((v+0.055)/1.055)^2.4 above), so relative luminance is the standard
// 0.2126·R + 0.7152·G + 0.0722·B over the linearized channels. We validate the
// math against the known reference black/white = 21.00 in TestContrastMath.

const (
	canvasDark  = "#0b0c14" // owned dark canvas (§1 / §2.9)
	canvasLight = "#e1e2e7" // owned light canvas (§1 / §2.9)

	floorNormal          = 4.5 // §2.3 normal text
	floorLargeUI         = 3.0 // §2.3 large / bold / UI (and text.dim, §2.9¹)
	floorFillPerceptible = 1.1 // approved fill-perceptibility threshold (NOT 3:1)
)

// relativeLuminance is the WCAG relative luminance of an sRGB hex. go-colorful's
// LinearRgb() is the WCAG sRGB linearization; the weighted sum is the standard
// luminance.
func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	c, err := colorful.Hex(hex)
	if err != nil {
		t.Fatalf("parse hex %q: %v", hex, err)
	}
	r, g, b := c.LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrastRatio is the WCAG contrast ratio (L_lighter+0.05)/(L_darker+0.05).
func contrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	l1 := relativeLuminance(t, fg)
	l2 := relativeLuminance(t, bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

// TestContrastMath validates the WCAG implementation against the canonical
// reference: pure black on pure white is exactly 21:1. Without this anchor a
// silent bug in the luminance math would let every floor assertion below pass
// vacuously.
func TestContrastMath(t *testing.T) {
	got := contrastRatio(t, "#000000", "#FFFFFF")
	if math.Abs(got-21.0) > 0.01 {
		t.Fatalf("black/white contrast = %.4f, want 21.00 (WCAG math is wrong)", got)
	}
	// Identity sanity: a colour against itself is 1:1.
	if got := contrastRatio(t, "#737AA2", "#737AA2"); math.Abs(got-1.0) > 0.001 {
		t.Fatalf("self contrast = %.4f, want 1.00", got)
	}
}

// TestForegroundFloorAgainstOwnCanvas is the core numeric gate. Each foreground
// token is measured against ITS OWN mode-canvas, independently — the dark variant
// vs #0b0c14 and the light variant vs #e1e2e7 — and must clear its per-token
// floor (§2.3 / §2.9). The two variants never need to hold on both canvases.
//
// text.dim is held to the 3:1 large/UI floor (§2.9¹, deliberately de-emphasised).
// text.faint is EXEMPT from the floor (§2.9², decorative-only) and is asserted
// separately in TestTextFaintDecorativeBand.
//
// text.on-selection and text.on-warning are measured against their TINT, not the
// canvas (they only ever render on the band), and are covered by the pair tests.
func TestForegroundFloorAgainstOwnCanvas(t *testing.T) {
	tests := []struct {
		name  string
		token theme.Token
		floor float64
	}{
		// Text ramp.
		{"text.primary", theme.MV.TextPrimary, floorNormal},
		{"text.strong", theme.MV.TextStrong, floorNormal},
		{"text.muted-bright", theme.MV.TextMutedBright, floorNormal},
		{"text.detail", theme.MV.TextDetail, floorNormal},
		{"text.dim", theme.MV.TextDim, floorLargeUI}, // §2.9¹ 3:1
		// Accents.
		{"accent.violet", theme.MV.AccentViolet, floorLargeUI}, // §2.9 floor 3.0
		{"accent.blue", theme.MV.AccentBlue, floorNormal},
		{"accent.cyan", theme.MV.AccentCyan, floorNormal},
		{"state.green", theme.MV.StateGreen, floorNormal},
		{"state.red", theme.MV.StateRed, floorNormal},
		{"accent.orange", theme.MV.AccentOrange, floorNormal},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/dark", func(t *testing.T) {
			got := contrastRatio(t, tt.token.Dark, canvasDark)
			if got < tt.floor {
				t.Errorf("%s dark %s vs %s = %.2f, want >= %.2f (§2.3 floor)",
					tt.name, tt.token.Dark, canvasDark, got, tt.floor)
			}
		})
		t.Run(tt.name+"/light", func(t *testing.T) {
			got := contrastRatio(t, tt.token.Light, canvasLight)
			if got < tt.floor {
				t.Errorf("%s light %s vs %s = %.2f, want >= %.2f (§2.3 floor)",
					tt.name, tt.token.Light, canvasLight, got, tt.floor)
			}
		})
	}
}

// TestTextDimHeldToThreeToOneFloor pins text.dim to the 3:1 large/UI floor (not
// 4.5:1) per §2.9¹ — deliberately de-emphasised but legible. This is a distinct
// assertion so a contributor cannot quietly tighten or loosen text.dim's floor.
func TestTextDimHeldToThreeToOneFloor(t *testing.T) {
	tok := theme.MV.TextDim
	if got := contrastRatio(t, tok.Dark, canvasDark); got < floorLargeUI {
		t.Errorf("text.dim dark = %.2f, want >= %.2f (§2.9¹ 3:1)", got, floorLargeUI)
	}
	if got := contrastRatio(t, tok.Light, canvasLight); got < floorLargeUI {
		t.Errorf("text.dim light = %.2f, want >= %.2f (§2.9¹ 3:1)", got, floorLargeUI)
	}
	// And it must NOT clear the normal-text floor — that would mean it is no
	// longer the de-emphasised step the ramp needs it to be.
	if got := contrastRatio(t, tok.Light, canvasLight); got >= floorNormal {
		t.Errorf("text.dim light = %.2f, unexpectedly >= normal floor %.2f — it is meant to stay de-emphasised (§2.9¹)", got, floorNormal)
	}
}

// TestTextFaintDecorativeBand asserts text.faint is EXEMPT from the contrast
// floor (§2.9²) but stays in the decorative band — visible, yet below the
// functional floor so it can never be promoted to carry functional text. We
// assert both bounds: above 1:1 (perceptible) and strictly below the 3:1 UI
// floor (so it is structurally decorative, never functional).
func TestTextFaintDecorativeBand(t *testing.T) {
	tok := theme.MV.TextFaint
	for _, c := range []struct {
		mode   string
		fg, bg string
	}{
		{"dark", tok.Dark, canvasDark},
		{"light", tok.Light, canvasLight},
	} {
		got := contrastRatio(t, c.fg, c.bg)
		if got <= 1.0 {
			t.Errorf("text.faint %s = %.2f, want > 1.0 (decorative but visible)", c.mode, got)
		}
		if got >= floorLargeUI {
			t.Errorf("text.faint %s = %.2f, want < %.2f — decorative-only, MUST NOT reach the functional UI floor (§2.9²)", c.mode, got, floorLargeUI)
		}
	}
}

// TestBgSelectionPairRule and TestBgWarningPairRule encode the APPROVED tint-pair
// rule (user-approved 2026-06-19), which intentionally DROPS the literal §2.9
// "fill-vs-canvas ≥3:1" requirement. The literal rule is dropped because a subtle
// highlight tint simply cannot meet 3:1 against its own canvas (the fills here are
// ~1.1–1.3); the §2.2 selector/warning ACCENT BAR carries the 3:1 UI distinction
// instead. Each text-carrying tint is verified on three legs:
//
//  1. text-on-tint ≥ its text floor (the on-band text stays legible),
//  2. the tint's accent bar ≥ 3:1 vs canvas (the bar carries the UI distinction),
//  3. the fill is perceptible vs canvas (a low threshold ~1.1, NOT 3:1).
//
// All three legs must clear together.

// TestBgSelectionPairRule verifies the selection band (bg.selection) as a pair:
// text.on-selection on the tint, accent.violet bar vs canvas, and a perceptible
// fill — in both modes, independently against each mode-canvas.
func TestBgSelectionPairRule(t *testing.T) {
	modes := []struct {
		name              string
		canvas            string
		tint, onTint, bar string
	}{
		{
			name:   "dark",
			canvas: canvasDark,
			tint:   theme.MV.BgSelection.Dark,
			onTint: theme.MV.TextOnSelection.Dark,
			bar:    theme.MV.AccentViolet.Dark, // §2.2 selector ▌ bar
		},
		{
			name:   "light",
			canvas: canvasLight,
			tint:   theme.MV.BgSelection.Light,
			onTint: theme.MV.TextOnSelection.Light,
			bar:    theme.MV.AccentViolet.Light,
		},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			// Leg 1: text-on-tint ≥ text floor.
			if got := contrastRatio(t, m.onTint, m.tint); got < floorNormal {
				t.Errorf("text.on-selection %s vs bg.selection = %.2f, want >= %.2f (leg 1: text-on-tint)", m.name, got, floorNormal)
			}
			// Leg 2: accent bar ≥ 3:1 vs canvas (carries the UI distinction).
			if got := contrastRatio(t, m.bar, m.canvas); got < floorLargeUI {
				t.Errorf("accent.violet bar %s vs canvas = %.2f, want >= %.2f (leg 2: bar carries 3:1)", m.name, got, floorLargeUI)
			}
			// Leg 3: fill perceptible vs canvas (low threshold; literal 3:1 dropped).
			if got := contrastRatio(t, m.tint, m.canvas); got < floorFillPerceptible {
				t.Errorf("bg.selection fill %s vs canvas = %.2f, want >= %.2f (leg 3: perceptible, NOT 3:1)", m.name, got, floorFillPerceptible)
			}
		})
	}
}

// TestBgWarningPairRule verifies the warning band (bg.warning) as a pair, same
// three legs as bg.selection but with text.on-warning and the accent.orange bar.
//
// The LIGHT bg.warning value (#E8D6A8) is PINNED at the 1-9 gate (§2.9 / §15.6):
// the 1-4 derived value held — it clears all three legs against the light canvas,
// so no more-contrast remedy was needed. Both modes' legs are firm.
func TestBgWarningPairRule(t *testing.T) {
	modes := []struct {
		name              string
		canvas            string
		tint, onTint, bar string
	}{
		{
			name:   "dark",
			canvas: canvasDark,
			tint:   theme.MV.BgWarning.Dark,
			onTint: theme.MV.TextOnWarning.Dark,
			bar:    theme.MV.AccentOrange.Dark, // §2.2 warning ⚠ left-bar
		},
		{
			name:   "light",
			canvas: canvasLight,
			tint:   theme.MV.BgWarning.Light,
			onTint: theme.MV.TextOnWarning.Light,
			bar:    theme.MV.AccentOrange.Light,
		},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			// Leg 1: text-on-tint ≥ text floor.
			if got := contrastRatio(t, m.onTint, m.tint); got < floorNormal {
				t.Errorf("text.on-warning %s vs bg.warning = %.2f, want >= %.2f (leg 1: text-on-tint)", m.name, got, floorNormal)
			}
			// Leg 2: accent bar ≥ 3:1 vs canvas.
			if got := contrastRatio(t, m.bar, m.canvas); got < floorLargeUI {
				t.Errorf("accent.orange bar %s vs canvas = %.2f, want >= %.2f (leg 2: bar carries 3:1)", m.name, got, floorLargeUI)
			}
			// Leg 3: fill perceptible vs canvas.
			if got := contrastRatio(t, m.tint, m.canvas); got < floorFillPerceptible {
				t.Errorf("bg.warning fill %s vs canvas = %.2f, want >= %.2f (leg 3: perceptible, NOT 3:1)", m.name, got, floorFillPerceptible)
			}
		})
	}
}

// TestInlineFlashWarningPairClearsFloor is the §11.2 inline-flash band's
// co-tuned-pair gate: the warning/success flash renders its message in
// text.on-warning ON the bg.warning tint, so that exact foreground-on-tint pair
// must clear the §2.3 normal-text floor (4.5:1) in BOTH modes, each against its
// own mode-tint. The flash bands reuse the single co-tuned text.on-warning /
// bg.warning pairing (§2.9 — no invented success-specific pairing), so this is
// the same numeric floor the band relies on at render time. It is asserted here
// distinctly (not just via the general pair rule) so a regression that breaks the
// inline-flash band specifically fails with a §11.2-scoped message.
func TestInlineFlashWarningPairClearsFloor(t *testing.T) {
	for _, m := range []struct {
		name         string
		onTint, tint string
	}{
		{"dark", theme.MV.TextOnWarning.Dark, theme.MV.BgWarning.Dark},
		{"light", theme.MV.TextOnWarning.Light, theme.MV.BgWarning.Light},
	} {
		t.Run(m.name, func(t *testing.T) {
			if got := contrastRatio(t, m.onTint, m.tint); got < floorNormal {
				t.Errorf("§11.2 inline-flash message text.on-warning %s vs bg.warning %s = %.2f, want >= %.2f (co-tuned pair floor)",
					m.onTint, m.tint, got, floorNormal)
			}
		})
	}
}

// TestEveryTokenHasLightVariant proves the §2.9 light column is fully populated:
// every token in the closed vocabulary carries a non-empty Light hex. (The DARK
// column is pinned by TestMVDarkVariantsPinned in theme_test.go.) This is the
// "every §2.9 token carries a light variant" acceptance criterion.
func TestEveryTokenHasLightVariant(t *testing.T) {
	for _, tok := range theme.MV.All() {
		if tok.Light == "" {
			t.Errorf("token %q has empty Light variant — every §2.9 token must carry a light value", tok.Name)
			continue
		}
		if _, err := colorful.Hex(tok.Light); err != nil {
			t.Errorf("token %q light %q is not a valid hex: %v", tok.Name, tok.Light, err)
		}
	}
}

// TestLightSurfaceTintsPinned pins the four light surface tints to their
// CONCRETE 1-9-locked hexes. Task 1-4 left these provisional; the 1-9 in-terminal
// validation gate pins each (derived from its dark anchor + the surface it renders
// on — §2.9 / §15.6) and the human eyeball confirms it against `#e1e2e7`. This
// test guards the pinned hexes so a later change is a deliberate, visible edit;
// the numeric legs each tint must clear are asserted by the pair tests below and
// TestLightTintFillsArePerceptible.
func TestLightSurfaceTintsPinned(t *testing.T) {
	pinned := []struct {
		name string
		got  string
		want string
	}{
		// bg.selection: confirmed at the 1-9 gate (derivation: dark violet anchor
		// #28243a lifted onto the light canvas #e1e2e7).
		{"bg.selection light", theme.MV.BgSelection.Light, "#D0C6F0"},
		// bg.warning: pinned at 1-9 (derivation: dark amber anchor #241B10 + light
		// canvas #e1e2e7; the 1-4 derived value held — it clears perceptible + the
		// on-warning leg, so no more-contrast remedy was needed).
		{"bg.warning light", theme.MV.BgWarning.Light, "#E8D6A8"},
		// bg.track: pinned at 1-9 (derivation: dark grey anchor #26283A + light
		// canvas #e1e2e7; the 1-4 derived value held — it clears perceptible and
		// reads as a distinct empty-track surface, so no remedy was needed).
		{"bg.track light", theme.MV.BgTrack.Light, "#D2D4DE"},
		// borders: confirmed at the 1-9 gate (shared separator/footer light rule).
		{"border.separator light", theme.MV.BorderSeparator.Light, "#C9CDDB"},
		{"border.footer light", theme.MV.BorderFooter.Light, "#C9CDDB"},
	}
	for _, p := range pinned {
		t.Run(p.name, func(t *testing.T) {
			if p.got != p.want {
				t.Errorf("%s = %q, want pinned %q (1-9 lock-in)", p.name, p.got, p.want)
			}
		})
	}
}

// TestLightTintFillsArePerceptible asserts every light surface tint reads as a
// perceptible surface against the light canvas `#e1e2e7` — the leg-3 fill check
// of the approved pair rule (≥1.1, NOT 3:1; a subtle highlight tint cannot meet
// 3:1 against its own canvas, §2.2's accent bar carries the UI distinction). This
// is the numeric floor for the light-tint-on-light-canvas case; the in-terminal
// eyeball (1-9) is the visual lock layered on top (a numeric pass alone is
// insufficient, §2.9 / §15.6).
func TestLightTintFillsArePerceptible(t *testing.T) {
	tints := []struct {
		name string
		hex  string
	}{
		{"bg.selection", theme.MV.BgSelection.Light},
		{"bg.warning", theme.MV.BgWarning.Light},
		{"bg.track", theme.MV.BgTrack.Light},
		{"border.separator", theme.MV.BorderSeparator.Light},
		{"border.footer", theme.MV.BorderFooter.Light},
	}
	for _, tnt := range tints {
		t.Run(tnt.name, func(t *testing.T) {
			if got := contrastRatio(t, tnt.hex, canvasLight); got < floorFillPerceptible {
				t.Errorf("%s light %s vs %s = %.2f, want >= %.2f (perceptible surface, NOT a wash-out)",
					tnt.name, tnt.hex, canvasLight, got, floorFillPerceptible)
			}
		})
	}
}

// TestBgTrackPairRule pins the now-concrete light bg.track value as a perceptible
// surface in both modes. bg.track is the loading-bar EMPTY track — it carries no
// on-band text (the filled portion uses accent.violet / the bar token, not text),
// so it is a single-leg fill-perceptibility check, not the three-leg text-pair
// rule. The dark anchor (#26283A) and the now-concrete light value (#D2D4DE) each
// clear the perceptible floor against their own canvas.
func TestBgTrackPairRule(t *testing.T) {
	modes := []struct {
		name   string
		tint   string
		canvas string
	}{
		{"dark", theme.MV.BgTrack.Dark, canvasDark},
		{"light", theme.MV.BgTrack.Light, canvasLight},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			if got := contrastRatio(t, m.tint, m.canvas); got < floorFillPerceptible {
				t.Errorf("bg.track fill %s vs canvas = %.2f, want >= %.2f (perceptible empty track, NOT 3:1)",
					m.tint, got, floorFillPerceptible)
			}
		})
	}
}

// TestForegroundOnTintPairings is the §4.1 foreground-on-tint gate: EVERY
// selected-row foreground (name text.on-selection, count text.strong, attached
// bullet state.green) measured against bg.selection, plus text.on-warning against
// bg.warning — in BOTH modes, each against the relevant tint (not the canvas).
// These are the pairings §4.1 / §2.9 require verified against the tints in ADDITION
// to the §2.3 canvas gate.
//
// Floor classification (§2.3): all are functional NORMAL TEXT → 4.5:1. The
// `● attached` marker keeps the SINGLE state.green token on the selected row; its
// light value was darkened to #3B5E18 so it clears 4.5 on bg.selection (#D0C6F0 =
// 4.65) as well as the canvas — folding the former on-selection override into the
// global token (no per-context green). Dark #9ECE6A clears 8.19 on dark bg.selection.
func TestForegroundOnTintPairings(t *testing.T) {
	pairings := []struct {
		name   string
		fg     theme.Token
		tint   theme.Token
		floor  float64
		darkFG string // for clarity in failure messages only
	}{
		{"text.on-selection on bg.selection", theme.MV.TextOnSelection, theme.MV.BgSelection, floorNormal, ""},
		{"text.strong on bg.selection", theme.MV.TextStrong, theme.MV.BgSelection, floorNormal, ""},
		// §6.2 selected-row PATH: text.muted-bright on bg.selection (light darkened to
		// #4C5478 to clear the floor — it was 4.17 in light).
		{"text.muted-bright on bg.selection", theme.MV.TextMutedBright, theme.MV.BgSelection, floorNormal, ""},
		// §4.1 attached marker: the single state.green (darkened light #3B5E18) is
		// held to the 4.5 normal-text floor on the tint.
		{"state.green on bg.selection", theme.MV.StateGreen, theme.MV.BgSelection, floorNormal, ""},
		{"text.on-warning on bg.warning", theme.MV.TextOnWarning, theme.MV.BgWarning, floorNormal, ""},
	}
	for _, p := range pairings {
		t.Run(p.name+"/dark", func(t *testing.T) {
			if got := contrastRatio(t, p.fg.Dark, p.tint.Dark); got < p.floor {
				t.Errorf("%s dark %s vs %s = %.2f, want >= %.2f (§4.1 foreground-on-tint)",
					p.name, p.fg.Dark, p.tint.Dark, got, p.floor)
			}
		})
		t.Run(p.name+"/light", func(t *testing.T) {
			if got := contrastRatio(t, p.fg.Light, p.tint.Light); got < p.floor {
				t.Errorf("%s light %s vs %s = %.2f, want >= %.2f (§4.1 foreground-on-tint)",
					p.name, p.fg.Light, p.tint.Light, got, p.floor)
			}
		})
	}
}

// TestStateGreenClearsCanvasAndSelection is the gate that justifies the SINGLE
// state.green token (no per-context on-selection override): the light value
// (#3B5E18) MUST clear the 4.5:1 normal-text floor against BOTH the canvas
// (#e1e2e7) AND the bg.selection tint (#D0C6F0). The former on-selection override
// existed only because the prior #456E1C washed out on bg.selection (3.72);
// darkening the global token to #3B5E18 clears both surfaces, so the special-case
// token was removed. The dark variant (#9ECE6A) clears both dark surfaces.
func TestStateGreenClearsCanvasAndSelection(t *testing.T) {
	g := theme.MV.StateGreen
	for _, c := range []struct {
		what string
		fg   string
		bg   string
	}{
		{"light vs canvas", g.Light, theme.MV.Canvas.Light},
		{"light vs bg.selection", g.Light, theme.MV.BgSelection.Light},
		{"dark vs canvas", g.Dark, theme.MV.Canvas.Dark},
		{"dark vs bg.selection", g.Dark, theme.MV.BgSelection.Dark},
	} {
		if got := contrastRatio(t, c.fg, c.bg); got < floorNormal {
			t.Errorf("state.green %s (%s vs %s) = %.2f, want >= %.2f (single token must clear canvas AND selection)",
				c.what, c.fg, c.bg, got, floorNormal)
		}
	}
	// The folded-in light value is the darkened #3B5E18 (the value that clears
	// bg.selection); a regression back to #456E1C would re-introduce the wash-out.
	if theme.MV.StateGreen.Light != "#3B5E18" {
		t.Errorf("state.green light = %q, want %q (darkened so the single token clears bg.selection)", theme.MV.StateGreen.Light, "#3B5E18")
	}
}
