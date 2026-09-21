package capture_test

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/themetest"
	"github.com/leeovery/portal/internal/tui"
)

// The fixtures whose frame IS a geometry, each with the fixture it is derived
// from: identical data, so only the declared render size can tell them apart.
var geometryFixtureBases = map[string]string{
	panelFixtureNamePrefix + "narrow":             panelFixtureNamePrefix + "adaptive-pair",
	panelFixtureNamePrefix + "min-height-message": panelFixtureNamePrefix + "commit-failed",
}

func frameDims(t *testing.T, fixture string, palette theme.Theme, w, h int) (int, int) {
	t.Helper()

	visible := ansi.Strip(panelFrameAt(t, fixture, palette, w, h))
	return lipgloss.Width(visible), lipgloss.Height(visible)
}

func TestFixtureRenderSize_GeometryFixturesDifferFromTheirBases(t *testing.T) {
	palette := themetest.Builtin(t, "nord")

	for name, base := range geometryFixtureBases {
		t.Run(name, func(t *testing.T) {
			if panelFixtureFrame(t, name, palette) == panelFixtureFrame(t, base, palette) {
				t.Errorf("%s renders identically to %s at the harness size; its frame is a geometry, so it must declare the size that produces it rather than depending on the caller to pass one", name, base)
			}
		})
	}
}

func TestFixtureRenderSize_DeclaredSizeSubstitutesForTheCallers(t *testing.T) {
	palette := themetest.Builtin(t, "nord")

	t.Run("the narrow fixture pins its width and takes the caller's height", func(t *testing.T) {
		w, h := frameDims(t, panelFixtureNamePrefix+"narrow", palette, harnessWidth, harnessHeight)
		if want := tui.ThemePanelMinWidthTerminal(); w != want {
			t.Errorf("the frame is %d columns wide, want the declared %d", w, want)
		}
		if h != harnessHeight {
			t.Errorf("the frame is %d rows tall, want the caller's %d — an undeclared dimension is the caller's", h, harnessHeight)
		}
	})

	t.Run("the min-height fixture pins both dimensions", func(t *testing.T) {
		w, h := frameDims(t, panelFixtureNamePrefix+"min-height-message", palette, harnessWidth, harnessHeight)
		if want := tui.ThemePanelMinWidthTerminal(); w != want {
			t.Errorf("the frame is %d columns wide, want the declared %d", w, want)
		}
		if want := tui.ThemePanelFloorTerminalHeight(); h != want {
			t.Errorf("the frame is %d rows tall, want the declared %d", h, want)
		}
	})

	t.Run("a fixture that declares no size renders at the caller's", func(t *testing.T) {
		const callerW, callerH = 100, 30
		w, h := frameDims(t, "sessions-flat", palette, callerW, callerH)
		if w != callerW || h != callerH {
			t.Errorf("the frame renders %d×%d, want the caller's %d×%d", w, h, callerW, callerH)
		}
	})
}

func TestFixtureRenderSize_DeclaredOnlyByTheGeometryFixtures(t *testing.T) {
	palette := themetest.Builtin(t, "nord")
	const callerW, callerH = 100, 30

	for _, name := range capture.FixtureNames() {
		if _, geometry := geometryFixtureBases[name]; geometry || capture.IsStandalone(name) {
			continue
		}
		t.Run(name, func(t *testing.T) {
			if w, h := frameDims(t, name, palette, callerW, callerH); w != callerW || h != callerH {
				t.Errorf("%s renders %d×%d at a caller size of %d×%d; only a fixture whose frame is a geometry declares its own", name, w, h, callerW, callerH)
			}
		})
	}
}
