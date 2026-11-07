package tui

import (
	"testing"

	"github.com/leeovery/portal/internal/theme"
)

const (
	goldenKeyHintYKillDark  = "\x1b[38;2;122;162;247;48;2;11;12;20my\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20mkill\x1b[m"
	goldenKeyHintYKillLight = "\x1b[38;2;45;92;202;48;2;225;226;231my\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231mkill\x1b[m"
	goldenKeyHintYKillNoCol = "y kill"
)

const (
	goldenKeyHintEmptyDark  = "\x1b[38;2;115;122;162;48;2;11;12;20mempty on save = delete\x1b[m"
	goldenKeyHintEmptyLight = "\x1b[38;2;88;96;147;48;2;225;226;231mempty on save = delete\x1b[m"
	goldenKeyHintEmptyNoCol = "empty on save = delete"
)

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

const (
	goldenPreviewHintDark  = "\x1b[38;2;122;162;247;48;2;11;12;20m↑↓\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20msession\x1b[m"
	goldenPreviewHintLight = "\x1b[38;2;45;92;202;48;2;225;226;231m↑↓\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231msession\x1b[m"
	goldenPreviewHintNoCol = "↑↓ session"
)

const (
	goldenEditEditDark  = "\x1b[38;2;122;162;247;48;2;11;12;20m⏎/e\x1b[m\x1b[48;2;11;12;20m \x1b[m\x1b[38;2;115;122;162;48;2;11;12;20medit\x1b[m"
	goldenEditEditLight = "\x1b[38;2;45;92;202;48;2;225;226;231m⏎/e\x1b[m\x1b[48;2;225;226;231m \x1b[m\x1b[38;2;88;96;147;48;2;225;226;231medit\x1b[m"
	goldenEditEditNoCol = "⏎/e edit"
)

func TestRenderKeyHint(t *testing.T) {
	cases := []struct {
		name       string
		key        string
		label      string
		keyTok     theme.Token
		th         theme.Theme
		colourless bool
		want       string
	}{
		{"normal/dark/colour", "y", "kill", testDarkTheme(t).AccentKey, testDarkTheme(t), false, goldenKeyHintYKillDark},
		{"normal/light/colour", "y", "kill", testLightTheme(t).AccentKey, testLightTheme(t), false, goldenKeyHintYKillLight},
		{"normal/dark/colourless", "y", "kill", testDarkTheme(t).AccentKey, testDarkTheme(t), true, goldenKeyHintYKillNoCol},
		{"normal/light/colourless", "y", "kill", testLightTheme(t).AccentKey, testLightTheme(t), true, goldenKeyHintYKillNoCol},

		{"empty/dark/colour", "", "empty on save = delete", testDarkTheme(t).AccentKey, testDarkTheme(t), false, goldenKeyHintEmptyDark},
		{"empty/light/colour", "", "empty on save = delete", testLightTheme(t).AccentKey, testLightTheme(t), false, goldenKeyHintEmptyLight},
		{"empty/dark/colourless", "", "empty on save = delete", testDarkTheme(t).AccentKey, testDarkTheme(t), true, goldenKeyHintEmptyNoCol},
		{"empty/light/colourless", "", "empty on save = delete", testLightTheme(t).AccentKey, testLightTheme(t), true, goldenKeyHintEmptyNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderKeyHint(tc.key, tc.label, tc.keyTok, tc.th, tc.colourless)
			if got != tc.want {
				t.Errorf("renderKeyHint(%q,%q) mismatch\n got: %q\nwant: %q", tc.key, tc.label, got, tc.want)
			}
		})
	}
}

func TestRenderBlueKeyHint(t *testing.T) {
	cases := []struct {
		name       string
		key, label string
		th         theme.Theme
		colourless bool
		want       string
	}{
		{"yKill/dark/colour", "y", "kill", testDarkTheme(t), false, goldenKeyHintYKillDark},
		{"yKill/light/colour", "y", "kill", testLightTheme(t), false, goldenKeyHintYKillLight},
		{"yKill/dark/colourless", "y", "kill", testDarkTheme(t), true, goldenKeyHintYKillNoCol},
		{"yKill/light/colourless", "y", "kill", testLightTheme(t), true, goldenKeyHintYKillNoCol},

		{"empty/dark/colour", "", "empty on save = delete", testDarkTheme(t), false, goldenKeyHintEmptyDark},
		{"empty/light/colour", "", "empty on save = delete", testLightTheme(t), false, goldenKeyHintEmptyLight},
		{"empty/dark/colourless", "", "empty on save = delete", testDarkTheme(t), true, goldenKeyHintEmptyNoCol},
		{"empty/light/colourless", "", "empty on save = delete", testLightTheme(t), true, goldenKeyHintEmptyNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderBlueKeyHint(tc.key, tc.label, tc.th, tc.colourless)
			if got != tc.want {
				t.Errorf("renderBlueKeyHint(%q,%q) golden mismatch\n got: %q\nwant: %q", tc.key, tc.label, got, tc.want)
			}
			if pinned := renderKeyHint(tc.key, tc.label, tc.th.AccentKey, tc.th, tc.colourless); got != pinned {
				t.Errorf("renderBlueKeyHint(%q,%q) does not pin AccentBlue\n got: %q\nwant: %q", tc.key, tc.label, got, pinned)
			}
		})
	}
}

func TestRenderConfirmCancelFooter(t *testing.T) {
	cases := []struct {
		name                                             string
		confirmKey, confirmLabel, cancelKey, cancelLabel string
		th                                               theme.Theme
		colourless                                       bool
		want                                             string
	}{
		{"kill/dark/colour", "y", "kill", "esc", "cancel", testDarkTheme(t), false, goldenKillFooterDark},
		{"kill/light/colour", "y", "kill", "esc", "cancel", testLightTheme(t), false, goldenKillFooterLight},
		{"kill/dark/colourless", "y", "kill", "esc", "cancel", testDarkTheme(t), true, goldenKillFooterNoCol},
		{"kill/light/colourless", "y", "kill", "esc", "cancel", testLightTheme(t), true, goldenKillFooterNoCol},

		{"delete/dark/colour", "y", "delete", "esc", "cancel", testDarkTheme(t), false, goldenDeleteFooterDark},
		{"delete/light/colour", "y", "delete", "esc", "cancel", testLightTheme(t), false, goldenDeleteFooterLight},
		{"delete/dark/colourless", "y", "delete", "esc", "cancel", testDarkTheme(t), true, goldenDeleteFooterNoCol},
		{"delete/light/colourless", "y", "delete", "esc", "cancel", testLightTheme(t), true, goldenDeleteFooterNoCol},

		{"rename/dark/colour", "⏎", "rename", "esc", "cancel", testDarkTheme(t), false, goldenRenameFooterDark},
		{"rename/light/colour", "⏎", "rename", "esc", "cancel", testLightTheme(t), false, goldenRenameFooterLight},
		{"rename/dark/colourless", "⏎", "rename", "esc", "cancel", testDarkTheme(t), true, goldenRenameFooterNoCol},
		{"rename/light/colourless", "⏎", "rename", "esc", "cancel", testLightTheme(t), true, goldenRenameFooterNoCol},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderConfirmCancelFooter(tc.confirmKey, tc.confirmLabel, tc.cancelKey, tc.cancelLabel, tc.th, tc.colourless)
			if got != tc.want {
				t.Errorf("renderConfirmCancelFooter mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestFooterHintCallSitesByteIdentical(t *testing.T) {
	modes := builtinThemeCases(t)

	t.Run("previewFooterHint", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renderBlueKeyHint("↑↓", "session", md.th, cl)
				exp := goldenPreviewHintDark
				if md.name == "light" {
					exp = goldenPreviewHintLight
				}
				if cl {
					exp = goldenPreviewHintNoCol
				}
				if got != exp {
					t.Errorf("preview footer hint %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("editFooterGroup/normal", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renderBlueKeyHint("⏎/e", "edit", md.th, cl)
				exp := goldenEditEditDark
				if md.name == "light" {
					exp = goldenEditEditLight
				}
				if cl {
					exp = goldenEditEditNoCol
				}
				if got != exp {
					t.Errorf("edit footer group(normal) %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("editFooterGroup/empty", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renderBlueKeyHint("", "empty on save = delete", md.th, cl)
				exp := goldenKeyHintEmptyDark
				if md.name == "light" {
					exp = goldenKeyHintEmptyLight
				}
				if cl {
					exp = goldenKeyHintEmptyNoCol
				}
				if got != exp {
					t.Errorf("edit footer group(empty) %s colourless=%v drift\n got: %q\nwant: %q", md.name, cl, got, exp)
				}
			}
		}
	})

	t.Run("renameModalFooterRow", func(t *testing.T) {
		for _, md := range modes {
			for _, cl := range []bool{false, true} {
				got := renameModalFooterRow(md.th, cl)
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
