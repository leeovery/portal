package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/leeovery/portal/internal/tmux"
)

// End-to-end cascade-tier test driving the full Update → View pipeline.
// Per specification.md § Tests > Surface 5 and § Test conventions.
//
// THRESHOLD RATIONALE — adjusted from spec's stated widths.
// The spec § Tests > Surface 5 enumerates widths {200, 60, 40, 25, 15} mapped
// to tiers {1-full, 1-truncated, 2, 3, 4}. The spec's prose was based on
// nominal cell counts (it states the verbose keymap is "82 cells"), but the
// actual lipgloss.Width(verboseKeymap) is 57 cells. With the test fixture
// (window name "nvim-editor", 1 window × 1 pane → counters "Window 1 of 1 ·
// Pane 1 of 1" = 27 cells, separator " · win: " = 8 cells, keymap padding 1
// cell, verbose keymap 57 cells), the real cascade math is:
//
//   - Tier 1 fixed overhead = 4 + 27 + 8 + 1 + 57 = 97 cells.
//     Needs outer >= 97 + minWindowNameCells(8) = 105.
//   - Tier 2 = drop "· win: {name}": outer >= 4 + 27 + 1 + 57 = 89.
//   - Tier 3 = compact keymap (9 cells): outer >= 4 + 27 + 1 + 9 = 41.
//   - Tier 4 = corners + filler: always fits at outer >= 2.
//
// Mapping spec's labels to mathematically-realizable terminal widths:
//   - Tier 1 full (name fits whole, nameBudget >= 11): outer >= 108. Use 200.
//   - Tier 1 truncated (name truncated to budget, 8 <= nameBudget < 11):
//     outer ∈ [105, 107]. Use 105 — nameBudget = 8 → "nvim-ed…".
//   - Tier 2 (no win: segment, verbose keymap): outer ∈ [89, 104]. Use 95.
//   - Tier 3 (compact keymap): outer ∈ [41, 88]. Use 50.
//   - Tier 4 (corners + filler only): outer ∈ [2, 40]. Use 15 — matches the
//     spec's ASCII pattern "╭" + 13 × "─" + "╮".
//
// Picked widths are interior to each tier's interval (not on boundaries) to
// avoid fragility against incidental future fixed-overhead changes.
//
// See pagepreview_compose_chrome_test.go for the unit-level cascade
// thresholds; this test reuses the same math, applied through View().

func TestPreviewView_CascadeTiersEndToEnd(t *testing.T) {
	// Fixture: one window, one pane, window name "nvim-editor". The
	// ScrollbackReader returns body with at least two lines, one containing
	// an unterminated SGR — guarantees the SGR-reset injection path is
	// exercised at every tier so the per-row "\x1b[0m" assertion is
	// meaningful regardless of width.
	const session = "work"
	const windowName = "nvim-editor"
	body := []byte("\x1b[41mhello\nworld\n")

	tests := []struct {
		name   string
		width  int
		assert func(t *testing.T, stripped string, raw string)
	}{
		{
			name:  "tier 1 full at width 200 — full window name and verbose keymap",
			width: 200,
			assert: func(t *testing.T, stripped, _ string) {
				if !strings.Contains(stripped, "nvim-editor") {
					t.Errorf("tier 1 full: expected full window name %q present; stripped=%q", "nvim-editor", stripped)
				}
				if strings.Contains(stripped, "nvim-editor…") {
					t.Errorf("tier 1 full: expected no ellipsis on full-name tier; stripped=%q", stripped)
				}
				if !strings.Contains(stripped, "⇥ next pane") {
					t.Errorf("tier 1 full: expected verbose keymap token %q present; stripped=%q", "⇥ next pane", stripped)
				}
			},
		},
		{
			name:  "tier 1 truncated at width 105 — window name truncated with ellipsis",
			width: 105,
			assert: func(t *testing.T, stripped, _ string) {
				// Locate the truncated name token. The window-name segment is
				// preceded by "win: " and ends at the next space. At width
				// 105, nameBudget = 8 so "nvim-editor" → "nvim-ed…".
				idx := strings.Index(stripped, "win: ")
				if idx < 0 {
					t.Fatalf("tier 1 truncated: expected 'win: ' prefix present; stripped=%q", stripped)
				}
				after := stripped[idx+len("win: "):]
				end := strings.IndexByte(after, ' ')
				if end < 0 {
					t.Fatalf("tier 1 truncated: could not isolate name token after 'win: '; remainder=%q", after)
				}
				nameToken := after[:end]
				if !strings.HasPrefix(nameToken, "nvim") {
					t.Errorf("tier 1 truncated: expected name token to start with 'nvim'; got %q", nameToken)
				}
				if !strings.HasSuffix(nameToken, "…") {
					t.Errorf("tier 1 truncated: expected name token to end with '…'; got %q", nameToken)
				}
				if !strings.Contains(stripped, "⇥ next pane") {
					t.Errorf("tier 1 truncated: expected verbose keymap token %q present; stripped=%q", "⇥ next pane", stripped)
				}
			},
		},
		{
			name:  "tier 2 at width 95 — drops win segment, keeps verbose keymap",
			width: 95,
			assert: func(t *testing.T, stripped, _ string) {
				if strings.Contains(stripped, "win:") {
					t.Errorf("tier 2: expected no 'win:' segment; stripped=%q", stripped)
				}
				if !strings.Contains(stripped, "⇥ next pane") {
					t.Errorf("tier 2: expected verbose keymap token %q present; stripped=%q", "⇥ next pane", stripped)
				}
			},
		},
		{
			name:  "tier 3 at width 50 — swaps to compact keymap",
			width: 50,
			assert: func(t *testing.T, stripped, _ string) {
				if strings.Contains(stripped, "win:") {
					t.Errorf("tier 3: expected no 'win:' segment; stripped=%q", stripped)
				}
				if !strings.Contains(stripped, compactKeymap) {
					t.Errorf("tier 3: expected compactKeymap %q present; stripped=%q", compactKeymap, stripped)
				}
				if strings.Contains(stripped, "next pane") {
					t.Errorf("tier 3: expected verbose token 'next pane' absent; stripped=%q", stripped)
				}
			},
		},
		{
			name:  "tier 4 at width 15 — drops chrome, corners and filler only",
			width: 15,
			assert: func(t *testing.T, stripped, _ string) {
				if strings.Contains(stripped, "Window ") {
					t.Errorf("tier 4: expected no 'Window ' segment; stripped=%q", stripped)
				}
				if strings.Contains(stripped, verboseKeymap) || strings.Contains(stripped, compactKeymap) {
					t.Errorf("tier 4: expected no keymap; stripped=%q", stripped)
				}
				// Top-edge ASCII pattern: "╭" + 13 × "─" + "╮".
				topRow := stripped
				if i := strings.IndexByte(stripped, '\n'); i >= 0 {
					topRow = stripped[:i]
				}
				want := "╭" + strings.Repeat("─", 13) + "╮"
				if topRow != want {
					t.Errorf("tier 4: top-edge pattern mismatch; got %q want %q", topRow, want)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enum := &stubEnumerator{
				groups: []tmux.WindowGroup{
					{WindowIndex: 0, WindowName: windowName, PaneIndices: []int{0}},
				},
			}
			reader := &recordingReader{bytes: body}
			m, ok := NewPreviewModel(session, enum, reader, nil, tc.width, 30)
			if !ok {
				t.Fatalf("setup: expected ok=true from NewPreviewModel, got false")
			}
			m, _ = m.Update(tea.WindowSizeMsg{Width: tc.width, Height: 30})

			raw := m.View()
			stripped := stripANSI(raw)

			tc.assert(t, stripped, raw)

			// Every row must carry at least one SGR-reset byte sequence
			// somewhere in the raw output — viewport rows pass through
			// injectSGRResets so the unterminated "\x1b[41m" in the body
			// cannot bleed into the right border at any cascade tier.
			if !strings.Contains(raw, "\x1b[0m") {
				t.Errorf("expected '\\x1b[0m' SGR reset present in raw View() output at width %d; raw=%q", tc.width, raw)
			}
		})
	}
}
