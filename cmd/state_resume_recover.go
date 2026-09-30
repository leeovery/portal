package cmd

import (
	"io"
	"log/slog"
	"os"

	"github.com/leeovery/portal/internal/state"
	"github.com/leeovery/portal/internal/tmux"
	"github.com/spf13/cobra"
)

type resumeRecoverConfig struct {
	HookKey string
	Pane    string
	PaneKey string
	Stdout  io.Writer
	Logger  *slog.Logger

	ReadMarker       func() (string, error)
	ClearMarker      func() error
	EnableTTYSignals func() error
	ExecShell        func(prog string, args []string)
	AltScreen        altScreenPin
}

// runResumeRecover is the tail of the chain a waiting pane parks: it runs once
// the waiter above it has ended, and gives the pane a shell so tmux does not
// close it — and with it the session and the whole transcript of a pane that is
// the only one in it.
//
// A pane no longer carrying the pending marker was answered and has already
// exec'd its own shell, so the tail does nothing at all: a second shell there
// would make a restored pane take two exits to close. A marker that could not be
// read counts as still pending, since a live pane carrying an extra shell is the
// lesser failure against one that closes under the user.
func runResumeRecover(cfg resumeRecoverConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	value, err := cfg.ReadMarker()
	if err == nil && !state.ResumePendingSet(value) {
		return nil
	}

	// The pane leaves the panel's screen before its protection drops: a tick
	// landing while the card is still up saves that card over the pane's own
	// last screenful.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// The shell runs whether or not the clear landed: a pane whose transcript
	// stands still is the lesser failure against one that closed. Nothing is
	// left to hold the answer back, so the record is what makes a wrongly-frozen
	// pane findable.
	pane := resumePaneRef{HookKey: cfg.HookKey, PaneKey: cfg.PaneKey}
	if err := cfg.ClearMarker(); err != nil {
		warnResumePendingClearFailed(cfg.Logger, pane, err)
	}
	releaseAltScreenPin(cfg.Logger, pane, cfg.AltScreen)

	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, pane)

	handOffToHookOrShell(cfg.Logger, cfg.ExecShell, "")
	return nil
}

// warnResumePendingClearFailed records a pending-marker clear that did not land,
// whichever answer or tail attempted it, under the one wording a search for a
// wrongly-frozen pane greps for.
func warnResumePendingClearFailed(logger *slog.Logger, pane resumePaneRef, err error) {
	logger.Warn("unset resume pending marker failed", pane.logAttrs("error", err)...)
}

var resumeRecoverRunFunc = runResumeRecover

// stateResumeRecoverCmd gives a pane back to its user when the waiter that held
// it ended without handing it over.
var stateResumeRecoverCmd = &cobra.Command{
	Use:    resumeRecoverSubcommand,
	Short:  "Recover a pane whose resume waiter ended without answering (internal)",
	Hidden: true,
	// Chains parked under another build call this subcommand by name, so it
	// tolerates flags and arguments it does not register: a parse it refuses
	// closes the pane it exists to keep open.
	Args:               cobra.ArbitraryArgs,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		pane, _ := cmd.Flags().GetString(resumeFlagPane)
		if pane == "" {
			pane = os.Getenv("TMUX_PANE")
		}
		paneKey, _ := cmd.Flags().GetString(resumeFlagPaneKey)
		hookKey, _ := cmd.Flags().GetString(resumeFlagHookKey)
		target := tmux.PaneIDTarget(pane)

		return resumeRecoverRunFunc(resumeRecoverConfig{
			HookKey: hookKey,
			Pane:    pane,
			PaneKey: paneKey,
			Stdout:  os.Stdout,
			Logger:  hydrateLogger,
			ReadMarker: func() (string, error) {
				return tmux.DefaultClient().ReadPaneOption(target, state.ResumePendingOption)
			},
			ClearMarker: func() error {
				return state.UnsetResumePendingMarker(tmux.DefaultClient(), target)
			},
			EnableTTYSignals: cookStdin,
			ExecShell:        defaultExecShell,
			AltScreen:        paneAltScreenPin(tmux.DefaultClient(), target),
		})
	},
}

func init() {
	stateResumeRecoverCmd.Flags().String(resumeFlagPane, "", "The pane id the marker reads and writes address (default $TMUX_PANE)")
	stateResumeRecoverCmd.Flags().String(resumeFlagPaneKey, "", "The pane key the chain's records name the pane by")
	stateResumeRecoverCmd.Flags().String(resumeFlagHookKey, "", "Saved pane token the chain's records also name the pane by")

	stateCmd.AddCommand(stateResumeRecoverCmd)
}
