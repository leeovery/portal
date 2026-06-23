package tui

import (
	"strings"
	"testing"

	"github.com/leeovery/portal/internal/tui/theme"
)

// These tests pin the §9.1 peek-mode marker, the §9.3 footer nav-hint content
// (derived from the previewKeymap descriptor), and the role token the preview
// chrome resolves. Drift is caught loudly — the spec is the source of truth and
// any change to these literals must be a deliberate spec update.

// TestPreviewFooterCanonicalByteContent pins the §9.1 footer's exact stripped
// content: `←→ window  ⇥ pane  ⏎ attach  ␣ back` — glyphs + labels,
// space-separated (no middots), in descriptor order.
func TestPreviewFooterCanonicalByteContent(t *testing.T) {
	const want = "←→ window  ⇥ pane  ⏎ attach  ␣ back"
	got := stripANSI(composePreviewFooterRow(200, theme.Dark, false))
	if got != want {
		t.Errorf("preview footer = %q, want %q", got, want)
	}
}

// TestPreviewFooterNoMiddots pins the shared footer convention: the §9.1 footer
// is SPACE-separated, never middot-separated (the verbose-bar middots were
// dropped in the §9 restructure).
func TestPreviewFooterNoMiddots(t *testing.T) {
	got := stripANSI(composePreviewFooterRow(200, theme.Dark, false))
	if strings.ContainsRune(got, '·') {
		t.Errorf("preview footer contains a middot U+00B7; want space-separated only: %q", got)
	}
}

// TestPreviewFooterCompactGlyphsOnly pins the narrow-width cascade: when the
// labelled form does not fit, the footer compacts to accent.blue glyphs only,
// dropping the labels but keeping every nav-hint glyph present.
func TestPreviewFooterCompactGlyphsOnly(t *testing.T) {
	// A content width too narrow for the labelled form (~38 cells) but wide
	// enough for the full compact glyph form (13 cells) forces the compact path.
	got := stripANSI(composePreviewFooterRow(20, theme.Dark, false))
	const want = "←→  ⇥  ⏎  ␣"
	if got != want {
		t.Errorf("compact preview footer = %q, want %q", got, want)
	}
	for _, label := range []string{"window", "pane", "attach", "back"} {
		if strings.Contains(got, label) {
			t.Errorf("compact preview footer must drop labels; found %q in %q", label, got)
		}
	}
}

func TestPreviewMarkerExactByteContent(t *testing.T) {
	want := "◉ preview"
	if previewMarker != want {
		t.Errorf("previewMarker = %q, want %q", previewMarker, want)
	}
}

// TestPreviewBorderColorPointsAtAccentCyanToken pins that the §9.1 preview
// chrome border resolves the accent.cyan §2.9 role token — NOT a raw light/dark
// hex pair. The former explicit previewBorderColorDark `#7B95BD` is retired in
// favour of the token (§2.9), so the cyan "peek mode" hue is owned by the single
// token layer.
func TestPreviewBorderColorPointsAtAccentCyanToken(t *testing.T) {
	if previewBorderColorToken != theme.MV.AccentCyan {
		t.Errorf("previewBorderColorToken = %#v, want theme.MV.AccentCyan %#v", previewBorderColorToken, theme.MV.AccentCyan)
	}
	// And the token's dark variant must NOT be the retired explicit hex — a
	// regression back to #7B95BD would mean the re-target was reverted.
	if previewBorderColorToken.Dark == "#7B95BD" {
		t.Errorf("previewBorderColorToken.Dark = %q; the retired explicit border hex must not survive", previewBorderColorToken.Dark)
	}
}
