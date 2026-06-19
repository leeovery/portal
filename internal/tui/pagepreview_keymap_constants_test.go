package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// These tests pin the exact byte content of the package-level keymap constants
// and the hex codes of the preview-frame border colour variants per
// specification.md § Keymap glyphs > Constants, § Border colour, and
// § Style sourcing. Drift is caught loudly — the spec is the source of truth
// and any change to these literals must be a deliberate spec update.

func TestVerboseKeymapExactByteContent(t *testing.T) {
	want := "] next win · [ prev win · ⇥ next pane · ⏎ attach · ⎋ back"
	if verboseKeymap != want {
		t.Errorf("verboseKeymap = %q, want %q", verboseKeymap, want)
	}
}

func TestCompactKeymapExactByteContent(t *testing.T) {
	want := "] [ ⇥ ⏎ ⎋"
	if compactKeymap != want {
		t.Errorf("compactKeymap = %q, want %q", compactKeymap, want)
	}
}

func TestCompactKeymapSingleSpaceSeparatedNoInterpuncts(t *testing.T) {
	// Per spec § Compact form: single-space separators (no interpunct).
	// Display-cell width is 9 cells. Asserting absence of the U+00B7
	// interpunct (the verbose form's separator) prevents accidental
	// reintroduction of the verbose separator into the compact form.
	for _, r := range compactKeymap {
		if r == '·' {
			t.Errorf("compactKeymap contains interpunct U+00B7; want single-space separators only: %q", compactKeymap)
		}
	}
}

func TestPreviewBorderColorHexCodes(t *testing.T) {
	// Lipgloss v2 removed AdaptiveColor (spec § 14.5); the light/dark variants
	// are now explicit named constants. Pin both exact hexes — unchanged from
	// the v1 AdaptiveColor{Light, Dark} — so the migration stays parity-only.
	if previewBorderColorLight != "#3B5577" {
		t.Errorf("previewBorderColorLight = %q, want %q", previewBorderColorLight, "#3B5577")
	}
	if previewBorderColorDark != "#7B95BD" {
		t.Errorf("previewBorderColorDark = %q, want %q", previewBorderColorDark, "#7B95BD")
	}
	// The resolved previewBorderColor must equal the DARK variant — the
	// dark-default resolution that v1's AdaptiveColor produced in the absence
	// of a detected light terminal background (OSC 11 light/dark detection is
	// wired by task 1-7). lipgloss.Color of the dark hex is the expected value.
	if want := lipgloss.Color(previewBorderColorDark); previewBorderColor != want {
		t.Errorf("previewBorderColor = %#v, want dark variant %#v", previewBorderColor, want)
	}
}
