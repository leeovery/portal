package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func TestPreviewView_CascadeTiersEndToEnd(t *testing.T) {
	const body = "\x1b[41mhello\r\nworld"
	labelledFooter := "↑↓ session  ⏎ attach  ␣ back"

	tests := []struct {
		name   string
		width  int
		assert func(t *testing.T, stripped string)
	}{
		{
			name:  "wide — marker and full session; full labelled footer",
			width: lipgloss.Width(labelledFooter) + previewFrameOverhead + 10,
			assert: func(t *testing.T, stripped string) {
				if !strings.Contains(stripped, "◉ preview work") {
					t.Errorf("wide: expected full header; stripped=%q", stripped)
				}
				if !strings.Contains(stripped, labelledFooter) {
					t.Errorf("wide: expected full labelled footer; stripped=%q", stripped)
				}
			},
		},
		{
			name:  "narrow — footer compacts to glyphs only",
			width: 20,
			assert: func(t *testing.T, stripped string) {
				if !strings.Contains(stripped, "↑↓  ⏎  ␣") {
					t.Errorf("narrow: expected compact glyph-only footer; stripped=%q", stripped)
				}
				if strings.Contains(stripped, "session ") || strings.Contains(stripped, "attach") {
					t.Errorf("narrow: expected footer labels dropped; stripped=%q", stripped)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestPreview(t, body, tc.width, 30)
			m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 30})

			raw := m.View()
			tc.assert(t, stripANSI(raw))

			if got := lipgloss.Width(firstLine(raw)); got != tc.width {
				t.Errorf("width %d: top frame line width = %d, want %d", tc.width, got, tc.width)
			}

			rawLines := strings.Split(raw, "\n")
			for _, fixtureToken := range []string{"hello", "world"} {
				found := false
				for _, line := range rawLines {
					if !strings.Contains(stripANSI(line), fixtureToken) {
						continue
					}
					found = true
					if !strings.Contains(line, "\x1b[0m") {
						t.Errorf("width %d: row containing %q lacks SGR reset; raw line=%q", tc.width, fixtureToken, line)
					}
				}
				if !found {
					t.Errorf("width %d: fixture token %q not rendered in any row; raw=%q", tc.width, fixtureToken, raw)
				}
			}
		})
	}
}
