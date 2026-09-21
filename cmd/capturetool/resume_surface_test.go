package main

import (
	"io"
	"os"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

// Restated rather than read off the surface, so a seed that drifts is a failure
// here rather than an expectation that drifts with it.
const (
	surfaceCommand = "cd ~/code/portal/internal/capture && ./scripts/watch --session 45604077-a9d0-41bd-9692-b97f63f237fc"
	surfaceReport  = "the pane is frozen and will not lift"
)

type surfaceWant struct {
	name   string
	render func(tui.ResumeScreen) string
	report string
	w, h   int
}

func surfaceWants() []surfaceWant {
	return []surfaceWant{
		{name: "resume-panel-waiting", render: tui.RenderResumePanel, w: 80, h: 24},
		{name: "resume-panel-waiting-report", render: tui.RenderResumePanel, report: surfaceReport, w: 80, h: 24},
		{name: "resume-panel-waiting-degraded", render: tui.RenderResumePanel, w: 40, h: 10},
		{name: "resume-panel-discard", render: tui.RenderResumeDiscardConfirm, w: 80, h: 24},
		{name: "resume-panel-discard-report", render: tui.RenderResumeDiscardConfirm, report: surfaceReport, w: 80, h: 24},
		{name: "resume-panel-discard-degraded", render: tui.RenderResumeDiscardConfirm, w: 40, h: 10},
	}
}

func (w surfaceWant) production(th theme.Theme, colourless bool) string {
	return w.render(tui.ResumeScreen{
		Command:    surfaceCommand,
		Report:     w.report,
		Width:      w.w,
		Height:     w.h,
		Theme:      th,
		Colourless: colourless,
	})
}

// The program's own path: resolve, then feed it the filtered resize a live
// terminal would produce.
func surfaceViewThrough(t *testing.T, name, themeArg string) tea.View {
	t.Helper()
	m, err := resolveProgram(name, themeArg, io.Discard)
	if err != nil {
		t.Fatalf("resolveProgram(%s, %s): %v", name, themeArg, err)
	}
	if m == nil {
		t.Fatalf("resolveProgram(%s, %s) returned a nil model", name, themeArg)
	}
	model, _ := m.Update(renderSizeFilter(name)(m, tea.WindowSizeMsg{Width: 200, Height: 60}))
	return model.View()
}

func TestResolveProgram_ResumeSurfaces(t *testing.T) {
	t.Run("a built-in slug pins the surface", func(t *testing.T) {
		th := themetest.Builtin(t, "nord")
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				if got := surfaceViewThrough(t, want.name, "nord").Content; got != want.production(th, false) {
					t.Errorf("the surface's view is not the production render at its pinned %d×%d\n got: %q\nwant: %q", want.w, want.h, got, want.production(th, false))
				}
			})
		}
	})

	t.Run("an explicit path pins the surface too", func(t *testing.T) {
		path := themetest.Write(t, t.TempDir(), "mytheme.theme", themetest.LinesWithCanvas("#1a2b3c"))
		result, rejection := theme.LoadPath(path)
		if rejection != nil {
			t.Fatalf("probe setup: the staged theme file is unusable: %v", rejection)
		}
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				if got := surfaceViewThrough(t, want.name, path).Content; got != want.production(result.Theme, false) {
					t.Errorf("the surface does not render the file's palette\n got: %q\nwant: %q", got, want.production(result.Theme, false))
				}
			})
		}
	})

	t.Run("it renders colourless under NO_COLOR", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		th := themetest.Builtin(t, "nord")
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				if got := surfaceViewThrough(t, want.name, "nord").Content; got != want.production(th, true) {
					t.Errorf("the surface is not the colourless production render; NO_COLOR must reach a surface by the same read a fixture takes\n got: %q\nwant: %q", got, want.production(th, true))
				}
			})
		}
	})

	t.Run("it sets no terminal background", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				view := surfaceViewThrough(t, want.name, "nord")
				if !view.AltScreen {
					t.Error("the view does not declare the alt screen")
				}
				if view.BackgroundColor != nil {
					t.Errorf("the view sets a terminal background (%v); a pane draw never does", view.BackgroundColor)
				}

				m, err := resolveProgram(want.name, "nord", io.Discard)
				if err != nil {
					t.Fatalf("resolveProgram(%s): %v", want.name, err)
				}
				if _, isModel := m.(tui.Model); isModel {
					t.Error("the surface is a tui.Model, so run's restore switch would set a terminal background back for a screen that never set one")
				}
				if _, restores := m.(interface {
					OriginalBackground() string
					PaintedCanvasHex() string
				}); restores {
					t.Error("the surface reports a captured background, so run's restore switch would set one back for a screen that never set one")
				}
			})
		}
	})
}

func TestRenderSizeFilter_PinsASurfaceSize(t *testing.T) {
	for _, want := range surfaceWants() {
		t.Run(want.name, func(t *testing.T) {
			got, ok := renderSizeFilter(want.name)(nil, tea.WindowSizeMsg{Width: 200, Height: 60}).(tea.WindowSizeMsg)
			if !ok {
				t.Fatal("the filter returned something other than a resize")
			}
			if got.Width != want.w || got.Height != want.h {
				t.Errorf("the resize reaches the surface as %d×%d, want its pinned %d×%d", got.Width, got.Height, want.w, want.h)
			}
		})
	}
}

// The env read is the thing that must not drift: a second one is how a surface
// and a fixture come to disagree about NO_COLOR.
func TestNoColorIsReadInOnePlace(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	if got := strings.Count(string(body), `"NO_COLOR"`); got != 1 {
		t.Errorf("main.go names NO_COLOR %d times, want exactly 1 — the surface branch and resolveModel read it through one helper", got)
	}
}

func TestCapturetool_SurfacesAreEnumeratedForTheUser(t *testing.T) {
	_, err := resolveProgram("", defaultThemeSlug, io.Discard)
	if err == nil {
		t.Fatal("resolveProgram with no fixture returned no error")
	}
	for _, name := range capture.SurfaceNames() {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("the --fixture-is-required error does not list %s, so the surface is unreachable by anyone reading it: %v", name, err)
		}
	}
}
