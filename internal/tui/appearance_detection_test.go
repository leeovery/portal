package tui

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/leeovery/portal/internal/tui/theme"
)

// darkBg / lightBg are deterministic OSC 11 BackgroundColorMsg payloads. The
// dark value is near-black (luminance < 0.5 → IsDark true); the light value is
// near-white (luminance ≥ 0.5 → IsDark false).
var (
	darkBg  = tea.BackgroundColorMsg{Color: color.RGBA{R: 0x0b, G: 0x0c, B: 0x14, A: 0xff}}
	lightBg = tea.BackgroundColorMsg{Color: color.RGBA{R: 0xe1, G: 0xe2, B: 0xe7, A: 0xff}}
)

// detectModel builds an appearance-driven Sessions model sized for rendering,
// with the deterministic flat session set ingested through the production path,
// and opens the detect-or-timeout window exactly as production (Build) does via
// armAppearanceDetection. In auto mode that holds the first paint on
// detection-or-timeout; a pinned appearance stays resolved (arm is a no-op).
func detectModel(t *testing.T, appearance prefs.Appearance) Model {
	t.Helper()
	sessions := []tmux.Session{
		{Name: "alpha", Windows: 3, Attached: true},
		{Name: "bravo", Windows: 1, Attached: false},
		{Name: "charlie", Windows: 2, Attached: false},
	}
	m := New(fakeLister{}, WithAppearance(appearance))
	m.termWidth = 90
	m.termHeight = 24
	m.applySessions(sessions)
	m.armAppearanceDetection()
	return m
}

// blankView is the neutral pre-resolution frame: a full-terminal blank canvas
// with no real content (no "Sessions" title row painted). The detect-or-timeout
// gate holds the first real paint until the mode resolves so Portal never paints
// one canvas then flips to the other.
func assertBlankFrame(t *testing.T, m Model) {
	t.Helper()
	if m.modeResolved() {
		t.Fatalf("modeResolved = true, want false (model must be unresolved for a blank frame)")
	}
	if got := m.View().Content; strings.Contains(got, "Sessions") {
		t.Errorf("pre-resolution View painted real content (found 'Sessions' title); want a neutral blank frame")
	}
}

func assertPaintedCanvas(t *testing.T, m Model, mode theme.Mode) {
	t.Helper()
	if !m.modeResolved() {
		t.Fatalf("modeResolved = false, want true (mode must be resolved before the real paint)")
	}
	if m.canvasMode != mode {
		t.Errorf("canvasMode = %v, want %v", m.canvasMode, mode)
	}
	view := m.View().Content
	if !strings.Contains(view, "Sessions") {
		t.Errorf("resolved View did not paint the real content (no 'Sessions' title)")
	}
	if seq := canvasSeq(t, mode); !strings.Contains(view, seq) {
		t.Errorf("resolved View does not contain the %v canvas background sequence %q", mode, seq)
	}
}

// TestAutoDetectsDark: auto + a dark BackgroundColorMsg resolves canvasMode Dark,
// marks resolved, and paints the dark canvas. Before the message the frame is the
// neutral blank (no pre-resolution real paint).
func TestAutoDetectsDark(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)
	assertBlankFrame(t, m)

	updated, _ := m.Update(darkBg)
	assertPaintedCanvas(t, updated.(Model), theme.Dark)
}

// TestAutoDetectsLight: auto + a light BackgroundColorMsg resolves canvasMode
// Light and paints the light canvas.
func TestAutoDetectsLight(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)
	assertBlankFrame(t, m)

	updated, _ := m.Update(lightBg)
	assertPaintedCanvas(t, updated.(Model), theme.Light)
}

// TestNoPaintThenFlip: before resolution the View is the neutral blank frame
// (not a painted canvas); after resolution it is the correct canvas; and a later
// message never re-resolves the mode (no second resolution, no flip).
func TestNoPaintThenFlip(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)
	assertBlankFrame(t, m)

	// OSC 11 answers dark first → resolves dark, paints.
	updated, _ := m.Update(darkBg)
	resolved := updated.(Model)
	assertPaintedCanvas(t, resolved, theme.Dark)

	// A late timeout (the loser of the race) must be ignored — the mode is
	// already resolved, so it must not flip to anything.
	after, _ := resolved.Update(appearanceTimeoutMsg{})
	if after.(Model).canvasMode != theme.Dark {
		t.Errorf("a late timeout flipped canvasMode to %v, want it pinned at Dark (no second resolution)", after.(Model).canvasMode)
	}

	// And a late, conflicting BackgroundColorMsg (light) must not flip either.
	after2, _ := after.(Model).Update(lightBg)
	if after2.(Model).canvasMode != theme.Dark {
		t.Errorf("a late light BackgroundColorMsg flipped canvasMode to %v, want it pinned at Dark (no flip)", after2.(Model).canvasMode)
	}
}

// TestTimeoutFallsBackToDark: the timeout fires before any BackgroundColorMsg, so
// the mode resolves to the dark fallback and paints.
func TestTimeoutFallsBackToDark(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)
	assertBlankFrame(t, m)

	updated, _ := m.Update(appearanceTimeoutMsg{})
	assertPaintedCanvas(t, updated.(Model), theme.Dark)
}

// TestPinLightSkipsDetection: appearance Light pins the mode and skips detection
// + the gate — the model is resolved at construction and paints the light canvas
// from frame one, and Init issues NO timeout tick.
func TestPinLightSkipsDetection(t *testing.T) {
	m := detectModel(t, prefs.AppearanceLight)
	assertPaintedCanvas(t, m, theme.Light)
	assertNoTimeoutTick(t, m)
}

// TestPinDarkSkipsDetection: appearance Dark pins the mode and skips detection +
// the gate — resolved at construction, paints the dark canvas from frame one, no
// timeout tick from Init.
func TestPinDarkSkipsDetection(t *testing.T) {
	m := detectModel(t, prefs.AppearanceDark)
	assertPaintedCanvas(t, m, theme.Dark)
	assertNoTimeoutTick(t, m)
}

// TestColorFGBGNeverOverridesOSC11: even with COLORFGBG advertising a light
// terminal, an OSC 11 answer of dark wins — COLORFGBG is a weak hint only and
// must never override the OSC 11 reply.
func TestColorFGBGNeverOverridesOSC11(t *testing.T) {
	t.Setenv("COLORFGBG", "0;15") // fg black on bg white → "light" by the weak hint
	m := detectModel(t, prefs.AppearanceAuto)

	updated, _ := m.Update(darkBg)
	assertPaintedCanvas(t, updated.(Model), theme.Dark)
}

// TestMisdetectionLegibleNotBroken: a mis-detected terminal resolves to the
// wrong-but-painted canvas (here light reported for a model that "should" be
// dark) — the canvas still paints (not blank, not crashed); the contrast floor
// holds against whichever canvas is painted (§2.3).
func TestMisdetectionLegibleNotBroken(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)

	// The terminal mis-reports light. The model paints the light canvas — wrong
	// mode but fully legible, not blank, not crashed.
	updated, _ := m.Update(lightBg)
	resolved := updated.(Model)
	assertPaintedCanvas(t, resolved, theme.Light)
	view := resolved.View().Content
	if strings.TrimSpace(view) == "" {
		t.Errorf("mis-detected canvas rendered blank, want a legible (wrong-mode) screen")
	}
}

// assertNoTimeoutTick drains Init's batched cmds and asserts none of them
// produces an appearanceTimeoutMsg — the pin path must not arm the detection
// timeout (it skips the wait entirely).
func assertNoTimeoutTick(t *testing.T, m Model) {
	t.Helper()
	for _, msg := range initCmds(t, m.Init()) {
		if _, ok := msg.(appearanceTimeoutMsg); ok {
			t.Errorf("Init armed a detection timeout on a pinned appearance, want none (the wait is skipped)")
		}
	}
}

// TestAutoArmsTimeoutTick: in auto mode Init arms the detect-or-timeout tick so a
// non-responding terminal still resolves to the dark fallback.
func TestAutoArmsTimeoutTick(t *testing.T) {
	m := detectModel(t, prefs.AppearanceAuto)
	found := false
	for _, msg := range initCmds(t, m.Init()) {
		if _, ok := msg.(appearanceTimeoutMsg); ok {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Init did not arm the detect-or-timeout tick in auto mode (the no-answer fallback would never fire)")
	}
}

// TestBuildArmsAutoGate pins the PRODUCTION wiring: Build (the cmd/open.go +
// capture-harness chokepoint) opens the detect-or-timeout window for an auto
// appearance (the live picker holds the blank frame until OSC 11 resolves) while
// a pinned light/dark appearance paints from frame one. This is the swap of the
// canvas-mode source from the temporary injected mode to real resolution.
func TestBuildArmsAutoGate(t *testing.T) {
	t.Run("auto appearance gates the first paint", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Appearance: prefs.AppearanceAuto})
		if m.modeResolved() {
			t.Errorf("Build(auto) is resolved at construction; want the detect-or-timeout gate open (unresolved)")
		}
		// Resolving via an OSC 11 reply paints the detected canvas.
		updated, _ := m.Update(lightBg)
		if !updated.(Model).modeResolved() || updated.(Model).canvasMode != theme.Light {
			t.Errorf("Build(auto) did not resolve to the OSC 11-detected canvas; resolved=%v mode=%v",
				updated.(Model).modeResolved(), updated.(Model).canvasMode)
		}
	})

	t.Run("pinned light appearance paints from frame one", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Appearance: prefs.AppearanceLight})
		if !m.modeResolved() {
			t.Errorf("Build(light pin) is unresolved; want immediate resolution (no gate, no wait)")
		}
		if m.canvasMode != theme.Light {
			t.Errorf("Build(light pin) canvasMode = %v, want theme.Light", m.canvasMode)
		}
	})

	t.Run("pinned dark appearance paints from frame one", func(t *testing.T) {
		m := Build(Deps{Lister: fakeLister{}, Appearance: prefs.AppearanceDark})
		if !m.modeResolved() {
			t.Errorf("Build(dark pin) is unresolved; want immediate resolution (no gate, no wait)")
		}
		if m.canvasMode != theme.Dark {
			t.Errorf("Build(dark pin) canvasMode = %v, want theme.Dark", m.canvasMode)
		}
	})
}
