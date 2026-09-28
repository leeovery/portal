package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

func colourlessTestModel(t *testing.T, w, h int) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 2, Attached: false},
	}
	m := Build(Deps{Lister: fakeLister{}, NoColor: true})
	m.termWidth = w
	m.termHeight = h
	m.applySessions(sessions)
	return m
}

func frameHasAnyBackgroundSGR(t *testing.T, frame string) bool {
	t.Helper()
	return frameEverActivates(t, frame, sgrBackgroundActive)
}

func frameHasAnyForegroundSGR(t *testing.T, frame string) bool {
	t.Helper()
	return frameEverActivates(t, frame, sgrForegroundActive)
}

func frameEverActivates(t *testing.T, frame string, fold func(bool, []string) bool) bool {
	t.Helper()
	parser := ansi.NewParser()
	src := []byte(frame)
	state := byte(0)
	active := false
	for len(src) > 0 {
		seq, _, n, newState := ansi.DecodeSequence(src, state, parser)
		s := string(seq)
		if strings.HasPrefix(s, "\x1b[") && strings.HasSuffix(s, "m") {
			active = fold(active, sgrParamsList(s))
			if active {
				return true
			}
		}
		state = newState
		src = src[n:]
	}
	return false
}

func sgrForegroundActive(active bool, params []string) bool {
	for i := 0; i < len(params); i++ {
		switch p := params[i]; {
		case p == "" || p == "0" || p == "39":
			active = false
		case p == "38":
			active = true
			i = consumeExtendedColorRun(params, i)
		case p == "48":
			i = consumeExtendedColorRun(params, i)
		case isNamedForeground(p):
			active = true
		}
	}
	return active
}

func isNamedForeground(p string) bool {
	switch p {
	case "30", "31", "32", "33", "34", "35", "36", "37",
		"90", "91", "92", "93", "94", "95", "96", "97":
		return true
	}
	return false
}

func TestColourless_SingleFlagFromDeps(t *testing.T) {
	t.Run("NoColor sets the colourless flag", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, NoColor: true})
		if !m.colourless {
			t.Errorf("Build(NoColor:true).colourless = false, want true")
		}
	})

	t.Run("without NoColor the flag is off", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}})
		if m.colourless {
			t.Errorf("Build(NoColor:false).colourless = true, want false")
		}
	})
}

func TestColourless_SkipsDetectionAndFirstPaintWait(t *testing.T) {
	m := Build(Deps{Lister: fakeLister{}, NoColor: true, Theme: testBuiltinPair(t)})

	if !m.modeResolved() {
		t.Errorf("colourless model is unresolved; want immediate resolution (no canvas to select, no first-paint wait)")
	}

	for _, msg := range initCmds(t, m.Init()) {
		if _, ok := msg.(appearanceTimeoutMsg); ok {
			t.Errorf("colourless Init armed a detect-or-timeout tick; want none (detection skipped)")
		}
	}
}

func TestColourless_ViewSetsNoBackgroundColor(t *testing.T) {
	m := colourlessTestModel(t, 90, 24)
	v := m.View()
	if v.BackgroundColor != nil {
		t.Errorf("colourless View.BackgroundColor = %v, want nil (no OSC 11 canvas set)", v.BackgroundColor)
	}
}

func TestColourless_FillEmitsNoCanvasBackground(t *testing.T) {
	m := colourlessTestModel(t, 90, 24)
	frame := m.View().Content
	if frameHasAnyBackgroundSGR(t, frame) {
		t.Errorf("colourless frame emits a background-colour SGR; want none (native bg, no painted canvas)")
	}
	if seq := canvasSeq(t, testDarkTheme(t)); strings.Contains(frame, seq) {
		t.Errorf("colourless frame contains the dark canvas background sequence %q", seq)
	}
	if seq := canvasSeq(t, testLightTheme(t)); strings.Contains(frame, seq) {
		t.Errorf("colourless frame contains the light canvas background sequence %q", seq)
	}
}

func TestColourless_NoTokenReachesTheWriter(t *testing.T) {
	frame := colourlessTestModel(t, 90, 24).View().Content

	if frameHasAnyForegroundSGR(t, frame) {
		t.Errorf("colourless frame emits a foreground-colour SGR; want none (no token may reach the writer)")
	}
	if frameHasAnyBackgroundSGR(t, frame) {
		t.Errorf("colourless frame emits a background-colour SGR; want none (no token may reach the writer)")
	}
}

func TestColourless_StateStaysGlyphDistinct(t *testing.T) {
	m := colourlessTestModel(t, 90, 24)
	frame := m.View().Content

	for _, want := range []string{"3 windows" + strings.Repeat(" ", 2+indicatorSlotWidth-1) + attachedIndicatorLetter, "alpha", "bravo", "charlie"} {
		if !strings.Contains(frame, want) {
			t.Errorf("colourless frame missing %q (state must stay glyph-distinct without colour)", want)
		}
	}
	if !strings.Contains(frame, "▌") {
		t.Errorf("colourless frame missing the selector bar glyph (state via glyph, not colour)")
	}
}

func TestColourless_StructureMatchesColouredFrame(t *testing.T) {
	const w, h = 90, 24
	m := colourlessTestModel(t, w, h)
	frame := m.View().Content

	if got := lipgloss.Height(frame); got != h {
		t.Errorf("colourless frame height = %d, want exactly %d (filled to termH)", got, h)
	}
	for i, line := range strings.Split(frame, "\n") {
		if lw := lipgloss.Width(line); lw != w {
			t.Errorf("colourless line %d width = %d, want exactly %d (padded to termW)", i, lw, w)
		}
	}
	if !strings.Contains(frame, "Sessions") {
		t.Errorf("colourless frame missing the 'Sessions' section header")
	}
}

func TestColourless_NavigationParity(t *testing.T) {
	m := colourlessTestModel(t, 90, 24)
	si, ok := m.selectedSessionItem()
	if !ok || si.Session.Name != "alpha" {
		t.Fatalf("initial selection = %q (ok=%v), want alpha", si.Session.Name, ok)
	}
	moved, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	si2, ok := moved.(Model).selectedSessionItem()
	if !ok || si2.Session.Name != "bravo" {
		t.Errorf("after down, selection = %q (ok=%v), want bravo (nav identical under NO_COLOR)", si2.Session.Name, ok)
	}
}

func TestColourless_FilterParity(t *testing.T) {
	colourless := colourlessTestModel(t, 90, 24)
	coloured := newCanvasTestModel(t, 90, 24, theme.MemberDark)

	colourless.SetSessionListFilter("charl")
	coloured.SetSessionListFilter("charl")

	if !colourless.sessionList.IsFiltered() {
		t.Fatalf("colourless filter did not apply")
	}
	cl, co := visibleSessionNames(colourless), visibleSessionNames(coloured)
	if !equalStrings(cl, co) {
		t.Errorf("colourless filtered rows %v != coloured filtered rows %v (filter parity broken)", cl, co)
	}
	if !equalStrings(cl, []string{"charlie"}) {
		t.Errorf("colourless applied filter rows = %v, want [charlie] (filter must narrow under NO_COLOR)", cl)
	}
	frame := colourless.View().Content
	if !strings.Contains(frame, "charlie") {
		t.Errorf("colourless filtered frame missing the matched row 'charlie'")
	}
	if strings.Contains(frame, "alpha") || strings.Contains(frame, "bravo") {
		t.Errorf("colourless filtered frame still shows a non-matching row (filter parity broken)")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestColourless_ColouredPathUnaffected(t *testing.T) {
	const w, h = 90, 24
	m := newCanvasTestModel(t, w, h, theme.MemberDark)
	v := m.View()
	if v.BackgroundColor == nil {
		t.Errorf("coloured View.BackgroundColor = nil, want the canvas colour set (coloured path must still paint)")
	}
	if seq := canvasSeq(t, testDarkTheme(t)); !strings.Contains(v.Content, seq) {
		t.Errorf("coloured frame missing the dark canvas background sequence %q (coloured path must still paint)", seq)
	}
}
