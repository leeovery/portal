package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/leeovery/portal/internal/theme"
)

func preHelpModalRow(e keymapEntry, th theme.Theme, colourless bool) string {
	keyTok := th.AccentKey
	if isDestructiveHelpKey(e) {
		keyTok = th.StateDestructive
	}
	key := headerStyle(keyTok, th, colourless).Bold(true).Render(helpKeyGlyph(e))
	keyWidth := lipgloss.Width(key)
	pad := ""
	if keyWidth < helpKeyColumnWidth {
		pad = headerCanvasBg(th, colourless).Render(strings.Repeat(" ", helpKeyColumnWidth-keyWidth))
	}
	gap := headerCanvasBg(th, colourless).Render(helpColumnGap)
	label := headerStyle(th.TextSecondary, th, colourless).Render(helpActionLabel(e))
	return lipgloss.JoinHorizontal(lipgloss.Top, key, pad, gap, label)
}

func preThemePanelFooterRow(e keymapEntry, width int, th theme.Theme, colourless bool) string {
	key := headerStyle(th.AccentKey, th, colourless).Render(helpKeyGlyph(e))
	keyWidth := lipgloss.Width(key)
	pad := ""
	if keyWidth < themePanelFooterKeyColumnWidth {
		pad = headerCanvasBg(th, colourless).Render(spaces(themePanelFooterKeyColumnWidth - keyWidth))
	}
	gap := headerCanvasBg(th, colourless).Render(footerKeyLabelGap)
	label := headerStyle(th.TextMuted, th, colourless).Render(e.Action)

	row := lipgloss.JoinHorizontal(lipgloss.Top, key, pad, gap, label)
	return headerPadRight(row, lipgloss.Width(row), width, th, colourless)
}

func keyColumnRowEntries() []keymapEntry {
	scopes := [][]keymapEntry{
		sessionsKeymap(),
		projectsKeymap(),
		previewKeymap(),
		themePanelKeymap(),
	}
	all := make([]keymapEntry, 0)
	for _, scope := range scopes {
		all = append(all, scope...)
	}
	return all
}

func TestHelpModalRow_ByteIdenticalAcrossExtraction(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, colourless := range []bool{false, true} {
			for _, e := range keyColumnRowEntries() {
				want := preHelpModalRow(e, th, colourless)
				if got := helpModalRow(e, th, colourless); got != want {
					t.Errorf("[%v col=%v] help row for %q drifted from the pre-extraction golden\n got: %q\nwant: %q",
						themeLabel(th), colourless, e.Key, got, want)
				}
			}
		}
	}
}

func TestThemePanelFooterRow_ByteIdenticalAcrossExtraction(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, colourless := range []bool{false, true} {
			for _, width := range []int{0, themePanelFooterTestMinWidth, themePanelFooterTestWidth} {
				for _, e := range keyColumnRowEntries() {
					want := preThemePanelFooterRow(e, width, th, colourless)
					if got := themePanelFooterRow(e, width, th, colourless); got != want {
						t.Errorf("[%v col=%v w=%d] panel footer row for %q drifted from the pre-extraction golden\n got: %q\nwant: %q",
							themeLabel(th), colourless, width, e.Key, got, want)
					}
				}
			}
		}
	}
}

func TestKeyColumnRow_BuildsBothSurfacesRows(t *testing.T) {
	for _, th := range []theme.Theme{testDarkTheme(t), testLightTheme(t)} {
		for _, colourless := range []bool{false, true} {
			for _, e := range keyColumnRowEntries() {
				keyTok := th.AccentKey
				if e.Destructive {
					keyTok = th.StateDestructive
				}
				help := keyColumnRow(
					helpKeyGlyph(e), helpActionLabel(e),
					headerStyle(keyTok, th, colourless).Bold(true),
					headerStyle(th.TextSecondary, th, colourless),
					helpKeyColumnWidth, helpColumnGap, th, colourless)
				if got := helpModalRow(e, th, colourless); got != help {
					t.Errorf("[%v col=%v] help row for %q is not the shared builder's row\n got: %q\nwant: %q",
						themeLabel(th), colourless, e.Key, got, help)
				}

				panel := keyColumnRow(
					helpKeyGlyph(e), e.Action,
					headerStyle(th.AccentKey, th, colourless),
					headerStyle(th.TextMuted, th, colourless),
					themePanelFooterKeyColumnWidth, footerKeyLabelGap, th, colourless)
				padded := headerPadRight(panel, lipgloss.Width(panel), themePanelFooterTestWidth, th, colourless)
				if got := themePanelFooterRow(e, themePanelFooterTestWidth, th, colourless); got != padded {
					t.Errorf("[%v col=%v] panel footer row for %q is not the shared builder's row\n got: %q\nwant: %q",
						themeLabel(th), colourless, e.Key, got, padded)
				}
			}
		}
	}
}

func TestKeyColumnRow_PadsOnlyWhenTheGlyphIsNarrowerThanTheColumn(t *testing.T) {
	const columnWidth = 3
	th := testDarkTheme(t)
	keyStyle := headerStyle(th.AccentKey, th, false)
	labelStyle := headerStyle(th.TextMuted, th, false)
	canvas := headerCanvasBg(th, false)

	for _, tc := range []struct {
		name  string
		glyph string
		pad   string
	}{
		{"narrower than the column", "d", spaces(columnWidth - 1)},
		{"exactly the column width", "esc", ""},
		{"wider than the column", "^↑/↓", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			segments := []string{keyStyle.Render(tc.glyph)}
			if tc.pad != "" {
				segments = append(segments, canvas.Render(tc.pad))
			}
			segments = append(segments, canvas.Render(footerKeyLabelGap), labelStyle.Render("close"))
			want := lipgloss.JoinHorizontal(lipgloss.Top, segments...)

			got := keyColumnRow(tc.glyph, "close", keyStyle, labelStyle, columnWidth, footerKeyLabelGap, th, false)
			if got != want {
				t.Errorf("keyColumnRow(%q) = %q, want %q", tc.glyph, got, want)
			}
		})
	}
}
