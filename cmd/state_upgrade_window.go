package cmd

import (
	"errors"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/leeovery/portal/internal/shellquote"
	"github.com/spf13/cobra"
)

// Between an upgrade from the tmux version and `portal migrate`, the tmux
// server still runs that version's global hooks and parked resume chains, and
// they run whatever binary its path now names: this one. This file answers
// them so nothing they run shows in a pane or ends one. Rolled back, the same
// path names the tmux version again and its own commands carry on.

// stateUpgradeHookCmds are the state subcommands the tmux version's global
// hooks run. Each accepts anything and does nothing: a help screen on stdout
// would show over the pane on every focus change, layout change and attach.
var stateUpgradeHookCmds = upgradeHookCommands("notify", "commit-now", "signal-hydrate")

func upgradeHookCommands(names ...string) []*cobra.Command {
	cmds := make([]*cobra.Command, len(names))
	for i, name := range names {
		cmds[i] = &cobra.Command{
			Use:                name,
			Hidden:             true,
			Args:               cobra.ArbitraryArgs,
			DisableFlagParsing: true,
			RunE:               func(*cobra.Command, []string) error { return nil },
		}
	}
	return cmds
}

func init() {
	stateCmd.AddCommand(stateUpgradeHookCmds...)

	holdTmuxForm(stateResumeDrawCmd, tmuxChainFlags...)
	holdTmuxForm(stateResumeWaitCmd, tmuxChainFlags...)
	holdTmuxForm(stateResumeRecoverCmd, tmuxChainFlags...)
	holdTmuxForm(stateHydrateCmd, tmuxHydrateFlags...)
}

// tmuxChainFlags are the flags only the tmux version's resume chain passes,
// to every one of its subcommands: the pane's token, its pane id and its
// address. tmuxHydrateFlags are those only its hydrate helper passes, the FIFO
// it waits on always.
var (
	tmuxChainFlags   = []string{"hook-key", "pane", "pane-key"}
	tmuxHydrateFlags = []string{"fifo", "hook-key"}
)

// holdTmuxForm registers flags on cmd, hidden, and holds an invocation that
// sets any of them before cmd's required flags are checked or its body runs.
// The tmux form's other flags are cmd's own, meaning the same.
func holdTmuxForm(cmd *cobra.Command, flags ...string) {
	for _, name := range flags {
		cmd.Flags().String(name, "", "")
		_ = cmd.Flags().MarkHidden(name)
	}
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if !slices.ContainsFunc(flags, cmd.Flags().Changed) {
			return nil
		}
		upgradeHoldFunc(upgradeHold{
			Argv:    os.Args,
			Signals: upgradeHoldSignals,
			Exec:    execUpgradeHold,
		})
		return errUpgradeHoldEnded
	}
}

// errUpgradeHoldEnded ends a hold that returned, which only a test's does:
// the body must never run on the tmux form.
var errUpgradeHoldEnded = errors.New("a held invocation of the earlier version ended")

var upgradeHoldFunc = holdUntilResized

// upgradeHold is one held invocation of the tmux version's form: its whole
// argv, what installs the signals it handles, and the exec that replaces it,
// which returns only on failure.
type upgradeHold struct {
	Argv    []string
	Signals func() <-chan os.Signal
	Exec    func(prog string, argv []string) error
}

// upgradeHoldSignals catches SIGTERM, as the tmux version's panel does, so a
// shutdown saves the pane still parked: it must stay a catch, never an
// ignored disposition, which would survive the exec into the tmux version's
// chain. SIGHUP is left at its default, so a kill still ends the pane.
func upgradeHoldSignals() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGWINCH)
	return ch
}

func execUpgradeHold(prog string, argv []string) error {
	return syscall.Exec(prog, argv, os.Environ())
}

// holdUntilResized keeps a tmux-version pane parked with its hook unrun,
// drawing nothing: on each resize it execs its own executable with the
// identical argv, which after a rollback is the tmux version's panel drawing
// again, and here holds again. A failed exec holds on.
func holdUntilResized(h upgradeHold) {
	for sig := range h.Signals() {
		if sig != syscall.SIGWINCH {
			continue
		}
		exe, err := resumeChainExe()
		if err != nil {
			hydrateLogger.Debug("exec failed", "error", err)
			continue
		}
		hydrateLogger.Debug("exec", "target", exe, "args", shellquote.Readable(h.Argv))
		err = h.Exec(exe, h.Argv)
		hydrateLogger.Debug("exec failed", "target", exe, "error", err)
	}
}
