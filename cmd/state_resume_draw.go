package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

const resumeCursorHome = "\x1b[H"

type resumeDrawConfig struct {
	resumeChainPayload

	Stdout     io.Writer
	Logger     *slog.Logger
	Colourless bool
	Size       func() (int, int, error)
	ExecSelf   func(prog string, args []string)

	// ResolveTheme runs dropInput, when handed one, after the appearance query
	// and before it returns, answering the drop's error beside the palette.
	ResolveTheme   func(colourless bool, dropInput func() error) (theme.Theme, error)
	DropInputQueue func() error
}

// runResumeDraw paints one screen of the waiting pane and replaces its own
// process image with the waiter, so nothing the draw touched stays resident for
// as long as the pane waits. The leave sequence is never written here: it
// belongs to whatever answers the panel.
func runResumeDraw(cfg resumeDrawConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	// The read's error is not consulted: a failed or non-positive size reaches
	// the renderer as it stands and resolves to its own bounded fallback, and a
	// pane that drew nothing would read as restored with a dead keyboard.
	width, height, _ := cfg.Size()

	var dropInput func() error
	if cfg.DropInput {
		dropInput = cfg.DropInputQueue
	}
	th, dropErr := cfg.ResolveTheme(cfg.Colourless, dropInput)

	shown := cfg.resumeChainPayload
	shown.DropInput = false
	// Input the drop could not discard may still answer the confirmation, so it
	// is never put up over that input.
	if dropErr != nil {
		shown.Screen = resumeScreenPanel
		shown.Report = dropErr.Error()
	}

	_, _ = io.WriteString(cfg.Stdout, hydrateAltScreenEnter)
	_, _ = io.WriteString(cfg.Stdout, resumeCursorHome)
	_, _ = io.WriteString(cfg.Stdout, renderResumeScreen(tui.ResumeScreen{
		Command:    shown.Command,
		Report:     shown.Report,
		Width:      width,
		Height:     height,
		Theme:      th,
		Colourless: cfg.Colourless,
	}, shown.Screen))

	shown.Width, shown.Height = width, height
	return resumeHandOff(cfg.Logger, cfg.ExecSelf, resumeWaitSubcommand, shown)
}

func renderResumeScreen(s tui.ResumeScreen, screen string) string {
	if screen == resumeScreenDiscard {
		return tui.RenderResumeDiscardConfirm(s)
	}
	return tui.RenderResumePanel(s)
}

// paneDrawTheme is the palette one draw paints in, read through the
// non-migrating prefs route: the migrating one performs the one-shot appearance
// translation and writes, and a boot's worth of panes taking it concurrently is
// a write storm over one file for a value none of them is setting.
func paneDrawTheme(colourless bool, dropInput func() error) (theme.Theme, error) {
	nomination := paneDrawNomination(loadPrefsStoreNoMigrate, newThemeLoader())
	return tui.ResolvePaneTheme(nomination, colourless, dropInput)
}

// Every failure along the route degrades to the shipped pair rather than
// aborting: a pane that could not be painted is worse than one painted in the
// wrong half of a pair.
func paneDrawNomination(openStore func() (*prefs.Store, error), loader theme.Loader) theme.Nomination {
	store, err := openStore()
	if err != nil {
		return shippedPaneThemePair()
	}
	keys, err := store.LoadThemeKeys()
	if err != nil {
		return shippedPaneThemePair()
	}
	resolution, _, err := themeResolution(keys, loader)
	if err != nil {
		return shippedPaneThemePair()
	}
	return resolution.Nomination
}

// Silent because seeding a fallback is not a use of a theme, which is what the
// theme component records.
func shippedPaneThemePair() theme.Nomination {
	loader := theme.NewSilentLoader()
	light, _, _ := loader.LoadBuiltin(theme.DefaultLightSlug)
	dark, _, _ := loader.LoadBuiltin(theme.DefaultDarkSlug)
	return theme.AdaptivePair(light.Theme, dark.Theme)
}

func dropStdinInputQueue() error {
	if err := flushTTYInput(int(os.Stdin.Fd())); err != nil {
		return fmt.Errorf("could not clear pending input: %w", err)
	}
	return nil
}

// The pane's own tty rather than a tmux read, so a boot's worth of panes costs
// no tmux calls at all.
func paneSizeFromStdin() (int, int, error) {
	return term.GetSize(os.Stdin.Fd())
}

var resumeDrawRunFunc = runResumeDraw

// stateResumeDrawCmd paints one screen of a waiting pane and hands the pane to
// the waiter.
var stateResumeDrawCmd = &cobra.Command{
	Use:    resumeDrawSubcommand,
	Short:  "Draw the resume panel into a waiting pane (internal)",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		command, _ := cmd.Flags().GetString(resumeFlagCommand)
		report, _ := cmd.Flags().GetString(resumeFlagReport)
		hookKey, _ := cmd.Flags().GetString(resumeFlagHookKey)
		pane, _ := cmd.Flags().GetString(resumeFlagPane)
		paneKey, _ := cmd.Flags().GetString(resumeFlagPaneKey)
		screen, _ := cmd.Flags().GetString(resumeFlagScreen)
		dropInput, _ := cmd.Flags().GetBool(resumeFlagDropInput)

		return resumeDrawRunFunc(resumeDrawConfig{
			resumeChainPayload: resumeChainPayload{
				Command:   command,
				Report:    report,
				HookKey:   hookKey,
				Pane:      pane,
				PaneKey:   paneKey,
				Screen:    screen,
				DropInput: dropInput,
			},
			Stdout:         os.Stdout,
			Logger:         hydrateLogger,
			Colourless:     noColorEnabled(),
			Size:           paneSizeFromStdin,
			ResolveTheme:   paneDrawTheme,
			DropInputQueue: dropStdinInputQueue,
			ExecSelf:       defaultExecShell,
		})
	},
}

func init() {
	stateResumeDrawCmd.Flags().String(resumeFlagCommand, "", "The registered on-resume command the panel states")
	stateResumeDrawCmd.Flags().String(resumeFlagReport, "", "What an answer that could not be carried out reported")
	stateResumeDrawCmd.Flags().String(resumeFlagHookKey, "", "Saved pane token identifying the pane's resume hook")
	stateResumeDrawCmd.Flags().String(resumeFlagPane, "", "The pane id the marker writes address")
	stateResumeDrawCmd.Flags().String(resumeFlagPaneKey, "", "The pane key the chain's records name the pane by")
	// Width and height are registered but not read: a draw measures the pane
	// itself, and a flag the waiter hands back that this command did not
	// register would fail its parse.
	stateResumeDrawCmd.Flags().Int(resumeFlagWidth, 0, "Width the previous screen was drawn at")
	stateResumeDrawCmd.Flags().Int(resumeFlagHeight, 0, "Height the previous screen was drawn at")
	stateResumeDrawCmd.Flags().String(resumeFlagScreen, resumeScreenPanel, "Which screen to draw: discard for the confirmation, else the panel")
	stateResumeDrawCmd.Flags().Bool(resumeFlagDropInput, false, "Discard input already queued on the pane before drawing")
	_ = stateResumeDrawCmd.MarkFlagRequired(resumeFlagCommand)

	stateCmd.AddCommand(stateResumeDrawCmd)
}
