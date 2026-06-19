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
	"github.com/leeovery/portal/internal/tui"
	"github.com/leeovery/portal/internal/tui/theme"
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

// run resolves the fixture into a production model and runs the Bubble Tea
// program on the alt screen until the user (or vhs) quits. It returns any
// resolution or program error so main can map it to a non-zero exit.
func run(fixture, appearance string) error {
	m, err := resolveModel(fixture, appearance)
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

// resolveModel maps a fixture name to the production tui.Model via the shared
// tui.Build constructor, injecting the owned-canvas mode resolved from the
// --appearance flag. An empty or unknown fixture, or an invalid appearance, is
// an error so a bad flag fails loudly.
func resolveModel(fixture, appearance string) (tui.Model, error) {
	if fixture == "" {
		return tui.Model{}, fmt.Errorf("--fixture is required (available: %v)", capture.FixtureNames())
	}
	mode, err := resolveMode(appearance)
	if err != nil {
		return tui.Model{}, err
	}
	fx, err := capture.FixtureByName(fixture)
	if err != nil {
		return tui.Model{}, err
	}
	deps := fx.Deps()
	deps.CanvasMode = mode
	return tui.Build(deps), nil
}

// resolveMode maps the --appearance flag to the resolved owned-canvas theme.Mode
// the model paints. Detection (task 1-7) is not landed, so the capture harness
// injects the mode explicitly: dark renders the #0b0c14 canvas, light the
// #e1e2e7 canvas. An unrecognised value fails loudly rather than silently
// defaulting, so a typo in a tape is caught.
func resolveMode(appearance string) (theme.Mode, error) {
	switch appearance {
	case "dark":
		return theme.Dark, nil
	case "light":
		return theme.Light, nil
	default:
		return theme.Dark, fmt.Errorf("--appearance must be dark or light, got %q", appearance)
	}
}
