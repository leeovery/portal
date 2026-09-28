package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tmux"
)

var (
	darkBg  = tea.BackgroundColorMsg{Color: color.RGBA{R: 0x0b, G: 0x0c, B: 0x14, A: 0xff}}
	lightBg = tea.BackgroundColorMsg{Color: color.RGBA{R: 0xe1, G: 0xe2, B: 0xe7, A: 0xff}}
)

func detectModel(t *testing.T, n theme.Nomination) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 2, Attached: false},
	}
	m := Build(Deps{Lister: fakeLister{}, Theme: n})
	m.termWidth = 90
	m.termHeight = 24
	m.applySessions(sessions, nil)
	return m
}

func assertBlankFrame(t *testing.T, m Model) {
	t.Helper()
	if m.modeResolved() {
		t.Fatalf("modeResolved = true, want false (model must be unresolved for a blank frame)")
	}
	if got := m.View().Content; strings.Contains(got, "Sessions") {
		t.Errorf("pre-resolution View painted real content (found 'Sessions' title); want a neutral blank frame")
	}
}

// Adaptive-only: a constant's in-force member stays at the dark zero value
// whatever palette it paints — use assertActiveTheme there.
func assertPaintedCanvas(t *testing.T, m Model, appearance theme.Member) {
	t.Helper()
	if !m.modeResolved() {
		t.Fatalf("modeResolved = false, want true (the canvas must be resolved before the real paint)")
	}
	if m.themeState.inForceMode() != appearance {
		t.Errorf("inForceMode() = %v, want %v", m.themeState.inForceMode(), appearance)
	}
	view := m.View().Content
	if !strings.Contains(view, "Sessions") {
		t.Errorf("resolved View did not paint the real content (no 'Sessions' title)")
	}
	if seq := canvasSeq(t, themeForAppearance(t, appearance)); !strings.Contains(view, seq) {
		t.Errorf("resolved View does not contain the %v canvas background sequence %q", appearance, seq)
	}
}

func themeForAppearance(t *testing.T, appearance theme.Member) theme.Theme {
	t.Helper()
	if appearance == theme.MemberLight {
		return testLightTheme(t)
	}
	return testDarkTheme(t)
}

func TestUnresolvedGateCarriesDarkFallback(t *testing.T) {
	var bare appearanceGate
	if bare.appearance != theme.MemberDark {
		t.Errorf("zero appearanceGate answer = %v, want %v", bare.appearance, theme.MemberDark)
	}

	g := newNominationGate(testBuiltinPair(t))
	g.arm()

	if g.resolved() {
		t.Fatalf("an armed adaptive gate reports resolved, want an open window")
	}
	if g.appearance != theme.MemberDark {
		t.Errorf("unresolved gate answer = %v, want %v (the no-answer fallback)", g.appearance, theme.MemberDark)
	}

	m := detectModel(t, testBuiltinPair(t))
	if m.themeState.inForceMode() != theme.MemberDark {
		t.Errorf("unresolved model inForceMode() = %v, want %v", m.themeState.inForceMode(), theme.MemberDark)
	}
}

func TestAdaptiveDetectsDark(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))
	assertBlankFrame(t, m)

	updated, _ := m.Update(darkBg)
	assertPaintedCanvas(t, updated.(Model), theme.MemberDark)
}

func TestAdaptiveDetectsLight(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))
	assertBlankFrame(t, m)

	updated, _ := m.Update(lightBg)
	assertPaintedCanvas(t, updated.(Model), theme.MemberLight)
}

func TestNoPaintThenFlip(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))
	assertBlankFrame(t, m)

	updated, _ := m.Update(darkBg)
	resolved := updated.(Model)
	assertPaintedCanvas(t, resolved, theme.MemberDark)

	after, _ := resolved.Update(appearanceTimeoutMsg{})
	if after.(Model).themeState.inForceMode() != theme.MemberDark {
		t.Errorf("a late timeout flipped the answer in force to %v, want it pinned at the dark canvas (no second resolution)", after.(Model).themeState.inForceMode())
	}

	after2, _ := after.(Model).Update(lightBg)
	if after2.(Model).themeState.inForceMode() != theme.MemberDark {
		t.Errorf("a late light BackgroundColorMsg flipped the answer in force to %v, want it pinned at the dark canvas (no flip)", after2.(Model).themeState.inForceMode())
	}
}

func TestTimeoutFallsBackToDark(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))
	assertBlankFrame(t, m)

	updated, _ := m.Update(appearanceTimeoutMsg{})
	assertPaintedCanvas(t, updated.(Model), theme.MemberDark)
}

func TestColorFGBGNeverOverridesOSC11(t *testing.T) {
	t.Setenv("COLORFGBG", "0;15") // fg black on bg white → "light" by the weak hint
	m := detectModel(t, testBuiltinPair(t))

	updated, _ := m.Update(darkBg)
	assertPaintedCanvas(t, updated.(Model), theme.MemberDark)
}

func TestMisdetectionLegibleNotBroken(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))

	updated, _ := m.Update(lightBg)
	resolved := updated.(Model)
	assertPaintedCanvas(t, resolved, theme.MemberLight)
	view := resolved.View().Content
	if strings.TrimSpace(view) == "" {
		t.Errorf("mis-detected canvas rendered blank, want a legible (wrong-mode) screen")
	}
}

func assertNoTimeoutTick(t *testing.T, m Model) {
	t.Helper()
	for _, msg := range initCmds(t, m.Init()) {
		if _, ok := msg.(appearanceTimeoutMsg); ok {
			t.Errorf("Init armed a detection timeout on a constant nomination, want none (the wait is skipped)")
		}
	}
}

func TestAdaptiveArmsTimeoutTick(t *testing.T) {
	m := detectModel(t, testBuiltinPair(t))
	found := false
	for _, msg := range initCmds(t, m.Init()) {
		if _, ok := msg.(appearanceTimeoutMsg); ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Init did not arm the detect-or-timeout tick under an adaptive pair (the no-answer fallback would never fire)")
	}
}
