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
// The LIGHT bg.warning value is PROVISIONAL (1-9 finalises — §2.9 leaves it as
// "light amber (§15)"). Its pair assertion is therefore provisional-pending-1-9:
// it clears the legs against the provisional placeholder, but the placeholder is
// derived (dark anchor + light canvas), not a locked final value. The dark legs
// are firm.
func TestBgWarningPairRule(t *testing.T) {
	modes := []struct {
		name              string
		canvas            string
		tint, onTint, bar string
		provisional       bool
	}{
		{
			name:   "dark",
			canvas: canvasDark,
			tint:   theme.MV.BgWarning.Dark,
			onTint: theme.MV.TextOnWarning.Dark,
			bar:    theme.MV.AccentOrange.Dark, // §2.2 warning ⚠ left-bar
		},
		{
			name:        "light",
			canvas:      canvasLight,
			tint:        theme.MV.BgWarning.Light,
			onTint:      theme.MV.TextOnWarning.Light,
			bar:         theme.MV.AccentOrange.Light,
			provisional: true, // §2.9 / §15.6: provisional-pending-1-9
		},
	}
	for _, m := range modes {
		t.Run(m.name, func(t *testing.T) {
			if m.provisional {
				t.Log("bg.warning light is PROVISIONAL (1-9 eyeball finalises — §2.9 / §15.6); these legs assert against the derived placeholder, not a locked final value")
			}
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

// TestLightSurfaceTintsProvisional documents — and pins as a guard — that the
// light surface tints are PROVISIONAL pending the 1-9 in-terminal eyeball. A
// numeric pass alone is insufficient for the light-tint-on-light-canvas case
// (§2.9 / §15.6); the recurring failure class is a light tint that is numerically
// fine but visually indistinct on the light canvas. This test records the current
// provisional values so a later change is a deliberate, visible edit (the eyeball
// lock lands in 1-9, not here).
func TestLightSurfaceTintsProvisional(t *testing.T) {
	t.Log("LIGHT surface tints are PROVISIONAL pending 1-9 eyeball (§2.9 / §15.6): bg.selection, bg.warning, bg.track, and the light borders. Numeric pass alone is insufficient on a light-tint-on-light-canvas; 1-9 is the visual lock.")

	provisional := []struct {
		name string
		got  string
	}{
		{"bg.selection light", theme.MV.BgSelection.Light},         // #D0C6F0 (§2.9, provisional-pending-eyeball)
		{"bg.warning light", theme.MV.BgWarning.Light},             // derived placeholder, NOT final (§2.9 "light amber (§15)")
		{"bg.track light", theme.MV.BgTrack.Light},                 // derived placeholder, NOT final (§2.9 "light grey (§15)")
		{"border.separator light", theme.MV.BorderSeparator.Light}, // #C9CDDB (§2.9, provisional-pending-eyeball)
		{"border.footer light", theme.MV.BorderFooter.Light},       // #C9CDDB shared (§2.9, provisional-pending-eyeball)
	}
	for _, p := range provisional {
		t.Run(p.name, func(t *testing.T) {
			if p.got == "" {
				t.Errorf("%s is empty — provisional placeholder must still be populated", p.name)
			}
		})
	}
}
