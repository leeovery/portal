package theme_test

import (
	"maps"
	"math"
	"slices"
	"testing"

	"github.com/leeovery/portal/internal/theme"
	"github.com/lucasb-eyer/go-colorful"
)

// WCAG 2.x contrast floors: normal text, large/bold/UI, and a
// fill-perceptible tint (deliberately not 3:1).
const (
	floorNormal          = 4.5
	floorLargeUI         = 3.0
	floorFillPerceptible = 1.10

	ratioIdentity = 1.0 // no contrast at all — text.faint's lower bound, not a floor
)

// WCAG relative luminance: go-colorful's LinearRgb applies the sRGB
// linearisation WCAG specifies, then the standard weighted sum.
func relativeLuminance(t *testing.T, hex string) float64 {
	t.Helper()
	c, err := colorful.Hex(hex)
	if err != nil {
		t.Fatalf("parse hex %q: %v", hex, err)
	}
	r, g, b := c.LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// WCAG contrast ratio: (L_lighter+0.05)/(L_darker+0.05).
func contrastRatio(t *testing.T, fg, bg string) float64 {
	t.Helper()
	l1 := relativeLuminance(t, fg)
	l2 := relativeLuminance(t, bg)
	if l1 < l2 {
		l1, l2 = l2, l1
	}
	return (l1 + 0.05) / (l2 + 0.05)
}

func TestContrastMath(t *testing.T) {
	got := contrastRatio(t, "#000000", "#FFFFFF")
	if math.Abs(got-21.0) > 0.01 {
		t.Fatalf("black/white contrast = %.4f, want 21.00 (WCAG math is wrong)", got)
	}
	if got := contrastRatio(t, "#737AA2", "#737AA2"); math.Abs(got-1.0) > 0.001 {
		t.Fatalf("self contrast = %.4f, want 1.00", got)
	}
}

func TestFloorsEnumerateTheEmbeddedSet(t *testing.T) {
	enrolled := slices.Sorted(maps.Keys(embeddedThemes(t)))

	if len(enrolled) == 0 {
		t.Fatal("no embedded theme is enrolled — every floor assertion in this file would pass vacuously")
	}
	if want := committedBuiltinSlugs(t); !slices.Equal(enrolled, want) {
		t.Errorf("the floor tests enrol %v, want every built-in committed to builtins/ %v", enrolled, want)
	}
}

func TestForegroundFloorAgainstOwnCanvas(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		rules := []struct {
			token string
			value string
			floor float64
		}{
			{"text.primary", th.TextPrimary.Value, floorNormal},
			{"text.secondary", th.TextSecondary.Value, floorNormal},
			{"text.tertiary", th.TextTertiary.Value, floorNormal},
			{"text.muted", th.TextMuted.Value, floorNormal},
			{"accent.primary", th.AccentPrimary.Value, floorLargeUI},
			{"accent.key", th.AccentKey.Value, floorNormal},
			{"accent.mode", th.AccentMode.Value, floorNormal},
			{"accent.attention", th.AccentAttention.Value, floorNormal},
			{"state.positive", th.StatePositive.Value, floorNormal},
			{"state.destructive", th.StateDestructive.Value, floorNormal},
		}

		for _, rule := range rules {
			t.Run(slug+"/"+rule.token, func(t *testing.T) {
				assertAtLeast(t, slug+" "+rule.token+" vs canvas", rule.value, th.Canvas.Value, rule.floor)
			})
		}
	}
}

func TestTextSubtleBand(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/text.subtle", func(t *testing.T) {
			leg := slug + " text.subtle vs canvas"
			assertAtLeast(t, leg, th.TextSubtle.Value, th.Canvas.Value, floorLargeUI)
			assertBelow(t, leg, th.TextSubtle.Value, th.Canvas.Value, floorNormal)
		})
	}
}

func TestTextFaintDecorativeBand(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/text.faint", func(t *testing.T) {
			leg := slug + " text.faint vs canvas"
			assertAbove(t, leg, th.TextFaint.Value, th.Canvas.Value, ratioIdentity)
			assertBelow(t, leg, th.TextFaint.Value, th.Canvas.Value, floorLargeUI)
		})
	}
}

func TestBgSelectionPairRule(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/bg.selection", func(t *testing.T) {
			assertAtLeast(t, slug+" text.on-selection on bg.selection", th.TextOnSelection.Value, th.BgSelection.Value, floorNormal)
			assertAtLeast(t, slug+" accent.primary bar vs canvas", th.AccentPrimary.Value, th.Canvas.Value, floorLargeUI)
			assertAtLeast(t, slug+" bg.selection fill vs canvas", th.BgSelection.Value, th.Canvas.Value, floorFillPerceptible)
		})
	}
}

func TestBgAttentionPairRule(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/bg.attention", func(t *testing.T) {
			assertAtLeast(t, slug+" text.on-attention on bg.attention", th.TextOnAttention.Value, th.BgAttention.Value, floorNormal)
			assertAtLeast(t, slug+" accent.attention bar vs canvas", th.AccentAttention.Value, th.Canvas.Value, floorLargeUI)
			assertAtLeast(t, slug+" bg.attention fill vs canvas", th.BgAttention.Value, th.Canvas.Value, floorFillPerceptible)
		})
	}
}

func TestInlineFlashAttentionPairClearsFloor(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/text.on-attention", func(t *testing.T) {
			assertAtLeast(t, slug+" inline-flash message text.on-attention on bg.attention",
				th.TextOnAttention.Value, th.BgAttention.Value, floorNormal)
		})
	}
}

func TestPreviewPeekChromeClearsFloorAgainstCanvas(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		chrome := []struct {
			token string
			role  string
			value string
		}{
			{"accent.mode", "frame + marker", th.AccentMode.Value},
			{"text.primary", "session name", th.TextPrimary.Value},
			{"text.muted", "counters + hints", th.TextMuted.Value},
		}

		for _, part := range chrome {
			t.Run(slug+"/"+part.token, func(t *testing.T) {
				assertAtLeast(t, slug+" preview chrome "+part.role+" ("+part.token+") vs canvas",
					part.value, th.Canvas.Value, floorNormal)
			})
		}
	}
}

func TestBgSubtlePairRule(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/bg.subtle", func(t *testing.T) {
			assertAtLeast(t, slug+" bg.subtle fill vs canvas", th.BgSubtle.Value, th.Canvas.Value, floorFillPerceptible)
		})
	}
}

func TestForegroundOnTintPairings(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		for _, pair := range foregroundOnTintPairings(themes[slug]) {
			t.Run(slug+"/"+pair.foreground+" on "+pair.tint, func(t *testing.T) {
				assertAtLeast(t, slug+" "+pair.foreground+" on "+pair.tint, pair.fg, pair.bg, pair.floor)
			})
		}
	}
}

// accent.attention takes the non-text floor on bg.selection because the
// session row's pending dot is a graphical indicator, not text.
func TestForegroundOnTintPairingFloors(t *testing.T) {
	want := map[string]float64{
		"text.on-selection on bg.selection": floorNormal,
		"text.secondary on bg.selection":    floorNormal,
		"text.tertiary on bg.selection":     floorNormal,
		"state.positive on bg.selection":    floorNormal,
		"accent.attention on bg.selection":  floorLargeUI,
		"text.on-attention on bg.attention": floorNormal,
	}

	got := make(map[string]float64)
	for _, pair := range foregroundOnTintPairings(theme.Theme{}) {
		got[pair.foreground+" on "+pair.tint] = pair.floor
	}
	if !maps.Equal(got, want) {
		t.Errorf("foreground-on-tint floors = %v, want %v", got, want)
	}
}

type tintPairing struct {
	foreground string
	tint       string
	fg, bg     string
	floor      float64
}

func foregroundOnTintPairings(th theme.Theme) []tintPairing {
	return []tintPairing{
		{"text.on-selection", "bg.selection", th.TextOnSelection.Value, th.BgSelection.Value, floorNormal},
		{"text.secondary", "bg.selection", th.TextSecondary.Value, th.BgSelection.Value, floorNormal},
		{"text.tertiary", "bg.selection", th.TextTertiary.Value, th.BgSelection.Value, floorNormal},
		{"state.positive", "bg.selection", th.StatePositive.Value, th.BgSelection.Value, floorNormal},
		{"accent.attention", "bg.selection", th.AccentAttention.Value, th.BgSelection.Value, floorLargeUI},
		{"text.on-attention", "bg.attention", th.TextOnAttention.Value, th.BgAttention.Value, floorNormal},
	}
}

func TestStatePositiveClearsCanvasAndSelection(t *testing.T) {
	themes := embeddedThemes(t)
	for _, slug := range slices.Sorted(maps.Keys(themes)) {
		th := themes[slug]
		t.Run(slug+"/state.positive", func(t *testing.T) {
			assertAtLeast(t, slug+" state.positive vs canvas", th.StatePositive.Value, th.Canvas.Value, floorNormal)
			assertAtLeast(t, slug+" state.positive on bg.selection", th.StatePositive.Value, th.BgSelection.Value, floorNormal)
		})
	}
}

func embeddedThemes(t *testing.T) map[string]theme.Theme {
	t.Helper()

	slugs := theme.BuiltinSlugs()
	if len(slugs) == 0 {
		t.Fatal("the embedded set is empty — every floor assertion in this file would pass vacuously")
	}

	loader := theme.NewSilentLoader()
	themes := make(map[string]theme.Theme, len(slugs))
	for _, slug := range slugs {
		result, rejection, found := loader.LoadBuiltin(slug)
		if !found {
			t.Fatalf("LoadBuiltin(%q) reported not found, want the embedded built-in", slug)
		}
		if rejection != nil {
			t.Fatalf("LoadBuiltin(%q) rejected the embedded file: %v", slug, rejection)
		}
		themes[slug] = result.Theme
	}
	return themes
}

func assertAtLeast(t *testing.T, leg, fg, bg string, floor float64) {
	t.Helper()
	if got := contrastRatio(t, fg, bg); got < floor {
		t.Errorf("%s: %s on %s = %.4f, want >= %.2f", leg, fg, bg, got, floor)
	}
}

func assertAbove(t *testing.T, leg, fg, bg string, floor float64) {
	t.Helper()
	if got := contrastRatio(t, fg, bg); got <= floor {
		t.Errorf("%s: %s on %s = %.4f, want > %.2f", leg, fg, bg, got, floor)
	}
}

func assertBelow(t *testing.T, leg, fg, bg string, ceiling float64) {
	t.Helper()
	if got := contrastRatio(t, fg, bg); got >= ceiling {
		t.Errorf("%s: %s on %s = %.4f, want < %.2f", leg, fg, bg, got, ceiling)
	}
}
