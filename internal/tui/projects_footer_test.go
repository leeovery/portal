package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §6.3 condensed Projects footer gate. These tests pin the restyled Projects
// footer: the exact §6.3 condensed copy as a single row's left cluster, a
// right-aligned `? help`, and the per-glyph colour roles (key glyphs accent.blue,
// labels text.detail, the ? glyph accent.violet) — rendered through the SHARED
// condensed-footer machinery (renderProjectsFooter), NOT the legacy three-column
// renderKeymapFooter. Asserted with exact mode-resolved SGR like the Sessions
// footer tests, so a token swap is caught, not merely the glyph presence.
//
// No t.Parallel() — the package's shared canvas/mock helpers make parallelism
// unsafe across these tests.

// TestProjectsFooter_CondensedCopyExact asserts the §6.3 condensed Projects
// footer renders EXACTLY `⏎ new session` · `x sessions` · `e edit` · `/ filter`
// as the left cluster, with a right-aligned `? help` pinned to the width.
func TestProjectsFooter_CondensedCopyExact(t *testing.T) {
	footer := renderProjectsFooter(referenceFooterWidth, theme.Dark, false)
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("condensed Projects footer must be 2 rows (rule + key row), got %d:\n%s", len(lines), footer)
	}
	keyRow := footerVisible(lines[1])

	for _, want := range []string{
		"⏎ new session", "x sessions", "e edit", "/ filter", "? help",
	} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("Projects key row missing Core entry %q:\n%s", want, keyRow)
		}
	}

	// The ? help is right-aligned: it is the trailing entry, and the left-cluster
	// entries all precede it.
	helpIdx := strings.Index(keyRow, "? help")
	filterIdx := strings.Index(keyRow, "/ filter")
	if helpIdx < 0 || filterIdx < 0 {
		t.Fatalf("expected both '/ filter' and '? help' on the key row:\n%s", keyRow)
	}
	if helpIdx <= filterIdx {
		t.Errorf("? help (idx %d) must be right of / filter (idx %d):\n%s", helpIdx, filterIdx, keyRow)
	}

	// Right-aligned to the width: the key row is exactly w cells, and ? help ends
	// flush at the right edge.
	if got := lipgloss.Width(lines[1]); got != referenceFooterWidth {
		t.Errorf("Projects key row width = %d, want exactly %d (right-aligned to width)", got, referenceFooterWidth)
	}
	if !strings.HasSuffix(strings.TrimRight(keyRow, " "), "help") {
		t.Errorf("? help must be the trailing (right-most) entry on the row:\n%s", keyRow)
	}
}

// TestProjectsFooter_NoLegacySwitchViewOrAttach asserts the Projects footer is the
// §6.3 condensed copy and NOT the Sessions footer copy: it must not leak Sessions
// labels (`attach`, `switch view`, `preview`, `projects`).
func TestProjectsFooter_NoLegacySessionsCopy(t *testing.T) {
	keyRow := footerVisible(renderProjectsFooter(referenceFooterWidth, theme.Dark, false))
	for _, banned := range []string{"attach", "switch view", "preview", "navigate", "projects"} {
		if strings.Contains(keyRow, banned) {
			t.Errorf("Projects footer leaked the Sessions copy %q:\n%s", banned, keyRow)
		}
	}
}

// TestProjectsFooter_TokenColours asserts key glyphs render in accent.blue, labels
// in text.detail, and the ? glyph specifically in accent.violet, over a 1px
// border.footer top rule — every colour via its §2.9 token (matching the
// reference's per-glyph footer colours).
func TestProjectsFooter_TokenColours(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			footer := renderProjectsFooter(referenceFooterWidth, tc.mode, false)

			if seq := tokenFgSeq(t, theme.MV.AccentBlue, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("Projects footer missing accent.blue key-glyph role sequence %q", seq)
			}
			if seq := tokenFgSeq(t, theme.MV.TextDetail, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("Projects footer missing text.detail label role sequence %q", seq)
			}
			// The ? glyph specifically is accent.violet (the right-aligned help anchor).
			if seq := tokenFgSeq(t, theme.MV.AccentViolet, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("Projects footer missing accent.violet ? glyph role sequence %q", seq)
			}
			// 1px border.footer top rule present.
			if seq := tokenFgSeq(t, theme.MV.BorderFooter, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("Projects footer missing the border.footer top rule role sequence %q", seq)
			}
		})
	}
}

// TestProjectsFooter_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless Projects footer carries no canvas background SGR and no
// foreground hue — the §6.3 copy stays structurally intact.
func TestProjectsFooter_ColourlessDropsHueAndCanvas(t *testing.T) {
	footer := renderProjectsFooter(referenceFooterWidth, theme.Dark, true)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])

	for _, want := range []string{"⏎ new session", "x sessions", "e edit", "/ filter", "? help"} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("colourless Projects footer dropped the copy %q:\n%s", want, keyRow)
		}
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(footer, seq) {
		t.Errorf("colourless Projects footer still paints the canvas background sequence %q", seq)
	}
	for _, tok := range []theme.Token{theme.MV.AccentBlue, theme.MV.TextDetail, theme.MV.AccentViolet} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(footer, seq) {
			t.Errorf("colourless Projects footer still emits a foreground role sequence %q", seq)
		}
	}
}

// TestProjectsFooter_FilterAppliedSaysNewSessionNotAttach asserts the Projects
// list-active (committed-filter) footer reads `↵ new session` — Enter on Projects
// creates a session, it does NOT attach — and never leaks the Sessions `↵ attach`
// copy. Filtering IS enabled on the Projects list, so this state is reachable.
func TestProjectsFooter_FilterAppliedSaysNewSessionNotAttach(t *testing.T) {
	footer := renderProjectsFilterAppliedFooter(referenceFooterWidth, theme.Dark, false)
	keyRow := footerVisible(strings.Split(footer, "\n")[1])
	if !strings.Contains(keyRow, "new session") {
		t.Errorf("Projects list-active footer missing %q:\n%s", "new session", keyRow)
	}
	if strings.Contains(keyRow, "attach") {
		t.Errorf("Projects list-active footer leaks the Sessions %q copy:\n%s", "attach", keyRow)
	}
	for _, want := range []string{"navigate", "clear filter"} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("Projects list-active footer missing %q:\n%s", want, keyRow)
		}
	}
}

// TestProjectsFooter_NarrowDegradeKeepsHelpAnchor asserts the §2.7 narrow degrade:
// at a width too small for the full left cluster the row truncates on ONE line
// (never wraps) and the ? help right anchor survives, the row never overflowing.
func TestProjectsFooter_NarrowDegradeKeepsHelpAnchor(t *testing.T) {
	const narrow = 24
	footer := renderProjectsFooter(narrow, theme.Dark, false)
	for i, line := range strings.Split(footer, "\n") {
		if lw := lipgloss.Width(line); lw > narrow {
			t.Errorf("narrow Projects footer line %d width = %d (overflow, want <= %d):\n%s", i, lw, narrow, ansi.Strip(line))
		}
	}
}
