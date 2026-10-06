package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/leeovery/portal/internal/hooks"
)

// The flags the resume chain's subcommands are addressed by. --pane is the pane
// id the marker writes need, --pane-key the positional key restore baked, and
// --hook-key the pane's durable token.
const (
	resumeFlagCommand       = "command"
	resumeFlagReport        = "report"
	resumeFlagHookKey       = "hook-key"
	resumeFlagPane          = "pane"
	resumeFlagPaneKey       = "pane-key"
	resumeFlagWidth         = "width"
	resumeFlagHeight        = "height"
	resumeFlagScreen        = "screen"
	resumeFlagDropInput     = "drop-input"
	resumeFlagInputArriving = "input-arriving"
)

// The screens one draw of the waiting pane can put up. The panel is the chain's
// default and names no flag; any value the selector does not recognise draws
// the panel.
const (
	resumeScreenPanel   = ""
	resumeScreenDiscard = "discard"
)

const (
	resumeDrawSubcommand    = "resume-draw"
	resumeWaitSubcommand    = "resume-wait"
	resumeRecoverSubcommand = "resume-recover"
)

// resumeChainPayload is everything one screen of the waiting pane needs, carried
// forward across each hand-off so no later process re-reads the store to decide
// whether to draw.
type resumeChainPayload struct {
	Command string
	Report  string
	HookKey string
	Pane    string
	PaneKey string
	Width   int
	Height  int
	Screen  string

	// DropInput is set by the hand-off that opens the confirmation and
	// cleared by the draw that honours it, so a later draw of the same screen
	// never discards a key the user typed while reading it.
	DropInput bool

	// InputArriving tells a waiter its pane was handed over while input was
	// still arriving, so what arrives first belongs to that input and answers
	// nothing.
	InputArriving bool
}

func (p resumeChainPayload) paneRef() resumePaneRef {
	return resumePaneRef{HookKey: p.HookKey, PaneKey: p.PaneKey}
}

// resumePaneRef is how the chain's records name a waiting pane. The pane key is
// the address restore baked, which a rename, a renumber or a moved pane leaves
// naming another pane or none over a wait; the hook key is the token the pane
// keeps through all of those.
type resumePaneRef struct {
	HookKey string
	PaneKey string
}

func (r resumePaneRef) logAttrs(attrs ...any) []any {
	return append([]any{"hook_key", r.HookKey, "pane_key", r.PaneKey}, attrs...)
}

func resumeChainArgv(exe, subcommand string, p resumeChainPayload) []string {
	argv := []string{exe, "state", subcommand}
	if subcommand == resumeRecoverSubcommand {
		return append(argv,
			flagArg(resumeFlagHookKey), p.HookKey,
			flagArg(resumeFlagPane), p.Pane,
			flagArg(resumeFlagPaneKey), p.PaneKey,
		)
	}

	argv = append(argv, flagArg(resumeFlagCommand), p.Command)
	if p.Report != "" {
		argv = append(argv, flagArg(resumeFlagReport), p.Report)
	}
	argv = append(argv,
		flagArg(resumeFlagHookKey), p.HookKey,
		flagArg(resumeFlagPane), p.Pane,
		flagArg(resumeFlagPaneKey), p.PaneKey,
	)
	if p.Width > 0 {
		argv = append(argv, flagArg(resumeFlagWidth), strconv.Itoa(p.Width))
	}
	if p.Height > 0 {
		argv = append(argv, flagArg(resumeFlagHeight), strconv.Itoa(p.Height))
	}
	if p.Screen == resumeScreenDiscard {
		argv = append(argv, flagArg(resumeFlagScreen), resumeScreenDiscard)
	}
	if p.DropInput {
		argv = append(argv, flagArg(resumeFlagDropInput))
	}
	if p.InputArriving {
		argv = append(argv, flagArg(resumeFlagInputArriving))
	}
	return argv
}

func flagArg(name string) string {
	return "--" + name
}

var osExecutable = os.Executable

// An empty path is an error: exec'ing it would replace the process image with
// nothing. Both failures carry the whole clause, so no consumer wraps them
// again and a pane's stderr names the resolution once.
func resumeChainExe() (string, error) {
	exe, err := osExecutable()
	if err != nil {
		return "", fmt.Errorf("resolve portal executable: %w", err)
	}
	if exe == "" {
		return "", errors.New("resolve portal executable: empty path")
	}
	return exe, nil
}

// resumeHandOff replaces this process's image with the chain's next
// subcommand. The exec marker must stay the statement immediately before the
// exec: the writer behind it is unbuffered, so it reaches the kernel before the
// image is replaced, and a marker emitted after the exec is never written at
// all. The trailing return is unreachable in production.
func resumeHandOff(logger *slog.Logger, execSelf func(prog string, args []string), subcommand string, p resumeChainPayload) error {
	exe, err := resumeChainExe()
	if err != nil {
		return err
	}
	argv := resumeChainArgv(exe, subcommand, p)

	logger.Info("exec", "target", exe, "args", strings.Join(argv, " "))
	execSelf(exe, argv)
	return nil
}

// execHandOff is the same hand-off under the same ordering rule, for the
// sites whose marker also records whether the pane carried a registered
// command.
func execHandOff(logger *slog.Logger, execShell func(prog string, args []string), prog string, args []string, hookPresent bool) {
	logger.Info("exec", "target", prog, "args", strings.Join(args, " "), "hook_present", hookPresent)
	execShell(prog, args)
}

// handOffToHookOrShell gives a pane answered on the panel or recovered by the
// tail the same invocation as one that never waited. An empty command means no
// registration.
func handOffToHookOrShell(logger *slog.Logger, execShell func(prog string, args []string), command string) {
	shell := resolveShell()
	if command == "" {
		execHandOff(logger, execShell, shell, []string{shell}, false)
		return
	}
	prog, args := hookExecArgs(command, shell)
	execHandOff(logger, execShell, prog, args, true)
}

// resumeRegistrationOrLog answers the zero value for a registration carrying no
// command as it does for a miss, so a pane with nothing to run never waits.
func resumeRegistrationOrLog(logger *slog.Logger, lookup func(hookKey string) (hooks.OnResume, error), hookKey string) hooks.OnResume {
	onResume, err := lookup(hookKey)
	if err != nil {
		logger.Debug("hook lookup", "hook_key", hookKey, "result", "error", "error", err)
		logger.Warn("lookup on-resume hook failed", "hook_key", hookKey, "error", err)
		return hooks.OnResume{}
	}
	if !onResume.Found || onResume.Command == "" {
		logger.Debug("hook lookup", "hook_key", hookKey, "result", "miss")
		return hooks.OnResume{}
	}
	logger.Debug("hook lookup", "hook_key", hookKey, "result", "hit")
	return onResume
}

// hookShellTrap keeps the shell running a resume hook alive through a SIGTERM,
// as an interactive shell is, so the pane outlasts a shutdown's SIGTERM until
// tmux itself exits. It must stay a caught trap: an ignored signal stays
// ignored across exec, and the hook and the user's shell would inherit it.
// SIGHUP stays uncaught so a kill still ends the pane.
const hookShellTrap = "trap : TERM; "

// hookExecArgs composes the argv a pane's registered command is run as. The
// command occupies its own argv slot so sh's parser handles any embedded quotes
// - Portal never interpolates it - and the trailing exec leaves the pane on its
// own shell, so it closes on the first exit.
func hookExecArgs(command, shell string) (prog string, args []string) {
	return "/bin/sh", []string{"sh", "-c", hookShellTrap + command + "; exec " + shell}
}

// A pane whose kill keys could not be given back is still handed on: a hook
// that ignores Ctrl-C is the lesser failure against a pane left on its panel.
func enableTTYSignalsOrLog(logger *slog.Logger, enable func() error, pane resumePaneRef) {
	if err := enable(); err != nil {
		logger.Warn("enable terminal signals failed", pane.logAttrs("error", err)...)
	}
}

func enableTTYEchoOrLog(logger *slog.Logger, enable func() error, pane resumePaneRef) {
	if err := enable(); err != nil {
		logger.Warn("enable terminal echo failed", pane.logAttrs("error", err)...)
	}
}
