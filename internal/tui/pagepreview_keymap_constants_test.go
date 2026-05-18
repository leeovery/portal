package tui

import "testing"

// These tests pin the exact byte content of the package-level keymap constants
// and the hex codes of the previewBorderColor AdaptiveColor variable per
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
	if previewBorderColor.Light != "#3B5577" {
		t.Errorf("previewBorderColor.Light = %q, want %q", previewBorderColor.Light, "#3B5577")
	}
	if previewBorderColor.Dark != "#7B95BD" {
		t.Errorf("previewBorderColor.Dark = %q, want %q", previewBorderColor.Dark, "#7B95BD")
	}
}
