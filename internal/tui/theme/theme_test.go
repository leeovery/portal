package theme_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/tui/theme"
)

// TestMVDarkVariantsPinned asserts every §2.9 token carries its exact pinned
// DARK hex. The §2.9 token table is the source of truth for these numbers (not
// the Paper PNG, not the old screen). A drift here means a renderer paints the
// wrong hue.
func TestMVDarkVariantsPinned(t *testing.T) {
	tests := []struct {
		name  string
		token theme.Token
		want  string
	}{
		// Text ramp.
		{"text.primary", theme.MV.TextPrimary, "#C0CAF5"},
		{"text.strong", theme.MV.TextStrong, "#A9B1D6"},
		{"text.muted-bright", theme.MV.TextMutedBright, "#828BB8"},
		{"text.detail", theme.MV.TextDetail, "#737AA2"},
		{"text.dim", theme.MV.TextDim, "#535C86"},
		{"text.faint", theme.MV.TextFaint, "#3B4261"},
		{"text.on-selection", theme.MV.TextOnSelection, "#FFFFFF"},
		// Accents.
		{"accent.violet", theme.MV.AccentViolet, "#BB9AF7"},
		{"accent.blue", theme.MV.AccentBlue, "#7AA2F7"},
		{"accent.cyan", theme.MV.AccentCyan, "#7DCFFF"},
		{"state.green", theme.MV.StateGreen, "#9ECE6A"},
		{"state.red", theme.MV.StateRed, "#F7768E"},
		{"accent.orange", theme.MV.AccentOrange, "#FF9E64"},
		// Surfaces.
		{"canvas", theme.MV.Canvas, "#0b0c14"},
		{"bg.selection", theme.MV.BgSelection, "#28243a"},
		{"bg.warning", theme.MV.BgWarning, "#241B10"},
		{"bg.track", theme.MV.BgTrack, "#26283A"},
		{"border.separator", theme.MV.BorderSeparator, "#292E42"},
		{"border.footer", theme.MV.BorderFooter, "#20232E"},
		{"text.on-warning", theme.MV.TextOnWarning, "#E8C9A0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.token.Dark != tt.want {
				t.Errorf("%s dark = %q, want %q", tt.name, tt.token.Dark, tt.want)
			}
		})
	}
}

// TestMVTokenCount pins the closed ~20-token vocabulary so a contributor cannot
// silently grow the palette without amending the spec (§2.8 / §2.9: "closed set
// of ~20 named tokens"). The table above and this count are kept in lock-step:
// 7 text-ramp + 6 accents + 7 surfaces = 20.
func TestMVTokenCount(t *testing.T) {
	const wantTokens = 20
	if got := len(theme.MV.All()); got != wantTokens {
		t.Errorf("MV token count = %d, want %d (closed §2.9 vocabulary)", got, wantTokens)
	}
}

// TestEachTokenCarriesLightVariant proves the token representation already holds
// a settable Light slot so task 1-4 can fill the light values WITHOUT
// re-pointing any call site. Light is a placeholder for now (1-4 owns the real
// hexes); the structure must already carry both variants. The proof: writing a
// Light value and resolving in light mode yields it, with the Dark resolution
// left untouched — so 1-4 fills Light and flips the resolver without editing a
// single renderer.
func TestEachTokenCarriesLightVariant(t *testing.T) {
	tok := theme.MV.AccentViolet

	// 1-4 will fill Light. Simulate that fill on a copy and confirm it resolves
	// independently of Dark — the field is the seam, no call-site re-point.
	tok.Light = "#8A3FD1"

	if got, want := tok.ColorFor(theme.Light), lipgloss.Color(tok.Light); got != want {
		t.Errorf("ColorFor(Light) = %v, want filled light variant %v", got, want)
	}
	if got, want := tok.ColorFor(theme.Dark), lipgloss.Color(tok.Dark); got != want {
		t.Errorf("ColorFor(Dark) = %v, want dark variant %v (unaffected by Light fill)", got, want)
	}
}
