package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/tui/theme"
)

// The golden strings below were CAPTURED from the PRE-refactor render functions
// (killModalKeyHint / editFooterGroup / *ModalFooterRow / previewFooterHint) before
// the shared renderKeyHint / renderConfirmCancelFooter helpers existed. They pin the
// exact rendered bytes — SGR colour runs (accent.blue key, text.detail label), the
// owned canvas background on every cell, and the canvas-painted gaps — so the
// refactor is proven byte-identical in both modes AND under the colourless carve-out.
// Do NOT regenerate these to match new behaviour: they ARE the regression contract.

// renderKeyHint(key=y, label=kill, AccentBlue): the canonical single footer hint.
const (
	goldenKeyHintYKillDark  = "\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mkill\x1b[m"
	goldenKeyHintYKillLight = "\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mkill\x1b[m"
	goldenKeyHintYKillNoCol = "y kill"
)

// renderKeyHint(key="", label="empty on save = delete"): the label-only fast path
// that editFooterGroup's "empty on save = delete" group collapses onto.
const (
	goldenKeyHintEmptyDark  = "\x1b[38;2;115;122;162;48;2;11;12;20mempty on save = delete\x1b[m"
	goldenKeyHintEmptyLight = "\x1b[38;2;88;96;147;48;2;225;226;231mempty on save = delete\x1b[m"
	goldenKeyHintEmptyNoCol = "empty on save = delete"
)

// renderConfirmCancelFooter golden rows captured from killModalFooterRow (y/kill,
// esc/cancel), deleteModalFooterRow (y/delete, esc/cancel) and renameModalFooterRow
// (⏎/rename, esc/cancel) — the fixed-gap "   " spacer between the two hints.
const (
	goldenKillFooterDark  = "\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mkill\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m"
	goldenKillFooterLight = "\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mkill\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m"
	goldenKillFooterNoCol = "y kill   esc cancel"

	goldenDeleteFooterDark  = "\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mdelete\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m"
	goldenDeleteFooterLight = "\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mdelete\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m"
	goldenDeleteFooterNoCol = "y delete   esc cancel"

	goldenRenameFooterDark  = "\x1b[38;2;122;162;247;48;2;11;12;20m⏎\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mrename\x1b[m\x1b[48;2;11;12;20m   \x1b[m\x1b[38;2;122;162;247;48;2;11;12;20mesc\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mcancel\x1b[m"
	goldenRenameFooterLight = "\x1b[38;2;45;92;202;48;2;225;226;231m⏎\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mrename\x1b[m\x1b[48;2;225;226;231m   \x1b[m\x1b[38;2;45;92;202;48;2;225;226;231mesc\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mcancel\x1b[m"
	goldenRenameFooterNoCol = "⏎ rename   esc cancel"
)

// previewFooterHint golden (glyph=←→, label=window) — proves the preview hint render.
const (
	goldenPreviewHintDark  = "\x1b[38;2;122;162;247;48;2;11;12;20m←→\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mwindow\x1b[m"
	goldenPreviewHintLight = "\x1b[38;2;45;92;202;48;2;225;226;231m←→\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mwindow\x1b[m"
	goldenPreviewHintNoCol = "←→ window"
)

// editFooterGroup golden (key=⏎/e, label=edit) — the normal non-empty edit hint.
const (
	goldenEditEditDark  = "\x1b[38;2;122;162;247;48;2;11;12;20m⏎/e\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20medit\x1b[m"
	goldenEditEditLight = "\x1b[38;2;45;92;202;48;2;225;226;231m⏎/e\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231medit\x1b[m"
	goldenEditEditNoCol = "⏎/e edit"
)

// TestRenderKeyHint asserts the shared renderKeyHint helper renders the canonical
// `<key> <label>` footer-hint shape (key glyph in keyTok over the owned canvas, a
// single canvas-painted gap, label in text.detail) byte-identically across both
// modes and the colourless carve-out, AND that the empty-key path collapses to the
// label-only render.
func TestRenderKeyHint(t *testing.T) {
	cases := []struct {
		name       string
		key        string
		label      string
		keyTok     theme.Token
		mode       theme.Mode
		colourless bool
		want       string
	}{
		{"normal/dark/colour", "y", "kill", theme.MV.AccentBlue, theme.Dark, false, goldenKeyHintYKillDark},
		{"normal/light/colour", "y", "kill", theme.MV.AccentBlue, theme.Light, false, goldenKeyHintYKillLight},
		{"normal/dark/colourless", "y", "kill", theme.MV.AccentBlue, theme.Dark, true, goldenKeyHintYKillNoCol},
		{"normal/light/colourless", "y", "kill", theme.MV.AccentBlue, theme.Light, true, goldenKeyHintYKillNoCol},

		{"empty/dark/colour", "", "empty on save = delete", theme.MV.AccentBlue, theme.Dark, false, goldenKeyHintEmptyDark},
		{"empty/light/colour", "", "empty on save = delete", theme.MV.AccentBlue, theme.Light, false, goldenKeyHintEmptyLight},
		{"empty/dark/colourless", "", "empty on save = delete", theme.MV.AccentBlue, theme.Dark, true, goldenKeyHintEmptyNoCol},
		{"empty/light/colourless", "", "empty on save = delete", theme.MV.AccentBlue, theme.Light, true, goldenKeyHintEmptyNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderKeyHint(tc.key, tc.label, tc.keyTok, tc.mode, tc.colourless)
			if got != tc.want {
				t.Errorf("renderKeyHint(%q,%q) mismatch\n got: %q\nwant: %q", tc.key, tc.label, got, tc.want)
			}
		})
	}
}

// TestRenderConfirmCancelFooter asserts the shared confirm/cancel row helper matches
// the prior hand-assembled output for the kill (y/esc), delete (y/esc) and rename
// (⏎/esc) constant sets — confirm hint + the fixed "   " canvas gap + cancel hint.
func TestRenderConfirmCancelFooter(t *testing.T) {
	cases := []struct {
		name                                             string
		confirmKey, confirmLabel, cancelKey, cancelLabel string
		mode                                             theme.Mode
		colourless                                       bool
		want                                             string
	}{
		{"kill/dark/colour", "y", "kill", "esc", "cancel", theme.Dark, false, goldenKillFooterDark},
		{"kill/light/colour", "y", "kill", "esc", "cancel", theme.Light, false, goldenKillFooterLight},
		{"kill/dark/colourless", "y", "kill", "esc", "cancel", theme.Dark, true, goldenKillFooterNoCol},
		{"kill/light/colourless", "y", "kill", "esc", "cancel", theme.Light, true, goldenKillFooterNoCol},

		{"delete/dark/colour", "y", "delete", "esc", "cancel", theme.Dark, false, goldenDeleteFooterDark},
		{"delete/light/colour", "y", "delete", "esc", "cancel", theme.Light, false, goldenDeleteFooterLight},
		{"delete/dark/colourless", "y", "delete", "esc", "cancel", theme.Dark, true, goldenDeleteFooterNoCol},
		{"delete/light/colourless", "y", "delete", "esc", "cancel", theme.Light, true, goldenDeleteFooterNoCol},

		{"rename/dark/colour", "⏎", "rename", "esc", "cancel", theme.Dark, false, goldenRenameFooterDark},
		{"rename/light/colour", "⏎", "rename", "esc", "cancel", theme.Light, false, goldenRenameFooterLight},
		{"rename/dark/colourless", "⏎", "rename", "esc", "cancel", theme.Dark, true, goldenRenameFooterNoCol},
		{"rename/light/colourless", "⏎", "rename", "esc", "cancel", theme.Light, true, goldenRenameFooterNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderConfirmCancelFooter(tc.confirmKey, tc.confirmLabel, tc.cancelKey, tc.cancelLabel, tc.mode, tc.colourless)
			if got != tc.want {
				t.Errorf("renderConfirmCancelFooter mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestFooterHintCallSitesByteIdentical pins each refactored call-site render function
// against the captured PRE-refactor golden, proving the consolidation produced zero
// output drift for every footer/modal-footer in both modes and the colourless carve-out.
func TestFooterHintCallSitesByteIdentical(t *testing.T) {
	modes := []struct {
		name string
		m    theme.Mode
	}{{"dark", theme.Dark}, {"light", theme.Light}}

	type golden struct{ colour, noColour string }

	t.Run("killModalKeyHint", func(t *testing.T) {
		want := map[string]golden{
			"dark":  {goldenKeyHintYKillDark, goldenKeyHintYKillNoCol},
			"light": {goldenKeyHintYKillLight, goldenKeyHintYKillNoCol},
		}
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := killModalKeyHint("y", "kill", md.m, cl)
				exp := want[md.name].colour
				if cl {
					exp = want[md.name].noColour
				}
				if got != exp {
					t.Errorf("killModalKeyHint %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("deleteModalKeyHint", func(t *testing.T) {
		// Reuse the y/kill golden shape via y/delete by asserting the footer row golden
		// instead — the per-hint helper is identical, so assert the row here.
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := deleteModalKeyHint("y", "kill", md.m, cl)
				exp := goldenKeyHintYKillDark
				if md.name == "light" {
					exp = goldenKeyHintYKillLight
				}
				if cl {
					exp = goldenKeyHintYKillNoCol
				}
				if got != exp {
					t.Errorf("deleteModalKeyHint %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("renameModalKeyHint", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renameModalKeyHint("y", "kill", md.m, cl)
				exp := goldenKeyHintYKillDark
				if md.name == "light" {
					exp = goldenKeyHintYKillLight
				}
				if cl {
					exp = goldenKeyHintYKillNoCol
				}
				if got != exp {
					t.Errorf("renameModalKeyHint %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("previewFooterHint", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := previewFooterHint("←→", "window", md.m, cl)
				exp := goldenPreviewHintDark
				if md.name == "light" {
					exp = goldenPreviewHintLight
				}
				if cl {
					exp = goldenPreviewHintNoCol
				}
				if got != exp {
					t.Errorf("previewFooterHint %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("editFooterGroup/normal", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := editFooterGroup("⏎/e", "edit", md.m, cl)
				exp := goldenEditEditDark
				if md.name == "light" {
					exp = goldenEditEditLight
				}
				if cl {
					exp = goldenEditEditNoCol
				}
				if got != exp {
					t.Errorf("editFooterGroup(normal) %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("editFooterGroup/empty", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := editFooterGroup("", "empty on save = delete", md.m, cl)
				exp := goldenKeyHintEmptyDark
				if md.name == "light" {
					exp = goldenKeyHintEmptyLight
				}
				if cl {
					exp = goldenKeyHintEmptyNoCol
				}
				if got != exp {
					t.Errorf("editFooterGroup(empty) %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("killModalFooterRow", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := killModalFooterRow(md.m, cl)
				exp := goldenKillFooterDark
				if md.name == "light" {
					exp = goldenKillFooterLight
				}
				if cl {
					exp = goldenKillFooterNoCol
				}
				if got != exp {
					t.Errorf("killModalFooterRow %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("deleteModalFooterRow", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := deleteModalFooterRow(md.m, cl)
				exp := goldenDeleteFooterDark
				if md.name == "light" {
					exp = goldenDeleteFooterLight
				}
				if cl {
					exp = goldenDeleteFooterNoCol
				}
				if got != exp {
					t.Errorf("deleteModalFooterRow %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("renameModalFooterRow", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renameModalFooterRow(md.m, cl)
				exp := goldenRenameFooterDark
				if md.name == "light" {
					exp = goldenRenameFooterLight
				}
				if cl {
					exp = goldenRenameFooterNoCol
				}
				if got != exp {
					t.Errorf("renameModalFooterRow %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})
}
