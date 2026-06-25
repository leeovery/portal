package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// referenceFooterWidth is a content width wide enough that the §3.4 condensed
// footer renders all six Core keys plus the right-aligned ? help without §2.7
// truncation. The full left cluster is ~83 cells + ? help (6) + spacer (1) = ~90,
// so 120 leaves comfortable headroom (it mirrors the reference capture width).
const referenceFooterWidth = 120

// footerVisible strips ANSI/SGR so the footer's printable glyphs can be asserted
// independent of the canvas/colour SGR painted across the cells.
func footerVisible(s string) string {
	return ansi.Strip(s)
}

// TestSessionsFooter_SingleRowCoreKeysWithRightAlignedHelp asserts the §3.4
// condensed footer renders exactly the Sessions Core keys as a single row's left
// cluster, with a right-aligned ? help pinned to the width. The footer is two
// lines (the 1px border.footer top rule + the single key row), so the key row is
// the LAST line — the ? help must sit at its right edge.
func TestSessionsFooter_SingleRowCoreKeysWithRightAlignedHelp(t *testing.T) {
	footer := renderSessionsFooter(referenceFooterWidth, theme.Dark, false)
	lines := strings.Split(footer, "\n")
	if len(lines) != 2 {
		t.Fatalf("condensed footer must be 2 rows (rule + key row), got %d:\n%s", len(lines), footer)
	}
	keyRow := footerVisible(lines[1])

	// Every Core label + glyph appears, in descriptor order, on the single key row.
	// Per §3.4 the footer reads the glyph Key forms: ↑↓ (no slash) for nav, ⏎ for
	// attach, ␣ for preview (matching the committed reference frames).
	for _, want := range []string{
		"↑↓ navigate", "⏎ attach", "/ filter", "␣ preview",
		"s switch view", "x projects", "? help",
	} {
		if !strings.Contains(keyRow, want) {
			t.Errorf("key row missing Core entry %q:\n%s", want, keyRow)
		}
	}

	// The ? help is right-aligned: it is the trailing entry, and the left-cluster
	// entries all precede it.
	helpIdx := strings.Index(keyRow, "? help")
	projectsIdx := strings.Index(keyRow, "x projects")
	if helpIdx < 0 || projectsIdx < 0 {
		t.Fatalf("expected both 'x projects' and '? help' on the key row:\n%s", keyRow)
	}
	if helpIdx <= projectsIdx {
		t.Errorf("? help (idx %d) must be right of x projects (idx %d):\n%s", helpIdx, projectsIdx, keyRow)
	}

	// Right-aligned: the gap between the left cluster and the ? help is wider than
	// any single inter-entry separator (the flex spacer), and ? help ends flush at
	// the row's right edge (width == w, no trailing chrome after "help").
	if got := lipgloss.Width(lines[1]); got != referenceFooterWidth {
		t.Errorf("key row width = %d, want exactly %d (right-aligned to width)", got, referenceFooterWidth)
	}
	if !strings.HasSuffix(strings.TrimRight(keyRow, " "), "help") {
		t.Errorf("? help must be the trailing (right-most) entry on the row:\n%s", keyRow)
	}
}

// TestSessionsFooter_TokenColours asserts key glyphs render in accent.blue, labels
// in text.detail, and the ? glyph specifically in accent.violet, over a 1px
// border.footer top rule — every colour via its §2.9 token.
func TestSessionsFooter_TokenColours(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode theme.Mode
	}{
		{"dark", theme.Dark},
		{"light", theme.Light},
	} {
		t.Run(tc.name, func(t *testing.T) {
			footer := renderSessionsFooter(referenceFooterWidth, tc.mode, false)

			// Left-cluster key glyphs: accent.blue.
			if seq := tokenFgSeq(t, theme.MV.AccentBlue, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("footer missing accent.blue key-glyph role sequence %q", seq)
			}
			// Labels (and separators): text.detail.
			if seq := tokenFgSeq(t, theme.MV.TextDetail, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("footer missing text.detail label role sequence %q", seq)
			}
			// The ? glyph: accent.violet.
			if seq := tokenFgSeq(t, theme.MV.AccentViolet, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("footer missing accent.violet ? role sequence %q", seq)
			}
			// The 1px top rule: border.footer (NOT border.separator).
			if seq := tokenFgSeq(t, theme.MV.BorderFooter, tc.mode); !strings.Contains(footer, seq) {
				t.Errorf("footer missing border.footer rule role sequence %q", seq)
			}
			// The footer rule must use border.footer, not the 2px header border.separator.
			// The two tokens are DISTINCT in dark mode (separator #292E42 ≠ footer #20232E)
			// but coincide in light mode (#C9CDDB), so the negative assertion only
			// discriminates in dark — assert there.
			if tc.mode == theme.Dark {
				if seq := tokenFgSeq(t, theme.MV.BorderSeparator, tc.mode); strings.Contains(footer, seq) {
					t.Errorf("footer must NOT use the 2px border.separator token (that is the header rule); found %q", seq)
				}
			}
		})
	}
}

// TestSessionsFooter_HelpGlyphIsViolet pins the §3.4 requirement that the ? glyph
// is rendered specifically in accent.violet — distinct from the accent.blue used
// by the left-cluster key glyphs. The ? glyph SGR run must carry the violet
// foreground, and there must be no accent.blue run on the "?" glyph itself.
func TestSessionsFooter_HelpGlyphIsViolet(t *testing.T) {
	footer := renderSessionsFooter(referenceFooterWidth, theme.Dark, false)
	violet := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark)

	// Locate the "?" glyph in the styled string; the immediately-preceding SGR run
	// opening must be the accent.violet foreground.
	qIdx := strings.IndexByte(footer, '?')
	if qIdx < 0 {
		t.Fatalf("footer does not contain the ? glyph:\n%s", footer)
	}
	// The SGR run for the ? glyph precedes it; the violet param sequence must appear
	// in the footer and be the foreground of the ? run.
	prefix := footer[:qIdx]
	lastEsc := strings.LastIndex(prefix, "\x1b[")
	if lastEsc < 0 {
		t.Fatalf("no SGR run precedes the ? glyph:\n%s", footer)
	}
	run := footer[lastEsc:qIdx]
	if !strings.Contains(run, violet) {
		t.Errorf("the ? glyph's SGR run %q is not accent.violet (%q)", run, violet)
	}
}

// TestSessionsFooter_OmitsHelpOnlyKeys asserts n/r/k/q and the Ctrl+↑/↓ paging
// entry are NOT in the footer — they are help-only (the ? help modal, Phase 3).
func TestSessionsFooter_OmitsHelpOnlyKeys(t *testing.T) {
	footer := footerVisible(renderSessionsFooter(referenceFooterWidth, theme.Dark, false))
	for _, banned := range []string{"new in cwd", "rename", "kill", "quit", "page", "Ctrl"} {
		if strings.Contains(footer, banned) {
			t.Errorf("footer must NOT show the help-only entry %q (it lives in ? help):\n%s", banned, footer)
		}
	}
}

// TestSessionsFooter_SourcedFromKeymapDescriptor asserts the footer entries are
// SOURCED from the task 2-1 sessionsKeymap descriptor (single source of truth):
// the footer shows exactly the descriptor's Core entries (by label) and nothing
// that is not a Core descriptor entry.
func TestSessionsFooter_SourcedFromKeymapDescriptor(t *testing.T) {
	footer := footerVisible(renderSessionsFooter(referenceFooterWidth, theme.Dark, false))

	for _, e := range sessionsKeymap() {
		shown := strings.Contains(footer, e.Action)
		if e.Core && !shown {
			t.Errorf("Core descriptor entry %q (%s) missing from the footer:\n%s", e.Action, e.Key, footer)
		}
		if !e.Core && shown {
			t.Errorf("non-Core descriptor entry %q leaked into the footer (help-only):\n%s", e.Action, footer)
		}
	}

	// And the partition helper returns exactly the six left-cluster Core entries +
	// the single right-aligned ? help, in descriptor order.
	core, right := splitFooterEntries(sessionsKeymap())
	wantCore := []string{"navigate", "attach", "filter", "preview", "switch view", "projects"}
	if len(core) != len(wantCore) {
		t.Fatalf("left-cluster Core entries = %d, want %d", len(core), len(wantCore))
	}
	for i, w := range wantCore {
		if core[i].Action != w {
			t.Errorf("left-cluster entry %d = %q, want %q (descriptor order)", i, core[i].Action, w)
		}
	}
	if right == nil || right.Action != "help" || !right.RightAligned {
		t.Errorf("right anchor = %+v, want the right-aligned ? help entry", right)
	}
}

// TestSessionsFooter_ColourlessDropsHueAndCanvas asserts the NO_COLOR carve-out
// (§2.5): a colourless footer carries no canvas background SGR and no foreground
// hue — it renders on the terminal's native fg/bg, the glyphs structurally intact.
func TestSessionsFooter_ColourlessDropsHueAndCanvas(t *testing.T) {
	footer := renderSessionsFooter(referenceFooterWidth, theme.Dark, true)

	// Structure preserved: every Core entry + the ? help still print.
	vis := footerVisible(footer)
	for _, want := range []string{"↑↓ navigate", "s switch view", "x projects", "? help"} {
		if !strings.Contains(vis, want) {
			t.Errorf("colourless footer missing %q:\n%s", want, vis)
		}
	}
	// No canvas background painted.
	if seq := canvasSeq(t, theme.Dark); strings.Contains(footer, seq) {
		t.Errorf("colourless footer still paints the canvas background sequence %q", seq)
	}
	// No foreground hue from any footer role (incl. the ? violet and the rule).
	for _, tok := range []theme.Token{
		theme.MV.AccentBlue, theme.MV.TextDetail, theme.MV.AccentViolet, theme.MV.BorderFooter,
	} {
		if seq := tokenFgSeq(t, tok, theme.Dark); strings.Contains(footer, seq) {
			t.Errorf("colourless footer still emits a foreground role sequence %q", seq)
		}
	}
}

// TestSessionsFooter_PaintsCanvasNoEdgeBleed asserts the footer cells carry the
// owned canvas background (leaf .Background(canvas)) so the right-aligned spacer
// gap is not a terminal-bg island.
func TestSessionsFooter_PaintsCanvasNoEdgeBleed(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		footer := renderSessionsFooter(referenceFooterWidth, mode, false)
		if seq := canvasSeq(t, mode); !strings.Contains(footer, seq) {
			t.Errorf("footer does not paint the canvas background sequence %q:\n%s", seq, footer)
		}
	}
}

// TestSessionsFooter_NarrowTruncationNoWrap asserts §2.7: below the width at which
// the full row fits, the footer truncates gracefully (drops lower-priority Core
// entries, marks the drop with an ellipsis) on ONE line — it never wraps to a
// second key row (which would steal a list row), never overflows, and the ? help
// right anchor survives as long as possible.
func TestSessionsFooter_NarrowTruncationNoWrap(t *testing.T) {
	for _, w := range []int{90, 76, 60, 40, 24, 12} {
		footer := renderSessionsFooter(w, theme.Dark, false)
		lines := strings.Split(footer, "\n")
		// Always exactly 2 rows: the rule + ONE key row. Never a third (wrapped) row.
		if len(lines) != 2 {
			t.Errorf("at width %d the footer has %d rows, want 2 (rule + single key row, no wrap):\n%s", w, len(lines), footer)
			continue
		}
		// No line overflows the width.
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("at width %d, footer line %d width = %d (overflow)", w, i, lw)
			}
		}
		// The ? help anchor survives as long as it fits beside at least one entry +
		// the ellipsis marker. At the moderately narrow widths the anchor must remain.
		if w >= 24 {
			if !strings.Contains(footerVisible(lines[1]), "? help") {
				t.Errorf("at width %d the ? help right anchor was dropped too early:\n%s", w, footerVisible(lines[1]))
			}
		}
	}
}

// TestSessionsFooter_NarrowTruncationKeepsHighestPriority asserts the §2.7 drop is
// priority-ordered: the highest-priority leading entries (navigate, attach…)
// survive while the lower-priority trailing entries (x projects, then s switch
// view…) drop first, with an ellipsis marking the truncation.
func TestSessionsFooter_NarrowTruncationKeepsHighestPriority(t *testing.T) {
	// A width that fits a few leading entries + the ? help but not the full cluster.
	footer := footerVisible(renderSessionsFooter(60, theme.Dark, false))
	if !strings.Contains(footer, "↑↓ navigate") {
		t.Errorf("highest-priority entry 'navigate' must survive truncation:\n%s", footer)
	}
	if !strings.Contains(footer, "…") {
		t.Errorf("a truncated footer must carry the ellipsis drop marker:\n%s", footer)
	}
	if !strings.Contains(footer, "? help") {
		t.Errorf("the ? help right anchor must survive at width 60:\n%s", footer)
	}
	// The lowest-priority trailing entry must have dropped at this width.
	if strings.Contains(footer, "x projects") {
		t.Errorf("lowest-priority entry 'x projects' should have dropped first at width 60:\n%s", footer)
	}
}

// TestSessionsFooterHeight_SubtractedFromListBudget asserts the single-row footer
// height (rule + key row) is folded into the list size budget at every Sessions
// SetSize site so pagination stays exact: the composed Sessions view never exceeds
// termH, and the filled frame is exactly termH (no overflow class regression).
func TestSessionsFooterHeight_SubtractedFromListBudget(t *testing.T) {
	const w, h = 120, 24
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)

	footerH := m.sessionFooterHeight(m.contentWidth())
	if footerH != 2 {
		t.Errorf("condensed footer height = %d, want 2 (1px rule + single key row)", footerH)
	}

	// The composed Sessions view (header + list + footer) must never exceed termH.
	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("composed Sessions view height = %d, want <= %d (footer folded into budget)", got, h)
	}
	// And the full filled frame is exactly termH.
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("filled frame height = %d, want exactly %d", got, h)
	}
}

// TestSessionsFooterHeight_CountedAtEverySizeApplySite asserts the construction
// seed, a window-resize, and a rebuild all reserve the condensed footer height —
// the composed view stays within termH and the frame fills exactly termH after
// each path (the three Sessions applySessionListSize call sites).
func TestSessionsFooterHeight_CountedAtEverySizeApplySite(t *testing.T) {
	const w, h = 120, 20
	var sessions []tmux.Session
	for i := range 60 {
		sessions = append(sessions, tmux.Session{Name: nameN(i), Windows: 1})
	}

	// Construction seed (New → applySessionListSize(80,24)) then a resize, then a
	// rebuild.
	m := New(fakeLister{}, WithCanvasMode(theme.Dark))
	updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = updated.(Model)
	m.applySessions(sessions)

	if got := lipgloss.Height(m.viewSessionList()); got > h {
		t.Errorf("after resize+rebuild composed view height = %d, want <= %d", got, h)
	}
	if got := lipgloss.Height(m.View().Content); got != h {
		t.Errorf("after resize+rebuild filled frame height = %d, want exactly %d", got, h)
	}
}

// TestSessionsFooter_OmittedKeysStillDispatchable asserts the footer swap is
// display-only: the keys NOT shown in the condensed footer (k kill, r rename, n
// new in cwd, q quit) still dispatch through updateSessionList exactly as before.
func TestSessionsFooter_OmittedKeysStillDispatchable(t *testing.T) {
	t.Run("k still opens the kill confirm modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionKiller = keymapParityKiller{}
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: 'k', Text: "k"})
		if updated.(Model).modal != modalKillConfirm {
			t.Errorf("k must still open the kill modal after the footer swap")
		}
	})
	t.Run("r still opens the rename modal", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		m.sessionRenamer = keymapParityRenamer{}
		updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: 'r', Text: "r"})
		if updated.(Model).modal != modalRename {
			t.Errorf("r must still open the rename modal after the footer swap")
		}
	})
	t.Run("q still quits", func(t *testing.T) {
		m := NewModelWithSessions([]tmux.Session{{Name: "alpha", Windows: 1}})
		_, cmd := m.updateSessionList(tea.KeyPressMsg{Code: 'q', Text: "q"})
		if cmd == nil {
			t.Fatalf("q must still produce a quit cmd after the footer swap")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("q must still quit after the footer swap")
		}
	})
}
