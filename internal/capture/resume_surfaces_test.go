package capture_test

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

// The seeds are restated here rather than read off the surface: an expectation
// built from the value the code renders asserts nothing about what it holds.
const (
	surfaceCommand = "cd ~/code/portal/internal/capture && ./scripts/watch --session 45604077-a9d0-41bd-9692-b97f63f237fc"
	surfaceReport  = "the pane is frozen and will not lift"
)

const (
	surfaceRoomyWidth     = 80
	surfaceRoomyHeight    = 24
	surfaceDegradedWidth  = 40
	surfaceDegradedHeight = 10
)

// The card's top-left corner: present on a framed render, absent from the
// degraded stack.
const cardCornerGlyph = "╭"

type surfaceWant struct {
	name     string
	render   func(tui.ResumeScreen) string
	report   string
	w, h     int
	degraded bool
}

func surfaceWants() []surfaceWant {
	return []surfaceWant{
		{name: "resume-panel-waiting", render: tui.RenderResumePanel, w: surfaceRoomyWidth, h: surfaceRoomyHeight},
		{name: "resume-panel-waiting-report", render: tui.RenderResumePanel, report: surfaceReport, w: surfaceRoomyWidth, h: surfaceRoomyHeight},
		{name: "resume-panel-waiting-degraded", render: tui.RenderResumePanel, w: surfaceDegradedWidth, h: surfaceDegradedHeight, degraded: true},
		{name: "resume-panel-discard", render: tui.RenderResumeDiscardConfirm, w: surfaceRoomyWidth, h: surfaceRoomyHeight},
		{name: "resume-panel-discard-report", render: tui.RenderResumeDiscardConfirm, report: surfaceReport, w: surfaceRoomyWidth, h: surfaceRoomyHeight},
		{name: "resume-panel-discard-degraded", render: tui.RenderResumeDiscardConfirm, w: surfaceDegradedWidth, h: surfaceDegradedHeight, degraded: true},
	}
}

func surfaceNamesWanted() []string {
	wants := surfaceWants()
	names := make([]string, 0, len(wants))
	for _, want := range wants {
		names = append(names, want.name)
	}
	return names
}

func surfacePalette(t *testing.T) theme.Theme {
	t.Helper()
	return themetest.Builtin(t, "nord")
}

// The surface driven as the program drives it: the pinned resize, then a view.
func surfaceFrame(t *testing.T, name string, th theme.Theme, colourless bool) string {
	t.Helper()
	sf, ok := capture.SurfaceByName(name)
	if !ok {
		t.Fatalf("SurfaceByName(%s): not registered", name)
	}
	var m tea.Model = sf.WithTheme(th, colourless)
	m, _ = m.Update(sf.PinRenderSize(tea.WindowSizeMsg{Width: 200, Height: 60}))
	return m.View().Content
}

func TestResumeSurfaces_Registry(t *testing.T) {
	t.Run("it enumerates every resume surface by name", func(t *testing.T) {
		want := surfaceNamesWanted()
		if got := capture.SurfaceNames(); !slices.Equal(got, want) {
			t.Errorf("SurfaceNames() = %v, want %v", got, want)
		}
		enumerated := capture.FixtureNames()
		for _, name := range want {
			if !slices.Contains(enumerated, name) {
				t.Errorf("FixtureNames() does not list %s, so `capturetool --fixture %s` is unreachable by name", name, name)
			}
		}
	})

	t.Run("it resolves each surface to a model and no fixture", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				sf, ok := capture.SurfaceByName(want.name)
				if !ok {
					t.Fatalf("SurfaceByName(%s) reports it is not registered", want.name)
				}
				var _ tea.Model = sf.WithTheme(surfacePalette(t), false)

				fx, err := capture.FixtureByName(want.name)
				if err == nil {
					t.Fatalf("FixtureByName(%s) resolved %v; a surface is no *Fixture and must be rejected as an unknown fixture", want.name, fx)
				}
				if !strings.Contains(err.Error(), "unknown fixture") {
					t.Errorf("FixtureByName(%s) error = %v, want the existing unknown-fixture error", want.name, err)
				}
			})
		}
	})
}

func TestResumeSurfaces_RendersTheProductionScreen(t *testing.T) {
	th := surfacePalette(t)

	t.Run("it renders the production panel function byte-identically", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				expected := want.render(tui.ResumeScreen{
					Command: surfaceCommand,
					Report:  want.report,
					Width:   want.w,
					Height:  want.h,
					Theme:   th,
				})
				if got := surfaceFrame(t, want.name, th, false); got != expected {
					t.Errorf("the surface's view is not the production render at the pinned %d×%d\n got: %q\nwant: %q", want.w, want.h, got, expected)
				}
			})
		}
	})

	t.Run("it renders the degraded stack at the pinned size", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				visible := ansi.Strip(surfaceFrame(t, want.name, th, false))
				if w, h := lipgloss.Width(visible), lipgloss.Height(visible); w != want.w || h != want.h {
					t.Errorf("the surface renders %d×%d, want its pinned %d×%d — a live window's size must be substituted away", w, h, want.w, want.h)
				}
				framed := strings.Contains(visible, cardCornerGlyph)
				if want.degraded && framed {
					t.Errorf("the degraded surface still draws the card frame at %d×%d; its pinned size is below the card's, so it must stack plainly:\n%s", want.w, want.h, visible)
				}
				if !want.degraded && !framed {
					t.Errorf("the surface draws no card frame at %d×%d, so its pinned size no longer holds the card:\n%s", want.w, want.h, visible)
				}
			})
		}
	})

	t.Run("it renders the report row on the report surfaces", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				visible := ansi.Strip(surfaceFrame(t, want.name, th, false))
				carries := strings.Contains(visible, surfaceReport)
				if want.report != "" && !carries {
					t.Errorf("the report surface does not carry %q:\n%s", surfaceReport, visible)
				}
				if want.report == "" && carries {
					t.Errorf("a surface seeded with no report carries %q; the row is present only when there is something to say:\n%s", surfaceReport, visible)
				}
			})
		}
	})

	t.Run("it renders colourless under NO_COLOR", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				frame := surfaceFrame(t, want.name, th, true)
				for _, form := range tokenForms(t, th) {
					if carriesRun(frame, form.fg) || carriesRun(frame, form.bg) {
						t.Errorf("the colourless surface paints token %s; under NO_COLOR no canvas and no colour is painted, and state stays glyph-backed:\n%q", form.name, frame)
					}
				}
			})
		}
	})
}

func TestResumeSurfaces_SetsNoTerminalBackground(t *testing.T) {
	th := surfacePalette(t)

	t.Run("it sets no terminal background", func(t *testing.T) {
		for _, want := range surfaceWants() {
			t.Run(want.name, func(t *testing.T) {
				sf, ok := capture.SurfaceByName(want.name)
				if !ok {
					t.Fatalf("SurfaceByName(%s): not registered", want.name)
				}
				view := sf.WithTheme(th, false).View()
				if !view.AltScreen {
					t.Error("the surface's view does not declare the alt screen, so the capture paints over the primary screen")
				}
				if view.BackgroundColor != nil {
					t.Errorf("the surface's view sets a terminal background (%v); a pane draw never does, so the capture would show a canvas the real screen does not paint", view.BackgroundColor)
				}
			})
		}
	})
}

func TestResumeSurfaces_QuitKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: tea.KeyEscape},
	} {
		t.Run(key.String(), func(t *testing.T) {
			sf, ok := capture.SurfaceByName("resume-panel-waiting")
			if !ok {
				t.Fatal("SurfaceByName(resume-panel-waiting): not registered")
			}
			var m tea.Model = sf.WithTheme(surfacePalette(t), false)
			if _, cmd := m.Update(key); cmd == nil {
				t.Errorf("%s returned no command; a capture surface must be dismissable from the keyboard", key.String())
			}
		})
	}
}

func TestResumeSurfaces_SkipSet(t *testing.T) {
	t.Run("it skips exactly the named standalone surfaces", func(t *testing.T) {
		want := append([]string{capture.ContrastValidationFixture}, surfaceNamesWanted()...)
		slices.Sort(want)
		got := slices.Sorted(slices.Values(capture.StandaloneNames()))
		if !slices.Equal(got, want) {
			t.Errorf("StandaloneNames() = %v, want the swatch plus exactly the six resume surfaces %v — a third kind of skip must be named here before it can be skipped anywhere", got, want)
		}
	})

	t.Run("the predicate answers for exactly that set", func(t *testing.T) {
		for _, name := range capture.StandaloneNames() {
			if !capture.IsStandalone(name) {
				t.Errorf("IsStandalone(%s) = false for a name StandaloneNames() lists", name)
			}
		}
		for _, name := range capture.FixtureNames() {
			if slices.Contains(capture.StandaloneNames(), name) {
				continue
			}
			if capture.IsStandalone(name) {
				t.Errorf("IsStandalone(%s) = true for a build-backed fixture; every enumerating guard reads through this predicate and would drop it", name)
			}
		}
	})
}
