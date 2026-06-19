// Command capturetool is the offline visual-capture harness for Portal's TUI.
//
// It is a SEPARATE, permanent program — NOT a subcommand of the shipped portal
// binary. It imports Portal's real internal/tui library, builds the production
// model via the shared tui.Build constructor, and binds every tmux seam to an
// in-memory fake from internal/capture. It runs NO bootstrap: it opens no tmux
// server, spawns no daemon, runs no orphan-sweep, and reads no real config.
//
// Its sole job is to render a deterministic, named fixture so vhs can screenshot
// the live TUI and a reviewer can compare it to the committed Paper reference
// (spec § 15). The captured frame is the REAL production TUI because the model is
// built through the exact constructor cmd/open.go uses.
//
// Usage:
//
//	capturetool --fixture sessions-flat
//
// Run via vhs (see testdata/vhs/README.md). The portal binary never imports
// internal/capture — an import guard test enforces that production stays clean.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
	"github.com/leeovery/portal/internal/capture"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/tui"
)

func main() {
	fixture := flag.String("fixture", "", "named fixture to render (e.g. sessions-flat)")
	appearance := flag.String("appearance", "dark", "owned-canvas mode to render: dark|light")
	flag.Parse()

	if err := run(*fixture, *appearance); err != nil {
		fmt.Fprintln(os.Stderr, "capturetool:", err)
		os.Exit(1)
	}
}

// run resolves the fixture into a renderable model and runs the Bubble Tea
// program on the alt screen until the user (or vhs) quits. It returns any
// resolution or program error so main can map it to a non-zero exit.
func run(fixture, appearance string) error {
	m, err := resolveProgram(fixture, appearance)
	if err != nil {
		return err
	}

	// Alt screen mirrors cmd/open.go's production launch so the captured frame
	// matches what a user sees. In Bubble Tea v2 the alt screen is declared via
	// the tea.View.AltScreen field (set in tui.Model.View()), not the removed
	// tea.WithAltScreen() option. No bootstrap, no warnings staging — this is
	// the inert, fixture-only render path.
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	// Restore the terminal's original background on exit (§ background restore-
	// on-exit). The owned canvas paint sets the terminal background via OSC 11;
	// terminals that ignore the OSC 111 reset would keep the canvas colour, so
	// SET the captured original back. No-op when no OSC 11 response was captured.
	// Wired identically to cmd/open.go via the shared tui helper. Writes to
	// os.Stdout (the program's output) so the sequence reaches the terminal.
	if fm, ok := finalModel.(tui.Model); ok {
		tui.RestoreTerminalBackground(os.Stdout, fm)
	}
	return nil
}

// resolveProgram maps a fixture name to the tea.Model the harness runs. Most
// fixtures resolve to the production tui.Model via the shared tui.Build
// constructor (resolveModel) so the capture is the REAL production frame. The
// contrast-validation swatch (§16.5 lock-in/bail gate, task 1-9) is the one
// exception: it is a standalone validation surface (a labelled tint swatch on the
// owned canvas) — the four tint SURFACES it validates land in later phases, so it
// deliberately does NOT route through tui.Build. Returning tea.Model lets run()
// drive both identically. An invalid appearance fails loudly for either path.
func resolveProgram(fixture, appearance string) (tea.Model, error) {
	if fixture == capture.ContrastValidationFixture {
		pin, err := resolveAppearance(appearance)
		if err != nil {
			return nil, err
		}
		return capture.NewContrastValidationModel(pin), nil
	}
	m, err := resolveModel(fixture, appearance)
	if err != nil {
		return nil, err
	}
	return m, nil
}

// resolveModel maps a fixture name to the production tui.Model via the shared
// tui.Build constructor, PINNING the owned-canvas appearance from the
// --appearance flag. Driving the pin (not a direct canvas-mode injection)
// exercises the real §2.6 resolution path: a pinned light/dark appearance
// resolves the canvas immediately (no OSC 11 detection, no first-paint wait), so
// the capture is byte-deterministic AND covers the production pin path. An empty
// or unknown fixture, or an invalid appearance, is an error so a bad flag fails
// loudly.
func resolveModel(fixture, appearance string) (tui.Model, error) {
	if fixture == "" {
		return tui.Model{}, fmt.Errorf("--fixture is required (available: %v)", capture.FixtureNames())
	}
	pin, err := resolveAppearance(appearance)
	if err != nil {
		return tui.Model{}, err
	}
	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		return tui.Model{}, err
	}
	deps := fx.Deps()
	deps.Appearance = pin
	// NO_COLOR carve-out (§2.5): read the env (present and non-empty, the
	// no-color.org convention) and inject the single colourless flag so the
	// NO_COLOR tape (NO_COLOR=1 inline) renders the colourless native-bg path —
	// the same flag cmd/open.go drives in production. When set it wins over the
	// appearance pin (no canvas to select), so the capture shows no painted canvas.
	if v, ok := os.LookupEnv("NO_COLOR"); ok && v != "" {
		deps.NoColor = true
	}
	return tui.Build(deps), nil
}

// resolveAppearance maps the --appearance flag to the pinned prefs.Appearance the
// model resolves the owned canvas from: dark pins the #0b0c14 canvas, light the
// #e1e2e7 canvas. Pinning skips OSC 11 detection and the first-paint wait, so the
// capture renders the correct canvas from frame one. An unrecognised value fails
// loudly rather than silently defaulting, so a typo in a tape is caught.
func resolveAppearance(appearance string) (prefs.Appearance, error) {
	switch appearance {
	case "dark":
		return prefs.AppearanceDark, nil
	case "light":
		return prefs.AppearanceLight, nil
	default:
		return prefs.AppearanceAuto, fmt.Errorf("--appearance must be dark or light, got %q", appearance)
	}
}
