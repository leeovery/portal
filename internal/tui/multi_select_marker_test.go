package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// The §5 multi-select marker gate. These tests pin the selection ● the delegate
// draws in the fixed 2-cell left-bar column for a MARKED row: violet on the dark
// canvas, carried on the bg.selection tint when the row is also the cursor,
// glyph-only under NO_COLOR, and NEVER shifting the name/count/attached columns
// (the ● occupies the SAME 2-cell column the ▌ selector would). The set is keyed
// on Session.Name, so a By-Tag multi-tag session shows the ● on every one of its
// rows, and a HeaderItem never carries one.
//
// No t.Parallel() — the package-level mock convention and shared canvas helpers
// make parallelism unsafe across this package's tests.

// markedSet builds a Selected set from the given session names.
func markedSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

// TestSessionRow_MarkedShowsVioletBulletInLeftBar asserts a marked (multi-select)
// row renders the ● at the far-left of the fixed 2-cell left-bar column in
// accent.violet (dark canvas), while an unmarked row in the same set renders no ●.
func TestSessionRow_MarkedShowsVioletBulletInLeftBar(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)

	// alpha is marked; render it unselected (cursor parked on bravo).
	marked := renderRow(d, 80, items, 0, 1)
	if col := visibleColOf(marked, multiSelectMarker); col != 0 {
		t.Errorf("marked row ● should sit at the left-bar col 0, got col %d: %q", col, ansi.Strip(marked))
	}
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); !strings.Contains(marked, seq) {
		t.Errorf("marked ● missing accent.violet fg %q: %q", seq, escSeq(marked))
	}

	// bravo is NOT marked and unattached; render it unselected — no ● anywhere.
	unmarked := renderRow(d, 80, items, 1, 0)
	if strings.Contains(ansi.Strip(unmarked), multiSelectMarker) {
		t.Errorf("unmarked, unattached row must render no ●: %q", ansi.Strip(unmarked))
	}
}

// TestSessionRow_NoBulletWhenMultiSelectFalse asserts the marker is gated on the
// MultiSelect flag: a populated Selected set with MultiSelect == false renders no
// ● (the mode is off, so the set is inert).
func TestSessionRow_NoBulletWhenMultiSelectFalse(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: false, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	// alpha is in the set but MultiSelect is off; render it unselected.
	out := renderRow(d, 80, items, 0, 1)
	if strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("MultiSelect==false must render no ● even for a set member: %q", ansi.Strip(out))
	}
}

// TestSessionRow_CursorRowMarkedShowsBandAndBullet asserts the cursor+marked row
// renders BOTH the bg.selection band AND the ● (the marker takes precedence over
// the ▌ selector in the shared left-bar column, and its violet is carried on the
// selection tint).
func TestSessionRow_CursorRowMarkedShowsBandAndBullet(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Attached: false})

	// Cursor ON alpha (selected) AND alpha is marked.
	out := renderRow(d, 80, items, 0, 0)

	if col := visibleColOf(out, multiSelectMarker); col != 0 {
		t.Errorf("cursor+marked row ● should sit at left-bar col 0, got col %d: %q", col, ansi.Strip(out))
	}
	// The ● supersedes the ▌ selector in this column.
	if strings.Contains(ansi.Strip(out), selectorBar) {
		t.Errorf("cursor+marked row must render ● not the ▌ selector: %q", ansi.Strip(out))
	}
	// The bg.selection band still spans the row.
	if params := selectionBgParams(t, theme.Dark); !lineHasBgParams(out, params) {
		t.Errorf("cursor+marked row missing the bg.selection tint %q: %q", params, escSeq(out))
	}
	// The ● is violet.
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); !strings.Contains(out, seq) {
		t.Errorf("cursor+marked ● missing accent.violet fg %q: %q", seq, escSeq(out))
	}
}

// TestSessionRow_MarkedAlignmentByteUnchanged is the §3.5 / §4.1 no-width-shift
// gate: a marked and an unmarked row (same session, same width) place the name,
// the window count, and the attached marker at the SAME columns, and the rows are
// the same total width — the ● occupies the same 2-cell column the blank/▌ left
// bar would, with no downstream shift.
func TestSessionRow_MarkedAlignmentByteUnchanged(t *testing.T) {
	const w = 80
	sess := tmux.Session{Name: "alpha", Windows: 3, Attached: true}
	items := flatItems(sess, tmux.Session{Name: "bravo", Windows: 1})

	marked := renderRow(SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("alpha")}, w, items, 0, 1)
	unmarked := renderRow(SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("bravo")}, w, items, 0, 1)

	for _, sub := range []string{"alpha", "window", "attached"} {
		mc, uc := visibleColOf(marked, sub), visibleColOf(unmarked, sub)
		if mc < 0 || uc < 0 {
			t.Fatalf("column %q missing: marked=%q unmarked=%q", sub, ansi.Strip(marked), ansi.Strip(unmarked))
		}
		if mc != uc {
			t.Errorf("column %q shifted by the ●: marked col %d, unmarked col %d", sub, mc, uc)
		}
	}
	if mw, uw := lipgloss.Width(marked), lipgloss.Width(unmarked); mw != uw || mw != w {
		t.Errorf("row width changed by the ●: marked=%d unmarked=%d, want %d", mw, uw, w)
	}
}

// TestSessionRow_ByTagMarkedBulletOnEveryRow asserts the By-Tag multi-membership
// case: a session that spans several grouped rows (all sharing Session.Name) is
// marked on that single name, so EVERY one of its rows renders the ●.
func TestSessionRow_ByTagMarkedBulletOnEveryRow(t *testing.T) {
	sess := tmux.Session{Name: "portal-abc", Windows: 1, Attached: false}
	items := []list.Item{
		SessionItem{Session: sess, GroupKey: "work", GroupHeading: "work"},
		SessionItem{Session: sess, GroupKey: "infra", GroupHeading: "infra"},
	}
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("portal-abc")}

	// Render each row unselected (cursor parked on the other row).
	for _, tc := range []struct{ index, sel int }{{0, 1}, {1, 0}} {
		out := renderRow(d, 80, items, tc.index, tc.sel)
		bulletCol := visibleColOf(out, multiSelectMarker)
		nameCol := visibleColOf(out, "portal-abc")
		if bulletCol < 0 {
			t.Errorf("By-Tag row %d of a marked multi-tag session missing the ●: %q", tc.index, ansi.Strip(out))
			continue
		}
		if bulletCol >= nameCol {
			t.Errorf("By-Tag row %d ● (col %d) must sit left of the name (col %d): %q", tc.index, bulletCol, nameCol, ansi.Strip(out))
		}
	}
}

// TestSessionRow_HeaderNeverRendersBullet asserts a HeaderItem never carries a ●,
// even while the delegate is in multi-select mode (the header render arm is
// untouched by the marker logic).
func TestSessionRow_HeaderNeverRendersBullet(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, MultiSelect: true, Selected: markedSet("work")}
	items := []list.Item{HeaderItem{Heading: "work", Count: 3, Key: "work"}}
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("HeaderItem must never render a ●: %q", ansi.Strip(out))
	}
}

// TestMultiSelectMarkerReflectsSetLive is the model-level wiring gate: the
// session delegate must track the multi-select set live, so marking a session
// through the `m` toggle shows the ● in the rendered list on the next frame, and
// Esc-exiting clears it. It proves applyCanvasMode/refreshSessionDelegate feed the
// current multiSelectMode + selectedSessions into the delegate (the default
// SessionDelegate the list is constructed with has MultiSelect == false).
func TestMultiSelectMarkerReflectsSetLive(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	})

	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Fatalf("precondition: no ● should render before multi-select mode")
	}

	// Enter mode, then toggle the highlighted row (alpha) ON.
	m = pressSession(t, m, pressM)
	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("alpha") {
		t.Fatalf("precondition: alpha should be marked after the toggle")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("marking a session must render the ● (delegate not refreshed): %q", ansi.Strip(m.sessionList.View()))
	}

	// Esc exits and clears the set; the ● must disappear.
	updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("exiting multi-select must clear the ● from the rendered list: %q", ansi.Strip(m.sessionList.View()))
	}
}

// TestSessionRow_MarkedColourlessGlyphSurvivesNoHue asserts the NO_COLOR carve-out
// (§2.5): a marked row under Colourless keeps the ● glyph (structurally present)
// but emits NO violet hue and NO canvas/selection background — glyph-backed, never
// colour-only.
func TestSessionRow_MarkedColourlessGlyphSurvivesNoHue(t *testing.T) {
	d := SessionDelegate{Mode: theme.Dark, Colourless: true, MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	// Marked + selected (cursor on alpha): the glyph must still render, with no hue
	// and no band.
	out := renderRow(d, 80, items, 0, 0)

	if !strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("colourless marked row dropped the ● glyph: %q", ansi.Strip(out))
	}
	if seq := tokenFgSeq(t, theme.MV.AccentViolet, theme.Dark); strings.Contains(out, seq) {
		t.Errorf("colourless ● still emits the accent.violet fg %q: %q", seq, escSeq(out))
	}
	if seq := canvasSeq(t, theme.Dark); strings.Contains(out, seq) {
		t.Errorf("colourless marked row still paints the canvas background %q: %q", seq, escSeq(out))
	}
	if params := selectionBgParams(t, theme.Dark); lineHasBgParams(out, params) {
		t.Errorf("colourless marked+selected row still carries the bg.selection tint %q: %q", params, escSeq(out))
	}
}
