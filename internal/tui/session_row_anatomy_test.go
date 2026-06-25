package tui

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §4.1 flat-row anatomy gate. These tests pin the restyled SessionDelegate
// row: a 2-cell violet left-bar column on the selected row over a bg.selection
// tint, the name as a flexing left column, and FIXED-WIDTH right-pinned trailing
// slots for the window count and the attached marker. Colour roles are asserted
// with exact mode-resolved SGR sequences (like the footer / canvas tests), so a
// token swap is caught, not merely the presence of the glyph.
//
// No t.Parallel() — the cmd-style package-level mock convention and the shared
// canvas helpers make parallelism unsafe across this package's tests.

// renderRow renders one session row at the given list width with the cursor on
// selIndex, then returns the styled string the delegate emitted for `index`.
func renderRow(d SessionDelegate, width int, items []list.Item, index, selIndex int) string {
	m := list.New(items, d, width, 10)
	m.Select(selIndex)
	var buf bytes.Buffer
	d.Render(&buf, m, index, items[index])
	return buf.String()
}

// visibleColOf returns the printable COLUMN (not byte offset) at which `sub`
// first appears in the ANSI-stripped line, or -1 when absent. It measures the
// display width of the prefix so a multibyte glyph (the ▌ bar, the ● bullet)
// before `sub` doesn't skew the column — column alignment across rows is the
// §4.1 invariant (counts + bullets line up regardless of name length).
func visibleColOf(line, sub string) int {
	stripped := ansi.Strip(line)
	before, _, ok := strings.Cut(stripped, sub)
	if !ok {
		return -1
	}
	return ansi.StringWidth(before)
}

// selectionBgParams returns the raw background-parameter form (e.g.
// "48;2;208;198;240") that the mode's bg.selection tint renders as, derived from
// lipgloss so the test pins the SAME bytes production paints.
func selectionBgParams(t *testing.T, m theme.Mode) string {
	t.Helper()
	probe := lipgloss.NewStyle().Background(theme.MV.BgSelection.ColorFor(m)).Render(" ")
	inner := strings.TrimSuffix(strings.TrimPrefix(probe[:strings.IndexByte(probe, ' ')], "\x1b["), "m")
	if inner == "" {
		t.Fatalf("could not derive bg.selection params from %q", probe)
	}
	return inner
}

// flatItems builds a flat (no group metadata) item slice from name/window/attached
// triples.
func flatItems(specs ...tmux.Session) []list.Item {
	items := make([]list.Item, len(specs))
	for i, s := range specs {
		items[i] = SessionItem{Session: s}
	}
	return items
}

// TestSessionRow_FlexNameFixedTrailingSlots asserts the row is laid out as a
// flexing name column with fixed-width window-count and attached-marker trailing
// slots: the name, the count text, and the attached marker all print, and the
// count is positioned to the RIGHT of the name (a trailing slot, not abutting).
func TestSessionRow_FlexNameFixedTrailingSlots(t *testing.T) {
	const w = 80
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
	out := renderRow(SessionDelegate{}, w, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "alpha") {
		t.Errorf("row missing name 'alpha': %q", vis)
	}
	if !strings.Contains(vis, "3 windows") {
		t.Errorf("row missing window count '3 windows': %q", vis)
	}
	if !strings.Contains(vis, "● attached") {
		t.Errorf("row missing attached marker '● attached': %q", vis)
	}
	nameCol := visibleColOf(out, "alpha")
	countCol := visibleColOf(out, "3 windows")
	if countCol <= nameCol {
		t.Errorf("count slot (col %d) must sit right of the name (col %d): %q", countCol, nameCol, vis)
	}
	// The whole row is exactly the list width (the fixed trailing slots are
	// right-pinned to the list's right edge).
	if got := lipgloss.Width(out); got != w {
		t.Errorf("row width = %d, want exactly %d (trailing slots right-pinned to the list width)", got, w)
	}
}

// TestSessionRow_ColumnAlignsRegardlessOfNameLength asserts the §4.1 invariant:
// the window counts AND the attached bullets are vertically column-aligned across
// rows whose names differ wildly in length, because the trailing slots are
// fixed-width and right-pinned.
func TestSessionRow_ColumnAlignsRegardlessOfNameLength(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "a", Windows: 1, Attached: true},
		tmux.Session{Name: "a-much-longer-session-name-here", Windows: 5, Attached: true},
	)
	// Cursor parked off both rows so neither is the selected (tinted) row.
	short := renderRow(SessionDelegate{}, w, items, 0, 0)
	long := renderRow(SessionDelegate{}, w, items, 1, 0)

	shortCount := visibleColOf(short, "window")
	longCount := visibleColOf(long, "window")
	if shortCount < 0 || longCount < 0 {
		t.Fatalf("a count column is missing: short=%q long=%q", ansi.Strip(short), ansi.Strip(long))
	}
	if shortCount != longCount {
		t.Errorf("window counts not column-aligned: short name count col %d, long name count col %d", shortCount, longCount)
	}

	shortBullet := visibleColOf(short, "●")
	longBullet := visibleColOf(long, "●")
	if shortBullet < 0 || longBullet < 0 {
		t.Fatalf("an attached bullet is missing: short=%q long=%q", ansi.Strip(short), ansi.Strip(long))
	}
	if shortBullet != longBullet {
		t.Errorf("attached bullets not column-aligned: short col %d, long col %d", shortBullet, longBullet)
	}
}

// TestSessionRow_EmptyAttachedSlotPreservesAlignment asserts that an UNATTACHED
// row renders an empty attached slot of the SAME width as the marker — so the
// window counts of an attached and an unattached row stay column-aligned (the
// slot is reserved, not omitted).
func TestSessionRow_EmptyAttachedSlotPreservesAlignment(t *testing.T) {
	const w = 80
	items := flatItems(
		tmux.Session{Name: "attached-one", Windows: 2, Attached: true},
		tmux.Session{Name: "detached-one", Windows: 2, Attached: false},
	)
	attached := renderRow(SessionDelegate{}, w, items, 0, 0)
	detached := renderRow(SessionDelegate{}, w, items, 1, 0)

	// The unattached row carries no marker text.
	if strings.Contains(ansi.Strip(detached), "attached") {
		t.Errorf("unattached row must not render the attached marker: %q", ansi.Strip(detached))
	}
	// But its count column aligns with the attached row's (the slot is reserved).
	if a, d := visibleColOf(attached, "window"), visibleColOf(detached, "window"); a != d {
		t.Errorf("count columns misaligned across attached/unattached: %d vs %d", a, d)
	}
	// And both rows are the same total width (the empty slot is the same width).
	if a, d := lipgloss.Width(attached), lipgloss.Width(detached); a != d {
		t.Errorf("row widths differ across attached/unattached: %d vs %d (empty slot must match marker width)", a, d)
	}
}

// TestSessionRow_SelectedShowsVioletBarTintAndOnSelectionName asserts the §3.3
// selection treatment in exact SGR: the selected row carries a violet ▌ bar
// (accent.violet foreground), every structural cell tints with bg.selection, and
// the name renders in text.on-selection.
func TestSessionRow_SelectedShowsVioletBarTintAndOnSelectionName(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		items := flatItems(tmux.Session{Name: "selected-row", Windows: 2, Attached: false})
		out := renderRow(d, 80, items, 0, 0)

		// The ▌ selector bar glyph is present and rendered in accent.violet.
		if !strings.Contains(ansi.Strip(out), "▌") {
			t.Errorf("[%v] selected row missing the ▌ selector bar: %q", mode, ansi.Strip(out))
		}
		if seq := tokenFgSeq(t, theme.MV.AccentViolet, mode); !strings.Contains(out, seq) {
			t.Errorf("[%v] selected bar missing accent.violet fg %q", mode, seq)
		}
		// The row is tinted with bg.selection (not the plain canvas).
		if params := selectionBgParams(t, mode); !lineHasBgParams(out, params) {
			t.Errorf("[%v] selected row missing the bg.selection tint %q: %q", mode, params, escSeq(out))
		}
		// The name renders in text.on-selection.
		if seq := tokenFgSeq(t, theme.MV.TextOnSelection, mode); !strings.Contains(out, seq) {
			t.Errorf("[%v] selected name missing text.on-selection fg %q", mode, seq)
		}
	}
}

// TestSessionRow_UnselectedHasNoBarOrTint asserts the negative of §3.3: an
// unselected row carries neither the ▌ bar nor the bg.selection tint — it paints
// on the plain canvas.
func TestSessionRow_UnselectedHasNoBarOrTint(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		items := flatItems(
			tmux.Session{Name: "row-zero", Windows: 1, Attached: false},
			tmux.Session{Name: "row-one", Windows: 1, Attached: false},
		)
		// Render row 1 while the cursor is on row 0 → row 1 is unselected.
		out := renderRow(d, 80, items, 1, 0)

		if strings.Contains(ansi.Strip(out), "▌") {
			t.Errorf("[%v] unselected row must not carry the ▌ bar: %q", mode, ansi.Strip(out))
		}
		if params := selectionBgParams(t, mode); lineHasBgParams(out, params) {
			t.Errorf("[%v] unselected row must not carry the bg.selection tint %q: %q", mode, params, escSeq(out))
		}
		// It does paint the canvas.
		if params := wantCanvasBgParams(t, mode); !lineHasBgParams(out, params) {
			t.Errorf("[%v] unselected row missing the canvas paint %q: %q", mode, params, escSeq(out))
		}
	}
}

// TestSessionRow_AttachedKeepsStateGreenWhenSelected is the §4.1 attached-marker
// colour gate: the ● attached marker renders in the SINGLE state.green token on
// BOTH the selected and the unselected row (the marker keeps state.green on the
// selected row — it is NOT recoloured to text.on-selection). The light state.green
// (#3B5E18) clears the contrast floor against the bg.selection tint as well as the
// canvas (the numeric floor is gated in theme/contrast_test.go), so no per-context
// on-selection override is needed.
func TestSessionRow_AttachedKeepsStateGreenWhenSelected(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		items := flatItems(
			tmux.Session{Name: "attached-selected", Windows: 1, Attached: true},
			tmux.Session{Name: "attached-unselected", Windows: 1, Attached: true},
		)

		green := tokenFgSeq(t, theme.MV.StateGreen, mode)
		onSelName := tokenFgSeq(t, theme.MV.TextOnSelection, mode)

		// Selected row (cursor on row 0): attached marker in state.green, NOT
		// recoloured to the text.on-selection name colour.
		sel := renderRow(d, 80, items, 0, 0)
		if !strings.Contains(sel, green) {
			t.Errorf("[%v] selected attached marker missing state.green fg %q", mode, green)
		}
		// The marker is its own green run, distinct from the name's text.on-selection.
		if mode == theme.Light && green == onSelName {
			t.Fatalf("[light] test precondition broken: state.green == text.on-selection")
		}

		// Unselected attached row (cursor on row 0, render row 1): same state.green.
		uns := renderRow(d, 80, items, 1, 0)
		if !strings.Contains(uns, green) {
			t.Errorf("[%v] unselected attached marker missing state.green fg %q", mode, green)
		}
	}
}

// TestSessionRow_SelectedCountInTextStrong asserts the §4.1 selected-row count
// role: the window count renders in text.strong on the selected row (text.detail
// on unselected).
func TestSessionRow_SelectedCountInTextStrong(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		items := flatItems(
			tmux.Session{Name: "row-zero", Windows: 4, Attached: false},
			tmux.Session{Name: "row-one", Windows: 4, Attached: false},
		)

		sel := renderRow(d, 80, items, 0, 0)
		uns := renderRow(d, 80, items, 1, 0)

		strong := tokenFgSeq(t, theme.MV.TextStrong, mode)
		detail := tokenFgSeq(t, theme.MV.TextDetail, mode)

		if !strings.Contains(sel, strong) {
			t.Errorf("[%v] selected-row count missing text.strong fg %q", mode, strong)
		}
		if !strings.Contains(uns, detail) {
			t.Errorf("[%v] unselected-row count missing text.detail fg %q", mode, detail)
		}
	}
}

// TestSessionRow_OverLongNameTruncatesWithoutPushingSlots asserts §2.7: an
// over-long name truncates with an ellipsis to the flex width so the fixed
// trailing slots are never pushed off-row — the count and attached marker still
// render and the row width never exceeds the list width.
func TestSessionRow_OverLongNameTruncatesWithoutPushingSlots(t *testing.T) {
	const w = 40 // narrow: a long name cannot fit beside the slots
	longName := "this-is-a-really-very-long-session-name-that-overflows"
	items := flatItems(tmux.Session{Name: longName, Windows: 7, Attached: true})
	out := renderRow(SessionDelegate{}, w, items, 0, 0)
	vis := ansi.Strip(out)

	if strings.Contains(vis, longName) {
		t.Errorf("over-long name should be truncated, but the full name rendered: %q", vis)
	}
	if !strings.Contains(vis, "…") {
		t.Errorf("truncated name should carry the ellipsis glyph: %q", vis)
	}
	// The trailing slots survive on-row.
	if !strings.Contains(vis, "7 windows") {
		t.Errorf("window-count slot pushed off-row by the long name: %q", vis)
	}
	if !strings.Contains(vis, "● attached") {
		t.Errorf("attached slot pushed off-row by the long name: %q", vis)
	}
	// The row never overflows the list width.
	if got := lipgloss.Width(out); got != w {
		t.Errorf("truncated row width = %d, want exactly %d (no overflow, slots right-pinned)", got, w)
	}
}

// TestSessionRow_NeverOverflowsAtNarrowWidths is the §3.5/§2.7 no-overflow guard at
// PATHOLOGICAL narrow widths. The trailing slots are a fixed 25 cells, so below ~26
// the flex name floors to 1 and the assembled row would otherwise be ~26 cells
// REGARDLESS of total — overflowing the list width and bleeding past the content
// gutter (corrupting the inset frame, the exact failure the one-row invariant
// prevents). The final-row truncate guard must clamp the row to total at ANY width.
// Exercised over a short and a long name, attached + unattached, in both modes.
func TestSessionRow_NeverOverflowsAtNarrowWidths(t *testing.T) {
	for _, w := range []int{1, 5, 10, 20, 25, 26, 29, 40, 80} {
		for _, sess := range []tmux.Session{
			{Name: "x", Windows: 1, Attached: false},
			{Name: "agentic-workflows-code-based-that-is-quite-long", Windows: 12, Attached: true},
		} {
			items := flatItems(sess)
			for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
				out := renderRow(SessionDelegate{Mode: mode}, w, items, 0, 0)
				if got := lipgloss.Width(out); got > w {
					t.Errorf("[w=%d %v %q] row width = %d, overflows the list width %d", w, mode, sess.Name, got, w)
				}
			}
		}
	}
}

// TestSessionRow_FlatIsNameOnly asserts the flat row shows the NAME only — no
// project/path column (that dimension is served by the grouping modes). The
// session's Dir must never leak into a flat row render.
func TestSessionRow_FlatIsNameOnly(t *testing.T) {
	items := flatItems(tmux.Session{
		Name:     "flat-name",
		Windows:  2,
		Attached: false,
		Dir:      "/home/user/code/some-project",
	})
	out := renderRow(SessionDelegate{}, 80, items, 0, 0)
	vis := ansi.Strip(out)

	if !strings.Contains(vis, "flat-name") {
		t.Errorf("flat row missing the name: %q", vis)
	}
	if strings.Contains(vis, "/home/user") || strings.Contains(vis, "some-project") {
		t.Errorf("flat row leaked the directory/path column: %q", vis)
	}
}

// TestSessionRow_NoRawAnsiColourLiterals asserts the delegate emits no run that
// opens the OLD scattered literals — pink ANSI-256 index 212, green index 76, and
// the grey hex #777777 — that the reskin removed. (The source-level guard is
// colour_literal_guard_test.go; this is the render-level cross-check.)
func TestSessionRow_NoRawAnsiColourLiterals(t *testing.T) {
	for _, mode := range []theme.Mode{theme.Dark, theme.Light} {
		d := SessionDelegate{Mode: mode}
		items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
		out := renderRow(d, 80, items, 0, 0)

		// The legacy ANSI-256 index colours render as "38;5;212" / "38;5;76".
		for _, banned := range []string{"38;5;212", "38;5;76", "48;5;212", "48;5;76"} {
			if strings.Contains(out, banned) {
				t.Errorf("[%v] delegate emitted a legacy ANSI-256 colour sequence %q: %q", mode, banned, escSeq(out))
			}
		}
		// The legacy grey hex #777777 == rgb(119,119,119) → "38;2;119;119;119".
		if strings.Contains(out, "38;2;119;119;119") {
			t.Errorf("[%v] delegate emitted the legacy #777777 grey: %q", mode, escSeq(out))
		}
	}
}

// TestSessionRow_HeightStaysOne re-pins the §3.5 / §4.1 one-delegate-line
// invariant for the restyled row: a SessionItem renders exactly one line, and the
// delegate Height stays 1.
func TestSessionRow_HeightStaysOne(t *testing.T) {
	d := SessionDelegate{}
	if d.Height() != 1 {
		t.Fatalf("Height() = %d, want 1", d.Height())
	}
	items := flatItems(tmux.Session{Name: "alpha", Windows: 3, Attached: true})
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(out, "\n") {
		t.Errorf("session row emitted more than one line: %q", out)
	}
}

// lineHasBgParams reports whether any printable cell on the line carries the
// given background parameters — reusing the canvas-cell SGR walker so the test is
// agnostic to how the sub-renderers emitted their SGR.
func lineHasBgParams(line, params string) bool {
	for _, c := range scanCellBackgrounds(line) {
		if c.set && c.params == params {
			return true
		}
	}
	return false
}

// escSeq renders ESC visibly for failure messages.
func escSeq(s string) string { return strings.ReplaceAll(s, "\x1b", "\\e") }
