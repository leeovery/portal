package cmd

import (
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

type resumeRecoverConfig struct {
	Session string
	Stdout  io.Writer
	Logger  *slog.Logger

	// Pending reports whether the session's resume-pending flag is raised;
	// ClearPending lowers it.
	Pending          func() (bool, error)
	ClearPending     func() error
	EnableTTYSignals func() error
	ExecShell        func(prog string, args []string)
}

// runResumeRecover is the tail of the chain a waiting session parks: it runs
// once the waiter above it has ended, and gives the session a shell so it does
// not end, and with it the whole transcript it restored.
//
// A session whose flag is lowered was answered and has already exec'd its own
// shell, so the tail does nothing at all: a second shell there would make a
// restored session take two exits to close. A flag that could not be looked
// for counts as still raised, since a live session carrying an extra shell is
// the lesser failure against one that closes under the user.
func runResumeRecover(cfg resumeRecoverConfig) error {
	cfg.Logger = hydrateLoggerOrDefault(cfg.Logger)

	if raised, err := cfg.Pending(); err == nil && !raised {
		return nil
	}

	// The session leaves the panel's screen before its protection drops: a
	// tick landing while the card is still up saves that card over the
	// session's own last screenful.
	_, _ = io.WriteString(cfg.Stdout, hydrateResetPreamble)

	// The shell runs whether or not the clear landed: a session whose
	// transcript stands still is the lesser failure against one that closed.
	// Nothing is left to hold the answer back, so the record is what makes a
	// session left flagged findable.
	if err := cfg.ClearPending(); err != nil {
		warnResumePendingClearFailed(cfg.Logger, cfg.Session, err)
	}

	enableTTYSignalsOrLog(cfg.Logger, cfg.EnableTTYSignals, cfg.Session)

	handOffToHookOrShell(cfg.Logger, cfg.ExecShell, "")
	return nil
}

// warnResumePendingClearFailed records a flag clear that did not land,
// whichever answer or tail attempted it, under the one wording a search for a
// session left flagged greps for.
func warnResumePendingClearFailed(logger *slog.Logger, session string, err error) {
	logger.Warn("clear resume pending flag failed", resumeLogAttrs(session, "error", err)...)
}

var resumeRecoverRunFunc = runResumeRecover

// stateResumeRecoverCmd gives a session back to its user when the waiter that
// held it ended without handing it over.
var stateResumeRecoverCmd = &cobra.Command{
	Use:    resumeRecoverSubcommand,
	Short:  "Recover a session whose resume waiter ended without answering (internal)",
	Hidden: true,
	// Chains parked under another build call this subcommand by name, so it
	// tolerates flags and arguments it does not register: a parse it refuses
	// ends the session it exists to keep open.
	Args:               cobra.ArbitraryArgs,
	FParseErrWhitelist: cobra.FParseErrWhitelist{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		session, _ := cmd.Flags().GetString(resumeFlagSession)

		return resumeRecoverRunFunc(resumeRecoverConfig{
			Session:          session,
			Stdout:           os.Stdout,
			Logger:           hydrateLogger,
			Pending:          func() (bool, error) { return resumePendingRaised(session) },
			ClearPending:     func() error { return clearResumePending(session) },
			EnableTTYSignals: cookStdin,
			ExecShell:        defaultExecShell,
		})
	},
}

func init() {
	stateResumeRecoverCmd.Flags().String(resumeFlagSession, "", "The waiting session whose resume-pending flag the tail reads and lowers")

	stateCmd.AddCommand(stateResumeRecoverCmd)
}
