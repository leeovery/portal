package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/tmux"
)

func markedSet(names ...string) map[string]struct{} {
	set := make(map[string]struct{}, len(names))
	for _, n := range names {
		set[n] = struct{}{}
	}
	return set
}

func TestSessionRow_MarkedShowsVioletBulletInLeftBar(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)

	marked := renderRow(d, 80, items, 0, 1)
	if col := visibleColOf(marked, multiSelectMarker); col != 0 {
		t.Errorf("marked row ● should sit at the left-bar col 0, got col %d: %q", col, ansi.Strip(marked))
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); !strings.Contains(marked, seq) {
		t.Errorf("marked ● missing accent.primary fg %q: %q", seq, escSeq(marked))
	}

	unmarked := renderRow(d, 80, items, 1, 0)
	if strings.Contains(ansi.Strip(unmarked), multiSelectMarker) {
		t.Errorf("unmarked, unattached row must render no ●: %q", ansi.Strip(unmarked))
	}
}

func TestSessionRow_NoBulletWhenMultiSelectFalse(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: false, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	out := renderRow(d, 80, items, 0, 1)
	if strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("MultiSelect==false must render no ● even for a set member: %q", ansi.Strip(out))
	}
}

func TestSessionRow_CursorRowMarkedShowsBandAndBullet(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(tmux.Session{Name: "alpha", Windows: 2, Attached: false})

	out := renderRow(d, 80, items, 0, 0)

	if col := visibleColOf(out, multiSelectMarker); col != 0 {
		t.Errorf("cursor+marked row ● should sit at left-bar col 0, got col %d: %q", col, ansi.Strip(out))
	}
	if strings.Contains(ansi.Strip(out), selectorBar) {
		t.Errorf("cursor+marked row must render ● not the ▌ selector: %q", ansi.Strip(out))
	}
	if params := selectionBgParams(t, testDarkTheme(t)); !lineHasBgParams(out, params) {
		t.Errorf("cursor+marked row missing the bg.selection tint %q: %q", params, escSeq(out))
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); !strings.Contains(out, seq) {
		t.Errorf("cursor+marked ● missing accent.primary fg %q: %q", seq, escSeq(out))
	}
}

func TestSessionRow_MarkedAlignmentByteUnchanged(t *testing.T) {
	const w = 80
	sess := tmux.Session{Name: "alpha", Windows: 3, Attached: true}
	items := flatItems(sess, tmux.Session{Name: "bravo", Windows: 1})

	marked := renderRow(SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("alpha")}, w, items, 0, 1)
	unmarked := renderRow(SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("bravo")}, w, items, 0, 1)

	for _, sub := range []string{"alpha", "window", "windows" + strings.Repeat(" ", 2+indicatorSlotWidth-1) + rowIndicatorGlyph} {
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

func TestSessionRow_ByTagMarkedBulletOnEveryRow(t *testing.T) {
	sess := tmux.Session{Name: "portal-abc", Windows: 1, Attached: false}
	items := []list.Item{
		SessionItem{Session: sess, GroupKey: "work", GroupHeading: "work"},
		SessionItem{Session: sess, GroupKey: "infra", GroupHeading: "infra"},
	}
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("portal-abc")}

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

func TestSessionRow_HeaderNeverRendersBullet(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), MultiSelect: true, Selected: markedSet("work")}
	items := []list.Item{HeaderItem{Heading: "work", Count: 3, Key: "work"}}
	out := renderRow(d, 80, items, 0, 0)
	if strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("HeaderItem must never render a ●: %q", ansi.Strip(out))
	}
}

func TestMultiSelectMarkerReflectsSetLive(t *testing.T) {
	m := NewModelWithSessions([]tmux.Session{
		{Name: "alpha", Windows: 1, Attached: false},
		{Name: "bravo", Windows: 2, Attached: false},
	})

	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Fatalf("precondition: no ● should render before multi-select mode")
	}

	m = pressSession(t, m, pressM)
	if !m.IsSessionSelected("alpha") {
		t.Fatalf("precondition: alpha should be marked after entering")
	}
	if !strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("marking a session must render the ● (delegate not refreshed): %q", ansi.Strip(m.sessionList.View()))
	}

	updated, _ := m.updateSessionList(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = updated.(Model)
	if strings.Contains(ansi.Strip(m.sessionList.View()), multiSelectMarker) {
		t.Errorf("exiting multi-select must clear the ● from the rendered list: %q", ansi.Strip(m.sessionList.View()))
	}
}

func TestSessionRow_MarkedColourlessGlyphSurvivesNoHue(t *testing.T) {
	d := SessionDelegate{Theme: testDarkTheme(t), Colourless: true, MultiSelect: true, Selected: markedSet("alpha")}
	items := flatItems(
		tmux.Session{Name: "alpha", Windows: 1, Attached: false},
		tmux.Session{Name: "bravo", Windows: 1, Attached: false},
	)
	out := renderRow(d, 80, items, 0, 0)

	if !strings.Contains(ansi.Strip(out), multiSelectMarker) {
		t.Errorf("colourless marked row dropped the ● glyph: %q", ansi.Strip(out))
	}
	if seq := tokenFgSeq(t, testDarkTheme(t).AccentPrimary); strings.Contains(out, seq) {
		t.Errorf("colourless ● still emits the accent.primary fg %q: %q", seq, escSeq(out))
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(out, seq) {
		t.Errorf("colourless marked row still paints the canvas background %q: %q", seq, escSeq(out))
	}
	if params := selectionBgParams(t, testDarkTheme(t)); lineHasBgParams(out, params) {
		t.Errorf("colourless marked+selected row still carries the bg.selection tint %q: %q", params, escSeq(out))
	}
}
