package capture

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
)

// The registered surface names, in the order SurfaceNames() reports them. A
// caller names a surface as the user does, on the command line, so these stay
// unexported.
const (
	resumeWaitingSurface         = "resume-panel-waiting"
	resumeWaitingReportSurface   = "resume-panel-waiting-report"
	resumeWaitingDegradedSurface = "resume-panel-waiting-degraded"
	resumeDiscardSurface         = "resume-panel-discard"
	resumeDiscardReportSurface   = "resume-panel-discard-report"
	resumeDiscardDegradedSurface = "resume-panel-discard-degraded"
)

// A directory and an identifier, which is what a real registration carries and
// what makes the card's wrap visible rather than theoretical.
const resumeSurfaceCommand = "cd ~/code/portal/internal/capture && ./scripts/watch --session 45604077-a9d0-41bd-9692-b97f63f237fc"

// The report a freeze that will not lift leaves on the card.
const resumeSurfaceReport = "the pane is frozen and will not lift"

// The size a pane holds the card at, and one below it: the degraded form is
// reachable by name rather than by dragging a window to find the fallback
// point, which is where the screen is most likely to be wrong.
const (
	resumeSurfaceRoomyWidth     = 80
	resumeSurfaceRoomyHeight    = 24
	resumeSurfaceDegradedWidth  = 40
	resumeSurfaceDegradedHeight = 10
)

// Surface is a standalone named capture screen: one of the resume panel's two
// draws at a pinned size, rendered through the production function rather than
// a copy of its layout. It is no *Fixture — these screens are drawn straight
// into a pane by the waiting program and are not a picker model at all.
type Surface struct {
	name string
	draw func(tui.ResumeScreen) string
	// The seed the screen is drawn from. Theme and Colourless arrive with
	// WithTheme; Width and Height are the pinned size.
	screen tui.ResumeScreen

	// The live window's size, which the pinned size is substituted over.
	termWidth, termHeight int
}

// WithTheme returns the surface painted in th. The palette is not part of a
// registration: the same surface renders at whatever --theme names.
func (s *Surface) WithTheme(th theme.Theme, colourless bool) *Surface {
	painted := *s
	painted.screen.Theme = th
	painted.screen.Colourless = colourless
	return &painted
}

func (s *Surface) Name() string { return s.name }

func (s *Surface) Init() tea.Cmd { return nil }

func (s *Surface) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		s.termWidth, s.termHeight = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return s, tea.Quit
		}
	}
	return s, nil
}

// No BackgroundColor, deliberately: the picker and the swatch own the whole
// terminal and set OSC 11, while a pane draw never does — a surface that set
// one would show a canvas the real screen does not paint.
func (s *Surface) View() tea.View {
	seed := s.screen
	seed.Width, seed.Height = substituteDeclaredSize(s.screen.Width, s.screen.Height, s.termWidth, s.termHeight)

	v := tea.NewView(s.draw(seed))
	v.AltScreen = true
	return v
}

// PinRenderSize rewrites a resize to the surface's pinned size, as a fixture's
// declared size is pinned, so a live terminal's window cannot decide which of
// the card and the degraded stack is captured.
func (s *Surface) PinRenderSize(msg tea.Msg) tea.Msg {
	resize, ok := msg.(tea.WindowSizeMsg)
	if !ok {
		return msg
	}
	resize.Width, resize.Height = substituteDeclaredSize(s.screen.Width, s.screen.Height, resize.Width, resize.Height)
	return resize
}

// The registry, one entry per name. Rebuilt per lookup: a Surface records the
// live window's size, so handing out a shared value would let one render's
// resize decide another's.
func surfaceRegistry() []Surface {
	waiting, discard := tui.RenderResumePanel, tui.RenderResumeDiscardConfirm
	return []Surface{
		newSurface(resumeWaitingSurface, waiting, "", resumeSurfaceRoomyWidth, resumeSurfaceRoomyHeight),
		newSurface(resumeWaitingReportSurface, waiting, resumeSurfaceReport, resumeSurfaceRoomyWidth, resumeSurfaceRoomyHeight),
		newSurface(resumeWaitingDegradedSurface, waiting, "", resumeSurfaceDegradedWidth, resumeSurfaceDegradedHeight),
		newSurface(resumeDiscardSurface, discard, "", resumeSurfaceRoomyWidth, resumeSurfaceRoomyHeight),
		newSurface(resumeDiscardReportSurface, discard, resumeSurfaceReport, resumeSurfaceRoomyWidth, resumeSurfaceRoomyHeight),
		newSurface(resumeDiscardDegradedSurface, discard, "", resumeSurfaceDegradedWidth, resumeSurfaceDegradedHeight),
	}
}

// SurfaceNames lists the standalone resume surfaces. They are enumerated by
// FixtureNames() and resolve here rather than through FixtureByName.
func SurfaceNames() []string {
	registry := surfaceRegistry()
	names := make([]string, 0, len(registry))
	for _, sf := range registry {
		names = append(names, sf.Name())
	}
	return names
}

func SurfaceByName(name string) (*Surface, bool) {
	for _, sf := range surfaceRegistry() {
		if sf.Name() == name {
			return &sf, true
		}
	}
	return nil, false
}

// StandaloneNames are the enumerated names that FixtureByName does not resolve:
// the contrast swatch and the resume surfaces. Declared once so a surface added
// later joins every enumerating guard at this edit rather than fataling each of
// them on a name it cannot resolve.
func StandaloneNames() []string {
	return append([]string{ContrastValidationFixture}, SurfaceNames()...)
}

func IsStandalone(name string) bool {
	return slices.Contains(StandaloneNames(), name)
}

func newSurface(name string, draw func(tui.ResumeScreen) string, report string, w, h int) Surface {
	return Surface{
		name: name,
		draw: draw,
		screen: tui.ResumeScreen{
			Command: resumeSurfaceCommand,
			Report:  report,
			Width:   w,
			Height:  h,
		},
	}
}
