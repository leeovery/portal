package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
	"github.com/leeovery/portal/internal/prefs"
	"github.com/leeovery/portal/internal/theme"
	"github.com/leeovery/portal/internal/tui"
	"github.com/spf13/cobra"
)

const resumeCursorHome = "\x1b[H"

type resumeDrawConfig struct {
	resumeChainPayload

	Stdout       io.Writer
	Logger       *slog.Logger
	Colourless   bool
	Size         func() (int, int, error)
	ResolveTheme func(colourless bool) theme.Theme
	ExecSelf     func(prog string, args []string)
}

// runResumeDraw paints one screen of the waiting pane and replaces its own
// process image with the waiter, so nothing the draw touched stays resident for
// as long as the pane waits. The leave sequence is never written here: it
// belongs to whatever answers the panel. The trailing return is unreachable in
// production, where ExecSelf replaces the process image.
func runResumeDraw(cfg resumeDrawConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	// The read's error is not consulted: a failed or non-positive size reaches
	// the renderer as it stands and resolves to its own bounded fallback, and a
	// pane that drew nothing would read as restored with a dead keyboard.
	width, height, _ := cfg.Size()

	th := cfg.ResolveTheme(cfg.Colourless)

	_, _ = io.WriteString(cfg.Stdout, hydrateAltScreenEnter)
	_, _ = io.WriteString(cfg.Stdout, resumeCursorHome)
	_, _ = io.WriteString(cfg.Stdout, tui.RenderResumePanel(tui.ResumeScreen{
		Command:    cfg.Command,
		Report:     cfg.Report,
		Width:      width,
		Height:     height,
		Theme:      th,
		Colourless: cfg.Colourless,
	}))

	exe, err := resumeChainExe()
	if err != nil {
		return fmt.Errorf("resolve portal executable: %w", err)
	}

	next := cfg.resumeChainPayload
	next.Width, next.Height = width, height
	argv := resumeChainArgv(exe, resumeWaitSubcommand, next)

	// Must stay the statement immediately before the exec: the unbuffered writer
	// puts the marker in the kernel before the process image is replaced.
	cfg.Logger.Info("exec", "target", exe, "args", strings.Join(argv, " "))
	cfg.ExecSelf(exe, argv)
	return nil
}

// paneDrawTheme is the palette one draw paints in, read through the
// non-migrating prefs route: the migrating one performs the one-shot appearance
// translation and writes, and a boot's worth of panes taking it concurrently is
// a write storm over one file for a value none of them is setting.
func paneDrawTheme(colourless bool) theme.Theme {
	nomination := paneDrawNomination(loadPrefsStoreNoMigrate, newThemeLoader())
	// The error is the input drop's alone and this draw performs no drop.
	th, _ := tui.ResolvePaneTheme(nomination, colourless, nil)
	return th
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

		return resumeDrawRunFunc(resumeDrawConfig{
			resumeChainPayload: resumeChainPayload{
				Command: command,
				Report:  report,
				HookKey: hookKey,
				Pane:    pane,
				PaneKey: paneKey,
			},
			Stdout:       os.Stdout,
			Logger:       hydrateLogger,
			Colourless:   noColorEnabled(),
			Size:         paneSizeFromStdin,
			ResolveTheme: paneDrawTheme,
			ExecSelf:     defaultExecShell,
		})
	},
}

func init() {
	stateResumeDrawCmd.Flags().String(resumeFlagCommand, "", "The registered on-resume command the panel states")
	stateResumeDrawCmd.Flags().String(resumeFlagReport, "", "What an answer that could not be carried out reported")
	stateResumeDrawCmd.Flags().String(resumeFlagHookKey, "", "Saved pane token identifying the pane's resume hook")
	stateResumeDrawCmd.Flags().String(resumeFlagPane, "", "The pane id the marker writes address")
	stateResumeDrawCmd.Flags().String(resumeFlagPaneKey, "", "The pane key the chain's records name the pane by")
	// Registered but not read: a draw measures the pane itself, and a flag the
	// waiter hands back that this command did not register would fail its parse.
	stateResumeDrawCmd.Flags().Int(resumeFlagWidth, 0, "Width the previous screen was drawn at")
	stateResumeDrawCmd.Flags().Int(resumeFlagHeight, 0, "Height the previous screen was drawn at")
	_ = stateResumeDrawCmd.MarkFlagRequired(resumeFlagCommand)

	stateCmd.AddCommand(stateResumeDrawCmd)
}
